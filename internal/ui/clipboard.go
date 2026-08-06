package ui

import (
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
)

// clipboardRead and clipboardWrite are the system-clipboard seams; tests
// swap them so no test ever touches the real clipboard.
var clipboardRead = clipboard.ReadAll
var clipboardWrite = clipboard.WriteAll

// clipboardPaste reads the system clipboard and splices it into the
// focused text input at its cursor.
func (a *App) clipboardPaste() (tea.Model, tea.Cmd) {
	text, err := clipboardRead()
	if err != nil {
		a.pushToast("Paste failed: " + err.Error())
		return a, nil
	}
	if text == "" {
		a.pushToast("Clipboard is empty")
		return a, nil
	}
	a.insertIntoFocusedInput(text)
	return a, nil
}

// clipboardCopy writes the focused text input's value to the system
// clipboard. With no focused input or an empty value it reports instead.
func (a *App) clipboardCopy() (tea.Model, tea.Cmd) {
	text := ""
	switch {
	case a.view == viewInput:
		text = a.input.Value()
	case a.filtering:
		text = a.filter.Value()
	case a.view == viewCatalog && a.search.Focused():
		text = a.search.Value()
	}
	if text == "" {
		a.pushToast("Nothing to copy")
		return a, nil
	}
	if err := clipboardWrite(text); err != nil {
		a.pushToast("Copy failed: " + err.Error())
		return a, nil
	}
	a.pushToast("Copied " + truncate(text, 40))
	return a, nil
}

// insertIntoFocusedInput splices s into the focused input at its cursor
// position. With no input focused it does nothing.
func (a *App) insertIntoFocusedInput(s string) {
	var m *textinput.Model
	switch {
	case a.view == viewInput:
		m = &a.input
	case a.filtering:
		m = &a.filter
	case a.view == viewCatalog && a.search.Focused():
		m = &a.search
	default:
		return
	}
	value := []rune(m.Value())
	pos := m.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(value) {
		pos = len(value)
	}
	merged := append(value[:pos], append([]rune(s), value[pos:]...)...)
	m.SetValue(string(merged))
	m.SetCursor(pos + len([]rune(s)))
}
