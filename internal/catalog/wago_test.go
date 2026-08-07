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

const wagoSearchFixture = `{
	"hits": [
		{
			"id": "pvBs8htuW",
			"name": "Sea Swell - CountDown Move Bar",
			"descriptionSanitized": "A countdown bar for the Sea Swell mechanic",
			"userName": "DoGGyFromPlanetWoof",
			"versionString": "1.0.1",
			"type": "WEAKAURA",
			"timestamp": 1700000000
		},
		{
			"id": "ak3iS95aa",
			"name": "Jundies - M+ Plater Profile",
			"descriptionSanitized": "Plater profile for mythic plus",
			"userName": "Jundies",
			"versionString": "3.2.0",
			"type": "PLATER",
			"timestamp": 1690000000
		}
	],
	"total": 2,
	"query": "sea",
	"index": "imports"
}`

const wagoCheckFixture = `[
	{
		"_id": "pvBs8htuW",
		"name": "Sea Swell - CountDown Move Bar",
		"username": "DoGGyFromPlanetWoof",
		"game": "tww",
		"type": "WEAKAURA",
		"version": 2,
		"versionString": "1.0.1",
		"url": "https://wago.io/pvBs8htuW",
		"modified": "2024-11-14T09:30:00Z"
	}
]`

const wagoImportString = "!DEvBZjUnq4FnDM(LJy78YLPFdsGEmdbsrox6DdJ5ewcqnglxj582hUF7D3vYyt6LRmntgW6flT7ZUpp7swCwAgBxgtKXSzSKEXX9ofhbZQY1LlTkHmJnz4iycH0YD1gUtMTkJLR1fc9tLPYNDdl5RkKISbWzPfYILpNnncor5UhLMmwCVOEXzmNAN0WuVkZME6fWQoE(d2R0fAdCDtJP)tOppL(8m8txg7jLWTfggbhP2OKLoUtPlZyFA28XFD200(tGtRIBEyqHSuCJgn5(xFn4cLoPPKx8zPXIVX0y0ma"

// wagoTestServer serves the search, check and encoded endpoints the
// way data.wago.io does, including the 302 the encoded endpoint
// performs before serving the import string.
func wagoTestServer(t *testing.T, failPaths map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code, ok := failPaths[r.URL.Path]; ok {
			http.Error(w, "boom", code)
			return
		}
		switch r.URL.Path {
		case "/search/es":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(wagoSearchFixture))
		case "/api/check/":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(wagoCheckFixture))
		case "/api/raw/encoded":
			if r.URL.Query().Get("version") == "" {
				// Mirror the API: redirect to the resolved version,
				// then serve the raw import string.
				http.Redirect(w, r, "/api/raw/encoded?id="+r.URL.Query().Get("id")+"&version=1.0.1", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(wagoImportString))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestWagoSearchResolveDownload(t *testing.T) {
	ts := wagoTestServer(t, nil)
	defer ts.Close()

	p := newWagoProvider(ts.Client(), ts.URL)
	ctx := context.Background()

	addons, err := p.Search(ctx, "sea", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(addons) != 2 {
		t.Fatalf("got %d addons, want 2", len(addons))
	}
	a := addons[0]
	if a.Provider != ProviderWago || a.ID != "pvBs8htuW" || a.Name != "Sea Swell - CountDown Move Bar" {
		t.Errorf("unexpected addon: %+v", a)
	}
	if a.Author != "DoGGyFromPlanetWoof" || a.Summary != "A countdown bar for the Sea Swell mechanic" {
		t.Errorf("author/summary: %q / %q", a.Author, a.Summary)
	}
	if a.LatestVersion != "1.0.1" {
		t.Errorf("LatestVersion = %q, want 1.0.1", a.LatestVersion)
	}
	if a.Homepage != "https://wago.io/pvBs8htuW" {
		t.Errorf("Homepage = %q", a.Homepage)
	}
	wantUpdated := time.Unix(1700000000, 0)
	if !a.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt = %v, want %v", a.UpdatedAt, wantUpdated)
	}
	// The search response carries no flavor field; GameVersion must be
	// empty rather than guessed.
	if a.GameVersion != "" {
		t.Errorf("search GameVersion = %q, want empty", a.GameVersion)
	}
	if addons[1].ID != "ak3iS95aa" || addons[1].LatestVersion != "3.2.0" {
		t.Errorf("second hit: %+v", addons[1])
	}

	limited, err := p.Search(ctx, "sea", 1)
	if err != nil {
		t.Fatalf("Search limit: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited search gave %d results, want 1", len(limited))
	}

	resolved, err := p.Resolve(ctx, "pvBs8htuW")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Name != "Sea Swell - CountDown Move Bar" {
		t.Errorf("Resolve gave %q", resolved.Name)
	}
	// The check response carries the game flavor key.
	if resolved.GameVersion != "retail" {
		t.Errorf("Resolve GameVersion = %q, want retail (game=tww)", resolved.GameVersion)
	}
	wantModified, err := time.Parse(time.RFC3339, "2024-11-14T09:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.UpdatedAt.Equal(wantModified) {
		t.Errorf("Resolve UpdatedAt = %v, want %v", resolved.UpdatedAt, wantModified)
	}
	if resolved.Homepage != "https://wago.io/pvBs8htuW" {
		t.Errorf("Resolve Homepage = %q", resolved.Homepage)
	}

	latest, err := p.Latest(ctx, resolved)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest.LatestVersion != "1.0.1" || latest.GameVersion != "retail" {
		t.Errorf("Latest = %+v", latest)
	}

	dest := t.TempDir() + "/sea-swell.txt"
	var progressCalls int
	var lastDone, lastTotal int64
	if err := p.Download(ctx, resolved, dest, func(done, total int64) {
		progressCalls++
		lastDone, lastTotal = done, total
	}); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != wagoImportString {
		t.Errorf("downloaded %d bytes, want the raw import string", len(data))
	}
	if progressCalls == 0 || lastDone != lastTotal || lastDone != int64(len(wagoImportString)) {
		t.Errorf("progress calls = %d, last %d/%d", progressCalls, lastDone, lastTotal)
	}
}

func TestWagoSearchNoMatches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits": [], "total": 0, "query": "zzz", "index": "imports"}`))
	}))
	defer ts.Close()

	p := newWagoProvider(ts.Client(), ts.URL)
	addons, err := p.Search(context.Background(), "zzz", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(addons) != 0 {
		t.Fatalf("got %d addons, want 0", len(addons))
	}
}

