// Collections: named addon loadouts. Each collection records which addon
// folders it contains and whether each is enabled; switching activates a
// loadout (folder renames behind a backup snapshot). Details expand inline
// with per-addon enable toggles. Export/Import share this view: bundle a
// collection into a ZIP (optionally with saved variables) or install one
// from a file, URL or owner/repo source.

import type { View } from "../view";
import type {
  CollectionAddonState,
  CollectionDetail,
  CollectionInfo,
  CollectionsResult,
  ExportResult,
  ImportResult,
} from "../types";
import { icon } from "../icons";
import { service, mockActive } from "../api";
import { toast } from "../toast";
import { confirmDialog } from "../dialog";
import "./collections.css";

export const view: View = {
  id: "collections",
  label: "Collections",
  icon: "stack",
  mount(host) {
    mountCollections(host);
  },
};

function mountCollections(host: HTMLElement): void {
  let result: CollectionsResult | null = null;
  let loading = false;
  let loadError: string | null = null;

  // create form
  let creating = false;
  let createBusy = false;

  // per-op busy state: only the acting control is disabled (not the app)
  let switchBusy = "";
  let deleteBusy = "";
  let toggleBusy = "";

  const expanded = new Set<string>();
  const details = new Map<string, CollectionDetail | null>(); // null = loading

  // export / import surface
  let outPath = "";
  let collectionID = "";
  let includeSavedVars = false;
  let exportBusy = false;
  let exportResult: ExportResult | null = null;
  let exportError: string | null = null;
  let importSrc = "";
  let importBusy = false;
  let importResult: ImportResult | null = null;
  let importError: string | null = null;

  // focus preservation: re-renders rebuild the DOM, so capture the focused
  // control before a render and restore it after. pendingFocus survives
  // async flows (switch / delete / toggle / export) whose final re-render
  // happens while the control is still disabled.
  let pendingFocus: string | null = null;
  let refocus: { sel: string; pos: number } | null = null;

  const focusKeyOf = (node: HTMLElement | null): string | null => {
    if (!node) return null;
    if (node.closest("[data-out]")) return "input:[data-out]";
    if (node.closest("[data-collection]")) return "input:[data-collection]";
    if (node.closest("[data-import-path]")) return "input:[data-import-path]";
    if (node.closest("[data-name]")) return "input:[data-name]";
    if (node.closest("[data-new]")) return "btn:new";
    if (node.closest("[data-create]")) return "btn:create";
    if (node.closest("[data-export]")) return "btn:export";
    if (node.closest("[data-import]")) return "btn:import";
    const sw = node.closest<HTMLElement>("[data-switch]");
    if (sw) return `btn:switch:${sw.dataset.switch}`;
    const ex = node.closest<HTMLElement>("[data-expand]");
    if (ex) return `btn:expand:${ex.dataset.expand}`;
    const del = node.closest<HTMLElement>("[data-delete]");
    if (del) return `btn:delete:${del.dataset.delete}`;
    const tg = node.closest<HTMLElement>("[data-toggle]");
    if (tg) return `btn:toggle:${tg.dataset.col}:${tg.dataset.folder}`;
    return null;
  };

  const restoreFocus = (key: string): boolean => {
    const parts = key.split(":");
    if (parts[0] === "input") {
      const target = host.querySelector<HTMLInputElement>(parts[1]);
      if (!target) return false;
      const pos = refocus ? refocus.pos : target.value.length;
      target.focus();
      if (pos >= 0) {
        try {
          target.setSelectionRange(pos, pos);
        } catch {
          /* non-text inputs (select) */
        }
      }
      return true;
    }
    if (parts[0] !== "btn") return false;
    const what = parts[1];
    let target: HTMLElement | null = null;
    if (what === "new") target = host.querySelector("[data-new]");
    else if (what === "create") target = host.querySelector("[data-create]");
    else if (what === "export") target = host.querySelector("[data-export]");
    else if (what === "import") target = host.querySelector("[data-import]");
    else if (what === "switch") target = host.querySelector(`[data-switch="${parts[2]}"]`);
    else if (what === "expand") target = host.querySelector(`[data-expand="${parts[2]}"]`);
    else if (what === "delete") target = host.querySelector(`[data-delete="${parts[2]}"]`);
    else if (what === "toggle")
      target = host.querySelector(
        `[data-toggle][data-col="${parts[2]}"][data-folder="${parts.slice(3).join(":")}"]`,
      );
    if (!target || (target as HTMLButtonElement).disabled) return false;
    target.focus();
    return true;
  };

  const busyInFlight = (): boolean =>
    loading ||
    createBusy ||
    Boolean(switchBusy) ||
    Boolean(deleteBusy) ||
    Boolean(toggleBusy) ||
    exportBusy ||
    importBusy;

  const rerender = (): void => {
    const active = document.activeElement;
    const key =
      refocus ? `input:${refocus.sel}` : pendingFocus ?? focusKeyOf(active instanceof HTMLElement ? active : null);
    render();
    if (!key) {
      refocus = null;
      pendingFocus = null;
      return;
    }
    if (restoreFocus(key)) {
      refocus = null;
      pendingFocus = null;
    } else if (refocus) {
      refocus = null;
    } else if (busyInFlight()) {
      // keep pendingFocus: the final re-render lands it on the re-enabled control
    } else {
      pendingFocus = null;
    }
  };

  // ---- list ops -----------------------------------------------------------

  const load = async (): Promise<void> => {
    if (loading) return;
    loading = true;
    loadError = null;
    rerender();
    try {
      result = await service.Collections();
    } catch (err) {
      loadError = errText(err);
      if (result) {
        toast({ type: "error", title: "Could not refresh collections", message: loadError ?? undefined });
      }
    } finally {
      loading = false;
      rerender();
    }
  };

  const create = async (name: string): Promise<void> => {
    if (createBusy) return;
    createBusy = true;
    rerender();
    try {
      const c = await service.CreateCollection(name);
      creating = false;
      toast({ type: "ok", title: "Collection created", message: c.name });
      await load();
    } catch (err) {
      toast({ type: "error", title: "Could not create collection", message: errText(err) });
    } finally {
      createBusy = false;
      rerender();
    }
  };

  const switchTo = async (id: string): Promise<void> => {
    if (switchBusy) return;
    const c = result?.collections.find((x) => x.id === id);
    if (!c) return;
    const confirmed = await confirmDialog({
      title: `Switch to “${c.name}”?`,
      message:
        "Addon folders are renamed to match this loadout — a backup snapshot is taken first.",
      confirmLabel: "Switch",
    });
    if (!confirmed) return;
    switchBusy = id;
    rerender();
    try {
      const res = await service.SwitchCollection(id);
      toast({
        type: "ok",
        title: `Switched to “${c.name}”`,
        message:
          res.message ||
          `${res.applied.length} folder${res.applied.length === 1 ? "" : "s"} renamed. Backup snapshot taken.`,
      });
      await load();
    } catch (err) {
      toast({ type: "error", title: "Could not switch collection", message: errText(err) });
    } finally {
      switchBusy = "";
      rerender();
    }
  };

  const remove = async (id: string): Promise<void> => {
    if (deleteBusy) return;
    const c = result?.collections.find((x) => x.id === id);
    if (!c) return;
    const confirmed = await confirmDialog({
      title: `Delete collection “${c.name}”?`,
      message: "Only the collection record is removed — addon folders on disk are untouched.",
      confirmLabel: "Delete",
      danger: true,
    });
    if (!confirmed) return;
    deleteBusy = id;
    rerender();
    try {
      await service.DeleteCollection(id);
      expanded.delete(id);
      details.delete(id);
      toast({ type: "ok", title: "Collection deleted", message: c.name });
      await load();
    } catch (err) {
      toast({ type: "error", title: "Could not delete collection", message: errText(err) });
    } finally {
      deleteBusy = "";
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
        toast({ type: "error", title: "Could not load collection", message: errText(err) });
        rerender();
      });
  };

  const toggleAddon = async (id: string, folder: string, enabled: boolean): Promise<void> => {
    if (toggleBusy) return;
    toggleBusy = folder;
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
      toast({ type: "error", title: "Could not update collection", message: errText(err) });
    } finally {
      toggleBusy = "";
      rerender();
    }
  };

  // ---- export / import ops ------------------------------------------------

  const pickOutPath = async (): Promise<void> => {
    const rt = (window as unknown as {
      runtime?: { SaveFileDialog?: (opts: unknown) => Promise<string | null> };
    }).runtime;
    if (!rt?.SaveFileDialog) return;
    try {
      const p = await rt.SaveFileDialog({
        defaultFilename: "wowfix-collection-export.zip",
        filters: [{ displayName: "ZIP archives", pattern: "*.zip" }],
      });
      if (!p) return;
      outPath = p;
      refocus = { sel: "[data-out]", pos: outPath.length };
      rerender();
    } catch {
      /* dialog unavailable/cancelled — the manual input stays usable */
    }
  };

  const pickOpenPath = async (): Promise<void> => {
    const rt = (window as unknown as {
      runtime?: { OpenFileDialog?: (opts: unknown) => Promise<string | null> };
    }).runtime;
    if (!rt?.OpenFileDialog) return;
    try {
      const p = await rt.OpenFileDialog({
        filters: [{ displayName: "ZIP archives", pattern: "*.zip" }],
      });
      if (!p) return;
      importSrc = p;
      refocus = { sel: "[data-import-path]", pos: importSrc.length };
      rerender();
    } catch {
      /* dialog unavailable/cancelled */
    }
  };

  const doExport = async (): Promise<void> => {
    const path = outPath.trim();
    if (exportBusy || !path) return;
    exportBusy = true;
    exportError = null;
    exportResult = null;
    rerender();
    try {
      const res = await service.ExportCollection(path, collectionID, includeSavedVars);
      exportResult = res;
      toast({
        type: "ok",
        title: "Export complete",
        message: `${res.addons} addon${res.addons === 1 ? "" : "s"} written to ${res.out}`,
      });
    } catch (err) {
      exportError = `Export failed: ${errText(err)}`;
      toast({ type: "error", title: "Export failed", message: errText(err) });
    } finally {
      exportBusy = false;
      rerender();
    }
  };

  const doImport = async (): Promise<void> => {
    const src = importSrc.trim();
    if (importBusy || !src) return;
    const confirmed = await confirmDialog({
      title: "Import addons?",
      message: `Addons from “${src}” are installed into the current AddOns folder.`,
      confirmLabel: "Import",
    });
    if (!confirmed) return;
    importBusy = true;
    importError = null;
    importResult = null;
    rerender();
    try {
      const res = await service.ImportCollection(src);
      importResult = res;
      toast({
        type: res.installed.length > 0 ? "ok" : "info",
        title: "Import complete",
        message:
          res.installed.length > 0
            ? `Installed ${res.installed.length} addon${res.installed.length === 1 ? "" : "s"}`
            : "Nothing new was installed",
      });
    } catch (err) {
      importError = `Import failed: ${errText(err)}`;
      toast({ type: "error", title: "Import failed", message: errText(err) });
    } finally {
      importBusy = false;
      rerender();
    }
  };

  // ---- render -------------------------------------------------------------

  const render = (): void => {
    const cols = result?.collections ?? [];

    host.innerHTML = `
      <section class="view-page">
        <div class="view-hero">
          <h1 class="view-title">Collections</h1>
          <p class="view-sub">Addon loadouts per character — switch, tune, and share them as portable exports.</p>
        </div>

        <div class="collections">
          <div class="collections-toolbar">
            <button class="btn-primary" data-new ${busyAny() ? "disabled" : ""}>
              ${icon("plus", 15)}<span>New collection</span>
            </button>
            ${result ? `<span class="collections-count tnum">${cols.length} collection${cols.length === 1 ? "" : "s"}</span>` : ""}
          </div>

          ${creating ? renderCreateForm() : ""}
          ${renderList()}
        </div>

        <div class="collections-section">
          <h2 class="collections-section-title">Export &amp; import</h2>
          <p class="collections-section-sub">Bundle a loadout into a portable ZIP — or restore one from a file, URL, or owner/repo source.</p>
          <div class="transfer-grid">
            ${renderExportCard()}
            ${renderImportCard()}
          </div>
        </div>
      </section>`;

    bindEvents(cols);
  };

  const busyAny = (): boolean =>
    Boolean(switchBusy) || Boolean(deleteBusy) || Boolean(toggleBusy);

  const renderCreateForm = (): string => `
    <div class="collections-create">
      <input class="text-input" type="text" placeholder="Collection name…" spellcheck="false" maxlength="60" aria-label="New collection name" data-name />
      <button class="btn-primary" data-create ${createBusy ? "disabled" : ""}>
        ${createBusy ? `<span class="collections-spin"></span>` : icon("check", 15)}
        <span>${createBusy ? "Creating…" : "Create"}</span>
      </button>
      <button class="btn-secondary" data-cancel-create ${createBusy ? "disabled" : ""}>Cancel</button>
    </div>`;

  const renderList = (): string => {
    if (!result) {
      if (loading) {
        return `<div class="collections-loading"><span class="collections-spin"></span><span>Loading collections…</span></div>`;
      }
      if (loadError) return renderError(loadError, true);
      return "";
    }
    const cols = result.collections;
    if (cols.length === 0) {
      return `
        <div class="empty-state">
          <span class="empty-title">No collections yet</span>
          <span class="empty-body">Capture your current addon setup as a named loadout, then switch between characters with one click.</span>
          <button class="btn-primary" data-new>${icon("plus", 15)}<span>New collection</span></button>
        </div>`;
    }
    return `<div class="collections-list">${cols.map((c, i) => renderRow(c, i)).join("")}</div>`;
  };

  const renderRow = (c: CollectionInfo, i: number): string => {
    const isActive = c.active || c.id === result?.active_id;
    const isOpen = expanded.has(c.id);
    const busy = switchBusy === c.id || deleteBusy === c.id;
    return `
      <div class="collection-item">
        <div class="collection-row${isActive ? " is-active" : ""}">
          <div class="collection-info">
            <span class="collection-name">${escapeHtml(c.name)}</span>
            ${isActive ? `<span class="collections-chip is-ok">${icon("check", 11)}<span>Active</span></span>` : ""}
            <span class="collection-count tnum">${c.addon_count} addon${c.addon_count === 1 ? "" : "s"}</span>
          </div>
          <div class="collection-actions">
            ${
              isActive
                ? `<span class="collection-current">Currently active</span>`
                : `<button class="btn-primary collections-btn-small" data-switch="${i}" ${busy ? "disabled" : ""}>
                    ${switchBusy === c.id ? `<span class="collections-spin"></span>` : icon("refresh", 13)}
                    <span>${switchBusy === c.id ? "Switching…" : "Switch"}</span>
                  </button>`
            }
            <button class="btn-secondary collections-btn-small" data-expand="${i}" aria-expanded="${isOpen}" ${busy ? "disabled" : ""}>
              ${icon(isOpen ? "chevron-down" : "chevron-right", 13)}
              <span>${isOpen ? "Hide" : "Details"}</span>
            </button>
            <button class="btn-danger collections-btn-small" data-delete="${i}" ${busy ? "disabled" : ""}>
              ${deleteBusy === c.id ? `<span class="collections-spin"></span>` : icon("trash", 13)}
              <span>${deleteBusy === c.id ? "Deleting…" : "Delete"}</span>
            </button>
          </div>
        </div>
        ${isOpen ? renderDetail(c.id) : ""}
      </div>`;
  };

  const renderDetail = (id: string): string => {
    const d = details.get(id);
    return `
      <div class="collection-detail">
        <div class="collection-detail-head">
          <h3 class="collection-detail-title">Addons${d ? ` · ${d.addons.length}` : ""}</h3>
          <span class="collection-detail-hint">Per-addon toggle — saved to this collection only</span>
        </div>
        ${
          !d
            ? `<div class="collections-loading"><span class="collections-spin"></span><span>Loading addons…</span></div>`
            : d.addons.length === 0
              ? `<p class="collection-detail-empty">No addons captured in this collection yet.</p>`
              : `<div class="collection-addons">${d.addons.map((a) => renderAddonRow(id, a)).join("")}</div>`
        }
      </div>`;
  };

  const renderAddonRow = (id: string, a: CollectionAddonState): string => `
    <div class="collection-addon">
      <span class="collection-addon-folder mono">${escapeHtml(a.folder)}</span>
      <span class="collections-chip${a.enabled ? " is-ok" : ""}">${a.enabled ? "enabled" : "disabled"}</span>
      <button class="collections-switch${a.enabled ? " is-on" : ""}" role="switch" aria-checked="${a.enabled}"
        aria-label="${escapeAttr(a.folder)}: ${a.enabled ? "enabled" : "disabled"}"
        data-toggle data-col="${escapeAttr(id)}" data-folder="${escapeAttr(a.folder)}"
        ${toggleBusy ? "disabled" : ""}>
        <span class="switch-knob"></span>
      </button>
    </div>`;

  const renderExportCard = (): string => {
    const cols = result?.collections ?? [];
    const options = [
      `<option value="">Current on-disk state</option>`,
      ...cols.map(
        (c) =>
          `<option value="${escapeAttr(c.id)}"${c.id === collectionID ? " selected" : ""}>${escapeHtml(c.name)}</option>`,
      ),
    ].join("");
    return `
      <div class="transfer-card">
        <div class="transfer-card-head">
          <h3 class="transfer-title">Export</h3>
          <p class="transfer-sub">Bundle a collection — optionally with saved variables — into a portable ZIP.</p>
        </div>
        <div class="field">
          <label class="field-label" for="col-export-out">Output file</label>
          <div class="field-row">
            <input class="text-input mono" id="col-export-out" type="text" placeholder="C:\\path\\to\\export.zip"
              spellcheck="false" autocomplete="off" value="${escapeAttr(outPath)}" data-out ${exportBusy ? "disabled" : ""} />
            ${!mockActive ? `<button class="btn-secondary collections-btn-small" data-browse-out ${exportBusy ? "disabled" : ""}>${icon("folder", 13)}<span>Choose…</span></button>` : ""}
          </div>
        </div>
        <div class="field">
          <label class="field-label" for="col-export-src">Collection</label>
          <select class="text-input" id="col-export-src" data-collection ${exportBusy ? "disabled" : ""}>
            ${options}
          </select>
        </div>
        <label class="check-row">
          <input type="checkbox" data-savedvars ${includeSavedVars ? "checked" : ""} ${exportBusy ? "disabled" : ""} />
          <span class="check-box">${icon("check", 12)}</span>
          <span class="check-text">Include saved variables<span class="check-hint">Bundles the active account's SavedVariables.</span></span>
        </label>
        <div class="transfer-actions">
          <button class="btn-primary" data-export ${exportBusy || !outPath.trim() ? "disabled" : ""}>
            ${exportBusy ? `<span class="collections-spin"></span>` : icon("download", 15)}
            <span>${exportBusy ? "Exporting…" : "Export"}</span>
          </button>
          ${exportResult ? `<span class="transfer-result mono">${escapeHtml(exportResult.out)} · ${exportResult.addons} addon${exportResult.addons === 1 ? "" : "s"}</span>` : ""}
        </div>
        ${exportError ? renderError(exportError, false) : ""}
      </div>`;
  };

  const renderImportCard = (): string => `
    <div class="transfer-card">
      <div class="transfer-card-head">
        <h3 class="transfer-title">Import</h3>
        <p class="transfer-sub">Install from a wowfix export file, a provider URL, or an owner/repo source.</p>
      </div>
      <div class="field">
        <label class="field-label" for="col-import-src">Path or URL</label>
        <div class="field-row">
          <input class="text-input" id="col-import-src" type="text" placeholder="C:\\path\\to\\export.zip · github.com/owner/repo · https://…"
            spellcheck="false" autocomplete="off" value="${escapeAttr(importSrc)}" data-import-path ${importBusy ? "disabled" : ""} />
          ${!mockActive ? `<button class="btn-secondary collections-btn-small" data-browse-in ${importBusy ? "disabled" : ""}>${icon("folder", 13)}<span>Choose…</span></button>` : ""}
        </div>
      </div>
      <div class="transfer-actions">
        <button class="btn-primary" data-import ${importBusy || !importSrc.trim() ? "disabled" : ""}>
          ${importBusy ? `<span class="collections-spin"></span>` : icon("plus", 15)}
          <span>${importBusy ? "Importing…" : "Import"}</span>
        </button>
      </div>
      ${
        importResult
          ? importResult.installed.length > 0
            ? `<div class="transfer-installed"><span class="transfer-installed-label">Installed:</span>${importResult.installed.map((n) => `<span class="collections-chip is-ok">${escapeHtml(n)}</span>`).join("")}</div>`
            : `<p class="transfer-result">Nothing new was installed — the addons may already be present.</p>`
          : ""
      }
      ${importError ? renderError(importError, false) : ""}
    </div>`;

  const renderError = (msg: string, withRetry: boolean): string => `
    <div class="collections-error" role="alert">
      ${icon("alert", 15)}
      <div class="collections-error-body">
        <p>${escapeHtml(msg)}</p>
        ${withRetry ? `<button class="btn-secondary collections-btn-small" data-retry>${icon("refresh", 13)}<span>Retry</span></button>` : ""}
      </div>
    </div>`;

  const bindEvents = (cols: CollectionInfo[]): void => {
    host.querySelectorAll<HTMLElement>("[data-new]").forEach((btn) =>
      btn.addEventListener("click", () => {
        creating = !creating;
        pendingFocus = creating ? null : "btn:new";
        rerender();
        if (creating) host.querySelector<HTMLInputElement>("[data-name]")?.focus();
      }),
    );
    host.querySelector<HTMLInputElement>("[data-name]")?.addEventListener("keydown", (e) => {
      const input = e.target as HTMLInputElement;
      if (e.key === "Enter") {
        e.preventDefault();
        const name = input.value.trim();
        if (name) void create(name);
      } else if (e.key === "Escape") {
        creating = false;
        pendingFocus = "btn:new";
        rerender();
      }
    });
    host.querySelector("[data-create]")?.addEventListener("click", () => {
      const name = host.querySelector<HTMLInputElement>("[data-name]")?.value.trim() ?? "";
      if (name) void create(name);
    });
    host.querySelector("[data-cancel-create]")?.addEventListener("click", () => {
      creating = false;
      pendingFocus = "btn:new";
      rerender();
    });
    host.querySelectorAll<HTMLElement>("[data-retry]").forEach((btn) =>
      btn.addEventListener("click", () => void load()),
    );
    host.querySelectorAll<HTMLElement>("[data-switch]").forEach((btn) => {
      const c = cols[Number(btn.dataset.switch)];
      if (!c) return;
      btn.addEventListener("click", () => {
        pendingFocus = `btn:switch:${btn.dataset.switch}`;
        void switchTo(c.id);
      });
    });
    host.querySelectorAll<HTMLElement>("[data-expand]").forEach((btn) => {
      const c = cols[Number(btn.dataset.expand)];
      if (!c) return;
      btn.addEventListener("click", () => {
        if (expanded.has(c.id)) expanded.delete(c.id);
        else {
          expanded.add(c.id);
          ensureDetail(c.id);
        }
        pendingFocus = `btn:expand:${btn.dataset.expand}`;
        rerender();
      });
    });
    host.querySelectorAll<HTMLElement>("[data-delete]").forEach((btn) => {
      const c = cols[Number(btn.dataset.delete)];
      if (!c) return;
      btn.addEventListener("click", () => {
        pendingFocus = `btn:delete:${btn.dataset.delete}`;
        void remove(c.id);
      });
    });
    host.querySelectorAll<HTMLElement>("[data-toggle]").forEach((btn) => {
      const colId = btn.dataset.col ?? "";
      const folder = btn.dataset.folder ?? "";
      const a = details.get(colId)?.addons.find((x) => x.folder === folder);
      if (!a) return;
      btn.addEventListener("click", () => {
        pendingFocus = `btn:toggle:${colId}:${folder}`;
        void toggleAddon(colId, folder, a.enabled);
      });
    });
    host.querySelector<HTMLInputElement>("[data-out]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      outPath = input.value;
      refocus = { sel: "[data-out]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    host.querySelector<HTMLSelectElement>("[data-collection]")?.addEventListener("change", (e) => {
      collectionID = (e.target as HTMLSelectElement).value;
      refocus = { sel: "[data-collection]", pos: -1 };
      rerender();
    });
    host.querySelector<HTMLInputElement>("[data-savedvars]")?.addEventListener("change", (e) => {
      includeSavedVars = (e.target as HTMLInputElement).checked;
      rerender();
    });
    host.querySelector("[data-browse-out]")?.addEventListener("click", () => void pickOutPath());
    host.querySelector("[data-export]")?.addEventListener("click", () => {
      pendingFocus = "btn:export";
      void doExport();
    });
    host.querySelector<HTMLInputElement>("[data-import-path]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      importSrc = input.value;
      refocus = { sel: "[data-import-path]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    host.querySelector<HTMLInputElement>("[data-import-path]")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        void doImport();
      }
    });
    host.querySelector("[data-browse-in]")?.addEventListener("click", () => void pickOpenPath());
    host.querySelector("[data-import]")?.addEventListener("click", () => {
      pendingFocus = "btn:import";
      void doImport();
    });
  };

  void load();
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
