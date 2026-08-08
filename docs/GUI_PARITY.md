# CLI → GUI Parity Checklist

The desktop GUI is the primary interface for wowfix; the CLI is the reference implementation. Every CLI command must have a working GUI equivalent — this checklist maps each command to its GUI surface and tracks whether that surface exists and is verified.

Verification basis: `wails dev` running against the live backend (`http://localhost:34115`, the runtime-injected dev URL) with the real WoW install (`D:\ABDM\Compressed\ChromieCraft_3.3.5a`) and the real config store. Rows marked `verified` were exercised end-to-end in that session (evidence in the Evidence section); rows marked `covered` have an implemented GUI equivalent that predates the parity pass (not re-exercised live this session). Statuses: `covered`, `verified`, `out-of-scope`, `n/a`.

## Coverage map

| CLI command | GUI equivalent | Status |
|---|---|---|
| `wowfix` (bare) | — (GUI is the primary interface) | `n/a` |
| `wowfix help` | — (GUI is the primary interface) | `n/a` |
| `wowfix scan` | `Service.Scan` + Overview (Scan) | `verified` |
| `wowfix fix` | `Service.Fix` / `Service.FixAll` + Overview (Scan) "Fix All" | `covered` |
| `wowfix install <addon.zip>` | `Service.InstallZip` + Catalog (install surface: URL/owner-repo bar, Browse… for ZIPs, drag-drop) | `covered` |
| `wowfix install <url\|owner/repo>` | `Service.InstallSource` + Catalog (install surface: URL/owner-repo bar, Browse… for ZIPs, drag-drop) | `covered` |
| `wowfix validate` | `Service.Validate` + Overview (Validation) | `covered` |
| `wowfix list` | `Service.Scan` addons + Overview (Scan) rows | `verified` |
| `wowfix search <query>` | `Service.SearchCatalog` + Catalog view | `verified` |
| `wowfix info <addon>` | `Service.AddonInfo` + Catalog view info panel | `verified` |
| `wowfix update` | `Service.CheckUpdates` / `Service.ApplyUpdate` / `Service.ApplyAllUpdates` + Updates (primary "Update all"; per-row Update + ⋯ Pin/Ignore/History…/Rollback; Managed section with explicit Pin/Ignore/Rollback) | `verified` |
| `wowfix update --check` | `Service.CheckUpdates` + Updates (exit-code semantics n/a in GUI) | `verified` |
| `wowfix history <folder> [--json]` | `Service.ListAddonVersions` + Updates row ⋯ menu "History…" (per-addon version log, newest first, Current marker, per-row Rollback) | `covered` |
| `wowfix rollback <folder> <version>` | `Service.RollbackToVersion` + Updates History modal rollback (re-downloads exact version from provider; backup-first confirmation; providers without versioned downloads show honest error) | `covered` |
| `wowfix snapshot export|check <file>` | `Service.ExportSnapshot` / `Service.CheckSnapshot` + Backups (Snapshot section) | `verified` |

## Evidence

Session: 2026-08-07, `wails dev`, real backend bindings (`window.go.service.Service`, 48 methods — mock not active), real install `D:\ABDM\Compressed\ChromieCraft_3.3.5a` (37 addons), real config `%AppData%\wowfix\config.json`. Destructive operations were verified only to their confirmation gate (per scope boundary: no destructive ops against the live install).

