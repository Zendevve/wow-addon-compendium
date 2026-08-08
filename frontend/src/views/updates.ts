// Updates: the update-review surface (the LEARNINGS showpiece). Check once
// (cached provider loop), review the pending list — needs-update rows first,
// side-by-side old → new versions, per-addon status chips — then apply per
// addon or in one badge-counted pass. The managed table carries pin/ignore
// toggles, per-version rollback history, and restore for drifted installs.
// The offline snapshot section exports the registry to JSON and diffs any
// pasted snapshot against the current install with no network access.

import type { View } from "../view";
import "./updates.css";
import type {
  ApplyBatch,
  CheckUpdatesResult,
  Provider,
  SnapshotCheck,
  SnapshotResult,
  TrackedAddon,
  UpdateEntry,
  VersionEntry,
  VersionHistoryResult,
} from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast } from "../toast";
import { confirmDialog } from "../dialog";

const PROVIDER_LABEL: Record<string, string> = {
  github: "GitHub",
  curseforge: "CurseForge",
  wowinterface: "WoWInterface",
  tukui: "Tukui",
  wago: "Wago",
};

type ManagedFilter = "all" | "pinned" | "ignored";

const MANAGED_FILTERS: { value: ManagedFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "pinned", label: "Pinned" },
  { value: "ignored", label: "Ignored" },
];

// Open version-history modals, cleaned up on unmount so navigating away
// never leaves a stray overlay or a dangling keydown listener.
const modalCleanups: Array<() => void> = [];
let modalSeq = 0;

