package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// failingLatestProvider wraps fakeProvider and fails Latest for one
// id, so an export can exercise the per-entry failure path.
type failingLatestProvider struct {
	fakeProvider
	failID string
}

func (f *failingLatestProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	if addon.ID == f.failID {
		return nil, errors.New("network down")
	}
	return f.fakeProvider.Latest(ctx, addon)
}

// snapshotTestCatalog builds a catalog with two fake providers wired
// to the given addons.
func snapshotTestCatalog(ghAddons, cfAddons []*Addon) *Catalog {
	return &Catalog{providers: map[string]Provider{
		ProviderGitHub:     &fakeProvider{name: ProviderGitHub, addons: ghAddons},
		ProviderCurseForge: &fakeProvider{name: ProviderCurseForge, addons: cfAddons},
	}}
}

func TestExportSnapshot(t *testing.T) {
	cat := snapshotTestCatalog(
		[]*Addon{
			{Provider: ProviderGitHub, ID: "Vendethiel/Questie", LatestVersion: "1.13.0", GameVersion: "wrath"},
			{Provider: ProviderGitHub, ID: "acme/atlas", LatestVersion: "3.0.0", GameVersion: "wrath"},
		},
		[]*Addon{{Provider: ProviderCurseForge, ID: "456", LatestVersion: "1.2.0", GameVersion: "retail"}},
	)
	reg := &Registry{}
	_ = reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "1.12.2",
		Provider: ProviderGitHub, ID: "Vendethiel/Questie", Source: "Vendethiel/Questie"})
	_ = reg.Track(Entry{Folder: "Atlas", Title: "Atlas", Version: "2.0.0", Pinned: true,
		Provider: ProviderGitHub, ID: "acme/atlas", Source: "acme/atlas"})
	_ = reg.Track(Entry{Folder: "DBM", Title: "DBM", Version: "1.0.0", Ignored: true,
		Provider: ProviderCurseForge, ID: "456", Source: "456"})

	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	snap, err := ExportSnapshot(context.Background(), cat, reg, "wrath", now)
	if err != nil {
		t.Fatalf("ExportSnapshot: %v", err)
	}
	if snap.Version != SnapshotVersion {
		t.Errorf("version = %d, want %d", snap.Version, SnapshotVersion)
	}
	if !snap.ExportedAt.Equal(now) {
		t.Errorf("exported_at = %v, want %v", snap.ExportedAt, now)
	}
	if snap.Profile != "wrath" {
		t.Errorf("profile = %q, want wrath", snap.Profile)
	}
	if len(snap.Addons) != 3 {
		t.Fatalf("addons = %d, want 3: %+v", len(snap.Addons), snap.Addons)
	}
	if len(snap.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", snap.Warnings)
	}

	byFolder := map[string]SnapshotAddon{}
	for _, a := range snap.Addons {
		byFolder[a.Folder] = a
	}
	q := byFolder["Questie"]
	if q.Provider != ProviderGitHub || q.ID != "Vendethiel/Questie" || q.Source != "Vendethiel/Questie" ||
		q.InstalledVersion != "1.12.2" || q.LatestVersion != "1.13.0" || q.GameVersion != "wrath" {
		t.Errorf("Questie snapshot addon = %+v", q)
	}
	if q.Pinned || q.Ignored {
		t.Errorf("Questie flags should be false: %+v", q)
	}
	if a := byFolder["Atlas"]; !a.Pinned || a.LatestVersion != "3.0.0" {
		t.Errorf("pinned Atlas = %+v, want Pinned with resolved latest", a)
	}
	if a := byFolder["DBM"]; !a.Ignored || a.LatestVersion != "1.2.0" || a.GameVersion != "retail" {
		t.Errorf("ignored DBM = %+v, want Ignored with resolved latest", a)
	}
}

