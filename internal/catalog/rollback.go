package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/logger"
)

// RollbackToVersion re-downloads the specific past version of one
// tracked addon from its provider and installs it into installDir,
// replacing the current folder (snapshotted first through backups) and
// re-recording the registry entry. The version must be addressable by
// the provider: GitHub tags and CurseForge files are; WowInterface and
// Tukui serve only the latest version, so they fail with an error
// wrapping ErrVersionNotServed rather than silently installing the
// latest. hist is the history record being restored (its Version and
// Ref drive the provider resolution). It returns the installed folder
// name.
func RollbackToVersion(ctx context.Context, cat *Catalog, installDir string, e Entry, hist VersionHistory, backups *backup.Manager, log *logger.Logger) (string, error) {
	if cat == nil {
		return "", fmt.Errorf("catalog is nil")
	}
	prov, ok := cat.Provider(e.Provider)
	if !ok {
		return "", fmt.Errorf("provider %q is not enabled", e.Provider)
	}
	resolver, ok := prov.(VersionResolver)
	if !ok {
		return "", fmt.Errorf("%s can no longer re-download version %q — only the latest is available (%w)",
			e.Provider, hist.Version, ErrVersionNotServed)
	}

	base := &Addon{Provider: e.Provider, ID: e.ID, Name: e.Title}
	addon, err := resolver.ResolveVersion(ctx, base, hist.Version, hist.Ref)
	if err != nil {
		return "", fmt.Errorf("resolve version %q of %s: %w", hist.Version, e.Folder, err)
	}

	tmp, err := os.CreateTemp("", "wowfix-rollback-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := prov.Download(ctx, addon, tmpPath, nil); err != nil {
		return "", fmt.Errorf("download %s: %w", e.Folder, err)
	}

	inst := installer.New(installer.Options{
		AddonsDir: installDir,
		Profile:   cat.Profile,
		Backups:   backups,
		Log:       log,
	})
	res, err := inst.Install(ctx, tmpPath)
	if err != nil {
		return "", err
	}
	if len(res.Installed) == 0 {
		if len(res.Errors) > 0 {
			return "", errors.Join(res.Errors...)
		}
		return "", fmt.Errorf("installer installed nothing for %s", e.Folder)
	}
	folder := pickInstalled(res.Installed, e.Folder)

	if cat.Reg != nil {
		version := addon.LatestVersion
		if v := readTOCVersion(filepath.Join(installDir, folder)); v != "" {
			version = v
		}
		title := addon.Name
		if title == "" {
			title = e.Title
		}
		// Best-effort provenance, same rule as installs: a manifest
		// failure records no checksum instead of failing the rollback.
		checksum, _ := ComputeManifest(filepath.Join(installDir, folder))
		if err := cat.Reg.Track(Entry{
			Folder:     folder,
			Title:      title,
			Version:    version,
			Provider:   e.Provider,
			ID:         e.ID,
			Source:     e.Source,
			Checksum:   checksum,
			VersionRef: addon.VersionRef,
		}); err != nil {
			return folder, err
		}
	}
	if len(res.Errors) > 0 {
		return folder, errors.Join(res.Errors...)
	}
	return folder, nil
}
