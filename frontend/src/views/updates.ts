// Updates: dry-run check of tracked addons against their providers. The
// list IS the preview — nothing downloads until the user clicks Update /
// Update all. Flavor mismatches are surfaced with an amber badge and a
// confirm dialog before applying, but never block the list.

import type { AppState, Actions } from "../app";
import type {
  UpdateEntry,
  CheckUpdatesResult,
  ApplyBatch,
  TrackedAddon,
  SnapshotResult,
  SnapshotCheck,
} from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast, type ToastOpts } from "../components/toast";
import { confirmDialog } from "../components/dialog";

const PROVIDER_LABEL: Record<string, string> = {
  github: "GH",
  curseforge: "CF",
  wowinterface: "WoWI",
  tukui: "Tukui",
};

type ManagedFilter = "all" | "pinned" | "ignored";

const MANAGED_FILTERS: { value: ManagedFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "pinned", label: "Pinned" },
  { value: "ignored", label: "Ignored" },
];

// Textareas have no stylesheet rules in this design system; the snapshot
// boxes are themed through the tokens contract inline (canvas field on the
// charcoal surface, hairline border, mono type).
const JSON_TEXTAREA_STYLE =
  "width:100%; box-sizing:border-box; resize:vertical; min-height:130px; " +
  "background:var(--canvas); color:var(--ink); border:1px solid var(--hairline); " +
  "border-radius:var(--r-md); padding:10px 12px; font-size:var(--body-sm); line-height:1.5;";

