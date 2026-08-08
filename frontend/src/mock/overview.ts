import type { MockData } from "./index";
import type { Addon, FixEntry, Issue } from "../types";

// Overview fixtures: a mixed addon folder (healthy + every repair problem:
// missing TOC, nested folder, GitHub download name, TOC mismatch,
// duplicate), a doctor report with a few warnings, and a validate table
// with mixed verdicts. Fixes mutate module state so the next Scan reflects
// them — and the honest-failure demo stays alive: Details' missing TOC can
// never be repaired, which exercises row-level failure surfacing.

const SCAN_DIR =
  "C:\\Program Files (x86)\\World of Warcraft\\_retail_\\Interface\\AddOns";

/** Folders whose issues were repaired by a successful Fix/FixAll. */
const fixed = new Set<string>();

const issue = (
  kind: string,
  severity: Issue["severity"],
  message: string,
  suggestion: string,
  action: Issue["action"] = "",
  action_label = "",
  suggested_name = "",
): Issue => ({
  kind,
  severity,
  message,
  suggestion,
  action,
  action_label,
  options: [],
  suggested_name,
});

function makeAddon(a: Partial<Addon> & { folder_name: string }): Addon {
  return {
    base_name: a.folder_name,
    suggested_name: a.folder_name,
    status: "ok",
    nested: false,
    size_bytes: 1024 * 1024,
    fixable: false,
    health: 100,
    issues: [],
    compat: [],
    tracked: false,
    drifted: false,
    pinned: false,
    ignored: false,
    ...a,
  };
}

function baseAddons(): Addon[] {
  return [
    makeAddon({
      folder_name: "Questie",
      status: "ok",
      size_bytes: 24 * 1024 * 1024,
      health: 100,
      tracked: true,
      tracked_source: "curseforge:questie",
      toc: { name: "Questie", title: "Questie", interface: 110200, version: "10.1.5" },
    }),
    makeAddon({
      folder_name: "DeadlyBossMods",
      base_name: "DBM-Core",
      suggested_name: "DBM-Core",
      status: "warn",
      nested: true,
      size_bytes: 9 * 1024 * 1024,
      fixable: true,
      health: 62,
      tracked: true,
      tracked_source: "curseforge:deadly-boss-mods",
      toc: { name: "DBM-Core", title: "Deadly Boss Mods", interface: 110200, version: "11.0.1" },
      issues: [
        issue(
          "nested",
          "warn",
          "Installed inside DeadlyBossMods/ — belongs at the AddOns root.",
          "Flattening moves it to Interface/AddOns/DBM-Core.",
          "flatten",
          "Flatten",
          "DBM-Core",
        ),
      ],
    }),
    makeAddon({
      folder_name: "WeakAuras2-master",
      base_name: "WeakAuras2",
      suggested_name: "WeakAuras2",
      status: "warn",
      size_bytes: 18 * 1024 * 1024,
      fixable: true,
      health: 68,
      tracked: true,
      drifted: true,
      tracked_source: "github:WeakAuras/WeakAuras2",
      toc: { name: "WeakAuras", title: "WeakAuras", interface: 110200, version: "5.15.0" },
      issues: [
        issue(
          "github-name",
          "warn",
          "Folder name is the GitHub download name, not the addon name.",
          "Rename the folder to WeakAuras2.",
          "rename",
          "Rename",
          "WeakAuras2",
        ),
      ],
    }),
    makeAddon({
      folder_name: "Details",
      status: "error",
      size_bytes: 21 * 1024 * 1024,
      fixable: true,
      health: 24,
      tracked: true,
      tracked_source: "curseforge:details",
      issues: [
        issue(
          "missing-toc",
          "error",
          "No .toc file found — WoW will not load this addon.",
          "Recreate the TOC from the addon manifest.",
          "resolve-toc",
          "Resolve TOC",
        ),
      ],
    }),
    makeAddon({
      folder_name: "BigWigs",
      status: "ok",
      size_bytes: 6 * 1024 * 1024,
      health: 100,
      tracked: true,
      tracked_source: "github:BigWigsMods/BigWigs",
      toc: { name: "BigWigs", title: "BigWigs", interface: 110200, version: "11.0.2" },
    }),
    makeAddon({
      folder_name: "Plater",
      status: "warn",
      size_bytes: 12 * 1024 * 1024,
      fixable: false,
      health: 74,
      tracked: true,
      drifted: true,
      tracked_source: "curseforge:plater",
      toc: { name: "Plater", title: "Plater Nameplates", interface: 100200, version: "2.0.3" },
      issues: [
        issue(
          "toc-mismatch",
          "warn",
          "TOC declares interface 100200 but the profile expects 110200.",
          "Update the addon to a version built for this client.",
        ),
      ],
    }),
    makeAddon({
      folder_name: "ElvUI",
      status: "error",
      size_bytes: 15 * 1024 * 1024,
      fixable: true,
      health: 30,
      tracked: true,
      tracked_source: "tukui:ElvUI",
      toc: { name: "ElvUI", title: "ElvUI", interface: 110200, version: "13.85" },
      issues: [
        issue(
          "duplicate",
          "error",
          "Two copies of ElvUI are installed (ElvUI and ElvUI_old).",
          "Remove the stale copy after confirming which one loads.",
          "merge",
          "Remove duplicate",
        ),
      ],
    }),
    makeAddon({
      folder_name: "AtlasLoot",
      status: "ok",
      size_bytes: 10 * 1024 * 1024,
      health: 100,
      tracked: true,
      tracked_source: "curseforge:atlasloot",
      toc: { name: "AtlasLoot", title: "AtlasLoot", interface: 110202, version: "8.1.0" },
    }),
    makeAddon({
      folder_name: "Recount",
      status: "warn",
      size_bytes: 4 * 1024 * 1024,
      fixable: false,
      health: 55,
      toc: { name: "Recount", title: "Recount", interface: 11403, version: "5.0.4" },
      issues: [
        issue(
          "legacy-interface",
          "warn",
          "Declares a Vanilla-era interface (11403) — likely abandoned.",
          "Consider a maintained replacement such as Details or Skada.",
        ),
      ],
    }),
  ];
}

