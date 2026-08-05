package ui

import (
	"fmt"
	"path/filepath"

	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
)

// RenderPreview builds a realistic snapshot of the TUI screens with
// sample data. `wowfix preview` prints it; the README embeds it as a
// text screenshot so no terminal capture is required.
func RenderPreview() string {
	store, _ := config.NewStore()
	cfg := config.Default()
	cfg.WoWPath = `C:\Games\World of Warcraft`
	cfg.Profile = "wrath"
	log := logger.New(50)
	log.Infof("Scan complete: 9 addons, 6 problems, 1 errors")
	log.Infof("Renamed Questie-main → Questie")
	log.Infof("Flattened DPSMate-main")
	log.Infof("Validated AtlasLoot")
	log.Infof("Backup Created: Backups/2026-08-05T16-30-00")

	app := NewApp(cfg, store, log)
	app.width = 100
	app.height = 30
	app.install = &detector.Installation{
		Root:       `C:\Games\World of Warcraft`,
		Flavor:     "",
		AddonsPath: `C:\Games\World of Warcraft\Interface\AddOns`,
		Exe:        "Wow.exe",
		Version:    "3.3.5.12340",
		ProfileID:  "wrath",
		Confidence: "high",
	}
	app.scan = &models.ScanResult{
		AddonsDir: app.install.AddonsPath,
		Profile:   models.ProfileByID("wrath"),
		Addons:    sampleAddons(),
	}
	models.SortAddons(app.scan.Addons)
	app.view = viewList

	list := app.View()
	app.view = viewInspect
	app.inspectAddon = app.scan.Addons[1] // Questie-main
	inspect := app.View()

	app.view = viewList
	app.confirmTitle = "Fix Questie-main?"
	app.confirmMsg = "Rename Folder → Rename folder to \"Questie\"\nA backup is created first."
	app.view = viewConfirm
	confirm := app.View()

	return fmt.Sprintf("%s\n\n%s\n\n%s", list, inspect, confirm)
}

func sampleAddons() []*models.Addon {
	mk := func(name string, status models.AddonStatus) *models.Addon {
		return &models.Addon{FolderName: name, Path: filepath.Join("Interface", "AddOns", name), SourceDir: name, Status: status}
	}

	atlas := mk("AtlasLoot", models.StatusOK)
	atlas.TOCs = []*models.TOC{{
		Name: "AtlasLoot", Interface: 30300, Primary: true,
		Title: "AtlasLoot", Version: "7.0.4",
	}}

	questie := mk("Questie-main", models.StatusWarn)
	questie.BaseName = "Questie"
	questie.SuggestedName = "Questie"
	questie.TOCs = []*models.TOC{{Name: "Questie", Interface: 30300, Primary: true, Version: "1.12.2"}}
	questie.AddIssue(&models.Issue{
		Kind: models.IssueGitHubName, Severity: models.SeverityWarn,
		Message:    "Folder name \"Questie-main\" looks like a Git checkout ref",
		Suggestion: "Rename folder to \"Questie\"",
		Action:     models.ActionRename, SuggestedName: "Questie",
	})

	dps := mk("DPSMate", models.StatusWarn)
	dps.Nested = true
	dps.SourceDir = "DPSMate-main"
	dps.SuggestedName = "DPSMate"
	dps.TOCs = []*models.TOC{{Name: "DPSMate", Interface: 30300, Primary: true, Version: "1.0"}}
	dps.AddIssue(&models.Issue{
		Kind: models.IssueNested, Severity: models.SeverityWarn,
		Message:    "Addon is nested inside \"DPSMate-main\"",
		Suggestion: "Flatten the folder so the game finds the addon",
		Action:     models.ActionFlatten,
	})

	inv := mk("Inventory", models.StatusError)
	inv.AddIssue(&models.Issue{
		Kind: models.IssueMissingTOC, Severity: models.SeverityError,
		Message:    "No .toc file found in this folder",
		Suggestion: "Move to trash or restore the missing TOC",
		Action:     models.ActionDelete,
	})

	aux := mk("Aux", models.StatusWarn)
	aux.BaseName = "Aux"
	aux.SuggestedName = "Aux-Classic"
	aux.TOCs = []*models.TOC{{Name: "Aux-Classic", Interface: 30300, Primary: true, Version: "1.0"}}
	aux.AddIssue(&models.Issue{
		Kind: models.IssueTocMismatch, Severity: models.SeverityWarn,
		Message:    "TOC \"Aux-Classic.toc\" does not match folder name \"Aux\"",
		Suggestion: "Rename folder to \"Aux-Classic\"",
		Action:     models.ActionRename, SuggestedName: "Aux-Classic",
	})

	bigwigs := mk("BigWigs", models.StatusOK)
	bigwigs.TOCs = []*models.TOC{{Name: "BigWigs", Interface: 30300, Primary: true}}

	vanilla := mk("GFW_Shaman", models.StatusWarn)
	vanilla.TOCs = []*models.TOC{{Name: "GFW_Shaman", Interface: 11200, Primary: true}}
	vanilla.AddIssue(&models.Issue{
		Kind: models.IssueTocMismatch, Severity: models.SeverityInfo,
		Message: "Vanilla addon", Suggestion: "Compatible with Vanilla 1.12 only",
	})

	empty := mk("TempFolder", models.StatusWarn)
	empty.AddIssue(&models.Issue{
		Kind: models.IssueEmpty, Severity: models.SeverityWarn,
		Message:    "Folder is empty",
		Suggestion: "Move to trash to keep the collection clean",
		Action:     models.ActionDelete,
	})

	return []*models.Addon{atlas, questie, dps, inv, aux, bigwigs, vanilla, empty}
}
