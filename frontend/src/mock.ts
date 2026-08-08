// In-browser mock of the Go Service binding.
//
// Activated by `?mock=1` in the URL, by `window.__WOWFIX_MOCK__ === true`,
// or automatically when `window.go` is absent (plain browser, no backend).
// Every method simulates realistic latency and mutates an in-memory "disk"
// so Fix / FixAll / InstallZip produce believable follow-up scans.
//
// Screenshot helpers:
//   index.html?mock=1            -> installed state, scan view
//   index.html?mock=1&state=setup -> first-run state (no install)
//   index.html?mock=1&view=validate / view=install -> open a specific tab
//   index.html?mock=1&view=updates / view=catalog -> new-tab screenshots
//   index.html?mock=1&view=updates&tracked=none -> empty Managed section

import type {
  Service,
  State,
  Install,
  Profile,
  ScanResult,
  ValidateResult,
  FixResult,
  InstallResult,
  InstallSourceResult,
  Addon,
  Issue,
  CompatEntry,
  UpdateEntry,
  ApplyBatch,
  CatalogEntry,
  CheckUpdatesResult,
  SearchCatalogResult,
  WagoImportResult,
  CuratedResult,
  CollectionsResult,
  CollectionInfo,
  CollectionDetail,
  SwitchCollectionResult,
  InstallsStatusResult,
  InstallStatus,
  SyncResult,
  TrackedResult,
  RollbackResult,
  VersionEntry,
  VersionHistoryResult,
  InfoResult,
  ProviderInfo,
  DoctorReport,
  SavedVarsListResult,
  SavedVarsBackupResult,
  SavedVarsMigrateResult,
  BackupInfo,
  BackupResult,
  ListBackupsResult,
  RestoreBackupResult,
  ExportResult,
  ImportResult,
  ConfigView,
  SnapshotResult,
  SnapshotCheck,
} from "./types";

const DELAY_MS = 350;

function delay(ms = DELAY_MS): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  setTimeout(resolve, ms);
  return promise;
}

function mockEnabled(): boolean {
  return (
    (window as unknown as { __WOWFIX_MOCK__?: boolean }).__WOWFIX_MOCK__ ===
      true ||
    new URLSearchParams(window.location.search).get("mock") === "1"
  );
}

const PROFILES: Profile[] = [
  { id: "vanilla", name: "Vanilla 1.12", family: "vanilla", interface: 11200 },
  { id: "turtle", name: "TurtleWoW", family: "vanilla", interface: 11200 },
  { id: "tbc", name: "The Burning Crusade 2.4.3", family: "tbc", interface: 20400 },
  { id: "wrath", name: "Wrath of the Lich King 3.3.5a", family: "wrath", interface: 30300 },
  { id: "cata", name: "Cataclysm 4.3.4", family: "cata", interface: 40300 },
  { id: "classic", name: "Classic Era", family: "vanilla", interface: 11403 },
  { id: "hardcore", name: "Hardcore", family: "vanilla", interface: 11403 },
  { id: "sod", name: "Season of Discovery", family: "vanilla", interface: 11504 },
  { id: "retail", name: "Retail", family: "retail", interface: 100207 },
];

const DETECTED_INSTALLS: Install[] = [
  {
    root: "C:\\Games\\World of Warcraft",
    flavor: "_retail_",
    addons_path: "C:\\Games\\World of Warcraft\\_retail_\\Interface\\AddOns",
    exe: "C:\\Games\\World of Warcraft\\_retail_\\Wow.exe",
    version: "10.2.7.52789",
    profile_id: "retail",
    confidence: "high",
  },
  {
    root: "C:\\Games\\World of Warcraft Classic",
    flavor: "root",
    addons_path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns",
    exe: "C:\\Games\\World of Warcraft Classic\\Wow.exe",
    version: "3.4.3.54468",
    profile_id: "wrath",
    confidence: "high",
  },
  {
    root: "D:\\Games\\TurtleWoW",
    flavor: "root",
    addons_path: "D:\\Games\\TurtleWoW\\Interface\\AddOns",
    exe: "D:\\Games\\TurtleWoW\\WoW.exe",
    version: "1.17.2.5834",
    profile_id: "turtle",
    confidence: "medium",
  },
];

interface MockInstallState {
  root: string;
  flavor: string;
  addons_dir: string;
  profile_id: string;
}

interface MockCollection {
  id: string;
  name: string;
  addons: { folder: string; enabled: boolean }[];
}

interface MockDB {
  install: MockInstallState | null;
  addons: Addon[];
  scanErrors: string[];
  scannedAt: string;
  lastInstall: InstallResult | null;
  tracked: UpdateEntry[];
  /** Pin/ignore flags per tracked folder, mirroring the registry. */
  trackedState: Record<string, { pinned: boolean; ignored: boolean }>;
  /** Per-addon version logs, newest first (mirrors the registry history). */
  history: Record<string, VersionEntry[]>;
  collections: MockCollection[];
  activeCollectionId: string;
  backups: BackupInfo[];
}

// Version logs seeded per tracked addon. Questie (github) and DBM
// (curseforge) are addressable; WeakAuras (tukui) serves only the
// latest version.
const SEED_HISTORY: Record<string, VersionEntry[]> = {
  Questie: [
    { version: "1.12.2", provider: "github", source: "https://github.com/Questie/Questie", ref: "v1.12.2", at: "2026-08-06T14:20:00.000Z" },
    { version: "1.12.0", provider: "github", source: "https://github.com/Questie/Questie", ref: "v1.12.0", at: "2026-07-30T09:05:00.000Z" },
    { version: "1.11.4", provider: "github", source: "https://github.com/Questie/Questie", ref: "v1.11.4", at: "2026-07-12T17:40:00.000Z" },
  ],
  DeadlyBossMods: [
    { version: "9.5.2", provider: "curseforge", source: "https://www.curseforge.com/wow/addons/deadly-boss-mods", ref: "29847123", at: "2026-08-05T11:00:00.000Z" },
    { version: "9.5.0", provider: "curseforge", source: "https://www.curseforge.com/wow/addons/deadly-boss-mods", ref: "29710001", at: "2026-07-28T08:30:00.000Z" },
  ],
  WeakAuras: [
    { version: "5.12.0", provider: "tukui", source: "https://www.tukui.org/addons.php?id=weakauras2", at: "2026-08-04T12:15:00.000Z" },
    { version: "5.11.9", provider: "tukui", source: "https://www.tukui.org/addons.php?id=weakauras2", at: "2026-07-15T10:00:00.000Z" },
  ],
};

// Refs the provider no longer serves: a deleted GitHub tag. Rolling
// back to such a version fails with the honest not-found error.
const GONE_REFS = new Set(["v1.11.4"]);

// Seeded collections: "pve" is the active loadout (4 enabled of 6), "pvp"
// is inactive. SetCollectionAddon toggles `enabled` on the detail rows.
const SEED_COLLECTIONS: MockCollection[] = [
  {
    id: "pve",
    name: "pve",
    addons: [
      { folder: "Questie", enabled: true },
      { folder: "DeadlyBossMods", enabled: true },
      { folder: "AtlasLoot", enabled: true },
      { folder: "WeakAuras2", enabled: true },
      { folder: "Inventory", enabled: false },
      { folder: "TempFolder", enabled: false },
    ],
  },
  {
    id: "pvp",
    name: "pvp",
    addons: [
      { folder: "ArenaMaster", enabled: true },
      { folder: "Gladius", enabled: true },
      { folder: "InterruptBar", enabled: true },
      { folder: "WeakAuras2", enabled: true },
      { folder: "VoiceOverlay", enabled: false },
    ],
  },
];

