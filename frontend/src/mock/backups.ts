// Mock fixtures for the Backups view. Stateful so "Backup now" prepends a
// snapshot that the reloaded list shows (LEARNINGS: reactive lists, no
// manual refresh). Timestamps are relative to "now" so a fresh snapshot
// always lands on top; each id is derived from the same instant as its
// created_at so the two columns stay consistent. Only methods owned by this
// slice are overridden.

import type { MockData } from "./index";
import type { BackupInfo } from "../types";

const pad = (n: number): string => String(n).padStart(2, "0");

/** Local-wall-clock snapshot id ("yyyymmdd-hhmm") + UTC ISO created_at. */
const stamp = (d: Date): { id: string; created_at: string } => ({
  id: `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`,
  created_at: d.toISOString(),
});

const HOUR = 3_600_000;

let snapshots: BackupInfo[] = [
  { ...stamp(new Date(Date.now() - 0.6 * HOUR)), reason: "Manual — before PvP arena testing", folders: 132 },
  { ...stamp(new Date(Date.now() - 5 * HOUR)), reason: "Before update: DeadlyBossMods 10.2.7 → 10.2.8", folders: 131 },
  { ...stamp(new Date(Date.now() - 26 * HOUR)), reason: "Before switch to collection Raiding", folders: 128 },
  { ...stamp(new Date(Date.now() - 30 * HOUR)), reason: "Auto-backup before install: Details Damage Meter", folders: 127 },
  { ...stamp(new Date(Date.now() - 50 * HOUR)), reason: "Before update: WeakAuras 5.12.5 → 5.13.0", folders: 126 },
  { ...stamp(new Date(Date.now() - 74 * HOUR)), reason: "Weekly auto-backup", folders: 124 },
];

export const data: MockData = {
  BackupNow: () => {
    const entry: BackupInfo = {
      ...stamp(new Date()),
      reason: "Manual backup",
      folders: 132,
    };
    snapshots = [entry, ...snapshots];
    return { id: entry.id };
  },

  ListBackups: () => ({ snapshots: snapshots.map((s) => ({ ...s })) }),

  RestoreBackup: () => ({
    restored: ["DeadlyBossMods", "WeakAuras", "Details", "BigWigs"],
    skipped: [],
  }),
};