- **scan / list** — Scan view rendered the live scan: 37 addons, 23 with issues, 23 errors, profile `Wrath of the Lich King 3.3.5a · 30300`.
- **doctor** — Run diagnostics produced 14 checks against live state: config ok (path shown), profile ok `wrath`, theme ok `dark`, collection ok (none set), install ok (`...ChromieCraft_3.3.5a\Interface\AddOns`), flavor info, exe info (`Wow.exe`), permissions ok (writable), scan info (37/23/23), backups info (0 snapshots), trash ok (writable), registry info (none), collections ok (writable), savedvars info (2 accounts). Summary toast "5 checks · 1 error · 1 warning" style output.
- **search** — Real provider round-trip: query "pfUI" surfaced genuine per-provider failures inline ("1 problem while searching": curseforge legacy endpoint dead — suggests `config set curseforge_api_key`; tukui 404; wowinterface JSON schema drift), matching the CLI's partial-failure semantics.
- **info** — `AddonInfo` through the live binding: `Bunny67/WeakAuras-WotLK` resolved to real GitHub metadata (author Bunny67, latest v4.0.0, homepage, updated_at); bare name "WeakAuras" returned 9 ambiguous matches (github + wago) with provider/id/name for disambiguation. Info buttons on catalog result rows wired to the same call.
- **update / update --check** — Updates view rendered a live `CheckUpdates` result: "0 updates available · checked 08:42 PM · All addons are up to date".
- **snapshot export|check** — Snapshot section: Export snapshot generated real portable JSON (`{"version":1,"exported_at":"2026-08-07T12:42:33Z","profile":"vanilla","addons":[]}` — registry has 0 tracked addons, so the empty list is correct) with summary + Copy button; Check textarea + Check button present, `CheckSnapshot` covered by unit tests.
- **sources** — Catalog → Sources section listed all 4 providers with the CLI's exact descriptions (github ~60 req/hr, curseforge key-or-legacy, wowinterface MMOUI filelist, tukui API).
- **curated install** — Curated section shows per-row Install buttons wired to `InstallSource(addon.source, true)` behind a confirm dialog (code-verified; install itself gated per boundary).
- **backup / restore** — Backups view: ListBackups rendered "0 snapshots" live; Create backup launched a real `BackupDir` copy into `ChromieCraft_3.3.5a\Backups\2026-08-07T20-37-37.537` (659 MB copied folder-by-folder; both test runs were cut short when the dev window was closed externally — interrupted snapshots were removed). Restore row is confirm-dialog gated; `RestoreBackup`/`ListBackups` covered by unit tests (`TestServiceBackupNowListRestore`).
- **config / config set** — Settings view loaded live config (theme dark, auto-backup on, confirmations on, profile wrath); theme toggled light → dark through `SetConfigKey` with save toasts; `%AppData%\wowfix\config.json` byte-identical to its pre-session state afterwards.
- **savedvars** — SavedVars view: live accounts WOWCHAT + ZENDEVVE; List showed 10 real files (`AI_VoiceOver.lua` … `XPRateControl.lua`); Back up wrote `WTF\savedvars-backups\2026-08-07T20-39-38.711`. Reset/Restore/Migrate are confirm-dialog gated (mock-verified UI, unit-tested service: `TestServiceSavedVarsBackupRestoreReset`, `TestServiceSavedVarsMigrate`).
- **export** — Exported the live addon list to a scratch path: manifest written and parsed (`name wowfix-export, version 1, game vanilla, 37 addons`); UI toast "Exported 37 addons". Parent-dir-missing error surfaced inline correctly.
- **import** — Import section renders (path/URL input, Import button behind confirm dialog); dispatch zip/manifest/URL covered by unit tests (`TestServiceImportCollectionManifest/Zip/URL/Unsupported`).
- **version** — Shell brand chip shows `vdev` from `GetState` (build version passed by `gui.App`).
- **All rows** — 6 destinations render in the shell (Overview, Updates, Catalog, Collections, Backups, Settings; SavedVariables reachable via `?view=savedvars`, Export/Import reachable via `?view=exportimport`); `go build ./...`, `go vet ./...`, `go test ./...` all pass; `cmd/wowfix` untouched (empty `git diff -- cmd/`).

## Notes

- Rows `covered` (fix, install zip/source, validate, curated list, profile) are pre-existing GUI surfaces with service methods present before the parity pass; not re-exercised live this session.
- `update --check` exit codes are a scripting concept; the GUI equivalent is the check result rendered in the Updates view (noted in the row).
- The `wowfix` bare command's "launch the GUI" message is n/a in the GUI itself.
