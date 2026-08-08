package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// curseLegacyBase is the deprecated public CurseForge endpoint.
	// It still answers anonymous requests but is no longer
	// maintained; it is only used when no API key is configured.
	curseLegacyBase = "https://addons-ecs.forgesvc.net/api/v2"
	// curseModernBase is the current CurseForge Core API. It requires
	// an API key sent as the "x-api-key" header.
	curseModernBase = "https://api.curseforge.com/v1"
)

// ErrCurseForgeUnavailable marks a CurseForge failure the user can
// act on: no API key while the legacy endpoint is failing, or a
// rejected key on the modern API. It is wrapped with context ("no API
// key", "API key rejected", "legacy API unreachable") and is surfaced
// by Catalog.Search alongside results from the other providers.
var ErrCurseForgeUnavailable = errors.New("CurseForge unavailable")

func curseUnavailable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCurseForgeUnavailable, fmt.Sprintf(format, args...))
}

// curseforgeProvider talks to CurseForge through one of two
// endpoints:
//
//   - the modern Core API (api.curseforge.com/v1) when an API key is
//     available (key func returns non-empty);
//   - the deprecated legacy API (addons-ecs.forgesvc.net/api/v2)
//     otherwise.
//
// A keyless provider degrades loudly: every auth or reachability
// failure is wrapped in ErrCurseForgeUnavailable instead of being
// swallowed as an empty result set.
type curseforgeProvider struct {
	client *http.Client
	legacy string // legacy forgesvc base, overridable in tests
	modern string // modern Core API base, overridable in tests
	key    func() string
}

func newCurseForgeProvider(client *http.Client, legacyBase, modernBase string, key func() string) *curseforgeProvider {
	if legacyBase == "" {
		legacyBase = curseLegacyBase
	}
	if modernBase == "" {
		modernBase = curseModernBase
	}
	return &curseforgeProvider{client: client, legacy: legacyBase, modern: modernBase, key: key}
}

func (p *curseforgeProvider) Name() string { return ProviderCurseForge }

// apiKey returns the configured key; empty means the legacy endpoint.
func (p *curseforgeProvider) apiKey() string {
	if p.key == nil {
		return ""
	}
	return p.key()
}

func (p *curseforgeProvider) usingModern() bool { return p.apiKey() != "" }

// ---- legacy (forgesvc v2) payloads ----

type cfAuthor struct {
	Name string `json:"name"`
}

type cfLegacyFile struct {
	DisplayName  string   `json:"displayName"`
	DateReleased string   `json:"dateReleased"`
	GameVersion  []string `json:"gameVersion"`
}

