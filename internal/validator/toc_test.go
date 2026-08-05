package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wowfix/wowfix/internal/models"
)

func writeTOC(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Addon.toc")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTOCFull(t *testing.T) {
	path := writeTOC(t, `## Interface: 30300
## Title: AtlasLoot
## Version: 7.0.4
## Author: AtlasLoot Team
## Notes: loot browser
## X-Curse-Project-ID: 22

AtlasLoot.lua
AtlasLoot.xml
`)
	toc, err := ParseTOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if toc.Name != "Addon" {
		t.Fatalf("Name = %q, want Addon", toc.Name)
	}
	if toc.Interface != 30300 {
		t.Fatalf("Interface = %d, want 30300", toc.Interface)
	}
	if toc.Title != "AtlasLoot" {
		t.Fatalf("Title = %q", toc.Title)
	}
	if toc.Version != "7.0.4" {
		t.Fatalf("Version = %q", toc.Version)
	}
	if toc.Author != "AtlasLoot Team" {
		t.Fatalf("Author = %q", toc.Author)
	}
	if toc.Fields["X-Curse-Project-ID"] != "22" {
		t.Fatalf("unknown field not preserved: %v", toc.Fields)
	}
}

func TestParseTOCMissingInterface(t *testing.T) {
	path := writeTOC(t, "## Title: NoVersion\nAddon.lua\n")
	toc, err := ParseTOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if toc.Interface != -1 {
		t.Fatalf("Interface = %d, want -1 (missing)", toc.Interface)
	}
}

func TestParseTOCMalformedInterface(t *testing.T) {
	path := writeTOC(t, "## Interface: three-zero-three\n")
	toc, err := ParseTOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if toc.Interface != -1 {
		t.Fatalf("non-numeric Interface must stay -1, got %d", toc.Interface)
	}
	if toc.RawInterface != "three-zero-three" {
		t.Fatalf("RawInterface = %q", toc.RawInterface)
	}
}

func TestParseTOCLocalizedTitle(t *testing.T) {
	path := writeTOC(t, "## Title-zhCN: 插件\n## Title: RealTitle\n")
	toc, err := ParseTOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if toc.Title != "RealTitle" {
		t.Fatalf("localized title must not shadow the base title, got %q", toc.Title)
	}
}

func TestParseTOCOnlyLocalizedTitle(t *testing.T) {
	path := writeTOC(t, "## Title-zhCN: 插件\n")
	toc, err := ParseTOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if toc.Title != "插件" {
		t.Fatalf("expected localized title fallback, got %q", toc.Title)
	}
}

func TestParseTOCMissingFile(t *testing.T) {
	if _, err := ParseTOC(filepath.Join(t.TempDir(), "nope.toc")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- compatibility classification -------------------------------------

func TestClassifyInterface(t *testing.T) {
	wrath := models.ProfileByID("wrath")
	retail := models.ProfileByID("retail")
	vanilla := models.ProfileByID("vanilla")
	classic := models.ProfileByID("classic")

	cases := []struct {
		name     string
		profile  *models.Profile
		detected int
		want     models.CompatStatus
	}{
		{"exact match", wrath, 30300, models.CompatOK},
		{"same family newer minor", wrath, 30307, models.CompatOK},
		{"vanilla addon on wrath", wrath, 11200, models.CompatVanilla},
		{"retail addon on wrath", wrath, 100207, models.CompatRetail},
		{"tbc addon on wrath", wrath, 20400, models.CompatMismatch},
		{"classic era on classic", classic, 11403, models.CompatOK},
		{"classic era on vanilla", vanilla, 11403, models.CompatOK},
		{"vanilla on classic", classic, 11200, models.CompatOK},
		{"retail on retail", retail, 100207, models.CompatOK},
		{"missing interface", wrath, 0, models.CompatUnknown},
		{"negative interface", wrath, -1, models.CompatUnknown},
	}
	for _, c := range cases {
		if got := models.ClassifyInterface(c.profile, c.detected); got != c.want {
			t.Errorf("%s: ClassifyInterface(%s, %d) = %s, want %s",
				c.name, c.profile.ID, c.detected, got, c.want)
		}
	}
}

func TestValidateTOCDetail(t *testing.T) {
	wrath := models.ProfileByID("wrath")
	toc := &models.TOC{Name: "X", Interface: 11200}

	compat := ValidateTOC(toc, wrath)
	if compat.Status != models.CompatVanilla {
		t.Fatalf("status = %s, want vanilla", compat.Status)
	}
	if compat.Label != "Vanilla Addon" {
		t.Fatalf("label = %q", compat.Label)
	}
	if compat.Detail == "" {
		t.Fatal("expected a detail sentence")
	}
}

func TestValidateAddonPrimaryDrivesVerdict(t *testing.T) {
	wrath := models.ProfileByID("wrath")
	addon := &models.Addon{TOCs: []*models.TOC{
		{Name: "Atlas", Interface: 30300, Primary: true},
		{Name: "Atlas_TBC", Interface: 20400},
	}}
	compat := ValidateAddon(addon, wrath)
	if len(compat) != 2 {
		t.Fatalf("expected 2 compat rows, got %d", len(compat))
	}
	if compat[0].Status != models.CompatOK {
		t.Fatalf("primary TOC status = %s, want compatible", compat[0].Status)
	}
	if compat[1].Status != models.CompatMismatch {
		t.Fatalf("alternate TOC status = %s, want mismatch", compat[1].Status)
	}
}
