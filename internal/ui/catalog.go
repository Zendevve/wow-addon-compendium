package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
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

// httpClient is the client used for out-of-band UI fetches such as
// GitHub release notes. It is a package-level variable so tests can
// substitute a stub.
var httpClient = &http.Client{}

// Catalog sort modes, cycled with S in the catalog view.
const (
	catalogSortName     = "name"
	catalogSortUpdated  = "updated"
	catalogSortProvider = "provider"
)

// catalogFilterOrder is the game-version family cycle driven by W in the
// catalog view. "" is "all" and matches every addon.
var catalogFilterOrder = []string{"", "vanilla", "tbc", "wrath", "cata", "retail"}

type catalogSearchMsg struct {
	query   string
	results []*catalog.Addon
	err     error
}

// searchDebounceMsg is delivered when the search box has been quiet for
// searchDebounceDelay.
type searchDebounceMsg struct{}

// releaseNotesMsg carries the body of a GitHub latest-release fetched for
// the catalog detail view.
type releaseNotesMsg struct {
	id   string
	text string
	err  error
}

// releaseNotesResult is the per-addon cache entry for release notes; a
// failed or empty fetch is cached as unavailable so the rate-limited
// GitHub API is never re-polled within a session.
type releaseNotesResult struct {
	text string
	ok   bool
}

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