// Per-install status fed to InstallsStatus: one Wrath install in the
// attention band (72), one retail install with a missing AddOns folder,
// and a healthy Classic install (95).
const INSTALLS_STATUS: InstallStatus[] = [
  {
    root: "C:\\Games\\ChromieCraft",
    flavor: "_classic_",
    addons_path: "C:\\Games\\ChromieCraft\\_classic_\\Interface\\AddOns",
    exe: "C:\\Games\\ChromieCraft\\_classic_\\Wow.exe",
    version: "3.3.5a",
    profile_id: "wrath",
    confidence: "high",
    exists: true,
    addons: 37,
    problems: 6,
    errors: 2,
    health: 72,
  },
  {
    root: "C:\\Games\\World of Warcraft",
    flavor: "_retail_",
    addons_path: "C:\\Games\\World of Warcraft\\_retail_\\Interface\\AddOns",
    exe: "C:\\Games\\World of Warcraft\\_retail_\\Wow.exe",
    version: "10.2.7.52789",
    profile_id: "retail",
    confidence: "high",
    exists: false,
    addons: 0,
    problems: 0,
    errors: 0,
    health: 95,
  },
  {
    root: "C:\\Games\\World of Warcraft Classic",
    flavor: "root",
    addons_path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns",
    exe: "C:\\Games\\World of Warcraft Classic\\Wow.exe",
    version: "3.4.3.54468",
    profile_id: "wrath",
    confidence: "high",
    exists: true,
    addons: 12,
    problems: 1,
    errors: 0,
    health: 95,
  },
];

// Saved-variables accounts under WTF\Account, with the per-account file
// stems the SavedVarsList view renders as "<name>.lua".
const SAVED_VAR_ACCOUNTS = ["WrathMain", "TurtleAlt", "RetailMain"];

const SAVED_VAR_FILES = ["QuestieDB", "AtlasLoot", "DBM", "WeakAuras", "AddOns"];

// Editable settings for the Settings view. SetConfigKey mutates these;
// Config() merges them with the live install state.
const SETTINGS: {
  theme: string;
  auto_backup: boolean;
  confirmations: boolean;
  backups_dir: string;
  curseforge_api_key: string;
  collections_dir: string;
} = {
  theme: "dark",
  auto_backup: true,
  confirmations: true,
  backups_dir: "C:\\Users\\mock\\wowfix\\backups",
  curseforge_api_key: "cf-************",
  collections_dir: "C:\\Users\\mock\\wowfix\\collections",
};

// Snapshot history for the Backups view. BackupNow unshifts a fresh entry so
// a create → list cycle shows the new snapshot.
const SEED_BACKUPS: BackupInfo[] = [
  {
    id: "2026-08-07T10-00-00.000Z",
    created_at: "2026-08-07T10:00:00.000Z",
    reason: "manual",
    folders: ["Questie", "DeadlyBossMods", "WeakAuras"],
  },
  {
    id: "2026-08-05T18-30-00.000Z",
    created_at: "2026-08-05T18:30:00.000Z",
    reason: "collection switch pvp",
    folders: ["Questie", "AtlasLoot"],
  },
];

// Addons the updater follows. Mutable: ApplyUpdate / ApplyAllUpdates bump
// current_version to latest_version (and clear flavor_mismatch), so the next
// CheckUpdates reflects what was applied.
const TRACKED_UPDATES: UpdateEntry[] = [
  {
    folder: "Questie",
    title: "Questie",
    current_version: "1.12.2",
    latest_version: "1.13.0",
    provider: "github",
    id: "Questie/Questie",
    source: "https://github.com/Questie/Questie",
    flavor_mismatch: false,
    flavor_label: "",
  },
  {
    folder: "DeadlyBossMods",
    title: "Deadly Boss Mods",
    current_version: "9.5.2",
    latest_version: "9.5.9",
    provider: "curseforge",
    id: "deadly-boss-mods",
    source: "https://www.curseforge.com/wow/addons/deadly-boss-mods",
    flavor_mismatch: true,
    flavor_label: "retail addon · profile wrath",
  },
  {
    folder: "WeakAuras",
    title: "WeakAuras 2",
    current_version: "5.12.0",
    latest_version: "5.12.5",
    provider: "tukui",
    id: "weakauras2",
    source: "https://www.tukui.org/addons.php?id=weakauras2",
    flavor_mismatch: false,
    flavor_label: "",
  },
];

// Fixed catalog pool, one representative addon per provider. SearchCatalog
// filters by query substring; the view filters by provider chip client-side.
const CATALOG_POOL: CatalogEntry[] = [
  {
    provider: "github",
    name: "Questie",
    author: "Questie Team",
    summary: "Quest helper for Classic and Wrath with full quest data, waypoints and objectives.",
    latest_version: "1.13.0",
    game_version: "wrath",
    id: "Questie/Questie",
    homepage: "https://github.com/Questie/Questie",
  },
  {
    provider: "github",
    name: "Grid2",
    author: "michael",
    summary: "Modular grid-based raid frames with lightweight, customizable unit bars.",
    latest_version: "2.0.9",
    game_version: "",
    id: "michael/grid2",
    homepage: "https://github.com/michael/grid2",
  },
  {
    provider: "curseforge",
    name: "Deadly Boss Mods",
    author: "MysticalOS",
    summary: "Boss encounter alerts, timers and warnings for dungeons and raids.",
    latest_version: "9.5.9",
    game_version: "retail",
    id: "deadly-boss-mods",
    homepage: "https://www.curseforge.com/wow/addons/deadly-boss-mods",
  },
  {
    provider: "curseforge",
    name: "Details!",
    author: "Terciob",
    summary: "Damage and healing meter with deep breakdowns, graphs and custom skins.",
    latest_version: "3.10.6",
    game_version: "retail",
    id: "details",
    homepage: "https://www.curseforge.com/wow/addons/details",
  },
  {
    provider: "wowinterface",
    name: "Plater Nameplates",
    author: "Devv",
    summary: "Highly configurable nameplates with threat coloring, scripts and profiles.",
    latest_version: "1.11.4",
    game_version: "retail",
    id: "plater-nameplates",
    homepage: "https://www.wowinterface.com/downloads/info24911-PlaterNameplates.html",
  },
  {
    provider: "wowinterface",
    name: "BigWigs Boss Mods",
    author: "BigWigs",
    summary: "Boss mod with precise pull timers and minimal, readable alerts.",
    latest_version: "10.2.2",
    game_version: "wrath",
    id: "bigwigs-boss-mods",
    homepage: "https://www.wowinterface.com/downloads/info3656-BigWigsBossMods.html",
  },
  {
    provider: "tukui",
    name: "WeakAuras 2",
    author: "Luxocracy",
    summary: "Powerful aura display framework, icons, bars, textures and custom code.",
    latest_version: "5.12.5",
    game_version: "retail",
    id: "weakauras2",
    homepage: "https://www.tukui.org/addons.php?id=weakauras2",
  },
  {
    provider: "tukui",
    name: "ElvUI",
    author: "Elv",
    summary: "Complete UI replacement: unit frames, action bars, bags and chat skins.",
    latest_version: "13.85",
    game_version: "retail",
    id: "elvui",
    homepage: "https://www.tukui.org/addons.php?id=elvui",
  },
  {
    provider: "wago",
    name: "Luxthos WeakAuras Suite",
    author: "Luxthos",
    summary: "WEAKAURA suite: class rotation, cooldown and utility auras with full customization.",
    latest_version: "3.4.2",
    game_version: "",
    id: "pvBs8htuW",
    homepage: "https://wago.io/pvBs8htuW",
  },
  {
    provider: "wago",
    name: "Jundies - Midnight M+ and Raid Plater",
    author: "Jundies",
    summary: "PLATER profile: nameplate styling, buff/debuff tracking and M+ affix bars.",
    latest_version: "3.2.0",
    game_version: "",
    id: "ak3iS95aa",
    homepage: "https://wago.io/ak3iS95aa",
  },
];

