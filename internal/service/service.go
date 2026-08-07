// Package service exposes the wowfix core (scan, fix, validate, install)
// as a Wails-bound facade. Every method returns plain JSON-marshalled
// DTOs so the frontend never sees internal model types.
//
// The package deliberately performs no prompting: confirmation flags
// arrive from the frontend as method arguments (allowDestructive /
// allowReplace). It never touches the filesystem except through the
// core packages and never blocks on user input.
package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/fixer"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/profiles"
	"github.com/wowfix/wowfix/internal/scanner"
	"github.com/wowfix/wowfix/internal/validator"
)

// Service is the Wails-bound backend facade.
type Service struct {
	store *config.Store
	log   *logger.Logger
	// Version is the build version reported in AppState; defaults to "dev".
	Version string
	// httpClient is used for catalog provider traffic; nil uses
	// http.DefaultClient. Tests point it at an in-memory mock.
	httpClient *http.Client
	// registryPath overrides the catalog registry location; empty uses
	// catalog.DefaultPath(). Tests isolate the registry in a temp dir.
	registryPath string
	// enabledProviders selects the catalog providers; nil enables all.
	// Tests enable only the provider they mock.
	enabledProviders map[string]bool
}

// New returns a Service backed by store. A nil store falls back to the
// platform user config store.
func New(store *config.Store) *Service {
	if store == nil {
		if s, err := config.NewStore(); err == nil {
			store = s
		} else {
			// Last resort: a writable temp store so the GUI still runs.
			store = config.NewStoreAt(filepath.Join(os.TempDir(), "wowfix-config.json"))
		}
	}
	return &Service{store: store, log: logger.New(500), Version: "dev"}
}

// env bundles the resolved runtime context, mirroring cmd/wowfix's
// newEnvironment: config, detected install and active profile.
type env struct {
	store   *config.Store
	cfg     *config.Config
	install *detector.Installation
	profile *models.Profile
}

// resolveInstall resolves the configured installation, falling back to
// auto-detection when no path is saved. It is strict: any resolution
// failure is returned as an error for callers that require an install.
func (s *Service) resolveInstall(cfg *config.Config) (*detector.Installation, error) {
	if cfg.WoWPath != "" {
		return detector.DetectPath(cfg.WoWPath)
	}
	installs, err := detector.AutoDetect(context.Background())
	if err != nil {
		return nil, err
	}
	if len(installs) == 0 {
		return nil, nil
	}
	return &installs[0], nil
}

// env loads the config, resolves the installation (the saved wow_path,
// or auto-detection) and picks the active profile.
func (s *Service) env() (*env, error) {
	cfg, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	install, err := s.resolveInstall(cfg)
	if err != nil {
		return nil, err
	}
	return s.buildEnv(cfg, install), nil
}

// buildEnv assembles the runtime context from a loaded config and a
// possibly nil install, applying the install's profile override.
func (s *Service) buildEnv(cfg *config.Config, install *detector.Installation) *env {
	profile := models.ProfileByID(cfg.Profile)
	if profile == nil {
		profile = models.DefaultProfile()
	}
	if install != nil && install.ProfileID != "" {
		if p := models.ProfileByID(install.ProfileID); p != nil {
			profile = p
		}
	}
	return &env{store: s.store, cfg: cfg, install: install, profile: profile}
}

// requireInstall returns the resolved env or a clear error when no
// installation is available.
func (s *Service) requireInstall() (*env, error) {
	e, err := s.env()
	if err != nil {
		return nil, err
	}
	if e.install == nil {
		return nil, fmt.Errorf("no WoW installation found")
	}
	return e, nil
}

// scan runs a fresh scan of the environment's AddOns directory,
// creating it when the install has none yet.
func (s *Service) scan(e *env) (*models.ScanResult, error) {
	if _, err := detector.EnsureAddons(e.install); err != nil {
		return nil, err
	}
	return scanner.New(e.install.AddonsPath, e.profile).Scan(context.Background())
}

// backupRoot returns where snapshots live: the saved backups_dir, the
// Backups folder next to the game, or the config directory as a last
// resort.
func (s *Service) backupRoot(e *env) string {
	if e.cfg.BackupsDir != "" {
		return e.cfg.BackupsDir
	}
	if e.install != nil && e.install.Root != "" {
		return filepath.Join(e.install.Root, "Backups")
	}
	return filepath.Join(s.store.Dir(), "backups")
}

// backupRootFor returns where snapshots live for an arbitrary AddOns
// directory: the saved backups_dir, the Backups folder next to the
// install that owns the directory, or the config directory. It
// mirrors backupRoot for cross-install operations where the active
// install's root does not own the target addons dir.
func (s *Service) backupRootFor(e *env, addonsDir string) string {
	if e.cfg.BackupsDir != "" {
		return e.cfg.BackupsDir
	}
	if inst, err := detector.DetectPath(addonsDir); err == nil && inst.Root != "" {
		return filepath.Join(inst.Root, "Backups")
	}
	return filepath.Join(s.store.Dir(), "backups")
}

// profilesFor builds the collection manager for the environment,
// mirroring cmd/wowfix's newProfileManager: collections live at the
// config override or <config dir>/collections, and the manager gets
// the logger plus (when auto_backup is on) a backup manager for
// pre-switch snapshots.
func (s *Service) profilesFor(e *env) (*profiles.Manager, error) {
	dir := e.cfg.CollectionsDir
	if dir == "" {
		dir = filepath.Join(s.store.Dir(), "collections")
	}
	m, err := profiles.NewManager(dir, e.install.AddonsPath)
	if err != nil {
		return nil, err
	}
	m.Log = s.log
	if e.cfg.AutoBackup {
		m.Backups = backup.New(s.backupRoot(e), s.log)
	}
	return m, nil
}

