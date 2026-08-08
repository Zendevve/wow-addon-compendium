// Mock fixtures for the Updates view — owned methods only (see the
// ownership table in mock/index.ts; Scan/GetState live in mock/overview.ts).
// A small mutable registry mirrors the real semantics: applying an update
// bumps current → latest and records a history entry; pinned/ignored addons
// drop out of the pending list; Details! always fails so the row-level
// failure state and the end-of-batch error dump render; the snapshot
// export/check flow works end to end with no backend.

import type { MockData } from "./index";
import type { Provider, TrackedAddon, UpdateEntry, VersionEntry } from "../types";

interface TrackedLike {
  folder: string;
  title: string;
  version: string;
  latest_version?: string;
  provider: Provider;
  id: string;
  source: string;
  pinned: boolean;
  ignored: boolean;
  installed_at: string;
  /** Folder no longer matches the recorded install — Restore offered. */
  drifted?: boolean;
  flavor_mismatch?: boolean;
  flavor_label?: string;
}

const TRACKED: TrackedLike[] = [
  {
    folder: "Questie",
    title: "Questie",
    version: "1.12.2",
    latest_version: "1.13.0",
    provider: "github",
    id: "Vendethiel/Questie",
    source: "https://github.com/Vendethiel/Questie",
    pinned: false,
    ignored: false,
    installed_at: "2026-08-01T09:15:00.000Z",
  },
  {
    folder: "DeadlyBossMods",
    title: "Deadly Boss Mods",
    version: "9.5.2",
    latest_version: "9.5.9",
    provider: "curseforge",
    id: "deadly-boss-mods",
    source: "https://www.curseforge.com/wow/addons/deadly-boss-mods",
    pinned: false,
    ignored: false,
    installed_at: "2026-08-02T14:40:00.000Z",
    flavor_mismatch: true,
    flavor_label: "retail addon · profile wrath",
  },
  {
    folder: "WeakAuras2-master",
    title: "WeakAuras 2",
    version: "5.12.0",
    latest_version: "5.12.5",
    provider: "github",
    id: "WeakAuras/WeakAuras2",
    source: "https://github.com/WeakAuras/WeakAuras2",
    pinned: false,
    ignored: false,
    installed_at: "2026-08-01T11:20:00.000Z",
  },
  {
    folder: "Details",
    title: "Details! Damage Meter",
    version: "3.10.2",
    latest_version: "3.10.6",
    provider: "curseforge",
    id: "details",
    source: "https://www.curseforge.com/wow/addons/details",
    pinned: false,
    ignored: false,
    installed_at: "2026-07-28T18:05:00.000Z",
  },
  {
    folder: "Plater",
    title: "Plater Nameplates",
    version: "1.11.0",
    latest_version: "1.11.4",
    provider: "wowinterface",
    id: "plater-nameplates",
    source: "https://www.wowinterface.com/downloads/info24911-PlaterNameplates.html",
    pinned: false,
    ignored: false,
    installed_at: "2026-08-03T08:30:00.000Z",
    drifted: true,
  },
  {
    folder: "AtlasLoot",
    title: "AtlasLoot",
    version: "7.0.4",
    provider: "curseforge",
    id: "atlasloot",
    source: "https://www.curseforge.com/wow/addons/atlasloot",
    pinned: true,
    ignored: false,
    installed_at: "2026-07-20T10:00:00.000Z",
  },
  {
    folder: "BigWigs",
    title: "BigWigs",
    version: "10.2.2",
    provider: "wowinterface",
    id: "bigwigs",
    source: "https://www.wowinterface.com/downloads/info3656-BigWigs.html",
    pinned: false,
    ignored: true,
    installed_at: "2026-07-25T16:45:00.000Z",
  },
  {
    folder: "ElvUI",
    title: "ElvUI",
    version: "13.85",
    provider: "tukui",
    id: "elvui",
    source: "https://www.tukui.org/addons.php?id=elvui",
    pinned: false,
    ignored: false,
    installed_at: "2026-07-30T09:10:00.000Z",
  },
  {
    folder: "Bartender4",
    title: "Bartender4",
    version: "4.12.2",
    provider: "curseforge",
    id: "bartender4",
    source: "https://www.curseforge.com/wow/addons/bartender4",
    pinned: false,
    ignored: false,
    installed_at: "2026-08-04T13:25:00.000Z",
  },
  {
    folder: "pfQuest",
    title: "pfQuest",
    version: "1.9.0",
    provider: "github",
    id: "shagu/pfQuest",
    source: "https://github.com/shagu/pfQuest",
    pinned: false,
    ignored: false,
    installed_at: "2026-07-22T19:50:00.000Z",
  },
];

