package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
)

// newCatalog builds a catalog wired with the environment's registry,
// backups, logger, profile and CurseForge API key, so CLI installs and
// updates behave exactly like the TUI's catalog.
func newCatalog(env *environment) (*catalog.Catalog, error) {
	path, err := catalog.DefaultPath()
	if err != nil {
		return nil, err
	}
	reg, err := catalog.NewRegistry(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	cat, err := catalog.New(nil, http.DefaultClient)
	if err != nil {
		return nil, err
	}
	cat.Reg = reg
	root, err := env.backupRoot()
	if err != nil {
		root = filepath.Join(env.store.Dir(), "backups")
	}
	cat.Backups = backup.New(root, env.log)
	cat.Log = env.log
	cat.Profile = env.profile
	// The WOWFIX_CURSEFORGE_API_KEY environment variable takes
	// precedence; the saved config value is the fallback (the catalog
	// checks the env var itself, so this field only needs the config).
	key := os.Getenv("WOWFIX_CURSEFORGE_API_KEY")
	if key == "" {
		key = env.cfg.CurseForgeAPIKey
	}
	cat.CurseForgeAPIKey = key
	return cat, nil
}

// runSearch implements the `wowfix search <query>` command.
func runSearch(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("search requires a single query argument")
	}
	query := rest[0]

	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	cat, err := newCatalog(env)
	if err != nil {
		return err
	}

	results, searchErr := cat.Search(context.Background(), query, 20)
	if searchErr != nil {
		fmt.Fprintf(os.Stderr, "note: search partially failed: %s\n", searchErr)
	}

	if opts.json {
		return printSearchJSON(results)
	}
	printSearchTable(results)
	return nil
}

// printSearchTable prints search results as a formatted table.
func printSearchTable(addons []*catalog.Addon) {
	if len(addons) == 0 {
		fmt.Println("No results found.")
		return
	}
	fmt.Printf("%-20s %-30s %-15s %-15s %s\n", "Provider", "Name", "Version", "Author", "ID")
	fmt.Println(strings.Repeat("-", 100))
	for _, a := range addons {
		fmt.Printf("%-20s %-30s %-15s %-15s %s\n",
			truncateRunes(a.Provider, 19),
			truncateRunes(a.Name, 29),
			truncateRunes(a.LatestVersion, 14),
			truncateRunes(a.Author, 14),
			truncateRunes(a.ID, 30))
	}
}

// printSearchJSON prints search results as JSON.
func printSearchJSON(addons []*catalog.Addon) error {
	out := make([]map[string]any, 0, len(addons))
	for _, a := range addons {
		out = append(out, map[string]any{
			"provider": a.Provider,
			"name":     a.Name,
			"version":  a.LatestVersion,
			"author":   a.Author,
			"id":       a.ID,
		})
	}
	return printJSON(out)
}

// runUpdate implements the `wowfix update [--yes]` command: it checks
// every registry-tracked addon against its provider and applies the
// available updates.
func runUpdate(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("update takes no arguments")
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

	updates, checkErr := catalog.Check(context.Background(), cat, cat.Reg, env.install.AddonsPath)
	if checkErr != nil {
		fmt.Fprintf(os.Stderr, "note: update check partially failed: %s\n", checkErr)
	}
	if len(updates) == 0 {
		fmt.Println("No updates available.")
		return nil
	}

	mismatches := 0
	for _, u := range updates {
		latest := "?"
		family := ""
		if u.Latest != nil {
			latest = u.Latest.LatestVersion
			family = u.Latest.GameVersion
		}
		if u.Mismatch {
			mismatches++
			fmt.Printf("⚠ %s: %s -> %s (%s, targets %s)\n",
				u.Entry.Folder, u.Entry.Version, latest, u.Entry.Provider, family)
			continue
		}
		fmt.Printf("%s: %s -> %s (%s)\n", u.Entry.Folder, u.Entry.Version, latest, u.Entry.Provider)
	}
	if !opts.yes && !confirm("Apply %d update(s)?", len(updates)) {
		fmt.Println("Aborted.")
		return nil
	}
	skipMismatch := false
	if !opts.yes && mismatches > 0 &&
		!confirm("%d update(s) target a different game version — apply anyway?", mismatches) {
		skipMismatch = true
	}

	applied := 0
	var failed []string
	skipped := 0
	for _, u := range updates {
		if u.Mismatch && skipMismatch {
			skipped++
			continue
		}
		if _, err := catalog.Apply(context.Background(), cat, env.install.AddonsPath, u, cat.Backups, env.log); err != nil {
			fmt.Printf("✖ %s: %v\n", u.Entry.Folder, err)
			failed = append(failed, u.Entry.Folder)
			continue
		}
		fmt.Printf("✔ %s\n", u.Entry.Folder)
		applied++
	}
	fmt.Printf("Update all: %d applied, %d failed.\n", applied, len(failed))
	if skipped > 0 {
		fmt.Printf("Skipped %d update(s) for a different game version.\n", skipped)
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d update(s) failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// runInstallFromSource installs an addon from a URL or provider-scoped
// id ("owner/repo", a CurseForge/WowInterface/Tukui addon page or id)
// through the catalog.
func runInstallFromSource(env *environment, opts *cliOptions, source string) error {
	cat, err := newCatalog(env)
	if err != nil {
		return err
	}

	fmt.Printf("Installing %s\n", source)
	names, err := cat.InstallFromSource(context.Background(), source, env.install.AddonsPath,
		func(done, total int64) {
			if total > 0 {
				fmt.Fprintf(os.Stderr, "\r  downloaded %d/%d bytes", done, total)
				if done == total {
					fmt.Fprintln(os.Stderr)
				}
			}
		})
	if err != nil {
		return err
	}

	if opts.json {
		return printJSON(map[string]any{
			"source":    source,
			"installed": names,
		})
	}
	if len(names) == 0 {
		fmt.Println("Nothing installed.")
		return nil
	}
	for _, n := range names {
		fmt.Printf("Installed %s\n", n)
	}
	return nil
}

// runSources implements the `wowfix sources` command: it lists the
// catalog providers with their honest caveats.
func runSources(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("sources takes no arguments")
	}

	providers := []map[string]string{
		{"name": "github", "description": "GitHub releases API — unauthenticated, ~60 requests/hour"},
		{"name": "curseforge", "description": "modern Core API with WOWFIX_CURSEFORGE_API_KEY, else deprecated legacy endpoint"},
		{"name": "wowinterface", "description": "MMOUI filelist JSON"},
		{"name": "tukui", "description": "tukui.org API"},
	}
	if opts.json {
		return printJSON(providers)
	}

	fmt.Println("Catalog providers:")
	for _, p := range providers {
		fmt.Printf("  %-12s %s\n", p["name"], p["description"])
	}
	return nil
}

// truncateRunes shortens s to max runes, appending "..." when longer.
// Rune-based so non-ASCII names never split mid-character.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
