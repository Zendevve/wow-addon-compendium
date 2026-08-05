// Package ui implements the Bubble Tea terminal interface: an addon
// list with status, problem and suggested fix, plus inspect, logs,
// installation picker, profile picker and confirmation dialogs.
package ui

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/fixer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/profiles"
	"github.com/wowfix/wowfix/internal/scanner"
	"github.com/wowfix/wowfix/internal/utils"
)

// Version is stamped by the build; the UI shows it in the header.
var Version = "dev"

// keyMap holds every key binding.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Escape    key.Binding
	Fix       key.Binding
	FixAll    key.Binding
	Delete    key.Binding
	Rescan    key.Binding
	Backup    key.Binding
	Logs      key.Binding
	Export    key.Binding
	Install   key.Binding
	Profile   key.Binding
	Theme     key.Binding
	Filter    key.Binding
	Help      key.Binding
	Catalog   key.Binding
	Updates   key.Binding
	Source    key.Binding
	Profiles  key.Binding
	SavedVars key.Binding
	Quit      key.Binding
	Yes       key.Binding
	No        key.Binding
	ScrollUp  key.Binding
	ScrollDn  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "inspect")),
		Escape:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Fix:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix")),
		FixAll:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "fix all")),
		Delete:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "trash")),
		Rescan:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Backup:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backup")),
		Logs:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Export:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export logs")),
		Profile:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "profile")),
		Theme:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Catalog:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "catalog")),
		Updates:   key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "updates")),
		Source:    key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install from source")),
		Install:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch install")),
		Profiles:  key.NewBinding(key.WithKeys("o", "O"), key.WithHelp("O", "profiles")),
		SavedVars: key.NewBinding(key.WithKeys("v", "V"), key.WithHelp("V", "savedvars")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Yes:       key.NewBinding(key.WithKeys("y", "enter")),
		No:        key.NewBinding(key.WithKeys("n", "esc")),
		ScrollUp:  key.NewBinding(key.WithKeys("up", "k", "pgup")),
		ScrollDn:  key.NewBinding(key.WithKeys("down", "j", "pgdn")),
	}
}

// view identifies the active screen.
type view int

const (
	viewList view = iota
	viewInspect
	viewLogs
	viewPicker
	viewProfile
	viewConfirm
	viewInput
	viewHelp
	viewCatalog
	viewCatalogAction
	viewCatalogDetail
	viewUpdates
	viewUpdatesDetail
	viewProfiles
	viewSavedVars
)

// inputKind selects what the shared manual input prompt is asking for.
type inputKind int

const (
	inputPath inputKind = iota
	inputSource
	inputProfileCreate
	inputProfileDuplicate
	inputProfileRename
)

// --- messages ---------------------------------------------------------

type scanResultMsg struct {
	result  *models.ScanResult
	install *detector.Installation
	err     error
}

type detectMsg struct {
	installs []detector.Installation
	err      error
}

type fixDoneMsg struct {
	results []fixer.Result
}

type backupDoneMsg struct {
	path string
	err  error
}

type exportDoneMsg struct {
	path string
	err  error
}

type toastMsg struct {
	text string
}

// toastItem is one timestamped notification in the toast stack.
type toastItem struct {
	text string
	at   time.Time
}

const (
	toastLifetime = 6 * time.Second
	maxToasts     = 4
)

type busyDoneMsg struct{}

// App is the root model.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg     *config.Config
	store   *config.Store
	log     *logger.Logger
	keys    keyMap
	theme   Theme
	styles  Styles
	spinner spinner.Model
	input   textinput.Model

	install *detector.Installation
	profile *models.Profile
	scan    *models.ScanResult

	backups *backup.Manager

	// UI state
	view     view
	cursor   int
	offset   int
	width    int
	height   int
	busy     bool
	busyText string
	toasts   []toastItem
	quitting bool

	// help overlay
	helpPrev view

	// filter state (main addon list)
	filtering bool
	filter    textinput.Model
	filterIdx []int // scan.Addons indices matching the filter, ranked

	// catalog services
	catalog  *catalog.Catalog
	registry *catalog.Registry
	catErr   error

	// registry badges: folder (lowercased) -> tracked entry, built per scan
	registryByFolder map[string]catalog.Entry

	// catalog browser
	search    textinput.Model
	results   []*catalog.Addon
	resultCur int

	// catalog sort/filter toggles (S and W in the catalog view).
	// catalogSort cycles name -> updated -> provider; catalogFilter
	// cycles the game-version family, "" meaning all.
	catalogSort   string
	catalogFilter string

	// catalog detail view; releaseNotes caches GitHub release notes per
	// addon ID so the rate-limited API is never re-polled.
	detailAddon  *catalog.Addon
	detailOffset int
	releaseNotes map[string]releaseNotesResult

	// catalog search debounce/rate limiting. searchCancel aborts the
	// pending debounce timer; searchPending/searching drive the
	// "searching…" hint; lastSearchAt enforces the per-session minimum
	// interval between provider calls.
	searchCancel  func()
	searchPending bool
	searching     bool
	lastSearchAt  time.Time

	// catalog action dialog (install / open homepage)
	actionAddon    *catalog.Addon
	actionChoices  []string
	actionCursor   int
	actionCallback func(choice string)

	// updates view
	updates    []catalog.Update
	updatesCur int
	updatesOff int
	upToDate   int

	// install/update progress; the counters are written by the download
	// callback on the command goroutine and read on spinner ticks.
	installRunning  bool
	installProgress *Progress
	progressDone    atomic.Int64
	progressTotal   atomic.Int64

	// recheckAfterScan re-runs the update check once the next scan lands
	// (used after installs/updates that change the registry).
	recheckAfterScan bool

	// shared manual input mode (path vs install source)
	inputMode inputKind

	// inspect state
	inspectAddon  *models.Addon
	inspectSize   int64
	inspectOffset int

	// cmd is a deferred tea.Cmd produced by a confirm callback.
	cmd tea.Cmd

	// logs state
	logsOffset int

	// picker state
	picker    []detector.Installation
	pickerCur int

	// confirm state
	confirmTitle string
	confirmMsg   string
	confirmYes   func()
	confirmNo    func()

	// toc choice state
	tocAddon    *models.Addon
	tocChoices  []string
	tocCursor   int
	tocCallback func(choice string)

	// addon-collections view state
	profiles       []profiles.Collection
	profilesCursor int
	profilesOffset int
	inputProfileID string

	// SavedVariables view state
	svAccounts []string
	svAccount  string
	svFiles    []string
	svCursor   int
	svOffset   int
}

