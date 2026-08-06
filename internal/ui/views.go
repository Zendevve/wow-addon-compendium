package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/validator"
)

// --- list view --------------------------------------------------------

// listColumnWeights describes how the row budget is split. NAME takes
// the largest share, FIX a fixed minimum chip, the rest a fixed minimum.
const (
	listStatusW = 6
	listSrcW    = 4
	listFixW    = 18
	listVerW    = 9
)

// listColumnRatios returns the proportional widths the row should split
// into given the available inner width. NAME and PROBLEM are the elastic
// columns; PROBLEM collapses entirely when no addon has issues. Rows,
// header, rule and footer all use contentWidth() so nothing lags the
// panel inner width.
func (a *App) listColumnRatios() (statusW, nameW, problemW, verW, srcW, fixW int) {
	statusW = listStatusW
	verW = listVerW
	srcW = listSrcW
	fixW = listFixW
	innerW := a.contentWidth()
	if innerW < 40 {
		innerW = 40
	}
	hasProblems := false
	if a.scan != nil {
		for _, ad := range a.scan.Addons {
			if len(ad.Issues) > 0 {
				hasProblems = true
				break
			}
		}
	}
	seps := 4
	if hasProblems {
		seps = 5
	}
	used := statusW + verW + srcW + fixW + seps
	elastic := innerW - used
	if elastic < 20 {
		elastic = 20
	}
	if !hasProblems {
		problemW = 0
		nameW = elastic
		return
	}
	nameW = elastic * 2 / 5
	if nameW < 12 {
		nameW = 12
	}
	problemW = elastic - nameW
	return
}

// renderList draws the main addon table.
func (a *App) renderList() string {
	if a.scan == nil {
		return a.styles.Hint.Render("No scan yet.")
	}
	if len(a.scan.Addons) == 0 {
		return a.renderEmptyState(
			"No addons here yet.",
			"i install from source · drop a .zip on the window to extract addons")
	}

	statusW, nameW, problemW, verW, srcW, fixW := a.listColumnRatios()
	rows := a.visibleRows()
	var b strings.Builder
	b.WriteString(a.renderListHeader(statusW, nameW, problemW, verW, srcW, fixW))

	n := a.listLen()
	end := a.offset + rows
	if end > n {
		end = n
	}
	if n == 0 {
		b.WriteString(a.renderEmptyState(
			fmt.Sprintf("No addons match %q", a.filter.Value()),
			"esc close the filter · r rescan"))
	} else {
		for row := a.offset; row < end; row++ {
			idx := a.rowToIndex(row)
			if idx < 0 {
				continue
			}
			addon := a.scan.Addons[idx]
			b.WriteString(a.renderRow(addon, row, statusW, nameW, problemW, verW, srcW, fixW))
			if row < end-1 {
				b.WriteString("\n")
			}
		}
	}

	if n > end {
		b.WriteString("\n" + a.styles.RowMuted.Render(fmt.Sprintf("… %d more", n-end)))
	}

	// Filter bar sits at the bottom of the list, not in a dialog.
	if a.filtering {
		b.WriteString("\n" + a.styles.FilterBar.Render(a.filter.View()))
	}
	// Summary status line under the table.
	b.WriteString("\n" + a.renderListSummary())
	b.WriteString(a.renderFooterHints([]string{
		a.hintChip("↑/k", "up"),
		a.hintChip("↓/j", "down"),
		a.hintChip("enter", "inspect"),
		a.hintChip("f", "fix"),
		a.hintChip("/", "filter"),
		a.hintChip("c", "catalog"),
		a.hintChip("u", "updates"),
		a.hintChip("?", "help"),
	}))

	return a.styles.ListBox.Width(a.width).Render(b.String())
}

// renderListHeader draws the column titles. The PROBLEM column is only
// present when the list has at least one flagged addon.
func (a *App) renderListHeader(statusW, nameW, problemW, verW, srcW, fixW int) string {
	st := a.styles
	col := func(label string, w int) string { return st.ColumnHdr.Render(pad(label, w)) }
	cols := []string{
		col("STATUS", statusW),
		col("ADDON", nameW),
	}
	if problemW > 0 {
		cols = append(cols, col("PROBLEM", problemW))
	}
	cols = append(cols,
		col("VERSION", verW),
		col("SRC", srcW),
		col("FIX", fixW))
	line := strings.Join(cols, " ")
	return line + "\n" + st.Rule.Render(strings.Repeat("─", a.contentWidth())) + "\n"
}

