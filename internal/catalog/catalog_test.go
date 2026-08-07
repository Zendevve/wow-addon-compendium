package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeProvider implements Provider with canned data for catalog-level
// tests.
type fakeProvider struct {
	name   string
	addons []*Addon
	err    error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*Addon
	for _, a := range f.addons {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(query)) {
			out = append(out, a)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	for _, a := range f.addons {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (f *fakeProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	return f.Resolve(ctx, addon.ID)
}

func (f *fakeProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	return errors.New("unused in this test")
}

func TestNewEnabledProviders(t *testing.T) {
	c, err := New(map[string]bool{ProviderGitHub: true}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := c.Provider(ProviderGitHub); !ok {
		t.Error("github should be enabled")
	}
	for _, name := range []string{ProviderCurseForge, ProviderWowInterface, ProviderTukui} {
		if _, ok := c.Provider(name); ok {
			t.Errorf("%s should be disabled", name)
		}
	}

	c2, err := New(nil, nil)
	if err != nil {
		t.Fatalf("New(nil): %v", err)
	}
	for _, name := range []string{ProviderGitHub, ProviderCurseForge, ProviderWowInterface, ProviderTukui} {
		if _, ok := c2.Provider(name); !ok {
			t.Errorf("%s should be enabled with a nil map", name)
		}
	}
}

func TestSearchMergeDedupeSort(t *testing.T) {
	github := &fakeProvider{name: ProviderGitHub, addons: []*Addon{
		{Provider: ProviderGitHub, ID: "a/questie", Name: "Questie", LatestVersion: "9.2.0"},
		{Provider: ProviderGitHub, ID: "a/atlas", Name: "Atlas", LatestVersion: "1.2.3"},
	}}
	curse := &fakeProvider{name: ProviderCurseForge, addons: []*Addon{
		{Provider: ProviderCurseForge, ID: "123", Name: "Questie", LatestVersion: "9.3.0"},
		{Provider: ProviderCurseForge, ID: "456", Name: "DBM", LatestVersion: "1.0.0"},
		{Provider: ProviderCurseForge, ID: "789", Name: "dbm", LatestVersion: "1.1.0"},
	}}
	c := &Catalog{providers: map[string]Provider{
		ProviderGitHub:     github,
		ProviderCurseForge: curse,
	}}

	res, err := c.Search(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3 (deduped): %s", len(res), addonNames(res))
	}
	want := []string{"Atlas", "DBM", "Questie"}
	for i, w := range want {
		if res[i].Name != w {
			t.Errorf("result[%d] = %q, want %q (sorted by name)", i, res[i].Name, w)
		}
	}
	// First hit wins: curseforge sorts before github, so the merged
	// Questie must be the curseforge one.
	if res[2].Provider != ProviderCurseForge || res[2].ID != "123" {
		t.Errorf("Questie came from %s/%s, want curseforge/123", res[2].Provider, res[2].ID)
	}
}

func TestSearchPartialAndTotalFailure(t *testing.T) {
	ok := &fakeProvider{name: "ok", addons: []*Addon{{Provider: "ok", Name: "Good"}}}
	bad := &fakeProvider{name: "bad", err: errors.New("boom")}

	c := &Catalog{providers: map[string]Provider{"ok": ok, "bad": bad}}
	res, err := c.Search(context.Background(), "", 10)
	if err == nil {
		t.Fatal("partial failure should return an error")
	}
	if len(res) != 1 || res[0].Name != "Good" {
		t.Fatalf("partial results lost: %+v", res)
	}

	all := &Catalog{providers: map[string]Provider{"bad1": bad, "bad2": &fakeProvider{name: "bad2", err: errors.New("nope")}}}
	res, err = all.Search(context.Background(), "", 10)
	if err == nil {
		t.Fatal("total failure should return an error")
	}
	if res != nil {
		t.Fatalf("total failure returned results: %+v", res)
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		in       string
		prov, id string
	}{
		{"owner/repo", ProviderGitHub, "owner/repo"},
		{"https://github.com/owner/repo", ProviderGitHub, "owner/repo"},
		{"https://github.com/owner/repo/tree/main", ProviderGitHub, "owner/repo"},
		{"https://www.curseforge.com/wow/addons/deadly-boss-mods", ProviderCurseForge, "deadly-boss-mods"},
		{"https://www.wowinterface.com/downloads/info25345-questie.html", ProviderWowInterface, "25345"},
		{"https://www.tukui.org/downloads/1", ProviderTukui, "1"},
	}
	for _, tt := range tests {
		prov, id, err := parseSource(tt.in)
		if err != nil {
			t.Errorf("parseSource(%q): %v", tt.in, err)
			continue
		}
		if prov != tt.prov || id != tt.id {
			t.Errorf("parseSource(%q) = (%q, %q), want (%q, %q)", tt.in, prov, id, tt.prov, tt.id)
		}
	}
	bad := []string{
		"",
		"https://example.com/foo",
		"https://www.wowinterface.com/downloads/file/25345",
		"https://github.com/owner",
		"12345",
	}
	for _, in := range bad {
		if _, _, err := parseSource(in); err == nil {
			t.Errorf("parseSource(%q) should fail", in)
		}
	}
}

func TestInstallFromSource(t *testing.T) {
	var ts *httptest.Server
	zipData := addonZip(t, "TestAddon", "## Title: TestAddon\n## Version: 1.0.0\n## Interface: 30300\n")
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/testaddon":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"full_name": "acme/testaddon",
				"name": "testaddon",
				"description": "A test addon",
				"html_url": "https://github.com/acme/testaddon",
				"default_branch": "main",
				"owner": {"login": "acme"}
			}`))
		case "/repos/acme/testaddon/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"tag_name": "v1.0.0",
				"assets": [{"name": "testaddon.zip", "browser_download_url": "%s/testaddon.zip"}]
			}`, ts.URL)
		case "/testaddon.zip":
			w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := &Catalog{
		providers: map[string]Provider{ProviderGitHub: newGitHubProvider(ts.Client(), ts.URL, ts.URL)},
	}
	addonsDir := filepath.Join(t.TempDir(), "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	c.Reg = reg

	source := "https://github.com/acme/testaddon"
	folders, err := c.InstallFromSource(context.Background(), source, addonsDir, nil)
	if err != nil {
		t.Fatalf("InstallFromSource: %v", err)
	}
	if len(folders) != 1 || folders[0] != "TestAddon" {
		t.Fatalf("installed folders = %v, want [TestAddon]", folders)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "TestAddon", "TestAddon.toc")); err != nil {
		t.Fatalf("installed TOC missing: %v", err)
	}

	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("registry entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Folder != "TestAddon" || e.Provider != ProviderGitHub || e.ID != "acme/testaddon" {
		t.Errorf("entry = %+v", e)
	}
	if e.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0 (read from installed TOC)", e.Version)
	}
	if e.Source != source {
		t.Errorf("source = %q", e.Source)
	}
}

func TestNewCurseForgeKeyPrecedence(t *testing.T) {
	// Env var wins over the catalog field.
	t.Setenv("WOWFIX_CURSEFORGE_API_KEY", "env-key")
	c, err := New(map[string]bool{ProviderCurseForge: true}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prov, ok := c.Provider(ProviderCurseForge)
	if !ok {
		t.Fatal("curseforge provider missing")
	}
	cf := prov.(*curseforgeProvider)
	if got := cf.apiKey(); got != "env-key" {
		t.Errorf("apiKey with env set = %q, want env-key", got)
	}

	// Catalog field is used when the env var is empty.
	t.Setenv("WOWFIX_CURSEFORGE_API_KEY", "")
	c.CurseForgeAPIKey = "cfg-key"
	if got := cf.apiKey(); got != "cfg-key" {
		t.Errorf("apiKey with config field = %q, want cfg-key", got)
	}

	// Empty both -> legacy mode.
	c.CurseForgeAPIKey = ""
	if got := cf.apiKey(); got != "" {
		t.Errorf("apiKey with no key = %q, want empty (legacy)", got)
	}
	if cf.usingModern() {
		t.Error("provider should be in legacy mode with no key")
	}
}

func TestSearchSurfacesCurseForgeFailure(t *testing.T) {
	// A keyless provider hitting a 403 legacy endpoint must surface
	// ErrCurseForgeUnavailable next to the healthy providers' results
	// instead of looking like an empty catalog.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer ts.Close()

	curse := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	github := &fakeProvider{name: ProviderGitHub, addons: []*Addon{
		{Provider: ProviderGitHub, ID: "a/questie", Name: "Questie", LatestVersion: "9.2.0"},
	}}
	c := &Catalog{providers: map[string]Provider{
		ProviderCurseForge: curse,
		ProviderGitHub:     github,
	}}

	res, err := c.Search(context.Background(), "questie", 10)
	if err == nil {
		t.Fatal("Search should surface the CurseForge failure")
	}
	if !strings.Contains(err.Error(), "CurseForge") {
		t.Errorf("joined error should mention CurseForge, got %q", err)
	}
	if len(res) != 1 || res[0].Name != "Questie" {
		t.Fatalf("healthy provider results lost: %+v", res)
	}
}

func addonNames(addons []*Addon) string {
	var names []string
	for _, a := range addons {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

func TestComputeManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Addon")
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Addon.toc", "## Version: 1.0.0\n")
	write("Libs/Lib.lua", "local L = {}\n")

	first, err := ComputeManifest(dir)
	if err != nil {
		t.Fatalf("ComputeManifest: %v", err)
	}
	if first == "" {
		t.Fatal("manifest digest is empty")
	}
	// Deterministic: an unchanged tree hashes identically.
	again, err := ComputeManifest(dir)
	if err != nil {
		t.Fatalf("ComputeManifest (repeat): %v", err)
	}
	if again != first {
		t.Errorf("determinism violated: %q != %q", again, first)
	}
	// Adding a file changes the digest.
	write("extra.lua", "-- new\n")
	withExtra, err := ComputeManifest(dir)
	if err != nil {
		t.Fatalf("ComputeManifest (added file): %v", err)
	}
	if withExtra == first {
		t.Error("adding a file did not change the digest")
	}
	// Modifying a file's contents changes the digest.
	write("Libs/Lib.lua", "local L = {changed = true}\n")
	modified, err := ComputeManifest(dir)
	if err != nil {
		t.Fatalf("ComputeManifest (modified file): %v", err)
	}
	if modified == withExtra {
		t.Error("modifying a file did not change the digest")
	}
}

func TestComputeManifestEmptyAndMissing(t *testing.T) {
	digest, err := ComputeManifest(t.TempDir())
	if err != nil {
		t.Fatalf("ComputeManifest on empty dir: %v", err)
	}
	if digest == "" {
		t.Error("empty dir should hash to a non-empty digest")
	}
	if _, err := ComputeManifest(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("ComputeManifest on a missing dir should error")
	}
}

func TestInstallRecordsManifestChecksum(t *testing.T) {
	var ts *httptest.Server
	zipData := addonZip(t, "TestAddon", "## Title: TestAddon\n## Version: 1.0.0\n## Interface: 30300\n")
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/testaddon":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"full_name": "acme/testaddon",
				"name": "testaddon",
				"description": "A test addon",
				"html_url": "https://github.com/acme/testaddon",
				"default_branch": "main",
				"owner": {"login": "acme"}
			}`))
		case "/repos/acme/testaddon/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"tag_name": "v1.0.0",
				"assets": [{"name": "testaddon.zip", "browser_download_url": "%s/testaddon.zip"}]
			}`, ts.URL)
		case "/testaddon.zip":
			w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := &Catalog{
		providers: map[string]Provider{ProviderGitHub: newGitHubProvider(ts.Client(), ts.URL, ts.URL)},
	}
	addonsDir := filepath.Join(t.TempDir(), "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	c.Reg = reg

	if _, err := c.InstallFromSource(context.Background(), "https://github.com/acme/testaddon", addonsDir, nil); err != nil {
		t.Fatalf("InstallFromSource: %v", err)
	}
	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("registry entries = %d, want 1", len(entries))
	}
	stored := entries[0].Checksum
	if stored == "" {
		t.Fatal("installed entry has no checksum")
	}
	if want, err := ComputeManifest(filepath.Join(addonsDir, "TestAddon")); err != nil {
		t.Fatalf("ComputeManifest: %v", err)
	} else if stored != want {
		t.Errorf("stored checksum %q does not match computed %q", stored, want)
	}
	// Post-install drift (e.g. a manual edit) must change the digest.
	toc := filepath.Join(addonsDir, "TestAddon", "TestAddon.toc")
	if err := os.WriteFile(toc, []byte("## Title: TestAddon\n## Version: 1.0.0\n## Interface: 30300\n-- edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drifted, err := ComputeManifest(filepath.Join(addonsDir, "TestAddon"))
	if err != nil {
		t.Fatalf("ComputeManifest after edit: %v", err)
	}
	if drifted == stored {
		t.Error("checksum unchanged after editing an installed file")
	}
}
