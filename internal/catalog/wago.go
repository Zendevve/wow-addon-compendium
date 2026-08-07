package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// wagoBase is the Wago data API host, the same one the official
// WeakAuras Companion talks to. None of the endpoints used here
// require an API key (the Companion sends an empty "api-key" header
// and an optional account "Identifier" hash, both of which are
// ignored when absent).
const wagoBase = "https://data.wago.io"

// wagoProvider serves WeakAuras and Plater imports from wago.io.
// The API is undocumented but stable: it is the production channel of
// the open-source WeakAuras Companion, which has consumed these
// endpoints for years. A "download" of an import is not an addon
// archive; it is the raw encoded import string, which must be applied
// in-game (see Download).
//
// Caveats: no documented rate limits — keep calls sparse, mirroring
// the Companion's once-per-hour check cadence. Wago-hosted addon
// archives are NOT covered: addons.wago.io has no public download API
// (only an upload API behind a Bearer key and expiring signed URLs
// generated for the Wago App), so this provider serves imports only.
type wagoProvider struct {
	client *http.Client
	base   string // API base, overridable in tests
}

func newWagoProvider(client *http.Client, base string) *wagoProvider {
	if base == "" {
		base = wagoBase
	}
	return &wagoProvider{client: client, base: base}
}

func (p *wagoProvider) Name() string { return ProviderWago }

// wagoSearchHit mirrors one hit of the Elasticsearch-backed search.
// The flavor is only exposed as a numeric internal index, so
// GameVersion stays empty for search results.
type wagoSearchHit struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"descriptionSanitized"`
	Author      string `json:"userName"`
	Version     string `json:"versionString"`
	Type        string `json:"type"`
	Timestamp   int64  `json:"timestamp"`
}

type wagoSearchResponse struct {
	Hits []wagoSearchHit `json:"hits"`
	// Total is checked to distinguish a legitimate no-match (200 with
	// an empty hit list) from a silently mis-parsed response.
	Total int `json:"total"`
}

// wagoCheck mirrors one /api/check entry. The game field is Wago's
// internal flavor key (see wagoGameFamily).
type wagoCheck struct {
	ID         string `json:"_id"`
	Name       string `json:"name"`
	Author     string `json:"username"`
	Game       string `json:"game"`
	Type       string `json:"type"`
	Version    int    `json:"version"`
	VersionStr string `json:"versionString"`
	URL        string `json:"url"`
	Modified   string `json:"modified"`
}

// Search looks imports up on the Wago search API. The extra
// parameters the site sends (mode, game, expansion, type, sort,
// domain) are accepted for fidelity but ignored server-side.
func (p *wagoProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("mode", "imports")
	q.Set("game", "wow")
	q.Set("expansion", "")
	q.Set("type", "all")
	q.Set("page", "0")
	q.Set("sort", "bestmatchv3")
	q.Set("domain", "0")
	u := p.base + "/search/es?" + q.Encode()

	body, err := get(ctx, p.client, u)
	if err != nil {
		return nil, err
	}
	var resp wagoSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("wago: parse search response: %w", err)
	}
	if resp.Total == 0 && len(resp.Hits) == 0 {
		return nil, nil
	}
	out := make([]*Addon, 0, len(resp.Hits))
	for i := range resp.Hits {
		out = append(out, p.addonFromSearch(&resp.Hits[i]))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Resolve fetches one import by its wago id (the 8-character slug).
func (p *wagoProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	e, err := p.check(ctx, id)
	if err != nil {
		return nil, err
	}
	return p.addonFromCheck(e), nil
}

// Latest refreshes version information for one import.
func (p *wagoProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	if addon == nil || addon.ID == "" {
		return nil, fmt.Errorf("wago: missing import id")
	}
	return p.Resolve(ctx, addon.ID)
}

// Download fetches the import string and writes it to dest as a
// plain-text file (the "archive" for WeakAuras and Plater imports).
// This is NOT an addon archive: the file must be imported in-game
// (WeakAuras / Plater import panel) rather than unzipped into
// Interface/AddOns. The companion-style alternative — writing a Lua
// data file into Interface/AddOns/WeakAurasCompanion for in-game
// pickup — requires parsing and re-serializing Lua and is deliberately
// not implemented; writing WTF SavedVariables is even more fragile.
func (p *wagoProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	if addon == nil || addon.ID == "" {
		return fmt.Errorf("wago: missing import id")
	}
	// The endpoint 302-redirects to itself with the resolved version
	// query parameter; the default client follows it.
	u := p.base + "/api/raw/encoded?id=" + url.QueryEscape(addon.ID)
	body, err := get(ctx, p.client, u)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if progress != nil {
		progress(0, int64(len(body)))
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return err
	}
	if progress != nil {
		progress(int64(len(body)), int64(len(body)))
	}
	return nil
}

// check fetches one /api/check entry by id. A non-existent id yields
// an empty list from the API, which surfaces as an error here.
func (p *wagoProvider) check(ctx context.Context, id string) (*wagoCheck, error) {
	u := p.base + "/api/check/?ids=" + url.QueryEscape(id)
	body, err := get(ctx, p.client, u)
	if err != nil {
		return nil, err
	}
	var list []wagoCheck
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("wago: parse check response: %w", err)
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("wago: no import %q", id)
}

func (p *wagoProvider) addonFromSearch(h *wagoSearchHit) *Addon {
	a := &Addon{
		Provider:      ProviderWago,
		ID:            h.ID,
		Name:          h.Name,
		Author:        h.Author,
		Summary:       h.Description,
		LatestVersion: h.Version,
		Homepage:      "https://wago.io/" + h.ID,
	}
	if h.Timestamp > 0 {
		a.UpdatedAt = time.Unix(h.Timestamp, 0)
	}
	return a
}

func (p *wagoProvider) addonFromCheck(e *wagoCheck) *Addon {
	a := &Addon{
		Provider:      ProviderWago,
		ID:            e.ID,
		Name:          e.Name,
		Author:        e.Author,
		LatestVersion: e.VersionStr,
		Homepage:      e.URL,
		GameVersion:   wagoGameFamily(e.Game),
	}
	if a.Homepage == "" {
		a.Homepage = "https://wago.io/" + e.ID
	}
	if t, err := time.Parse(time.RFC3339, e.Modified); err == nil {
		a.UpdatedAt = t
	}
	return a
}

// wagoGameFamily maps Wago's internal game keys (the "game" field of
// /api/check) onto the catalog's canonical client families. Modern
// expansions all map to retail; classic-era keys map to their family.
// Unknown keys map to "".
func wagoGameFamily(game string) string {
	switch strings.ToLower(strings.TrimSpace(game)) {
	case "classic", "sod", "hardcore", "turtle":
		return "vanilla"
	case "titan-wotlk", "wrath":
		return "wrath"
	case "tbc":
		return "tbc"
	case "cata":
		return "cata"
	case "mop", "tww", "midnight", "df", "sl", "bfa", "legion", "wod",
		"retail", "dragonflight", "shadowlands":
		return "retail"
	default:
		return ""
	}
}
