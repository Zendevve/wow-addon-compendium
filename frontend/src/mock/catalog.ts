// Mock fixtures for the catalog view: a multi-provider search dataset,
// curated set for the active profile family, addon info with a
// disambiguation case, per-provider install outcomes (including an honest
// per-addon failure to exercise the result panel), and a wago import.
// Owned methods: SearchCatalog, Curated, AddonInfo, InstallSource,
// InstallZip, SaveWagoImport, Sources. Everything else falls through to
// the Proxy defaults in mock/index.ts.

import type { MockData } from "./index";
import type { CatalogEntry, InfoResult } from "../types";

const CATALOG: CatalogEntry[] = [
  {
    provider: "github",
    name: "WeakAuras",
    author: "WeakAuras Team",
    summary: "Powerful and flexible aura framework. Build custom auras, icons and textures for anything the game tells you.",
    latest_version: "5.17.1",
    game_version: "retail",
    id: "WeakAuras/WeakAuras2",
    homepage: "https://github.com/WeakAuras/WeakAuras2",
  },
  {
    provider: "curseforge",
    name: "WeakAuras Companion",
    author: "WeakAuras Team",
    summary: "Desktop companion that keeps your WeakAuras in sync — download, manage and update aura packs.",
    latest_version: "5.0.5",
    game_version: "retail",
    id: "weakauras-companion",
    homepage: "https://www.curseforge.com/wow/addons/weakauras-companion",
  },
  {
    provider: "curseforge",
    name: "Details! Damage Meter",
    author: "Tercio",
    summary: "Combat analysis and damage/healing meters with a deep customization and window system.",
    latest_version: "3.6.9",
    game_version: "retail",
    id: "details-damage-meter",
    homepage: "https://www.curseforge.com/wow/addons/details-damage-meter",
  },
  {
    provider: "curseforge",
    name: "BigWigs",
    author: "BigWigs Team",
    summary: "Encounter boss mods with clean, configurable alerts for raids and dungeons.",
    latest_version: "11.1.5",
    game_version: "retail",
    id: "bigwigs",
    homepage: "https://www.curseforge.com/wow/addons/bigwigs",
  },
  {
    provider: "curseforge",
    name: "Deadly Boss Mods",
    author: "MysticalOS",
    summary: "Boss mods with timers, warnings and radar for every raid and dungeon.",
    latest_version: "11.1.5",
    game_version: "retail",
    id: "deadly-boss-mods",
    homepage: "https://www.curseforge.com/wow/addons/deadly-boss-mods",
  },
  {
    provider: "curseforge",
    name: "GTFO",
    author: "Froznone",
    summary: "Plays a loud warning when you are standing in fire, void zones or other avoidable damage.",
    latest_version: "1.9.7",
    game_version: "retail",
    id: "gtfo",
    homepage: "https://www.curseforge.com/wow/addons/gtfo",
  },
  {
    provider: "wago",
    name: "Plater Nameplates",
    author: "Plater",
    summary: "Highly customizable nameplates with a profile system — the community standard for tanking and raid frames.",
    latest_version: "2.0.0",
    game_version: "retail",
    id: "plater",
    homepage: "https://wago.io/addons/plater",
  },
  {
    provider: "wago",
    name: "WeakAuras: Healer Rows",
    author: "alienaar",
    summary: "Compact raid frame aura pack for healers: rows, cooldowns and buff tracking.",
    latest_version: "2026-08-01",
    game_version: "retail",
    id: "alienaar-healer-rows",
    homepage: "https://wago.io/healer-rows",
  },
  {
    provider: "tukui",
    name: "ElvUI",
    author: "Elv",
    summary: "All-in-one UI replacement: unit frames, bags, action bars and chat in one coherent theme.",
    latest_version: "13.88",
    game_version: "retail",
    id: "elvui",
    homepage: "https://www.tukui.org/addons.php?id=1",
  },
  {
    provider: "tukui",
    name: "TukUI",
    author: "Tukz",
    summary: "Minimalistic UI replacement — the lean ancestor of ElvUI, still maintained for classic flavors.",
    latest_version: "1.9.2",
    game_version: "vanilla",
    id: "tukui",
    homepage: "https://www.tukui.org/addons.php?id=6",
  },
  {
    provider: "wowinterface",
    name: "Questie",
    author: "QuestieDevs",
    summary: "Quest helper for WoW Classic: quest tracker, map icons and objective routing.",
    latest_version: "10.4.1",
    game_version: "vanilla",
    id: "questie",
    homepage: "https://www.wowinterface.com/downloads/info23304",
  },
  {
    provider: "wowinterface",
    name: "AtlasLoot",
    author: "AtlasLoot Team",
    summary: "Browse loot tables for every raid, dungeon and reputation — no instance required.",
    latest_version: "3.0.10",
    game_version: "vanilla",
    id: "atlasloot",
    homepage: "https://www.wowinterface.com/downloads/info17877",
  },
  {
    provider: "github",
    name: "pfQuest",
    author: "shagu",
    summary: "Quest database for vanilla and Turtle WoW, with the extended Turtle content built in.",
    latest_version: "1.9.10",
    game_version: "vanilla",
    id: "shagu/pfQuest",
    homepage: "https://github.com/shagu/pfQuest",
  },
  {
    provider: "github",
    name: "VanillaFixes",
    author: "hazzik",
    summary: "Client performance and quality-of-life fixes for the 1.12 game client.",
    latest_version: "2.1.3",
    game_version: "vanilla",
    id: "hazzik/VanillaFixes",
    homepage: "https://github.com/hazzik/VanillaFixes",
  },
  {
    provider: "github",
    name: "ShaguTweaks",
    author: "shagu",
    summary: "Quality-of-life tweaks for the vanilla UI: action bars, chat, bags and tooltips.",
    latest_version: "1.7.2",
    game_version: "vanilla",
    id: "shagu/ShaguTweaks",
    homepage: "https://github.com/shagu/ShaguTweaks",
  },
];

