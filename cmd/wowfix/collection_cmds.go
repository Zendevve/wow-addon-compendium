package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/importexport"
	"github.com/wowfix/wowfix/internal/profiles"
	"github.com/wowfix/wowfix/internal/savedvars"
	"github.com/wowfix/wowfix/internal/utils"
)

const profileUsage = `usage:
  wowfix profile list
  wowfix profile show <id>
  wowfix profile create <name>
  wowfix profile duplicate <id> <new name>
  wowfix profile rename <id> <new name>
  wowfix profile delete <id>
  wowfix profile switch <id> [--yes]
  wowfix profile enable <id> <folder>
  wowfix profile disable <id> <folder>`

const savedVarsUsage = `usage:
  wowfix savedvars list [--account X]
  wowfix savedvars backup [--account X] [--dest DIR]
  wowfix savedvars restore <backupPath> [--account X] [--yes]
  wowfix savedvars reset <addon> [--account X] [--yes]`

// wtfRoot returns the WTF directory of an installation: the game root
// plus the flavor subfolder plus WTF.
func wtfRoot(root, flavor string) string {
	return filepath.Join(root, flavor, "WTF")
}

// collectionsDir resolves where collection files live: the config
// override, else <config dir>/collections.
func collectionsDir(env *environment) string {
	if env.cfg.CollectionsDir != "" {
		return env.cfg.CollectionsDir
	}
	return filepath.Join(env.store.Dir(), "collections")
}

// newProfileManager builds a profile manager wired with the
// environment's AddOns dir, logger and (when auto_backup is on) a
// backup manager for pre-switch snapshots.
func newProfileManager(env *environment) (*profiles.Manager, error) {
	addonsDir := ""
	if env.install != nil {
		addonsDir = env.install.AddonsPath
	}
	m, err := profiles.NewManager(collectionsDir(env), addonsDir)
	if err != nil {
		return nil, err
	}
	m.Log = env.log
	if env.cfg.AutoBackup && env.install != nil {
		root, err := env.backupRoot()
		if err != nil {
			root = filepath.Join(env.store.Dir(), "backups")
		}
		m.Backups = backup.New(root, env.log)
	}
	return m, nil
}

func runProfile(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("profile requires a subcommand\n\n%s", profileUsage)
	}
	cmd, rest := rest[0], rest[1:]

	m, err := newProfileManager(env)
	if err != nil {
		return err
	}

	switch cmd {
	case "list":
		return profileList(env, m, opts)
	case "show":
		if len(rest) != 1 {
			return fmt.Errorf("usage: wowfix profile show <id>")
		}
		return profileShow(m, rest[0])
	case "create":
		if len(rest) != 1 {
			return fmt.Errorf("usage: wowfix profile create <name>")
		}
		if env.install == nil {
			return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
		}
		c, err := m.Create(rest[0])
		if err != nil {
			return err
		}
		fmt.Printf("Created collection %q (id %s) from %d addon(s).\n", c.Name, c.ID, len(c.Addons))
		return nil
	case "duplicate":
		if len(rest) != 2 {
			return fmt.Errorf("usage: wowfix profile duplicate <id> <new name>")
		}
		c, err := m.Duplicate(rest[0], rest[1])
		if err != nil {
			return err
		}
		fmt.Printf("Duplicated %q as %q (id %s).\n", rest[0], c.Name, c.ID)
		return nil
	case "rename":
		if len(rest) != 2 {
			return fmt.Errorf("usage: wowfix profile rename <id> <new name>")
		}
		if err := m.Rename(rest[0], rest[1]); err != nil {
			return err
		}
		fmt.Printf("Renamed collection %q to %q.\n", rest[0], rest[1])
		return nil
	case "delete":
		if len(rest) != 1 {
			return fmt.Errorf("usage: wowfix profile delete <id>")
		}
		if err := m.Delete(rest[0]); err != nil {
			return err
		}
		fmt.Printf("Deleted collection %q.\n", rest[0])
		return nil
	case "switch":
		if len(rest) != 1 {
			return fmt.Errorf("usage: wowfix profile switch <id> [--yes]")
		}
		if env.install == nil {
			return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
		}
		id := rest[0]
		c, err := m.Get(id)
		if err != nil {
			return err
		}
		if !opts.yes && !confirm("Switch to collection %q (renames %d addon folder(s))?", c.Name, len(c.Addons)) {
			return nil
		}
		applied, err := m.SwitchTo(id)
		if err != nil {
			return err
		}
		env.cfg.Collection = id
		if err := env.store.Save(env.cfg); err != nil {
			return err
		}
		fmt.Printf("Switched to %q: %d folder(s) renamed.\n", c.Name, len(applied))
		for _, f := range applied {
			fmt.Printf("  %s\n", f)
		}
		return nil
	case "enable", "disable":
		if len(rest) != 2 {
			return fmt.Errorf("usage: wowfix profile %s <id> <folder>", cmd)
		}
		enabled := cmd == "enable"
		if err := m.SetEnabled(rest[0], rest[1], enabled); err != nil {
			return err
		}
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		fmt.Printf("Collection %q: %s is now %s.\n", rest[0], rest[1], state)
		return nil
	default:
		return fmt.Errorf("unknown profile subcommand %q\n\n%s", cmd, profileUsage)
	}
}

