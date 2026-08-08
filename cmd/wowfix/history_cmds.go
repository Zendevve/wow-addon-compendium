package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/catalog"
)

// runHistory implements the `wowfix history <folder>` command: it
// prints the recorded version log of one tracked addon, newest first,
// marking the currently installed version.
func runHistory(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("history requires one addon folder")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	cat, err := newCatalog(env)
	if err != nil {
		return err
	}

	entry, ok := findTrackedEntry(cat.Reg, rest[0])
	if !ok {
		return fmt.Errorf("addon %q is not tracked in the registry", rest[0])
	}
	if len(entry.History) == 0 {
		fmt.Printf("%s: no version history recorded yet (installed %s)\n", entry.Folder, entry.Version)
		return nil
	}

	if opts.json {
		rows := make([]map[string]any, 0, len(entry.History))
		for _, h := range entry.History {
			rows = append(rows, map[string]any{
				"version":  h.Version,
				"provider": h.Provider,
				"source":   h.Source,
				"ref":      h.Ref,
				"at":       h.At.UTC().Format(time.RFC3339),
				"current":  h.Version == entry.Version,
			})
		}
		return printJSON(map[string]any{"folder": entry.Folder, "current": entry.Version, "versions": rows})
	}

	fmt.Printf("%s (currently %s)\n", entry.Folder, entry.Version)
	fmt.Println("----------------------------------------------")
	for _, h := range entry.History {
		mark := " "
		if h.Version == entry.Version {
			mark = "*"
		}
		ref := h.Ref
		if ref != "" {
			ref = " (" + ref + ")"
		}
		fmt.Printf("%s %-18s %-12s %s%s\n", mark, h.Version, h.Provider, h.At.Format("2006-01-02 15:04"), ref)
	}
	return nil
}

// runRollback implements the `wowfix rollback <folder> <version>`
// command: it re-downloads the specific past version of a tracked
// addon from its provider and replaces the folder, snapshotting the
// current state first. Providers that only serve the latest version
// fail with an honest error instead of installing the latest.
func runRollback(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return fmt.Errorf("rollback requires an addon folder and a version")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	cat, err := newCatalog(env)
	if err != nil {
		return err
	}

	folder, version := rest[0], rest[1]
	entry, ok := findTrackedEntry(cat.Reg, folder)
	if !ok {
		return fmt.Errorf("addon %q is not tracked in the registry", folder)
	}
	var hist *catalog.VersionHistory
	for i := range entry.History {
		if entry.History[i].Version == version {
			hist = &entry.History[i]
			break
		}
	}
	if hist == nil {
		return fmt.Errorf("no recorded version %q for %q", version, folder)
	}

	fmt.Printf("Rolling back %s to %s (%s)\n", entry.Folder, hist.Version, entry.Provider)
	installed, err := catalog.RollbackToVersion(context.Background(), cat, env.install.AddonsPath, entry, *hist, cat.Backups, env.log)
	if err != nil {
		return err
	}
	if opts.json {
		return printJSON(map[string]any{"folder": installed, "version": hist.Version})
	}
	fmt.Printf("✔ %s rolled back to %s\n", installed, hist.Version)
	return nil
}

// findTrackedEntry looks one registry entry up by folder,
// case-insensitively.
func findTrackedEntry(reg *catalog.Registry, folder string) (catalog.Entry, bool) {
	for _, e := range reg.Entries() {
		if strings.EqualFold(e.Folder, folder) {
			return e, true
		}
	}
	return catalog.Entry{}, false
}
