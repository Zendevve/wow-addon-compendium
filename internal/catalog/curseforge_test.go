package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const cfSearchFixture = `[
	{
		"id": 123,
		"name": "Deadly Boss Mods",
		"summary": "Scans boss mods",
		"websiteUrl": "https://www.curseforge.com/wow/addons/deadly-boss-mods",
		"authors": [{"name": "MysticalOS"}],
		"latestFiles": [
			{"displayName": "v11.0.2.8", "dateReleased": "2024-07-25T10:00:00", "gameVersion": ["11.0.2"]}
		]
	},
	{
		"id": 456,
		"name": "AtlasLoot",
		"summary": "Loot tables",
		"websiteUrl": "https://www.curseforge.com/wow/addons/atlasloot",
		"authors": [{"name": "Atlas"}],
		"latestFiles": [
			{"displayName": "1.13.4", "dateReleased": "2023-03-01T00:00:00Z", "gameVersion": ["3.3.5"]}
		]
	}
]`

const cfResolveFixture = `{
	"id": 456,
	"name": "AtlasLoot",
	"summary": "Loot tables",
	"websiteUrl": "https://www.curseforge.com/wow/addons/atlasloot",
	"authors": [{"name": "Atlas"}],
	"latestFiles": [
		{"displayName": "1.13.4", "dateReleased": "2023-03-01T00:00:00Z", "gameVersion": ["3.3.5"]}
	]
}`

const cfZip = "PK\x03\x04 curseforge zip fixture"

func TestCurseForgeSearch(t *testing.T) {
	var ts *httptest.Server
	var gotPageSize string
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/addon/search" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("gameId") != "1" {
			t.Errorf("gameId = %q, want 1", q.Get("gameId"))
		}
		if q.Get("searchFilter") != "dbm" {
			t.Errorf("searchFilter = %q, want dbm", q.Get("searchFilter"))
		}
		gotPageSize = q.Get("pageSize")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cfSearchFixture))
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	addons, err := p.Search(context.Background(), "dbm", 7)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPageSize != "7" {
		t.Errorf("pageSize = %q, want 7", gotPageSize)
	}
	if len(addons) != 2 {
		t.Fatalf("got %d addons, want 2", len(addons))
	}
	a := addons[0]
	if a.ID != "123" || a.Name != "Deadly Boss Mods" || a.Author != "MysticalOS" {
		t.Errorf("unexpected addon: %+v", a)
	}
	if a.LatestVersion != "v11.0.2.8" {
		t.Errorf("LatestVersion = %q, want v11.0.2.8", a.LatestVersion)
	}
	if a.GameVersion != "retail" {
		t.Errorf("GameVersion = %q, want retail (from 11.0.2)", a.GameVersion)
	}
	if a.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not parsed from dateReleased without zone")
	}
	if addons[1].GameVersion != "wrath" {
		t.Errorf("GameVersion = %q, want wrath (from 3.3.5)", addons[1].GameVersion)
	}
}

func TestCurseForgeResolveByID(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/addon/456" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cfResolveFixture))
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	a, err := p.Resolve(context.Background(), "456")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a.ID != "456" || a.Name != "AtlasLoot" || a.LatestVersion != "1.13.4" {
		t.Errorf("unexpected addon: %+v", a)
	}
	if a.Homepage != "https://www.curseforge.com/wow/addons/atlasloot" {
		t.Errorf("Homepage = %q", a.Homepage)
	}
}

func TestCurseForgeResolveBySlug(t *testing.T) {
	var ts *httptest.Server
	var searches int
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/addon/search":
			searches++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(cfSearchFixture))
		case "/addon/123":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"id": 123,
				"name": "Deadly Boss Mods",
				"summary": "Scans boss mods",
				"latestFiles": [{"displayName": "v11.0.2.8", "gameVersion": ["11.0.2"]}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	a, err := p.Resolve(context.Background(), "Deadly Boss Mods")
	if err != nil {
		t.Fatalf("Resolve by slug: %v", err)
	}
	if searches != 1 {
		t.Errorf("search calls = %d, want 1", searches)
	}
	if a.ID != "123" {
		t.Errorf("ID = %q, want 123", a.ID)
	}
}

func TestCurseForgeDownloadFollowsRedirect(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/addon/456/download":
			http.Redirect(w, r, ts.URL+"/file.zip", http.StatusFound)
		case "/file.zip":
			w.Write([]byte(cfZip))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	dest := t.TempDir() + "/out.zip"
	var calls int
	progress := func(done, total int64) {
		calls++
		if total != int64(len(cfZip)) {
			t.Errorf("progress total = %d, want %d", total, len(cfZip))
		}
	}
	if err := p.Download(context.Background(), &Addon{Provider: ProviderCurseForge, ID: "456"}, dest, progress); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != cfZip {
		t.Errorf("downloaded %q, want %q", data, cfZip)
	}
	if calls == 0 {
		t.Error("progress callback never invoked")
	}
}