// NewApp wires the model with services.
func NewApp(cfg *config.Config, store *config.Store, log *logger.Logger) *App {
	ctx, cancel := context.WithCancel(context.Background())
	theme := ThemeByName(cfg.Theme)
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(theme.accent)
	sp.Spinner = spinner.Dot

	input := textinput.New()
	input.Placeholder = "C:\\Games\\World of Warcraft or /opt/wow"
	input.CharLimit = 500

	filter := textinput.New()
	filter.Placeholder = "filter addons…"
	filter.Prompt = "/"
	filter.CharLimit = 200

	search := textinput.New()
	search.Placeholder = "search addons across providers…"
	search.CharLimit = 200

	// Catalog services: all providers enabled, default HTTP client. The
	// registry lives under the config dir and tracks catalog installs so
	// the main list can show provider badges and the updater can run.
	cat, catErr := catalog.New(nil, http.DefaultClient)
	reg, regErr := catalog.NewRegistry(filepath.Join(store.Dir(), "registry.json"))
	if catErr == nil && regErr == nil {
		cat.Reg = reg
		cat.Log = log
		cat.Profile = models.ProfileByID(cfg.Profile)
	} else {
		cat = nil
		if catErr == nil {
			catErr = regErr
		}
	}
	if catErr != nil {
		log.Errorf("catalog unavailable: %v", catErr)
	}

	return &App{
		ctx:          ctx,
		cancel:       cancel,
		cfg:          cfg,
		store:        store,
		log:          log,
		keys:         defaultKeys(),
		theme:        theme,
		styles:       NewStyles(theme),
		spinner:      sp,
		input:        input,
		filter:       filter,
		search:       search,
		catalog:      cat,
		registry:     reg,
		catErr:       catErr,
		profile:      models.ProfileByID(cfg.Profile),
		catalogSort:  catalogSortName,
		releaseNotes: map[string]releaseNotesResult{},
	}
}

// Init starts scanning or the installation picker.
func (a *App) Init() tea.Cmd {
	if a.profile == nil {
		a.profile = models.DefaultProfile()
	}
	if a.cfg.WoWPath != "" {
		return tea.Batch(a.scanCmd(a.cfg.WoWPath, a.cfg.Flavor), a.spinner.Tick)
	}
	return tea.Batch(a.detectCmd(), a.spinner.Tick)
}

// --- commands ---------------------------------------------------------

func (a *App) scanCmd(root, flavor string) tea.Cmd {
	return func() tea.Msg {
		install, err := detector.DetectPath(root)
		if err != nil {
			return scanResultMsg{err: err}
		}
		res, err := scanner.New(install.AddonsPath, a.profile).Scan(a.ctx)
		if err != nil {
			return scanResultMsg{install: install, err: err}
		}
		return scanResultMsg{result: res, install: install}
	}
}

func (a *App) detectCmd() tea.Cmd {
	return func() tea.Msg {
		found, err := detector.AutoDetect(a.ctx)
		return detectMsg{installs: found, err: err}
	}
}

func (a *App) fixCmd(addons []*models.Addon) tea.Cmd {
	return func() tea.Msg {
		f := a.newFixer(true)
		return fixDoneMsg{results: f.FixAll(a.ctx, addons)}
	}
}

func (a *App) backupCmd() tea.Cmd {
	return func() tea.Msg {
		root, err := a.backupRoot()
		if err != nil {
			return backupDoneMsg{err: err}
		}
		m := backup.New(root, a.log)
		id, err := m.BackupDir(a.install.AddonsPath, "manual backup")
		if err != nil {
			return backupDoneMsg{err: err}
		}
		return backupDoneMsg{path: id}
	}
}

func (a *App) exportCmd() tea.Cmd {
	dir := filepath.Join(a.store.Dir(), "logs")
	return func() tea.Msg {
		path, err := a.log.Export(dir)
		return exportDoneMsg{path: path, err: err}
	}
}

// --- helpers ----------------------------------------------------------

// newFixer builds a fixer bound to the current installation.
// preApproved skips per-action confirmation prompts.
func (a *App) newFixer(preApproved bool) *fixer.Fixer {
	var confirm func(string, ...any) bool
	if preApproved {
		confirm = func(string, ...any) bool { return true }
	}
	opts := fixer.Options{
		AddonsDir:        a.install.AddonsPath,
		Profile:          a.profile,
		Log:              a.log,
		Confirm:          confirm,
		TrashFallbackDir: filepath.Join(a.store.Dir(), "trash"),
	}
	if a.cfg.AutoBackup {
		root, err := a.backupRoot()
		if err != nil {
			root = filepath.Join(a.store.Dir(), "backups")
		}
		opts.Backups = backup.New(root, a.log)
	}
	return fixer.New(opts)
}

