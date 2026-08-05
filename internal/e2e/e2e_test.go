// Package e2e proves the whole repair pipeline works together against a
// fake WoW AddOns tree, entirely under t.TempDir(): scan -> fix ->
// backup/restore -> install. No network, no real game installation.
package e2e

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/fixer"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/scanner"
)

// TestRepairPipeline drives scanner, fixer, backup and installer over
// one fake AddOns directory and checks the on-disk state after every
// stage.
func TestRepairPipeline(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "Interface", "AddOns")
	writeFixture(t, addonsDir)
	ctx := context.Background()
	profile := models.ProfileByID("wrath")
	if profile == nil {
		t.Fatal("wrath profile not found")
	}

	// Stage 1: scan the fixture and expect the known problems.
	res, err := scanner.New(addonsDir, profile).Scan(ctx)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if got := len(res.Addons); got != 7 {
		t.Fatalf("scan found %d addons, want 7", got)
	}
	problems, errAddons := 0, 0
	var inventory *models.Addon
	for _, a := range res.Addons {
		if len(a.Issues) > 0 {
			problems++
		}
		if a.Status == models.StatusError {
			errAddons++
		}
		if a.FolderName == "Inventory" {
			inventory = a
		}
	}
	if errAddons < 1 {
		t.Fatalf("expected at least one addon with StatusError, got %d", errAddons)
	}
	if inventory == nil || inventory.Status != models.StatusError {
		t.Fatalf("Inventory should be StatusError (missing TOC), got %+v", inventory)
	}
	if problems < 2 {
		t.Fatalf("expected at least two addons with issues, got %d", problems)
	}

	// Stage 2: fix everything, then rescan for a clean tree.
	fixerResults := fixer.New(fixer.Options{
		AddonsDir:        addonsDir,
		Profile:          profile,
		Backups:          backup.New(filepath.Join(root, "Backups"), nil),
		Confirm:          func(string, ...any) bool { return true },
		TrashFallbackDir: filepath.Join(root, "trash"),
	}).FixAll(ctx, res.Addons)
	for _, r := range fixerResults {
		if !r.OK || r.Err != nil {
			t.Fatalf("fixer result not OK: %s", r.String())
		}
	}

	res2, err := scanner.New(addonsDir, profile).Scan(ctx)
	if err != nil {
		t.Fatalf("rescan failed: %v", err)
	}
	if got := len(res2.Addons); got != 4 {
		t.Fatalf("expected 4 addons after fix, got %d", got)
	}
	remaining := map[string]bool{}
	for _, a := range res2.Addons {
		remaining[a.FolderName] = true
		if len(a.Issues) > 0 {
			t.Fatalf("addon %q still has issues: %v", a.FolderName, a.Issues)
		}
	}
	for _, want := range []string{"AtlasLoot", "Aux-Classic", "DPSMate", "Questie"} {
		if !remaining[want] {
			t.Fatalf("addon %q missing after fix; remaining: %v", want, remaining)
		}
	}

	// Stage 3: snapshot the clean tree, corrupt it, restore it.
	mgr := backup.New(filepath.Join(root, "Backups"), nil)
	snapDir, err := mgr.BackupDir(addonsDir, "e2e")
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapDir, "manifest.json")); err != nil {
		t.Fatalf("backup manifest missing: %v", err)
	}

	// Corrupt the tree: junk inside an addon plus a deleted addon.
	if err := os.MkdirAll(filepath.Join(addonsDir, "AtlasLoot", "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(addonsDir, "Questie")); err != nil {
		t.Fatal(err)
	}

	restored, skipped, err := mgr.Restore(filepath.Base(snapDir), func(string) bool { return true })
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("restore skipped destinations: %v", skipped)
	}
	if len(restored) != 4 {
		t.Fatalf("expected 4 folders restored, got %d", len(restored))
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "AtlasLoot", "junk")); err == nil {
		t.Fatal("junk folder survived restore")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat junk folder: %v", err)
	}
	for _, name := range []string{"AtlasLoot", "Aux-Classic", "DPSMate", "Questie"} {
		if _, err := os.Stat(filepath.Join(addonsDir, name)); err != nil {
			t.Fatalf("addon %q not restored: %v", name, err)
		}
	}

	// Stage 4: install a zip with a nested and a flat addon.
	zipPath := filepath.Join(root, "addons.zip")
	writeZip(t, zipPath, map[string]string{
		"DeadlyBossMods-main/DBM-Core/DBM-Core.toc": "## Interface: 30300\n## Title: DBM-Core\n## Version: 10.2.7\n",
		"WeakAuras/WeakAuras.toc":                   "## Interface: 30300\n## Title: WeakAuras\n## Version: 5.12\n",
	})
	instRes, err := installer.New(installer.Options{
		AddonsDir: addonsDir,
		Profile:   profile,
		Confirm:   func(string) bool { return true },
	}).Install(ctx, zipPath)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if len(instRes.Errors) != 0 {
		t.Fatalf("install reported errors: %v", instRes.Errors)
	}
	if want := []string{"DBM-Core", "WeakAuras"}; !slices.Equal(instRes.Installed, want) {
		t.Fatalf("installed %v, want %v", instRes.Installed, want)
	}
	for _, name := range []string{"DBM-Core", "WeakAuras"} {
		if _, err := os.Stat(filepath.Join(addonsDir, name)); err != nil {
			t.Fatalf("installed addon %q missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "DeadlyBossMods-main")); err == nil {
		t.Fatal("wrapper folder leaked into AddOns")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat wrapper folder: %v", err)
	}
}

// writeFixture recreates the testdata/wow fixture layout in a temp
// AddOns directory: the same folders, TOC names and versions.
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
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
