package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestManager builds a Manager over a fresh AddOns dir with the
// given top-level folders (names ending in ".disabled" are created as
// such).
func newTestManager(t *testing.T, folders ...string) (*Manager, string) {
	t.Helper()
	addons := filepath.Join(t.TempDir(), "Interface", "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range folders {
		if err := os.MkdirAll(filepath.Join(addons, f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "collections")
	m, err := NewManager(path, addons)
	if err != nil {
		t.Fatal(err)
	}
	return m, addons
}

func enabledMap(c *Collection) map[string]bool {
	out := make(map[string]bool, len(c.Addons))
	for _, s := range c.Addons {
		out[s.Folder] = s.Enabled
	}
	return out
}

func TestCreateSnapshotsCurrentState(t *testing.T) {
	m, _ := newTestManager(t, "Questie", "AtlasLoot", "Dead.disabled", "TempFolder")
	c, err := m.Create("pve")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "pve" {
		t.Fatalf("id = %q, want %q", c.ID, "pve")
	}
	got := enabledMap(c)
	want := map[string]bool{
		"Questie":    true,
		"AtlasLoot":  true,
		"Dead":       false,
		"TempFolder": true,
	}
	if len(got) != len(want) {
		t.Fatalf("snapshot has %d addons, want %d: %v", len(got), len(want), got)
	}
	for folder, enabled := range want {
		if got[folder] != enabled {
			t.Fatalf("folder %q enabled = %v, want %v (all: %v)", folder, got[folder], enabled, got)
		}
	}
	if got["Dead"] {
		t.Fatalf("Dead.disabled must snapshot as disabled, got %v", got)
	}
	if _, ok := got["Dead.disabled"]; ok {
		t.Fatalf(".disabled suffix must be stripped from the folder name: %v", got)
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: created=%v updated=%v", c.CreatedAt, c.UpdatedAt)
	}
}

func TestCreateUniqueIDs(t *testing.T) {
	m, _ := newTestManager(t, "Questie")
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	cols, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(cols))
	}
	if cols[0].ID == cols[1].ID {
		t.Fatalf("duplicate names must get distinct ids, got %q twice", cols[0].ID)
	}
	// Repeated names keep distinct ids even after a delete.
	if err := m.Delete(cols[1].ID); err != nil {
		t.Fatal(err)
	}
	c, err := m.Create("pve")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == cols[0].ID {
		t.Fatalf("recreated id %q collides with the surviving collection", c.ID)
	}
}

func TestListSortedByName(t *testing.T) {
	m, _ := newTestManager(t, "Questie")
	for _, name := range []string{"zeta", "Alpha", "beta"} {
		if _, err := m.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	cols, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(cols))
	for i, c := range cols {
		got[i] = c.Name
	}
	want := []string{"Alpha", "beta", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List order = %v, want %v", got, want)
		}
	}
}

func TestSwitchToRenamesCorrectly(t *testing.T) {
	m, addons := newTestManager(t, "Questie", "AtlasLoot", "Dead.disabled")
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	// Flip Questie off and AtlasLoot on, then apply.
	if err := m.SetEnabled("pve", "Questie", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled("pve", "AtlasLoot", true); err != nil {
		t.Fatal(err)
	}
	// Dead is disabled in the snapshot; force it enabled for the check.
	if err := m.SetEnabled("pve", "Dead", true); err != nil {
		t.Fatal(err)
	}

	applied, err := m.SwitchTo("pve")
	if err != nil {
		t.Fatal(err)
	}
	// AtlasLoot is already enabled, so only Questie and Dead move.
	if len(applied) != 2 {
		t.Fatalf("applied = %v, want 2 renames", applied)
	}
	for _, f := range applied {
		if f != "Dead" && f != "Questie" {
			t.Fatalf("unexpected rename target %q in %v", f, applied)
		}
	}
	if !dirExists(t, filepath.Join(addons, "Questie.disabled")) {
		t.Fatal("Questie should now be Questie.disabled")
	}
	if dirExists(t, filepath.Join(addons, "Questie")) {
		t.Fatal("Questie should no longer exist")
	}
	if !dirExists(t, filepath.Join(addons, "AtlasLoot")) {
		t.Fatal("AtlasLoot should be enabled")
	}
	if !dirExists(t, filepath.Join(addons, "Dead")) {
		t.Fatal("Dead should be re-enabled")
	}

	// Switching again is a no-op: no further renames.
	applied, err = m.SwitchTo("pve")
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second switch renamed %v, want none", applied)
	}
}

