package ui

import (
	"strings"

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
	bgSubtle  lipgloss.Color
	bgSel     lipgloss.Color
	bgRowErr  lipgloss.Color
	border    lipgloss.Color
	borderDim lipgloss.Color
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
		bgSubtle:  lipgloss.Color("#141b27"),
		bgSel:     lipgloss.Color("#1c2434"),
		bgRowErr:  lipgloss.Color("#2a1a23"),
		border:    lipgloss.Color("#2c3a52"),
		borderDim: lipgloss.Color("#1f2a3c"),
		text:      lipgloss.Color("#e2e8f0"),
		muted:     lipgloss.Color("#94a3b8"),
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
		bgSubtle:  lipgloss.Color("#eef2f7"),
		bgSel:     lipgloss.Color("#e2e8f0"),
		bgRowErr:  lipgloss.Color("#fdecef"),
		border:    lipgloss.Color("#cbd5e1"),
		borderDim: lipgloss.Color("#dde4ee"),
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

	// chrome
	App      lipgloss.Style
	Header   lipgloss.Style
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Footer   lipgloss.Style
	KeyHint  lipgloss.Style
	KeyKey   lipgloss.Style

	// lists & tables
	ListBox     lipgloss.Style
	ListPanel   lipgloss.Style
	ColumnHdr   lipgloss.Style
	Row         lipgloss.Style
	RowSelected lipgloss.Style
	RowName     lipgloss.Style
	RowMuted    lipgloss.Style
	RowProblem  lipgloss.Style
	RowNameSel  lipgloss.Style
	RowError    lipgloss.Style
	StatusOK    lipgloss.Style
	StatusWarn  lipgloss.Style
	StatusErr   lipgloss.Style
	Badge       lipgloss.Style
	BadgeMuted  lipgloss.Style
	AccentChip  lipgloss.Style

	// dialogs / inputs
	Dialog     lipgloss.Style
	Option     lipgloss.Style
	OptionSel  lipgloss.Style
	Input      lipgloss.Style
	FilterBar  lipgloss.Style
	FilterHint lipgloss.Style

	// status
	ProgressFill  lipgloss.Style
	ProgressTrack lipgloss.Style

	// toasts
	ToastInfo lipgloss.Style
	ToastOK   lipgloss.Style
	ToastWarn lipgloss.Style
	ToastErr  lipgloss.Style
	ToastTime lipgloss.Style

	// misc
	Section  lipgloss.Style
	Detail   lipgloss.Style
	Path     lipgloss.Style
	Hint     lipgloss.Style
	Empty    lipgloss.Style
	EmptySub lipgloss.Style
	Rule     lipgloss.Style
	Summary  lipgloss.Style
	SummaryN lipgloss.Style
}

