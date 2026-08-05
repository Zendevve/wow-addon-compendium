package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/profiles"
)

// collectionsDir resolves where collection files live: the config
// override, else <config dir>/collections.
func (a *App) collectionsDir() string {
	if a.cfg.CollectionsDir != "" {
		return a.cfg.CollectionsDir
	}
	return filepath.Join(a.store.Dir(), "collections")
}

// profilesManager builds a manager bound to the current installation
// and collections dir, with pre-switch backups when enabled.
func (a *App) profilesManager() (*profiles.Manager, error) {
	addonsDir := ""
	if a.install != nil {
		addonsDir = a.install.AddonsPath
	}
	m, err := profiles.NewManager(a.collectionsDir(), addonsDir)
	if err != nil {
		return nil, err
	}
	m.Log = a.log
	if a.cfg.AutoBackup && a.install != nil {
		root, err := a.backupRoot()
		if err != nil {
			root = filepath.Join(a.store.Dir(), "backups")
		}
		m.Backups = backup.New(root, a.log)
	}
	return m, nil
}

// loadProfiles refreshes the collections list from disk.
func (a *App) loadProfiles() {
	m, err := a.profilesManager()
	if err != nil {
		a.profiles = nil
		a.pushToast("Collections unavailable: " + err.Error())
		return
	}
	cols, err := m.List()
	if err != nil {
		a.profiles = nil
		a.pushToast("Cannot list collections: " + err.Error())
		return
	}
	a.profiles = cols
	a.clampProfiles()
}

// clampProfiles keeps the cursor inside the list and scrolls the offset.
func (a *App) clampProfiles() {
	n := len(a.profiles)
	if n == 0 {
		a.profilesCursor, a.profilesOffset = 0, 0
		return
	}
	if a.profilesCursor < 0 {
		a.profilesCursor = 0
	}
	if a.profilesCursor >= n {
		a.profilesCursor = n - 1
	}
	rows := a.visibleRows()
	if a.profilesCursor < a.profilesOffset {
		a.profilesOffset = a.profilesCursor
	}
	if a.profilesCursor >= a.profilesOffset+rows {
		a.profilesOffset = a.profilesCursor - rows + 1
	}
}

// openProfilePrompt opens the shared text input for a collection
// operation; id is the target collection for duplicate/rename.
func (a *App) openProfilePrompt(kind inputKind, id string) {
	a.inputProfileID = id
	a.inputMode = kind
	a.input.Reset()
	switch kind {
	case inputProfileCreate:
		a.input.Placeholder = "new collection name"
	case inputProfileDuplicate:
		a.input.Placeholder = "name for the duplicate"
	case inputProfileRename:
		a.input.Placeholder = "new collection name"
	}
	a.input.Focus()
	a.view = viewInput
}

// applyProfileInput runs the collection operation the input prompt was
// asking for and reports the outcome via toast.
func (a *App) applyProfileInput(val string) error {
	m, err := a.profilesManager()
	if err != nil {
		return err
	}
	switch a.inputMode {
	case inputProfileCreate:
		c, err := m.Create(val)
		if err == nil {
			a.pushToast(fmt.Sprintf("Created collection %q (%d addon(s))", c.Name, len(c.Addons)))
		}
		return err
	case inputProfileDuplicate:
		c, err := m.Duplicate(a.inputProfileID, val)
		if err == nil {
			a.pushToast(fmt.Sprintf("Duplicated %q as %q", a.inputProfileID, c.Name))
		}
		return err
	case inputProfileRename:
		if err := m.Rename(a.inputProfileID, val); err != nil {
			return err
		}
		a.pushToast(fmt.Sprintf("Renamed collection to %q", val))
		return nil
	}
	return fmt.Errorf("unknown profile input mode")
}

