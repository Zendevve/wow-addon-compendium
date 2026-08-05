package importexport

import (
	"archive/zip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	mf := Manifest{
		Version:     1,
		Name:        "pve",
		GameVersion: "wrath",
		Addons: []ManifestAddon{
			{Folder: "Questie", Provider: "github", ID: "Vendethiel/Questie", Source: "Vendethiel/Questie", Version: "1.2.3"},
			{Folder: "LocalOnly"},
		},
	}
	path := filepath.Join(t.TempDir(), "out.json")
	if err := ExportManifest(mf.Name, mf.GameVersion, mf.Addons, path); err != nil {
		t.Fatal(err)
	}
	got, err := ImportManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &mf) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, &mf)
	}
}

func makeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExportZipAndLocalImportRoundTrip(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "AddOns", "LocalAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "AddOns", "LocalAddon", "LocalAddon.toc"),
		[]byte("## Interface: 30300\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sv := filepath.Join(work, "sv")
	if err := os.MkdirAll(filepath.Join(sv, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sv, "Questie.lua"), []byte("SV={}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sv, "sub", "nested.lua"), []byte("SV={}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(work, "bundle.zip")
	srcDir := filepath.Join(work, "AddOns")
	addons := []ManifestAddon{{Folder: "LocalAddon"}}
	if err := ExportZip("pve", "wrath", addons, srcDir, sv, out); err != nil {
		t.Fatal(err)
	}

	wtf := filepath.Join(work, "WTF")
	if err := os.MkdirAll(wtf, 0o755); err != nil {
		t.Fatal(err)
	}
	// Import into a fresh AddOns directory, not the one we exported from.
	addonsDir := filepath.Join(work, "AddOns2")
	installed, err := ImportZip(out, addonsDir, wtf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(installed, []string{"LocalAddon"}) {
		t.Fatalf("installed = %v", installed)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "LocalAddon", "LocalAddon.toc")); err != nil {
		t.Fatalf("local addon not installed: %v", err)
	}
	// savedvars/ was restored into Account/A1/SavedVariables.
	for _, f := range []string{"Questie.lua", filepath.Join("sub", "nested.lua")} {
		if _, err := os.Stat(filepath.Join(wtf, "Account", "A1", "SavedVariables", f)); err != nil {
			t.Fatalf("savedvars %s not restored: %v", f, err)
		}
	}
}

func TestImportZipRejectsRemoteWithoutCatalog(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "AddOns", "LocalAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "bundle.zip")
	srcDir := filepath.Join(work, "AddOns")
	addons := []ManifestAddon{
		{Folder: "LocalAddon"},
		{Folder: "Questie", Provider: "github", ID: "Vendethiel/Questie", Source: "Vendethiel/Questie"},
	}
	if err := ExportZip("pve", "wrath", addons, srcDir, "", out); err != nil {
		t.Fatal(err)
	}

	addonsDir := filepath.Join(work, "AddOns2")
	_, err := ImportZip(out, addonsDir, filepath.Join(work, "WTF"), nil, nil)
	if err == nil {
		t.Fatal("expected 'catalog required' error, got nil")
	}
	if !errors.Is(err, errCatalogRequired) && err.Error() != "catalog required" {
		t.Fatalf("error = %q, want %q", err, "catalog required")
	}
	// The catalog gate runs before any mutation.
	if entries, err := os.ReadDir(addonsDir); err == nil && len(entries) != 0 {
		t.Fatalf("remote import must not extract anything without a catalog: %v", entries)
	}
}

// errCatalogRequired mirrors the exact sentinel text the API promises.
var errCatalogRequired = errors.New("catalog required")

func TestImportZipZipSlipGuard(t *testing.T) {
	work := t.TempDir()
	bad := filepath.Join(work, "evil.zip")
	manifest := `{"version":1,"name":"evil","addons":[{"folder":"X"}]}`
	makeZip(t, bad, map[string]string{
		"manifest.json":                 manifest,
		"../escape.txt":                 "pwned",
		"addons/X/../../../escape2.txt": "pwned",
	})
	addonsDir := filepath.Join(work, "AddOns")
	_, err := ImportZip(bad, addonsDir, filepath.Join(work, "WTF"), nil, nil)
	if err == nil {
		t.Fatal("expected zip-slip error, got nil")
	}
	if !strings.Contains(err.Error(), "traversal") && !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("error %q should mention traversal", err)
	}
	for _, f := range []string{"escape.txt", "escape2.txt"} {
		if _, err := os.Stat(filepath.Join(work, f)); err == nil {
			t.Fatalf("zip-slip file %q escaped the extraction root", f)
		}
	}
}

func TestImportZipMissingManifest(t *testing.T) {
	work := t.TempDir()
	bad := filepath.Join(work, "nomanifest.zip")
	makeZip(t, bad, map[string]string{"addons/X/f.txt": "x"})
	_, err := ImportZip(bad, filepath.Join(work, "AddOns"), filepath.Join(work, "WTF"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("error = %v, want missing manifest error", err)
	}
}

func TestImportZipLocalExistsErrors(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "AddOns", "LocalAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "bundle.zip")
	addonsDir := filepath.Join(work, "AddOns")
	if err := ExportZip("pve", "wrath", []ManifestAddon{{Folder: "LocalAddon"}}, addonsDir, "", out); err != nil {
		t.Fatal(err)
	}
	_, err := ImportZip(out, addonsDir, filepath.Join(work, "WTF"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want 'already exists'", err)
	}
}

func TestImportGitHubList(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("# a comment\n\nowner/repo\n\n"))
	}))
	defer srv.Close()

	// A source list needs a catalog.
	_, err := ImportGitHubList(srv.URL, t.TempDir(), nil, nil)
	if err == nil || err.Error() != "catalog required" {
		t.Fatalf("error = %v, want 'catalog required'", err)
	}
	if hits != 1 {
		t.Fatalf("server hit %d times, want 1", hits)
	}
}

func TestImportGitHubListCommentsOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("# nothing here\n\n   \n"))
	}))
	defer srv.Close()
	installed, err := ImportGitHubList(srv.URL, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("comments-only list must succeed without a catalog: %v", err)
	}
	if len(installed) != 0 {
		t.Fatalf("installed = %v, want none", installed)
	}
}

func TestImportGitHubListHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := ImportGitHubList(srv.URL, t.TempDir(), nil, nil); err == nil {
		t.Fatal("expected HTTP error, got nil")
	}
}
