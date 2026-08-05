// Package fixer turns scanner-detected issues into filesystem repairs.
// Every mutation is preceded by a backup snapshot and every destructive
// step goes through the confirmation hook.
package fixer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wowfix/wowfix/internal/backup"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/utils"
	"github.com/wowfix/wowfix/internal/validator"
)

// Options configures a Fixer.
type Options struct {
	// AddonsDir is the Interface/AddOns directory being repaired.
	AddonsDir string
	// Profile is used to auto-pick a TOC among several.
	Profile *models.Profile
	// Backups receives a snapshot before each mutation. nil disables backups.
	Backups *backup.Manager
	// Log receives human-readable action lines.
	Log *logger.Logger
	// Confirm is invoked for destructive or overwriting actions. When
	// nil, all actions are confirmed.
	Confirm func(format string, args ...any) bool
	// ChooseTOC resolves the IssueMultipleTOCs prompt; the default picks
	// the TOC whose interface family matches the profile, then a TOC
	// matching the folder name, then the first.
	ChooseTOC func(addon *models.Addon) (*models.TOC, error)
	// TrashFallbackDir receives folders when no native trash exists.
	TrashFallbackDir string
}

// Fixer applies repairs.
type Fixer struct {
	opts Options
}

// New returns a Fixer with the given options.
func New(opts Options) *Fixer {
	if opts.Profile == nil {
		opts.Profile = models.DefaultProfile()
	}
	return &Fixer{opts: opts}
}

// Result is the outcome of fixing one issue.
type Result struct {
	Addon   string
	Action  string
	OK      bool
	Message string
	Err     error
}

func (r Result) String() string {
	if r.Err != nil {
		return fmt.Sprintf("%s %s failed: %v", r.Addon, r.Action, r.Err)
	}
	if !r.OK {
		return fmt.Sprintf("%s %s skipped: %s", r.Addon, r.Action, r.Message)
	}
	return fmt.Sprintf("%s %s: %s", r.Addon, r.Action, r.Message)
}

// FixAll repairs every fixable addon in order. It returns one result
// per attempted issue and never stops on the first failure.
func (f *Fixer) FixAll(ctx context.Context, addons []*models.Addon) []Result {
	var results []Result
	for _, a := range addons {
		if strings.HasSuffix(strings.ToLower(a.FolderName), ".disabled") {
			continue // disabled addons are not fix targets
		}
		if ctx.Err() != nil {
			results = append(results, Result{Addon: a.FolderName, Action: "fix-all", Message: "cancelled"})
			break
		}
		results = append(results, f.Fix(ctx, a)...)
	}
	return results
}

// Fix applies all issues of one addon in a safe order. Structural fixes
// (merge/delete/flatten) supersede naming fixes for the same folder.
func (f *Fixer) Fix(ctx context.Context, addon *models.Addon) []Result {
	if len(addon.Issues) == 0 {
		return []Result{{Addon: addon.FolderName, Action: "fix", OK: true, Message: "no issues"}}
	}

	issues := append([]*models.Issue(nil), addon.Issues...)
	sort.SliceStable(issues, func(i, j int) bool {
		return fixPriority(issues[i].Action) < fixPriority(issues[j].Action)
	})

	var results []Result
	for _, issue := range issues {
		if ctx.Err() != nil {
			break
		}
		res := f.FixIssue(addon, issue)
		results = append(results, res)
		// After the folder moved or vanished, later naming fixes are
		// moot; a failed structural fix invalidates remaining paths too,
		// so stop rather than rename a stale folder.
		if res.Err != nil || (res.OK && structural(issue.Action)) {
			break
		}
	}
	return results
}

func fixPriority(a models.FixAction) int {
	switch a {
	case models.ActionDelete, models.ActionMerge, models.ActionRepairStructure:
		return 0
	case models.ActionFlatten:
		return 1
	case models.ActionResolveTOC:
		return 2
	case models.ActionRename:
		return 3
	default:
		return 4
	}
}

func structural(a models.FixAction) bool {
	switch a {
	case models.ActionDelete, models.ActionMerge, models.ActionFlatten, models.ActionRepairStructure:
		return true
	default:
		return false
	}
}

