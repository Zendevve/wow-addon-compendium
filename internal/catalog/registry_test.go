package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if got := reg.Entries(); len(got) != 0 {
		t.Fatalf("fresh registry has %d entries, want 0", len(got))
	}

	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "9.2.0", Provider: "github", ID: "Questie/Questie"}); err != nil {
		t.Fatalf("Track: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Atlas", Title: "Atlas", Version: "1.2.3", Provider: "curseforge", ID: "12345"}); err != nil {
		t.Fatalf("Track: %v", err)
	}

	// A fresh registry from the same path must see the entries.
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entries := reloaded.Entries()
	if len(entries) != 2 {
		t.Fatalf("reloaded registry has %d entries, want 2", len(entries))
	}
	if entries[0].Folder != "Atlas" || entries[1].Folder != "Questie" { // sorted by folder
		t.Errorf("unexpected order: %+v", entries)
	}
	if entries[1].Version != "9.2.0" || entries[1].Provider != "github" {
		t.Errorf("entry fields not persisted: %+v", entries[1])
	}
	if entries[1].InstalledAt.IsZero() {
		t.Error("InstalledAt not set on Track")
	}
}

func TestRegistryUpsertByFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "1.0.0"}); err != nil {
		t.Fatalf("Track: %v", err)
	}
	// Case-insensitive upsert replaces the same folder.
	if err := reg.Track(Entry{Folder: "questie", Title: "Questie", Version: "2.0.0", Provider: "github", ID: "a/b"}); err != nil {
		t.Fatalf("Track upsert: %v", err)
	}
	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("upsert created %d entries, want 1", len(entries))
	}
	if entries[0].Folder != "questie" || entries[0].Version != "2.0.0" {
		t.Errorf("upsert did not replace the entry: %+v", entries[0])
	}

	// Upsert must be persisted too.
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Entries(); len(got) != 1 || got[0].Version != "2.0.0" {
		t.Errorf("upsert not persisted: %+v", got)
	}
}

func TestRegistryForget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	_ = reg.Track(Entry{Folder: "A", Version: "1"})
	_ = reg.Track(Entry{Folder: "B", Version: "2"})

	if err := reg.Forget("a"); err != nil { // case-insensitive
		t.Fatalf("Forget: %v", err)
	}
	entries := reg.Entries()
	if len(entries) != 1 || entries[0].Folder != "B" {
		t.Fatalf("after forget: %+v", entries)
	}
	// Forgetting an unknown folder is a no-op.
	if err := reg.Forget("nope"); err != nil {
		t.Fatalf("Forget unknown: %v", err)
	}
	// Persisted forget.
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Entries(); len(got) != 1 {
		t.Errorf("forget not persisted: %+v", got)
	}
}

func TestRegistryTrackWithoutFolder(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Version: "1.0"}); err == nil {
		t.Fatal("Track without folder should error")
	}
}

func TestRegistryCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistry(path); err == nil {
		t.Fatal("NewRegistry on corrupt file should error")
	}
}

func TestRegistryChecksumRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "9.2.0", Provider: "github", ID: "Questie/Questie", Checksum: "abc123"}); err != nil {
		t.Fatalf("Track: %v", err)
	}
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entries := reloaded.Entries()
	if len(entries) != 1 || entries[0].Checksum != "abc123" {
		t.Errorf("checksum not preserved across save/reload: %+v", entries)
	}
}

func TestRegistryOldFormatWithoutChecksum(t *testing.T) {
	// Registries written before the checksum field existed must load
	// with Checksum == "".
	path := filepath.Join(t.TempDir(), "registry.json")
	data := `{
  "version": 1,
  "entries": [
    {"folder": "Questie", "title": "Questie", "version": "9.2.0", "provider": "github", "id": "Questie/Questie", "source": "Questie/Questie", "installed_at": "2024-01-02T03:04:05Z"}
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Checksum != "" {
		t.Errorf("Checksum = %q, want empty for old-format registry", entries[0].Checksum)
	}
	if entries[0].Version != "9.2.0" {
		t.Errorf("Version = %q, want 9.2.0", entries[0].Version)
	}
	if entries[0].Pinned || entries[0].Ignored {
		t.Errorf("Pinned/Ignored = %v/%v, want false for old-format registry",
			entries[0].Pinned, entries[0].Ignored)
	}
}

func TestRegistrySetPinnedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "9.2.0", Provider: "github", ID: "Questie/Questie"}); err != nil {
		t.Fatalf("Track: %v", err)
	}

	// Case-insensitive folder lookup.
	if err := reg.SetPinned("questie", true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entries := reloaded.Entries()
	if len(entries) != 1 || !entries[0].Pinned {
		t.Errorf("pinned flag not persisted: %+v", entries)
	}
	if entries[0].Ignored {
		t.Error("Ignored should stay false when only pinned is set")
	}

	// Unpin persists too.
	if err := reg.SetPinned("Questie", false); err != nil {
		t.Fatalf("SetPinned(false): %v", err)
	}
	reloaded, err = NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Entries(); len(got) != 1 || got[0].Pinned {
		t.Errorf("unpin not persisted: %+v", got)
	}
}

func TestRegistrySetIgnoredRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Questie", Title: "Questie", Version: "9.2.0"}); err != nil {
		t.Fatalf("Track: %v", err)
	}

	if err := reg.SetIgnored("questie", true); err != nil { // case-insensitive
		t.Fatalf("SetIgnored: %v", err)
	}
	reloaded, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entries := reloaded.Entries()
	if len(entries) != 1 || !entries[0].Ignored {
		t.Errorf("ignored flag not persisted: %+v", entries)
	}
	if entries[0].Pinned {
		t.Error("Pinned should stay false when only ignored is set")
	}
}

func TestRegistrySetFlagsUnknownFolder(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Track(Entry{Folder: "Questie", Version: "1.0"}); err != nil {
		t.Fatalf("Track: %v", err)
	}
	for name, op := range map[string]func(string, bool) error{
		"SetPinned":  reg.SetPinned,
		"SetIgnored": reg.SetIgnored,
	} {
		t.Run(name, func(t *testing.T) {
			err := op("Nope", true)
			if err == nil {
				t.Fatal("unknown folder should error")
			}
			if !strings.Contains(err.Error(), `is not tracked in the registry`) {
				t.Errorf("error = %q, want tracked-message", err.Error())
			}
		})
	}
	// The failed set must not modify anything.
	if entries := reg.Entries(); len(entries) != 1 || entries[0].Pinned || entries[0].Ignored {
		t.Errorf("failed set modified the registry: %+v", entries)
	}
}

func TestRegistryAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := reg.Track(Entry{Folder: "X", Version: "1"}); err != nil {
			t.Fatalf("Track: %v", err)
		}
	}
	// No temp file may be left behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file left behind after save")
	}
	// The file must be valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f registryFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("registry file is not valid JSON: %v", err)
	}
}