func TestSwitchToErrorsOnTargetCollision(t *testing.T) {
	m, addons := newTestManager(t, "Questie", "Questie.disabled")
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	// Force the state to "enable Questie" while BOTH names exist.
	if err := m.SetEnabled("pve", "Questie", true); err != nil {
		t.Fatal(err)
	}
	_, err := m.SwitchTo("pve")
	if err == nil {
		t.Fatal("expected a collision error, got none")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error %q should mention the existing target", err)
	}
	// Nothing may be overwritten: both folders still exist.
	if !dirExists(t, filepath.Join(addons, "Questie")) ||
		!dirExists(t, filepath.Join(addons, "Questie.disabled")) {
		t.Fatal("collision must not delete either folder")
	}
}

func TestSwitchToSkipsMissingAddons(t *testing.T) {
	m, addons := newTestManager(t, "Questie")
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	// Disable Questie and enable an addon that is not installed at all.
	if err := m.SetEnabled("pve", "Questie", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled("pve", "Ghost", true); err != nil {
		t.Fatal(err)
	}
	applied, err := m.SwitchTo("pve")
	if err != nil {
		t.Fatalf("missing addons must not fail the switch: %v", err)
	}
	if len(applied) != 1 || applied[0] != "Questie" {
		t.Fatalf("applied = %v, want [Questie]", applied)
	}
	if !dirExists(t, filepath.Join(addons, "Questie.disabled")) {
		t.Fatal("Questie should be disabled")
	}
}

func TestDuplicateRenameDelete(t *testing.T) {
	m, _ := newTestManager(t, "Questie", "AtlasLoot")
	orig, err := m.Create("pve")
	if err != nil {
		t.Fatal(err)
	}
	dup, err := m.Duplicate("pve", "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if dup.ID == orig.ID {
		t.Fatal("duplicate must get a fresh id")
	}
	if len(dup.Addons) != len(orig.Addons) {
		t.Fatalf("duplicate has %d addons, want %d", len(dup.Addons), len(orig.Addons))
	}

	if err := m.Rename("pvp", "PvP Setup"); err != nil {
		t.Fatal(err)
	}
	renamed, err := m.Get("pvp")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "PvP Setup" {
		t.Fatalf("name = %q, want %q", renamed.Name, "PvP Setup")
	}
	if renamed.ID != "pvp" {
		t.Fatalf("rename must keep the id, got %q", renamed.ID)
	}

	if err := m.Delete("pvp"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("pvp"); err == nil {
		t.Fatal("deleted collection should not load")
	}
	if err := m.Delete("pvp"); err == nil {
		t.Fatal("deleting a missing collection must error")
	}
}

func TestSetEnabledAddsUnknownFolder(t *testing.T) {
	m, _ := newTestManager(t, "Questie")
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled("pve", "AtlasLoot", false); err != nil {
		t.Fatal(err)
	}
	c, err := m.Get("pve")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Addons) != 2 {
		t.Fatalf("expected 2 addon states, got %d", len(c.Addons))
	}
	if c.Addons[1].Folder != "AtlasLoot" || c.Addons[1].Enabled {
		t.Fatalf("new folder not appended correctly: %+v", c.Addons)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	m, _ := newTestManager(t, "Questie", "AtlasLoot", "Dead.disabled")
	if _, err := m.Create("pve"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled("pve", "AtlasLoot", false); err != nil {
		t.Fatal(err)
	}

	// A fresh manager over the same Path must see the same data.
	m2, err := NewManager(m.Path, "")
	if err != nil {
		t.Fatal(err)
	}
	cols, err := m2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 1 || cols[0].ID != "pve" {
		t.Fatalf("round-trip list = %+v", cols)
	}
	got := enabledMap(&cols[0])
	want := map[string]bool{"Questie": true, "AtlasLoot": false, "Dead": false}
	if len(got) != len(want) {
		t.Fatalf("round-trip addons = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("round-trip %q = %v, want %v", k, got[k], v)
		}
	}
}

func dirExists(t *testing.T, path string) bool {
	t.Helper()
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
