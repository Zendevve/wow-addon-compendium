// Backups view — full-addon snapshots. Create a snapshot, list the history
// (id, created time, reason, folders) and restore one, gated behind a
// destructive confirmation dialog.

import type { AppState, Actions } from "../app";
import type { BackupInfo, ListBackupsResult } from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
import { confirmDialog } from "../components/dialog";

export function mountBackups(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let result: ListBackupsResult | null = null;
  let loading = false;
  let creating = false;
  let restoring = false;

  const load = async (): Promise<void> => {
    loading = true;
    render();
    try {
      result = await service.ListBackups();
    } catch (err) {
      toast({
        type: "error",
        title: "Could not load backups",
        message: errText(err),
      });
    } finally {
      loading = false;
      render();
    }
  };

  const create = async (): Promise<void> => {
    if (creating) return;
    creating = true;
    render();
    try {
      const res = await service.BackupNow();
      toast({
        type: "ok",
        title: "Backup created",
        message: `Snapshot ${res.id}`,
      });
      await load();
    } catch (err) {
      toast({
        type: "error",
        title: "Backup failed",
        message: errText(err),
      });
    } finally {
      creating = false;
      render();
    }
  };

  const restore = async (snap: BackupInfo): Promise<void> => {
    if (restoring) return;
    const confirmed = await confirmDialog({
      title: `Restore snapshot ${snap.id}?`,
      message:
        "Every addon folder in this snapshot is replaced by its backed-up state. Current folders are snapshotted first.",
      details: snap.folders.length
        ? [`${snap.folders.length} folder${snap.folders.length === 1 ? "" : "s"}: ${snap.folders.join(", ")}`]
        : undefined,
      confirmLabel: "Restore",
      danger: true,
    });
    if (!confirmed) return;
    restoring = true;
    render();
    try {
      const res = await service.RestoreBackup(snap.id, true);
      toast({
        type: "ok",
        title: "Backup restored",
        message: `${res.restored.length} restored · ${res.skipped.length} skipped`,
      });
      await load();
    } catch (err) {
      toast({
        type: "error",
        title: "Restore failed",
        message: errText(err),
      });
    } finally {
      restoring = false;
      render();
    }
  };

  const render = (): void => {
    const busy = loading || creating || restoring;
    const snaps = result?.snapshots ?? [];

    el.innerHTML = `
      <div class="backups">
        <div class="backups-toolbar">
          <button class="btn btn-primary" data-create ${busy ? "disabled" : ""}>
            ${creating ? `<span class="spinner"></span>` : icon("archive", 15)}
            <span>${creating ? "Creating…" : "Create backup"}</span>
          </button>
          ${
            result
              ? `<span class="muted">${snaps.length} snapshot${snaps.length === 1 ? "" : "s"}</span>`
              : ""
          }
          <span class="backups-hint muted">Snapshots are taken automatically before every change when auto-backup is on.</span>
        </div>

        ${
          !result
            ? `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Loading backups…</span></div>`
            : snaps.length === 0
              ? `<div class="empty">
                  <span class="empty-icon">${icon("archive", 28)}</span>
                  <h2 class="empty-title">No backups yet</h2>
                  <p class="empty-sub">Create a snapshot to capture the current addon state for a safe restore point.</p>
                  <div class="empty-actions">
                    <button class="btn btn-primary" data-create>${icon("archive", 16)}<span>Create backup</span></button>
                  </div>
                </div>`
              : `<div class="table-wrap"><table class="table">
                  <thead><tr><th>ID</th><th>Created</th><th>Reason</th><th>Folders</th><th></th></tr></thead>
                  <tbody>
                    ${snaps.map((s) => renderRow(s, busy)).join("")}
                  </tbody>
                </table></div>`
        }
      </div>`;

    el.querySelectorAll<HTMLElement>("[data-create]").forEach((btn) => {
      btn.addEventListener("click", () => void create());
    });
    el.querySelectorAll<HTMLElement>("[data-restore]").forEach((btn) => {
      const snap = snaps.find((x) => x.id === btn.dataset.restore);
      if (snap) btn.addEventListener("click", () => void restore(snap));
    });
  };

  const renderRow = (s: BackupInfo, busy: boolean): string => `
    <tr>
      <td class="mono">${escapeHtml(s.id)}</td>
      <td class="mono">${escapeHtml(formatCreated(s.created_at))}</td>
      <td>${escapeHtml(s.reason || "—")}</td>
      <td class="backups-folders">
        ${s.folders.length
          ? s.folders
              .slice(0, 4)
              .map((f) => `<span class="tag tag-tracked">${escapeHtml(f)}</span>`)
              .join("") +
            (s.folders.length > 4
              ? `<span class="muted">+${s.folders.length - 4}</span>`
              : "")
          : `<span class="muted">—</span>`}
      </td>
      <td class="backups-actions">
        <button class="btn btn-danger btn-sm" data-restore="${escapeAttr(s.id)}" ${busy ? "disabled" : ""}>
          ${icon("download", 13)}<span>Restore</span>
        </button>
      </td>
    </tr>`;

  render();
  void load();

  return { refresh: render };
}

function formatCreated(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function errText(err: unknown, fallback?: string): string {
  if (err instanceof Error && err.message) return err.message;
  if (fallback) return fallback;
  return String(err ?? "Unknown error");
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESC[c]);
}
function escapeAttr(s: string): string {
  return escapeHtml(s).replaceAll("'", "&#39;");
}
const ESC: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};