// backupRoot resolves the Backups directory: config override, else the
// game root (spec: Backups/<timestamp>), else the config dir.
func (a *App) backupRoot() (string, error) {
	if a.cfg.BackupsDir != "" {
		return a.cfg.BackupsDir, nil
	}
	if a.install != nil && a.install.Root != "" {
		root := filepath.Join(a.install.Root, "Backups")
		if err := utils.IsWritable(root); err == nil {
			return root, nil
		}
		if utils.EnsureDir(root) == nil && utils.IsWritable(root) == nil {
			return root, nil
		}
	}
	fallback := filepath.Join(a.store.Dir(), "backups")
	if err := utils.EnsureDir(fallback); err != nil {
		return "", err
	}
	return fallback, nil
}

// save persists the config.
func (a *App) save() {
	_ = a.store.Save(a.cfg)
}

func (a *App) toastMsg(text string) tea.Cmd {
	return func() tea.Msg { return toastMsg{text: text} }
}

func (a *App) pushToast(text string) {
	a.toasts = append(a.toasts, toastItem{text: text, at: time.Now()})
	if len(a.toasts) > maxToasts {
		a.toasts = a.toasts[len(a.toasts)-maxToasts:]
	}
}

// listLen returns the number of visible rows in the main list, honoring
// the active filter.
func (a *App) listLen() int {
	if a.scan == nil {
		return 0
	}
	if a.filtering {
		return len(a.filterIdx)
	}
	return len(a.scan.Addons)
}

// rowToIndex maps a visible list row to an index into scan.Addons, or -1
// when the row is out of range.
func (a *App) rowToIndex(row int) int {
	if a.filtering {
		if row < 0 || row >= len(a.filterIdx) {
			return -1
		}
		return a.filterIdx[row]
	}
	if row < 0 || row >= len(a.scan.Addons) {
		return -1
	}
	return row
}

// applyFilter recomputes filterIdx from the filter input, ranking matches
// by fuzzy score (stable: ties keep scan order).
func (a *App) applyFilter() {
	a.filterIdx = a.filterIdx[:0]
	if a.scan == nil {
		a.clampCursor()
		return
	}
	q := a.filter.Value()
	if q == "" {
		for i := range a.scan.Addons {
			a.filterIdx = append(a.filterIdx, i)
		}
	} else {
		type scored struct {
			idx, score int
		}
		ranked := make([]scored, 0, len(a.scan.Addons))
		for i, ad := range a.scan.Addons {
			if s := FuzzyScore(q, ad.FolderName); s > 0 {
				ranked = append(ranked, scored{i, s})
			}
		}
		sort.SliceStable(ranked, func(x, y int) bool {
			return ranked[x].score > ranked[y].score
		})
		for _, r := range ranked {
			a.filterIdx = append(a.filterIdx, r.idx)
		}
	}
	a.clampCursor()
}

// reloadRegistryBadges rebuilds the folder -> registry entry map used
// for the SOURCE column of the main list.
func (a *App) reloadRegistryBadges() {
	a.registryByFolder = nil
	if a.registry == nil {
		return
	}
	a.registryByFolder = make(map[string]catalog.Entry)
	for _, e := range a.registry.Entries() {
		a.registryByFolder[strings.ToLower(e.Folder)] = e
	}
}

// closeFilter exits filter mode and restores the unfiltered list.
func (a *App) closeFilter() {
	a.filtering = false
	a.filter.Blur()
	a.filter.SetValue("")
	a.filterIdx = a.filterIdx[:0]
	a.clampCursor()
}

// addonAt returns the addon under the cursor, or nil.
func (a *App) addonAt() *models.Addon {
	if a.scan == nil {
		return nil
	}
	idx := a.rowToIndex(a.cursor)
	if idx < 0 {
		return nil
	}
	return a.scan.Addons[idx]
}

func (a *App) fixableAddons() []*models.Addon {
	if a.scan == nil {
		return nil
	}
	var out []*models.Addon
	for _, ad := range a.scan.Addons {
		if ad.Fixable() {
			out = append(out, ad)
		}
	}
	return out
}

// visibleRows computes how many list rows fit in the current height.
// The filter bar takes a row while it is open.
func (a *App) visibleRows() int {
	rows := a.height - 12
	if rows < 3 {
		rows = 3
	}
	if a.filtering {
		rows--
		if rows < 2 {
			rows = 2
		}
	}
	return rows
}

// clampCursor keeps the cursor inside the (possibly filtered) list and
// scrolls the offset.
func (a *App) clampCursor() {
	if a.scan == nil {
		a.cursor, a.offset = 0, 0
		return
	}
	n := a.listLen()
	if n == 0 {
		a.cursor, a.offset = 0, 0
		return
	}
	if a.cursor < 0 {
		a.cursor = 0
	}
	if a.cursor >= n {
		a.cursor = n - 1
	}
	rows := a.visibleRows()
	if a.cursor < a.offset {
		a.offset = a.cursor
	}
	if a.cursor >= a.offset+rows {
		a.offset = a.cursor - rows + 1
	}
}

// --- update -----------------------------------------------------------

