// Package scanner walks an Interface/AddOns directory and detects the
// common addon installation problems: Git ref folder names, TOC name
// mismatches, nested folders, missing or multiple TOCs, empty folders,
// duplicates and broken extraction structures.
//
// The scanner is pure detection: it never modifies the filesystem.
// Repairs live in the fixer package.
package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/utils"
	"github.com/wowfix/wowfix/internal/validator"
)

// Scanner detects issues in an AddOns directory.
type Scanner struct {
	// AddonsDir is the Interface/AddOns path to scan.
	AddonsDir string
	// Profile drives TOC compatibility classification.
	Profile *models.Profile
}

// New returns a Scanner for the given AddOns directory.
func New(addonsDir string, profile *models.Profile) *Scanner {
	if profile == nil {
		profile = models.DefaultProfile()
	}
	return &Scanner{AddonsDir: addonsDir, Profile: profile}
}

// Scan analyzes every addon folder in the AddOns directory and returns
// the full report. Directory-level permission failures are collected in
// ScanResult.Errors instead of aborting the scan.
func (s *Scanner) Scan(ctx context.Context) (*models.ScanResult, error) {
	entries, err := os.ReadDir(s.AddonsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read AddOns directory %q: %w", s.AddonsDir, err)
	}

	result := &models.ScanResult{
		AddonsDir: s.AddonsDir,
		Profile:   s.Profile,
		ScannedAt: time.Now(),
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if !entry.IsDir() {
			continue // stray files at the AddOns root are not addons
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue // hidden folders (.DS_Store, .git, ...) are noise
		}
		addon, err := s.analyzeEntry(ctx, filepath.Join(s.AddonsDir, name), name)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", name, err))
			continue
		}
		result.Addons = append(result.Addons, addon...)
	}

	markDuplicates(result.Addons)
	models.SortAddons(result.Addons)
	return result, nil
}

// Discover analyzes every top-level folder of the scanner's root
// directory and returns the addon units found, without duplicate
// grouping. The installer uses it on freshly extracted archives, where
// the root is a temporary directory rather than an AddOns folder.
func (s *Scanner) Discover(ctx context.Context) ([]*models.Addon, []error) {
	entries, err := os.ReadDir(s.AddonsDir)
	if err != nil {
		return nil, []error{fmt.Errorf("cannot read %q: %w", s.AddonsDir, err)}
	}
	var addons []*models.Addon
	var errs []error
	for _, entry := range entries {
		if ctx.Err() != nil {
			return addons, errs
		}
		if !entry.IsDir() {
			continue
		}
		found, err := s.analyzeEntry(ctx, filepath.Join(s.AddonsDir, entry.Name()), entry.Name())
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		addons = append(addons, found...)
	}
	return addons, errs
}