// FixIssue applies a single issue's repair.
func (f *Fixer) FixIssue(addon *models.Addon, issue *models.Issue) Result {
	res := Result{Addon: addon.FolderName, Action: issue.Action.Label()}
	switch issue.Action {
	case models.ActionRename:
		f.rename(addon, issue, &res)
	case models.ActionFlatten:
		f.flatten(addon, &res)
	case models.ActionResolveTOC:
		f.resolveTOC(addon, &res)
	case models.ActionDelete:
		f.delete(addon, &res)
	case models.ActionMerge:
		f.merge(addon, issue, &res)
	default:
		res.OK = false
		res.Message = "no automatic fix available"
	}
	if res.Err != nil && f.opts.Log != nil {
		f.opts.Log.Errorf("%s", res.String())
	} else if f.opts.Log != nil && res.Message != "" {
		f.opts.Log.Infof("%s", res.String())
	}
	return res
}

func (f *Fixer) confirmed(format string, args ...any) bool {
	if f.opts.Confirm == nil {
		return true
	}
	return f.opts.Confirm(format, args...)
}

func (f *Fixer) backup(path, reason string) error {
	if f.opts.Backups == nil {
		return nil
	}
	_, err := f.opts.Backups.Backup([]string{path}, reason)
	return err
}

// rename fixes ActionRename: folder -> SuggestedName.
func (f *Fixer) rename(addon *models.Addon, issue *models.Issue, res *Result) {
	from := addon.Path
	to := filepath.Join(filepath.Dir(from), utils.CleanName(issue.SuggestedName))
	if from == to {
		res.OK = true
		res.Message = "name already correct"
		return
	}
	if utils.Exists(to) {
		if !f.confirmed("Overwrite existing folder %q with %q?", filepath.Base(to), filepath.Base(from)) {
			res.Message = "destination exists, user declined"
			return
		}
		if err := f.backup(to, "pre-overwrite snapshot"); err != nil {
			res.Err = err
			return
		}
		if err := os.RemoveAll(to); err != nil {
			res.Err = fmt.Errorf("cannot remove %q: %w", to, err)
			return
		}
	}
	if err := f.backup(from, "rename "+filepath.Base(from)); err != nil {
		res.Err = err
		return
	}
	if err := utils.SafeRename(from, to); err != nil {
		res.Err = err
		return
	}
	addon.Path = to
	addon.FolderName = filepath.Base(to)
	res.OK = true
	res.Message = fmt.Sprintf("Renamed %s → %s", filepath.Base(from), filepath.Base(to))
}

// flatten fixes ActionNested: move the inner addon up to the AddOns
// root under its suggested name, then trash the emptied outer folder.
func (f *Fixer) flatten(addon *models.Addon, res *Result) {
	inner := addon.Path
	target := filepath.Join(f.opts.AddonsDir, utils.CleanName(addon.SuggestedName))
	if target == inner {
		res.OK = true
		res.Message = "already at top level"
		return
	}
	if utils.Exists(target) {
		res.Message = fmt.Sprintf("target %q already exists", filepath.Base(target))
		return
	}
	outer := addon.SourceDir
	outerPath := filepath.Join(f.opts.AddonsDir, outer)
	if err := f.backup(outerPath, "flatten "+outer); err != nil {
		res.Err = err
		return
	}
	if err := utils.SafeRename(inner, target); err != nil {
		res.Err = err
		return
	}
	// The outer wrapper is now empty (or contains only the moved addon).
	if outerPath != target && utils.IsDir(outerPath) {
		if err := f.trash(outerPath); err != nil {
			res.Err = fmt.Errorf("flattened but could not remove wrapper: %w", err)
			return
		}
	}
	addon.Path = target
	addon.FolderName = filepath.Base(target)
	addon.Nested = false
	res.OK = true
	res.Message = fmt.Sprintf("Flattened %s → %s", outer, filepath.Base(target))
}

// resolveTOC fixes ActionResolveTOC: pick the defining TOC and rename
// the folder to match it when necessary.
func (f *Fixer) resolveTOC(addon *models.Addon, res *Result) {
	pick := f.opts.ChooseTOC
	if pick == nil {
		pick = defaultTOCPicker(f.opts.Profile)
	}
	toc, err := pick(addon)
	if err != nil {
		res.Err = err
		return
	}
	for _, t := range addon.TOCs {
		t.Primary = t == toc
	}
	if !strings.EqualFold(toc.Name, addon.FolderName) {
		issue := &models.Issue{
			Kind:          models.IssueTocMismatch,
			Action:        models.ActionRename,
			SuggestedName: toc.Name,
			Severity:      models.SeverityWarn,
		}
		f.rename(addon, issue, res)
		if res.Err != nil || !res.OK {
			return
		}
		res.Action = "Resolve TOC"
		return
	}
	res.OK = true
	res.Message = fmt.Sprintf("Using TOC %s.toc", toc.Name)
}

