// Catalog — the single install surface. Search five providers (debounced,
// with per-provider filter chips whose tooltips carry the Sources()
// caveats), install from a result row, from the install bar (URL /
// owner-repo / addon name), or from a browsed/dropped ZIP. Curated
// private-server sets sit above the search as ≤2 gradient spotlight cards;
// Wago import strings save from their own section or from wago rows.
// Provider outages surface in the search error box — never suppressed.
// Install outcomes render per-addon status chips plus every error the
// backend reported (LEARNINGS #5/#6).

import type { View } from "../view";
import "./catalog.css";

import { mockActive, service } from "../api";
import { confirmDialog } from "../dialog";
import { icon, type IconName } from "../icons";
import { toast } from "../toast";
import { formatBytes } from "../types";
import type {
  CatalogEntry,
  CuratedAddon,
  CuratedResult,
  InfoResult,
  InstallResult,
  Provider,
  ProviderInfo,
  SearchCatalogResult,
  WagoImportResult,
} from "../types";

const DEBOUNCE_MS = 350;

const PROVIDER_LABEL: Record<Provider, string> = {
  github: "GitHub",
  curseforge: "CurseForge",
  wowinterface: "WowInterface",
  tukui: "Tukui",
  wago: "Wago",
};

const PROVIDER_ORDER: Provider[] = [
  "github",
  "curseforge",
  "wowinterface",
  "tukui",
  "wago",
];

// Classic-era catalog flavors all mean "vanilla" for compatibility
// (mirrors the backend's knownGameFamilies map).
const GAME_VERSION_NORMAL: Record<string, string> = {
  classic: "vanilla",
  hardcore: "vanilla",
  sod: "vanilla",
  turtle: "vanilla",
};

// Profile families normalize onto the compatibility vocabulary used by the
// curated manifests: vanilla | tbc | wrath | cata | retail.
const FAMILY_NORMAL: Record<string, string> = {
  vanilla: "vanilla",
  turtle: "vanilla",
  classic: "vanilla",
  hardcore: "vanilla",
  sod: "vanilla",
  tbc: "tbc",
  wrath: "wrath",
  cata: "cata",
  retail: "retail",
};

const GRADIENTS = ["-violet", "-magenta", "-orange", "-coral"];

function normalizeFamily(s: string): string {
  const k = s.trim().toLowerCase();
  return FAMILY_NORMAL[k] ?? k;
}

function normalizeGameVersion(s: string): string {
  const k = s.trim().toLowerCase();
  return GAME_VERSION_NORMAL[k] ?? k;
}

// The debounce timer lives at module scope so unmount() can cancel a
// pending search when the router swaps views.
let debounceTimer = 0;

export const view: View = {
  id: "catalog",
  label: "Catalog",
  icon: "search",
  mount(host) {
    mountCatalog(host);
  },
  unmount() {
    window.clearTimeout(debounceTimer);
  },
};

