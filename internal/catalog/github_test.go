package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const repoJSON = `{
	"full_name": "owner/questie",
	"name": "questie",
	"description": "Quest tracker for WoW",
	"html_url": "https://github.com/owner/questie",
	"default_branch": "main",
	"updated_at": "2024-01-15T10:00:00Z",
	"owner": {"login": "owner"}
}`

const releaseJSON = `{
	"tag_name": "v9.2.0",
	"published_at": "2024-02-01T12:00:00Z",
	"assets": [
		{"name": "questie-v9.2.0.zip", "browser_download_url": "%s/questie-v9.2.0.zip"},
		{"name": "questie-v9.2.0.asc", "browser_download_url": "%s/questie-v9.2.0.asc"}
	]
}`

const zipBytes = "PK\x03\x04 fake zip payload for tests"

func TestGitHubSearch(t *testing.T) {
	var ts *httptest.Server
	var topicQueries, plainQueries int
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "topic:world-of-warcraft-addon") {
			topicQueries++
			// Degrade gracefully: the qualified query is rejected.
			http.Error(w, "422 Unprocessable Entity", http.StatusUnprocessableEntity)
			return
		}
		plainQueries++
		if q != "questie" {
			t.Errorf("plain query = %q, want questie", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ghSearchResult{Items: []ghRepo{
			{
				FullName:      "owner/questie",
				Name:          "questie",
				Description:   "Quest tracker",
				HTMLURL:       "https://github.com/owner/questie",
				DefaultBranch: "main",
				Owner: struct {
					Login string `json:"login"`
				}{Login: "owner"},
			},
		}})
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	addons, err := p.Search(context.Background(), "questie", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if topicQueries != 1 || plainQueries != 1 {
		t.Fatalf("requests: topic=%d plain=%d, want 1/1", topicQueries, plainQueries)
	}
	if len(addons) != 1 {
		t.Fatalf("got %d addons, want 1", len(addons))
	}
	a := addons[0]
	if a.Name != "questie" || a.ID != "owner/questie" || a.Author != "owner" || a.Summary != "Quest tracker" {
		t.Errorf("unexpected addon: %+v", a)
	}
}

func TestGitHubSearchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit", http.StatusForbidden)
	}))
	defer ts.Close()
	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	if _, err := p.Search(context.Background(), "questie", 10); err == nil {
		t.Fatal("Search should fail when both queries are rejected")
	}
}

func TestGitHubResolveReleaseAsset(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/questie":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(repoJSON))
		case "/repos/owner/questie/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(releaseJSON, ts.URL, ts.URL)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	addon, err := p.Resolve(context.Background(), "owner/questie")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if addon.LatestVersion != "v9.2.0" {
		t.Errorf("LatestVersion = %q, want v9.2.0", addon.LatestVersion)
	}
	if addon.downloadURL != ts.URL+"/questie-v9.2.0.zip" {
		t.Errorf("downloadURL = %q, want first zip asset", addon.downloadURL)
	}
	if addon.Homepage != "https://github.com/owner/questie" {
		t.Errorf("Homepage = %q", addon.Homepage)
	}
	if addon.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not parsed")
	}
}

func TestGitHubResolveNoReleases(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/questie":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(repoJSON))
		case "/repos/owner/questie/releases/latest":
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	addon, err := p.Resolve(context.Background(), "owner/questie")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if addon.LatestVersion != "main@HEAD" {
		t.Errorf("LatestVersion = %q, want main@HEAD", addon.LatestVersion)
	}
	want := ts.URL + "/owner/questie/zip/refs/heads/main"
	if addon.downloadURL != want {
		t.Errorf("downloadURL = %q, want %q", addon.downloadURL, want)
	}
}

