// Package profiles manages addon collections: named sets of addon
// enable/disable state that can be applied to the AddOns directory.
//
// WoW disables an addon when its folder name ends in ".disabled", so
// applying a collection means renaming folders between "<name>" and
// "<name>.disabled". A collection is persisted as one JSON file
// (<id>.json) in the manager's directory.
package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/utils"
)

// disabledSuffix is the folder-name suffix WoW treats as "addon off".
const disabledSuffix = ".disabled"

// AddonState records the desired enable state of one addon folder.
type AddonState struct {
	// Folder is the folder name as installed, e.g. "Questie".
	Folder  string
	Enabled bool
}

// Collection is one named set of addon enable/disable state.
type Collection struct {
	ID        string
	Name      string
	Addons    []AddonState
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Manager loads, saves and applies collections.
type Manager struct {
	// Path is the directory holding one <id>.json per collection.
	Path string
	// AddonsDir is the Interface/AddOns directory collections apply to.
	AddonsDir string
	// Log, when set, receives human-readable action lines.
	Log *logger.Logger
	// Backups, when set, snapshots the AddOns directory before a
	// SwitchTo renames anything.
	Backups *backup.Manager
}

// NewManager returns a Manager rooted at path, creating the directory
// when missing.
func NewManager(path, addonsDir string) (*Manager, error) {
	if err := utils.EnsureDir(path); err != nil {
		return nil, fmt.Errorf("cannot create collections directory: %w", err)
	}
	return &Manager{Path: path, AddonsDir: addonsDir}, nil
}

// List returns every collection, sorted by name.
func (m *Manager) List() ([]Collection, error) {
	entries, err := os.ReadDir(m.Path)
	if err != nil {
		return nil, fmt.Errorf("cannot read collections directory: %w", err)
	}
	var cols []Collection
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		c, err := m.Get(id)
		if err != nil {
			return nil, err
		}
		cols = append(cols, *c)
	}
	sort.Slice(cols, func(i, j int) bool {
		return strings.ToLower(cols[i].Name) < strings.ToLower(cols[j].Name)
	})
	return cols, nil
}

// Get loads one collection by id.
func (m *Manager) Get(id string) (*Collection, error) {
	data, err := os.ReadFile(m.fileFor(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("collection %q not found", id)
		}
		return nil, fmt.Errorf("cannot read collection %q: %w", id, err)
	}
	c := &Collection{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("collection %q is corrupt: %w", id, err)
	}
	return c, nil
}

// Create snapshots the current on-disk state of AddonsDir into a new
// collection. Folders ending in ".disabled" are recorded as disabled.
func (m *Manager) Create(name string) (*Collection, error) {
	states, err := m.scanAddonsDir()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	c := &Collection{
		ID:        m.uniqueID(name),
		Name:      name,
		Addons:    states,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := m.save(c); err != nil {
		return nil, err
	}
	if m.Log != nil {
		m.Log.Infof("Collection %q (%s) created from current state (%d addon(s))", c.Name, c.ID, len(c.Addons))
	}
	return c, nil
}

// Duplicate copies the collection's addon state under a new name.
func (m *Manager) Duplicate(id, newName string) (*Collection, error) {
	src, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	states := make([]AddonState, len(src.Addons))
	copy(states, src.Addons)
	now := time.Now()
	c := &Collection{
		ID:        m.uniqueID(newName),
		Name:      newName,
		Addons:    states,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := m.save(c); err != nil {
		return nil, err
	}
	if m.Log != nil {
		m.Log.Infof("Collection %q (%s) duplicated from %q", c.Name, c.ID, src.ID)
	}
	return c, nil
}

// Rename changes a collection's display name (the id stays stable).
func (m *Manager) Rename(id, newName string) error {
	c, err := m.Get(id)
	if err != nil {
		return err
	}
	c.Name = newName
	c.UpdatedAt = time.Now()
	if err := m.save(c); err != nil {
		return err
	}
	if m.Log != nil {
		m.Log.Infof("Collection %q renamed to %q", id, newName)
	}
	return nil
}

// Delete removes a collection file. Installed addons are untouched.
func (m *Manager) Delete(id string) error {
	path := m.fileFor(id)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("collection %q not found", id)
		}
		return fmt.Errorf("cannot delete collection %q: %w", id, err)
	}
	if m.Log != nil {
		m.Log.Infof("Collection %q deleted", id)
	}
	return nil
}

