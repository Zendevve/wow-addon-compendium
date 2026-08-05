package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
// available updates. With --check it only reports: exit code 0 when no
// updates are available, 1 when there are, 2 when the check fails.
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
		return updateCheckErr(opts, err)
	}
	if env.install == nil {
		return updateCheckErr(opts, fmt.Errorf("no WoW installation found; use --path or set wow_path in config"))
	}
	cat, err := newCatalog(env)
	if err != nil {
		return updateCheckErr(opts, err)
	}

	updates, checkErr := catalog.Check(context.Background(), cat, cat.Reg, env.install.AddonsPath)

	if opts.check {
		if checkErr != nil {
			fmt.Fprintf(os.Stderr, "note: update check partially failed: %s\n", checkErr)
			return errUpdateCheckFailed
		}
		if len(updates) == 0 {
			fmt.Println("No updates available.")
			return nil
		}
		printUpdateRows(updates)
		fmt.Printf("%d update(s) available.\n", len(updates))
		return errUpdatesAvailable
	}

	if checkErr != nil {
		fmt.Fprintf(os.Stderr, "note: update check partially failed: %s\n", checkErr)
	}
	if len(updates) == 0 {
		fmt.Println("No updates available.")
		return nil
	}

	mismatches := printUpdateRows(updates)
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

// printUpdateRows prints one row per pending update and returns the
// number of game-version mismatches among them.
func printUpdateRows(updates []catalog.Update) int {
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
	return mismatches
}

// updateCheckErr maps a setup or check failure onto the `update
// --check` sentinel: the underlying error is printed as a note on
// stderr and main exits with code 2. In plain update mode the error is
// returned unchanged so main prints it normally.
func updateCheckErr(opts *cliOptions, err error) error {
	if opts.check {
		fmt.Fprintf(os.Stderr, "note: %s\n", err)
		return errUpdateCheckFailed
	}
	return err
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

var (
	infoWowInterfaceIDRe = regexp.MustCompile(`(?i)info(\d+)(?:-|\.)`)
	infoTukuiIDRe        = regexp.MustCompile(`(?i)/downloads?/([^/?#]+)`)

	mdLinkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdCodeRe    = regexp.MustCompile("`([^`]*)`")
	mdBoldRe    = regexp.MustCompile(`\*\*([^*]*)\*\*`)
	mdItalicRe  = regexp.MustCompile(`\*([^*]*)\*`)
	mdHeadingRe = regexp.MustCompile(`(?m)^\s*#{1,6}\s*`)
)

// runInfo implements the `wowfix info <addon>` command: it resolves an
// addon from a provider source ("owner/repo" or a provider URL) or by
// searching a bare name, then prints the catalog details and, for
// GitHub addons, the latest release notes.
func runInfo(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: wowfix info <addon> (owner/repo, provider URL, or name)")
	}
	arg := rest[0]

	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	cat, err := newCatalog(env)
	if err != nil {
		return err
	}
	ctx := context.Background()

	var addon *catalog.Addon
	if strings.Contains(arg, "/") {
		providerName, id, err := classifySource(arg)
		if err != nil {
			return err
		}
		prov, ok := cat.Provider(providerName)
		if !ok {
			return fmt.Errorf("provider %q is not available", providerName)
		}
		addon, err = prov.Resolve(ctx, id)
		if err != nil {
			return fmt.Errorf("cannot resolve %q: %w", arg, err)
		}
	} else {
		matches, searchErr := cat.Search(ctx, arg, 5)
		if searchErr != nil {
			fmt.Fprintf(os.Stderr, "note: search partially failed: %s\n", searchErr)
		}
		switch {
		case len(matches) == 0:
			fmt.Printf("No matches for %q.\n", arg)
			return fmt.Errorf("no matches for %q", arg)
		case len(matches) > 1:
			fmt.Printf("Multiple matches for %q:\n", arg)
			for _, m := range matches {
				fmt.Printf("  %-10s %-30s %s\n", m.Provider, truncateRunes(m.Name, 29), m.ID)
			}
			return fmt.Errorf("addon %q is ambiguous; re-run with an owner/repo or a provider URL", arg)
		default:
			addon = matches[0]
		}
	}

	if opts.json {
		return printInfoJSON(addon)
	}
	printInfo(addon)
	return nil
}

// classifySource classifies an addon argument into a provider name and
// provider-scoped id, mirroring the catalog's parseSource so `info`
// needs no catalog API change. It additionally accepts a scheme-less
// "github.com/owner/repo" form.
func classifySource(source string) (string, string, error) {
	s := strings.TrimSpace(source)
	if s == "" {
		return "", "", fmt.Errorf("empty addon argument")
	}
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "github.com"):
		owner, repo, err := infoGithubPath(s)
		if err != nil {
			return "", "", err
		}
		return catalog.ProviderGitHub, owner + "/" + repo, nil
	case strings.Contains(lower, "curseforge.com"):
		slug, err := infoCurseSlug(s)
		if err != nil {
			return "", "", err
		}
		return catalog.ProviderCurseForge, slug, nil
	case strings.Contains(lower, "wowinterface.com"):
		m := infoWowInterfaceIDRe.FindStringSubmatch(s)
		if m == nil {
			return "", "", fmt.Errorf("cannot parse WowInterface URL %q (expected .../info<id>-<slug>.html)", source)
		}
		return catalog.ProviderWowInterface, m[1], nil
	case strings.Contains(lower, "tukui.org"):
		m := infoTukuiIDRe.FindStringSubmatch(s)
		if m == nil {
			return "", "", fmt.Errorf("cannot parse Tukui URL %q (expected .../downloads/<id>)", source)
		}
		return catalog.ProviderTukui, m[1], nil
	case strings.Contains(s, "/"):
		owner, repo, err := infoGithubPath(s)
		if err != nil {
			return "", "", err
		}
		return catalog.ProviderGitHub, owner + "/" + repo, nil
	default:
		return "", "", fmt.Errorf("unknown addon %q", source)
	}
}