// renderRow renders one addon row in the redesigned list.
func (a *App) renderRow(addon *models.Addon, idx, statusW, nameW, problemW, verW, srcW, fixW int) string {
	st := a.styles
	selected := idx == a.cursor

	icon, iconStyle := a.theme.IconForStatus(addon.Status)
	issueCount := len(addon.Issues)
	var firstIssue *models.Issue
	if issueCount > 0 {
		firstIssue = addon.Issues[0]
	}

	// Name.
	name := addon.FolderName
	nameCell := pad(name, nameW)
	if selected {
		nameCell = st.RowNameSel.Render(pad(name, nameW))
	} else {
		nameCell = st.RowName.Render(pad(name, nameW))
	}

	// Version (muted meta).
	version := "—"
	if toc := addon.PrimaryTOC(); toc != nil && toc.Version != "" {
		version = toc.Version
	}
	verCell := st.RowMuted.Render(pad(version, verW))

	// Source badge — only render for tracked addons; "local" is implicit.
	srcCell := ""
	if badge := a.providerBadge(addon.FolderName); badge != "local" {
		srcCell = st.Badge.Render(pad(badge, srcW))
	} else {
		srcCell = st.BadgeMuted.Render(pad("·", srcW))
	}

	// Status: glyph + (optional) issue count. The cell is padded with a
	// lipgloss Width style so the ANSI escapes inside the styled glyph
	// are never counted as runes.
	statusContent := iconStyle.Render(icon)
	if issueCount > 1 {
		statusContent += " " + st.RowMuted.Render(fmt.Sprintf("%d", issueCount))
	}
	statusCell := lipgloss.NewStyle().Width(statusW).Render(statusContent)

	// Fix cell: compact chip with action label, only when fixable.
	fixCell := ""
	if firstIssue != nil && firstIssue.Action != models.ActionNone {
		label := firstIssue.Action.Label()
		fixCell = st.AccentChip.Render(pad(" "+truncate(label, fixW-2)+" ", fixW))
	} else {
		fixCell = st.RowMuted.Render(pad("—", fixW))
	}

	// Problem cell: first issue's message, present only while the column
	// is active; healthy rows keep the slot so versions stay aligned.
	fields := []string{statusCell, nameCell}
	if problemW > 0 {
		problemCell := strings.Repeat(" ", problemW)
		if firstIssue != nil {
			problemCell = st.RowProblem.Render(pad(truncate(firstIssue.Message, problemW), problemW))
		}
		fields = append(fields, problemCell)
	}
	fields = append(fields, verCell, srcCell, fixCell)
	row := strings.Join(fields, " ")

	switch {
	case selected && addon.Status == models.StatusError:
		return st.RowError.Render(row)
	case selected:
		return st.RowSelected.Render(row)
	case addon.Status == models.StatusError:
		return st.RowError.Render(row)
	default:
		return st.Row.Render(row)
	}
}

// providerBadge maps a folder to its registry source badge: a short
// provider tag when the addon is tracked, "local" otherwise.
func (a *App) providerBadge(folder string) string {
	if a.registryByFolder == nil {
		return "local"
	}
	e, ok := a.registryByFolder[strings.ToLower(folder)]
	if !ok || e.Provider == "" {
		return "local"
	}
	switch e.Provider {
	case catalog.ProviderGitHub:
		return "GH"
	case catalog.ProviderCurseForge:
		return "CF"
	case catalog.ProviderWowInterface:
		return "WI"
	case catalog.ProviderTukui:
		return "TK"
	}
	return "local"
}

// --- inspect view -----------------------------------------------------