func profileList(env *environment, m *profiles.Manager, opts *cliOptions) error {
	cols, err := m.List()
	if err != nil {
		return err
	}
	if opts.json {
		return printJSON(map[string]any{
			"collections_dir": collectionsDir(env),
			"active":          env.cfg.Collection,
			"collections":     cols,
		})
	}
	if len(cols) == 0 {
		fmt.Println("No collections yet. Create one with: wowfix profile create <name>")
		return nil
	}
	fmt.Printf("%d collection(s)\n", len(cols))
	for _, c := range cols {
		active := ""
		if env.cfg.Collection == c.ID {
			active = " [active]"
		}
		fmt.Printf("  %-28s %3d addon(s)  (id %s)%s\n",
			truncateRunes(c.Name, 27), len(c.Addons), c.ID, active)
	}
	return nil
}

func profileShow(m *profiles.Manager, id string) error {
	c, err := m.Get(id)
	if err != nil {
		return err
	}
	fmt.Printf("Collection %q (id %s)\n", c.Name, c.ID)
	fmt.Printf("Created %s  ·  Updated %s\n",
		c.CreatedAt.Format("2006-01-02 15:04:05"), c.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("%d addon(s):\n", len(c.Addons))
	for _, st := range c.Addons {
		state := "enabled "
		if !st.Enabled {
			state = "disabled"
		}
		fmt.Printf("  %s  %s\n", state, st.Folder)
	}
	return nil
}

// extractFlag pulls the value of --name (or -name) out of args,
// returning it and the remaining arguments.
func extractFlag(args []string, name string) (string, []string, error) {
	var rest []string
	val := ""
	found := false
	short := "-" + strings.TrimPrefix(name, "--")
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == name || a == short {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a value", name)
			}
			val = args[i+1]
			found = true
			i++
			continue
		}
		rest = append(rest, a)
	}
	if !found {
		return "", rest, nil
	}
	return val, rest, nil
}

// extractBoolFlag removes a presence flag (e.g. --savedvars) from args.
func extractBoolFlag(args []string, name string) (bool, []string) {
	var rest []string
	present := false
	for _, a := range args {
		if a == name {
			present = true
			continue
		}
		rest = append(rest, a)
	}
	return present, rest
}

// pickAccount resolves the account for a savedvars command: the
// requested one, or the first existing one (announced when several
// exist).
func pickAccount(m *savedvars.Manager, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	accts := m.Accounts()
	if len(accts) == 0 {
		return "", fmt.Errorf("no accounts found under %s/Account", m.Root)
	}
	if len(accts) > 1 {
		fmt.Fprintf(os.Stderr, "Using account %q (pass --account to choose)\n", accts[0])
	}
	return accts[0], nil
}