// Update handles all messages and key input.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.clampCursor()
		return a, nil

	case tea.KeyMsg:
		return a.updateKey(m)

	case tea.MouseMsg:
		return a.updateMouse(m)

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		if a.installRunning && a.installProgress != nil {
			done := a.progressDone.Load()
			total := a.progressTotal.Load()
			if total > 0 {
				a.installProgress.Indeterminate = false
				a.installProgress.Percent = float64(done) / float64(total)
			} else {
				a.installProgress.Indeterminate = true
			}
			a.installProgress.Frame++
		}
		return a, cmd

	case scanResultMsg:
		a.busy = false
		if m.err != nil {
			a.pushToast("Scan failed: " + m.err.Error())
			a.log.Errorf("Scan failed: %v", m.err)
			a.view = viewPicker
			a.picker = nil
			return a, a.detectCmd()
		}
		a.install = m.install
		a.scan = m.result
		a.cfg.WoWPath = m.install.Root
		a.cfg.Flavor = m.install.Flavor
		a.cfg.LastScan = m.result.ScannedAt
		a.save()
		a.reloadRegistryBadges()
		if a.catalog != nil {
			a.catalog.Profile = a.profile
		}
		total, problems, errors := m.result.Stats()
		a.pushToast(fmt.Sprintf("Scanned %d addon(s) — %d with issues, %d errors", total, problems, errors))
		a.log.Infof("Scan complete: %d addons, %d problems, %d errors", total, problems, errors)
		a.view = viewList
		a.closeFilter()
		a.clampCursor()
		if a.recheckAfterScan {
			a.recheckAfterScan = false
			a.busy = true
			a.busyText = "Checking for updates…"
			return a, a.checkUpdatesCmd()
		}
		return a, nil

	case detectMsg:
		a.picker = m.installs
		a.view = viewPicker
		a.busy = false
		if len(m.installs) == 0 {
			a.pushToast("No WoW installation auto-detected — enter a path or pick one")
		}
		return a, nil

	case fixDoneMsg:
		a.busy = false
		ok, skipped, failed := 0, 0, 0
		for _, r := range m.results {
			switch {
			case r.Err != nil:
				failed++
			case r.OK:
				ok++
			default:
				skipped++
			}
		}
		a.pushToast(fmt.Sprintf("Fix run: %d applied, %d skipped, %d failed", ok, skipped, failed))
		if a.install != nil {
			return a, a.scanCmd(a.install.Root, a.install.Flavor)
		}
		return a, nil

	case backupDoneMsg:
		a.busy = false
		if m.err != nil {
			a.pushToast("Backup failed: " + m.err.Error())
		} else {
			a.pushToast("Backup created: " + m.path)
		}
		return a, nil

	case exportDoneMsg:
		if m.err != nil {
			a.pushToast("Export failed: " + m.err.Error())
		} else {
			a.pushToast("Logs exported to " + m.path)
		}
		return a, nil

	case toastMsg:
		a.pushToast(m.text)
		return a, nil

	case catalogSearchMsg:
		// Drop stale replies: the query has moved on.
		if m.query != a.search.Value() {
			return a, nil
		}
		a.results = m.results
		a.resultCur = 0
		a.searching = false
		a.searchPending = false
		if m.err != nil && len(m.results) == 0 {
			a.pushToast("Search failed: " + m.err.Error())
		}
		return a, nil

	case searchDebounceMsg:
		// The quiet period elapsed: run the search (rate limit applied).
		return a, a.fireSearch()

	case releaseNotesMsg:
		a.releaseNotes[m.id] = releaseNotesResult{text: m.text, ok: m.err == nil && m.text != ""}
		return a, nil

	case installDoneMsg:
		a.busy = false
		a.installRunning = false
		a.installProgress = nil
		if m.err != nil {
			a.pushToast("Install failed: " + m.err.Error())
			a.log.Errorf("Install failed: %v", m.err)
			return a, nil
		}
		a.pushToast("Installed " + strings.Join(m.names, ", "))
		a.log.Infof("Installed %s", strings.Join(m.names, ", "))
		if a.install != nil {
			return a, a.scanCmd(a.install.Root, a.install.Flavor)
		}
		return a, nil

	case updatesMsg:
		a.busy = false
		if m.updates == nil {
			m.updates = []catalog.Update{}
		}
		a.updates = m.updates
		a.upToDate = maxInt(0, m.checked-len(m.updates))
		a.view = viewUpdates
		a.updatesCur, a.updatesOff = 0, 0
		if m.err != nil {
			a.pushToast("Update check: " + m.err.Error())
			a.log.Warn("Update check: " + m.err.Error())
		}
		return a, nil

	case updateDoneMsg:
		a.busy = false
		if m.err != nil {
			a.pushToast("Update failed: " + m.err.Error())
			a.log.Errorf("Update failed: %v", m.err)
			return a, nil
		}
		a.pushToast("Updated " + m.folder)
		a.log.Infof("Updated %s", m.folder)
		return a, a.refreshAfterUpdate()

	case updateAllDoneMsg:
		a.busy = false
		if m.err != nil {
			a.pushToast(fmt.Sprintf("Update all: %d applied, %d failed (%v)", m.applied, m.failed, m.err))
			a.log.Errorf("Update all: %d applied, %d failed: %v", m.applied, m.failed, m.err)
		} else {
			a.pushToast(fmt.Sprintf("Update all: %d applied, %d failed", m.applied, m.failed))
			a.log.Infof("Update all: %d applied, %d failed", m.applied, m.failed)
		}
		return a, a.refreshAfterUpdate()

	case busyDoneMsg:
		a.busy = false
		return a, nil

	case svBackupMsg:
		a.busy = false
		if m.err != nil {
			a.pushToast("SavedVariables backup failed: " + m.err.Error())
		} else {
			a.pushToast("SavedVariables backed up to " + m.path)
		}
		return a, nil
	}

	return a, tea.Batch(cmds...)
}

// inputFocused reports whether a text input is currently consuming typed
// keys: the manual path/source prompt, the list filter, or a focused
// catalog search box.
func (a *App) inputFocused() bool {
	return a.view == viewInput || a.filtering ||
		(a.view == viewCatalog && a.search.Focused())
}