// releaseNotesCmd lazily fetches the latest GitHub release notes for a
// github-provider addon. The result is cached per addon ID so opening
// the detail view twice never re-fetches (GitHub's unauthenticated API
// is rate limited). Non-github addons and unparseable IDs return no
// command; failures are cached as unavailable.
func (a *App) releaseNotesCmd(addon *catalog.Addon) tea.Cmd {
	if !isGitHubRepo(addon) {
		return nil
	}
	if _, ok := a.releaseNotes[addon.ID]; ok {
		return nil
	}
	parts := strings.SplitN(addon.ID, "/", 2)
	owner, repo := parts[0], parts[1]
	return func() tea.Msg {
		req, err := http.NewRequestWithContext(a.ctx, http.MethodGet,
			"https://api.github.com/repos/"+owner+"/"+repo+"/releases/latest", nil)
		if err != nil {
			return releaseNotesMsg{id: addon.ID, err: err}
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "wowfix")
		resp, err := httpClient.Do(req)
		if err != nil {
			return releaseNotesMsg{id: addon.ID, err: err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return releaseNotesMsg{id: addon.ID, err: err}
		}
		var rel struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(body, &rel); err != nil {
			return releaseNotesMsg{id: addon.ID, err: err}
		}
		return releaseNotesMsg{id: addon.ID, text: stripMarkdown(rel.Body)}
	}
}

// isGitHubRepo reports whether the addon is github-sourced with an ID
// that parses as owner/repo.
func isGitHubRepo(addon *catalog.Addon) bool {
	if addon == nil || addon.Provider != catalog.ProviderGitHub {
		return false
	}
	parts := strings.SplitN(addon.ID, "/", 2)
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
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

// confirmMismatchUpdate opens the confirm dialog for an update (or
// several) whose latest release targets a different game version than
// the configured profile. The "yes" callback applies; the default
// "no" skips the mismatching updates.
func (a *App) confirmMismatchUpdate(n int, message string, yes func()) {
	a.openConfirm(fmt.Sprintf("%d update(s) target a different game version. Apply anyway?", n),
		message, yes)
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
	a.scanOrigin = scanOriginRescan
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
	case key.Matches(msg, key.NewBinding(key.WithKeys("S"))):
		a.cycleCatalogSort()
		a.clampCatalogCursor()
	case key.Matches(msg, key.NewBinding(key.WithKeys("W"))):
		a.cycleCatalogFilter()
		a.clampCatalogCursor()
	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		addon := a.catalogResultAt()
		if addon == nil {
			return a, nil
		}
		a.openCatalogDetail(addon)
		return a, a.releaseNotesCmd(addon)
	case key.Matches(msg, a.keys.Enter):
		addon := a.catalogResultAt()
		if addon == nil {
			return a, nil
		}
		a.openCatalogAction(addon)
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
	a.actionChoices = []string{"Install", "Open homepage", "View details"}
	a.actionCursor = 0
	a.actionCallback = func(choice string) {
		switch choice {
		case "Install":
			source := installSourceFor(addon)
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
		case "View details":
			a.openCatalogDetail(addon)
			a.cmd = a.releaseNotesCmd(addon)
		}
	}
	a.view = viewCatalogAction
}

// installSourceFor resolves the install source for an addon: the
// homepage when known, else the provider-scoped id for GitHub.
func installSourceFor(addon *catalog.Addon) string {
	if addon.Homepage != "" {
		return addon.Homepage
	}
	if addon.Provider == catalog.ProviderGitHub {
		return addon.ID
	}
	return ""
}

// openCatalogDetail switches to the catalog detail view.
func (a *App) openCatalogDetail(addon *catalog.Addon) {
	a.detailAddon = addon
	a.detailOffset = 0
	a.view = viewCatalogDetail
}

func (a *App) updateCatalogDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	addon := a.detailAddon
	switch {
	case key.Matches(msg, a.keys.Escape), key.Matches(msg, a.keys.Enter):
		a.view = viewCatalog
		a.detailAddon = nil
		return a, nil
	case key.Matches(msg, a.keys.Up):
		a.detailOffset = maxInt(0, a.detailOffset-1)
	case key.Matches(msg, a.keys.Down):
		a.detailOffset++
	case key.Matches(msg, key.NewBinding(key.WithKeys("o"))):
		if addon == nil || addon.Homepage == "" {
			if addon != nil {
				a.pushToast("No homepage for " + addon.Name)
			}
			return a, nil
		}
		if err := openURL(addon.Homepage); err != nil {
			a.pushToast("Could not open browser: " + err.Error())
			return a, nil
		}
		a.pushToast("Opened " + addon.Homepage)
	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		if !isGitHubRepo(addon) {
			return a, nil
		}
		u := "https://github.com/" + addon.ID + "/releases"
		if err := openURL(u); err != nil {
			a.pushToast("Could not open browser: " + err.Error())
			return a, nil
		}
		a.pushToast("Opened " + u)
	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		if addon == nil {
			return a, nil
		}
		source := installSourceFor(addon)
		if source == "" {
			a.pushToast("No install source available for " + addon.Name)
			return a, nil
		}
		if a.install == nil {
			a.pushToast("No installation selected")
			return a, nil
		}
		a.view = viewCatalog
		a.detailAddon = nil
		a.beginInstall(addon.Name)
		return a, a.installCmd(source)
	}
	return a, nil
}

// catalogRows returns the displayed catalog results: the message slice
// filtered by the active version filter and sorted by the active sort
// mode. The returned slice is a copy — the message slice is never
// mutated.
func (a *App) catalogRows() []*catalog.Addon {
	out := make([]*catalog.Addon, 0, len(a.results))
	for _, r := range a.results {
		if a.catalogFilter != "" && r.GameVersion != a.catalogFilter {
			continue
		}
		out = append(out, r)
	}
	switch a.catalogSort {
	case catalogSortUpdated:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		})
	case catalogSortProvider:
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Provider != out[j].Provider {
				return out[i].Provider < out[j].Provider
			}
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	default:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	}
	return out
}

// catalogResultAt returns the addon under the catalog cursor in the
// displayed (sorted/filtered) order, or nil.
func (a *App) catalogResultAt() *catalog.Addon {
	rows := a.catalogRows()
	if a.resultCur < 0 || a.resultCur >= len(rows) {
		return nil
	}
	return rows[a.resultCur]
}

// clampCatalogCursor keeps the catalog cursor inside the displayed rows.
func (a *App) clampCatalogCursor() {
	n := len(a.catalogRows())
	if n == 0 {
		a.resultCur = 0
		return
	}
	if a.resultCur >= n {
		a.resultCur = n - 1
	}
}

