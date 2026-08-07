# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Frontend restyle — Framer design language** — the GUI now uses a
  near-black canvas (`#0b0b0a`) with warm charcoal surfaces, a white-pill
  CTA system and accent blue reserved for links, focus rings and
  selection states; Inter Variable + Mona Sans Variable are bundled via
  `@fontsource`. `frontend/src/style.css` is a module entry importing
  tokens/base/components/shell/setup/scan/lists/catalog, and gradient
  spotlight cards anchor the setup brand panel and the catalog's curated
  band.

## [2.1.0] - 2026-08-07

### Added

- **Updates view** — check for updates, dry-run update-all, per-addon
  apply and flavor-mismatch gating.
- **Catalog view** — five providers including WeakAuras/Wago imports and
  paste-URL install.
- **Collections view** — create, switch, detail and per-addon toggles.
- **Installs view** — per-install health cards and cross-install
  update-all.
- **Addon Doctor** — health-score scan view with a fix-all diff toast.
- **Managed addons** — pin/ignore/rollback for tracked addons, integrity
  drift badges and restore-from-source.
- **Recommended section** — curated private-server addon sets (vanilla
  clones, ChromieCraft 3.3.5a) with a flavor-compat filter.
- **Wago provider** — WeakAuras search and imports in the catalog; addon
  manifest checksums.
- **Offline catalog snapshots** — `wowfix snapshot export|check`; backup
  per-folder rollback.
- **CLI** — `wowfix curated list|install`.

### Changed

- **Go 1.25+ required (wails v2.13.0); Node.js needed to build the
  frontend** — the CI test job now builds the frontend before vet/test
  (embed fix) on Go 1.25.x pins.

## [2.0.0] - 2026-08-07

### Added

- **Wails v2 desktop GUI** — a Windows desktop app (scan/fix, TOC
  validation, ZIP install) bound to the same core packages as the CLI;
  the Bubble Tea TUI is removed. Building now requires Go 1.25+ (wails
  v2.13.0) and Node.js for the frontend; the GUI runs on Windows with
  the WebView2 runtime.

### Fixed

- **Manual path entry accepts `Interface\AddOns` paths** — pasting an
  AddOns (or `Interface`) folder into the path prompt now resolves back to
  the game root and flavor instead of failing with `not a directory`. The
  same input works for `wowfix --path`.
- **Clearer path errors** — the scan path error now distinguishes a missing
  path (`path does not exist`), a non-directory (`not a directory`) and a
  root without an AddOns folder, so a bad manual entry is actionable.
- **No more dead-end picker bounce** — when a manually entered or saved
  path fails to scan, the UI returns to the path prompt with the value
  prefilled instead of dropping the user in an empty picker with a second
  `No WoW installation auto-detected` toast.

### Added

- **Private-server client detection** — a folder is accepted as a WoW
  installation when it contains a known client executable (`wow.exe`,
  `WowClassic_TBC.exe`, `wow-64.exe`, …) or an `Interface` folder, even
  when `Interface\AddOns` does not exist yet. The folder is created on the
  first scan so fresh or partial clients work immediately (UI and CLI).
- **Clipboard in text inputs** — `ctrl+v` pastes into the focused input
  (path, filter, catalog search) at the cursor, and `ctrl+y` copies its
  value. `ctrl+c` still quits; terminal-native copy shortcuts keep working.

### Changed

- **TUI visual redesign** — the whole interface now shares one design
  language: a structured header that stays on one line (path middle-
  truncates), a reworked addon list where the problem is always visible
  (status · addon · problem · version · source · fix), two-line
  installation picker, severity-colored toasts, actionable empty states,
  a per-view summary line ("N addons · K with issues · E errors"), a
  two-column help overlay and width-constrained rows in every list
  (catalog, updates, profiles, SavedVariables, logs).

### Removed

- Bubble Tea terminal UI (`internal/ui`) and the `preview` command; the bare
  `wowfix` invocation now prints help and points to the desktop GUI.

## [1.0.0] - 2026-08-05

### Added

- **Repair engine** — scans `Interface/AddOns` and detects the common addon
  installation problems (GitHub-style folder names, TOC name mismatches,
  nested folders, missing TOCs, multiple TOCs, empty folders, duplicates,
  broken extraction structures), then repairs them safely with backups and
  OS-trash removal.
- **TOC validation** — parses every TOC and reports expected/detected
  interface compatibility against nine game profiles (Vanilla 1.12,
  TurtleWoW, TBC 2.4.3, WotLK 3.3.5a, Cataclysm, Classic Era, Hardcore,
  Season of Discovery, Retail).
- **Catalog & providers** — parallel search across GitHub, CurseForge,
  WowInterface and Tukui with merged results; installs addons from ZIP
  archives, provider URLs and `owner/repo` sources; graceful degradation
  when a provider is unreachable.
- **Update manager** — tracks catalog installs in a registry; `wowfix update`
  checks every tracked addon against its provider and applies newer
  releases, flagging game-version mismatches.
- **Addon profiles (collections)** — capture the current addon setup as a
  named collection, switch between collections via `.disabled` folder
  renames, and duplicate/rename/delete them.
- **SavedVariables** — per-account backup, restore, reset and migration of
  `WTF/Account/<account>/SavedVariables` files.
- **Import / export** — share addon setups as JSON/YAML manifests, bundle
  ZIPs (manifest + local addons + SavedVariables) or GitHub repo lists.
- **Terminal UI v2** — Bubble Tea TUI with fuzzy addon filter, help overlay,
  catalog browser, updates panel, collections view, SavedVariables view,
  install-from-source, logs and mouse-wheel scrolling throughout.
- **CLI** — full command set (`scan`, `fix`, `install`, `validate`, `list`,
  `search`, `update`, `backup`, `restore`, `doctor`, `config`, `profile`,
  `savedvars`, `export`, `import`, `preview`, `version`) with `--json`
  machine-readable output.
- **Safety model** — snapshot backup before every mutation, removal via the
  OS trash (never permanent deletion), confirmation prompts for destructive
  actions, and graceful handling of permission errors.
- **CI & e2e** — GitHub Actions workflow (fmt, vet, test, build,
  cross-compile) on Ubuntu and Windows, plus an end-to-end pipeline test
  (scan → fix → backup/restore → install) against a fake addon tree.
- **Release** — `v1.0.0` tag with reproducible versioned builds
  (`-ldflags` metadata) and this changelog.
