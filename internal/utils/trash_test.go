package utils

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTrashRemovesSource verifies Trash relocates a directory so the
// source no longer exists, through either the native trash or the
// fallback move. It exercises the Windows Recycle Bin path when
// running on Windows.
func TestTrashRemovesSource(t *testing.T) {
	fallback := t.TempDir()

	// A directory with a nested file.
	victim := filepath.Join(t.TempDir(), "SomeAddon")
	if err := os.MkdirAll(filepath.Join(victim, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "sub", "file.lua"), []byte("x=1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Trash(victim, fallback); err != nil {
		t.Fatalf("Trash(%q) error: %v", victim, err)
	}
	if Exists(victim) {
		t.Fatalf("source %q still exists after Trash", victim)
	}
}

// TestFallbackMoveCopiesCrossDevice exercises the copy path directly by
// simulating a failed rename via a destination on a different volume is
// not possible portably; instead we assert the fallback itself works
// and leaves the source gone.
func TestFallbackMoveLeavesSourceGone(t *testing.T) {
	src := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.toc"), []byte("## Interface: 30300\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fallback := t.TempDir()

	if err := FallbackMove(src, fallback); err != nil {
		t.Fatalf("FallbackMove error: %v", err)
	}
	if Exists(src) {
		t.Fatalf("source %q still exists after FallbackMove", src)
	}

	// The content must be preserved in the fallback trash.
	trashDir := filepath.Join(fallback, "trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one trash entry in %q, got %v (err %v)", trashDir, entries, err)
	}
	if !Exists(filepath.Join(trashDir, entries[0].Name(), "a.toc")) {
		t.Fatalf("trashed content missing in %q", entries[0].Name())
	}
}