// cycleCatalogSort advances the catalog sort mode: name -> updated ->
// provider -> name.
func (a *App) cycleCatalogSort() {
	switch a.catalogSort {
	case catalogSortUpdated:
		a.catalogSort = catalogSortProvider
	case catalogSortProvider:
		a.catalogSort = catalogSortName
	default:
		a.catalogSort = catalogSortUpdated
	}
}

// cycleCatalogFilter advances the game-version filter: all -> vanilla ->
// tbc -> wrath -> cata -> retail -> all. Empty GameVersion rows match
// only under "all".
func (a *App) cycleCatalogFilter() {
	for i, f := range catalogFilterOrder {
		if f == a.catalogFilter {
			a.catalogFilter = catalogFilterOrder[(i+1)%len(catalogFilterOrder)]
			return
		}
	}
	a.catalogFilter = ""
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
		if u.Mismatch {
			a.confirmMismatchUpdate(1,
				"Updates for other game versions are skipped by “update all”.",
				func() {
					a.busy = true
					a.busyText = fmt.Sprintf("Updating %s…", u.Entry.Folder)
					a.cmd = a.updateCmd(u)
				})
			return a, nil
		}
		a.busy = true
		a.busyText = fmt.Sprintf("Updating %s…", u.Entry.Folder)
		return a, a.updateCmd(u)
	case key.Matches(msg, key.NewBinding(key.WithKeys("U"))): // U: update all
		if n == 0 {
			return a, nil
		}
		mismatches := 0
		for _, u := range a.updates {
			if u.Mismatch {
				mismatches++
			}
		}
		if mismatches > 0 {
			a.confirmMismatchUpdate(mismatches,
				fmt.Sprintf("%d update(s) match your profile; the rest would be skipped unless applied anyway.",
					n-mismatches),
				func() {
					a.busy = true
					a.busyText = "Updating all addons…"
					a.cmd = a.updateAllCmd(a.updates)
				})
			// no (default): skip mismatches, apply the rest.
			a.confirmNo = func() {
				var ok []catalog.Update
				for _, u := range a.updates {
					if !u.Mismatch {
						ok = append(ok, u)
					}
				}
				a.view = viewUpdates
				a.busy = true
				a.busyText = "Updating addons…"
				a.cmd = a.updateAllCmd(ok)
			}
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
			if u.Mismatch {
				a.confirmMismatchUpdate(1,
					"Updates for other game versions are skipped by “update all”.",
					func() {
						a.busy = true
						a.busyText = fmt.Sprintf("Updating %s…", u.Entry.Folder)
						a.cmd = a.updateCmd(u)
					})
				return a, nil
			}
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

// relTime renders a timestamp as a short relative string: "just now",
// "Nm", "Nh", "Nd", or the absolute date once older than 90 days.
func relTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d < 90*24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	default:
		return t.Format("2006-01-02")
	}
}

// Markdown stripdown regexes, applied in order by stripMarkdown. The
// line-anchored ones use [ \t] for leading whitespace so they never
// consume blank-line newlines before the marker.
var (
	mdImageRe  = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdFenceRe  = regexp.MustCompile("(?ms)```.*?```")
	mdInlineRe = regexp.MustCompile("`([^`]*)`")
	mdLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	mdBoldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdItalicRe = regexp.MustCompile(`\*([^*]+)\*`)
	mdStrikeRe = regexp.MustCompile(`~~([^~]+)~~`)
	mdHeaderRe = regexp.MustCompile(`(?m)^[ \t]{0,3}#{1,6}[ \t]*`)
	mdQuoteRe  = regexp.MustCompile(`(?m)^[ \t]*>[ \t]?`)
	mdBulletRe = regexp.MustCompile(`(?m)^[ \t]*[-*+][ \t]+`)
	mdRuleRe   = regexp.MustCompile(`(?m)^[ \t]*[-*_]{3,}[ \t]*$`)
	mdBlankRe  = regexp.MustCompile(`\n{3,}`)
)

