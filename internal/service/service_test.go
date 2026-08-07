// Package service tests the Wails facade end to end against a fake WoW
// tree, entirely under t.TempDir(): no real config, no real game folder.
package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/scanner"
)

// writeFixture recreates the testdata/wow fixture layout in a temp
// AddOns directory: the same folders, TOC names and versions as
// internal/e2e's writeFixture.
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
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// newTestService wires a Service to a fresh config in a temp dir and a
// fake game root with the fixture tree. It returns the service and the
// AddOns path.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	addonsDir := filepath.Join(root, "Interface", "AddOns")
	writeFixture(t, addonsDir)

	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	cfg := config.Default()
	cfg.WoWPath = root
	cfg.Flavor = ""
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return New(store), addonsDir
}

func findIssue(addons []Addon, folder, kind string) bool {
	for _, a := range addons {
		if a.FolderName != folder {
			continue
		}
		for _, i := range a.Issues {
			if i.Kind == kind {
				return true
			}
		}
	}
	return false
}

// TestScanFindsProblems scans the fixture and expects every seeded
// problem to surface in the DTO.
func TestScanFindsProblems(t *testing.T) {
	s, _ := newTestService(t)
	res, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if res.Stats.Total < 5 {
		t.Fatalf("stats.total = %d, want >= 5", res.Stats.Total)
	}
	if res.Stats.Problems < 4 {
		t.Fatalf("stats.problems = %d, want >= 4", res.Stats.Problems)
	}

	for _, want := range []struct{ folder, kind string }{
		{"Questie-main", "github-name"},
		{"AuxUI", "toc-mismatch"},
		{"DPSMate", "nested"},
		{"Inventory", "missing-toc"},
		{"TempFolder", "empty"},
	} {
		if !findIssue(res.Addons, want.folder, want.kind) {
			t.Errorf("issue %q on %q not found", want.kind, want.folder)
		}
	}

	// Health score: 100 minus 30 per error issue, 15 per warn, 5 per info.
	// AtlasLoot is clean -> 100; Inventory carries a missing-toc error -> 70.
	for _, a := range res.Addons {
		switch a.FolderName {
		case "AtlasLoot":
			if a.Health != 100 {
				t.Errorf("AtlasLoot health = %d, want 100 (clean addon)", a.Health)
			}
		case "Inventory":
			if a.Health != 70 {
				t.Errorf("Inventory health = %d, want 70 (one error issue)", a.Health)
			}
		}
		if len(a.Issues) > 0 && a.Health >= 100 {
			t.Errorf("addon %q has %d issue(s) but health = %d, want < 100", a.FolderName, len(a.Issues), a.Health)
		}
	}
}

// TestFixAllRepairs fixes the fixture without destructive confirmations
// and expects the safe renames and the flatten to be applied on disk.
func TestFixAllRepairs(t *testing.T) {
	s, addonsDir := newTestService(t)
	if _, err := s.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	batch, err := s.FixAll(false)
	if err != nil {
		t.Fatalf("FixAll failed: %v", err)
	}
	if len(batch.Fixes) == 0 {
		t.Fatal("FixAll produced no results")
	}
	for _, f := range batch.Fixes {
		if f.Error != "" {
			t.Errorf("fix %s %s errored: %s", f.Addon, f.Action, f.Error)
		}
	}
	if batch.Fixed < 2 {
		t.Errorf("fixed = %d, want >= 2", batch.Fixed)
	}

	// Safe rename applied: AuxUI -> Aux-Classic.
	for _, name := range []string{"Questie", "Aux-Classic", "DPSMate"} {
		if _, err := os.Stat(filepath.Join(addonsDir, name)); err != nil {
			t.Errorf("addon %q missing after fix: %v", name, err)
		}
	}
	// Nested DPSMate promoted to top level, wrapper gone.
	if _, err := os.Stat(filepath.Join(addonsDir, "DPSMate-main")); !os.IsNotExist(err) {
		t.Errorf("wrapper DPSMate-main should be gone after flatten, stat err = %v", err)
	}
}

// TestValidateTable checks the compatibility table: one row per addon,
// expected interface from the profile, detected values from the TOCs.
func TestValidateTable(t *testing.T) {
	s, _ := newTestService(t)
	vr, err := s.Validate()
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if vr.ProfileID != "wrath" {
		t.Errorf("profile_id = %q, want wrath (default)", vr.ProfileID)
	}
	if vr.Expected != 30300 {
		t.Errorf("expected = %d, want 30300 (wrath)", vr.Expected)
	}
	if len(vr.Addons) != 7 {
		t.Fatalf("rows = %d, want one per fixture addon (7)", len(vr.Addons))
	}

	byName := map[string]Compat{}
	for _, c := range vr.Addons {
		if c.Expected != 30300 {
			t.Errorf("row %q expected = %d, want 30300", c.FolderName, c.Expected)
		}
		byName[c.FolderName] = c
	}
	if c := byName["AtlasLoot"]; c.Detected != 30300 || c.Status != "compatible" {
		t.Errorf("AtlasLoot row = %+v, want detected 30300 / compatible", c)
	}
	if c := byName["Inventory"]; c.TOC != "" || c.Detected != -1 || c.Status != "unknown" {
		t.Errorf("Inventory row = %+v, want empty toc / detected -1 / unknown", c)
	}
}

// TestGetStateStalePath saves a wow_path that does not exist and
// expects GetState to report the setup state (no install, stale path
// kept for the path picker) instead of failing the whole UI.
func TestGetStateStalePath(t *testing.T) {
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	cfg := config.Default()
	stale := filepath.Join(t.TempDir(), "missing")
	cfg.WoWPath = stale
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s := New(store)

	st, err := s.GetState()
	if err != nil {
		t.Fatalf("GetState failed for stale wow_path: %v", err)
	}
	if st.HasInstall {
		t.Fatalf("has_install = true, want false for stale path")
	}
	if st.WoWPath != stale {
		t.Fatalf("wow_path = %q, want stale value %q preserved for prefill", st.WoWPath, stale)
	}
	if st.ProfileID != "wrath" {
		t.Fatalf("profile_id = %q, want %q", st.ProfileID, "wrath")
	}
}