// NewStyles builds the full style set for a theme.
func NewStyles(t Theme) Styles {
	s := Styles{Theme: t}

	s.App = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.text)

	s.Header = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.text)

	s.Title = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.accent).
		Bold(true)

	s.Subtitle = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.muted)

	s.Footer = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.muted)

	s.KeyHint = lipgloss.NewStyle().Background(t.bg).Foreground(t.muted)
	s.KeyKey = lipgloss.NewStyle().Background(t.bg).Foreground(t.accent).Bold(true)

	s.ListBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.border).
		Background(t.bg).
		Foreground(t.text).
		Padding(0, 1)

	s.ListPanel = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.text)

	s.ColumnHdr = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.muted).
		Bold(true)

	s.Row = lipgloss.NewStyle().Background(t.bg).Foreground(t.text)
	s.RowSelected = lipgloss.NewStyle().
		Background(t.bgSel).
		Foreground(t.text).
		Bold(false)
	s.RowError = lipgloss.NewStyle().
		Background(t.bgRowErr).
		Foreground(t.text)
	s.RowName = lipgloss.NewStyle().Foreground(t.text).Bold(true)
	s.RowNameSel = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	s.RowMuted = lipgloss.NewStyle().Foreground(t.muted)
	s.RowProblem = lipgloss.NewStyle().Foreground(t.text)

	s.StatusOK = lipgloss.NewStyle().Foreground(t.ok)
	s.StatusWarn = lipgloss.NewStyle().Foreground(t.warn)
	s.StatusErr = lipgloss.NewStyle().Foreground(t.err)

	s.Badge = lipgloss.NewStyle().
		Foreground(t.bg).
		Background(t.accentDim).
		Bold(true)
	s.BadgeMuted = lipgloss.NewStyle().
		Foreground(t.muted).
		Background(t.bgSubtle)
	s.AccentChip = lipgloss.NewStyle().
		Foreground(t.accent).
		Background(t.bgSubtle).
		Bold(true)

	s.Dialog = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.accent).
		Background(t.bg).
		Foreground(t.text).
		Padding(1, 2)

	s.Option = lipgloss.NewStyle().
		Background(t.bg).
		Foreground(t.text).
		Padding(0, 1)
	s.OptionSel = lipgloss.NewStyle().
		Background(t.accent).
		Foreground(t.bg).
		Bold(true).
		Padding(0, 1)

	s.Input = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.accent).
		Background(t.bg).
		Foreground(t.text).
		Padding(0, 1)

	s.FilterBar = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(t.accentDim).
		Background(t.bg).
		Foreground(t.text).
		Padding(0, 1)
	s.FilterHint = lipgloss.NewStyle().Foreground(t.muted).Italic(true)

	s.ProgressFill = lipgloss.NewStyle().Foreground(t.accent)
	s.ProgressTrack = lipgloss.NewStyle().Foreground(t.muted)

	s.ToastInfo = lipgloss.NewStyle().
		Background(t.bgSubtle).
		Foreground(t.info).
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(t.border).
		Padding(0, 1)
	s.ToastOK = lipgloss.NewStyle().
		Background(t.bgSubtle).
		Foreground(t.ok).
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(t.border).
		Padding(0, 1)
	s.ToastWarn = lipgloss.NewStyle().
		Background(t.bgSubtle).
		Foreground(t.warn).
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(t.warn).
		Padding(0, 1)
	s.ToastErr = lipgloss.NewStyle().
		Background(t.bgRowErr).
		Foreground(t.err).
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(t.err).
		Padding(0, 1)
	s.ToastTime = lipgloss.NewStyle().Foreground(t.muted)

	s.Section = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	s.Detail = lipgloss.NewStyle().Foreground(t.text)
	s.Path = lipgloss.NewStyle().Foreground(t.muted)
	s.Hint = lipgloss.NewStyle().Foreground(t.muted).Italic(true)
	s.Empty = lipgloss.NewStyle().Foreground(t.text).Bold(true)
	s.EmptySub = lipgloss.NewStyle().Foreground(t.muted)
	s.Rule = lipgloss.NewStyle().Foreground(t.borderDim)
	s.Summary = lipgloss.NewStyle().Foreground(t.muted)
	s.SummaryN = lipgloss.NewStyle().Foreground(t.text).Bold(true)

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

// pad truncates to width and pads to width with spaces.
func pad(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return truncate(s, width)
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// padToVisibleWidth pads a possibly-styled string to a target visible
// (terminal cell) width. Use this instead of pad when the input may
// contain ANSI escape sequences, since pad counts raw runes and
// over-truncates styled strings.
func padToVisibleWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// padRight left-aligns and pads s to width on the right.
func padRight(s string, width int) string {
	return pad(s, width)
}

// flavorLabel maps a detector flavor slug to a short user-facing label.
func flavorLabel(flavor string) string {
	switch flavor {
	case "_retail_":
		return "Retail"
	case "_classic_":
		return "WotLK"
	case "_classic_era_":
		return "Era"
	case "_classic_tbc_":
		return "TBC"
	case "":
		return "Root"
	}
	return flavor
}

// flavorRootLabel is the "WotLK root" style label used in the header and
// picker when the flavor slugs need a friendlier name.
func flavorRootLabel(flavor string) string {
	l := flavorLabel(flavor)
	if l == "Root" {
		return l + " installation"
	}
	return l + " root"
}
