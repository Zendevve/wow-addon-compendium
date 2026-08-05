// Package ui implements the Bubble Tea terminal interface: an addon
// list with status, problem and suggested fix, plus inspect, logs,
// installation picker, profile picker and confirmation dialogs.
package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/fixer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/scanner"
	"github.com/wowfix/wowfix/internal/utils"
)

// Version is stamped by the build; the UI shows it in the header.
var Version = "dev"

// keyMap holds every key binding.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Escape   key.Binding
	Fix      key.Binding
	FixAll   key.Binding
	Delete   key.Binding
	Rescan   key.Binding
	Backup   key.Binding
	Logs     key.Binding
	Export   key.Binding
	Install  key.Binding
	Profile  key.Binding
	Theme    key.Binding
	Quit     key.Binding
	Yes      key.Binding
	No       key.Binding
	ScrollUp key.Binding
	ScrollDn key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "inspect")),
		Escape:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Fix:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix")),
		FixAll:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "fix all")),
		Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "trash")),
		Rescan:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Backup:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backup")),
		Logs:     key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Export:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export logs")),
		Profile:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "profile")),
		Theme:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		Install:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch install")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Yes:      key.NewBinding(key.WithKeys("y", "enter")),
		No:       key.NewBinding(key.WithKeys("n", "esc")),
		ScrollUp: key.NewBinding(key.WithKeys("up", "k", "pgup")),
		ScrollDn: key.NewBinding(key.WithKeys("down", "j", "pgdn")),
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
	toast    string
	toastAt  time.Time
	quitting bool

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

	return &App{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		store:   store,
		log:     log,
		keys:    defaultKeys(),
		theme:   theme,
		styles:  NewStyles(theme),
		spinner: sp,
		input:   input,
		profile: models.ProfileByID(cfg.Profile),
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
	a.toast = text
	a.toastAt = time.Now()
}

// addonAt returns the addon under the cursor, or nil.
func (a *App) addonAt() *models.Addon {
	if a.scan == nil || len(a.scan.Addons) == 0 {
		return nil
	}
	if a.cursor < 0 || a.cursor >= len(a.scan.Addons) {
		return nil
	}
	return a.scan.Addons[a.cursor]
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
func (a *App) visibleRows() int {
	rows := a.height - 12
	if rows < 3 {
		rows = 3
	}
	return rows
}

// clampCursor keeps the cursor inside the list and scrolls the offset.
func (a *App) clampCursor() {
	if a.scan == nil {
		a.cursor, a.offset = 0, 0
		return
	}
	n := len(a.scan.Addons)
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
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
		total, problems, errors := m.result.Stats()
		a.pushToast(fmt.Sprintf("Scanned %d addon(s) — %d with issues, %d errors", total, problems, errors))
		a.log.Infof("Scan complete: %d addons, %d problems, %d errors", total, problems, errors)
		a.view = viewList
		a.clampCursor()
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

	case busyDoneMsg:
		a.busy = false
		return a, nil
	}

	return a, tea.Batch(cmds...)
}

// updateKey dispatches keyboard input by view.
func (a *App) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	default:
		return a.updateListKey(msg)
	}
}

// --- list view keys ---------------------------------------------------

func (a *App) updateListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Quit):
		if a.busy {
			return a, nil
		}
		a.quitting = true
		return a, tea.Quit

	case key.Matches(msg, a.keys.Up):
		if a.scan != nil && len(a.scan.Addons) > 0 {
			a.cursor--
			a.clampCursor()
		}

	case key.Matches(msg, a.keys.Down):
		if a.scan != nil && len(a.scan.Addons) > 0 {
			a.cursor++
			a.clampCursor()
		}

	case key.Matches(msg, a.keys.Enter):
		if a.scan == nil || len(a.scan.Addons) == 0 {
			return a, nil
		}
		a.view = viewInspect
		a.inspectAddon = a.addonAt()
		a.inspectSize = 0
		if size, err := utils.DirSize(a.inspectAddon.Path); err == nil {
			a.inspectSize = size
		}
		return a, nil

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

	case key.Matches(msg, a.keys.Install):
		a.view = viewInput
		a.input.Reset()
		a.input.Placeholder = "Path to WoW installation (e.g. C:\\Games\\World of Warcraft)"
		a.input.Focus()
		return a, textinput.Blink
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
		a.view = viewPicker
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		path := strings.TrimSpace(a.input.Value())
		a.input.Blur()
		if path == "" {
			a.view = viewPicker
			return a, nil
		}
		a.cfg.WoWPath = path
		a.cfg.Flavor = ""
		a.save()
		a.view = viewList
		a.busy = true
		a.busyText = "Scanning…"
		return a, a.scanCmd(path, "")
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
	default:
		body = a.renderList()
	}
	return a.renderFrame(body)
}

// renderFrame wraps body in header/footer chrome.
func (a *App) renderFrame(body string) string {
	var b strings.Builder
	b.WriteString(a.renderHeader())
	b.WriteString("\n")
	b.WriteString(body)
	if a.toast != "" && time.Since(a.toastAt) < 6*time.Second {
		b.WriteString("\n" + a.styles.Toast.Render(a.toast))
	}
	return a.styles.App.Render(b.String())
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