// analyzeEntry inspects one top-level folder and returns the addon
// units it contains. A single folder can yield several addons when a
// zip-in-zip extraction dumped multiple addons into one directory.
func (s *Scanner) analyzeEntry(ctx context.Context, path, name string) ([]*models.Addon, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read folder: %w", err)
	}

	if len(entries) == 0 {
		addon := &models.Addon{FolderName: name, Path: path, BaseName: utils.StripGitRef(name), SourceDir: name, Status: models.StatusOK}
		addon.SuggestedName = addon.BaseName
		addon.AddIssue(&models.Issue{
			Kind:       models.IssueEmpty,
			Severity:   models.SeverityWarn,
			Message:    "Folder is empty",
			Suggestion: "Move to trash to keep the collection clean",
			Action:     models.ActionDelete,
		})
		return []*models.Addon{addon}, nil
	}

	topTOCs := tocPaths(path, entries)
	if len(topTOCs) > 0 {
		addon, err := s.buildAddon(ctx, path, name, entries, topTOCs, "")
		if err != nil {
			return nil, err
		}
		return []*models.Addon{addon}, nil
	}

	// No TOC at the top level: look one level down.
	var tocDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(path, e.Name())
		if len(tocPaths(sub, mustReadDir(sub))) > 0 {
			tocDirs = append(tocDirs, e.Name())
		}
	}

	switch len(tocDirs) {
	case 0:
		addon := &models.Addon{FolderName: name, Path: path, BaseName: utils.StripGitRef(name), SourceDir: name, Status: models.StatusOK}
		addon.SuggestedName = addon.BaseName
		addon.AddIssue(&models.Issue{
			Kind:       models.IssueMissingTOC,
			Severity:   models.SeverityError,
			Message:    "No .toc file found in this folder",
			Suggestion: "The game will not load this folder; move it to trash or restore the missing TOC",
			Action:     models.ActionDelete,
		})
		return []*models.Addon{addon}, nil
	case 1:
		// Nested single addon, e.g. DPSMate-main/DPSMate/DPSMate.toc.
		inner := filepath.Join(path, tocDirs[0])
		innerEntries := mustReadDir(inner)
		innerTOCs := tocPaths(inner, innerEntries)
		addon, err := s.buildAddon(ctx, inner, tocDirs[0], innerEntries, innerTOCs, name)
		if err != nil {
			return nil, err
		}
		addon.Nested = true
		addon.SourceDir = name
		addon.AddIssue(&models.Issue{
			Kind:       models.IssueNested,
			Severity:   models.SeverityWarn,
			Message:    fmt.Sprintf("Addon is nested inside %q", name),
			Suggestion: "Flatten the folder so the game finds the addon",
			Action:     models.ActionFlatten,
		})
		return []*models.Addon{addon}, nil
	default:
		// Multiple addons inside one folder (broken extraction).
		var out []*models.Addon
		for _, sub := range tocDirs {
			inner := filepath.Join(path, sub)
			innerEntries := mustReadDir(inner)
			innerTOCs := tocPaths(inner, innerEntries)
			addon, err := s.buildAddon(ctx, inner, sub, innerEntries, innerTOCs, name)
			if err != nil {
				continue
			}
			addon.Nested = true
			addon.SourceDir = name
			addon.AddIssue(&models.Issue{
				Kind:       models.IssueNested,
				Severity:   models.SeverityWarn,
				Message:    fmt.Sprintf("Addon is nested inside %q along with %d other addons", name, len(tocDirs)-1),
				Suggestion: "Promote each addon to the top level",
				Action:     models.ActionFlatten,
			})
			out = append(out, addon)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("cannot read any nested addon folder")
		}
		return out, nil
	}
}

// buildAddon constructs the addon record for a folder that contains
// TOC files directly. parentName is the enclosing folder name when the
// addon is nested ("" otherwise).
func (s *Scanner) buildAddon(ctx context.Context, path, name string, entries []fs.DirEntry, tocPaths []string, parentName string) (*models.Addon, error) {
	tocs, parseErrs := validator.ParseTOCs(tocPaths)
	_ = parseErrs // individual TOC parse failures are reported via compat; keep scanning

	addon := &models.Addon{
		FolderName: name,
		Path:       path,
		BaseName:   utils.StripGitRef(name),
		TOCs:       tocs,
		Status:     models.StatusOK,
	}
	if parentName != "" {
		addon.SourceDir = parentName
	} else {
		addon.SourceDir = name
	}
	addon.SuggestedName = suggestedName(addon)

	s.checkGitRef(addon)
	s.checkTOCNames(addon)
	return addon, nil
}

// checkGitRef flags -main/-master/-dev folder names.
func (s *Scanner) checkGitRef(addon *models.Addon) {
	if addon.BaseName == addon.FolderName {
		return
	}
	addon.AddIssue(&models.Issue{
		Kind:          models.IssueGitHubName,
		Severity:      models.SeverityWarn,
		Message:       fmt.Sprintf("Folder name %q looks like a Git checkout ref", addon.FolderName),
		Suggestion:    fmt.Sprintf("Rename folder to %q", addon.BaseName),
		Action:        models.ActionRename,
		SuggestedName: addon.BaseName,
	})
}

