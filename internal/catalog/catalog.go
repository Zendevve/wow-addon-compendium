// Package catalog implements the addon source layer of wowfix:
// providers for GitHub, CurseForge, WowInterface, Tukui and Wago, a
// merged catalog search, a persisted install registry and an updater.
//
// Network access always goes through the *http.Client injected at
// construction, so every provider can be exercised against an
// httptest.Server in tests. Caveats that every addon manager shares:
// the GitHub REST API is rate-limited to 60 requests/hour per IP for
// unauthenticated clients, and CurseForge's current API requires an
// API key.
//
// CurseForge is served through two endpoints: the modern Core API
// (api.curseforge.com/v1, "x-api-key" header) is used when a key is
// configured — the WOWFIX_CURSEFORGE_API_KEY environment variable
// first, then the Catalog.CurseForgeAPIKey field. Without a key the
// provider falls back to the deprecated legacy endpoint
// (addons-ecs.forgesvc.net/api/v2) and surfaces failures as
// ErrCurseForgeUnavailable so a dead source never looks like an empty
// result set.
package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/validator"
)

// Addon is a catalog entry returned by a provider.
type Addon struct {
	Provider      string // "github" | "curseforge" | "wowinterface" | "tukui"
	ID            string // provider-scoped id ("owner/repo" for github, addon id for curseforge/wowinterface)
	Name          string
	Author        string
	Summary       string
	LatestVersion string
	Homepage      string
	GameVersion   string // best-effort interface family: "vanilla","tbc","wrath","cata","classic","retail",""
	UpdatedAt     time.Time
	// VersionRef is the provider-scoped reference of the resolved
	// version: the GitHub tag name, or the CurseForge file id (as a
	// string). Empty when the provider cannot address the version
	// individually. The registry records it into the addon's version
	// history so a later rollback can re-download exactly this
	// version.
	VersionRef string

	// downloadURL is the provider-resolved archive location, set
	// during Resolve/Latest (and search where derivable). It is
	// unexported because only the provider that produced the addon
	// knows how to fetch it.
	downloadURL string
	// fileID is the CurseForge file id of the latest release, set by
	// the modern Core API provider so Download can use the
	// /files/<id>/download-url endpoint. 0 means "use the legacy
	// download endpoint".
	fileID int
}

// Provider fetches addon metadata and archives from one source.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, limit int) ([]*Addon, error)
	// Resolve fetches one addon by its provider-scoped ID.
	Resolve(ctx context.Context, id string) (*Addon, error)
	// Latest refreshes version information for one addon.
	Latest(ctx context.Context, addon *Addon) (*Addon, error)
	// Download fetches the addon archive to dest (a file path).
	// progress is called with bytes done/total; total may be 0 when
	// the source does not report a length.
	Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error
}

// ErrVersionNotServed marks a rollback that cannot happen because the
// provider only serves the latest version of an addon. Rollback
// surfaces this instead of silently installing the latest release.
var ErrVersionNotServed = errors.New("provider cannot re-download a specific version")

// VersionResolver is implemented by providers that can re-download a
// specific past version of an addon: GitHub (release tags) and
// CurseForge (files by display name / file id). Providers that only
// serve the latest version — WowInterface, Tukui, Wago — do not
// implement it, so a version rollback reports ErrVersionNotServed.
type VersionResolver interface {
	// ResolveVersion resolves addon at the requested version and
	// returns an *Addon whose Download fetches exactly that version.
	// version is the provider's version label (a GitHub tag or a
	// CurseForge file display name); ref, when non-empty, is the
	// provider-scoped reference recorded in the registry history (the
	// tag, or the CurseForge file id). A version that no longer
	// exists is an error: resolution never falls back to a different
	// release.
	ResolveVersion(ctx context.Context, addon *Addon, version, ref string) (*Addon, error)
}

// Provider names.
const (
	ProviderGitHub       = "github"
	ProviderCurseForge   = "curseforge"
	ProviderWowInterface = "wowinterface"
	ProviderTukui        = "tukui"
	ProviderWago         = "wago"
)

// Catalog aggregates the enabled providers and performs installs.
type Catalog struct {
	providers map[string]Provider

	// Reg, when set, makes InstallFromSource and Apply register the
	// installed folders in the registry.
	Reg *Registry
	// Backups, when set, is passed to the installer so replaced
	// folders are snapshotted before being overwritten.
	Backups *backup.Manager
	// Log, when set, is passed to the installer.
	Log *logger.Logger
	// Profile, when set, overrides the installer's default profile.
	Profile *models.Profile
	// CurseForgeAPIKey enables the modern CurseForge Core API
	// (api.curseforge.com/v1). The WOWFIX_CURSEFORGE_API_KEY
	// environment variable takes precedence when set. An empty key
	// routes the provider to the deprecated legacy endpoint.
	CurseForgeAPIKey string
}