// fixerOptions assembles a fixer.Options for the environment. The
// confirmation hook is replaced by the allowDestructive flag.
func (s *Service) fixerOptions(e *env, allowDestructive bool) fixer.Options {
	opts := fixer.Options{
		AddonsDir:        e.install.AddonsPath,
		Profile:          e.profile,
		Log:              s.log,
		Confirm:          func(format string, args ...any) bool { return allowDestructive },
		TrashFallbackDir: filepath.Join(s.store.Dir(), "trash"),
	}
	if e.cfg.AutoBackup {
		opts.Backups = backup.New(s.backupRoot(e), s.log)
	}
	return opts
}

// installerOptions assembles an installer.Options for the environment.
func (s *Service) installerOptions(e *env, allowReplace bool) installer.Options {
	opts := installer.Options{
		AddonsDir: e.install.AddonsPath,
		Profile:   e.profile,
		Log:       s.log,
		Confirm:   func(addonName string) bool { return allowReplace },
	}
	if e.cfg.AutoBackup {
		opts.Backups = backup.New(s.backupRoot(e), s.log)
	}
	return opts
}

// classifyTOC returns the compatibility verdict for an addon's primary
// TOC against the profile, or an "unknown" verdict when the addon has
// no parseable TOC (mirrors cmd/wowfix classifyTOC).
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

// errStrings flattens errors into their messages, skipping nils.
func errStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			out = append(out, e.Error())
		}
	}
	return out
}

// catalogFor builds a catalog wired with the environment's registry,
// backups, logger, profile and CurseForge API key, mirroring
// cmd/wowfix's newCatalog. The registry lives at the conventional
// location: a missing file yields an empty registry, a corrupt one an
// error.
func (s *Service) catalogFor(e *env) (*catalog.Catalog, error) {
	path := s.registryPath
	if path == "" {
		var err error
		if path, err = catalog.DefaultPath(); err != nil {
			return nil, err
		}
	}
	reg, err := catalog.NewRegistry(path)
	if err != nil {
		return nil, err
	}
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	cat, err := catalog.New(s.enabledProviders, client)
	if err != nil {
		return nil, err
	}
	cat.Reg = reg
	cat.Backups = backup.New(s.backupRoot(e), s.log)
	cat.Log = s.log
	cat.Profile = e.profile
	// The WOWFIX_CURSEFORGE_API_KEY environment variable takes
	// precedence; the saved config value is the fallback (the catalog
	// checks the env var itself, so this field only needs the config).
	key := os.Getenv("WOWFIX_CURSEFORGE_API_KEY")
	if key == "" {
		key = e.cfg.CurseForgeAPIKey
	}
	cat.CurseForgeAPIKey = key
	return cat, nil
}

// flavorLabel describes an update's game-family mismatch as a short
// human string, e.g. "retail addon · profile wrath". It is empty when
// the game version is unknown.
func flavorLabel(gameVersion string, profile *models.Profile) string {
	addon := strings.ToLower(strings.TrimSpace(gameVersion))
	if profile == nil || addon == "" {
		return ""
	}
	return fmt.Sprintf("%s addon · profile %s", addon, strings.ToLower(models.FamilyLabel(profile.Family)))
}

// folderExists reports whether addonsDir contains a directory named
// name, case-insensitively (an install may differ in case from the
// registry's tracked folder).
func folderExists(addonsDir, name string) bool {
	if _, err := os.Stat(filepath.Join(addonsDir, name)); err == nil {
		return true
	}
	entries, err := os.ReadDir(addonsDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), name) {
			return true
		}
	}
	return false
}

// registryEntries loads the tracked registry entries keyed by lowercase
// folder. Integrity reporting is best-effort: an unreadable or missing
// registry yields an empty map so scans never fail over provenance.
func (s *Service) registryEntries(e *env) map[string]catalog.Entry {
	path := s.registryPath
	if path == "" {
		var err error
		if path, err = catalog.DefaultPath(); err != nil {
			return nil
		}
	}
	reg, err := catalog.NewRegistry(path)
	if err != nil {
		return nil
	}
	out := make(map[string]catalog.Entry, len(reg.Entries()))
	for _, ent := range reg.Entries() {
		out[strings.ToLower(ent.Folder)] = ent
	}
	return out
}

// trackedEntry returns the registry entry for folder, matched
// case-insensitively, and whether one exists.
func (s *Service) trackedEntry(e *env, folder string) (catalog.Entry, bool) {
	entry, ok := s.registryEntries(e)[strings.ToLower(folder)]
	return entry, ok
}

// DTOs ---------------------------------------------------------------------

// AppState is the initial UI snapshot.
type AppState struct {
	Version       string `json:"version"`
	WoWPath       string `json:"wow_path"`
	Flavor        string `json:"flavor"`
	AddonsDir     string `json:"addons_dir"`
	ProfileID     string `json:"profile_id"`
	ProfileName   string `json:"profile_name"`
	HasInstall    bool   `json:"has_install"`
	AutoBackup    bool   `json:"auto_backup"`
	Confirmations bool   `json:"confirmations"`
}

// Install is one detected (or selected) WoW installation.
type Install struct {
	Root       string `json:"root"`
	Flavor     string `json:"flavor"`
	AddonsPath string `json:"addons_path"`
	Exe        string `json:"exe"`
	Version    string `json:"version"`
	ProfileID  string `json:"profile_id"`
	Confidence string `json:"confidence"`
}

func toInstall(i detector.Installation) Install {
	return Install{
		Root:       i.Root,
		Flavor:     i.Flavor,
		AddonsPath: i.AddonsPath,
		Exe:        i.Exe,
		Version:    i.Version,
		ProfileID:  i.ProfileID,
		Confidence: i.Confidence,
	}
}

// Profile is a supported game-version profile.
type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Family    string `json:"family"`
	Interface int    `json:"interface"`
}

