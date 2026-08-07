// Modal confirmation dialog. Promise-based; resolves true only on an
// explicit confirm. Esc / backdrop click / cancel resolve false.

import { icon } from "../icons";

export interface ConfirmOpts {
  title: string;
  message: string;
  details?: string[]; // bullet list rendered under the message
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean; // red confirm button for destructive actions
}

const root = document.createElement("div");
root.className = "dialog-root";

export function mountDialog(host: HTMLElement): void {
  host.appendChild(root);
}

export function confirmDialog(opts: ConfirmOpts): Promise<boolean> {
  const { promise, resolve } = Promise.withResolvers<boolean>();
  root.replaceChildren();

  const backdrop = document.createElement("div");
  backdrop.className = "dialog-backdrop";
  const dialog = document.createElement("div");
  dialog.className = "dialog";
  dialog.setAttribute("role", "dialog");
  dialog.setAttribute("aria-modal", "true");
  dialog.setAttribute("aria-labelledby", "dialog-title");

  const dangerIcon = opts.danger
    ? icon("warning-oct", 22, "dialog-warn-icon")
    : "";
  dialog.innerHTML = `
    <div class="dialog-head">
      <span class="dialog-title" id="dialog-title">${escapeHtml(opts.title)}</span>
      <button class="icon-btn" data-close aria-label="Close dialog">${icon("x", 16)}</button>
    </div>
    <div class="dialog-body">
      ${dangerIcon}
      <div>
        <p class="dialog-message">${escapeHtml(opts.message)}</p>
        ${
          opts.details && opts.details.length > 0
            ? `<ul class="dialog-details">${opts.details
                .map((d) => `<li>${escapeHtml(d)}</li>`)
                .join("")}</ul>`
            : ""
        }
      </div>
    </div>
    <div class="dialog-actions">
      <button class="btn btn-ghost" data-cancel>${escapeHtml(opts.cancelLabel ?? "Cancel")}</button>
      <button class="btn ${opts.danger ? "btn-danger" : "btn-primary"}" data-confirm>
        ${icon(opts.danger ? "trash" : "check", 16)}<span>${escapeHtml(opts.confirmLabel ?? "Confirm")}</span>
      </button>
    </div>`;
  backdrop.appendChild(dialog);
  root.appendChild(backdrop);

  let settled = false;
  const done = (value: boolean): void => {
    if (settled) return;
    settled = true;
    document.removeEventListener("keydown", onKey);
    resolve(value);
    backdrop.remove();
  };

  function onKey(e: KeyboardEvent): void {
    if (e.key === "Escape") done(false);
    if (e.key === "Enter") {
      const t = e.target as HTMLElement;
      if (t.tagName !== "BUTTON") done(true);
    }
  }

  backdrop.addEventListener("click", (e) => {
    if (e.target === backdrop) done(false);
  });
  dialog.querySelector("[data-close]")!.addEventListener("click", () => done(false));
  dialog.querySelector("[data-cancel]")!.addEventListener("click", () => done(false));
  dialog.querySelector("[data-confirm]")!.addEventListener("click", () => done(true));
  document.addEventListener("keydown", onKey);
  window.setTimeout(() => {
    (dialog.querySelector("[data-confirm]") as HTMLButtonElement | null)?.focus();
  }, 0);

  return promise;
}

const ESC_MAP: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESC_MAP[c]);
}
