package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

const mmouiFixture = `[
	{
		"uid": 25345,
		"name": "questie",
		"label": "Questie",
		"changelog": "Fixed some quest markers.",
		"category": 5,
		"url": "%s/questie.zip",
		"nid": 1,
		"author": "Questie Devs",
		"version": "9.2.0",
		"date": "2024-01-15"
	},
	{
		"uid": 5555,
		"name": "atlasloot",
		"label": "AtlasLoot",
		"changelog": "Updated loot tables.",
		"category": 3,
		"url": "%s/atlas.zip",
		"nid": 2,
		"author": "Atlas",
		"version": "1.2.3",
		"date": "2023-06-01"
	}
]`

const mmouiZip = "PK\x03\x04 mmoui zip fixture"

func newMMOUIServer(t *testing.T, urlFunc func(ts string) string) (*httptest.Server, *wowInterfaceProvider, *int) {
	t.Helper()
	var ts *httptest.Server
	var loads int
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/filelist.json":
			loads++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(urlFunc(ts.URL)))
		case "/questie.zip":
			w.Write([]byte(mmouiZip))
		default:
			http.NotFound(w, r)
		}
	}))
	p := newWowInterfaceProvider(ts.Client(), ts.URL+"/filelist.json")
	return ts, p, &loads
}

func TestWowInterfaceSearchAndResolve(t *testing.T) {
	ts, p, loads := newMMOUIServer(t, func(ts string) string {
		return sprintfFixture(mmouiFixture, ts, ts)
	})
	defer ts.Close()

	ctx := context.Background()
	addons, err := p.Search(ctx, "quest", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(addons) != 1 {
		t.Fatalf("got %d addons, want 1", len(addons))
	}
	a := addons[0]
	if a.ID != "25345" || a.Name != "Questie" || a.Author != "Questie Devs" {
		t.Errorf("unexpected addon: %+v", a)
	}
	if a.LatestVersion != "9.2.0" {
		t.Errorf("LatestVersion = %q, want 9.2.0", a.LatestVersion)
	}
	if a.Homepage != "https://www.wowinterface.com/downloads/info25345-questie.html" {
		t.Errorf("Homepage = %q", a.Homepage)
	}
	if a.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not parsed")
	}
	if a.downloadURL != ts.URL+"/questie.zip" {
		t.Errorf("downloadURL = %q", a.downloadURL)
	}

	resolved, err := p.Resolve(ctx, "25345")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Name != "Questie" {
		t.Errorf("Resolve gave %q", resolved.Name)
	}
	// Resolve by slug also works.
	bySlug, err := p.Resolve(ctx, "atlasloot")
	if err != nil {
		t.Fatalf("Resolve by slug: %v", err)
	}
	if bySlug.ID != "5555" {
		t.Errorf("slug resolve ID = %q, want 5555", bySlug.ID)
	}
	// The filelist must be fetched exactly once despite three calls.
	if *loads != 1 {
		t.Errorf("filelist loads = %d, want 1 (cached)", *loads)
	}
}

func TestWowInterfaceSearchLimit(t *testing.T) {
	ts, p, _ := newMMOUIServer(t, func(ts string) string {
		return sprintfFixture(mmouiFixture, ts, ts)
	})
	defer ts.Close()

	addons, err := p.Search(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(addons) != 1 {
		t.Errorf("limit=1 gave %d addons, want 1", len(addons))
	}
}

func TestWowInterfaceResolveMissing(t *testing.T) {
	ts, p, _ := newMMOUIServer(t, func(ts string) string {
		return sprintfFixture(mmouiFixture, ts, ts)
	})
	defer ts.Close()

	if _, err := p.Resolve(context.Background(), "999999"); err == nil {
		t.Fatal("Resolve of unknown id should error")
	}
}

func TestWowInterfaceDownload(t *testing.T) {
	ts, p, _ := newMMOUIServer(t, func(ts string) string {
		return sprintfFixture(mmouiFixture, ts, ts)
	})
	defer ts.Close()

	addon, err := p.Resolve(context.Background(), "25345")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dest := t.TempDir() + "/out.zip"
	if err := p.Download(context.Background(), addon, dest, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != mmouiZip {
		t.Errorf("downloaded %q, want %q", data, mmouiZip)
	}
}

func TestWowInterfaceLatest(t *testing.T) {
	ts, p, _ := newMMOUIServer(t, func(ts string) string {
		return sprintfFixture(mmouiFixture, ts, ts)
	})
	defer ts.Close()

	a, err := p.Latest(context.Background(), &Addon{Provider: ProviderWowInterface, ID: "5555"})
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if a.LatestVersion != "1.2.3" {
		t.Errorf("LatestVersion = %q, want 1.2.3", a.LatestVersion)
	}
}

func TestWowInterfaceFixtureShape(t *testing.T) {
	var v []map[string]any
	if err := json.Unmarshal([]byte(mmouiFixture), &v); err != nil {
		t.Fatalf("fixture broken: %v", err)
	}
	if len(v) != 2 {
		t.Fatalf("fixture has %d entries", len(v))
	}
}

// sprintfFixture substitutes every %s placeholder with args in order.
func sprintfFixture(format string, args ...string) string {
	out := format
	for _, a := range args {
		out = replaceFirst(out, "%s", a)
	}
	return out
}

func replaceFirst(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}
