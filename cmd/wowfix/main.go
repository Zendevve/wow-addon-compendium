// Command wowfix is a cross-platform terminal utility that scans a
// World of Warcraft AddOns folder, detects common installation
// problems, repairs them safely (with backups and trash), validates
// TOC compatibility and installs addons from ZIP archives.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/fixer"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/savedvars"
	"github.com/wowfix/wowfix/internal/scanner"
	"github.com/wowfix/wowfix/internal/ui"
	"github.com/wowfix/wowfix/internal/utils"
	"github.com/wowfix/wowfix/internal/validator"
)

// Build metadata, overridable via -ldflags.
var (
	version = "1.0.0"
	commit  = "none"
	date    = "unknown"
)

const usage = `wowfix — World of Warcraft addon fixer

Usage:
  wowfix                        launch the terminal UI
  wowfix scan                   scan the AddOns folder and report problems
  wowfix fix                    fix all detected problems [--yes]
  wowfix install <addon.zip>    install an addon archive [--yes]
  wowfix install <url|owner/repo>  install from a provider source
  wowfix validate               validate TOC compatibility
  wowfix list                   list addons with status
  wowfix search <query>         search the addon catalog
  wowfix update [--yes]         check and apply addon updates
  wowfix sources                list catalog providers and their caveats
  wowfix backup                 snapshot all addons
  wowfix restore [id]           list backups, or restore one
  wowfix doctor                 check environment and permissions
  wowfix config                 show configuration
  wowfix config set <key> <val> set a config value
  wowfix profile                manage addon collections (list/create/switch/...)
  wowfix savedvars              list/back up/restore/reset SavedVariables
  wowfix export <out.json|out.zip>  export a collection [--collection <id>] [--savedvars]
  wowfix import <file|url>      import a manifest, bundle zip or GitHub repo list
  wowfix version                print version
  wowfix preview                render a text preview of the TUI (README)
  wowfix help                   show this help

Flags:
  --path <dir>   WoW installation root (overrides saved config)
  --yes          skip confirmation prompts (used with fix/install/update)
  --json         machine-readable output for list/scan/validate/search

Drag-and-drop: drop an addon.zip onto the executable (or pass it as the
first argument) to install it non-interactively.

Environment:
  WOWFIX_CURSEFORGE_API_KEY   API key for the CurseForge Core API
`

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (isZipArg(args[0]) || isSourceArg(args[0])) {
		args = append([]string{"install"}, args...)
	}
	if err := run(args); err != nil {
		fmt.Fprintln(os.Stderr, "wowfix: "+err.Error())
		os.Exit(1)
	}
}

func isZipArg(s string) bool {
	return strings.EqualFold(filepath.Ext(s), ".zip")
}

// isSourceArg reports whether the argument is an install source rather
// than a local archive: a URL or a GitHub "owner/repo" pair. Windows
// drive paths ("C:\...") never contain "/", so they are not confused
// with sources.
func isSourceArg(s string) bool {
	return !isZipArg(s) && strings.Contains(s, "/")
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version", "-v", "--version":
		fmt.Printf("wowfix %s (commit %s, built %s)\n", version, commit, date)
		return nil
	case "scan":
		return runScan(rest)
	case "fix":
		return runFix(rest)
	case "install":
		return runInstall(rest)
	case "validate":
		return runValidate(rest)
	case "list":
		return runList(rest)
	case "search":
		return runSearch(rest)
	case "update":
		return runUpdate(rest)
	case "sources":
		return runSources(rest)
	case "backup":
		return runBackup(rest)
	case "restore":
		return runRestore(rest)
	case "doctor":
		return runDoctor(rest)
	case "config":
		return runConfig(rest)
	case "profile":
		return runProfile(rest)
	case "savedvars":
		return runSavedVars(rest)
	case "export":
		return runExport(rest)
	case "import":
		return runImport(rest)
	case "preview":
		return runPreview()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}

// cliOptions carries the shared flag values for every command.
type cliOptions struct {
	path string
	yes  bool
	json bool
}

// parseCLIOptions splits the shared flags from the positional
// arguments. Flags may appear anywhere on the command line.
func parseCLIOptions(args []string) (*cliOptions, []string, error) {
	opts := &cliOptions{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--path" || a == "-path":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--path requires a directory argument")
			}
			i++
			opts.path = args[i]
		case a == "--yes" || a == "-y":
			opts.yes = true
		case a == "--json" || a == "-j":
			opts.json = true
		case strings.HasPrefix(a, "-"):
			return nil, nil, fmt.Errorf("unknown flag %q", a)
		default:
			rest = append(rest, a)
		}
	}
	return opts, rest, nil
}

