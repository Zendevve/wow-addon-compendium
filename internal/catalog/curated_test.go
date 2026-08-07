package catalog

import (
	"strings"
	"testing"
)

// TestLoadCurated checks the embedded manifest is well-formed: both
// families present, every record complete, and every source the exact
// "owner/repo" string InstallFromSource resolves.
func TestLoadCurated(t *testing.T) {
	m, err := LoadCurated()
	if err != nil {
		t.Fatalf("LoadCurated: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, want 1", m.Version)
	}
	families := map[string]bool{}
	for _, s := range m.Sets {
		families[s.Family] = true
		if s.Label == "" {
			t.Errorf("set %q has no label", s.Family)
		}
		if len(s.Addons) == 0 {
			t.Errorf("set %q has no addons", s.Family)
		}
		for _, a := range s.Addons {
			if a.Name == "" || a.Source == "" || a.Summary == "" || a.Homepage == "" {
				t.Errorf("%s/%s: incomplete record %+v", s.Family, a.Name, a)
			}
			if strings.Contains(a.Source, " ") || !strings.Contains(a.Source, "/") {
				t.Errorf("%s: source %q is not a resolvable owner/repo source", a.Name, a.Source)
			}
			if !strings.HasPrefix(a.Homepage, "https://github.com/") {
				t.Errorf("%s: homepage %q is not a GitHub URL", a.Name, a.Homepage)
			}
			if !strings.HasSuffix(a.Homepage, "/"+a.Source) {
				t.Errorf("%s: homepage %q does not match source %q", a.Name, a.Homepage, a.Source)
			}
		}
	}
	if !families["vanilla"] || !families["wrath"] {
		t.Errorf("manifest families = %v, want vanilla and wrath", families)
	}
}

// TestSetForFamily matches family names case-insensitively and rejects
// unknown ones.
func TestSetForFamily(t *testing.T) {
	m, err := LoadCurated()
	if err != nil {
		t.Fatalf("LoadCurated: %v", err)
	}
	for _, family := range []string{"vanilla", "VANILLA", "Vanilla", "wrath", "Wrath"} {
		if set, ok := m.SetForFamily(family); !ok || set.Family != strings.ToLower(family) {
			t.Errorf("SetForFamily(%q) = %+v, %v; want the %s set", family, set, ok, strings.ToLower(family))
		}
	}
	for _, family := range []string{"tbc", "cata", "retail", "", " "} {
		if _, ok := m.SetForFamily(family); ok {
			t.Errorf("SetForFamily(%q) = true, want false", family)
		}
	}
}

// TestAddonByName resolves addon names case-insensitively within a set.
func TestAddonByName(t *testing.T) {
	m, err := LoadCurated()
	if err != nil {
		t.Fatalf("LoadCurated: %v", err)
	}
	set, ok := m.SetForFamily("vanilla")
	if !ok {
		t.Fatal("vanilla set missing")
	}
	if a, ok := set.AddonByName("pfUI"); !ok || a.Source != "shagu/pfUI" {
		t.Errorf("AddonByName(pfUI) = %+v, %v; want shagu/pfUI", a, ok)
	}
	if a, ok := set.AddonByName("pfui"); !ok || a.Source != "shagu/pfUI" {
		t.Errorf("AddonByName(pfui) = %+v, %v; want case-insensitive match", a, ok)
	}
	if _, ok := set.AddonByName("nope"); ok {
		t.Error("AddonByName(nope) = true, want false")
	}
}
