// Mock harness. Activated by `?mock=1` (or window.__WOWFIX_MOCK__ === true,
// set by an embedding harness). When active, a Proxy over
// window.go.service.Service answers every method with a Promise: function
// entries are invoked with the caller's args, plain values are returned
// wrapped. Per-view data files (mock/<view>.ts) export `data: MockData`
// overrides merged on top of the shape defaults below.

import type { Service } from "../types";
import { data as setupData } from "./setup";
import { data as overviewData } from "./overview";
import { data as catalogData } from "./catalog";
import { data as updatesData } from "./updates";
import { data as collectionsData } from "./collections";
import { data as backupsData } from "./backups";
import { data as savedvarsData } from "./savedvars";
import { data as settingsData } from "./settings";

export function isMock(): boolean {
  return (
    (window as unknown as { __WOWFIX_MOCK__?: boolean }).__WOWFIX_MOCK__ ===
    true
  );
}

/**
 * Per-method mock override. A function receives the caller's arguments and
 * returns the mock value; a plain value is returned as-is. `never[]` keeps
 * override signatures loose (zero-arg or rest-arg functions assign cleanly).
 */
export type MockData = {
  [K in keyof Service]?: ((...args: never[]) => unknown) | unknown;
};

/** Sensible empty shapes for every method without a view override. */
const defaults: MockData = {
  GetState: () => ({
    version: "0.0.0-mock",
    wow_path: "",
    flavor: "",
    addons_dir: "",
    profile_id: "",
    profile_name: "",
    has_install: true,
    auto_backup: true,
    confirmations: true,
  }),
  DetectInstalls: [],
  SetInstall: () => ({
    root: "",
    flavor: "",
    addons_path: "",
    exe: "",
    version: "",
    profile_id: "",
    confidence: "",
  }),
  SetProfile: () => {},
  Profiles: [],
  Scan: () => ({
    addons_dir: "",
    profile_id: "",
    scanned_at: "",
    addons: [],
    errors: [],
    stats: { total: 0, problems: 0, errors: 0 },
  }),
  Validate: () => ({ profile_id: "", expected: 0, addons: [] }),
  Fix: () => ({ fixes: [], fixed: 0, failed: 0 }),
  FixAll: () => ({ fixes: [], fixed: 0, failed: 0 }),
  InstallZip: () => ({ installed: [], replaced: [], skipped: [], errors: [] }),
  CheckUpdates: () => ({ updates: [], errors: [], checked_at: "" }),
  ApplyUpdate: () => ({ applied: [], applied_count: 0, failed_count: 0 }),
  ApplyAllUpdates: () => ({ applied: [], applied_count: 0, failed_count: 0 }),
  TrackedAddons: () => ({ addons: [] }),
  SetAddonPinned: () => {},
  SetAddonIgnored: () => {},
  RollbackAddon: () => ({
    folder: "",
    restored_from: "",
    version: "",
    pinned: false,
    message: "",
  }),
  ListAddonVersions: () => ({ folder: "", current: "", versions: [] }),
  RollbackToVersion: () => ({ installed: [], replaced: [], skipped: [], errors: [] }),
  SearchCatalog: () => ({ results: [], errors: [] }),
  Curated: () => ({ family: "", label: "", profile_id: "", addons: [] }),
  InstallSource: () => ({ installed: [], replaced: [], skipped: [], errors: [] }),
  SaveWagoImport: () => ({ path: "", name: "", bytes: 0, applied_hint: "" }),
  RestoreAddon: () => ({ installed: [], replaced: [], skipped: [], errors: [] }),
  Collections: () => ({ collections: [], active_id: "" }),
  CreateCollection: () => ({ id: "", name: "", addon_count: 0, active: false }),
  SwitchCollection: () => ({ applied: [], message: "" }),
  DeleteCollection: () => {},
  CollectionDetail: () => ({ id: "", name: "", addons: [] }),
  SetCollectionAddon: () => {},
  InstallsStatus: () => ({ installs: [] }),
  SyncUpdatesToAll: () => ({ installs: [], total_updated: 0, total_failed: 0 }),
  AddonInfo: () => ({
    provider: "",
    id: "",
    name: "",
    author: "",
    summary: "",
    latest_version: "",
    homepage: "",
    game_version: "",
    updated_at: "",
  }),
  Sources: [],
  Doctor: () => ({ checks: [] }),
  SavedVarsAccounts: [],
  SavedVarsList: () => ({ wtf_root: "", account: "", files: [] }),
  SavedVarsBackup: () => ({ path: "", account: "" }),
  SavedVarsRestore: () => {},
  SavedVarsReset: () => {},
  SavedVarsMigrate: () => ({ copied: [] }),
  BackupNow: () => ({ id: "" }),
  ListBackups: () => ({ snapshots: [] }),
  RestoreBackup: () => ({ restored: [], skipped: [] }),
  ExportCollection: () => ({ out: "", addons: 0, collection: "" }),
  ImportCollection: () => ({ installed: [] }),
  Config: () => ({
    wow_path: "",
    flavor: "",
    profile: "",
    collection: "",
    theme: "dark",
    auto_backup: true,
    confirmations: true,
    backups_dir: "",
    curseforge_api_key: "",
    collections_dir: "",
  }),
  SetConfigKey: () => {},
  ExportSnapshot: () => ({ snapshot_json: "", exported_at: "", addon_count: 0, warnings: [] }),
  CheckSnapshot: () => ({ updates: [], errors: [] }),
};

const data: MockData = {
  ...defaults,
  ...setupData,
  ...overviewData,
  ...catalogData,
  ...updatesData,
  ...collectionsData,
  ...backupsData,
  ...savedvarsData,
  ...settingsData,
};

/**
 * Install the mock Service if mock mode is active. Returns true when the
 * mock was installed. Called once at module load from api.ts; every view
 * call then follows one code path.
 */
export function installMockIfNeeded(): boolean {
  if (!isMock()) return false;
  const g = window as unknown as {
    go?: { service?: { Service?: Record<string, unknown> } };
  };
  if (!g.go) g.go = {};
  if (!g.go.service) g.go.service = {};
  g.go.service.Service = new Proxy({} as Record<string, unknown>, {
    get(_target, prop: string | symbol): unknown {
      if (typeof prop !== "string") return undefined;
      const entry = data[prop as keyof Service];
      if (typeof entry === "function") {
        return (...args: unknown[]) => Promise.resolve((entry as (...a: unknown[]) => unknown)(...args));
      }
      return async () => entry;
    },
  });
  return true;
}
