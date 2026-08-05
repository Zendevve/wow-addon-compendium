package models

import "strings"

// Profile describes a supported WoW game version. The fixing logic is
// version-agnostic; profiles only drive the TOC compatibility report.
type Profile struct {
	ID        string // stable identifier stored in config
	Name      string // human-readable name
	Family    string // interface-number family: vanilla | tbc | wrath | cata | retail
	Interface int    // expected ## Interface: value
}

// Well-known profiles. Interface numbers follow the historical values:
//
//	Vanilla 1.12.x        11200
//	TurtleWoW 1.17.x      11200 (vanilla-compatible TOCs)
//	TBC 2.4.3             20400
//	WotLK 3.3.5a          30300
//	Cataclysm 4.3.4       40300
//	Classic Era 1.14.x    11403
//	Hardcore 1.14.x       11403
//	SoD 1.15.x            11504
//	Retail (current)      100207
var Profiles = []Profile{
	{ID: "vanilla", Name: "Vanilla 1.12", Family: "vanilla", Interface: 11200},
	{ID: "turtle", Name: "TurtleWoW", Family: "vanilla", Interface: 11200},
	{ID: "tbc", Name: "The Burning Crusade 2.4.3", Family: "tbc", Interface: 20400},
	{ID: "wrath", Name: "Wrath of the Lich King 3.3.5a", Family: "wrath", Interface: 30300},
	{ID: "cata", Name: "Cataclysm 4.3.4", Family: "cata", Interface: 40300},
	{ID: "classic", Name: "Classic Era", Family: "vanilla", Interface: 11403},
	{ID: "hardcore", Name: "Hardcore", Family: "vanilla", Interface: 11403},
	{ID: "sod", Name: "Season of Discovery", Family: "vanilla", Interface: 11504},
	{ID: "retail", Name: "Retail", Family: "retail", Interface: 100207},
}

// ProfileByID resolves a profile by its ID. Returns nil when unknown.
func ProfileByID(id string) *Profile {
	for i := range Profiles {
		if Profiles[i].ID == id {
			return &Profiles[i]
		}
	}
	return nil
}

// DefaultProfile is used when nothing has been configured yet.
func DefaultProfile() *Profile {
	return ProfileByID("wrath")
}

// FamilyOf maps an interface number to its family name.
func FamilyOf(iface int) string {
	switch {
	case iface <= 0:
		return "unknown"
	case iface < 20000:
		return "vanilla"
	case iface < 30000:
		return "tbc"
	case iface < 40000:
		return "wrath"
	case iface < 50000:
		return "cata"
	default:
		return "retail"
	}
}

// CompatStatus classifies an addon's detected interface against a profile.
type CompatStatus string

const (
	CompatOK       CompatStatus = "compatible"
	CompatVanilla  CompatStatus = "vanilla"
	CompatRetail   CompatStatus = "retail"
	CompatMismatch CompatStatus = "mismatch"
	CompatUnknown  CompatStatus = "unknown"
)

// ClassifyInterface returns the compatibility status of a detected
// interface number against the expected profile interface.
func ClassifyInterface(profile *Profile, detected int) CompatStatus {
	if profile == nil || detected <= 0 {
		return CompatUnknown
	}
	if FamilyOf(detected) == profile.Family {
		return CompatOK
	}
	switch FamilyOf(detected) {
	case "vanilla":
		return CompatVanilla
	case "retail":
		return CompatRetail
	default:
		return CompatMismatch
	}
}

// CompatLabel is a short human-readable label for a status.
func (c CompatStatus) Label() string {
	switch c {
	case CompatOK:
		return "Compatible"
	case CompatVanilla:
		return "Vanilla Addon"
	case CompatRetail:
		return "Retail Addon"
	case CompatMismatch:
		return "Version Mismatch"
	default:
		return "Unknown"
	}
}

// FamilyLabel names an interface-number family for reports.
func FamilyLabel(family string) string {
	switch family {
	case "vanilla":
		return "Vanilla"
	case "tbc":
		return "TBC"
	case "wrath":
		return "Wrath"
	case "cata":
		return "Cataclysm"
	case "retail":
		return "Retail"
	default:
		return "Unknown"
	}
}

// ProfileVersion parses a detected interface family into words, e.g.
// "Vanilla addon" for 11200.
func ProfileVersion(detected int) string {
	family := FamilyOf(detected)
	return strings.ToLower(FamilyLabel(family))
}