func (a *App) updateProfilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(a.profiles)
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.view = viewList
		return a, nil

	case key.Matches(msg, a.keys.Up):
		a.profilesCursor = maxInt(0, a.profilesCursor-1)
		a.clampProfiles()

	case key.Matches(msg, a.keys.Down):
		if n > 0 {
			a.profilesCursor = minInt(n-1, a.profilesCursor+1)
		}
		a.clampProfiles()

	case key.Matches(msg, a.keys.Enter):
		if n == 0 {
			return a, nil
		}
		c := a.profiles[a.profilesCursor]
		a.openConfirm(fmt.Sprintf("Switch to collection %q?", c.Name),
			fmt.Sprintf("Renames %d addon folder(s) between <name> and <name>.disabled. A backup is created first.",
				len(c.Addons)),
			func() { a.switchProfile(c.ID) })
		return a, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
		a.openProfilePrompt(inputProfileCreate, "")
		return a, textinput.Blink

	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		if n == 0 {
			return a, nil
		}
		a.openProfilePrompt(inputProfileDuplicate, a.profiles[a.profilesCursor].ID)
		return a, textinput.Blink

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		if n == 0 {
			return a, nil
		}
		a.openProfilePrompt(inputProfileRename, a.profiles[a.profilesCursor].ID)
		return a, textinput.Blink

	case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
		if n == 0 {
			return a, nil
		}
		c := a.profiles[a.profilesCursor]
		a.openConfirm(fmt.Sprintf("Delete collection %q?", c.Name),
			"Only the collection file is removed; installed addons are untouched.",
			func() { a.deleteProfile(c.ID) })
		return a, nil
	}
	return a, nil
}

// switchProfile applies a collection on disk, records it as the active
// collection and rescans the AddOns directory.
func (a *App) switchProfile(id string) {
	m, err := a.profilesManager()
	if err != nil {
		a.pushToast("Switch failed: " + err.Error())
		a.view = viewProfiles
		return
	}
	applied, err := m.SwitchTo(id)
	if err != nil {
		a.pushToast("Switch failed: " + err.Error())
		a.view = viewProfiles
		a.loadProfiles()
		return
	}
	a.cfg.Collection = id
	a.save()
	a.pushToast(fmt.Sprintf("Switched collection: %d folder(s) renamed", len(applied)))
	a.busy = true
	a.busyText = "Rescanning…"
	a.cmd = a.scanCmd(a.install.Root, a.install.Flavor)
}

// deleteProfile removes a collection file and refreshes the list.
func (a *App) deleteProfile(id string) {
	m, err := a.profilesManager()
	if err != nil {
		a.pushToast("Delete failed: " + err.Error())
		a.view = viewProfiles
		return
	}
	if err := m.Delete(id); err != nil {
		a.pushToast("Delete failed: " + err.Error())
		a.view = viewProfiles
		return
	}
	a.pushToast("Deleted collection " + id)
	a.view = viewProfiles
	a.loadProfiles()
	if a.cfg.Collection == id {
		a.cfg.Collection = ""
		a.save()
	}
}

func (a *App) renderProfiles() string {
	width := a.width - 6
	if width < 60 {
		width = 60
	}
	var b strings.Builder
	b.WriteString(a.styles.Section.Render("Addon collections"))
	b.WriteString("\n\n")
	if len(a.profiles) == 0 {
		b.WriteString(a.styles.Hint.Render(
			"No collections yet — press n to capture the current addon setup."))
		b.WriteString("\n")
	} else {
		rows := a.visibleRows()
		end := a.profilesOffset + rows
		if end > len(a.profiles) {
			end = len(a.profiles)
		}
		for i := a.profilesOffset; i < end; i++ {
			c := a.profiles[i]
			mark := " "
			if i == a.profilesCursor {
				mark = "▸"
			}
			active := ""
			if a.cfg.Collection == c.ID {
				active = "  (active)"
			}
			row := fmt.Sprintf("%s %-30s %3d addon(s)%s", mark,
				truncate(c.Name, 30), len(c.Addons), active)
			if i == a.profilesCursor {
				b.WriteString(a.styles.RowSelected.Render(pad(row, width)))
			} else {
				b.WriteString(a.styles.Row.Render(pad(row, width)))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" + a.styles.Hint.Render(
		"↑/↓ navigate · enter switch · n create · d duplicate · r rename · x delete · esc back"))
	return a.styles.ListBox.Width(width).Render(b.String())
}