// infoGithubPath extracts owner and repo from "owner/repo" or a
// github.com URL, ignoring any trailing segments. Unlike the catalog's
// classifier it also accepts a scheme-less "github.com/owner/repo".
func infoGithubPath(s string) (owner, repo string, err error) {
	u := s
	if strings.Contains(strings.ToLower(s), "github.com") {
		parsed, err := url.Parse(s)
		if err != nil {
			return "", "", fmt.Errorf("invalid GitHub URL %q: %w", s, err)
		}
		u = parsed.Path
	}
	u = strings.Trim(u, "/")
	parts := strings.Split(u, "/")
	if len(parts) > 1 && (strings.EqualFold(parts[0], "github.com") || strings.EqualFold(parts[0], "www.github.com")) {
		parts = parts[1:]
	}
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("GitHub source %q must be owner/repo", s)
	}
	return parts[0], parts[1], nil
}

// infoCurseSlug extracts the addon slug from a CurseForge URL path
// such as /wow/addons/<slug>.
func infoCurseSlug(s string) (string, error) {
	parsed, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid CurseForge URL %q: %w", s, err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, p := range parts {
		if strings.EqualFold(p, "addons") && i+1 < len(parts) && parts[i+1] != "" {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("cannot parse CurseForge URL %q (expected .../wow/addons/<slug>)", s)
}

// printInfo prints one addon's catalog details in a stable field order.
func printInfo(addon *catalog.Addon) {
	fmt.Printf("Name:          %s\n", addon.Name)
	fmt.Printf("Provider:      %s\n", addon.Provider)
	fmt.Printf("ID:            %s\n", addon.ID)
	fmt.Printf("Author:        %s\n", addon.Author)
	fmt.Printf("LatestVersion: %s\n", addon.LatestVersion)
	fmt.Printf("GameVersion:   %s\n", addon.GameVersion)
	fmt.Printf("Homepage:      %s\n", addon.Homepage)
	updated := ""
	if !addon.UpdatedAt.IsZero() {
		updated = addon.UpdatedAt.Format(time.RFC1123)
	}
	fmt.Printf("UpdatedAt:     %s\n", updated)
	if addon.Provider != catalog.ProviderGitHub {
		return
	}
	notes, err := infoReleaseNotes(addon)
	if err != nil || strings.TrimSpace(notes) == "" {
		fmt.Println("Release notes: release notes unavailable")
		return
	}
	lines := strings.Split(notes, "\n")
	if len(lines) > 20 {
		lines = lines[:20]
	}
	fmt.Println("Release notes:")
	for _, l := range lines {
		fmt.Println("  " + l)
	}
}

// printInfoJSON renders one addon's details as JSON. Release notes are
// best-effort: an empty string means they could not be fetched.
func printInfoJSON(addon *catalog.Addon) error {
	out := map[string]any{
		"provider":      addon.Provider,
		"id":            addon.ID,
		"name":          addon.Name,
		"author":        addon.Author,
		"version":       addon.LatestVersion,
		"game_version":  addon.GameVersion,
		"homepage":      addon.Homepage,
		"updated_at":    addon.UpdatedAt,
		"release_notes": "",
	}
	if addon.Provider == catalog.ProviderGitHub {
		if notes, err := infoReleaseNotes(addon); err == nil {
			out["release_notes"] = notes
		}
	}
	return printJSON(out)
}

// infoReleaseNotes fetches the latest GitHub release notes for the
// addon's repository, stripped of markdown formatting. Any failure
// (network, rate limit, no release) returns an error; callers degrade
// to "release notes unavailable" instead of failing the command.
func infoReleaseNotes(addon *catalog.Addon) (string, error) {
	owner, repo, ok := strings.Cut(addon.ID, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", fmt.Errorf("github: not an owner/repo id")
	}
	u := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases/latest"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: unexpected status %d", resp.StatusCode)
	}
	var rel struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return stripMarkdown(rel.Body), nil
}

// stripMarkdown reduces release-note markdown to plain text: headings,
// bold/italic emphasis, inline code and links keep their content while
// the syntax is dropped, and blank lines are collapsed.
func stripMarkdown(s string) string {
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = mdCodeRe.ReplaceAllString(s, "$1")
	s = mdBoldRe.ReplaceAllString(s, "$1")
	s = mdItalicRe.ReplaceAllString(s, "$1")
	s = mdHeadingRe.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
