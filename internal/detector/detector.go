// Package detector locates World of Warcraft installations on the host
// and identifies the game version from the client executable.
package detector

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wowfix/wowfix/internal/utils"
)

// Flavor names for client subfolders.
const (
	FlavorRoot    = "" // Interface/AddOns at the root
	FlavorRetail  = "_retail_"
	FlavorClassic = "_classic_"
	FlavorEra     = "_classic_era_"
	FlavorTBC     = "_classic_tbc_"
)

// AddonsPath returns the Interface/AddOns path for a flavor.
func AddonsPath(root, flavor string) string {
	if flavor == "" {
		return filepath.Join(root, "Interface", "AddOns")
	}
	return filepath.Join(root, flavor, "Interface", "AddOns")
}

// Installation is one detected WoW install.
type Installation struct {
	Root       string
	Flavor     string
	AddonsPath string
	// Exe is the detected client executable name, e.g. "Wow.exe".
	Exe string
	// Version is the raw executable version string when readable.
	Version string
	// ProfileID is the best-guess game profile; empty when unknown.
	ProfileID string
	// Confidence: "high" | "medium" | "low"
	Confidence string
}

// Exists reports whether the install's AddOns directory is present.
func (inst Installation) Exists() bool {
	return utils.IsDir(inst.AddonsPath)
}

// DetectPath inspects a game root directory and returns the best
// installation found there (the flavor with an existing AddOns dir).
func DetectPath(root string) (*Installation, error) {
	if !utils.IsDir(root) {
		return nil, fmt.Errorf("not a directory: %s", root)
	}
	for _, flavor := range []string{FlavorRoot, FlavorRetail, FlavorClassic, FlavorEra, FlavorTBC} {
		addons := AddonsPath(root, flavor)
		if utils.IsDir(addons) {
			return classify(root, flavor, addons), nil
		}
	}
	return nil, fmt.Errorf("no Interface/AddOns directory found under %s", root)
}

// classify inspects the client binaries to identify the game version.
func classify(root, flavor, addons string) *Installation {
	inst := &Installation{Root: root, Flavor: flavor, AddonsPath: addons}

	candidates := []string{"Wow.exe", "wow.exe", "WowClassic.exe", "WowClassic_TBC.exe", "World of Warcraft.app"}
	for _, exe := range candidates {
		p := filepath.Join(root, exe)
		if utils.Exists(p) {
			inst.Exe = exe
			break
		}
	}

	if inst.Exe != "" {
		if v, ok, err := utils.FileVersion(filepath.Join(root, inst.Exe)); err == nil && ok {
			inst.Version = v
		}
		inst.ProfileID = profileFromExe(inst.Exe, inst.Version)
		inst.Confidence = "high"
		return inst
	}

	// No executable readable: fall back to the flavor folder.
	switch flavor {
	case FlavorRetail:
		inst.ProfileID = "retail"
	case FlavorClassic:
		inst.ProfileID = "wrath" // modern Classic uses 3.4.x TOCs
	case FlavorEra:
		inst.ProfileID = "classic"
	case FlavorTBC:
		inst.ProfileID = "tbc"
	default:
		inst.ProfileID = ""
	}
	inst.Confidence = "medium"
	return inst
}

// profileFromExe maps the client executable to a profile ID using the
// PE version resource when available.
func profileFromExe(exe, version string) string {
	lower := strings.ToLower(exe)
	switch {
	case strings.Contains(lower, "classic_tbc"):
		return "tbc"
	case strings.Contains(lower, "classic"):
		return "classic"
	}

	// Wow.exe: disambiguate by version number.
	major, minor, ok := parseMajorMinor(version)
	if !ok {
		return ""
	}
	switch {
	case major == 1 && minor == 12:
		return "vanilla"
	case major == 1 && minor >= 16:
		return "turtle"
	case major == 1:
		return "vanilla"
	case major == 2:
		return "tbc"
	case major == 3:
		return "wrath"
	case major == 4:
		return "cata"
	default:
		return "retail"
	}
}

func parseMajorMinor(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, false
	}
	for _, p := range parts[:2] {
		for _, r := range p {
			if r < '0' || r > '9' {
				return 0, 0, false
			}
		}
	}
	major = 0
	for _, r := range parts[0] {
		major = major*10 + int(r-'0')
	}
	minor = 0
	for _, r := range parts[1] {
		minor = minor*10 + int(r-'0')
	}
	return major, minor, true
}

// AutoDetect searches common installation locations and returns every
// install found, best-confidence first.
func AutoDetect(ctx context.Context) ([]Installation, error) {
	var found []Installation
	seen := map[string]bool{}

	add := func(root, flavor string) {
		root = filepath.Clean(root)
		key := strings.ToLower(root + "|" + flavor)
		if seen[key] {
			return
		}
		seen[key] = true
		if inst, err := DetectPath(root); err == nil && inst.Flavor == flavor {
			found = append(found, *inst)
		}
	}

	for _, candidate := range candidateRoots() {
		if ctx.Err() != nil {
			break
		}
		root, flavor := candidate[0], candidate[1]
		if utils.IsDir(root) {
			add(root, flavor)
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		return confidenceRank(found[i].Confidence) < confidenceRank(found[j].Confidence)
	})
	return found, nil
}

func confidenceRank(c string) int {
	switch c {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}
