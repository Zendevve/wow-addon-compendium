// Scan list — the hero view. Live text filter, quick health chips,
// doctor health panel (Fix All hero CTA), per-row status + health badge
// + fix actions, expandable detail (all issues + compat table), empty
// states.

import type { AppState, Actions } from "../app";
import type { Addon, Issue } from "../types";
import { ACTION_LABELS, DESTRUCTIVE_ACTIONS, formatBytes } from "../types";
import { icon, type IconName } from "../icons";
import { confirmDialog } from "../components/dialog";

type HealthFilter = "all" | "issues" | "healthy";

const HEALTH_FILTERS: { value: HealthFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "issues", label: "With issues" },
  { value: "healthy", label: "Healthy" },
];

export function mountScan(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  const expanded = new Set<string>();
  let healthFilter: HealthFilter = "all";
  const toolbar = document.createElement("div");
  toolbar.className = "scan-toolbar";
  const list = document.createElement("div");
  list.className = "addon-list";
  el.append(toolbar, list);

  const matches = (a: Addon): boolean => {
    const q = app.filter.trim().toLowerCase();
    if (!q) return true;
    const hay = [
      a.folder_name,
      a.base_name,
      a.suggested_name,
      a.toc?.title ?? "",
      ...a.issues.map((i) => `${i.message} ${i.suggestion}`),
    ]
      .join(" ")
      .toLowerCase();
    return hay.includes(q);
  };

  const byHealth = (a: Addon): boolean =>
    healthFilter === "all"
      ? true
      : healthFilter === "issues"
        ? a.issues.length > 0
        : a.issues.length === 0;

  // Mean of the per-addon health scores (integer, 0-100).
  const avgHealth = (): number => {
    const addons = app.scan?.addons ?? [];
    if (addons.length === 0) return 100;
    const sum = addons.reduce((acc, a) => acc + a.health, 0);
    return Math.round(sum / addons.length);
  };

  // --- focus preservation ------------------------------------------------
  // Re-renders rebuild the list/toolbar DOM, which drops keyboard focus to
  // <body>. Capture the focused control before a re-render and restore focus
  // to its replacement after, so keyboard users stay anchored. pendingFocus
  // survives async action flows (fix / restore / Fix All) whose re-render
  // happens after the backend call, while the control is disabled.
  let pendingFocus: string | null = null;

  const focusKeyOf = (el: HTMLElement): string | null => {
    if (el.closest(".search-input")) return "search";
    const row = el.closest<HTMLElement>("[data-row]");
    if (row) {
      const ctrl = el.closest<HTMLElement>(
        "[data-expand], [data-fix], [data-restore]",
      );
      const kind = ctrl
        ? ctrl.hasAttribute("data-expand")
          ? "expand"
          : ctrl.hasAttribute("data-fix")
            ? "fix"
            : "restore"
        : "row";
      return `row:${row.dataset.row}:${kind}`;
    }
    const chip = el.closest<HTMLElement>("[data-health]");
    if (chip) return `chip:${chip.dataset.health}`;
    if (el.closest("[data-panel-fixall]")) return "panel-fixall";
    if (el.closest("[data-rescan]")) return "rescan";
    return null;
  };

  const restoreFocus = (key: string | null): boolean => {
    if (!key) return false;
    let target: HTMLElement | null = null;
    if (key === "search") {
      const input = toolbar.querySelector<HTMLInputElement>(".search-input");
      if (input) {
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
        return true;
      }
      return false;
    }
    const [kind, ...rest] = key.split(":");
    if (kind === "row") {
      const [, id, ctrl] = rest;
      const sel =
        ctrl === "expand"
          ? `[data-row="${id}"] [data-expand]`
          : ctrl === "fix"
            ? `[data-row="${id}"] [data-fix]`
            : ctrl === "restore"
              ? `[data-row="${id}"] [data-restore]`
              : `[data-row="${id}"]`;
      target = list.querySelector<HTMLElement>(sel);
    } else if (kind === "chip") {
      target = list.querySelector<HTMLElement>(`[data-health="${rest[0]}"]`);
    } else if (kind === "panel-fixall") {
      target = list.querySelector<HTMLElement>("[data-panel-fixall]");
    } else if (kind === "rescan") {
      target = list.querySelector<HTMLElement>("[data-rescan]");
    }
    if (!target) return false;
    target.focus();
    return true;
  };

  const renderToolbar = (): void => {
    const scan = app.scan;
    const filterActive = app.filter.length > 0;
    const shown = scan
      ? scan.addons.filter((a) => matches(a) && byHealth(a)).length
      : 0;
    toolbar.innerHTML = `
      <div class="search-box">
        <span class="search-icon">${icon("search", 16)}</span>
        <input class="search-input" type="text" placeholder="Filter addons…" spellcheck="false"
          value="${escapeAttr(app.filter)}" aria-label="Filter addons" />
        ${
          filterActive
            ? `<button class="search-clear" aria-label="Clear filter">${icon("x", 14)}</button>`
            : ""
        }
      </div>
      ${
        scan
          ? `<div class="toolbar-counts" aria-label="Scan summary">
              <span class="count-item"><span class="status-dot ok"></span><span class="count-num">${scan.stats.total}</span> addons</span>
              <span class="count-item"><span class="status-dot warn"></span><span class="count-num">${scan.stats.problems}</span> with issues</span>
              ${
                scan.stats.errors > 0
                  ? `<span class="count-item"><span class="status-dot error"></span><span class="count-num">${scan.stats.errors}</span> error${scan.stats.errors === 1 ? "" : "s"}</span>`
                  : ""
              }
              ${
                filterActive || healthFilter !== "all"
                  ? `<span class="count-item muted">showing ${shown} of ${scan.stats.total}</span>`
                  : ""
              }
            </div>`
          : ""
      }`;
    const input = toolbar.querySelector<HTMLInputElement>(".search-input")!;
    input.addEventListener("input", () => {
      app.filter = input.value;
      renderList();
      renderToolbar();
      const next = toolbar.querySelector<HTMLInputElement>(".search-input")!;
      next.focus();
      next.setSelectionRange(next.value.length, next.value.length);
    });
    input.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && app.filter) {
        app.filter = "";
        renderList();
        renderToolbar();
        toolbar.querySelector<HTMLInputElement>(".search-input")?.focus();
      }
    });
    toolbar.querySelector(".search-clear")?.addEventListener("click", () => {
      app.filter = "";
      renderList();
      renderToolbar();
      toolbar.querySelector<HTMLInputElement>(".search-input")?.focus();
    });
  };

  const renderChips = (): string => `
    <div class="scan-chips" role="group" aria-label="Filter addons by health">
      ${HEALTH_FILTERS.map((f) => {
        const count = app.scan!.addons.filter((a) =>
          f.value === "all"
            ? true
            : f.value === "issues"
              ? a.issues.length > 0
              : a.issues.length === 0,
        ).length;
        return `<button class="chip-btn${healthFilter === f.value ? " active" : ""}" data-health="${f.value}">
          ${f.label}<span class="chip-count">${count}</span>
        </button>`;
      }).join("")}
    </div>`;

  const renderPanel = (): string => {
    const scan = app.scan!;
    const avg = avgHealth();
    const band =
      avg >= 85 ? "healthy" : avg >= 60 ? "attention" : "repair";
    const bandLabel =
      band === "healthy"
        ? "Healthy"
        : band === "attention"
          ? "Needs attention"
          : "Needs repair";
    const fixable = scan.addons.filter((a) => a.fixable).length;
    const s = scan.stats;
    return `
      <div class="doctor-panel band-${band}">
        <div class="doctor-health">
          <span class="doctor-kicker">Addon Health</span>
          <div class="doctor-scoreline">
            <span class="doctor-score">${avg}<span class="doctor-denom">/100</span></span>
            <span class="doctor-band">${bandLabel}</span>
          </div>
        </div>
        <div class="doctor-side">
          <p class="doctor-stats">
            <b>${s.total}</b> addons · <b>${s.problems}</b> with issues
            ${s.errors > 0 ? ` · <b>${s.errors}</b> error${s.errors === 1 ? "" : "s"}` : ""}
          </p>
          <button class="btn btn-primary btn-lg" data-panel-fixall ${fixable === 0 || app.busy ? "disabled" : ""}>
            ${icon("wrench", 16)}<span>Fix All${fixable ? ` (${fixable})` : ""}</span>
          </button>
        </div>
      </div>`;
  };

  const renderAllHealthy = (): string => {
    const avg = avgHealth();
    const s = app.scan!.stats;
    return `
      <div class="empty doctor-healthy">
        <span class="empty-icon doctor-healthy-icon">${icon("check-circle", 28)}</span>
        <h2 class="empty-title">All addons healthy</h2>
        <p class="empty-sub">Addon Health: ${avg}/100 — ${s.total} addon${s.total === 1 ? "" : "s"}, no issues found.</p>
        <div class="empty-actions">
          <button class="btn btn-outline" data-rescan>${icon("refresh", 16)}<span>Rescan</span></button>
        </div>
      </div>`;
  };

  const renderList = (): void => {
    if (!app.state.has_install) {
      list.innerHTML = emptyCard(
        "folder",
        "No WoW install configured",
        "Set up your World of Warcraft path to scan your addons.",
        `<button class="btn btn-primary" data-go-setup>${icon("folder", 16)}<span>Go to setup</span></button>`,
      );
      list.querySelector("[data-go-setup]")?.addEventListener("click", () => actions.go("setup"));
      return;
    }
    if (!app.scan) {
      list.innerHTML = `
        <div class="list-loading">
          <span class="spinner spinner-lg"></span>
          <span>${app.busy === "scan" ? "Scanning " + escapeHtml(app.state.addons_dir) + "…" : "Loading…"}</span>
        </div>`;
      return;
    }

    const scanErrors = app.scan.errors;
    const errorsHtml = scanErrors.length
      ? `<div class="scan-errors" role="alert">
          <span class="scan-errors-head">${icon("alert", 15)}<span>${scanErrors.length} problem${scanErrors.length === 1 ? "" : "s"} while scanning</span></span>
          <ul>${scanErrors.map((e) => `<li>${escapeHtml(e)}</li>`).join("")}</ul>
        </div>`
      : "";

    const hasAddons = app.scan.addons.length > 0;
    const allHealthy = hasAddons && app.scan.stats.problems === 0;
    const filtered = app.scan.addons.filter((a) => matches(a) && byHealth(a));

    let rows = "";
    if (app.scan.addons.length === 0) {
      rows = emptyCard(
        "check-circle",
        "No addons found",
        "Interface/AddOns is empty or unreadable. Rescan after adding addons.",
        `<button class="btn btn-outline" data-rescan>${icon("refresh", 16)}<span>Rescan</span></button>`,
      );
    } else if (filtered.length === 0) {
      const filteredByChip = healthFilter !== "all" && !app.filter;
      rows = emptyCard(
        "search",
        filteredByChip
          ? healthFilter === "issues"
            ? "No addons with issues"
            : "No healthy addons"
          : `No addons match “${escapeHtml(app.filter)}”`,
        filteredByChip
          ? "Switch to another view to see every addon."
          : "Try a different filter — it matches folder names, titles and problem messages.",
        filteredByChip
          ? `<button class="btn btn-outline" data-clear-health>${icon("x", 16)}<span>Show all</span></button>`
          : `<button class="btn btn-outline" data-clear-filter>${icon("x", 16)}<span>Clear filter</span></button>`,
      );
    } else {
      rows = filtered.map((a, i) => renderRow(a, i)).join("");
    }

    list.innerHTML = `
      ${errorsHtml}
      ${hasAddons ? renderChips() : ""}
      ${hasAddons ? (allHealthy ? renderAllHealthy() : renderPanel()) : ""}
      <div class="addon-rows">${rows}</div>`;

    list.querySelector("[data-rescan]")?.addEventListener("click", () => {
      pendingFocus = "rescan";
      actions.scan();
    });
    list.querySelector("[data-clear-filter]")?.addEventListener("click", () => {
      app.filter = "";
      renderToolbar();
      renderList();
      toolbar.querySelector<HTMLInputElement>(".search-input")?.focus();
    });
    list.querySelector("[data-clear-health]")?.addEventListener("click", () => {
      healthFilter = "all";
      renderList();
      list.querySelector<HTMLElement>('[data-health="all"]')?.focus();
    });
    list.querySelector("[data-panel-fixall]")?.addEventListener("click", () => {
      pendingFocus = "panel-fixall";
      void actions.fixAll();
    });
    list.querySelectorAll<HTMLElement>("[data-health]").forEach((chip) => {
      chip.addEventListener("click", () => {
        healthFilter = (chip.dataset.health ?? "all") as HealthFilter;
        renderList();
        list.querySelector<HTMLElement>(`[data-health="${healthFilter}"]`)?.focus();
      });
    });

    list.querySelectorAll<HTMLElement>("[data-row]").forEach((row) => {
      const addon = filtered[Number(row.dataset.row)];
      row.addEventListener("click", () => {
        toggle(addon.folder_name);
        // Mouse click on the row: land focus on the rebuilt expand control
        // so it does not drop to <body> after the re-render.
        list.querySelector<HTMLElement>(
          `[data-row="${row.dataset.row}"] [data-expand]`,
        )?.focus();
      });
    });
    list.querySelectorAll<HTMLElement>("[data-fix]").forEach((btn) => {
      const addon = filtered[Number(btn.dataset.fix)];
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        pendingFocus = focusKeyOf(btn);
        actions.fixOne(addon);
      });
    });
    list.querySelectorAll<HTMLElement>("[data-restore]").forEach((btn) => {
      const addon = filtered[Number(btn.dataset.restore)];
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        pendingFocus = focusKeyOf(btn);
        void (async () => {
          const confirmed = await confirmDialog({
            title: `Restore ${addon.folder_name}?`,
            message: `Restore ${addon.folder_name} from its source? This re-downloads and replaces the folder.`,
            details: addon.tracked_source
              ? [`Source: ${addon.tracked_source}`]
              : undefined,
            confirmLabel: "Restore",
          });
          if (confirmed) await actions.restoreAddon(addon);
        })();
      });
    });
  };

  const toggle = (folder: string): void => {
    if (expanded.has(folder)) expanded.delete(folder);
    else expanded.add(folder);
    renderList();
  };

  const renderRow = (a: Addon, i: number): string => {
    const isOpen = expanded.has(a.folder_name);
    const glyph: IconName =
      a.status === "error" ? "x-circle" : a.status === "warn" ? "alert" : "check-circle";
    const badgeClass =
      a.status === "error" ? "hb-error" : a.status === "warn" ? "hb-warn" : "hb-ok";
    const issue = a.issues[0];
    const rename =
      a.suggested_name && a.suggested_name !== a.folder_name
        ? `<span class="addon-rename mono">→ ${escapeHtml(a.suggested_name)}</span>`
        : "";
    const version = a.toc?.version ? `v${escapeHtml(a.toc.version)}` : "—";
    const integrityTag = a.tracked
      ? a.drifted
        ? `<span class="tag tag-drifted" title="Changed since install — differs from the recorded manifest checksum">${icon("alert", 11)}Modified</span>`
        : `<span class="tag tag-tracked" title="Installed from ${escapeAttr(a.tracked_source ?? "its provider")}">tracked</span>`
      : "";
    // Management state, mirroring the registry flags: pin/ignore live in
    // the Updates view, these are read-only indicators.
    const stateTag = [
      a.pinned
        ? `<span class="tag tag-pinned" title="Pinned — locked at the current version">${icon("lock", 11)}pinned</span>`
        : "",
      a.ignored
        ? `<span class="tag tag-ignored" title="Ignored — excluded from update management">ignored</span>`
        : "",
    ].join("");
    const fixableIssue = a.issues.find((x) => x.action);
    const destructive = fixableIssue
      ? DESTRUCTIVE_ACTIONS.has(fixableIssue.action)
      : false;
    const fixBtn = fixableIssue
      ? `<button class="btn btn-sm ${destructive ? "btn-danger" : "btn-primary"}" data-fix="${i}" ${
          app.busy ? "disabled" : ""
        }>${icon(destructive ? "trash" : "wrench", 14)}<span>${escapeHtml(
          fixableIssue.action_label || ACTION_LABELS[fixableIssue.action] || "Fix",
        )}</span></button>`
      : "";
    const restoreBtn =
      a.tracked && a.drifted
        ? `<button class="btn btn-sm btn-restore" data-restore="${i}" ${
            app.busy ? "disabled" : ""
          } title="Re-download ${escapeAttr(a.folder_name)} from ${escapeAttr(a.tracked_source ?? "its source")}">${icon(
            "download",
            14,
          )}<span>Restore</span></button>`
        : "";
    const more = a.issues.length > 1 ? `<span class="addon-more">+${a.issues.length - 1} more</span>` : "";

    const detail = isOpen ? renderDetail(a, i) : "";

    return `
      <div class="addon-row status-${a.status}${isOpen ? " expanded" : ""}" data-row="${i}" role="group"
        aria-label="${escapeAttr(a.folder_name)}: ${a.status}">
        <span class="addon-status-cell">
          <button class="addon-expand" data-expand="${i}" aria-expanded="${isOpen}"
            aria-controls="addon-detail-${i}"
            aria-label="${isOpen ? "Collapse" : "Expand"} ${escapeAttr(a.folder_name)} details">
            ${icon(isOpen ? "chevron-down" : "chevron-right", 14)}
          </button>
          <span class="addon-status status-${a.status}">${icon(glyph, 18)}</span>
          <span class="health-badge ${badgeClass}" title="Health ${a.health}/100">${a.health}</span>
        </span>
        <div class="addon-info">
          <div class="addon-name-line">
            <span class="addon-name">${escapeHtml(a.folder_name)}</span>
            ${a.nested ? `<span class="tag tag-nested">${icon("flatten", 12)}nested</span>` : ""}
            ${integrityTag}
            ${stateTag}
            ${rename}
          </div>
          <div class="addon-issue">${issue ? escapeHtml(issue.message) : `<span class="addon-clean">No issues</span>`}${more}</div>
        </div>
        <div class="addon-meta">
          <span class="addon-ver mono">${version}</span>
          <span class="addon-size mono">${formatBytes(a.size_bytes)}</span>
        </div>
        <div class="addon-fix">${restoreBtn}${fixBtn}</div>
      </div>
      ${detail}`;
  };

  const renderDetail = (a: Addon, i: number): string => {
    const issuesHtml = a.issues.length
      ? `<section class="detail-section">
          <h3 class="detail-title">Issues (${a.issues.length})</h3>
          <ul class="issue-list">${a.issues.map(issueHtml).join("")}</ul>
        </section>`
      : "";
    const compatHtml = a.compat.length
      ? `<section class="detail-section">
          <h3 class="detail-title">Compatibility</h3>
          <div class="table-wrap">
            <table class="table table-compact">
              <thead><tr><th>TOC</th><th>Expected</th><th>Detected</th><th>Status</th></tr></thead>
              <tbody>${a.compat
                .map(
                  (c) => `<tr>
                    <td class="mono">${escapeHtml(c.toc)}</td>
                    <td class="mono">${c.expected}</td>
                    <td class="mono">${c.detected > 0 ? c.detected : "—"}</td>
                    <td><span class="status-label status-${c.status}"><span class="status-dot ${dotClass(c.status)}"></span>${escapeHtml(c.label)}</span></td>
                  </tr>`,
                )
                .join("")}</tbody>
            </table>
          </div>
        </section>`
      : "";
    const tocHtml = a.toc
      ? `<p class="path-line mono">${escapeHtml(a.toc.title)} — ${escapeHtml(a.toc.name)}.toc</p>`
      : "";
    const path = app.scan ? `${app.scan.addons_dir}\\${a.folder_name}` : a.folder_name;
    return `<div class="addon-detail" id="addon-detail-${i}">
      ${issuesHtml}
      ${compatHtml}
      ${tocHtml}
      <p class="path-line mono">${icon("folder", 13)}<span>${escapeHtml(path)}</span></p>
    </div>`;
  };

  const issueHtml = (iss: Issue): string => `
    <li class="issue-item">
      <span class="issue-sev status-${iss.severity}">${icon(
        iss.severity === "error" ? "x-circle" : iss.severity === "warn" ? "alert" : "info",
        15,
      )}</span>
      <span class="issue-text">
        <span class="issue-msg">${escapeHtml(iss.message)}</span>
        ${iss.suggestion ? `<span class="issue-sugg">${escapeHtml(iss.suggestion)}</span>` : ""}
        ${iss.options.length ? `<span class="issue-options mono">${iss.options.map((o) => escapeHtml(o)).join(" · ")}</span>` : ""}
      </span>
    </li>`;

  const emptyCard = (glyph: IconName, title: string, sub: string, cta: string): string => `
    <div class="empty">
      <span class="empty-icon">${icon(glyph, 28)}</span>
      <h2 class="empty-title">${title}</h2>
      <p class="empty-sub">${sub}</p>
      <div class="empty-actions">${cta}</div>
    </div>`;

  renderToolbar();
  renderList();
  if (!app.scan && app.state.has_install && !app.busy) {
    void actions.scan();
  }

  return {
    refresh: () => {
      // Restore focus to the control the user last activated (async flows
      // re-render after the backend call); fall back to whatever currently
      // holds focus. Give up when the target is gone while idle.
      const active = document.activeElement;
      const key =
        pendingFocus ?? (active instanceof HTMLElement ? focusKeyOf(active) : null);
      renderToolbar();
      renderList();
      if (!key) return;
      if (restoreFocus(key)) pendingFocus = null;
      else if (!app.busy) pendingFocus = null;
    },
  };
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
