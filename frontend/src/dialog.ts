// Modal confirmation dialog. Promise-based; resolves true only on an
// explicit confirm. Esc / backdrop click / cancel resolve false.

import { icon } from "./icons";

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

  // Element that opened the dialog; focus returns to it when the dialog
  // closes so keyboard users stay anchored to the control they activated.
  const trigger = document.activeElement as HTMLElement | null;

  const backdrop = document.createElement("div");
  backdrop.className = "dialog-backdrop";
  const dialog = document.createElement("div");
  dialog.className = "dialog";
  dialog.setAttribute("role", "dialog");
  dialog.setAttribute("aria-modal", "true");
  dialog.setAttribute("aria-labelledby", "dialog-title");

  const dangerIcon = opts.danger
    ? `<span class="dialog-warn-icon">${icon("warning-oct", 22)}</span>`
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
      <button class="btn-secondary" data-cancel>${escapeHtml(opts.cancelLabel ?? "Cancel")}</button>
      <button class="${opts.danger ? "btn-danger" : "btn-primary"}" data-confirm>
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
    backdrop.remove();
    // Focus goes back to whatever opened the dialog (after the backdrop is
    // gone so the restored element is not covered). Only restore when the
    // element is still connected.
    if (trigger && trigger.isConnected) trigger.focus();
    resolve(value);
  };

  function onKey(e: KeyboardEvent): void {
    if (e.key === "Escape") {
      done(false);
      return;
    }
    if (e.key === "Tab") {
      // Focus trap: keep Tab / Shift+Tab cycling inside the dialog so
      // keyboard users cannot reach the page behind the modal.
      const focusables = Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled"));
      if (focusables.length === 0) {
        e.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || !(active instanceof Node && dialog.contains(active))) {
          e.preventDefault();
          last.focus();
        }
      } else if (active === last || !(active instanceof Node && dialog.contains(active))) {
        e.preventDefault();
        first.focus();
      }
      return;
    }
    if (e.key === "Enter") {
      const t = e.target as HTMLElement;
      // Enter from a form field is a submit keystroke for that field, not a
      // confirmation of the dialog that opened in response to it.
      if (
        t.tagName !== "BUTTON" &&
        !["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName)
      ) {
        done(true);
      }
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
    // Destructive dialogs start on Cancel so an Enter keystroke can never
    // fire the dangerous action by accident; safe dialogs start on Confirm.
    const target = opts.danger ? "[data-cancel]" : "[data-confirm]";
    (dialog.querySelector<HTMLButtonElement>(target))?.focus();
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