// environment bundles the services a command needs: the persisted
// config, the detected installation and the shared logger and profile.
type environment struct {
	store   *config.Store
	cfg     *config.Config
	install *detector.Installation
	log     *logger.Logger
	profile *models.Profile
}

// newEnvironment loads the config, resolves the installation (the
// --path flag, the saved wow_path, or auto-detection) and picks the
// active profile.
func newEnvironment(opts *cliOptions) (*environment, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	profile := models.ProfileByID(cfg.Profile)
	if profile == nil {
		profile = models.DefaultProfile()
	}

	var install *detector.Installation
	root := opts.path
	if root == "" {
		root = cfg.WoWPath
	}
	if root != "" {
		inst, err := detector.DetectPath(root)
		if err != nil {
			return nil, err
		}
		install = inst
	} else {
		installs, err := detector.AutoDetect(context.Background())
		if err != nil {
			return nil, err
		}
		if len(installs) > 0 {
			install = &installs[0]
		}
	}
	if install != nil && install.ProfileID != "" {
		if p := models.ProfileByID(install.ProfileID); p != nil {
			profile = p
		}
	}

	return &environment{
		store:   store,
		cfg:     cfg,
		install: install,
		log:     logger.New(500),
		profile: profile,
	}, nil
}

// scan runs a fresh scan of the environment's AddOns directory.
func (e *environment) scan(ctx context.Context) (*models.ScanResult, error) {
	if e.install == nil {
		return nil, fmt.Errorf("no WoW installation found")
	}
	return scanner.New(e.install.AddonsPath, e.profile).Scan(ctx)
}

// backupRoot returns where snapshots live: the saved backups_dir, the
// Backups folder next to the game, or the config directory as a last
// resort.
func (e *environment) backupRoot() (string, error) {
	if e.cfg.BackupsDir != "" {
		return e.cfg.BackupsDir, nil
	}
	if e.install != nil && e.install.Root != "" {
		return filepath.Join(e.install.Root, "Backups"), nil
	}
	return filepath.Join(e.store.Dir(), "backups"), nil
}

// confirm asks a yes/no question on stderr and reads the answer from
// stdin. It returns false when stdin is not available or the answer is
// not an explicit yes.
func confirm(format string, args ...any) bool {
	fmt.Fprintf(os.Stderr, format+" [y/N] ", args...)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func runScan(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("scan takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	res, err := env.scan(context.Background())
	if err != nil {
		return err
	}
	env.cfg.LastScan = time.Now()
	_ = env.store.Save(env.cfg)

	if opts.json {
		return printScanJSON(res)
	}
	printScanReport(res, env.profile)
	return nil
}

func printScanReport(res *models.ScanResult, profile *models.Profile) {
	total, problems, errs := res.Stats()
	fmt.Printf("%d addon(s): %d problem(s), %d error(s).\n", total, problems, errs)
	if profile != nil {
		fmt.Printf("Profile: %s (interface %d)\n", profile.Name, profile.Interface)
	}
	fmt.Printf("%-26s %-46s %s\n", "ADDON", "PROBLEM", "FIX")
	fmt.Println(strings.Repeat("-", 100))
	for _, a := range res.Addons {
		fmt.Println(statusLine(a))
	}
	for _, e := range res.Errors {
		fmt.Fprintln(os.Stderr, "scan:", e)
	}
}

func printScanJSON(res *models.ScanResult) error {
	profile := ""
	if res.Profile != nil {
		profile = res.Profile.ID
	}
	return printJSON(map[string]any{
		"addons_dir": res.AddonsDir,
		"profile":    profile,
		"scanned_at": res.ScannedAt,
		"addons":     addonsJSON(res.Addons),
		"errors":     errStrings(res.Errors),
	})
}

func errStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			out = append(out, e.Error())
		}
	}
	return out
}

func addonsJSON(addons []*models.Addon) []map[string]any {
	out := make([]map[string]any, 0, len(addons))
	for _, a := range addons {
		row := map[string]any{
			"folder": a.FolderName,
			"name":   a.BaseName,
			"status": string(a.Status),
			"nested": a.Nested,
			"size":   a.SizeBytes,
			"issues": issuesJSON(a.Issues),
		}
		if toc := a.PrimaryTOC(); toc != nil {
			row["toc"] = map[string]any{
				"name":      toc.Name,
				"interface": toc.Interface,
				"version":   toc.Version,
				"title":     toc.Title,
			}
		}
		out = append(out, row)
	}
	return out
}

