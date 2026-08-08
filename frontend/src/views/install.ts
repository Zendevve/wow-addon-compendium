// ZIP install: drop zone + browse button, replace toggle, progress and a
// result summary with a re-scan hook.

import type { AppState, Actions } from "../app";
import { icon, type IconName } from "../icons";
import { mockActive } from "../api";
import { toast } from "../components/toast";

export function mountInstall(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let allowReplace = false;
  let pendingFile = "";

  const render = (): void => {
    if (!app.state.has_install) {
      el.innerHTML = emptyCard(
        "folder",
        "No WoW install configured",
        "Set up your World of Warcraft path before installing addons.",
        `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
      );
      el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
      return;
    }

    const busy = app.busy === "install";
    const result = app.installResult;
    const dest = app.state.addons_dir || app.state.wow_path;

    el.innerHTML = `
      <div class="install">
        <div class="dropzone ${busy ? "busy" : ""}" data-zone tabindex="0" role="button"
          aria-label="Drop an addon ZIP archive here to install it">
          <span class="dropzone-icon">${icon(busy ? "refresh" : "package", 34)}</span>
          <h2 class="dropzone-title">${busy ? `Installing ${escapeHtml(basename(pendingFile))}…` : "Drop addon ZIP here"}</h2>
          <p class="dropzone-sub">${busy ? "Extracting, flattening and validating…" : "or"}</p>
          <div class="dropzone-browse">
            <button class="btn btn-primary" data-browse ${busy ? "disabled" : ""}>${icon("folder", 16)}<span>Browse…</span></button>
          </div>
          <input type="file" class="file-input" accept=".zip,application/zip,application/x-zip-compressed" hidden />
        </div>

        <label class="checkbox-row">
          <input type="checkbox" class="checkbox" data-replace ${busy ? "disabled" : ""} />
          <span class="checkbox-box">${icon("check", 13)}</span>
          <span class="checkbox-text">
            <span>Allow replacing existing addons</span>
            <span class="checkbox-hint">An existing folder is backed up, then replaced by the archive contents.</span>
          </span>
        </label>

        <p class="install-dest mono">${icon("folder", 13)} Installing into ${escapeHtml(dest)}</p>

        ${
          result
            ? `<div class="install-result" role="status">
                <div class="result-tiles">
                  <div class="result-tile tile-ok"><span class="tile-num">${result.installed}</span><span class="tile-label">Installed</span></div>
                  <div class="result-tile tile-replaced"><span class="tile-num">${result.replaced}</span><span class="tile-label">Replaced</span></div>
                  <div class="result-tile tile-warn"><span class="tile-num">${result.skipped}</span><span class="tile-label">Skipped</span></div>
                  <div class="result-tile ${result.errors.length ? "tile-error" : "tile-muted"}"><span class="tile-num">${result.errors.length}</span><span class="tile-label">Errors</span></div>
                </div>
                ${
                  result.errors.length
                    ? `<ul class="result-errors">${result.errors
                        .map((e) => `<li>${icon("x-circle", 14)}<span>${escapeHtml(e)}</span></li>`)
                        .join("")}</ul>`
                    : ""
                }
                <div class="result-actions">
                  <button class="btn btn-primary" data-rescan>${icon("refresh", 16)}<span>Scan addons now</span></button>
                  <span class="result-hint">New addons appear in the scan list.</span>
                </div>
              </div>`
            : ""
        }
      </div>`;

    const zone = el.querySelector<HTMLElement>("[data-zone]")!;
    const fileInput = el.querySelector<HTMLInputElement>(".file-input")!;

    el.querySelector("[data-browse]")?.addEventListener("click", async () => {
      // Prefer the Wails v2 native dialog (returns a real filesystem path);
      // fall back to the HTML file input when not running under Wails.
      const nativePath = await pickZipPath();
      if (nativePath) startInstallPath(nativePath);
      else fileInput.click();
    });
    fileInput.addEventListener("change", () => {
      const f = fileInput.files?.[0];
      if (f) startInstall(f);
      fileInput.value = "";
    });

    zone.addEventListener("click", (e) => {
      if (e.target instanceof HTMLElement && e.target.closest("button")) return;
      if (!busy) fileInput.click();
    });
    zone.addEventListener("keydown", (e) => {
      if ((e.key === "Enter" || e.key === " ") && !busy) {
        e.preventDefault();
        fileInput.click();
      }
    });
    zone.addEventListener("dragover", (e) => {
      e.preventDefault();
      zone.classList.add("dragover");
    });
    zone.addEventListener("dragleave", () => zone.classList.remove("dragover"));
    zone.addEventListener("drop", (e) => {
      e.preventDefault();
      zone.classList.remove("dragover");
      const f = e.dataTransfer?.files?.[0];
      if (f) startInstall(f);
    });

    el.querySelector("[data-replace]")?.addEventListener("change", (e) => {
      allowReplace = (e.target as HTMLInputElement).checked;
    });
    el.querySelector("[data-rescan]")?.addEventListener("click", () => {
      void actions.scan().then(() => actions.go("scan"));
    });
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
  };

  const startInstall = (file: File): void => {
    // Wails v2 patches dropped/selected File objects with a real `path`
    // property in some versions; browsers never do. Without a real path the
    // backend has nothing to read, so fail loudly instead of passing a bare
    // filename. Mock mode accepts the name so browser demos keep working.
    const path = zipPathOf(file);
    if (!path) {
      toast({
        type: "error",
        title: "Could not resolve file path",
        message: "This build cannot read the file location. Use Browse… instead.",
      });
      return;
    }
    pendingFile = path;
    void startZipInstall(actions, path, allowReplace);
  };

  const startInstallPath = (path: string): void => {
    pendingFile = path;
    void startZipInstall(actions, path, allowReplace);
  };

  render();
  return { refresh: render };
}

// Wails v2 exposes the native file dialog on the JS runtime; returns the
// chosen path or null when unavailable or cancelled.
export function pickZipPath(): Promise<string | null> {
  const rt = (window as unknown as {
    runtime?: { OpenFileDialog?: (opts: unknown) => Promise<string | null> };
  }).runtime;
  if (!rt?.OpenFileDialog) return Promise.resolve(null);
  return rt
    .OpenFileDialog({
      filters: [{ displayName: "ZIP archives", pattern: "*.zip" }],
    })
    .then((p) => p || null)
    .catch(() => null);
}

// Resolve the installable path for a picked/dropped File. Wails v2 patches
// File objects with a real `path` property in some versions; browsers never
// do. Mock mode accepts the bare name so browser demos keep working.
// Returns "" when nothing readable was resolved.
export function zipPathOf(file: File): string {
  // Wails v2 patches File with a real `path` prop; browsers never do.
  const wailsFile = file as File & { path?: string };
  return wailsFile.path || (mockActive ? file.name : "");
}

// Start a ZIP install from a resolved filesystem path. Resolves when the
// store finishes; the outcome lands in `app.installResult`.
export function startZipInstall(
  actions: Actions,
  path: string,
  allowReplace = false,
): Promise<void> {
  return actions.installZip(path, allowReplace);
}

// Start a ZIP install from a File (drop or HTML file input). Resolves the
// path via zipPathOf, toasts when unresolvable, and resolves to false when
// nothing was started.
export function startZipInstallFromFile(
  actions: Actions,
  file: File,
  allowReplace = false,
): Promise<boolean> {
  const path = zipPathOf(file);
  if (!path) {
    toast({
      type: "error",
      title: "Could not resolve file path",
      message: "This build cannot read the file location. Use Browse… instead.",
    });
    return Promise.resolve(false);
  }
  return startZipInstall(actions, path, allowReplace).then(() => true);
}

function basename(p: string): string {
  return p.split(/[\\/]/).pop() ?? p;
}

function emptyCard(glyph: IconName, title: string, sub: string, cta: string): string {
  return `<div class="empty">
    <span class="empty-icon">${icon(glyph, 28)}</span>
    <h2 class="empty-title">${title}</h2>
    <p class="empty-sub">${sub}</p>
    <div class="empty-actions">${cta}</div>
  </div>`;
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESC[c]);
}
const ESC: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};