// updateKey dispatches keyboard input by view. Quit works from every
// view except the manual path input and the open filter (where q is a
// normal character) and the help overlay (where q closes it).
func (a *App) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.view == viewHelp {
		return a.updateHelpKey(msg)
	}
	if !a.inputFocused() && key.Matches(msg, a.keys.Quit) && !a.busy {
		a.quitting = true
		return a, tea.Quit
	}
	if a.view != viewConfirm && !a.inputFocused() && key.Matches(msg, a.keys.Help) {
		a.helpPrev = a.view
		a.view = viewHelp
		return a, nil
	}
	switch a.view {
	case viewConfirm:
		return a.updateConfirmKey(msg)
	case viewInput:
		return a.updateInputKey(msg)
	case viewLogs:
		return a.updateLogsKey(msg)
	case viewPicker:
		return a.updatePickerKey(msg)
	case viewProfile:
		return a.updateProfileKey(msg)
	case viewInspect:
		return a.updateInspectKey(msg)
	case viewCatalog:
		return a.updateCatalogKey(msg)
	case viewCatalogAction:
		return a.updateCatalogActionKey(msg)
	case viewCatalogDetail:
		return a.updateCatalogDetailKey(msg)
	case viewUpdates:
		return a.updateUpdatesKey(msg)
	case viewUpdatesDetail:
		return a.updateUpdatesDetailKey(msg)
	case viewProfiles:
		return a.updateProfilesKey(msg)
	case viewSavedVars:
		return a.updateSavedVarsKey(msg)
	default:
		return a.updateListKey(msg)
	}
}

// --- list view keys ---------------------------------------------------

func (a *App) updateListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.filtering {
		return a.updateFilterKey(msg)
	}
	switch {
	case key.Matches(msg, a.keys.Quit):
		if a.busy {
			return a, nil
		}
		a.quitting = true
		return a, tea.Quit

	case key.Matches(msg, a.keys.Up):
		if a.scan != nil && a.listLen() > 0 {
			a.cursor--
			a.clampCursor()
		}

	case key.Matches(msg, a.keys.Down):
		if a.scan != nil && a.listLen() > 0 {
			a.cursor++
			a.clampCursor()
		}

	case key.Matches(msg, a.keys.Enter):
		if a.scan == nil || a.listLen() == 0 {
			return a, nil
		}
		a.view = viewInspect
		a.inspectAddon = a.addonAt()
		a.inspectSize = 0
		if size, err := utils.DirSize(a.inspectAddon.Path); err == nil {
			a.inspectSize = size
		}
		return a, nil

	case key.Matches(msg, a.keys.Filter):
		if a.busy {
			return a, nil
		}
		a.filtering = true
		a.filter.SetValue("")
		a.filter.Focus()
		a.applyFilter()
		return a, textinput.Blink

	case key.Matches(msg, a.keys.Fix):
		addon := a.addonAt()
		if addon == nil || !addon.Fixable() {
			return a, nil
		}
		a.openConfirm(fmt.Sprintf("Fix %s?", addon.FolderName),
			a.describeFixes(addon),
			func() {
				a.busy = true
				a.busyText = fmt.Sprintf("Fixing %s…", addon.FolderName)
				a.cmd = a.fixCmd([]*models.Addon{addon})
			})
		return a, nil

	case key.Matches(msg, a.keys.FixAll):
		addons := a.fixableAddons()
		if len(addons) == 0 {
			return a, nil
		}
		count := 0
		for _, ad := range addons {
			count += len(ad.Issues)
		}
		a.openConfirm(fmt.Sprintf("Fix all (%d issue(s) on %d addon(s))?", count, len(addons)),
			"Every change is backed up first. Damaged folders are sent to the trash (recoverable).",
			func() {
				a.busy = true
				a.busyText = "Fixing all addons…"
				a.cmd = a.fixCmd(addons)
			})
		return a, nil

	case key.Matches(msg, a.keys.Delete):
		addon := a.addonAt()
		if addon == nil {
			return a, nil
		}
		a.openConfirm(fmt.Sprintf("Move %q to trash?", addon.SourceDir),
			"A backup is created first. Nothing is deleted permanently.",
			func() {
				a.busy = true
				a.busyText = fmt.Sprintf("Removing %s…", addon.FolderName)
				f := a.newFixer(true)
				a.cmd = func() tea.Msg {
					res := f.Fix(a.ctx, addon)
					return fixDoneMsg{results: res}
				}
			})
		return a, nil

	case key.Matches(msg, a.keys.Rescan):
		if a.install != nil {
			a.busy = true
			a.busyText = "Scanning…"
			return a, a.scanCmd(a.install.Root, a.install.Flavor)
		}
		a.busy = true
		a.busyText = "Detecting installations…"
		return a, a.detectCmd()

	case key.Matches(msg, a.keys.Backup):
		if a.install == nil {
			return a, nil
		}
		a.busy = true
		a.busyText = "Creating backup…"
		return a, a.backupCmd()

	case key.Matches(msg, a.keys.Logs):
		a.view = viewLogs
		a.logsOffset = 0
		return a, nil

	case key.Matches(msg, a.keys.Export):
		return a, a.exportCmd()

	case key.Matches(msg, a.keys.Profile):
		a.view = viewProfile
		return a, nil

	case key.Matches(msg, a.keys.Theme):
		if a.theme.Name == "dark" {
			a.theme = Light()
			a.cfg.Theme = "light"
		} else {
			a.theme = Dark()
			a.cfg.Theme = "dark"
		}
		a.styles = NewStyles(a.theme)
		a.spinner.Style = lipgloss.NewStyle().Foreground(a.theme.accent)
		a.save()
		return a, nil

	case key.Matches(msg, a.keys.Catalog):
		if a.catalog == nil {
			a.pushToast("Catalog unavailable: " + a.catErr.Error())
			return a, nil
		}
		a.cancelSearchDebounce()
		a.view = viewCatalog
		a.search.Blur()
		a.search.SetValue("")
		a.results = nil
		a.resultCur = 0
		return a, nil

	case key.Matches(msg, a.keys.Updates):
		if a.catalog == nil || a.registry == nil {
			a.pushToast("Catalog unavailable: " + a.catErr.Error())
			return a, nil
		}
		if a.install == nil {
			a.pushToast("No installation selected")
			return a, nil
		}
		a.busy = true
		a.busyText = "Checking for updates…"
		return a, a.checkUpdatesCmd()

	case key.Matches(msg, a.keys.Source):
		a.view = viewInput
		a.inputMode = inputSource
		a.input.Reset()
		a.input.Placeholder = "owner/repo or addon URL (github, curseforge, wowinterface, tukui)"
		a.input.Focus()
		return a, textinput.Blink

	case key.Matches(msg, a.keys.Install):
		a.view = viewInput
		a.inputMode = inputPath
		a.input.Reset()
		a.input.Placeholder = "Path to WoW installation (e.g. C:\\Games\\World of Warcraft)"
		a.input.Focus()
		return a, textinput.Blink

	case key.Matches(msg, a.keys.Profiles):
		if a.install == nil {
			a.pushToast("No installation selected")
			return a, nil
		}
		a.loadProfiles()
		a.profilesCursor, a.profilesOffset = 0, 0
		a.view = viewProfiles
		return a, nil

	case key.Matches(msg, a.keys.SavedVars):
		if a.install == nil {
			a.pushToast("No installation selected")
			return a, nil
		}
		a.loadSavedVars()
		a.svCursor, a.svOffset = 0, 0
		a.view = viewSavedVars
		return a, nil
	}

	return a, nil
}

