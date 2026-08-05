package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/logger"
)

// addonZip builds a valid addon archive with one folder carrying a
// single TOC file.
func addonZip(t *testing.T, folder, tocContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(folder + "/" + folder + ".toc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(tocContent)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// versionedProvider reports a fixed latest version per addon id.
type versionedProvider struct {
	name     string
	versions map[string]string
}

func (p *versionedProvider) Name() string { return p.name }

func (p *versionedProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	return nil, nil
}

func (p *versionedProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	return &Addon{Provider: p.name, ID: id, Name: id, LatestVersion: p.versions[id]}, nil
}

func (p *versionedProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	return &Addon{Provider: p.name, ID: addon.ID, Name: addon.ID, LatestVersion: p.versions[addon.ID]}, nil
}

func (p *versionedProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	return errors.New("unused in this test")
}

// zipDownloadProvider serves a real archive for downloads.
type zipDownloadProvider struct {
	name    string
	version string
	zipData []byte
}

func (p *zipDownloadProvider) Name() string { return p.name }

func (p *zipDownloadProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	return nil, nil
}

func (p *zipDownloadProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	return &Addon{Provider: p.name, ID: id, Name: id, LatestVersion: p.version}, nil
}

func (p *zipDownloadProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	return &Addon{Provider: p.name, ID: addon.ID, Name: addon.Name, LatestVersion: p.version}, nil
}

func (p *zipDownloadProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	return os.WriteFile(dest, p.zipData, 0o644)
}

// errLatestProvider fails every lookup.
type errLatestProvider struct {
	name string
	err  error
}

func (p *errLatestProvider) Name() string { return p.name }

func (p *errLatestProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	return nil, p.err
}

func (p *errLatestProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	return nil, p.err
}

func (p *errLatestProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	return nil, p.err
}

func (p *errLatestProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	return p.err
}

func TestCheckFindsUpdates(t *testing.T) {
	vp := &versionedProvider{name: "testprov", versions: map[string]string{
		"alpha": "1.0.0", // entry 1.0.0-beta -> newer
		"beta":  "2.0.0", // entry 2.0.0 -> same
		"gamma": "1.1.0", // entry 0.9.0 -> newer
		"delta": "",      // latest unknown -> skip
	}}
	c := &Catalog{providers: map[string]Provider{"testprov": vp}}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(Entry{Folder: "Alpha", Title: "Alpha", Version: "1.0.0-beta", Provider: "testprov", ID: "alpha"})
	_ = reg.Track(Entry{Folder: "Beta", Title: "Beta", Version: "2.0.0", Provider: "testprov", ID: "beta"})
	_ = reg.Track(Entry{Folder: "Gamma", Title: "Gamma", Version: "0.9.0", Provider: "testprov", ID: "gamma"})
	_ = reg.Track(Entry{Folder: "Delta", Title: "Delta", Version: "1.0.0", Provider: "testprov", ID: "delta"})
	_ = reg.Track(Entry{Folder: "Epsilon", Title: "Epsilon", Version: "1.0.0", Provider: "not-enabled", ID: "x"})
	_ = reg.Track(Entry{Folder: "Zeta", Title: "Zeta", Version: "", Provider: "testprov", ID: "zeta"})

	updates, err := Check(context.Background(), c, reg, "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("got %d updates, want 2 (Alpha, Gamma): %+v", len(updates), updates)
	}
	if updates[0].Entry.Folder != "Alpha" || updates[1].Entry.Folder != "Gamma" {
		t.Errorf("update order = %s, %s; want Alpha, Gamma (sorted by name)",
			updates[0].Entry.Folder, updates[1].Entry.Folder)
	}
	if updates[0].Latest.LatestVersion != "1.0.0" {
		t.Errorf("Alpha latest = %q, want 1.0.0", updates[0].Latest.LatestVersion)
	}
}

func TestCheckPropagatesProviderError(t *testing.T) {
	ep := &errLatestProvider{name: "testprov", err: errors.New("api down")}
	c := &Catalog{providers: map[string]Provider{"testprov": ep}}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(Entry{Folder: "A", Version: "1.0.0", Provider: "testprov", ID: "a"})

	updates, err := Check(context.Background(), c, reg, "")
	if err == nil {
		t.Fatal("Check should propagate the provider error")
	}
	if len(updates) != 0 {
		t.Errorf("updates = %d, want 0", len(updates))
	}
}

func TestApplyInstallsAndTracks(t *testing.T) {
	zipData := addonZip(t, "Questie", "## Title: Questie\n## Version: 9.2.0\n## Interface: 30300\n")
	prov := &zipDownloadProvider{name: "testprov", version: "9.2.0", zipData: zipData}

	root := t.TempDir()
	addonsDir := filepath.Join(root, "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(filepath.Join(root, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{
		providers: map[string]Provider{"testprov": prov},
		Reg:       reg,
	}
	backups := backup.New(filepath.Join(root, "Backups"), nil)
	log := logger.New(10)

	u := Update{
		Entry:  Entry{Folder: "Questie", Title: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie", Source: "https://github.com/x/questie"},
		Latest: &Addon{Provider: "testprov", ID: "Questie", Name: "Questie", LatestVersion: "9.2.0"},
	}
	folder, err := Apply(context.Background(), c, addonsDir, u, backups, log)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if folder != "Questie" {
		t.Errorf("returned folder = %q, want Questie", folder)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "Questie", "Questie.toc")); err != nil {
		t.Fatalf("installed addon missing: %v", err)
	}

	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("registry entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Version != "9.2.0" {
		t.Errorf("tracked version = %q, want 9.2.0 (from TOC)", e.Version)
	}
	if e.Provider != "testprov" || e.ID != "Questie" || e.Source != "https://github.com/x/questie" {
		t.Errorf("entry = %+v", e)
	}
}

func TestApplyReplacesAndBacksUp(t *testing.T) {
	v1 := addonZip(t, "Questie", "## Title: Questie\n## Version: 9.0.0\n## Interface: 30300\n")
	v2 := addonZip(t, "Questie", "## Title: Questie\n## Version: 9.2.0\n## Interface: 30300\n")
	prov := &zipDownloadProvider{name: "testprov", version: "9.2.0", zipData: v2}

	root := t.TempDir()
	addonsDir := filepath.Join(root, "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backups := backup.New(filepath.Join(root, "Backups"), nil)
	reg, err := NewRegistry(filepath.Join(root, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{providers: map[string]Provider{"testprov": prov}, Reg: reg}
	log := logger.New(10)

	// Seed the old version through the same Apply path.
	first := Update{
		Entry:  Entry{Folder: "Questie", Title: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie", Source: "seed"},
		Latest: &Addon{Provider: "testprov", ID: "Questie", Name: "Questie", LatestVersion: "9.0.0"},
	}
	prov.zipData = v1
	if _, err := Apply(context.Background(), c, addonsDir, first, backups, log); err != nil {
		t.Fatalf("seed apply: %v", err)
	}
	if got := readTOCVersion(filepath.Join(addonsDir, "Questie")); got != "9.0.0" {
		t.Fatalf("seeded version = %q", got)
	}

	// Second apply replaces the folder and snapshots it first.
	prov.zipData = v2
	second := Update{
		Entry:  Entry{Folder: "Questie", Title: "Questie", Version: "9.0.0", Provider: "testprov", ID: "Questie", Source: "seed"},
		Latest: &Addon{Provider: "testprov", ID: "Questie", Name: "Questie", LatestVersion: "9.2.0"},
	}
	if _, err := Apply(context.Background(), c, addonsDir, second, backups, log); err != nil {
		t.Fatalf("update apply: %v", err)
	}
	if got := readTOCVersion(filepath.Join(addonsDir, "Questie")); got != "9.2.0" {
		t.Errorf("updated version = %q, want 9.2.0", got)
	}
	snapshots, err := backups.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) == 0 {
		t.Error("expected a backup snapshot of the replaced folder")
	}
	entries := reg.Entries()
	if len(entries) != 1 || entries[0].Version != "9.2.0" {
		t.Errorf("registry after update = %+v", entries)
	}
}

func TestApplyUnknownProvider(t *testing.T) {
	c := &Catalog{providers: map[string]Provider{}}
	u := Update{Entry: Entry{Folder: "X", Provider: "nope", ID: "x"}}
	if _, err := Apply(context.Background(), c, t.TempDir(), u, nil, nil); err == nil {
		t.Fatal("Apply with unknown provider should error")
	}
}

func TestPickInstalled(t *testing.T) {
	if got := pickInstalled([]string{"A", "B"}, "b"); got != "B" {
		t.Errorf("case-insensitive match failed: %q", got)
	}
	if got := pickInstalled([]string{"A", "B"}, "Z"); got != "A" {
		t.Errorf("fallback to first failed: %q", got)
	}
}
