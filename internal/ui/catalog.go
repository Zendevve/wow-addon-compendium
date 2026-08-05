package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
)

// --- catalog / updates messages --------------------------------------

// searchDebounceDelay is how long the catalog search box must be quiet
// before a provider search fires. minSearchInterval is the minimum gap
// between provider searches per session (GitHub's unauthenticated search
// API allows ~10 requests/minute). Both are variables so tests can
// shrink them.
var (
	searchDebounceDelay = 400 * time.Millisecond
	minSearchInterval   = time.Second
)

type catalogSearchMsg struct {
	query   string
	results []*catalog.Addon
	err     error
}

// searchDebounceMsg is delivered when the search box has been quiet for
// searchDebounceDelay.
type searchDebounceMsg struct{}

type installDoneMsg struct {
	names []string
	err   error
}

type updatesMsg struct {
	updates []catalog.Update
	checked int
	err     error
}

type updateDoneMsg struct {
	folder string
	err    error
}

type updateAllDoneMsg struct {
	applied, failed int
	err             error
}

// --- commands ---------------------------------------------------------

// searchCmd queries every enabled provider; the result is applied only
// if the query still matches the search box (stale replies are dropped).
func (a *App) searchCmd(query string) tea.Cmd {
	if a.catalog == nil {
		return func() tea.Msg {
			return catalogSearchMsg{query: query, err: a.catErr}
		}
	}
	return func() tea.Msg {
		res, err := a.catalog.Search(a.ctx, query, 15)
		return catalogSearchMsg{query: query, results: res, err: err}
	}
}

// debounceSearch arms a quiet-period timer for the catalog search box,
// cancelling any previous pending timer. The returned command resolves
// to searchDebounceMsg when the box stays quiet for the full delay, or
// to nothing when superseded.
func (a *App) debounceSearch() tea.Cmd {
	return a.debounceFor(searchDebounceDelay)
}

func (a *App) debounceFor(d time.Duration) tea.Cmd {
	if a.searchCancel != nil {
		a.searchCancel()
	}
	done := make(chan struct{})
	a.searchCancel = func() { close(done) }
	return func() tea.Msg {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			return searchDebounceMsg{}
		case <-done:
			return nil
		}
	}
}

// cancelSearchDebounce aborts a pending debounce and clears the
// "searching…" pending flag.
func (a *App) cancelSearchDebounce() {
	if a.searchCancel != nil {
		a.searchCancel()
		a.searchCancel = nil
	}
	a.searchPending = false
}

// fireSearch runs the provider search for the current box value now,
// unless the per-session minimum interval has not elapsed, in which case
// it re-arms the debounce for the remaining time.
func (a *App) fireSearch() tea.Cmd {
	q := a.search.Value()
	if q == "" {
		a.searching = false
		a.results = nil
		a.resultCur = 0
		return nil
	}
	if wait := minSearchInterval - time.Since(a.lastSearchAt); wait > 0 {
		return a.debounceFor(wait)
	}
	a.searchPending = false
	a.searching = true
	a.lastSearchAt = time.Now()
	return a.searchCmd(q)
}

// beginInstall arms the busy state and progress widget for a download;
// the handler must follow up with installCmd.
func (a *App) beginInstall(label string) {
	a.busy = true
	a.busyText = "Installing…"
	a.installRunning = true
	p := NewProgress(a.styles)
	p.Indeterminate = true
	p.Label = label
	a.installProgress = &p
	a.progressDone.Store(0)
	a.progressTotal.Store(0)
}

// installCmd installs an addon from a URL or owner/repo source. The
// download callback writes progress counters that the main loop reads on
// spinner ticks.
func (a *App) installCmd(source string) tea.Cmd {
	return func() tea.Msg {
		if a.catalog == nil || a.install == nil {
			return installDoneMsg{err: fmt.Errorf("catalog or installation unavailable")}
		}
		names, err := a.catalog.InstallFromSource(a.ctx, source, a.install.AddonsPath,
			func(done, total int64) {
				a.progressDone.Store(done)
				a.progressTotal.Store(total)
			})
		return installDoneMsg{names: names, err: err}
	}
}

// installFromSourceCmd wires the "i" manual source prompt to the install
// pipeline. The caller (updateInputKey) has already set view and mode.
func (a *App) installFromSourceCmd(source string) tea.Cmd {
	if a.catalog == nil {
		a.pushToast("Catalog unavailable: " + a.catErr.Error())
		return nil
	}
	if a.install == nil {
		a.pushToast("No installation selected")
		return nil
	}
	a.beginInstall(source)
	return a.installCmd(source)
}

