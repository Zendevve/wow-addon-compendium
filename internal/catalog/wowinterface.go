package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const mmouiFilelistURL = "https://api.mmoui.com/v3/game/WOW/filelist.json"

// wowInterfaceProvider serves addons from the MMOUI filelist, the
// public JSON index behind WowInterface. The list is a large file, so
// it is fetched at most once per provider instance and cached in
// memory.
type wowInterfaceProvider struct {
	client *http.Client
	url    string // filelist URL, overridable in tests

	mu      sync.Mutex
	loaded  bool
	entries []mmouiEntry
}

func newWowInterfaceProvider(client *http.Client, filelistURL string) *wowInterfaceProvider {
	if filelistURL == "" {
		filelistURL = mmouiFilelistURL
	}
	return &wowInterfaceProvider{client: client, url: filelistURL}
}

func (p *wowInterfaceProvider) Name() string { return ProviderWowInterface }

// mmouiEntry mirrors one MMOUI filelist entry. The filelist schema is
// fixed by the MMOUI v3 API; unknown fields are ignored.
type mmouiEntry struct {
	UID       int    `json:"uid"`
	Name      string `json:"name"`
	Label     string `json:"label"`
	Changelog string `json:"changelog"`
	Category  int    `json:"category"`
	URL       string `json:"url"`
	NID       int    `json:"nid"`
	Author    string `json:"author"`
	Version   string `json:"version"`
	Date      string `json:"date"`
}

// load fetches and parses the filelist once, caching the result.
func (p *wowInterfaceProvider) load(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wowfix")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &statusError{code: resp.StatusCode, url: p.url, msg: resp.Status}
	}
	// The filelist is several megabytes; decode directly from the
	// body instead of buffering it.
	var list []mmouiEntry
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return err
	}
	p.entries = list
	p.loaded = true
	return nil
}

// Search filters the cached filelist client-side by label or slug.
func (p *wowInterfaceProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	if err := p.load(ctx); err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []*Addon
	for i := range p.entries {
		e := &p.entries[i]
		if q != "" &&
			!strings.Contains(strings.ToLower(e.Label), q) &&
			!strings.Contains(strings.ToLower(e.Name), q) {
			continue
		}
		out = append(out, p.addonFromEntry(e))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Resolve fetches one addon by its numeric uid; non-numeric ids are
// matched against the entry slug.
func (p *wowInterfaceProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	if err := p.load(ctx); err != nil {
		return nil, err
	}
	if uid, err := strconv.Atoi(id); err == nil {
		for i := range p.entries {
			if p.entries[i].UID == uid {
				return p.addonFromEntry(&p.entries[i]), nil
			}
		}
		return nil, fmt.Errorf("wowinterface: no addon with id %q", id)
	}
	for i := range p.entries {
		if strings.EqualFold(p.entries[i].Name, id) {
			return p.addonFromEntry(&p.entries[i]), nil
		}
	}
	return nil, fmt.Errorf("wowinterface: no addon %q", id)
}

// Latest refreshes version information from the cached filelist.
func (p *wowInterfaceProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	if addon == nil || addon.ID == "" {
		return nil, fmt.Errorf("wowinterface: missing addon id")
	}
	return p.Resolve(ctx, addon.ID)
}

// Download fetches the archive URL the filelist entry points at.
func (p *wowInterfaceProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	if addon == nil || addon.downloadURL == "" {
		return fmt.Errorf("wowinterface: no download URL for %q", addonName(addon))
	}
	return downloadFile(ctx, p.client, addon.downloadURL, dest, progress)
}

func (p *wowInterfaceProvider) addonFromEntry(e *mmouiEntry) *Addon {
	name := e.Label
	if name == "" {
		name = e.Name
	}
	return &Addon{
		Provider:      ProviderWowInterface,
		ID:            strconv.Itoa(e.UID),
		Name:          name,
		Author:        e.Author,
		LatestVersion: e.Version,
		Summary:       truncate(e.Changelog, 300),
		Homepage:      fmt.Sprintf("https://www.wowinterface.com/downloads/info%d-%s.html", e.UID, e.Name),
		UpdatedAt:     parseMMOIDate(e.Date),
		downloadURL:   e.URL,
	}
}

// parseMMOIDate handles the date formats seen in the filelist.
func parseMMOIDate(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		"01/02/2006",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// truncate cuts a string to n runes, appending an ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// addonName returns a display name for error messages.
func addonName(a *Addon) string {
	if a == nil {
		return "?"
	}
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}
