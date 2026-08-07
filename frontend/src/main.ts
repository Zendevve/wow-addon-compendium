// App shell and store. Owns the header, tab bar, status bar and the
// orchestration of every backend call; the four views are mounted into
// the content area and only read state / call actions.

import "./style.css";
import { service, mockActive } from "./api";
import type {
  State,
  Profile,
  ScanResult,
  ValidateResult,
  InstallResult,
  View,
  Addon,
  FixResult,
} from "./types";
import { DESTRUCTIVE_ACTIONS } from "./types";
import type { AppState, Actions } from "./app";
import { icon } from "./icons";
import { mountToasts, toast } from "./components/toast";
import { mountDialog, confirmDialog } from "./components/dialog";
import { mountSetup } from "./views/setup";
import { mountScan } from "./views/scan";
import { mountValidate } from "./views/validate";
import { mountInstall } from "./views/install";

const appEl = document.getElementById("app")!;
appEl.innerHTML = `
  <div class="app">
    <header class="header" id="header"></header>
    <nav class="tabbar" id="tabbar" aria-label="Views"></nav>
    <main class="main"><div class="main-inner" id="content"></div></main>
    <footer class="statusbar" id="statusbar"></footer>
  </div>`;

const header = appEl.querySelector<HTMLElement>("#header")!;
const tabbar = appEl.querySelector<HTMLElement>("#tabbar")!;
const content = appEl.querySelector<HTMLElement>("#content")!;
const statusbar = appEl.querySelector<HTMLElement>("#statusbar")!;

mountToasts(appEl);
mountDialog(appEl);

const TABS: { view: View; label: string; glyph: string }[] = [
  { view: "scan", label: "Scan", glyph: "list" },
  { view: "validate", label: "Validation", glyph: "table" },
  { view: "install", label: "Install", glyph: "package" },
];

let app: AppState;
let current: { refresh: () => void } | null = null;

function boot(): void {
  void (async () => {
    try {
      const [state, profiles] = await Promise.all([
        service.GetState(),
        service.Profiles(),
      ]);
      const requested = new URLSearchParams(window.location.search).get("view");
      const initialView: View = state.has_install
        ? requested === "validate" || requested === "install" || requested === "scan"
          ? requested
          : "scan"
        : "setup";
      app = {
        state,
        profiles,
        scan: null,
        validation: null,
        installResult: null,
        view: initialView,
        busy: null,
        filter: "",
        mock: mockActive,
      };
      renderHeader();
      renderTabs();
      renderStatus();
      mountView();
    } catch (err) {
      appEl.innerHTML = `<div class="fatal">${icon("x-circle", 26)}
        <h1>Could not start wowfix</h1>
        <p>${escapeHtml(errText(err, "The backend connection failed."))}</p></div>`;
    }
  })();
}