func issuesJSON(issues []*models.Issue) []map[string]any {
	out := make([]map[string]any, 0, len(issues))
	for _, i := range issues {
		out = append(out, map[string]any{
			"kind":       string(i.Kind),
			"severity":   string(i.Severity),
			"message":    i.Message,
			"suggestion": i.Suggestion,
			"action":     string(i.Action),
		})
	}
	return out
}

func runFix(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("fix takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	res, err := env.scan(context.Background())
	if err != nil {
		return err
	}

	fopts := fixer.Options{
		AddonsDir:        env.install.AddonsPath,
		Profile:          env.profile,
		Log:              env.log,
		TrashFallbackDir: filepath.Join(env.store.Dir(), "trash"),
	}
	if env.cfg.AutoBackup {
		root, err := env.backupRoot()
		if err != nil {
			root = filepath.Join(env.store.Dir(), "backups")
		}
		fopts.Backups = backup.New(root, env.log)
	}
	if opts.yes {
		fopts.Confirm = func(string, ...any) bool { return true }
	} else {
		fopts.Confirm = confirm
	}
	f := fixer.New(fopts)
	results := f.FixAll(context.Background(), res.Addons)

	if opts.json {
		rows := make([]map[string]any, 0, len(results))
		for _, r := range results {
			row := map[string]any{
				"addon":   r.Addon,
				"action":  r.Action,
				"ok":      r.OK,
				"message": r.Message,
			}
			if r.Err != nil {
				row["error"] = r.Err.Error()
			}
			rows = append(rows, row)
		}
		return printJSON(map[string]any{"fixes": rows})
	}

	ok, failed := 0, 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		} else if r.OK {
			ok++
		}
		fmt.Println(r.String())
	}
	fmt.Printf("%d fixed, %d failed.\n", ok, failed)
	if failed > 0 {
		return fmt.Errorf("%d fix(es) failed", failed)
	}
	return nil
}

func runInstall(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("install requires an addon archive or a source (URL or owner/repo)")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}

	source := rest[0]
	if !isZipArg(source) || !utils.Exists(source) {
		// A URL or provider id ("owner/repo") goes through the catalog.
		return runInstallFromSource(env, opts, source)
	}

	// Existing ZIP archive: classic file install.
	iopts := installer.Options{
		AddonsDir: env.install.AddonsPath,
		Profile:   env.profile,
		Log:       env.log,
	}
	if env.cfg.AutoBackup {
		root, err := env.backupRoot()
		if err != nil {
			root = filepath.Join(env.store.Dir(), "backups")
		}
		iopts.Backups = backup.New(root, env.log)
	}
	if !opts.yes {
		iopts.Confirm = func(name string) bool {
			return confirm("Replace existing folder %q?", name)
		}
	}
	inst := installer.New(iopts)

	fmt.Printf("Installing %s\n", source)
	res, err := inst.Install(context.Background(), source)
	if err != nil {
		return err
	}
	for _, e := range res.Errors {
		fmt.Fprintln(os.Stderr, "install:", e)
	}
	fmt.Printf("%d installed, %d replaced, %d skipped, %d error(s).\n",
		len(res.Installed), len(res.Replaced), len(res.Skipped), len(res.Errors))

	// Re-validate the freshly installed folders so remaining problems
	// surface immediately instead of on the next scan.
	if fresh, err := env.scan(context.Background()); err == nil {
		byFolder := make(map[string]*models.Addon, len(fresh.Addons))
		for _, a := range fresh.Addons {
			byFolder[strings.ToLower(a.FolderName)] = a
		}
		var problems []string
		for _, f := range res.Installed {
			if a, ok := byFolder[strings.ToLower(f)]; ok && len(a.Issues) > 0 {
				problems = append(problems, f)
			}
		}
		if len(problems) > 0 {
			fmt.Println("Re-validating: some installed addons still have issues:")
			for _, p := range problems {
				fmt.Println("  " + p)
			}
		}
	}

	if opts.json {
		return printJSON(map[string]any{
			"installed": res.Installed,
			"replaced":  res.Replaced,
			"skipped":   res.Skipped,
			"errors":    errStrings(res.Errors),
		})
	}
	return nil
}

