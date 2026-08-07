// Package backup implements the pre-change snapshot system. Before any
// fix or install touches an addon, the original folder is copied into
// Backups/<timestamp>/ and can be restored later.
package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/utils"
)

// Manifest records what a backup snapshot contains and where each
// folder originally lived, so restore can place them back.
type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason"`
	Entries   []Entry   `json:"entries"`
}

// Entry is one backed-up folder.
type Entry struct {
	// Name is the folder name inside the snapshot.
	Name string `json:"name"`
	// OriginalPath is where the folder was before the backup.
	OriginalPath string `json:"original_path"`
}

// Info describes one snapshot on disk.
type Info struct {
	ID        string // timestamp directory name
	Path      string // snapshot directory
	CreatedAt time.Time
	Reason    string
	Size      int64
}

// Manager creates and lists backups.
type Manager struct {
	// Root is the Backups directory (created on demand).
	Root string
	Log  *logger.Logger
}

// New returns a Manager rooted at dir.
func New(dir string, log *logger.Logger) *Manager {
	return &Manager{Root: dir, Log: log}
}

// Backup copies the given source folders into a fresh timestamped
// snapshot and writes a manifest. reason is logged and stored.
func (m *Manager) Backup(sources []string, reason string) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("nothing to back up")
	}
	if m.Root == "" {
		return "", fmt.Errorf("backup directory is not configured")
	}
	id := time.Now().Format("2006-01-02T15-04-05.000")
	dir := filepath.Join(m.Root, id)
	// Collisions (several backups inside one millisecond) get a
	// numeric suffix so no snapshot is ever overwritten.
	for n := 2; utils.Exists(dir); n++ {
		dir = filepath.Join(m.Root, fmt.Sprintf("%s-%d", id, n))
	}
	if err := utils.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("cannot create backup directory: %w", err)
	}

	manifest := Manifest{
		Version:   1,
		CreatedAt: time.Now(),
		Reason:    reason,
	}

	for _, src := range sources {
		name := filepath.Base(src)
		dst := filepath.Join(dir, name)
		if utils.Exists(dst) {
			dst = filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))
		}
		if err := utils.CopyDir(src, dst); err != nil {
			return "", fmt.Errorf("backup of %q failed: %w", src, err)
		}
		manifest.Entries = append(manifest.Entries, Entry{Name: filepath.Base(dst), OriginalPath: src})
	}

	if err := writeManifest(dir, manifest); err != nil {
		return "", err
	}
	if m.Log != nil {
		m.Log.Infof("Backup created: %s (%d folder(s), reason: %s)", dir, len(sources), reason)
	}
	return dir, nil
}

// BackupDir backs up every subdirectory of addonsDir as one snapshot.
func (m *Manager) BackupDir(addonsDir string, reason string) (string, error) {
	entries, err := os.ReadDir(addonsDir)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, filepath.Join(addonsDir, e.Name()))
		}
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no addon folders to back up")
	}
	return m.Backup(dirs, reason)
}

// List returns snapshots newest-first.
func (m *Manager) List() ([]Info, error) {
	if !utils.IsDir(m.Root) {
		return nil, nil
	}
	entries, err := os.ReadDir(m.Root)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(m.Root, e.Name())
		info := Info{ID: e.Name(), Path: dir}
		for _, layout := range []string{"2006-01-02T15-04-05.000", "2006-01-02T15-04-05"} {
			if t, err := time.Parse(layout, e.Name()); err == nil {
				info.CreatedAt = t
				break
			}
		}
		if size, err := utils.DirSize(dir); err == nil {
			info.Size = size
		}
		if mf, err := readManifest(dir); err == nil {
			info.Reason = mf.Reason
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Restore copies every folder of a snapshot back to its original
// location. The current state of each destination is backed up first so
// a restore is never destructive. confirm is invoked per destination
// when it already exists; returning false skips that entry.
func (m *Manager) Restore(id string, confirm func(originalPath string) bool) (restored []string, skipped []string, err error) {
	dir := filepath.Join(m.Root, id)
	mf, err := readManifest(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot restore %q: %w", id, err)
	}

	// Snapshot the current state of every destination before touching it.
	var current []string
	for _, e := range mf.Entries {
		if utils.Exists(e.OriginalPath) {
			current = append(current, e.OriginalPath)
		}
	}
	if len(current) > 0 {
		if _, err := m.Backup(current, "pre-restore snapshot of "+id); err != nil {
			return nil, nil, fmt.Errorf("pre-restore backup failed: %w", err)
		}
	}

	for _, e := range mf.Entries {
		src := filepath.Join(dir, e.Name)
		if !utils.Exists(src) {
			skipped = append(skipped, e.OriginalPath)
			continue
		}
		if utils.Exists(e.OriginalPath) {
			if confirm != nil && !confirm(e.OriginalPath) {
				skipped = append(skipped, e.OriginalPath)
				continue
			}
			if err := os.RemoveAll(e.OriginalPath); err != nil {
				return restored, skipped, fmt.Errorf("cannot remove %q: %w", e.OriginalPath, err)
			}
		}
		if err := utils.CopyDir(src, e.OriginalPath); err != nil {
			return restored, skipped, fmt.Errorf("restore of %q failed: %w", e.OriginalPath, err)
		}
		restored = append(restored, e.OriginalPath)
	}
	if m.Log != nil {
		m.Log.Infof("Restore completed from %s: %d folder(s)", id, len(restored))
	}
	return restored, skipped, nil
}

// RollbackFolder restores the folder at originalPath from the newest
// snapshot that contains it (matched case-insensitively on the path)
// and returns the snapshot id, the base name of the snapshot
// directory. The current state of the destination is snapshotted
// first, so a rollback is never destructive, mirroring Restore. No
// snapshot containing the folder is an error.
func (m *Manager) RollbackFolder(originalPath string) (restoredFrom string, err error) {
	snapshots, err := m.List()
	if err != nil {
		return "", err
	}
	for _, snap := range snapshots { // newest first
		mf, err := readManifest(snap.Path)
		if err != nil {
			continue // snapshot without a manifest is not restorable
		}
		for _, e := range mf.Entries {
			if !strings.EqualFold(e.OriginalPath, originalPath) {
				continue
			}
			src := filepath.Join(snap.Path, e.Name)
			if !utils.Exists(src) {
				return "", fmt.Errorf("rollback of %q: snapshot %s is missing %s", originalPath, snap.ID, e.Name)
			}
			if utils.Exists(originalPath) {
				if _, err := m.Backup([]string{originalPath}, "pre-rollback of "+filepath.Base(originalPath)); err != nil {
					return "", fmt.Errorf("pre-rollback backup failed: %w", err)
				}
			}
			if err := os.RemoveAll(originalPath); err != nil {
				return "", fmt.Errorf("cannot remove %q: %w", originalPath, err)
			}
			if err := utils.CopyDir(src, originalPath); err != nil {
				return "", fmt.Errorf("rollback of %q failed: %w", originalPath, err)
			}
			if m.Log != nil {
				m.Log.Infof("Rollback of %s from snapshot %s", originalPath, snap.ID)
			}
			return snap.ID, nil
		}
	}
	return "", fmt.Errorf("no backup snapshot contains %q", originalPath)
}

func writeManifest(dir string, mf Manifest) error {
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)
}

func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot has no manifest")
		}
		return nil, err
	}
	var mf Manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}
