// Export / Import view — collection export to a file (optionally with saved
// variables) and import from a path or URL. Import is gated behind a
// confirmation dialog; both sections surface errors inline.

import type { AppState, Actions } from "../app";
import type { CollectionInfo, ExportResult, ImportResult } from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
import { confirmDialog } from "../components/dialog";

export function mountExportImport(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let collections: CollectionInfo[] = [];
  let collectionsError = "";
  let busy: string | null = null; // "export" | "import"
  let exportResult: ExportResult | null = null;
  let importResult: ImportResult | null = null;
  let error: string | null = null;
  let outPath = "";
  let collectionID = "";
  let includeSavedVars = false;
  let importPath = "";
  let refocus: { sel: string; pos: number } | null = null;

  const rerender = (): void => {
    render();
    if (!refocus) return;
    const target = el.querySelector<HTMLInputElement>(refocus.sel);
    if (target) {
      target.focus();
      if (refocus.pos >= 0) {
        const pos = Math.min(refocus.pos, target.value.length);
        target.setSelectionRange(pos, pos);
      }
    }
    refocus = null;
  };

  const loadCollections = async (): Promise<void> => {
    try {
      const res = await service.Collections();
      collections = res.collections;
      collectionsError = "";
    } catch (err) {
      collectionsError = errText(err);
      collections = [];
    }
    rerender();
  };

  const doExport = async (): Promise<void> => {
    if (busy) return;
    busy = "export";
    error = null;
    exportResult = null;
    rerender();
    try {
      const res = await service.ExportCollection(
        outPath.trim(),
        collectionID,
        includeSavedVars,
      );
      exportResult = res;
      toast({
        type: "ok",
        title: "Export complete",
        message: `Exported ${res.addons} addon${res.addons === 1 ? "" : "s"} to ${res.out}`,
      });
    } catch (err) {
      error = `Export failed: ${errText(err)}`;
    } finally {
      busy = null;
      rerender();
    }
  };

  const doImport = async (): Promise<void> => {
    const src = importPath.trim();
    if (busy || !src) return;
    const confirmed = await confirmDialog({
      title: "Import addons?",
      message: `Addons from ${src} are installed into the current AddOns folder.`,
      confirmLabel: "Import",
    });
    if (!confirmed) return;
    busy = "import";
    error = null;
    importResult = null;
    rerender();
    try {
      const res = await service.ImportCollection(src);
      importResult = res;
      toast({
        type: res.installed.length > 0 ? "ok" : "info",
        title: "Import complete",
        message: `Installed ${res.installed.length} addon${res.installed.length === 1 ? "" : "s"}`,
      });
    } catch (err) {
      error = `Import failed: ${errText(err)}`;
    } finally {
      busy = null;
      rerender();
    }
  };

  const render = (): void => {
    const exporting = busy === "export";
    const importing = busy === "import";
    const collectionOptions = [
      `<option value="">Current on-disk state</option>`,
      ...collections.map(
        (c) =>
          `<option value="${escapeAttr(c.id)}" ${c.id === collectionID ? "selected" : ""}>${escapeHtml(c.name)}</option>`,
      ),
    ].join("");

    el.innerHTML = `
      <div class="exportimport">
        <h2 class="detail-title">Export</h2>
        <div class="field exportimport-field">
          <label class="field-label" for="export-out">Output file</label>
          <div class="input-row">
            <span class="input-icon">${icon("download", 15)}</span>
            <input class="input" id="export-out" type="text" placeholder="C:\\path\\to\\export.zip"
              spellcheck="false" autocomplete="off" value="${escapeAttr(outPath)}" data-out ${exporting ? "disabled" : ""} />
          </div>
        </div>
        <div class="field exportimport-field">
          <label class="field-label" for="export-collection">Collection</label>
          <div class="input-row">
            <span class="input-icon">${icon("stack", 15)}</span>
            <select class="input" id="export-collection" data-collection ${exporting ? "disabled" : ""}>
              ${collectionOptions}
            </select>
          </div>
          ${collectionsError ? `<p class="field-hint">${escapeHtml(collectionsError)}</p>` : ""}
        </div>
        <label class="checkbox-row exportimport-check">
          <input type="checkbox" class="checkbox" data-savedvars ${includeSavedVars ? "checked" : ""} ${exporting ? "disabled" : ""} />
          <span class="checkbox-box">${icon("check", 13)}</span>
          <span class="checkbox-text">
            <span>Include saved variables</span>
            <span class="checkbox-hint">Bundles the selected account's SavedVariables files.</span>
          </span>
        </label>
        <div class="exportimport-row">
          <button class="btn btn-primary" data-export ${exporting ? "disabled" : ""}>
            ${exporting ? `<span class="spinner"></span>` : icon("download", 15)}
            <span>${exporting ? "Exporting…" : "Export"}</span>
          </button>
          ${
            exportResult
              ? `<span class="muted mono">${escapeHtml(exportResult.out)}, ${exportResult.addons} addon${exportResult.addons === 1 ? "" : "s"}${exportResult.collection ? ` · collection ${escapeHtml(exportResult.collection)}` : ""}</span>`
              : ""
          }
        </div>

        <h2 class="detail-title exportimport-import-title">Import</h2>
        <div class="field exportimport-field">
          <label class="field-label" for="import-src">Path or URL</label>
          <div class="input-row">
            <span class="input-icon">${icon("upload", 15)}</span>
            <input class="input" id="import-src" type="text"
              placeholder="Path to a wowfix export, or a provider URL / owner-repo…"
              spellcheck="false" autocomplete="off" value="${escapeAttr(importPath)}" data-import-path ${importing ? "disabled" : ""} />
          </div>
          <p class="field-hint">Exports from this app (ZIP), provider URLs and owner/repo sources are accepted.</p>
        </div>
        <div class="exportimport-row">
          <button class="btn btn-primary" data-import ${importing || !importPath.trim() ? "disabled" : ""}>
            ${importing ? `<span class="spinner"></span>` : icon("upload", 15)}
            <span>${importing ? "Importing…" : "Import"}</span>
          </button>
        </div>
        ${
          importResult && importResult.installed.length > 0
            ? `<div class="exportimport-installed">
                <span class="muted">Installed:</span>
                ${importResult.installed.map((n) => `<span class="tag tag-ok">${escapeHtml(n)}</span>`).join("")}
              </div>`
            : importResult && importResult.installed.length === 0
              ? `<p class="muted exportimport-installed">Nothing new was installed. The addon may already be present.</p>`
              : ""
        }

        ${error ? `<p class="settings-error" role="alert" style="color: var(--error)">${icon("x-circle", 14)}<span>${escapeHtml(error)}</span></p>` : ""}
      </div>`;

    el.querySelector<HTMLInputElement>("[data-out]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      outPath = input.value;
      refocus = { sel: "[data-out]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    el.querySelector<HTMLSelectElement>("[data-collection]")?.addEventListener("change", (e) => {
      collectionID = (e.target as HTMLSelectElement).value;
      rerender();
    });
    el.querySelector<HTMLInputElement>("[data-savedvars]")?.addEventListener("change", (e) => {
      includeSavedVars = (e.target as HTMLInputElement).checked;
      rerender();
    });
    el.querySelector("[data-export]")?.addEventListener("click", () => void doExport());
    el.querySelector<HTMLInputElement>("[data-import-path]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      importPath = input.value;
      refocus = { sel: "[data-import-path]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    el.querySelector<HTMLInputElement>("[data-import-path]")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        void doImport();
      }
    });
    el.querySelector("[data-import]")?.addEventListener("click", () => void doImport());
  };

  render();
  void loadCollections();

  return { refresh: rerender };
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