// Curated private-server sets, keyed by profile family. `installed` is
// derived live from the mock addons db (a matching folder with a TOC), so a
// curated install flips its row to Installed on the next Curated() call.
const CURATED_MANIFEST: Record<
  string,
  { name: string; source: string; summary: string; homepage: string }[]
> = {
  vanilla: [
    {
      name: "pfQuest",
      source: "shagu/pfQuest",
      summary: "Quest helper for Vanilla and TurtleWoW with extensive quest data.",
      homepage: "https://github.com/shagu/pfQuest",
    },
    {
      name: "ShaguTweaks",
      source: "shagu/ShaguTweaks",
      summary: "Lightweight UI tweaks for a clean Vanilla experience.",
      homepage: "https://github.com/shagu/ShaguTweaks",
    },
  ],
  wrath: [
    {
      name: "Questie",
      source: "Vendethiel/Questie",
      summary: "Quest helper for WotLK with full quest data, waypoints and objectives.",
      homepage: "https://github.com/Vendethiel/Questie",
    },
    {
      name: "Details",
      source: "Terciob/Details",
      summary: "Damage and healing meter with deep breakdowns and graphs.",
      homepage: "https://github.com/Terciob/Details",
    },
    {
      name: "Bartender4",
      source: "Tukz/Bartender4",
      summary: "Fully customizable action bars for any class and spec.",
      homepage: "https://github.com/Tukz/Bartender4",
    },
  ],
};

function issue(
  partial: Partial<Issue> & { kind: string; severity: Issue["severity"] },
): Issue {
  return {
    message: "",
    suggestion: "",
    action: "",
    action_label: "",
    options: [],
    suggested_name: "",
    ...partial,
  };
}

function compat(
  toc: string,
  detected: number,
  expected = 30300,
): CompatEntry {
  const status =
    detected <= 0
      ? "unknown"
      : detected === expected
        ? "compatible"
        : detected < 20000
          ? "vanilla"
          : detected >= 100000
            ? "retail"
            : "mismatch";
  const label =
    status === "compatible"
      ? "Compatible"
      : status === "vanilla"
        ? "Vanilla Addon"
        : status === "retail"
          ? "Retail Addon"
          : status === "mismatch"
            ? "Version Mismatch"
            : "Unknown";
  return { toc, expected, detected, status, label };
}

function seedAddons(): Addon[] {
  const base: Array<Omit<Addon, "tracked" | "drifted" | "pinned" | "ignored">> = [
    {
      folder_name: "Inventory",
      base_name: "Inventory",
      suggested_name: "Inventory",
      status: "error",
      nested: false,
      size_bytes: 14520,
      fixable: true,
      health: 70,
      toc: null,
      issues: [
        issue({
          kind: "missing-toc",
          severity: "error",
          message: "No .toc file found in this folder",
          suggestion:
            "The folder contains no TOC file, so the game will not load it as an addon.",
          action: "delete",
          action_label: "Move to Trash",
        }),
      ],
      compat: [],
    },
    {
      folder_name: "Atlas",
      base_name: "Atlas",
      suggested_name: "Atlas",
      status: "warn",
      nested: false,
      size_bytes: 3841204,
      fixable: true,
      health: 85,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\Atlas\\Atlas.toc",
        name: "Atlas",
        title: "Atlas",
        interface: 100207,
        raw_interface: "100207",
        version: "7.0.4",
        primary: true,
      },
      issues: [
        issue({
          kind: "multiple-tocs",
          severity: "warn",
          message: "Multiple TOCs found. Pick which one defines this addon",
          suggestion:
            "Atlas.toc, Atlas_Wrath.toc and Atlas_TBC.toc are present; the chosen TOC drives compatibility reporting.",
          action: "resolve-toc",
          action_label: "Pick TOC",
          options: ["Atlas", "Atlas_Wrath", "Atlas_TBC"],
        }),
      ],
      compat: [
        compat("Atlas.toc", 100207),
        compat("Atlas_Wrath.toc", 30300),
        compat("Atlas_TBC.toc", 20400),
      ],
    },
    {
      folder_name: "AuxUI",
      base_name: "AuxUI",
      suggested_name: "AuxUI-Classic",
      status: "warn",
      nested: false,
      size_bytes: 268912,
      fixable: true,
      health: 85,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\AuxUI\\AuxUI-Classic.toc",
        name: "AuxUI-Classic",
        title: "Aux",
        interface: 11200,
        raw_interface: "11200",
        version: "1.0",
        primary: false,
      },
      issues: [
        issue({
          kind: "toc-mismatch",
          severity: "warn",
          message: "TOC \"AuxUI-Classic.toc\" does not match the folder name",
          suggestion:
            "Rename the folder to match the TOC so the game loads it reliably.",
          action: "rename",
          action_label: "Rename Folder",
          suggested_name: "AuxUI-Classic",
        }),
      ],
      compat: [compat("AuxUI-Classic.toc", 11200)],
    },
    {
      folder_name: "DeadlyBossMods",
      base_name: "DeadlyBossMods",
      suggested_name: "DeadlyBossMods",
      status: "warn",
      nested: false,
      size_bytes: 9812080,
      fixable: false,
      health: 85,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\DeadlyBossMods\\DBM.toc",
        name: "DBM",
        title: "Deadly Boss Mods",
        interface: 100207,
        raw_interface: "100207",
        version: "9.5.2",
        primary: true,
      },
      issues: [
        issue({
          kind: "compat",
          severity: "warn",
          message: "Targets Retail (interface 100207), not this profile",
          suggestion:
            "DBM is installed for Retail; it will not load on Wrath without the Classic version.",
        }),
      ],
      compat: [compat("DBM.toc", 100207)],
    },
    {
      folder_name: "DPSMate-main",
      base_name: "DPSMate",
      suggested_name: "DPSMate",
      status: "warn",
      nested: true,
      size_bytes: 892318,
      fixable: true,
      health: 85,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\DPSMate-main\\DPSMate\\DPSMate.toc",
        name: "DPSMate",
        title: "DPSMate",
        interface: 11200,
        raw_interface: "11200",
        version: "1.0",
        primary: true,
      },
      issues: [
        issue({
          kind: "nested",
          severity: "warn",
          message: "Addon is nested inside another folder",
          suggestion:
            "Move the addon to the top level of Interface/AddOns so the game detects it.",
          action: "flatten",
          action_label: "Flatten Folder",
          suggested_name: "DPSMate",
        }),
      ],
      compat: [compat("DPSMate.toc", 11200)],
    },
    {
      folder_name: "Questie",
      base_name: "Questie",
      suggested_name: "Questie",
      status: "warn",
      nested: false,
      size_bytes: 5321088,
      fixable: true,
      health: 85,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\Questie\\Questie.toc",
        name: "Questie",
        title: "Questie",
        interface: 30300,
        raw_interface: "30300",
        version: "1.12.2",
        primary: true,
      },
      issues: [
        issue({
          kind: "duplicate",
          severity: "warn",
          message: "Another folder \"Questie-main\" provides the same addon",
          suggestion:
            "Merge the duplicate folders so the game does not load the addon twice.",
          action: "merge",
          action_label: "Merge Duplicates",
          options: ["Questie-main"],
        }),
      ],
      compat: [compat("Questie.toc", 30300)],
    },
    {
      folder_name: "Questie-main",
      base_name: "Questie",
      suggested_name: "Questie",
      status: "warn",
      nested: false,
      size_bytes: 5321088,
      fixable: true,
      health: 85,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\Questie-main\\Questie.toc",
        name: "Questie",
        title: "Questie",
        interface: 30300,
        raw_interface: "30300",
        version: "1.12.2",
        primary: true,
      },
      issues: [
        issue({
          kind: "github-name",
          severity: "warn",
          message: "Folder name \"Questie-main\" looks like a GitHub download",
          suggestion:
            "Rename to \"Questie\" to keep version suffixes out of the folder name.",
          action: "rename",
          action_label: "Rename Folder",
          suggested_name: "Questie",
        }),
      ],
      compat: [compat("Questie.toc", 30300)],
    },
    {
      folder_name: "TempFolder",
      base_name: "TempFolder",
      suggested_name: "TempFolder",
      status: "warn",
      nested: false,
      size_bytes: 0,
      fixable: true,
      health: 85,
      toc: null,
      issues: [
        issue({
          kind: "empty",
          severity: "warn",
          message: "Folder is empty",
          suggestion: "It does not contain any addon files.",
          action: "delete",
          action_label: "Move to Trash",
        }),
      ],
      compat: [],
    },
    {
      folder_name: "AtlasLoot",
      base_name: "AtlasLoot",
      suggested_name: "AtlasLoot",
      status: "ok",
      nested: false,
      size_bytes: 2411520,
      fixable: false,
      health: 100,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\AtlasLoot\\AtlasLoot.toc",
        name: "AtlasLoot",
        title: "AtlasLoot",
        interface: 30300,
        raw_interface: "30300",
        version: "7.0.4",
        primary: true,
      },
      issues: [],
      compat: [compat("AtlasLoot.toc", 30300)],
    },
    {
      folder_name: "BigWigs",
      base_name: "BigWigs",
      suggested_name: "BigWigs",
      status: "ok",
      nested: false,
      size_bytes: 6123520,
      fixable: false,
      health: 100,
      toc: {
        path: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns\\BigWigs\\BigWigs.toc",
        name: "BigWigs",
        title: "BigWigs",
        interface: 30300,
        raw_interface: "30300",
        version: "",
        primary: true,
      },
      issues: [],
      compat: [compat("BigWigs.toc", 30300)],
    },
  ];
  // Integrity defaults: untracked by default. Questie is the seeded
  // tracked addon and drifted, so the scan view shows the modified
  // badge + Restore button out of the box (mock screenshot workflow).
  // Questie is pinned and DeadlyBossMods ignored, mirroring the
  // Managed-section seed, so the scan rows carry their state badges.
  return base.map((a) => {
    if (a.folder_name === "Questie") {
      return {
        ...a,
        tracked: true,
        drifted: true,
        tracked_source: "Vendethiel/Questie",
        pinned: true,
        ignored: false,
      };
    }
    if (a.folder_name === "DeadlyBossMods") {
      return {
        ...a,
        tracked: true,
        drifted: false,
        tracked_source: "https://www.curseforge.com/wow/addons/deadly-boss-mods",
        pinned: false,
        ignored: true,
      };
    }
    return { ...a, tracked: false, drifted: false, pinned: false, ignored: false };
  });
}

