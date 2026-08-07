package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedFolder writes a TOC with the given version into dir.
func seedFolder(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Addon.toc"), []byte("## Title: Addon\n## Version: "+version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tocVersion reads the version line back from dir's TOC.
func tocVersion(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "Addon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "## Version: "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func TestRollbackFolderRestores(t *testing.T) {
	root := t.TempDir()
	m := New(filepath.Join(root, "Backups"), nil)
	dir := filepath.Join(root, "Interface", "AddOns", "Questie")
	seedFolder(t, dir, "1.0.0")

	if _, err := m.Backup([]string{dir}, "seed"); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Modify the folder after the snapshot: new version, extra file.
	seedFolder(t, dir, "2.0.0")
	if err := os.WriteFile(filepath.Join(dir, "extra.lua"), []byte("local x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restoredFrom, err := m.RollbackFolder(dir)
	if err != nil {
		t.Fatalf("RollbackFolder: %v", err)
	}
	if restoredFrom == "" {
		t.Error("restoredFrom is empty, want a snapshot id")
	}
	if got := tocVersion(t, dir); got != "1.0.0" {
		t.Errorf("version after rollback = %q, want 1.0.0", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "extra.lua")); !os.IsNotExist(err) {
		t.Error("file added after the snapshot still present after rollback")
	}
}

func TestRollbackFolderNewestWins(t *testing.T) {
	root := t.TempDir()
	m := New(filepath.Join(root, "Backups"), nil)
	dir := filepath.Join(root, "AddOns", "Questie")
	seedFolder(t, dir, "1.0.0")
	first, err := m.Backup([]string{dir}, "first")
	if err != nil {
		t.Fatalf("Backup 1: %v", err)
	}
	seedFolder(t, dir, "2.0.0")
	second, err := m.Backup([]string{dir}, "second")
	if err != nil {
		t.Fatalf("Backup 2: %v", err)
	}
	seedFolder(t, dir, "3.0.0")

	restoredFrom, err := m.RollbackFolder(dir)
	if err != nil {
		t.Fatalf("RollbackFolder: %v", err)
	}
	if restoredFrom != filepath.Base(second) {
		t.Errorf("restoredFrom = %q, want %q (the newer snapshot)", restoredFrom, filepath.Base(second))
	}
	if restoredFrom == filepath.Base(first) {
		t.Error("restored from the older snapshot, want the newer one")
	}
	if got := tocVersion(t, dir); got != "2.0.0" {
		t.Errorf("version after rollback = %q, want 2.0.0", got)
	}
}

func TestRollbackFolderMissingSnapshot(t *testing.T) {
	root := t.TempDir()
	m := New(filepath.Join(root, "Backups"), nil)
	dir := filepath.Join(root, "AddOns", "Questie")
	seedFolder(t, dir, "1.0.0")

	if _, err := m.RollbackFolder(dir); err == nil {
		t.Fatal("RollbackFolder with no snapshot should error")
	} else if !strings.Contains(err.Error(), "no backup snapshot contains") {
		t.Errorf("error = %q, want no-snapshot message", err.Error())
	}
}

func TestRollbackFolderBacksUpDestination(t *testing.T) {
	root := t.TempDir()
	m := New(filepath.Join(root, "Backups"), nil)
	dir := filepath.Join(root, "AddOns", "Questie")
	seedFolder(t, dir, "1.0.0")
	if _, err := m.Backup([]string{dir}, "seed"); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	seedFolder(t, dir, "2.0.0") // modified state that must be preserved

	if _, err := m.RollbackFolder(dir); err != nil {
		t.Fatalf("RollbackFolder: %v", err)
	}

	// A snapshot of the pre-rollback (modified) state must exist.
	snapshots, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snapshots {
		if !strings.HasPrefix(s.Reason, "pre-rollback of ") {
			continue
		}
		mf, err := readManifest(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range mf.Entries {
			if !strings.EqualFold(e.OriginalPath, dir) {
				continue
			}
			if got := tocVersion(t, filepath.Join(s.Path, e.Name)); got != "2.0.0" {
				t.Errorf("pre-rollback snapshot content = %q, want the modified 2.0.0", got)
			}
			return
		}
	}
	t.Error("no pre-rollback snapshot of the destination was created")
}

func TestRollbackFolderMissingSnapshotCopy(t *testing.T) {
	root := t.TempDir()
	m := New(filepath.Join(root, "Backups"), nil)
	dir := filepath.Join(root, "AddOns", "Questie")
	seedFolder(t, dir, "1.0.0")
	snap, err := m.Backup([]string{dir}, "seed")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	// Corrupt the snapshot: remove the stored copy but keep the manifest.
	mf, err := readManifest(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(snap, mf.Entries[0].Name)); err != nil {
		t.Fatal(err)
	}

	if _, err := m.RollbackFolder(dir); err == nil {
		t.Fatal("RollbackFolder with a missing snapshot copy should error")
	} else if !strings.Contains(err.Error(), "is missing") {
		t.Errorf("error = %q, want missing-copy message", err.Error())
	}
}
