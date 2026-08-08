// Saved Variables view. Per-account WTF\Account\<name>\SavedVariables files:
// account pill tabs, an auto-listed file list (first list of each account
// per session auto-creates one backup), and restore / reset / migrate
// operations. Destructive ops go through confirmDialog; results toast; the
// acting control is the only thing disabled while busy.

import type { View } from "../view";
import type { SavedVarsListResult } from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast } from "../toast";
import { confirmDialog } from "../dialog";
import "./savedvars.css";

/** Wails runtime surface used for the native file picker (packaged app only). */
interface WailsRuntime {
  OpenFileDialog?: (opts: Record<string, unknown>) => Promise<string | null>;
}

export const view: View = {
  id: "savedvars",
  label: "Saved Variables",
  icon: "save",
  mount(host) {
    let accounts: string[] = [];
    let selected = "";
    let listResult: SavedVarsListResult | null = null;
    let loading = true;
    let busy: string | null = null; // label of the in-flight operation
    let lastCopied: string[] | null = null;
    // Accounts auto-backed-up on first list, per mount ("per session").
    const autoBackedUp = new Set<string>();
    let restorePath = "";
    let resetAddon = "";
    let migrateFrom = "";
    let migrateTo = "";
    let migrateAddon = "";
    let refocus: { sel: string; pos: number } | null = null;

    const rerender = (): void => {
      render();
      if (!refocus) return;
      const target = host.querySelector<HTMLInputElement>(refocus.sel);
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
          migrateTo = accounts[1] ?? "";
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
      if (selected) await listFiles();
    };

    const listFiles = async (): Promise<void> => {
      if (busy || !selected) return;
      busy = "list";
      rerender();
      let ok = false;
      try {
        listResult = await service.SavedVarsList(selected);
        ok = true;
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
      if (!ok) return;
      // First successful list of this account per session → one auto-backup.
      if (!autoBackedUp.has(selected)) {
        const backedUp = await backup(true);
        if (backedUp) autoBackedUp.add(selected);
      }
    };

    const backup = async (auto = false): Promise<boolean> => {
      if (busy || !selected) return false;
      busy = "backup";
      rerender();
      try {
        const res = await service.SavedVarsBackup(selected);
        toast({
          type: "ok",
          title: auto ? "Auto-backup created" : "Saved variables backed up",
          message: res.path || `account ${selected}`,
        });
        return true;
      } catch (err) {
        toast({
          type: "error",
          title: auto ? "Auto-backup failed" : "Backup failed",
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
          "The account's current SavedVariables files are replaced by the backup contents.",
        confirmLabel: "Restore",
        danger: true,
      });
      if (!confirmed) return;
      busy = "restore";
      rerender();
      let ok = false;
      try {
        await service.SavedVarsRestore(selected, path);
        toast({
          type: "ok",
          title: "Saved variables restored",
          message: path,
        });
        ok = true;
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
      if (ok) await listFiles();
    };

    const reset = async (): Promise<void> => {
      const addon = resetAddon.trim();
      if (busy || !selected || !addon) return;
      const confirmed = await confirmDialog({
        title: `Reset ${addon} for ${selected}?`,
        message:
          `“${addon}” is matched by exact file stem and its SavedVariables are deleted so the game recreates them fresh. DBM-Core is always preserved.`,
        confirmLabel: "Reset",
        danger: true,
      });
      if (!confirmed) return;
      busy = "reset";
      rerender();
      let ok = false;
      try {
        await service.SavedVarsReset(selected, addon);
        toast({
          type: "ok",
          title: "Saved variables reset",
          message: `${addon} — ${selected}`,
        });
        resetAddon = "";
        ok = true;
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
      if (ok) await listFiles();
    };

    const migrate = async (): Promise<void> => {
      const from = migrateFrom;
      const to = migrateTo;
      const addon = migrateAddon.trim();
      if (busy || !from || !to || from === to) return;
      const confirmed = await confirmDialog({
        title: `Migrate saved variables to ${to}?`,
        message: addon
          ? `“${addon}” is copied from ${from} to ${to}. Existing files are never overwritten.`
          : `Every saved variable is copied from ${from} to ${to}. Existing files are never overwritten.`,
        confirmLabel: "Migrate",
      });
      if (!confirmed) return;
      busy = "migrate";
      rerender();
      let ok = false;
      try {
        const res = await service.SavedVarsMigrate(from, to, addon);
        lastCopied = res.copied;
        toast({
          type: "ok",
          title: "Saved variables migrated",
          message: `${res.copied.length} file${res.copied.length === 1 ? "" : "s"} copied${res.copied.length === 0 ? " — nothing new to copy" : ""}`,
        });
        ok = true;
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
      if (ok) await listFiles();
    };

    const render = (): void => {
      if (loading) {
        host.innerHTML = `
          <section class="view-page">
            <div class="view-hero">
              <h1 class="view-title">Saved Variables</h1>
              <p class="view-sub">Per-account WTF SavedVariables files — list, back up, restore, reset and migrate.</p>
            </div>
            <div class="list-loading"><span class="loading-pulse"></span><span>Loading accounts…</span></div>
          </section>`;
        return;
      }

      if (accounts.length === 0) {
        host.innerHTML = `
          <section class="view-page">
            <div class="view-hero">
              <h1 class="view-title">Saved Variables</h1>
              <p class="view-sub">Per-account WTF SavedVariables files — list, back up, restore, reset and migrate.</p>
            </div>
            <div class="card">
              <p class="section-desc">No WTF\\Account folders were found for the configured install.</p>
            </div>
          </section>`;
        return;
      }

      const accountTabs = accounts
        .map(
          (a) => `
        <button class="tab account-tab" data-account="${escapeAttr(a)}" aria-pressed="${a === selected}" ${busy ? "disabled" : ""}>
          ${icon(a === selected ? "check-circle" : "save", 14)}
          <span>${escapeHtml(a)}</span>
        </button>`,
        )
        .join("");

      host.innerHTML = `
        <section class="view-page">
          <div class="view-hero">
            <h1 class="view-title">Saved Variables</h1>
            <p class="view-sub">Per-account WTF SavedVariables files — list, back up, restore, reset and migrate.</p>
          </div>

          <section class="savedvars-section" aria-labelledby="savedvars-account-title">
            <div class="section-head">
              <div class="section-head-text">
                <h2 id="savedvars-account-title" class="section-title">Account</h2>
                <p class="section-desc">Select an account to list its files. The first list of each account per session creates a backup automatically.</p>
              </div>
            </div>
            <div class="card savedvars-card">
              <div class="savedvars-toolbar">
                <div class="tabs account-tabs" role="group" aria-label="WoW account">
                  ${accountTabs}
                </div>
                <button class="btn-primary" data-backup ${busy ? "disabled" : ""}>
                  ${busy === "backup" ? icon("refresh", 15) : icon("archive", 15)}
                  <span>${busy === "backup" ? "Backing up…" : "Back up"}</span>
                </button>
              </div>

              ${
                listResult
                  ? `
                <div class="list-head">
                  <span class="list-root" title="${escapeAttr(listResult.wtf_root)}">${escapeHtml(listResult.wtf_root)}</span>
                  <span class="list-count">${listResult.files.length} file${listResult.files.length === 1 ? "" : "s"}</span>
                </div>
                ${renderFiles(listResult.files)}`
                  : `<p class="section-desc">Pick an account to list its files.</p>`
              }
            </div>
          </section>

          <section class="savedvars-section" aria-labelledby="savedvars-ops-title">
            <div class="section-head">
              <div class="section-head-text">
                <h2 id="savedvars-ops-title" class="section-title">Operations</h2>
                <p class="section-desc">Restore from a backup, reset one addon's settings, or copy settings between accounts.</p>
              </div>
            </div>
            <div class="card ops-card">
              <div class="ops-row">
                <div class="ops-label">
                  <span class="ops-name">Restore from backup</span>
                  <span class="ops-hint">Replaces the account's SavedVariables with the backup contents.</span>
                </div>
                <div class="ops-control">
                  <div class="ops-inline">
                    <input class="text-input mono" type="text" placeholder="Path to a saved-variables backup…"
                      spellcheck="false" autocomplete="off" value="${escapeAttr(restorePath)}" data-restore-path ${busy ? "disabled" : ""} />
                    <button class="btn-secondary" data-browse ${busy ? "disabled" : ""}>
                      ${icon("folder", 15)}<span>Browse…</span>
                    </button>
                    <button class="btn-danger" data-restore ${busy || !restorePath.trim() ? "disabled" : ""}>
                      ${busy === "restore" ? icon("refresh", 15) : icon("download", 15)}
                      <span>${busy === "restore" ? "Restoring…" : "Restore"}</span>
                    </button>
                  </div>
                </div>
              </div>

              <div class="ops-row">
                <div class="ops-label">
                  <span class="ops-name">Reset addon</span>
                  <span class="ops-hint">Deletes the addon's exact file stem so the game recreates it fresh. DBM-Core is always preserved.</span>
                </div>
                <div class="ops-control">
                  <div class="ops-inline">
                    <input class="text-input" type="text" placeholder="Addon name, e.g. Questie"
                      spellcheck="false" autocomplete="off" value="${escapeAttr(resetAddon)}" data-reset-addon ${busy ? "disabled" : ""} />
                    <button class="btn-danger" data-reset ${busy || !resetAddon.trim() ? "disabled" : ""}>
                      ${busy === "reset" ? icon("refresh", 15) : icon("trash", 15)}
                      <span>${busy === "reset" ? "Resetting…" : "Reset"}</span>
                    </button>
                  </div>
                </div>
              </div>

              <div class="ops-row">
                <div class="ops-label">
                  <span class="ops-name">Migrate to another account</span>
                  <span class="ops-hint">Copies saved variables between accounts in the same WTF root. Existing files are never overwritten.</span>
                </div>
                <div class="ops-control">
                  <div class="ops-inline">
                    <div class="migrate-picker">
                      <select class="text-input" data-migrate-from ${busy ? "disabled" : ""} aria-label="From account">
                        ${migrateOptions(migrateFrom)}
                      </select>
                      <span class="migrate-arrow">${icon("chevron-right", 16)}</span>
                      <select class="text-input" data-migrate-to ${busy ? "disabled" : ""} aria-label="To account">
                        ${migrateOptions(migrateTo)}
                      </select>
                    </div>
                    <input class="text-input" type="text" placeholder="Addon (optional) — empty migrates the whole account"
                      spellcheck="false" autocomplete="off" value="${escapeAttr(migrateAddon)}" data-migrate-addon ${busy ? "disabled" : ""} />
                  </div>
                  <div class="ops-inline">
                    <button class="btn-secondary" data-migrate ${busy || !migrateFrom || !migrateTo || migrateFrom === migrateTo ? "disabled" : ""}>
                      ${busy === "migrate" ? icon("refresh", 15) : icon("chevron-right", 15)}
                      <span>${busy === "migrate" ? "Migrating…" : "Migrate"}</span>
                    </button>
                    ${
                      lastCopied && lastCopied.length > 0
                        ? `<div class="copied-result">
                            <span>Copied:</span>
                            <span class="copied-files">${lastCopied
                              .map((f) => `<span class="copied-file">${escapeHtml(withLua(f))}</span>`)
                              .join("")}</span>
                          </div>`
                        : ""
                    }
                  </div>
                </div>
              </div>
            </div>
          </section>
        </section>`;

      host.querySelectorAll<HTMLElement>("[data-account]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const name = btn.dataset.account ?? "";
          if (name === selected || busy) return;
          selected = name;
          listResult = null;
          lastCopied = null;
          resetAddon = "";
          refocus = { sel: `[data-account="${escapeAttr(name)}"]`, pos: -1 };
          void listFiles();
        });
      });
      host.querySelector("[data-backup]")?.addEventListener("click", () => {
        void backup(false);
      });
      host.querySelector<HTMLInputElement>("[data-restore-path]")?.addEventListener("input", (e) => {
        const input = e.target as HTMLInputElement;
        restorePath = input.value;
        refocus = { sel: "[data-restore-path]", pos: input.selectionStart ?? input.value.length };
        rerender();
      });
      host.querySelector("[data-browse]")?.addEventListener("click", () => {
        void browseForBackup();
      });
      host.querySelector("[data-restore]")?.addEventListener("click", () => {
        void restore();
      });
      host.querySelector<HTMLInputElement>("[data-reset-addon]")?.addEventListener("input", (e) => {
        const input = e.target as HTMLInputElement;
        resetAddon = input.value;
        refocus = { sel: "[data-reset-addon]", pos: input.selectionStart ?? input.value.length };
        rerender();
      });
      host.querySelector("[data-reset]")?.addEventListener("click", () => {
        void reset();
      });
      host.querySelector<HTMLSelectElement>("[data-migrate-from]")?.addEventListener("change", (e) => {
        migrateFrom = (e.target as HTMLSelectElement).value;
        refocus = { sel: "[data-migrate-from]", pos: -1 };
        rerender();
      });
      host.querySelector<HTMLSelectElement>("[data-migrate-to]")?.addEventListener("change", (e) => {
        migrateTo = (e.target as HTMLSelectElement).value;
        refocus = { sel: "[data-migrate-to]", pos: -1 };
        rerender();
      });
      host.querySelector<HTMLInputElement>("[data-migrate-addon]")?.addEventListener("input", (e) => {
        const input = e.target as HTMLInputElement;
        migrateAddon = input.value;
        refocus = { sel: "[data-migrate-addon]", pos: input.selectionStart ?? input.value.length };
        rerender();
      });
      host.querySelector("[data-migrate]")?.addEventListener("click", () => {
        void migrate();
      });
    };

    const renderFiles = (files: string[]): string => {
      if (files.length === 0) {
        return `<p class="files-empty">No saved-variable files for this account.</p>`;
      }
      return `<ul class="savedvars-files">
        ${files
          .map(
            (f) => `
          <li class="file-row">
            <span class="file-icon">${icon("save", 13)}</span>
            <span>${escapeHtml(withLua(f))}</span>
          </li>`,
          )
          .join("")}
      </ul>`;
    };

    const migrateOptions = (current: string): string =>
      accounts
        .map(
          (a) =>
            `<option value="${escapeAttr(a)}" ${a === current ? "selected" : ""}>${escapeHtml(a)}</option>`,
        )
        .join("");

    const browseForBackup = async (): Promise<void> => {
      if (busy) return;
      const rt = (window as unknown as { runtime?: WailsRuntime }).runtime;
      if (!rt?.OpenFileDialog) {
        toast({
          type: "info",
          title: "File picker unavailable",
          message: "Only the packaged app has a native picker — type the backup path instead.",
        });
        return;
      }
      try {
        const path = await rt.OpenFileDialog({
          title: "Select a saved-variables backup",
          filters: [
            { displayName: "Saved variables backups", pattern: "*.lua;*.zip" },
            { displayName: "All files", pattern: "*" },
          ],
          properties: ["openFile"],
        });
        if (path) {
          restorePath = path;
          refocus = { sel: "[data-restore-path]", pos: path.length };
          rerender();
        }
      } catch (err) {
        toast({
          type: "error",
          title: "Could not open file picker",
          message: errText(err),
        });
      }
    };

    rerender();
    void loadAccounts();
  },
};

function withLua(name: string): string {
  return name.endsWith(".lua") ? name : `${name}.lua`;
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
