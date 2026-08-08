# wowfix v3 — LEARNINGS: the 30-project field, and what we do about it

Source: `refs/competitors/study/` (gitignored corpus — 25 notes + synthesis `study/README.md`),
compiled 2026-08-08 from the complete CSV field: every row cloned (or recovered via re-uploads)
and read; closed apps studied from their public surfaces; deleted repos documented with their
provenance. This file is the tracked distillation: what the field taught us and which of it lands
in the v3 ground-up rebuild (vs the follow-up roadmap).

## The field, in one paragraph

Of 30 tracked tools, exactly **three are alive**: instawow (Python CLI, the most complete),
CurseBreaker (Python TUI, the most opinionated), wowman (Clojure GUI, the most elaborate state
model — and doesn't target Windows). Everything else is dead, frozen, or deleted: ~90% of the
field. The dominant death cause is **HTML scraping as the primary provider path** — ~20 tools
scraped CurseForge/WoWInterface/Tukui markup and died when the markup changed (wttw, vargen2,
grrttedwards, kuhnertdm's magic byte offsets, wowam's dead mods.curse regex, braier's rotted
XPath, lcurse's cfscrape, worldofaddons' jsdom, saionaro's CSS-class scrapes, classicaddonmanager's
dead forgesvc). The second killer is **string-equality update detection** with no ordering or
drift concept. The third is **error handling and safety**: batch aborts, swallowed errors,
result-blind success UX, `rmtree`-then-copy installs with no backup, unconditional `extractall`,
permanent deletes, and one crash-on-one-bad-repo (`async void` rethrow in OpenAddOnManager).
The survivors share a shape: typed per-addon results, provider adapters against stable APIs,
caching instead of rate limiting, backup/trash before any mutation.

## What wowfix already has (carry as-is — "the go things we did")

The study repeatedly confirms the core we built is ahead of the field:

- **Scan/repair engine** — no competitor repairs broken addon installs (only cousins: wttw's
  bootstrap warnings, GitAddonsManager's reset/reclone).
- **Safety model** — OS-trash + `Backups/` snapshots before every mutation + TOC validation
  before install; the field's #1 weakness is exactly what we never regress. WoWAceUpdater
  (2007) had our Recycle-Bin delete and backup-before-update rotation, then the field forgot.
- **Semver-aware comparator** (`internal/catalog/semver.go`) — the field runs on `!=` and
  date comparisons; we parse and order.
- **Offline snapshot export/check** — exists nowhere else (wowman's HTTP cache is the closest).
- **Multi-provider isolation** (CurseForge/GitHub/WoWInterface/Tukui/Wago, all API-based, no
  scraping) — the direct response to the single-provider deaths (wowaceupdater's one RSS base,
  classicaddonmanager's one forgesvc key-wall).
- **TOC validation across 9 game versions**, Collections, SavedVariables as first-class data
  (OpenAddOnManager only deletes them on uninstall; instawow only reads WeakAuras').

## Adoptions for v3 (UI-visible now — shapes the frontend)

These come from the study and land in the ground-up frontend this rebuild:

1. **Bulk pre-check, then cache-only loop** (CurseBreaker): one provider call per batch
   populates caches; the per-addon loop never re-hits the network. The Updates view design:
   check-all → review list → apply, with an end-of-batch error dump.
2. **Status-surfacing family** (Minion/OpenAddOnManager/lcurse/saionaro):
   - badge-counted update-all affordance (OpenAddOnManager FAB, Minion red badges);
   - needs-update rows sorted first; updates group on top;
   - side-by-side old → new version + in-app changelog (Minion's benchmark since 2014);
   - per-addon status chips keyed to the right addon (saionaro) — Minion 3.0.0's
     "progress bars flash on other addons" bug is the warning;
   - per-addon row status coloring (lcurse yellow/red/white).
3. **Update-review flow** (Minion): updates shown with previous+new versions, clickable
   changelog, per-addon ignore with a visible stop icon — the commercial UX to match.
4. **Reactive, zero-manual-refresh list rendering** (OpenAddOnManager's live queries):
   derived counts and badges, no manual refresh buttons.
5. **Honest per-addon failure surfacing**: a failed addon is data — row-level error state,
   never a batch abort, never a blanket toast (saionaro's "All Addons Updated." when
   everything failed is the anti-pattern).
6. **Provider outage surfacing, not suppression** (classicaddonmanager's silent CF suppression
   is the anti-pattern): catalog shows per-provider status with honest caveats.

## Adoptions — Go core roadmap (after v3; tracked here so they aren't lost)

| # | Adoption | Source | Effort |
|---|---|---|---|
| R1 | Per-addon result algebra formalized (typed error variants per catalog/update op; dummy resolver for disabled sources) | instawow | M |
| R2 | Game-track cascade for flavor matching (release-name regex → date → diff-fill → release.json → remote TOC), data-driven | wowman | L |
| R3 | GitHub partial-zip TOC sniffing + release.json fast path | instawow | M |
| R4 | Match cascade `[source id] > [source name] > [name] > [label] > [dirname]` + TOC-as-identity (`## X-Curse-Project-ID` etc.) | wowman, wowutils, CurseBreaker | M |
| R5 | Per-addon `.source` provenance file beside the install; registry = cache, provenance travels; record owner+URL so a deleted repo is re-derivable (manual re-uploads survived every deletion we found) | WoWAceUpdater, unreachable-repos study | S |
| R6 | Stale-file diff on update (old member inventory vs new zip — delete files the new version no longer ships) | WoWAceUpdater `ziplist.wau` | S |
| R7 | Version-log + backup rotation + "undo last update" | instawow `pkg_version_log`, WoWAceUpdater 10-gen rotation | M |
| R8 | SavedVariables tracked at install time (TOC `SavedVariables`/`SavedVariablesPerCharacter`); wire into remove | OpenAddOnManager | S |
| R9 | Snapshot catalog (bundled artifact, offline Catalog tab) + author-site-direct preference (WowMatrix hybrid) | wowman/vargen2/wttw, WowMatrix | L |
| R10 | Git-branch tracking as an optional exact-update mode (DB row: remote+branch; fetch + ahead/behind; force checkout via trash/backup first) | GitAddonsManager | L |
| R11 | Hash-based addon import (TOC include-graph + md5s) for first-run matching | CurseBreaker | M |
| R12 | `wowfix://` deep-link registration | CurseBreaker, wowutils (2015!) | S |
| R13 | License gate at install (read LICENSE, agree once, persist) | OpenAddOnManager | S |
| R14 | Fixture-based provider tests (committed fixture zips + static HTML) | grrttedwards, qwezarty | S |
| R15 | Archive cache: hash + dedupe downloaded zips | braier, wowutils | S |

## What to avoid (the consolidated anti-pattern list)

- Scraping as a primary path; magic offsets, CSS/XPath literals, Cloudflare bypasses, fake UAs.
- String-equality / date-only updates with no ordering; lexicographic `>` on versions (wowam
  misorders 9.x vs 10.x); opaque provider tokens without a parsed fallback.
- Batch aborts, swallowed errors, result-blind success toasts, crash-on-one-bad-item.
- `rmtree`-then-copy, delete-before-extract, unconditional `extractall`, no zip-slip guard,
  permanent deletes, escapable path guards, force checkout without trash.
- Dead providers/stubs left in tree; hardcoded flavor/exe enums (every expansion needed a new
  binary); single-provider coupling; silent provider suppression.
- Live-network tests; config/registry inside the game dir; grid-as-database state
  (braier's XML grid, classicaddonmanager's whole-object GSON, worldofaddons' wholesale
  JSON rewrites); no tests at all (most of the corpus).
- Hardcoded secrets in build automation (wowam's signtool password); embedded ads
  (wowaceupdater's ads.html, Minion's removed 2018).

## Position

The defensible story: **the only maintained multi-provider tool that both repairs AND manages**,
with honest offline snapshot semantics — against a field that is ~90% dead or frozen, whose
survivors are a Python CLI, a Python TUI, and a Clojure GUI that doesn't target Windows.
The competitors converge on "download latest, compare strings, hope"; our
Validation/Doctor/snapshot axis is unclaimed territory. v3 keeps that core and rebuilds the
surface to the commercial benchmark (Minion's update-review UX) on a dark, Framer-grade design
language (DESIGN.md), GUI-only.
