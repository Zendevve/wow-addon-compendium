// Package service tests the Wails facade end to end against a fake WoW
// tree, entirely under t.TempDir(): no real config, no real game folder.
package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wowfix/wowfix/internal/config"
)

// writeFixture recreates the testdata/wow fixture layout in a temp
// AddOns directory: the same folders, TOC names and versions as
// internal/e2e's writeFixture.
func writeFixture(t *testing.T, addonsDir string) {
	t.Helper()
	writeTOC := func(relDir, tocName, body string) {
		t.Helper()
		dir := filepath.Join(addonsDir, relDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, tocName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeTOC("AtlasLoot", "AtlasLoot.toc", "## Interface: 30300\n## Title: AtlasLoot\n## Version: 7.0.4\n")
	writeTOC("Questie-main", "Questie.toc", "## Interface: 30300\n## Title: Questie\n## Version: 1.12.2\n")
	writeTOC(filepath.Join("DPSMate-main", "DPSMate"), "DPSMate.toc", "## Interface: 30300\n## Title: DPSMate\n## Version: 1.0\n")
	writeTOC("AuxUI", "Aux-Classic.toc", "## Interface: 30300\n## Title: Aux-Classic\n## Version: 1.0\n")
	writeTOC("Questie", "Questie.toc", "## Interface: 30300\n## Title: Questie\n## Version: 1.12.2\n")

	if err := os.MkdirAll(filepath.Join(addonsDir, "Inventory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonsDir, "Inventory", "Inventory.lua"), []byte("local x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(addonsDir, "TempFolder"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeZip creates a zip archive with the given relative paths.
func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// newTestService wires a Service to a fresh config in a temp dir and a
// fake game root with the fixture tree. It returns the service and the
// AddOns path.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	addonsDir := filepath.Join(root, "Interface", "AddOns")
	writeFixture(t, addonsDir)

	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	cfg := config.Default()
	cfg.WoWPath = root
	cfg.Flavor = ""
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return New(store), addonsDir
}

func findIssue(addons []Addon, folder, kind string) bool {
	for _, a := range addons {
		if a.FolderName != folder {
			continue
		}
		for _, i := range a.Issues {
			if i.Kind == kind {
				return true
			}
		}
	}
	return false
}

// TestScanFindsProblems scans the fixture and expects every seeded
// problem to surface in the DTO.
func TestScanFindsProblems(t *testing.T) {
	s, _ := newTestService(t)
	res, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if res.Stats.Total < 5 {
		t.Fatalf("stats.total = %d, want >= 5", res.Stats.Total)
	}
	if res.Stats.Problems < 4 {
		t.Fatalf("stats.problems = %d, want >= 4", res.Stats.Problems)
	}

	for _, want := range []struct{ folder, kind string }{
		{"Questie-main", "github-name"},
		{"AuxUI", "toc-mismatch"},
		{"DPSMate", "nested"},
		{"Inventory", "missing-toc"},
		{"TempFolder", "empty"},
	} {
		if !findIssue(res.Addons, want.folder, want.kind) {
			t.Errorf("issue %q on %q not found", want.kind, want.folder)
		}
	}
}

// TestFixAllRepairs fixes the fixture without destructive confirmations
// and expects the safe renames and the flatten to be applied on disk.
func TestFixAllRepairs(t *testing.T) {
	s, addonsDir := newTestService(t)
	if _, err := s.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	batch, err := s.FixAll(false)
	if err != nil {
		t.Fatalf("FixAll failed: %v", err)
	}
	if len(batch.Fixes) == 0 {
		t.Fatal("FixAll produced no results")
	}
	for _, f := range batch.Fixes {
		if f.Error != "" {
			t.Errorf("fix %s %s errored: %s", f.Addon, f.Action, f.Error)
		}
	}
	if batch.Fixed < 2 {
		t.Errorf("fixed = %d, want >= 2", batch.Fixed)
	}

	// Safe rename applied: AuxUI -> Aux-Classic.
	for _, name := range []string{"Questie", "Aux-Classic", "DPSMate"} {
		if _, err := os.Stat(filepath.Join(addonsDir, name)); err != nil {
			t.Errorf("addon %q missing after fix: %v", name, err)
		}
	}
	// Nested DPSMate promoted to top level, wrapper gone.
	if _, err := os.Stat(filepath.Join(addonsDir, "DPSMate-main")); !os.IsNotExist(err) {
		t.Errorf("wrapper DPSMate-main should be gone after flatten, stat err = %v", err)
	}
}

// TestValidateTable checks the compatibility table: one row per addon,
// expected interface from the profile, detected values from the TOCs.
func TestValidateTable(t *testing.T) {
	s, _ := newTestService(t)
	vr, err := s.Validate()
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if vr.ProfileID != "wrath" {
		t.Errorf("profile_id = %q, want wrath (default)", vr.ProfileID)
	}
	if vr.Expected != 30300 {
		t.Errorf("expected = %d, want 30300 (wrath)", vr.Expected)
	}
	if len(vr.Addons) != 7 {
		t.Fatalf("rows = %d, want one per fixture addon (7)", len(vr.Addons))
	}

	byName := map[string]Compat{}
	for _, c := range vr.Addons {
		if c.Expected != 30300 {
			t.Errorf("row %q expected = %d, want 30300", c.FolderName, c.Expected)
		}
		byName[c.FolderName] = c
	}
	if c := byName["AtlasLoot"]; c.Detected != 30300 || c.Status != "compatible" {
		t.Errorf("AtlasLoot row = %+v, want detected 30300 / compatible", c)
	}
	if c := byName["Inventory"]; c.TOC != "" || c.Detected != -1 || c.Status != "unknown" {
		t.Errorf("Inventory row = %+v, want empty toc / detected -1 / unknown", c)
	}
}

// TestInstallZip installs an archive built in memory and checks the
// folder lands on disk and is reported as installed.
func TestInstallZip(t *testing.T) {
	s, addonsDir := newTestService(t)
	zipPath := filepath.Join(t.TempDir(), "newaddon.zip")
	writeZip(t, zipPath, map[string]string{
		"NewAddon/NewAddon.toc": "## Interface: 30300\n## Title: NewAddon\n## Version: 1.0\n",
	})

	res, err := s.InstallZip(zipPath, true)
	if err != nil {
		t.Fatalf("InstallZip failed: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("install errors: %v", res.Errors)
	}
	if !slices.Contains(res.Installed, "NewAddon") {
		t.Errorf("installed = %v, want NewAddon", res.Installed)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "NewAddon")); err != nil {
		t.Errorf("NewAddon folder missing after install: %v", err)
	}
}
