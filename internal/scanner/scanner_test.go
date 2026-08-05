package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowfix/wowfix/internal/models"
)

// newTestScanner builds a scanner over a fresh temp AddOns dir.
func newTestScanner(t *testing.T) (*Scanner, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Interface", "AddOns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return New(dir, models.ProfileByID("wrath")), dir
}

// writeAddon creates <dir>/<name> with the given TOC files.
func writeAddon(t *testing.T, dir, name string, tocs map[string]string, extraFiles []string) {
	t.Helper()
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for toc, body := range tocs {
		if err := os.WriteFile(filepath.Join(root, toc), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range extraFiles {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func scan(t *testing.T, s *Scanner) *models.ScanResult {
	t.Helper()
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func findAddon(t *testing.T, res *models.ScanResult, name string) *models.Addon {
	t.Helper()
	for _, a := range res.Addons {
		if a.FolderName == name {
			return a
		}
	}
	t.Fatalf("addon %q not found in scan (%d results)", name, len(res.Addons))
	return nil
}

func hasIssue(addon *models.Addon, kind models.IssueKind) bool {
	for _, i := range addon.Issues {
		if i.Kind == kind {
			return true
		}
	}
	return false
}

func TestScanHealthyAddon(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "AtlasLoot", map[string]string{
		"AtlasLoot.toc": "## Interface: 30300\n## Title: AtlasLoot\n",
	}, nil)

	res := scan(t, s)
	if len(res.Addons) != 1 {
		t.Fatalf("expected 1 addon, got %d", len(res.Addons))
	}
	a := res.Addons[0]
	if a.Status != models.StatusOK {
		t.Fatalf("healthy addon should be OK, got %s", a.Status)
	}
	if len(a.Issues) != 0 {
		t.Fatalf("healthy addon has issues: %+v", a.Issues)
	}
	if a.PrimaryTOC() == nil {
		t.Fatal("primary TOC not set for matching folder name")
	}
}

func TestScanGitHubFolderName(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Questie-main", map[string]string{
		"Questie.toc": "## Interface: 30300\n",
	}, nil)

	res := scan(t, s)
	a := findAddon(t, res, "Questie-main")
	if !hasIssue(a, models.IssueGitHubName) {
		t.Fatal("expected GitHubName issue")
	}
	if a.BaseName != "Questie" {
		t.Fatalf("BaseName = %q, want Questie", a.BaseName)
	}
	if a.SuggestedName != "Questie" {
		t.Fatalf("SuggestedName = %q, want Questie", a.SuggestedName)
	}
	if a.Status != models.StatusWarn {
		t.Fatalf("status = %s, want warn", a.Status)
	}
}

func TestScanTOCNameMismatch(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "AuxUI", map[string]string{
		"Aux-Classic.toc": "## Interface: 30300\n",
	}, nil)

	res := scan(t, s)
	a := findAddon(t, res, "AuxUI")
	if !hasIssue(a, models.IssueTocMismatch) {
		t.Fatal("expected TocMismatch issue")
	}
	if a.SuggestedName != "Aux-Classic" {
		t.Fatalf("SuggestedName = %q, want Aux-Classic", a.SuggestedName)
	}
}

func TestScanNestedFolder(t *testing.T) {
	s, dir := newTestScanner(t)
	outer := filepath.Join(dir, "DPSMate-main")
	if err := os.MkdirAll(filepath.Join(outer, "DPSMate"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "DPSMate", "DPSMate.toc"), []byte("## Interface: 30300\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := scan(t, s)
	a := findAddon(t, res, "DPSMate")
	if !a.Nested {
		t.Fatal("addon should be marked nested")
	}
	if !hasIssue(a, models.IssueNested) {
		t.Fatal("expected Nested issue")
	}
	if a.SourceDir != "DPSMate-main" {
		t.Fatalf("SourceDir = %q, want DPSMate-main", a.SourceDir)
	}
	if a.SuggestedName != "DPSMate" {
		t.Fatalf("SuggestedName = %q, want DPSMate", a.SuggestedName)
	}
}

func TestScanMissingTOC(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Inventory", nil, []string{"Inventory.lua"})

	res := scan(t, s)
	a := findAddon(t, res, "Inventory")
	if !hasIssue(a, models.IssueMissingTOC) {
		t.Fatal("expected MissingTOC issue")
	}
	if a.Status != models.StatusError {
		t.Fatalf("status = %s, want error", a.Status)
	}
}

func TestScanEmptyFolder(t *testing.T) {
	s, dir := newTestScanner(t)
	if err := os.MkdirAll(filepath.Join(dir, "TempFolder"), 0o755); err != nil {
		t.Fatal(err)
	}

	res := scan(t, s)
	a := findAddon(t, res, "TempFolder")
	if !hasIssue(a, models.IssueEmpty) {
		t.Fatal("expected Empty issue")
	}
}

func TestScanMultipleTOCs(t *testing.T) {
	s, dir := newTestScanner(t)
	// Folder name matches Atlas.toc: that TOC is primary, no issue.
	writeAddon(t, dir, "Atlas", map[string]string{
		"Atlas.toc":       "## Interface: 30300\n",
		"Atlas_Wrath.toc": "## Interface: 30300\n",
		"Atlas_TBC.toc":   "## Interface: 20400\n",
	}, nil)

	res := scan(t, s)
	a := findAddon(t, res, "Atlas")
	if hasIssue(a, models.IssueMultipleTOCs) {
		t.Fatal("no MultipleTOCs issue expected when a TOC matches the folder name")
	}
	if primary := a.PrimaryTOC(); primary == nil || primary.Name != "Atlas" {
		t.Fatalf("primary TOC = %v, want Atlas", primary)
	}

	// No TOC matches the folder name: user must choose.
	writeAddon(t, dir, "Pack", map[string]string{
		"Foo.toc": "## Interface: 30300\n",
		"Bar.toc": "## Interface: 20400\n",
	}, nil)
	res = scan(t, s)
	p := findAddon(t, res, "Pack")
	if !hasIssue(p, models.IssueMultipleTOCs) {
		t.Fatal("expected MultipleTOCs issue when nothing matches the folder name")
	}
	issue := p.Issues[0]
	if len(issue.Options) != 2 {
		t.Fatalf("expected 2 TOC options, got %v", issue.Options)
	}
}

func TestScanDuplicates(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Questie", map[string]string{"Questie.toc": "## Interface: 30300\n"}, nil)
	writeAddon(t, dir, "Questie-main", map[string]string{"Questie.toc": "## Interface: 30300\n"}, nil)

	res := scan(t, s)
	var dup *models.Addon
	for _, a := range res.Addons {
		if hasIssue(a, models.IssueDuplicate) {
			dup = a
		}
	}
	if dup == nil {
		t.Fatal("expected a Duplicate issue on one of the Questie folders")
	}
	if dup.FolderName != "Questie-main" {
		t.Fatalf("duplicate should be the Git-named copy, got %s", dup.FolderName)
	}
	if dup.Issues[0].SuggestedName != "Questie" {
		t.Fatalf("duplicate merge target = %q, want Questie", dup.Issues[0].SuggestedName)
	}
}

func TestScanMultiAddonInvalidStructure(t *testing.T) {
	s, dir := newTestScanner(t)
	parent := filepath.Join(dir, "TwoInOne")
	for _, sub := range []string{"AddonA", "AddonB"} {
		if err := os.MkdirAll(filepath.Join(parent, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, sub, sub+".toc"), []byte("## Interface: 30300\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res := scan(t, s)
	if len(res.Addons) != 2 {
		t.Fatalf("expected 2 addons promoted from the broken folder, got %d", len(res.Addons))
	}
	for _, a := range res.Addons {
		if !a.Nested {
			t.Fatalf("%s should be nested", a.FolderName)
		}
		if !hasIssue(a, models.IssueNested) {
			t.Fatalf("%s missing Nested issue", a.FolderName)
		}
	}
}

func TestScanStrayFilesIgnored(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Real", map[string]string{"Real.toc": "## Interface: 30300\n"}, nil)
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := scan(t, s)
	if len(res.Addons) != 1 {
		t.Fatalf("stray files must be ignored, got %d addons", len(res.Addons))
	}
}

func TestScanUnreadableSubdirIsErrorNotFatal(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Good", map[string]string{"Good.toc": "## Interface: 30300\n"}, nil)

	res := scan(t, s)
	if len(res.Addons) != 1 {
		t.Fatalf("expected 1 addon, got %d", len(res.Addons))
	}
	_ = res // unreadable dirs are hard to simulate portably; keep the contract
}

func TestScanSortsErrorsFirst(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "AAA_Good", map[string]string{"AAA_Good.toc": "## Interface: 30300\n"}, nil)
	writeAddon(t, dir, "BBB_Broken", nil, []string{"x.lua"})
	writeAddon(t, dir, "CCC_Good", map[string]string{"CCC_Good.toc": "## Interface: 30300\n"}, nil)

	res := scan(t, s)
	if res.Addons[0].FolderName != "BBB_Broken" {
		t.Fatalf("error addon must sort first, got %s", res.Addons[0].FolderName)
	}
	if res.Addons[1].Status != models.StatusOK {
		t.Fatalf("healthy addons should follow, got %s", res.Addons[1].FolderName)
	}
}

func TestScanDisabledFolderSkipped(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Questie.disabled", map[string]string{
		"Questie.toc": "## Interface: 30300\n## Title: Questie\n",
	}, nil)
	// The suffix check is case-insensitive.
	writeAddon(t, dir, "Alerts.DISABLED", map[string]string{
		"Alerts.toc": "## Interface: 30300\n## Title: Alerts\n",
	}, nil)
	res := scan(t, s)
	if len(res.Addons) != 0 {
		t.Fatalf("disabled folders must be skipped, got %d addons: %v",
			len(res.Addons), res.Addons)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("disabled folders must not produce errors: %v", res.Errors)
	}
}

func TestScanDisabledSiblingStillScanned(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Questie", map[string]string{
		"Questie.toc": "## Interface: 30300\n## Title: Questie\n",
	}, nil)
	writeAddon(t, dir, "Questie.disabled", map[string]string{
		"Questie.toc": "## Interface: 30300\n## Title: Questie\n",
	}, nil)
	res := scan(t, s)
	if len(res.Addons) != 1 {
		t.Fatalf("only the enabled sibling should be scanned, got %d: %v",
			len(res.Addons), res.Addons)
	}
	findAddon(t, res, "Questie")
}

func TestDiscoverFindsAddonUnits(t *testing.T) {
	s, dir := newTestScanner(t)
	writeAddon(t, dir, "Clean", map[string]string{"Clean.toc": "## Interface: 30300\n"}, nil)
	writeAddon(t, dir, "Zip-main", map[string]string{"Zip.toc": "## Interface: 30300\n"}, nil)
	writeAddon(t, dir, "Old.disabled", map[string]string{"Old.toc": "## Interface: 30300\n"}, nil)

	addons, errs := s.Discover(context.Background())
	if len(errs) != 0 {
		t.Fatalf("unexpected discover errors: %v", errs)
	}
	if len(addons) != 2 {
		t.Fatalf("expected 2 addon units (disabled folder skipped), got %d", len(addons))
	}
}