// --- filter bar -------------------------------------------------------

// updateFilterKey handles input while the list filter is open. Every key
// types into the filter except esc/enter (close) and ctrl+c (quit).
func (a *App) updateFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
		a.quitting = true
		return a, tea.Quit

	case key.Matches(msg, a.keys.Escape):
		a.closeFilter()
		return a, nil

	case key.Matches(msg, a.keys.Enter):
		addon := a.addonAt()
		a.closeFilter()
		if addon == nil {
			return a, nil
		}
		a.view = viewInspect
		a.inspectAddon = addon
		a.inspectSize = 0
		if size, err := utils.DirSize(addon.Path); err == nil {
			a.inspectSize = size
		}
		return a, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))) && a.filter.Value() == "":
		a.closeFilter()
		return a, nil

	default:
		var cmd tea.Cmd
		a.filter, cmd = a.filter.Update(msg)
		a.applyFilter()
		return a, cmd
	}
}

// --- help overlay -----------------------------------------------------

func (a *App) updateHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape),
		key.Matches(msg, key.NewBinding(key.WithKeys("q"))),
		key.Matches(msg, a.keys.Help):
		a.view = a.helpPrev
		if a.view == viewHelp {
			a.view = viewList
		}
	case key.Matches(msg, a.keys.Quit): // ctrl+c
		a.quitting = true
		return a, tea.Quit
	}
	return a, nil
}

// --- mouse ------------------------------------------------------------

// updateMouse advances the list cursor or scrolls the current view on
// wheel events. Keyboard paths are unchanged.
func (a *App) updateMouse(m tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.Button {
	case tea.MouseButtonWheelUp:
		switch a.view {
		case viewList:
			if a.scan != nil && a.listLen() > 0 {
				a.cursor--
				a.clampCursor()
			}
		case viewInspect:
			a.inspectOffset = maxInt(0, a.inspectOffset-3)
		case viewLogs:
			a.logsOffset = maxInt(0, a.logsOffset-1)
		}
	case tea.MouseButtonWheelDown:
		switch a.view {
		case viewList:
			if a.scan != nil && a.listLen() > 0 {
				a.cursor++
				a.clampCursor()
			}
		case viewInspect:
			a.inspectOffset += 3
		case viewLogs:
			entries := a.log.Entries()
			rows := a.visibleRows()
			a.logsOffset = minInt(maxInt(0, len(entries)-rows), a.logsOffset+1)
		}
	}
	return a, nil
}

// describeFixes summarizes what fixing an addon will do.
func (a *App) describeFixes(addon *models.Addon) string {
	var parts []string
	for _, issue := range addon.Issues {
		if issue.Action != models.ActionNone {
			parts = append(parts, issue.Action.Label()+" → "+issue.Suggestion)
		}
	}
	if len(parts) == 0 {
		return "No automatic fixes available."
	}
	return strings.Join(parts, "\n")
}

// --- confirm dialog ---------------------------------------------------

func (a *App) openConfirm(title, message string, yes func()) {
	a.view = viewConfirm
	a.confirmTitle = title
	a.confirmMsg = message
	a.confirmYes = yes
	a.confirmNo = func() { a.view = viewList }
}

func (a *App) updateConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Yes):
		yes := a.confirmYes
		a.view = viewList
		if yes != nil {
			yes()
			if a.cmd != nil {
				cmd := a.cmd
				a.cmd = nil
				return a, cmd
			}
		}
	case key.Matches(msg, a.keys.No):
		no := a.confirmNo
		if no != nil {
			no()
			if a.cmd != nil {
				cmd := a.cmd
				a.cmd = nil
				return a, cmd
			}
		} else {
			a.view = viewList
		}
	}
	return a, nil
}