// checkUpdatesCmd compares the registry against provider latest versions.
func (a *App) checkUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		ups, err := catalog.Check(a.ctx, a.catalog, a.registry, a.install.AddonsPath)
		checked := 0
		if a.registry != nil {
			checked = len(a.registry.Entries())
		}
		return updatesMsg{updates: ups, checked: checked, err: err}
	}
}

// updateCmd updates one addon through the catalog updater.
func (a *App) updateCmd(u catalog.Update) tea.Cmd {
	return func() tea.Msg {
		if a.catalog == nil || a.install == nil {
			return updateDoneMsg{err: fmt.Errorf("catalog or installation unavailable")}
		}
		folder, err := catalog.Apply(a.ctx, a.catalog, a.install.AddonsPath, u,
			a.updateBackups(), a.log)
		return updateDoneMsg{folder: folder, err: err}
	}
}

// updateAllCmd applies every pending update sequentially.
func (a *App) updateAllCmd(updates []catalog.Update) tea.Cmd {
	return func() tea.Msg {
		applied, failed := 0, 0
		backups := a.updateBackups()
		var errs []error
		for _, u := range updates {
			if a.ctx.Err() != nil {
				break
			}
			if _, err := catalog.Apply(a.ctx, a.catalog, a.install.AddonsPath, u,
				backups, a.log); err != nil {
				failed++
				errs = append(errs, fmt.Errorf("%s: %w", u.Entry.Folder, err))
				continue
			}
			applied++
		}
		return updateAllDoneMsg{applied: applied, failed: failed, err: errors.Join(errs...)}
	}
}

// updateBackups mirrors the fixer's backup wiring for catalog installs.
func (a *App) updateBackups() *backup.Manager {
	if !a.cfg.AutoBackup {
		return nil
	}
	root, err := a.backupRoot()
	if err != nil {
		root = filepath.Join(a.store.Dir(), "backups")
	}
	return backup.New(root, a.log)
}

// refreshAfterUpdate rescans the install and re-checks for updates once
// the scan lands.
func (a *App) refreshAfterUpdate() tea.Cmd {
	if a.install == nil {
		return nil
	}
	a.recheckAfterScan = true
	return a.scanCmd(a.install.Root, a.install.Flavor)
}

// --- catalog view -----------------------------------------------------

func (a *App) updateCatalogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the search box is focused it consumes every key except esc
	// (blur), enter (blur + search now) and ctrl+c (quit).
	if a.search.Focused() {
		switch {
		case key.Matches(msg, a.keys.Escape):
			a.search.Blur()
			a.cancelSearchDebounce()
			return a, nil
		case key.Matches(msg, a.keys.Enter):
			a.search.Blur()
			return a, a.fireSearch()
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			a.quitting = true
			return a, tea.Quit
		default:
			var cmd tea.Cmd
			a.search, cmd = a.search.Update(msg)
			if a.search.Value() == "" {
				a.results = nil
				a.resultCur = 0
				a.searchPending = false
				return a, cmd
			}
			a.searchPending = true
			return a, tea.Batch(cmd, a.debounceSearch())
		}
	}
	switch {
	case key.Matches(msg, a.keys.Up):
		if len(a.results) > 0 {
			a.resultCur = maxInt(0, a.resultCur-1)
		}
	case key.Matches(msg, a.keys.Down):
		if len(a.results) > 0 {
			a.resultCur = minInt(len(a.results)-1, a.resultCur+1)
		}
	case key.Matches(msg, a.keys.Filter):
		a.search.Focus()
		return a, textinput.Blink
	case key.Matches(msg, a.keys.Enter):
		if len(a.results) == 0 || a.resultCur >= len(a.results) {
			return a, nil
		}
		a.openCatalogAction(a.results[a.resultCur])
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.cancelSearchDebounce()
		a.view = viewList
		return a, nil
	}
	return a, nil
}

func (a *App) openCatalogAction(addon *catalog.Addon) {
	a.actionAddon = addon
	a.actionChoices = []string{"Install", "Open homepage"}
	a.actionCursor = 0
	a.actionCallback = func(choice string) {
		switch choice {
		case "Install":
			source := addon.Homepage
			if source == "" && addon.Provider == catalog.ProviderGitHub {
				source = addon.ID // "owner/repo"
			}
			if source == "" {
				a.pushToast("No install source available for " + addon.Name)
				return
			}
			if a.install == nil {
				a.pushToast("No installation selected")
				return
			}
			a.beginInstall(addon.Name)
			a.cmd = a.installCmd(source)
		case "Open homepage":
			if err := openURL(addon.Homepage); err != nil {
				a.pushToast("Could not open browser: " + err.Error())
				return
			}
			a.pushToast("Opened " + addon.Homepage)
		}
	}
	a.view = viewCatalogAction
}