// Health score mirrors the backend rule: 100 minus 30 per error issue,
// 15 per warn, 5 per info, clamped at 0.
function healthOf(a: Addon): number {
  let h = 100;
  for (const i of a.issues) {
    h -= i.severity === "error" ? 30 : i.severity === "warn" ? 15 : 5;
  }
  return Math.max(0, h);
}

// recordVersion prepends a history entry for a tracked addon, newest
// first, mirroring the registry's bounded version log.
function recordVersion(db: MockDB, u: UpdateEntry, version: string): void {
  const history = db.history[u.folder] ?? [];
  db.history[u.folder] = [
    {
      version,
      provider: u.provider,
      source: u.source,
      ref: u.provider === "github" ? `v${version}` : "",
      at: new Date().toISOString(),
    },
    ...history,
  ];
}

function freshDB(): MockDB {
  return {
    install: {
      root: "C:\\Games\\World of Warcraft Classic",
      flavor: "root",
      addons_dir: "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns",
      profile_id: "wrath",
    },
    addons: seedAddons(),
    scanErrors: [],
    scannedAt: new Date().toISOString(),
    lastInstall: null,
    tracked: TRACKED_UPDATES.map((u) => ({ ...u })),
    trackedState: {
      Questie: { pinned: false, ignored: false },
      DeadlyBossMods: { pinned: false, ignored: true },
      WeakAuras: { pinned: false, ignored: false },
    },
    history: Object.fromEntries(
      Object.entries(SEED_HISTORY).map(([k, v]) => [k, v.map((e) => ({ ...e }))]),
    ),
    collections: SEED_COLLECTIONS.map((c) => ({
      id: c.id,
      name: c.name,
      addons: c.addons.map((a) => ({ ...a })),
    })),
    activeCollectionId: "pve",
    backups: SEED_BACKUPS.map((b) => ({ ...b, folders: [...b.folders] })),
  };
}

function recomputeStats(db: MockDB): ScanResult["stats"] {
  let problems = 0;
  let errors = 0;
  for (const a of db.addons) {
    if (a.issues.length > 0) problems++;
    if (a.status === "error") errors++;
  }
  return { total: db.addons.length, problems, errors };
}

function scanOf(db: MockDB): ScanResult {
  return {
    addons_dir: db.install?.addons_dir ?? "",
    profile_id: db.install?.profile_id ?? "wrath",
    scanned_at: db.scannedAt,
    addons: db.addons.map((a) => ({ ...a, health: healthOf(a) })),
    errors: [...db.scanErrors],
    stats: recomputeStats(db),
  };
}

function validateOf(db: MockDB): ValidateResult {
  const expected = PROFILES.find((p) => p.id === db.install?.profile_id)
    ?.interface ?? 30300;
  const addons = db.addons
    .filter((a) => a.toc)
    .map((a) => {
      const c = a.compat.find((x) => x.toc === `${a.toc!.name}.toc`);
      return {
        folder_name: a.folder_name,
        toc: `${a.toc!.name}.toc`,
        expected,
        detected: c?.detected ?? a.toc!.interface,
        status: c?.status ?? "unknown",
        label: c?.label ?? "Unknown",
      };
    });
  const rank = { mismatch: 0, retail: 1, vanilla: 2, unknown: 3, compatible: 4 };
  addons.sort(
    (a, b) =>
      (rank[a.status as keyof typeof rank] ?? 5) -
      (rank[b.status as keyof typeof rank] ?? 5) ||
      a.folder_name.localeCompare(b.folder_name),
  );
  return { profile_id: db.install?.profile_id ?? "wrath", expected, addons };
}