func TestExportSnapshotPerEntryFailure(t *testing.T) {
	good := &fakeProvider{name: ProviderGitHub, addons: []*Addon{
		{Provider: ProviderGitHub, ID: "acme/good", LatestVersion: "2.0.0"},
	}}
	bad := &failingLatestProvider{
		fakeProvider: fakeProvider{name: ProviderCurseForge, addons: []*Addon{
			{Provider: ProviderCurseForge, ID: "123", LatestVersion: "9.0.0"},
		}},
		failID: "123",
	}
	cat := &Catalog{providers: map[string]Provider{ProviderGitHub: good, ProviderCurseForge: bad}}
	reg := &Registry{}
	_ = reg.Track(Entry{Folder: "Good", Version: "1.0.0", Provider: ProviderGitHub, ID: "acme/good"})
	_ = reg.Track(Entry{Folder: "Bad", Version: "1.0.0", Provider: ProviderCurseForge, ID: "123"})
	_ = reg.Track(Entry{Folder: "Unknown", Version: "1.0.0", Provider: "nosuch", ID: "x"})

	snap, err := ExportSnapshot(context.Background(), cat, reg, "wrath", time.Now())
	if err != nil {
		t.Fatalf("ExportSnapshot: %v", err)
	}
	if len(snap.Addons) != 3 {
		t.Fatalf("addons = %d, want all 3 despite failures: %+v", len(snap.Addons), snap.Addons)
	}
	if len(snap.Warnings) != 2 {
		t.Fatalf("warnings = %v, want 2 (Bad lookup + Unknown provider)", snap.Warnings)
	}
	byFolder := map[string]SnapshotAddon{}
	for _, a := range snap.Addons {
		byFolder[a.Folder] = a
	}
	if a := byFolder["Good"]; a.LatestVersion != "2.0.0" {
		t.Errorf("Good latest = %q, want 2.0.0", a.LatestVersion)
	}
	if a := byFolder["Bad"]; a.LatestVersion != "" {
		t.Errorf("Bad latest = %q, want empty on resolution failure", a.LatestVersion)
	}
	if a := byFolder["Unknown"]; a.LatestVersion != "" {
		t.Errorf("Unknown latest = %q, want empty for unknown provider", a.LatestVersion)
	}
}

func TestExportSnapshotEmptyRegistry(t *testing.T) {
	snap, err := ExportSnapshot(context.Background(), snapshotTestCatalog(nil, nil), &Registry{}, "wrath", time.Now())
	if err != nil {
		t.Fatalf("ExportSnapshot on empty registry: %v", err)
	}
	if len(snap.Addons) != 0 || len(snap.Warnings) != 0 {
		t.Errorf("expected empty snapshot, got %d addons / %d warnings", len(snap.Addons), len(snap.Warnings))
	}
}

