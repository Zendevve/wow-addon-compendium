// Package savedvars manages the per-account SavedVariables files that
// WoW stores under <wtfRoot>/Account/<ACCOUNT>/SavedVariables/*.lua.
//
// The wtfRoot is derived from the installation by the caller (e.g.
// <root>/WTF or <root>/_retail_/WTF). Every derived path is validated
// to stay inside Root so untrusted account/addon names can never
// traverse out of the WTF directory.
package savedvars

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/utils"
)

// Manager locates and mutates SavedVariables files.
type Manager struct {
	// Root is the WTF directory holding Account/.
	Root string
	// Log, when set, receives human-readable action lines.
	Log *logger.Logger
}

// New returns a Manager rooted at the given WTF directory.
func New(root string, log *logger.Logger) *Manager {
	return &Manager{Root: root, Log: log}
}

// Accounts returns the account directory names under Root/Account,
// sorted. Directories that are not folders are skipped.
func (m *Manager) Accounts() []string {
	entries, err := os.ReadDir(filepath.Join(m.Root, "Account"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// List returns the sorted *.lua stems (file name without extension)
// under Root/Account/<account>/SavedVariables.
func (m *Manager) List(account string) ([]string, error) {
	dir, err := m.savedVarsDir(account)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read SavedVariables directory %q: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".lua") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, filepath.Ext(name)))
	}
	sort.Strings(out)
	return out, nil
}

// Backup copies the account's SavedVariables directory to
// dest/<timestamp>/ (a numeric suffix avoids collisions) and returns
// the created path.
func (m *Manager) Backup(account, dest string) (string, error) {
	dir, err := m.savedVarsDir(account)
	if err != nil {
		return "", err
	}
	if !utils.IsDir(dir) {
		return "", fmt.Errorf("no SavedVariables directory for account %q (%s)", account, dir)
	}
	id := time.Now().Format("2006-01-02T15-04-05.000")
	target := filepath.Join(dest, id)
	for n := 2; utils.Exists(target); n++ {
		target = filepath.Join(dest, fmt.Sprintf("%s-%d", id, n))
	}
	if err := utils.CopyDir(dir, target); err != nil {
		return "", fmt.Errorf("cannot back up SavedVariables: %w", err)
	}
	if m.Log != nil {
		m.Log.Infof("SavedVariables backed up: %s", target)
	}
	return target, nil
}

// Restore replaces the account's SavedVariables directory with the
// contents of backupPath. The current state is snapshotted first into
// <backupPath parent>/<timestamp>-prerestore so a restore is never
// destructive. backupPath must resolve inside Root.
func (m *Manager) Restore(account, backupPath string) error {
	backupPath, err := m.underRoot(backupPath)
	if err != nil {
		return err
	}
	if !utils.IsDir(backupPath) {
		return fmt.Errorf("backup path %q is not a directory", backupPath)
	}
	dir, err := m.savedVarsDir(account)
	if err != nil {
		return err
	}

	// Pre-restore snapshot of the current state, kept next to the
	// backup being restored.
	if utils.IsDir(dir) {
		id := time.Now().Format("2006-01-02T15-04-05.000")
		pre := filepath.Join(filepath.Dir(backupPath), id+"-prerestore")
		if err := utils.CopyDir(dir, pre); err != nil {
			return fmt.Errorf("pre-restore snapshot failed: %w", err)
		}
		if m.Log != nil {
			m.Log.Infof("Pre-restore snapshot: %s", pre)
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("cannot clear existing SavedVariables: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := utils.CopyDir(backupPath, dir); err != nil {
		return fmt.Errorf("cannot restore SavedVariables: %w", err)
	}
	if m.Log != nil {
		m.Log.Infof("SavedVariables restored from %s", backupPath)
	}
	return nil
}

// Reset deletes SavedVariables/<stem>.lua where stem equals addon
// case-insensitively. The match is on the exact file stem, so "DBM"
// never deletes "DBM-Core.lua". Nothing outside Root is ever touched.
func (m *Manager) Reset(account, addon string) error {
	dir, err := m.savedVarsDir(account)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no SavedVariables directory for account %q", account)
		}
		return fmt.Errorf("cannot read SavedVariables directory: %w", err)
	}
	var removed []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".lua") {
			continue
		}
		if !strings.EqualFold(strings.TrimSuffix(name, filepath.Ext(name)), addon) {
			continue
		}
		path, err := m.underRoot(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("cannot remove %q: %w", path, err)
		}
		removed = append(removed, name)
	}
	if len(removed) == 0 {
		return fmt.Errorf("no SavedVariables file for addon %q in account %q", addon, account)
	}
	if m.Log != nil {
		m.Log.Infof("Reset SavedVariables: %s", strings.Join(removed, ", "))
	}
	return nil
}

// savedVarsDir validates the account name and returns the account's
// SavedVariables directory. Account names must resolve inside
// Root/Account.
func (m *Manager) savedVarsDir(account string) (string, error) {
	if account == "" {
		return "", fmt.Errorf("no account given")
	}
	acct := filepath.Clean(account)
	acctRoot := filepath.Clean(filepath.Join(m.Root, "Account"))
	if filepath.IsAbs(acct) || acct == ".." || strings.HasPrefix(acct, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid account name %q", account)
	}
	dir := filepath.Join(acctRoot, acct)
	if !strings.HasPrefix(dir, acctRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid account name %q", account)
	}
	return filepath.Join(dir, "SavedVariables"), nil
}

// underRoot cleans p and refuses anything that resolves outside Root.
func (m *Manager) underRoot(p string) (string, error) {
	clean := filepath.Clean(p)
	root := filepath.Clean(m.Root)
	if clean != root && !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the WTF directory %q", p, m.Root)
	}
	return clean, nil
}
