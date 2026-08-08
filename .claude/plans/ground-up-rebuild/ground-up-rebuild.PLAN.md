# wowfix v3 — Ground-up rebuild (GUI-only)

## Goal

Archive v2.3.0 (DONE: branch `archive/v2-legacy` + `D:/COMPROG/wow addon compendium/archive/wowfix-v2.3.0-2026-08-08.zip`), then rebuild wowfix from the ground up as a **GUI-only** desktop app. Keep the Go core ("the go things we did"), drop the CLI, apply the Framer design system (see Design System section), and embody everything learned from the full 30-project competitor study (`refs/competitors/study/` — in flight; `docs/LEARNINGS.md` is written after it lands).

## Decisions (settled)

| # | Decision | Rationale |
|---|---|---|
| D1 | Same repo `github.com/wowfix/wowfix`, same name. v2 lives on `archive/v2-legacy`; rebuild on `main` | Archive-branch rewrite; history preserved |
| D2 | GUI only. Delete `cmd/wowfix/*` (CLI). Root `main.go` (wails entry) stays. | "gui only this time" |
| D3 | Carry the Go core packages + `internal/service` facade as-is (they are tested, safety-first, and ahead of the field) | "still gonna be using the go things we did" |
| D4 | Frontend rebuilt from scratch per the Framer design system (Design System section below) | "start from the ground up" + "this design system" |
| D5 | Feature surface = same capabilities as v2: Setup, Scan/Doctor/Validate, Catalog (5 providers + curated), Updates (tracked/pin/ignore/history/rollback), Collections, Backups (+offline snapshot export/check), SavedVariables, Installations, Import/Export | Feature completeness is the point; only the surface is rebuilt |
| D6 | No command palette, no standalone `install`/`exportimport` destinations, no CLI-parity deep links. Install surface = Catalog; Export/Import = Collections; Installations = Settings | Ground-up = leaner shell |
| D7 | Mock mode kept (`?mock=1&view=<view>`), rebuilt per-view (`src/mock/<view>.ts`), for screenshot regeneration | Documented workflow (README) depends on it |
| D8 | Destructive ops keep the safety model: backups first, OS trash, never delete | Competitor field's #1 weakness; wowfix is best-in-class, never regress |

## Current state (verified 2026-08-08)

- v2.3.0 tagged, tree clean, archived. Go 1.23.4 installed; go.mod pins `go 1.25.0`; GOTOOLCHAIN=auto fetches it (builds verified working).
- `wails` CLI v2.13.0 installed; `CGO_ENABLED=0`, no gcc — modern Wails v2 on Windows builds pure-Go (WebView2 via go-winloader). `wails build` worked previously.
- Go core (`internal/`): catalog (curseforge/github/tukui/wowinterface/wago + curated + snapshot + registry + updater + semver), scanner, fixer, validator, installer, detector, backup, savedvars, profiles, importexport, config, models, utils, logger, e2e, gui. All tested (e2e is pure Go, no CLI dependency).
- `internal/service` = ~50 Wails-bound methods (DTO-facade; the frontend contract, frozen).
- Frontend: Vite + vanilla TS, no framework; fonts via `@fontsource-variable/inter` + `@fontsource-variable/mona-sans`; `frontend/wailsjs/` generated bindings (gitignored).

## Target tree

