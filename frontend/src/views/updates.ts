// Updates: dry-run check of tracked addons against their providers. The
// list IS the preview — nothing downloads until the user clicks Update /
// Update all. Flavor mismatches are surfaced with an amber badge and a
// confirm dialog before applying, but never block the list.

import type { AppState, Actions } from "../app";
import type { UpdateEntry, CheckUpdatesResult, ApplyBatch } from "../types";
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

export function mountUpdates(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let result: CheckUpdatesResult | null = null;
  let checking = false;
  let applying: string | null = null; // folder being updated, or "all"
  const failures = new Map<string, string>(); // folder -> error text

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
      await check();
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
      await check();
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
      </div>`;

    el.querySelector("[data-check]")?.addEventListener("click", () => void check());
    el.querySelector("[data-apply-all]")?.addEventListener("click", () => void applyAll());
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
    el.querySelectorAll<HTMLElement>("[data-apply-one]").forEach((btn) => {
      const u = updates[Number(btn.dataset.applyOne)];
      btn.addEventListener("click", () => void applyOne(u));
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

  render();
  void check();

  return {
    refresh: render,
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
