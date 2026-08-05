package ui

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
)

// newTestApp builds an App with a synthetic scan of the given addons,
// sized to a fixed terminal. The catalog is swapped for an empty
// provider set so no test ever touches the network.
func newTestApp(t *testing.T, addonNames ...string) *App {
	t.Helper()
	store := config.NewStoreAt(t.TempDir() + "/config.json")
	app := NewApp(config.Default(), store, logger.New(50))
	offline, err := catalog.New(map[string]bool{}, http.DefaultClient)
	if err != nil {
		t.Fatalf("offline catalog: %v", err)
	}
	reg, err := catalog.NewRegistry(t.TempDir() + "/registry.json")
	if err != nil {
		t.Fatalf("test registry: %v", err)
	}
	app.catalog = offline
	app.registry = reg
	app.width = 100
	app.height = 30
	scan := &models.ScanResult{AddonsDir: "C:\\AddOns"}
	for _, name := range addonNames {
		scan.Addons = append(scan.Addons, &models.Addon{
			FolderName: name,
			Path:       "C:\\AddOns\\" + name,
		})
	}
	app.scan = scan
	app.view = viewList
	app.clampCursor()
	return app
}

func keyPress(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func TestFilterTypingDoesNotQuit(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot", "WeakAuras")
	// Open the filter, then type "q" — this must type, not quit.
	app.Update(keyPress("/"))
	if !app.filtering {
		t.Fatal("expected filter to open on /")
	}
	for _, r := range "quest" {
		app.Update(keyPress(string(r)))
	}
	if app.quitting {
		t.Fatal("typing q in the filter must not quit the app")
	}
	if app.filter.Value() != "quest" {
		t.Fatalf("filter value = %q, want %q", app.filter.Value(), "quest")
	}
	if len(app.filterIdx) != 1 || app.scan.Addons[app.filterIdx[0]].FolderName != "Questie" {
		t.Fatalf("filtered indices = %v, want only Questie", app.filterIdx)
	}
}

func TestFilterCloseKeys(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot")
	// esc closes the filter.
	app.Update(keyPress("/"))
	app.Update(keyPress("at"))
	if !app.filtering {
		t.Fatal("filter should be open")
	}
	app.Update(keyPress("esc"))
	if app.filtering || app.filter.Value() != "" {
		t.Fatal("esc should close the filter and clear it")
	}
	// backspace at an empty filter closes it.
	app.Update(keyPress("/"))
	app.Update(keyPress("backspace"))
	if app.filtering {
		t.Fatal("backspace on an empty filter should close it")
	}
	// backspace with text only edits the filter.
	app.Update(keyPress("/"))
	app.Update(keyPress("at"))
	app.Update(keyPress("backspace"))
	if !app.filtering || app.filter.Value() != "a" {
		t.Fatalf("filter = %q (open=%v), want %q with filter open", app.filter.Value(), app.filtering, "a")
	}
}

func TestFilterEnterInspectsFilteredAddon(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot", "WeakAuras")
	app.Update(keyPress("/"))
	for _, r := range "wa" {
		app.Update(keyPress(string(r)))
	}
	if len(app.filterIdx) != 1 {
		t.Fatalf("filtered indices = %v, want one match", app.filterIdx)
	}
	app.Update(keyPress("enter"))
	if app.view != viewInspect {
		t.Fatalf("view = %v, want viewInspect", app.view)
	}
	if app.inspectAddon == nil || app.inspectAddon.FolderName != "WeakAuras" {
		t.Fatalf("inspected %v, want WeakAuras", app.inspectAddon)
	}
}

func TestFilterNoMatches(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot")
	app.Update(keyPress("/"))
	for _, r := range "zzz" {
		app.Update(keyPress(string(r)))
	}
	if len(app.filterIdx) != 0 {
		t.Fatalf("filtered indices = %v, want none", app.filterIdx)
	}
	if !strings.Contains(app.View(), "No addons match") {
		t.Fatal("list view should report no matches")
	}
}

func TestCtrlCQuitsWhileFiltering(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.Update(keyPress("/"))
	app.Update(keyPress("ctrl+c"))
	if !app.quitting {
		t.Fatal("ctrl+c must quit even while the filter is open")
	}
}

func TestFilterResetsOnScan(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot")
	app.Update(keyPress("/"))
	app.Update(keyPress("at"))
	if len(app.filterIdx) != 1 {
		t.Fatalf("filtered indices = %v, want one", app.filterIdx)
	}
	// A fresh scan arrives: filter must reset.
	res := &models.ScanResult{AddonsDir: "C:\\AddOns"}
	res.Addons = append(res.Addons, &models.Addon{FolderName: "Questie", Path: "C:\\AddOns\\Questie"})
	inst := &detector.Installation{Root: "C:\\Games\\WoW", Flavor: "_retail_", AddonsPath: "C:\\AddOns"}
	app.Update(scanResultMsg{result: res, install: inst})
	if app.filtering || len(app.filterIdx) != 0 {
		t.Fatal("scan should close the filter")
	}
	if app.listLen() != 1 {
		t.Fatalf("listLen = %d, want 1", app.listLen())
	}
}

func TestMouseWheelMovesCursor(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot", "WeakAuras")
	wheel := func(btn tea.MouseButton) {
		t.Helper()
		app.Update(tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: btn,
		})
	}
	wheel(tea.MouseButtonWheelUp)
	if app.cursor != 0 {
		t.Fatalf("wheel up at top: cursor = %d, want 0", app.cursor)
	}
	wheel(tea.MouseButtonWheelDown)
	if app.cursor != 1 {
		t.Fatalf("wheel down: cursor = %d, want 1", app.cursor)
	}
	// Respects the filtered view.
	app.Update(keyPress("/"))
	for _, r := range "atlas" {
		app.Update(keyPress(string(r)))
	}
	before := app.cursor
	wheel(tea.MouseButtonWheelDown)
	if app.cursor != before {
		t.Fatalf("wheel down on single filtered row: cursor %d -> %d", before, app.cursor)
	}
	if app.addonAt().FolderName != "AtlasLoot" {
		t.Fatalf("cursor addon = %v, want AtlasLoot", app.addonAt())
	}
}

