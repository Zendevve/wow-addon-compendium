package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wowfix/wowfix/internal/catalog"
)

// --- relTime ---------------------------------------------------------

func TestRelTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero", time.Time{}, "—"},
		{"seconds", now.Add(-30 * time.Second), "just now"},
		{"future", now.Add(time.Hour), "just now"},
		{"minutes", now.Add(-5 * time.Minute), "5m"},
		{"hours", now.Add(-3 * time.Hour), "3h"},
		{"days", now.Add(-2 * 24 * time.Hour), "2d"},
		{"old", now.Add(-120 * 24 * time.Hour), now.Add(-120 * 24 * time.Hour).Format("2006-01-02")},
	}
	for _, c := range cases {
		if got := relTime(c.in); got != c.want {
			t.Errorf("relTime(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

// --- sort cycle ------------------------------------------------------

func TestCatalogSortCycle(t *testing.T) {
	app := newTestApp(t)
	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-2 * time.Hour)
	app.results = []*catalog.Addon{
		{Name: "Zeta", Provider: catalog.ProviderCurseForge, GameVersion: "wrath", UpdatedAt: older},
		{Name: "Alpha", Provider: catalog.ProviderGitHub, GameVersion: "vanilla", UpdatedAt: newer},
	}
	app.view = viewCatalog

	// Default mode: by name.
	got := app.catalogRows()
	if got[0].Name != "Alpha" || got[1].Name != "Zeta" {
		t.Fatalf("name sort order = %q, %q; want Alpha, Zeta", got[0].Name, got[1].Name)
	}

	// S -> updated (newest first).
	app.Update(keyPress("S"))
	if app.catalogSort != catalogSortUpdated {
		t.Fatalf("sort = %q, want %q", app.catalogSort, catalogSortUpdated)
	}
	got = app.catalogRows()
	if got[0].Name != "Alpha" || got[1].Name != "Zeta" {
		t.Fatalf("updated sort order = %q, %q; want newest first", got[0].Name, got[1].Name)
	}

	// S -> provider.
	app.Update(keyPress("S"))
	if app.catalogSort != catalogSortProvider {
		t.Fatalf("sort = %q, want %q", app.catalogSort, catalogSortProvider)
	}
	got = app.catalogRows()
	if got[0].Provider != catalog.ProviderCurseForge || got[0].Name != "Zeta" {
		t.Fatalf("provider sort first = %s %q, want curseforge Zeta", got[0].Provider, got[0].Name)
	}

	// S -> back to name.
	app.Update(keyPress("S"))
	if app.catalogSort != catalogSortName {
		t.Fatalf("sort = %q, want %q", app.catalogSort, catalogSortName)
	}

	// The message slice is never mutated by sorting.
	if len(app.results) != 2 || app.results[0].Name != "Zeta" || app.results[1].Name != "Alpha" {
		t.Fatal("catalogRows must return a sorted copy, never mutating the results slice")
	}
}

// --- version filter --------------------------------------------------

func TestCatalogVersionFilter(t *testing.T) {
	app := newTestApp(t)
	app.results = []*catalog.Addon{
		{Name: "A", GameVersion: "wrath"},
		{Name: "B", GameVersion: "retail"},
		{Name: "C", GameVersion: "wrath"},
		{Name: "D", GameVersion: ""},
	}
	app.view = viewCatalog

	// "all" matches every row, including empty GameVersion.
	if got := app.catalogRows(); len(got) != 4 {
		t.Fatalf("rows under all = %d, want 4", len(got))
	}

	// Cycle all -> vanilla -> tbc -> wrath.
	app.Update(keyPress("W"))
	app.Update(keyPress("W"))
	app.Update(keyPress("W"))
	if app.catalogFilter != "wrath" {
		t.Fatalf("filter = %q, want wrath", app.catalogFilter)
	}
	got := app.catalogRows()
	if len(got) != 2 {
		t.Fatalf("wrath rows = %d, want 2", len(got))
	}
	for _, r := range got {
		if r.GameVersion != "wrath" {
			t.Fatalf("row %q has GameVersion %q, want wrath", r.Name, r.GameVersion)
		}
	}

	// Cycle wrath -> cata -> retail -> all.
	app.Update(keyPress("W"))
	app.Update(keyPress("W"))
	app.Update(keyPress("W"))
	if app.catalogFilter != "" {
		t.Fatalf("filter = %q, want all after a full cycle", app.catalogFilter)
	}
	if got := app.catalogRows(); len(got) != 4 {
		t.Fatalf("rows after full cycle = %d, want 4", len(got))
	}

	// Header shows the active filter and hides it when "all"; the sort
	// is always visible.
	app.catalogFilter = "wrath"
	v := app.View()
	if !strings.Contains(v, "filter: wrath") {
		t.Fatal("catalog header should show the active filter")
	}
	app.catalogFilter = ""
	v = app.View()
	if strings.Contains(v, "filter: ") {
		t.Fatal("catalog header should hide the filter when all")
	}
	if !strings.Contains(v, "sort: name") {
		t.Fatal("catalog header should show the active sort")
	}
}

// --- markdown stripper -----------------------------------------------

func TestStripMarkdown(t *testing.T) {
	fence := "```"
	fixture := strings.Join([]string{
		"# Awesome Addon v2.0",
		"",
		"**New features** and a [link](https://example.com/docs) plus `inline code`.",
		"",
		"- feature one",
		"- feature two",
		"",
		fence + "go",
		"func main() {}",
		fence,
		"",
		"> quoted note",
		"",
		"---",
	}, "\n")
	want := "Awesome Addon v2.0\n\n" +
		"New features and a https://example.com/docs plus inline code.\n\n" +
		"feature one\nfeature two\n\n" +
		"quoted note"
	if got := stripMarkdown(fixture); got != want {
		t.Errorf("stripMarkdown:\n got %q\nwant %q", got, want)
	}
}

// --- detail view -----------------------------------------------------

func TestCatalogDetailRendersOfflineSafe(t *testing.T) {
	app := newTestApp(t)
	app.results = []*catalog.Addon{{
		Name: "Bare", Provider: catalog.ProviderWowInterface, LatestVersion: "1.0",
	}}
	app.view = viewCatalog

	// d opens the detail view. Non-github providers never arm a fetch,
	// so no network is touched.
	app.Update(keyPress("d"))
	if app.view != viewCatalogDetail {
		t.Fatalf("view = %v, want viewCatalogDetail", app.view)
	}
	v := app.View()
	for _, want := range []string{"Bare", "Provider:", "wowinterface", "Latest version:", "1.0"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail view missing %q", want)
		}
	}

	// esc returns to the results.
	app.Update(keyPress("esc"))
	if app.view != viewCatalog {
		t.Fatalf("view after esc = %v, want viewCatalog", app.view)
	}
}

func TestCatalogDetailGitHubReleaseNotesCached(t *testing.T) {
	app := newTestApp(t)
	app.results = []*catalog.Addon{{
		Name: "G", Provider: catalog.ProviderGitHub, ID: "owner/repo",
	}}
	app.view = viewCatalog

	_, cmd := app.Update(keyPress("d"))
	if app.view != viewCatalogDetail {
		t.Fatalf("view = %v, want viewCatalogDetail", app.view)
	}
	if cmd == nil {
		t.Fatal("opening a github addon should arm a release-notes fetch")
	}
	// The command is not executed here, so no network is touched.
	v := app.View()
	if !strings.Contains(v, "Release notes") || !strings.Contains(v, "Loading release notes") {
		t.Fatalf("detail view missing release-notes placeholder: %q", v)
	}

	// A failed fetch is cached and renders as unavailable.
	app.Update(releaseNotesMsg{id: "owner/repo", err: fmt.Errorf("offline")})
	v = app.View()
	if !strings.Contains(v, "Release notes unavailable") {
		t.Fatalf("detail view should report unavailable notes: %q", v)
	}

	// Reopening the detail view must not re-arm a fetch (cache hit).
	app.Update(keyPress("esc"))
	if _, cmd = app.Update(keyPress("d")); cmd != nil {
		t.Fatal("reopening detail must not re-fetch cached release notes")
	}

	// A cached body renders.
	app.Update(releaseNotesMsg{id: "owner/repo", text: "v2.0 notes"})
	if v = app.View(); !strings.Contains(v, "v2.0 notes") {
		t.Fatalf("detail view missing cached release notes: %q", v)
	}

	// g on a non-github addon is a graceful no-op — no browser spawn,
	// no toast, no panic.
	app.detailAddon = &catalog.Addon{Name: "B", Provider: catalog.ProviderCurseForge, ID: "123"}
	app.Update(keyPress("g"))
	if len(app.toasts) != 0 {
		t.Fatal("g on a non-github addon must not open anything")
	}
	if app.view != viewCatalogDetail {
		t.Fatalf("view = %v, want viewCatalogDetail", app.view)
	}
}

func TestCatalogActionDialogViewDetails(t *testing.T) {
	app := newTestApp(t)
	app.results = []*catalog.Addon{{
		Name: "FakeAddon", Provider: catalog.ProviderGitHub, ID: "a/b",
		LatestVersion: "1.2.3",
		Homepage:      "https://github.com/a/b",
	}}
	app.view = viewCatalog
	app.Update(keyPress("enter")) // open the action dialog
	if app.view != viewCatalogAction {
		t.Fatalf("view = %v, want viewCatalogAction", app.view)
	}
	v := app.View()
	if !strings.Contains(v, "View details") {
		t.Fatalf("action dialog missing View details option: %q", v)
	}
	// The dialog still offers install and homepage first.
	if !strings.Contains(v, "Install") || !strings.Contains(v, "Open homepage") {
		t.Fatalf("action dialog missing existing options: %q", v)
	}
	// Third option opens the detail view (github: fetch armed but not
	// executed, so no network).
	app.Update(keyPress("down"))
	app.Update(keyPress("down"))
	_, cmd := app.Update(keyPress("enter"))
	if app.view != viewCatalogDetail {
		t.Fatalf("view = %v, want viewCatalogDetail", app.view)
	}
	if cmd == nil {
		t.Fatal("View details on a github addon should arm the release-notes fetch")
	}
}

// --- rendering -------------------------------------------------------

func TestCatalogColumnsShowUpdatedAndSummary(t *testing.T) {
	app := newTestApp(t)
	app.results = []*catalog.Addon{{
		Name: "Cols", Author: "author", Provider: catalog.ProviderGitHub,
		LatestVersion: "1.2.3", Summary: "A useful one-line summary.",
		UpdatedAt: time.Now().Add(-3 * 24 * time.Hour),
	}}
	app.view = viewCatalog
	v := app.View()
	for _, want := range []string{"Cols", "3d", "A useful one-line summary."} {
		if !strings.Contains(v, want) {
			t.Errorf("catalog row missing %q", want)
		}
	}
}

func TestCatalogRenderNarrowWidth(t *testing.T) {
	app := newTestApp(t)
	app.width = 40
	app.height = 20
	app.results = []*catalog.Addon{{
		Name: "A very long addon name by a very long author", Author: "someone",
		Provider: catalog.ProviderGitHub, LatestVersion: "1.2.3",
		Summary:   strings.Repeat("s", 200),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	}}
	app.view = viewCatalog
	if v := app.View(); !strings.Contains(v, "A very long addon name") {
		t.Fatalf("narrow catalog render lost the row: %q", v)
	}
}
