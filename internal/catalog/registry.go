package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry records one installed addon in the registry.
type Entry struct {
	Folder      string    `json:"folder"`
	Title       string    `json:"title"`
	Version     string    `json:"version"`
	Provider    string    `json:"provider"`
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	// Checksum is the content digest of the installed folder recorded
	// at install/update time (see ComputeManifest). Empty for entries
	// installed before integrity tracking existed or when the manifest
	// could not be computed; this field is best-effort provenance.
	Checksum string `json:"checksum,omitempty"`
}

// Registry persists the set of addons installed through the catalog
// as a JSON file. Mutations are written atomically (temp file +
// rename).
type Registry struct {
	mu      sync.Mutex
	path    string
	entries []Entry
}

type registryFile struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// NewRegistry loads the registry at path. A missing file yields an
// empty registry; a corrupt one is an error.
func NewRegistry(path string) (*Registry, error) {
	r := &Registry{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("read registry %q: %w", path, err)
	}
	var f registryFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("registry %q is corrupt: %w", path, err)
	}
	r.entries = f.Entries
	return r, nil
}

// DefaultPath returns the conventional registry location
// (<user config dir>/wowfix/registry.json).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate user config directory: %w", err)
	}
	return filepath.Join(dir, "wowfix", "registry.json"), nil
}

// Track upserts an entry by folder (case-insensitive) and saves.
func (r *Registry) Track(e Entry) error {
	if strings.TrimSpace(e.Folder) == "" {
		return fmt.Errorf("registry: cannot track an entry without a folder")
	}
	if e.InstalledAt.IsZero() {
		e.InstalledAt = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if strings.EqualFold(r.entries[i].Folder, e.Folder) {
			r.entries[i] = e
			return r.saveLocked()
		}
	}
	r.entries = append(r.entries, e)
	return r.saveLocked()
}

// Entries returns a copy of the tracked entries, sorted by folder.
func (r *Registry) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Folder) < strings.ToLower(out[j].Folder)
	})
	return out
}

// Forget removes the entry for folder (case-insensitive) and saves.
// Removing an unknown folder is a no-op.
func (r *Registry) Forget(folder string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if strings.EqualFold(r.entries[i].Folder, folder) {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return r.saveLocked()
		}
	}
	return nil
}

// saveLocked writes the registry atomically. The caller holds mu.
func (r *Registry) saveLocked() error {
	if r.path == "" {
		return nil // in-memory registry
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("cannot create registry directory: %w", err)
	}
	f := registryFile{Version: 1, Entries: r.entries}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		// Windows cannot rename over an existing file; retry after
		// removing the destination.
		if _, statErr := os.Stat(r.path); statErr == nil {
			_ = os.Remove(r.path)
			if err := os.Rename(tmp, r.path); err == nil {
				return nil
			}
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("save registry %q: %w", r.path, err)
	}
	return nil
}
