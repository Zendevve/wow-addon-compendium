import type { MockData } from "./index";
import type { ConfigView, InstallStatus } from "../types";

// Owned by the settings slice: Config, SetConfigKey, InstallsStatus,
// SyncUpdatesToAll. SetConfigKey mutates the shared Config fixture so the
// view reflects saved values without a reload.

const cfg: ConfigView = {
  wow_path: "D:\\Games\\World of Warcraft\\_retail_",
  flavor: "retail",
  profile: "main",
  collection: "Raid",
  theme: "dark",
  auto_backup: true,
  confirmations: true,
  backups_dir: "C:\\Users\\natha\\Documents\\wowfix\\backups",
  curseforge_api_key: "cf_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  collections_dir: "C:\\Users\\natha\\Documents\\wowfix\\collections",
};

const INSTALLS: InstallStatus[] = [
  {
    root: "D:\\Games\\World of Warcraft\\_retail_",
    flavor: "retail",
    addons_path: "D:\\Games\\World of Warcraft\\_retail_\\Interface\\AddOns",
    exe: "D:\\Games\\World of Warcraft\\_retail_\\Wow.exe",
    version: "11.0.2.56371",
    profile_id: "main",
    confidence: "high",
    exists: true,
    addons: 87,
    problems: 2,
    errors: 0,
    health: 94,
  },
  {
    root: "D:\\Games\\World of Warcraft\\_classic_",
    flavor: "classic",
    addons_path: "D:\\Games\\World of Warcraft\\_classic_\\Interface\\AddOns",
    exe: "D:\\Games\\World of Warcraft\\_classic_\\WowClassic.exe",
    version: "4.4.2.56819",
    profile_id: "classic",
    confidence: "high",
    exists: true,
    addons: 41,
    problems: 1,
    errors: 1,
    health: 81,
  },
  {
    root: "D:\\Games\\World of Warcraft\\_classic_era_",
    flavor: "classic_era",
    addons_path: "D:\\Games\\World of Warcraft\\_classic_era_\\Interface\\AddOns",
    exe: "D:\\Games\\World of Warcraft\\_classic_era_\\WowClassic.exe",
    version: "1.15.3.56819",
    profile_id: "classic_era",
    confidence: "medium",
    exists: true,
    addons: 23,
    problems: 5,
    errors: 3,
    health: 62,
  },
  {
    root: "D:\\Games\\World of Warcraft\\_ptr_",
    flavor: "ptr",
    addons_path: "D:\\Games\\World of Warcraft\\_ptr_\\Interface\\AddOns",
    exe: "D:\\Games\\World of Warcraft\\_ptr_\\Wow.exe",
    version: "11.1.0.56471",
    profile_id: "ptr",
    confidence: "low",
    exists: false,
    addons: 0,
    problems: 0,
    errors: 0,
    health: 0,
  },
];

export const data: MockData = {
  Config: () => cfg,
  SetConfigKey: (...args: unknown[]) => {
    const key = String(args[0]);
    const value = String(args[1]);
    (cfg as unknown as Record<string, unknown>)[key] = coerceValue(key, value);
  },
  InstallsStatus: () => ({ installs: INSTALLS }),
  SyncUpdatesToAll: () => ({
    installs: [
      {
        root: "D:\\Games\\World of Warcraft\\_retail_",
        updated: 4,
        failed: 0,
        errors: [],
      },
      {
        root: "D:\\Games\\World of Warcraft\\_classic_",
        updated: 2,
        failed: 1,
        errors: ["WeakAuras: version check timed out"],
      },
      {
        root: "D:\\Games\\World of Warcraft\\_classic_era_",
        updated: 0,
        failed: 2,
        errors: [
          "Details: CurseForge provider unreachable",
          "Plater: hash mismatch after download",
        ],
      },
    ],
    total_updated: 6,
    total_failed: 3,
  }),
};

function coerceValue(key: string, value: string): unknown {
  if (key === "auto_backup" || key === "confirmations") return value === "true";
  return value;
}
