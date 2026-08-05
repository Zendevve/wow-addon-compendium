package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/validator"
)

// renderList draws the main addon table.
func (a *App) renderList() string {
	if a.scan == nil {
		return a.styles.Hint.Render("No scan yet.")
	}
	if len(a.scan.Addons) == 0 {
		return a.styles.Hint.Render("AddOns directory is empty: " + a.scan.AddonsDir)
	}

	rows := a.visibleRows()
	width := a.width - 4 // list box padding + borders
	if width < 40 {
		width = 40
	}
	statusW, nameW, problemW, fixW := 9, 26, width-9-26-18, 18
	if problemW < 12 {
		problemW = 12
	}

	var b strings.Builder
	header := fmt.Sprintf("%s %s %s %s",
		pad("STATUS", statusW),
		pad("ADDON", nameW),
		pad("PROBLEM", problemW),
		pad("FIX", fixW))
	b.WriteString(a.styles.ColumnHeader.Render(header))
	b.WriteString("\n")

	end := a.offset + rows
	if end > len(a.scan.Addons) {
		end = len(a.scan.Addons)
	}
	for i := a.offset; i < end; i++ {
		addon := a.scan.Addons[i]
		b.WriteString(a.renderRow(addon, i, statusW, nameW, problemW, fixW))
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Footer note for any unlisted rows.
	if len(a.scan.Addons) > end {
		b.WriteString("\n" + a.styles.RowMuted.Render(fmt.Sprintf("… %d more", len(a.scan.Addons)-end)))
	}

	return a.styles.ListBox.Width(width + 2).Render(b.String())
}

func (a *App) renderRow(addon *models.Addon, idx, statusW, nameW, problemW, fixW int) string {
	icon, iconStyle := a.theme.IconForStatus(addon.Status)
	selected := idx == a.cursor

	name := addon.FolderName
	problem := "—"
	fix := ""
	for _, issue := range addon.Issues {
		problem = issue.Message
		if issue.Action != models.ActionNone {
			fix = issue.Action.Label()
		}
		break
	}
	if addon.Nested {
		problem = "Nested folder"
	}

	status := iconStyle.Render(pad("  "+icon, statusW))
	problemCell := a.styles.RowMuted.Render(pad(problem, problemW))
	fixCell := a.styles.RowMuted.Render(pad(fix, fixW))

	row := fmt.Sprintf("%s %s %s %s",
		status,
		a.styles.RowName.Render(pad(name, nameW)),
		problemCell,
		fixCell)

	if selected {
		return a.styles.RowSelected.Render(row)
	}
	return a.styles.Row.Render(row)
}

// renderInspect shows the addon detail: TOC compatibility table and
// issues with suggestions.
func (a *App) renderInspect() string {
	addon := a.inspectAddon
	if addon == nil {
		return ""
	}
	width := a.width - 6
	if width < 50 {
		width = 50
	}

	icon, iconStyle := a.theme.IconForStatus(addon.Status)
	title := iconStyle.Render(icon) + " " + a.styles.Section.Render(addon.FolderName)

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(a.styles.Detail.Render("Folder: ") + a.styles.Path.Render(filepathDisplay(addon.Path)))
	b.WriteString("\n")
	b.WriteString(a.styles.Detail.Render("Source: ") + a.styles.Path.Render(addon.SourceDir))
	if addon.SuggestedName != "" && addon.SuggestedName != addon.FolderName {
		b.WriteString("\n" + a.styles.Detail.Render("Suggested name: ") +
			a.styles.RowName.Render(addon.SuggestedName))
	}
	if a.inspectSize > 0 {
		b.WriteString("\n" + a.styles.Detail.Render("Size: ") + a.styles.Path.Render(humanSize(a.inspectSize)))
	}

	// TOC compatibility table.
	if len(addon.TOCs) > 0 {
		b.WriteString("\n\n")
		b.WriteString(a.styles.Section.Render("TOC validation"))
		b.WriteString("\n")
		if a.profile != nil {
			b.WriteString(a.styles.RowMuted.Render(fmt.Sprintf(
				"Expected interface: %d (%s)", a.profile.Interface, a.profile.Name)))
			b.WriteString("\n")
		}
		b.WriteString(a.styles.ColumnHeader.Render(pad("TOC", 26) + pad("IFACE", 8) + "STATUS"))
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
			detail := "  " + a.styles.RowMuted.Render(compat.Detail)
			b.WriteString(fmt.Sprintf("%s %s %s%s\n",
				pad(compat.TOC.Name+".toc", 26),
				pad(iface, 8),
				label, detail))
		}
	} else {
		b.WriteString("\n")
		b.WriteString(a.styles.RowMuted.Render("No TOC files found."))
	}

	// Issues.
	if len(addon.Issues) > 0 {
		b.WriteString("\n\n")
		b.WriteString(a.styles.Section.Render("Issues"))
		b.WriteString("\n")
		for _, issue := range addon.Issues {
			sev := "•"
			sevStyle := a.styles.Detail
			switch issue.Severity {
			case models.SeverityError:
				sev, sevStyle = "✖", a.theme.StyleForCompat(models.CompatRetail)
			case models.SeverityWarn:
				sev, sevStyle = "⚠", a.theme.StyleForCompat(models.CompatMismatch)
			}
			b.WriteString(sevStyle.Render(sev) + " " + a.styles.Detail.Render(issue.Message))
			b.WriteString("\n  " + a.styles.RowMuted.Render("→ "+issue.Suggestion))
			if len(issue.Options) > 0 {
				b.WriteString("\n  " + a.styles.RowMuted.Render("TOC candidates: "+strings.Join(issue.Options, ", ")))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n" + a.styles.Hint.Render(
		"f fix · d trash · enter/esc back"))

	return a.styles.ListBox.Width(width).Render(b.String())
}

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
	width := a.width - 4
	if width < 40 {
		width = 40
	}

	var b strings.Builder
	b.WriteString(a.styles.Section.Render("Logs"))
	b.WriteString("\n")
	if len(entries) == 0 {
		b.WriteString(a.styles.Hint.Render("No log entries yet."))
	} else {
		for i := start; i < len(entries) && i < start+rows; i++ {
			e := entries[i]
			level := e.Level
			style := a.styles.RowMuted
			switch e.Level {
			case "ERROR":
				style = a.theme.StyleForCompat(models.CompatRetail)
			case "WARN":
				style = a.theme.StyleForCompat(models.CompatMismatch)
			}
			line := fmt.Sprintf("%s %-5s %s", e.Time.Format("15:04:05"), level, e.Message)
			b.WriteString(style.Render(truncate(line, width)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" + a.styles.Hint.Render("e export · esc back"))
	return a.styles.ListBox.Width(width).Render(b.String())
}

// renderPicker lists auto-detected installations and offers manual entry.
func (a *App) renderPicker() string {
	width := a.width - 6
	if width < 60 {
		width = 60
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render("Select a WoW installation"))
	b.WriteString("\n\n")

	if len(a.picker) == 0 {
		b.WriteString(a.styles.Hint.Render("No installations found. Press s to type a path manually."))
		b.WriteString("\n\n")
	} else {
		for i, inst := range a.picker {
			mark := " "
			if i == a.pickerCur {
				mark = "▸"
			}
			ver := inst.ProfileID
			if inst.Version != "" {
				ver = inst.Version + " · " + ver
			}
			if ver == "" || ver == " · " {
				ver = "unknown version"
			}
			flavor := inst.Flavor
			if flavor == "" {
				flavor = "root"
			}
			row := fmt.Sprintf("%s %s  (%s, flavor %s)", mark, inst.AddonsPath, ver, flavor)
			if i == a.pickerCur {
				b.WriteString(a.styles.RowSelected.Render(pad(row, width)))
			} else {
				b.WriteString(a.styles.Row.Render(pad(row, width)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(a.styles.Hint.Render("↑/↓ navigate · enter select · s manual path · esc back · q quit"))
	return a.styles.ListBox.Width(width).Render(b.String())
}

// renderProfile lists the supported game profiles.
func (a *App) renderProfile() string {
	width := a.width - 6
	if width < 60 {
		width = 60
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render("Game profile"))
	b.WriteString("\n\n")
	for i, p := range models.Profiles {
		mark := " "
		if i == a.pickerCur {
			mark = "▸"
		}
		current := ""
		if a.profile != nil && a.profile.ID == p.ID {
			current = "  (active)"
		}
		row := fmt.Sprintf("%s %-38s interface %d%s", mark, p.Name, p.Interface, current)
		if i == a.pickerCur {
			b.WriteString(a.styles.RowSelected.Render(pad(row, width)))
		} else {
			b.WriteString(a.styles.Row.Render(pad(row, width)))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + a.styles.Hint.Render("↑/↓ navigate · enter select · esc back"))
	return a.styles.ListBox.Width(width).Render(b.String())
}

// renderConfirm shows the confirmation dialog.
func (a *App) renderConfirm() string {
	msg := a.confirmMsg
	// Allow multiline messages to render.
	msg = strings.ReplaceAll(msg, "\n", "\n    ")
	dialog := a.styles.Dialog.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			a.styles.Section.Render(a.confirmTitle),
			"\n    "+msg,
			"\n\n    "+a.styles.Hint.Render("y/enter confirm · n/esc cancel"),
		))
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, dialog)
}

// renderInput shows the manual path prompt.
func (a *App) renderInput() string {
	dialog := a.styles.Dialog.Render(lipgloss.JoinVertical(lipgloss.Left,
		a.styles.Section.Render("WoW installation path"),
		"\n    "+a.input.View(),
		"\n\n    "+a.styles.Hint.Render("enter confirm · esc cancel"),
	))
	return lipgloss.Place(a.width, a.height-3, lipgloss.Center, lipgloss.Center, dialog)
}

// filepathDisplay shortens long paths for display.
func filepathDisplay(p string) string {
	if len(p) > 70 {
		return "…" + p[len(p)-69:]
	}
	return p
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
