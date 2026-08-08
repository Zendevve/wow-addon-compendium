// Version history modal: lists the recorded versions of one tracked
// addon with a Current marker and per-row Rollback. It is a sibling of
// the confirm dialog, not a reuse of it: rows need a list layout and
// inline result rendering, so this file owns its own overlay, focus
// trap (Esc / Tab / backdrop) and focus restore. Rollback is gated by
// the confirm dialog (danger) and its outcome renders inline.

import { icon } from "../icons";
import { service } from "../api";
import { toast } from "./toast";
import { confirmDialog } from "./dialog";
import type { VersionHistoryResult } from "../types";

export interface VersionHistoryOptions {
  folder: string;
  title: string;
  /** Element focus returns to when the modal closes. */
  restoreFocus?: HTMLElement | null;
  /** Called after a successful rollback (e.g. to refresh the view). */
  onRolledBack?: () => void;
}

const PROVIDER_LABEL: Record<string, string> = {
  github: "GitHub",
  curseforge: "CurseForge",
  wowinterface: "WowInterface",
  tukui: "Tukui",
};

let root: HTMLDivElement | null = null;
let seq = 0;

export function openVersionHistory(opts: VersionHistoryOptions): void {
  if (!root) {
    root = document.createElement("div");
    root.className = "vh-root";
    document.body.appendChild(root);
  }

  const trigger = opts.restoreFocus ?? (document.activeElement as HTMLElement | null);
  const id = `vh-title-${++seq}`;

  const backdrop = document.createElement("div");
  backdrop.className = "vh-backdrop";
  const modal = document.createElement("div");
  modal.className = "vh-modal";
  modal.setAttribute("role", "dialog");
  modal.setAttribute("aria-modal", "true");
  modal.setAttribute("aria-labelledby", id);
  modal.innerHTML = `
    <div class="vh-head">
      <span class="vh-title" id="${id}">Version history · ${escapeHtml(opts.title)}</span>
      <button class="icon-btn" data-vh-close aria-label="Close version history">${icon("x", 16)}</button>
    </div>
    <div class="vh-body" data-vh-body>
      <div class="list-loading"><span class="spinner spinner-lg"></span><span>Loading version history…</span></div>
    </div>
    <div class="vh-foot">
      Rolling back re-downloads the exact version from the provider; the current state is backed up first.
    </div>`;
  backdrop.appendChild(modal);
  root.appendChild(backdrop);

  let settled = false;
  const close = (): void => {
    if (settled) return;
    settled = true;
    document.removeEventListener("keydown", onKey);
    backdrop.remove();
    if (trigger && trigger.isConnected) trigger.focus();
  };

  // --- focus trap (mirrors the confirm dialog's keyboard contract) ---
  function onKey(e: KeyboardEvent): void {
    if (e.key === "Escape") {
      e.stopPropagation();
      close();
      return;
    }
    if (e.key === "Tab") {
      const focusables = Array.from(
        modal.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled"));
      if (focusables.length === 0) {
        e.preventDefault();
        modal.focus();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || !(active instanceof Node && modal.contains(active))) {
          e.preventDefault();
          last.focus();
        }
      } else if (active === last || !(active instanceof Node && modal.contains(active))) {
        e.preventDefault();
        first.focus();
      }
    }
  }

  backdrop.addEventListener("click", (e) => {
    if (e.target === backdrop) close();
  });
  modal.querySelector("[data-vh-close]")!.addEventListener("click", close);
  document.addEventListener("keydown", onKey);
  window.setTimeout(() => {
    (modal.querySelector<HTMLButtonElement>("[data-vh-close]"))?.focus();
  }, 0);

  // --- load + render ---------------------------------------------------
  const body = modal.querySelector<HTMLElement>("[data-vh-body]") ?? modal;
  let rollbacking = "";

  const render = (res: VersionHistoryResult, error?: string): void => {
    const rows = res.versions
      .map((v) => {
        const isCurrent = v.version === res.current;
        const side = isCurrent
          ? `<span class="vh-current-tag">${icon("check", 12)}<span>Current</span></span>`
          : `<button class="btn btn-sm btn-restore" data-vh-rollback="${escapeAttr(v.version)}"
               ${rollbacking ? "disabled" : ""}>${rollbacking === v.version ? `<span class="spinner"></span>` : icon("download", 12)}<span>Rollback</span></button>`;
        return `
          <div class="vh-row${isCurrent ? " is-current" : ""}">
            <div class="vh-row-main">
              <span class="vh-version mono">${escapeHtml(v.version || "n/a")}</span>
              <span class="vh-provider">${escapeHtml(PROVIDER_LABEL[v.provider ?? ""] ?? v.provider ?? "")}</span>
              <span class="vh-date">${formatDateTime(v.at)}</span>
            </div>
            <div class="vh-row-side">${side}</div>
          </div>`;
      })
      .join("");

    body.innerHTML = `
      ${error ? `<div class="vh-error" role="alert">${icon("alert", 15)}<span>${escapeHtml(error)}</span></div>` : ""}
      <div class="vh-rows" role="list" aria-label="Recorded versions">
        ${rows || `<div class="vh-empty">No version history recorded for this addon yet.</div>`}
      </div>`;

    body.querySelectorAll<HTMLButtonElement>("[data-vh-rollback]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const version = btn.dataset.vhRollback ?? "";
        void rollbackTo(version, res);
      });
    });
  };

  const rollbackTo = async (version: string, res: VersionHistoryResult): Promise<void> => {
    const confirmed = await confirmDialog({
      title: `Roll back ${opts.title} to ${version}?`,
      message:
        "Re-downloads this exact version from the provider and replaces the folder. A backup snapshot of the current state is taken first.",
      confirmLabel: "Roll Back",
      danger: true,
    });
    if (!confirmed) return;
    rollbacking = version;
    render(res);
    try {
      const out = await service.RollbackToVersion(opts.folder, version);
      if (out.errors.length > 0) {
        render(res, out.errors.join("\n"));
        return;
      }
      toast({
        type: "ok",
        title: `Rolled back ${opts.title}`,
        message: `Re-downloaded ${version} from the provider; a backup snapshot was taken first.`,
      });
      opts.onRolledBack?.();
      close();
    } catch (err) {
      rollbacking = "";
      render(res, errText(err));
    }
  };

  service
    .ListAddonVersions(opts.folder)
    .then((res) => render(res))
    .catch((err) => {
      body.innerHTML = `<div class="vh-error" role="alert">${icon("alert", 15)}<span>${escapeHtml(errText(err))}</span></div>`;
    });
}

function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return (
    d.toLocaleDateString([], { year: "numeric", month: "short", day: "numeric" }) +
    " " +
    d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  );
}

function errText(err: unknown, fallback?: string): string {
  if (err instanceof Error && err.message) return err.message;
  if (fallback) return fallback;
  return String(err ?? "Unknown error");
}

const ESC: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESC[c]);
}
function escapeAttr(s: string): string {
  return escapeHtml(s).replaceAll("'", "&#39;");
}