func TestWagoSearchMalformedResponse(t *testing.T) {
	// A 200 that is not the expected shape (an error body after an
	// API change) must surface as an error, never as an empty result
	// set.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"Route GET:/api/search not found","statusCode":404}`))
	}))
	defer ts.Close()

	p := newWagoProvider(ts.Client(), ts.URL)
	addons, err := p.Search(context.Background(), "sea", 10)
	if err == nil {
		t.Fatal("Search should error on a response missing hits")
	}
	if addons != nil {
		t.Fatalf("Search returned %d addons alongside an error", len(addons))
	}
}

func TestWagoResolveNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	p := newWagoProvider(ts.Client(), ts.URL)
	_, err := p.Resolve(context.Background(), "doesnotexist")
	if err == nil || !strings.Contains(err.Error(), "no import") {
		t.Fatalf("Resolve = %v, want a no-import error", err)
	}
}

func TestWagoFailureDegradation(t *testing.T) {
	// Every endpoint failing must surface as an error, never as an
	// empty result set.
	ts := wagoTestServer(t, map[string]int{
		"/search/es":       http.StatusInternalServerError,
		"/api/check/":      http.StatusBadGateway,
		"/api/raw/encoded": http.StatusForbidden,
	})
	defer ts.Close()

	p := newWagoProvider(ts.Client(), ts.URL)
	ctx := context.Background()

	if _, err := p.Search(ctx, "sea", 10); err == nil {
		t.Fatal("Search should error on a non-200 response")
	}
	if _, err := p.Resolve(ctx, "pvBs8htuW"); err == nil {
		t.Fatal("Resolve should error on a non-200 response")
	}
	addon := &Addon{Provider: ProviderWago, ID: "pvBs8htuW"}
	if err := p.Download(ctx, addon, t.TempDir()+"/out.txt", nil); err == nil {
		t.Fatal("Download should error on a non-200 response")
	}
}

func TestWagoGameFamily(t *testing.T) {
	tests := []struct {
		game string
		want string
	}{
		{"classic", "vanilla"},
		{"sod", "vanilla"},
		{"hardcore", "vanilla"},
		{"titan-wotlk", "wrath"},
		{"wrath", "wrath"},
		{"tbc", "tbc"},
		{"cata", "cata"},
		{"tww", "retail"},
		{"midnight", "retail"},
		{"df", "retail"},
		{"sl", "retail"},
		{"bfa", "retail"},
		{"mop", "retail"},
		{"retail", "retail"},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := wagoGameFamily(tt.game); got != tt.want {
			t.Errorf("wagoGameFamily(%q) = %q, want %q", tt.game, got, tt.want)
		}
	}
}

func TestWagoFixtureShape(t *testing.T) {
	var sr wagoSearchResponse
	if err := json.Unmarshal([]byte(wagoSearchFixture), &sr); err != nil {
		t.Fatalf("search fixture broken: %v", err)
	}
	if sr.Hits == nil || len(*sr.Hits) != 2 || sr.Total != 2 {
		t.Fatalf("search fixture: total=%d hits=%v", sr.Total, sr.Hits)
	}
	var list []wagoCheck
	if err := json.Unmarshal([]byte(wagoCheckFixture), &list); err != nil {
		t.Fatalf("check fixture broken: %v", err)
	}
	if len(list) != 1 || list[0].ID != "pvBs8htuW" {
		t.Fatalf("check fixture: %+v", list)
	}
}
