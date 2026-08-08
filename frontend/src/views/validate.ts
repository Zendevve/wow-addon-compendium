// TOC validation table: addon / TOC / expected interface / detected /
// status, with semantic status coloring.

import type { AppState, Actions } from "../app";
import { icon, type IconName } from "../icons";

export function mountValidate(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  const render = (): void => {
    if (!app.state.has_install) {
      el.innerHTML = emptyCard(
        "folder",
        "No WoW install configured",
        "Set up your World of Warcraft path before validating addons.",
        `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
      );
      el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
      return;
    }
    if (!app.validation) {
      el.innerHTML = `
        <div class="list-loading">
          <span class="spinner spinner-lg"></span>
          <span>${app.busy === "validate" ? "Validating TOC files…" : "Loading…"}</span>
        </div>`;
      return;
    }

    const v = app.validation;
    const profile = app.profiles.find((p) => p.id === v.profile_id);
    const bad = v.addons.filter((a) => a.status !== "compatible").length;

    el.innerHTML = `
      <div class="validate-toolbar">
        <div class="validate-profile chip-mono">
          ${icon("table", 15)}
          <span>Expected interface <b>${v.expected}</b></span>
          <span class="muted">·</span>
          <span>${escapeHtml(profile?.name ?? v.profile_id)}</span>
        </div>
        <button class="btn btn-outline" data-validate ${app.busy ? "disabled" : ""}>
          ${icon("refresh", 15)}<span>${app.busy === "validate" ? "Validating…" : "Re-validate"}</span>
        </button>
      </div>

      ${
        v.addons.length === 0
          ? emptyCard(
              "table",
              "Nothing to validate",
              "No addon folders with a TOC file were found.",
              "",
            )
          : `
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr><th>Addon</th><th>TOC</th><th>Expected</th><th>Detected</th><th>Status</th></tr>
            </thead>
            <tbody>
              ${v.addons
                .map(
                  (a) => `<tr>
                    <td class="cell-addon">${escapeHtml(a.folder_name)}</td>
                    <td class="mono">${escapeHtml(a.toc)}</td>
                    <td class="mono">${a.expected}</td>
                    <td class="mono">${a.detected > 0 ? a.detected : "n/a"}</td>
                    <td><span class="status-label status-${a.status}">
                      <span class="status-dot ${dotClass(a.status)}"></span>${escapeHtml(a.label)}
                    </span></td>
                  </tr>`,
                )
                .join("")}
            </tbody>
          </table>
        </div>
        <div class="validate-summary">
          ${v.addons.length} addons · <span class="${bad > 0 ? "text-warn" : "text-ok"}">${bad} not compatible with this profile</span>
        </div>`
      }`;

    el.querySelector("[data-validate]")?.addEventListener("click", () => actions.validate());
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
  };

  if (!app.validation && app.state.has_install && !app.busy) {
    void actions.validate();
  }
  render();
  return { refresh: render };
}

function dotClass(status: string): string {
  switch (status) {
    case "compatible":
      return "ok";
    case "vanilla":
    case "retail":
    case "unknown":
      return "warn";
    case "mismatch":
      return "error";
    default:
      return "muted";
  }
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