function mountCatalog(el: HTMLElement): void {
  let query = "";
  let provider: "all" | Provider = "all";
  let compatOnly = false;
  let result: SearchCatalogResult | null = null;
  let searching = false;
  let installing = false;
  let busySource: string | null = null; // the source whose control shows the spinner
  let saving = false;
  let savingId: string | null = null; // the wago id whose control shows the spinner
  let installResult: InstallResult | null = null;
  let wagoArg = "";
  let wagoResult: WagoImportResult | null = null;
  let curated: CuratedResult | null = null;
  let curatedError = false;
  let sources: ProviderInfo[] | null = null;
  let info: InfoResult | null = null;
  let infoLoading = false;
  let infoErr: string | null = null;
  let infoArg = "";
  let pendingFocus: string | null = null;
  let barSource: string | null = null; // source submitted from the install bar

  // --- search ------------------------------------------------------------

  const doSearch = async (q: string): Promise<void> => {
    query = q;
    if (!q.trim()) {
      result = null;
      rerender();
      return;
    }
    searching = true;
    rerender();
    try {
      result = await service.SearchCatalog(q);
    } catch (err) {
      toast({ type: "error", title: "Search failed", message: errText(err) });
      result = { results: [], errors: [errText(err)] };
    } finally {
      searching = false;
      rerender();
    }
  };

  const scheduleSearch = (): void => {
    window.clearTimeout(debounceTimer);
    debounceTimer = window.setTimeout(() => void doSearch(query), DEBOUNCE_MS);
  };

  const clearSearch = (): void => {
    query = "";
    window.clearTimeout(debounceTimer);
    result = null;
    rerender();
  };

  // --- installs -----------------------------------------------------------

  const markCuratedInstalled = (res: InstallResult): void => {
    if (!curated) return;
    const names = new Set([...res.installed, ...res.replaced]);
    for (const a of curated.addons) {
      if (names.has(a.name)) {
        a.installed = true;
        a.installed_version = undefined;
      }
    }
  };

  const applyInstallResult = (res: InstallResult): void => {
    installResult = res;
    markCuratedInstalled(res);
    // Honest surfacing: never a blanket success toast when rows failed.
    if (res.errors.length > 0) {
      toast({
        type: "error",
        title: "Install finished with errors",
        message: `${res.errors.length} error${res.errors.length === 1 ? "" : "s"} — see the result panel`,
      });
    } else if (res.installed.length > 0) {
      toast({
        type: "ok",
        title: "Addon installed",
        message:
          `${res.installed.join(", ")}` +
          `${res.replaced.length > 0 ? ` · ${res.replaced.length} replaced` : ""}` +
          `${res.skipped.length > 0 ? ` · ${res.skipped.length} skipped` : ""}`,
      });
    } else if (res.replaced.length > 0) {
      toast({ type: "ok", title: "Addon replaced", message: `${res.replaced.join(", ")} replaced after backup` });
    } else if (res.skipped.length > 0) {
      toast({ type: "info", title: "Already installed", message: "The addon exists and replace is off. Nothing changed." });
    }
  };

  const installSource = async (source: string, label: string): Promise<void> => {
    if (installing) return;
    const confirmed = await confirmDialog({
      title: `Install ${label}?`,
      message: `Install “${label}” into your addons folder? An existing folder is backed up first.`,
      details: [source],
      confirmLabel: "Install",
    });
    if (!confirmed) return;
    installing = true;
    busySource = source;
    rerender();
    try {
      const res = await service.InstallSource(source, true);
      applyInstallResult(res);
    } catch (err) {
      toast({ type: "error", title: "Install failed", message: errText(err) });
    } finally {
      installing = false;
      busySource = null;
      rerender();
    }
  };

  const installZipPath = async (path: string): Promise<void> => {
    if (installing) return;
    installing = true;
    busySource = path;
    rerender();
    try {
      const res = await service.InstallZip(path, false);
      applyInstallResult(res);
    } catch (err) {
      toast({ type: "error", title: "Install failed", message: errText(err) });
    } finally {
      installing = false;
      busySource = null;
      rerender();
    }
  };

  const installZipFile = async (file: File): Promise<void> => {
    if (installing) return;
    const path = zipPathOf(file);
    if (!path) {
      toast({
        type: "error",
        title: "Could not resolve file path",
        message: "This build cannot read the file location. Use Browse… instead.",
      });
      return;
    }
    await installZipPath(path);
  };

  // --- wago imports -------------------------------------------------------

  const saveImport = async (id: string): Promise<void> => {
    const arg = id.trim();
    if (!arg || saving) return;
    saving = true;
    savingId = arg;
    rerender();
    try {
      const res = await service.SaveWagoImport(arg);
      wagoResult = res;
      wagoArg = arg;
      toast({ type: "ok", title: "Import saved", message: res.applied_hint });
    } catch (err) {
      toast({ type: "error", title: "Save failed", message: errText(err) });
    } finally {
      saving = false;
      savingId = null;
      rerender();
    }
  };

  // --- curated band -------------------------------------------------------

  const loadCurated = async (): Promise<void> => {
    try {
      curated = await service.Curated();
      curatedError = false;
    } catch (err) {
      curated = null;
      curatedError = true;
      toast({ type: "error", title: "Could not load recommendations", message: errText(err) });
    }
    rerender();
  };

  // --- addon info ---------------------------------------------------------

  const fetchInfo = async (arg: string): Promise<void> => {
    if (infoLoading) return;
    infoArg = arg;
    info = null;
    infoErr = null;
    infoLoading = true;
    rerender();
    try {
      info = await service.AddonInfo(arg);
    } catch (err) {
      infoErr = errText(err);
      toast({ type: "error", title: "Addon info failed", message: errText(err) });
    } finally {
      infoLoading = false;
      rerender();
    }
  };

  const closeInfo = (): void => {
    info = null;
    infoErr = null;
    infoArg = "";
    rerender();
  };

  // --- provider details (chip tooltips) -----------------------------------

  const loadSources = async (): Promise<void> => {
    try {
      sources = await service.Sources();
    } catch (err) {
      toast({ type: "warn", title: "Could not load provider details", message: errText(err) });
    }
    rerender();
  };

  const normProviderName = (s: string): string => s.trim().toLowerCase().replace(/\s+/g, "");

  const sourceTooltip = (p: Provider): string => {
    const desc = sources?.find((s) => normProviderName(s.name) === p)?.description;
    return desc ?? `${PROVIDER_LABEL[p]} — provider status unknown right now.`;
  };

  // --- focus preservation -------------------------------------------------
  // Rerenders rebuild the DOM and drop focus to <body>. Capture what had
  // focus before the render and restore it after; pendingFocus survives
  // async flows (install / save / info) whose final render happens after
  // the backend call while the control is disabled.

  const focusKeyOf = (active: HTMLElement): string | null => {
    if (active.closest("[data-search]")) return "search";
    if (active.closest("[data-source]")) return "source";
    if (active.closest("[data-wago]")) return "wago";
    const chip = active.closest<HTMLElement>("[data-filter]");
    if (chip) return `filter:${chip.dataset.filter}`;
    const row = active.closest<HTMLElement>("[data-install-row]");
    if (row) return `install:${row.dataset.installRow}`;
    const match = active.closest<HTMLElement>("[data-info-match]");
    if (match) return `match:${match.dataset.infoMatch}`;
    if (active.closest("[data-info-install]")) return "info-install";
    if (active.closest("[data-info-close]")) return "info-close";
    if (active.closest("[data-wago-save]")) return "wago-save";
    if (active.closest("[data-curated-install]")) return "curated-install";
    return null;
  };

  const restoreFocus = (key: string | null): boolean => {
    if (!key) return false;
    const find = <T extends HTMLElement>(sel: string): T | null => el.querySelector<T>(sel);
    const simple: Record<string, string> = {
      search: "[data-search]",
      source: "[data-source]",
      wago: "[data-wago]",
      "info-install": "[data-info-install]",
      "info-close": "[data-info-close]",
      "wago-save": "[data-wago-save]",
      "curated-install": "[data-curated-install]",
    };
    if (key in simple) {
      const t = find(simple[key]);
      if (t && !t.hasAttribute("disabled")) {
        t.focus();
        if (t instanceof HTMLInputElement) t.setSelectionRange(t.value.length, t.value.length);
        return true;
      }
      return false;
    }
    const [kind, id] = key.split(":");
    if (kind === "filter") {
      const t = find<HTMLElement>(`[data-filter="${id}"]`);
      if (t) {
        t.focus();
        return true;
      }
    } else if (kind === "install") {
      const t = find<HTMLElement>(`[data-install-row="${id}"]`);
      if (t && !t.hasAttribute("disabled")) {
        t.focus();
        return true;
      }
    } else if (kind === "match") {
      const t = find<HTMLElement>(`[data-info-match="${id}"]`);
      if (t) {
        t.focus();
        return true;
      }
    }
    return false;
  };

  const rerender = (): void => {
    const active = document.activeElement;
    const key = pendingFocus ?? (active instanceof HTMLElement ? focusKeyOf(active) : null);
    render();
    if (key && restoreFocus(key)) pendingFocus = null;
    else if (!searching && !installing && !saving && !infoLoading) pendingFocus = null;
  };

  // --- render --------------------------------------------------------------

  const render = (): void => {
    const results = result?.results ?? [];
    const searchErrors = result?.errors ?? [];
    const familyNorm = normalizeFamily(curated?.family ?? "");
    const compatOk = (r: CatalogEntry): boolean =>
      !r.game_version || normalizeGameVersion(r.game_version) === familyNorm;
    const filtered =
      provider === "all" ? results : results.filter((r) => r.provider === provider);
    const shown = compatOnly ? filtered.filter(compatOk) : filtered;
    const compatCount = results.filter(compatOk).length;
    const uninstalled = (curated?.addons ?? []).filter((a) => !a.installed);
    const featured = uninstalled.slice(0, 2);
    const installedCount = (curated?.addons ?? []).length - uninstalled.length;

    const chipsHtml = `
      <button class="chip-btn${provider === "all" ? " active" : ""}" data-filter="all" aria-pressed="${provider === "all"}">
        <span>All</span>${result ? `<span class="chip-count">${results.length}</span>` : ""}
      </button>
      ${PROVIDER_ORDER.map(
        (p) => `
        <button class="chip-btn${provider === p ? " active" : ""}" data-filter="${p}" aria-pressed="${provider === p}" title="${escapeAttr(sourceTooltip(p))}">
          <span>${PROVIDER_LABEL[p]}</span>${result ? `<span class="chip-count">${results.filter((r) => r.provider === p).length}</span>` : ""}
        </button>`,
      ).join("")}
      ${familyNorm
        ? `<button class="chip-btn${compatOnly ? " active" : ""}" data-compat-toggle aria-pressed="${compatOnly}" title="Hide results built for other game versions">
            ${icon("shield", 13)}<span>${escapeHtml(familyNorm)}</span>${compatOnly ? `<span class="chip-count">${compatCount}</span>` : ""}
          </button>`
        : ""}`;

    const bandLabel = curated?.label || curated?.family || "";
    const curatedHtml =
      curated && curated.addons.length > 0
        ? `
        <section class="curated" aria-label="Curated addons">
          <div class="curated-head">
            <h2 class="curated-title">${icon("shield", 18)}<span>Recommended for ${escapeHtml(bandLabel)}</span></h2>
            <p class="curated-sub">Verified private-server sets, installed from the catalog source.</p>
          </div>
          <div class="curated-grid">
            ${featured
              .map(
                (a, i) => `
                <article class="spotlight-card curated-card ${GRADIENTS[i % GRADIENTS.length]}">
                  <div class="curated-card-top">
                    <span class="spotlight-kicker">Curated · ${escapeHtml(bandLabel)}</span>
                    <span class="spotlight-title">${escapeHtml(a.name)}</span>
                    <span class="spotlight-body">${escapeHtml(a.summary)}</span>
                  </div>
                  <div class="curated-card-foot">
                    <span class="curated-source mono" title="${escapeAttr(a.homepage || a.source)}">${escapeHtml(a.source)}</span>
                    <button class="btn-translucent" data-curated-install="${i}" ${installing ? "disabled" : ""}>
                      ${installing && busySource === a.source ? `<span class="spinner"></span>` : icon("download", 14)}
                      <span>${installing && busySource === a.source ? "Installing…" : "Install"}</span>
                    </button>
                  </div>
                </article>`,
              )
              .join("")}
          </div>
          ${installedCount > 0 ? `<p class="curated-more">${installedCount} of ${curated.addons.length} already installed — search above for more.</p>` : ""}
        </section>`
        : curatedError
          ? `<p class="curated-more">Recommendations are unavailable right now — search still works.</p>`
          : "";

    const resultsHtml = (() => {
      if (!result) {
        return emptyState(
          "search",
          "Search the addon catalog",
          "Find addons across GitHub, CurseForge, WowInterface, Tukui and Wago — or paste a URL, owner/repo or addon name into the install bar.",
        );
      }
      if (searching) {
        return `<div class="list-loading"><span class="spinner"></span><span>Searching…</span></div>`;
      }
      if (shown.length === 0) {
        const title =
          compatOnly && filtered.length > 0
            ? `No addons compatible with ${escapeHtml(familyNorm)}`
            : query
              ? `No addons found for “${escapeHtml(query)}”`
              : "No addons found";
        const body =
          compatOnly && filtered.length > 0
            ? "Try turning off the compatibility filter."
            : "Try a different query or provider filter.";
        return emptyState("search", title, body);
      }
      return `<div class="catalog-rows">${shown.map((r, i) => resultRow(r, i)).join("")}</div>`;
    })();

    const errorBox = searchErrors.length
      ? `
      <div class="error-box" role="alert">
        <span class="error-box-head">${icon("alert", 15)}<span>${searchErrors.length === 1 ? "One provider reported a problem" : `${searchErrors.length} problems while searching`}</span></span>
        <ul>${searchErrors.map((e) => `<li>${escapeHtml(e)}</li>`).join("")}</ul>
      </div>`
      : "";

    const installPanel = installResult ? resultPanel(installResult) : "";
    const wagoPanel = wagoResult ? wagoResultPanel(wagoResult) : "";
    const infoPanel = infoPanelHtml();

    el.innerHTML = `
      <div class="catalog">
        <header class="catalog-head">
          <h1 class="catalog-head-title">Catalog</h1>
          <p class="catalog-head-sub">Search five providers and the curated set, then install straight from the results — or paste a source into the bar below.</p>
        </header>

        <section class="install-bar" data-bar aria-label="Install an addon">
          <div class="install-bar-main">
            <span class="install-bar-icon">${icon("download", 17)}</span>
            <input class="text-input install-bar-input" data-source type="text"
              placeholder="Paste a URL, owner/repo or addon name…" spellcheck="false" autocomplete="off"
              aria-label="Install from URL or owner/repo" />
            <button class="btn-primary" data-install ${installing ? "disabled" : ""}>
              ${installing && busySource === barSource ? `<span class="spinner"></span>` : icon("download", 16)}
              <span>${installing && busySource === barSource ? "Installing…" : "Install"}</span>
            </button>
            <button class="btn-secondary" data-browse ${installing ? "disabled" : ""} title="Install a local addon ZIP archive">
              ${icon("folder", 16)}<span>Browse…</span>
            </button>
          </div>
          <div class="install-bar-hint">${icon("info", 13)}<span>Accepts a URL, owner/repo or addon name — or drop a .zip anywhere on this bar.</span></div>
          <input type="file" class="file-input" accept=".zip,application/zip,application/x-zip-compressed" hidden />
        </section>

        ${curatedHtml}

        <div class="search-box">
          <span class="search-icon">${icon("search", 16)}</span>
          <input class="text-input search-input" type="text" placeholder="Search addons by name, author or summary…"
            spellcheck="false" autocomplete="off" value="${escapeAttr(query)}" aria-label="Search addon catalog" data-search />
          ${query ? `<button class="search-clear" data-search-clear aria-label="Clear search">${icon("x", 14)}</button>` : ""}
        </div>

        <div class="filter-chips" role="group" aria-label="Filter by provider">${chipsHtml}</div>

        ${errorBox}

        ${resultsHtml}

        ${installPanel}

        ${wagoPanel}

        ${infoPanel}

        ${wagoSectionHtml()}
      </div>`;

    bind(el, { shown, featured });
  };

  const resultRow = (r: CatalogEntry, i: number): string => {
    const rowBusy = installing && busySource === r.id;
    return `
    <div class="catalog-row">
      <div class="catalog-row-info">
        <div class="catalog-row-name-line">
          <span class="catalog-row-name">${escapeHtml(r.name)}</span>
          ${r.game_version ? `<span class="game-badge tnum">${escapeHtml(r.game_version)}</span>` : ""}
        </div>
        <span class="catalog-row-author">${escapeHtml(r.author)}</span>
        <span class="catalog-row-summary">${escapeHtml(r.summary)}</span>
        <span class="catalog-row-id mono">${escapeHtml(r.id)}</span>
      </div>
      <div class="catalog-row-meta">
        <span class="catalog-ver mono tnum">v${escapeHtml(r.latest_version || "n/a")}</span>
        ${providerChip(r.provider, sourceTooltip(r.provider))}
      </div>
      <div class="catalog-row-actions">
        <button class="btn-secondary btn-sm" data-info-row="${i}" ${infoLoading ? "disabled" : ""} title="Details for ${escapeAttr(r.name)}">
          ${icon("info", 14)}<span>Info</span>
        </button>
        ${
          r.provider === "wago"
            ? `<button class="btn-secondary btn-sm" data-save-import="${i}" ${saving ? "disabled" : ""} title="Save the import string for in-game import">
                ${saving && savingId === r.id ? `<span class="spinner"></span>` : icon("save", 14)}<span>Save import</span>
              </button>`
            : `<button class="btn-primary btn-sm" data-install-row="${i}" ${installing ? "disabled" : ""}>
                ${rowBusy ? `<span class="spinner"></span>` : icon("download", 14)}<span>${rowBusy ? "Installing…" : "Install"}</span>
              </button>`
        }
      </div>
    </div>`;
  };

  const resultPanel = (res: InstallResult): string => {
    const hasErrors = res.errors.length > 0;
    const group = (label: string, names: string[], cls: string): string =>
      names.length === 0
        ? ""
        : `
        <div class="result-group">
          <span class="result-group-label">${label}</span>
          <div class="chip-list">${names.map((n) => `<span class="chip ${cls}">${escapeHtml(n)}</span>`).join("")}</div>
        </div>`;
    const summary = [
      res.installed.length ? `${res.installed.length} installed` : "",
      res.replaced.length ? `${res.replaced.length} replaced` : "",
      res.skipped.length ? `${res.skipped.length} skipped` : "",
      hasErrors ? `${res.errors.length} error${res.errors.length === 1 ? "" : "s"}` : "",
    ]
      .filter(Boolean)
      .join(" · ");
    return `
    <section class="result-panel" role="status" aria-label="Install results">
      <div class="result-panel-head">
        <span class="result-panel-icon ${hasErrors ? "is-error" : "is-ok"}">${icon(hasErrors ? "alert" : "check-circle", 18)}</span>
        <div class="result-panel-title">
          <b>${hasErrors ? "Install finished with errors" : "Install complete"}</b>
          <span class="result-panel-sub">${summary || "Nothing was installed."}</span>
        </div>
        <button class="icon-btn" data-result-dismiss aria-label="Dismiss result">${icon("x", 15)}</button>
      </div>
      <div class="result-groups">
        ${group("Installed", res.installed, "chip-dot chip-ok")}
        ${group("Replaced", res.replaced, "chip-dot chip-warn")}
        ${group("Skipped", res.skipped, "chip-dot chip-skip")}
      </div>
      ${hasErrors ? `<ul class="result-errors">${res.errors.map((e) => `<li class="error-row"><span class="chip chip-error">Error</span><span>${escapeHtml(e)}</span></li>`).join("")}</ul>` : ""}
    </section>`;
  };

  const wagoResultPanel = (res: WagoImportResult): string => `
    <section class="result-panel" role="status" aria-label="Import saved">
      <div class="result-panel-head">
        <span class="result-panel-icon is-ok">${icon("check-circle", 18)}</span>
        <div class="result-panel-title">
          <b>Import saved</b>
          <span class="result-panel-sub mono">${escapeHtml(res.name)} · ${formatBytes(res.bytes)} · ${escapeHtml(res.path)}</span>
        </div>
        <button class="icon-btn" data-wago-dismiss aria-label="Dismiss result">${icon("x", 15)}</button>
      </div>
      <p class="result-hint">${escapeHtml(res.applied_hint)}</p>
    </section>`;

  const infoPanelHtml = (): string => {
    if (!info && !infoLoading && !infoErr) return "";
    let head: string;
    let body: string;
    let installable = false;
    let installId = "";
    if (infoLoading) {
      head = `<b>Looking up ${escapeHtml(infoArg)}…</b>`;
      body = `<div class="list-loading"><span class="spinner"></span><span>Fetching details…</span></div>`;
    } else if (infoErr) {
      head = `<b>${escapeHtml(infoArg)}</b>`;
      body = `<div class="error-box" role="alert"><span class="error-box-head">${icon("alert", 15)}<span>${escapeHtml(infoErr)}</span></span></div>`;
    } else if (info && info.matches && info.matches.length > 0) {
      head = `<b>“${escapeHtml(infoArg)}” is ambiguous</b>`;
      body = `
        <div class="match-list">
          <p class="result-hint">More than one addon matches. Pick one to see its details:</p>
          ${info.matches
            .map(
              (m, i) => `
            <div class="match-row">
              <div class="match-info">
                <span class="match-name">${escapeHtml(m.name)}</span>
                <span class="match-meta mono">${escapeHtml(m.id)}</span>
              </div>
              ${providerChip(m.provider)}
              <button class="btn-primary btn-sm" data-info-match="${i}">${icon("info", 14)}<span>Details</span></button>
            </div>`,
            )
            .join("")}
        </div>`;
    } else if (info) {
      head = `<b>${escapeHtml(info.name || infoArg)}</b>${info.provider ? providerChip(info.provider) : ""}`;
      body = infoDetails(info);
      installId = info.id || info.name || infoArg;
      installable = Boolean(installId);
    } else {
      return "";
    }
    return `
    <section class="info-panel" role="region" aria-label="Addon details">
      <div class="info-head">
        <span class="info-icon">${icon("info", 18)}</span>
        <div class="info-title">${head}</div>
        <button class="icon-btn" data-info-close aria-label="Close details">${icon("x", 15)}</button>
      </div>
      ${body}
      ${installable ? `
      <div class="info-actions">
        <button class="btn-primary btn-sm" data-info-install ${installing ? "disabled" : ""}>
          ${installing && busySource === installId ? `<span class="spinner"></span>` : icon("download", 14)}<span>Install</span>
        </button>
        <button class="btn-secondary btn-sm" data-info-close>${icon("x", 14)}<span>Close</span></button>
      </div>` : ""}
    </section>`;
  };

  const infoDetails = (r: InfoResult): string => {
    const homepage = (r.homepage ?? "").trim();
    const notes = (r.release_notes ?? "").trim();
    const kv = (label: string, value: string, mono = false): string => `
      <div class="info-kv">
        <dt>${label}</dt>
        <dd class="${mono ? "mono tnum" : ""}">${value || "n/a"}</dd>
      </div>`;
    return `
      <dl class="info-grid">
        ${kv("Author", escapeHtml(r.author || ""))}
        ${kv("Provider", providerChip(r.provider))}
        ${kv("Latest version", escapeHtml(r.latest_version || ""), true)}
        ${kv("Game version", escapeHtml(r.game_version || ""))}
        ${kv("Updated", escapeHtml(formatDate(r.updated_at || "")))}
        ${kv("ID", escapeHtml(r.id || ""), true)}
        ${kv("Homepage", homepage ? `<a href="${escapeAttr(homepage)}" target="_blank" rel="noreferrer">${escapeHtml(homepage)}</a>` : "")}
      </dl>
      <p class="info-summary">${escapeHtml(r.summary || "No description available.")}</p>
      ${notes ? `<div class="info-notes"><span class="info-notes-label">Release notes</span>${notes.split("\n").map((l) => (l ? `<p>${escapeHtml(l)}</p>` : "<p>&nbsp;</p>")).join("")}</div>` : ""}`;
  };

  const wagoSectionHtml = (): string => `
    <section class="wago-card card" aria-label="Wago imports">
      <div class="wago-head">
        <span class="wago-icon">${icon("save", 18)}</span>
        <div class="wago-head-text">
          <h3 class="wago-title">Wago imports</h3>
          <p class="wago-sub">Save a WeakAuras or Plater import string from Wago into your SavedVariables, then import it in-game.</p>
        </div>
      </div>
      <div class="wago-form">
        <input class="text-input wago-input" data-wago type="text" placeholder="Wago id — e.g. aNVtPkzRn3, or pick one from the search results above"
          spellcheck="false" autocomplete="off" value="${escapeAttr(wagoArg)}" aria-label="Wago import id" />
        <button class="btn-secondary" data-wago-save ${saving ? "disabled" : ""}>
          ${saving && savingId === (wagoArg.trim() || null) ? `<span class="spinner"></span>` : icon("save", 15)}<span>Save import</span>
        </button>
      </div>
      ${wagoResult ? `<div class="wago-result">${icon("check-circle", 15)}<span><b>${escapeHtml(wagoResult.name)}</b> · ${formatBytes(wagoResult.bytes)} · <span class="mono">${escapeHtml(wagoResult.path)}</span></span></div>` : ""}
    </section>`;

  const bind = (
    root: HTMLElement,
    ctx: { shown: CatalogEntry[]; featured: CuratedAddon[] },
  ): void => {
    // Every element below is rendered by this view's own template.
    const searchInput = root.querySelector<HTMLInputElement>("[data-search]")!;
    const sourceInput = root.querySelector<HTMLInputElement>("[data-source]")!;
    const fileInput = root.querySelector<HTMLInputElement>(".file-input")!;
    const bar = root.querySelector<HTMLElement>("[data-bar]")!;
    const wagoInput = root.querySelector<HTMLInputElement>("[data-wago]")!;

    searchInput?.addEventListener("input", () => {
      query = searchInput.value;
      scheduleSearch();
      rerender();
    });
    searchInput?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        window.clearTimeout(debounceTimer);
        void doSearch(searchInput.value);
      } else if (e.key === "Escape" && query) {
        e.preventDefault();
        clearSearch();
      }
    });
    root.querySelector("[data-search-clear]")?.addEventListener("click", clearSearch);

    sourceInput?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        submitSource();
      } else if (e.key === "Escape" && sourceInput.value) {
        sourceInput.value = "";
      }
    });
    root.querySelector("[data-install]")?.addEventListener("click", submitSource);
    root.querySelector("[data-browse]")?.addEventListener("click", async () => {
      if (installing) return;
      const nativePath = await pickZipPath();
      if (nativePath) void installZipPath(nativePath);
      else fileInput?.click();
    });
    fileInput?.addEventListener("change", () => {
      const f = fileInput.files?.[0];
      if (f) void installZipFile(f);
      fileInput.value = "";
    });
    bar?.addEventListener("dragover", (e) => {
      e.preventDefault();
      bar.classList.add("dragover");
    });
    bar?.addEventListener("dragleave", () => bar.classList.remove("dragover"));
    bar?.addEventListener("drop", (e) => {
      e.preventDefault();
      bar.classList.remove("dragover");
      const f = e.dataTransfer?.files?.[0];
      if (f) void installZipFile(f);
    });

    root.querySelectorAll<HTMLElement>("[data-filter]").forEach((chip) => {
      chip.addEventListener("click", () => {
        provider = (chip.dataset.filter as "all" | Provider) ?? "all";
        rerender();
      });
    });
    root.querySelector("[data-compat-toggle]")?.addEventListener("click", () => {
      compatOnly = !compatOnly;
      rerender();
    });
    root.querySelectorAll<HTMLElement>("[data-install-row]").forEach((btn) => {
      const entry = ctx.shown[Number(btn.dataset.installRow)];
      if (!entry) return;
      btn.addEventListener("click", () => {
        pendingFocus = `install:${btn.dataset.installRow}`;
        void installSource(entry.id, entry.name);
      });
    });
    root.querySelectorAll<HTMLElement>("[data-info-row]").forEach((btn) => {
      const entry = ctx.shown[Number(btn.dataset.infoRow)];
      if (!entry) return;
      btn.addEventListener("click", () => {
        pendingFocus = `info:${btn.dataset.infoRow}`;
        void fetchInfo(entry.id || entry.name);
      });
    });
    root.querySelectorAll<HTMLElement>("[data-save-import]").forEach((btn) => {
      const entry = ctx.shown[Number(btn.dataset.saveImport)];
      if (!entry) return;
      btn.addEventListener("click", () => {
        pendingFocus = `save:${btn.dataset.saveImport}`;
        void saveImport(entry.id);
      });
    });
    root.querySelectorAll<HTMLElement>("[data-curated-install]").forEach((btn) => {
      const a = ctx.featured[Number(btn.dataset.curatedInstall)];
      if (!a) return;
      btn.addEventListener("click", () => {
        pendingFocus = "curated-install";
        void installSource(a.source, a.name);
      });
    });
    root.querySelectorAll<HTMLElement>("[data-info-match]").forEach((btn) => {
      const m = info?.matches?.[Number(btn.dataset.infoMatch)];
      if (!m) return;
      btn.addEventListener("click", () => {
        pendingFocus = `match:${btn.dataset.infoMatch}`;
        const arg = m.id || m.name || "";
        if (!arg) {
          toast({ type: "error", title: "Cannot look up match", message: "This match has no id or name to look up." });
          return;
        }
        void fetchInfo(arg);
      });
    });
    root.querySelector("[data-info-close]")?.addEventListener("click", () => {
      pendingFocus = null;
      closeInfo();
    });
    root.querySelector("[data-info-install]")?.addEventListener("click", () => {
      if (!info) return;
      pendingFocus = "info-install";
      void installSource(info.id || info.name || infoArg, info.name || infoArg);
    });
    root.querySelector("[data-result-dismiss]")?.addEventListener("click", () => {
      installResult = null;
      rerender();
    });
    root.querySelector("[data-wago-dismiss]")?.addEventListener("click", () => {
      wagoResult = null;
      rerender();
    });
    wagoInput?.addEventListener("input", () => {
      wagoArg = wagoInput.value;
    });
    wagoInput?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        void saveImport(wagoInput.value);
      }
    });
    root.querySelector("[data-wago-save]")?.addEventListener("click", () => {
      void saveImport(wagoInput?.value ?? "");
    });

    function submitSource(): void {
      const src = sourceInput?.value.trim() ?? "";
      if (!src || installing) return;
      barSource = src;
      sourceInput.value = "";
      void installSource(src, displayNameFromSource(src));
    }
  };

  render();
  void loadCurated();
  void loadSources();
}

