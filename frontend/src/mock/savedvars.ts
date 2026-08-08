import type { MockData } from "./index";

// Owned by the savedvars slice: SavedVarsAccounts, SavedVarsList,
// SavedVarsBackup, SavedVarsRestore, SavedVarsReset, SavedVarsMigrate.
// Fixtures are mutable so reset/migrate are reflected on the next list.
// File names are stems; the view appends ".lua" for display (matching the
// backend contract).

const WTF_ROOT = "D:\\Games\\World of Warcraft\\_retail_\\WTF";

// Account folder -> SavedVariables stems (no extension).
const ACCOUNTS: Record<string, string[]> = {
  Arthalion: [
    "DBM",
    "Details",
    "WeakAuras",
    "Plater",
    "BigWigs",
    "Questie",
    "Leatrix_Plus",
    "Bartender4",
    "AdiBags",
    "ElvUI",
    "ShadowedUnitFrames",
    "MythicPlusTimer",
  ],
  Thalyssra: [
    "DBM",
    "WeakAuras",
    "Plater",
    "Questie",
    "AllTheThings",
    "ElvUI",
    "Rarity",
    "HandyNotes",
  ],
};

// DBM-Core shares its SavedVariables with DBM; resets never touch it.
const PROTECTED = new Set(["DBM", "DBM-Core"]);

export const data: MockData = {
  SavedVarsAccounts: () => Object.keys(ACCOUNTS),
  SavedVarsList: (...args: unknown[]) => {
    const account = String(args[0]);
    return {
      wtf_root: WTF_ROOT,
      account,
      files: [...(ACCOUNTS[account] ?? [])],
    };
  },
  SavedVarsBackup: (...args: unknown[]) => {
    const account = String(args[0]);
    return {
      path: `${WTF_ROOT}\\Account\\${account}\\SavedVariables\\backups\\savedvars-2026-08-08-14-30.lua`,
      account,
    };
  },
  SavedVarsRestore: () => {},
  SavedVarsReset: (...args: unknown[]) => {
    const account = String(args[0]);
    const addon = String(args[1]);
    if (PROTECTED.has(addon)) return; // DBM-Core survives resets
    const files = ACCOUNTS[account];
    if (!files) return;
    const idx = files.findIndex((f) => f.toLowerCase() === addon.toLowerCase());
    if (idx === -1) {
      throw new Error(`no SavedVariables file for addon "${addon}" in account "${account}"`);
    }
    files.splice(idx, 1);
  },
  SavedVarsMigrate: (...args: unknown[]) => {
    const from = String(args[0]);
    const to = String(args[1]);
    const addon = String(args[2] ?? "").trim();
    const fromFiles = ACCOUNTS[from] ?? [];
    const toFiles = ACCOUNTS[to] ?? [];
    const existing = new Set(toFiles.map((f) => f.toLowerCase()));
    const copied: string[] = [];
    for (const f of fromFiles) {
      if (addon && f.toLowerCase() !== addon.toLowerCase()) continue;
      if (existing.has(f.toLowerCase())) continue; // never overwrite
      toFiles.push(f);
      existing.add(f.toLowerCase());
      copied.push(f);
    }
    return { copied };
  },
};
