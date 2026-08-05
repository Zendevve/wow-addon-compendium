// Package installer extracts addon ZIP archives, normalizes their
// folder structure (Git ref names, nesting, TOC mismatches) and
// installs the result into the AddOns directory.
package installer

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/scanner"
	"github.com/wowfix/wowfix/internal/utils"
)

// Options configures an Installer.
type Options struct {
	// AddonsDir is the destination Interface/AddOns directory.
	AddonsDir string
	// Profile classifies TOC compatibility after install.
	Profile *models.Profile
	// Backups snapshots folders that get replaced. nil disables backups.
	Backups *backup.Manager
	// Log receives install action lines.
	Log *logger.Logger
	// Confirm decides whether an existing folder may be replaced.
	// nil means always replace.
	Confirm func(addonName string) bool
}

// Installer installs ZIP archives.
type Installer struct {
	opts Options
}

// New returns an Installer.
func New(opts Options) *Installer {
	if opts.Profile == nil {
		opts.Profile = models.DefaultProfile()
	}
	return &Installer{opts: opts}
}

// Result reports what an install did.
type Result struct {
	Installed []string
	Replaced  []string
	Skipped   map[string]string // folder -> reason
	Errors    []error
}

// Install extracts zipPath, normalizes the addon folders and copies
// them into the AddOns directory. The archive itself is never modified.
func (i *Installer) Install(ctx context.Context, zipPath string) (*Result, error) {
	res := &Result{Skipped: map[string]string{}}

	if !strings.EqualFold(filepath.Ext(zipPath), ".zip") {
		return nil, fmt.Errorf("%q is not a .zip archive", zipPath)
	}
	if err := utils.IsWritable(i.opts.AddonsDir); err != nil {
		return nil, fmt.Errorf("AddOns directory is not writable: %w", err)
	}

	tmp, err := os.MkdirTemp("", "wowfix-install-")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	if err := extractZip(ctx, zipPath, tmp); err != nil {
		return nil, fmt.Errorf("extract %q: %w", filepath.Base(zipPath), err)
	}

	// Analyze the extraction with the same logic as the scanner.
	discovered, scanErrs := scanner.New(tmp, i.opts.Profile).Discover(ctx)
	res.Errors = append(res.Errors, scanErrs...)
	if len(discovered) == 0 {
		return nil, fmt.Errorf("no addons (folders containing .toc files) found in %q", filepath.Base(zipPath))
	}

	for _, addon := range discovered {
		if ctx.Err() != nil {
			res.Errors = append(res.Errors, ctx.Err())
			break
		}
		i.installAddon(ctx, addon, res)
	}
	return res, nil
}

// installAddon normalizes one discovered addon and copies it into place.
func (i *Installer) installAddon(ctx context.Context, addon *models.Addon, res *Result) {
	source := addon.Path
	name := addon.SuggestedName

	// Flatten nested addons out of their wrapper folder so the copy
	// below targets the real addon directory.
	if addon.Nested {
		target := filepath.Join(filepath.Dir(source), name)
		if target != source && !utils.Exists(target) {
			if err := utils.SafeRename(source, target); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("%s: %w", addon.FolderName, err))
				return
			}
			source = target
		}
	}

	dst := filepath.Join(i.opts.AddonsDir, utils.CleanName(name))
	if utils.Exists(dst) {
		if i.opts.Confirm != nil && !i.opts.Confirm(name) {
			res.Skipped[name] = "folder already exists, user declined to replace"
			if i.opts.Log != nil {
				i.opts.Log.Infof("Skipped %s: already installed", name)
			}
			return
		}
		if i.opts.Backups != nil {
			if _, err := i.opts.Backups.Backup([]string{dst}, "replace "+name); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("%s: pre-replace backup failed: %w", name, err))
				return
			}
		}
		if err := os.RemoveAll(dst); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("%s: cannot remove existing folder: %w", name, err))
			return
		}
		res.Replaced = append(res.Replaced, name)
	}

	if err := utils.CopyDir(source, dst); err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("%s: %w", name, err))
		return
	}

	// Drop the emptied wrapper folder of a nested install.
	if addon.Nested {
		_ = os.RemoveAll(filepath.Join(filepath.Dir(source), addon.SourceDir))
	}

	res.Installed = append(res.Installed, name)
	if i.opts.Log != nil {
		i.opts.Log.Infof("Installed %s (%d TOC file(s))", name, len(addon.TOCs))
	}
}

// extractZip extracts all entries of the archive into dst, guarding
// against zip-slip path traversal.
func extractZip(ctx context.Context, zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := extractEntry(f, dst); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(f *zip.File, dst string) error {
	clean := filepath.Clean(f.Name)
	if clean == "." || filepath.IsAbs(clean) {
		return fmt.Errorf("archive contains unsafe path %q", f.Name)
	}
	// Reject traversal on every platform.
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("archive contains path traversal %q", f.Name)
	}
	target := filepath.Join(dst, clean)
	if !strings.HasPrefix(target, filepath.Clean(dst)+string(filepath.Separator)) &&
		target != filepath.Clean(dst) {
		return fmt.Errorf("archive contains path traversal %q", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
