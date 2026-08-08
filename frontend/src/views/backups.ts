// Backups view — full-addon snapshots. Create a snapshot, list the history
// (id, created time, reason, folders) and restore one, gated behind a
// destructive confirmation dialog. The Snapshot section below the list is
// the offline export/check flow (CLI `snapshot export|check` parity).

import type { AppState, Actions } from "../app";
import type {
  BackupInfo,
  ListBackupsResult,
  UpdateEntry,
  SnapshotResult,
  SnapshotCheck,
} from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast, type ToastOpts } from "../components/toast";
import { confirmDialog } from "../components/dialog";

const PROVIDER_LABEL: Record<string, string> = {
  github: "GH",
  curseforge: "CF",
  wowinterface: "WoWI",
  tukui: "Tukui",
};

// Textareas have no stylesheet rules in this design system; the snapshot
// boxes are themed through the tokens contract inline (canvas field on the
// charcoal surface, hairline border, mono type).
const JSON_TEXTAREA_STYLE =
  "width:100%; box-sizing:border-box; resize:vertical; min-height:130px; " +
  "background:var(--canvas); color:var(--ink); border:1px solid var(--hairline); " +
  "border-radius:var(--r-md); padding:10px 12px; font-size:var(--body-sm); line-height:1.5;";

export function mountBackups(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let result: ListBackupsResult | null = null;
  let loading = false;
  let creating = false;
  let restoring = false;
  // --- snapshot export/check (CLI `snapshot export|check` parity) ----------
  let snapshotOpen = false;
  let exporting = false;
  let snapshot: SnapshotResult | null = null;
  let snapshotInput = "";
  let checkingSnapshot = false;
  let snapshotCheck: SnapshotCheck | null = null;
  let snapshotCheckErr: string | null = null;

  const load = async (): Promise<void> => {
    loading = true;
    rerender();
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
      rerender();
    }
  };

  const create = async (): Promise<void> => {
    if (creating) return;
    creating = true;
    rerender();
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
      rerender();
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
    rerender();
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
      rerender();
    }
  };

  const exportSnapshot = async (): Promise<void> => {
    if (exporting) return;
    exporting = true;
    rerender();
    try {
      snapshot = await service.ExportSnapshot();
      toast({
        type: "ok",
        title: "Snapshot exported",
        message: `${snapshot.addon_count} addon${snapshot.addon_count === 1 ? "" : "s"} frozen. Copy it, or save it as a file for later.`,
      });
    } catch (err) {
      toast({ type: "error", title: "Snapshot export failed", message: errText(err) });
    } finally {
      exporting = false;
      rerender();
    }
  };

  const copySnapshot = async (): Promise<void> => {
    if (!snapshot) return;
    const text = snapshot.snapshot_json;
    const okToast: ToastOpts = {
      type: "ok",
      title: "Snapshot copied",
      message: "Paste it into the check box, or save it for later.",
    };
    try {
      await navigator.clipboard.writeText(text);
      toast(okToast);
    } catch {
      // Clipboard API unavailable (non-secure context / older webview) —
      // fall back to select-and-execCommand on the read-only textarea.
      const ta = el.querySelector<HTMLTextAreaElement>("[data-snapshot-json]");
      if (!ta) {
        toast({ type: "error", title: "Copy failed", message: "Select the JSON and press Ctrl+C." });
        return;
      }
      ta.focus();
      ta.select();
      try {
        if (document.execCommand("copy")) toast(okToast);
        else toast({ type: "error", title: "Copy failed", message: "Select the JSON and press Ctrl+C." });
      } catch {
        toast({ type: "error", title: "Copy failed", message: "Select the JSON and press Ctrl+C." });
      }
    }
  };

  const checkSnapshot = async (): Promise<void> => {
    if (checkingSnapshot) return;
    const json = snapshotInput.trim();
    if (!json) {
      toast({ type: "warn", title: "Nothing to check", message: "Paste a snapshot JSON first." });
      return;
    }
    checkingSnapshot = true;
    snapshotCheck = null;
    snapshotCheckErr = null;
    rerender();
    try {
      snapshotCheck = await service.CheckSnapshot(json);
    } catch (err) {
      snapshotCheckErr = errText(err);
      toast({ type: "error", title: "Snapshot check failed", message: errText(err) });
    } finally {
      checkingSnapshot = false;
      rerender();
    }
  };

  // --- focus preservation ------------------------------------------------
  // Re-renders rebuild the view DOM, dropping keyboard focus to <body>.
  // Capture the focused snapshot control before a re-render and restore focus
  // to its replacement after. pendingFocus survives async flows (export /
  // check) whose final re-render happens after the backend call, while the
  // control is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (el: HTMLElement): string | null => {
    if (el.closest("[data-snapshot-toggle]")) return "snapshot-toggle";
    if (el.closest("[data-snapshot-export]")) return "snapshot-export";
    if (el.closest("[data-snapshot-copy]")) return "snapshot-copy";
    if (el.closest("[data-snapshot-check-input]")) return "snapshot-input";
    if (el.closest("[data-snapshot-check]")) return "snapshot-check";
    return null;
  };

  const restoreFocus = (key: string | null): boolean => {
    if (!key) return false;
    let target: HTMLElement | null = null;
    if (key === "snapshot-toggle") target = el.querySelector<HTMLElement>("[data-snapshot-toggle]");
    else if (key === "snapshot-export") target = el.querySelector<HTMLElement>("[data-snapshot-export]");
    else if (key === "snapshot-copy") target = el.querySelector<HTMLElement>("[data-snapshot-copy]");
    else if (key === "snapshot-input") target = el.querySelector<HTMLElement>("[data-snapshot-check-input]");
    else if (key === "snapshot-check") target = el.querySelector<HTMLElement>("[data-snapshot-check]");
    if (!target) return false;
    if (target.hasAttribute("disabled")) return false;
    target.focus();
    return true;
  };

  const rerender = (): void => {
    const active = document.activeElement;
    const key = pendingFocus ?? (active instanceof HTMLElement ? focusKeyOf(active) : null);
    render();
    if (!key) return;
    if (restoreFocus(key)) pendingFocus = null;
    else if (!loading && !creating && !restoring && !exporting && !checkingSnapshot) pendingFocus = null;
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

        ${snapshotSectionHtml()}
      </div>`;

    el.querySelectorAll<HTMLElement>("[data-create]").forEach((btn) => {
      btn.addEventListener("click", () => void create());
    });
    el.querySelectorAll<HTMLElement>("[data-restore]").forEach((btn) => {
      const snap = snaps.find((x) => x.id === btn.dataset.restore);
      if (snap) btn.addEventListener("click", () => void restore(snap));
    });
    el.querySelector("[data-snapshot-toggle]")?.addEventListener("click", () => {
      pendingFocus = "snapshot-toggle";
      snapshotOpen = !snapshotOpen;
      rerender();
    });
    el.querySelector("[data-snapshot-export]")?.addEventListener("click", () => {
      pendingFocus = "snapshot-export";
      void exportSnapshot();
    });
    el.querySelector("[data-snapshot-copy]")?.addEventListener("click", () => {
      pendingFocus = "snapshot-copy";
      void copySnapshot();
    });
    el.querySelector<HTMLTextAreaElement>("[data-snapshot-check-input]")?.addEventListener("input", (e) => {
      snapshotInput = (e.target as HTMLTextAreaElement).value;
    });
    el.querySelector("[data-snapshot-check]")?.addEventListener("click", () => {
      pendingFocus = "snapshot-check";
      void checkSnapshot();
    });
  };

  const renderRow = (s: BackupInfo, busy: boolean): string => `
    <tr>
      <td class="mono">${escapeHtml(s.id)}</td>
      <td class="mono">${escapeHtml(formatCreated(s.created_at))}</td>
      <td>${escapeHtml(s.reason || "n/a")}</td>
      <td class="backups-folders">
        ${s.folders.length
          ? s.folders
              .slice(0, 4)
              .map((f) => `<span class="tag tag-tracked">${escapeHtml(f)}</span>`)
              .join("") +
            (s.folders.length > 4
              ? `<span class="muted">+${s.folders.length - 4}</span>`
              : "")
          : `<span class="muted">n/a</span>`}
      </td>
      <td class="backups-actions">
        <button class="btn btn-danger btn-sm" data-restore="${escapeAttr(s.id)}" ${busy ? "disabled" : ""}>
          ${icon("download", 13)}<span>Restore</span>
        </button>
      </td>
    </tr>`;

  // Read-only variant of the update row: same layout, no apply button —
  // the snapshot check is a pure offline diff, applying stays a live
  // CheckUpdates/ApplyUpdate flow.
  const renderSnapshotRow = (u: UpdateEntry): string => `
    <div class="update-row${u.flavor_mismatch ? " has-mismatch" : ""}">
      <div class="update-info">
        <div class="update-name-line">
          <span class="update-name">${escapeHtml(u.title)}</span>
          <span class="update-folder mono">${escapeHtml(u.folder)}</span>
        </div>
        ${
          u.flavor_mismatch
            ? `<span class="mismatch-badge" title="Different game version - confirm before applying">${icon("alert", 12)}<span>${escapeHtml(u.flavor_label)}</span></span>`
            : ""
        }
      </div>
      <div class="update-versions mono" aria-label="Version change">
        <span class="update-ver-cur">${escapeHtml(u.current_version || "n/a")}</span>
        <span class="update-arrow">${icon("chevron-right", 13)}</span>
        <span class="update-ver-latest">${escapeHtml(u.latest_version || "n/a")}</span>
      </div>
      <div class="update-provider">${providerChip(u.provider)}</div>
      <div class="update-action"></div>
    </div>`;

  const checkResultHtml = (): string => {
    if (checkingSnapshot) {
      return `<div class="list-loading"><span class="spinner"></span><span>Checking snapshot…</span></div>`;
    }
    if (snapshotCheckErr) {
      return `<div class="managed-error" role="alert">${icon("alert", 14)}<span>${escapeHtml(snapshotCheckErr)}</span></div>`;
    }
    if (!snapshotCheck) return "";
    const ups = snapshotCheck.updates ?? [];
    const errs = snapshotCheck.errors ?? [];
    return `
      <div class="updates-summary" aria-label="Snapshot check summary">
        <span class="count-item"><span class="status-dot ok"></span><span class="count-num">${ups.length}</span> update${ups.length === 1 ? "" : "s"} available</span>
        <span class="count-item ${errs.length ? "" : "muted"}"><span class="status-dot ${errs.length ? "warn" : "muted"}"></span><span class="count-num">${errs.length}</span> warning${errs.length === 1 ? "" : "s"}</span>
      </div>
      ${
        errs.length
          ? `<div class="snapshot-errors">${errs
              .map((e) => `<div class="managed-error" role="alert">${icon("alert", 14)}<span>${escapeHtml(e)}</span></div>`)
              .join("")}</div>`
          : ""
      }
      ${
        ups.length === 0
          ? `<p class="result-hint">Nothing to update. The registry already matches this snapshot.</p>`
          : `<div class="update-rows">${ups.map(renderSnapshotRow).join("")}</div>`
      }`;
  };

  const snapshotSectionHtml = (): string => {
    const exportSummary = snapshot
      ? `<p class="result-hint">${snapshot.addon_count} addon${snapshot.addon_count === 1 ? "" : "s"} · exported ${formatDateTime(snapshot.exported_at)}</p>`
      : "";
    const exportWarnings =
      snapshot && snapshot.warnings.length > 0
        ? `<ul class="snapshot-warnings" style="margin:0; padding-left:18px; display:flex; flex-direction:column; gap:2px; font-size:var(--caption); color:var(--warn)">
            ${snapshot.warnings.map((w) => `<li>${escapeHtml(w)}</li>`).join("")}
          </ul>`
        : "";
    return `
      <section class="snapshot" aria-label="Snapshot - offline update check">
        <button class="btn btn-ghost btn-sm snapshot-toggle" data-snapshot-toggle aria-expanded="${snapshotOpen}" aria-controls="snapshot-body">
          ${icon(snapshotOpen ? "chevron-down" : "chevron-right", 14)}
          <span>Snapshot</span>
          <span class="muted">offline update check</span>
        </button>
        ${
          snapshotOpen
            ? `<div class="snapshot-body" id="snapshot-body">
                <div class="field-row snapshot-cols">
                  <div class="field snapshot-col">
                    <h3 class="detail-title">Export</h3>
                    <p class="result-hint">Freeze the tracked addons and their latest known versions into portable JSON for an offline check.</p>
                    <div class="snapshot-actions">
                      <button class="btn btn-outline btn-sm" data-snapshot-export ${exporting ? "disabled" : ""}>
                        ${exporting ? `<span class="spinner"></span>` : icon("download", 14)}
                        <span>${exporting ? "Exporting…" : "Export snapshot"}</span>
                      </button>
                      ${
                        snapshot
                          ? `<button class="btn btn-primary btn-sm" data-snapshot-copy>${icon("file", 14)}<span>Copy</span></button>`
                          : ""
                      }
                    </div>
                    ${exportSummary}
                    ${
                      snapshot
                        ? `<textarea class="mono" data-snapshot-json readonly rows="10" spellcheck="false" aria-label="Exported snapshot JSON" style="${JSON_TEXTAREA_STYLE}">${escapeHtml(snapshot.snapshot_json)}</textarea>`
                        : ""
                    }
                    ${exportWarnings}
                  </div>
                  <div class="field snapshot-col">
                    <h3 class="detail-title">Check</h3>
                    <p class="result-hint">Paste a snapshot and diff it against the current registry. No network access.</p>
                    <textarea class="mono" data-snapshot-check-input rows="7" spellcheck="false" aria-label="Snapshot JSON to check" placeholder="Paste snapshot JSON here…" style="${JSON_TEXTAREA_STYLE}">${escapeHtml(snapshotInput)}</textarea>
                    <div class="snapshot-actions">
                      <button class="btn btn-primary btn-sm" data-snapshot-check ${checkingSnapshot ? "disabled" : ""}>
                        ${checkingSnapshot ? `<span class="spinner"></span>` : icon("search", 14)}
                        <span>${checkingSnapshot ? "Checking…" : "Check"}</span>
                      </button>
                    </div>
                    ${checkResultHtml()}
                  </div>
                </div>
              </div>`
            : ""
        }
      </section>`;
  };

  render();
  void load();

  return { refresh: render };
}

function providerChip(provider: string): string {
  const label = PROVIDER_LABEL[provider] ?? provider;
  return `<span class="provider-chip prov-${escapeAttr(provider)}" title="${escapeAttr(provider)}">${escapeHtml(label)}</span>`;
}

function formatCreated(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return (
    d.toLocaleDateString([], { year: "numeric", month: "short", day: "numeric" }) +
    " " +
    d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  );
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