func TestHelpOverlay(t *testing.T) {
	app := newTestApp(t, "Questie", "AtlasLoot")
	app.Update(keyPress("?"))
	if app.view != viewHelp {
		t.Fatalf("view = %v, want viewHelp", app.view)
	}
	v := app.View()
	for _, want := range []string{"Keybindings", "Navigation", "Addon actions", "Views", "filter", "help", "quit", "esc / q close help"} {
		if !strings.Contains(v, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	// q closes help without quitting, back to the previous view.
	app.Update(keyPress("q"))
	if app.view != viewList {
		t.Fatalf("view after q = %v, want viewList", app.view)
	}
	if app.quitting {
		t.Fatal("q in help must close help, not quit")
	}
	// From inspect, ? returns to inspect.
	app.Update(keyPress("enter"))
	if app.view != viewInspect {
		t.Fatalf("view = %v, want viewInspect", app.view)
	}
	app.Update(keyPress("?"))
	app.Update(keyPress("esc"))
	if app.view != viewInspect {
		t.Fatalf("view after esc in help = %v, want viewInspect", app.view)
	}
}

func TestToastsStack(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.pushToast("one")
	app.pushToast("two")
	app.pushToast("three")
	v := app.View()
	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(v, want) {
			t.Errorf("toast stack missing %q", want)
		}
	}
}

// --- catalog / updates views -----------------------------------------

func TestCatalogViewFlow(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.Update(keyPress("c"))
	if app.view != viewCatalog {
		t.Fatalf("view = %v, want viewCatalog", app.view)
	}
	// "/" focuses the search box; typing must not quit.
	app.Update(keyPress("/"))
	if !app.search.Focused() {
		t.Fatal("/ should focus the catalog search box")
	}
	app.Update(keyPress("q"))
	app.Update(keyPress("u"))
	if app.search.Value() != "qu" {
		t.Fatalf("search value = %q, want qu", app.search.Value())
	}
	if app.quitting {
		t.Fatal("typing q in the catalog search must not quit")
	}
	// The async search result (offline providers -> empty, no network).
	app.Update(catalogSearchMsg{query: "qu"})
	if app.results != nil {
		t.Fatalf("results = %v, want none", app.results)
	}
	if !strings.Contains(app.View(), "No results") {
		t.Fatal("catalog view should report no results")
	}
	// esc blurs the search box; esc again returns to the list.
	app.Update(keyPress("esc"))
	if app.search.Focused() {
		t.Fatal("esc should blur the search box")
	}
	app.Update(keyPress("esc"))
	if app.view != viewList {
		t.Fatalf("view = %v, want viewList", app.view)
	}
}

func TestCatalogActionDialog(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.results = []*catalog.Addon{{
		Name: "FakeAddon", Author: "someone",
		Provider: catalog.ProviderGitHub, LatestVersion: "1.2.3",
		Homepage: "https://github.com/a/b",
	}}
	app.view = viewCatalog
	app.Update(keyPress("enter"))
	if app.view != viewCatalogAction {
		t.Fatalf("view = %v, want viewCatalogAction", app.view)
	}
	v := app.View()
	for _, want := range []string{"Install", "Open homepage"} {
		if !strings.Contains(v, want) {
			t.Errorf("action dialog missing %q", want)
		}
	}
	// esc cancels back to the catalog.
	app.Update(keyPress("esc"))
	if app.view != viewCatalog {
		t.Fatalf("view after esc = %v, want viewCatalog", app.view)
	}
	// enter again and pick "Open homepage" with an empty URL: must fail
	// gracefully with a toast, no browser spawned.
	app.Update(keyPress("enter"))
	app.Update(keyPress("down"))
	app.Update(keyPress("enter"))
	if app.view != viewCatalog {
		t.Fatalf("view after action = %v, want viewCatalog", app.view)
	}
	if len(app.toasts) == 0 {
		t.Fatal("expected a toast after the homepage action")
	}
}

func TestCatalogInstallFlow(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.install = &detector.Installation{
		Root: "C:\\Games\\WoW", Flavor: "_retail_", AddonsPath: "C:\\AddOns",
	}
	app.results = []*catalog.Addon{{
		Name: "FakeAddon", Provider: catalog.ProviderGitHub, LatestVersion: "1.2.3",
		Homepage: "https://github.com/a/b",
	}}
	app.view = viewCatalog
	app.Update(keyPress("enter"))
	if app.view != viewCatalogAction {
		t.Fatalf("view = %v, want viewCatalogAction", app.view)
	}
	_, cmd := app.Update(keyPress("enter")) // Install (cursor 0)
	if app.view != viewCatalog {
		t.Fatalf("view after choosing install = %v, want viewCatalog", app.view)
	}
	if !app.busy || !app.installRunning {
		t.Fatal("install should arm the busy/progress state")
	}
	// Run the returned command: the offline catalog fails fast.
	var msg tea.Msg
	if cmd == nil {
		t.Fatal("install should return a command")
	} else {
		msg = cmd()
	}
	if _, ok := msg.(installDoneMsg); !ok {
		t.Fatalf("cmd returned %T, want installDoneMsg", msg)
	}
	app.Update(msg)
	if app.busy || app.installRunning {
		t.Fatal("install completion should clear busy state")
	}
}

func TestUpdatesViewFlow(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.install = &detector.Installation{
		Root: "C:\\Games\\WoW", Flavor: "_retail_", AddonsPath: "C:\\AddOns",
	}
	app.Update(keyPress("u"))
	if !app.busy {
		t.Fatal("update check should set busy")
	}
	// Empty registry -> no updates, no network.
	app.Update(updatesMsg{updates: []catalog.Update{}, checked: 0})
	if app.view != viewUpdates {
		t.Fatalf("view = %v, want viewUpdates", app.view)
	}
	if app.busy {
		t.Fatal("updates result should clear busy")
	}
	if !strings.Contains(app.View(), "up to date") {
		t.Fatal("updates view missing the up-to-date summary")
	}
	// esc returns to the list.
	app.Update(keyPress("esc"))
	if app.view != viewList {
		t.Fatalf("view after esc = %v, want viewList", app.view)
	}
}

func TestUpdateSelectedAndAll(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.install = &detector.Installation{
		Root: "C:\\Games\\WoW", Flavor: "_retail_", AddonsPath: "C:\\AddOns",
	}
	app.updates = []catalog.Update{{
		Entry:  catalog.Entry{Folder: "Questie", Version: "1.0.0", Provider: catalog.ProviderGitHub},
		Latest: &catalog.Addon{Name: "Questie", LatestVersion: "2.0.0"},
	}}
	app.view = viewUpdates

	// enter inspects the selected update.
	app.Update(keyPress("enter"))
	if app.view != viewUpdatesDetail {
		t.Fatalf("view = %v, want viewUpdatesDetail", app.view)
	}
	v := app.View()
	if !strings.Contains(v, "1.0.0") || !strings.Contains(v, "2.0.0") {
		t.Fatalf("detail view missing versions: %q", v)
	}
	app.Update(keyPress("esc"))
	if app.view != viewUpdates {
		t.Fatalf("view after esc = %v, want viewUpdates", app.view)
	}

	// u updates the selected addon through the offline catalog (fails
	// fast, no network).
	_, cmd := app.Update(keyPress("u"))
	if !app.busy {
		t.Fatal("update should set busy")
	}
	var msg tea.Msg
	if cmd == nil {
		t.Fatal("update should return a command")
	} else {
		msg = cmd()
	}
	if _, ok := msg.(updateDoneMsg); !ok {
		t.Fatalf("cmd returned %T, want updateDoneMsg", msg)
	}
	app.Update(msg)
	if app.busy {
		t.Fatal("update completion should clear busy")
	}

	// U update all opens a confirmation; y runs it.
	app.Update(keyPress("U"))
	if app.view != viewConfirm {
		t.Fatalf("view = %v, want viewConfirm", app.view)
	}
	_, cmd = app.Update(keyPress("y"))
	if cmd == nil {
		t.Fatal("confirming update all should return a command")
	}
	if msg = cmd(); msg == nil {
		t.Fatal("update-all command returned nil message")
	}
	app.Update(msg)
	if app.busy {
		t.Fatal("update-all completion should clear busy")
	}
}

func TestCatalogUnavailableToast(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.catalog = nil
	app.catErr = fmt.Errorf("broken")
	app.Update(keyPress("c"))
	if app.view != viewList {
		t.Fatalf("view = %v, want viewList when catalog is nil", app.view)
	}
	if len(app.toasts) == 0 {
		t.Fatal("expected a catalog-unavailable toast")
	}
}

// --- catalog search debounce / rate limiting --------------------------

func TestSearchDebounceNoSynchronousSearches(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.view = viewCatalog
	app.Update(keyPress("/"))
	for _, r := range "atlas" {
		app.Update(keyPress(string(r)))
	}
	// Typing must not fire provider searches synchronously; only a
	// debounce is pending.
	if app.searching {
		t.Fatal("no search should fire while typing")
	}
	if !app.searchPending {
		t.Fatal("a debounce should be pending while typing")
	}
	if app.results != nil {
		t.Fatal("results must not appear before the quiet period elapses")
	}
}

func TestSearchDebounceFiresAfterQuietPeriod(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.view = viewCatalog
	app.Update(keyPress("/"))
	for _, r := range "atlas" {
		app.Update(keyPress(string(r)))
	}
	// Quiet period elapses: exactly one search fires with the final value.
	_, cmd := app.Update(searchDebounceMsg{})
	if cmd == nil {
		t.Fatal("debounce expiry should produce a search command")
	}
	if !app.searching {
		t.Fatal("searching flag should be set while the search is in flight")
	}
	msg := cmd()
	sm, ok := msg.(catalogSearchMsg)
	if !ok {
		t.Fatalf("search command returned %T, want catalogSearchMsg", msg)
	}
	if sm.query != "atlas" {
		t.Fatalf("search query = %q, want atlas", sm.query)
	}
	app.Update(msg)
	if app.searching || app.searchPending {
		t.Fatal("the result should clear the searching flags")
	}
}

func TestSearchMinIntervalReArms(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.view = viewCatalog
	app.search.SetValue("atlas")
	old := minSearchInterval
	minSearchInterval = 5 * time.Millisecond
	defer func() { minSearchInterval = old }()
	app.lastSearchAt = time.Now()
	cmd := app.fireSearch()
	if cmd == nil {
		t.Fatal("expected a re-arm debounce when the interval has not elapsed")
	}
	// The re-arm waits only for the remaining interval, then resolves.
	if msg := cmd(); msg == nil {
		t.Fatal("re-arm command returned nothing")
	} else if _, ok := msg.(searchDebounceMsg); !ok {
		t.Fatalf("re-arm returned %T, want searchDebounceMsg", msg)
	}
	// Once the interval has elapsed, the same call fires the search.
	app.lastSearchAt = time.Time{}
	cmd = app.fireSearch()
	if msg := cmd(); msg == nil {
		t.Fatal("interval elapsed: expected a search command")
	} else if _, ok := msg.(catalogSearchMsg); !ok {
		t.Fatalf("expected catalogSearchMsg, got %T", msg)
	}
}

func TestSearchShowsSearchingHint(t *testing.T) {
	app := newTestApp(t, "Questie")
	app.view = viewCatalog
	app.searching = true
	if !strings.Contains(app.View(), "searching") {
		t.Fatal("catalog view should show a searching hint while in flight")
	}
	app.searching = false
	app.searchPending = true
	if !strings.Contains(app.View(), "searching") {
		t.Fatal("catalog view should show a searching hint while a debounce is pending")
	}
}
