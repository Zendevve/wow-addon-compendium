# wowfix

A cross-platform terminal utility that scans your World of Warcraft `Interface/AddOns`
folder, finds the common addon installation problems, repairs them safely (with
backups and trash), validates TOC compatibility, and installs addons from ZIP
archives. One binary, no runtime dependencies.

![Go](https://img.shields.io/badge/Go-1.23+-00ADD8) ![Platform](https://img.shields.io/badge/Windows-Linux-macOS-lightgrey)

---

## Screenshot

```
⚔ wowfix dev C:\Games\World of Warcraft\Interface\AddOns  ·  3.3.5.12340 · Wrath of the Lich King 3.3.5a (expected interface 30300)  ·  flavor root
╭──────────────────────────────────────────────────────────────────────────────────────────────────╮
│ STATUS  ADDON        PROBLEM                          FIX                                          │
│ ▍ ✖  Inventory      No .toc file found in this folder  Move to Trash                              │
│    ⚠  Aux           TOC "Aux-Classic.toc" does not m…  Rename Folder                              │
│    ⚠  DPSMate       Nested folder                      Flatten Folder                             │
│    ⚠  GFW_Shaman    Vanilla addon                                                               │
│    ⚠  Questie-main  Folder name "Questie-main" looks …  Rename Folder                            │
│    ⚠  TempFolder    Folder is empty                    Move to Trash                             │
│    ✔  AtlasLoot     —                                                                           │
│    ✔  BigWigs       —                                                                           │
╰──────────────────────────────────────────────────────────────────────────────────────────────────╯
```

Run `wowfix preview` to render live text previews of the list, inspect and
confirmation screens.

## Features

- **Scan & detect** — every folder in `Interface/AddOns` is analyzed for:
  1. **GitHub folder names** — `Questie-main` → rename to `Questie` (`-main`, `-master`, `-dev`, version suffixes)
  2. **TOC name mismatch** — `Aux/` containing `Aux-Classic.toc` → rename folder to `Aux-Classic`
  3. **Nested folders** — `DPSMate-main/DPSMate/DPSMate.toc` → flatten to top level
  4. **Missing TOC** — folder without any `*.toc` → marked invalid, move to trash offered
  5. **Multiple TOCs** — `Atlas.toc` + `Atlas_Wrath.toc` + `Atlas_TBC.toc` → user picks the defining TOC
  6. **Empty folders** — detected, optionally trashed
  7. **Duplicate addons** — `Questie/` + `Questie-main/` → merge or delete
  8. **Broken extraction structure** — several addons dumped into one folder → promoted to top level
- **TOC validation** — parses every TOC, reports `Expected interface / Detected interface / Status`
  against any of 9 profiles: Vanilla 1.12, TurtleWoW, TBC 2.4.3, WotLK 3.3.5a,
  Cataclysm, Classic Era, Hardcore, Season of Discovery, Retail. TOCs are never edited.
- **ZIP installer** — `wowfix install addon.zip` extracts, flattens, renames and
  validates before installing. Drag-and-drop works (drop a zip onto the executable).
- **Backups** — every mutation is preceded by a `Backups/<timestamp>/` snapshot;
  `wowfix restore` brings folders back (the current state is snapshotted first).
- **Trash, never delete** — removals go to the OS trash (Recycle Bin on Windows,
  XDG trash on Linux, `~/.Trash` on macOS) with a cross-device fallback.
- **Auto-detection** — finds WoW installs in standard locations, Battle.net/Steam
  registry keys (Windows), Wine/Lutris/Proton prefixes (Linux), Applications (macOS);
  the game version is read from the client executable's PE version resource.
- **Logging** — every action is logged; view in the TUI (`l`) and export to text (`e`).
- **Config** — saved in the platform user config dir; remembers the WoW path,
  flavor, profile, theme, and last scan.

## Install / Build

Requires Go 1.23+.

```sh
go build -o wowfix ./cmd/wowfix
```

Version metadata:

```sh
go build -ldflags "-X github.com/wowfix/wowfix/internal/ui.Version=1.0.0 \
                   -X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD)" \
  -o wowfix ./cmd/wowfix
```

Cross-compile:

```sh
GOOS=windows GOARCH=amd64 go build -o wowfix.exe ./cmd/wowfix
GOOS=linux   GOARCH=amd64 go build -o wowfix      ./cmd/wowfix
GOOS=darwin  GOARCH=arm64 go build -o wowfix      ./cmd/wowfix
```

## Usage

### Terminal UI

```sh
wowfix
```

| Key             | Action                            |
|-----------------|-----------------------------------|
| `↑`/`↓` `k`/`j` | Navigate the addon list           |
| `Enter`         | Inspect the selected addon        |
| `f`             | Fix the selected addon            |
| `a`             | Fix all detected problems         |
| `d`             | Move the selected folder to trash |
| `r`             | Rescan                            |
| `b`             | Backup all addons                 |
| `l` / `e`       | Logs / export logs to a file      |
| `p`             | Choose game profile               |
| `s`             | Switch WoW installation           |
| `t`             | Toggle dark/light theme           |
| `q` / `Ctrl+C`  | Quit                              |

The inspect screen shows the TOC compatibility table
(expected vs detected interface per TOC), the issue list with suggested fixes,
and the target folder name.

### CLI

```
wowfix                        launch the terminal UI
wowfix scan                   scan and report problems
wowfix fix [--yes]            fix everything (backups first)
wowfix install addon.zip      install an addon archive
wowfix validate               TOC compatibility table
wowfix list                   list addons with status
wowfix backup                 snapshot all addons
wowfix restore [id]           list backups, or restore one
wowfix doctor                 environment & permission checks
wowfix config [set k v]       show or edit configuration
wowfix version                print version
wowfix preview                render a text preview of the TUI
```

Common flags: `--path <dir>` (WoW root, overrides config), `--yes` (skip
prompts), `--json` (machine-readable output for `scan`/`list`/`validate`).

### Configuration

Stored at `os.UserConfigDir()/wowfix/config.json`:

- `wow_path` — WoW installation root
- `flavor` — client subfolder (`_retail_`, `_classic_`, `_classic_era_`, `_classic_tbc_`, or root)
- `profile` — one of `vanilla, turtle, tbc, wrath, cata, classic, hardcore, sod, retail`
- `theme` — `dark` | `light`
- `auto_backup` — snapshot before every mutation (default `true`)
- `confirmations` — confirm destructive actions (default `true`)
- `backups_dir` — override the `Backups/` location (default: next to the game)

```sh
wowfix config set wow_path "D:\Games\World of Warcraft"
wowfix config set profile wrath
wowfix config set theme light
```

## Safety model

1. **Never overwrite without confirmation** — every rename/replace/trash/restore
   goes through a confirmation prompt (CLI) or dialog (TUI); `--yes` opts out explicitly.
2. **Always back up first** — each affected folder is copied to
   `Backups/<timestamp>/` before any change; disabled only via `auto_backup: false`.
3. **Never delete permanently** — removal means moving to the OS trash; if no
   native trash exists (or it fails, e.g. cross-device), a copy is kept in the
   fallback trash directory and the source is removed.
4. **Permission errors are graceful** — unreadable folders are reported per-addon
   and never abort a scan; unwritable destinations fail with a clear message.

## Architecture

```
cmd/wowfix/          CLI entry point (command dispatch, JSON output)
internal/
  models/            shared data types: Addon, TOC, Issue, Profile
  scanner/           detection only — never touches the filesystem
  validator/         TOC parser + compatibility classification
  fixer/             repairs: rename, flatten, merge, delete (with backups)
  installer/         ZIP extraction, normalization, install, validate
  backup/            timestamped snapshots + manifest + restore
  detector/          WoW install discovery + PE version parsing
  config/            persisted user configuration
  logger/            ring-buffer logger with file sink + export
  ui/                Bubble Tea TUI (list, inspect, logs, pickers, dialogs)
  utils/             filesystem helpers, cross-platform trash, PE parser
```

The core packages (scanner, validator, fixer, installer, backup, config) are
pure business logic with no UI dependency — the CLI and the TUI are two thin
front-ends over the same engine, so the whole feature set is available from
both.

## Testing

```sh
go test ./...
go vet ./...
```

The scanner and validator have unit tests covering every detection rule and
every compatibility classification. A manual smoke-test fixture lives in
`testdata/wow`:

```sh
wowfix scan     --path testdata/wow
wowfix fix      --path testdata/wow --yes
wowfix restore  --path testdata/wow
```

## Extensibility

The architecture is designed so the tool can grow into a full addon manager
without refactoring:

- **GitHub integration / Update manager** — a new provider package can sit
  beside `installer` and reuse `scanner` + `fixer` for the drop-in step;
  `TOC.Version` is already parsed for version comparison.
- **Profiles** — the profile table in `models` is the single source of truth;
  enable/disable state per profile slots in naturally alongside `config`.
- **Import/export** — `scan`/`list` already emit JSON; YAML/manifest exports are
  thin serializers over `ScanResult`.
- **Plugin rules** — scanner issues are plain data (`IssueKind`); new rules plug
  into `analyzeEntry` without touching the fixer.
- **Public API** — all business logic lives in importable packages with no UI
  coupling, so a desktop GUI, web UI, REST API or scripting front-end can be
  built against the same engine.

## Roadmap

- Install from GitHub releases / default-branch zips, with update checks
- Addon profiles (enable/disable sets, PvE/PvP/Raiding/Leveling presets)
- Export/import collections (zip, manifest, JSON, YAML)
- Addon catalog browser (search, filter, compatibility info)
- SavedVariables backup/reset/migration

## License

Proprietary — see [LICENSE](LICENSE). Copyright (c) 2026 Zendevve. All rights
reserved. Personal, non-commercial use within World of Warcraft is permitted;
modification, redistribution, and reuse of the source code require prior
written permission from the copyright holder.
