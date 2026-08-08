// Backups: full-addon snapshots. Create a snapshot, list the history
// (newest first — id, created time, reason, folder count) and restore one
// behind a destructive confirmation dialog. The offline snapshot export/check
// flow is owned by the Updates view; this view only cross-links to it.

import type { View } from "../view";
import type { BackupInfo, ListBackupsResult } from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast } from "../toast";
import { confirmDialog } from "../dialog";
import "./backups.css";

export const view: View = {
  id: "backups",
  label: "Backups",
  icon: "archive",
  mount(host) {
    mountBackups(host);
  },
};

function mountBackups(host: HTMLElement): void {
  let result: ListBackupsResult | null = null;
  let loading = false;
  let loadError: string | null = null;
  let creating = false;
  let restoringId = "";

  // focus preservation: re-renders rebuild the DOM, so capture the focused
  // control before a render and restore it after. pendingFocus survives
  // async flows whose final re-render happens while the control is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (node: HTMLElement | null): string | null => {
    if (!node) return null;
    if (node.closest("[data-create]")) return "create";
    const r = node.closest<HTMLElement>("[data-restore]");
    if (r) return `restore:${r.dataset.restore}`;
    return null;
  };

  const restoreFocus = (key: string): boolean => {
    let target: HTMLElement | null = null;
    if (key === "create") target = host.querySelector("[data-create]");
    else if (key.startsWith("restore:"))
      target = host.querySelector(`[data-restore="${key.slice("restore:".length)}"]`);
    if (!target || (target as HTMLButtonElement).disabled) return false;
    target.focus();
    return true;
  };

  const rerender = (): void => {
    const active = document.activeElement;
    const key = pendingFocus ?? focusKeyOf(active instanceof HTMLElement ? active : null);
    render();
    if (!key) return;
    if (restoreFocus(key)) pendingFocus = null;
    else if (loading || creating || restoringId) {
      // keep pendingFocus: the final re-render lands it on the re-enabled control
    } else {
      pendingFocus = null;
    }
  };

  const load = async (): Promise<void> => {
    if (loading) return;
    loading = true;
    loadError = null;
    rerender();
    try {
      result = await service.ListBackups();
    } catch (err) {
      loadError = errText(err);
      if (result) {
        toast({ type: "error", title: "Could not refresh backups", message: loadError });
      }
    } finally {
      loading = false;
      rerender();
    }
  };

  const create = async (): Promise<void> => {
    if (creating) return;
    creating = true;
    rerender();
    try {
      const res = await service.BackupNow();
      toast({ type: "ok", title: "Backup created", message: `Snapshot ${res.id}` });
      await load();
    } catch (err) {
      toast({ type: "error", title: "Backup failed", message: errText(err) });
    } finally {
      creating = false;
      rerender();
    }
  };

  const restore = async (snap: BackupInfo): Promise<void> => {
    if (restoringId) return;
    const confirmed = await confirmDialog({
      title: `Restore snapshot ${snap.id}?`,
      message:
        "Every addon folder in this snapshot is replaced by its backed-up state. Current folders are snapshotted first.",
      details: [`${snap.folders} addon folder${snap.folders === 1 ? "" : "s"} will be replaced`],
      confirmLabel: "Restore",
      danger: true,
    });
    if (!confirmed) return;
    restoringId = snap.id;
    rerender();
    try {
      const res = await service.RestoreBackup(snap.id, true);
      toast({
        type: "ok",
        title: "Backup restored",
        message: `${res.restored.length} restored${res.skipped.length ? ` · ${res.skipped.length} skipped` : ""}`,
      });
      await load();
    } catch (err) {
      toast({ type: "error", title: "Restore failed", message: errText(err) });
    } finally {
      restoringId = "";
      rerender();
    }
  };

  const render = (): void => {
    // Newest first. Date.parse handles any RFC3339 offset; localeCompare is
    // locale-collated and misorders mixed-offset timestamps, so never use it.
    const ts = (iso: string): number => Date.parse(iso) || 0;
    const snaps = result
      ? [...result.snapshots].sort((a, b) => ts(b.created_at) - ts(a.created_at))
      : [];
    const busy = creating || Boolean(restoringId);

    host.innerHTML = `
      <section class="view-page">
        <div class="view-hero">
          <h1 class="view-title">Backups</h1>
          <p class="view-sub">Full-addon snapshots taken before every change — restore any point, or diff an offline snapshot.</p>
        </div>

        <div class="backups">
          <div class="backups-toolbar">
            <button class="btn-primary" data-create ${busy ? "disabled" : ""}>
              ${creating ? `<span class="backups-spin"></span>` : icon("archive", 15)}
              <span>${creating ? "Creating…" : "Backup now"}</span>
            </button>
            ${result ? `<span class="backups-count tnum">${snaps.length} snapshot${snaps.length === 1 ? "" : "s"}</span>` : ""}
            <span class="backups-hint">Snapshots are taken automatically before every change when auto-backup is on.</span>
          </div>

          ${renderList(snaps, busy)}

          <div class="backups-note">
            ${icon("info", 15)}
            <span>Offline snapshot export and check live in the Updates view — export the tracked set to portable JSON and diff it with no network.</span>
          </div>
        </div>
      </section>`;

    host.querySelectorAll<HTMLElement>("[data-create]").forEach((btn) =>
      btn.addEventListener("click", () => void create()),
    );
    host.querySelectorAll<HTMLElement>("[data-retry]").forEach((btn) =>
      btn.addEventListener("click", () => void load()),
    );
    host.querySelectorAll<HTMLElement>("[data-restore]").forEach((btn) => {
      const snap = snaps.find((s) => s.id === btn.dataset.restore);
      if (snap) {
        btn.addEventListener("click", () => {
          pendingFocus = `restore:${snap.id}`;
          void restore(snap);
        });
      }
    });
  };

  const renderList = (snaps: BackupInfo[], busy: boolean): string => {
    if (!result) {
      if (loading) {
        return `<div class="backups-loading"><span class="backups-spin"></span><span>Loading backups…</span></div>`;
      }
      if (loadError) return renderError(loadError);
      return "";
    }
    if (snaps.length === 0) {
      return `
        <div class="empty-state">
          <span class="empty-title">No backups yet</span>
          <span class="empty-body">Create a snapshot to capture the current addon state as a safe restore point.</span>
          <button class="btn-primary" data-create>${icon("archive", 15)}<span>Backup now</span></button>
        </div>`;
    }
    return `
      <div class="backups-table-wrap">
        <table class="backups-table">
          <thead>
            <tr><th>Snapshot</th><th>Created</th><th>Reason</th><th>Folders</th><th></th></tr>
          </thead>
          <tbody>
            ${snaps.map((s) => renderRow(s, busy)).join("")}
          </tbody>
        </table>
      </div>`;
  };

  const renderRow = (s: BackupInfo, busy: boolean): string => `
    <tr>
      <td class="backups-id mono" data-label="Snapshot">${escapeHtml(s.id)}</td>
      <td class="backups-when tnum" data-label="Created">${escapeHtml(formatCreated(s.created_at))}</td>
      <td class="backups-reason" data-label="Reason">${escapeHtml(s.reason || "—")}</td>
      <td class="backups-folders tnum" data-label="Folders">${s.folders}</td>
      <td class="backups-actions" data-label="">
        <button class="btn-danger backups-btn-small" data-restore="${escapeAttr(s.id)}" ${busy ? "disabled" : ""}>
          ${restoringId === s.id ? `<span class="backups-spin"></span>` : icon("download", 13)}
          <span>${restoringId === s.id ? "Restoring…" : "Restore"}</span>
        </button>
      </td>
    </tr>`;

  const renderError = (msg: string): string => `
    <div class="backups-error" role="alert">
      ${icon("alert", 15)}
      <div class="backups-error-body">
        <p>${escapeHtml(msg)}</p>
        <button class="btn-secondary backups-btn-small" data-retry>${icon("refresh", 13)}<span>Retry</span></button>
      </div>
    </div>`;

  void load();
}

function formatCreated(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString([], {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
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
