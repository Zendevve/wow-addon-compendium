// Saved variables view — per-account WTF\Account\SavedVariables files.
// List / back up / restore / reset / migrate, with the destructive
// operations gated behind confirmDialog and results surfaced via toast.

import type { AppState, Actions } from "../app";
import type { SavedVarsListResult } from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
import { confirmDialog } from "../components/dialog";

export function mountSavedVars(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let accounts: string[] = [];
  let selected = "";
  let listResult: SavedVarsListResult | null = null;
  let loading = false;
  let busy: string | null = null; // label of the in-flight operation
  let lastCopied: string[] | null = null;
  // Accounts that have been auto-backed-up this session (per mount).
  const autoBackedUp = new Set<string>();
  // Text input values survive re-renders through these mirrors.
  let restorePath = "";
  let resetAddon = "";
  let migrateFrom = "";
  let migrateTo = "";
  let migrateAddon = "";
  // Advanced operations disclosure state
  let advancedOpen = false;

  // Re-renders rebuild the view DOM; these hold the control to re-focus
  // (and, for text inputs, the caret position) after the rebuild.
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

  const loadAccounts = async (): Promise<void> => {
    loading = true;
    rerender();
    try {
      accounts = await service.SavedVarsAccounts();
      if (!accounts.includes(selected)) {
        selected = accounts[0] ?? "";
        migrateFrom = selected;
        if (accounts[1]) migrateTo = accounts[1];
      }
    } catch (err) {
      toast({
        type: "error",
        title: "Could not load accounts",
        message: errText(err),
      });
    } finally {
      loading = false;
      rerender();
    }
  };

  const listFiles = async (): Promise<void> => {
    if (busy || !selected) return;
    busy = "list";
    rerender();
    try {
      listResult = await service.SavedVarsList(selected);
      // Auto-backup on first successful list per account per session.
      if (!autoBackedUp.has(selected)) {
        busy = null;
        rerender();
        if (await backup()) {
          autoBackedUp.add(selected);
        }
      }
    } catch (err) {
      toast({
        type: "error",
        title: "Could not list saved variables",
        message: errText(err),
      });
    } finally {
      busy = null;
      rerender();
    }
  };

  const backup = async (): Promise<boolean> => {
    if (busy || !selected) return false;
    busy = "backup";
    rerender();
    try {
      const res = await service.SavedVarsBackup(selected);
      toast({
        type: "ok",
        title: "Saved variables backed up",
        message: res.path,
      });
      return true;
    } catch (err) {
      toast({
        type: "error",
        title: "Backup failed",
        message: errText(err),
      });
      return false;
    } finally {
      busy = null;
      rerender();
    }
  };

  const restore = async (): Promise<void> => {
    const path = restorePath.trim();
    if (busy || !selected || !path) return;
    const confirmed = await confirmDialog({
      title: `Restore saved variables for ${selected}?`,
      message:
        "The account's current saved variables are replaced by the backup file contents.",
      confirmLabel: "Restore",
      danger: true,
    });
    if (!confirmed) return;
    busy = "restore";
    rerender();
    try {
      await service.SavedVarsRestore(selected, path);
      toast({
        type: "ok",
        title: "Saved variables restored",
        message: path,
      });
    } catch (err) {
      toast({
        type: "error",
        title: "Restore failed",
        message: errText(err),
      });
    } finally {
      busy = null;
      rerender();
    }
  };

  const reset = async (): Promise<void> => {
    const addon = resetAddon.trim();
    if (busy || !selected || !addon) return;
    const confirmed = await confirmDialog({
      title: `Reset ${addon} for ${selected}?`,
      message:
        "The addon's saved variables are deleted so the game recreates them fresh.",
      confirmLabel: "Reset",
      danger: true,
    });
    if (!confirmed) return;
    busy = "reset";
    rerender();
    try {
      await service.SavedVarsReset(selected, addon);
      toast({
        type: "ok",
        title: "Saved variables reset",
        message: `${addon}: ${selected}`,
      });
    } catch (err) {
      toast({
        type: "error",
        title: "Reset failed",
        message: errText(err),
      });
    } finally {
      busy = null;
      rerender();
    }
  };

  const migrate = async (): Promise<void> => {
    const from = migrateFrom;
    const to = migrateTo;
    const addon = migrateAddon.trim();
    if (busy || !from || !to || from === to) return;
    const confirmed = await confirmDialog({
      title: `Migrate saved variables to ${to}?`,
      message: addon
        ? `“${addon}” saved variables are copied from ${from} to ${to}.`
        : `Saved variables are copied from ${from} to ${to}.`,
      confirmLabel: "Migrate",
    });
    if (!confirmed) return;
    busy = "migrate";
    rerender();
    try {
      const res = await service.SavedVarsMigrate(from, to, addon);
      lastCopied = res.copied;
      toast({
        type: "ok",
        title: "Saved variables migrated",
        message: `${res.copied.length} file${res.copied.length === 1 ? "" : "s"} copied`,
      });
    } catch (err) {
      toast({
        type: "error",
        title: "Migration failed",
        message: errText(err),
      });
    } finally {
      busy = null;
      rerender();
    }
  };

  const render = (): void => {
    if (!app.state.has_install) {
      el.innerHTML = emptyCard(
        "folder",
        "No WoW install configured",
        "Set up your World of Warcraft path before managing saved variables.",
        `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
      );
      el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
      return;
    }
    if (loading) {
      el.innerHTML = `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Loading accounts…</span></div>`;
      return;
    }
    if (accounts.length === 0) {
      el.innerHTML = emptyCard(
        "file",
        "No accounts found",
        "No WTF\\Account folders were found for the configured install.",
      );
      return;
    }

    const acctOptions = accounts
      .map(
        (a) =>
          `<option value="${escapeAttr(a)}" ${a === selected ? "selected" : ""}>${escapeHtml(a)}</option>`,
      )
      .join("");
    const migrateOptions = (current: string): string =>
      accounts
        .map(
          (a) =>
            `<option value="${escapeAttr(a)}" ${a === current ? "selected" : ""}>${escapeHtml(a)}</option>`,
        )
        .join("");

    el.innerHTML = `
      <div class="savedvars">
        <div class="savedvars-toolbar">
          <div class="field savedvars-account">
            <label class="field-label" for="savedvars-account">Account</label>
            <div class="input-row">
              <span class="input-icon">${icon("file", 15)}</span>
              <select class="input" id="savedvars-account" data-account ${busy ? "disabled" : ""}>
                ${acctOptions}
              </select>
            </div>
          </div>
          <div class="savedvars-actions">
            <button class="btn btn-outline" data-list ${busy ? "disabled" : ""}>
              ${busy === "list" ? `<span class="spinner"></span>` : icon("list", 15)}
              <span>${busy === "list" ? "Listing…" : "List files"}</span>
            </button>
            <button class="btn btn-primary" data-backup ${busy ? "disabled" : ""}>
              ${busy === "backup" ? `<span class="spinner"></span>` : icon("archive", 15)}
              <span>${busy === "backup" ? "Backing up…" : "Back up"}</span>
            </button>
          </div>
        </div>

        ${
          listResult
            ? `<div class="table-wrap savedvars-list">
                <div class="savedvars-list-head">
                  <span class="muted mono">${escapeHtml(listResult.wtf_root)}</span>
                  <span class="muted mono">${escapeHtml(listResult.account)}</span>
                  <span class="muted">${listResult.files.length} file${listResult.files.length === 1 ? "" : "s"}</span>
                </div>
                ${renderFiles(listResult.files)}
              </div>`
            : `<div class="savedvars-hint muted">
                ${icon("info", 14)}<span>Pick an account and list its <span class="mono">SavedVariables</span> files, then back them up.</span>
              </div>`
        }

        <button class="btn btn-ghost btn-sm savedvars-advanced-toggle" data-advanced-toggle aria-expanded="${advancedOpen}" aria-controls="savedvars-advanced-body">
          ${icon(advancedOpen ? "chevron-down" : "chevron-right", 14)}
          <span>Advanced operations</span>
        </button>
        ${advancedOpen
          ? `<div class="savedvars-advanced-body" id="savedvars-advanced-body">
              <div class="savedvars-actions-grid">
                <div class="field">
                  <label class="field-label" for="savedvars-restore-path">Restore from backup</label>
                  <div class="input-row">
                    <span class="input-icon">${icon("download", 15)}</span>
                    <input class="input" id="savedvars-restore-path" type="text" placeholder="Path to a saved-variables backup…"
                      spellcheck="false" autocomplete="off" value="${escapeAttr(restorePath)}" data-restore-path ${busy ? "disabled" : ""} />
                  </div>
                  <button class="btn btn-danger btn-sm savedvars-inline-btn" data-restore ${busy || !restorePath.trim() ? "disabled" : ""}>
                    ${icon("download", 13)}<span>Restore</span>
                  </button>
                </div>

                <div class="field">
                  <label class="field-label" for="savedvars-reset-addon">Reset addon</label>
                  <div class="input-row">
                    <span class="input-icon">${icon("trash", 15)}</span>
                    <input class="input" id="savedvars-reset-addon" type="text" placeholder="Addon name, e.g. Questie"
                      spellcheck="false" autocomplete="off" value="${escapeAttr(resetAddon)}" data-reset-addon ${busy ? "disabled" : ""} />
                  </div>
                  <button class="btn btn-danger btn-sm savedvars-inline-btn" data-reset ${busy || !resetAddon.trim() ? "disabled" : ""}>
                    ${icon("trash", 13)}<span>Reset</span>
                  </button>
                </div>
              </div>

              <div class="field savedvars-migrate">
                <label class="field-label" for="savedvars-migrate-to">Migrate saved variables</label>
                <div class="field-row">
                  <div class="field">
                    <select class="input" id="savedvars-migrate-from" data-migrate-from ${busy ? "disabled" : ""}>
                      ${migrateOptions(migrateFrom)}
                    </select>
                  </div>
                  <div class="field">
                    <select class="input" id="savedvars-migrate-to" data-migrate-to ${busy ? "disabled" : ""}>
                      ${migrateOptions(migrateTo)}
                    </select>
                  </div>
                </div>
                <div class="input-row">
                  <span class="input-icon">${icon("merge", 15)}</span>
                  <input class="input" type="text" placeholder="Addon (optional) - empty migrates the whole account"
                    spellcheck="false" autocomplete="off" value="${escapeAttr(migrateAddon)}" data-migrate-addon ${busy ? "disabled" : ""} />
                </div>
                <div class="savedvars-migrate-row">
                  <button class="btn btn-outline btn-sm" data-migrate ${busy || !migrateFrom || !migrateTo || migrateFrom === migrateTo ? "disabled" : ""}>
                    ${busy === "migrate" ? `<span class="spinner spinner-xs"></span>` : icon("merge", 13)}
                    <span>${busy === "migrate" ? "Migrating…" : "Migrate"}</span>
                  </button>
                  ${
                    lastCopied && lastCopied.length
                      ? `<span class="muted">Copied: ${lastCopied.map((f) => `<span class="mono">${escapeHtml(f)}</span>`).join(", ")}</span>`
                      : ""
                  }
                </div>
              </div>
            </div>`
          : ""}
      </div>`;

    el.querySelector<HTMLSelectElement>("[data-account]")?.addEventListener("change", (e) => {
      selected = (e.target as HTMLSelectElement).value;
      listResult = null;
      lastCopied = null;
      refocus = { sel: "[data-account]", pos: -1 };
      rerender();
    });
    el.querySelector("[data-list]")?.addEventListener("click", () => void listFiles());
    el.querySelector("[data-backup]")?.addEventListener("click", () => void backup());
    el.querySelector<HTMLInputElement>("[data-restore-path]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      restorePath = input.value;
      refocus = { sel: "[data-restore-path]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    el.querySelector("[data-restore]")?.addEventListener("click", () => void restore());
    el.querySelector<HTMLInputElement>("[data-reset-addon]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      resetAddon = input.value;
      refocus = { sel: "[data-reset-addon]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    el.querySelector("[data-reset]")?.addEventListener("click", () => void reset());
    el.querySelector<HTMLSelectElement>("[data-migrate-from]")?.addEventListener("change", (e) => {
      migrateFrom = (e.target as HTMLSelectElement).value;
      refocus = { sel: "[data-migrate-from]", pos: -1 };
      rerender();
    });
    el.querySelector<HTMLSelectElement>("[data-migrate-to]")?.addEventListener("change", (e) => {
      migrateTo = (e.target as HTMLSelectElement).value;
      refocus = { sel: "[data-migrate-to]", pos: -1 };
      rerender();
    });
    el.querySelector<HTMLInputElement>("[data-migrate-addon]")?.addEventListener("input", (e) => {
      const input = e.target as HTMLInputElement;
      migrateAddon = input.value;
      refocus = { sel: "[data-migrate-addon]", pos: input.selectionStart ?? input.value.length };
      rerender();
    });
    el.querySelector("[data-migrate]")?.addEventListener("click", () => void migrate());
    el.querySelector("[data-advanced-toggle]")?.addEventListener("click", () => {
      advancedOpen = !advancedOpen;
      refocus = { sel: "[data-advanced-toggle]", pos: -1 };
      rerender();
    });
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
  };

  const renderFiles = (files: string[]): string => {
    if (files.length === 0) {
      return `<p class="muted savedvars-files-empty">No saved-variable files for this account.</p>`;
    }
    return `<ul class="savedvars-files">
      ${files
        .map((f) => {
          const name = f.endsWith(".lua") ? f : `${f}.lua`;
          return `<li class="mono">${icon("file", 13)}<span>${escapeHtml(name)}</span></li>`;
        })
        .join("")}
    </ul>`;
  };

  rerender();
  void loadAccounts();

  return { refresh: render };
}

function emptyCard(
  glyph: "folder" | "file",
  title: string,
  sub: string,
  cta?: string,
): string {
  return `<div class="empty">
    <span class="empty-icon">${icon(glyph, 28)}</span>
    <h2 class="empty-title">${title}</h2>
    <p class="empty-sub">${sub}</p>
    ${cta ? `<div class="empty-actions">${cta}</div>` : ""}
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
