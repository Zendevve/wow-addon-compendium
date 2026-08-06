package detector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkAddons builds tmp/<flavor>/Interface/AddOns with one addon subfolder.
func mkAddons(t *testing.T, tmp, flavor string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(AddonsPath(tmp, flavor), "SomeAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// detect calls DetectPath and fails the test on error.
func detect(t *testing.T, input string) *Installation {
	t.Helper()
	inst, err := DetectPath(input)
	if err != nil {
		t.Fatalf("DetectPath(%q) error: %v", input, err)
	}
	return inst
}

// detectErr calls DetectPath and asserts the error contains wantSub.
func detectErr(t *testing.T, input, wantSub string) {
	t.Helper()
	_, err := DetectPath(input)
	if err == nil {
		t.Fatalf("DetectPath(%q): expected error containing %q, got nil", input, wantSub)
	}
	if !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("DetectPath(%q) error = %q, want substring %q", input, err, wantSub)
	}
}

func TestDetectPathRootFlavorRoot(t *testing.T) {
	tmp := t.TempDir()
	mkAddons(t, tmp, FlavorRoot)

	inst := detect(t, tmp)
	if inst.Flavor != FlavorRoot {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRoot)
	}
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
	wantAddons := filepath.Join(tmp, "Interface", "AddOns")
	if inst.AddonsPath != wantAddons {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, wantAddons)
	}
}

func TestDetectPathRootRetail(t *testing.T) {
	tmp := t.TempDir()
	mkAddons(t, tmp, FlavorRetail)

	inst := detect(t, tmp)
	if inst.Flavor != FlavorRetail {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRetail)
	}
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
}

func TestDetectPathAddonsPath(t *testing.T) {
	tmp := t.TempDir()
	mkAddons(t, tmp, FlavorRoot)
	pasted := filepath.Join(tmp, "Interface", "AddOns")

	inst := detect(t, pasted)
	if inst.Flavor != FlavorRoot {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRoot)
	}
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
	if inst.AddonsPath != pasted {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, pasted)
	}
}

func TestDetectPathAddonsPathUnderFlavor(t *testing.T) {
	tmp := t.TempDir()
	mkAddons(t, tmp, FlavorRetail)

	inst := detect(t, filepath.Join(tmp, FlavorRetail, "Interface", "AddOns"))
	if inst.Flavor != FlavorRetail {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRetail)
	}
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
}

func TestDetectPathInterfaceFolder(t *testing.T) {
	tmp := t.TempDir()
	mkAddons(t, tmp, FlavorRoot)

	inst := detect(t, filepath.Join(tmp, "Interface"))
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
	wantAddons := filepath.Join(tmp, "Interface", "AddOns")
	if inst.AddonsPath != wantAddons {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, wantAddons)
	}
}

func TestDetectPathMissing(t *testing.T) {
	detectErr(t, filepath.Join(t.TempDir(), "nope"), "path does not exist")
}

func TestDetectPathFile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	detectErr(t, f, "not a directory")
}

func TestDetectPathAddonsMissingRootExists(t *testing.T) {
	tmp := t.TempDir() // derived root exists
	pasted := filepath.Join(tmp, "Interface", "AddOns")
	detectErr(t, pasted, "AddOns directory does not exist")
}