export function mountUpdates(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let result: CheckUpdatesResult | null = null;
  let checking = false;
  let applying: string | null = null; // folder being updated, or "all"
  const failures = new Map<string, string>(); // folder -> error text
  let tracked: TrackedAddon[] | null = null;
  let trackedErr: string | null = null;
  let managedFilter: ManagedFilter = "all";
  let mutating: string | null = null; // folder being pinned/ignored/rolled back
  // --- snapshot export/check (CLI `snapshot export|check` parity) ----------
  let snapshotOpen = false;
  let exporting = false;
  let snapshot: SnapshotResult | null = null;
  let snapshotInput = "";
  let checkingSnapshot = false;
  let snapshotCheck: SnapshotCheck | null = null;
  let snapshotCheckErr: string | null = null;

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

  const exportSnapshot = async (): Promise<void> => {
    if (exporting) return;
    exporting = true;
    rerender();
    try {
      snapshot = await service.ExportSnapshot();
      toast({
        type: "ok",
        title: "Snapshot exported",
        message: `${snapshot.addon_count} addon${snapshot.addon_count === 1 ? "" : "s"} frozen — copy it, or save it as a file for later.`,
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
  // Capture the focused control before a re-render and restore focus to its
  // replacement after. pendingFocus survives async flows (apply / pin /
  // ignore / rollback) whose final re-render happens after the backend call,
  // while the control is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (el: HTMLElement): string | null => {
    if (el.closest("[data-check]")) return "check";
    if (el.closest("[data-apply-all]")) return "apply-all";
    const one = el.closest<HTMLElement>("[data-apply-one]");
    if (one) return `apply-one:${one.dataset.applyOne}`;
    const filter = el.closest<HTMLElement>("[data-managed-filter]");
    if (filter) return `managed-filter:${filter.dataset.managedFilter}`;
    const pin = el.closest<HTMLElement>("[data-managed-pin]");
    if (pin) return `managed-pin:${pin.dataset.managedPin}`;
    const ignore = el.closest<HTMLElement>("[data-managed-ignore]");
    if (ignore) return `managed-ignore:${ignore.dataset.managedIgnore}`;
    const rollback = el.closest<HTMLElement>("[data-managed-rollback]");
    if (rollback) return `managed-rollback:${rollback.dataset.managedRollback}`;
    if (el.closest("[data-snapshot-toggle]")) return "snapshot-toggle";
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
    if (kind === "check") target = el.querySelector<HTMLElement>("[data-check]");
    else if (kind === "apply-all") target = el.querySelector<HTMLElement>("[data-apply-all]");
    else if (kind === "apply-one") target = el.querySelector<HTMLElement>(`[data-apply-one="${id}"]`);
    else if (kind === "managed-filter") target = el.querySelector<HTMLElement>(`[data-managed-filter="${id}"]`);
    else if (kind === "managed-pin") target = el.querySelector<HTMLElement>(`[data-managed-pin="${id}"]`);
    else if (kind === "managed-ignore") target = el.querySelector<HTMLElement>(`[data-managed-ignore="${id}"]`);
    else if (kind === "managed-rollback") target = el.querySelector<HTMLElement>(`[data-managed-rollback="${id}"]`);
    else if (kind === "snapshot-toggle") target = el.querySelector<HTMLElement>("[data-snapshot-toggle]");
    else if (kind === "snapshot-export") target = el.querySelector<HTMLElement>("[data-snapshot-export]");
    else if (kind === "snapshot-copy") target = el.querySelector<HTMLElement>("[data-snapshot-copy]");
    else if (kind === "snapshot-input") target = el.querySelector<HTMLElement>("[data-snapshot-check-input]");
    else if (kind === "snapshot-check") target = el.querySelector<HTMLElement>("[data-snapshot-check]");
    if (!target) return false;
    if (target.hasAttribute("disabled")) return false;
    target.focus();
    return true;
  };

  // Render with focus preservation; in-flight work keeps pendingFocus alive
  // so the final render can land it on the (re-enabled) control.
  const rerender = (): void => {
    const active = document.activeElement;
    const key = pendingFocus ?? (active instanceof HTMLElement ? focusKeyOf(active) : null);
    render();
    if (!key) return;
    if (restoreFocus(key)) pendingFocus = null;
    else if (!checking && applying === null && mutating === null && !exporting && !checkingSnapshot) pendingFocus = null;
  };

  const loadTracked = async (): Promise<void> => {
    if (!app.state.has_install) return;
    try {
      tracked = (await service.TrackedAddons()).addons;
      trackedErr = null;
    } catch (err) {
      trackedErr = errText(err, "Could not load tracked addons");
    }
    rerender();
  };

  const reloadAll = async (): Promise<void> => {
    await Promise.all([check(), loadTracked()]);
  };

  const setPinned = async (a: TrackedAddon, pinned: boolean): Promise<void> => {
    if (mutating) return;
    mutating = a.folder;
    rerender();
    try {
      await service.SetAddonPinned(a.folder, pinned);
      toast({
        type: "ok",
        title: pinned ? `Pinned ${a.title}` : `Unpinned ${a.title}`,
        message: pinned
          ? "Update checks for this addon are paused until unpinned."
          : "This addon will be checked for updates again.",
      });
    } catch (err) {
      toast({ type: "error", title: "Could not change pin", message: errText(err) });
    } finally {
      mutating = null;
      await reloadAll();
    }
  };

  const setIgnored = async (a: TrackedAddon, ignored: boolean): Promise<void> => {
    if (mutating) return;
    mutating = a.folder;
    rerender();
    try {
      await service.SetAddonIgnored(a.folder, ignored);
      toast({
        type: "ok",
        title: ignored ? `Ignored ${a.title}` : `${a.title} tracked again`,
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

  const rollback = async (a: TrackedAddon): Promise<void> => {
    if (mutating) return;
    const confirmed = await confirmDialog({
      title: `Roll back ${a.folder}?`,
      message:
        "Restores the folder from the newest backup snapshot and pins it (updates stop until unpinned).",
      confirmLabel: "Roll Back",
    });
    if (!confirmed) return;
    mutating = a.folder;
    rerender();
    try {
      const res = await service.RollbackAddon(a.folder);
      toast({
        type: "ok",
        title: `Rolled back ${res.folder}`,
        message: `Restored from ${res.restored_from}, pinned`,
      });
    } catch (err) {
      toast({
        type: "error",
        title: `Rollback failed for ${a.folder}`,
        message: errText(err),
      });
    } finally {
      mutating = null;
      await reloadAll();
    }
  };

  const toastBatch = (res: ApplyBatch, doneTitle: string): void => {
    const failed = res.applied.filter((a) => !a.ok);
    toast({
      type: failed.length > 0 ? (res.applied_count > 0 ? "warn" : "error") : "ok",
      title: failed.length > 0 ? `${failed.length} update${failed.length === 1 ? "" : "s"} failed` : doneTitle,
      message: `${res.applied_count} updated · ${res.failed_count} failed`,
    });
  };

  const recordBatch = (res: ApplyBatch): void => {
    for (const a of res.applied) {
      if (a.ok) failures.delete(a.folder);
      else failures.set(a.folder, a.error || a.message || "Update failed");
    }
  };

  const applyOne = async (u: UpdateEntry): Promise<void> => {
    if (checking || applying) return;
    if (u.flavor_mismatch) {
      const confirmed = await confirmDialog({
        title: `Update “${u.title}” anyway?`,
        message: `This addon is for a different game version — applying it may not work on your current profile.`,
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
    const mismatched = result.updates.filter((u) => u.flavor_mismatch).length;
    if (mismatched > 0) {
      const confirmed = await confirmDialog({
        title: `Update all ${result.updates.length} addons?`,
        message: `${mismatched} update${mismatched === 1 ? " is" : "s are"} for a different game version — apply anyway?`,
        details: result.updates
          .filter((u) => u.flavor_mismatch)
          .map((u) => `${u.title} — ${u.flavor_label}`),
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

  const render = (): void => {
    if (!app.state.has_install) {
      el.innerHTML = emptyCard(
        "folder",
        "No WoW install configured",
        "Set up your World of Warcraft path before checking for updates.",
        `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
      );
      el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
      return;
    }

    const updates = result?.updates ?? [];
    const errors = result?.errors ?? [];
    const mismatched = updates.filter((u) => u.flavor_mismatch).length;
    const busy = checking || applying !== null;
    const checkedAt = result ? formatCheckedAt(result.checked_at) : "";

    const managedAll = tracked ?? [];
    const managedVisible = managedAll.filter((t) =>
      managedFilter === "all" ? true : managedFilter === "pinned" ? t.pinned : t.ignored,
    );
    const managedPinned = managedAll.filter((t) => t.pinned).length;
    const managedIgnored = managedAll.filter((t) => t.ignored).length;

    el.innerHTML = `
      <div class="updates">
        <div class="updates-toolbar">
          <div class="updates-toolbar-left">
            <button class="btn btn-outline" data-check ${busy ? "disabled" : ""}>
              ${checking ? `<span class="spinner"></span>` : icon("refresh", 15)}
              <span>${checking ? "Checking…" : "Check for updates"}</span>
            </button>
            ${
              result
                ? `<div class="updates-summary" aria-label="Update summary">
                    <span class="count-item"><span class="status-dot ok"></span><span class="count-num">${updates.length}</span> update${updates.length === 1 ? "" : "s"} available</span>
                    <span class="count-item"><span class="status-dot warn"></span><span class="count-num">${mismatched}</span> skipped</span>
                    <span class="count-item ${errors.length ? "" : "muted"}"><span class="status-dot ${errors.length ? "error" : "muted"}"></span><span class="count-num">${errors.length}</span> error${errors.length === 1 ? "" : "s"}</span>
                    ${checkedAt ? `<span class="count-item muted">checked ${checkedAt}</span>` : ""}
                  </div>`
                : ""
            }
          </div>
          <button class="btn btn-primary" data-apply-all ${busy || updates.length === 0 ? "disabled" : ""}>
            ${applying === "all" ? `<span class="spinner"></span>` : icon("download", 15)}
            <span>${applying === "all" ? "Updating…" : `Update all (${updates.length})`}</span>
          </button>
        </div>

        ${
          errors.length
            ? `<div class="error-box" role="alert">
                <span class="error-box-head">${icon("alert", 15)}<span>${errors.length} problem${errors.length === 1 ? "" : "s"} while checking for updates</span></span>
                <ul>${errors.map((e) => `<li>${escapeHtml(e)}</li>`).join("")}</ul>
              </div>`
            : ""
        }

        ${
          !result
            ? `<div class="list-loading">
                <span class="spinner spinner-lg"></span>
                <span>${checking ? "Checking for updates…" : "Loading…"}</span>
              </div>`
            : updates.length === 0
              ? emptyCard(
                  "check-circle",
                  "All addons are up to date",
                  "Every tracked addon matches its latest provider release.",
                  `<button class="btn btn-outline" data-check>${icon("refresh", 16)}<span>Check again</span></button>`,
                )
              : `<div class="update-rows">${updates.map(renderRow).join("")}</div>`
        }

        ${snapshotSectionHtml()}

        ${managedSectionHtml(managedAll, managedVisible, managedPinned, managedIgnored)}
      </div>`;

    el.querySelector("[data-check]")?.addEventListener("click", () => {
      pendingFocus = "check";
      void check();
    });
    el.querySelector("[data-apply-all]")?.addEventListener("click", () => {
      pendingFocus = "apply-all";
      void applyAll();
    });
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
    el.querySelectorAll<HTMLElement>("[data-apply-one]").forEach((btn) => {
      const u = updates[Number(btn.dataset.applyOne)];
      btn.addEventListener("click", () => {
        pendingFocus = `apply-one:${btn.dataset.applyOne}`;
        void applyOne(u);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-managed-filter]").forEach((chip) => {
      chip.addEventListener("click", () => {
        managedFilter = (chip.dataset.managedFilter ?? "all") as ManagedFilter;
        rerender();
      });
    });
    el.querySelectorAll<HTMLElement>("[data-managed-pin]").forEach((btn) => {
      const t = managedVisible[Number(btn.dataset.managedPin)];
      btn.addEventListener("click", () => {
        pendingFocus = `managed-pin:${btn.dataset.managedPin}`;
        void setPinned(t, !t.pinned);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-managed-ignore]").forEach((btn) => {
      const t = managedVisible[Number(btn.dataset.managedIgnore)];
      btn.addEventListener("click", () => {
        pendingFocus = `managed-ignore:${btn.dataset.managedIgnore}`;
        void setIgnored(t, !t.ignored);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-managed-rollback]").forEach((btn) => {
      const t = managedVisible[Number(btn.dataset.managedRollback)];
      btn.addEventListener("click", () => {
        pendingFocus = `managed-rollback:${btn.dataset.managedRollback}`;
        void rollback(t);
      });
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

  const renderRow = (u: UpdateEntry, i: number): string => {
    const failed = failures.get(u.folder);
    const mismatch = u.flavor_mismatch
      ? `<span class="mismatch-badge" title="Different game version — confirm before applying">${icon("alert", 12)}<span>${escapeHtml(u.flavor_label)}</span></span>`
      : "";
    const error = failed
      ? `<div class="row-error" role="alert">${icon("x-circle", 14)}<span>${escapeHtml(failed)}</span></div>`
      : "";
    return `
      <div class="update-row${failed ? " has-error" : ""}${u.flavor_mismatch ? " has-mismatch" : ""}">
        <div class="update-info">
          <div class="update-name-line">
            <span class="update-name">${escapeHtml(u.title)}</span>
            <span class="update-folder mono">${escapeHtml(u.folder)}</span>
          </div>
          ${mismatch}
          ${error}
        </div>
        <div class="update-versions mono" aria-label="Version change">
          <span class="update-ver-cur">${escapeHtml(u.current_version || "—")}</span>
          <span class="update-arrow">${icon("chevron-right", 13)}</span>
          <span class="update-ver-latest">${escapeHtml(u.latest_version || "—")}</span>
        </div>
        <div class="update-provider">${providerChip(u.provider)}</div>
        <div class="update-action">
          <button class="btn btn-primary btn-sm" data-apply-one="${i}" ${checking || applying ? "disabled" : ""}>
            ${applying === u.folder ? `<span class="spinner"></span>` : icon("download", 14)}
            <span>${applying === u.folder ? "Updating…" : "Update"}</span>
          </button>
        </div>
      </div>`;
  };

  const renderManagedRow = (t: TrackedAddon, i: number): string => {
    const state = t.pinned
      ? `<span class="tag tag-pinned" title="Pinned — locked at the current version">${icon("lock", 11)}<span>Pinned</span></span>`
      : t.ignored
        ? `<span class="tag tag-ignored" title="Ignored — excluded from update management">Ignored</span>`
        : "";
    const busy = mutating === t.folder;
    return `
      <div class="managed-row">
        <div class="managed-info">
          <div class="managed-name-line">
            <span class="managed-name">${escapeHtml(t.title)}</span>
            ${state}
          </div>
          <span class="managed-folder mono">${escapeHtml(t.folder)}</span>
        </div>
        <div class="managed-version mono">${escapeHtml(t.version || "—")}</div>
        <div class="managed-provider">${providerChip(t.provider)}</div>
        <div class="managed-actions">
          <button class="btn btn-outline btn-sm" data-managed-pin="${i}" ${busy ? "disabled" : ""}
            title="${t.pinned ? "Unlock and resume update checks" : "Lock at the current version"}"
            aria-label="${t.pinned ? "Unpin" : "Pin"} ${escapeAttr(t.title)}">
            ${icon("lock", 12)}<span>${t.pinned ? "Unpin" : "Pin"}</span>
          </button>
          <button class="btn btn-outline btn-sm" data-managed-ignore="${i}" ${busy ? "disabled" : ""}
            title="${t.ignored ? "Include in update management again" : "Exclude from update management"}"
            aria-label="${t.ignored ? "Track" : "Ignore"} ${escapeAttr(t.title)}">
            ${icon(t.ignored ? "eye" : "eye-off", 12)}<span>${t.ignored ? "Track again" : "Ignore"}</span>
          </button>
          <button class="btn btn-sm btn-restore" data-managed-rollback="${i}" ${busy ? "disabled" : ""}
            title="Restore ${escapeAttr(t.folder)} from the newest backup snapshot">
            ${icon("download", 12)}<span>Rollback</span>
          </button>
        </div>
      </div>`;
  };

  const managedSectionHtml = (
    all: TrackedAddon[],
    visible: TrackedAddon[],
    pinnedCount: number,
    ignoredCount: number,
  ): string => {
    const chips = MANAGED_FILTERS.map((f) => {
      const count =
        f.value === "all"
          ? all.length
          : f.value === "pinned"
            ? pinnedCount
            : ignoredCount;
      return `<button class="chip-btn${managedFilter === f.value ? " active" : ""}" data-managed-filter="${f.value}" aria-pressed="${managedFilter === f.value}">
        ${f.label}<span class="chip-count">${count}</span>
      </button>`;
    }).join("");

    let body: string;
    if (trackedErr) {
      body = `<div class="managed-error" role="alert">${icon("alert", 14)}<span>${escapeHtml(trackedErr)}</span></div>`;
    } else if (tracked === null) {
      body = `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Loading tracked addons…</span></div>`;
    } else if (all.length === 0) {
      body = `<div class="managed-empty">
        <span class="managed-empty-icon">${icon("list", 18)}</span>
        <span>No tracked addons yet — install from the catalog to track addons here.</span>
      </div>`;
    } else {
      body = `<div class="managed-rows">${visible.map((t, i) => renderManagedRow(t, i)).join("")}</div>`;
    }

    return `
      <section class="managed" aria-label="Tracked addons">
        <div class="managed-head">
          <h2 class="managed-title">Managed</h2>
          <span class="managed-summary">${all.length} tracked · ${pinnedCount} pinned · ${ignoredCount} ignored</span>
        </div>
        <div class="managed-chips" role="group" aria-label="Filter managed addons">${chips}</div>
        ${body}
      </section>`;
  };

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
            ? `<span class="mismatch-badge" title="Different game version — confirm before applying">${icon("alert", 12)}<span>${escapeHtml(u.flavor_label)}</span></span>`
            : ""
        }
      </div>
      <div class="update-versions mono" aria-label="Version change">
        <span class="update-ver-cur">${escapeHtml(u.current_version || "—")}</span>
        <span class="update-arrow">${icon("chevron-right", 13)}</span>
        <span class="update-ver-latest">${escapeHtml(u.latest_version || "—")}</span>
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
          ? `<p class="result-hint">Nothing to update — the registry already matches this snapshot.</p>`
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
      <section class="snapshot" aria-label="Snapshot — offline update check">
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
                    <p class="result-hint">Paste a snapshot and diff it against the current registry — no network access.</p>
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
  void check();
  void loadTracked();

  return {
    refresh: () => {
      render();
      void loadTracked();
    },
  };
}

function providerChip(provider: string): string {
  const label = PROVIDER_LABEL[provider] ?? provider;
  return `<span class="provider-chip prov-${escapeAttr(provider)}" title="${escapeAttr(provider)}">${escapeHtml(label)}</span>`;
}

function formatCheckedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
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

function emptyCard(glyph: IconName, title: string, sub: string, cta: string): string {
  return `<div class="empty">
    <span class="empty-icon">${icon(glyph, 28)}</span>
    <h2 class="empty-title">${title}</h2>
    <p class="empty-sub">${sub}</p>
    <div class="empty-actions">${cta}</div>
  </div>`;
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
