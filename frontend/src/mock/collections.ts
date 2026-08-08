// Mock fixtures for the Collections view. Overrides are stateful so create /
// switch / delete / toggle flows update the rendered list (LEARNINGS:
// reactive, zero-manual-refresh rendering). Only methods owned by this slice
// are overridden; everything else falls through to the Proxy defaults.

import type { MockData } from "./index";
import type { CollectionAddonState, CollectionDetail, CollectionInfo } from "../types";

interface Stored {
  info: CollectionInfo;
  addons: CollectionAddonState[];
}

const seed: Stored[] = [
  {
    info: { id: "col-pve", name: "PvE — Main", addon_count: 12, active: true },
    addons: [
      { folder: "DeadlyBossMods", enabled: true },
      { folder: "WeakAuras", enabled: true },
      { folder: "Details", enabled: true },
      { folder: "PlaterNameplates", enabled: true },
      { folder: "GTFO", enabled: true },
      { folder: "OmniCC", enabled: true },
      { folder: "Simulationcraft", enabled: true },
      { folder: "RaiderIO", enabled: true },
      { folder: "Leatrix_Plus", enabled: true },
      { folder: "MinimapButtonBag", enabled: true },
      { folder: "MythicDungeonTools", enabled: false },
      { folder: "Scrap", enabled: true },
    ],
  },
  {
    info: { id: "col-pvp", name: "PvP — Arena", addon_count: 7, active: false },
    addons: [
      { folder: "GladiusEx", enabled: true },
      { folder: "OmniBar", enabled: true },
      { folder: "sArena", enabled: true },
      { folder: "Diminish", enabled: true },
      { folder: "BattleGroundEnemies", enabled: false },
      { folder: "BigDebuffs", enabled: true },
      { folder: "NameplateAuras", enabled: true },
    ],
  },
  {
    info: { id: "col-raid", name: "Raiding", addon_count: 10, active: false },
    addons: [
      { folder: "DeadlyBossMods", enabled: true },
      { folder: "BigWigs", enabled: true },
      { folder: "WeakAuras", enabled: true },
      { folder: "ExorsusRaidTools", enabled: true },
      { folder: "MethodRaidTools", enabled: true },
      { folder: "Details", enabled: true },
      { folder: "GTFO", enabled: true },
      { folder: "RaiderIO", enabled: true },
      { folder: "VuhDo", enabled: true },
      { folder: "RCLootCouncil", enabled: false },
    ],
  },
  {
    info: { id: "col-alt", name: "Leveling", addon_count: 3, active: false },
    addons: [
      { folder: "GatherMate2", enabled: true },
      { folder: "HandyNotes", enabled: true },
      { folder: "TomTom", enabled: true },
    ],
  },
];

const clone = (src: Stored[]): Stored[] =>
  src.map((s) => ({ info: { ...s.info }, addons: s.addons.map((a) => ({ ...a })) }));

const stored = clone(seed);

const detailOf = (s: Stored): CollectionDetail => ({
  id: s.info.id,
  name: s.info.name,
  addons: s.addons.map((a) => ({ ...a })),
});

export const data: MockData = {
  Collections: () => ({
    collections: stored.map((s) => ({
      ...s.info,
      addon_count: s.addons.length,
    })),
    active_id: stored.find((s) => s.info.active)?.info.id ?? "",
  }),

  CreateCollection: (...args: unknown[]) => {
    const name = String(args[0] ?? "").trim() || "New collection";
    const entry: Stored = {
      info: { id: `col-${Date.now()}`, name, addon_count: 0, active: false },
      addons: [],
    };
    stored.push(entry);
    return { ...entry.info };
  },

  SwitchCollection: (...args: unknown[]) => {
    const id = String(args[0]);
    for (const s of stored) s.info.active = s.info.id === id;
    const target = stored.find((s) => s.info.id === id);
    const folders = target?.addons.map((a) => a.folder) ?? [];
    return {
      applied: folders,
      message: `${folders.length} folder${folders.length === 1 ? "" : "s"} renamed to match. Backup snapshot taken.`,
    };
  },

  DeleteCollection: (...args: unknown[]) => {
    const id = String(args[0]);
    const i = stored.findIndex((s) => s.info.id === id);
    if (i >= 0) stored.splice(i, 1);
  },

  CollectionDetail: (...args: unknown[]) => {
    const id = String(args[0]);
    const s = stored.find((x) => x.info.id === id);
    return s ? detailOf(s) : { id, name: "Unknown", addons: [] };
  },

  SetCollectionAddon: (...args: unknown[]) => {
    const [id, folder, enabled] = args as [string, string, boolean];
    const s = stored.find((x) => x.info.id === id);
    const a = s?.addons.find((x) => x.folder === folder);
    if (a) a.enabled = Boolean(enabled);
  },

  ExportCollection: (...args: unknown[]) => {
    const [outPath, collectionID, includeSavedVars] = args as [string, string, boolean];
    const s = stored.find((x) => x.info.id === collectionID);
    const addons = s?.addons.filter((a) => a.enabled).length ?? 0;
    return {
      out: String(outPath || "collections-export.zip"),
      addons,
      collection: s ? s.info.name : "",
    };
  },

  ImportCollection: (...args: unknown[]) => {
    const src = String(args[0] ?? "");
    if (/\.zip$/i.test(src) || /github\.com|curseforge\.com|wago\.io/i.test(src)) {
      return { installed: ["DeadlyBossMods", "PlaterNameplates", "WeakAuras"] };
    }
    return { installed: [] };
  },
};