const HISTORY: Record<string, VersionEntry[]> = {
  Questie: [
    { version: "1.12.2", provider: "github", source: "https://github.com/Vendethiel/Questie", ref: "v1.12.2", at: "2026-08-06T14:20:00.000Z" },
    { version: "1.12.0", provider: "github", source: "https://github.com/Vendethiel/Questie", ref: "v1.12.0", at: "2026-07-30T09:05:00.000Z" },
    { version: "1.11.4", provider: "github", source: "https://github.com/Vendethiel/Questie", ref: "v1.11.4", at: "2026-07-12T17:40:00.000Z" },
  ],
  DeadlyBossMods: [
    { version: "9.5.2", provider: "curseforge", source: "https://www.curseforge.com/wow/addons/deadly-boss-mods", ref: "29847123", at: "2026-08-05T11:00:00.000Z" },
    { version: "9.5.0", provider: "curseforge", source: "https://www.curseforge.com/wow/addons/deadly-boss-mods", ref: "29710001", at: "2026-07-28T08:30:00.000Z" },
  ],
  "WeakAuras2-master": [
    { version: "5.12.0", provider: "github", source: "https://github.com/WeakAuras/WeakAuras2", ref: "5.12.0", at: "2026-08-04T12:15:00.000Z" },
    { version: "5.11.9", provider: "github", source: "https://github.com/WeakAuras/WeakAuras2", ref: "5.11.9", at: "2026-07-15T10:00:00.000Z" },
  ],
};

/** Pinned and ignored addons are excluded from update checks. */
function pendingEntries(): UpdateEntry[] {
  return TRACKED.filter(
    (t) => t.latest_version && t.latest_version !== t.version && !t.pinned && !t.ignored,
  ).map((t) => ({
    folder: t.folder,
    title: t.title,
    current_version: t.version,
    latest_version: t.latest_version as string,
    provider: t.provider,
    id: t.id,
    source: t.source,
    flavor_mismatch: t.flavor_mismatch ?? false,
    flavor_label: t.flavor_label ?? "",
  }));
}

function recordVersion(t: TrackedLike, version: string): void {
  const prev = HISTORY[t.folder] ?? [];
  HISTORY[t.folder] = [
    { version, provider: t.provider, source: t.source, ref: version, at: new Date().toISOString() },
    ...prev,
  ];
}

function toTracked(t: TrackedLike): TrackedAddon {
  return {
    folder: t.folder,
    title: t.title,
    version: t.version,
    provider: t.provider,
    id: t.id,
    source: t.source,
    pinned: t.pinned,
    ignored: t.ignored,
    installed_at: t.installed_at,
    has_history: (HISTORY[t.folder] ?? []).length > 0,
  };
}

// Details! stands in for a provider-side failure so the honest row-level
// error and the end-of-batch error dump render in the mock.
const DETAILS_FAILURE =
  "curseforge: 404 — project file 30124710 no longer available";

