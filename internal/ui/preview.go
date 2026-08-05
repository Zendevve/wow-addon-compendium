package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/config"
	"github.com/wowfix/wowfix/internal/detector"
	"github.com/wowfix/wowfix/internal/logger"
	"github.com/wowfix/wowfix/internal/models"
	"github.com/wowfix/wowfix/internal/profiles"
)

// RenderPreview builds a realistic snapshot of the current TUI screens
// with sample data, one panel per view. `wowfix preview` prints it;
// the README embeds the first panel as a text screenshot so no
// terminal capture is required.
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

	// Track a couple of the sample folders in a scratch registry so the
	// main list shows real provider badges via reloadRegistryBadges.
	if reg := sampleRegistry(); reg != nil {
		app.registry = reg
		app.reloadRegistryBadges()
	}

	// Catalog samples: one addon per provider, so every badge appears.
	samples := sampleCatalogAddons()

	app.view = viewList
	list := app.View()

	app.view = viewCatalog
	app.results = samples
	app.search.SetValue("elvui")
	catPanel := app.View()

	app.view = viewUpdates
	app.updates = sampleUpdates()
	updates := app.View()

	// Detail of the row under the catalog cursor (resultCur = 0, name
	// sort): Deadly Boss Mods, a CurseForge addon with no release-notes
	// fetch so the static panel never shows a live "Loading…" state.
	app.view = viewCatalogDetail
	app.detailAddon = samples[1]
	detail := app.View()

	app.view = viewHelp
	help := app.View()

	app.view = viewProfiles
	app.profiles = sampleCollections()
	app.cfg.Collection = "raiding"
	profiles := app.View()

	app.view = viewSavedVars
	app.svAccounts = []string{"Account1", "Account2"}
	app.svAccount = "Account1"
	app.svFiles = []string{"Details", "Questie", "WeakAuras"}
	savedvars := app.View()

	return fmt.Sprintf(
		"── LIST ──\n%s\n\n── CATALOG ──\n%s\n\n── UPDATES ──\n%s\n\n── DETAIL ──\n%s\n\n── HELP ──\n%s\n\n── PROFILES ──\n%s\n\n── SAVEDVARS ──\n%s",
		list, catPanel, updates, detail, help, profiles, savedvars)
}

// sampleRegistry writes a scratch registry file with a couple of
// tracked addons and loads it, exercising the same NewRegistry path
// the app uses. A nil result leaves the list badge-free ("local").
func sampleRegistry() *catalog.Registry {
	dir, err := os.MkdirTemp("", "wowfix-preview-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	now := time.Now()
	entries := []catalog.Entry{
		{Folder: "AtlasLoot", Title: "AtlasLoot", Version: "7.0.4",
			Provider: catalog.ProviderCurseForge, ID: "atlasloot",
			Source: "curseforge:atlasloot", InstalledAt: now.Add(-72 * time.Hour)},
		{Folder: "Questie-main", Title: "Questie", Version: "1.12.2",
			Provider: catalog.ProviderGitHub, ID: "Questie/Questie",
			Source: "github:Questie/Questie", InstalledAt: now.Add(-24 * time.Hour)},
	}
	data, err := json.Marshal(map[string]any{"version": 1, "entries": entries})
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "registry.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil
	}
	reg, err := catalog.NewRegistry(path)
	if err != nil {
		return nil
	}
	return reg
}

