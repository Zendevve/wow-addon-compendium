// Updates: dry-run check of tracked addons against their providers. The
// list IS the preview — nothing downloads until the user clicks Update /
// Update all. Flavor mismatches are surfaced with an amber badge and a
// confirm dialog before applying, but never block the list.

import type { AppState, Actions } from "../app";
import type { UpdateEntry, CheckUpdatesResult, ApplyBatch, TrackedAddon } from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
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

  const check = async (): Promise<void> => {
    checking = true;
    render();
    try {
      result = await service.CheckUpdates();
    } catch (err) {
      toast({ type: "error", title: "Check failed", message: errText(err) });
    } finally {
      checking = false;
      render();
    }
  };

  const loadTracked = async (): Promise<void> => {
    if (!app.state.has_install) return;
    try {
      tracked = (await service.TrackedAddons()).addons;
      trackedErr = null;
    } catch (err) {
      trackedErr = errText(err, "Could not load tracked addons");
    }
    render();
  };

  const reloadAll = async (): Promise<void> => {
    await Promise.all([check(), loadTracked()]);
  };

  const setPinned = async (a: TrackedAddon, pinned: boolean): Promise<void> => {
    if (mutating) return;
    mutating = a.folder;
    render();
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
    render();
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
    render();
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
    render();
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
    render();
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

        ${managedSectionHtml(managedAll, managedVisible, managedPinned, managedIgnored)}
      </div>`;

    el.querySelector("[data-check]")?.addEventListener("click", () => void check());
    el.querySelector("[data-apply-all]")?.addEventListener("click", () => void applyAll());
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
    el.querySelectorAll<HTMLElement>("[data-apply-one]").forEach((btn) => {
      const u = updates[Number(btn.dataset.applyOne)];
      btn.addEventListener("click", () => void applyOne(u));
    });
    el.querySelectorAll<HTMLElement>("[data-managed-filter]").forEach((chip) => {
      chip.addEventListener("click", () => {
        managedFilter = (chip.dataset.managedFilter ?? "all") as ManagedFilter;
        render();
      });
    });
    el.querySelectorAll<HTMLElement>("[data-managed-pin]").forEach((btn) => {
      const t = managedVisible[Number(btn.dataset.managedPin)];
      btn.addEventListener("click", () => void setPinned(t, !t.pinned));
    });
    el.querySelectorAll<HTMLElement>("[data-managed-ignore]").forEach((btn) => {
      const t = managedVisible[Number(btn.dataset.managedIgnore)];
      btn.addEventListener("click", () => void setIgnored(t, !t.ignored));
    });
    el.querySelectorAll<HTMLElement>("[data-managed-rollback]").forEach((btn) => {
      const t = managedVisible[Number(btn.dataset.managedRollback)];
      btn.addEventListener("click", () => void rollback(t));
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