func runValidate(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("validate takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	res, err := env.scan(context.Background())
	if err != nil {
		return err
	}

	rows := make([]map[string]any, 0, len(res.Addons))
	for _, a := range res.Addons {
		compat := classifyTOC(a, env.profile)
		row := map[string]any{
			"folder":   a.FolderName,
			"toc":      "",
			"expected": env.profile.Interface,
			"detected": -1,
			"status":   string(compat.Status),
			"label":    compat.Label,
		}
		if compat.TOC != nil {
			row["toc"] = compat.TOC.Name
			row["detected"] = compat.TOC.Interface
		}
		rows = append(rows, row)
	}
	if opts.json {
		return printJSON(map[string]any{"profile": env.profile.ID, "addons": rows})
	}

	fmt.Printf("Profile: %s (interface %d)\n", env.profile.Name, env.profile.Interface)
	fmt.Printf("%-24s %-20s %-10s %-10s %s\n", "ADDON", "TOC", "Expected", "Detected", "Status")
	fmt.Println(strings.Repeat("-", 90))
	for _, a := range res.Addons {
		compat := classifyTOC(a, env.profile)
		tocName, detected := "-", "-"
		if compat.TOC != nil {
			tocName = compat.TOC.Name
			if compat.TOC.Interface >= 0 {
				detected = strconv.Itoa(compat.TOC.Interface)
			}
		}
		fmt.Printf("%-24s %-20s %-10d %-10s %s\n",
			truncateRunes(a.FolderName, 23), truncateRunes(tocName, 19),
			env.profile.Interface, detected, compat.Label)
	}
	return nil
}

// statusLine renders one addon row for the scan/list reports.
func statusLine(a *models.Addon) string {
	glyph := "✔"
	switch a.Status {
	case models.StatusError:
		glyph = "✖"
	case models.StatusWarn:
		glyph = "⚠"
	}
	problem, fix := "-", "-"
	if len(a.Issues) > 0 {
		problem = a.Issues[0].Message
		fix = a.Issues[0].Action.Label()
	}
	return fmt.Sprintf("%s %s  %s  %s", glyph, padText(a.FolderName, 24), padText(problem, 44), fix)
}

// classifyTOC returns the compatibility verdict for an addon's primary
// TOC against the profile, or an "unknown" verdict when the addon has
// no parseable TOC.
func classifyTOC(a *models.Addon, profile *models.Profile) validator.Compatibility {
	toc := a.PrimaryTOC()
	if toc == nil && len(a.TOCs) > 0 {
		toc = a.TOCs[0]
	}
	if toc == nil {
		return validator.Compatibility{
			Status: models.CompatUnknown,
			Label:  "No TOC",
		}
	}
	return validator.ValidateTOC(toc, profile)
}

func runList(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("list takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	res, err := env.scan(context.Background())
	if err != nil {
		return err
	}
	models.SortAddons(res.Addons)

	if opts.json {
		return printJSON(map[string]any{
			"addons": addonsJSON(res.Addons),
			"errors": errStrings(res.Errors),
		})
	}
	printScanReport(res, env.profile)
	return nil
}

func runBackup(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("backup takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	root, err := env.backupRoot()
	if err != nil {
		root = filepath.Join(env.store.Dir(), "backups")
	}
	m := backup.New(root, env.log)
	id, err := m.BackupDir(env.install.AddonsPath, "manual backup")
	if err != nil {
		return err
	}
	fmt.Printf("Backup created: %s\n", id)
	return nil
}

func runRestore(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	root, err := env.backupRoot()
	if err != nil {
		root = filepath.Join(env.store.Dir(), "backups")
	}
	m := backup.New(root, env.log)

	if len(rest) == 0 {
		infos, err := m.List()
		if err != nil {
			return err
		}
		if len(infos) == 0 {
			fmt.Println("No backups found.")
			return nil
		}
		fmt.Printf("%d snapshot(s)\n", len(infos))
		for _, in := range infos {
			fmt.Printf("  %s  %s\n", in.ID, in.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		return nil
	}

	id := rest[0]
	restored, skipped, err := m.Restore(id, func(originalPath string) bool {
		if opts.yes {
			return true
		}
		return confirm("Replace existing folder %q?", originalPath)
	})
	if err != nil {
		return err
	}
	for _, p := range restored {
		fmt.Printf("  restored %s\n", p)
	}
	for _, p := range skipped {
		fmt.Printf("  skipped %s\n", p)
	}
	if opts.json {
		return printJSON(map[string]any{"restored": restored, "skipped": skipped})
	}
	return nil
}

func runDoctor(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("doctor takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}

	fmt.Println("wowfix doctor report")
	fmt.Printf("  config: %s\n", env.store.Path())

	if p := models.ProfileByID(env.cfg.Profile); p == nil {
		fmt.Printf("  profile: ✖ unknown profile %q (valid: %s)\n",
			env.cfg.Profile, strings.Join(profileIDs(), ", "))
	} else {
		fmt.Printf("  profile: ✔ %s\n", p.ID)
	}
	switch env.cfg.Theme {
	case "dark", "light":
		fmt.Printf("  theme: ✔ %s\n", env.cfg.Theme)
	default:
		fmt.Printf("  theme: ✖ must be dark or light (got %q)\n", env.cfg.Theme)
	}
	if env.cfg.Collection == "" {
		fmt.Println("  collection: ✔ (none set)")
	} else if utils.Exists(filepath.Join(collectionsDir(env), env.cfg.Collection+".json")) {
		fmt.Printf("  collection: ✔ %s\n", env.cfg.Collection)
	} else {
		fmt.Printf("  collection: ✖ %q not found in %s\n", env.cfg.Collection, collectionsDir(env))
	}

	if env.install == nil {
		fmt.Println("  install: none found (use --path or set wow_path in config)")
	} else {
		conf := env.install.Confidence
		if conf == "" {
			conf = "unknown"
		}
		fmt.Printf("  install: %s\n", env.install.AddonsPath)
		fmt.Printf("  flavor:  %q (confidence %s)\n", env.install.Flavor, conf)
		if env.install.Exe != "" {
			fmt.Printf("  exe:     %s (version %s)\n", env.install.Exe, env.install.Version)
		}
		if err := utils.IsWritable(env.install.AddonsPath); err != nil {
			fmt.Printf("  permissions: AddOns directory is not writable: %v\n", err)
		} else {
			fmt.Println("  permissions: AddOns directory is writable")
		}
		if res, err := env.scan(context.Background()); err == nil {
			total, problems, errs := res.Stats()
			fmt.Printf("  %d addon(s): %d problem(s), %d error(s).\n", total, problems, errs)
		}
	}

	root, err := env.backupRoot()
	if err != nil {
		root = filepath.Join(env.store.Dir(), "backups")
	}
	if infos, err := backup.New(root, env.log).List(); err == nil {
		fmt.Printf("  backup history: %d snapshot(s)\n", len(infos))
	}

	trashDir := filepath.Join(env.store.Dir(), "trash")
	if err := utils.EnsureDir(trashDir); err != nil {
		fmt.Printf("  trash fallback: %v\n", err)
	} else if err := utils.IsWritable(trashDir); err != nil {
		fmt.Printf("  trash fallback: not writable (%v)\n", err)
	} else {
		fmt.Printf("  trash fallback: %s (writable)\n", trashDir)
	}

	regPath, err := catalog.DefaultPath()
	if err != nil {
		fmt.Printf("  registry: %v\n", err)
	} else if !utils.Exists(regPath) {
		fmt.Println("  registry: none (addons installed via catalog will appear here)")
	} else if reg, err := catalog.NewRegistry(regPath); err != nil {
		fmt.Printf("  registry: %v\n", err)
	} else {
		fmt.Printf("  registry: OK (%d entries)\n", len(reg.Entries()))
	}

	colsDir := collectionsDir(env)
	switch {
	case !utils.IsDir(colsDir):
		if env.cfg.CollectionsDir != "" {
			fmt.Printf("  collections: not configured (%s does not exist)\n", colsDir)
		} else {
			fmt.Printf("  collections: not configured (%s)\n", colsDir)
		}
	default:
		if err := utils.IsWritable(colsDir); err != nil {
			fmt.Printf("  collections: %s (not writable: %v)\n", colsDir, err)
		} else {
			fmt.Printf("  collections: %s (writable)\n", colsDir)
		}
	}

	if env.install == nil {
		fmt.Println("  savedvars: WTF not found")
	} else {
		wtf := wtfRoot(env.install.Root, env.install.Flavor)
		if !utils.IsDir(wtf) {
			fmt.Println("  savedvars: WTF not found")
		} else {
			fmt.Printf("  savedvars: %d account(s)\n", len(savedvars.New(wtf, nil).Accounts()))
		}
	}

	if env.cfg.Theme != "dark" && env.cfg.Theme != "light" {
		fmt.Println("  warning: theme must be dark or light")
	}
	return nil
}

// padText pads s to width (measured in runes), truncating with "..."
// when it is longer. Table columns stay aligned for non-ASCII names.
func padText(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		if width < 3 {
			return string(r[:width])
		}
		return string(r[:width-3]) + "..."
	}
	if len(r) < width {
		return s + strings.Repeat(" ", width-len(r))
	}
	return s
}

const configKeysHelp = "keys: wow_path, flavor, profile, theme, autobackup, confirmations, backups_dir, curseforge_api_key, collection, collections_dir"

func runConfig(args []string) error {
	opts, rest, err := parseCLIOptions(args)
	if err != nil {
		return err
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}

	if len(rest) == 0 {
		cfg := env.cfg
		if opts.json {
			return printJSON(map[string]any{
				"wow_path":           cfg.WoWPath,
				"flavor":             cfg.Flavor,
				"profile":            cfg.Profile,
				"theme":              cfg.Theme,
				"auto_backup":        cfg.AutoBackup,
				"confirmations":      cfg.Confirmations,
				"backups_dir":        cfg.BackupsDir,
				"curseforge_api_key": cfg.CurseForgeAPIKey,
				"collection":         cfg.Collection,
				"collections_dir":    cfg.CollectionsDir,
			})
		}
		fmt.Printf("wow_path:           %s\n", cfg.WoWPath)
		fmt.Printf("flavor:             %s\n", cfg.Flavor)
		fmt.Printf("profile:            %s\n", cfg.Profile)
		fmt.Printf("theme:              %s\n", cfg.Theme)
		fmt.Printf("auto_backup:        %t\n", cfg.AutoBackup)
		fmt.Printf("confirmations:      %t\n", cfg.Confirmations)
		fmt.Printf("backups_dir:        %s\n", cfg.BackupsDir)
		fmt.Printf("curseforge_api_key: %s\n", cfg.CurseForgeAPIKey)
		fmt.Printf("collection:         %s\n", cfg.Collection)
		fmt.Printf("collections_dir:    %s\n", cfg.CollectionsDir)
		fmt.Printf("\n%s\n", configKeysHelp)
		return nil
	}

	if rest[0] == "set" && len(rest) == 3 {
		return setConfigValue(env, rest[1], rest[2])
	}
	return fmt.Errorf("usage: wowfix config [set <key> <val>]")
}

func setConfigValue(env *environment, key, value string) error {
	cfg := env.cfg
	switch key {
	case "wow_path":
		if err := validateWowPath(value); err != nil {
			return err
		}
		cfg.WoWPath = value
	case "flavor":
		cfg.Flavor = value
	case "profile":
		if models.ProfileByID(value) == nil {
			return fmt.Errorf("unknown profile %q (valid: %s)", value, strings.Join(profileIDs(), ", "))
		}
		cfg.Profile = value
	case "theme":
		if value != "dark" && value != "light" {
			return fmt.Errorf("theme must be dark or light")
		}
		cfg.Theme = value
	case "autobackup", "auto_backup":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("autobackup must be true or false")
		}
		cfg.AutoBackup = b
	case "confirmations":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("confirmations must be true or false")
		}
		cfg.Confirmations = b
	case "backups_dir":
		cfg.BackupsDir = value
	case "curseforge_api_key":
		cfg.CurseForgeAPIKey = value
	case "collection":
		cfg.Collection = value
	case "collections_dir":
		cfg.CollectionsDir = value
	default:
		return fmt.Errorf("unknown key %q\n%s", key, configKeysHelp)
	}
	if err := env.store.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("config: %s = %s\n", key, value)
	return nil
}

func profileIDs() []string {
	ids := make([]string, 0, len(models.Profiles))
	for _, p := range models.Profiles {
		ids = append(ids, p.ID)
	}
	return ids
}

// validateWowPath rejects paths that do not contain an AddOns directory.
func validateWowPath(path string) error {
	if _, err := detector.DetectPath(path); err != nil {
		return err
	}
	return nil
}

func runPreview() error {
	fmt.Print(ui.RenderPreview())
	return nil
}

func runTUI() error {
	store, err := config.NewStore()
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	log := logger.New(500)
	defer log.Close()

	app := ui.NewApp(cfg, store, log)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// printJSON renders v as indented JSON on stdout.
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