// renderInspect shows the addon detail: TOC compatibility table and
// issues with suggestions.
func (a *App) renderInspect() string {
	addon := a.inspectAddon
	if addon == nil {
		return ""
	}
	width := a.contentWidth()
	if width < 50 {
		width = 50
	}

	icon, iconStyle := a.theme.IconForStatus(addon.Status)
	st := a.styles
	var b strings.Builder
	meta := flavorLabel("")
	if a.profile != nil {
		meta = a.profile.Name
	}
	b.WriteString(a.renderViewHeader(iconStyle.Render(icon)+"  "+addon.FolderName, meta, width))
	b.WriteString(st.Detail.Render("Folder:   ") + st.Path.Render(filepathDisplay(addon.Path)))
	b.WriteString("\n")
	b.WriteString(st.Detail.Render("Source:   ") + st.Path.Render(addon.SourceDir))
	if addon.SuggestedName != "" && addon.SuggestedName != addon.FolderName {
		b.WriteString("\n" + st.Detail.Render("Suggested: ") +
			st.RowName.Render(addon.SuggestedName))
	}
	if a.inspectSize > 0 {
		b.WriteString("\n" + st.Detail.Render("Size:     ") + st.Path.Render(humanSize(a.inspectSize)))
	}

	// TOC compatibility table.
	if len(addon.TOCs) > 0 {
		b.WriteString("\n\n")
		b.WriteString(st.Section.Render("TOC validation"))
		b.WriteString("\n")
		if a.profile != nil {
			b.WriteString(st.RowMuted.Render(fmt.Sprintf(
				"Expected interface: %d (%s)", a.profile.Interface, a.profile.Name)))
			b.WriteString("\n")
		}
		b.WriteString(st.ColumnHdr.Render(pad("TOC", 26) + pad("IFACE", 8) + "STATUS"))
		b.WriteString("\n")
		for _, compat := range validator.ValidateAddon(addon, a.profile) {
			mark := "?"
			switch compat.Status {
			case models.CompatOK:
				mark = "✓"
			case models.CompatVanilla, models.CompatRetail, models.CompatMismatch:
				mark = "⚠"
			default:
				mark = "?"
			}
			iface := "—"
			if compat.TOC.Interface > 0 {
				iface = fmt.Sprintf("%d", compat.TOC.Interface)
			}
			label := a.theme.StyleForCompat(compat.Status).Render(mark + " " + compat.Label)
			detail := "  " + st.RowMuted.Render(compat.Detail)
			b.WriteString(fmt.Sprintf("%s %s %s%s\n",
				pad(compat.TOC.Name+".toc", 26),
				pad(iface, 8),
				label, detail))
		}
	} else {
		b.WriteString("\n")
		b.WriteString(st.RowMuted.Render("No TOC files found."))
	}

	// Issues.
	if len(addon.Issues) > 0 {
		b.WriteString("\n\n")
		b.WriteString(st.Section.Render(fmt.Sprintf("Issues (%d)", len(addon.Issues))))
		b.WriteString("\n")
		for _, issue := range addon.Issues {
			sev := "•"
			sevStyle := st.Detail
			switch issue.Severity {
			case models.SeverityError:
				sev, sevStyle = "✖", st.StatusErr
			case models.SeverityWarn:
				sev, sevStyle = "⚠", st.StatusWarn
			}
			b.WriteString(sevStyle.Render(sev) + " " + st.Detail.Render(issue.Message))
			b.WriteString("\n  " + st.RowMuted.Render("→ "+issue.Suggestion))
			if len(issue.Options) > 0 {
				b.WriteString("\n  " + st.RowMuted.Render("TOC candidates: "+strings.Join(issue.Options, ", ")))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n" + a.renderFooterHints([]string{
		a.hintChip("enter/esc", "back"),
		a.hintChip("f", "fix"),
		a.hintChip("d", "trash"),
		a.hintChip("↑/↓", "scroll"),
	}))

	return st.ListBox.Width(a.width).Render(b.String())
}

// --- logs view --------------------------------------------------------

// renderLogs draws the scrolling log buffer.
func (a *App) renderLogs() string {
	entries := a.log.Entries()
	rows := a.visibleRows()
	start := len(entries) - rows
	if start < 0 {
		start = 0
	}
	if a.logsOffset > 0 {
		start = maxInt(0, start-a.logsOffset)
	}
	width := a.contentWidth()
	if width < 40 {
		width = 40
	}

	st := a.styles
	var b strings.Builder
	b.WriteString(a.renderViewHeader("Logs", fmt.Sprintf("%d entries", len(entries)), width))
	if len(entries) == 0 {
		b.WriteString(a.renderEmptyState("No log entries yet.", "errors and scan events show up here"))
	} else {
		for i := start; i < len(entries) && i < start+rows; i++ {
			e := entries[i]
			style := st.RowMuted
			levelStyle := st.RowMuted
			switch e.Level {
			case "ERROR":
				style = st.Row
				levelStyle = st.StatusErr
			case "WARN":
				style = st.Row
				levelStyle = st.StatusWarn
			case "INFO":
				levelStyle = st.StatusOK
			}
			ts := st.RowMuted.Render(e.Time.Format("15:04:05"))
			tag := levelStyle.Render(pad(" "+string(e.Level)+" ", 6))
			msgMax := maxInt(10, width-18)
			msg := style.Render(truncate(e.Message, msgMax))
			b.WriteString(ts + "  " + tag + "  " + msg)
			b.WriteString("\n")
		}
	}
	b.WriteString(a.renderFooterHints([]string{
		a.hintChip("e", "export"),
		a.hintChip("↑/↓", "scroll"),
		a.hintChip("esc", "back"),
	}))
	return st.ListBox.Width(a.width).Render(b.String())
}

