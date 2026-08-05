package ui

import (
	"fmt"
	"strings"
)

// SpinFrames is the default animation for indeterminate progress.
var SpinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Progress renders a determinate progress bar or an indeterminate spinner
// line, themed by Styles. For indeterminate mode the caller advances Frame
// on each tick; Percent is ignored.
type Progress struct {
	Styles        Styles
	Percent       float64 // completion ratio 0..1 (determinate mode)
	Indeterminate bool
	Frame         int
	Label         string // trailing label (e.g. the addon being downloaded)
	Width         int    // bar width in cells; 0 means the default 20
}

// NewProgress builds a determinate progress bar in the given theme.
func NewProgress(st Styles) Progress {
	return Progress{Styles: st, Width: 20}
}

// View renders the progress indicator.
func (p Progress) View() string {
	if p.Indeterminate {
		frame := SpinFrames[p.Frame%len(SpinFrames)]
		line := p.Styles.ProgressFill.Render(frame) + " " + p.Label
		return strings.TrimSpace(line)
	}
	width := p.Width
	if width <= 0 {
		width = 20
	}
	percent := p.Percent
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	filled := int(percent * float64(width))
	bar := p.Styles.ProgressFill.Render(strings.Repeat("█", filled)) +
		p.Styles.ProgressTrack.Render(strings.Repeat("░", width-filled))
	line := "[" + bar + "] " + fmt.Sprintf("%3.0f%%", percent*100)
	if p.Label != "" {
		line += " " + p.Label
	}
	return line
}