func runSavedVars(args []string) error {
	account, rest, err := extractFlag(args, "--account")
	if err != nil {
		return err
	}
	dest, rest, err := extractFlag(rest, "--dest")
	if err != nil {
		return err
	}
	opts, rest, err := parseCLIOptions(rest)
	if err != nil {
		return err
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	if len(rest) == 0 {
		return fmt.Errorf("savedvars requires a subcommand\n\n%s", savedVarsUsage)
	}
	cmd, rest := rest[0], rest[1:]

	wtf := wtfRoot(env.install.Root, env.install.Flavor)
	m := savedvars.New(wtf, env.log)

	switch cmd {
	case "list":
		acct, err := pickAccount(m, account)
		if err != nil {
			return err
		}
		files, err := m.List(acct)
		if err != nil {
			return err
		}
		if opts.json {
			return printJSON(map[string]any{
				"wtf_root": wtf,
				"account":  acct,
				"files":    files,
			})
		}
		fmt.Printf("Account %q: %s\n", acct, filepath.Join(wtf, "Account", acct, "SavedVariables"))
		if len(files) == 0 {
			fmt.Println("  (no SavedVariables yet)")
			return nil
		}
		for _, f := range files {
			fmt.Printf("  %s.lua\n", f)
		}
		return nil

	case "backup":
		acct, err := pickAccount(m, account)
		if err != nil {
			return err
		}
		if dest == "" {
			dest = filepath.Join(wtf, "savedvars-backups")
		}
		path, err := m.Backup(acct, dest)
		if err != nil {
			return err
		}
		if opts.json {
			return printJSON(map[string]any{"path": path, "account": acct})
		}
		fmt.Printf("SavedVariables backed up to %s\n", path)
		return nil

	case "restore":
		if len(rest) != 1 {
			return fmt.Errorf("usage: wowfix savedvars restore <backupPath> [--account X]")
		}
		acct, err := pickAccount(m, account)
		if err != nil {
			return err
		}
		if !opts.yes && !confirm("Restore SavedVariables for account %q from %s?", acct, rest[0]) {
			return nil
		}
		if err := m.Restore(acct, rest[0]); err != nil {
			return err
		}
		if opts.json {
			return printJSON(map[string]any{"restored": rest[0], "account": acct})
		}
		fmt.Printf("Restored SavedVariables for account %q from %s\n", acct, rest[0])
		return nil

	case "reset":
		if len(rest) != 1 {
			return fmt.Errorf("usage: wowfix savedvars reset <addon> [--account X]")
		}
		acct, err := pickAccount(m, account)
		if err != nil {
			return err
		}
		if !opts.yes && !confirm("Reset SavedVariables for %q (account %q)?", rest[0], acct) {
			return nil
		}
		if err := m.Reset(acct, rest[0]); err != nil {
			return err
		}
		if opts.json {
			return printJSON(map[string]any{"reset": rest[0], "account": acct})
		}
		fmt.Printf("Reset %s.lua for account %q\n", rest[0], acct)
		return nil

	default:
		return fmt.Errorf("unknown savedvars subcommand %q\n\n%s", cmd, savedVarsUsage)
	}
}

func runExport(args []string) error {
	collection, rest, err := extractFlag(args, "--collection")
	if err != nil {
		return err
	}
	bundleSavedVars, rest := extractBoolFlag(rest, "--savedvars")
	opts, rest, err := parseCLIOptions(rest)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: wowfix export <out.json|out.zip> [--collection <id>] [--savedvars]")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	out := rest[0]

	addons, name, err := buildManifestAddons(env, collection)
	if err != nil {
		return err
	}

	switch strings.ToLower(filepath.Ext(out)) {
	case ".json":
		if err := importexport.ExportManifest(name, env.cfg.Profile, addons, out); err != nil {
			return err
		}
	case ".zip":
		svDir := ""
		if bundleSavedVars {
			svDir = firstSavedVarsDir(env)
			if svDir == "" {
				fmt.Fprintln(os.Stderr, "note: no SavedVariables directory found, bundling without it")
			}
		}
		if err := importexport.ExportZip(name, env.cfg.Profile, addons, env.install.AddonsPath, svDir, out); err != nil {
			return err
		}
	default:
		return fmt.Errorf("export requires a .json or .zip output path")
	}
	if opts.json {
		return printJSON(map[string]any{"out": out, "addons": len(addons), "collection": env.cfg.Collection})
	}
	fmt.Printf("Exported %d addon(s) to %s\n", len(addons), out)
	return nil
}

// buildManifestAddons assembles the manifest entries for an export:
// either the named collection's addons or the current on-disk scan,
// enriched with registry source information when tracked.
func buildManifestAddons(env *environment, collectionID string) ([]importexport.ManifestAddon, string, error) {
	byFolder := map[string]catalog.Entry{}
	if path, err := catalog.DefaultPath(); err == nil {
		if reg, err := catalog.NewRegistry(path); err == nil {
			for _, e := range reg.Entries() {
				byFolder[strings.ToLower(e.Folder)] = e
			}
		}
	}
	enrich := func(folder string) importexport.ManifestAddon {
		a := importexport.ManifestAddon{Folder: folder}
		if e, ok := byFolder[strings.ToLower(folder)]; ok && e.Provider != "" {
			a.Provider = e.Provider
			a.ID = e.ID
			a.Source = e.Source
			a.Version = e.Version
		}
		return a
	}

	if collectionID != "" {
		m, err := profiles.NewManager(collectionsDir(env), env.install.AddonsPath)
		if err != nil {
			return nil, "", err
		}
		c, err := m.Get(collectionID)
		if err != nil {
			return nil, "", err
		}
		addons := make([]importexport.ManifestAddon, 0, len(c.Addons))
		for _, st := range c.Addons {
			addons = append(addons, enrich(st.Folder))
		}
		return addons, c.Name, nil
	}

	entries, err := os.ReadDir(env.install.AddonsPath)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read AddOns directory: %w", err)
	}
	var addons []importexport.ManifestAddon
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		folder := e.Name()
		if strings.HasSuffix(strings.ToLower(folder), ".disabled") {
			continue
		}
		addons = append(addons, enrich(folder))
	}
	return addons, "wowfix-export", nil
}