// TOC summarizes the primary TOC of an addon.
type TOC struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Interface int    `json:"interface"`
	Version   string `json:"version"`
}

// Issue is one problem attached to an addon.
type Issue struct {
	Kind          string   `json:"kind"`
	Severity      string   `json:"severity"`
	Message       string   `json:"message"`
	Suggestion    string   `json:"suggestion"`
	Action        string   `json:"action"`
	ActionLabel   string   `json:"action_label"`
	Options       []string `json:"options"`
	SuggestedName string   `json:"suggested_name"`
}

// Compat is a TOC-vs-profile compatibility verdict.
type Compat struct {
	FolderName string `json:"folder_name"`
	TOC        string `json:"toc"`
	Expected   int    `json:"expected"`
	Detected   int    `json:"detected"`
	Status     string `json:"status"`
	Label      string `json:"label"`
}

// Addon is one addon in the scan report.
type Addon struct {
	FolderName    string   `json:"folder_name"`
	BaseName      string   `json:"base_name"`
	SuggestedName string   `json:"suggested_name"`
	Status        string   `json:"status"`
	Nested        bool     `json:"nested"`
	SizeBytes     int64    `json:"size_bytes"`
	Fixable       bool     `json:"fixable"`
	Health        int      `json:"health"`
	TOC           *TOC     `json:"toc"`
	Issues        []Issue  `json:"issues"`
	Compat        []Compat `json:"compat"`
	// Tracked reports whether the addon was installed through the
	// catalog and is recorded in the registry. Drifted reports that a
	// tracked addon's folder no longer matches the manifest checksum
	// recorded at install/update time; entries without a recorded
	// checksum (pre-integrity installs) are never drifted.
	Tracked       bool   `json:"tracked"`
	Drifted       bool   `json:"drifted"`
	TrackedSource string `json:"tracked_source,omitempty"`
}

// Stats summarizes a scan.
type Stats struct {
	Total    int `json:"total"`
	Problems int `json:"problems"`
	Errors   int `json:"errors"`
}

// ScanResult is the full scan report.
type ScanResult struct {
	AddonsDir string    `json:"addons_dir"`
	ProfileID string    `json:"profile_id"`
	ScannedAt time.Time `json:"scanned_at"`
	Addons    []Addon   `json:"addons"`
	Errors    []string  `json:"errors"`
	Stats     Stats     `json:"stats"`
}

// ValidateResult is the per-addon compatibility table.
type ValidateResult struct {
	ProfileID string   `json:"profile_id"`
	Expected  int      `json:"expected"`
	Addons    []Compat `json:"addons"`
}

// Fix is the outcome of fixing one issue.
type Fix struct {
	Addon   string `json:"addon"`
	Action  string `json:"action"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// FixBatch is the outcome of a fix run.
type FixBatch struct {
	Fixes  []Fix `json:"fixes"`
	Fixed  int   `json:"fixed"`
	Failed int   `json:"failed"`
}

// InstallResult is the outcome of installing a ZIP archive.
type InstallResult struct {
	Installed []string `json:"installed"`
	Replaced  []string `json:"replaced"`
	Skipped   []string `json:"skipped"`
	Errors    []string `json:"errors"`
}

// UpdateInfo is one pending addon update.
type UpdateInfo struct {
	Folder         string `json:"folder"`
	Title          string `json:"title"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	Source         string `json:"source"`
	FlavorMismatch bool   `json:"flavor_mismatch"`
	FlavorLabel    string `json:"flavor_label"`
}

// UpdatesResult is the outcome of a catalog update check.
type UpdatesResult struct {
	Updates   []UpdateInfo `json:"updates"`
	Errors    []string     `json:"errors"`
	CheckedAt string       `json:"checked_at"`
}