func TestMatchAddonsPath(t *testing.T) {
	tmp := t.TempDir()
	// Interface-folder inputs require the AddOns dir to exist.
	mkAddons(t, tmp, FlavorRoot)
	mkAddons(t, tmp, FlavorClassic)

	tests := []struct {
		name       string
		input      string
		wantRoot   string
		wantFlavor string
		wantAddons string
		wantOK     bool
	}{
		{
			name:       "root-level AddOns path",
			input:      filepath.Join(tmp, "Interface", "AddOns"),
			wantRoot:   tmp,
			wantFlavor: FlavorRoot,
			wantAddons: filepath.Join(tmp, "Interface", "AddOns"),
			wantOK:     true,
		},
		{
			name:       "flavor AddOns path",
			input:      filepath.Join(tmp, FlavorRetail, "Interface", "AddOns"),
			wantRoot:   tmp,
			wantFlavor: FlavorRetail,
			wantAddons: filepath.Join(tmp, FlavorRetail, "Interface", "AddOns"),
			wantOK:     true,
		},
		{
			name:       "case-insensitive names and flavor",
			input:      filepath.Join(tmp, "_RETAIL_", "Interface", "AddOns"),
			wantRoot:   tmp,
			wantFlavor: FlavorRetail,
			wantAddons: filepath.Join(tmp, "_RETAIL_", "Interface", "AddOns"),
			wantOK:     true,
		},
		{
			name:       "Interface folder",
			input:      filepath.Join(tmp, "Interface"),
			wantRoot:   tmp,
			wantFlavor: FlavorRoot,
			wantAddons: filepath.Join(tmp, "Interface", "AddOns"),
			wantOK:     true,
		},
		{
			name:       "Interface folder under flavor",
			input:      filepath.Join(tmp, FlavorClassic, "Interface"),
			wantRoot:   tmp,
			wantFlavor: FlavorClassic,
			wantAddons: filepath.Join(tmp, FlavorClassic, "Interface", "AddOns"),
			wantOK:     true,
		},
		{
			name:       "plain root",
			input:      tmp,
			wantRoot:   "",
			wantFlavor: "",
			wantAddons: "",
			wantOK:     false,
		},
		{
			name:       "junk",
			input:      filepath.Join(tmp, "whatever"),
			wantRoot:   "",
			wantFlavor: "",
			wantAddons: "",
			wantOK:     false,
		},
		{
			name:       "folder named AddOns with no Interface parent",
			input:      filepath.Join(tmp, "AddOns"),
			wantRoot:   "",
			wantFlavor: "",
			wantAddons: "",
			wantOK:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, flavor, addons, ok := matchAddonsPath(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("matchAddonsPath(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if root != tt.wantRoot || flavor != tt.wantFlavor || addons != tt.wantAddons {
				t.Errorf("matchAddonsPath(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.input, root, flavor, addons, tt.wantRoot, tt.wantFlavor, tt.wantAddons)
			}
		})
	}
}

// writeExe creates a dummy client executable file in root.
func writeExe(t *testing.T, root, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte("not a real executable"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectPathExeOnlyRoot(t *testing.T) {
	tmp := t.TempDir()
	writeExe(t, tmp, "Wow.exe")

	inst := detect(t, tmp)
	if inst.Flavor != FlavorRoot {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRoot)
	}
	if inst.Exe != "Wow.exe" {
		t.Errorf("Exe = %q, want %q", inst.Exe, "Wow.exe")
	}
	wantAddons := filepath.Join(tmp, "Interface", "AddOns")
	if inst.AddonsPath != wantAddons {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, wantAddons)
	}
	if inst.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q", inst.Confidence, "high")
	}
}

func TestDetectPathExeCaseInsensitive(t *testing.T) {
	tmp := t.TempDir()
	writeExe(t, tmp, "wowclassic_tbc.exe")

	inst := detect(t, tmp)
	if inst.Flavor != FlavorRoot {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRoot)
	}
	if inst.Exe != "wowclassic_tbc.exe" {
		t.Errorf("Exe = %q, want %q", inst.Exe, "wowclassic_tbc.exe")
	}
	if inst.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q", inst.Confidence, "high")
	}
	if inst.ProfileID != "tbc" {
		t.Errorf("ProfileID = %q, want %q", inst.ProfileID, "tbc")
	}
}

func TestDetectPathInterfaceOnlyRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "Interface"), 0o755); err != nil {
		t.Fatal(err)
	}

	inst := detect(t, tmp)
	if inst.Flavor != FlavorRoot {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRoot)
	}
	wantAddons := filepath.Join(tmp, "Interface", "AddOns")
	if inst.AddonsPath != wantAddons {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, wantAddons)
	}
}

func TestDetectPathFlavorInterfaceOnly(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, FlavorRetail, "Interface"), 0o755); err != nil {
		t.Fatal(err)
	}

	inst := detect(t, tmp)
	if inst.Flavor != FlavorRetail {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRetail)
	}
	wantAddons := filepath.Join(tmp, FlavorRetail, "Interface", "AddOns")
	if inst.AddonsPath != wantAddons {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, wantAddons)
	}
}

