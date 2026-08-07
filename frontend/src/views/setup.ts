// First-run guided setup: WoW install location (manual or detected), game
// client flavor, and game-version profile.

import type { AppState, Actions } from "../app";
import type { Install } from "../types";
import { icon } from "../icons";
import { service } from "../api";

interface SetupLocal {
  root: string;
  flavor: string;
  profileId: string;
  detected: Install[] | null;
  busy: string | null;
  error: string | null;
}

const FLAVORS: { value: string; label: string }[] = [
  { value: "_retail_", label: "_retail_ — Retail" },
  { value: "_classic_", label: "_classic_ — Wrath / Cataclysm Classic" },
  { value: "_classic_era_", label: "_classic_era_ — Classic Era" },
  { value: "_classic_tbc_", label: "_classic_tbc_ — TBC Classic" },
  { value: "root", label: "root — addons at the top level" },
];

export function mountSetup(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  const local: SetupLocal = {
    root: app.state.wow_path || "",
    flavor: app.state.flavor || "root",
    profileId: app.state.profile_id || "wrath",
    detected: null,
    busy: null,
    error: null,
  };

  const render = (): void => {
    const flavorOptions = FLAVORS.map(
      (f) =>
        `<option value="${f.value}" ${f.value === local.flavor ? "selected" : ""}>${f.label}</option>`,
    ).join("");
    const profileOptions = app.profiles
      .map(
        (p) =>
          `<option value="${p.id}" ${p.id === local.profileId ? "selected" : ""}>${p.name} (${p.family})</option>`,
      )
      .join("");
    const detectBtn = `<button class="btn btn-outline" data-detect ${
      local.busy ? "disabled" : ""
    }>${icon("radar", 16)}<span>${local.busy === "detect" ? "Detecting…" : "Detect"}</span></button>`;

    el.innerHTML = `
      <div class="setup">
        <div class="setup-card">
          <div class="setup-brand spotlight spotlight-violet">
            <span class="setup-brand-mark">${icon("shield", 34)}</span>
            <h1 class="setup-title">Repair your addons.</h1>
            <p class="setup-sub">Point wowfix at a World of Warcraft install to scan for the
              common addon installation problems and fix them safely.</p>
            <ul class="setup-features">
              <li class="feature">
                <span class="feature-icon">${icon("search", 18)}</span>
                <span class="feature-text"><span class="feature-title">Deep scan</span>
                  <span class="feature-desc">Detects 8 common problems — nested folders, GitHub names, missing TOCs and more.</span></span>
              </li>
              <li class="feature">
                <span class="feature-icon">${icon("shield", 18)}</span>
                <span class="feature-text"><span class="feature-title">Safe fixes</span>
                  <span class="feature-desc">Every change is backed up first; removals go to the OS trash, never permanent.</span></span>
              </li>
              <li class="feature">
                <span class="feature-icon">${icon("table", 18)}</span>
                <span class="feature-text"><span class="feature-title">TOC validation</span>
                  <span class="feature-desc">Checks every addon against 9 game versions, from Vanilla to Retail.</span></span>
              </li>
              <li class="feature">
                <span class="feature-icon">${icon("package", 18)}</span>
                <span class="feature-text"><span class="feature-title">ZIP install</span>
                  <span class="feature-desc">Drop an addon archive anywhere in the app — it is extracted, flattened and validated.</span></span>
              </li>
            </ul>
            <span class="setup-version">wowfix v${escapeHtml(app.state.version)}</span>
          </div>

          <div class="setup-form">
            <div class="field">
              <label class="field-label" for="setup-root">World of Warcraft install</label>
              <div class="input-row">
                <span class="input-icon">${icon("folder", 16)}</span>
                <input id="setup-root" class="input" type="text" autocomplete="off" spellcheck="false"
                  placeholder="C:\\Games\\World of Warcraft" value="${escapeAttr(local.root)}" />
                ${detectBtn}
              </div>
              <p class="field-hint">The folder that contains <span class="mono">Interface/AddOns</span> for the client you play.</p>
            </div>

            ${
              local.detected
                ? `<div class="detect-list" aria-label="Detected installs">
                     ${local.detected
                       .map(
                         (d, i) => `
                       <button type="button" class="detect-item" data-detect-pick="${i}">
                         <span class="detect-radio"></span>
                         <span class="detect-body">
                           <span class="detect-path mono">${escapeHtml(d.root)}</span>
                           <span class="detect-meta">
                             <span class="flavor-tag">${escapeHtml(d.flavor)}</span>
                             <span class="mono">${escapeHtml(d.version ?? "unknown version")}</span>
                             <span class="chip chip-${d.confidence === "high" ? "ok" : d.confidence === "medium" ? "warn" : "error"}">${escapeHtml(d.confidence ?? "unknown")} confidence</span>
                           </span>
                         </span>
                       </button>`,
                       )
                       .join("")}
                   </div>`
                : ""
            }

            <div class="field-row">
              <div class="field">
                <label class="field-label" for="setup-flavor">Client flavor</label>
                <div class="select-wrap">${icon("chevron-down", 14)}<select id="setup-flavor" class="select">${flavorOptions}</select></div>
              </div>
              <div class="field">
                <label class="field-label" for="setup-profile">Game version</label>
                <div class="select-wrap">${icon("chevron-down", 14)}<select id="setup-profile" class="select">${profileOptions}</select></div>
              </div>
            </div>

            ${
              local.error
                ? `<div class="setup-error" role="alert">${icon("x-circle", 16)}<span>${escapeHtml(local.error)}</span></div>`
                : ""
            }

            <button class="btn btn-primary btn-lg btn-block" data-continue ${local.busy ? "disabled" : ""}>
              ${local.busy === "setup" ? `<span class="spinner"></span><span>Setting up…</span>` : `${icon("check", 18)}<span>Continue to scan</span>`}
            </button>
          </div>
        </div>
      </div>`;

    const rootInput = el.querySelector<HTMLInputElement>("#setup-root")!;
    const flavorSel = el.querySelector<HTMLSelectElement>("#setup-flavor")!;
    const profileSel = el.querySelector<HTMLSelectElement>("#setup-profile")!;

    rootInput.addEventListener("input", () => {
      local.root = rootInput.value.trim();
      local.detected = null;
      render();
      el.querySelector<HTMLInputElement>("#setup-root")!.focus();
    });
    flavorSel.addEventListener("change", () => {
      local.flavor = flavorSel.value;
    });
    profileSel.addEventListener("change", () => {
      local.profileId = profileSel.value;
    });

    el.querySelector("[data-detect]")?.addEventListener("click", async () => {
      local.busy = "detect";
      local.error = null;
      render();
      try {
        local.detected = await service.DetectInstalls();
        local.busy = null;
        render();
        el.querySelector<HTMLInputElement>("#setup-root")!.focus();
      } catch (err) {
        local.busy = null;
        local.error = errText(err, "Detection failed");
        render();
      }
    });

    el.querySelectorAll<HTMLElement>("[data-detect-pick]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const d = local.detected![Number(btn.dataset.detectPick)];
        local.root = d.root;
        local.flavor = d.flavor;
        if (d.profile_id) local.profileId = d.profile_id;
        local.detected = null;
        render();
        el.querySelector<HTMLInputElement>("#setup-root")!.focus();
      });
    });

    el.querySelector("[data-continue]")?.addEventListener("click", async () => {
      if (!local.root) {
        local.error = "Enter the path to your World of Warcraft install.";
        render();
        return;
      }
      local.busy = "setup";
      local.error = null;
      render();
      try {
        await actions.completeSetup(local.root, local.flavor, local.profileId);
        actions.go("scan");
      } catch (err) {
        local.busy = null;
        local.error = errText(err, "Could not set up this install");
        render();
      }
    });
  };

  render();
  window.setTimeout(() => el.querySelector<HTMLInputElement>("#setup-root")?.focus(), 0);
  return { refresh: render };
}

function errText(err: unknown, fallback: string): string {
  if (err instanceof Error) return err.message || fallback;
  return String(err ?? fallback);
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