// TestGetStateValidPath checks the normal case: a resolvable wow_path
// yields the full install state with no error.
func TestGetStateValidPath(t *testing.T) {
	s, addonsDir := newTestService(t)
	st, err := s.GetState()
	if err != nil {
		t.Fatalf("GetState failed for valid path: %v", err)
	}
	if !st.HasInstall {
		t.Fatal("has_install = false, want true for valid path")
	}
	if st.AddonsDir != addonsDir {
		t.Errorf("addons_dir = %q, want %q", st.AddonsDir, addonsDir)
	}
	if st.ProfileID != "wrath" {
		t.Errorf("profile_id = %q, want wrath (default)", st.ProfileID)
	}
}

// TestInstallZip installs an archive built in memory and checks the
// folder lands on disk and is reported as installed.
func TestInstallZip(t *testing.T) {
	s, addonsDir := newTestService(t)
	zipPath := filepath.Join(t.TempDir(), "newaddon.zip")
	writeZip(t, zipPath, map[string]string{
		"NewAddon/NewAddon.toc": "## Interface: 30300\n## Title: NewAddon\n## Version: 1.0.0\n",
	})

	res, err := s.InstallZip(zipPath, true)
	if err != nil {
		t.Fatalf("InstallZip failed: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("install errors: %v", res.Errors)
	}
	if !slices.Contains(res.Installed, "NewAddon") {
		t.Errorf("installed = %v, want NewAddon", res.Installed)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "NewAddon")); err != nil {
		t.Errorf("NewAddon folder missing after install: %v", err)
	}
}

// rewriteTransport redirects api.github.com and codeload.github.com
// traffic to a mock server so the real GitHub provider never touches
// the network.
type rewriteTransport struct {
	mock string // mock origin "host:port"
	base http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Host {
	case "api.github.com", "codeload.github.com", "github.com":
	default:
		return nil, fmt.Errorf("test transport refuses non-GitHub host %s", req.URL.Host)
	}
	r := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = "http"
	u.Host = t.mock
	r.URL = &u
	return t.base.RoundTrip(r)
}

// mockGitHub serves the GitHub endpoints the real provider hits:
// repository metadata, latest releases and release zip assets.
type mockGitHub struct {
	repos   map[string]string // "owner/repo" -> latest release tag
	zips    map[string][]byte // "owner/repo" -> archive bytes for Download
	results []string          // "owner/repo" names returned by search
}