export function createMockService(): Service {
  const db = freshDB();
  const setupOnly = new URLSearchParams(window.location.search).get("state") === "setup";
  if (setupOnly) db.install = null;
  // ?profile=<id> switches the active mock profile (e.g. retail) so
  // screenshot workflows can exercise family-specific behavior.
  const profileParam = new URLSearchParams(window.location.search).get("profile");
  if (profileParam && db.install && PROFILES.some((p) => p.id === profileParam)) {
    db.install.profile_id = profileParam;
  }
  // ?tracked=none renders the updates view with an empty Managed section
  // (screenshot / empty-state workflow).
  const trackedNone =
    new URLSearchParams(window.location.search).get("tracked") === "none";

  return {
    async GetState(): Promise<State> {
      await delay(120);
      const profile = PROFILES.find((p) => p.id === db.install?.profile_id);
      return {
        version: "0.1.0",
        wow_path: db.install?.root ?? "",
        flavor: db.install?.flavor ?? "",
        addons_dir: db.install?.addons_dir ?? "",
        profile_id: db.install?.profile_id ?? "wrath",
        profile_name: profile?.name ?? "Wrath of the Lich King 3.3.5a",
        has_install: db.install !== null,
        auto_backup: true,
        confirmations: true,
      };
    },

    async DetectInstalls(): Promise<Install[]> {
      await delay(650);
      return DETECTED_INSTALLS.map((i) => ({ ...i }));
    },

    async SetInstall(root: string, flavor: string): Promise<Install> {
      await delay(450);
      const flavorDir = flavor === "root" ? "" : `${flavor}\\`;
      const addons_dir = `${root.replace(/[\\/]+$/, "")}\\${flavorDir}Interface\\AddOns`;
      const found = DETECTED_INSTALLS.find(
        (i) => i.root === root && i.flavor === flavor,
      );
      db.install = {
        root,
        flavor,
        addons_dir,
        profile_id: found?.profile_id ?? "wrath",
      };
      return { root, flavor, addons_dir, profile_id: db.install.profile_id };
    },

    async SetProfile(id: string): Promise<void> {
      await delay(200);
      if (db.install) db.install.profile_id = id;
    },

    async Profiles(): Promise<Profile[]> {
      await delay(80);
      return PROFILES.map((p) => ({ ...p }));
    },

    async Scan(): Promise<ScanResult> {
      await delay(750);
      db.scannedAt = new Date().toISOString();
      return scanOf(db);
    },

    async Validate(): Promise<ValidateResult> {
      await delay(550);
      return validateOf(db);
    },

    async Fix(folderName: string, allowDestructive: boolean): Promise<FixResult> {
      await delay(700);
      return applyFix(db, folderName, allowDestructive);
    },

    async FixAll(allowDestructive: boolean): Promise<FixResult> {
      await delay(1100);
      const fixes: FixResult["fixes"] = [];
      const snapshot = [...db.addons];
      for (const a of snapshot) {
        const r = applyFixToAddon(db, a, allowDestructive);
        if (r) fixes.push(r);
      }
      const fixed = fixes.filter((f) => f.ok).length;
      const failed = fixes.length - fixed;
      return { fixes, fixed, failed };
    },

    async InstallZip(zipPath: string, allowReplace: boolean): Promise<InstallResult> {
      await delay(900);
      const name = zipPath.split(/[\\/]/).pop() ?? zipPath;
      if (!/\.zip$/i.test(name)) {
        return {
          installed: 0,
          replaced: 0,
          skipped: 0,
          errors: [`Not a ZIP archive: "${name}"`],
        };
      }
      const exists = db.addons.some(
        (a) => a.folder_name.toLowerCase() === "weakauras",
      );
      let installed = 0;
      let replaced = 0;
      let skipped = 0;
      if (!exists) {
        db.addons.push({
          folder_name: "WeakAuras",
          base_name: "WeakAuras",
          suggested_name: "WeakAuras",
          status: "ok",
          nested: false,
          size_bytes: 2842624,
          fixable: false,
          health: 100,
          tracked: false,
          drifted: false,
          pinned: false,
          ignored: false,
          toc: {
            path: `${db.install?.addons_dir ?? "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns"}\\WeakAuras\\WeakAuras.toc`,
            name: "WeakAuras",
            title: "WeakAuras",
            interface: 30300,
            raw_interface: "30300",
            version: "5.12.5",
            primary: true,
          },
          issues: [],
          compat: [compat("WeakAuras.toc", 30300)],
        });
        installed = 1;
      } else if (allowReplace) {
        replaced = 1;
      } else {
        skipped = 1;
      }
      const result: InstallResult = { installed, replaced, skipped, errors: [] };
      db.lastInstall = result;
      db.scannedAt = new Date().toISOString();
      return result;
    },

    async CheckUpdates(): Promise<CheckUpdatesResult> {
      await delay(600);
      // Pinned entries are locked at their current version and ignored
      // entries are excluded from update management — neither is ever
      // reported as updatable (mirrors the Go updater).
      const updates = db.tracked.filter((u) => {
        const st = db.trackedState[u.folder] ?? { pinned: false, ignored: false };
        return (
          !st.pinned && !st.ignored && u.current_version !== u.latest_version
        );
      });
      return {
        updates: updates.map((u) => ({ ...u })),
        errors: [],
        checked_at: new Date().toISOString(),
      };
    },

    async ApplyUpdate(folder: string, allowReplace: boolean): Promise<ApplyBatch> {
      await delay(700);
      const u = db.tracked.find((x) => x.folder === folder);
      if (!u) {
        return {
          applied: [
            { folder, ok: false, message: "Addon is not tracked", error: "unknown folder" },
          ],
          applied_count: 0,
          failed_count: 1,
        };
      }
      recordVersion(db, u, u.latest_version);
      u.current_version = u.latest_version;
      u.flavor_mismatch = false;
      u.flavor_label = "";
      return {
        applied: [{ folder, ok: true, message: `Updated to ${u.latest_version}`, error: "" }],
        applied_count: 1,
        failed_count: 0,
      };
    },

    async ApplyAllUpdates(allowReplace: boolean): Promise<ApplyBatch> {
      await delay(1100);
      const applied: ApplyBatch["applied"] = [];
      for (const u of db.tracked) {
        if (u.current_version === u.latest_version) continue;
        recordVersion(db, u, u.latest_version);
        u.current_version = u.latest_version;
        u.flavor_mismatch = false;
        u.flavor_label = "";
        applied.push({
          folder: u.folder,
          ok: true,
          message: `Updated to ${u.latest_version}`,
          error: "",
        });
      }
      return { applied, applied_count: applied.length, failed_count: 0 };
    },

    async TrackedAddons(): Promise<TrackedResult> {
      await delay(400);
      if (trackedNone) return { addons: [] };
      return {
        addons: db.tracked.map((u) => {
          const st = db.trackedState[u.folder] ?? {
            pinned: false,
            ignored: false,
          };
          return {
            folder: u.folder,
            title: u.title,
            version: u.current_version,
            provider: u.provider,
            id: u.id,
            source: u.source,
            pinned: st.pinned,
            ignored: st.ignored,
            installed_at: "2026-08-01T09-15-00.000Z",
            has_history: (db.history[u.folder] ?? []).length > 0,
          };
        }),
      };
    },

    async SetAddonPinned(folder: string, pinned: boolean): Promise<void> {
      await delay(250);
      const st = db.trackedState[folder];
      if (!st) throw new Error(`addon "${folder}" not tracked in registry`);
      st.pinned = pinned;
    },

    async SetAddonIgnored(folder: string, ignored: boolean): Promise<void> {
      await delay(250);
      const st = db.trackedState[folder];
      if (!st) throw new Error(`addon "${folder}" not tracked in registry`);
      st.ignored = ignored;
    },

    async RollbackAddon(folder: string): Promise<RollbackResult> {
      await delay(700);
      const u = db.tracked.find((x) => x.folder === folder);
      const st = db.trackedState[folder];
      if (!u || !st) throw new Error(`addon "${folder}" not tracked in registry`);
      st.pinned = true; // rollback stops updates until unpinned
      return {
        folder: u.folder,
        restored_from: "2026-08-07T10-00-00.000",
        version: u.current_version,
        pinned: true,
        message: "restored from snapshot 2026-08-07T10-00-00.000 and pinned",
      };
    },

    async ListAddonVersions(folder: string): Promise<VersionHistoryResult> {
      await delay(350);
      const u = db.tracked.find((x) => x.folder === folder);
      if (!u) throw new Error(`addon "${folder}" not tracked in registry`);
      const versions = (db.history[folder] ?? []).map((v) => ({ ...v }));
      return { folder: u.folder, current: u.current_version, versions };
    },

    async RollbackToVersion(folder: string, version: string): Promise<InstallSourceResult> {
      await delay(800);
      const u = db.tracked.find((x) => x.folder === folder);
      if (!u) throw new Error(`addon "${folder}" not tracked in registry`);
      const entry = (db.history[folder] ?? []).find((v) => v.version === version);
      if (!entry) throw new Error(`no recorded version "${version}" for "${folder}"`);
      // Providers that only serve the latest version cannot re-download
      // a past one: the honest error, never a silent latest install.
      if (u.provider === "wowinterface" || u.provider === "tukui") {
        throw new Error(
          `${u.provider} can no longer re-download version "${version}" — only the latest is available`,
        );
      }
      // A recorded ref the provider has since dropped (deleted tag).
      if (entry.ref && GONE_REFS.has(entry.ref)) {
        throw new Error(`github: no release or tag "${entry.ref}" for ${u.id}`);
      }
      recordVersion(db, u, version);
      u.current_version = version;
      u.latest_version = version;
      u.flavor_mismatch = false;
      u.flavor_label = "";
      return { installed: [folder], replaced: [folder], skipped: [], errors: [] };
    },

    async SearchCatalog(query: string): Promise<SearchCatalogResult> {
      await delay(500);
      const q = query.trim().toLowerCase();
      const results = CATALOG_POOL.filter(
        (c) =>
          q.length === 0 ||
          [c.name, c.author, c.summary, c.id].join(" ").toLowerCase().includes(q),
      ).map((c) => ({ ...c }));
      return { results, errors: [] };
    },

    async Curated(): Promise<CuratedResult> {
      await delay(450);
      const profile = PROFILES.find((p) => p.id === db.install?.profile_id);
      const family = profile?.family ?? "wrath";
      const set = CURATED_MANIFEST[family] ?? [];
      return {
        family,
        label:
          family === "vanilla"
            ? "Vanilla 1.12 / Turtle-style clones"
            : family === "wrath"
              ? "WotLK 3.3.5a (ChromieCraft)"
              : "",
        profile_id: db.install?.profile_id ?? "wrath",
        addons: set.map((m) => {
          const folder = folderFor(m.name);
          const existing = db.addons.find(
            (a) => a.folder_name.toLowerCase() === folder.toLowerCase(),
          );
          return {
            name: m.name,
            source: m.source,
            summary: m.summary,
            homepage: m.homepage,
            installed: existing !== undefined,
            installed_version: existing?.toc?.version ?? "",
          };
        }),
      };
    },

    async InstallSource(source: string, allowReplace: boolean): Promise<InstallSourceResult> {
      await delay(900);
      const src = source.trim();
      const found =
        CATALOG_POOL.find((c) => c.id.toLowerCase() === src.toLowerCase()) ?? null;
      const display = found?.name ?? displayNameFromSource(src);
      const folder = found ? folderFor(found.name) : folderFor(displayNameFromSource(src));
      const exists = db.addons.some(
        (a) => a.folder_name.toLowerCase() === folder.toLowerCase(),
      );
      if (exists && !allowReplace) {
        return { installed: [], replaced: [], skipped: [folder], errors: [] };
      }
      if (!exists) {
        db.addons.push({
          folder_name: folder,
          base_name: folder,
          suggested_name: folder,
          status: "ok",
          nested: false,
          size_bytes: 1048576 + (folder.length % 9) * 262144,
          fixable: false,
          health: 100,
          tracked: true,
          drifted: false,
          pinned: false,
          ignored: false,
          tracked_source: src,
          toc: {
            path: `${db.install?.addons_dir ?? "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns"}\\${folder}\\${folder}.toc`,
            name: folder,
            title: display,
            interface: 30300,
            raw_interface: "30300",
            version: found?.latest_version ?? "1.0",
            primary: true,
          },
          issues: [],
          compat: [compat(`${folder}.toc`, 30300)],
        });
        db.scannedAt = new Date().toISOString();
        return { installed: [folder], replaced: [], skipped: [], errors: [] };
      }
      return { installed: [], replaced: [folder], skipped: [], errors: [] };
    },

    async SaveWagoImport(id: string): Promise<WagoImportResult> {
      await delay(700);
      const entry = CATALOG_POOL.find((c) => c.id.toLowerCase() === id.toLowerCase());
      const name = entry?.name ?? "import";
      const path = `C:\\Users\\mock\\Downloads\\WagoImports\\${name}.txt`;
      return {
        path,
        name,
        bytes: 2100,
        applied_hint: `Saved to ${path}. Import it in-game via WeakAuras → Import`,
      };
    },

    async RestoreAddon(folder: string, allowReplace: boolean): Promise<InstallSourceResult> {
      await delay(900);
      const a = db.addons.find((x) => x.folder_name === folder);
      if (!a || !a.tracked) {
        return { installed: [], replaced: [], skipped: [], errors: ["addon not tracked in registry"] };
      }
      if (!allowReplace) {
        return { installed: [], replaced: [], skipped: [folder], errors: [] };
      }
      // A restore re-downloads the pristine folder: the drift badge
      // clears and the next scan shows the addon tracked and clean.
      a.drifted = false;
      db.scannedAt = new Date().toISOString();
      return { installed: [folder], replaced: [], skipped: [], errors: [] };
    },

    async Collections(): Promise<CollectionsResult> {
      await delay(300);
      return {
        collections: db.collections.map((c) => ({
          id: c.id,
          name: c.name,
          addon_count: c.addons.length,
          active: c.id === db.activeCollectionId,
        })),
        active_id: db.activeCollectionId,
      };
    },

    async CreateCollection(name: string): Promise<CollectionInfo> {
      await delay(400);
      const trimmed = name.trim();
      if (!trimmed) throw new Error("Collection name is required");
      const id = uniqueId(slugify(trimmed), db.collections.map((c) => c.id));
      db.collections.push({ id, name: trimmed, addons: [] });
      return { id, name: trimmed, addon_count: 0, active: false };
    },

    async SwitchCollection(id: string): Promise<SwitchCollectionResult> {
      await delay(450);
      const c = db.collections.find((x) => x.id === id);
      if (!c) throw new Error(`Collection "${id}" not found`);
      db.activeCollectionId = id;
      const applied = c.addons.filter((a) => a.enabled).map((a) => a.folder);
      return {
        applied,
        message: `Switched to “${c.name}”. ${applied.length} addon folder${applied.length === 1 ? "" : "s"} renamed to match (backup snapshot taken first)`,
      };
    },

    async DeleteCollection(id: string): Promise<void> {
      await delay(400);
      const idx = db.collections.findIndex((x) => x.id === id);
      if (idx < 0) throw new Error(`Collection "${id}" not found`);
      db.collections.splice(idx, 1);
      if (db.activeCollectionId === id) db.activeCollectionId = "";
    },

    async CollectionDetail(id: string): Promise<CollectionDetail> {
      await delay(300);
      const c = db.collections.find((x) => x.id === id);
      if (!c) throw new Error(`Collection "${id}" not found`);
      return {
        id: c.id,
        name: c.name,
        addons: c.addons.map((a) => ({ ...a })),
      };
    },

    async SetCollectionAddon(id: string, folder: string, enabled: boolean): Promise<void> {
      await delay(250);
      const c = db.collections.find((x) => x.id === id);
      if (!c) throw new Error(`Collection "${id}" not found`);
      const a = c.addons.find((x) => x.folder === folder);
      if (!a) throw new Error(`Addon "${folder}" is not in collection "${c.name}"`);
      a.enabled = enabled;
    },

    async InstallsStatus(): Promise<InstallsStatusResult> {
      await delay(350);
      return { installs: INSTALLS_STATUS.map((i) => ({ ...i })) };
    },

    async SyncUpdatesToAll(allowReplace: boolean): Promise<SyncResult> {
      await delay(1200);
      return {
        installs: [
          {
            root: "C:\\Games\\ChromieCraft",
            updated: 2,
            failed: 0,
            errors: [],
          },
        ],
        total_updated: 2,
        total_failed: 0,
      };
    },

    async AddonInfo(arg: string): Promise<InfoResult> {
      await delay(500);
      const q = arg.trim();
      const ql = q.toLowerCase();
      const exact = CATALOG_POOL.filter(
        (c) =>
          c.id.toLowerCase() === ql ||
          c.homepage.toLowerCase() === ql ||
          c.name.toLowerCase() === ql,
      );
      if (exact.length === 1) {
        const c = exact[0];
        return {
          provider: c.provider,
          id: c.id,
          name: c.name,
          author: c.author,
          summary: c.summary,
          latest_version: c.latest_version,
          homepage: c.homepage,
          game_version: c.game_version,
          updated_at: "2026-07-28T12-00-00.000Z",
          release_notes: c.game_version
            ? `Updated for ${c.game_version}.`
            : "Updated release.",
        };
      }
      // Ambiguous (or unknown): return candidates so the caller re-runs with
      // the chosen provider-scoped id.
      const fuzzy = CATALOG_POOL.filter((c) =>
        [c.name, c.id, c.author, c.summary].join(" ").toLowerCase().includes(ql),
      );
      const matches = (exact.length > 1 ? exact : fuzzy.slice(0, 5)).map((c) => ({
        provider: c.provider,
        id: c.id,
        name: c.name,
        summary: c.summary,
        homepage: c.homepage,
      }));
      return { ...emptyInfo(q), matches };
    },

    async Sources(): Promise<ProviderInfo[]> {
      await delay(200);
      return [
        { name: "github", description: "GitHub releases: owner/repo or repository URL" },
        { name: "curseforge", description: "CurseForge addon page: addon slug or page URL" },
        { name: "wowinterface", description: "WoWInterface download page: file info URL" },
        { name: "tukui", description: "Tukui addon page: addon id or page URL" },
        { name: "wago", description: "Wago.io: WeakAuras / Plater import strings" },
      ];
    },

    async Doctor(): Promise<DoctorReport> {
      await delay(600);
      const problems = db.addons.filter((a) => a.issues.length > 0).length;
      const pending = db.tracked.filter(
        (u) => u.current_version !== u.latest_version,
      ).length;
      const checks: DoctorReport["checks"] = [
        {
          name: "install",
          status: db.install ? "ok" : "error",
          message: db.install
            ? `Resolved ${db.install.flavor || "root"} install at ${db.install.root}`
            : "No WoW installation configured",
        },
        {
          name: "addons",
          status: problems === 0 ? "ok" : problems < 3 ? "warn" : "error",
          message: `${db.addons.length} addon${db.addons.length === 1 ? "" : "s"}, ${problems} with issues`,
        },
        {
          name: "updates",
          status: pending === 0 ? "ok" : "warn",
          message:
            pending === 0
              ? "All tracked addons are up to date"
              : `${pending} tracked addon${pending === 1 ? "" : "s"} have pending updates`,
        },
        {
          name: "saved-variables",
          status: "info",
          message: `${SAVED_VAR_ACCOUNTS.length} account${SAVED_VAR_ACCOUNTS.length === 1 ? "" : "s"} under WTF\\Account`,
        },
        {
          name: "backups",
          status: "ok",
          message: `${db.backups.length} snapshot${db.backups.length === 1 ? "" : "s"} in ${SETTINGS.backups_dir}`,
        },
      ];
      return { checks };
    },

    async SavedVarsAccounts(): Promise<string[]> {
      await delay(250);
      return [...SAVED_VAR_ACCOUNTS];
    },

    async SavedVarsList(account: string): Promise<SavedVarsListResult> {
      await delay(450);
      if (!SAVED_VAR_ACCOUNTS.includes(account)) {
        throw new Error(`account "${account}" not found under WTF\\Account`);
      }
      return {
        wtf_root: "C:\\Games\\World of Warcraft Classic\\WTF",
        account: `WTF\\Account\\${account}`,
        files: [...SAVED_VAR_FILES],
      };
    },

    async SavedVarsBackup(account: string): Promise<SavedVarsBackupResult> {
      await delay(700);
      if (!SAVED_VAR_ACCOUNTS.includes(account)) {
        throw new Error(`account "${account}" not found under WTF\\Account`);
      }
      return {
        path: `${SETTINGS.backups_dir}\\SavedVariables\\${account}-${new Date()
          .toISOString()
          .replace(/[:.]/g, "-")}.zip`,
        account,
      };
    },

    async SavedVarsRestore(account: string, backupPath: string): Promise<void> {
      await delay(700);
      if (!SAVED_VAR_ACCOUNTS.includes(account)) {
        throw new Error(`account "${account}" not found under WTF\\Account`);
      }
      if (!backupPath.trim()) throw new Error("backup path is required");
    },

    async SavedVarsReset(account: string, addon: string): Promise<void> {
      await delay(500);
      if (!SAVED_VAR_ACCOUNTS.includes(account)) {
        throw new Error(`account "${account}" not found under WTF\\Account`);
      }
      if (!addon.trim()) throw new Error("addon name is required");
    },

    async SavedVarsMigrate(
      fromAccount: string,
      toAccount: string,
      addon: string,
    ): Promise<SavedVarsMigrateResult> {
      await delay(800);
      if (fromAccount === toAccount) {
        throw new Error("source and target account must differ");
      }
      const file = addon.trim() || "CharacterSettings";
      return { copied: [`${file}.lua`] };
    },

    async BackupNow(): Promise<BackupResult> {
      await delay(900);
      const id = new Date().toISOString();
      db.backups.unshift({
        id,
        created_at: id,
        reason: "manual",
        folders: db.addons.map((a) => a.folder_name),
      });
      return { id };
    },

    async ListBackups(): Promise<ListBackupsResult> {
      await delay(350);
      return {
        snapshots: db.backups.map((b) => ({ ...b, folders: [...b.folders] })),
      };
    },

    async RestoreBackup(
      id: string,
      allowReplace: boolean,
    ): Promise<RestoreBackupResult> {
      await delay(1000);
      const snap = db.backups.find((b) => b.id === id);
      if (!snap) throw new Error(`backup snapshot "${id}" not found`);
      return allowReplace
        ? { restored: [...snap.folders], skipped: [] }
        : { restored: [], skipped: [...snap.folders] };
    },

    async ExportCollection(
      outPath: string,
      collectionID: string,
      includeSavedVars: boolean,
    ): Promise<ExportResult> {
      await delay(800);
      const addons = collectionID
        ? (db.collections.find((c) => c.id === collectionID)?.addons.map((a) => a.folder) ?? [])
        : db.addons.map((a) => a.folder_name);
      return {
        out:
          outPath.trim() ||
          `C:\\Users\\mock\\Downloads\\wowfix-export${collectionID ? `-${collectionID}` : ""}.zip`,
        addons: addons.length,
        collection: collectionID,
      };
    },

    async ImportCollection(pathOrURL: string): Promise<ImportResult> {
      await delay(900);
      const src = pathOrURL.trim();
      if (!src) return { installed: [] };
      const display = displayNameFromSource(src);
      const folder = folderFor(display);
      const exists = db.addons.some(
        (a) => a.folder_name.toLowerCase() === folder.toLowerCase(),
      );
      if (exists) return { installed: [] };
      db.addons.push({
        folder_name: folder,
        base_name: folder,
        suggested_name: folder,
        status: "ok",
        nested: false,
        size_bytes: 1572864,
        fixable: false,
        health: 100,
        tracked: true,
        drifted: false,
        pinned: false,
        ignored: false,
        tracked_source: src,
        toc: {
          path: `${db.install?.addons_dir ?? "C:\\Games\\World of Warcraft Classic\\Interface\\AddOns"}\\${folder}\\${folder}.toc`,
          name: folder,
          title: display,
          interface: 30300,
          raw_interface: "30300",
          version: "1.0",
          primary: true,
        },
        issues: [],
        compat: [compat(`${folder}.toc`, 30300)],
      });
      db.scannedAt = new Date().toISOString();
      return { installed: [folder] };
    },

    async Config(): Promise<ConfigView> {
      await delay(250);
      return {
        wow_path: db.install?.root ?? "",
        flavor: db.install?.flavor ?? "",
        profile: db.install?.profile_id ?? "wrath",
        collection: db.activeCollectionId,
        theme: SETTINGS.theme,
        auto_backup: SETTINGS.auto_backup,
        confirmations: SETTINGS.confirmations,
        backups_dir: SETTINGS.backups_dir,
        curseforge_api_key: SETTINGS.curseforge_api_key,
        collections_dir: SETTINGS.collections_dir,
      };
    },

    async SetConfigKey(key: string, value: string): Promise<void> {
      await delay(200);
      switch (key) {
        case "theme":
        case "backups_dir":
        case "curseforge_api_key":
        case "collections_dir":
          SETTINGS[key] = value;
          break;
        case "auto_backup":
        case "confirmations":
          SETTINGS[key] = value === "true";
          break;
        default:
          throw new Error(`config key "${key}" is read-only in the app UI`);
      }
    },

    async ExportSnapshot(): Promise<SnapshotResult> {
      await delay(700);
      const snapshot = {
        version: 1,
        profile:
          PROFILES.find((p) => p.id === db.install?.profile_id)?.family ?? "wrath",
        exported_at: new Date().toISOString(),
        addons: db.tracked.map((u) => ({
          folder: u.folder,
          title: u.title,
          current_version: u.current_version,
          latest_version: u.latest_version,
          provider: u.provider,
          id: u.id,
          source: u.source,
        })),
      };
      return {
        snapshot_json: JSON.stringify(snapshot, null, 2),
        exported_at: snapshot.exported_at,
        addon_count: db.tracked.length,
        warnings: [],
      };
    },

    async CheckSnapshot(snapshotJSON: string): Promise<SnapshotCheck> {
      await delay(450);
      if (!snapshotJSON.trim()) throw new Error("snapshot JSON is empty");
      return {
        updates: db.tracked
          .filter((u) => u.current_version !== u.latest_version)
          .map((u) => ({ ...u })),
        errors: [],
      };
    },
  };
}