const actions: Actions = {
  go(view: View): void {
    app.view = view;
    renderTabs();
    mountView();
  },

  async scan(): Promise<void> {
    if (app.busy || !app.state.has_install) return;
    app.busy = "scan";
    syncChrome();
    try {
      const res = await service.Scan();
      app.scan = res;
      const s = res.stats;
      toast({
        type: s.errors > 0 ? "error" : s.problems > 0 ? "warn" : "ok",
        title: "Scan complete",
        message: `${s.total} addons · ${s.problems} with issues${s.errors ? ` · ${s.errors} error${s.errors === 1 ? "" : "s"}` : ""}`,
      });
    } catch (err) {
      toast({ type: "error", title: "Scan failed", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async fixOne(addon: Addon): Promise<void> {
    if (app.busy) return;
    const destructive = addon.issues.find((i) =>
      DESTRUCTIVE_ACTIONS.has(i.action),
    );
    if (destructive) {
      const confirmed = await confirmDialog({
        title: `Move “${addon.folder_name}” to the trash?`,
        message:
          "This fix is destructive: the folder is moved to the OS trash (never permanently deleted).",
        details: destructive.suggestion ? [destructive.suggestion] : undefined,
        confirmLabel: "Move to Trash",
        danger: true,
      });
      if (!confirmed) return;
    }
    app.busy = "fix";
    syncChrome();
    current?.refresh();
    try {
      const res = await service.Fix(addon.folder_name, destructive ? true : false);
      await rescanAfterMutation();
      toastFixResult(res, "Fix applied");
    } catch (err) {
      toast({ type: "error", title: "Fix failed", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async fixAll(): Promise<void> {
    if (app.busy || !app.scan) return;
    const fixable = app.scan.addons.filter((a) => a.fixable);
    if (fixable.length === 0) return;
    const destructive = fixable.filter((a) =>
      a.issues.some((i) => DESTRUCTIVE_ACTIONS.has(i.action)),
    );
    const allowDestructive = destructive.length > 0;
    if (allowDestructive) {
      const safe = fixable.length - destructive.length;
      const confirmed = await confirmDialog({
        title: `Fix all ${fixable.length} addons?`,
        message: `${destructive.length} destructive (move to trash / merge) and ${safe} safe fix${safe === 1 ? "" : "es"} will be applied.`,
        details: [
          "A backup snapshot is created before every change.",
          "Nothing is deleted permanently — removals go to the OS trash.",
        ],
        confirmLabel: `Fix All (${fixable.length})`,
        danger: true,
      });
      if (!confirmed) return;
    }
    app.busy = "fixall";
    syncChrome();
    current?.refresh();
    try {
      const res = await service.FixAll(allowDestructive);
      await rescanAfterMutation();
      toastFixResult(res, "Fix All complete");
    } catch (err) {
      toast({ type: "error", title: "Fix All failed", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async setProfile(id: string): Promise<void> {
    if (app.busy) return;
    app.busy = "profile";
    syncChrome();
    try {
      await service.SetProfile(id);
      const profile = app.profiles.find((p) => p.id === id);
      app.state = {
        ...app.state,
        profile_id: id,
        profile_name: profile?.name ?? app.state.profile_name,
      };
      if (app.state.has_install) {
        app.scan = await service.Scan();
        app.validation = null;
      }
      toast({
        type: "ok",
        title: "Profile switched",
        message: `${profile?.name ?? id}`,
      });
    } catch (err) {
      toast({ type: "error", title: "Could not switch profile", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async validate(): Promise<void> {
    if (app.busy) return;
    app.busy = "validate";
    syncChrome();
    current?.refresh();
    try {
      app.validation = await service.Validate();
    } catch (err) {
      toast({ type: "error", title: "Validation failed", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async installZip(zipPath: string, allowReplace: boolean): Promise<void> {
    if (app.busy) return;
    app.busy = "install";
    app.installResult = null;
    syncChrome();
    current?.refresh();
    try {
      const res = await service.InstallZip(zipPath, allowReplace);
      app.installResult = res;
      if (res.errors.length > 0) {
        toast({
          type: "error",
          title: "Install completed with errors",
          message: `${res.errors.length} error${res.errors.length === 1 ? "" : "s"} — see the result panel`,
        });
      } else if (res.installed > 0) {
        toast({
          type: "ok",
          title: "Addon installed",
          message: `${res.installed} installed · ${res.replaced} replaced · ${res.skipped} skipped`,
        });
      } else if (res.replaced > 0) {
        toast({
          type: "ok",
          title: "Addon replaced",
          message: `Replaced ${res.replaced} existing folder${res.replaced === 1 ? "" : "s"} after backup`,
        });
      } else if (res.skipped > 0) {
        toast({
          type: "info",
          title: "Already installed",
          message: "The addon exists and replace is off — nothing was changed.",
        });
      }
    } catch (err) {
      toast({ type: "error", title: "Install failed", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async completeSetup(root: string, flavor: string, profileId: string): Promise<void> {
    app.busy = "setup";
    syncChrome();
    try {
      await service.SetInstall(root, flavor);
      await service.SetProfile(profileId);
      const [state, scan] = await Promise.all([service.GetState(), service.Scan()]);
      app.state = state;
      app.scan = state.has_install ? scan : null;
      app.validation = null;
      toast({
        type: "ok",
        title: "Install configured",
        message: state.has_install ? "Scanning your addons…" : undefined,
      });
    } catch (err) {
      throw err;
    } finally {
      app.busy = null;
      syncChrome();
    }
  },
};

async function rescanAfterMutation(): Promise<void> {
  app.scan = await service.Scan();
  app.validation = null;
}

function toastFixResult(res: FixResult, doneTitle: string): void {
  const failed = res.fixes.filter((f) => !f.ok);
  if (res.failed > 0) {
    toast({
      type: res.fixed > 0 ? "warn" : "error",
      title: `${res.failed} fix${res.failed === 1 ? "" : "es"} failed`,
      message: failed.map((f) => `${f.addon}: ${f.message}`).join(" · "),
    });
  } else if (res.fixed > 0) {
    toast({
      type: "ok",
      title: doneTitle,
      message: `${res.fixed} fix${res.fixed === 1 ? "" : "es"} applied`,
    });
  }
}

function renderHeader(): void {
  const hasInstall = app.state.has_install;
  const busy = app.busy !== null;
  const profileOptions = app.profiles
    .map(
      (p) =>
        `<option value="${p.id}" ${p.id === app.state.profile_id ? "selected" : ""}>${escapeHtml(p.name)}</option>`,
    )
    .join("");
  const fixableCount = app.scan
    ? app.scan.addons.filter((a) => a.fixable).length
    : 0;

  header.innerHTML = `
    <div class="header-left">
      <span class="brand-mark">${icon("shield", 22)}</span>
      <span class="brand-name">wowfix</span>
      <span class="brand-ver mono">v${escapeHtml(app.state.version)}</span>
    </div>
    <div class="header-center">
      ${
        hasInstall
          ? `<span class="path-chip" title="${escapeAttr(app.state.addons_dir || app.state.wow_path)}">
              ${icon("folder", 14)}<span class="path-chip-value mono">${escapeHtml(app.state.addons_dir || app.state.wow_path)}</span>
            </span>`
          : `<button class="btn btn-ghost btn-sm" data-setup-link>${icon("folder", 14)}<span>No install configured — set up</span></button>`
      }
    </div>
    <div class="header-right">
      ${
        hasInstall
          ? `<div class="select-wrap select-small">${icon("chevron-down", 13)}
              <select class="select" aria-label="Game version profile" data-profile ${busy ? "disabled" : ""}>
                ${profileOptions}
              </select>
            </div>`
          : ""
      }
      ${
        hasInstall
          ? `<button class="btn btn-outline" data-scan ${busy ? "disabled" : ""}>
              ${app.busy === "scan" ? `<span class="spinner"></span>` : icon("refresh", 16)}
              <span>${app.busy === "scan" ? "Scanning…" : "Scan"}</span>
            </button>
            <button class="btn btn-primary" data-fixall ${busy || fixableCount === 0 ? "disabled" : ""}>
              ${app.busy === "fixall" ? `<span class="spinner"></span>` : icon("wrench", 16)}
              <span>${app.busy === "fixall" ? "Fixing…" : `Fix All${fixableCount ? ` (${fixableCount})` : ""}`}</span>
            </button>`
          : ""
      }
    </div>`;

  header.querySelector("[data-profile]")?.addEventListener("change", (e) => {
    void actions.setProfile((e.target as HTMLSelectElement).value);
  });
  header.querySelector("[data-scan]")?.addEventListener("click", () => void actions.scan());
  header.querySelector("[data-fixall]")?.addEventListener("click", () => void actions.fixAll());
  header.querySelector("[data-setup-link]")?.addEventListener("click", () => actions.go("setup"));
}

function renderTabs(): void {
  if (app.view === "setup") {
    appEl.classList.add("no-tabs");
    tabbar.innerHTML = "";
    return;
  }
  appEl.classList.remove("no-tabs");
  const problemBadge = app.scan && app.scan.stats.problems > 0 ? app.scan.stats.problems : 0;
  tabbar.innerHTML = `
    <div class="tabs" role="tablist" aria-label="Sections">
      ${TABS.map(
        (t, i) => `
        <button class="tab${app.view === t.view ? " active" : ""}" role="tab" aria-selected="${app.view === t.view}"
          data-tab="${i}" tabindex="${app.view === t.view ? 0 : -1}">
          ${icon(t.glyph as never, 15)}
          <span>${t.label}</span>
          ${t.view === "scan" && problemBadge ? `<span class="tab-badge">${problemBadge}</span>` : ""}
        </button>`,
      ).join("")}
    </div>`;

  const tabs = Array.from(tabbar.querySelectorAll<HTMLButtonElement>(".tab"));
  tabs.forEach((tab, i) => {
    tab.addEventListener("click", () => {
      actions.go(TABS[i].view);
      tab.focus();
    });
    tab.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
      e.preventDefault();
      const next = (i + (e.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length;
      actions.go(TABS[next].view);
      tabs[next].focus();
    });
  });
}

function renderStatus(): void {
  const s = app.scan?.stats;
  const profile = app.profiles.find((p) => p.id === app.state.profile_id);
  const busyChip = app.busy
    ? `<span class="status-chip busy"><span class="spinner spinner-xs"></span>${escapeHtml(app.busy)}</span>`
    : "";
  const scanLine = s
    ? `<span class="status-chip"><span class="status-dot ok"></span>${s.total} addons</span>
       <span class="status-chip"><span class="status-dot warn"></span>${s.problems} with issues</span>
       ${s.errors > 0 ? `<span class="status-chip"><span class="status-dot error"></span>${s.errors} error${s.errors === 1 ? "" : "s"}</span>` : ""}`
    : `<span class="status-chip">${app.state.has_install ? "Ready — run a scan" : "Set up your WoW install"}</span>`;

  statusbar.innerHTML = `
    <div class="statusbar-left">${busyChip}${scanLine}</div>
    <div class="statusbar-right">
      ${
        app.state.has_install
          ? `<span class="status-chip">${escapeHtml(profile?.name ?? app.state.profile_name)}${profile ? ` · ${profile.interface}` : ""}</span>
             <span class="status-chip">${app.state.auto_backup ? "auto-backup on" : "auto-backup off"}</span>`
          : ""
      }
      ${app.mock ? `<span class="status-chip mock">MOCK</span>` : ""}
      <span class="status-chip mono">v${escapeHtml(app.state.version)}</span>
    </div>`;
}

function syncChrome(): void {
  renderHeader();
  renderTabs();
  renderStatus();
}

function mountView(): void {
  content.innerHTML = "";
  current = null;
  switch (app.view) {
    case "setup":
      current = mountSetup(content, app, actions);
      break;
    case "scan":
      current = mountScan(content, app, actions);
      break;
    case "validate":
      current = mountValidate(content, app, actions);
      break;
    case "install":
      current = mountInstall(content, app, actions);
      break;
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
function escapeAttr(s: string): string {
  return escapeHtml(s).replaceAll("'", "&#39;");
}
const ESC: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};

boot();