export const view: View = {
  id: "updates",
  label: "Updates",
  icon: "refresh",
  mount(host) {
    let install = false;
    let bootErr: string | null = null;
    let result: CheckUpdatesResult | null = null;
    let checking = false;
    let applying: string | null = null; // folder being updated, or "all"
    // Row-level failures from apply batches: folder -> error text. Failed
    // rows keep their error state (and the end-of-batch dump stays) until
    // the update succeeds — never a blanket toast.
    const failures = new Map<string, string>();
    let tracked: TrackedAddon[] | null = null;
    let trackedErr: string | null = null;
    let scanErr: string | null = null;
    let drifted = new Set<string>();
    let managedFilter: ManagedFilter = "all";
    let mutating: string | null = null; // folder being pinned/ignored/restored
    // --- offline snapshot ---
    let exporting = false;
    let snapshot: SnapshotResult | null = null;
    let snapshotInput = "";
    let checkingSnapshot = false;
    let snapshotCheck: SnapshotCheck | null = null;
    let snapshotCheckErr: string | null = null;
    // Re-renders rebuild the DOM, dropping keyboard focus to <body>. Capture
    // the focused control before a re-render and restore focus to its
    // replacement after. pendingFocus survives async flows (check / apply /
    // pin / ignore / restore / snapshot) whose final re-render happens after
    // the backend call, while the control is disabled.
    let pendingFocus: string | null = null;

    // --- data loading -----------------------------------------------------

    const check = async (): Promise<void> => {
      checking = true;
      rerender();
      try {
        result = await service.CheckUpdates();
      } catch (err) {
        toast({ type: "error", title: "Check failed", message: errText(err) });
      } finally {
        checking = false;
        rerender();
      }
    };

    const loadTracked = async (): Promise<void> => {
      try {
        tracked = (await service.TrackedAddons()).addons;
        trackedErr = null;
      } catch (err) {
        trackedErr = errText(err, "Could not load tracked addons");
      }
      rerender();
    };

    // Drift only exists on the scan Addon DTO; join it onto tracked rows by
    // folder so drifted installs get the Restore action.
    const loadDrift = async (): Promise<void> => {
      scanErr = null;
      try {
        const scan = await service.Scan();
        drifted = new Set(
          scan.addons.filter((a) => a.drifted).map((a) => a.folder_name),
        );
      } catch (err) {
        scanErr = errText(err, "Scan unavailable — Restore hidden for drifted addons");
      }
      rerender();
    };

    const reloadAll = async (): Promise<void> => {
      await Promise.all([check(), loadTracked(), loadDrift()]);
    };

    // --- apply flows ------------------------------------------------------

    const recordBatch = (res: ApplyBatch): void => {
      for (const a of res.applied) {
        if (a.ok) failures.delete(a.folder);
        else failures.set(a.folder, a.error || a.message || "Update failed");
      }
    };

    // The toast is a count summary only; per-row error states and the
    // end-of-batch dump carry the details (never a blanket success).
    const toastBatch = (res: ApplyBatch, doneTitle: string): void => {
      const failed = res.applied.filter((a) => !a.ok);
      toast({
        type: failed.length > 0 ? (res.applied_count > 0 ? "warn" : "error") : "ok",
        title: failed.length > 0 ? `${failed.length} update${failed.length === 1 ? "" : "s"} failed` : doneTitle,
        message: `${res.applied_count} updated · ${res.failed_count} failed`,
      });
    };

    const applyOne = async (u: UpdateEntry): Promise<void> => {
      if (checking || applying) return;
      if (u.flavor_mismatch) {
        const confirmed = await confirmDialog({
          title: `Update “${u.title}” anyway?`,
          message:
            "This addon targets a different game version. Applying it may not work on your current profile.",
          details: [u.flavor_label],
          confirmLabel: "Update Anyway",
        });
        if (!confirmed) return;
      }
      applying = u.folder;
      rerender();
      try {
        const res = await service.ApplyUpdate(u.folder, true);
        recordBatch(res);
        toastBatch(res, `Updated ${u.title}`);
      } catch (err) {
        failures.set(u.folder, errText(err));
        toast({ type: "error", title: `Update failed for ${u.title}`, message: errText(err) });
      } finally {
        applying = null;
        await reloadAll();
      }
    };

    const applyAll = async (): Promise<void> => {
      if (!result || checking || applying) return;
      const updates = result.updates;
      const mismatched = updates.filter((u) => u.flavor_mismatch);
      if (mismatched.length > 0) {
        const confirmed = await confirmDialog({
          title: `Update all ${updates.length} addons?`,
          message: `${mismatched.length} update${mismatched.length === 1 ? " is" : "s are"} for a different game version. Apply anyway?`,
          details: mismatched.map((u) => `${u.title}: ${u.flavor_label}`),
          confirmLabel: "Update All Anyway",
        });
        if (!confirmed) return;
      }
      applying = "all";
      rerender();
      try {
        const res = await service.ApplyAllUpdates(true);
        recordBatch(res);
        toastBatch(res, "Update all complete");
      } catch (err) {
        toast({ type: "error", title: "Update all failed", message: errText(err) });
      } finally {
        applying = null;
        await reloadAll();
      }
    };

    // --- managed-table actions -------------------------------------------

    const setPinned = async (t: TrackedAddon, pinned: boolean): Promise<void> => {
      if (mutating) return;
      mutating = t.folder;
      rerender();
      try {
        await service.SetAddonPinned(t.folder, pinned);
        toast({
          type: "ok",
          title: pinned ? `Pinned ${t.title}` : `Unpinned ${t.title}`,
          message: pinned
            ? "Update checks for this addon pause until unpinned."
            : "This addon will be checked for updates again.",
        });
      } catch (err) {
        toast({ type: "error", title: "Could not change pin", message: errText(err) });
      } finally {
        mutating = null;
        await reloadAll();
      }
    };

    const setIgnored = async (t: TrackedAddon, ignored: boolean): Promise<void> => {
      if (mutating) return;
      mutating = t.folder;
      rerender();
      try {
        await service.SetAddonIgnored(t.folder, ignored);
        toast({
          type: "ok",
          title: ignored ? `Ignored ${t.title}` : `${t.title} tracked again`,
          message: ignored
            ? "This addon is excluded from update management."
            : "This addon is included in update management again.",
        });
      } catch (err) {
        toast({ type: "error", title: "Could not change ignore state", message: errText(err) });
      } finally {
        mutating = null;
        await reloadAll();
      }
    };

    const restore = async (t: TrackedAddon): Promise<void> => {
      if (mutating) return;
      const confirmed = await confirmDialog({
        title: `Restore ${t.title}?`,
        message: `Re-downloads ${t.folder} from its recorded source and replaces the folder. A backup snapshot of the current state is taken first.`,
        confirmLabel: "Restore",
      });
      if (!confirmed) return;
      mutating = t.folder;
      rerender();
      try {
        const res = await service.RestoreAddon(t.folder, true);
        if (res.errors.length > 0) {
          toast({ type: "error", title: `Restore failed for ${t.title}`, message: res.errors.join("\n") });
        } else {
          toast({ type: "ok", title: `Restored ${t.title}`, message: "Re-downloaded from the recorded source; the previous state was backed up." });
        }
      } catch (err) {
        toast({ type: "error", title: `Restore failed for ${t.title}`, message: errText(err) });
      } finally {
        mutating = null;
        await reloadAll();
      }
    };

    // --- version history modal -------------------------------------------

    const openHistory = (t: TrackedAddon, trigger: HTMLElement | null): void => {
      const rootEl = document.createElement("div");
      rootEl.className = "upd-modal-root";
      const id = `upd-modal-title-${++modalSeq}`;
      const backdrop = document.createElement("div");
      backdrop.className = "upd-modal-backdrop";
      const modal = document.createElement("div");
      modal.className = "upd-modal";
      modal.setAttribute("role", "dialog");
      modal.setAttribute("aria-modal", "true");
      modal.setAttribute("aria-labelledby", id);
      modal.innerHTML = `
        <div class="upd-modal-head">
          <span class="upd-modal-title" id="${id}">Version history · ${escapeHtml(t.title)}</span>
          <button class="icon-btn" data-upd-modal-close aria-label="Close version history">${icon("x", 16)}</button>
        </div>
        <div class="upd-modal-body" data-upd-modal-body>
          <div class="upd-loading"><span class="upd-spinner"></span><span>Loading version history…</span></div>
        </div>
        <div class="upd-modal-foot">Rolling back re-downloads the exact version from the provider; the current state is backed up first.</div>`;
      backdrop.appendChild(modal);
      rootEl.appendChild(backdrop);
      document.body.appendChild(rootEl);

      let settled = false;
      const removeCleanup = (): void => {
        const i = modalCleanups.indexOf(close);
        if (i >= 0) modalCleanups.splice(i, 1);
      };
      const close = (): void => {
        if (settled) return;
        settled = true;
        document.removeEventListener("keydown", onKey);
        rootEl.remove();
        removeCleanup();
        if (trigger && trigger.isConnected) trigger.focus();
      };

      // Focus trap: Esc closes, Tab cycles within the modal.
      function onKey(e: KeyboardEvent): void {
        if (e.key === "Escape") {
          e.stopPropagation();
          close();
          return;
        }
        if (e.key === "Tab") {
          const focusables = Array.from(
            modal.querySelectorAll<HTMLElement>(
              'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
            ),
          ).filter((el) => !el.hasAttribute("disabled"));
          if (focusables.length === 0) {
            e.preventDefault();
            modal.focus();
            return;
          }
          const first = focusables[0];
          const last = focusables[focusables.length - 1];
          const active = document.activeElement;
          if (e.shiftKey) {
            if (active === first || !(active instanceof Node && modal.contains(active))) {
              e.preventDefault();
              last.focus();
            }
          } else if (active === last || !(active instanceof Node && modal.contains(active))) {
            e.preventDefault();
            first.focus();
          }
        }
      }

      backdrop.addEventListener("click", (e) => {
        if (e.target === backdrop) close();
      });
      modal.querySelector("[data-upd-modal-close]")!.addEventListener("click", close);
      document.addEventListener("keydown", onKey);
      modalCleanups.push(close);
      window.setTimeout(() => {
        modal.querySelector<HTMLButtonElement>("[data-upd-modal-close]")?.focus();
      }, 0);

      const body = modal.querySelector<HTMLElement>("[data-upd-modal-body]") ?? modal;
      let rollbacking = "";

      const renderHistory = (res: VersionHistoryResult, error?: string): void => {
        const rows = res.versions
          .map((v: VersionEntry) => {
            const isCurrent = v.version === res.current;
            const side = isCurrent
              ? `<span class="upd-modal-current">${icon("check", 12)}<span>Current</span></span>`
              : `<button class="btn-secondary upd-row-btn" data-upd-modal-rollback="${escapeAttr(v.version)}" ${rollbacking ? "disabled" : ""}>
                  ${rollbacking === v.version ? `<span class="upd-spinner"></span>` : icon("download", 12)}
                  <span>Rollback</span>
                </button>`;
            return `
              <div class="upd-modal-row${isCurrent ? " is-current" : ""}">
                <div class="upd-modal-row-main">
                  <span class="upd-modal-version mono tnum"${v.ref ? ` title="${escapeAttr(v.ref)}"` : ""}>${escapeHtml(v.version || "n/a")}</span>
                  <span class="upd-modal-provider">${escapeHtml(PROVIDER_LABEL[v.provider ?? ""] ?? v.provider ?? "")}</span>
                  <span class="upd-modal-date">${formatDateTime(v.at)}</span>
                </div>
                <div class="upd-modal-side">${side}</div>
              </div>`;
          })
          .join("");
        body.innerHTML = `
          ${error ? `<div class="upd-error-box" role="alert">${icon("alert", 15)}<span>${escapeHtml(error)}</span></div>` : ""}
          ${rows || `<div class="upd-modal-empty">No version history recorded for this addon yet.</div>`}`;
        body.querySelectorAll<HTMLButtonElement>("[data-upd-modal-rollback]").forEach((btn) => {
          btn.addEventListener("click", () => {
            const version = btn.dataset.updModalRollback ?? "";
            void rollbackTo(version, res);
          });
        });
      };

      const rollbackTo = async (version: string, res: VersionHistoryResult): Promise<void> => {
        const confirmed = await confirmDialog({
          title: `Roll back ${t.title} to ${version}?`,
          message:
            "Re-downloads this exact version from the provider and replaces the folder. A backup snapshot of the current state is taken first.",
          confirmLabel: "Roll Back",
          danger: true,
        });
        if (!confirmed) return;
        rollbacking = version;
        renderHistory(res);
        try {
          const out = await service.RollbackToVersion(t.folder, version);
          if (out.errors.length > 0) {
            renderHistory(res, out.errors.join("\n"));
            return;
          }
          toast({
            type: "ok",
            title: `Rolled back ${t.title}`,
            message: `Re-downloaded ${version} from the provider; a backup snapshot was taken first.`,
          });
          close();
          void reloadAll();
        } catch (err) {
          rollbacking = "";
          renderHistory(res, errText(err));
        }
      };

      service
        .ListAddonVersions(t.folder)
        .then((res) => renderHistory(res))
        .catch((err) => {
          body.innerHTML = `<div class="upd-error-box" role="alert">${icon("alert", 15)}<span>${escapeHtml(errText(err))}</span></div>`;
        });
    };

    // --- offline snapshot ------------------------------------------------

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
      const okToast = {
        type: "ok" as const,
        title: "Snapshot copied",
        message: "Paste it into the check box, or save it for later.",
      };
      try {
        await navigator.clipboard.writeText(text);
        toast(okToast);
      } catch {
        // Clipboard API unavailable (non-secure context / older webview) —
        // fall back to select-and-execCommand on the read-only textarea.
        const ta = host.querySelector<HTMLTextAreaElement>("[data-snapshot-json]");
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

    // --- focus preservation ----------------------------------------------

    const focusKeyOf = (el: HTMLElement): string | null => {
      if (el.closest("[data-check]")) return "check";
      if (el.closest("[data-apply-all]")) return "apply-all";
      const one = el.closest<HTMLElement>("[data-apply-one]");
      if (one) return `apply-one:${one.dataset.applyOne}`;
      const filter = el.closest<HTMLElement>("[data-managed-filter]");
      if (filter) return `managed-filter:${filter.dataset.managedFilter}`;
      for (const key of ["managed-pin", "managed-ignore", "managed-history", "managed-restore"] as const) {
        const t = el.closest<HTMLElement>(`[data-${key}]`);
        if (t) return `${key}:${t.dataset[key]}`;
      }
      if (el.closest("[data-snapshot-export]")) return "snapshot-export";
      if (el.closest("[data-snapshot-copy]")) return "snapshot-copy";
      if (el.closest("[data-snapshot-check-input]")) return "snapshot-input";
      if (el.closest("[data-snapshot-check]")) return "snapshot-check";
      return null;
    };

    const restoreFocus = (key: string | null): boolean => {
      if (!key) return false;
      const [kind, id] = key.split(":");
      let target: HTMLElement | null = null;
      if (kind === "check") target = host.querySelector<HTMLElement>("[data-check]");
      else if (kind === "apply-all") target = host.querySelector<HTMLElement>("[data-apply-all]");
      else if (kind === "apply-one") target = host.querySelector<HTMLElement>(`[data-apply-one="${id}"]`);
      else if (kind === "managed-filter") target = host.querySelector<HTMLElement>(`[data-managed-filter="${id}"]`);
      else if (kind === "managed-pin") target = host.querySelector<HTMLElement>(`[data-managed-pin="${id}"]`);
      else if (kind === "managed-ignore") target = host.querySelector<HTMLElement>(`[data-managed-ignore="${id}"]`);
      else if (kind === "managed-history") target = host.querySelector<HTMLElement>(`[data-managed-history="${id}"]`);
      else if (kind === "managed-restore") target = host.querySelector<HTMLElement>(`[data-managed-restore="${id}"]`);
      else if (kind === "snapshot-export") target = host.querySelector<HTMLElement>("[data-snapshot-export]");
      else if (kind === "snapshot-copy") target = host.querySelector<HTMLElement>("[data-snapshot-copy]");
      else if (kind === "snapshot-input") target = host.querySelector<HTMLElement>("[data-snapshot-check-input]");
      else if (kind === "snapshot-check") target = host.querySelector<HTMLElement>("[data-snapshot-check]");
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
      else if (!checking && applying === null && mutating === null && !exporting && !checkingSnapshot) {
        pendingFocus = null;
      }
    };

    // --- rendering --------------------------------------------------------

    const emptyState = (glyph: IconName, title: string, body: string, cta: string): string => `
      <div class="empty-state">
        <span class="upd-empty-icon">${icon(glyph, 30)}</span>
        <h2 class="empty-title">${title}</h2>
        <p class="empty-body">${body}</p>
        ${cta ? `<div class="upd-empty-actions">${cta}</div>` : ""}
      </div>`;

    const providerChip = (provider: Provider): string => {
      const label = PROVIDER_LABEL[provider] ?? provider;
      return `<span class="upd-provider-chip" title="${escapeAttr(provider)}">${escapeHtml(label)}</span>`;
    };

    const statusChips = (u: UpdateEntry): string => {
      const failed = failures.get(u.folder);
      if (failed) {
        return `<span class="upd-chip upd-chip-error" role="status">${icon("x-circle", 12)}<span>failed</span></span>`;
      }
      if (u.flavor_mismatch) {
        return `<span class="upd-chip upd-chip-warn" role="status" title="Different game version — confirm before applying">${icon("alert", 12)}<span>${escapeHtml(u.flavor_label || "flavor mismatch")}</span></span>`;
      }
      return `<span class="upd-chip upd-chip-ok" role="status">${icon("check-circle", 12)}<span>ready</span></span>`;
    };

    const renderPendingRow = (u: UpdateEntry): string => {
      const failed = failures.get(u.folder);
      const busy = applying !== null || checking;
      const rowBusy = applying === u.folder;
      return `
      <div class="upd-row${failed ? " has-error" : ""}">
        <div class="upd-row-info">
          <div class="upd-row-name-line">
            <span class="upd-row-name">${escapeHtml(u.title)}</span>
            <span class="upd-row-folder mono">${escapeHtml(u.folder)}</span>
          </div>
          <div class="upd-row-chips">${statusChips(u)}</div>
          ${failed ? `<div class="upd-row-error" role="alert">${icon("x-circle", 13)}<span>${escapeHtml(failed)}</span></div>` : ""}
        </div>
        <div class="upd-row-versions mono tnum" aria-label="Version change">
          <span class="upd-ver-cur">${escapeHtml(u.current_version || "n/a")}</span>
          <span class="upd-arrow">${icon("chevron-right", 12)}</span>
          <span class="upd-ver-latest">${escapeHtml(u.latest_version || "n/a")}</span>
        </div>
        <div class="upd-provider">${providerChip(u.provider)}</div>
        <div class="upd-row-action">
          <button class="btn-primary upd-row-btn" data-apply-one="${escapeAttr(u.folder)}" ${busy ? "disabled" : ""}>
            ${rowBusy ? `<span class="upd-spinner"></span>` : icon("download", 13)}
            <span>${rowBusy ? "Updating…" : "Update"}</span>
          </button>
        </div>
      </div>`;
    };

    // Read-only variant of the pending row for the offline snapshot check.
    const renderSnapshotRow = (u: UpdateEntry): string => `
      <div class="upd-row">
        <div class="upd-row-info">
          <div class="upd-row-name-line">
            <span class="upd-row-name">${escapeHtml(u.title)}</span>
            <span class="upd-row-folder mono">${escapeHtml(u.folder)}</span>
          </div>
          <div class="upd-row-chips">${u.flavor_mismatch ? `<span class="upd-chip upd-chip-warn" title="Different game version">${icon("alert", 12)}<span>${escapeHtml(u.flavor_label || "flavor mismatch")}</span></span>` : ""}</div>
        </div>
        <div class="upd-row-versions mono tnum" aria-label="Version change">
          <span class="upd-ver-cur">${escapeHtml(u.current_version || "n/a")}</span>
          <span class="upd-arrow">${icon("chevron-right", 12)}</span>
          <span class="upd-ver-latest">${escapeHtml(u.latest_version || "n/a")}</span>
        </div>
        <div class="upd-provider">${providerChip(u.provider)}</div>
        <div class="upd-row-action"></div>
      </div>`;

    const renderManagedRow = (t: TrackedAddon): string => {
      const busy = mutating === t.folder;
      const driftedRow = drifted.has(t.folder);
      const stateChips =
        (t.pinned
          ? `<span class="upd-chip upd-chip-state" title="Pinned — locked at the current version, skipped by update checks">${icon("pin", 11)}<span>Pinned</span></span>`
          : "") +
        (t.ignored
          ? `<span class="upd-chip upd-chip-state" title="Ignored — excluded from update management">Ignored</span>`
          : "") +
        (driftedRow
          ? `<span class="upd-chip upd-chip-warn" title="Folder no longer matches the recorded install — restore a clean copy">${icon("alert", 11)}<span>drifted</span></span>`
          : "");
      const historyDisabled = !t.has_history;
      return `
      <div class="upd-managed-row">
        <div class="upd-managed-info">
          <div class="upd-managed-name-line">
            <span class="upd-managed-name">${escapeHtml(t.title)}</span>
            ${stateChips}
          </div>
          <span class="upd-managed-folder mono">${escapeHtml(t.folder)}</span>
        </div>
        <div class="upd-managed-version mono tnum">${escapeHtml(t.version || "n/a")}</div>
        <div class="upd-managed-provider">${providerChip(t.provider)}</div>
        <div class="upd-managed-actions">
          <button class="btn-secondary upd-row-btn" data-managed-pin="${escapeAttr(t.folder)}" ${busy ? "disabled" : ""}
            title="${t.pinned ? "Unlock and resume update checks" : "Lock at the current version"}"
            aria-label="${t.pinned ? "Unpin" : "Pin"} ${escapeAttr(t.title)}">
            ${icon("pin", 12)}<span>${t.pinned ? "Unpin" : "Pin"}</span>
          </button>
          <button class="btn-secondary upd-row-btn" data-managed-ignore="${escapeAttr(t.folder)}" ${busy ? "disabled" : ""}
            title="${t.ignored ? "Include in update management again" : "Exclude from update management"}"
            aria-label="${t.ignored ? "Track" : "Ignore"} ${escapeAttr(t.title)}">
            <span>${t.ignored ? "Track" : "Ignore"}</span>
          </button>
          <button class="btn-secondary upd-row-btn" data-managed-history="${escapeAttr(t.folder)}" ${busy || historyDisabled ? "disabled" : ""}
            title="${historyDisabled ? "No version history recorded" : "Open version history"}"
            aria-label="Version history for ${escapeAttr(t.title)}">
            ${icon("stack", 12)}<span>History</span>
          </button>
          ${driftedRow
            ? `<button class="btn-secondary upd-row-btn" data-managed-restore="${escapeAttr(t.folder)}" ${busy ? "disabled" : ""}
                title="Re-download ${escapeAttr(t.folder)} from its recorded source (backup first)"
                aria-label="Restore ${escapeAttr(t.title)}">
                ${icon("download", 12)}<span>Restore</span>
              </button>`
            : ""}
        </div>
      </div>`;
    };

    const dumpHtml = (): string => {
      if (failures.size === 0) return "";
      const items = [...failures.entries()]
        .map(
          ([folder, err]) =>
            `<li><span class="mono">${escapeHtml(folder)}</span><span>${escapeHtml(err)}</span></li>`,
        )
        .join("");
      return `
        <div class="upd-dump" role="alert">
          <div class="upd-dump-head">${icon("x-circle", 14)}<span>${failures.size} update${failures.size === 1 ? "" : "s"} failed — nothing was silently skipped</span></div>
          <ul>${items}</ul>
        </div>`;
    };

    const toolbarHtml = (): string => {
      const updates = result?.updates ?? [];
      const busy = checking || applying !== null;
      const mismatched = updates.filter((u) => u.flavor_mismatch).length;
      const meta: string[] = [];
      if (result) {
        if (mismatched) meta.push(`${mismatched} flavor mismatch${mismatched === 1 ? "" : "es"}`);
        if (result.errors.length) meta.push(`${result.errors.length} check warning${result.errors.length === 1 ? "" : "s"}`);
        const checked = formatCheckedAt(result.checked_at);
        if (checked) meta.push(`checked ${checked}`);
      }
      return `
        <div class="upd-toolbar">
          <div class="upd-toolbar-actions">
            <button class="btn-primary" data-apply-all ${busy || updates.length === 0 ? "disabled" : ""}>
              ${applying === "all" ? `<span class="upd-spinner"></span>` : icon("download", 15)}
              <span>${applying === "all" ? "Updating…" : "Update all"}</span>
              ${applying !== "all" ? `<span class="upd-count badge" aria-label="${updates.length} update${updates.length === 1 ? "" : "s"} available">${updates.length}</span>` : ""}
            </button>
            <button class="btn-secondary" data-check ${busy ? "disabled" : ""}>
              ${checking ? `<span class="upd-spinner"></span>` : icon("refresh", 15)}
              <span>${checking ? "Checking…" : "Check for updates"}</span>
            </button>
          </div>
          ${meta.length ? `<span class="upd-meta">${meta.join(" · ")}</span>` : ""}
        </div>`;
    };

    // Provider outage surfacing: check errors are shown, never suppressed.
    const checkErrorsHtml = (): string => {
      if (!result || result.errors.length === 0) return "";
      return `
        <div class="upd-check-errors" role="alert">
          <div class="upd-check-errors-head">${icon("alert", 14)}<span>${result.errors.length} problem${result.errors.length === 1 ? "" : "s"} while checking — some providers did not respond</span></div>
          <ul>${result.errors.map((e) => `<li>${escapeHtml(e)}</li>`).join("")}</ul>
        </div>`;
    };

    const pendingSectionHtml = (): string => {
      if (!result) {
        return `<section class="upd-section"><div class="upd-loading"><span class="upd-spinner"></span><span>${checking ? "Checking for updates…" : "Loading…"}</span></div></section>`;
      }
      const updates = result.updates;
      if (updates.length === 0) {
        return `<section class="upd-section">${emptyState(
          "check-circle",
          "All addons are up to date",
          "Every tracked addon matches its latest provider release.",
          `<button class="btn-secondary" data-check>${icon("refresh", 15)}<span>Check again</span></button>`,
        )}</section>`;
      }
      // Needs-update rows first; flavor-mismatch rows sink to the bottom so
      // the straightforward updates read first.
      const pendingRows = [...updates].sort(
        (a, b) => Number(a.flavor_mismatch) - Number(b.flavor_mismatch),
      );
      const mismatched = updates.filter((u) => u.flavor_mismatch).length;
      return `
        <section class="upd-section" aria-label="Pending updates">
          <div class="upd-section-head">
            <div class="upd-section-titles">
              <h2 class="upd-section-title">Pending updates</h2>
              <span class="upd-section-meta">${updates.length} available${mismatched ? ` · ${mismatched} flavor mismatch${mismatched === 1 ? "" : "es"}` : ""}</span>
            </div>
          </div>
          <div class="upd-pending-rows">${pendingRows.map(renderPendingRow).join("")}</div>
          ${dumpHtml()}
        </section>`;
    };

    const managedSectionHtml = (): string => {
      const all = tracked ?? [];
      const visible = all.filter((t) =>
        managedFilter === "all" ? true : managedFilter === "pinned" ? t.pinned : t.ignored,
      );
      const pinnedCount = all.filter((t) => t.pinned).length;
      const ignoredCount = all.filter((t) => t.ignored).length;
      const tabs = MANAGED_FILTERS.map((f) => {
        const count =
          f.value === "all" ? all.length : f.value === "pinned" ? pinnedCount : ignoredCount;
        return `<button class="tab${managedFilter === f.value ? " selected" : ""}" data-managed-filter="${f.value}"
          aria-pressed="${managedFilter === f.value}">${f.label}<span class="upd-tab-count">${count}</span></button>`;
      }).join("");
      let body: string;
      if (trackedErr) {
        body = `<div class="upd-error-box" role="alert">${icon("alert", 14)}<span>${escapeHtml(trackedErr)}</span></div>`;
      } else if (tracked === null) {
        body = `<div class="upd-loading"><span class="upd-spinner"></span><span>Loading tracked addons…</span></div>`;
      } else if (all.length === 0) {
        body = `<div class="upd-section-empty">${icon("stack", 16)}<span>No tracked addons yet — install from the Catalog and they'll be managed here.</span></div>`;
      } else {
        body = `<div class="upd-managed-rows">${visible.map(renderManagedRow).join("")}</div>`;
      }
      return `
        <section class="upd-section" aria-label="Tracked addons">
          <div class="upd-section-head">
            <div class="upd-section-titles">
              <h2 class="upd-section-title">Managed</h2>
              <span class="upd-section-meta">${all.length} tracked · ${pinnedCount} pinned · ${ignoredCount} ignored</span>
            </div>
            <div class="tabs" role="group" aria-label="Filter tracked addons">${tabs}</div>
          </div>
          ${scanErr ? `<div class="upd-scan-note" role="note">${icon("alert", 13)}<span>${escapeHtml(scanErr)}</span></div>` : ""}
          ${body}
        </section>`;
    };

    const snapshotCheckResultHtml = (): string => {
      if (checkingSnapshot) return `<div class="upd-loading"><span class="upd-spinner"></span><span>Diffing snapshot…</span></div>`;
      if (snapshotCheckErr) return `<div class="upd-error-box" role="alert">${icon("alert", 14)}<span>${escapeHtml(snapshotCheckErr)}</span></div>`;
      if (!snapshotCheck) return "";
      const ups = snapshotCheck.updates;
      const errs = snapshotCheck.errors;
      const head = `<div class="upd-snapshot-result-head">
        <span class="upd-chip upd-chip-ok">${icon("check-circle", 12)}<span>${ups.length} update${ups.length === 1 ? "" : "s"} available</span></span>
        ${errs.length ? `<span class="upd-chip upd-chip-warn">${icon("alert", 12)}<span>${errs.length} warning${errs.length === 1 ? "" : "s"}</span></span>` : ""}
      </div>`;
      const errsHtml = errs.length
        ? `<ul class="upd-snapshot-warnings">${errs.map((e) => `<li>${escapeHtml(e)}</li>`).join("")}</ul>`
        : "";
      const rows =
        ups.length === 0
          ? `<p class="upd-snapshot-clean">${icon("check-circle", 14)}<span>Registry matches this snapshot — nothing to update offline.</span></p>`
          : `<div class="upd-pending-rows">${ups.map(renderSnapshotRow).join("")}</div>`;
      return `${head}${errsHtml}${rows}`;
    };

    const snapshotSectionHtml = (): string => {
      const summary = snapshot
        ? `${snapshot.addon_count} addon${snapshot.addon_count === 1 ? "" : "s"} · exported ${formatDateTime(snapshot.exported_at)}`
        : "";
      const warnings =
        snapshot && snapshot.warnings.length > 0
          ? `<ul class="upd-snapshot-warnings">${snapshot.warnings.map((w) => `<li>${escapeHtml(w)}</li>`).join("")}</ul>`
          : "";
      return `
        <section class="upd-section" aria-label="Offline snapshots">
          <div class="spotlight-card -violet upd-snapshot-spotlight">
            <span class="spotlight-kicker">The unclaimed differentiator</span>
            <span class="spotlight-title">Offline snapshots</span>
            <span class="spotlight-body">Every other manager dies with its web scrapers. Freeze the tracked registry to portable JSON, then diff any snapshot against the current install — no network, no provider, no rate limits.</span>
          </div>
          <div class="card-featured upd-snapshot-panel">
            <div class="upd-snapshot-cols">
              <div class="upd-snapshot-col">
                <h3 class="upd-snapshot-col-title">Export</h3>
                <p class="upd-snapshot-hint">Freeze tracked addons and their latest known versions into portable JSON for an offline check.</p>
                <div class="upd-snapshot-actions">
                  <button class="btn-primary upd-row-btn" data-snapshot-export ${exporting ? "disabled" : ""}>
                    ${exporting ? `<span class="upd-spinner"></span>` : icon("archive", 13)}
                    <span>${exporting ? "Exporting…" : "Export snapshot"}</span>
                  </button>
                  ${snapshot ? `<button class="btn-secondary upd-row-btn" data-snapshot-copy>${icon("save", 13)}<span>Copy</span></button>` : ""}
                </div>
                ${summary ? `<p class="upd-snapshot-summary">${summary}</p>` : ""}
                ${snapshot ? `<textarea class="upd-snapshot-json mono" data-snapshot-json readonly rows="10" spellcheck="false" aria-label="Exported snapshot JSON">${escapeHtml(snapshot.snapshot_json)}</textarea>` : ""}
                ${warnings}
              </div>
              <div class="upd-snapshot-col">
                <h3 class="upd-snapshot-col-title">Check</h3>
                <p class="upd-snapshot-hint">Paste a snapshot and diff it against the current registry. No providers are queried — the diff is fully offline.</p>
                <textarea class="upd-snapshot-json mono" data-snapshot-check-input rows="7" spellcheck="false" aria-label="Snapshot JSON to check" placeholder="Paste snapshot JSON here…">${escapeHtml(snapshotInput)}</textarea>
                <div class="upd-snapshot-actions">
                  <button class="btn-primary upd-row-btn" data-snapshot-check ${checkingSnapshot ? "disabled" : ""}>
                    ${checkingSnapshot ? `<span class="upd-spinner"></span>` : icon("search", 13)}
                    <span>${checkingSnapshot ? "Checking…" : "Check snapshot"}</span>
                  </button>
                </div>
                ${snapshotCheckResultHtml()}
              </div>
            </div>
          </div>
        </section>`;
    };

    const render = (): void => {
      if (bootErr) {
        host.innerHTML = emptyState(
          "x-circle",
          "Could not load updates",
          bootErr,
          "",
        );
        return;
      }
      if (!install) {
        host.innerHTML = emptyState(
          "folder",
          "No WoW install configured",
          "Set up your World of Warcraft path in Setup before checking for updates.",
          "",
        );
        return;
      }
      host.innerHTML = `
        <div class="view-page updates">
          <div class="view-hero">
            <h1 class="view-title">Updates</h1>
            <p class="view-sub">Check once, review the diff, apply — then pin, ignore, roll back or restore. Offline snapshots keep it honest when the network is gone.</p>
          </div>
          ${toolbarHtml()}
          ${checkErrorsHtml()}
          ${pendingSectionHtml()}
          ${managedSectionHtml()}
          ${snapshotSectionHtml()}
        </div>`;
      bindHandlers();
    };

    // --- event wiring -----------------------------------------------------

    const bindHandlers = (): void => {
      host.querySelector("[data-check]")?.addEventListener("click", () => {
        pendingFocus = "check";
        void check();
      });
      host.querySelector("[data-apply-all]")?.addEventListener("click", () => {
        pendingFocus = "apply-all";
        void applyAll();
      });
      host.querySelectorAll<HTMLElement>("[data-apply-one]").forEach((btn) => {
        const folder = btn.dataset.applyOne ?? "";
        btn.addEventListener("click", () => {
          const u = result?.updates.find((x) => x.folder === folder);
          if (u) {
            pendingFocus = `apply-one:${folder}`;
            void applyOne(u);
          }
        });
      });
      host.querySelectorAll<HTMLElement>("[data-managed-filter]").forEach((chip) => {
        chip.addEventListener("click", () => {
          managedFilter = (chip.dataset.managedFilter ?? "all") as ManagedFilter;
          rerender();
        });
      });
      host.querySelectorAll<HTMLElement>("[data-managed-pin]").forEach((btn) => {
        const folder = btn.dataset.managedPin ?? "";
        const t = tracked?.find((x) => x.folder === folder);
        if (t) {
          btn.addEventListener("click", () => {
            pendingFocus = `managed-pin:${folder}`;
            void setPinned(t, !t.pinned);
          });
        }
      });
      host.querySelectorAll<HTMLElement>("[data-managed-ignore]").forEach((btn) => {
        const folder = btn.dataset.managedIgnore ?? "";
        const t = tracked?.find((x) => x.folder === folder);
        if (t) {
          btn.addEventListener("click", () => {
            pendingFocus = `managed-ignore:${folder}`;
            void setIgnored(t, !t.ignored);
          });
        }
      });
      host.querySelectorAll<HTMLElement>("[data-managed-history]").forEach((btn) => {
        const folder = btn.dataset.managedHistory ?? "";
        const t = tracked?.find((x) => x.folder === folder);
        if (t) {
          btn.addEventListener("click", () => {
            pendingFocus = `managed-history:${folder}`;
            openHistory(t, btn);
          });
        }
      });
      host.querySelectorAll<HTMLElement>("[data-managed-restore]").forEach((btn) => {
        const folder = btn.dataset.managedRestore ?? "";
        const t = tracked?.find((x) => x.folder === folder);
        if (t) {
          btn.addEventListener("click", () => {
            pendingFocus = `managed-restore:${folder}`;
            void restore(t);
          });
        }
      });
      host.querySelector("[data-snapshot-export]")?.addEventListener("click", () => {
        pendingFocus = "snapshot-export";
        void exportSnapshot();
      });
      host.querySelector("[data-snapshot-copy]")?.addEventListener("click", () => {
        pendingFocus = "snapshot-copy";
        void copySnapshot();
      });
      host.querySelector<HTMLTextAreaElement>("[data-snapshot-check-input]")?.addEventListener("input", (e) => {
        snapshotInput = (e.target as HTMLTextAreaElement).value;
      });
      host.querySelector("[data-snapshot-check]")?.addEventListener("click", () => {
        pendingFocus = "snapshot-check";
        void checkSnapshot();
      });
    };

    // --- boot -------------------------------------------------------------

    host.innerHTML = `<div class="upd-loading"><span class="upd-spinner"></span><span>Loading updates…</span></div>`;

    const boot = async (): Promise<void> => {
      try {
        const st = await service.GetState();
        install = st.has_install;
      } catch (err) {
        bootErr = errText(err, "Could not load app state");
        render();
        return;
      }
      render();
      if (!install) return;
      await reloadAll();
    };
    void boot();
  },
  unmount() {
    while (modalCleanups.length) modalCleanups.pop()?.();
  },
};

function formatCheckedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const now = new Date();
  return d.toDateString() === now.toDateString()
    ? d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : `${d.toLocaleDateString([], { month: "short", day: "numeric" })} ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
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