// ApplyEntry is the outcome of applying one update.
type ApplyEntry struct {
	Folder  string `json:"folder"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// ApplyBatch is the outcome of applying one or more updates.
type ApplyBatch struct {
	Applied      []ApplyEntry `json:"applied"`
	AppliedCount int          `json:"applied_count"`
	FailedCount  int          `json:"failed_count"`
	// errors carries hard per-install failures (e.g. catalog setup)
	// that are not per-addon outcomes. It is not part of the JSON
	// contract; SyncUpdatesToAll consumes it for its per-install
	// error lists.
	errors []string
}

// CollectionInfo is one addon collection in the list view.
type CollectionInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AddonCount int    `json:"addon_count"`
	Active     bool   `json:"active"`
}

// CollectionsResult is the full collection list with the active id.
type CollectionsResult struct {
	Collections []CollectionInfo `json:"collections"`
	ActiveID    string           `json:"active_id"`
}

// CollectionAddon is one addon's desired state inside a collection.
type CollectionAddon struct {
	Folder  string `json:"folder"`
	Enabled bool   `json:"enabled"`
}

// CollectionDetail is one collection's full addon state table.
type CollectionDetail struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Addons []CollectionAddon `json:"addons"`
}

// SwitchResult is the outcome of activating a collection.
type SwitchResult struct {
	Applied []string `json:"applied"`
	Message string   `json:"message"`
}

// InstallStatus is one detected installation with live scan health.
type InstallStatus struct {
	Root       string `json:"root"`
	Flavor     string `json:"flavor"`
	AddonsPath string `json:"addons_path"`
	Exe        string `json:"exe"`
	Version    string `json:"version"`
	ProfileID  string `json:"profile_id"`
	Confidence string `json:"confidence"`
	Exists     bool   `json:"exists"`
	Addons     int    `json:"addons"`
	Problems   int    `json:"problems"`
	Errors     int    `json:"errors"`
	Health     int    `json:"health"`
}

// InstallsStatusResult is the status of every detected installation.
type InstallsStatusResult struct {
	Installs []InstallStatus `json:"installs"`
}

// SyncInstallResult is the outcome of a bulk update run for one install.
type SyncInstallResult struct {
	Root    string   `json:"root"`
	Updated int      `json:"updated"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// SyncResult aggregates a cross-install bulk update run.
type SyncResult struct {
	Installs     []SyncInstallResult `json:"installs"`
	TotalUpdated int                 `json:"total_updated"`
	TotalFailed  int                 `json:"total_failed"`
}

// SearchHit is one addon found by a catalog search.
type SearchHit struct {
	Provider      string `json:"provider"`
	Name          string `json:"name"`
	Author        string `json:"author"`
	Summary       string `json:"summary"`
	LatestVersion string `json:"latest_version"`
	GameVersion   string `json:"game_version"`
	ID            string `json:"id"`
	Homepage      string `json:"homepage"`
}

// SearchResult is the outcome of a catalog search.
type SearchResult struct {
	Results []SearchHit `json:"results"`
	Errors  []string    `json:"errors"`
}

// DTO conversions -----------------------------------------------------------

func (s *Service) toScanResult(e *env, res *models.ScanResult) ScanResult {
	out := ScanResult{
		AddonsDir: res.AddonsDir,
		ProfileID: e.profile.ID,
		ScannedAt: res.ScannedAt,
		Errors:    errStrings(res.Errors),
	}
	out.Stats.Total, out.Stats.Problems, out.Stats.Errors = res.Stats()
	tracked := s.registryEntries(e)
	out.Addons = make([]Addon, 0, len(res.Addons))
	for _, a := range res.Addons {
		ad := toAddon(a, e.profile)
		if entry, ok := tracked[strings.ToLower(a.FolderName)]; ok {
			ad.Tracked = true
			ad.TrackedSource = entry.Source
			if entry.Checksum != "" {
				if sum, err := catalog.ComputeManifest(filepath.Join(res.AddonsDir, a.FolderName)); err == nil {
					ad.Drifted = sum != entry.Checksum
				}
				// A manifest error (folder vanished mid-scan, unreadable
				// file) leaves Drifted false: best-effort, never fails
				// the scan.
			}
		}
		out.Addons = append(out.Addons, ad)
	}
	return out
}

// addonHealth derives a 0-100 health score from the addon's issues:
// 100 minus 30 per error, 15 per warn and 5 per info, clamped at 0.
// A clean addon scores 100.
func addonHealth(a *models.Addon) int {
	score := 100
	for _, i := range a.Issues {
		switch i.Severity {
		case models.SeverityError:
			score -= 30
		case models.SeverityWarn:
			score -= 15
		case models.SeverityInfo:
			score -= 5
		}
	}
	if score < 0 {
		return 0
	}
	return score
}

func toAddon(a *models.Addon, profile *models.Profile) Addon {
	ad := Addon{
		FolderName:    a.FolderName,
		BaseName:      a.BaseName,
		SuggestedName: a.SuggestedName,
		Status:        string(a.Status),
		Nested:        a.Nested,
		SizeBytes:     a.SizeBytes,
		Fixable:       a.Fixable(),
		Health:        addonHealth(a),
	}
	if pt := a.PrimaryTOC(); pt != nil {
		ad.TOC = &TOC{Name: pt.Name, Title: pt.Title, Interface: pt.Interface, Version: pt.Version}
	}
	for _, i := range a.Issues {
		ad.Issues = append(ad.Issues, Issue{
			Kind:          string(i.Kind),
			Severity:      string(i.Severity),
			Message:       i.Message,
			Suggestion:    i.Suggestion,
			Action:        string(i.Action),
			ActionLabel:   i.Action.Label(),
			Options:       i.Options,
			SuggestedName: i.SuggestedName,
		})
	}
	ad.Compat = append(ad.Compat, toCompat(a, profile))
	return ad
}

func toCompat(a *models.Addon, profile *models.Profile) Compat {
	c := classifyTOC(a, profile)
	detected, tocName := -1, ""
	if c.TOC != nil {
		tocName = c.TOC.Name
		detected = c.TOC.Interface
	}
	return Compat{
		FolderName: a.FolderName,
		TOC:        tocName,
		Expected:   profile.Interface,
		Detected:   detected,
		Status:     string(c.Status),
		Label:      c.Label,
	}
}

func toFixBatch(results []fixer.Result) FixBatch {
	batch := FixBatch{Fixes: make([]Fix, 0, len(results))}
	for _, r := range results {
		f := Fix{Addon: r.Addon, Action: r.Action, OK: r.OK, Message: r.Message}
		if r.Err != nil {
			f.Error = r.Err.Error()
		}
		batch.Fixes = append(batch.Fixes, f)
		if r.OK {
			batch.Fixed++
		} else {
			batch.Failed++
		}
	}
	return batch
}

func toInstallResult(res *installer.Result) InstallResult {
	out := InstallResult{
		Installed: res.Installed,
		Replaced:  res.Replaced,
		Skipped:   []string{},
		Errors:    errStrings(res.Errors),
	}
	for folder, reason := range res.Skipped {
		out.Skipped = append(out.Skipped, fmt.Sprintf("%s: %s", folder, reason))
	}
	sort.Strings(out.Skipped)
	return out
}

// statusForInstall builds one install status row: scan stats and the
// average addon health when the AddOns directory exists, zeroes
// otherwise. A failed scan keeps exists=true and zeroes every count:
// one broken install never fails the surrounding call.
func (s *Service) statusForInstall(inst *detector.Installation, profile *models.Profile) InstallStatus {
	st := InstallStatus{
		Root:       inst.Root,
		Flavor:     inst.Flavor,
		AddonsPath: inst.AddonsPath,
		Exe:        inst.Exe,
		Version:    inst.Version,
		ProfileID:  inst.ProfileID,
		Confidence: inst.Confidence,
		Exists:     inst.Exists(),
	}
	if !st.Exists {
		return st
	}
	res, err := scanner.New(inst.AddonsPath, profile).Scan(context.Background())
	if err != nil {
		return st
	}
	st.Addons, st.Problems, st.Errors = res.Stats()
	if st.Addons > 0 {
		total := 0
		for _, a := range res.Addons {
			total += addonHealth(a)
		}
		st.Health = total / st.Addons
	}
	return st
}

// Wails-bound methods -------------------------------------------------------

// GetState returns the initial UI state, install or not. A saved
// wow_path that no longer resolves is not fatal: the state degrades to
// HasInstall=false and keeps the stale path so the setup view can
// prefill the path picker. Only a genuinely fatal problem (config store
// unreadable) surfaces as an error.
func (s *Service) GetState() (AppState, error) {
	cfg, err := s.store.Load()
	if err != nil {
		return AppState{}, err
	}
	install, err := s.resolveInstall(cfg)
	if err != nil || install == nil {
		// No usable install (stale saved path, nothing auto-detected):
		// report the setup state instead of failing the whole UI.
		e := s.buildEnv(cfg, nil)
		return AppState{
			Version:       s.Version,
			WoWPath:       cfg.WoWPath,
			ProfileID:     e.profile.ID,
			ProfileName:   e.profile.Name,
			AutoBackup:    e.cfg.AutoBackup,
			Confirmations: e.cfg.Confirmations,
		}, nil
	}
	e := s.buildEnv(cfg, install)
	st := AppState{
		Version:       s.Version,
		ProfileID:     e.profile.ID,
		ProfileName:   e.profile.Name,
		AutoBackup:    e.cfg.AutoBackup,
		Confirmations: e.cfg.Confirmations,
	}
	st.WoWPath = e.install.Root
	st.Flavor = e.install.Flavor
	st.AddonsDir = e.install.AddonsPath
	st.HasInstall = true
	return st, nil
}

// DetectInstalls auto-detects every WoW installation on the host.
func (s *Service) DetectInstalls() ([]Install, error) {
	installs, err := detector.AutoDetect(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Install, 0, len(installs))
	for _, i := range installs {
		out = append(out, toInstall(i))
	}
	return out, nil
}

// SetInstall selects the installation at root and persists it. When
// flavor is non-empty it overrides the detected flavor.
func (s *Service) SetInstall(root, flavor string) (Install, error) {
	inst, err := detector.DetectPath(root)
	if err != nil {
		return Install{}, err
	}
	if flavor != "" {
		inst.Flavor = flavor
		inst.AddonsPath = detector.AddonsPath(inst.Root, flavor)
	}
	cfg, err := s.store.Load()
	if err != nil {
		return Install{}, err
	}
	cfg.WoWPath = inst.Root
	cfg.Flavor = inst.Flavor
	if err := s.store.Save(cfg); err != nil {
		return Install{}, err
	}
	return toInstall(*inst), nil
}

// SetProfile persists the active game-version profile.
func (s *Service) SetProfile(id string) error {
	p := models.ProfileByID(id)
	if p == nil {
		return fmt.Errorf("unknown profile %q", id)
	}
	cfg, err := s.store.Load()
	if err != nil {
		return err
	}
	cfg.Profile = p.ID
	return s.store.Save(cfg)
}

// Profiles lists every supported game-version profile.
func (s *Service) Profiles() ([]Profile, error) {
	out := make([]Profile, 0, len(models.Profiles))
	for _, p := range models.Profiles {
		out = append(out, Profile{ID: p.ID, Name: p.Name, Family: p.Family, Interface: p.Interface})
	}
	return out, nil
}

// Scan runs a fresh scan of the selected installation.
func (s *Service) Scan() (ScanResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return ScanResult{}, err
	}
	res, err := s.scan(e)
	if err != nil {
		return ScanResult{}, err
	}
	return s.toScanResult(e, res), nil
}

