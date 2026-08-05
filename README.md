# wowfix

A cross-platform terminal utility that scans your World of Warcraft `Interface/AddOns`
folder, finds the common addon installation problems, repairs them safely (with
backups and trash), validates TOC compatibility, and installs addons from ZIP
archives. One binary, no runtime dependencies.

![Go](https://img.shields.io/badge/Go-1.23+-00ADD8) ![Platform](https://img.shields.io/badge/Windows-Linux-macOS-lightgrey) ![CI](https://img.shields.io/github/actions/workflow/status/Zendevve/wow-addon-compendium/ci.yml?branch=main)

---

## Screenshot

The main list (live output of `wowfix preview`, which renders seven panels:
list, catalog browser, updates, catalog detail, help, collections and
SavedVariables):

```
── LIST ──
⚔ wowfix dev C:\Games\World of Warcraft\Interface\AddOns  ·  3.3.5.12340 · Wrath of the Lich King 3.3.5a (expected interface 30300)  ·  flavor root
╭──────────────────────────────────────────────────────────────────────────────────────────────────╮
│ STATUS   ADDON                VERSION  SRC   PROBLEM                            FIX              │
│ ▍  ✖      Inventory            —        local No .toc file found in this folder  Move to Trash   │
│   ⚠      Aux                  1.0      local TOC "Aux-Classic.toc" does not ma… Rename Folder    │
│   ⚠      DPSMate              1.0      local Nested folder                      Flatten Folder   │
│   ⚠      GFW_Shaman           —        local Vanilla addon                                       │
│   ⚠      Questie-main         1.12.2   GH    Folder name "Questie-main" looks … Rename Folder    │
│   ⚠      TempFolder           —        local Folder is empty                    Move to Trash    │
│   ✔      AtlasLoot            7.0.4    CF    —                                                   │
│   ✔      BigWigs              —        local —                                                   │
╰──────────────────────────────────────────────────────────────────────────────────────────────────╯
```

Full seven-panel preview: run `wowfix preview`.

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
- **Catalog & providers** — `wowfix search <query>` queries GitHub,
  CurseForge, WowInterface and Tukui in parallel and merges the results;
  `wowfix install <url|owner/repo>` installs straight from any provider.
- **Update manager** — catalog installs are tracked in a registry;
  `wowfix update` (or `u`/`U` in the TUI) checks every tracked addon
  against its provider and applies newer releases. Update safety:
  updates targeting a different game version are flagged (⚠) and
  skipped by default unless you confirm.
- **Addon profiles** — capture the current addon setup as a named
  collection (PvE/PvP/Raiding/Leveling presets) and switch between
  them; switching renames folders to `<name>.disabled` and back
  (`o` in the TUI).
- **SavedVariables** — back up, restore, reset and migrate between
  accounts the per-account `SavedVariables` files under
  `WTF/Account/<account>/` (`v` in the TUI).
- **Import / Export** — share addon setups as a JSON or YAML manifest,
  a bundle ZIP (manifest + local addon folders + SavedVariables) or a
  GitHub repo list; importing installs through the catalog.
- **TUI v2** — fuzzy addon filter (`/`), a help overlay (`?`), a catalog
  browser (`c`), an updates panel (`u`/`U`), install-from-source (`i`)
  and mouse-wheel scrolling throughout.
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
| mouse wheel     | Scroll the list / inspect / logs  |
| `Enter`         | Inspect the selected addon        |
| `Esc`           | Back to the previous view         |
| `f`             | Fix the selected addon            |
| `a`             | Fix all detected problems         |
| `d`             | Move the selected folder to trash |
| `r`             | Rescan                            |
| `b`             | Backup all addons                 |
| `l` / `e`       | Logs / export logs to a file      |
| `c`             | Open the addon catalog browser    |
| `u` / `U`       | Updates view: update selected / update all |
| `i`             | Install an addon from a source (URL or `owner/repo`) |
| `s`             | Switch WoW installation           |
| `p`             | Choose game profile               |
| `o` / `O`       | Manage addon collections (profiles) |
| `v` / `V`       | SavedVariables (backup / reset)   |
| `/`             | Fuzzy-filter the addon list       |
| `?`             | Help overlay (all keybindings)    |
| `t`             | Toggle dark/light theme           |
| `q` / `Ctrl+C`  | Quit                              |

Per-view extra keys (all views also take `Esc` to go back):

- **Catalog** (`c`) — `↑`/`↓` move, `/` focus search, `S` cycle sort,
  `W` cycle version filter, `d` open details, `Enter` install action.
- **Catalog detail** (`d` in the catalog) — `↑`/`↓` scroll, `o` open
  homepage, `g` open GitHub releases, `i` install, `Enter` back.
- **Updates** (`u`) — `↑`/`↓` move, `u` update selected, `U` update all,
  `Enter` open update details.
- **Collections** (`o`) — `↑`/`↓` move, `Enter` switch (confirmed),
  `n` create from the current setup, `d` duplicate, `r` rename, `x` delete.
- **SavedVariables** (`v`) — `↑`/`↓` move, `Enter` cycle accounts,
  `b` back up to the config dir, `r` reset the selected file (confirmed).

The inspect screen shows the TOC compatibility table
(expected vs detected interface per TOC), the issue list with suggested fixes,
and the target folder name.

### CLI

```
wowfix                        launch the terminal UI
wowfix scan                   scan the AddOns folder and report problems
wowfix fix [--yes]            fix all detected problems (backups first)
wowfix install <addon.zip>    install an addon archive [--yes]
wowfix install <url|owner/repo>  install from a provider source
wowfix validate               validate TOC compatibility
wowfix list                   list addons with status
wowfix search <query>         search the addon catalog
wowfix update [--yes]         check and apply addon updates
wowfix sources                list catalog providers and their caveats
wowfix backup                 snapshot all addons
wowfix restore [id]           list backups, or restore one
wowfix doctor                 check environment and permissions
wowfix config [set <key> <val>]  show or edit configuration
wowfix profile                manage addon collections (list/show/create/duplicate/
                              rename/delete/switch/enable/disable)
wowfix savedvars              list/back up/restore/reset/migrate SavedVariables
wowfix export <out.json|out.yaml|out.zip>  export a collection [--collection <id>] [--savedvars]
wowfix import <file|url>      import a manifest (JSON/YAML), bundle zip or GitHub repo list
wowfix version                print version
wowfix preview                render a text preview of the TUI
wowfix help                   show this help
```

Common flags: `--path <dir>` (WoW root, overrides config), `--yes` (skip
prompts), `--json` (machine-readable output for `scan`/`list`/`validate`/
`search`/`sources`/`install`/`restore`/`config`/`profile`/`savedvars`/
`export`/`import`). Command-specific flags: `--account <name>` and
`--dest <dir>` (`savedvars`), `--from <account>` `--to <account>` and
`--addon <name>` (`savedvars migrate`), `--collection <id>` and
`--savedvars` (`export`).

`install` accepts a local `.zip`, a provider URL (CurseForge, WowInterface,
Tukui, GitHub) or a GitHub `owner/repo` pair. `search` degrades gracefully
when a provider is unreachable: the working results are printed and the
provider errors go to stderr. GitHub's unauthenticated API is rate-limited
to ~60 requests/hour per IP.

### Configuration

Stored at `os.UserConfigDir()/wowfix/config.json`:

- `wow_path` — WoW installation root
- `flavor` — client subfolder (`_retail_`, `_classic_`, `_classic_era_`, `_classic_tbc_`, or root)
- `profile` — one of `vanilla, turtle, tbc, wrath, cata, classic, hardcore, sod, retail`
- `theme` — `dark` | `light`
- `auto_backup` — snapshot before every mutation (default `true`)
- `confirmations` — confirm destructive actions (default `true`)
- `backups_dir` — override the `Backups/` location (default: next to the game)
- `curseforge_api_key` — enables the modern CurseForge Core API
  (without it the catalog falls back to the deprecated legacy endpoint);
  the `WOWFIX_CURSEFORGE_API_KEY` environment variable takes precedence
- `collection` — the active addon-collection id (set by `profile switch`)
- `collections_dir` — where collection files live (default: `<config dir>/collections`)

```sh
wowfix config set wow_path "D:\Games\World of Warcraft"
wowfix config set profile wrath
wowfix config set theme light
wowfix config set curseforge_api_key "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
WOWFIX_CURSEFORGE_API_KEY=... wowfix search weakauras
```

### Addon Profiles

A profile (called "collection" in the config and CLI to avoid colliding
with the game-version `profile` key) is a named snapshot of which addons
are enabled. WoW disables an addon when its folder name ends in
`.disabled`, so switching a collection renames folders between `<name>`
and `<name>.disabled` — nothing is deleted and every switch is preceded
by a backup snapshot.

```sh
wowfix profile create pve            # capture the current setup
wowfix profile list                  # list collections (active marked)
wowfix profile show pve              # per-addon enable/disable state
wowfix profile duplicate pve raiding # copy a collection
wowfix profile rename pve pvp        # rename (id stays stable)
wowfix profile enable pve Questie    # flip one addon's state
wowfix profile disable pve Dead      # ...
wowfix profile switch pve --yes      # apply: renames folders, sets config collection
wowfix profile delete pve
```

In the TUI press `o` for the collections view: `enter` switches (with a
confirm dialog), `n` creates from the current state, `d`/`r` duplicate
and rename, `x` deletes. Collections live as one `<id>.json` file per
collection in `collections_dir` (default `<config dir>/collections`);
ids are sanitized names, `-2`, `-3`, ... appended on collisions.

### SavedVariables

Addon settings live in `<wtfRoot>/Account/<account>/SavedVariables/*.lua`
where `wtfRoot` is derived from the install (`<root>/WTF` for root
installs, `<root>/<flavor>/WTF` for flavor subfolders).

```sh
wowfix savedvars list                        # files of the first account
wowfix savedvars list --account "123#1"      # or an explicit account
wowfix savedvars backup --dest D:\sv-backups # timestamped copy, prints path
wowfix savedvars restore D:\sv-backups\2026-…  # restores; current state snapshotted first
wowfix savedvars reset DBM                   # deletes DBM.lua (exact stem: DBM-Core.lua survives)
wowfix savedvars migrate --from "123#1" --to "alt#1"          # copy all SavedVariables between accounts
wowfix savedvars migrate --from "123#1" --to "alt#1" --addon DBM  # copy a single addon's settings
```

With several accounts and no `--account`, the first is used and the
choice is announced. Restore refuses paths outside the WTF root, and
reset matches the exact file stem so `DBM` never deletes `DBM-Core.lua`.
Migration never overwrites existing destination files (they are skipped
and reported) and requires both accounts to exist.
In the TUI press `v`: `enter` cycles accounts, `b` backs up to the
config dir, `r` resets the selected file (confirmed).

### Import / Export

A manifest is JSON describing an addon setup:

```json
{
  "version": 1,
  "name": "pve",
  "game_version": "wrath",
  "addons": [
    {"folder": "Questie", "provider": "github", "id": "Vendethiel/Questie",
     "source": "Vendethiel/Questie", "version": "1.2.3"},
    {"folder": "LocalOnly"}
  ]
}
```

`provider`/`id`/`source` describe where to install the addon from
(`source` is the URL form accepted by `install`); entries without a
provider are local-only and travel inside a bundle.

```sh
wowfix export out.json                    # manifest of the current setup
wowfix export out.yaml                    # the same manifest as YAML
wowfix export bundle.zip --savedvars      # bundle: manifest + local addon folders + SavedVariables
wowfix export out.json --collection pve   # export a specific collection
wowfix import out.json                    # install remote entries; local ones are checked/skipped
wowfix import out.yaml                    # YAML manifests import the same way
wowfix import bundle.zip                  # installs remote + local entries, restores savedvars/
wowfix import https://gist.github.com/…/list.txt   # one "owner/repo" per line, # = comment
```

A manifest is JSON or YAML describing an addon setup; the extension
(`.json`, `.yaml`, `.yml`) picks the format on import.

A bundle zip contains `manifest.json` at the root, local addon folders
under `addons/<Folder>/` and SavedVariables under `savedvars/` (restored
into the first account's SavedVariables on import). Zip entries are
checked against path traversal before anything is extracted, and local
addons are never silently overwritten. Importing a bundle with remote
addons requires the catalog (`wowfix import` wires it automatically).

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
  catalog/           providers (GitHub/CurseForge/WowInterface/Tukui), registry, updater
  scanner/           detection only — never touches the filesystem
  validator/         TOC parser + compatibility classification
  fixer/             repairs: rename, flatten, merge, delete (with backups)
  installer/         ZIP extraction, normalization, install, validate
  backup/            timestamped snapshots + manifest + restore
  profiles/          addon collections: capture, apply (.disabled renames)
  savedvars/         SavedVariables backup/restore/reset under WTF/Account
  importexport/      manifest/bundle/GitHub-list export & import
  detector/          WoW install discovery + PE version parsing
  config/            persisted user configuration
  logger/            ring-buffer logger with file sink + export
  ui/                Bubble Tea TUI (list, inspect, logs, catalog, updates, profiles, savedvars, dialogs)
  utils/             filesystem helpers, cross-platform trash, PE parser
```

The core packages (scanner, validator, fixer, installer, backup, catalog,
config, profiles, savedvars, importexport) are pure business logic with
no UI dependency — the CLI and the TUI are two thin front-ends over the
same engine, so the whole feature set is available from both.

## Testing

```sh
go test ./...
go test ./internal/e2e/ -count=1 -v   # end-to-end pipeline: scan -> fix -> backup/restore -> install
go vet ./...
```

The scanner and validator have unit tests covering every detection rule and
every compatibility classification. The `internal/e2e` test drives the whole
pipeline against a fake `Interface/AddOns` tree in a temp dir: it scans the
fixture, fixes every problem (with backups and trash), restores a corrupted
tree from a snapshot and installs addons from a ZIP archive. A manual
smoke-test fixture lives in `testdata/wow`:

```sh
wowfix scan     --path testdata/wow
wowfix fix      --path testdata/wow --yes
wowfix restore  --path testdata/wow
```

**CI** — a GitHub Actions workflow (`.github/workflows/ci.yml`) runs
`gofmt`, `go vet`, `go test` and `go build` on Ubuntu and Windows for every
push/PR touching Go sources, and cross-compiles the CLI for
linux/amd64, darwin/arm64 and windows/amd64.

## Extensibility

The architecture is designed so the tool can grow into a full addon manager
without refactoring:

- **Providers** — a provider is one `catalog.Provider` implementation
  (`Search`, `Resolve`, `Latest`, `Download`); the merged search, the
  registry and the updater all treat providers uniformly, so adding
  Wago, GitLab or a private source is a new file, not a rewrite.
- **Profiles** — the profile table in `models` is the single source of truth;
  addon collections (enable/disable state per addon) are implemented in
  `internal/profiles`.
- **Import/export** — collection sharing ships as `internal/importexport`
  (manifest, bundle zip, GitHub repo list); new formats are thin
  serializers over the same `Manifest` type.
- **Plugin rules** — scanner issues are plain data (`IssueKind`); new rules plug
  into `analyzeEntry` without touching the fixer.
- **Public API** — all business logic lives in importable packages with no UI
  coupling, so a desktop GUI, web UI, REST API or scripting front-end can be
  built against the same engine.

## Roadmap

- Wago/other provider plugins (the provider interface is ready; a new
  source is a new `catalog.Provider` implementation)
- Catalog screenshots (thumbnails of addon pages in the catalog browser)
- Plugin-architecture formalization (stable public interfaces so
  third-party scanner rules and providers can be loaded without forks)

## License

Proprietary — see [LICENSE](LICENSE). Copyright (c) 2026 Zendevve. All rights
reserved. Personal, non-commercial use within World of Warcraft is permitted;
modification, redistribution, and reuse of the source code require prior
written permission from the copyright holder.