/** Placeholder InfoResult for unresolved or ambiguous lookups. */
function emptyInfo(q: string): InfoResult {
  return {
    provider: "",
    id: "",
    name: q,
    author: "",
    summary: "",
    latest_version: "",
    homepage: "",
    game_version: "",
    updated_at: "",
  };
}

// Derive a display name for sources the catalog does not know, e.g.
// "owner/repo", a GitHub URL, or a direct ZIP link.
function displayNameFromSource(src: string): string {
  const seg = src.split(/[\\/?#]/).filter(Boolean).pop() ?? src;
  return seg.replace(/\.zip$/i, "").replace(/[-_](main|master)$/i, "");
}

// Canonical folder name for a known catalog addon.
function folderFor(name: string): string {
  switch (name) {
    case "WeakAuras 2":
      return "WeakAuras";
    case "Plater Nameplates":
      return "Plater";
    case "Details!":
      return "Details";
    default:
      return name.replace(/[^A-Za-z0-9]/g, "");
  }
}

function slugify(s: string): string {
  const slug = s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || "collection";
}

// Make `base` unique against `existing` ids by appending -2, -3, …
function uniqueId(base: string, existing: string[]): string {
  if (!existing.includes(base)) return base;
  let n = 2;
  while (existing.includes(`${base}-${n}`)) n++;
  return `${base}-${n}`;
}

function applyFix(db: MockDB, folderName: string, allowDestructive: boolean): FixResult {
  const a = db.addons.find((x) => x.folder_name === folderName);
  const fixes: FixResult["fixes"] = [];
  if (a) {
    const r = applyFixToAddon(db, a, allowDestructive);
    if (r) fixes.push(r);
  }
  const fixed = fixes.filter((f) => f.ok).length;
  const failed = fixes.length - fixed;
  return { fixes, fixed, failed };
}

function applyFixToAddon(
  db: MockDB,
  a: Addon,
  allowDestructive: boolean,
): FixResult["fixes"][number] | null {
  const fixable = a.issues.find((i) => i.action);
  if (!fixable) return null;
  if (
    (fixable.action === "delete" || fixable.action === "merge") &&
    !allowDestructive
  ) {
    return {
      addon: a.folder_name,
      action: fixable.action,
      ok: false,
      message: "Destructive fix requires confirmation",
      error: "destructive fix rejected (allowDestructive=false)",
    };
  }
  const action = fixable.action;
  switch (action) {
    case "rename":
      a.folder_name = fixable.suggested_name || a.folder_name;
      a.nested = false;
      break;
    case "flatten":
      a.folder_name = fixable.suggested_name || a.base_name || a.folder_name;
      a.nested = false;
      break;
    case "resolve-toc":
      if (fixable.options.length > 0 && a.toc) {
        a.toc.name = fixable.options[0];
        a.toc.primary = true;
      }
      break;
    case "delete":
      db.addons = db.addons.filter((x) => x !== a);
      break;
    case "merge": {
      const targets = fixable.options.map((o) =>
        db.addons.find((x) => x.folder_name === o),
      );
      db.addons = db.addons.filter((x) => !targets.includes(x));
      break;
    }
    case "repair-structure":
      a.nested = false;
      break;
    default:
      return {
        addon: a.folder_name,
        action,
        ok: false,
        message: `Unsupported action "${action}"`,
        error: "unknown action",
      };
  }
  a.issues = [];
  a.status = "ok";
  a.fixable = false;
  a.suggested_name = a.folder_name;
  return {
    addon: a.folder_name,
    action,
    ok: true,
    message: `${ACTION_MESSAGES[action] ?? "Fixed"} "${a.folder_name}"`,
  };
}

const ACTION_MESSAGES: Record<string, string> = {
  rename: "Renamed folder",
  flatten: "Flattened folder",
  "resolve-toc": "Set defining TOC for",
  delete: "Moved to trash:",
  merge: "Merged duplicates into",
  "repair-structure": "Repaired structure of",
};

export function installMockIfNeeded(): boolean {
  const g = window as unknown as {
    go?: { service?: { Service?: unknown } };
  };
  if (!mockEnabled() && g.go?.service?.Service) return false;
  // No mock flag but also no backend binding (plain browser, e.g. the mock
  // screenshot workflow): install the mock so the UI is fully renderable.
  g.go = { service: { Service: createMockService() } };
  return true;
}
