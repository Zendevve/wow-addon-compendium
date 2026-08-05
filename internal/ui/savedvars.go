package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wowfix/wowfix/internal/savedvars"
)

// svBackupMsg is delivered when a SavedVariables backup command finishes.
type svBackupMsg struct {
	path string
	err  error
}

// wtfRoot returns the WTF directory of the current installation.
func (a *App) wtfRoot() string {
	if a.install == nil {
		return ""
	}
	return filepath.Join(a.install.Root, a.install.Flavor, "WTF")
}

// loadSavedVars refreshes the account list and the current account's
// SavedVariables file stems.
func (a *App) loadSavedVars() {
	m := savedvars.New(a.wtfRoot(), a.log)
	a.svAccounts = m.Accounts()
	if len(a.svAccounts) == 0 {
		a.svAccount = ""
		a.svFiles = nil
		return
	}
	known := false
	for _, acct := range a.svAccounts {
		if acct == a.svAccount {
			known = true
			break
		}
	}
	if a.svAccount == "" || !known {
		a.svAccount = a.svAccounts[0]
	}
	a.refreshSavedVars()
}

// refreshSavedVars re-lists the current account's files.
func (a *App) refreshSavedVars() {
	if a.svAccount == "" {
		a.svFiles = nil
		return
	}
	m := savedvars.New(a.wtfRoot(), a.log)
	files, err := m.List(a.svAccount)
	if err != nil {
		a.svFiles = nil
		a.pushToast("SavedVariables: " + err.Error())
		return
	}
	a.svFiles = files
	a.clampSavedVars()
}

// clampSavedVars keeps the cursor inside the file list and scrolls.
func (a *App) clampSavedVars() {
	n := len(a.svFiles)
	if n == 0 {
		a.svCursor, a.svOffset = 0, 0
		return
	}
	if a.svCursor < 0 {
		a.svCursor = 0
	}
	if a.svCursor >= n {
		a.svCursor = n - 1
	}
	rows := a.visibleRows()
	if a.svCursor < a.svOffset {
		a.svOffset = a.svCursor
	}
	if a.svCursor >= a.svOffset+rows {
		a.svOffset = a.svCursor - rows + 1
	}
}

// svBackupCmd snapshots the current account's SavedVariables into
// <config dir>/savedvars-backups.
func (a *App) svBackupCmd() tea.Cmd {
	acct := a.svAccount
	dest := filepath.Join(a.store.Dir(), "savedvars-backups")
	return func() tea.Msg {
		m := savedvars.New(a.wtfRoot(), a.log)
		path, err := m.Backup(acct, dest)
		return svBackupMsg{path: path, err: err}
	}
}

func (a *App) updateSavedVarsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(a.svFiles)
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.view = viewList
		return a, nil

	case key.Matches(msg, a.keys.Up):
		a.svCursor = maxInt(0, a.svCursor-1)
		a.clampSavedVars()

	case key.Matches(msg, a.keys.Down):
		if n > 0 {
			a.svCursor = minInt(n-1, a.svCursor+1)
		}
		a.clampSavedVars()

	case key.Matches(msg, a.keys.Enter):
		// Cycle to the next account (if any) and refresh the list.
		if len(a.svAccounts) > 1 {
			next := 0
			for i, acct := range a.svAccounts {
				if acct == a.svAccount {
					next = (i + 1) % len(a.svAccounts)
					break
				}
			}
			a.svAccount = a.svAccounts[next]
			a.svCursor = 0
		}
		a.refreshSavedVars()
		return a, nil

	case key.Matches(msg, a.keys.Backup):
		if a.svAccount == "" {
			a.pushToast("No account with SavedVariables found")
			return a, nil
		}
		a.busy = true
		a.busyText = "Backing up SavedVariables…"
		return a, a.svBackupCmd()

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		if n == 0 {
			return a, nil
		}
		addon := a.svFiles[a.svCursor]
		a.openConfirm(fmt.Sprintf("Reset SavedVariables for %q?", addon),
			fmt.Sprintf("%s.lua is deleted from Account/%s/SavedVariables.", addon, a.svAccount),
			func() { a.resetSavedVar(addon) })
		return a, nil
	}
	return a, nil
}

// resetSavedVar deletes the selected SavedVariables file and refreshes.
func (a *App) resetSavedVar(addon string) {
	m := savedvars.New(a.wtfRoot(), a.log)
	if err := m.Reset(a.svAccount, addon); err != nil {
		a.pushToast("Reset failed: " + err.Error())
	} else {
		a.pushToast(fmt.Sprintf("Reset %s.lua", addon))
	}
	a.view = viewSavedVars
	a.refreshSavedVars()
}

func (a *App) renderSavedVars() string {
	width := a.width - 6
	if width < 60 {
		width = 60
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render("SavedVariables"))
	b.WriteString("\n")
	if a.svAccount != "" {
		b.WriteString(a.styles.Detail.Render("Account: ") +
			a.styles.Path.Render(a.svAccount) +
			a.styles.RowMuted.Render("  (enter cycles accounts)"))
	}
	b.WriteString("\n\n")
	if len(a.svAccounts) == 0 {
		b.WriteString(a.styles.Hint.Render("No accounts found under " + filepathDisplay(a.wtfRoot()) + "/Account"))
		b.WriteString("\n")
	} else if len(a.svFiles) == 0 {
		b.WriteString(a.styles.Hint.Render("No SavedVariables yet for account " + a.svAccount))
		b.WriteString("\n")
	} else {
		rows := a.visibleRows()
		end := a.svOffset + rows
		if end > len(a.svFiles) {
			end = len(a.svFiles)
		}
		for i := a.svOffset; i < end; i++ {
			mark := " "
			if i == a.svCursor {
				mark = "▸"
			}
			row := fmt.Sprintf("%s %s.lua", mark, a.svFiles[i])
			if i == a.svCursor {
				b.WriteString(a.styles.RowSelected.Render(pad(row, width)))
			} else {
				b.WriteString(a.styles.Row.Render(pad(row, width)))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" + a.styles.Hint.Render(
		"↑/↓ navigate · enter refresh/account · b backup · r reset · esc back"))
	return a.styles.ListBox.Width(width).Render(b.String())
}
