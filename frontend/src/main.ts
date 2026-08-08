// App shell and store. Owns the header, tab bar, status bar and the
// orchestration of every backend call; the four views are mounted into
// the content area and only read state / call actions.

import "@fontsource-variable/inter";
import "@fontsource-variable/mona-sans";
import "./style.css";
import { service, mockActive } from "./api";
import type {
  State,
  Profile,
  ScanResult,
  ScanStats,
  ValidateResult,
  InstallResult,
  View,
  Addon,
  FixResult,
} from "./types";
import { DESTRUCTIVE_ACTIONS } from "./types";
import type { AppState, Actions } from "./app";
import { icon, type IconName } from "./icons";
import { mountToasts, toast } from "./components/toast";
import { mountDialog, confirmDialog } from "./components/dialog";
import { mountPalette } from "./components/palette";
import { mountSetup } from "./views/setup";
import { mountScan } from "./views/scan";
import { mountDoctor } from "./views/doctor";
import { mountValidate } from "./views/validate";
import { mountOverview } from "./views/overview";
import { mountInstall } from "./views/install";
import { mountUpdates } from "./views/updates";
import { mountCatalog } from "./views/catalog";
import { mountCollections } from "./views/collections";
import { mountExportImport } from "./views/exportimport";
import { mountSavedVars } from "./views/savedvars";
import { mountBackups } from "./views/backups";
import { mountInstalls } from "./views/installs";
import { mountSettings } from "./views/settings";

const appEl = document.getElementById("app")!;
appEl.innerHTML = `
  <div class="app">
    <aside class="sidebar" id="sidebar" aria-label="Sidebar"></aside>
    <header class="header" id="header"></header>
    <main class="main"><div class="main-inner" id="content"></div></main>
    <footer class="statusbar" id="statusbar" aria-live="polite"></footer>
  </div>`;

// The CSS grid lives on the inner .app div, not on #app itself; every
// layout-state toggle (collapse, no-sidebar) must hit the grid element.
const grid = appEl.querySelector<HTMLElement>(".app")!;

const sidebar = appEl.querySelector<HTMLElement>("#sidebar")!;
const header = appEl.querySelector<HTMLElement>("#header")!;
const content = appEl.querySelector<HTMLElement>("#content")!;
const statusbar = appEl.querySelector<HTMLElement>("#statusbar")!;

mountToasts(appEl);
mountDialog(appEl);

// Command palette (Ctrl+K / Ctrl+P). The handler is registered once at
// startup; openPalette stays a no-op until boot() mounts the palette, and
// the palette's open() toggles (a second Ctrl+K closes it).
let openPalette: () => void = () => {};
document.addEventListener("keydown", (e) => {
  if (!e.ctrlKey && !e.metaKey) return;
  const key = e.key.toLowerCase();
  if (key !== "k" && key !== "p") return;
  e.preventDefault();
  openPalette();
});