// --- picker -----------------------------------------------------------

// renderPicker lists auto-detected installations and offers manual entry.
func (a *App) renderPicker() string {
	width := a.contentWidth()
	if width < 60 {
		width = 60
	}
	st := a.styles
	var b strings.Builder
	b.WriteString(a.renderViewHeader("Select a WoW installation",
		fmt.Sprintf("%d found", len(a.picker)), width))

	if len(a.picker) == 0 {
		b.WriteString(a.renderEmptyState(
			"No installations found.",
			"s type a path manually · esc back"))
	} else {
		for i, inst := range a.picker {
			selected := i == a.pickerCur
			row := a.renderPickerRow(inst, selected, width)
			b.WriteString(row)
			b.WriteString("\n")
		}
	}
	b.WriteString(a.renderFooterHints([]string{
		a.hintChip("↑/↓", "navigate"),
		a.hintChip("enter", "select"),
		a.hintChip("s", "type a path"),
		a.hintChip("esc", "back"),
		a.hintChip("q", "quit"),
	}))
	return st.ListBox.Width(a.width).Render(b.String())
}

// renderPickerRow renders a single installation in the picker.
func (a *App) renderPickerRow(inst detector.Installation, selected bool, width int) string {
	st := a.styles
	// Two-line row: path prominent, meta line muted.
	flavorName := flavorLabel(inst.Flavor)
	confGlyph := "·"
	confStyle := st.RowMuted
	if inst.Confidence == "high" {
		confGlyph = "●"
		confStyle = st.StatusOK
	} else if inst.Confidence == "low" {
		confGlyph = "○"
		confStyle = st.StatusWarn
	}

	ver := inst.Version
	if ver == "" {
		ver = "unknown version"
	}
	metaLine := fmt.Sprintf("%s  %s  ·  %s  ·  %s",
		flavorName,
		confStyle.Render(confGlyph)+" "+st.RowMuted.Render(inst.Confidence),
		st.RowMuted.Render(ver),
		st.RowMuted.Render("interface "+flavorName),
	)

	marker := "  "
	rowStyle := st.Row
	nameStyle := st.Row
	if selected {
		marker = "▸ "
		rowStyle = st.RowSelected
		nameStyle = st.RowNameSel
	}
	path := filepathDisplay(inst.AddonsPath)
	line1 := marker + nameStyle.Render(pad(path, width-lipgloss.Width(marker)))
	line2 := "    " + metaLine

	// No trailing newline: renderPicker appends one per row.
	row := line1 + "\n" + line2
	if selected {
		return rowStyle.Render(row)
	}
	return row
}

// --- profile ----------------------------------------------------------

// renderProfile lists the supported game profiles.
func (a *App) renderProfile() string {
	width := a.contentWidth()
	if width < 60 {
		width = 60
	}
	st := a.styles
	var b strings.Builder
	b.WriteString(a.renderViewHeader("Game profile", "", width))
	for i, p := range models.Profiles {
		selected := i == a.pickerCur
		active := ""
		if a.profile != nil && a.profile.ID == p.ID {
			active = "  active"
		}
		row := a.renderProfileRow(p, selected, active, width)
		b.WriteString(row)
		b.WriteString("\n")
	}
	b.WriteString(a.renderFooterHints([]string{
		a.hintChip("↑/↓", "navigate"),
		a.hintChip("enter", "select"),
		a.hintChip("esc", "back"),
	}))
	return st.ListBox.Width(a.width).Render(b.String())
}