func TestCurseForgeLatest(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/addon/456" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cfResolveFixture))
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	a, err := p.Latest(context.Background(), &Addon{Provider: ProviderCurseForge, ID: "456"})
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if a.LatestVersion != "1.13.4" {
		t.Errorf("LatestVersion = %q, want 1.13.4", a.LatestVersion)
	}
}

// TestCurseForgeSearchJSONShape guards against our fixtures becoming
// invalid JSON (encoding/json would silently produce nothing).
func TestCurseForgeSearchJSONShape(t *testing.T) {
	var v []map[string]any
	if err := json.Unmarshal([]byte(cfSearchFixture), &v); err != nil {
		t.Fatalf("fixture broken: %v", err)
	}
	if len(v) != 2 {
		t.Fatalf("fixture has %d entries", len(v))
	}
	_ = fmt.Sprintf("%v", v[0]["id"])
}

const cfModernModFixture = `{
	"id": 456,
	"name": "AtlasLoot",
	"summary": "Loot tables",
	"websiteUrl": "https://www.curseforge.com/wow/addons/atlasloot",
	"authors": [{"name": "Atlas"}],
	"latestFiles": [
		{"id": 77, "displayName": "v1.13.4", "fileDate": "2023-03-01T00:00:00Z", "gameVersions": ["3.3.5", "3.4.1"]}
	]
}`

func TestCurseForgeModernSearch(t *testing.T) {
	var ts *httptest.Server
	var gotPath, gotKey string
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		q := r.URL.Query()
		if q.Get("gameId") != "1" || q.Get("searchFilter") != "dbm" {
			t.Errorf("query = %v", q)
		}
		if gotKey != "secret-key" {
			t.Errorf("x-api-key = %q, want secret-key", gotKey)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": [` + cfModernModFixture + `]}`))
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), "", ts.URL, func() string { return "secret-key" })
	addons, err := p.Search(context.Background(), "dbm", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/mods/search" {
		t.Errorf("path = %q, want /mods/search (Core API)", gotPath)
	}
	if len(addons) != 1 {
		t.Fatalf("got %d addons, want 1", len(addons))
	}
	a := addons[0]
	if a.ID != "456" || a.Name != "AtlasLoot" || a.LatestVersion != "v1.13.4" {
		t.Errorf("unexpected addon: %+v", a)
	}
	if a.GameVersion != "wrath" {
		t.Errorf("GameVersion = %q, want wrath (from 3.3.5)", a.GameVersion)
	}
	if a.fileID != 77 {
		t.Errorf("fileID = %d, want 77", a.fileID)
	}
}

func TestCurseForgeModernResolveAndDownload(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mods/456":
			if r.Header.Get("x-api-key") != "secret-key" {
				t.Errorf("resolve: x-api-key missing")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data": ` + cfModernModFixture + `}`))
		case "/mods/456/files/77/download-url":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"data": %q}`, ts.URL+"/file.zip")))
		case "/file.zip":
			w.Write([]byte(cfZip))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), "", ts.URL, func() string { return "secret-key" })
	a, err := p.Resolve(context.Background(), "456")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a.fileID != 77 {
		t.Fatalf("fileID = %d, want 77", a.fileID)
	}
	dest := t.TempDir() + "/out.zip"
	if err := p.Download(context.Background(), a, dest, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != cfZip {
		t.Errorf("downloaded %q, want %q", data, cfZip)
	}
}

func TestCurseForgeModernKeyRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusForbidden)
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), "", ts.URL, func() string { return "bad-key" })
	_, err := p.Search(context.Background(), "dbm", 5)
	if err == nil {
		t.Fatal("Search should fail with a rejected key")
	}
	if !errors.Is(err, ErrCurseForgeUnavailable) {
		t.Errorf("error should wrap ErrCurseForgeUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "CurseForge") || !strings.Contains(err.Error(), "rejected") {
		t.Errorf("error should mention rejection, got %q", err)
	}
}

func TestCurseForgeLegacyForbiddenIsNotSilent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil) // no key -> legacy
	_, err := p.Search(context.Background(), "dbm", 5)
	if err == nil {
		t.Fatal("Search should fail on a 403 legacy endpoint")
	}
	if !errors.Is(err, ErrCurseForgeUnavailable) {
		t.Errorf("error should wrap ErrCurseForgeUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "CurseForge") {
		t.Errorf("error should mention CurseForge, got %q", err)
	}
}

func TestCurseForgeLegacyUnreachableWithoutKey(t *testing.T) {
	// A server that never answers makes the keyless provider surface
	// unavailability rather than an empty result set.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	p := newCurseForgeProvider(ts.Client(), ts.URL, "", nil)
	_, err := p.Search(context.Background(), "dbm", 5)
	if err == nil {
		t.Fatal("Search should fail when the legacy endpoint is down")
	}
	if !errors.Is(err, ErrCurseForgeUnavailable) {
		t.Errorf("error should wrap ErrCurseForgeUnavailable, got %v", err)
	}
}