// Validate reports the TOC compatibility table for every addon.
func (s *Service) Validate() (ValidateResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return ValidateResult{}, err
	}
	res, err := s.scan(e)
	if err != nil {
		return ValidateResult{}, err
	}
	out := ValidateResult{ProfileID: e.profile.ID, Expected: e.profile.Interface}
	out.Addons = make([]Compat, 0, len(res.Addons))
	for _, a := range res.Addons {
		out.Addons = append(out.Addons, toCompat(a, e.profile))
	}
	return out, nil
}

// Fix repairs every issue of one addon. allowDestructive stands in for
// the user's confirmation of destructive steps.
func (s *Service) Fix(folderName string, allowDestructive bool) (FixBatch, error) {
	e, err := s.requireInstall()
	if err != nil {
		return FixBatch{}, err
	}
	res, err := s.scan(e)
	if err != nil {
		return FixBatch{}, err
	}
	var addon *models.Addon
	for _, a := range res.Addons {
		if strings.EqualFold(a.FolderName, folderName) {
			addon = a
			break
		}
	}
	if addon == nil {
		return FixBatch{}, fmt.Errorf("addon %q not found", folderName)
	}
	f := fixer.New(s.fixerOptions(e, allowDestructive))
	return toFixBatch(f.Fix(context.Background(), addon)), nil
}

// FixAll repairs every fixable addon. allowDestructive stands in for
// the user's confirmation of destructive steps.
func (s *Service) FixAll(allowDestructive bool) (FixBatch, error) {
	e, err := s.requireInstall()
	if err != nil {
		return FixBatch{}, err
	}
	res, err := s.scan(e)
	if err != nil {
		return FixBatch{}, err
	}
	f := fixer.New(s.fixerOptions(e, allowDestructive))
	return toFixBatch(f.FixAll(context.Background(), res.Addons)), nil
}