function providerChip(provider: string, tooltip?: string): string {
  const label = PROVIDER_LABEL[provider as Provider] ?? provider;
  return `<span class="provider-chip" title="${escapeAttr(tooltip ?? label)}">${escapeHtml(label)}</span>`;
}

function displayNameFromSource(src: string): string {
  const seg = src.split(/[\\/?#]/).filter(Boolean).pop() ?? src;
  return seg.replace(/\.zip$/i, "").replace(/[-_](main|master)$/i, "");
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return (
    d.toLocaleDateString([], { year: "numeric", month: "short", day: "numeric" }) +
    " " +
    d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  );
}

function emptyState(glyph: IconName, title: string, body: string): string {
  return `
    <div class="empty-state">
      <span class="empty-glyph">${icon(glyph, 30)}</span>
      <h2 class="empty-title">${escapeHtml(title)}</h2>
      <p class="empty-body">${escapeHtml(body)}</p>
    </div>`;
}

// Wails v2 exposes the native file dialog on the JS runtime; returns the
// chosen path or null when unavailable or cancelled.
function pickZipPath(): Promise<string | null> {
  const rt = (window as unknown as {
    runtime?: { OpenFileDialog?: (opts: unknown) => Promise<string | null> };
  }).runtime;
  if (!rt?.OpenFileDialog) return Promise.resolve(null);
  return rt
    .OpenFileDialog({
      filters: [{ displayName: "ZIP archives", pattern: "*.zip" }],
    })
    .then((p) => p || null)
    .catch(() => null);
}

// Resolve the installable path for a picked/dropped File. Wails v2 patches
// File objects with a real `path` property in some versions; browsers never
// do. Mock mode accepts the bare name so browser demos keep working.
function zipPathOf(file: File): string {
  const wailsFile = file as File & { path?: string };
  return wailsFile.path || (mockActive ? file.name : "");
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
