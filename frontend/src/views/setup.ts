// First-run setup: full-window (main.ts hides the sidebar). Auto-detects
// WoW installs and shows them as spotlight cards, or takes a manual path
// (native folder picker via the Wails runtime, folder-input fallback).
// Completing the flow persists the install and reloads the shell, which
// re-reads state and routes into the app.

import type { View } from "../view";
import type { Install, Profile } from "../types";
import { service, mockActive } from "../api";
import { icon } from "../icons";
import { toast } from "../toast";
import "./setup.css";

const FLAVORS: { value: string; label: string }[] = [
  { value: "_retail_", label: "_retail_ — Retail" },
  { value: "_classic_", label: "_classic_ — Wrath / Cataclysm Classic" },
  { value: "_classic_era_", label: "_classic_era_ — Classic Era" },
  { value: "_classic_tbc_", label: "_classic_tbc_ — TBC Classic" },
  { value: "root", label: "root — addons at the top level" },
];

const FLAVOR_LABEL: Record<string, string> = {
  _retail_: "Retail",
  _classic_: "Wrath / Cataclysm Classic",
  _classic_era_: "Classic Era",
  _classic_tbc_: "TBC Classic",
  root: "Top-level install",
};

// Manual path typing: derive the client flavor from a known folder segment.
// Order matters — the longer segments contain `_classic_` as a substring.
const FLAVOR_SEGMENTS: { segment: string; flavor: string }[] = [
  { segment: "_classic_era_", flavor: "_classic_era_" },
  { segment: "_classic_tbc_", flavor: "_classic_tbc_" },
  { segment: "_retail_", flavor: "_retail_" },
  { segment: "_classic_", flavor: "_classic_" },
];

function deriveFlavorFromPath(path: string): string | null {
  const segments = path.split(/[\\/]/);
  for (const { segment, flavor } of FLAVOR_SEGMENTS) {
    if (segments.some((seg) => seg.includes(segment))) return flavor;
  }
  return null;
}

const CARD_TINTS = ["-violet", "-magenta", "-orange"] as const;

type Busy = "detect" | "setup";

// Only one view mounts at a time; a single flag keeps post-await renders
// from clobbering the host after the shell has unmounted us.
let disposed: { gone: boolean } | null = null;

