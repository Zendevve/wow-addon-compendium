package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrTrashUnsupported is returned by platform trash implementations when
// no native trash facility is available; callers should fall back to
// FallbackMove.
var ErrTrashUnsupported = errors.New("no native trash facility available")

// Trash moves a file or directory to the platform trash (Recycle Bin on
// Windows, XDG Trash on Linux, ~/.Trash on macOS). When the native
// trash fails, it falls back to a timestamped move under fallbackDir so
// data is never silently dropped. The returned error names the failure
// of each stage for diagnostics.
func Trash(path, fallbackDir string) error {
	if err := nativeTrash(path); err == nil {
		return nil
	}
	if err := FallbackMove(path, fallbackDir); err == nil {
		return nil
	}
	return fmt.Errorf("trash failed for %q: native trash unavailable and fallback move failed", path)
}

// FallbackMove relocates path into fallbackDir/trash/<timestamp>-<name>.
// It never deletes data permanently. Cross-device moves (rename fails,
// e.g. ERROR_NOT_SAME_DEVICE) are handled by copying and then removing
// the source.
func FallbackMove(path, fallbackDir string) error {
	if fallbackDir == "" {
		fallbackDir = "."
	}
	dir := filepath.Join(fallbackDir, "trash")
	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("cannot create trash dir %q: %w", dir, err)
	}
	ts := time.Now().Format("20060102-150405.000")
	dst := filepath.Join(dir, ts+"-"+filepath.Base(path))

	if err := os.Rename(path, dst); err == nil {
		return nil
	}
	// Rename failed (typically a cross-device move): copy instead.
	if err := CopyDir(path, dst); err != nil {
		return fmt.Errorf("move to trash failed for %q: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("copied %q to trash but could not remove source: %w", path, err)
	}
	return nil
}
