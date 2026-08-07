// Settings view — config keys editable from the app. Theme, auto-backup and
// confirmation checkboxes, plus the backups / collections / API-key paths.
// Every save goes through SetConfigKey; the install-derived keys (wow_path,
// flavor, profile, collection) are shown read-only with a hint that they are
// managed from Setup and Collections.

import type { AppState, Actions } from "../app";
import type { ConfigView } from "../types";
import { icon } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";

const TEXT_KEYS = ["backups_dir", "curseforge_api_key", "collections_dir"] as const;

const KEY_LABELS: Record<string, string> = {
  theme: "Theme",
  auto_backup: "Auto-backup",
  confirmations: "Confirmations",
  backups_dir: "Backups folder",
  curseforge_api_key: "CurseForge API key",
  collections_dir: "Collections folder",
  wow_path: "WoW install path",
  flavor: "Flavor",
  profile: "Profile",
  collection: "Active collection",
};

export function mountSettings(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let cfg: ConfigView | null = null;
  let loading = false;
  let saving: string | null = null; // key currently saving
  let error: string | null = null; // inline error for the last save
  let drafts: Record<string, string> = {};
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

  const load = async (): Promise<void> => {
    loading = true;
    rerender();
    try {
      cfg = await service.Config();
      drafts = {
        backups_dir: cfg.backups_dir,
        curseforge_api_key: cfg.curseforge_api_key,
        collections_dir: cfg.collections_dir,
      };
    } catch (err) {
      toast({
        type: "error",
        title: "Could not load settings",
        message: errText(err),
      });
    } finally {
      loading = false;
      rerender();
    }
  };

  const save = async (key: string, value: string): Promise<void> => {
    if (saving) return;
    saving = key;
    error = null;
    rerender();
    try {
      await service.SetConfigKey(key, value);
      if (cfg) (cfg as unknown as Record<string, unknown>)[key] = coerceValue(key, value);
      if (key in drafts) drafts[key] = value;
      toast({
        type: "ok",
        title: `${KEY_LABELS[key] ?? key} saved`,
        message: coerceValue(key, value) === true
          ? "on"
          : coerceValue(key, value) === false
            ? "off"
            : value,
      });
    } catch (err) {
      error = `${KEY_LABELS[key] ?? key}: ${errText(err)}`;
      rerender();
    } finally {
      saving = null;
      rerender();
    }
  };

  const render = (): void => {
    if (loading) {
      el.innerHTML = `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Loading settings…</span></div>`;
      return;
    }
    if (!cfg) {
      el.innerHTML = `<div class="empty">
        <span class="empty-icon">${icon("edit", 28)}</span>
        <h2 class="empty-title">Settings unavailable</h2>
        <p class="empty-sub">The backend did not return a configuration.</p>
      </div>`;
      return;
    }

    const themeOptions = ["dark", "light"]
      .map(
        (t) =>
          `<option value="${t}" ${cfg!.theme === t ? "selected" : ""}>${t === "dark" ? "Dark" : "Light"}</option>`,
      )
      .join("");
    const textFields = TEXT_KEYS.map((key) => renderTextField(key, cfg!)).join("");
    const busy = saving !== null;

    el.innerHTML = `
      <div class="settings">
        <h2 class="detail-title">Settings</h2>
        <div class="settings-section">
          <h3 class="detail-title">Appearance</h3>
          <div class="field settings-field">
            <label class="field-label" for="settings-theme">Theme</label>
            <div class="input-row">
              <span class="input-icon">${icon("eye", 15)}</span>
              <select class="input" id="settings-theme" data-theme ${busy ? "disabled" : ""}>
                ${themeOptions}
              </select>
            </div>
          </div>
        </div>

        <div class="settings-section">
          <h3 class="detail-title">Behavior</h3>
          ${renderCheckbox("auto_backup", "Auto-backup", "Snapshot addons before every change (fixes, installs, switches).")}
          ${renderCheckbox("confirmations", "Confirmations", "Ask before destructive operations in the app.")}
        </div>

        <div class="settings-section">
          <h3 class="detail-title">Paths & API key</h3>
          ${textFields}
        </div>

        <div class="settings-section">
          <h3 class="detail-title">Managed elsewhere</h3>
          <p class="field-hint">These keys come from Setup and Collections — edit them there.</p>
          <div class="settings-readonly">
            ${renderReadonly("wow_path", cfg!.wow_path || "—")}
            ${renderReadonly("flavor", cfg!.flavor || "—")}
            ${renderReadonly("profile", cfg!.profile || "—")}
            ${renderReadonly("collection", cfg!.collection || "—")}
          </div>
        </div>

        ${error ? `<p class="settings-error" role="alert" style="color: var(--error)">${icon("x-circle", 14)}<span>${escapeHtml(error)}</span></p>` : ""}
      </div>`;

    el.querySelector<HTMLSelectElement>("[data-theme]")?.addEventListener("change", (e) => {
      void save("theme", (e.target as HTMLSelectElement).value);
    });
    el.querySelectorAll<HTMLInputElement>("[data-check]").forEach((input) => {
      input.addEventListener("change", () => {
        void save(input.dataset.check ?? "", input.checked ? "true" : "false");
      });
    });
    el.querySelectorAll<HTMLInputElement>("[data-text]").forEach((input) => {
      const key = input.dataset.text ?? "";
      input.addEventListener("input", () => {
        drafts[key] = input.value;
        refocus = { sel: `[data-text="${key}"]`, pos: input.selectionStart ?? input.value.length };
        rerender();
      });
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          void save(key, input.value.trim());
        }
      });
    });
    el.querySelectorAll<HTMLElement>("[data-save]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const key = btn.dataset.save ?? "";
        void save(key, (drafts[key] ?? "").trim());
      });
    });
  };

  const renderTextField = (key: string, c: ConfigView): string => {
    const value = drafts[key] ?? String(c[key as keyof ConfigView] ?? "");
    const savingKey = saving === key;
    const isSecret = key === "curseforge_api_key";
    return `
      <div class="field settings-field">
        <label class="field-label" for="settings-${key}">${KEY_LABELS[key] ?? key}</label>
        <div class="input-row">
          <span class="input-icon">${icon(isSecret ? "lock" : "folder", 15)}</span>
          <input class="input" id="settings-${key}" type="${isSecret ? "password" : "text"}"
            spellcheck="false" autocomplete="off" value="${escapeAttr(value)}"
            data-text="${key}" ${saving ? "disabled" : ""} />
          <button class="btn btn-outline btn-sm" data-save="${key}" ${saving || value === String(c[key as keyof ConfigView]) ? "disabled" : ""}>
            ${savingKey ? `<span class="spinner spinner-xs"></span>` : ""}
            <span>${savingKey ? "Saving…" : "Save"}</span>
          </button>
        </div>
      </div>`;
  };

  const renderCheckbox = (key: string, label: string, hint: string): string => {
    const on = Boolean(cfg![key as keyof ConfigView]);
    const savingKey = saving === key;
    return `
      <label class="checkbox-row settings-check">
        <input type="checkbox" class="checkbox" data-check="${key}" ${on ? "checked" : ""} ${saving ? "disabled" : ""} />
        <span class="checkbox-box">${icon("check", 13)}</span>
        <span class="checkbox-text">
          <span>${label}${savingKey ? " — saving…" : ""}</span>
          <span class="checkbox-hint">${hint}</span>
        </span>
      </label>`;
  };

  const renderReadonly = (key: string, value: string): string => `
    <div class="settings-readonly-row">
      <span class="field-label">${KEY_LABELS[key] ?? key}</span>
      <span class="mono muted">${escapeHtml(value)}</span>
    </div>`;

  render();
  void load();

  return { refresh: rerender };
}

/** Config values arrive as JSON strings/bools; SetConfigKey takes strings. */
function coerceValue(key: string, value: string): unknown {
  if (key === "auto_backup" || key === "confirmations") return value === "true";
  return value;
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