export const view: View = {
  id: "setup",
  label: "Setup",
  icon: "shield",
  async mount(host) {
    disposed = { gone: false };
    const isGone = () => disposed?.gone ?? false;
    const local = {
      root: "",
      flavor: "_retail_",
      profileId: "",
      selected: -1,
      busy: "detect" as Busy | null,
      error: null as string | null,
    };
    let detected: Install[] = [];
    let profiles: Profile[] = [];

    const render = (): void => {
      const busy = local.busy !== null;
      const flavorOptions = FLAVORS.map(
        (f) =>
          `<option value="${f.value}" ${f.value === local.flavor ? "selected" : ""}>${escapeHtml(f.label)}</option>`,
      ).join("");
      const profileOptions = profiles
        .map(
          (p) =>
            `<option value="${p.id}" ${p.id === local.profileId ? "selected" : ""}>${escapeHtml(p.name)} (${escapeHtml(p.family)})</option>`,
        )
        .join("");
      const cards =
        local.busy === "detect" && detected.length === 0
          ? `<p class="setup-detecting">${icon("refresh", 14)} Looking for World of Warcraft installs…</p>`
          : detected
              .map((d, i) => {
                const tint = CARD_TINTS[i % CARD_TINTS.length];
                return `
                  <button type="button" class="spotlight-card setup-card ${tint}${i === local.selected ? " selected" : ""}"
                    data-pick="${i}" aria-pressed="${i === local.selected}" ${busy ? "disabled" : ""}>
                    <span class="spotlight-kicker">${escapeHtml(d.flavor)}</span>
                    <span class="spotlight-title">${escapeHtml(FLAVOR_LABEL[d.flavor] ?? d.flavor)}</span>
                    <span class="spotlight-body setup-card-path">${escapeHtml(d.root)}</span>
                    <span class="setup-card-meta">
                      <span class="setup-chip">${icon("check-circle", 12)}v${escapeHtml(d.version)}</span>
                      <span class="setup-chip">${escapeHtml(d.confidence)} confidence</span>
                      ${
                        i === local.selected
                          ? `<span class="setup-chip -on">${icon("check", 12)}Selected</span>`
                          : ""
                      }
                    </span>
                  </button>`;
              })
              .join("");

      host.innerHTML = `
        <main class="setup-page">
          <header class="setup-hero">
            <p class="setup-kicker">wowfix — first run</p>
            <h1 class="setup-title">Repair your addons.</h1>
            <p class="setup-sub">Point wowfix at a World of Warcraft install to scan for the common
              addon installation problems and fix them safely. Every change is backed up first;
              removals go to the OS trash, never permanent.</p>
          </header>

          <section class="setup-section" aria-label="Detected installs">
            <div class="setup-section-head">
              <h2 class="setup-section-title">Detected installs</h2>
              <span class="setup-section-status">
                ${local.busy === "detect" ? "Scanning common locations…" : detected.length > 0 ? `${detected.length} found` : ""}
              </span>
            </div>
            <div class="setup-cards">${cards}</div>
            ${detected.length === 0 && local.busy !== "detect" ? `<p class="setup-section-status">Nothing found — enter the path manually below.</p>` : ""}
          </section>

          <section class="setup-form" aria-label="Manual install entry">
            <div class="setup-form-head">
              <h2 class="setup-form-title">Or point us at your install</h2>
              <p class="setup-form-sub">The folder that contains <span class="mono">Interface/AddOns</span> for the client you play.</p>
            </div>

            <div class="setup-field">
              <label class="setup-label" for="setup-root">World of Warcraft install</label>
              <div class="setup-path-row">
                <input id="setup-root" class="text-input setup-root-input" type="text"
                  spellcheck="false" autocomplete="off" placeholder="C:\\Games\\World of Warcraft"
                  value="${escapeAttr(local.root)}" aria-describedby="setup-root-hint" />
                <button type="button" class="btn-secondary setup-browse" data-browse ${busy ? "disabled" : ""}>
                  ${icon("folder", 15)}<span>Browse…</span>
                </button>
              </div>
              <p class="setup-hint" id="setup-root-hint">The client flavor is derived from the path when possible.</p>
            </div>

            <div class="setup-fields-row">
              <div class="setup-field">
                <label class="setup-label" for="setup-flavor">Client flavor</label>
                <div class="setup-select">${icon("chevron-down", 14)}<select id="setup-flavor">${flavorOptions}</select></div>
              </div>
              <div class="setup-field">
                <label class="setup-label" for="setup-profile">Game version</label>
                <div class="setup-select">${icon("chevron-down", 14)}<select id="setup-profile">${profileOptions}</select></div>
              </div>
            </div>

            ${local.error ? `<p class="setup-error" role="alert">${icon("x-circle", 14)}<span>${escapeHtml(local.error)}</span></p>` : ""}

            <button class="btn-primary setup-cta" data-continue ${busy ? "disabled" : ""}>
              ${
                local.busy === "setup"
                  ? `<span class="setup-spinner"></span><span>Setting up…</span>`
                  : `${icon("check", 16)}<span>Continue to scan</span>`
              }
            </button>
          </section>
        </main>`;

      const rootInput = host.querySelector<HTMLInputElement>("#setup-root")!;
      const flavorSel = host.querySelector<HTMLSelectElement>("#setup-flavor");
      const profileSel = host.querySelector<HTMLSelectElement>("#setup-profile");

      rootInput.addEventListener("input", () => {
        local.root = rootInput.value.trim();
        const derived = deriveFlavorFromPath(local.root);
        if (derived && flavorSel) flavorSel.value = derived;
        local.selected = -1;
        local.error = null;
      });

      flavorSel?.addEventListener("change", () => {
        local.flavor = flavorSel.value;
        local.selected = -1;
      });

      profileSel?.addEventListener("change", () => {
        local.profileId = profileSel.value;
      });

      host.querySelector<HTMLElement>("[data-browse]")?.addEventListener("click", async () => {
        local.busy = "detect";
        local.error = null;
        render();
        try {
          const picked = await pickFolder();
          if (picked) {
            local.root = picked;
            const derived = deriveFlavorFromPath(picked);
            if (derived) local.flavor = derived;
            local.selected = -1;
          }
        } catch (err) {
          local.error = errText(err, "Could not open the folder picker");
        } finally {
          local.busy = null;
          if (!isGone()) {
            render();
            host.querySelector<HTMLInputElement>("#setup-root")?.focus();
          }
        }
      });

      host.querySelectorAll<HTMLElement>("[data-pick]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const d = detected[Number(btn.dataset.pick)];
          if (!d) return;
          local.root = d.root;
          local.flavor = d.flavor;
          if (d.profile_id) local.profileId = d.profile_id;
          local.selected = Number(btn.dataset.pick);
          local.error = null;
          render();
          host.querySelector<HTMLInputElement>("#setup-root")?.focus();
        });
      });

      host.querySelector<HTMLElement>("[data-continue]")?.addEventListener("click", async () => {
        if (!local.root) {
          local.error = "Enter the path to your World of Warcraft install.";
          render();
          return;
        }
        local.busy = "setup";
        local.error = null;
        render();
        try {
          await service.SetInstall(local.root, local.flavor);
          if (local.profileId) await service.SetProfile(local.profileId);
          // The shell reads state once at boot; after the install is
          // persisted, reloading hands control back to it cleanly.
          history.replaceState(null, "", mockActive ? "?mock=1" : "?view=overview");
          window.location.reload();
        } catch (err) {
          local.busy = null;
          local.error = errText(err, "Could not set up this install");
          toast({ type: "error", title: "Setup failed", message: local.error });
          render();
        }
      });
    };

    render();
    const rootInput = host.querySelector<HTMLInputElement>("#setup-root");
    rootInput?.focus();
    rootInput?.setSelectionRange(rootInput.value.length, rootInput.value.length);

    // Auto-detect on first run; the wizard has no manual refresh button.
    try {
      const [det, profs] = await Promise.all([
        service.DetectInstalls(),
        service.Profiles(),
      ]);
      detected = det;
      profiles = profs;
      if (detected.length > 0) {
        local.root = detected[0].root;
        local.flavor = detected[0].flavor;
        if (detected[0].profile_id) local.profileId = detected[0].profile_id;
        local.selected = 0;
      }
      local.profileId = local.profileId || profiles[0]?.id || "";
    } catch (err) {
      local.error = errText(err, "Install detection failed");
    } finally {
      local.busy = null;
      if (!isGone()) render();
    }
  },
  unmount() {
    if (disposed) disposed.gone = true;
  },
};

/** Native folder picker when the Wails runtime is present; folder-input
 *  fallback otherwise (returns the selected folder's name — the browser
 *  never exposes absolute paths). */
function pickFolder(): Promise<string | null> {
  const rt = (window as unknown as {
    runtime?: { OpenDirectoryDialog?: (opts: unknown) => Promise<string | null> };
  }).runtime;
  if (rt?.OpenDirectoryDialog) {
    return rt.OpenDirectoryDialog({
      title: "Select your World of Warcraft install",
    });
  }
  return new Promise<string | null>((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.setAttribute("webkitdirectory", "");
    input.setAttribute("directory", "");
    input.style.display = "none";
    input.addEventListener("change", () => {
      const first = input.files?.[0];
      resolve(first ? (first.webkitRelativePath.split("/")[0] || null) : null);
      input.remove();
    });
    document.body.appendChild(input);
    input.click();
  });
}

function errText(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message;
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
