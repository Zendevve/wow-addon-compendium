// Shared UI-state contract between main.ts (which owns the store) and the
// view modules (which only read it and call actions).

import type {
  State,
  Profile,
  ScanResult,
  ValidateResult,
  InstallResult,
  View,
  Addon,
} from "./types";

export interface AppState {
  state: State;
  profiles: Profile[];
  scan: ScanResult | null;
  validation: ValidateResult | null;
  installResult: InstallResult | null;
  view: View;
  busy: string | null; // label of the in-flight operation, if any
  filter: string;
  mock: boolean;
}

export interface Actions {
  go(view: View): void;
  scan(): Promise<void>;
  fixOne(addon: Addon): Promise<void>;
  fixAll(): Promise<void>;
  restoreAddon(addon: Addon): Promise<void>;
  setProfile(id: string): Promise<void>;
  validate(): Promise<void>;
  installZip(zipPath: string, allowReplace: boolean): Promise<void>;
  completeSetup(root: string, flavor: string, profileId: string): Promise<void>;
}
