// The curated catalog is the private-server curation layer over the
// catalog providers: a hand-verified manifest of known-good addons for
// the vanilla-family clones (Turtle-style 1.12 clients) and WotLK
// 3.3.5a (ChromieCraft), each anchored to a GitHub source the github
// provider resolves natively ("owner/repo").
//
// There is no private-server addon database to query — Turtle WoW shut
// down in May 2026 and ChromieCraft points players at the public
// providers — so this manifest is the moat: every entry was verified
// against its repository's TOC (## Interface) and description on
// 2026-08-07. Sources should be re-verified on a maintenance cycle
// before new installs are assumed to work.
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed curated.json
var curatedFS embed.FS

// CuratedAddon is one verified private-server addon.
type CuratedAddon struct {
	Name     string `json:"name"`
	Source   string `json:"source"` // the exact string InstallFromSource accepts ("owner/repo")
	Summary  string `json:"summary"`
	Homepage string `json:"homepage"`
}

// Set is one game family's curated addon list.
type Set struct {
	Family string         `json:"family"`
	Label  string         `json:"label"`
	Addons []CuratedAddon `json:"addons"`
}

// CuratedManifest is the versioned curated catalog.
type CuratedManifest struct {
	Version int   `json:"version"`
	Sets    []Set `json:"sets"`
}

// LoadCurated loads the embedded curated manifest. The embed always
// resolves at build time; the only error is malformed JSON.
func LoadCurated() (*CuratedManifest, error) {
	data, err := curatedFS.ReadFile("curated.json")
	if err != nil {
		return nil, fmt.Errorf("load curated manifest: %w", err)
	}
	var m CuratedManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse curated manifest: %w", err)
	}
	return &m, nil
}

// SetForFamily returns the curated set for a family name,
// case-insensitively. Unknown families return false.
func (m *CuratedManifest) SetForFamily(family string) (*Set, bool) {
	want := strings.ToLower(strings.TrimSpace(family))
	for i := range m.Sets {
		if strings.ToLower(m.Sets[i].Family) == want {
			return &m.Sets[i], true
		}
	}
	return nil, false
}

// AddonByName returns the set's addon named name, case-insensitively.
func (s *Set) AddonByName(name string) (*CuratedAddon, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for i := range s.Addons {
		if strings.ToLower(s.Addons[i].Name) == want {
			return &s.Addons[i], true
		}
	}
	return nil, false
}