// cfLegacyAddon mirrors the legacy addon object. The API is loosely
// typed (missing fields, time strings without a zone), so all fields
// are kept as plain strings.
type cfLegacyAddon struct {
	ID          int            `json:"id"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	WebsiteURL  string         `json:"websiteUrl"`
	Authors     []cfAuthor     `json:"authors"`
	LatestFiles []cfLegacyFile `json:"latestFiles"`
}

// ---- modern (Core API v1) payloads ----

type cfModFile struct {
	ID           int      `json:"id"`
	DisplayName  string   `json:"displayName"`
	FileDate     string   `json:"fileDate"`
	GameVersions []string `json:"gameVersions"`
}

type cfMod struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Summary     string      `json:"summary"`
	WebsiteURL  string      `json:"websiteUrl"`
	Authors     []cfAuthor  `json:"authors"`
	LatestFiles []cfModFile `json:"latestFiles"`
}

type cfModListResponse struct {
	Data []cfMod `json:"data"`
}

type cfModResponse struct {
	Data cfMod `json:"data"`
}

type cfModFileListResponse struct {
	Data []cfModFile `json:"data"`
}

// cfLegacyFileItem mirrors one file in the legacy addon file list
// (/addon/<id>/files). downloadUrl, when present, is the direct
// archive link for exactly that file.
type cfLegacyFileItem struct {
	ID          int    `json:"id"`
	DisplayName string `json:"displayName"`
	DownloadURL string `json:"downloadUrl"`
}

type cfDownloadURLResponse struct {
	Data string `json:"data"`
}

// ---- shared plumbing ----

// curseKeyHint is appended to keyless failure messages so the user
// knows the modern API is the fix.
const curseKeyHint = "set WOWFIX_CURSEFORGE_API_KEY or `wowfix config set curseforge_api_key`"

// do performs a request against the active endpoint, attaching the
// API key when present, and maps auth/reachability failures to
// ErrCurseForgeUnavailable.
func (p *curseforgeProvider) do(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowfix")
	if key := p.apiKey(); key != "" {
		req.Header.Set("x-api-key", key)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if !p.usingModern() {
			return nil, curseUnavailable("no API key and legacy API unreachable: %v; %s", err, curseKeyHint)
		}
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return data, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		if p.usingModern() {
			return nil, curseUnavailable("API key rejected (%s)", resp.Status)
		}
		return nil, curseUnavailable("no API key and legacy API rejected request (%s); %s", resp.Status, curseKeyHint)
	default:
		if !p.usingModern() {
			// The legacy endpoint is deprecated; any failure is worth
			// surfacing as unavailability rather than empty results.
			return nil, curseUnavailable("no API key and legacy API error (%s); %s", resp.Status, curseKeyHint)
		}
		return nil, &statusError{code: resp.StatusCode, url: u, msg: resp.Status}
	}
}

// activeBase returns the base of the endpoint in use.
func (p *curseforgeProvider) activeBase() string {
	if p.usingModern() {
		return p.modern
	}
	return p.legacy
}

// ---- search ----

// Search queries the active endpoint's addon search. The two APIs
// use different paths (/addon/search legacy, /mods/search modern).
func (p *curseforgeProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	qp := url.Values{}
	qp.Set("gameId", "1") // gameId 1 is World of Warcraft
	qp.Set("searchFilter", query)
	if limit > 0 {
		qp.Set("pageSize", strconv.Itoa(limit))
	}
	path := "/addon/search"
	if p.usingModern() {
		path = "/mods/search"
	}
	body, err := p.do(ctx, p.activeBase()+path+"?"+qp.Encode())
	if err != nil {
		return nil, err
	}
	if p.usingModern() {
		var resp cfModListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("curseforge: parse search response: %w", err)
		}
		return addonsFromMods(resp.Data), nil
	}
	var list []cfLegacyAddon
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("curseforge: parse search response: %w", err)
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	out := make([]*Addon, 0, len(list))
	for i := range list {
		out = append(out, p.addonFromLegacy(&list[i]))
	}
	return out, nil
}

// ---- resolve / latest ----

// Resolve fetches one addon. A numeric id hits the addon endpoint
// directly; a slug (from an install URL) is resolved through search
// because neither API has a slug lookup.
func (p *curseforgeProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	if _, err := strconv.Atoi(id); err == nil {
		return p.resolveID(ctx, id)
	}
	found, err := p.Search(ctx, id, 5)
	if err != nil {
		return nil, err
	}
	for _, a := range found {
		if strings.EqualFold(a.Name, id) {
			return p.resolveID(ctx, a.ID)
		}
	}
	if len(found) > 0 {
		return p.resolveID(ctx, found[0].ID)
	}
	return nil, fmt.Errorf("curseforge: no addon found for %q", id)
}

func (p *curseforgeProvider) resolveID(ctx context.Context, id string) (*Addon, error) {
	path := "/addon/" + id
	if p.usingModern() {
		path = "/mods/" + id
	}
	body, err := p.do(ctx, p.activeBase()+path)
	if err != nil {
		if errors.Is(err, ErrCurseForgeUnavailable) || isNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("curseforge resolve %s: %w", id, err)
	}
	if p.usingModern() {
		var resp cfModResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		return addonFromMod(&resp.Data), nil
	}
	var a cfLegacyAddon
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, err
	}
	return p.addonFromLegacy(&a), nil
}

// Latest refreshes version information from the same addon payload.
func (p *curseforgeProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	if addon == nil || addon.ID == "" {
		return nil, fmt.Errorf("curseforge: missing addon id")
	}
	return p.resolveID(ctx, addon.ID)
}

// ---- download ----

// ResolveVersion resolves the addon at a specific past file: the file
// whose display name matches version. On the modern Core API the file
// id is looked up from the mod's file list so Download hits
// /mods/<id>/files/<fileId>/download-url; on the legacy API the
// file's own downloadUrl is used. A file that no longer exists is an
// error, so rollback never silently serves the latest release.
func (p *curseforgeProvider) ResolveVersion(ctx context.Context, addon *Addon, version, ref string) (*Addon, error) {
	if addon == nil || addon.ID == "" {
		return nil, fmt.Errorf("curseforge: missing addon id")
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, fmt.Errorf("curseforge: no version to resolve for %s", addon.ID)
	}
	out := &Addon{Provider: ProviderCurseForge, ID: addon.ID, Name: addon.Name}
	if p.usingModern() {
		file, err := p.findModernFile(ctx, addon.ID, version)
		if err != nil {
			return nil, err
		}
		out.LatestVersion = file.DisplayName
		out.VersionRef = strconv.Itoa(file.ID)
		out.fileID = file.ID
		return out, nil
	}
	file, err := p.findLegacyFile(ctx, addon.ID, version)
	if err != nil {
		return nil, err
	}
	out.LatestVersion = file.DisplayName
	out.VersionRef = strconv.Itoa(file.ID)
	if file.DownloadURL != "" {
		out.downloadURL = file.DownloadURL
	}
	return out, nil
}

// findModernFile walks the modern files endpoint (50 per page, capped
// at 250 files) for the file whose display name matches version.
func (p *curseforgeProvider) findModernFile(ctx context.Context, id, version string) (*cfModFile, error) {
	for index := 0; index < 250; index += 50 {
		u := fmt.Sprintf("%s/mods/%s/files?index=%d&pageSize=50", p.modern, id, index)
		body, err := p.do(ctx, u)
		if err != nil {
			return nil, err
		}
		var resp cfModFileListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("curseforge: parse file list: %w", err)
		}
		if len(resp.Data) == 0 {
			break
		}
		for i := range resp.Data {
			if resp.Data[i].DisplayName == version {
				return &resp.Data[i], nil
			}
		}
	}
	return nil, fmt.Errorf("curseforge: no file named %q for addon %s", version, id)
}

// findLegacyFile looks the version up in the legacy addon file list.
func (p *curseforgeProvider) findLegacyFile(ctx context.Context, id, version string) (*cfLegacyFileItem, error) {
	body, err := p.do(ctx, p.legacy+"/addon/"+id+"/files")
	if err != nil {
		return nil, err
	}
	var list []cfLegacyFileItem
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("curseforge: parse file list: %w", err)
	}
	for i := range list {
		if list[i].DisplayName == version {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("curseforge: no file named %q for addon %s", version, id)
}

// Download fetches the addon archive. On the modern API the file
// download URL is looked up first (/mods/<id>/files/<fileId>/
// download-url) and then fetched; on the legacy endpoint the addon's
// own download URL is used when set (ResolveVersion pins one),
// otherwise the /download endpoint, which redirects straight to the
// latest zip.
func (p *curseforgeProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	if addon == nil || addon.ID == "" {
		return fmt.Errorf("curseforge: missing addon id")
	}
	if addon.fileID > 0 {
		u := fmt.Sprintf("%s/mods/%s/files/%d/download-url", p.modern, addon.ID, addon.fileID)
		body, err := p.do(ctx, u)
		if err != nil {
			return err
		}
		var resp cfDownloadURLResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("curseforge: parse download-url response: %w", err)
		}
		if resp.Data == "" {
			return fmt.Errorf("curseforge: empty download URL for %s", addon.ID)
		}
		return downloadFile(ctx, p.client, resp.Data, dest, progress)
	}
	if addon.downloadURL != "" {
		// A pinned file URL (from ResolveVersion): fetch exactly that
		// file, never the latest.
		return downloadFile(ctx, p.client, addon.downloadURL, dest, progress)
	}
	err := downloadFile(ctx, p.client, p.legacy+"/addon/"+addon.ID+"/download", dest, progress)
	if err != nil && !p.usingModern() {
		var se *statusError
		if errors.As(err, &se) && se.code == http.StatusNotFound {
			return err // legit "file gone", not an availability problem
		}
		return curseUnavailable("no API key and legacy download failed: %v; %s", err, curseKeyHint)
	}
	return err
}

// ---- mapping ----

func (p *curseforgeProvider) addonFromLegacy(a *cfLegacyAddon) *Addon {
	out := &Addon{
		Provider: ProviderCurseForge,
		ID:       strconv.Itoa(a.ID),
		Name:     a.Name,
		Summary:  a.Summary,
		Homepage: a.WebsiteURL,
	}
	if len(a.Authors) > 0 {
		out.Author = a.Authors[0].Name
	}
	if len(a.LatestFiles) > 0 {
		f := a.LatestFiles[0]
		out.LatestVersion = f.DisplayName
		out.UpdatedAt = parseCFTime(f.DateReleased)
		if len(f.GameVersion) > 0 {
			out.GameVersion = gameFamily(f.GameVersion[0])
		}
	}
	return out
}

func addonsFromMods(mods []cfMod) []*Addon {
	out := make([]*Addon, 0, len(mods))
	for i := range mods {
		out = append(out, addonFromMod(&mods[i]))
	}
	return out
}

func addonFromMod(m *cfMod) *Addon {
	out := &Addon{
		Provider: ProviderCurseForge,
		ID:       strconv.Itoa(m.ID),
		Name:     m.Name,
		Summary:  m.Summary,
		Homepage: m.WebsiteURL,
	}
	if len(m.Authors) > 0 {
		out.Author = m.Authors[0].Name
	}
	if len(m.LatestFiles) > 0 {
		f := m.LatestFiles[0]
		out.LatestVersion = f.DisplayName
		out.UpdatedAt = parseCFTime(f.FileDate)
		if len(f.GameVersions) > 0 {
			out.GameVersion = gameFamily(f.GameVersions[0])
		}
		out.fileID = f.ID
		out.VersionRef = strconv.Itoa(f.ID)
	}
	return out
}

// parseCFTime handles the APIs' loose timestamps (RFC3339 with a
// zone, or local times without one).
func parseCFTime(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
