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

import type {
  Service,
  State,
  Install,
  Profile,
  ScanResult,
  ValidateResult,
  FixResult,
  InstallResult,
  Addon,
  Issue,
  CompatEntry,
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

interface MockDB {
  install: MockInstallState | null;
  addons: Addon[];
  scanErrors: string[];
  scannedAt: string;
  lastInstall: InstallResult | null;
}

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
  return [
    {
      folder_name: "Inventory",
      base_name: "Inventory",
      suggested_name: "Inventory",
      status: "error",
      nested: false,
      size_bytes: 14520,
      fixable: true,
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
          message: "Multiple TOCs found — pick which one defines this addon",
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
    addons: [...db.addons],
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
  };
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
