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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/fixer"
	"github.com/wowfix/wowfix/internal/installer"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/scanner"
	"github.com/wowfix/wowfix/internal/validator"
)

// Service is the Wails-bound backend facade.
type Service struct {
	store *config.Store
	log   *logger.Logger
	// Version is the build version reported in AppState; defaults to "dev".
	Version string
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
	TOC           *TOC     `json:"toc"`
	Issues        []Issue  `json:"issues"`
	Compat        []Compat `json:"compat"`
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

// DTO conversions -----------------------------------------------------------

func (s *Service) toScanResult(e *env, res *models.ScanResult) ScanResult {
	out := ScanResult{
		AddonsDir: res.AddonsDir,
		ProfileID: e.profile.ID,
		ScannedAt: res.ScannedAt,
		Errors:    errStrings(res.Errors),
	}
	out.Stats.Total, out.Stats.Problems, out.Stats.Errors = res.Stats()
	out.Addons = make([]Addon, 0, len(res.Addons))
	for _, a := range res.Addons {
		out.Addons = append(out.Addons, toAddon(a, e.profile))
	}
	return out
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