func TestDetectPathEmptyRootRejected(t *testing.T) {
	detectErr(t, t.TempDir(), "no Interface/AddOns directory found")
}

func TestDetectPathMissingAddonsExeRoot(t *testing.T) {
	tmp := t.TempDir()
	writeExe(t, tmp, "Wow.exe")
	pasted := filepath.Join(tmp, "Interface", "AddOns")

	inst := detect(t, pasted)
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
	if inst.AddonsPath != pasted {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, pasted)
	}
	if inst.Flavor != FlavorRoot {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRoot)
	}
}

func TestDetectPathMissingAddonsExeRootFlavor(t *testing.T) {
	tmp := t.TempDir()
	writeExe(t, tmp, "Wow.exe")
	pasted := filepath.Join(tmp, FlavorRetail, "Interface", "AddOns")

	inst := detect(t, pasted)
	if inst.Root != tmp {
		t.Errorf("Root = %q, want %q", inst.Root, tmp)
	}
	if inst.AddonsPath != pasted {
		t.Errorf("AddonsPath = %q, want %q", inst.AddonsPath, pasted)
	}
	if inst.Flavor != FlavorRetail {
		t.Errorf("Flavor = %q, want %q", inst.Flavor, FlavorRetail)
	}
}

func TestDetectPathMissingAddonsJunkRoot(t *testing.T) {
	tmp := t.TempDir() // empty root: no exe, no Interface
	detectErr(t, filepath.Join(tmp, "Interface", "AddOns"), "AddOns directory does not exist")
}

func TestEnsureAddons(t *testing.T) {
	tmp := t.TempDir()
	addons := filepath.Join(tmp, "Interface", "AddOns")
	inst := &Installation{AddonsPath: addons}

	created, err := EnsureAddons(inst)
	if err != nil {
		t.Fatalf("EnsureAddons error: %v", err)
	}
	if !created {
		t.Error("EnsureAddons = false on fresh install, want true")
	}
	if _, err := os.Stat(addons); err != nil {
		t.Errorf("AddonsPath not created on disk: %v", err)
	}

	created, err = EnsureAddons(inst)
	if err != nil {
		t.Fatalf("EnsureAddons second call error: %v", err)
	}
	if created {
		t.Error("EnsureAddons = true on existing dir, want false")
	}
}

func TestEnsureAddonsExistingDir(t *testing.T) {
	tmp := t.TempDir()
	mkAddons(t, tmp, FlavorRoot)
	inst := &Installation{AddonsPath: AddonsPath(tmp, FlavorRoot)}

	created, err := EnsureAddons(inst)
	if err != nil {
		t.Fatalf("EnsureAddons error: %v", err)
	}
	if created {
		t.Error("EnsureAddons = true on existing dir, want false")
	}
}

func TestFindClientExe(t *testing.T) {
	for _, name := range []string{"wow.exe", "Wow.exe", "WowClassic_TBC.exe", "wow-64.exe", "World of Warcraft.app"} {
		t.Run("accept "+name, func(t *testing.T) {
			tmp := t.TempDir()
			if strings.HasSuffix(name, ".app") {
				if err := os.Mkdir(filepath.Join(tmp, name), 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				writeExe(t, tmp, name)
			}
			got, ok := findClientExe(tmp)
			if !ok || got != name {
				t.Errorf("findClientExe = (%q, %v), want (%q, true)", got, ok, name)
			}
		})
	}

	for _, name := range []string{"readme.txt", "addon.dll", "wow_voice.exe"} {
		t.Run("reject "+name, func(t *testing.T) {
			tmp := t.TempDir()
			writeExe(t, tmp, name)
			if got, ok := findClientExe(tmp); ok {
				t.Errorf("findClientExe = (%q, true), want rejected", got)
			}
		})
	}

	t.Run("empty dir", func(t *testing.T) {
		if got, ok := findClientExe(t.TempDir()); ok {
			t.Errorf("findClientExe = (%q, true) in empty dir, want not found", got)
		}
	})
}