// New returns a Catalog with the enabled providers. A provider name
// mapped to true is enabled; absent or false disables it. A nil map
// enables every provider. A nil client uses http.DefaultClient.
func New(enabled map[string]bool, client *http.Client) (*Catalog, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if enabled == nil {
		enabled = map[string]bool{
			ProviderGitHub:       true,
			ProviderCurseForge:   true,
			ProviderWowInterface: true,
			ProviderTukui:        true,
			ProviderWago:         true,
		}
	}
	c := &Catalog{providers: map[string]Provider{}}
	if enabled[ProviderGitHub] {
		c.providers[ProviderGitHub] = newGitHubProvider(client, "", "")
	}
	if enabled[ProviderCurseForge] {
		c.providers[ProviderCurseForge] = newCurseForgeProvider(client, "", "", c.curseForgeKey)
	}
	if enabled[ProviderWowInterface] {
		c.providers[ProviderWowInterface] = newWowInterfaceProvider(client, "")
	}
	if enabled[ProviderTukui] {
		c.providers[ProviderTukui] = newTukuiProvider(client, "")
	}
	if enabled[ProviderWago] {
		// Wago serves WeakAuras/Plater import strings, not addon
		// archives: its API (data.wago.io) is undocumented but stable
		// and keyless, and downloads are import strings for in-game
		// import. Wago-hosted addon archives are not covered.
		c.providers[ProviderWago] = newWagoProvider(client, "")
	}
	return c, nil
}

// Provider returns the named provider and whether it is enabled.
func (c *Catalog) Provider(name string) (Provider, bool) {
	p, ok := c.providers[name]
	return p, ok
}

// curseForgeKey returns the CurseForge API key: the environment
// variable first, then the catalog field. An empty key routes the
// provider to the deprecated legacy endpoint.
func (c *Catalog) curseForgeKey() string {
	if k := strings.TrimSpace(os.Getenv("WOWFIX_CURSEFORGE_API_KEY")); k != "" {
		return k
	}
	return strings.TrimSpace(c.CurseForgeAPIKey)
}

// Search queries every enabled provider with the same limit and
// merges the results, deduped by addon name (first hit wins,
// case-insensitive) and sorted by name. A provider that fails is
// skipped; when every provider fails the joined error is returned,
// otherwise the merged results are returned together with any joined
// provider errors.
func (c *Catalog) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	names := make([]string, 0, len(c.providers))
	for name := range c.providers {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []*Addon
	seen := map[string]bool{}
	var errs []error
	for _, name := range names {
		found, err := c.providers[name].Search(ctx, query, limit)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		for _, a := range found {
			key := strings.ToLower(strings.TrimSpace(a.Name))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li != lj {
			return li < lj
		}
		return out[i].Provider < out[j].Provider
	})
	if len(errs) > 0 {
		if len(out) == 0 {
			return nil, errors.Join(errs...)
		}
		return out, errors.Join(errs...)
	}
	return out, nil
}

var (
	wowInterfaceIDRe = regexp.MustCompile(`(?i)info(\d+)(?:-|\.)`)
	tukuiIDRe        = regexp.MustCompile(`(?i)/downloads?/([^/?#]+)`)
)

// InstallFromSource installs an addon from a URL or provider-scoped
// id and returns the installed folder names. Supported source forms:
//
//	"owner/repo"                                  github
//	"https://github.com/owner/repo"               github
//	"https://www.curseforge.com/wow/addons/slug"  curseforge
//	"https://www.wowinterface.com/downloads/info123-name.html" wowinterface
//	"https://www.tukui.org/downloads/1"           tukui
//
// The archive is downloaded to a temporary file, installed through
// internal/installer and, when c.Reg is set, registered with the
// version read from the installed TOC (falling back to the addon's
// latest version).
func (c *Catalog) InstallFromSource(ctx context.Context, source, addonsDir string, progress func(done, total int64)) ([]string, error) {
	providerName, id, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	prov, ok := c.Provider(providerName)
	if !ok {
		return nil, fmt.Errorf("provider %q is not enabled", providerName)
	}
	addon, err := prov.Resolve(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", source, err)
	}
	return c.installAddon(ctx, prov, addon, addonsDir, source, progress)
}

// ComputeManifest returns a deterministic content digest of every file
// under dir. For each file it hashes the slash-separated path relative
// to dir, a NUL byte and the file's contents with SHA-256; the
// per-file digests are sorted by path and hashed again, then
// hex-encoded. Paths are sorted and no timestamps or directory entries
// participate, so identical trees always yield identical digests
// regardless of filesystem ordering. An empty dir hashes the empty
// input and yields a non-empty digest; a missing dir is an error.
func ComputeManifest(dir string) (string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("manifest %q: %w", dir, err)
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("manifest %q: read %s: %w", dir, rel, err)
		}
		fh := sha256.New()
		fh.Write([]byte(rel))
		fh.Write([]byte{0})
		fh.Write(data)
		h.Write(fh.Sum(nil))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// installAddon downloads, installs and registers one resolved addon.
