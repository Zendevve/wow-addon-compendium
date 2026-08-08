// Type definitions mirroring the frozen Wails binding contract.
// All shapes come from the Go `Service` struct; field names are the JSON
// names the backend emits (see the binding contract in the project brief).

export type AddonStatus = "ok" | "warn" | "error";
export type Severity = "info" | "warn" | "error";

export interface Install {
  root: string;
  flavor: string;
  addons_path?: string;
  addons_dir?: string;
  exe?: string;
  version?: string;
  profile_id?: string;
  confidence?: string;
}

export interface Profile {
  id: string;
  name: string;
  family: string;
  interface: number;
}

export interface State {
  version: string;
  wow_path: string;
  flavor: string;
  addons_dir: string;
  profile_id: string;
  profile_name: string;
  has_install: boolean;
  auto_backup: boolean;
  confirmations: boolean;
}

export interface TOC {
  path?: string;
  name: string;
  title: string;
  interface: number; // -1 when absent
  raw_interface?: string;
  version: string;
  author?: string;
  notes?: string;
  primary?: boolean;
}

export interface Issue {
  kind: string;
  severity: Severity;
  message: string;
  suggestion: string;
  action: string; // "" | "rename" | "flatten" | "resolve-toc" | "delete" | "merge" | "repair-structure"
  action_label: string;
  options: string[];
  suggested_name: string;
}

export interface CompatEntry {
  toc: string;
  expected: number;
  detected: number;
  status: "compatible" | "vanilla" | "retail" | "mismatch" | "unknown";
  label: string;
}

export interface Addon {
  folder_name: string;
  base_name: string;
  suggested_name: string;
  status: AddonStatus;
  nested: boolean;
  size_bytes: number;
  fixable: boolean;
  health: number;
  toc: TOC | null;
  issues: Issue[];
  compat: CompatEntry[];
  /** Installed through the catalog and recorded in the registry. */
  tracked: boolean;
  /** Tracked addon whose folder no longer matches the recorded manifest checksum. */
  drifted: boolean;
  /** The provider source (URL or owner/repo) the addon was installed from. */
  tracked_source?: string;
  /** Locked at the current version — skipped by update checks. */
  pinned: boolean;
  /** Excluded from update management entirely. */
  ignored: boolean;
}

export interface ScanStats {
  total: number;
  problems: number;
  errors: number;
}

export interface ScanResult {
  addons_dir: string;
  profile_id: string;
  scanned_at: string;
  addons: Addon[];
  errors: string[];
  stats: ScanStats;
}

export interface ValidateEntry {
  folder_name: string;
  toc: string;
  expected: number;
  detected: number;
  status: CompatEntry["status"];
  label: string;
}

export interface ValidateResult {
  profile_id: string;
  expected: number;
  addons: ValidateEntry[];
}

export interface FixEntry {
  addon: string;
  action: string;
  ok: boolean;
  message: string;
  error?: string;
}

export interface FixResult {
  fixes: FixEntry[];
  fixed: number;
  failed: number;
}

export interface InstallResult {
  installed: number;
  replaced: number;
  skipped: number;
  errors: string[];
}

/** Addon update sources (used for provider chips/badges). */
export type Provider = "github" | "curseforge" | "wowinterface" | "tukui" | "wago";

export interface UpdateEntry {
  folder: string;
  title: string;
  current_version: string;
  latest_version: string;
  provider: Provider;
  id: string;
  source: string;
  flavor_mismatch: boolean;
  flavor_label: string;
}

export interface CheckUpdatesResult {
  updates: UpdateEntry[];
  errors: string[];
  checked_at: string;
}

/** One addon recorded in the catalog registry (the Managed section). */
export interface TrackedAddon {
  folder: string;
  title: string;
  version: string;
  provider: Provider;
  id: string;
  source: string;
  pinned: boolean;
  ignored: boolean;
  installed_at: string;
  /** True when the addon has a recorded version log (drives the History menu). */
  has_history: boolean;
}

export interface TrackedResult {
  addons: TrackedAddon[];
}

/** One recorded version of a tracked addon, newest first. */
export interface VersionEntry {
  version: string;
  provider?: string;
  source?: string;
  /** Provider-scoped reference: GitHub tag or CurseForge file id. */
  ref?: string;
  at: string;
}

/** The recorded version log of one tracked addon. */
export interface VersionHistoryResult {
  folder: string;
  current: string;
  versions: VersionEntry[];
}

export interface RollbackResult {
  folder: string;
  restored_from: string;
  version: string;
  pinned: boolean;
  message: string;
}

