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
export type Provider = "github" | "curseforge" | "wowinterface" | "tukui";

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
  SearchCatalog(query: string): Promise<SearchCatalogResult>;
  InstallSource(source: string, allowReplace: boolean): Promise<InstallSourceResult>;
  RestoreAddon(folder: string, allowReplace: boolean): Promise<InstallSourceResult>;
}

export type View = "setup" | "scan" | "validate" | "install" | "updates" | "catalog";

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
  if (n <= 0) return "—";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return i === 0 ? `${v} B` : `${v.toFixed(1)} ${units[i]}`;
}
