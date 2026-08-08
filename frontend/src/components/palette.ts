// Command palette (Ctrl+K / Ctrl+P): keyboard escape hatch for navigation
// and primary actions. A modal overlay (same backdrop language as dialog.ts)
// with a filter input and a list of real buttons; roving focus, no mouse
// path required. Styling lives in components/palette.css.

import type { AppState, Actions } from "../app";
import type { View } from "../types";
import { icon } from "../icons";

export interface PaletteEntry {
  id: string;
  label: string;
  group: "Navigate" | "Actions";
  hint?: string;
  keywords: string[];
  /** Rendered disabled; hint should explain why (e.g. "Run a scan first"). */
  disabled?: boolean;
  run(): void;
}

export interface PaletteHandle {
  /** Toggles the palette: open() closes it when already open. */
  open(): void;
}

const root = document.createElement("div");
root.className = "palette-root";

export function mountPalette(
  host: HTMLElement,
  app: AppState,
  actions: Actions,
): PaletteHandle {
  host.appendChild(root);

  const backdrop = document.createElement("div");
  backdrop.className = "palette-backdrop";
  const panel = document.createElement("div");
  panel.className = "palette";
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-modal", "true");
  panel.setAttribute("aria-label", "Command palette");
  panel.innerHTML = `
    <div class="palette-input-row">
      ${icon("search", 16, "palette-input-icon")}
      <input class="palette-input" type="text" aria-label="Search actions" placeholder="Search actions" spellcheck="false" autocomplete="off" />
    </div>
    <div class="palette-results" aria-label="Results"></div>
    <div class="palette-footer">
      <span class="palette-keys">↑↓</span> navigate
      <span class="palette-keys">Enter</span> run
      <span class="palette-keys">Esc</span> close
    </div>`;
  backdrop.appendChild(panel);
  const input = panel.querySelector<HTMLInputElement>(".palette-input")!;
  const resultsEl = panel.querySelector<HTMLElement>(".palette-results")!;

  let entries: PaletteEntry[] = [];
  let visible: PaletteEntry[] = [];
  let trigger: HTMLElement | null = null;

  // --- registry ------------------------------------------------------------

  // Built per open so every entry reflects current state (install present,
  // scan results, in-flight operation). Entries without an app-level action
  // navigate to the view that hosts the workflow — the palette routes by
  // keyboard, it never re-implements view behaviour.
  const buildEntries = (): PaletteEntry[] => {
    const hasInstall = app.state.has_install;
    const busy = app.busy !== null;
    const fixable = app.scan
      ? app.scan.addons.filter((a) => a.fixable).length
      : 0;

    const nav = (
      view: View,
      label: string,
      hint: string,
      keywords: string[],
    ): PaletteEntry => ({
      id: `nav-${view}`,
      label,
      group: "Navigate",
      hint,
      keywords,
      run: () => actions.go(view),
    });

    const entries: PaletteEntry[] = [
      nav("overview", "Overview", "Health workflows", ["home", "start"]),
      nav("updates", "Updates", "Check for available updates", ["refresh", "update"]),
      nav("catalog", "Catalog", "Browse and search addons", ["search", "browse"]),
      nav("collections", "Collections", "Manage addon collections", ["sets", "groups"]),
      nav("backups", "Backups", "Snapshots and restores", ["snapshot", "restore", "archive"]),
      nav("settings", "Settings", "Profiles and preferences", ["options", "config", "preferences"]),
      {
        id: "scan",
        label: "Scan now",
        group: "Actions",
        hint: !hasInstall
          ? "Set up your WoW install first"
          : busy
            ? "An operation is running"
            : "Re-scan all addons",
        keywords: ["rescan", "refresh", "health", "issues"],
        disabled: !hasInstall || busy,
        run: () => void actions.scan(),
      },
      {
        id: "fix-all",
        label: "Fix all problems",
        group: "Actions",
        hint: !app.scan
          ? "Run a scan first"
          : fixable === 0
            ? "Nothing to fix right now"
            : `${fixable} fixable problem${fixable === 1 ? "" : "s"}`,
        keywords: ["repair", "resolve", "fix", "issues"],
        disabled: !hasInstall || !app.scan || fixable === 0 || busy,
        run: () => void actions.fixAll(),
      },
      {
        id: "diagnostics",
        label: "Run diagnostics",
        group: "Actions",
        hint: "Open Doctor checks",
        keywords: ["doctor", "check", "health"],
        disabled: !hasInstall || busy,
        run: () => actions.go("doctor"),
      },
      {
        id: "validate",
        label: "Validate addons",
        group: "Actions",
        hint: "Check addon metadata",
        keywords: ["validation", "check", "metadata"],
        disabled: !hasInstall || busy,
        run: () => void actions.validate(),
      },
      {
        id: "check-updates",
        label: "Check for updates",
        group: "Actions",
        hint: "Open Updates and check",
        keywords: ["check", "refresh", "update"],
        disabled: !hasInstall,
        run: () => actions.go("updates"),
      },
      {
        id: "update-all",
        label: "Update all addons",
        group: "Actions",
        hint: "Open Updates to apply",
        keywords: ["update", "upgrade", "apply", "refresh"],
        disabled: !hasInstall,
        run: () => actions.go("updates"),
      },
      {
        id: "backup",
        label: "Create backup",
        group: "Actions",
        hint: "Open Backups to snapshot",
        keywords: ["snapshot", "save", "archive"],
        disabled: !hasInstall,
        run: () => actions.go("backups"),
      },
      {
        id: "refresh",
        label: "Refresh current view",
        group: "Actions",
        hint: "Re-render the current view",
        keywords: ["reload", "refresh", "rerender", "redraw"],
        run: () => actions.go(app.view),
      },
    ];

    if (!hasInstall) {
      entries.push({
        id: "setup",
        label: "Go to setup",
        group: "Actions",
        hint: "Configure your WoW install",
        keywords: ["install", "configure", "wizard", "path"],
        run: () => actions.go("setup"),
      });
    }
    return entries;
  };

  // --- rendering ------------------------------------------------------------

  const matches = (e: PaletteEntry, q: string): boolean =>
    `${e.label} ${e.hint ?? ""} ${e.keywords.join(" ")}`
      .toLowerCase()
      .includes(q);

  const itemHtml = (e: PaletteEntry, i: number): string => `
    <button class="palette-item${e.disabled ? " is-disabled" : ""}" type="button" data-index="${i}"
      tabindex="-1" ${e.disabled ? "disabled" : ""}>
      <span class="palette-item-label">${escapeHtml(e.label)}</span>
      ${e.hint ? `<span class="palette-item-hint">${escapeHtml(e.hint)}</span>` : ""}
    </button>`;

  const renderResults = (): void => {
    const q = input.value.trim().toLowerCase();
    visible =
      q === ""
        ? entries
        : entries.filter((e) => matches(e, q));
    if (visible.length === 0) {
      resultsEl.innerHTML = `<div class="palette-empty">No actions found</div>`;
      return;
    }
    let html = "";
    let group: string | null = null;
    visible.forEach((e, i) => {
      if (e.group !== group) {
        group = e.group;
        html += `<div class="palette-group"><div class="palette-group-label">${group}</div>`;
      }
      html += itemHtml(e, i);
    });
    html += "</div>";
    resultsEl.innerHTML = html;
  };

  // --- focus / keyboard -----------------------------------------------------

  // Enabled result buttons in DOM order — matches `visible` via data-index.
  const items = (): HTMLButtonElement[] =>
    Array.from(
      resultsEl.querySelectorAll<HTMLButtonElement>(".palette-item:not([disabled])"),
    );

  const focusAt = (i: number): void => {
    const list = items();
    if (list.length === 0) return;
    const idx = ((i % list.length) + list.length) % list.length;
    list[idx].focus();
  };

  const runFocused = (btn: HTMLButtonElement): void => {
    const entry = visible[Number(btn.dataset.index)];
    if (!entry || entry.disabled) return;
    close();
    entry.run();
  };

  function onKey(e: KeyboardEvent): void {
    if (e.key === "Escape") {
      e.preventDefault();
      close();
      return;
    }
    if (e.key === "Tab") {
      // Lightweight focus trap: Tab / Shift+Tab cycle input and results.
      e.preventDefault();
      const list: HTMLElement[] = [input, ...items()];
      const i = list.indexOf(document.activeElement as HTMLElement);
      if (i === -1) {
        input.focus();
        return;
      }
      const next = ((i + (e.shiftKey ? -1 : 1)) % list.length + list.length) % list.length;
      list[next].focus();
      return;
    }
    const list = items();
    const active = document.activeElement;
    if (e.key === "Enter" && active === input) {
      // No focused result yet: Enter runs the first enabled match.
      e.preventDefault();
      if (list.length > 0) list[0].click();
      return;
    }
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      if (active === input) {
        // From the input, arrows jump straight into the results: Down to the
        // first match, Up to the last.
        e.preventDefault();
        focusAt(e.key === "ArrowDown" ? 0 : -1);
        return;
      }
      const i = list.indexOf(active as HTMLButtonElement);
      if (i !== -1) {
        e.preventDefault();
        focusAt(i + (e.key === "ArrowDown" ? 1 : -1));
      }
    }
  }

  const close = (): void => {
    if (!root.firstChild) return;
    document.removeEventListener("keydown", onKey);
    root.replaceChildren();
    // Focus returns to whatever opened the palette, like dialog.ts.
    if (trigger && trigger.isConnected) trigger.focus();
  };

  // --- events ---------------------------------------------------------------

  input.addEventListener("input", renderResults);
  resultsEl.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>(".palette-item");
    if (btn) runFocused(btn);
  });
  backdrop.addEventListener("click", (e) => {
    if (e.target === backdrop) close();
  });

  const open = (): void => {
    if (root.firstChild) {
      close(); // Ctrl+K toggles: close when already open.
      return;
    }
    // Another modal (e.g. a confirm dialog) owns the keyboard and its focus
    // trap; don't stack a second modal on top of it.
    if (document.querySelector('[role="dialog"][aria-modal="true"]')) return;
    trigger = document.activeElement as HTMLElement | null;
    entries = buildEntries();
    input.value = "";
    renderResults();
    root.appendChild(backdrop);
    document.addEventListener("keydown", onKey);
    window.setTimeout(() => input.focus(), 0);
  };

  return { open };
}

const ESC_HTML: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};
function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESC_HTML[c]);
}
