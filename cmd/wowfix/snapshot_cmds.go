package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/wowfix/wowfix/internal/catalog"
)

// runSnapshot implements the `wowfix snapshot export|check` command:
// exporting freezes the tracked addon state with the latest known
// versions into a portable JSON file while online; checking diffs a
// snapshot against the current registry entirely offline.
func runSnapshot(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("snapshot requires a subcommand: export <out.json> or check <snapshot.json>")
	}
	switch rest[0] {
	case "export":
		return runSnapshotExport(opts, rest[1:])
	case "check":
		return runSnapshotCheck(opts, rest[1:])
	default:
		return fmt.Errorf("unknown snapshot subcommand %q (want export or check)", rest[0])
	}
}

// runSnapshotExport implements `wowfix snapshot export <out.json>`: it
// resolves the environment, exports every tracked addon with its
// latest known version into the given path and prints the count.
// --json additionally prints the snapshot itself.
func runSnapshotExport(opts *cliOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("snapshot export requires an output path")
	}
	out := args[0]
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
	snap, err := catalog.ExportSnapshot(context.Background(), cat, cat.Reg, env.profile.ID, time.Now())
	if err != nil {
		return err
	}
	if err := snap.Save(out); err != nil {
		return err
	}
	for _, w := range snap.Warnings {
		fmt.Fprintf(os.Stderr, "note: %s\n", w)
	}
	if opts.json {
		return printJSON(snap)
	}
	fmt.Printf("Exported %d addon(s) to %s\n", len(snap.Addons), out)
	return nil
}

// runSnapshotCheck implements `wowfix snapshot check <snapshot.json>`:
// it loads the snapshot and diffs it against the current registry with
// no network access. Exit codes mirror `update --check`: 0 when no
// updates are available, 1 when there are, 2 when the check fails.
func runSnapshotCheck(opts *cliOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("snapshot check requires a snapshot file path")
	}
	snap, err := catalog.LoadSnapshot(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: %v\n", err)
		return errUpdateCheckFailed
	}
	regPath, err := catalog.DefaultPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: %v\n", err)
		return errUpdateCheckFailed
	}
	reg, err := catalog.NewRegistry(regPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: %v\n", err)
		return errUpdateCheckFailed
	}
	updates, err := snap.Diff(reg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: %v\n", err)
		return errUpdateCheckFailed
	}
	for _, w := range snap.Warnings {
		fmt.Fprintf(os.Stderr, "note: %s\n", w)
	}
	if len(updates) == 0 {
		fmt.Println("No updates available.")
		return nil
	}
	printSnapshotUpdateRows(updates)
	fmt.Printf("%d update(s) available.\n", len(updates))
	return errUpdatesAvailable
}

// printSnapshotUpdateRows prints one row per pending update from a
// snapshot diff.
func printSnapshotUpdateRows(updates []catalog.Update) {
	fmt.Printf("%-30s %-18s %-18s %s\n", "Folder", "Current", "Latest", "Provider")
	fmt.Println("--------------------------------------------------------------")
	for _, u := range updates {
		fmt.Printf("%-30s %-18s %-18s %s\n",
			truncateRunes(u.Entry.Folder, 29),
			truncateRunes(u.Entry.Version, 17),
			truncateRunes(u.Latest.LatestVersion, 17),
			u.Entry.Provider)
	}
}