// --- inspect view -----------------------------------------------------

func (a *App) updateInspectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape), key.Matches(msg, a.keys.Enter):
		a.view = viewList
		return a, nil

	case key.Matches(msg, a.keys.Fix):
		addon := a.inspectAddon
		if addon == nil || !addon.Fixable() {
			return a, nil
		}
		a.view = viewList
		a.busy = true
		a.busyText = fmt.Sprintf("Fixing %s…", addon.FolderName)
		return a, a.fixCmd([]*models.Addon{addon})

	case key.Matches(msg, a.keys.Delete):
		addon := a.inspectAddon
		if addon == nil {
			return a, nil
		}
		a.view = viewList
		a.busy = true
		a.busyText = fmt.Sprintf("Removing %s…", addon.FolderName)
		f := a.newFixer(true)
		return a, func() tea.Msg {
			return fixDoneMsg{results: f.Fix(a.ctx, addon)}
		}

	case key.Matches(msg, a.keys.ScrollUp):
		a.inspectOffset = maxInt(0, a.inspectOffset-3)
	case key.Matches(msg, a.keys.ScrollDn):
		a.inspectOffset += 3
	}
	return a, nil
}

// --- logs view --------------------------------------------------------

func (a *App) updateLogsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := a.log.Entries()
	rows := a.visibleRows()
	maxOffset := maxInt(0, len(entries)-rows)
	switch {
	case key.Matches(msg, a.keys.Escape), key.Matches(msg, a.keys.Enter), key.Matches(msg, a.keys.Logs):
		a.view = viewList
		return a, nil
	case key.Matches(msg, a.keys.ScrollUp):
		a.logsOffset = maxInt(0, a.logsOffset-1)
	case key.Matches(msg, a.keys.ScrollDn):
		a.logsOffset = minInt(maxOffset, a.logsOffset+1)
	case key.Matches(msg, a.keys.Export):
		return a, a.exportCmd()
	}
	return a, nil
}

// --- picker -----------------------------------------------------------

func (a *App) updatePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Up):
		if len(a.picker) > 0 {
			a.pickerCur = maxInt(0, a.pickerCur-1)
		}
	case key.Matches(msg, a.keys.Down):
		if len(a.picker) > 0 {
			a.pickerCur = minInt(len(a.picker)-1, a.pickerCur+1)
		}
	case key.Matches(msg, a.keys.Enter):
		if len(a.picker) > 0 {
			inst := a.picker[a.pickerCur]
			a.cfg.WoWPath = inst.Root
			a.cfg.Flavor = inst.Flavor
			a.save()
			if inst.ProfileID != "" {
				a.profile = models.ProfileByID(inst.ProfileID)
				if a.profile != nil {
					a.cfg.Profile = a.profile.ID
					a.save()
				}
			}
			a.busy = true
			a.busyText = "Scanning…"
			return a, a.scanCmd(inst.Root, inst.Flavor)
		}
	case key.Matches(msg, a.keys.Escape):
		if a.install != nil {
			a.view = viewList
		} else {
			a.quitting = true
			return a, tea.Quit
		}
	case key.Matches(msg, a.keys.Install):
		a.view = viewInput
		a.inputMode = inputPath
		a.input.Reset()
		a.input.Placeholder = "Path to WoW installation (e.g. C:\\Games\\World of Warcraft)"
		a.input.Focus()
		return a, textinput.Blink
	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit
	}
	return a, nil
}

// --- profile picker ---------------------------------------------------

func (a *App) updateProfileKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	profiles := models.Profiles
	switch {
	case key.Matches(msg, a.keys.Up):
		a.pickerCur = maxInt(0, a.pickerCur-1)
	case key.Matches(msg, a.keys.Down):
		a.pickerCur = minInt(len(profiles)-1, a.pickerCur+1)
	case key.Matches(msg, a.keys.Enter):
		p := profiles[a.pickerCur]
		a.profile = &p
		a.cfg.Profile = p.ID
		a.save()
		if a.catalog != nil {
			a.catalog.Profile = a.profile
		}
		a.view = viewList
		a.pushToast(fmt.Sprintf("Profile set to %s (interface %d)", p.Name, p.Interface))
		if a.install != nil {
			a.busy = true
			a.busyText = "Re-validating…"
			return a, a.scanCmd(a.install.Root, a.install.Flavor)
		}
	case key.Matches(msg, a.keys.Escape):
		a.view = viewList
	}
	return a, nil
}

// --- manual path input ------------------------------------------------

func (a *App) updateInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.input.Blur()
		switch a.inputMode {
		case inputSource:
			a.view = viewList
		case inputProfileCreate, inputProfileDuplicate, inputProfileRename:
			a.view = viewProfiles
		default:
			a.view = viewPicker
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		val := strings.TrimSpace(a.input.Value())
		a.input.Blur()
		switch a.inputMode {
		case inputSource:
			a.view = viewList
			if val == "" {
				return a, nil
			}
			return a, a.installFromSourceCmd(val)
		case inputProfileCreate, inputProfileDuplicate, inputProfileRename:
			a.view = viewProfiles
			if val == "" {
				return a, nil
			}
			if err := a.applyProfileInput(val); err != nil {
				a.pushToast("Collection: " + err.Error())
			} else {
				a.loadProfiles()
			}
			return a, nil
		}
		if val == "" {
			a.view = viewPicker
			return a, nil
		}
		a.cfg.WoWPath = val
		a.cfg.Flavor = ""
		a.save()
		a.view = viewList
		a.busy = true
		a.busyText = "Scanning…"
		return a, a.scanCmd(val, "")
	default:
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}
}