func (a *App) updateCatalogActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Up):
		a.actionCursor = maxInt(0, a.actionCursor-1)
	case key.Matches(msg, a.keys.Down):
		a.actionCursor = minInt(len(a.actionChoices)-1, a.actionCursor+1)
	case key.Matches(msg, a.keys.Enter):
		choice := a.actionChoices[a.actionCursor]
		cb := a.actionCallback
		a.view = viewCatalog
		if cb != nil {
			cb(choice)
			if a.cmd != nil {
				cmd := a.cmd
				a.cmd = nil
				return a, cmd
			}
		}
	case key.Matches(msg, a.keys.Escape):
		a.view = viewCatalog
	}
	return a, nil
}

// --- updates view -----------------------------------------------------

func (a *App) updateUpdatesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(a.updates)
	switch {
	case key.Matches(msg, a.keys.Up):
		if n > 0 {
			a.updatesCur = maxInt(0, a.updatesCur-1)
			a.clampUpdates()
		}
	case key.Matches(msg, a.keys.Down):
		if n > 0 {
			a.updatesCur = minInt(n-1, a.updatesCur+1)
			a.clampUpdates()
		}
	case key.Matches(msg, a.keys.Updates): // u: update selected
		if n == 0 || a.updatesCur >= n {
			return a, nil
		}
		u := a.updates[a.updatesCur]
		a.busy = true
		a.busyText = fmt.Sprintf("Updating %s…", u.Entry.Folder)
		return a, a.updateCmd(u)
	case key.Matches(msg, key.NewBinding(key.WithKeys("U"))): // U: update all
		if n == 0 {
			return a, nil
		}
		a.openConfirm(fmt.Sprintf("Update all %d addon(s)?", n),
			"Each update downloads the latest version and replaces the installed folder.",
			func() {
				a.busy = true
				a.busyText = "Updating all addons…"
				a.cmd = a.updateAllCmd(a.updates)
			})
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		if n == 0 {
			return a, nil
		}
		a.view = viewUpdatesDetail
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.view = viewList
		return a, nil
	}
	return a, nil
}

func (a *App) updateUpdatesDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape), key.Matches(msg, a.keys.Enter):
		a.view = viewUpdates
		return a, nil
	case key.Matches(msg, a.keys.Updates):
		if a.updatesCur >= 0 && a.updatesCur < len(a.updates) {
			u := a.updates[a.updatesCur]
			a.view = viewUpdates
			a.busy = true
			a.busyText = fmt.Sprintf("Updating %s…", u.Entry.Folder)
			return a, a.updateCmd(u)
		}
	}
	return a, nil
}

// clampUpdates keeps the updates cursor inside the visible window.
func (a *App) clampUpdates() {
	rows := a.visibleRows()
	if a.updatesCur < a.updatesOff {
		a.updatesOff = a.updatesCur
	}
	if a.updatesCur >= a.updatesOff+rows {
		a.updatesOff = a.updatesCur - rows + 1
	}
}

// --- rendering --------------------------------------------------------

// providerTag shortens a provider name to a two-letter badge.
func providerTag(name string) string {
	switch name {
	case catalog.ProviderGitHub:
		return "GH"
	case catalog.ProviderCurseForge:
		return "CF"
	case catalog.ProviderWowInterface:
		return "WI"
	case catalog.ProviderTukui:
		return "TK"
	}
	return "?"
}

