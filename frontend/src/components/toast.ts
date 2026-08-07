// Toast notifications for async outcomes. Mounted once per app shell;
// toasts stack bottom-right, auto-dismiss (paused on hover) and are
// announced through an aria-live region.

import { icon, type IconName } from "../icons";

export type ToastType = "ok" | "warn" | "error" | "info";

export interface ToastOpts {
  type: ToastType;
  title: string;
  message?: string;
  duration?: number; // ms; default 5000, errors 9000
}

const ICON: Record<ToastType, IconName> = {
  ok: "check-circle",
  warn: "alert",
  error: "x-circle",
  info: "info",
};

const root = document.createElement("div");
root.className = "toast-root";
root.setAttribute("role", "status");
root.setAttribute("aria-live", "polite");

export function mountToasts(host: HTMLElement): void {
  host.appendChild(root);
}

export function toast(opts: ToastOpts): void {
  const duration = opts.duration ?? (opts.type === "error" ? 9000 : 5000);
  const el = document.createElement("div");
  el.className = `toast toast-${opts.type}`;
  el.setAttribute("role", opts.type === "error" ? "alert" : "status");
  el.innerHTML = `
    <span class="toast-icon">${icon(ICON[opts.type], 18)}</span>
    <span class="toast-body">
      <span class="toast-title">${escapeHtml(opts.title)}</span>
      ${opts.message ? `<span class="toast-msg">${escapeHtml(opts.message)}</span>` : ""}
    </span>
    <button class="toast-close" aria-label="Dismiss notification">${icon("x", 14)}</button>`;
  root.appendChild(el);

  let timer = window.setTimeout(dismiss, duration);
  const pause = () => window.clearTimeout(timer);
  const resume = () => {
    timer = window.setTimeout(dismiss, duration);
  };
  el.addEventListener("mouseenter", pause);
  el.addEventListener("mouseleave", resume);
  el.querySelector(".toast-close")!.addEventListener("click", dismiss);

  function dismiss(): void {
    el.classList.add("toast-out");
    window.setTimeout(() => el.remove(), 220);
  }
}

function escapeHtml(s: string): string {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