export const data: MockData = {
  CheckUpdates: () => ({
    updates: pendingEntries(),
    errors: [],
    checked_at: new Date().toISOString(),
  }),

  ApplyUpdate: (folder: string, _allowReplace: boolean) => {
    const f = folder as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) {
      return {
        applied: [{ folder: f, ok: false, message: "Addon is not tracked", error: "unknown folder" }],
        applied_count: 0,
        failed_count: 1,
      };
    }
    if (f === "Details") {
      return {
        applied: [{ folder: f, ok: false, message: DETAILS_FAILURE, error: DETAILS_FAILURE }],
        applied_count: 0,
        failed_count: 1,
      };
    }
    recordVersion(t, t.latest_version ?? t.version);
    t.version = t.latest_version ?? t.version;
    t.flavor_mismatch = false;
    t.flavor_label = "";
    return {
      applied: [{ folder: f, ok: true, message: `Updated to ${t.version}`, error: "" }],
      applied_count: 1,
      failed_count: 0,
    };
  },

  ApplyAllUpdates: (_allowReplace: boolean) => {
    const applied: Array<{ folder: string; ok: boolean; message: string; error?: string }> = [];
    for (const t of TRACKED) {
      if (!t.latest_version || t.latest_version === t.version || t.pinned || t.ignored) continue;
      if (t.folder === "Details") {
        applied.push({ folder: t.folder, ok: false, message: DETAILS_FAILURE, error: DETAILS_FAILURE });
        continue;
      }
      recordVersion(t, t.latest_version);
      t.version = t.latest_version;
      t.flavor_mismatch = false;
      t.flavor_label = "";
      applied.push({ folder: t.folder, ok: true, message: `Updated to ${t.version}`, error: "" });
    }
    const failed = applied.filter((a) => !a.ok).length;
    return { applied, applied_count: applied.length - failed, failed_count: failed };
  },

  TrackedAddons: () => ({
    addons: TRACKED.map((t) => toTracked(t)),
  }),

  SetAddonPinned: (folder: string, pinned: boolean) => {
    const f = folder as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) throw new Error(`addon "${f}" not tracked in registry`);
    t.pinned = Boolean(pinned);
  },

  SetAddonIgnored: (folder: string, ignored: boolean) => {
    const f = folder as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) throw new Error(`addon "${f}" not tracked in registry`);
    t.ignored = Boolean(ignored);
  },

  RollbackAddon: (folder: string) => {
    const f = folder as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) throw new Error(`addon "${f}" not tracked in registry`);
    t.pinned = true;
    return {
      folder: t.folder,
      restored_from: "2026-08-07T10:00:00.000Z",
      version: t.version,
      pinned: true,
      message: "restored from snapshot 2026-08-07T10:00:00.000Z and pinned",
    };
  },

  ListAddonVersions: (folder: string) => {
    const f = folder as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) throw new Error(`addon "${f}" not tracked in registry`);
    return {
      folder: t.folder,
      current: t.version,
      versions: (HISTORY[f] ?? []).map((v) => ({ ...v })),
    };
  },

  RollbackToVersion: (folder: string, version: string) => {
    const f = folder as string;
    const v = version as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) throw new Error(`addon "${f}" not tracked in registry`);
    const entry = (HISTORY[f] ?? []).find((e) => e.version === v);
    if (!entry) throw new Error(`no recorded version "${v}" for "${f}"`);
    // Tukui/WoWInterface only serve the latest release: the honest error,
    // never a silent install of something else.
    if (t.provider === "tukui" || t.provider === "wowinterface") {
      throw new Error(`${t.provider} can no longer re-download version "${v}" — only the latest is available`);
    }
    recordVersion(t, v);
    t.version = v;
    t.latest_version = v;
    t.flavor_mismatch = false;
    t.flavor_label = "";
    return { installed: [f], replaced: [f], skipped: [], errors: [] };
  },

  RestoreAddon: (folder: string, _allowReplace: boolean) => {
    const f = folder as string;
    const t = TRACKED.find((x) => x.folder === f);
    if (!t) throw new Error(`addon "${f}" not tracked in registry`);
    t.drifted = false;
    return { installed: [f], replaced: [f], skipped: [], errors: [] };
  },

  ExportSnapshot: () => {
    const exportedAt = new Date().toISOString();
    const snapshot = {
      version: 1,
      profile: "wrath",
      exported_at: exportedAt,
      addons: TRACKED.map((t) => ({
        folder: t.folder,
        title: t.title,
        current_version: t.version,
        latest_version: t.latest_version ?? t.version,
        provider: t.provider,
        id: t.id,
        source: t.source,
      })),
    };
    return {
      snapshot_json: JSON.stringify(snapshot, null, 2),
      exported_at: exportedAt,
      addon_count: TRACKED.length,
      warnings: [],
    };
  },

  CheckSnapshot: (snapshotJSON: string) => {
    const json = snapshotJSON as string;
    let snap: { addons?: Array<{ folder?: string }> };
    try {
      snap = JSON.parse(json);
    } catch {
      throw new Error("invalid snapshot JSON");
    }
    if (!Array.isArray(snap.addons)) {
      throw new Error("invalid snapshot JSON: missing addons");
    }
    const folders = new Set(
      snap.addons.map((a) => a.folder).filter((x): x is string => Boolean(x)),
    );
    return {
      updates: pendingEntries().filter((u) => folders.has(u.folder)),
      errors: [],
    };
  },
};