func (c *Catalog) installAddon(ctx context.Context, prov Provider, addon *Addon, addonsDir, source string, progress func(done, total int64)) ([]string, error) {
	tmp, err := os.CreateTemp("", "wowfix-download-*.zip")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := prov.Download(ctx, addon, tmpPath, progress); err != nil {
		return nil, fmt.Errorf("download %s: %w", addon.Name, err)
	}

	inst := installer.New(installer.Options{
		AddonsDir: addonsDir,
		Profile:   c.Profile,
		Backups:   c.Backups,
		Log:       c.Log,
	})
	res, err := inst.Install(ctx, tmpPath)
	if err != nil {
		return nil, err
	}
	if len(res.Installed) == 0 {
		if len(res.Errors) > 0 {
			return nil, errors.Join(res.Errors...)
		}
		return nil, fmt.Errorf("installer installed nothing")
	}

	if c.Reg != nil {
		for _, folder := range res.Installed {
			version := addon.LatestVersion
			if v := readTOCVersion(filepath.Join(addonsDir, folder)); v != "" {
				version = v
			}
			// Best-effort provenance: a manifest failure (e.g. a
			// folder that vanished between install and registration)
			// leaves Checksum empty rather than failing the install.
			checksum, _ := ComputeManifest(filepath.Join(addonsDir, folder))
			if err := c.Reg.Track(Entry{
				Folder:     folder,
				Title:      addon.Name,
				Version:    version,
				Provider:   addon.Provider,
				ID:         addon.ID,
				Source:     source,
				Checksum:   checksum,
				VersionRef: addon.VersionRef,
			}); err != nil {
				return res.Installed, err
			}
		}
	}
	if len(res.Errors) > 0 {
		return res.Installed, errors.Join(res.Errors...)
	}
	return res.Installed, nil
}

// parseSource classifies an install source string into a provider
// name and a provider-scoped id.
func parseSource(source string) (string, string, error) {
	s := strings.TrimSpace(source)
	if s == "" {
		return "", "", fmt.Errorf("empty install source")
	}
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "github.com"):
		owner, repo, err := githubPath(s)
		if err != nil {
			return "", "", err
		}
		return ProviderGitHub, owner + "/" + repo, nil
	case strings.Contains(lower, "curseforge.com"):
		slug, err := curseSlug(s)
		if err != nil {
			return "", "", err
		}
		return ProviderCurseForge, slug, nil
	case strings.Contains(lower, "wowinterface.com"):
		m := wowInterfaceIDRe.FindStringSubmatch(s)
		if m == nil {
			return "", "", fmt.Errorf("cannot parse WowInterface URL %q (expected .../info<id>-<slug>.html)", source)
		}
		return ProviderWowInterface, m[1], nil
	case strings.Contains(lower, "tukui.org"):
		m := tukuiIDRe.FindStringSubmatch(s)
		if m == nil {
			return "", "", fmt.Errorf("cannot parse Tukui URL %q (expected .../downloads/<id>)", source)
		}
		return ProviderTukui, m[1], nil
	case strings.Contains(s, "/"):
		owner, repo, err := githubPath(s)
		if err != nil {
			return "", "", err
		}
		return ProviderGitHub, owner + "/" + repo, nil
	default:
		return "", "", fmt.Errorf("unknown install source %q", source)
	}
}

// githubPath extracts owner and repo from "owner/repo" or a
// github.com URL path, ignoring any trailing segments.
func githubPath(s string) (owner, repo string, err error) {
	u := s
	if strings.Contains(strings.ToLower(s), "github.com") {
		parsed, err := url.Parse(s)
		if err != nil {
			return "", "", fmt.Errorf("invalid GitHub URL %q: %w", s, err)
		}
		u = parsed.Path
	}
	u = strings.Trim(u, "/")
	parts := strings.Split(u, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("GitHub source %q must be owner/repo", s)
	}
	return parts[0], parts[1], nil
}

// curseSlug extracts the addon slug from a CurseForge URL path such
// as /wow/addons/<slug>.
func curseSlug(s string) (string, error) {
	parsed, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid CurseForge URL %q: %w", s, err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, p := range parts {
		if strings.EqualFold(p, "addons") && i+1 < len(parts) && parts[i+1] != "" {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("cannot parse CurseForge URL %q (expected .../wow/addons/<slug>)", s)
}

// readTOCVersion returns the ## Version value of the addon folder's
// first TOC, or "" when the folder has none.
func readTOCVersion(folder string) string {
	matches, err := filepath.Glob(filepath.Join(folder, "*.toc"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	toc, err := validator.ParseTOC(matches[0])
	if err != nil || toc.Version == "" {
		return ""
	}
	return toc.Version
}