/** A repaired addon scans clean: renamed (if suggested), no issues left. */
function applyFixes(a: Addon): Addon {
  if (!fixed.has(a.folder_name) && !fixed.has(a.suggested_name)) return a;
  const name = a.suggested_name || a.folder_name;
  return { ...a, folder_name: name, base_name: name, status: "ok", nested: false, fixable: false, health: 100, issues: [] };
}

const scanAddons = (): Addon[] => baseAddons().map(applyFixes);

function failureEntry(addon: string): FixEntry {
  return {
    addon,
    action: "Resolve TOC",
    ok: false,
    message: "No .toc file found in Details — nothing to repair",
    error: "toc-not-found",
  };
}

export const data: MockData = {
  GetState: () => {
    const requested = new URLSearchParams(window.location.search).get("view");
    const firstRun = requested === "setup";
    return {
      version: "0.0.0-mock",
      wow_path: firstRun ? "" : "C:\\Program Files (x86)\\World of Warcraft\\_retail_",
      flavor: firstRun ? "" : "_retail_",
      addons_dir: firstRun ? "" : SCAN_DIR,
      profile_id: firstRun ? "" : "retail",
      profile_name: firstRun ? "" : "Retail",
      has_install: !firstRun,
      auto_backup: true,
      confirmations: true,
    };
  },
  Scan: () => {
    const addons = scanAddons();
    return {
      addons_dir: SCAN_DIR,
      profile_id: "retail",
      scanned_at: new Date().toISOString(),
      addons,
      errors: [],
      stats: {
        total: addons.length,
        problems: addons.filter((a) => a.status !== "ok").length,
        errors: addons.filter((a) => a.status === "error").length,
      },
    };
  },
  Fix: (...args: unknown[]) => {
    const [folder] = args as [string, boolean];
    if (folder === "Details") {
      return { fixes: [failureEntry("Details")], fixed: 0, failed: 1 };
    }
    const a = baseAddons().find(
      (x) => x.folder_name === folder || x.suggested_name === folder,
    );
    fixed.add(folder);
    const entry: FixEntry = {
      addon: folder,
      action: a?.issues[0]?.action_label ?? "Repair",
      ok: true,
      message: `${folder} repaired`,
    };
    return { fixes: [entry], fixed: 1, failed: 0 };
  },
  FixAll: () => {
    const candidates = baseAddons().filter(
      (a) => a.fixable && !fixed.has(a.folder_name) && !fixed.has(a.suggested_name),
    );
    const fixes: FixEntry[] = candidates.map((a) => {
      if (a.folder_name === "Details") return failureEntry("Details");
      fixed.add(a.folder_name);
      return {
        addon: a.folder_name,
        action: a.issues[0]?.action_label ?? "Repair",
        ok: true,
        message: `${a.folder_name} repaired`,
      };
    });
    const fixedCount = fixes.filter((f) => f.ok).length;
    return { fixes, fixed: fixedCount, failed: fixes.length - fixedCount };
  },
  Doctor: () => ({
    checks: [
      {
        name: "install",
        status: "ok",
        message: "World of Warcraft found at C:\\Program Files (x86)\\World of Warcraft\\_retail_",
      },
      {
        name: "addons-path",
        status: "ok",
        message: "Interface/AddOns exists and is readable",
      },
      {
        name: "profile",
        status: "ok",
        message: "Retail profile configured (interface 110200)",
      },
      {
        name: "toc-consistency",
        status: "warn",
        message: "3 addons declare an interface older than the profile",
      },
      {
        name: "updates",
        status: "warn",
        message: "5 tracked addons have newer versions available",
      },
      {
        name: "backups",
        status: "ok",
        message: "Last snapshot 2026-08-07 · 9 folders · 41.2 MB",
      },
      {
        name: "saved-variables",
        status: "error",
        message: "Account folder is unreadable: _retail_/Account is missing",
      },
      {
        name: "network",
        status: "info",
        message: "Providers reachable — last check 12 min ago",
      },
    ],
  }),
  Validate: () => ({
    profile_id: "retail",
    expected: 110200,
    addons: [
      { folder_name: "Questie", toc: "Questie.toc", expected: 110200, detected: 110200, status: "compatible", label: "Compatible" },
      { folder_name: "DeadlyBossMods", toc: "DBM-Core.toc", expected: 110200, detected: 110200, status: "compatible", label: "Compatible" },
      { folder_name: "WeakAuras2", toc: "WeakAuras.toc", expected: 110200, detected: 110200, status: "compatible", label: "Compatible" },
      { folder_name: "Details", toc: "Details.toc", expected: 110200, detected: -1, status: "unknown", label: "No TOC found" },
      { folder_name: "BigWigs", toc: "BigWigs.toc", expected: 110200, detected: 110200, status: "compatible", label: "Compatible" },
      { folder_name: "Plater", toc: "Plater.toc", expected: 110200, detected: 100200, status: "mismatch", label: "Outdated" },
      { folder_name: "ElvUI", toc: "ElvUI.toc", expected: 110200, detected: 110200, status: "compatible", label: "Compatible" },
      { folder_name: "AtlasLoot", toc: "AtlasLoot.toc", expected: 110200, detected: 110202, status: "compatible", label: "Compatible" },
      { folder_name: "Recount", toc: "Recount.toc", expected: 110200, detected: 11403, status: "vanilla", label: "Vanilla-era" },
    ],
  }),
};