// renderProfileRow renders a single profile row.
func (a *App) renderProfileRow(p models.Profile, selected bool, active string, width int) string {
	st := a.styles
	marker := "  "
	rowStyle := st.Row
	nameStyle := st.Row
	if selected {
		marker = "▸ "
		rowStyle = st.RowSelected
		nameStyle = st.RowNameSel
	}
	if active != "" {
		active = st.StatusOK.Render("  active")
	}
	// Lay out plain cells first, then style; the row is padded with an
	// ANSI-aware helper so styled text is never rune-counted.
	nameCell := pad(p.Name, 28)
	line1 := marker + nameStyle.Render(nameCell) + st.RowMuted.Render(fmt.Sprintf("interface %d", p.Interface)) + active
	return rowStyle.Render(padToVisibleWidth(line1, width))
}

// --- confirm / input / help ------------------------------------------

// renderConfirm shows the confirmation dialog.
func (a *App) renderConfirm() string {
	st := a.styles
	msg := a.confirmMsg
	msg = strings.ReplaceAll(msg, "\n", "\n    ")
	body := lipgloss.JoinVertical(lipgloss.Left,
		st.Section.Render(a.confirmTitle),
		"",
		"    "+st.Detail.Render(msg),
		"",
		"    "+a.hintChip("y/enter", "confirm")+"  "+a.hintChip("n/esc", "cancel"),
	)
	dialog := st.Dialog.Render(body)
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, dialog)
}

// renderInput shows the manual path or install-source prompt.
func (a *App) renderInput() string {
	st := a.styles
	title := "WoW installation path"
	hint := "paste a folder path and press enter"
	extraHint := a.hintChip("ctrl+v", "paste")
	if a.inputMode == inputSource {
		title = "Install from source"
		hint = "owner/repo or full URL (github, curseforge, wowinterface, tukui)"
		extraHint = a.hintChip("ctrl+v", "paste")
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		st.Section.Render(title),
		"",
		"    "+st.Input.Render(a.input.View()),
		"",
		"    "+st.Hint.Render(hint),
		"    "+a.hintChip("enter", "confirm")+"  "+a.hintChip("esc", "cancel")+"  "+extraHint,
	)
	dialog := st.Dialog.Render(body)
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, dialog)
}

// renderHelp shows a full-screen keybinding reference in a bordered
// panel centered on the screen.
func (a *App) renderHelp() string {
	outerW := a.width - 8
	if outerW < 60 {
		outerW = 60
	}
	innerW := a.dialogContentWidth(outerW)
	st := a.styles
	var b strings.Builder
	b.WriteString(a.renderViewHeader("Keybindings", "esc / q close", innerW))
	groups := a.helpGroups()
	// Two-column layout when width allows; otherwise single column.
	colW := (innerW - 4) / 2
	if colW < 30 {
		colW = 0
	}
	if colW > 0 && len(groups) > 4 {
		left := groups[:(len(groups)+1)/2]
		right := groups[(len(groups)+1)/2:]
		leftR := a.renderHelpColumn(left, colW)
		rightR := a.renderHelpColumn(right, colW)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftR, "  ", rightR))
	} else {
		for _, g := range groups {
			b.WriteString(a.renderHelpGroup(g, innerW))
		}
	}
	b.WriteString("\n" + lipgloss.NewStyle().Width(innerW).Render(
		st.Hint.Render("esc / q close help")))
	panel := st.Dialog.Width(outerW).Render(b.String())
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, panel)
}

// renderHelpColumn lays a stack of help groups one after another.
func (a *App) renderHelpColumn(groups []helpGroup, colW int) string {
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(a.renderHelpGroup(g, colW))
	}
	return b.String()
}

