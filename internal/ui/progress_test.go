package ui

import (
	"strings"
	"testing"
)

func TestProgressDeterminate(t *testing.T) {
	st := NewStyles(Dark())
	p := NewProgress(st)
	p.Percent = 0.5
	v := p.View()
	if !strings.Contains(v, "50%") {
		t.Errorf("View() = %q, want 50%%", v)
	}
	if !strings.HasPrefix(v, "[") || !strings.Contains(v, "]") {
		t.Errorf("View() = %q, want bracketed bar", v)
	}
	p.Percent = 1
	if !strings.Contains(p.View(), "100%") {
		t.Errorf("View() = %q, want 100%%", p.View())
	}
}

func TestProgressClampsPercent(t *testing.T) {
	st := NewStyles(Dark())
	p := NewProgress(st)
	p.Percent = 1.7
	if !strings.Contains(p.View(), "100%") {
		t.Errorf("over 1: View() = %q, want 100%%", p.View())
	}
	p.Percent = -0.4
	if !strings.Contains(p.View(), "0%") {
		t.Errorf("under 0: View() = %q, want 0%%", p.View())
	}
}

func TestProgressIndeterminate(t *testing.T) {
	st := NewStyles(Dark())
	p := NewProgress(st)
	p.Indeterminate = true
	p.Label = "downloading"
	v0 := p.View()
	p.Frame++
	v1 := p.View()
	if v0 == v1 {
		t.Errorf("adjacent frames render identically: %q", v0)
	}
	if !strings.Contains(v1, "downloading") {
		t.Errorf("View() = %q, want label", v1)
	}
	if !strings.Contains(v1, "⠙") {
		t.Errorf("View() = %q, want second spinner frame", v1)
	}
}

func TestProgressLabel(t *testing.T) {
	st := NewStyles(Dark())
	p := NewProgress(st)
	p.Percent = 0.25
	p.Label = "Questie.zip"
	if !strings.Contains(p.View(), "Questie.zip") {
		t.Errorf("View() = %q, want label", p.View())
	}
}
