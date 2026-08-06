// Package detector locates World of Warcraft installations on the host
// and identifies the game version from the client executable.
package detector

import (
	"context"
	"fmt"
	"os"
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

// EnsureAddons creates the install's AddOns directory when it is missing.
// It returns whether the directory was created.
func EnsureAddons(inst *Installation) (bool, error) {
	if utils.IsDir(inst.AddonsPath) {
		return false, nil
	}
	err := utils.EnsureDir(inst.AddonsPath)
	return true, err
}

// DetectPath inspects a path and returns the best installation found there.
// It accepts either a game root directory (probing each flavor for an
// existing Interface/AddOns dir) or an Interface/AddOns (or Interface)
// folder path, from which the root and flavor are derived structurally.
func DetectPath(input string) (*Installation, error) {
	if root, flavor, addons, ok := matchAddonsPath(input); ok {
		if utils.IsDir(addons) {
			return classify(root, flavor, addons), nil
		}
		if utils.IsDir(root) {
			// AddOns is missing, but the root is still a recognisable
			// installation: keep the user's pasted structure for flavor
			// and addons even though the directory does not exist yet.
			if _, _, ok := looksLikeInstallRoot(root); ok {
				return classify(root, flavor, addons), nil
			}
			return nil, fmt.Errorf("AddOns directory does not exist: %s", addons)
		}
		return nil, fmt.Errorf("path does not exist: %s", input)
	}

	info, err := os.Stat(input)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", input)
		}
		return nil, fmt.Errorf("cannot access %q: %w", input, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", input)
	}
	for _, flavor := range []string{FlavorRoot, FlavorRetail, FlavorClassic, FlavorEra, FlavorTBC} {
		addons := AddonsPath(input, flavor)
		if utils.IsDir(addons) {
			return classify(input, flavor, addons), nil
		}
	}
	// No Interface/AddOns yet: accept the root when a client executable
	// or a bare Interface folder marks it as an installation.
	if flavor, addons, ok := looksLikeInstallRoot(input); ok {
		return classify(input, flavor, addons), nil
	}
	return nil, fmt.Errorf("no Interface/AddOns directory found under %s", input)
}

// matchAddonsPath recognizes an Interface/AddOns (or Interface) folder path
// and derives the game root and flavor from it. Basename comparisons are
// case-insensitive. Returns ok=false when input is not such a path.
func matchAddonsPath(input string) (root, flavor, addons string, ok bool) {
	clean := filepath.Clean(input)
	var interfaceDir string
	switch {
	case strings.EqualFold(filepath.Base(clean), "AddOns"):
		interfaceDir = filepath.Dir(clean)
		if !strings.EqualFold(filepath.Base(interfaceDir), "Interface") {
			return "", "", "", false
		}
		addons = clean
	case strings.EqualFold(filepath.Base(clean), "Interface"):
		interfaceDir = clean
		addons = filepath.Join(clean, "AddOns")
		if !utils.IsDir(addons) {
			return "", "", "", false
		}
	default:
		return "", "", "", false
	}

	// The folder directly above Interface may be a flavor subfolder.
	above := filepath.Dir(interfaceDir)
	switch strings.ToLower(filepath.Base(above)) {
	case FlavorRetail, FlavorClassic, FlavorEra, FlavorTBC:
		root = filepath.Dir(above)
		flavor = strings.ToLower(filepath.Base(above))
	default:
		root = above
		flavor = FlavorRoot
	}
	return root, flavor, addons, true
}

// findClientExe looks for a known WoW client executable in root and
// returns the entry's original name when one is found. The macOS app
// bundle is a directory; all other accepted names are regular files.
// Matches are case-insensitive.
func findClientExe(root string) (name string, ok bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		entry := e.Name()
		if e.IsDir() && strings.EqualFold(entry, "World of Warcraft.app") {
			return entry, true
		}
		if !e.Type().IsRegular() {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(entry, filepath.Ext(entry)))
		switch stem {
		case "wow", "wow-64", "wow_beta", "wowclassic", "wowclassic_beta",
			"wowclassic_era", "wowclassic_ptr", "wowclassic_tbc", "world of warcraft":
			return entry, true
		}
	}
	return "", false
}

// looksLikeInstallRoot reports whether root is a game installation that has
// no AddOns directory yet: a flavor folder with an Interface subfolder, or
// a client executable at the root.
func looksLikeInstallRoot(root string) (flavor, addons string, ok bool) {
	for _, flavor := range []string{FlavorRoot, FlavorRetail, FlavorClassic, FlavorEra, FlavorTBC} {
		if utils.IsDir(filepath.Join(root, flavor, "Interface")) {
			return flavor, AddonsPath(root, flavor), true
		}
	}
	if _, found := findClientExe(root); found {
		return FlavorRoot, AddonsPath(root, FlavorRoot), true
	}
	return "", "", false
}

// classify inspects the client binaries to identify the game version.
func classify(root, flavor, addons string) *Installation {
	inst := &Installation{Root: root, Flavor: flavor, AddonsPath: addons}

	if exe, ok := findClientExe(root); ok {
		inst.Exe = exe
		if v, ok, err := utils.FileVersion(filepath.Join(root, exe)); err == nil && ok {
			inst.Version = v
		}
		inst.ProfileID = profileFromExe(exe, inst.Version)
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
