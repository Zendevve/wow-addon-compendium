package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const tukuiFixture = `[
	{
		"id": "1",
		"name": "ElvUI",
		"tagline": "A full UI replacement",
		"version": "13.50",
		"changelog": "Fixes and tweaks",
		"url": "https://www.tukui.org/download.php?ui=elvui",
		"web_url": "https://www.tukui.org/addons.php?id=1",
		"download_path": "%s/elvui.zip",
		"author": "Elv",
		"lastupdate": "1700000000"
	},
	{
		"id": "2",
		"name": "Tukui",
		"tagline": "A clean UI",
		"version": "22.0",
		"changelog": "Updates",
		"url": "https://www.tukui.org/download.php?ui=tukui",
		"web_url": "https://www.tukui.org/addons.php?id=2",
		"download_path": "%s/tukui.zip",
		"author": "Tukz",
		"lastupdate": "1690000000"
	}
]`

const tukuiZip = "PK\x03\x04 tukui zip fixture"

func TestTukuiSearchResolveDownload(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api.php") {
			if r.URL.Path == "/elvui.zip" {
				w.Write([]byte(tukuiZip))
				return
			}
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("ui"); got != "elvui|tukui|benikui|kaitheme" {
			t.Errorf("ui = %q, want combined query", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fixtureWithURLs(tukuiFixture, ts.URL)))
	}))
	defer ts.Close()

	p := newTukuiProvider(ts.Client(), ts.URL+"/api.php")
	ctx := context.Background()

	addons, err := p.Search(ctx, "elv", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(addons) != 1 {
		t.Fatalf("got %d addons, want 1", len(addons))
	}
	a := addons[0]
	if a.ID != "1" || a.Name != "ElvUI" || a.Author != "Elv" || a.Summary != "A full UI replacement" {
		t.Errorf("unexpected addon: %+v", a)
	}
	if a.LatestVersion != "13.50" {
		t.Errorf("LatestVersion = %q, want 13.50", a.LatestVersion)
	}
	wantUpdated := time.Unix(1700000000, 0)
	if !a.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt = %v, want %v", a.UpdatedAt, wantUpdated)
	}
	if a.Homepage != "https://www.tukui.org/addons.php?id=1" {
		t.Errorf("Homepage = %q", a.Homepage)
	}
	if a.downloadURL != ts.URL+"/elvui.zip" {
		t.Errorf("downloadURL = %q, want download_path", a.downloadURL)
	}

	resolved, err := p.Resolve(ctx, "1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Name != "ElvUI" {
		t.Errorf("Resolve gave %q", resolved.Name)
	}
	byName, err := p.Resolve(ctx, "Tukui")
	if err != nil {
		t.Fatalf("Resolve by name: %v", err)
	}
	if byName.ID != "2" {
		t.Errorf("by-name ID = %q, want 2", byName.ID)
	}

	dest := t.TempDir() + "/out.zip"
	if err := p.Download(ctx, resolved, dest, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tukuiZip {
		t.Errorf("downloaded %q, want %q", data, tukuiZip)
	}
}

func TestTukuiSingleObjectResponse(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A single object instead of an array must decode too.
		w.Write([]byte(`{"id": "1", "name": "ElvUI", "version": "13.50"}`))
	}))
	defer ts.Close()

	p := newTukuiProvider(ts.Client(), ts.URL)
	addons, err := p.Search(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(addons) != 1 || addons[0].ID != "1" {
		t.Fatalf("single-object response gave %+v", addons)
	}
}

func TestTukuiPerUIQueryFallback(t *testing.T) {
	var ts *httptest.Server
	var perUI []string
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ui := r.URL.Query().Get("ui")
		if strings.Contains(ui, "|") {
			// The combined query is rejected; the provider must fall
			// back to one request per UI.
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		perUI = append(perUI, ui)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":            "9",
			"name":          ui,
			"version":       "1.0",
			"download_path": ts.URL + "/" + ui + ".zip",
		})
	}))
	defer ts.Close()

	p := newTukuiProvider(ts.Client(), ts.URL)
	addons, err := p.Search(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(perUI) != 4 {
		t.Errorf("per-UI requests = %v, want 4", perUI)
	}
	if len(addons) != 4 {
		t.Fatalf("got %d addons, want 4", len(addons))
	}
}

func TestTukuiFixtureShape(t *testing.T) {
	var v []map[string]any
	if err := json.Unmarshal([]byte(tukuiFixture), &v); err != nil {
		t.Fatalf("fixture broken: %v", err)
	}
	if len(v) != 2 {
		t.Fatalf("fixture has %d entries", len(v))
	}
}

func fixtureWithURLs(format string, tsURL string) string {
	out := format
	out = replaceFirst(out, "%s", tsURL)
	out = replaceFirst(out, "%s", tsURL)
	return out
}