```
wowfix/
  main.go                  (wails entry; unchanged)
  wails.json  go.mod  go.sum  .github/  testdata/  (carried)
  internal/                (carried; cmd/ deleted)
  frontend/                (REBUILT)
    index.html  package.json  vite.config.ts  tsconfig.json
    src/
      tokens.css           design tokens (Design System)
      base.css             reset, scrollbars, selection, focus-visible
      components.css       named components (button-primary, card, spotlight-card, …)
      shell.css            sidebar / statusbar / view frame
      api.ts               typed facade over window.go.wowfix.Service.* + mock switch
      types.ts             DTO types mirroring internal/service DTOs
      view.ts              View/ViewRegistration contract (below)
      main.ts              bootstrap, ?view= router, sidebar shell, view registry
      mock/                mock/<view>.ts per view (mock data), mock/index.ts flag helper
      views/               setup.ts overview.ts catalog.ts updates.ts collections.ts
                           backups.ts savedvars.ts settings.ts (+ per-view .css)
  DESIGN.md                design system (front-matter token format)
  docs/LEARNINGS.md        competitor study → wowfix v3 decisions (after study lands)
  README.md  CHANGELOG.md  rewritten for v3 (Verify phase)
```

## Frontend contracts

### View registration (src/view.ts — scaffold-owned)
```ts
export interface View {
  id: string;                 // ?view=<id>
  label: string;              // sidebar label
  icon: string;               // icon id from icons.ts
  mount(host: HTMLElement): void | Promise<void>;
  unmount?(): void;
}
```
- `views/<id>.ts` exports `const view: View`. main.ts imports ALL eight statically (scaffold writes stubs for every view up front so tsc always passes; view agents REPLACE only their own file).
- Router: `?view=<id>` selects; no match → `overview` (or `setup` when `GetState().HasInstall === false`). Setup is full-window (no sidebar).
- Mock: `window.__WOWFIX_MOCK__` flag (set in index.html pre-module script, same as v2). Views check it and use their `mock/<view>.ts` data instead of api calls. `mock/index.ts` exports `isMock()`.

### File ownership (zero-conflict rule)
- Scaffold owns: index.html, package.json, vite.config.ts, tsconfig.json, tokens.css, base.css, components.css, shell.css, api.ts, types.ts, view.ts, main.ts, app.ts, icons.ts, mock/index.ts, ALL stub views.
- Each view slice owns ONLY: `views/<name>.ts`, `views/<name>.css`, `mock/<name>.ts`. No other file edits.

### Service API (frozen contract; bindings via `wails generate module` → frontend/wailsjs)
GetState, DetectInstalls, SetInstall, SetProfile, Profiles, Scan, Validate, Fix, FixAll, InstallZip, CheckUpdates, ExportSnapshot, CheckSnapshot, ApplyUpdate, ApplyAllUpdates, SearchCatalog, Curated, InstallSource, SaveWagoImport, RestoreAddon, TrackedAddons, SetAddonPinned, SetAddonIgnored, RollbackAddon, ListAddonVersions, RollbackToVersion, Collections, CreateCollection, SwitchCollection, DeleteCollection, CollectionDetail, SetCollectionAddon, InstallsStatus, SyncUpdatesToAll, SavedVarsAccounts, SavedVarsList, SavedVarsBackup, SavedVarsRestore, SavedVarsReset, SavedVarsMigrate, BackupNow, ListBackups, RestoreBackup, ExportCollection, ImportCollection, Config, SetConfigKey, AddonInfo, Sources, Doctor.

## Design System (from the Framer spec, app-adapted)

Dark-only. Canvas is the ground; hierarchy via surface lift; ONE accent (blue) for links/focus/selection; gradient spotlight cards are CARDS (≤2 per viewport), never section backgrounds; pill CTAs only; display tracking negative by percentage; Inter OpenType variants are the body voice.