// InstallZip installs an addon archive. allowReplace decides whether
// existing folders may be replaced.
func (s *Service) InstallZip(zipPath string, allowReplace bool) (InstallResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return InstallResult{}, err
	}
	if _, err := detector.EnsureAddons(e.install); err != nil {
		return InstallResult{}, err
	}
	res, err := installer.New(s.installerOptions(e, allowReplace)).Install(context.Background(), zipPath)
	if err != nil {
		return InstallResult{}, err
	}
	return toInstallResult(res), nil
}

// CheckUpdates compares every registry-tracked addon against its
// provider and reports the available updates. Partial provider
// failures land in Errors while the found updates are still returned;
// only hard failures (no install, unreadable registry) surface as the
// error.
func (s *Service) CheckUpdates() (UpdatesResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return UpdatesResult{}, err
	}
	cat, err := s.catalogFor(e)
	if err != nil {
		return UpdatesResult{}, err
	}
	updates, checkErr := catalog.Check(context.Background(), cat, cat.Reg, e.install.AddonsPath)
	out := UpdatesResult{
		Errors:    []string{},
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, u := range updates {
		info := UpdateInfo{
			Folder:         u.Entry.Folder,
			Title:          u.Entry.Title,
			CurrentVersion: u.Entry.Version,
			LatestVersion:  u.Latest.LatestVersion,
			Provider:       u.Entry.Provider,
			ID:             u.Entry.ID,
			Source:         u.Entry.Source,
			FlavorMismatch: u.Mismatch,
		}
		if u.Mismatch {
			info.FlavorLabel = flavorLabel(u.Latest.GameVersion, e.profile)
		}
		out.Updates = append(out.Updates, info)
	}
	if checkErr != nil {
		out.Errors = append(out.Errors, checkErr.Error())
	}
	return out, nil
}

// ApplyUpdate applies the pending update for one folder, matched
// case-insensitively against a fresh check. allowReplace stands in
// for the user's confirmation: without it an update that would
// replace an existing folder is skipped with a message.
func (s *Service) ApplyUpdate(folder string, allowReplace bool) (ApplyBatch, error) {
	e, err := s.requireInstall()
	if err != nil {
		return ApplyBatch{}, err
	}
	cat, err := s.catalogFor(e)
	if err != nil {
		return ApplyBatch{}, err
	}
	updates, _ := catalog.Check(context.Background(), cat, cat.Reg, e.install.AddonsPath)
	var target *catalog.Update
	for i := range updates {
		if strings.EqualFold(updates[i].Entry.Folder, folder) {
			target = &updates[i]
			break
		}
	}
	if target == nil {
		return ApplyBatch{
			Applied:     []ApplyEntry{{Folder: folder, Message: fmt.Sprintf("no update available for %q", folder)}},
			FailedCount: 1,
		}, nil
	}
	if !allowReplace && folderExists(e.install.AddonsPath, target.Entry.Folder) {
		return ApplyBatch{
			Applied:     []ApplyEntry{{Folder: folder, Message: "folder already exists, replace declined"}},
			FailedCount: 1,
		}, nil
	}
	installed, err := catalog.Apply(context.Background(), cat, e.install.AddonsPath, *target, cat.Backups, s.log)
	if err != nil {
		return ApplyBatch{
			Applied:     []ApplyEntry{{Folder: folder, Message: err.Error(), Error: err.Error()}},
			FailedCount: 1,
		}, nil
	}
	return ApplyBatch{
		Applied:      []ApplyEntry{{Folder: installed, OK: true, Message: "applied"}},
		AppliedCount: 1,
	}, nil
}

// ApplyAllUpdates applies every pending update and collects the
// outcomes into one batch. A failure never stops the remaining
// updates.
func (s *Service) ApplyAllUpdates(allowReplace bool) (ApplyBatch, error) {
	e, err := s.requireInstall()
	if err != nil {
		return ApplyBatch{}, err
	}
	return s.applyAllIn(e, e.install.AddonsPath, allowReplace), nil
}

// checkAllIn resolves the catalog for the environment and checks the
// tracked addons against one AddOns directory. Check is read-only, so
// several installs can be checked against the same registry state
// before any update is applied. A catalog setup failure returns the
// error in the string list with nil updates. Check's own error is
// deliberately dropped, matching the single-install path: provider
// failures surface per-entry as skipped updates, never as a hard
// failure.
func (s *Service) checkAllIn(e *env, addonsDir string) ([]catalog.Update, []string) {
	cat, err := s.catalogFor(e)
	if err != nil {
		return nil, []string{err.Error()}
	}
	updates, _ := catalog.Check(context.Background(), cat, cat.Reg, addonsDir)
	return updates, nil
}

// applyUpdates applies a pre-collected set of updates into one AddOns
// directory, snapshotting that install separately via backupRootFor. A
// catalog setup failure lands in the batch's errors field with zero
// counts; per-update failures are counted and never stop the rest.
func (s *Service) applyUpdates(e *env, addonsDir string, updates []catalog.Update, allowReplace bool) ApplyBatch {
	cat, err := s.catalogFor(e)
	if err != nil {
		return ApplyBatch{Applied: []ApplyEntry{}, errors: []string{err.Error()}}
	}
	backups := backup.New(s.backupRootFor(e, addonsDir), s.log)
	batch := ApplyBatch{Applied: []ApplyEntry{}}
	for _, u := range updates {
		entry := ApplyEntry{Folder: u.Entry.Folder}
		if !allowReplace && folderExists(addonsDir, u.Entry.Folder) {
			entry.Message = "folder already exists, replace declined"
			batch.Applied = append(batch.Applied, entry)
			batch.FailedCount++
			continue
		}
		installed, err := catalog.Apply(context.Background(), cat, addonsDir, u, backups, s.log)
		if err != nil {
			entry.Message = err.Error()
			entry.Error = err.Error()
			batch.Applied = append(batch.Applied, entry)
			batch.FailedCount++
			continue
		}
		entry.OK = true
		entry.Folder = installed
		entry.Message = "applied"
		batch.Applied = append(batch.Applied, entry)
		batch.AppliedCount++
	}
	return batch
}