function fullInfo(c: CatalogEntry): InfoResult {
  const release_notes =
    c.id === "WeakAuras/WeakAuras2"
      ? "5.17.1 (2026-07-28)\n- Fix aura drag desync after combat\n- Custom triggers: better multi-state tab handling\n- Performance: halve icon update cost for hidden auras\n\n5.17.0\n- Options search across all tabs\n- Fix import strings failing on legacy escapes"
      : c.id === "shagu/pfQuest"
        ? "1.9.10 (2026-07-15)\n- Added the T2.5 quest chain to the database\n- Map icons scale with the in-game world map\n- Tooltip cache disabled for memory savings"
        : `Latest stable release of ${c.name}. See the homepage for the full changelog.`;
  return {
    provider: c.provider,
    id: c.id,
    name: c.name,
    author: c.author,
    summary: c.summary,
    latest_version: c.latest_version,
    homepage: c.homepage,
    game_version: c.game_version,
    updated_at: "2026-07-28T14:30:00Z",
    release_notes,
  };
}

/** Resolve a pasted source to a display name: exact catalog hit, else the
 * last path segment with archive suffixes stripped. */
function sourceName(s: string): string {
  const key = s.trim().toLowerCase();
  const hit = CATALOG.find(
    (c) => key === c.id.toLowerCase() || key === c.name.toLowerCase() || key.endsWith(c.id.toLowerCase()),
  );
  if (hit) return hit.name;
  const seg = key.split(/[\\/?#]/).filter(Boolean).pop() ?? key;
  return seg.replace(/\.zip$/i, "").replace(/[-_](main|master)$/i, "");
}

export const data: MockData = {
  Sources: () => [
    {
      name: "GitHub",
      description:
        "GitHub Releases — tags are matched with a semver-aware comparator. No API key needed; unauthenticated rate limits apply.",
    },
    {
      name: "CurseForge",
      description:
        "CurseForge API — requires an API key in Settings; free keys are rate-limited. Outages are reported here, never silently skipped.",
    },
    {
      name: "WowInterface",
      description:
        "WowInterface file downloads — no API key, but the site rate-limits automated clients and is occasionally unreachable.",
    },
    {
      name: "Tukui",
      description:
        "Tukui — small stable API hosting ElvUI and TukUI. Unaffected by CurseForge outages.",
    },
    {
      name: "Wago",
      description:
        "Wago — import-string addons (WeakAuras, Plater profiles). Saving an import needs no account.",
    },
  ],

  SearchCatalog: (query?: string) => {
    const q = (query ?? "").trim().toLowerCase();
    if (!q) return { results: [], errors: [] };
    const results = CATALOG.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.author.toLowerCase().includes(q) ||
        c.summary.toLowerCase().includes(q) ||
        c.id.toLowerCase().includes(q),
    );
    // Provider outages are data, not silence: the search error box renders
    // these per-provider messages (LEARNINGS #6). Trigger via a demo query.
    const errors: string[] = [];
    if (q.includes("offline")) {
      errors.push(
        "WowInterface is unreachable (timeout after 10s). Results from other providers are unaffected.",
      );
      errors.push("CurseForge API key exhausted for this hour — results may be incomplete.");
    } else if (q.includes("curse")) {
      errors.push("CurseForge API returned 429 (rate limit). Other providers are unaffected.");
    }
    return { results, errors };
  },

  Curated: () => ({
    family: "vanilla",
    label: "Turtle WoW",
    profile_id: "turtle-wow",
    addons: [
      {
        name: "pfQuest",
        source: "shagu/pfQuest",
        summary: "Quest database with the extended Turtle WoW content — the server's quest helper.",
        homepage: "https://github.com/shagu/pfQuest",
        installed: true,
        installed_version: "1.9.9",
      },
      {
        name: "VanillaFixes",
        source: "hazzik/VanillaFixes",
        summary: "Client performance and quality-of-life fixes for the 1.12 game client.",
        homepage: "https://github.com/hazzik/VanillaFixes",
        installed: false,
      },
      {
        name: "AtlasLoot",
        source: "atlasloot",
        summary: "Browse loot tables for every raid, dungeon and reputation — no instance needed.",
        homepage: "https://www.wowinterface.com/downloads/info17877",
        installed: false,
      },
      {
        name: "ShaguTweaks",
        source: "shagu/ShaguTweaks",
        summary: "Quality-of-life tweaks for the vanilla UI: action bars, chat, bags and tooltips.",
        homepage: "https://github.com/shagu/ShaguTweaks",
        installed: false,
      },
    ],
  }),

  AddonInfo: (arg?: string) => {
    const key = (arg ?? "").trim().toLowerCase();
    if (!key) throw new Error("Nothing to look up — enter an addon id or name.");
    const entry = CATALOG.find((c) => c.id.toLowerCase() === key);
    if (entry) return fullInfo(entry);
    // Bare ambiguous names return matches — the view renders the picker.
    if (key === "weakauras" || key === "wa") {
      return {
        provider: "",
        id: "",
        name: "",
        author: "",
        summary: "",
        latest_version: "",
        homepage: "",
        game_version: "",
        updated_at: "",
        matches: CATALOG.filter((c) => c.name.toLowerCase().startsWith("weakauras")),
      };
    }
    throw new Error(`No addon found for “${arg}”. Try the exact provider id from a search result.`);
  },

  InstallSource: (source?: string, allowReplace?: boolean) => {
    const s = (source ?? "").trim().toLowerCase();
    // Honest per-addon failure: one row fails while the rest succeed.
    if (s.includes("deadly") || s.includes("dbm")) {
      return {
        installed: ["WeakAuras"],
        replaced: [],
        skipped: [],
        errors: [
          "Deadly Boss Mods: latest release asset missing (provider returned 404) — install the CurseForge build instead.",
        ],
      };
    }
    if (s.includes("weakauras")) return { installed: ["WeakAuras"], replaced: [], skipped: [], errors: [] };
    if (s.includes("vanillafixes")) return { installed: ["VanillaFixes"], replaced: [], skipped: [], errors: [] };
    if (s.includes("atlasloot")) return { installed: [], replaced: ["AtlasLoot"], skipped: [], errors: [] };
    if (s.includes("questie")) return { installed: [], replaced: [], skipped: ["Questie"], errors: [] };
    return { installed: [sourceName(s)], replaced: [], skipped: [], errors: [] };
  },

  InstallZip: (zipPath?: string) => {
    const name = sourceName(zipPath ?? "");
    return { installed: [name], replaced: [], skipped: [], errors: [] };
  },

  SaveWagoImport: (id?: string) => {
    const key = (id ?? "").trim();
    const isPlater = key.toLowerCase().includes("plater") || key.toLowerCase().includes("quazii");
    return {
      path: `WTF/Account/EXAMPLE/SavedVariables/${isPlater ? "Plater" : "WeakAuras"}.lua`,
      name: isPlater ? "Plater Profile: Quazii" : `WeakAuras: ${key}`,
      bytes: 48213,
      applied_hint: "Saved to the file above — import it in-game via WeakAuras → Import.",
    };
  },
};
