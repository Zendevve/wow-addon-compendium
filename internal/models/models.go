// Package models defines the shared data types used across wowfix:
// addons, TOC files, issues, scan results and game profiles.
//
// The types in this package are intentionally free of I/O and UI
// concerns so they can be shared by the GUI and any future API
// surface (desktop, web, REST).
package models

import (
	"sort"
	"strings"
	"time"
)

// Severity of an issue detected on an addon.
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// AddonStatus is the overall health of an addon folder.
type AddonStatus string

const (
	StatusOK    AddonStatus = "ok"
	StatusWarn  AddonStatus = "warn"
	StatusError AddonStatus = "error"
)

// IssueKind identifies the type of problem detected.
type IssueKind string

const (
	// IssueGitHubName: folder name carries a Git ref suffix (-master/-main).
	IssueGitHubName IssueKind = "github-name"
	// IssueTocMismatch: the single TOC base name differs from the folder name.
	IssueTocMismatch IssueKind = "toc-mismatch"
	// IssueNested: the real addon sits one directory below the top level.
	IssueNested IssueKind = "nested"
	// IssueMissingTOC: no *.toc anywhere in the folder.
	IssueMissingTOC IssueKind = "missing-toc"
	// IssueMultipleTOCs: several TOCs with no name match; user must choose.
	IssueMultipleTOCs IssueKind = "multiple-tocs"
	// IssueEmpty: the folder is empty.
	IssueEmpty IssueKind = "empty"
	// IssueDuplicate: another folder provides the same addon.
	IssueDuplicate IssueKind = "duplicate"
	// IssueInvalidStructure: a top-level folder without TOC that contains
	// several TOC-bearing subfolders (zip-in-zip style extraction).
	IssueInvalidStructure IssueKind = "invalid-structure"
)

// FixAction is the repair strategy suggested for an issue.
type FixAction string

const (
	ActionNone            FixAction = ""
	ActionRename          FixAction = "rename"
	ActionFlatten         FixAction = "flatten"
	ActionResolveTOC      FixAction = "resolve-toc"
	ActionDelete          FixAction = "delete"
	ActionMerge           FixAction = "merge"
	ActionRepairStructure FixAction = "repair-structure"
)

// FixAction names for display.
func (a FixAction) Label() string {
	switch a {
	case ActionRename:
		return "Rename Folder"
	case ActionFlatten:
		return "Flatten Folder"
	case ActionResolveTOC:
		return "Pick TOC"
	case ActionDelete:
		return "Move to Trash"
	case ActionMerge:
		return "Merge Duplicates"
	case ActionRepairStructure:
		return "Repair Structure"
	default:
		return "Inspect"
	}
}

// Issue is a single problem attached to an addon. It is pure data;
// the fixer package turns it into filesystem operations.
type Issue struct {
	Kind       IssueKind
	Severity   Severity
	Message    string
	Suggestion string
	Action     FixAction
	// Options holds TOC candidate base names for IssueMultipleTOCs.
	Options []string
	// SuggestedName is the target folder name for rename-style fixes.
	SuggestedName string
}

// TOC represents a single parsed .toc file.
type TOC struct {
	Path         string
	Name         string // file base name without extension
	Title        string
	Interface    int // -1 when absent or not a number
	RawInterface string
	Version      string
	Author       string
	Notes        string
	Fields       map[string]string
	// Primary is true when this TOC's base name matches the folder name
	// (or the user picked it explicitly). Only primary TOCs drive
	// compatibility reporting.
	Primary bool
}

// Addon is one addon unit discovered by the scanner.
type Addon struct {
	// FolderName is the on-disk folder name.
	FolderName string
	// Path is the full path to the addon folder.
	Path string
	// BaseName is the folder name with Git ref suffixes stripped.
	BaseName string
	// SuggestedName is the folder name after all rename fixes.
	SuggestedName string
	// TOCs lists every TOC file belonging to the addon.
	TOCs []*TOC
	// Issues detected on this addon.
	Issues []*Issue
	// Status is derived from issues.
	Status AddonStatus
	// Nested is true when the addon sits inside another folder.
	Nested bool
	// Children holds addons found inside this folder (invalid structure).
	Children []*Addon
	// SourceDir is the top-level folder this addon came from; equal to
	// FolderName for regular addons, differs for nested/multi-addon entries.
	SourceDir string
	// SizeBytes is the total on-disk size of the folder.
	SizeBytes int64
}

// PrimaryTOC returns the TOC that drives compatibility reporting, or nil.
func (a *Addon) PrimaryTOC() *TOC {
	for _, t := range a.TOCs {
		if t.Primary {
			return t
		}
	}
	return nil
}

// AddIssue appends an issue and refreshes the derived status.
func (a *Addon) AddIssue(issue *Issue) {
	a.Issues = append(a.Issues, issue)
	switch issue.Severity {
	case SeverityError:
		a.Status = StatusError
	case SeverityWarn:
		if a.Status != StatusError {
			a.Status = StatusWarn
		}
	default:
		if a.Status == "" {
			a.Status = StatusOK
		}
	}
}

// Fixable reports whether any issue has a non-empty action.
func (a *Addon) Fixable() bool {
	for _, i := range a.Issues {
		if i.Action != ActionNone {
			return true
		}
	}
	return false
}

// ScanResult is the outcome of scanning an AddOns directory.
type ScanResult struct {
	AddonsDir string
	Addons    []*Addon
	// Errors are non-fatal problems encountered while scanning
	// (unreadable subdirectories, invalid TOCs, ...).
	Errors []error
	// Profile used for validation during this scan.
	Profile   *Profile
	ScannedAt time.Time
}

// SortAddons orders addons by status (errors first) then name, so the
// worst problems surface at the top of the list.
func SortAddons(addons []*Addon) {
	sort.SliceStable(addons, func(i, j int) bool {
		si, sj := statusRank(addons[i].Status), statusRank(addons[j].Status)
		if si != sj {
			return si < sj
		}
		return strings.ToLower(addons[i].FolderName) < strings.ToLower(addons[j].FolderName)
	})
}

func statusRank(s AddonStatus) int {
	switch s {
	case StatusError:
		return 0
	case StatusWarn:
		return 1
	default:
		return 2
	}
}

// Stats summarizes a scan.
func (r *ScanResult) Stats() (total, problems, errors int) {
	total = len(r.Addons)
	for _, a := range r.Addons {
		if len(a.Issues) > 0 {
			problems++
		}
		if a.Status == StatusError {
			errors++
		}
	}
	return total, problems, errors
}
