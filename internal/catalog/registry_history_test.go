package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryRecordsVersionHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Fresh install records the initial version.
	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "9.0.0", Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie", VersionRef: "v9.0.0"}); err != nil {
		t.Fatalf("Track v9.0.0: %v", err)
	}
	// Update records the new version, keeping the old one.
	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "9.2.0", Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie", VersionRef: "v9.2.0"}); err != nil {
		t.Fatalf("Track v9.2.0: %v", err)
	}

	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	h := entries[0].History
	if len(h) != 2 {
		t.Fatalf("history = %d entries, want 2: %+v", len(h), h)
	}
	// Newest first.
	if h[0].Version != "9.2.0" || h[1].Version != "9.0.0" {
		t.Errorf("history order = [%s, %s], want [9.2.0, 9.0.0]", h[0].Version, h[1].Version)
	}
	if h[0].Provider != "github" || h[0].Source != "Questie/Questie" {
		t.Errorf("history entry fields = %+v", h[0])
	}
	if h[0].Ref != "v9.2.0" {
		t.Errorf("history ref = %q, want v9.2.0", h[0].Ref)
	}
	if h[1].Ref != "v9.0.0" {
		t.Errorf("history ref = %q, want v9.0.0", h[1].Ref)
	}
	if h[0].At.IsZero() || h[1].At.IsZero() {
		t.Error("history timestamps not set")
	}
	// The transient VersionRef must not leak onto the persisted entry.
	if entries[0].VersionRef != "" {
		t.Errorf("VersionRef persisted on entry: %q", entries[0].VersionRef)
	}

	// History survives a reload.
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.Entries()[0].History
	if len(got) != 2 || got[0].Version != "9.2.0" || got[1].Version != "9.0.0" {
		t.Errorf("history not persisted: %+v", got)
	}
}

func TestRegistryHistoryBounded(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for i := 1; i <= maxVersionHistory+3; i++ {
		if err := reg.Track(Entry{Folder: "X", Version: string(rune('0'+i%10)) + "x"}); err != nil {
			t.Fatalf("Track %d: %v", i, err)
		}
	}
	h := reg.Entries()[0].History
	if len(h) != maxVersionHistory {
		t.Fatalf("history = %d entries, want %d (bounded)", len(h), maxVersionHistory)
	}
	// The log is newest-first, so the oldest version was dropped.
	if h[0].Version != "3x" {
		t.Errorf("newest history entry = %q, want 3x", h[0].Version)
	}
}

func TestRegistryHistorySameVersionNotDuplicated(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "X", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	// Re-tracking the same version (e.g. a reinstall) is not a new
	// version event.
	if err := reg.Track(Entry{Folder: "X", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Track(Entry{Folder: "X", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if h := reg.Entries()[0].History; len(h) != 1 {
		t.Errorf("history = %d entries, want 1 (same version tracked 3x)", len(h))
	}
}

func TestRegistryHistoryRollbackRecordsEvent(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	_ = reg.Track(Entry{Folder: "X", Version: "1.0.0"})
	_ = reg.Track(Entry{Folder: "X", Version: "2.0.0"})
	// Rolling back to an older version is a version change and is
	// recorded even though 1.0.0 is already in the log.
	if err := reg.Track(Entry{Folder: "X", Version: "1.0.0"}); err != nil {
		t.Fatalf("rollback track: %v", err)
	}
	h := reg.Entries()[0].History
	if len(h) != 3 {
		t.Fatalf("history = %d entries, want 3: %+v", len(h), h)
	}
	if h[0].Version != "1.0.0" || h[1].Version != "2.0.0" || h[2].Version != "1.0.0" {
		t.Errorf("history order = %+v, want [1.0.0, 2.0.0, 1.0.0]", h)
	}
}

func TestRegistryHistoryVersionChangeOnUpsertByFolder(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Questie", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive upsert with a version change records history.
	if err := reg.Track(Entry{Folder: "questie", Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	h := reg.Entries()[0].History
	if len(h) != 2 || h[0].Version != "2.0.0" {
		t.Errorf("history after upsert = %+v", h)
	}
}

func TestRegistryOldFormatWithoutHistory(t *testing.T) {
	// Registries written before history existed load with nil History
	// and keep working.
	path := filepath.Join(t.TempDir(), "registry.json")
	data := `{
  "version": 1,
  "entries": [
    {"folder": "Questie", "title": "Questie", "version": "9.2.0", "provider": "github", "id": "Questie/Questie", "installed_at": "2024-01-02T03:04:05Z"}
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	e := reg.Entries()[0]
	if e.History != nil {
		t.Errorf("History = %+v, want nil for old-format registry", e.History)
	}
	// The first version change still starts a fresh log.
	if err := reg.Track(Entry{Folder: "Questie", Version: "9.3.0", Provider: "github", ID: "Questie/Questie"}); err != nil {
		t.Fatal(err)
	}
	h := reg.Entries()[0].History
	if len(h) != 1 || h[0].Version != "9.3.0" {
		t.Errorf("history after first change = %+v", h)
	}
}

func TestRegistryHistoryAtRoughlyNow(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	before := time.Now().Add(-time.Minute)
	_ = reg.Track(Entry{Folder: "X", Version: "1.0.0"})
	after := time.Now().Add(time.Minute)
	at := reg.Entries()[0].History[0].At
	if at.Before(before) || at.After(after) {
		t.Errorf("history timestamp %v outside expected window", at)
	}
}

func TestRegistryHistoryEmptyVersionNotRecorded(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "X", Version: ""}); err != nil {
		t.Fatal(err)
	}
	if h := reg.Entries()[0].History; len(h) != 0 {
		t.Errorf("history = %+v, want none for empty version", h)
	}
}
