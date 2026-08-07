// Collections: named addon loadouts. Each collection records which addon
// folders it contains and whether each is enabled; switching activates a
// loadout (renaming folders in the real backend, always after a backup
// snapshot). Details expand inline with per-addon enable toggles.

import type { AppState, Actions } from "../app";
import type {
  CollectionAddonState,
  CollectionDetail,
  CollectionInfo,
  CollectionsResult,
} from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
import { confirmDialog } from "../components/dialog";

export function mountCollections(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let result: CollectionsResult | null = null;
  let loading = false;
  let creating = false; // inline create form visible
  let busyCreate = false;
  let switching = false;
  let deleting = false;
  let toggling: string | null = null; // folder being toggled
  const expanded = new Set<string>();
  const details = new Map<string, CollectionDetail | null>(); // null = loading

  const load = async (): Promise<void> => {
    loading = true;
    rerender();
    try {
      result = await service.Collections();
    } catch (err) {
      toast({
        type: "error",
        title: "Could not load collections",
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
  // replacement after. pendingFocus survives async flows (switch / delete /
  // toggle) whose final re-render happens after the backend call, while the
  // control is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (el: HTMLElement): string | null => {
    if (el.closest("[data-new]")) return "new";
    if (el.closest("[data-refresh]")) return "refresh";
    const sw = el.closest<HTMLElement>("[data-switch]");
    if (sw) return `switch:${sw.dataset.switch}`;
    const ex = el.closest<HTMLElement>("[data-expand]");
    if (ex) return `expand:${ex.dataset.expand}`;
    const del = el.closest<HTMLElement>("[data-delete]");
    if (del) return `delete:${del.dataset.delete}`;
    const tg = el.closest<HTMLElement>("[data-toggle]");
    if (tg) return `toggle:${tg.dataset.col}:${tg.dataset.folder}`;
    if (el.closest("[data-name]")) return "name";
    if (el.closest("[data-create]")) return "create";
    if (el.closest("[data-cancel-create]")) return "cancel-create";
    return null;
  };

  const restoreFocus = (key: string | null): boolean => {
    if (!key) return false;
    const [kind, ...rest] = key.split(":");
    let target: HTMLElement | null = null;
    if (kind === "new") target = el.querySelector<HTMLElement>("[data-new]");
    else if (kind === "refresh") target = el.querySelector<HTMLElement>("[data-refresh]");
    else if (kind === "switch") target = el.querySelector<HTMLElement>(`[data-switch="${rest[0]}"]`);
    else if (kind === "expand") target = el.querySelector<HTMLElement>(`[data-expand="${rest[0]}"]`);
    else if (kind === "delete") target = el.querySelector<HTMLElement>(`[data-delete="${rest[0]}"]`);
    else if (kind === "toggle")
      target = el.querySelector<HTMLElement>(
        `[data-toggle][data-col="${rest[0]}"][data-folder="${rest[1]}"]`,
      );
    else if (kind === "name") target = el.querySelector<HTMLElement>("[data-name]");
    else if (kind === "create") target = el.querySelector<HTMLElement>("[data-create]");
    else if (kind === "cancel-create") target = el.querySelector<HTMLElement>("[data-new]");
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
    else if (!switching && !deleting && !busyCreate && toggling === null) pendingFocus = null;
  };

  const create = async (name: string): Promise<void> => {
    if (busyCreate) return;
    busyCreate = true;
    rerender();
    try {
      const c = await service.CreateCollection(name);
      creating = false;
      toast({ type: "ok", title: "Collection created", message: c.name });
      await load();
    } catch (err) {
      toast({
        type: "error",
        title: "Could not create collection",
        message: errText(err),
      });
    } finally {
      busyCreate = false;
      rerender();
    }
  };

  const switchTo = async (id: string): Promise<void> => {
    if (switching || deleting) return;
    const c = result?.collections.find((x) => x.id === id);
    if (!c) return;
    const confirmed = await confirmDialog({
      title: `Switch to collection “${c.name}”?`,
      message:
        "This renames addon folders to match the collection (backup snapshot taken first).",
      confirmLabel: "Switch",
    });
    if (!confirmed) return;
    switching = true;
    rerender();
    try {
      const res = await service.SwitchCollection(id);
      toast({
        type: "ok",
        title: `Switched to “${c.name}”`,
        message: res.message || `${res.applied.length} folder${res.applied.length === 1 ? "" : "s"} renamed — backup snapshot taken`,
      });
      await load();
    } catch (err) {
      toast({
        type: "error",
        title: "Could not switch collection",
        message: errText(err),
      });
    } finally {
      switching = false;
      rerender();
    }
  };

  const remove = async (id: string): Promise<void> => {
    if (switching || deleting) return;
    const c = result?.collections.find((x) => x.id === id);
    if (!c) return;
    const confirmed = await confirmDialog({
      title: `Delete collection “${c.name}”?`,
      message:
        "The collection record is removed. Addon folders on disk are not touched.",
      confirmLabel: "Delete",
      danger: true,
    });
    if (!confirmed) return;
    deleting = true;
    rerender();
    try {
      await service.DeleteCollection(id);
      expanded.delete(id);
      details.delete(id);
      toast({ type: "ok", title: "Collection deleted", message: c.name });
      await load();
    } catch (err) {
      toast({
        type: "error",
        title: "Could not delete collection",
        message: errText(err),
      });
    } finally {
      deleting = false;
      rerender();
    }
  };

  const ensureDetail = (id: string): void => {
    if (details.has(id)) return;
    details.set(id, null);
    void service
      .CollectionDetail(id)
      .then((d) => {
        details.set(id, d);
        rerender();
      })
      .catch((err) => {
        details.delete(id);
        expanded.delete(id);
        toast({
          type: "error",
          title: "Could not load collection",
          message: errText(err),
        });
        rerender();
      });
  };

  const toggleAddon = async (
    id: string,
    folder: string,
    enabled: boolean,
  ): Promise<void> => {
    if (toggling) return;
    toggling = folder;
    rerender();
    try {
      await service.SetCollectionAddon(id, folder, !enabled);
      details.set(id, await service.CollectionDetail(id));
      toast({
        type: "ok",
        title: `${!enabled ? "Enabled" : "Disabled"} ${folder}`,
        message: "Collection loadout updated",
      });
    } catch (err) {
      toast({
        type: "error",
        title: "Could not update collection",
        message: errText(err),
      });
    } finally {
      toggling = null;
      rerender();
    }
  };

  const render = (): void => {
    const cols = result?.collections ?? [];
    const busy = switching || deleting || busyCreate;
    const activeId = result?.active_id ?? "";

    el.innerHTML = `
      <div class="collections">
        <div class="collections-toolbar">
          <div class="collections-toolbar-left">
            <button class="btn btn-primary" data-new ${busy ? "disabled" : ""}>
              ${icon("stack", 15)}<span>New collection</span>
            </button>
            ${
              result
                ? `<span class="collections-count muted">${cols.length} collection${cols.length === 1 ? "" : "s"}</span>`
                : ""
            }
          </div>
          <button class="btn btn-outline" data-refresh ${busy ? "disabled" : ""}>
            ${icon("refresh", 15)}<span>Refresh</span>
          </button>
        </div>

        ${
          creating
            ? `<div class="collections-create">
                <input class="collections-create-input" type="text" placeholder="Collection name…" spellcheck="false"
                  maxlength="60" aria-label="New collection name" data-name />
                <button class="btn btn-primary" data-create ${busyCreate ? "disabled" : ""}>
                  ${busyCreate ? `<span class="spinner"></span>` : icon("check", 15)}
                  <span>${busyCreate ? "Creating…" : "Create"}</span>
                </button>
                <button class="btn btn-ghost" data-cancel-create ${busyCreate ? "disabled" : ""}>Cancel</button>
              </div>`
            : ""
        }

        ${
          !result
            ? `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Loading collections…</span></div>`
            : cols.length === 0
              ? emptyCard(
                  "stack",
                  "No collections yet",
                  "Capture your current addon setup as a collection.",
                  `<button class="btn btn-primary" data-new>${icon("stack", 16)}<span>New collection</span></button>`,
                )
              : `<div class="collection-rows">${cols.map((c, i) => renderRow(c, i, activeId, busy)).join("")}</div>`
        }
      </div>`;

    el.querySelector("[data-new]")?.addEventListener("click", () => {
      creating = !creating;
      rerender();
      if (creating) el.querySelector<HTMLInputElement>("[data-name]")?.focus();
    });
    el.querySelector("[data-refresh]")?.addEventListener("click", () => void load());
    el.querySelector("[data-cancel-create]")?.addEventListener("click", () => {
      creating = false;
      rerender();
    });
    el.querySelector("[data-create]")?.addEventListener("click", () => {
      const name = el.querySelector<HTMLInputElement>("[data-name]")?.value.trim() ?? "";
      if (name) void create(name);
    });
    el.querySelector<HTMLInputElement>("[data-name]")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        const name = (e.target as HTMLInputElement).value.trim();
        if (name) void create(name);
      } else if (e.key === "Escape") {
        creating = false;
        rerender();
      }
    });
    el.querySelectorAll<HTMLElement>("[data-switch]").forEach((btn) => {
      const c = cols[Number(btn.dataset.switch)];
      btn.addEventListener("click", () => {
        pendingFocus = `switch:${btn.dataset.switch}`;
        void switchTo(c.id);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-expand]").forEach((btn) => {
      const c = cols[Number(btn.dataset.expand)];
      btn.addEventListener("click", () => {
        if (expanded.has(c.id)) expanded.delete(c.id);
        else {
          expanded.add(c.id);
          ensureDetail(c.id);
        }
        rerender();
      });
    });
    el.querySelectorAll<HTMLElement>("[data-delete]").forEach((btn) => {
      const c = cols[Number(btn.dataset.delete)];
      btn.addEventListener("click", () => {
        pendingFocus = `delete:${btn.dataset.delete}`;
        void remove(c.id);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-toggle]").forEach((btn) => {
      const colId = btn.dataset.col ?? "";
      const folder = btn.dataset.folder ?? "";
      const a = details.get(colId)?.addons.find((x) => x.folder === folder);
      if (!a) return;
      btn.addEventListener("click", () => {
        pendingFocus = `toggle:${colId}:${folder}`;
        void toggleAddon(colId, folder, a.enabled);
      });
    });
  };

  const renderRow = (
    c: CollectionInfo,
    i: number,
    activeId: string,
    busy: boolean,
  ): string => {
    const isOpen = expanded.has(c.id);
    const isActive = c.active || c.id === activeId;
    const detail = isOpen ? renderDetail(c.id) : "";
    return `
      <div class="collection-item">
        <div class="collection-row${isActive ? " is-active" : ""}${isOpen ? " expanded" : ""}">
          <div class="collection-info">
            <div class="collection-name-line">
              <span class="collection-name">${escapeHtml(c.name)}</span>
              ${isActive ? `<span class="tag tag-active">${icon("check", 11)}Active</span>` : ""}
            </div>
            <span class="collection-count">${c.addon_count} addon${c.addon_count === 1 ? "" : "s"}</span>
          </div>
          <div class="collection-actions">
            ${
              isActive
                ? `<span class="collection-active-note muted">Currently active</span>`
                : `<button class="btn btn-primary btn-sm" data-switch="${i}" ${busy ? "disabled" : ""}>
                    ${icon("refresh", 13)}<span>Switch</span>
                  </button>`
            }
            <button class="btn btn-outline btn-sm" data-expand="${i}" aria-expanded="${isOpen}">
              ${isOpen ? icon("chevron-down", 14) : icon("chevron-right", 14)}
              <span>${isOpen ? "Hide" : "Details"}</span>
            </button>
            <button class="btn btn-danger btn-sm" data-delete="${i}" ${busy ? "disabled" : ""}>
              ${icon("trash", 13)}<span>Delete</span>
            </button>
          </div>
        </div>
        ${detail}
      </div>`;
  };

  const renderDetail = (id: string): string => {
    const d = details.get(id);
    return `
      <div class="collection-detail">
        <h3 class="detail-title">Addons${d ? ` (${d.addons.length})` : ""}</h3>
        ${
          !d
            ? `<div class="detail-loading"><span class="spinner spinner-xs"></span><span>Loading…</span></div>`
            : d.addons.length === 0
              ? `<p class="detail-empty muted">No addons in this collection yet — capture a loadout from the scan list.</p>`
              : `<div class="collection-addons">${d.addons
                  .map((a) => renderAddonRow(id, a))
                  .join("")}</div>`
        }
      </div>`;
  };

  const renderAddonRow = (id: string, a: CollectionAddonState): string => `
    <div class="collection-addon${a.enabled ? "" : " disabled"}">
      <span class="collection-addon-folder mono">${escapeHtml(a.folder)}</span>
      ${
        a.enabled
          ? `<span class="tag tag-enabled">enabled</span>`
          : `<span class="tag tag-disabled">disabled</span>`
      }
      <button class="switch${a.enabled ? " on" : ""}" role="switch" aria-checked="${a.enabled}"
        aria-label="${escapeAttr(a.folder)}: ${a.enabled ? "enabled" : "disabled"}"
        data-toggle="1" data-col="${escapeAttr(id)}" data-folder="${escapeAttr(a.folder)}"
        ${toggling ? "disabled" : ""}>
        <span class="switch-knob"></span>
      </button>
    </div>`;

  render();
  void load();

  return {
    refresh: rerender,
  };
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
