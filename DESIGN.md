# wowfix DESIGN — Framer design language (app-adapted)

> Brand reference: Framer's marketing system (framer.com) as documented 2026. This file is the
> cross-agent design contract for the wowfix v3 desktop app. Implement tokens exactly; adapt
> scale to a 900×600+ desktop window. The app is dark-only — there is no light mode.

## Tokens

```yaml
colors:
  primary: "#ffffff"            # pure white — primary CTA fill, display ink on canvas
  on-primary: "#0b0b0a"
  accent-blue: "#0099ff"        # THE single chromatic accent: links, focus rings, selection only
  accent-blue-strong: "#33adff"
  accent-blue-dim: "rgba(0,153,255,0.12)"
  canvas: "#0b0b0a"             # near-black with faint warmth — the page ground (hero, body, footer)
  surface-1: "#161615"          # one step up: cards, secondary pills, inputs, row hover
  surface-2: "#1f1f1e"          # two steps up: selected tabs, featured cards, button hover, dialogs
  inverse-canvas: "#ffffff"     # light side of dark-on-light CTAs
  hairline: "rgba(255,255,255,0.08)"
  hairline-strong: "rgba(255,255,255,0.14)"
  hairline-soft: "rgba(255,255,255,0.055)"
  border-field: "rgba(255,255,255,0.25)"
  ink: "#f7f7f6"                # all headline + emphasized body — pure-ish white
  ink-muted: "#999999"          # secondary type; hierarchy is ink → ink-muted, not weight ramps
  ink-faint: "#7f7f7f"          # metadata, versions, kickers (WCAG 4.5:1 on canvas/surface-1)
  ink-disabled: "rgba(153,153,153,0.55)"
  semantic-success: "#3fb950"
  semantic-warning: "#d29922"
  semantic-error: "#f85149"
  gradient-violet: "linear-gradient(135deg,#8b5cf6 0%,#6d3fd4 48%,#4c2e9e 100%)"
  gradient-magenta: "linear-gradient(135deg,#ff5ecf 0%,#e6339a 48%,#b01f72 100%)"
  gradient-orange: "linear-gradient(135deg,#ffb066 0%,#ff7a3c 52%,#ea551f 100%)"
  gradient-coral: "linear-gradient(135deg,#ff8a85 0%,#ff5f6d 52%,#eb3a55 100%)"

typography:
  display-family: "Mona Sans Variable"        # GT Walsheim substitute per Framer spec
  display: {size: 40px, weight: 500, line-height: 1.0, letter-spacing: -2.0px}     # -5% tracking
  title:    {size: 28px, weight: 500, line-height: 1.1, letter-spacing: -1.4px}   # -5%
  sub:      {size: 20px, weight: 500, line-height: 1.2, letter-spacing: -0.6px}   # -3%
  headline: {size: 17px, weight: 600, line-height: 1.25, letter-spacing: -0.4px}
  body:     {size: 15px, weight: 400, line-height: 1.35, letter-spacing: -0.15px} # ~-1%
  body-sm:  {size: 14px, weight: 500, line-height: 1.4, letter-spacing: -0.14px}
  caption:  {size: 13px, weight: 500, line-height: 1.2, letter-spacing: -0.13px}
  micro:    {size: 12px, weight: 400, line-height: 1.2, letter-spacing: -0.12px}
  button:   {size: 14px, weight: 500, line-height: 1.0, letter-spacing: -0.14px}
  body-family: "Inter Variable"               # with cv01,cv05,cv09,cv11,ss03,ss07,dlig (+tnum)

spacing:
  hair: 1px  xxs: 4px  xs: 8px  sm: 12px  md: 15px  lg: 20px  xl: 30px  xxl: 40px  section: 96px

rounded:
  xs: 4px  sm: 6px  md: 10px  lg: 15px  xl: 20px  xxl: 30px  pill: 100px  full: 9999px

elevation:
  0: "none"                                                   # canvas-mounted type, rows, footer
  1: "surface lift (surface-1 on canvas)"                     # cards, inputs, secondary pills
  2: "0 10px 30px rgba(0,0,0,.25), inset 0 0.5px 0 rgba(255,255,255,.10)"   # floating cards, dialogs
  3: "0 0 0 1px rgba(0,153,255,.15)"                          # selected option, focused input group
  focus: "0 0 0 2px var(--accent), 0 0 0 4px rgba(0,153,255,.35)"

components:
  button-primary:      "white pill on canvas; bg primary, text on-primary, type button, pad 10px 15px, rounded pill; pressed = scale shrink (not darken)"
  button-secondary:    "charcoal pill; bg surface-1, text ink, type button, pad 10px 15px, rounded pill"
  button-translucent:  "on busy/gradient grounds; bg surface-2, text ink, type button, pad 8px 14px, rounded xxl"
  button-icon-circular: "40px circle; bg surface-1, text ink, rounded full; 44px on touch"
  text-input:          "bg surface-1, text ink, type body, pad 10px 14px, rounded md; focused = elevation 3 ring, same surface"
  tab:                 "pill toggle; default bg canvas + ink-muted text; selected bg surface-2 + ink (selected = lift, not color)"
  card:                "bg surface-1, text ink, type body, rounded xl, pad 20px"
  card-featured:       "bg surface-2, otherwise card — hierarchy via one surface step, never a border"
  spotlight-card:      "gradient ground (violet/magenta/orange/coral), text ink, type sub, rounded xxl, pad 30px; CARDS only, ≤2 per viewport"
  row:                 "canvas ground, type body-sm, 1px hairline-soft underline; secondary cells ink-muted"
  sidebar:             "canvas, icon rail + labels; selected item surface-2 + elevation-3 ring; caption type"
  statusbar:           "canvas, caption type, ink-muted; sits at window bottom"
  dialog:              "surface-2, elevation 2, rounded xl, pad xl; focus ring on interactive rows"
  toast:               "surface-2, elevation 2, rounded lg, caption type"

breakpoints:
  desktop: 1199px  tablet: 810px  mobile: 809px
  # Desktop app window min 900×600 → effectively desktop+tablet; never design mobile-only paths.
```