// stripMarkdown reduces a GitHub release body to plain text: code
// fences and images are removed, inline formatting is unwrapped, links
// keep their URL, and header/quote/list markers are dropped.
func stripMarkdown(s string) string {
	s = mdImageRe.ReplaceAllString(s, "")
	s = mdFenceRe.ReplaceAllString(s, "")
	s = mdInlineRe.ReplaceAllString(s, "$1")
	s = mdLinkRe.ReplaceAllString(s, "$2")
	s = mdBoldRe.ReplaceAllString(s, "$1")
	s = mdItalicRe.ReplaceAllString(s, "$1")
	s = mdStrikeRe.ReplaceAllString(s, "$1")
	s = mdHeaderRe.ReplaceAllString(s, "")
	s = mdQuoteRe.ReplaceAllString(s, "")
	s = mdBulletRe.ReplaceAllString(s, "")
	s = mdRuleRe.ReplaceAllString(s, "")
	s = mdBlankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func (a *App) renderCatalog() string {
	width := a.contentWidth()
	if width < 60 {
		width = 60
	}
	st := a.styles
	var b strings.Builder
	meta := "sort: " + a.catalogSort
	if a.catalogFilter != "" {
		meta += "  ·  filter: " + a.catalogFilter
	}
	b.WriteString(a.renderViewHeader("Catalog", meta, width))
	b.WriteString(st.FilterBar.Render(a.search.View()))
	b.WriteString("\n")

	switch {
	case a.catalog == nil:
		b.WriteString(a.renderEmptyState("Catalog unavailable.", a.catErr.Error()))
	case a.searchPending || a.searching:
		b.WriteString(st.RowMuted.Render(a.spinner.View() + " searching…"))
	case len(a.catalogRows()) == 0:
		switch {
		case a.search.Value() == "":
			b.WriteString(a.renderEmptyState(
				"Search the catalog.",
				"type a name and press / to search GitHub, CurseForge, WowInterface, Tukui"))
		case a.catalogFilter != "":
			b.WriteString(a.renderEmptyState(
				fmt.Sprintf("No %s addons match %q.", a.catalogFilter, a.search.Value()),
				"W cycle filter · S sort · esc back"))
		default:
			b.WriteString(a.renderEmptyState(
				fmt.Sprintf("No results for %q.", a.search.Value()),
				"try a shorter name · esc back"))
		}
	default:
		rows := a.catalogRows()
		vis := a.visibleRows()
		start := 0
		if a.resultCur >= vis {
			start = a.resultCur - vis + 1
		}
		end := start + vis
		if end > len(rows) {
			end = len(rows)
		}
		updatedW := 10
		for i := start; i < end; i++ {
			r := rows[i]
			selected := i == a.resultCur
			name := r.Name
			if r.Author != "" {
				name += "  by " + r.Author
			}
			tag := providerTag(r.Provider)
			ver := ""
			if r.LatestVersion != "" {
				ver = "  v" + r.LatestVersion
			}
			// Lay the row out in plain text first so the truncate/pad
			// helpers work on runes, then apply styles at the end.
			prefix := fmt.Sprintf("%s%s [%s]%s", a.pickerMarker(selected), name, tag, ver)
			// The prefix is elastic too: truncate it so the summary
			// keeps its minimum width instead of overflowing the line.
			prefixMax := width - updatedW - 3 - 12
			if prefixMax < 8 {
				prefixMax = 8
			}
			prefix = truncate(prefix, prefixMax)
			summaryW := width - lipgloss.Width(prefix) - updatedW - 3
			if summaryW < 12 {
				summaryW = 12
			}
			summary := strings.ReplaceAll(r.Summary, "\n", " ")
			line := fmt.Sprintf("%s %s %s", prefix, pad(relTime(r.UpdatedAt), updatedW), truncate(summary, summaryW))
			b.WriteString(a.renderCatalogRow(line, selected, width))
		}
		if len(rows) > end {
			b.WriteString("\n" + st.RowMuted.Render(fmt.Sprintf("… %d more", len(rows)-end)))
		}
		b.WriteString("\n" + a.renderCatalogSummary(rows))
	}
	b.WriteString(a.renderFooterHints([]string{
		a.hintChip("↑/k", "up"),
		a.hintChip("↓/j", "down"),
		a.hintChip("/", "search"),
		a.hintChip("S", "sort"),
		a.hintChip("W", "filter"),
		a.hintChip("enter", "open"),
		a.hintChip("d", "details"),
		a.hintChip("esc", "back"),
	}))
	return st.ListBox.Width(a.width).Render(b.String())
}

// pickerMarker returns the left-side row marker: ▸ when selected, two
// spaces otherwise. It keeps the gutter aligned across the catalog,
// updates, profiles and savedvars lists.
func (a *App) pickerMarker(selected bool) string {
	if selected {
		return "▸ "
	}
	return "  "
}

// renderRowLine applies the per-row selection style to a pre-built
// line of text. Kept here so the catalog, updates, profiles and
// savedvars views share one selection treatment.
func (a *App) renderRowLine(line string, selected bool, width int) string {
	padded := padToVisibleWidth(line, width)
	if selected {
		return a.styles.RowSelected.Render(padded) + "\n"
	}
	return a.styles.Row.Render(padded) + "\n"
}

// renderCatalogRow applies the selection style to a plain-text row
// already laid out to the target visible width.
func (a *App) renderCatalogRow(line string, selected bool, width int) string {
	return a.renderRowLine(line, selected, width)
}

// renderCatalogSummary renders the catalog footer summary: result
// count and a small badge of the active sort + filter.
func (a *App) renderCatalogSummary(rows []*catalog.Addon) string {
	st := a.styles
	n := len(rows)
	count := st.SummaryN.Render(fmt.Sprintf("%d", n)) + st.Summary.Render(" result"+plural(n))
	return st.Summary.Render(count)
}

// renderCatalogDetail draws the addon detail panel: metadata plus, for
// github addons, the lazily fetched release notes.
func (a *App) renderCatalogDetail() string {
	addon := a.detailAddon
	if addon == nil {
		return ""
	}
	width := a.width - 8
	if width < 50 {
		width = 50
	}
	contentW := width - 6 // dialog border + padding
	if contentW < 40 {
		contentW = 40
	}
	wrap := lipgloss.NewStyle().Width(contentW)

	var lines []string
	lines = append(lines, a.styles.Section.Render(addon.Name))
	lines = append(lines, "")
	lines = append(lines, a.styles.Detail.Render("Provider: ")+a.styles.Path.Render(addon.Provider))
	if addon.ID != "" {
		lines = append(lines, a.styles.Detail.Render("ID: ")+a.styles.Path.Render(addon.ID))
	}
	if addon.Author != "" {
		lines = append(lines, a.styles.Detail.Render("Author: ")+a.styles.Path.Render(addon.Author))
	}
	if addon.Summary != "" {
		lines = append(lines, "", a.styles.Detail.Render(wrap.Render(addon.Summary)))
	}
	if addon.LatestVersion != "" {
		lines = append(lines, "", a.styles.Detail.Render("Latest version: ")+
			a.styles.Path.Render(addon.LatestVersion))
	}
	if addon.GameVersion != "" {
		lines = append(lines, a.styles.Detail.Render("Game version: ")+
			a.styles.Path.Render(addon.GameVersion))
	}
	if !addon.UpdatedAt.IsZero() {
		lines = append(lines, a.styles.Detail.Render("Updated: ")+
			a.styles.Path.Render(addon.UpdatedAt.Format("2006-01-02 15:04")))
	}
	if addon.Homepage != "" {
		lines = append(lines, "", a.styles.Detail.Render("Homepage: ")+
			a.styles.Path.Render(addon.Homepage))
	}
	if isGitHubRepo(addon) {
		lines = append(lines, "", a.styles.Section.Render("Release notes"))
		if rn, ok := a.releaseNotes[addon.ID]; ok {
			if rn.ok {
				lines = append(lines, a.releaseNotesLines(rn.text, wrap)...)
			} else {
				lines = append(lines, a.styles.RowMuted.Render("Release notes unavailable"))
			}
		} else {
			lines = append(lines, a.styles.RowMuted.Render("Loading release notes…"))
		}
	}

	rows := a.height - 8
	if rows < 5 {
		rows = 5
	}
	if len(lines) > rows {
		start := a.detailOffset
		if start > len(lines)-rows {
			start = len(lines) - rows
		}
		if start < 0 {
			start = 0
		}
		lines = lines[start : start+rows]
	}
	body := strings.Join(lines, "\n")
	body += "\n\n" + a.styles.Hint.Render(
		"enter/esc back · o homepage · g releases · i install · ↑/↓ scroll")
	panel := a.styles.Dialog.Width(width).Render(body)
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, panel)
}