// sampleCatalogAddons returns one catalog result per provider.
func sampleCatalogAddons() []*catalog.Addon {
	now := time.Now()
	return []*catalog.Addon{
		{Provider: catalog.ProviderGitHub, ID: "WeakAuras/WeakAuras2",
			Name: "WeakAuras 2", Author: "WeakAuras",
			Summary:       "Powerful and flexible framework for custom buff and debuff tracking.",
			LatestVersion: "5.12.10", GameVersion: "retail",
			Homepage:  "https://github.com/WeakAuras/WeakAuras2",
			UpdatedAt: now.Add(-3 * time.Hour)},
		{Provider: catalog.ProviderCurseForge, ID: "deadly-boss-mods",
			Name: "Deadly Boss Mods", Author: "Tandanu",
			Summary:       "Tactical encounter alerts for dungeons and raids.",
			LatestVersion: "10.2.30", GameVersion: "retail",
			Homepage:  "https://www.curseforge.com/wow/addons/deadly-boss-mods",
			UpdatedAt: now.Add(-26 * time.Hour)},
		{Provider: catalog.ProviderWowInterface, ID: "13707",
			Name: "Questie", Author: "Questie team",
			Summary:       "Quest helper for WoW Classic.",
			LatestVersion: "9.3.10", GameVersion: "wrath",
			Homepage:  "https://www.wowinterface.com/downloads/info13707",
			UpdatedAt: now.Add(-50 * time.Hour)},
		{Provider: catalog.ProviderTukui, ID: "elvui",
			Name: "ElvUI", Author: "Elv",
			Summary:       "A complete UI replacement for World of Warcraft.",
			LatestVersion: "13.31", GameVersion: "retail",
			Homepage:  "https://www.tukui.org/addons.php?id=1",
			UpdatedAt: now.Add(-4 * 24 * time.Hour)},
	}
}

// sampleUpdates returns three pending updates, one of which targets a
// different game-version family so the ⚠ row renders.
func sampleUpdates() []catalog.Update {
	return []catalog.Update{
		{Entry: catalog.Entry{Folder: "Questie", Title: "Questie", Version: "9.3.9",
			Provider: catalog.ProviderGitHub, ID: "Questie/Questie", Source: "Questie/Questie"},
			Latest: &catalog.Addon{Name: "Questie", Author: "Questie team",
				LatestVersion: "9.3.10", GameVersion: "wrath",
				Summary: "Quest helper for WoW Classic.", Homepage: "https://github.com/Questie/Questie"}},
		{Entry: catalog.Entry{Folder: "Details", Title: "Details! Damage Meter", Version: "1.0.4",
			Provider: catalog.ProviderCurseForge, ID: "details-damage-meter", Source: "curseforge:details-damage-meter"},
			Latest: &catalog.Addon{Name: "Details! Damage Meter", Author: "Tercio",
				LatestVersion: "1.0.5", GameVersion: "retail",
				Summary: "Combat log parser.", Homepage: "https://www.curseforge.com/wow/addons/details-damage-meter"}},
		{Entry: catalog.Entry{Folder: "AtlasLoot", Title: "AtlasLoot", Version: "7.0.3",
			Provider: catalog.ProviderCurseForge, ID: "atlasloot", Source: "curseforge:atlasloot"},
			Latest: &catalog.Addon{Name: "AtlasLoot", Author: "Lag",
				LatestVersion: "7.0.4", GameVersion: "cata",
				Summary: "Loot tables browser.", Homepage: "https://www.curseforge.com/wow/addons/atlasloot"},
			Mismatch: true},
	}
}

// sampleCollections returns three collections; the caller marks one
// active so the view shows the "(active)" tag.
func sampleCollections() []profiles.Collection {
	now := time.Now()
	mk := func(id, name string, n int, created, updated time.Time) profiles.Collection {
		addons := make([]profiles.AddonState, n)
		for i := range addons {
			addons[i] = profiles.AddonState{Folder: fmt.Sprintf("Addon-%d", i+1), Enabled: true}
		}
		return profiles.Collection{ID: id, Name: name, Addons: addons, CreatedAt: created, UpdatedAt: updated}
	}
	return []profiles.Collection{
		mk("raiding", "Raiding", 24, now.Add(-30*24*time.Hour), now.Add(-24*time.Hour)),
		mk("leveling", "Leveling", 9, now.Add(-60*24*time.Hour), now.Add(-48*time.Hour)),
		mk("pvp", "PvP", 12, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)),
	}
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
