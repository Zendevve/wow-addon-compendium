//go:build !windows

package utils

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// nativeTrash implements the XDG Trash specification on Linux and
// moves to ~/.Trash on macOS. Files are relocated, never deleted.
func nativeTrash(path string) error {
	trashRoot, infoDir := trashLocations()
	if trashRoot == "" {
		return ErrTrashUnsupported
	}
	if !Exists(trashRoot) {
		if err := EnsureDir(trashRoot); err != nil {
			return ErrTrashUnsupported
		}
	}

	name := filepath.Base(path)
	// Avoid name collisions inside the trash.
	stamp := time.Now().Format("20060102-150405.000000")
	if Exists(filepath.Join(trashRoot, name)) {
		name = stamp + "-" + name
	}

	target := filepath.Join(trashRoot, name)
	if err := os.Rename(path, target); err != nil {
		return err
	}

	// Write the .trashinfo metadata file when the trash layout supports it.
	if infoDir != "" {
		infoPath := filepath.Join(infoDir, name+".trashinfo")
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		escaped := strings.NewReplacer("\n", "", "\r", "").Replace(abs)
		info := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
			escaped, time.Now().Format("2006-01-02T15:04:05"))
		_ = os.WriteFile(infoPath, []byte(info), 0o644)
	}
	return nil
}

// trashLocations returns (filesDir, infoDir). infoDir is empty when the
// platform trash has no metadata directory (macOS).
func trashLocations() (string, string) {
	if home, err := os.UserHomeDir(); err == nil {
		// macOS
		if _, err := os.Stat("/Applications"); err == nil {
			return filepath.Join(home, ".Trash"), ""
		}
		// XDG spec
		xdg := os.Getenv("XDG_DATA_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(xdg, "Trash", "files"), filepath.Join(xdg, "Trash", "info")
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return filepath.Join(u.HomeDir, ".local", "share", "Trash", "files"),
			filepath.Join(u.HomeDir, ".local", "share", "Trash", "info")
	}
	return "", ""
}
