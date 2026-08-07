package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/models"
)

// SnapshotVersion is the on-disk format version of a catalog snapshot.
// Bump it when the shape of Snapshot/SnapshotAddon changes incompatibly.
const SnapshotVersion = 1

// Snapshot is a portable, frozen view of the tracked addons and their
// latest known versions, taken while online. Offline consumers diff it
// against the current registry to answer "is anything newer than what
// I have?" without any network access.
type Snapshot struct {
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Profile    string          `json:"profile"` // profile ID the snapshot was exported under
	Addons     []SnapshotAddon `json:"addons"`
	// Warnings lists per-addon resolution failures encountered during
	// export; those addons carry an empty LatestVersion. Omitted when
	// every addon resolved.
	Warnings []string `json:"warnings,omitempty"`
}

// SnapshotAddon is one tracked addon's state in a snapshot. Pinned and
// Ignored are recorded so an offline consumer can honor the flags even
// without the registry; GameVersion is the latest release's game
// family so flavor mismatches can be reported offline.
type SnapshotAddon struct {
	Folder           string `json:"folder"`
	Provider         string `json:"provider"`
	ID               string `json:"id"`
	Source           string `json:"source"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version"`
	GameVersion      string `json:"game_version"`
	Checksum         string `json:"checksum,omitempty"`
	Pinned           bool   `json:"pinned,omitempty"`
	Ignored          bool   `json:"ignored,omitempty"`
}

// ExportSnapshot resolves the latest version of every tracked addon
// through its provider and freezes the result into a Snapshot. Pinned
// and ignored entries are included with their flags; their latest is
// still resolved so offline consumers can decide from the flags.
// Per-entry resolution failures never abort the export: the entry is
// recorded with LatestVersion "" and a note is appended to Warnings.
// An empty registry exports an empty snapshot with a nil error.
func ExportSnapshot(ctx context.Context, cat *Catalog, reg *Registry, profileID string, now time.Time) (*Snapshot, error) {
	if cat == nil {
		return nil, fmt.Errorf("catalog is nil")
	}
	if reg == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	snap := &Snapshot{
		Version:    SnapshotVersion,
		ExportedAt: now.UTC(),
		Profile:    profileID,
		Addons:     make([]SnapshotAddon, 0, len(reg.Entries())),
	}
	var warnings []string
	for _, e := range reg.Entries() {
		sa := SnapshotAddon{
			Folder:           e.Folder,
			Provider:         e.Provider,
			ID:               e.ID,
			Source:           e.Source,
			InstalledVersion: e.Version,
			Checksum:         e.Checksum,
			Pinned:           e.Pinned,
			Ignored:          e.Ignored,
		}
		prov, ok := cat.Provider(e.Provider)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: provider %q is not available", e.Folder, e.Provider))
			snap.Addons = append(snap.Addons, sa)
			continue
		}
		latest, err := prov.Latest(ctx, &Addon{Provider: e.Provider, ID: e.ID})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", e.Folder, err))
			snap.Addons = append(snap.Addons, sa)
			continue
		}
		if latest != nil {
			sa.LatestVersion = latest.LatestVersion
			sa.GameVersion = latest.GameVersion
		}
		snap.Addons = append(snap.Addons, sa)
	}
	snap.Warnings = warnings
	return snap, nil
}

// Marshal renders the snapshot as indented JSON.
func (s *Snapshot) Marshal() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// UnmarshalSnapshot parses snapshot JSON, rejecting unknown versions
// and malformed input with a clear error.
func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("snapshot is corrupt: %w", err)
	}
	if s.Version != SnapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version %d (expected %d)", s.Version, SnapshotVersion)
	}
	return &s, nil
}

// Save writes the snapshot atomically (temp file + rename), mirroring
// Registry.saveLocked including the Windows retry after removing an
// existing destination.
func (s *Snapshot) Save(path string) error {
	data, err := s.Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cannot create snapshot directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			_ = os.Remove(path)
			if err := os.Rename(tmp, path); err == nil {
				return nil
			}
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("save snapshot %q: %w", path, err)
	}
	return nil
}

// LoadSnapshot reads and parses a snapshot file. A missing file and a
// corrupt one are both errors.
func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot %q: %w", path, err)
	}
	return UnmarshalSnapshot(data)
}

// Diff compares the snapshot's latest versions against the live
// registry WITHOUT any network access and returns the pending updates,
// sorted by addon name exactly like Check. For each snapshot addon
// whose folder matches a registry entry (case-insensitive) with a
// non-empty LatestVersion, an update is reported when Compare decides
// the latest is newer than the entry's current version (semver when
// both parse, case-insensitive string comparison otherwise — the same
// rule Check uses, so GitHub branch-tip "main@HEAD" entries always
// report newer). Pinned and ignored entries are skipped, mirroring the
// updater. Entries in the registry but not in the snapshot yield no
// opinion, and so do entries with an empty current version (no
// baseline to compare against) or a snapshot addon with an empty
// latest. Mismatch is computed against the profile recorded in the
// snapshot.
func (s *Snapshot) Diff(reg *Registry) ([]Update, error) {
	if reg == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	byFolder := make(map[string]Entry, len(reg.Entries()))
	for _, e := range reg.Entries() {
		byFolder[strings.ToLower(e.Folder)] = e
	}
	profile := models.ProfileByID(s.Profile)
	var updates []Update
	for _, sa := range s.Addons {
		e, ok := byFolder[strings.ToLower(sa.Folder)]
		if !ok {
			continue // not tracked anymore: no opinion
		}
		if e.Pinned || e.Ignored {
			continue // mirror the updater skip
		}
		if strings.TrimSpace(sa.LatestVersion) == "" || strings.TrimSpace(e.Version) == "" {
			continue // no latest or no baseline to compare against
		}
		if Compare(sa.LatestVersion, e.Version) > 0 {
			updates = append(updates, Update{
				Entry: e,
				Latest: &Addon{
					LatestVersion: sa.LatestVersion,
					GameVersion:   sa.GameVersion,
				},
				Mismatch: gameFamilyMismatch(profile, sa.GameVersion),
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
	return updates, nil
}