// checkTOCNames flags single-TOC name mismatches and ambiguous
// multi-TOC folders, following the spec:
//
//	Aux/ containing Aux-Classic.toc  -> rename folder to Aux-Classic
//	Atlas/ with Atlas/Atlas_Wrath/Atlas_TBC -> ask the user
func (s *Scanner) checkTOCNames(addon *models.Addon) {
	switch len(addon.TOCs) {
	case 0:
		// handled by the missing-TOC path in analyzeEntry
		return
	case 1:
		toc := addon.TOCs[0]
		if !strings.EqualFold(toc.Name, addon.FolderName) &&
			!strings.EqualFold(toc.Name, addon.BaseName) {
			addon.AddIssue(&models.Issue{
				Kind:          models.IssueTocMismatch,
				Severity:      models.SeverityWarn,
				Message:       fmt.Sprintf("TOC %q does not match folder name %q", toc.Name+".toc", addon.FolderName),
				Suggestion:    fmt.Sprintf("Rename folder to %q", toc.Name),
				Action:        models.ActionRename,
				SuggestedName: toc.Name,
			})
		} else {
			toc.Primary = true
		}
		addon.SuggestedName = suggestedName(addon)
	default:
		// Multiple TOCs: pick the one matching the folder name as primary;
		// the rest are alternate profile TOCs (e.g. Atlas.toc +
		// Atlas_Wrath.toc). If nothing matches, ask the user.
		matched := false
		for _, t := range addon.TOCs {
			if strings.EqualFold(t.Name, addon.FolderName) || strings.EqualFold(t.Name, addon.BaseName) {
				t.Primary = true
				matched = true
				break
			}
		}
		if !matched {
			options := make([]string, 0, len(addon.TOCs))
			for _, t := range addon.TOCs {
				options = append(options, t.Name)
			}
			addon.AddIssue(&models.Issue{
				Kind:       models.IssueMultipleTOCs,
				Severity:   models.SeverityWarn,
				Message:    "Multiple TOC files and none matches the folder name",
				Suggestion: "Choose which TOC identifies this addon",
				Action:     models.ActionResolveTOC,
				Options:    options,
			})
		}
	}
}

// suggestedName computes the folder name the addon should have after
// all automatic fixes are applied.
func suggestedName(addon *models.Addon) string {
	name := addon.BaseName
	if len(addon.TOCs) == 1 {
		t := addon.TOCs[0]
		return t.Name
	}
	for _, t := range addon.TOCs {
		if strings.EqualFold(t.Name, name) {
			return t.Name
		}
	}
	return name
}

// markDuplicates flags addons that resolve to the same folder name.
// The cleanest copy (no Git ref, not nested) is kept as the primary.
func markDuplicates(addons []*models.Addon) {
	groups := map[string][]*models.Addon{}
	for _, a := range addons {
		key := strings.ToLower(a.SuggestedName)
		if key == "" {
			key = strings.ToLower(a.FolderName)
		}
		groups[key] = append(groups[key], a)
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			return duplicateRank(group[i]) < duplicateRank(group[j])
		})
		primary := group[0]
		for _, dup := range group[1:] {
			dup.AddIssue(&models.Issue{
				Kind:          models.IssueDuplicate,
				Severity:      models.SeverityWarn,
				Message:       fmt.Sprintf("Duplicate of %q", primary.FolderName),
				Suggestion:    "Merge the folders or delete the duplicate",
				Action:        models.ActionMerge,
				SuggestedName: primary.FolderName,
			})
		}
	}
}

// duplicateRank orders duplicates so the "best" copy comes first:
// clean names beat Git-ref names, top-level beats nested.
func duplicateRank(a *models.Addon) int {
	rank := 0
	if a.Nested {
		rank += 10
	}
	if a.BaseName != a.FolderName {
		rank += 5
	}
	if len(a.Issues) > 0 {
		rank += 2
	}
	return rank
}

// tocPaths returns every *.toc file directly inside the directory
// described by entries, sorted and case-insensitively matched.
func tocPaths(dir string, entries []fs.DirEntry) []string {
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func mustReadDir(path string) []fs.DirEntry {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	return entries
}
