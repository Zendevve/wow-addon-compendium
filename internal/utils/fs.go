// Package utils contains small cross-platform helpers shared by the
// other packages: filesystem operations, PE version reading and
// platform-aware trash support.
package utils

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureDir creates dir (and parents) if missing.
func EnsureDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// IsDir reports whether path exists and is a directory.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Exists reports whether path exists on disk.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsWritable reports whether dir can be written to by creating and
// removing a probe file. Errors are returned instead of guessed.
func IsWritable(dir string) error {
	probe := filepath.Join(dir, ".wowfix-write-probe")
	f, err := os.Create(probe)
	if err != nil {
		return err
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return nil
}

// CopyFile copies a single file, preserving permissions.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// CopyDir recursively copies src into dst. Symlinks are recreated as
// symlinks where possible. Missing parent directories are created.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := EnsureDir(filepath.Dir(target)); err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		default:
			return CopyFile(path, target)
		}
	})
}

// DirSize computes the recursive size of a directory in bytes.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// StripGitRef removes common Git ref suffixes from a folder name:
//
//	Questie-main    -> Questie
//	Questie-master  -> Questie
//	Questie-1.2.3   -> Questie
//	questie-main    -> questie
func StripGitRef(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{"-master", "-main", "-develop", "-dev"} {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	// Version-ish suffixes: -1.2.3, -v1.2.3, _1.2.3
	if i := lastDashIndex(name); i > 0 {
		rest := name[i+1:]
		if looksLikeVersion(rest) {
			return name[:i]
		}
	}
	return name
}

func lastDashIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' || s[i] == '_' {
			return i
		}
	}
	return -1
}

func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// SafeRename renames src to dst, resolving case-only renames and
// destination collisions through a backup suffix.
func SafeRename(src, dst string) error {
	if src == dst {
		return nil
	}
	if Exists(dst) {
		// Case-only rename on case-insensitive filesystems must go
		// through a temporary name.
		if strings.EqualFold(filepath.Base(src), filepath.Base(dst)) {
			tmp := dst + ".wowfix-tmp"
			if Exists(tmp) {
				_ = os.RemoveAll(tmp)
			}
			if err := os.Rename(src, tmp); err != nil {
				return err
			}
			return os.Rename(tmp, dst)
		}
		return fmt.Errorf("target %q already exists", dst)
	}
	return os.Rename(src, dst)
}

// CleanName removes characters that are unsafe in folder names on any
// supported platform. Returns the sanitized name.
func CleanName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			continue
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		out = "addon"
	}
	return out
}
