package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/logger"
)

// versionResolvingProvider serves two distinct archives per version
// and implements VersionResolver, like GitHub/CurseForge.
type versionResolvingProvider struct {
	name    string
	zips    map[string][]byte // version -> archive bytes
	missing map[string]bool   // versions that no longer exist
}

func (p *versionResolvingProvider) Name() string { return p.name }

func (p *versionResolvingProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	return nil, nil
}

func (p *versionResolvingProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	return &Addon{Provider: p.name, ID: id, Name: id}, nil
}

func (p *versionResolvingProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	return &Addon{Provider: p.name, ID: addon.ID, Name: addon.Name, LatestVersion: "2.0.0", VersionRef: "2.0.0"}, nil
}

func (p *versionResolvingProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	data, ok := p.zips[addon.LatestVersion]
	if !ok {
		return errors.New("no archive for " + addon.LatestVersion)
	}
	return os.WriteFile(dest, data, 0o644)
}

func (p *versionResolvingProvider) ResolveVersion(ctx context.Context, addon *Addon, version, ref string) (*Addon, error) {
	if p.missing[version] {
		return nil, errors.New("version gone")
	}
	return &Addon{Provider: p.name, ID: addon.ID, Name: addon.Name, LatestVersion: version, VersionRef: "ref-" + version}, nil
}

// latestOnlyProvider never implements VersionResolver, like
// WowInterface/Tukui.
type latestOnlyProvider struct {
	versionedProvider
}

func TestRollbackToVersionInstallsTracksAndBacksUp(t *testing.T) {
	v1 := addonZip(t, "Questie", "## Title: Questie\n## Version: 9.0.0\n## Interface: 30300\n")
	v2 := addonZip(t, "Questie", "## Title: Questie\n## Version: 9.2.0\n## Interface: 30300\n")
	prov := &versionResolvingProvider{name: "testprov", zips: map[string][]byte{"9.0.0": v1, "9.2.0": v2}}

	root := t.TempDir()
	addonsDir := filepath.Join(root, "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(filepath.Join(root, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{providers: map[string]Provider{"testprov": prov}, Reg: reg}
	backups := backup.New(filepath.Join(root, "Backups"), nil)
	log := logger.New(10)

	// Seed the current version through two update steps so the history
	// log records both 9.0.0 and 9.2.0.
	first := Update{
		Entry:  Entry{Folder: "Questie", Title: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie", Source: "acme/questie"},
		Latest: &Addon{Provider: "testprov", ID: "Questie", Name: "Questie", LatestVersion: "9.0.0", VersionRef: "ref-9.0.0"},
	}
	if _, err := Apply(context.Background(), c, addonsDir, first, backups, log); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second := Update{
		Entry:  Entry{Folder: "Questie", Title: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie", Source: "acme/questie"},
		Latest: &Addon{Provider: "testprov", ID: "Questie", Name: "Questie", LatestVersion: "9.2.0", VersionRef: "ref-9.2.0"},
	}
	if _, err := Apply(context.Background(), c, addonsDir, second, backups, log); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if got := readTOCVersion(filepath.Join(addonsDir, "Questie")); got != "9.2.0" {
		t.Fatalf("seeded version = %q, want 9.2.0", got)
	}

	// Roll back to the previously installed 9.0.0.
	entry := reg.Entries()[0]
	hist := entry.History[1] // history: [9.2.0, 9.0.0]
	folder, err := RollbackToVersion(context.Background(), c, addonsDir, entry, hist, backups, log)
	if err != nil {
		t.Fatalf("RollbackToVersion: %v", err)
	}
	if folder != "Questie" {
		t.Errorf("folder = %q, want Questie", folder)
	}
	if got := readTOCVersion(filepath.Join(addonsDir, "Questie")); got != "9.0.0" {
		t.Errorf("rolled back version = %q, want 9.0.0", got)
	}

	// The replaced folder was snapshotted (the safety path).
	snapshots, err := backups.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) < 2 {
		t.Errorf("expected a pre-replace snapshot of the rollback, got %d snapshots", len(snapshots))
	}

	// The registry re-records the rolled-back version with a fresh
	// history entry and the resolved provider ref.
	entries := reg.Entries()
	if len(entries) != 1 || entries[0].Version != "9.0.0" {
		t.Fatalf("registry after rollback = %+v", entries)
	}
	if entries[0].Provider != "testprov" || entries[0].ID != "Questie" || entries[0].Source != "acme/questie" {
		t.Errorf("entry provider fields not preserved: %+v", entries[0])
	}
	h := entries[0].History
	if len(h) != 3 || h[0].Version != "9.0.0" {
		t.Fatalf("history after rollback = %+v, want [9.0.0, 9.2.0, 9.0.0]", h)
	}
	if h[0].Ref != "ref-9.0.0" {
		t.Errorf("history ref = %q, want the resolved ref", h[0].Ref)
	}
}

func TestRollbackToVersionNotServed(t *testing.T) {
	prov := &latestOnlyProvider{versionedProvider: versionedProvider{name: "testprov"}}
	c := &Catalog{providers: map[string]Provider{"testprov": prov}}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(Entry{Folder: "Questie", Version: "9.2.0", Provider: "testprov", ID: "Questie"})
	_ = reg.Track(Entry{Folder: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie"})
	entry := reg.Entries()[0] // history: [9.0.0 current, 9.2.0]

	// Rolling back to the previously installed 9.2.0.
	_, err = RollbackToVersion(context.Background(), c, t.TempDir(), entry, entry.History[1], nil, nil)
	if err == nil {
		t.Fatal("RollbackToVersion on a latest-only provider should error")
	}
	if !errors.Is(err, ErrVersionNotServed) {
		t.Errorf("error should wrap ErrVersionNotServed, got %v", err)
	}
	// The honest message names the provider and version.
	if !strings.Contains(err.Error(), "testprov") || !strings.Contains(err.Error(), "9.2.0") {
		t.Errorf("error = %q, want provider and version named", err.Error())
	}
}

func TestRollbackToVersionMissingVersion(t *testing.T) {
	prov := &versionResolvingProvider{name: "testprov", zips: map[string][]byte{}, missing: map[string]bool{"9.2.0": true}}
	c := &Catalog{providers: map[string]Provider{"testprov": prov}}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(Entry{Folder: "Questie", Version: "9.2.0", Provider: "testprov", ID: "Questie"})
	_ = reg.Track(Entry{Folder: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie"})
	entry := reg.Entries()[0]

	_, err = RollbackToVersion(context.Background(), c, t.TempDir(), entry, entry.History[1], nil, nil)
	if err == nil {
		t.Fatal("RollbackToVersion with a gone version should error")
	}
	if !strings.Contains(err.Error(), "resolve version") {
		t.Errorf("error = %q, want resolve failure surfaced", err.Error())
	}
}

func TestRollbackToVersionUnknownProvider(t *testing.T) {
	c := &Catalog{providers: map[string]Provider{}}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(Entry{Folder: "Questie", Version: "9.0.0", Provider: "nope", ID: "Questie"})
	entry := reg.Entries()[0]
	hist := VersionHistory{Version: "9.0.0"}

	if _, err := RollbackToVersion(context.Background(), c, t.TempDir(), entry, hist, nil, nil); err == nil {
		t.Fatal("RollbackToVersion with an unknown provider should error")
	}
}
