package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/wowfix/wowfix/internal/models"
)

// Theme holds the color palette and derived styles. The palette is
// WoW-flavored: deep slate backgrounds, gold accents.
type Theme struct {
	Name string

	accent    lipgloss.Color
	accentDim lipgloss.Color
	bg        lipgloss.Color
	bgSel     lipgloss.Color
	border    lipgloss.Color
	text      lipgloss.Color
	muted     lipgloss.Color
	ok        lipgloss.Color
	warn      lipgloss.Color
	err       lipgloss.Color
	info      lipgloss.Color
}

// Dark is the default theme.
func Dark() Theme {
	return Theme{
		Name:      "dark",
		accent:    lipgloss.Color("#d4af37"),
		accentDim: lipgloss.Color("#8a7322"),
		bg:        lipgloss.Color("#0f141e"),
		bgSel:     lipgloss.Color("#1c2434"),
		border:    lipgloss.Color("#2c3a52"),
		text:      lipgloss.Color("#e2e8f0"),
		muted:     lipgloss.Color("#64748b"),
		ok:        lipgloss.Color("#22c55e"),
		warn:      lipgloss.Color("#f59e0b"),
		err:       lipgloss.Color("#f43f5e"),
		info:      lipgloss.Color("#38bdf8"),
	}
}

// Light is an alternative theme for light terminals.
func Light() Theme {
	return Theme{
		Name:      "light",
		accent:    lipgloss.Color("#92600a"),
		accentDim: lipgloss.Color("#b8860b"),
		bg:        lipgloss.Color("#f8fafc"),
		bgSel:     lipgloss.Color("#eef2f7"),
		border:    lipgloss.Color("#cbd5e1"),
		text:      lipgloss.Color("#1e293b"),
		muted:     lipgloss.Color("#64748b"),
		ok:        lipgloss.Color("#15803d"),
		warn:      lipgloss.Color("#b45309"),
		err:       lipgloss.Color("#be123c"),
		info:      lipgloss.Color("#0369a1"),
	}
}

// ThemeByName resolves "dark" or "light"; unknown names fall back to dark.
func ThemeByName(name string) Theme {
	if name == "light" {
		return Light()
	}
	return Dark()
}

// IconForStatus returns the status glyph for an addon.
func (t Theme) IconForStatus(s models.AddonStatus) (string, lipgloss.Style) {
	switch s {
	case models.StatusError:
		return "✖", lipgloss.NewStyle().Foreground(t.err)
	case models.StatusWarn:
		return "⚠", lipgloss.NewStyle().Foreground(t.warn)
	default:
		return "✔", lipgloss.NewStyle().Foreground(t.ok)
	}
}

// StyleForCompat colors a compatibility label by its verdict.
func (t Theme) StyleForCompat(c models.CompatStatus) lipgloss.Style {
	switch c {
	case models.CompatOK:
		return lipgloss.NewStyle().Foreground(t.ok)
	case models.CompatVanilla, models.CompatRetail, models.CompatMismatch:
		return lipgloss.NewStyle().Foreground(t.warn)
	default:
		return lipgloss.NewStyle().Foreground(t.muted)
	}
}

// Styles bundles every rendered piece of the UI.
type Styles struct {
	Theme Theme

	App      lipgloss.Style // outer container
	Header   lipgloss.Style
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Footer   lipgloss.Style
	KeyHint  lipgloss.Style
	KeyKey   lipgloss.Style

	ListBox      lipgloss.Style
	ColumnHeader lipgloss.Style
	Row          lipgloss.Style
	RowSelected  lipgloss.Style
	RowName      lipgloss.Style
	RowMuted     lipgloss.Style

	Dialog    lipgloss.Style
	Option    lipgloss.Style
	OptionSel lipgloss.Style

	Toast lipgloss.Style

	Section lipgloss.Style
	Detail  lipgloss.Style
	Path    lipgloss.Style
	Hint    lipgloss.Style
}

// NewStyles builds the full style set for a theme.
func NewStyles(t Theme) Styles {
	s := Styles{Theme: t}

	s.App = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.text)

	s.Header = lipgloss.NewStyle().
		Background(t.accent).
		Foreground(t.bg).
		Bold(true).
		Padding(0, 2)

	s.Title = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.accent).
		Bold(true)

	s.Subtitle = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.muted)

	s.Footer = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.muted).
		Padding(0, 1)

	s.KeyHint = lipgloss.NewStyle().Background(t.bg).Foreground(t.muted)
	s.KeyKey = lipgloss.NewStyle().Background(t.bg).Foreground(t.accent).Bold(true)

	s.ListBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.border).
		Padding(0, 1)

	s.ColumnHeader = lipgloss.NewStyle().
		Foreground(t.muted).
		Bold(true).
		Underline(true)

	s.Row = lipgloss.NewStyle().Foreground(t.text)
	s.RowSelected = lipgloss.NewStyle().
		Background(t.bgSel).
		Foreground(t.text).
		Border(lipgloss.Border{Left: "▍"}, true).
		BorderForeground(t.accent)
	s.RowName = lipgloss.NewStyle().Foreground(t.text).Bold(true)
	s.RowMuted = lipgloss.NewStyle().Foreground(t.muted)

	s.Dialog = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.accent).
		Padding(1, 2).
		Width(64)

	s.Option = lipgloss.NewStyle().Foreground(t.text).Padding(0, 1)
	s.OptionSel = lipgloss.NewStyle().
		Background(t.accent).
		Foreground(t.bg).
		Bold(true).
		Padding(0, 1)

	s.Toast = lipgloss.NewStyle().
		Background(t.bgSel).
		Foreground(t.info).
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(t.accentDim).
		Padding(0, 1)

	s.Section = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	s.Detail = lipgloss.NewStyle().Foreground(t.text)
	s.Path = lipgloss.NewStyle().Foreground(t.muted)
	s.Hint = lipgloss.NewStyle().Foreground(t.muted).Italic(true)

	return s
}

// truncate cuts s to max runes, appending an ellipsis when truncated.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// pad truncates to width and pads to width.
func pad(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return truncate(s, width)
	}
	return s + string(make([]rune, width-len(runes)))
}