// applyAllIn runs the shared apply-all body against one AddOns
// directory: check the tracked addons, then apply every pending
// update. This is the single-install composition (one check followed
// by its own applies); cross-install sync uses checkAllIn and
// applyUpdates separately so every install's check sees the same
// registry baseline.
func (s *Service) applyAllIn(e *env, addonsDir string, allowReplace bool) ApplyBatch {
	updates, errs := s.checkAllIn(e, addonsDir)
	if errs != nil {
		return ApplyBatch{Applied: []ApplyEntry{}, errors: errs}
	}
	return s.applyUpdates(e, addonsDir, updates, allowReplace)
}

// SearchCatalog queries every enabled provider with the same query
// and returns the merged results. Partial provider failures land in
// Errors; when every provider fails the error is returned alongside
// the empty results (matching the CLI).
func (s *Service) SearchCatalog(query string) (SearchResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return SearchResult{}, err
	}
	cat, err := s.catalogFor(e)
	if err != nil {
		return SearchResult{}, err
	}
	addons, searchErr := cat.Search(context.Background(), query, 20)
	out := SearchResult{Results: []SearchHit{}, Errors: []string{}}
	for _, a := range addons {
		out.Results = append(out.Results, SearchHit{
			Provider:      a.Provider,
			Name:          a.Name,
			Author:        a.Author,
			Summary:       a.Summary,
			LatestVersion: a.LatestVersion,
			GameVersion:   a.GameVersion,
			ID:            a.ID,
			Homepage:      a.Homepage,
		})
	}
	if searchErr != nil {
		out.Errors = append(out.Errors, searchErr.Error())
		if len(out.Results) == 0 {
			return out, searchErr
		}
	}
	return out, nil
}

// InstallSource installs an addon from a URL or provider-scoped id
// through the catalog (see catalog.InstallFromSource for the accepted
// source forms) and reports the outcome with the same DTO as
// InstallZip. The catalog layer reports installed folder names and
// errors only, so replaced/skipped stay empty; allowReplace is
// accepted for the frontend contract, but the catalog currently
// always replaces existing folders.
func (s *Service) InstallSource(source string, allowReplace bool) (InstallResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return InstallResult{}, err
	}
	return s.installSource(e, source, allowReplace)
}

// installSource runs the shared catalog install body: resolve the
// catalog for the environment, ensure the AddOns directory exists and
// install from source, mapping the outcome to the InstallResult DTO.
// Provider failures land in Errors with a nil Go error so the frontend
// renders them inline.
func (s *Service) installSource(e *env, source string, allowReplace bool) (InstallResult, error) {
	cat, err := s.catalogFor(e)
	if err != nil {
		return InstallResult{}, err
	}
	if _, err := detector.EnsureAddons(e.install); err != nil {
		return InstallResult{}, err
	}
	installed, err := cat.InstallFromSource(context.Background(), source, e.install.AddonsPath, nil)
	if err != nil {
		return InstallResult{
			Installed: []string{},
			Replaced:  []string{},
			Skipped:   []string{},
			Errors:    []string{err.Error()},
		}, nil
	}
	return InstallResult{
		Installed: installed,
		Replaced:  []string{},
		Skipped:   []string{},
		Errors:    []string{},
	}, nil
}

// RestoreAddon re-downloads a tracked addon from the provider source
// recorded in the registry and replaces its folder, restoring the
// pristine manifest the registry's checksum records. The catalog layer
// re-records the fresh checksum after a successful install. An
// untracked folder is reported in the DTO's Errors with a nil Go
// error, mirroring InstallSource's error-handling style.
func (s *Service) RestoreAddon(folder string, allowReplace bool) (InstallResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return InstallResult{}, err
	}
	entry, ok := s.trackedEntry(e, folder)
	if !ok {
		return InstallResult{
			Installed: []string{},
			Replaced:  []string{},
			Skipped:   []string{},
			Errors:    []string{"addon not tracked in registry"},
		}, nil
	}
	return s.installSource(e, entry.Source, allowReplace)
}

// Collections lists every collection with its addon count and marks
// the configured one active. The active id comes from the saved
// config, not from the on-disk folder state.
func (s *Service) Collections() (CollectionsResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return CollectionsResult{}, err
	}
	m, err := s.profilesFor(e)
	if err != nil {
		return CollectionsResult{}, err
	}
	cols, err := m.List()
	if err != nil {
		return CollectionsResult{}, err
	}
	out := CollectionsResult{
		Collections: []CollectionInfo{},
		ActiveID:    e.cfg.Collection,
	}
	for _, c := range cols {
		out.Collections = append(out.Collections, CollectionInfo{
			ID:         c.ID,
			Name:       c.Name,
			AddonCount: len(c.Addons),
			Active:     c.ID == e.cfg.Collection,
		})
	}
	return out, nil
}

// CreateCollection snapshots the current on-disk addon state into a
// new collection. The collection is not activated; the frontend
// decides whether to switch to it.
func (s *Service) CreateCollection(name string) (CollectionInfo, error) {
	e, err := s.requireInstall()
	if err != nil {
		return CollectionInfo{}, err
	}
	m, err := s.profilesFor(e)
	if err != nil {
		return CollectionInfo{}, err
	}
	c, err := m.Create(name)
	if err != nil {
		return CollectionInfo{}, err
	}
	return CollectionInfo{ID: c.ID, Name: c.Name, AddonCount: len(c.Addons)}, nil
}

