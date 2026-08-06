# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