// client returns an http.Client whose GitHub traffic reaches only the
// mock.
func (m *mockGitHub) client(t *testing.T) *http.Client {
	t.Helper()
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search/repositories":
			// The provider first tries a topic-qualified query; reject
			// it so the plain query fallback is exercised.
			if strings.Contains(r.URL.Query().Get("q"), "topic:") {
				http.Error(w, "422 Unprocessable Entity", http.StatusUnprocessableEntity)
				return
			}
			q := strings.ToLower(r.URL.Query().Get("q"))
			var items []string
			for _, full := range m.results {
				owner, name, ok := strings.Cut(full, "/")
				if !ok || (q != "" && !strings.Contains(strings.ToLower(name), q)) {
					continue
				}
				items = append(items, fmt.Sprintf(
					`{"full_name":%q,"name":%q,"description":"","html_url":"https://github.com/%s","default_branch":"main","owner":{"login":%q}}`,
					full, name, full, owner))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"items":[%s]}`, strings.Join(items, ","))
		case strings.HasPrefix(r.URL.EscapedPath(), "/dl/"):
			id := strings.ReplaceAll(strings.TrimPrefix(r.URL.EscapedPath(), "/dl/"), "%2F", "/")
			data, ok := m.zips[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(data)
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			rest := strings.TrimPrefix(r.URL.Path, "/repos/")
			if id, ok := strings.CutSuffix(rest, "/releases/latest"); ok {
				tag, ok := m.repos[id]
				if !ok {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":"addon.zip","browser_download_url":"https://github.com/dl/%s"}]}`,
					tag, url.PathEscape(id))
				return
			}
			owner, name, ok := strings.Cut(rest, "/")
			if !ok {
				http.NotFound(w, r)
				return
			}
			if _, known := m.repos[rest]; !known {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"full_name":%q,"name":%q,"description":"","html_url":"https://github.com/%s","default_branch":"main","owner":{"login":%q}}`,
				rest, name, rest, owner)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return &http.Client{
		Transport: &rewriteTransport{mock: strings.TrimPrefix(ts.URL, "http://"), base: ts.Client().Transport},
	}
}

// addonZipBytes builds an addon archive in memory: one folder with a
// single TOC file.
func addonZipBytes(t *testing.T, folder, toc string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(folder + "/" + folder + ".toc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(toc)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newTestCatalogService wires a Service to a fake install, an
// isolated registry and a github-only catalog whose traffic goes to a
// mock GitHub server. It returns the service, the AddOns path, the
// registry path and the mock.
func newTestCatalogService(t *testing.T) (*Service, string, string, *mockGitHub) {
	t.Helper()
	root := t.TempDir()
	addonsDir := filepath.Join(root, "Interface", "AddOns")
	writeFixture(t, addonsDir)

	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	cfg := config.Default()
	cfg.WoWPath = root
	cfg.Flavor = ""
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s := New(store)
	s.registryPath = filepath.Join(t.TempDir(), "registry.json")
	s.enabledProviders = map[string]bool{catalog.ProviderGitHub: true}
	mock := &mockGitHub{repos: map[string]string{}, zips: map[string][]byte{}}
	s.httpClient = mock.client(t)
	return s, addonsDir, s.registryPath, mock
}

// TestSearchCatalog checks the search wiring: mock GitHub results
// arrive in the DTO shape.
func TestSearchCatalog(t *testing.T) {
	s, _, _, mock := newTestCatalogService(t)
	mock.results = []string{"xperl/xperl", "flux/flux"}

	res, err := s.SearchCatalog("x")
	if err != nil {
		t.Fatalf("SearchCatalog: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results = %d, want 2: %+v", len(res.Results), res.Results)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %v, want none", res.Errors)
	}
	byName := map[string]SearchHit{}
	for _, h := range res.Results {
		byName[h.Name] = h
	}
	h, ok := byName["xperl"]
	if !ok {
		t.Fatalf("xperl missing from results: %+v", res.Results)
	}
	if h.Provider != "github" || h.ID != "xperl/xperl" || h.Author != "xperl" ||
		h.Homepage != "https://github.com/xperl/xperl" {
		t.Errorf("xperl row = %+v", h)
	}
}

// TestInstallSource installs a real archive through the github
// provider's Download and checks the folder lands on disk and is
// tracked in the registry.
func TestInstallSource(t *testing.T) {
	s, addonsDir, regPath, mock := newTestCatalogService(t)
	mock.repos["acme/newaddon"] = "v1.0.0"
	mock.zips["acme/newaddon"] = addonZipBytes(t, "NewAddon", "## Title: NewAddon\n## Version: 1.0.0\n## Interface: 30300\n")

	res, err := s.InstallSource("acme/newaddon", true)
	if err != nil {
		t.Fatalf("InstallSource: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %v, want none", res.Errors)
	}
	if !slices.Contains(res.Installed, "NewAddon") {
		t.Errorf("installed = %v, want NewAddon", res.Installed)
	}
	if len(res.Replaced) != 0 || len(res.Skipped) != 0 {
		t.Errorf("replaced/skipped should stay empty, got %v/%v", res.Replaced, res.Skipped)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "NewAddon", "NewAddon.toc")); err != nil {
		t.Fatalf("installed TOC missing: %v", err)
	}

	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("registry entries = %d, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Folder != "NewAddon" || e.Provider != "github" || e.ID != "acme/newaddon" {
		t.Errorf("entry = %+v", e)
	}
	if e.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0 (read from the installed TOC)", e.Version)
	}
	if e.Source != "acme/newaddon" {
		t.Errorf("source = %q, want acme/newaddon", e.Source)
	}
}

// TestCheckUpdates reports an update whose latest version bumps the
// tracked one.
func TestCheckUpdates(t *testing.T) {
	s, _, regPath, mock := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Track(catalog.Entry{
		Folder: "Alpha", Title: "Alpha", Version: "1.0.0",
		Provider: "github", ID: "acme/alpha", Source: "acme/alpha",
	}); err != nil {
		t.Fatal(err)
	}
	mock.repos["acme/alpha"] = "v2.0.0"

	res, err := s.CheckUpdates()
	if err != nil {
		t.Fatalf("CheckUpdates: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %v, want none", res.Errors)
	}
	if len(res.Updates) != 1 {
		t.Fatalf("updates = %d, want 1: %+v", len(res.Updates), res.Updates)
	}
	u := res.Updates[0]
	if u.Folder != "Alpha" || u.CurrentVersion != "1.0.0" || u.LatestVersion != "v2.0.0" ||
		u.Provider != "github" || u.ID != "acme/alpha" || u.Source != "acme/alpha" {
		t.Errorf("update = %+v", u)
	}
	if u.FlavorMismatch {
		t.Errorf("flavor_mismatch = true, want false (no game version in repo metadata)")
	}
	if _, err := time.Parse(time.RFC3339, res.CheckedAt); err != nil {
		t.Errorf("checked_at %q is not RFC3339: %v", res.CheckedAt, err)
	}
}

// TestCheckUpdatesPartialFailure keeps healthy updates when another
// entry's provider lookup fails.
func TestCheckUpdatesPartialFailure(t *testing.T) {
	s, _, regPath, mock := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{Folder: "Alpha", Title: "Alpha", Version: "1.0.0", Provider: "github", ID: "acme/alpha"})
	_ = reg.Track(catalog.Entry{Folder: "Broken", Title: "Broken", Version: "1.0.0", Provider: "github", ID: "acme/broken"})
	mock.repos["acme/alpha"] = "v2.0.0"
	// acme/broken has no repository metadata: the lookup fails.

	res, err := s.CheckUpdates()
	if err != nil {
		t.Fatalf("CheckUpdates: %v", err)
	}
	if len(res.Updates) != 1 || res.Updates[0].Folder != "Alpha" {
		t.Fatalf("updates = %+v, want only Alpha", res.Updates)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("errors = %v, want the Broken lookup failure", res.Errors)
	}
}

// TestApplyUpdate applies one pending update and checks the folder
// and registry are refreshed.
func TestApplyUpdate(t *testing.T) {
	s, addonsDir, regPath, mock := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Track(catalog.Entry{
		Folder: "Questie", Title: "Questie", Version: "9.0.0",
		Provider: "github", ID: "acme/questie", Source: "acme/questie",
	}); err != nil {
		t.Fatal(err)
	}
	mock.repos["acme/questie"] = "v9.2.0"
	mock.zips["acme/questie"] = addonZipBytes(t, "Questie", "## Title: Questie\n## Version: 9.2.0\n## Interface: 30300\n")

	batch, err := s.ApplyUpdate("Questie", true)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if batch.AppliedCount != 1 || batch.FailedCount != 0 || len(batch.Applied) != 1 {
		t.Fatalf("batch = %+v, want 1 applied / 0 failed", batch)
	}
	if a := batch.Applied[0]; !a.OK || a.Folder != "Questie" || a.Error != "" {
		t.Errorf("applied entry = %+v", batch.Applied[0])
	}
	toc, err := os.ReadFile(filepath.Join(addonsDir, "Questie", "Questie.toc"))
	if err != nil {
		t.Fatalf("read updated TOC: %v", err)
	}
	if !strings.Contains(string(toc), "## Version: 9.2.0") {
		t.Errorf("Questie TOC not updated: %s", toc)
	}
	reg, err = catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if entries := reg.Entries(); len(entries) != 1 || entries[0].Version != "9.2.0" {
		t.Errorf("registry after update = %+v", entries)
	}
}

// TestApplyUpdateDeclinesReplace skips the update when allowReplace
// is false and the folder exists.
func TestApplyUpdateDeclinesReplace(t *testing.T) {
	s, _, regPath, mock := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{
		Folder: "Questie", Title: "Questie", Version: "9.0.0",
		Provider: "github", ID: "acme/questie", Source: "acme/questie",
	})
	mock.repos["acme/questie"] = "v9.2.0"

	batch, err := s.ApplyUpdate("Questie", false)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if batch.FailedCount != 1 || len(batch.Applied) != 1 {
		t.Fatalf("batch = %+v, want 1 failed", batch)
	}
	a := batch.Applied[0]
	if a.OK {
		t.Error("entry should not be applied")
	}
	if a.Message != "folder already exists, replace declined" {
		t.Errorf("message = %q, want the replace-declined message", a.Message)
	}
}

// TestApplyUpdateNotFound reports a failed entry with a clear message
// when no update matches the folder.
func TestApplyUpdateNotFound(t *testing.T) {
	s, _, _, _ := newTestCatalogService(t)
	batch, err := s.ApplyUpdate("Missing", true)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if batch.FailedCount != 1 || len(batch.Applied) != 1 {
		t.Fatalf("batch = %+v, want 1 failed", batch)
	}
	if batch.Applied[0].OK {
		t.Error("entry should not be applied")
	}
	if !strings.Contains(batch.Applied[0].Message, "Missing") {
		t.Errorf("message = %q, want it to name the folder", batch.Applied[0].Message)
	}
}

// TestScanReportsTrackedDrifted seeds the registry with a checksummed
// entry for a fixture addon and checks the scan DTO reports it
// tracked and clean, then drifted once a file changes. Entries without
// a checksum baseline (pre-integrity installs) stay tracked but never
// drift.
func TestScanReportsTrackedDrifted(t *testing.T) {
	s, addonsDir, regPath, _ := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := catalog.ComputeManifest(filepath.Join(addonsDir, "AtlasLoot"))
	if err != nil {
		t.Fatalf("ComputeManifest: %v", err)
	}
	if err := reg.Track(catalog.Entry{
		Folder: "AtlasLoot", Title: "AtlasLoot", Version: "7.0.4",
		Provider: "github", ID: "acme/atlasloot", Source: "acme/atlasloot",
		Checksum: sum,
	}); err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{
		Folder: "Questie", Title: "Questie", Version: "1.12.2",
		Provider: "github", ID: "acme/questie", Source: "acme/questie",
	})

	byFolder := func() map[string]Addon {
		t.Helper()
		res, err := s.Scan()
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		out := map[string]Addon{}
		for _, a := range res.Addons {
			out[a.FolderName] = a
		}
		return out
	}

	clean := byFolder()
	atlas := clean["AtlasLoot"]
	if !atlas.Tracked || atlas.Drifted || atlas.TrackedSource != "acme/atlasloot" {
		t.Errorf("AtlasLoot = %+v, want tracked / not drifted / source acme/atlasloot", atlas)
	}
	questie := clean["Questie"]
	if !questie.Tracked || questie.Drifted {
		t.Errorf("Questie = %+v, want tracked without a checksum baseline, never drifted", questie)
	}
	aux := clean["AuxUI"]
	if aux.Tracked || aux.Drifted || aux.TrackedSource != "" {
		t.Errorf("AuxUI = %+v, want untracked", aux)
	}

	// Touching a file inside the tracked folder flips Drifted on the
	// next scan; the checksum-less entry is unaffected.
	toc := filepath.Join(addonsDir, "AtlasLoot", "AtlasLoot.toc")
	f, err := os.OpenFile(toc, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n-- tampered\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dirty := byFolder()
	if a := dirty["AtlasLoot"]; !a.Tracked || !a.Drifted {
		t.Errorf("AtlasLoot after edit = %+v, want tracked and drifted", a)
	}
	if a := dirty["Questie"]; a.Drifted {
		t.Errorf("Questie without checksum baseline = %+v, want not drifted", a)
	}
}

// TestRestoreAddon restores a tracked addon from its recorded source
// through the mock GitHub provider and checks the folder and registry
// are refreshed; an untracked folder lands in the DTO errors with a
// nil Go error.
func TestRestoreAddon(t *testing.T) {
	s, addonsDir, regPath, mock := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Track(catalog.Entry{
		Folder: "Questie", Title: "Questie", Version: "1.12.2",
		Provider: "github", ID: "acme/questie", Source: "acme/questie",
	}); err != nil {
		t.Fatal(err)
	}
	mock.repos["acme/questie"] = "v1.13.0"
	mock.zips["acme/questie"] = addonZipBytes(t, "Questie", "## Title: Questie\n## Version: 1.13.0\n## Interface: 30300\n")

	// Untracked folder: reported as an error in the DTO, nil Go error.
	missing, err := s.RestoreAddon("AtlasLoot", true)
	if err != nil {
		t.Fatalf("RestoreAddon(untracked): %v", err)
	}
	if !slices.Contains(missing.Errors, "addon not tracked in registry") {
		t.Errorf("errors = %v, want the not-tracked message", missing.Errors)
	}

	res, err := s.RestoreAddon("Questie", true)
	if err != nil {
		t.Fatalf("RestoreAddon: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %v, want none", res.Errors)
	}
	if !slices.Contains(res.Installed, "Questie") {
		t.Errorf("installed = %v, want Questie", res.Installed)
	}
	toc, err := os.ReadFile(filepath.Join(addonsDir, "Questie", "Questie.toc"))
	if err != nil {
		t.Fatalf("read restored TOC: %v", err)
	}
	if !strings.Contains(string(toc), "## Version: 1.13.0") {
		t.Errorf("Questie TOC not restored: %s", toc)
	}

	// The catalog re-records the manifest checksum after the restore.
	reg, err = catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("registry entries = %d, want 1: %+v", len(entries), entries)
	}
	got, err := catalog.ComputeManifest(filepath.Join(addonsDir, "Questie"))
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Checksum != got {
		t.Errorf("recorded checksum %q != computed %q", entries[0].Checksum, got)
	}
}

// collectionTestService wires a Service to a temp config with an
// install whose AddOns dir holds the given folder names, plus a temp
// collections dir via cfg.CollectionsDir.
func collectionTestService(t *testing.T, folders ...string) (*Service, string) {
	t.Helper()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	cfg := config.Default()
	root := t.TempDir()
	addonsDir := filepath.Join(root, "Interface", "AddOns")
	for _, name := range folders {
		if err := os.MkdirAll(filepath.Join(addonsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg.WoWPath = root
	cfg.Flavor = ""
	cfg.CollectionsDir = filepath.Join(t.TempDir(), "collections")
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return New(store), addonsDir
}

// TestCollectionsLifecycle walks the collection CRUD surface: create
// snapshots the on-disk folders (enabled and .disabled both counted),
// list reports one inactive collection, SetCollectionAddon /
// CollectionDetail round-trip the toggle, and delete empties the list.
func TestCollectionsLifecycle(t *testing.T) {
	s, _ := collectionTestService(t, "A", "A.disabled")

	created, err := s.CreateCollection("pve")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if created.Active {
		t.Error("created collection must not be active")
	}
	if created.AddonCount != 2 {
		t.Errorf("addon_count = %d, want 2 (A enabled + A.disabled)", created.AddonCount)
	}

	res, err := s.Collections()
	if err != nil {
		t.Fatalf("Collections: %v", err)
	}
	if res.ActiveID != "" {
		t.Errorf("active_id = %q, want empty (create must not activate)", res.ActiveID)
	}
	if len(res.Collections) != 1 {
		t.Fatalf("collections = %d, want 1", len(res.Collections))
	}
	c := res.Collections[0]
	if c.ID != created.ID || c.Name != "pve" || c.Active || c.AddonCount != 2 {
		t.Errorf("collection = %+v, want id/name %q/%q, active=false, addon_count=2",
			c, created.ID, "pve")
	}

	// Toggle the shared folder off: every recorded entry turns disabled.
	if err := s.SetCollectionAddon(created.ID, "A", false); err != nil {
		t.Fatalf("SetCollectionAddon(false): %v", err)
	}
	detail, err := s.CollectionDetail(created.ID)
	if err != nil {
		t.Fatalf("CollectionDetail: %v", err)
	}
	if detail.ID != created.ID || detail.Name != "pve" || len(detail.Addons) != 2 {
		t.Fatalf("detail = %+v, want 2 addon rows", detail)
	}
	for _, a := range detail.Addons {
		if a.Folder != "A" {
			t.Errorf("addon folder = %q, want A", a.Folder)
		}
		if a.Enabled {
			t.Errorf("addon %q still enabled after SetCollectionAddon(false)", a.Folder)
		}
	}

	// Toggle back on: at least the first entry flips to enabled.
	if err := s.SetCollectionAddon(created.ID, "A", true); err != nil {
		t.Fatalf("SetCollectionAddon(true): %v", err)
	}
	detail, err = s.CollectionDetail(created.ID)
	if err != nil {
		t.Fatalf("CollectionDetail after toggle: %v", err)
	}
	enabled := 0
	for _, a := range detail.Addons {
		if a.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		t.Error("no enabled addons after SetCollectionAddon(true)")
	}

	if err := s.DeleteCollection(created.ID); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	after, err := s.Collections()
	if err != nil {
		t.Fatalf("Collections after delete: %v", err)
	}
	if len(after.Collections) != 0 {
		t.Errorf("collections after delete = %d, want 0", len(after.Collections))
	}
}

// TestSwitchCollection activates a collection and verifies the folder
// renames on disk, the applied list, and that the active collection id
// is persisted in the saved config. The second switch exercises the
// reverse rename (A.disabled -> A) with a collection that wants the
// addon enabled.
func TestSwitchCollection(t *testing.T) {
	s, addonsDir := collectionTestService(t, "A")

	created, err := s.CreateCollection("pve")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	// On-disk state is A (enabled); record the addon as disabled so
	// the switch renames A -> A.disabled.
	if err := s.SetCollectionAddon(created.ID, "A", false); err != nil {
		t.Fatalf("SetCollectionAddon: %v", err)
	}

	res, err := s.SwitchCollection(created.ID)
	if err != nil {
		t.Fatalf("SwitchCollection: %v", err)
	}
	if !slices.Contains(res.Applied, "A") {
		t.Errorf("applied = %v, want A", res.Applied)
	}
	if res.Message == "" {
		t.Error("message must not be empty")
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "A")); !os.IsNotExist(err) {
		t.Errorf("folder A still present after switch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "A.disabled")); err != nil {
		t.Errorf("folder A.disabled missing after switch: %v", err)
	}

	// The active collection id is persisted in the saved config.
	loaded, err := s.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Collection != created.ID {
		t.Errorf("cfg.collection = %q, want %q", loaded.Collection, created.ID)
	}

	// Now the disk holds A.disabled. A second collection that wants the
	// addon enabled must rename it back.
	back, err := s.CreateCollection("pve-alt")
	if err != nil {
		t.Fatalf("CreateCollection(pve-alt): %v", err)
	}
	if err := s.SetCollectionAddon(back.ID, "A", true); err != nil {
		t.Fatalf("SetCollectionAddon(back): %v", err)
	}
	res, err = s.SwitchCollection(back.ID)
	if err != nil {
		t.Fatalf("second SwitchCollection: %v", err)
	}
	if !slices.Contains(res.Applied, "A") {
		t.Errorf("second applied = %v, want A", res.Applied)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "A")); err != nil {
		t.Errorf("folder A missing after switch back: %v", err)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "A.disabled")); !os.IsNotExist(err) {
		t.Errorf("folder A.disabled still present after switch back: %v", err)
	}
}

// TestInstallsStatus checks the per-install DTO mapping through the
// private seam (AutoDetect cannot be pointed at temp dirs): a
// hand-built installation over the fixture AddOns dir yields the same
// counts as a direct scan and the average addon health, and a missing
// AddOns dir stays exists=false with zeroed counts.
func TestInstallsStatus(t *testing.T) {
	s, addonsDir := newTestService(t)
	root := filepath.Dir(filepath.Dir(addonsDir))
	profile := models.DefaultProfile()

	inst := &detector.Installation{
		Root:       root,
		Flavor:     "",
		AddonsPath: addonsDir,
		Exe:        "Wow.exe",
		Version:    "3.4.3",
		ProfileID:  "wrath",
		Confidence: "high",
	}
	st := s.statusForInstall(inst, profile)
	if !st.Exists {
		t.Fatal("exists = false, want true for the fixture addons dir")
	}
	if st.Exe != "Wow.exe" || st.Version != "3.4.3" || st.ProfileID != "wrath" || st.Confidence != "high" {
		t.Errorf("identity fields not copied: %+v", st)
	}

	// Counts must match an independently run scan of the same dir.
	direct, err := scanner.New(addonsDir, profile).Scan(context.Background())
	if err != nil {
		t.Fatalf("direct scan: %v", err)
	}
	wantTotal, wantProblems, wantErrors := direct.Stats()
	if st.Addons != wantTotal || st.Problems != wantProblems || st.Errors != wantErrors {
		t.Errorf("counts = %d/%d/%d, want scan stats %d/%d/%d",
			st.Addons, st.Problems, st.Errors, wantTotal, wantProblems, wantErrors)
	}
	if st.Errors == 0 {
		t.Error("errors = 0, want the fixture's missing-TOC errors")
	}
	wantHealth := 0
	for _, a := range direct.Addons {
		wantHealth += addonHealth(a)
	}
	wantHealth /= wantTotal
	if st.Health != wantHealth {
		t.Errorf("health = %d, want %d (average addonHealth over the scan)", st.Health, wantHealth)
	}

	// Missing AddOns dir: exists false, zeroed counts, zero health.
	missing := &detector.Installation{Root: root, AddonsPath: filepath.Join(root, "Interface", "Missing")}
	st2 := s.statusForInstall(missing, profile)
	if st2.Exists {
		t.Error("exists = true, want false for a missing addons dir")
	}
	if st2.Addons != 0 || st2.Problems != 0 || st2.Errors != 0 || st2.Health != 0 {
		t.Errorf("missing install counts = %+v, want all zero", st2)
	}
}

// TestSyncUpdatesToAll iterates every install with an existing AddOns
// dir, applies the pending update to each, and aggregates the totals;
// an install without an AddOns dir is skipped.
func TestSyncUpdatesToAll(t *testing.T) {
	s, addonsDir, regPath, mock := newTestCatalogService(t)
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Track(catalog.Entry{
		Folder: "Questie", Title: "Questie", Version: "9.0.0",
		Provider: "github", ID: "acme/questie", Source: "acme/questie",
	}); err != nil {
		t.Fatal(err)
	}
	mock.repos["acme/questie"] = "v9.2.0"
	mock.zips["acme/questie"] = addonZipBytes(t, "Questie", "## Title: Questie\n## Version: 9.2.0\n## Interface: 30300\n")

	e, err := s.requireInstall()
	if err != nil {
		t.Fatalf("requireInstall: %v", err)
	}

	// Second install: an existing but empty AddOns dir. Third: no
	// AddOns dir at all (must be skipped).
	other := filepath.Join(t.TempDir(), "Interface", "AddOns")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(addonsDir))
	otherRoot := filepath.Dir(filepath.Dir(other))
	installs := []detector.Installation{
		{Root: root, Flavor: "", AddonsPath: addonsDir, ProfileID: "wrath", Confidence: "high"},
		{Root: otherRoot, Flavor: "", AddonsPath: other, ProfileID: "wrath", Confidence: "medium"},
		{Root: otherRoot, Flavor: "", AddonsPath: filepath.Join(other, "missing"), ProfileID: "wrath", Confidence: "medium"},
	}

	res := s.syncInstalls(e, installs, true)
	if len(res.Installs) != 2 {
		t.Fatalf("rows = %d, want 2 (missing install skipped): %+v", len(res.Installs), res.Installs)
	}
	// Both installs were checked against the same pre-apply registry
	// baseline (two-pass: check all, then apply all), so the tracked
	// addon is updated in each of them.
	if res.TotalUpdated != 2 || res.TotalFailed != 0 {
		t.Errorf("totals = %d updated / %d failed, want 2 / 0", res.TotalUpdated, res.TotalFailed)
	}
	for i, row := range res.Installs {
		if row.Updated != 1 || row.Failed != 0 || len(row.Errors) != 0 {
			t.Errorf("row %d = %+v, want 1 updated / 0 failed / no errors", i, row)
		}
	}

	// Both install directories received the updated TOC.
	for _, dir := range []string{addonsDir, other} {
		toc, err := os.ReadFile(filepath.Join(dir, "Questie", "Questie.toc"))
		if err != nil {
			t.Fatalf("read updated TOC in %s: %v", dir, err)
		}
		if !strings.Contains(string(toc), "## Version: 9.2.0") {
			t.Errorf("Questie TOC not updated in %s: %s", dir, toc)
		}
	}
}

// TestSyncUpdatesToAllCatalogError keeps processing every install when
// one's catalog cannot be resolved: the failure lands in the row's
// errors with zero counts and never aborts the loop.
func TestSyncUpdatesToAllCatalogError(t *testing.T) {
	s, addonsDir, regPath, _ := newTestCatalogService(t)
	// A corrupt registry makes catalogFor fail for every install.
	if err := os.WriteFile(regPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := s.requireInstall()
	if err != nil {
		t.Fatalf("requireInstall: %v", err)
	}
	root := filepath.Dir(filepath.Dir(addonsDir))
	installs := []detector.Installation{
		{Root: root, Flavor: "", AddonsPath: addonsDir, ProfileID: "wrath", Confidence: "high"},
		{Root: root, Flavor: "", AddonsPath: addonsDir, ProfileID: "wrath", Confidence: "high"},
	}

	res := s.syncInstalls(e, installs, true)
	if len(res.Installs) != 2 {
		t.Fatalf("rows = %d, want 2 (loop must not abort on the first failure)", len(res.Installs))
	}
	for _, row := range res.Installs {
		if row.Updated != 0 || row.Failed != 0 {
			t.Errorf("row = %+v, want 0 updated / 0 failed on a catalog error", row)
		}
		if len(row.Errors) != 1 || !strings.Contains(row.Errors[0], "corrupt") {
			t.Errorf("row errors = %v, want the corrupt-registry message", row.Errors)
		}
	}
	if res.TotalUpdated != 0 || res.TotalFailed != 0 {
		t.Errorf("totals = %d updated / %d failed, want 0 / 0", res.TotalUpdated, res.TotalFailed)
	}
}

// newTrackedTestService wires a Service to a fake install, an isolated
// registry, and an explicit backups directory (so the backup root does
// not depend on the detector). It returns the service, the AddOns
// path, the registry path and the backups directory.
func newTrackedTestService(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	root := t.TempDir()
	addonsDir := filepath.Join(root, "Interface", "AddOns")
	writeFixture(t, addonsDir)

	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	cfg := config.Default()
	cfg.WoWPath = root
	cfg.Flavor = ""
	backupsDir := filepath.Join(t.TempDir(), "backups")
	cfg.BackupsDir = backupsDir
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s := New(store)
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	s.registryPath = registryPath
	return s, addonsDir, registryPath, backupsDir
}

func TestTrackedAddons(t *testing.T) {
	s, _, registryPath, _ := newTrackedTestService(t)

	// Empty registry: empty list, nil error.
	res, err := s.TrackedAddons()
	if err != nil {
		t.Fatalf("TrackedAddons on empty registry: %v", err)
	}
	if len(res.Addons) != 0 {
		t.Fatalf("addons = %d, want 0", len(res.Addons))
	}

	reg, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{Folder: "Questie-main", Title: "Questie", Version: "1.12.2", Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie", Pinned: true})
	_ = reg.Track(catalog.Entry{Folder: "AtlasLoot", Title: "AtlasLoot", Version: "7.0.4", Provider: "curseforge", ID: "atlasloot", Source: "https://www.curseforge.com/wow/addons/atlasloot", Ignored: true})

	res, err = s.TrackedAddons()
	if err != nil {
		t.Fatalf("TrackedAddons: %v", err)
	}
	if len(res.Addons) != 2 {
		t.Fatalf("addons = %d, want 2: %+v", len(res.Addons), res.Addons)
	}
	// Sorted by folder: AtlasLoot first.
	first, second := res.Addons[0], res.Addons[1]
	if first.Folder != "AtlasLoot" || second.Folder != "Questie-main" {
		t.Errorf("order = %s, %s; want AtlasLoot, Questie-main", first.Folder, second.Folder)
	}
	if !first.Ignored || first.Pinned {
		t.Errorf("AtlasLoot flags = pinned %v / ignored %v, want ignored only", first.Pinned, first.Ignored)
	}
	if !second.Pinned || second.Ignored {
		t.Errorf("Questie-main flags = pinned %v / ignored %v, want pinned only", second.Pinned, second.Ignored)
	}
	if second.Provider != "github" || second.ID != "Questie/Questie" || second.Source != "Questie/Questie" {
		t.Errorf("Questie-main entry = %+v", second)
	}
	if _, err := time.Parse(time.RFC3339, second.InstalledAt); err != nil {
		t.Errorf("InstalledAt %q is not RFC3339: %v", second.InstalledAt, err)
	}
}

func TestSetAddonPinnedAndIgnored(t *testing.T) {
	s, _, registryPath, _ := newTrackedTestService(t)
	reg, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{Folder: "Questie-main", Title: "Questie", Version: "1.12.2", Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie"})

	// Case-insensitive folder matching.
	if err := s.SetAddonPinned("questie-main", true); err != nil {
		t.Fatalf("SetAddonPinned: %v", err)
	}
	if err := s.SetAddonIgnored("Questie-main", true); err != nil {
		t.Fatalf("SetAddonIgnored: %v", err)
	}

	reloaded, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := reloaded.Entries()
	if len(entries) != 1 || !entries[0].Pinned || !entries[0].Ignored {
		t.Errorf("flags not persisted: %+v", entries)
	}

	// Unknown folder is a Go error.
	if err := s.SetAddonPinned("Nope", true); err == nil {
		t.Fatal("SetAddonPinned on untracked folder should error")
	} else if !strings.Contains(err.Error(), "not tracked in the registry") {
		t.Errorf("error = %q", err.Error())
	}
}

// TestRollbackAddon runs the full rollback flow: a snapshot of the
// folder exists, the folder is modified, RollbackAddon restores the
// snapshot content, refreshes the registry entry (TOC version and
// checksum) and pins it.
func TestRollbackAddon(t *testing.T) {
	s, addonsDir, registryPath, backupsDir := newTrackedTestService(t)
	reg, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{
		Folder: "Questie-main", Title: "Questie", Version: "9.9.9",
		Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie", Checksum: "stale",
	})

	folder := filepath.Join(addonsDir, "Questie-main")
	backups := backup.New(backupsDir, nil)
	// Seed the snapshot with the fixture content (TOC version 1.12.2).
	if _, err := backups.Backup([]string{folder}, "seed"); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	// Modify the folder: bump the TOC, add a file.
	tocPath := filepath.Join(folder, "Questie.toc")
	data, err := os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	modified := strings.Replace(string(data), "## Version: 1.12.2", "## Version: 9.9.9", 1)
	if err := os.WriteFile(tocPath, []byte(modified), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "extra.lua"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.RollbackAddon("questie-main") // case-insensitive folder
	if err != nil {
		t.Fatalf("RollbackAddon: %v", err)
	}
	if res.Folder != "Questie-main" {
		t.Errorf("folder = %q, want Questie-main", res.Folder)
	}
	if res.RestoredFrom == "" {
		t.Error("RestoredFrom is empty")
	}
	if res.Version != "1.12.2" {
		t.Errorf("version = %q, want 1.12.2 (read back from the restored TOC)", res.Version)
	}
	if !res.Pinned {
		t.Error("Pinned = false, want true (rollback auto-pins)")
	}

	// Old content is back, the added file is gone.
	restored, err := os.ReadFile(tocPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), "## Version: 1.12.2") {
		t.Errorf("restored TOC = %q, want 1.12.2", restored)
	}
	if _, err := os.Stat(filepath.Join(folder, "extra.lua")); !os.IsNotExist(err) {
		t.Error("file added after the snapshot still present")
	}

	// Registry entry refreshed and pinned (reload: RollbackAddon saves
	// through its own registry instance).
	reloaded, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := reloaded.Entries()
	if len(entries) != 1 {
		t.Fatalf("registry entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if !e.Pinned {
		t.Errorf("entry not pinned: %+v", e)
	}
	if e.Version != "1.12.2" {
		t.Errorf("entry version = %q, want 1.12.2", e.Version)
	}
	want, err := catalog.ComputeManifest(folder)
	if err != nil {
		t.Fatal(err)
	}
	if e.Checksum != want {
		t.Errorf("entry checksum = %q, want recomputed %q", e.Checksum, want)
	}
	if e.Provider != "github" || e.ID != "Questie/Questie" || e.Source != "Questie/Questie" {
		t.Errorf("entry lost its provider/source: %+v", e)
	}

	// The pre-rollback snapshot of the modified state exists.
	snapshots, err := backups.List()
	if err != nil {
		t.Fatal(err)
	}
	foundPre := false
	for _, sn := range snapshots {
		if strings.HasPrefix(sn.Reason, "pre-rollback of ") {
			foundPre = true
		}
	}
	if !foundPre {
		t.Error("no pre-rollback snapshot of the destination")
	}
}

func TestRollbackAddonUntracked(t *testing.T) {
	s, _, _, _ := newTrackedTestService(t)
	_, err := s.RollbackAddon("Nope")
	if err == nil {
		t.Fatal("RollbackAddon on an untracked folder should error")
	}
	if !strings.Contains(err.Error(), `addon "Nope" not tracked in registry`) {
		t.Errorf("error = %q, want not-tracked message", err.Error())
	}
}

func TestRollbackAddonNoSnapshot(t *testing.T) {
	s, _, registryPath, _ := newTrackedTestService(t)
	reg, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{Folder: "Questie-main", Title: "Questie", Version: "1.12.2", Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie"})

	_, err = s.RollbackAddon("Questie-main")
	if err == nil {
		t.Fatal("RollbackAddon with no snapshot should error")
	}
	if !strings.Contains(err.Error(), "no backup snapshot contains") {
		t.Errorf("error = %q, want no-snapshot message", err.Error())
	}
}

// TestScanReportsPinAndIgnore checks the scan DTO enrichment: tracked
// addons carry their registry flags, untracked ones report false.
func TestScanReportsPinAndIgnore(t *testing.T) {
	s, _, registryPath, _ := newTrackedTestService(t)
	reg, err := catalog.NewRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.Track(catalog.Entry{Folder: "Questie-main", Title: "Questie", Version: "1.12.2", Provider: "github", ID: "Questie/Questie", Source: "Questie/Questie", Pinned: true})
	_ = reg.Track(catalog.Entry{Folder: "AtlasLoot", Title: "AtlasLoot", Version: "7.0.4", Provider: "curseforge", ID: "atlasloot", Source: "https://www.curseforge.com/wow/addons/atlasloot", Ignored: true})

	res, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byFolder := map[string]Addon{}
	for _, a := range res.Addons {
		byFolder[a.FolderName] = a
	}
	questie, ok := byFolder["Questie-main"]
	if !ok {
		t.Fatal("Questie-main not in scan results")
	}
	if !questie.Tracked || !questie.Pinned || questie.Ignored {
		t.Errorf("Questie-main = tracked %v / pinned %v / ignored %v, want tracked+pinned only",
			questie.Tracked, questie.Pinned, questie.Ignored)
	}
	atlas, ok := byFolder["AtlasLoot"]
	if !ok {
		t.Fatal("AtlasLoot not in scan results")
	}
	if !atlas.Tracked || atlas.Pinned || !atlas.Ignored {
		t.Errorf("AtlasLoot = tracked %v / pinned %v / ignored %v, want tracked+ignored only",
			atlas.Tracked, atlas.Pinned, atlas.Ignored)
	}
	aux, ok := byFolder["AuxUI"] // untracked
	if !ok {
		t.Fatal("AuxUI not in scan results")
	}
	if aux.Tracked || aux.Pinned || aux.Ignored {
		t.Errorf("AuxUI (untracked) = tracked %v / pinned %v / ignored %v, want all false",
			aux.Tracked, aux.Pinned, aux.Ignored)
	}
}