// SwitchCollection applies the collection's addon state on disk and
// persists it as the active collection. The switch renames folders
// between "<name>" and "<name>.disabled" and is backup-snapshotted
// when auto_backup is on; the frontend's dialog is the confirmation
// gate. A missing collection is a Go error.
func (s *Service) SwitchCollection(id string) (SwitchResult, error) {
	e, err := s.requireInstall()
	if err != nil {
		return SwitchResult{}, err
	}
	m, err := s.profilesFor(e)
	if err != nil {
		return SwitchResult{}, err
	}
	applied, err := m.SwitchTo(id)
	if err != nil {
		return SwitchResult{}, err
	}
	name := id
	if c, err := m.Get(id); err == nil {
		name = c.Name
	}
	if e.cfg.Collection != id {
		e.cfg.Collection = id
		if err := s.store.Save(e.cfg); err != nil {
			return SwitchResult{}, err
		}
	}
	return SwitchResult{
		Applied: applied,
		Message: fmt.Sprintf("Switched to collection %q (%d addon(s) applied)", name, len(applied)),
	}, nil
}

// DeleteCollection removes a collection. Installed addons are
// untouched; the active collection id is cleared from the saved
// config when the deleted collection was active.
func (s *Service) DeleteCollection(id string) error {
	e, err := s.requireInstall()
	if err != nil {
		return err
	}
	m, err := s.profilesFor(e)
	if err != nil {
		return err
	}
	if err := m.Delete(id); err != nil {
		return err
	}
	if e.cfg.Collection == id {
		e.cfg.Collection = ""
		if err := s.store.Save(e.cfg); err != nil {
			return err
		}
	}
	return nil
}

// CollectionDetail loads one collection's full addon state table.
func (s *Service) CollectionDetail(id string) (CollectionDetail, error) {
	e, err := s.requireInstall()
	if err != nil {
		return CollectionDetail{}, err
	}
	m, err := s.profilesFor(e)
	if err != nil {
		return CollectionDetail{}, err
	}
	c, err := m.Get(id)
	if err != nil {
		return CollectionDetail{}, err
	}
	out := CollectionDetail{ID: c.ID, Name: c.Name}
	out.Addons = make([]CollectionAddon, 0, len(c.Addons))
	for _, a := range c.Addons {
		out.Addons = append(out.Addons, CollectionAddon{Folder: a.Folder, Enabled: a.Enabled})
	}
	return out, nil
}

// SetCollectionAddon toggles one addon's desired state in a
// collection. Unknown folders are appended.
func (s *Service) SetCollectionAddon(id, folder string, enabled bool) error {
	e, err := s.requireInstall()
	if err != nil {
		return err
	}
	m, err := s.profilesFor(e)
	if err != nil {
		return err
	}
	return m.SetEnabled(id, folder, enabled)
}

// InstallsStatus reports every detected installation with live scan
// stats and average addon health. Every install is inspected, not
// just the active one; a failed scan for one install keeps exists
// true and zeroes its counts instead of failing the whole call.
func (s *Service) InstallsStatus() (InstallsStatusResult, error) {
	installs, err := detector.AutoDetect(context.Background())
	if err != nil {
		return InstallsStatusResult{}, err
	}
	out := InstallsStatusResult{Installs: []InstallStatus{}}
	for i := range installs {
		profile := models.ProfileByID(installs[i].ProfileID)
		if profile == nil {
			profile = models.DefaultProfile()
		}
		out.Installs = append(out.Installs, s.statusForInstall(&installs[i], profile))
	}
	return out, nil
}

// SyncUpdatesToAll applies every pending update to every detected
// install with an existing AddOns directory: all installs are checked
// against the shared per-user registry first, then each install's
// updates are applied, so a tracked addon present in several installs
// is updated in each one. Every install is snapshotted separately. A
// failing install lands in its row's errors with zero counts and
// never aborts the remaining installs. No installs yields an empty
// result with a nil error.
func (s *Service) SyncUpdatesToAll(allowReplace bool) (SyncResult, error) {
	installs, err := detector.AutoDetect(context.Background())
	if err != nil {
		return SyncResult{}, err
	}
	e, err := s.env()
	if err != nil {
		return SyncResult{}, err
	}
	return s.syncInstalls(e, installs, allowReplace), nil
}

// syncInstalls runs the shared cross-install update body against an
// explicit install list (the tests inject one; SyncUpdatesToAll
// passes AutoDetect's results). It is two-pass on purpose: every
// install is checked FIRST against the same pre-apply registry state
// (Check is read-only; catalog.Apply re-records bumped versions), then
// each install's collected updates are applied. A tracked addon present
// in several installs is therefore updated in every one of them.
func (s *Service) syncInstalls(e *env, installs []detector.Installation, allowReplace bool) SyncResult {
	// Pass 1: check every install with an existing AddOns dir. A
	// catalog setup failure lands in that row's errors with zero
	// counts and never aborts the pass.
	var rows []SyncInstallResult
	pending := make(map[string][]catalog.Update)
	var order []string
	for _, inst := range installs {
		if !inst.Exists() {
			continue
		}
		row := SyncInstallResult{Root: inst.Root}
		updates, errs := s.checkAllIn(e, inst.AddonsPath)
		if errs != nil {
			row.Errors = errs
		} else {
			pending[inst.AddonsPath] = updates
		}
		order = append(order, inst.AddonsPath)
		rows = append(rows, row)
	}

	// Pass 2: apply each install's collected updates.
	for i, dir := range order {
		row := &rows[i]
		if row.Errors != nil {
			continue // pass-1 failure: nothing to apply
		}
		batch := s.applyUpdates(e, dir, pending[dir], allowReplace)
		row.Updated = batch.AppliedCount
		row.Failed = batch.FailedCount
		if len(batch.errors) > 0 {
			row.Errors = batch.errors
		}
	}

	out := SyncResult{Installs: []SyncInstallResult{}}
	for _, row := range rows {
		out.Installs = append(out.Installs, row)
		out.TotalUpdated += row.Updated
		out.TotalFailed += row.Failed
	}
	return out
}