// SetEnabled toggles one addon's desired state in the collection.
// Unknown folders are appended.
func (m *Manager) SetEnabled(id, folder string, enabled bool) error {
	c, err := m.Get(id)
	if err != nil {
		return err
	}
	found := false
	for i := range c.Addons {
		if strings.EqualFold(c.Addons[i].Folder, folder) {
			c.Addons[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		c.Addons = append(c.Addons, AddonState{Folder: folder, Enabled: enabled})
	}
	c.UpdatedAt = time.Now()
	return m.save(c)
}

// SwitchTo applies the collection's state on disk: every addon's folder
// is renamed to "<folder>" when enabled or "<folder>.disabled" when not.
// When m.Backups is set, the current AddonsDir state is snapshotted
// first. It returns the base folder names that were renamed.
func (m *Manager) SwitchTo(id string) (applied []string, err error) {
	c, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if m.AddonsDir == "" {
		return nil, fmt.Errorf("no AddonsDir configured")
	}
	if m.Backups != nil {
		if _, err := m.Backups.BackupDir(m.AddonsDir, "profile switch to "+c.Name); err != nil {
			// A failed snapshot must not block the switch; the
			// collection itself still records the previous state.
			if m.Log != nil {
				m.Log.Warn(fmt.Sprintf("Pre-switch backup skipped: %v", err))
			}
		}
	}

	for _, st := range c.Addons {
		if st.Folder == "" {
			continue
		}
		base := filepath.Join(m.AddonsDir, st.Folder)
		disabled := base + disabledSuffix
		var src, dst string
		if st.Enabled {
			src, dst = disabled, base
		} else {
			src, dst = base, disabled
		}
		if !utils.Exists(src) {
			continue // addon not installed; nothing to rename
		}
		if utils.Exists(dst) {
			return applied, fmt.Errorf("cannot %s %q: %q already exists (would overwrite)",
				map[bool]string{true: "enable", false: "disable"}[st.Enabled], st.Folder, filepath.Base(dst))
		}
		if err := utils.SafeRename(src, dst); err != nil {
			return applied, fmt.Errorf("cannot %s %q: %w",
				map[bool]string{true: "enable", false: "disable"}[st.Enabled], st.Folder, err)
		}
		applied = append(applied, st.Folder)
		if m.Log != nil {
			m.Log.Infof("Profile switch %s: %s -> %s", c.Name, filepath.Base(src), filepath.Base(dst))
		}
	}
	return applied, nil
}

// scanAddonsDir reads the current AddOns directory into AddonState
// entries: folders ending in ".disabled" are disabled, everything else
// is enabled. Non-directories and dotfiles are ignored.
func (m *Manager) scanAddonsDir() ([]AddonState, error) {
	entries, err := os.ReadDir(m.AddonsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read AddOns directory %q: %w", m.AddonsDir, err)
	}
	var states []AddonState
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), disabledSuffix) {
			states = append(states, AddonState{Folder: strings.TrimSuffix(name, disabledSuffix), Enabled: false})
		} else {
			states = append(states, AddonState{Folder: name, Enabled: true})
		}
	}
	sort.Slice(states, func(i, j int) bool {
		return strings.ToLower(states[i].Folder) < strings.ToLower(states[j].Folder)
	})
	return states, nil
}

// uniqueID derives a filesystem-safe id from name, appending -2, -3, ...
// until it collides with no existing collection file.
func (m *Manager) uniqueID(name string) string {
	base := utils.CleanName(name)
	if base == "" {
		base = "collection"
	}
	id := base
	for n := 2; utils.Exists(m.fileFor(id)); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// fileFor returns the storage path of one collection.
func (m *Manager) fileFor(id string) string {
	return filepath.Join(m.Path, id+".json")
}

// save writes a collection atomically (temp file + rename).
func (m *Manager) save(c *Collection) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := m.fileFor(c.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write collection %q: %w", c.ID, err)
	}
	return nil
}
