package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
)

// Update is one addon with a newer version available.
type Update struct {
	Entry  Entry
	Latest *Addon
	// Mismatch reports that the latest release targets a different
	// game-version family than the configured profile. Check still
	// reports the update; callers decide whether to skip it.
	Mismatch bool
}

// knownGameFamilies maps the family names an addon or profile can
// carry onto the canonical client family. Classic Era, Hardcore, SoD
// and TurtleWoW are all vanilla-family clients.
var knownGameFamilies = map[string]string{
	"vanilla":  "vanilla",
	"tbc":      "tbc",
	"wrath":    "wrath",
	"cata":     "cata",
	"retail":   "retail",
	"classic":  "vanilla",
	"hardcore": "vanilla",
	"sod":      "vanilla",
	"turtle":   "vanilla",
}

// normalizeGameFamily reduces a provider game-version string to a
// canonical client family name. Family names are mapped directly;
// numeric version strings ("3.3.5") fall back to gameFamily. Unknown
// or empty values map to "".
func normalizeGameFamily(v string) string {
	if f, ok := knownGameFamilies[strings.ToLower(strings.TrimSpace(v))]; ok {
		return f
	}
	return gameFamily(v)
}

// gameFamilyMismatch reports whether an addon's game-version family
// differs from the configured profile's family. An empty profile or an
// unrecognized addon family means "no opinion" and returns false.
func gameFamilyMismatch(profile *models.Profile, gameVersion string) bool {
	if profile == nil || strings.TrimSpace(gameVersion) == "" {
		return false
	}
	got := normalizeGameFamily(gameVersion)
	if got == "" {
		return false
	}
	return got != profile.Family
}

// Check compares every registry entry against its provider's latest
// version and returns the updates, sorted by addon name. Entries
// whose provider is not enabled, whose version cannot be determined,
// or whose latest version is unknown are skipped. A source that only
// tracks a branch tip (GitHub "main@HEAD") always reports newer,
// because the branch may have moved. When providers fail, the joined
// error is returned alongside any updates that were found.
func Check(ctx context.Context, catalog *Catalog, reg *Registry, addonsDir string) ([]Update, error) {
	if catalog == nil {
		return nil, fmt.Errorf("catalog is nil")
	}
	if reg == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	var updates []Update
	var errs []error
	for _, e := range reg.Entries() {
		if e.Provider == "" || e.ID == "" {
			continue // not catalog-managed
		}
		prov, ok := catalog.Provider(e.Provider)
		if !ok {
			continue // provider disabled or unknown
		}
		current := e.Version
		if strings.TrimSpace(current) == "" {
			// Fall back to the installed TOC, e.g. for entries
			// registered before version tracking existed.
			current = readTOCVersion(filepath.Join(addonsDir, e.Folder))
		}
		if strings.TrimSpace(current) == "" {
			continue // no baseline to compare against
		}
		latest, err := prov.Latest(ctx, &Addon{Provider: e.Provider, ID: e.ID})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Folder, err))
			continue
		}
		if strings.TrimSpace(latest.LatestVersion) == "" {
			continue
		}
		if Compare(latest.LatestVersion, current) > 0 {
			updates = append(updates, Update{
				Entry:    e,
				Latest:   latest,
				Mismatch: gameFamilyMismatch(catalog.Profile, latest.GameVersion),
			})
		}
	}
	sort.Slice(updates, func(i, j int) bool {
		ni, nj := updates[i].Latest.Name, updates[j].Latest.Name
		if ni == "" {
			ni = updates[i].Entry.Title
		}
		if nj == "" {
			nj = updates[j].Entry.Title
		}
		li, lj := strings.ToLower(ni), strings.ToLower(nj)
		if li != lj {
			return li < lj
		}
		return strings.ToLower(updates[i].Entry.Folder) < strings.ToLower(updates[j].Entry.Folder)
	})
	if len(errs) > 0 {
		return updates, errors.Join(errs...)
	}
	return updates, nil
}

// Apply downloads the latest version of one update, installs it into
// installDir through internal/installer (with the given backups and
// log) and refreshes the registry entry via catalog.Reg. It returns
// the installed folder name.
func Apply(ctx context.Context, catalog *Catalog, installDir string, u Update, backups *backup.Manager, log *logger.Logger) (string, error) {
	if catalog == nil {
		return "", fmt.Errorf("catalog is nil")
	}
	prov, ok := catalog.Provider(u.Entry.Provider)
	if !ok {
		return "", fmt.Errorf("provider %q is not enabled", u.Entry.Provider)
	}
	latest := u.Latest
	if latest == nil {
		latest = &Addon{Provider: u.Entry.Provider, ID: u.Entry.ID}
	}

	tmp, err := os.CreateTemp("", "wowfix-update-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := prov.Download(ctx, latest, tmpPath, nil); err != nil {
		return "", fmt.Errorf("download %s: %w", u.Entry.Folder, err)
	}

	inst := installer.New(installer.Options{
		AddonsDir: installDir,
		Profile:   catalog.Profile,
		Backups:   backups,
		Log:       log,
	})
	res, err := inst.Install(ctx, tmpPath)
	if err != nil {
		return "", err
	}
	if len(res.Installed) == 0 {
		if len(res.Errors) > 0 {
			return "", errors.Join(res.Errors...)
		}
		return "", fmt.Errorf("installer installed nothing for %s", u.Entry.Folder)
	}
	folder := pickInstalled(res.Installed, u.Entry.Folder)

	if catalog.Reg != nil {
		version := latest.LatestVersion
		if v := readTOCVersion(filepath.Join(installDir, folder)); v != "" {
			version = v
		}
		title := latest.Name
		if title == "" {
			title = u.Entry.Title
		}
		// Best-effort provenance, same rule as installs: a manifest
		// failure records no checksum instead of failing the update.
		checksum, _ := ComputeManifest(filepath.Join(installDir, folder))
		if err := catalog.Reg.Track(Entry{
			Folder:   folder,
			Title:    title,
			Version:  version,
			Provider: latest.Provider,
			ID:       latest.ID,
			Source:   u.Entry.Source,
			Checksum: checksum,
		}); err != nil {
			return folder, err
		}
	}
	if len(res.Errors) > 0 {
		return folder, errors.Join(res.Errors...)
	}
	return folder, nil
}

// pickInstalled prefers the installed folder matching the tracked one
// (case-insensitively) and otherwise returns the first install.
func pickInstalled(installed []string, want string) string {
	for _, f := range installed {
		if strings.EqualFold(f, want) {
			return f
		}
	}
	return installed[0]
}