// View renders the active screen.
func (a *App) View() string {
	body := ""
	switch a.view {
	case viewInspect:
		body = a.renderInspect()
	case viewLogs:
		body = a.renderLogs()
	case viewPicker:
		body = a.renderPicker()
	case viewProfile:
		body = a.renderProfile()
	case viewConfirm:
		body = a.renderConfirm()
	case viewInput:
		body = a.renderInput()
	case viewHelp:
		body = a.renderHelp()
	case viewCatalog:
		body = a.renderCatalog()
	case viewCatalogAction:
		body = a.renderCatalogAction()
	case viewCatalogDetail:
		body = a.renderCatalogDetail()
	case viewUpdates:
		body = a.renderUpdates()
	case viewUpdatesDetail:
		body = a.renderUpdatesDetail()
	case viewProfiles:
		body = a.renderProfiles()
	case viewSavedVars:
		body = a.renderSavedVars()
	default:
		body = a.renderList()
	}
	return a.renderFrame(body)
}

// renderFrame wraps body in header/footer chrome and the toast stack.
func (a *App) renderFrame(body string) string {
	var b strings.Builder
	b.WriteString(a.renderHeader())
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString(a.renderHints())

	if a.installRunning && a.installProgress != nil {
		b.WriteString("\n" + a.installProgress.View())
	}

	now := time.Now()
	live := a.toasts[:0]
	for _, t := range a.toasts {
		if now.Sub(t.at) < toastLifetime {
			live = append(live, t)
		}
	}
	a.toasts = live
	for _, t := range live {
		b.WriteString("\n" + a.styles.Toast.Render(t.at.Format("15:04:05")+"  "+t.text))
	}
	return a.styles.App.Render(b.String())
}

// renderHints shows a per-view key hint bar at the bottom of the frame.
func (a *App) renderHints() string {
	var bindings []key.Binding
	switch a.view {
	case viewList:
		bindings = []key.Binding{
			a.keys.Up, a.keys.Down, a.keys.Enter,
			a.keys.Fix, a.keys.Filter,
			a.keys.Profiles, a.keys.SavedVars,
			a.keys.Catalog, a.keys.Updates, a.keys.Help, a.keys.Quit,
		}
	case viewInspect:
		bindings = []key.Binding{a.keys.Up, a.keys.Down, a.keys.Fix, a.keys.Delete, a.keys.Escape}
	case viewLogs:
		bindings = []key.Binding{a.keys.ScrollUp, a.keys.ScrollDn, a.keys.Export, a.keys.Escape}
	case viewPicker:
		bindings = []key.Binding{a.keys.Up, a.keys.Down, a.keys.Enter, a.keys.Install, a.keys.Escape}
	case viewProfile:
		bindings = []key.Binding{a.keys.Up, a.keys.Down, a.keys.Enter, a.keys.Escape}
	case viewCatalog:
		bindings = []key.Binding{
			a.keys.Up, a.keys.Down,
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
			key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "sort")),
			key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "filter")),
			a.keys.Enter,
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "details")),
			a.keys.Escape,
		}
	case viewCatalogAction:
		bindings = []key.Binding{a.keys.Up, a.keys.Down, a.keys.Enter, a.keys.Escape}
	case viewCatalogDetail:
		bindings = []key.Binding{
			a.keys.Up, a.keys.Down,
			key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "homepage")),
			key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "releases")),
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install")),
			a.keys.Escape,
		}
	case viewUpdates:
		bindings = []key.Binding{a.keys.Up, a.keys.Down, a.keys.Updates, key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "update all")), a.keys.Enter, a.keys.Escape}
	case viewUpdatesDetail:
		bindings = []key.Binding{a.keys.Updates, a.keys.Escape}
	case viewProfiles:
		bindings = []key.Binding{
			a.keys.Up, a.keys.Down, a.keys.Enter,
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "create")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "duplicate")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete")),
			a.keys.Escape,
		}
	case viewSavedVars:
		bindings = []key.Binding{
			a.keys.Up, a.keys.Down, a.keys.Enter,
			a.keys.Backup,
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset")),
			a.keys.Escape,
		}
	default:
		return ""
	}
	var parts []string
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, a.styles.KeyKey.Render(h.Key)+" "+a.styles.KeyHint.Render(h.Desc))
	}
	line := strings.Join(parts, "  ·  ")
	if w := lipgloss.Width(line); w < a.width {
		line += strings.Repeat(" ", a.width-w)
	}
	return "\n" + a.styles.Footer.Render(line)
}

// renderHeader shows the installation and detected version.
func (a *App) renderHeader() string {
	title := a.styles.Title.Render("⚔ wowfix " + Version)
	var sub string
	if a.install != nil {
		det := "unknown version"
		if a.profile != nil {
			det = fmt.Sprintf("%s (expected interface %d)", a.profile.Name, a.profile.Interface)
		}
		if a.install.Version != "" {
			det = a.install.Version + " · " + det
		}
		flavor := a.install.Flavor
		if flavor == "" {
			flavor = "root"
		}
		sub = fmt.Sprintf(" %s  ·  %s  ·  flavor %s", a.install.AddonsPath, det, flavor)
	} else if a.cfg.WoWPath != "" {
		sub = " " + a.cfg.WoWPath
	} else {
		sub = " no installation selected"
	}
	if a.busy {
		sub += "  " + a.spinner.View() + " " + a.busyText
	}
	return title + a.styles.Subtitle.Render(sub)
}

// maxInt/minInt avoid importing math for ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