func TestGitHubDownload(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/questie":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(repoJSON))
		case "/repos/owner/questie/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(releaseJSON, ts.URL, ts.URL)))
		case "/questie-v9.2.0.zip":
			w.Write([]byte(zipBytes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	addon, err := p.Resolve(context.Background(), "owner/questie")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dest := t.TempDir() + "/out.zip"
	var lastDone, lastTotal int64
	progress := func(done, total int64) { lastDone, lastTotal = done, total }
	if err := p.Download(context.Background(), addon, dest, progress); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if string(data) != zipBytes {
		t.Errorf("downloaded %q, want %q", data, zipBytes)
	}
	if lastDone != int64(len(zipBytes)) {
		t.Errorf("progress done = %d, want %d", lastDone, len(zipBytes))
	}
	if lastTotal != int64(len(zipBytes)) {
		t.Errorf("progress total = %d, want %d", lastTotal, len(zipBytes))
	}
}

func TestGitHubDownloadWithoutResolve(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	if err := p.Download(context.Background(), &Addon{Provider: ProviderGitHub, ID: "a/b"}, "x.zip", nil); err == nil {
		t.Fatal("Download without a resolved URL should error")
	}
}

func TestGitHubInvalidID(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	if _, err := p.Resolve(context.Background(), "no-slash"); err == nil {
		t.Fatal("Resolve with invalid id should error")
	}
}

func TestGitHubResolveVersionReleaseTag(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/questie/releases/tags/v9.0.0":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"tag_name": "v9.0.0",
				"published_at": "2024-01-20T10:00:00Z",
				"assets": [{"name": "questie-v9.0.0.zip", "browser_download_url": "` + ts.URL + `/questie-v9.0.0.zip"}]
			}`))
		case "/questie-v9.0.0.zip":
			w.Write([]byte(zipBytes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	base := &Addon{Provider: ProviderGitHub, ID: "owner/questie", Name: "questie"}
	addon, err := p.ResolveVersion(context.Background(), base, "9.0.0", "v9.0.0")
	if err != nil {
		t.Fatalf("ResolveVersion: %v", err)
	}
	if addon.LatestVersion != "v9.0.0" {
		t.Errorf("LatestVersion = %q, want v9.0.0", addon.LatestVersion)
	}
	if addon.VersionRef != "v9.0.0" {
		t.Errorf("VersionRef = %q, want v9.0.0", addon.VersionRef)
	}
	if addon.downloadURL != ts.URL+"/questie-v9.0.0.zip" {
		t.Errorf("downloadURL = %q, want release asset", addon.downloadURL)
	}
	// The resolved addon must be downloadable (rollback path).
	dest := t.TempDir() + "/out.zip"
	if err := p.Download(context.Background(), addon, dest, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != zipBytes {
		t.Errorf("downloaded %q, want %q", data, zipBytes)
	}
}

func TestGitHubResolveVersionTagWithoutRelease(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/questie/releases/tags/old-tag":
			http.NotFound(w, r)
		case "/repos/owner/questie/git/ref/tags/old-tag":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ref": "refs/tags/old-tag", "object": {"sha": "abc", "type": "commit"}}`))
		case "/owner/questie/zip/refs/tags/old-tag":
			w.Write([]byte(zipBytes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	addon, err := p.ResolveVersion(context.Background(), &Addon{Provider: ProviderGitHub, ID: "owner/questie"}, "old-tag", "")
	if err != nil {
		t.Fatalf("ResolveVersion: %v", err)
	}
	if addon.downloadURL != ts.URL+"/owner/questie/zip/refs/tags/old-tag" {
		t.Errorf("downloadURL = %q, want codeload source zip", addon.downloadURL)
	}
	if addon.LatestVersion != "old-tag" {
		t.Errorf("LatestVersion = %q, want old-tag", addon.LatestVersion)
	}
}

func TestGitHubResolveVersionMissingTag(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // neither release nor ref exists
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	_, err := p.ResolveVersion(context.Background(), &Addon{Provider: ProviderGitHub, ID: "owner/questie"}, "gone", "gone")
	if err == nil {
		t.Fatal("ResolveVersion for a missing tag should error")
	}
	if !strings.Contains(err.Error(), "no release or tag") {
		t.Errorf("error = %q, want honest no-release-or-tag message", err.Error())
	}
}

func TestGitHubResolveVersionPrefersRef(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/questie/releases/tags/v1.2.3":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"tag_name": "v1.2.3", "assets": []}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	p := newGitHubProvider(ts.Client(), ts.URL, ts.URL)
	// TOC version differs from the tag: the recorded ref must win.
	addon, err := p.ResolveVersion(context.Background(), &Addon{Provider: ProviderGitHub, ID: "owner/questie"}, "1.2.3", "v1.2.3")
	if err != nil {
		t.Fatalf("ResolveVersion: %v", err)
	}
	if addon.VersionRef != "v1.2.3" {
		t.Errorf("VersionRef = %q, want v1.2.3", addon.VersionRef)
	}
}
