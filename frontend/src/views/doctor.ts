// Doctor view — backend diagnostics. Runs the Service Doctor() check and
// renders every check as a status row (ok / warn / error / info), reusing
// the scan view's status-dot conventions.

import type { AppState, Actions } from "../app";
import type { DoctorCheck, DoctorReport } from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";

export function mountDoctor(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let report: DoctorReport | null = null;
  let running = false;

  const run = async (): Promise<void> => {
    if (running) return;
    running = true;
    report = null;
    render();
    try {
      report = await service.Doctor();
      const errors = report.checks.filter((c) => c.status === "error").length;
      const warns = report.checks.filter((c) => c.status === "warn").length;
      toast({
        type: errors > 0 ? "error" : warns > 0 ? "warn" : "ok",
        title: "Diagnostics complete",
        message: `${report.checks.length} check${report.checks.length === 1 ? "" : "s"} · ${errors} error${errors === 1 ? "" : "s"} · ${warns} warning${warns === 1 ? "" : "s"}`,
      });
    } catch (err) {
      toast({
        type: "error",
        title: "Diagnostics failed",
        message: errText(err),
      });
    } finally {
      running = false;
      render();
    }
  };

  const render = (): void => {
    const busy = running;
    el.innerHTML = `
      <div class="doctor">
        <div class="doctor-toolbar">
          <button class="btn btn-primary" data-run ${busy ? "disabled" : ""}>
            ${busy ? `<span class="spinner"></span>` : icon("radar", 15)}
            <span>${busy ? "Running…" : "Run diagnostics"}</span>
          </button>
          ${
            report
              ? `<span class="muted">${report.checks.length} check${report.checks.length === 1 ? "" : "s"}</span>`
              : `<span class="muted">Install · addons · updates · saved variables · backups</span>`
          }
        </div>

        ${
          !report && !busy
            ? `<div class="empty">
                <span class="empty-icon">${icon("radar", 28)}</span>
                <h2 class="empty-title">Doctor</h2>
                <p class="empty-sub">Run a diagnostic pass over the install, the addon folder, update state, saved variables and backup health.</p>
                <div class="empty-actions">
                  <button class="btn btn-primary" data-run>${icon("radar", 16)}<span>Run diagnostics</span></button>
                </div>
              </div>`
            : busy && !report
              ? `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Running diagnostics…</span></div>`
              : `<div class="table-wrap"><table class="table">
                  <thead><tr><th>Check</th><th>Status</th><th>Detail</th></tr></thead>
                  <tbody>
                    ${report!.checks.map((c) => renderRow(c)).join("")}
                  </tbody>
                </table></div>`
        }
      </div>`;

    el.querySelectorAll<HTMLElement>("[data-run]").forEach((btn) => {
      btn.addEventListener("click", () => void run());
    });
  };

  const renderRow = (c: DoctorCheck): string => {
    const dot = dotClass(c.status);
    const labelClass = c.status === "info" ? "muted" : `status-${c.status}`;
    const glyph: IconName =
      c.status === "ok"
        ? "check-circle"
        : c.status === "error"
          ? "x-circle"
          : c.status === "warn"
            ? "alert"
            : "info";
    return `
      <tr>
        <td><span class="mono">${escapeHtml(c.name)}</span></td>
        <td><span class="status-label ${labelClass}"><span class="status-dot ${dot}"></span>${escapeHtml(c.status)}</span></td>
        <td>${escapeHtml(c.message)}</td>
      </tr>`;
  };

  render();
  return { refresh: render };
}

function dotClass(status: DoctorCheck["status"]): string {
  switch (status) {
    case "ok":
      return "ok";
    case "warn":
      return "warn";
    case "error":
      return "error";
    default:
      return "muted";
  }
}

function errText(err: unknown, fallback?: string): string {
  if (err instanceof Error && err.message) return err.message;
  if (fallback) return fallback;
  return String(err ?? "Unknown error");
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
