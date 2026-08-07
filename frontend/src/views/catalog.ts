// Catalog: search addon providers (GitHub / CurseForge / WowInterface /
// Tukui), filter by provider chip, and install from the catalog or a pasted
// URL / owner-repo. Installs always confirm first; results offer a hook to
// jump to the scan view.

import type { AppState, Actions } from "../app";
import type {
  CatalogEntry,
  SearchCatalogResult,
  InstallSourceResult,
  WagoImportResult,
  CuratedAddon,
  CuratedResult,
  Provider,
  InfoResult,
  ProviderInfo,
} from "../types";
import { formatBytes } from "../types";
import { icon, type IconName } from "../icons";
import { service } from "../api";
import { toast } from "../components/toast";
import { confirmDialog } from "../components/dialog";

// `matches` on InfoResult is the candidate list for an ambiguous bare-name
// lookup (the CLI's `wowfix info` search hits). Only provider/name/id are
// guaranteed; the rest are optional so the UI degrades gracefully.
interface InfoMatch {
  provider?: string;
  id?: string;
  name?: string;
}

const PROVIDER_LABEL: Record<string, string> = {
  github: "GitHub",
  curseforge: "CurseForge",
  wowinterface: "WowInterface",
  tukui: "Tukui",
  wago: "Wago",
};

const FILTERS: { value: "all" | Provider; label: string }[] = [
  { value: "all", label: "All" },
  { value: "github", label: "GitHub" },
  { value: "curseforge", label: "CurseForge" },
  { value: "wowinterface", label: "WowInterface" },
  { value: "tukui", label: "Tukui" },
  { value: "wago", label: "Wago" },
];

const DEBOUNCE_MS = 350;

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
  tbc: "tbc",
  wrath: "wrath",
  cata: "cata",
  classic: "vanilla",
  hardcore: "vanilla",
  sod: "vanilla",
  retail: "retail",
};

function normalizeFamily(s: string): string {
  const k = s.trim().toLowerCase();
  return FAMILY_NORMAL[k] ?? k;
}

function normalizeGameVersion(s: string): string {
  const k = s.trim().toLowerCase();
  return GAME_VERSION_NORMAL[k] ?? k;
}