// releaseNotesLines wraps release-note paragraphs to the content width,
// one styled line per wrapped row.
func (a *App) releaseNotesLines(text string, wrap lipgloss.Style) []string {
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		for _, ln := range strings.Split(wrap.Render(para), "\n") {
			out = append(out, a.styles.RowMuted.Render(ln))
		}
		out = append(out, "")
	}
	return out
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
	width := a.contentWidth()
	if width < 60 {
		width = 60
	}
	st := a.styles
	var b strings.Builder
	meta := ""
	if a.updates == nil {
		meta = "checking…"
	} else {
		meta = fmt.Sprintf("%d available  ·  %d up to date", len(a.updates), a.upToDate)
		if total := updateMismatchCount(a.updates); total > 0 {
			meta += fmt.Sprintf("  ·  %d for a different game version", total)
		}
	}
	b.WriteString(a.renderViewHeader("Updates", meta, width))

	if a.updates == nil {
		b.WriteString(a.renderEmptyState("Checking for updates…", a.spinner.View()+" working"))
	} else if len(a.updates) == 0 {
		b.WriteString(a.renderEmptyState("All tracked addons are up to date.", "u re-check · esc back"))
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
			selected := i == a.updatesCur
			latest := "—"
			if u.Latest != nil {
				latest = versionOrDash(u.Latest.LatestVersion)
			}
			folder := u.Entry.Folder
			warn := ""
			if u.Mismatch {
				folder = "⚠ " + folder
				if u.Latest != nil && u.Latest.GameVersion != "" {
					warn = "targets " + u.Latest.GameVersion
				}
			}
			// The folder is the elastic field; everything else on the
			// line is short. Truncate it so long addon names cannot
			// push the row past the panel edge.
			folder = truncate(folder, maxInt(8, width-34))
			tag := st.Badge.Render(" " + providerTag(u.Entry.Provider) + " ")
			line := fmt.Sprintf("%s%s  %s  %s %s %s",
				a.pickerMarker(selected), folder, tag,
				st.RowMuted.Render(versionOrDash(u.Entry.Version)),
				st.RowMuted.Render("→"),
				latest,
			)
			b.WriteString(a.renderRowLine(line, selected, width))
			if warn != "" {
				b.WriteString(st.StatusWarn.Render(pad(truncate("    "+warn, width), width)))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString(a.renderFooterHints([]string{
		a.hintChip("↑/k", "up"),
		a.hintChip("↓/j", "down"),
		a.hintChip("u", "update"),
		a.hintChip("U", "update all"),
		a.hintChip("enter", "inspect"),
		a.hintChip("esc", "back"),
	}))
	return st.ListBox.Width(a.width).Render(b.String())
}

// updateMismatchCount returns how many tracked updates target a
// different game version than the active profile.
func updateMismatchCount(updates []catalog.Update) int {
	n := 0
	for _, u := range updates {
		if u.Mismatch {
			n++
		}
	}
	return n
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
