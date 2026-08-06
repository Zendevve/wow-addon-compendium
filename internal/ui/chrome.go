package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// contentWidth returns the number of cells available inside the
// rounded-bordered, padded listbox that wraps every view body. It is
// the single source of truth for header, rule, row and hint widths so
// they never disagree with the panel that contains them.
func (a *App) contentWidth() int {
	w := a.width - a.styles.ListBox.GetHorizontalFrameSize()
	if w < 20 {
		w = 20
	}
	return w
}

// dialogContentWidth returns the inner content width for a centered
// dialog of the given outer width. The dialog has a rounded border
// plus padding(1, 2), so its inner area is outer minus the dialog's
// own horizontal frame.
func (a *App) dialogContentWidth(outerWidth int) int {
	w := outerWidth - a.styles.Dialog.GetHorizontalFrameSize()
	if w < 20 {
		w = 20
	}
	return w
}

// renderViewHeader produces a single-line view title bar in the shared
// language: bold accent title, muted meta right-aligned, divider rule
// beneath. The whole row is built inside an innerW-constrained lipgloss
// style so the parent panel never has to wrap a long header mid-meta.
// Pass contentWidth() for listbox panels, or dialogContentWidth(outer)
// for centered dialogs.
func (a *App) renderViewHeader(title, meta string, innerW int) string {
	st := a.styles
	if innerW < 20 {
		innerW = 20
	}
	ruleChar := "─"
	var b strings.Builder
	if meta == "" {
		titleR := st.Section.Render(title)
		b.WriteString(lipgloss.NewStyle().Width(innerW).Render(titleR))
		b.WriteString("\n")
	} else {
		titleR := st.Section.Render(title)
		titleW := lipgloss.Width(titleR)
		metaR := st.Hint.Render(meta)
		if titleW+2+lipgloss.Width(metaR) > innerW {
			b.WriteString(lipgloss.NewStyle().Width(innerW).Render(titleR))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Width(innerW).Render(metaR))
			b.WriteString("\n")
		} else {
			titleSlot := lipgloss.NewStyle().Width(titleW).Render(titleR)
			metaSlot := lipgloss.NewStyle().
				Width(innerW - titleW).
				Align(lipgloss.Right).
				Render(metaR)
			b.WriteString(lipgloss.NewStyle().Width(innerW).Render(
				lipgloss.JoinHorizontal(lipgloss.Top, titleSlot, metaSlot)))
			b.WriteString("\n")
		}
	}
	b.WriteString(st.Rule.Render(strings.Repeat(ruleChar, innerW)))
	b.WriteString("\n")
	return b.String()
}

// renderEmptyState renders a friendly two-line empty state. The first
// line is the title (bold), the second is the hint (muted).
func (a *App) renderEmptyState(title, hint string) string {
	st := a.styles
	innerW := a.contentWidth()
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Width(innerW).Render(st.Empty.Render(title)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Width(innerW).Render(st.EmptySub.Render(hint)))
	b.WriteString("\n")
	return b.String()
}

// renderListSummary renders the "N addons · K with issues · E errors"
// status line under the main list.
func (a *App) renderListSummary() string {
	if a.scan == nil {
		return ""
	}
	total, problems, errors := a.scan.Stats()
	st := a.styles
	var parts []string
	parts = append(parts, fmt.Sprintf("%s addons", st.SummaryN.Render(fmt.Sprintf("%d", total))))
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%s error%s", st.StatusErr.Render(fmt.Sprintf("%d", errors)), plural(errors)))
	}
	if problems > 0 && problems > errors {
		parts = append(parts, fmt.Sprintf("%s with issues", st.StatusWarn.Render(fmt.Sprintf("%d", problems-errors))))
	}
	clean := total - problems
	if clean > 0 {
		parts = append(parts, fmt.Sprintf("%s healthy", st.StatusOK.Render(fmt.Sprintf("%d", clean))))
	}
	if a.filtering {
		parts = append(parts, fmt.Sprintf("%s of %d match", st.SummaryN.Render(fmt.Sprintf("%d", len(a.filterIdx))), total))
	}
	return lipgloss.NewStyle().Width(a.contentWidth()).Render(
		st.Summary.Render(strings.Join(parts, "  ·  ")))
}

// renderFooterHints renders the keymap hint bar with explicit
// separators. When the row would overflow the panel, trailing chips
// are dropped so the footer never wraps inside the box.
func (a *App) renderFooterHints(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	st := a.styles
	w := a.contentWidth()
	sep := "  ·  "
	plain := strings.Join(parts, sep)
	plainW := lipgloss.Width(plain)
	for plainW > w && len(parts) > 1 {
		parts = parts[:len(parts)-1]
		plain = strings.Join(parts, sep)
		plainW = lipgloss.Width(plain)
	}
	row := plain
	if plainW > w {
		if len(parts) > 1 {
			row = strings.Join(parts[:len(parts)-1], sep) + sep + "…"
		} else {
			// A single chip still too wide: truncate it instead of
			// discarding the hint. MaxWidth is ANSI-aware.
			row = lipgloss.NewStyle().MaxWidth(w).Render(parts[0])
		}
	}
	return "\n" + lipgloss.NewStyle().Width(w).Render(st.Footer.Render(row))
}

// hintChip formats a single key+desc pair as a styled chip: the key
// chord in accent-bold, a thin gap, and the description in muted.
func (a *App) hintChip(key, desc string) string {
	if key == "" {
		return a.styles.KeyHint.Render(desc)
	}
	return a.styles.KeyKey.Render(key) + " " + a.styles.KeyHint.Render(desc)
}

// plural returns "s" for any n != 1 (English plural suffix).
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