func (a *App) renderCatalog() string {
	width := a.width - 4
	if width < 60 {
		width = 60
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render("Catalog"))
	b.WriteString("\n")
	b.WriteString(a.styles.FilterBar.Render(a.search.View()))
	b.WriteString("\n")

	if a.catalog == nil {
		b.WriteString(a.styles.Hint.Render("Catalog unavailable: " + a.catErr.Error()))
	} else if a.searchPending || a.searching {
		b.WriteString(a.styles.Hint.Render(a.spinner.View() + " searching…"))
	} else if len(a.results) == 0 {
		if a.search.Value() == "" {
			b.WriteString(a.styles.Hint.Render(
				"Type to search GitHub, CurseForge, WowInterface and Tukui."))
		} else {
			b.WriteString(a.styles.Hint.Render(fmt.Sprintf("No results for %q", a.search.Value())))
		}
	} else {
		rows := a.visibleRows()
		start := 0
		if a.resultCur >= rows {
			start = a.resultCur - rows + 1
		}
		end := start + rows
		if end > len(a.results) {
			end = len(a.results)
		}
		for i := start; i < end; i++ {
			r := a.results[i]
			mark := " "
			if i == a.resultCur {
				mark = "▸"
			}
			name := r.Name
			if r.Author != "" {
				name += "  by " + r.Author
			}
			line := fmt.Sprintf("%s %s [%s]", mark, name, providerTag(r.Provider))
			if r.LatestVersion != "" {
				line += "  v" + r.LatestVersion
			}
			if i == a.resultCur {
				b.WriteString(a.styles.RowSelected.Render(pad(line, width)))
			} else {
				b.WriteString(a.styles.Row.Render(pad(line, width)))
			}
			b.WriteString("\n")
		}
		if len(a.results) > end {
			b.WriteString(a.styles.RowMuted.Render(fmt.Sprintf("… %d more", len(a.results)-end)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" + a.styles.Hint.Render("/ search · enter open · esc back · q quit"))
	return a.styles.ListBox.Width(width + 2).Render(b.String())
}

func (a *App) renderCatalogAction() string {
	if a.actionAddon == nil {
		return ""
	}
	width := a.width - 8
	if width < 50 {
		width = 50
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render(a.actionAddon.Name))
	b.WriteString("\n\n")
	for i, c := range a.actionChoices {
		mark := " "
		if i == a.actionCursor {
			mark = "▸"
			b.WriteString(a.styles.OptionSel.Render(mark + " " + c))
		} else {
			b.WriteString(a.styles.Option.Render(mark + " " + c))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + a.styles.Hint.Render("↑/↓ choose · enter select · esc cancel"))
	panel := a.styles.Dialog.Width(width).Render(b.String())
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, panel)
}

// versionOrDash renders a version, or "—" when unknown.
func versionOrDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

func (a *App) renderUpdates() string {
	width := a.width - 4
	if width < 60 {
		width = 60
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render("Updates"))
	b.WriteString("\n")

	if a.updates == nil {
		b.WriteString(a.styles.Hint.Render("Checking for updates…"))
	} else if len(a.updates) == 0 {
		b.WriteString(a.styles.Hint.Render("All tracked addons are up to date."))
		b.WriteString("\n")
	} else {
		rows := a.visibleRows()
		start := 0
		if a.updatesCur >= rows {
			start = a.updatesCur - rows + 1
		}
		end := start + rows
		if end > len(a.updates) {
			end = len(a.updates)
		}
		for i := start; i < end; i++ {
			u := a.updates[i]
			mark := " "
			if i == a.updatesCur {
				mark = "▸"
			}
			latest := "—"
			if u.Latest != nil {
				latest = versionOrDash(u.Latest.LatestVersion)
			}
			line := fmt.Sprintf("%s %s  %s -> %s [%s]",
				mark, u.Entry.Folder,
				versionOrDash(u.Entry.Version), latest,
				providerTag(u.Entry.Provider))
			if i == a.updatesCur {
				b.WriteString(a.styles.RowSelected.Render(pad(line, width)))
			} else {
				b.WriteString(a.styles.Row.Render(pad(line, width)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n" + a.styles.RowMuted.Render(
			fmt.Sprintf("%d addon(s) up to date", a.upToDate)))
		b.WriteString("\n")
	}
	b.WriteString(a.styles.Hint.Render("u update · U update all · enter inspect · esc back · q quit"))
	return a.styles.ListBox.Width(width + 2).Render(b.String())
}

func (a *App) renderUpdatesDetail() string {
	if a.updatesCur < 0 || a.updatesCur >= len(a.updates) {
		return ""
	}
	u := a.updates[a.updatesCur]
	width := a.width - 8
	if width < 50 {
		width = 50
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render(u.Entry.Folder))
	b.WriteString("\n")
	if u.Latest != nil {
		b.WriteString("\n" + a.styles.Detail.Render("Name: ") +
			a.styles.RowName.Render(u.Latest.Name))
		if u.Latest.Author != "" {
			b.WriteString("\n" + a.styles.Detail.Render("Author: ") +
				a.styles.Path.Render(u.Latest.Author))
		}
		b.WriteString("\n" + a.styles.Detail.Render("Source: ") +
			a.styles.Path.Render(providerTag(u.Entry.Provider)+" · "+u.Entry.Source))
		b.WriteString("\n" + a.styles.Detail.Render("Installed: ") +
			a.styles.Path.Render(versionOrDash(u.Entry.Version)))
		b.WriteString("\n" + a.styles.Detail.Render("Latest: ") +
			a.styles.Path.Render(versionOrDash(u.Latest.LatestVersion)))
		if u.Latest.Summary != "" {
			b.WriteString("\n\n" + a.styles.Detail.Render(u.Latest.Summary))
		}
		if u.Latest.Homepage != "" {
			b.WriteString("\n\n" + a.styles.Detail.Render("Homepage: ") +
				a.styles.Path.Render(u.Latest.Homepage))
		}
	}
	b.WriteString("\n\n" + a.styles.Hint.Render("u update · esc back"))
	panel := a.styles.Dialog.Width(width).Render(b.String())
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, panel)
}