export interface ApplyEntry {
  folder: string;
  ok: boolean;
  message: string;
  error: string;
}

export interface ApplyBatch {
  applied: ApplyEntry[];
  applied_count: number;
  failed_count: number;
}

export interface CatalogEntry {
  provider: Provider;
  name: string;
  author: string;
  summary: string;
  latest_version: string;
  game_version: string;
  id: string;
  homepage: string;
}

export interface SearchCatalogResult {
  results: CatalogEntry[];
  errors: string[];
}

export interface InstallSourceResult {
  installed: string[];
  replaced: string[];
  skipped: string[];
  errors: string[];
}

/** Outcome of saving a WeakAuras/Plater import string from Wago. */
export interface WagoImportResult {
  path: string;
  name: string;
  bytes: number;
  applied_hint: string;
}

// ---------- curated private-server sets ----------

/** One addon in a curated set (a verified private-server manifest entry). */
export interface CuratedAddon {
  name: string;
  source: string;
  summary: string;
  homepage: string;
  installed: boolean;
  installed_version: string;
}

export interface CuratedResult {
  family: string;
  label: string;
  profile_id: string;
  addons: CuratedAddon[];
}

// ---------- collections (addon loadouts) ----------

export interface CollectionInfo {
  id: string;
  name: string;
  addon_count: number;
  active: boolean;
}

export interface CollectionsResult {
  collections: CollectionInfo[];
  active_id: string;
}

export interface CollectionAddonState {
  folder: string;
  enabled: boolean;
}

export interface CollectionDetail {
  id: string;
  name: string;
  addons: CollectionAddonState[];
}

export interface SwitchCollectionResult {
  applied: string[];
  message: string;
}

// ---------- installs (per-install status + cross-install updates) ----------

export interface InstallStatus {
  root: string;
  flavor: string;
  addons_path: string;
  exe: string;
  version: string;
  profile_id: string;
  confidence: string;
  exists: boolean;
  addons: number;
  problems: number;
  errors: number;
  health: number;
}

export interface InstallsStatusResult {
  installs: InstallStatus[];
}

export interface SyncInstallEntry {
  root: string;
  updated: number;
  failed: number;
  errors: string[];
}

export interface SyncResult {
  installs: SyncInstallEntry[];
  total_updated: number;
  total_failed: number;
}

// ---------- diagnostics (doctor) ----------

export type CheckStatus = "ok" | "warn" | "error" | "info";

export interface DoctorCheck {
  name: string;
  status: CheckStatus;
  message: string;
}

export interface DoctorReport {
  checks: DoctorCheck[];
}

// ---------- addon providers ----------

export interface ProviderInfo {
  name: string;
  description: string;
}

// ---------- addon lookup (AddonInfo) ----------

/** One candidate when a bare addon name is ambiguous; re-call with its id. */
export interface InfoMatch {
  provider: string;
  id: string;
  name: string;
  summary: string;
  homepage: string;
}

export interface InfoResult {
  provider: string;
  id: string;
  name: string;
  author: string;
  summary: string;
  latest_version: string;
  homepage: string;
  game_version: string;
  updated_at: string;
  release_notes?: string;
  /** Populated when a bare name was ambiguous — the caller picks a match. */
  matches?: InfoMatch[];
}

// ---------- saved variables ----------

export interface SavedVarsListResult {
  wtf_root: string;
  account: string;
  files: string[];
}

export interface SavedVarsBackupResult {
  path: string;
  account: string;
}

export interface SavedVarsMigrateResult {
  copied: string[];
}

// ---------- snapshots (backups) ----------

export interface BackupInfo {
  id: string;
  created_at: string;
  reason: string;
  folders: string[];
}

export interface BackupResult {
  id: string;
}

export interface ListBackupsResult {
  snapshots: BackupInfo[];
}

export interface RestoreBackupResult {
  restored: string[];
  skipped: string[];
}

// ---------- collection export / import ----------

export interface ExportResult {
  out: string;
  addons: number;
  collection: string;
}

export interface ImportResult {
  installed: string[];
}

// ---------- settings ----------

export interface ConfigView {
  wow_path: string;
  flavor: string;
  profile: string;
  collection: string;
  theme: string;
  auto_backup: boolean;
  confirmations: boolean;
  backups_dir: string;
  curseforge_api_key: string;
  collections_dir: string;
}

// ---------- offline catalog snapshot (mirrors service.go) ----------

export interface SnapshotResult {
  snapshot_json: string;
  exported_at: string;
  addon_count: number;
  warnings: string[];
}