export function mountCatalog(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let query = "";
  let provider: "all" | Provider = "all";
  let result: SearchCatalogResult | null = null;
  let searching = false;
  let installing = false;
  let saving = false;
  let installResult: InstallSourceResult | null = null;
  let wagoResult: WagoImportResult | null = null;
  let timer = 0;
  let curated: CuratedResult | null = null;
  let compatOnly = false;
  // --- addon info + sources (CLI `info` / `sources` parity) ----------------
  let info: InfoResult | null = null;
  let infoLoading = false;
  let infoErr: string | null = null;
  let infoArg = "";
  let sourcesOpen = false;
  let sources: ProviderInfo[] | null = null;
  let sourcesLoading = false;
  let sourcesErr: string | null = null;

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

  // --- focus preservation ------------------------------------------------
  // Re-renders rebuild the view DOM, dropping keyboard focus to <body>.
  // Capture the focused control before a re-render and restore focus to its
  // replacement after. pendingFocus survives async flows (install / save)
  // whose final re-render happens after the backend call, while the control
  // is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (el: HTMLElement): string | null => {
    if (el.closest(".search-clear") || el.closest("[data-search]")) return "search";
    if (el.closest("[data-source]")) return "source";
    const filter = el.closest<HTMLElement>("[data-filter]");
    if (filter) return `filter:${filter.dataset.filter}`;
    if (el.closest("[data-compat-toggle]")) return "compat";
    const install = el.closest<HTMLElement>("[data-install-row]");
    if (install) return `install:${install.dataset.installRow}`;
    const save = el.closest<HTMLElement>("[data-save-import]");
    if (save) return `save:${save.dataset.saveImport}`;
    const curated = el.closest<HTMLElement>("[data-curated-install]");
    if (curated) return `curated:${curated.dataset.curatedInstall}`;
    const infoBtn = el.closest<HTMLElement>("[data-info-row]");
    if (infoBtn) return `info:${infoBtn.dataset.infoRow}`;
    const matchBtn = el.closest<HTMLElement>("[data-info-match]");
    if (matchBtn) return `info-match:${matchBtn.dataset.infoMatch}`;
    if (el.closest("[data-info-close]")) return "info-close";
    if (el.closest("[data-sources-toggle]")) return "sources";
    if (el.closest("[data-rescan]")) return "rescan";
    if (el.closest("[data-install]")) return "install";
    return null;
  };

  const restoreFocus = (key: string | null): boolean => {
    if (!key) return false;
    let target: HTMLElement | null = null;
    if (key === "search") {
      const input = el.querySelector<HTMLInputElement>("[data-search]");
      if (input) {
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
        return true;
      }
      return false;
    }
    if (key === "source") {
      target = el.querySelector<HTMLInputElement>("[data-source]");
    } else if (key === "install") {
      target = el.querySelector<HTMLElement>("[data-install]");
    } else if (key === "sources") {
      target = el.querySelector<HTMLElement>("[data-sources-toggle]");
    } else {
      const [kind, id] = key.split(":");
      if (kind === "filter") target = el.querySelector<HTMLElement>(`[data-filter="${id}"]`);
      else if (kind === "compat") target = el.querySelector<HTMLElement>("[data-compat-toggle]");
      else if (kind === "install") target = el.querySelector<HTMLElement>(`[data-install-row="${id}"]`);
      else if (kind === "save") target = el.querySelector<HTMLElement>(`[data-save-import="${id}"]`);
      else if (kind === "curated") target = el.querySelector<HTMLElement>(`[data-curated-install="${id}"]`);
      else if (kind === "info") target = el.querySelector<HTMLElement>(`[data-info-row="${id}"]`);
      else if (kind === "info-match") target = el.querySelector<HTMLElement>(`[data-info-match="${id}"]`);
      else if (kind === "info-close") target = el.querySelector<HTMLElement>("[data-info-close]");
      else if (kind === "rescan") target = el.querySelector<HTMLElement>("[data-rescan]");
    }
    if (!target) return false;
    if (target.hasAttribute("disabled")) return false;
    target.focus();
    return true;
  };

  // Render with focus preservation: capture what had focus, render, restore.
  // In-flight work keeps pendingFocus alive so the final render can land it.
  const rerender = (): void => {
    const active = document.activeElement;
    const key = pendingFocus ?? (active instanceof HTMLElement ? focusKeyOf(active) : null);
    render();
    if (!key) return;
    if (restoreFocus(key)) pendingFocus = null;
    else if (!searching && !installing && !saving && !infoLoading && !sourcesLoading) pendingFocus = null;
  };

  const scheduleSearch = (): void => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => void doSearch(query), DEBOUNCE_MS);
  };

  const installSource = async (source: string, label: string): Promise<void> => {
    if (installing) return;
    const confirmed = await confirmDialog({
      title: `Install ${label}?`,
      message: `Install “${label}” from this source into your addons folder?`,
      details: [source],
      confirmLabel: "Install",
    });
    if (!confirmed) return;
    installing = true;
    rerender();
    try {
      const res = await service.InstallSource(source, true);
      installResult = res;
      if (res.errors.length > 0) {
        toast({
          type: "error",
          title: "Install completed with errors",
          message: `${res.errors.length} error${res.errors.length === 1 ? "" : "s"} — see the result panel`,
        });
      } else if (res.installed.length > 0) {
        toast({
          type: "ok",
          title: "Addon installed",
          message: `${res.installed.join(", ")} installed · ${res.replaced.length} replaced · ${res.skipped.length} skipped`,
        });
      } else if (res.replaced.length > 0) {
        toast({
          type: "ok",
          title: "Addon replaced",
          message: `Replaced ${res.replaced.join(", ")} after backup`,
        });
      } else if (res.skipped.length > 0) {
        toast({
          type: "info",
          title: "Already installed",
          message: "The addon exists and replace is off — nothing was changed.",
        });
      }
    } catch (err) {
      toast({ type: "error", title: "Install failed", message: errText(err) });
    } finally {
      installing = false;
      rerender();
    }
  };

  const saveImport = async (entry: CatalogEntry): Promise<void> => {
    if (saving) return;
    saving = true;
    rerender();
    try {
      const res = await service.SaveWagoImport(entry.id);
      wagoResult = res;
      toast({ type: "ok", title: "Import saved", message: res.applied_hint });
    } catch (err) {
      toast({ type: "error", title: "Save failed", message: errText(err) });
    } finally {
      saving = false;
      rerender();
    }
  };

  const loadCurated = async (): Promise<void> => {
    try {
      curated = await service.Curated();
    } catch (err) {
      curated = null;
      toast({
        type: "error",
        title: "Could not load recommendations",
        message: errText(err),
      });
    }
    rerender();
  };

  const installCurated = async (addon: CuratedAddon): Promise<void> => {
    if (installing) return;
    const confirmed = await confirmDialog({
      title: `Install ${addon.name}?`,
      message: `Installs ${addon.source} via the catalog.`,
      confirmLabel: "Install",
    });
    if (!confirmed) return;
    installing = true;
    rerender();
    try {
      const res = await service.InstallSource(addon.source, true);
      installResult = res;
      if (res.errors.length > 0) {
        toast({
          type: "error",
          title: "Install completed with errors",
          message: `${res.errors.length} error${res.errors.length === 1 ? "" : "s"} — see the result panel`,
        });
      } else if (res.installed.length > 0) {
        toast({
          type: "ok",
          title: "Addon installed",
          message: `${res.installed.join(", ")} installed · ${res.replaced.length} replaced · ${res.skipped.length} skipped`,
        });
      } else if (res.replaced.length > 0) {
        toast({
          type: "ok",
          title: "Addon replaced",
          message: `Replaced ${res.replaced.join(", ")} after backup`,
        });
      } else if (res.skipped.length > 0) {
        toast({
          type: "info",
          title: "Already installed",
          message: "The addon exists and replace is off — nothing was changed.",
        });
      }
    } catch (err) {
      toast({ type: "error", title: "Install failed", message: errText(err) });
    } finally {
      installing = false;
      // Re-fetch so the row flips to Installed.
      await loadCurated();
    }
  };

  // CLI `wowfix info` parity: resolve a hit's provider-scoped id (GitHub
  // ids are "owner/repo"), or a bare name when no id is carried. An
  // ambiguous bare name returns `matches` — the panel renders them and the
  // user re-runs the lookup by clicking one.
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

  // CLI `wowfix sources` parity: lazy-load the provider list on first open.
  const toggleSources = (): void => {
    sourcesOpen = !sourcesOpen;
    if (sourcesOpen && sources === null && !sourcesLoading) {
      sourcesLoading = true;
      sourcesErr = null;
      rerender();
      service
        .Sources()
        .then((res) => {
          sources = res;
        })
        .catch((err) => {
          sourcesErr = errText(err);
          toast({ type: "error", title: "Could not load sources", message: errText(err) });
        })
        .finally(() => {
          sourcesLoading = false;
          rerender();
        });
    } else {
      rerender();
    }
  };

  const render = (): void => {
    if (!app.state.has_install) {
      el.innerHTML = emptyCard(
        "folder",
        "No WoW install configured",
        "Set up your World of Warcraft path before installing addons.",
        `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
      );
      el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
      return;
    }

    const results = result?.results ?? [];
    const errors = result?.errors ?? [];
    const family =
      curated?.family ??
      app.profiles.find((p) => p.id === app.state.profile_id)?.family ??
      "";
    const familyNorm = normalizeFamily(family);
    const compatOk = (r: CatalogEntry): boolean =>
      !r.game_version || normalizeGameVersion(r.game_version) === familyNorm;
    const filtered =
      provider === "all" ? results : results.filter((r) => r.provider === provider);
    const shown = compatOnly ? filtered.filter(compatOk) : filtered;
    const compatCount = results.filter(compatOk).length;

    const curatedHtml =
      curated && curated.addons.length > 0
        ? `
        <section class="curated" aria-label="Recommended addons">
          <div class="curated-head spotlight">
            <h2 class="curated-title spotlight-title">${icon("shield", 18)}<span>Recommended for ${escapeHtml(curated.label || "your server")}</span></h2>
            <p class="curated-context spotlight-sub">Curated addon sets for private servers — installed from verified sources.</p>
          </div>
          <div class="curated-rows">
            ${curated.addons
              .map(
                (a, i) => `
              <div class="curated-row">
                <div class="curated-info">
                  <span class="curated-name">${escapeHtml(a.name)}</span>
                  <span class="curated-summary">${escapeHtml(a.summary)}</span>
                  <span class="curated-source mono">${escapeHtml(a.source)}</span>
                </div>
                <div class="curated-action">
                  ${
                    a.installed
                      ? `<span class="tag tag-ok">Installed</span>${a.installed_version ? `<span class="curated-version mono muted">v${escapeHtml(a.installed_version)}</span>` : ""}`
                      : `<button class="btn btn-primary btn-sm" data-curated-install="${i}" ${installing ? "disabled" : ""}>${icon("package", 14)}<span>Install</span></button>`
                  }
                </div>
              </div>`,
              )
              .join("")}
          </div>
        </section>`
        : "";

    el.innerHTML = `
      <div class="catalog">
        <div class="catalog-toolbar">
          <div class="search-box search-box-lg">
            <span class="search-icon">${icon("search", 16)}</span>
            <input class="search-input" type="text" placeholder="Search addons by name, author or summary…" spellcheck="false"
              value="${escapeAttr(query)}" aria-label="Search addon catalog" data-search />
            ${
              query
                ? `<button class="search-clear" aria-label="Clear search">${icon("x", 14)}</button>`
                : ""
            }
          </div>
          <div class="install-bar">
            <span class="install-bar-icon">${icon("download", 16)}</span>
            <input class="install-bar-input" type="text" placeholder="Install from URL or owner/repo…" spellcheck="false"
              value="" aria-label="Install from URL or owner/repo" data-source />
            <button class="btn btn-primary" data-install ${installing ? "disabled" : ""}>
              ${installing ? `<span class="spinner"></span>` : icon("package", 15)}
              <span>${installing ? "Installing…" : "Install"}</span>
            </button>
          </div>
        </div>

        ${curatedHtml}

        ${
          result
            ? `<div class="catalog-chips" role="group" aria-label="Filter results">
                ${FILTERS.map(
                  (f) => `
                  <button class="chip-btn${provider === f.value ? " active" : ""}" data-filter="${f.value}">
                    ${f.label}
                    ${f.value === "all" ? `<span class="chip-count">${results.length}</span>` : `<span class="chip-count">${results.filter((r) => r.provider === f.value).length}</span>`}
                  </button>`,
                ).join("")}
                ${
                  familyNorm
                    ? `<button class="chip-btn${compatOnly ? " active" : ""}" data-compat-toggle aria-pressed="${compatOnly}" title="Hide results built for other game versions">
                        ${icon("shield", 13)}<span>Compatible with ${escapeHtml(familyNorm)}${compatOnly ? `<span class="chip-count">(${compatCount})</span>` : ""}</span>
                      </button>`
                    : ""
                }
              </div>`
            : ""
        }

        ${
          errors.length
            ? `<div class="error-box" role="alert">
                <span class="error-box-head">${icon("alert", 15)}<span>${errors.length} problem${errors.length === 1 ? "" : "s"} while searching</span></span>
                <ul>${errors.map((e) => `<li>${escapeHtml(e)}</li>`).join("")}</ul>
              </div>`
            : ""
        }

        ${
          installResult
            ? `<div class="catalog-result" role="status">
                <div class="result-summary">
                  <span class="result-summary-icon tile-ok">${icon("check-circle", 16)}</span>
                  <span class="result-summary-text">
                    <b>${installResult.installed.length} installed</b>
                    ${installResult.replaced.length ? ` · ${installResult.replaced.length} replaced` : ""}
                    ${installResult.skipped.length ? ` · ${installResult.skipped.length} skipped` : ""}
                    ${installResult.errors.length ? ` · ${installResult.errors.length} errors` : ""}
                  </span>
                </div>
                ${
                  installResult.errors.length
                    ? `<ul class="result-errors">${installResult.errors
                        .map((e) => `<li>${icon("x-circle", 14)}<span>${escapeHtml(e)}</span></li>`)
                        .join("")}</ul>`
                    : ""
                }
                <div class="result-actions">
                  <button class="btn btn-primary" data-rescan>${icon("refresh", 16)}<span>Scan addons now</span></button>
                  <span class="result-hint">New addons appear in the scan list.</span>
                </div>
              </div>`
            : ""
        }

        ${
          wagoResult
            ? `<div class="catalog-result" role="status">
                <div class="result-summary">
                  <span class="result-summary-icon tile-ok">${icon("check-circle", 16)}</span>
                  <span class="result-summary-text">
                    <b>Import saved</b>
                    <span class="muted">${escapeHtml(wagoResult.name)} · ${formatBytes(wagoResult.bytes)}</span>
                  </span>
                </div>
                <div class="result-actions">
                  <span class="result-hint mono">${escapeHtml(wagoResult.path)}</span>
                  <span class="result-hint">Import it in-game via WeakAuras → Import.</span>
                </div>
              </div>`
            : ""
        }

        ${
          !result
            ? emptyCard(
                "search",
                "Search the addon catalog",
                "Find addons across GitHub, CurseForge, WowInterface and Tukui — or paste a URL / owner-repo to install directly.",
                "",
              )
            : searching
              ? `<div class="list-loading"><span class="spinner spinner-lg"></span><span>Searching…</span></div>`
              : shown.length === 0
                ? emptyCard(
                    "search",
                    compatOnly && filtered.length > 0
                      ? `No addons compatible with ${escapeHtml(familyNorm)}`
                      : query
                        ? `No addons found for “${escapeHtml(query)}”`
                        : "No addons found",
                    compatOnly && filtered.length > 0
                      ? "Try disabling the compatibility filter."
                      : "Try a different query or provider filter.",
                    "",
                  )
                : `<div class="catalog-rows">${shown.map(renderRow).join("")}</div>`
        }

        ${infoPanelHtml()}
        ${sourcesSectionHtml()}
      </div>`;

    const searchInput = el.querySelector<HTMLInputElement>("[data-search]")!;
    const sourceInput = el.querySelector<HTMLInputElement>("[data-source]")!;

    searchInput.addEventListener("input", () => {
      query = searchInput.value;
      scheduleSearch();
      rerender();
    });
    searchInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        window.clearTimeout(timer);
        void doSearch(searchInput.value);
      }
      if (e.key === "Escape" && query) {
        query = "";
        window.clearTimeout(timer);
        result = null;
        rerender();
      }
    });
    el.querySelector(".search-clear")?.addEventListener("click", () => {
      query = "";
      window.clearTimeout(timer);
      result = null;
      rerender();
    });
    sourceInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        submitSource();
      }
      if (e.key === "Escape" && sourceInput.value) {
        sourceInput.value = "";
      }
    });
    el.querySelector("[data-install]")?.addEventListener("click", submitSource);
    el.querySelector("[data-rescan]")?.addEventListener("click", () => {
      void actions.scan().then(() => actions.go("scan"));
    });
    el.querySelectorAll<HTMLElement>("[data-filter]").forEach((chip) => {
      chip.addEventListener("click", () => {
        provider = (chip.dataset.filter as "all" | Provider) ?? "all";
        rerender();
      });
    });
    el.querySelectorAll<HTMLElement>("[data-install-row]").forEach((btn) => {
      const entry = filtered[Number(btn.dataset.installRow)];
      btn.addEventListener("click", () => {
        pendingFocus = `install:${btn.dataset.installRow}`;
        void installSource(entry.id, entry.name);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-save-import]").forEach((btn) => {
      const entry = filtered[Number(btn.dataset.saveImport)];
      btn.addEventListener("click", () => {
        pendingFocus = `save:${btn.dataset.saveImport}`;
        void saveImport(entry);
      });
    });
    el.querySelector("[data-compat-toggle]")?.addEventListener("click", () => {
      compatOnly = !compatOnly;
      rerender();
    });
    el.querySelectorAll<HTMLElement>("[data-curated-install]").forEach((btn) => {
      const addon = curated?.addons[Number(btn.dataset.curatedInstall)];
      if (!addon) return;
      btn.addEventListener("click", () => {
        pendingFocus = `curated:${btn.dataset.curatedInstall}`;
        void installCurated(addon);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-info-row]").forEach((btn) => {
      const entry = filtered[Number(btn.dataset.infoRow)];
      if (!entry) return;
      btn.addEventListener("click", () => {
        pendingFocus = `info:${btn.dataset.infoRow}`;
        void fetchInfo(entry.id || entry.name);
      });
    });
    el.querySelectorAll<HTMLElement>("[data-info-match]").forEach((btn) => {
      const m = infoMatchesOf(info)[Number(btn.dataset.infoMatch)];
      if (!m) return;
      btn.addEventListener("click", () => {
        pendingFocus = `info-match:${btn.dataset.infoMatch}`;
        const arg = m.id || m.name || "";
        if (!arg) {
          toast({ type: "error", title: "Cannot look up match", message: "This match has no id or name to look up." });
          return;
        }
        void fetchInfo(arg);
      });
    });
    el.querySelector("[data-info-close]")?.addEventListener("click", () => {
      pendingFocus = null;
      closeInfo();
    });
    el.querySelector("[data-sources-toggle]")?.addEventListener("click", () => {
      pendingFocus = "sources";
      toggleSources();
    });
    el.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));

    function submitSource(): void {
      const src = sourceInput.value.trim();
      if (!src) return;
      sourceInput.value = "";
      void installSource(src, displayNameFromSource(src));
    }
  };

  const renderRow = (r: CatalogEntry, i: number): string => `
    <div class="catalog-row">
      <div class="catalog-info">
        <div class="catalog-name-line">
          <span class="catalog-name">${escapeHtml(r.name)}</span>
          ${r.game_version ? `<span class="game-badge">${escapeHtml(r.game_version)}</span>` : ""}
        </div>
        <span class="catalog-author">${escapeHtml(r.author)}</span>
        <span class="catalog-summary">${escapeHtml(r.summary)}</span>
      </div>
      <div class="catalog-meta">
        <span class="catalog-ver mono">v${escapeHtml(r.latest_version || "—")}</span>
        ${providerChip(r.provider)}
      </div>
      <div class="catalog-action">
        <button class="btn btn-outline btn-sm" data-info-row="${i}" ${infoLoading ? "disabled" : ""}
          title="Show catalog details for ${escapeAttr(r.name)}">
          ${icon("info", 14)}<span>Info</span>
        </button>
        ${
          r.provider === "wago"
            ? `<button class="btn btn-outline btn-sm" data-save-import="${i}" ${saving ? "disabled" : ""} title="Save the import string for in-game import">
                ${saving ? `<span class="spinner"></span>` : icon("download", 14)}<span>Save import</span>
              </button>`
            : `<button class="btn btn-primary btn-sm" data-install-row="${i}" ${installing ? "disabled" : ""}>
                ${icon("package", 14)}<span>Install</span>
              </button>`
        }
      </div>
    </div>`;

  const infoMatchesOf = (r: InfoResult | null): InfoMatch[] =>
    r && Array.isArray(r.matches) ? (r.matches as unknown as InfoMatch[]) : [];

  const infoKv = (label: string, valueHtml: string, mono = false): string => `
    <div class="issue-item">
      <span class="issue-sugg" style="flex:none; width:120px">${label}</span>
      <span class="issue-msg${mono ? " mono" : ""}">${valueHtml}</span>
    </div>`;

  const renderInfoDetails = (r: InfoResult): string => {
    const homepage = (r.homepage ?? "").trim();
    const notes = (r.release_notes ?? "").trim();
    return `
      <div class="issue-list">
        ${infoKv("Provider", r.provider ? providerChip(r.provider) : "—")}
        ${infoKv("ID", escapeHtml(r.id || "—"), true)}
        ${infoKv("Author", escapeHtml(r.author || "—"))}
        ${infoKv("Latest version", escapeHtml(r.latest_version || "—"), true)}
        ${infoKv("Game version", escapeHtml(r.game_version || "—"))}
        ${infoKv("Updated", escapeHtml(formatDate(r.updated_at || "") || "—"))}
        ${
          homepage
            ? `<div class="issue-item">
                <span class="issue-sugg" style="flex:none; width:120px">Homepage</span>
                <span class="issue-msg"><a href="${escapeAttr(homepage)}" target="_blank" rel="noreferrer">${escapeHtml(homepage)}</a></span>
              </div>`
            : infoKv("Homepage", "—")
        }
        ${infoKv("Summary", escapeHtml(r.summary || "—"))}
        ${
          notes
            ? `<div class="issue-item">
                <span class="issue-sugg" style="flex:none; width:120px">Release notes</span>
                <span class="issue-msg">${notes
                  .split("\n")
                  .map((l) => (l ? `<div>${escapeHtml(l)}</div>` : "<div>&nbsp;</div>"))
                  .join("")}</span>
              </div>`
            : ""
        }
      </div>`;
  };

  const renderInfoMatchRow = (m: InfoMatch, i: number): string => `
    <div class="catalog-row">
      <div class="catalog-info">
        <div class="catalog-name-line">
          <span class="catalog-name">${escapeHtml(m.name || "—")}</span>
        </div>
        <span class="catalog-summary mono">${escapeHtml(m.id || "")}</span>
      </div>
      <div class="catalog-meta">${m.provider ? providerChip(m.provider) : ""}</div>
      <div class="catalog-action">
        <button class="btn btn-primary btn-sm" data-info-match="${i}" ${infoLoading ? "disabled" : ""}>
          ${icon("info", 14)}<span>Details</span>
        </button>
      </div>
    </div>`;

  const infoPanelHtml = (): string => {
    if (!info && !infoLoading && !infoErr) return "";
    let head: string;
    let body: string;
    if (infoLoading) {
      head = `<b>Looking up ${escapeHtml(infoArg)}…</b>`;
      body = `<div class="list-loading"><span class="spinner"></span><span>Fetching details…</span></div>`;
    } else if (infoErr) {
      head = `<b>${escapeHtml(infoArg)}</b>`;
      body = `<div class="managed-error" role="alert">${icon("alert", 14)}<span>${escapeHtml(infoErr)}</span></div>`;
    } else if (info && infoMatchesOf(info).length > 0) {
      head = `<b>“${escapeHtml(infoArg)}” is ambiguous</b>`;
      body = `
        <div class="catalog-info-body">
          <p class="result-hint">More than one addon matches — choose one to see its details:</p>
          <div class="catalog-rows">${infoMatchesOf(info).map(renderInfoMatchRow).join("")}</div>
        </div>`;
    } else if (info) {
      head = `<b>${escapeHtml(info.name || infoArg)}</b>${info.provider ? providerChip(info.provider) : ""}`;
      body = renderInfoDetails(info);
    } else {
      return "";
    }
    return `
      <section class="catalog-result" role="region" aria-label="Addon details">
        <div class="result-summary">
          <span class="result-summary-icon tile-ok">${icon("info", 16)}</span>
          <span class="result-summary-text">${head}</span>
        </div>
        ${body}
        <div class="result-actions">
          <button class="btn btn-outline btn-sm" data-info-close>${icon("x", 14)}<span>Close</span></button>
        </div>
      </section>`;
  };

  const sourcesSectionHtml = (): string => {
    const rows = sourcesLoading
      ? `<div class="list-loading"><span class="spinner"></span><span>Loading providers…</span></div>`
      : sourcesErr
        ? `<div class="managed-error" role="alert">${icon("alert", 14)}<span>${escapeHtml(sourcesErr)}</span></div>`
        : sources === null
          ? ""
          : `<ul class="issue-list">
              ${sources
                .map(
                  (s) => `
                <li class="issue-item">
                  <span class="issue-text">
                    <span class="issue-msg">${escapeHtml(s.name)}</span>
                    <span class="issue-sugg">${escapeHtml(s.description)}</span>
                  </span>
                </li>`,
                )
                .join("")}
            </ul>`;
    return `
      <section class="catalog-sources" aria-label="Catalog sources" style="margin:0 20px 12px">
        <button class="btn btn-ghost btn-sm" data-sources-toggle aria-expanded="${sourcesOpen}" aria-controls="catalog-sources-body">
          ${icon(sourcesOpen ? "chevron-down" : "chevron-right", 14)}
          <span>Sources</span>
          ${sources ? `<span class="muted">${sources.length} provider${sources.length === 1 ? "" : "s"}</span>` : ""}
        </button>
        ${sourcesOpen ? `<div class="catalog-sources-body" id="catalog-sources-body">${rows}</div>` : ""}
      </section>`;
  };

  render();
  void loadCurated();

  return {
    refresh: () => {
      void loadCurated();
    },
  };
}

function providerChip(provider: string): string {
  const label = PROVIDER_LABEL[provider] ?? provider;
  return `<span class="provider-chip prov-${escapeAttr(provider)}" title="${escapeAttr(PROVIDER_LABEL[provider] ?? provider)}">${escapeHtml(label)}</span>`;
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

function emptyCard(glyph: IconName, title: string, sub: string, cta: string): string {
  return `<div class="empty">
    <span class="empty-icon">${icon(glyph, 28)}</span>
    <h2 class="empty-title">${title}</h2>
    <p class="empty-sub">${sub}</p>
    <div class="empty-actions">${cta}</div>
  </div>`;
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
