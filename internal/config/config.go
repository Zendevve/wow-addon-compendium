// Package config persists user preferences in the platform-appropriate
// user config directory (os.UserConfigDir()/wowfix/config.json):
//
//	windows  %AppData%\wowfix\config.json
//	linux    ~/.config/wowfix/config.json
//	macos    ~/Library/Application Support/wowfix/config.json
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config is the persisted application state.
type Config struct {
	// Version of the config schema.
	Version int `json:"version"`
	// WoWPath is the detected/selected game root directory.
	WoWPath string `json:"wow_path,omitempty"`
	// Flavor selects the client subfolder when the install has several
	// ("_retail_", "_classic_", "_classic_era_", ...); empty means the
	// AddOns directory lives directly under Interface/.
	Flavor string `json:"flavor,omitempty"`
	// Profile is the active game-version profile ID.
	Profile string `json:"profile,omitempty"`
	// Theme is "dark" or "light".
	Theme string `json:"theme,omitempty"`
	// AutoBackup snapshots folders before every mutation.
	AutoBackup bool `json:"auto_backup"`
	// Confirmations enables prompt-before-mutate dialogs.
	Confirmations bool `json:"confirmations"`
	// LastScan records when the AddOns directory was last scanned.
	LastScan time.Time `json:"last_scan,omitempty"`
	// BackupsDir overrides the default backup location.
	BackupsDir string `json:"backups_dir,omitempty"`
}

// Default returns a fresh config with sane defaults.
func Default() *Config {
	return &Config{
		Version:       1,
		Profile:       "wrath",
		Theme:         "dark",
		AutoBackup:    true,
		Confirmations: true,
	}
}

// Store loads and saves the config.
type Store struct {
	path string
}

// NewStore returns a Store rooted at the user config directory.
func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("cannot locate user config directory: %w", err)
	}
	return &Store{path: filepath.Join(dir, "wowfix", "config.json")}, nil
}

// NewStoreAt returns a Store writing to an explicit path (used by tests).
func NewStoreAt(path string) *Store {
	return &Store{path: path}
}

// Path returns the config file location.
func (s *Store) Path() string { return s.path }

// Dir returns the config directory.
func (s *Store) Dir() string { return filepath.Dir(s.path) }

// Load reads the config; a missing file yields Default().
func (s *Store) Load() (*Config, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, fmt.Errorf("cannot read config %q: %w", s.path, err)
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config %q is corrupt: %w", s.path, err)
	}
	if cfg.Profile == "" {
		cfg.Profile = "wrath"
	}
	if cfg.Theme == "" {
		cfg.Theme = "dark"
	}
	return cfg, nil
}

// Save writes the config atomically (write temp, then rename).
func (s *Store) Save(cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