export interface SnapshotCheck {
  updates: UpdateEntry[];
  errors: string[];
}

/** The Service surface exposed by `window.go.service.Service`. */
export interface Service {
  GetState(): Promise<State>;
  DetectInstalls(): Promise<Install[]>;
  SetInstall(root: string, flavor: string): Promise<Install>;
  SetProfile(id: string): Promise<void>;
  Profiles(): Promise<Profile[]>;
  Scan(): Promise<ScanResult>;
  Validate(): Promise<ValidateResult>;
  Fix(folderName: string, allowDestructive: boolean): Promise<FixResult>;
  FixAll(allowDestructive: boolean): Promise<FixResult>;
  InstallZip(zipPath: string, allowReplace: boolean): Promise<InstallResult>;
  CheckUpdates(): Promise<CheckUpdatesResult>;
  ApplyUpdate(folder: string, allowReplace: boolean): Promise<ApplyBatch>;
  ApplyAllUpdates(allowReplace: boolean): Promise<ApplyBatch>;
  TrackedAddons(): Promise<TrackedResult>;
  SetAddonPinned(folder: string, pinned: boolean): Promise<void>;
  SetAddonIgnored(folder: string, ignored: boolean): Promise<void>;
  RollbackAddon(folder: string): Promise<RollbackResult>;
  ListAddonVersions(folder: string): Promise<VersionHistoryResult>;
  RollbackToVersion(folder: string, version: string): Promise<InstallSourceResult>;
  SearchCatalog(query: string): Promise<SearchCatalogResult>;
  Curated(): Promise<CuratedResult>;
  InstallSource(source: string, allowReplace: boolean): Promise<InstallSourceResult>;
  SaveWagoImport(id: string): Promise<WagoImportResult>;
  RestoreAddon(folder: string, allowReplace: boolean): Promise<InstallSourceResult>;
  Collections(): Promise<CollectionsResult>;
  CreateCollection(name: string): Promise<CollectionInfo>;
  SwitchCollection(id: string): Promise<SwitchCollectionResult>;
  DeleteCollection(id: string): Promise<void>;
  CollectionDetail(id: string): Promise<CollectionDetail>;
  SetCollectionAddon(id: string, folder: string, enabled: boolean): Promise<void>;
  InstallsStatus(): Promise<InstallsStatusResult>;
  SyncUpdatesToAll(allowReplace: boolean): Promise<SyncResult>;
  AddonInfo(arg: string): Promise<InfoResult>;
  Sources(): Promise<ProviderInfo[]>;
  Doctor(): Promise<DoctorReport>;
  SavedVarsAccounts(): Promise<string[]>;
  SavedVarsList(account: string): Promise<SavedVarsListResult>;
  SavedVarsBackup(account: string): Promise<SavedVarsBackupResult>;
  SavedVarsRestore(account: string, backupPath: string): Promise<void>;
  SavedVarsReset(account: string, addon: string): Promise<void>;
  SavedVarsMigrate(
    fromAccount: string,
    toAccount: string,
    addon: string,
  ): Promise<SavedVarsMigrateResult>;
  BackupNow(): Promise<BackupResult>;
  ListBackups(): Promise<ListBackupsResult>;
  RestoreBackup(id: string, allowReplace: boolean): Promise<RestoreBackupResult>;
  ExportCollection(
    outPath: string,
    collectionID: string,
    includeSavedVars: boolean,
  ): Promise<ExportResult>;
  ImportCollection(pathOrURL: string): Promise<ImportResult>;
  Config(): Promise<ConfigView>;
  SetConfigKey(key: string, value: string): Promise<void>;
  ExportSnapshot(): Promise<SnapshotResult>;
  CheckSnapshot(snapshotJSON: string): Promise<SnapshotCheck>;
}

export type View =
  | "setup"
  | "overview"
  | "scan"
  | "doctor"
  | "validate"
  | "install"
  | "updates"
  | "catalog"
  | "collections"
  | "exportimport"
  | "savedvars"
  | "backups"
  | "installs"
  | "settings";

export const DESTRUCTIVE_ACTIONS = new Set(["delete", "merge"]);

export const ACTION_LABELS: Record<string, string> = {
  rename: "Rename Folder",
  flatten: "Flatten Folder",
  "resolve-toc": "Pick TOC",
  delete: "Move to Trash",
  merge: "Merge Duplicates",
  "repair-structure": "Repair Structure",
};

export function formatBytes(n: number): string {
  if (n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return i === 0 ? `${v} B` : `${v.toFixed(1)} ${units[i]}`;
}