func TestSnapshotSaveLoadRoundTrip(t *testing.T) {
	snap := &Snapshot{
		Version:    SnapshotVersion,
		ExportedAt: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		Profile:    "wrath",
		Addons: []SnapshotAddon{{
			Folder: "Questie", Provider: ProviderGitHub, ID: "Vendethiel/Questie",
			Source: "Vendethiel/Questie", InstalledVersion: "1.12.2",
			LatestVersion: "1.13.0", GameVersion: "wrath", Checksum: "abc123",
			Pinned: true, Ignored: false,
		}},
		Warnings: []string{"Broken: network down"},
	}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := snap.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if got.Version != snap.Version || !got.ExportedAt.Equal(snap.ExportedAt) || got.Profile != snap.Profile {
		t.Errorf("header mismatch: %+v", got)
	}
	if len(got.Addons) != 1 {
		t.Fatalf("addons = %d, want 1", len(got.Addons))
	}
	a := got.Addons[0]
	if a.Folder != "Questie" || a.InstalledVersion != "1.12.2" || a.LatestVersion != "1.13.0" ||
		a.Checksum != "abc123" || !a.Pinned || a.Ignored {
		t.Errorf("addon round-trip mismatch: %+v", a)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "Broken: network down" {
		t.Errorf("warnings = %v", got.Warnings)
	}
}

func TestSnapshotLoadErrors(t *testing.T) {
	if _, err := LoadSnapshot(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("missing file should error")
	}
	if _, err := UnmarshalSnapshot([]byte("{not json")); err == nil {
		t.Error("corrupt JSON should error")
	}
	if _, err := UnmarshalSnapshot([]byte(`{"version": 99, "addons": []}`)); err == nil {
		t.Error("unknown version should error")
	}
}

func TestSnapshotDiff(t *testing.T) {
	snap := &Snapshot{
		Version: SnapshotVersion,
		Profile: "wrath",
		Addons: []SnapshotAddon{
			{Folder: "Questie", Provider: ProviderGitHub, ID: "Vendethiel/Questie",
				LatestVersion: "1.13.0", GameVersion: "wrath"},
			{Folder: "Atlas", Provider: ProviderGitHub, ID: "acme/atlas",
				LatestVersion: "3.0.0", GameVersion: "retail"}, // flavor mismatch vs wrath
			{Folder: "DBM", Provider: ProviderCurseForge, ID: "456",
				LatestVersion: "1.0.0"},
			{Folder: "Older", Provider: ProviderGitHub, ID: "acme/older",
				LatestVersion: "1.10.0"},
			{Folder: "BranchTip", Provider: ProviderGitHub, ID: "acme/branch",
				LatestVersion: "main@HEAD"},
			{Folder: "NoLatest", Provider: ProviderGitHub, ID: "acme/none"},
			{Folder: "Gone", Provider: ProviderGitHub, ID: "acme/gone",
				LatestVersion: "9.0.0"}, // not in the registry: no opinion
		},
	}
	reg := &Registry{}
	_ = reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "1.12.2",
		Provider: ProviderGitHub, ID: "Vendethiel/Questie"})
	_ = reg.Track(Entry{Folder: "Atlas", Title: "Atlas", Version: "2.0.0",
		Provider: ProviderGitHub, ID: "acme/atlas"})
	_ = reg.Track(Entry{Folder: "DBM", Title: "DBM", Version: "1.0.0",
		Provider: ProviderCurseForge, ID: "456"}) // installed == latest: no update
	_ = reg.Track(Entry{Folder: "Older", Title: "Older", Version: "1.12.2",
		Provider: ProviderGitHub, ID: "acme/older"}) // latest older than installed: no update
	_ = reg.Track(Entry{Folder: "BranchTip", Title: "BranchTip", Version: "1.2.3",
		Provider: ProviderGitHub, ID: "acme/branch"}) // branch tip always reports newer
	_ = reg.Track(Entry{Folder: "NoLatest", Title: "NoLatest", Version: "1.0.0",
		Provider: ProviderGitHub, ID: "acme/none"})
	_ = reg.Track(Entry{Folder: "Pinned", Title: "Pinned", Version: "1.0.0", Pinned: true,
		Provider: ProviderGitHub, ID: "acme/pinned"})
	_ = reg.Track(Entry{Folder: "Ignored", Title: "Ignored", Version: "1.0.0", Ignored: true,
		Provider: ProviderGitHub, ID: "acme/ignored"})

	updates, err := snap.Diff(reg)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(updates) != 3 {
		t.Fatalf("updates = %d, want 3 (Questie, Atlas, BranchTip): %+v", len(updates), updates)
	}
	// Sorted by title (Latest.Name is empty): Atlas, BranchTip, Questie.
	want := []string{"Atlas", "BranchTip", "Questie"}
	for i, w := range want {
		if updates[i].Entry.Title != w {
			t.Errorf("updates[%d].Entry.Title = %q, want %q", i, updates[i].Entry.Title, w)
		}
	}
	q := updates[2]
	if q.Entry.Version != "1.12.2" || q.Latest.LatestVersion != "1.13.0" || q.Latest.GameVersion != "wrath" {
		t.Errorf("Questie update = %+v", q)
	}
	if q.Mismatch {
		t.Errorf("Questie mismatch = true, want false (wrath addon on wrath profile)")
	}
	if a := updates[0]; a.Entry.Folder != "Atlas" || a.Latest.LatestVersion != "3.0.0" {
		t.Errorf("Atlas update = %+v", a)
	} else if !a.Mismatch {
		t.Errorf("Atlas mismatch = false, want true (retail addon on wrath profile)")
	}
	if b := updates[1]; b.Latest.LatestVersion != "main@HEAD" {
		t.Errorf("BranchTip latest = %q, want main@HEAD", b.Latest.LatestVersion)
	}
}

func TestSnapshotDiffSkipsPinnedIgnored(t *testing.T) {
	snap := &Snapshot{
		Version: SnapshotVersion,
		Addons: []SnapshotAddon{
			{Folder: "Pinned", LatestVersion: "9.0.0"},
			{Folder: "Ignored", LatestVersion: "9.0.0"},
			{Folder: "Free", LatestVersion: "9.0.0"},
		},
	}
	reg := &Registry{}
	_ = reg.Track(Entry{Folder: "Pinned", Title: "Pinned", Version: "1.0.0", Pinned: true})
	_ = reg.Track(Entry{Folder: "Ignored", Title: "Ignored", Version: "1.0.0", Ignored: true})
	_ = reg.Track(Entry{Folder: "Free", Title: "Free", Version: "1.0.0"})

	updates, err := snap.Diff(reg)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(updates) != 1 || updates[0].Entry.Folder != "Free" {
		t.Fatalf("updates = %+v, want only Free", updates)
	}
}