### Tokens (exact values — carry from current frontend/src/tokens.css, do not rename)
- Surfaces: `--canvas:#0b0b0a` `--surface-1:#161615` `--surface-2:#1f1f1e` `--canvas-inverse:#ffffff` `--hairline:rgba(255,255,255,.08)` `--hairline-strong:.14` `--hairline-soft:.055` `--border-field:rgba(255,255,255,.25)`
- Text: `--ink:#f7f7f6` `--ink-muted:#999` `--ink-faint:#7f7f7f` `--ink-disabled:rgba(153,153,153,.55)` — binary hierarchy ink/ink-muted only
- Accent: `--accent:#0099ff` `--accent-strong:#33adff` `--accent-dim:rgba(0,153,255,.12)` — links, focus rings, selection ONLY
- Status: `--ok:#3fb950` `--warn:#d29922` `--error:#f85149` (+dim variants)
- Gradients (cards only): violet/magenta/orange/coral as in tokens.css
- Type: display Mona Sans Variable; body Inter Variable with `font-feature-settings:"cv01","cv05","cv09","cv11","ss03","ss07","dlig"` (+`tnum` for numerics). Scale: display 40/-2.0/1.0 · title 28/-1.4/1.1 · sub 20/-0.6/1.2 · headline 17/600/-0.4/1.25 · body 15/-0.15/1.35 · body-sm 14/500/-0.14/1.4 · caption 13/500/-0.13/1.2 · micro 12/1.2 · button 14/500/-0.14/1.0
- Radius: 4/6/10/15/20/30/100/9999 (`--r-xs…full`); cards 20 (xl), spotlight cards 30 (xxl), CTAs pill
- Spacing (5px base): 4/8/12/15/20/30/40/96 (`--sp-1…8`)
- Elevation: 0 flat · 1 surface lift · 2 light-edge+drop (`--shadow-2`) · 3 blue ring (`--ring-selected`); focus = `--ring-focus` (2px accent + halo)

### Components (components.css)
`button-primary` (white pill, ink-on-white, press = scale shrink) · `button-secondary` (charcoal pill) · `text-input` (+focused blue ring) · `tabs` (selected = surface-2 lift, no color) · `card` (surface-1, r-xl) · `card-featured` (surface-2) · `spotlight-card` (gradient, r-xxl, pad 30) · `row` (hairline-soft underline) · `sidebar` (canvas, icon rail + labels, selected = surface-2 + accent ring) · `statusbar` (canvas, caption, muted Ctrl+K-free) · `dialog` (surface-2, shadow-2, focus ring) · `toast`.

### Rules
DO: dark only; binary ink/ink-muted; accent only for links/focus/selection; pill CTAs; negative display tracking; surface lift for hierarchy; ≤2 spotlight cards per viewport; Inter OpenType variants everywhere.
DON'T: light mode; extra grays; accent as button fill; square CTAs; gradient section backgrounds; multi-accent palettes; mid-weight ramps (500 display / 400 body band).

## Slices (dispatch order)

1. **S1 Core strip** (task): delete `cmd/wowfix/*`, update `ci.yml` (drop `Build (CLI only)` + `crosscompile` jobs, clean stale `cmd/wowfix-gui` exclusions) + `release.yml` (drop CLI job, keep gui), verify `go vet ./internal/...` + `go test ./internal/...`. No commit.
2. **S2 Scaffold** (task, after S1 or parallel): frontend ground-up per contracts above; stubs for all 8 views; `npm run build` (tsc + vite) must pass; DESIGN.md written from Design System section; brief includes full pasted Framer spec behavior notes where they exceed the tokens (component recipes, do/don'ts).
3. **S3 LEARNINGS** (task, after study wave lands): `docs/LEARNINGS.md` from `refs/competitors/study/README.md` + per-project notes + closed-source study → what wowfix v3 does about each finding.
4. **Views** (4 parallel tasks, after S2): V-setup/overview · V-catalog · V-updates · V-management (collections+backups+savedvars+settings). Each owns only its files per File ownership. Verify: `npx tsc --noEmit` + vite dev renders with `?mock=1`.
5. **Verify** (Main): `npm run build`, `wails build`, launch smoke, browser screenshot pass at 1440×900 (replace `screenshots/`), rewrite README/CHANGELOG (v3.0.0), single commit on main (no AI attribution).

## Non-goals
- No new Go features during the rebuild (carry as-is; LEARNINGS may propose follow-ups).
- No light mode. No CLI. No installer/signing work beyond existing release workflow.
- No changes to the archived branch.
