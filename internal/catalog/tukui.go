package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const tukuiAPI = "https://www.tukui.org/api.php"

// tukuiUIs are the addons exposed by the Tukui API.
const tukuiUIs = "elvui|tukui|benikui|kaitheme"

// tukuiProvider serves ElvUI, Tukui, BenikUI and KuiNameplates from
// the documented Tukui JSON API.
type tukuiProvider struct {
	client *http.Client
	api    string // API URL, overridable in tests
}

func newTukuiProvider(client *http.Client, apiURL string) *tukuiProvider {
	if apiURL == "" {
		apiURL = tukuiAPI
	}
	return &tukuiProvider{client: client, api: apiURL}
}

func (p *tukuiProvider) Name() string { return ProviderTukui }

// tukuiEntry mirrors one Tukui API entry.
type tukuiEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Tagline      string `json:"tagline"`
	Version      string `json:"version"`
	Changelog    string `json:"changelog"`
	URL          string `json:"url"`
	WebURL       string `json:"web_url"`
	DownloadPath string `json:"download_path"`
	Author       string `json:"author"`
	LastUpdate   string `json:"lastupdate"`
}

// fetchAll returns every Tukui addon. The combined multi-UI query is
// tried first; when the API rejects it, each UI is fetched separately.
// The response can be an array or a single object, both are handled.
func (p *tukuiProvider) fetchAll(ctx context.Context) ([]tukuiEntry, error) {
	body, err := get(ctx, p.client, p.api+"?ui="+tukuiUIs)
	if err != nil {
		return p.fetchEach(ctx)
	}
	var single tukuiEntry
	if err := json.Unmarshal(body, &single); err == nil && single.ID != "" {
		return []tukuiEntry{single}, nil
	}
	var list []tukuiEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("tukui: parse response: %w", err)
	}
	return list, nil
}

func (p *tukuiProvider) fetchEach(ctx context.Context) ([]tukuiEntry, error) {
	var out []tukuiEntry
	for _, name := range []string{"elvui", "tukui", "benikui", "kaitheme"} {
		body, err := get(ctx, p.client, p.api+"?ui="+name)
		if err != nil {
			return nil, fmt.Errorf("tukui: %s: %w", name, err)
		}
		var e tukuiEntry
		if err := json.Unmarshal(body, &e); err != nil {
			return nil, fmt.Errorf("tukui: parse %s: %w", name, err)
		}
		if e.ID == "" {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Search filters the small Tukui list client-side.
func (p *tukuiProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	list, err := p.fetchAll(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []*Addon
	for i := range list {
		e := &list[i]
		if q != "" &&
			!strings.Contains(strings.ToLower(e.Name), q) &&
			!strings.Contains(strings.ToLower(e.Tagline), q) {
			continue
		}
		out = append(out, p.addonFromEntry(e))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Resolve fetches one addon by its id or name.
func (p *tukuiProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	list, err := p.fetchAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id || strings.EqualFold(list[i].Name, id) {
			return p.addonFromEntry(&list[i]), nil
		}
	}
	return nil, fmt.Errorf("tukui: no addon %q", id)
}

// Latest refreshes version information from the API.
func (p *tukuiProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	if addon == nil || addon.ID == "" {
		return nil, fmt.Errorf("tukui: missing addon id")
	}
	return p.Resolve(ctx, addon.ID)
}

// Download fetches the addon archive from download_path, falling back
// to the entry url.
func (p *tukuiProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	if addon == nil || addon.downloadURL == "" {
		return fmt.Errorf("tukui: no download URL for %q", addonName(addon))
	}
	return downloadFile(ctx, p.client, addon.downloadURL, dest, progress)
}

func (p *tukuiProvider) addonFromEntry(e *tukuiEntry) *Addon {
	a := &Addon{
		Provider:      ProviderTukui,
		ID:            e.ID,
		Name:          e.Name,
		Author:        e.Author,
		Summary:       e.Tagline,
		LatestVersion: e.Version,
		Homepage:      e.WebURL,
		UpdatedAt:     parseTukuiTime(e.LastUpdate),
	}
	if a.Homepage == "" {
		a.Homepage = e.URL
	}
	if e.DownloadPath != "" {
		a.downloadURL = e.DownloadPath
	} else {
		a.downloadURL = e.URL
	}
	return a
}

// parseTukuiTime handles the API's unix timestamps and, as a
// fallback, ISO-like date strings.
func parseTukuiTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(n, 0)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