// renderHelpGroup renders a single titled help section, constrained to
// the given width so two-column mode never lets rows bleed across.
func (a *App) renderHelpGroup(g helpGroup, width int) string {
	st := a.styles
	var b strings.Builder
	b.WriteString(st.Detail.Render(g.title))
	b.WriteString("\n")
	// Row budget: 2 indent + 18 key slot + 1 gap; the description takes
	// the remainder and is truncated when the column is narrow. The
	// floor keeps rows inside the narrowest enabled column (colW 30).
	descMax := width - 21
	if descMax < 10 {
		descMax = 10
	}
	for _, item := range g.items {
		key := st.KeyKey.Render(pad(item.keys, 18))
		desc := st.KeyHint.Render(truncate(item.desc, descMax))
		b.WriteString("  " + key + desc)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// helpGroup is one titled section of the help overlay.
type helpGroup struct {
	title string
	items []helpItem
}

// helpItem is one binding row: the key chord and its description.
type helpItem struct {
	keys string
	desc string
}

// helpGroups lists every view's keybindings, sourced from the key map so
// new bindings appear here automatically.
func (a *App) helpGroups() []helpGroup {
	item := func(b key.Binding) helpItem {
		h := b.Help()
		return helpItem{keys: h.Key, desc: h.Desc}
	}
	group := func(title string, bs ...key.Binding) helpGroup {
		items := make([]helpItem, 0, len(bs))
		for _, b := range bs {
			items = append(items, item(b))
		}
		return helpGroup{title: title, items: items}
	}
	return []helpGroup{
		group("Navigation",
			a.keys.Up, a.keys.Down, a.keys.Enter, a.keys.Escape),
		group("Addon actions",
			a.keys.Fix, a.keys.FixAll, a.keys.Delete, a.keys.Rescan,
			a.keys.Backup, a.keys.Export),
		group("Views",
			a.keys.Logs, a.keys.Profile, a.keys.Theme, a.keys.Install,
			a.keys.Filter, a.keys.Help, a.keys.Quit),
		group("Text input",
			a.keys.Paste, a.keys.Copy),
		group("Collections",
			a.keys.Profiles, a.keys.SavedVars),
		group("Catalog",
			a.keys.Catalog, a.keys.Updates, a.keys.Source,
			key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "update all (updates view)")),
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search (catalog view)")),
			key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "sort results (catalog view)")),
			key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "filter by game version (catalog view)")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "addon details (catalog view)")),
			key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open homepage (detail view)")),
			key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "GitHub releases (detail view)")),
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install (detail view)")),
			a.keys.Enter),
	}
}

// filepathDisplay shortens long paths for display.
func filepathDisplay(p string) string {
	if len(p) > 70 {
		return "…" + p[len(p)-69:]
	}
	return p
}

// filepathMiddle truncates the middle of a long path so a path with
// a long parent or a long tail fits in the available width. It is used
// by the header and the picker where the path is a single line.
func filepathMiddle(p string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(p)
	if len(runes) <= max {
		return p
	}
	if max <= 1 {
		return "…"
	}
	// keep first and last segments, ellipsis in the middle
	head := max/2 - 1
	if head < 1 {
		head = 1
	}
	tail := max - head - 1
	if tail < 1 {
		tail = 1
	}
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

// humanSize renders bytes as a readable size.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// toastStyleFor returns a (style, glyph) pair for a toast message. The
// glyph + color encodes severity so users see at a glance whether the
// toast is informational, success, a warning, or a failure. The text
// is matched with case-insensitive prefixes; the unknown case falls
// back to the info style.
func (a *App) toastStyleFor(text string) (lipgloss.Style, string) {
	st := a.styles
	low := strings.ToLower(text)
	switch {
	case strings.HasPrefix(low, "scan failed"),
		strings.HasPrefix(low, "install failed"),
		strings.HasPrefix(low, "update failed"),
		strings.HasPrefix(low, "backup failed"),
		strings.HasPrefix(low, "export failed"),
		strings.HasPrefix(low, "savedvariables backup failed"),
		strings.HasPrefix(low, "search failed"),
		strings.HasPrefix(low, "update check:"),
		strings.Contains(low, "failed"):
		return st.ToastErr, "✖"
	case strings.HasPrefix(low, "all tracked addons are up to date"),
		strings.HasPrefix(low, "installed "),
		strings.HasPrefix(low, "updated "),
		strings.HasPrefix(low, "scanned "),
		strings.HasPrefix(low, "backup created:"),
		strings.HasPrefix(low, "logs exported to "),
		strings.HasPrefix(low, "savedvariables backed up to "),
		strings.HasPrefix(low, "profile set to"):
		return st.ToastOK, "✔"
	case strings.HasPrefix(low, "no wow installation auto-detected"),
		strings.HasPrefix(low, "no installation selected"),
		strings.HasPrefix(low, "catalog unavailable"):
		return st.ToastWarn, "⚠"
	default:
		return st.ToastInfo, "ℹ"
	}
}
