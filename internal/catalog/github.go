package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPIBase  = "https://api.github.com"
	githubCodeload = "https://codeload.github.com"
)

// githubProvider fetches addons from the GitHub REST API. The API is
// rate-limited to 60 requests/hour per IP for unauthenticated
// clients, so calls are kept sparse: Resolve/Latest hit two endpoints
// (repository + latest release). No authentication is used; see the
// package documentation for the rate-limit caveat.
type githubProvider struct {
	client   *http.Client
	api      string // API base, overridable in tests
	codeload string // codeload base, overridable in tests
}

func newGitHubProvider(client *http.Client, apiBase, codeloadBase string) *githubProvider {
	if apiBase == "" {
		apiBase = githubAPIBase
	}
	if codeloadBase == "" {
		codeloadBase = githubCodeload
	}
	return &githubProvider{client: client, api: apiBase, codeload: codeloadBase}
}

func (p *githubProvider) Name() string { return ProviderGitHub }

// ghRepo mirrors the GitHub repository object fields we use.
type ghRepo struct {
	FullName      string    `json:"full_name"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	HTMLURL       string    `json:"html_url"`
	DefaultBranch string    `json:"default_branch"`
	UpdatedAt     time.Time `json:"updated_at"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// ghRelease mirrors the GitHub release object fields we use.
type ghRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type ghSearchResult struct {
	Items []ghRepo `json:"items"`
}

// Search looks repositories up on GitHub. The query is first tried
// with a topic qualifier that biases towards WoW addons; if GitHub
// rejects that query (rate limit, malformed qualifier) the plain
// query is retried.
func (p *githubProvider) Search(ctx context.Context, query string, limit int) ([]*Addon, error) {
	items, err := p.search(ctx, query+" topic:world-of-warcraft-addon", limit)
	if err != nil {
		items, err = p.search(ctx, query, limit)
	}
	if err != nil {
		return nil, err
	}
	out := make([]*Addon, 0, len(items))
	for i := range items {
		out = append(out, p.addonFromRepo(&items[i]))
	}
	return out, nil
}

func (p *githubProvider) search(ctx context.Context, q string, limit int) ([]ghRepo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.api+"/search/repositories", nil)
	if err != nil {
		return nil, err
	}
	qp := url.Values{}
	qp.Set("q", q)
	if limit > 0 {
		if limit > 100 {
			limit = 100
		}
		qp.Set("per_page", strconv.Itoa(limit))
	}
	req.URL.RawQuery = qp.Encode()
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wowfix")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &statusError{code: resp.StatusCode, url: req.URL.String(), msg: strings.TrimSpace(string(body))}
	}
	var sr ghSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
	return sr.Items, nil
}

// Resolve fetches one repository by its "owner/repo" id.
func (p *githubProvider) Resolve(ctx context.Context, id string) (*Addon, error) {
	return p.fetch(ctx, id)
}

// Latest refreshes version information for one repository.
func (p *githubProvider) Latest(ctx context.Context, addon *Addon) (*Addon, error) {
	if addon == nil || addon.ID == "" {
		return nil, fmt.Errorf("github: missing repository id")
	}
	return p.fetch(ctx, addon.ID)
}

// fetch resolves a repository and its latest release, falling back to
// a default-branch source zip when the repository has no releases.
func (p *githubProvider) fetch(ctx context.Context, id string) (*Addon, error) {
	owner, repo, err := splitRepo(id)
	if err != nil {
		return nil, err
	}
	repoURL := fmt.Sprintf("%s/repos/%s/%s", p.api, owner, repo)
	body, err := p.get(ctx, repoURL)
	if err != nil {
		return nil, err
	}
	var r ghRepo
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	addon := p.addonFromRepo(&r)

	relURL := fmt.Sprintf("%s/repos/%s/%s/releases/latest", p.api, owner, repo)
	relBody, err := p.get(ctx, relURL)
	if err != nil {
		if isNotFound(err) {
			// No releases: track the default branch tip instead.
			branch := r.DefaultBranch
			if branch == "" {
				branch = "main"
			}
			addon.LatestVersion = branch + "@HEAD"
			addon.downloadURL = fmt.Sprintf("%s/%s/%s/zip/refs/heads/%s", p.codeload, owner, repo, branch)
			return addon, nil
		}
		return nil, err
	}
	var rel ghRelease
	if err := json.Unmarshal(relBody, &rel); err != nil {
		return nil, err
	}
	addon.LatestVersion = rel.TagName
	if !rel.PublishedAt.IsZero() {
		addon.UpdatedAt = rel.PublishedAt
	}
	for _, a := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
			addon.downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if addon.downloadURL == "" {
		// The release exists but has no zip asset: source zip it is.
		branch := r.DefaultBranch
		if branch == "" {
			branch = "main"
		}
		addon.downloadURL = fmt.Sprintf("%s/%s/%s/zip/refs/heads/%s", p.codeload, owner, repo, branch)
	}
	return addon, nil
}

func (p *githubProvider) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wowfix")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{code: resp.StatusCode, url: u, msg: strings.TrimSpace(string(body))}
	}
	return body, nil
}

// Download fetches the archive the addon points at (the latest
// release's first .zip asset, or the codeload source zip when the
// release has none). Resolve the addon first so the download URL is
// known.
func (p *githubProvider) Download(ctx context.Context, addon *Addon, dest string, progress func(done, total int64)) error {
	if addon == nil {
		return fmt.Errorf("github: nil addon")
	}
	if addon.downloadURL == "" {
		return fmt.Errorf("github: no download URL for %q (resolve the addon first)", addon.ID)
	}
	return downloadFile(ctx, p.client, addon.downloadURL, dest, progress)
}

// addonFromRepo builds an addon from repository metadata. The
// download URL defaults to the default-branch source zip; fetch()
// replaces it with the latest release asset when one exists.
func (p *githubProvider) addonFromRepo(r *ghRepo) *Addon {
	id := r.FullName
	if id == "" {
		id = r.Name
	}
	a := &Addon{
		Provider:  ProviderGitHub,
		ID:        id,
		Name:      r.Name,
		Author:    r.Owner.Login,
		Summary:   r.Description,
		Homepage:  r.HTMLURL,
		UpdatedAt: r.UpdatedAt,
	}
	if r.DefaultBranch != "" && a.ID != "" {
		owner, repo, err := splitRepo(a.ID)
		if err == nil {
			a.downloadURL = fmt.Sprintf("%s/%s/%s/zip/refs/heads/%s", p.codeload, owner, repo, r.DefaultBranch)
		}
	}
	return a
}

func splitRepo(id string) (owner, repo string, err error) {
	owner, repo, ok := strings.Cut(id, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("github: invalid repository id %q (want owner/repo)", id)
	}
	return owner, repo, nil
}