// firstSavedVarsDir returns the first account's SavedVariables
// directory, or "" when none exists.
func firstSavedVarsDir(env *environment) string {
	wtf := wtfRoot(env.install.Root, env.install.Flavor)
	m := savedvars.New(wtf, env.log)
	accts := m.Accounts()
	if len(accts) == 0 {
		return ""
	}
	dir := filepath.Join(wtf, "Account", accts[0], "SavedVariables")
	if !utils.IsDir(dir) {
		return ""
	}
	return dir
}

func runImport(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: wowfix import <manifest.json|bundle.zip|github-list-url> [--yes]")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	arg := rest[0]
	cat, err := newCatalog(env)
	if err != nil {
		return err
	}

	var installed []string
	switch {
	case utils.Exists(arg) && strings.EqualFold(filepath.Ext(arg), ".zip"):
		fmt.Printf("Importing bundle %s\n", arg)
		installed, err = importexport.ImportZip(arg, env.install.AddonsPath,
			wtfRoot(env.install.Root, env.install.Flavor), cat, nil)

	case utils.Exists(arg) && strings.EqualFold(filepath.Ext(arg), ".json"):
		var mf *importexport.Manifest
		mf, err = importexport.ImportManifest(arg)
		if err != nil {
			return err
		}
		fmt.Printf("Importing manifest %q (%d addon(s))\n", mf.Name, len(mf.Addons))
		installed, err = installManifest(env, cat, mf)

	case strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://"):
		fmt.Printf("Importing addon list %s\n", arg)
		installed, err = importexport.ImportGitHubList(arg, env.install.AddonsPath, cat, nil)

	default:
		return fmt.Errorf("import requires an existing .zip or .json file, or an http(s) URL")
	}
	if err != nil {
		return err
	}

	if opts.json {
		return printJSON(map[string]any{"installed": installed})
	}
	if len(installed) == 0 {
		fmt.Println("Nothing to install.")
		return nil
	}
	fmt.Printf("Installed %d addon(s):\n", len(installed))
	for _, n := range installed {
		fmt.Printf("  %s\n", n)
	}
	return nil
}

// installManifest installs a parsed manifest: remote entries through
// the catalog, local entries by presence check (a bare manifest has no
// addon payload to copy).
func installManifest(env *environment, cat *catalog.Catalog, mf *importexport.Manifest) ([]string, error) {
	var installed []string
	for _, a := range mf.Addons {
		switch {
		case a.Provider != "" || a.Source != "":
			source := a.Source
			if source == "" {
				source = a.ID
			}
			names, err := cat.InstallFromSource(context.Background(), source, env.install.AddonsPath, nil)
			if err != nil {
				return installed, fmt.Errorf("install %q: %w", a.Folder, err)
			}
			installed = append(installed, names...)
		default:
			if utils.IsDir(filepath.Join(env.install.AddonsPath, a.Folder)) {
				fmt.Printf("  %s: local addon, already installed\n", a.Folder)
			} else {
				fmt.Printf("  %s: local addon, not part of this manifest — skipped\n", a.Folder)
			}
		}
	}
	return installed, nil
}