func defaultTOCPicker(profile *models.Profile) func(*models.Addon) (*models.TOC, error) {
	return func(addon *models.Addon) (*models.TOC, error) {
		if len(addon.TOCs) == 0 {
			return nil, fmt.Errorf("addon has no TOC files")
		}
		for _, t := range addon.TOCs {
			if t.Interface > 0 && models.FamilyOf(t.Interface) == profile.Family {
				return t, nil
			}
		}
		for _, t := range addon.TOCs {
			if strings.EqualFold(t.Name, addon.BaseName) {
				return t, nil
			}
		}
		return addon.TOCs[0], nil
	}
}

// delete fixes ActionDelete: move the whole top-level folder to trash.
func (f *Fixer) delete(addon *models.Addon, res *Result) {
	target := addon.SourceDir
	targetPath := filepath.Join(f.opts.AddonsDir, target)
	if addon.Nested {
		// The nested addon's wrapper is the top-level entry.
		targetPath = filepath.Join(f.opts.AddonsDir, addon.SourceDir)
	}
	if !utils.Exists(targetPath) {
		res.Message = "folder already gone"
		return
	}
	if !f.confirmed("Move %q to trash?", filepath.Base(targetPath)) {
		res.Message = "user declined"
		return
	}
	if err := f.backup(targetPath, "delete "+filepath.Base(targetPath)); err != nil {
		res.Err = err
		return
	}
	if err := f.trash(targetPath); err != nil {
		res.Err = err
		return
	}
	res.OK = true
	res.Message = fmt.Sprintf("Moved %s to trash", filepath.Base(targetPath))
}

// merge fixes ActionDuplicate: copy the duplicate's unique files into
// the primary folder, then trash the duplicate.
func (f *Fixer) merge(addon *models.Addon, issue *models.Issue, res *Result) {
	primaryName := issue.SuggestedName
	if primaryName == "" {
		primaryName = addon.SuggestedName
	}
	primaryPath := filepath.Join(f.opts.AddonsDir, primaryName)
	if !utils.IsDir(primaryPath) {
		res.Err = fmt.Errorf("primary addon %q not found", primaryName)
		return
	}
	dupPath := filepath.Join(f.opts.AddonsDir, addon.SourceDir)
	if addon.Nested {
		dupPath = filepath.Join(f.opts.AddonsDir, addon.SourceDir)
	}
	if !utils.IsDir(dupPath) {
		res.Message = "duplicate folder already gone"
		return
	}
	if !f.confirmed("Merge %q into %q? Unique files will be copied over.", filepath.Base(dupPath), primaryName) {
		res.Message = "user declined"
		return
	}
	if err := f.backup(primaryPath, "merge target"); err != nil {
		res.Err = err
		return
	}
	if err := f.backup(dupPath, "merge source"); err != nil {
		res.Err = err
		return
	}
	copied, err := mergeInto(dupPath, primaryPath)
	if err != nil {
		res.Err = err
		return
	}
	if err := f.trash(dupPath); err != nil {
		res.Err = fmt.Errorf("merged %d file(s) but could not remove duplicate: %w", copied, err)
		return
	}
	res.OK = true
	res.Message = fmt.Sprintf("Merged %s into %s (%d unique file(s))", filepath.Base(dupPath), primaryName, copied)
}

// mergeInto copies files from src into dst without overwriting existing
// files. Returns the number of files copied.
func mergeInto(src, dst string) (int, error) {
	copied := 0
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if utils.Exists(target) {
			return nil // never overwrite during merge
		}
		if err := utils.CopyFile(path, target); err != nil {
			return err
		}
		copied++
		return nil
	})
	return copied, err
}

func (f *Fixer) trash(path string) error {
	return utils.Trash(path, f.opts.TrashFallbackDir)
}

// Validate runs a fresh validation of one addon against the profile,
// returning the per-TOC compatibility table.
func (f *Fixer) Validate(addon *models.Addon) []validator.Compatibility {
	return validator.ValidateAddon(addon, f.opts.Profile)
}