## Typography rules

- Letter-spacing scales with size, hard: display/title pull ~-5%, body ~-1%.
- **OpenType variants are the brand voice**: body text MUST set
  `font-feature-settings: "cv01","cv05","cv09","cv11","ss03","ss07","dlig"`; numerics in tables use `"tnum"`.
- Weight stays in a narrow band: display 500, body 400, body-sm/caption/button 500. No 700/900 ramps.
- Tight line-heights everywhere (body 1.35 max); the tone is denser than typical SaaS.

## Surface & depth rules

- Two anchor surfaces: `primary` (white) and `canvas` (near-black). Every band picks one.
- Hierarchy on dark = surface lift (canvas → surface-1 → surface-2), NEVER opacity changes on white type.
- Text hierarchy is binary: `ink` or `ink-muted` (+`ink-faint` for metadata, `ink-disabled` for disabled). No mid-tone grays.
- `accent-blue` is a signal color: hyperlinks, focus rings, selected indicators. Never a background, never a button fill.
- Gradients are CARDS, not section backgrounds. One or two per long view; three reads as a moodboard.
- The dark canvas IS the whitespace: prefer long stretches of canvas with one oversized statement over divider furniture.

## Do / Don't

- DO pill CTAs (`rounded.pill`) for every primary action; secondary = charcoal pill; never bordered ghost buttons.
- DO keep display tracking negative by percentage even when scaling down ("reduce size, not tracking").
- DO monochrome + one blue + the gradient family. Never "blue, green, and red".
- DO surface-lift to mark the active/selected state (tabs, featured cards) instead of chromatic fills.
- DON'T ship light mode. DON'T square off CTAs. DON'T apply gradients to whole sections. DON'T use accent as a fill.

## App adaptations (Framer marketing spec → 900×600+ desktop window)

- Display tier is capped at 40px (marketing's 110px is a poster scale); keep the -5% tracking percentage.
- `top-nav` → `sidebar` (icon rail + labels, canvas, 56px-wide equivalent). `footer` → `statusbar`.
- Comparison-table / accordion collapse at 810px applies to wide tables (Updates list, Validate table): below 810px prefer per-item cards over horizontal scroll.
- Touch targets: pill buttons keep ≥40px height (44px on touch); icon buttons 40→44px.
- The Framer marketing gradient stops are pixel-derived, not token-exact — treat `gradient-*` as base anchors; keep gradient ORIENTATION constant across states.
- Card grids go 2-up on desktop → 1-up below 810px.

## Iteration guide

1. Reference components by token name (`{components.button-primary}`, `{components.spotlight-card}`).
2. Before adding a section, decide its surface (canvas vs surface-1 cards vs surface-2 featured) — that is the most consequential choice.
3. Default body to `{typography.body}` with the OpenType variants; `sub` only inside spotlight cards.
4. New states are new component entries (`-pressed`, `-featured`, `-selected`), never prose exceptions.
5. `accent-blue` is a single-shot signal: links, focus, selection — if a second blue appears, the brand is drifting.
