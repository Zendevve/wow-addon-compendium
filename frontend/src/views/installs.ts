// Installs: per-WoW-install status cards with a health band, addon stats
// and Set-as-active / Scan actions, plus a cross-install "Update all"
// flow that syncs every detected install in one pass and reports the
// per-install result.

import type { AppState, Actions } from "../app";
import type {
  InstallStatus,
  InstallsStatusResult,
  SyncResult,
} from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
import { confirmDialog } from "../components/dialog";

export function mountInstalls(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let status: InstallsStatusResult | null = null;
  let loading = false;
  let syncing = false;
  let activating: string | null = null; // root being set active
  let syncResult: SyncResult | null = null;

  const load = async (): Promise<void> => {
    loading = true;
    rerender();
    try {
      status = await service.InstallsStatus();
    } catch (err) {
      toast({
        type: "error",
        title: "Could not load installs",
        message: errText(err),
      });
    } finally {
      loading = false;
      rerender();
    }
  };

  // --- focus preservation ------------------------------------------------
  // Re-renders rebuild the view DOM, dropping keyboard focus to <body>.
  // Capture the focused control before a re-render and restore focus to its
  // replacement after. pendingFocus survives async flows (update all /
  // activate) whose final re-render happens after the backend call, while
  // the control is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (el: HTMLElement): string | null => {
    if (el.closest("[data-refresh]")) return "refresh";
    if (el.closest("[data-update-all]")) return "update-all";
    const active = el.closest<HTMLElement>("[data-active]");
    if (active) return `active:${active.dataset.active}`;
    const scan = el.closest<HTMLElement>("[data-scan]");
    if (scan) return `scan:${scan.dataset.scan}`;
    return null;
  };

  const restoreFocus = (key: string | null): boolean => {
    if (!key) return false;
    const [kind, id] = key.split(":");
    let target: HTMLElement | null = null;
    if (kind === "refresh") target = el.querySelector<HTMLElement>("[data-refresh]");
    else if (kind === "update-all") target = el.querySelector<HTMLElement>("[data-update-all]");
    else if (kind === "active") target = el.querySelector<HTMLElement>(`[data-active="${id}"]`);
    else if (kind === "scan") target = el.querySelector<HTMLElement>(`[data-scan="${id}"]`);
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
    else if (!syncing && activating === null) pendingFocus = null;
  };

  const updateAll = async (): Promise<void> => {
    if (syncing || !status) return;
    const count = status.installs.filter((i) => i.exists).length;
    if (count === 0) return;
    const confirmed = await confirmDialog({
      title: `Update all installs (${count})?`,
      message: `Check and update addons in all ${count} install${count === 1 ? "" : "s"}? Flavor-mismatched updates are flagged and skipped unless confirmed.`,
      confirmLabel: "Update All",
    });
    if (!confirmed) return;
    syncing = true;
    syncResult = null;
    rerender();
    try {
      const res = await service.SyncUpdatesToAll(confirmed);
      syncResult = res;
      const touched = res.installs.filter(
        (i) => i.updated > 0 || i.failed > 0,
      ).length;
      toast({
        type:
          res.total_failed > 0
            ? res.total_updated > 0
              ? "warn"
              : "error"
            : "ok",
        title:
          res.total_failed > 0
            ? "Update all completed with errors"
            : "Update all complete",
        message: `${res.total_updated} updated across ${touched} install${touched === 1 ? "" : "s"} · ${res.total_failed} failed`,
      });
    } catch (err) {
      toast({ type: "error", title: "Update all failed", message: errText(err) });
    } finally {
      syncing = false;
      // Re-fetch status so the health cards reflect the sync.
      await load();
    }
  };

  // Point wowfix at this install (same path as first-run setup) and land on
  // the scan view so the user sees its addons. completeSetup refreshes the
  // shared state and rescans before navigating.
  const activate = async (inst: InstallStatus): Promise<void> => {
    if (activating) return;
    activating = inst.root;
    rerender();
    try {
      await actions.completeSetup(
        inst.root,
        inst.flavor,
        inst.profile_id || app.state.profile_id,
      );
      actions.go("scan");
    } catch (err) {
      toast({
        type: "error",
        title: "Could not switch install",
        message: errText(err),
      });
      activating = null;
      rerender();
    }
  };

  const render = (): void => {
    const installs = status?.installs ?? [];
    const ready = installs.filter((i) => i.exists).length;
    const busy = syncing || activating !== null;

    el.innerHTML = `
      <div class="installs">
        <div class="installs-toolbar">
          <div class="installs-toolbar-left">
            <button class="btn btn-primary" data-update-all ${busy || ready === 0 ? "disabled" : ""}>
              ${syncing ? `<span class="spinner"></span>` : icon("download", 15)}
              <span>${syncing ? "Updating…" : `Update all installs (${ready})`}</span>
            </button>
            ${
              status
                ? `<span class="installs-summary muted">${installs.length} install${installs.length === 1 ? "" : "s"} detected · ${ready} ready</span>`
                : ""
            }
          </div>
          <button class="btn btn-outline" data-refresh ${busy ? "disabled" : ""}>
            ${icon("refresh", 15)}<span>Refresh</span>
          </button>
        </div>

        ${syncResult ? renderSyncResult(syncResult) : ""}

        ${
          !status
            ? `<div class="list-loading"><span class="spinner spinner-lg"></span><span>${loading ? "Checking installs…" : "Loading…"}</span></div>`
            : installs.length === 0
              ? emptyCard(
                  "folder",
                  "No WoW installations detected",
                  "Point wowfix at a World of Warcraft install from the setup screen.",
                  `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
                )
              : `<div class="install-cards">${installs
                  .map((inst, i) => renderCard(inst, i, busy))
                  .join("")}</div>`
        }
      </div>`;

    el.querySelector("[data-refresh]")?.addEventListener("click", () => void load());
    el.querySelector("[data-update-all]")?.addEventListener("click", () => {
      pendingFocus = "update-all";
      void updateAll();
    });
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
    el.querySelectorAll<HTMLElement>("[data-active]").forEach((btn) => {
      const inst = installs[Number(btn.dataset.active)];
      btn.addEventListener("click", () => {
        pendingFocus = `active:${btn.dataset.active}`;
        void activate(inst);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-scan]").forEach((btn) => {
      const inst = installs[Number(btn.dataset.scan)];
      btn.addEventListener("click", () => {
        pendingFocus = `scan:${btn.dataset.scan}`;
        void activate(inst);
      });
    });
  };

  const renderCard = (inst: InstallStatus, i: number, busy: boolean): string => {
    const band =
      inst.health >= 85 ? "healthy" : inst.health >= 60 ? "attention" : "repair";
    const bandLabel =
      band === "healthy"
        ? "Healthy"
        : band === "attention"
          ? "Needs attention"
          : "Needs repair";
    const profile = app.profiles.find((p) => p.id === inst.profile_id);

    const head = `
      <div class="install-card-head">
        <span class="install-path mono" title="${escapeAttr(inst.root)}">${escapeHtml(truncateMiddle(inst.root, 46))}</span>
        ${inst.flavor ? `<span class="flavor-tag">${escapeHtml(inst.flavor)}</span>` : ""}
        ${profile ? `<span class="chip chip-neutral">${escapeHtml(profile.name)}</span>` : ""}
        ${inst.version ? `<span class="install-version mono">v${escapeHtml(inst.version)}</span>` : ""}
        ${inst.confidence ? `<span class="chip chip-${confClass(inst.confidence)}">${escapeHtml(inst.confidence)} confidence</span>` : ""}
      </div>`;

    if (!inst.exists) {
      return `
        <div class="install-card missing">
          ${head}
          <div class="install-missing">
            <span class="install-missing-icon">${icon("alert", 16)}</span>
            <span>AddOns folder not found</span>
          </div>
        </div>`;
    }

    return `
      <div class="install-card band-${band}">
        ${head}
        <div class="install-card-body">
          <div class="install-health">
            <span class="install-score">${inst.health}</span>
            <span class="install-denom">/100</span>
            <span class="install-band-label">${bandLabel}</span>
          </div>
          <div class="install-stats">
            <span class="count-item"><span class="status-dot ok"></span><span class="count-num">${inst.addons}</span> addons</span>
            <span class="count-item"><span class="status-dot warn"></span><span class="count-num">${inst.problems}</span> with issues</span>
            <span class="count-item ${inst.errors > 0 ? "" : "muted"}">
              <span class="status-dot ${inst.errors > 0 ? "error" : "muted"}"></span><span class="count-num">${inst.errors}</span> error${inst.errors === 1 ? "" : "s"}
            </span>
          </div>
          <div class="install-actions">
            <button class="btn btn-outline btn-sm" data-scan="${i}" ${busy ? "disabled" : ""}>
              ${icon("radar", 13)}<span>Scan</span>
            </button>
            <button class="btn btn-primary btn-sm" data-active="${i}" ${busy ? "disabled" : ""}>
              ${icon("check", 13)}<span>Set as active</span>
            </button>
          </div>
        </div>
      </div>`;
  };

  const renderSyncResult = (res: SyncResult): string => `
    <div class="sync-result" role="status">
      <div class="result-summary">
        <span class="result-summary-icon ${res.total_failed > 0 ? "sync-warn" : "sync-ok"}">
          ${icon(res.total_failed > 0 ? "alert" : "check-circle", 16)}
        </span>
        <span class="result-summary-text">
          <b>${res.total_updated} updated</b>
          ${res.total_failed > 0 ? ` · ${res.total_failed} failed` : ""}
          <span class="muted">across ${res.installs.length} install${res.installs.length === 1 ? "" : "s"}</span>
        </span>
      </div>
      <div class="sync-rows">
        ${res.installs
          .map(
            (r) => `
          <div class="sync-row">
            <span class="sync-root mono" title="${escapeAttr(r.root)}">${escapeHtml(truncateMiddle(r.root, 44))}</span>
            <span class="sync-nums mono">
              <span class="text-ok">${r.updated} updated</span>
              ${r.failed > 0 ? `<span class="text-warn"> · ${r.failed} failed</span>` : ""}
            </span>
            ${
              r.errors.length
                ? `<ul class="sync-errors">${r.errors
                    .map(
                      (e) =>
                        `<li>${icon("x-circle", 13)}<span>${escapeHtml(e)}</span></li>`,
                    )
                    .join("")}</ul>`
                : ""
            }
          </div>`,
          )
          .join("")}
      </div>
    </div>`;

  render();
  void load();

  return {
    refresh: rerender,
  };
}

function confClass(confidence: string): string {
  switch (confidence) {
    case "high":
      return "ok";
    case "medium":
      return "warn";
    default:
      return "error";
  }
}

// Truncate a path in the middle so the drive + final folders stay visible.
function truncateMiddle(s: string, max: number): string {
  if (s.length <= max) return s;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${s.slice(0, head)}…${s.slice(-tail)}`;
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