// Navigation model: the flat list of primary destinations. Power workflows
// (scan/doctor/validation live under Overview; install, export/import, saved
// variables and installs stay reachable via deep links and actions) are
// deliberately out of the main visual path — order is taste, not contract.
const NAV: { view: View; label: string; glyph: IconName }[] = [
  { view: "overview", label: "Overview", glyph: "shield" },
  { view: "updates", label: "Updates", glyph: "refresh" },
  { view: "catalog", label: "Catalog", glyph: "search" },
  { view: "collections", label: "Collections", glyph: "stack" },
  { view: "backups", label: "Backups", glyph: "archive" },
  { view: "settings", label: "Settings", glyph: "edit" },
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
      const allowedViews: View[] = ["overview", "scan", "doctor", "validate", "install", "updates", "catalog", "collections", "exportimport", "savedvars", "backups", "installs", "settings"];
      const initialView: View = state.has_install
        ? allowedViews.includes(requested as View)
          ? (requested as View)
          : "overview"
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
      renderSidebar();
      renderHeader();
      renderNav();
      renderStatus();
      mountView();
      openPalette = mountPalette(appEl, app, actions).open;
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
    renderNav();
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
    const before = { ...app.scan.stats };
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
          "Nothing is deleted permanently. Removals go to the OS trash.",
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
      // Rescan immediately so the health panel and the toast reflect the
      // before/after diff, not just the fix batch.
      const after = await service.Scan();
      app.scan = after;
      app.validation = null;
      toastFixAllSummary(res, before, after.stats);
    } catch (err) {
      toast({ type: "error", title: "Fix All failed", message: errText(err) });
    } finally {
      app.busy = null;
      syncChrome();
      current?.refresh();
    }
  },

  async restoreAddon(addon: Addon): Promise<void> {
    if (app.busy || !app.state.has_install) return;
    app.busy = "restore";
    syncChrome();
    current?.refresh();
    try {
      const res = await service.RestoreAddon(addon.folder_name, true);
      await rescanAfterMutation();
      if (res.errors.length > 0) {
        toast({
          type: "error",
          title: "Restore failed",
          message: res.errors.join(" · "),
        });
      } else {
        toast({
          type: "ok",
          title: "Restore complete",
          message: `${addon.folder_name} re-downloaded from its source`,
        });
      }
    } catch (err) {
      toast({ type: "error", title: "Restore failed", message: errText(err) });
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
          message: `${res.errors.length} error${res.errors.length === 1 ? "" : "s"}. See the result panel`,
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
          message: "The addon exists and replace is off. Nothing was changed.",
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

// Toast the before/after scan diff of a Fix All run: how many problems were
// repaired and what the with-issues / error counts dropped to.
function toastFixAllSummary(res: FixResult, before: ScanStats, after: ScanStats): void {
  const failed = res.fixes.filter((f) => !f.ok);
  if (res.failed > 0) {
    toast({
      type: res.fixed > 0 ? "warn" : "error",
      title: `${res.failed} fix${res.failed === 1 ? "" : "es"} failed`,
      message: failed.map((f) => `${f.addon}: ${f.message}`).join(" · "),
    });
    return;
  }
  const fixedProblems = before.problems - after.problems;
  if (fixedProblems <= 0) {
    toast({
      type: "ok",
      title: "Fix All complete",
      message: `${before.problems} with issues → ${after.problems}, ${before.errors} error${before.errors === 1 ? "" : "s"} → ${after.errors}`,
    });
    return;
  }
  toast({
    type: "ok",
    title: `Fixed ${fixedProblems} problem${fixedProblems === 1 ? "" : "s"}`,
    message: `${before.problems} with issues → ${after.problems}, ${before.errors} error${before.errors === 1 ? "" : "s"} → ${after.errors}`,
  });
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

  header.innerHTML = `
    <div class="header-left">
      ${
        hasInstall
          ? `<span class="path-chip" title="${escapeAttr(app.state.addons_dir || app.state.wow_path)}">
              ${icon("folder", 14)}<span class="path-chip-value mono">${escapeHtml(app.state.addons_dir || app.state.wow_path)}</span>
            </span>`
          : `<button class="btn btn-ghost btn-sm" data-setup-link>${icon("folder", 14)}<span>No install configured - set up</span></button>`
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
            </button>`
          : ""
      }
    </div>`;

  header.querySelector("[data-profile]")?.addEventListener("change", (e) => {
    void actions.setProfile((e.target as HTMLSelectElement).value);
  });
  header.querySelector("[data-scan]")?.addEventListener("click", () => void actions.scan());
  header.querySelector("[data-setup-link]")?.addEventListener("click", () => actions.go("setup"));
}

// renderSidebar draws the brand block and the collapse toggle; the nav
// list itself is renderNav's job so scan-badge updates never rebuild the
// chrome around it.
function renderSidebar(): void {
  const collapsed = grid.classList.contains("collapsed");
  sidebar.innerHTML = `
    <div class="sidebar-brand">
      ${icon("shield", 20)}
      <span class="brand-name">wowfix</span>
      <span class="brand-ver mono">v${escapeHtml(app.state.version)}</span>
    </div>
    <nav class="nav" id="nav" aria-label="Views"></nav>
    <button class="sidebar-collapse" data-collapse aria-expanded="${!collapsed}" aria-controls="sidebar"
      title="${collapsed ? "Expand sidebar" : "Collapse sidebar"}">
      ${icon(collapsed ? "chevron-right" : "chevron-left", 15)}
    </button>`;
  sidebar.querySelector("[data-collapse]")?.addEventListener("click", () => {
    grid.classList.toggle("collapsed");
    renderSidebar();
    renderNav();
  });
}

function renderNav(): void {
  if (app.view === "setup") {
    grid.classList.add("no-sidebar");
    return;
  }
  grid.classList.remove("no-sidebar");
  const nav = sidebar.querySelector<HTMLElement>("#nav");
  if (!nav) return;
  const problemBadge = app.scan && app.scan.stats.problems > 0 ? app.scan.stats.problems : 0;
  // Scan / Doctor / Validation are Overview's segments; the nav reads
  // Overview as active while any of them is mounted (deep links bypass
  // the wrapper and mount those views full-screen).
  const activeView: View =
    app.view === "scan" || app.view === "doctor" || app.view === "validate"
      ? "overview"
      : app.view;
  nav.innerHTML = NAV.map(
    (t) => `
    <button class="nav-item${t.view === activeView ? " active" : ""}"${t.view === activeView ? ' aria-current="page"' : ""} data-view="${t.view}">
      ${icon(t.glyph, 17)}
      <span class="nav-item-label">${t.label}</span>
      ${t.view === "overview" && problemBadge ? `<span class="nav-badge">${problemBadge}</span>` : ""}
    </button>`,
  ).join("");

  const items = Array.from(nav.querySelectorAll<HTMLButtonElement>(".nav-item"));
  items.forEach((btn, i) => {
    btn.addEventListener("click", () => {
      actions.go(btn.dataset.view as View);
      btn.focus();
    });
    btn.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
      e.preventDefault();
      const next = (i + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length;
      actions.go(items[next].dataset.view as View);
      items[next].focus();
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
    : `<span class="status-chip">${app.state.has_install ? "Ready - run a scan" : "Set up your WoW install"}</span>`;

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
      ${app.state.has_install ? `<span class="status-chip mono muted">Ctrl+K</span>` : ""}
    </div>`;
}

function syncChrome(): void {
  renderHeader();
  renderNav();
  renderStatus();
  // Announce view-region busy state to assistive tech: true while any
  // backend operation is running, false again once idle.
  content.setAttribute("aria-busy", app.busy !== null ? "true" : "false");
}

function mountView(): void {
  content.innerHTML = "";
  current = null;
  switch (app.view) {
    case "setup":
      current = mountSetup(content, app, actions);
      break;
    case "overview":
      current = mountOverview(content, app, actions);
      break;
    case "scan":
      current = mountScan(content, app, actions);
      break;
    case "doctor":
      current = mountDoctor(content, app, actions);
      break;
    case "validate":
      current = mountValidate(content, app, actions);
      break;
    case "install":
      current = mountInstall(content, app, actions);
      break;
    case "updates":
      current = mountUpdates(content, app, actions);
      break;
    case "catalog":
      current = mountCatalog(content, app, actions);
      break;
    case "collections":
      current = mountCollections(content, app, actions);
      break;
    case "exportimport":
      current = mountExportImport(content, app, actions);
      break;
    case "savedvars":
      current = mountSavedVars(content, app, actions);
      break;
    case "backups":
      current = mountBackups(content, app, actions);
      break;
    case "installs":
      current = mountInstalls(content, app, actions);
      break;
    case "settings":
      current = mountSettings(content, app, actions);
      break;
  }
  // Restart the entry transition on view switches only: #content's child
  // is destroyed and recreated here, while in-view refreshes reuse it.
  content.classList.remove("view-in");
  void content.offsetWidth;
  content.classList.add("view-in");
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
