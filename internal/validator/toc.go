// Package validator parses World of Warcraft TOC files and reports
// compatibility between an addon's declared interface version and the
// profile the user plays.
//
// TOC files are line-based: `## Key: Value` metadata lines followed by
// the load order file list. This parser is lenient: it ignores unknown
// keys (localized titles, curse/git metadata) and never modifies files.
package validator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wowfix/wowfix/internal/models"
)

// MaxTOCSize bounds how much of a TOC file we read; metadata lives at
// the top of the file, so a large embedded file list is irrelevant.
const MaxTOCSize = 1 << 20 // 1 MiB

// ParseTOC reads and parses a single TOC file. A missing or unreadable
// file yields an error; malformed content yields a TOC with whatever
// fields could be read.
func ParseTOC(path string) (*models.TOC, error) {
	toc := &models.TOC{
		Path:      path,
		Name:      strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Interface: -1,
		Fields:    map[string]string{},
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open TOC %q: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), MaxTOCSize)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "##") {
			continue // load-order entry or comment
		}
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, "##"))
		key, value, ok := strings.Cut(body, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}

		toc.Fields[key] = value
		switch key {
		case "Interface":
			toc.RawInterface = value
			if n, err := strconv.Atoi(value); err == nil {
				toc.Interface = n
			}
		case "Title":
			toc.Title = value
		case "Version":
			toc.Version = value
		case "Author":
			toc.Author = value
		case "Notes":
			toc.Notes = value
		}
		// Localized title, e.g. "Title-zhCN: 插件名".
		if strings.HasPrefix(key, "Title-") && toc.Title == "" {
			toc.Title = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read TOC %q: %w", path, err)
	}
	return toc, nil
}

// ParseTOCs parses every path in paths, returning the parsed TOCs and
// the errors encountered for the individual files.
func ParseTOCs(paths []string) ([]*models.TOC, []error) {
	tocs := make([]*models.TOC, 0, len(paths))
	var errs []error
	for _, p := range paths {
		t, err := ParseTOC(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tocs = append(tocs, t)
	}
	return tocs, errs
}

// Compatibility describes how a single TOC fares against a profile.
type Compatibility struct {
	TOC    *models.TOC
	Status models.CompatStatus
	Label  string
	// Detail is a human sentence, e.g. "addon targets Wrath 3.3.5a".
	Detail string
}

// ValidateTOC classifies one TOC against the expected profile.
func ValidateTOC(toc *models.TOC, profile *models.Profile) Compatibility {
	status := models.ClassifyInterface(profile, toc.Interface)
	compat := Compatibility{TOC: toc, Status: status, Label: status.Label()}
	if toc.Interface <= 0 {
		compat.Label = "Unknown"
		compat.Detail = "no ## Interface: line"
		return compat
	}
	switch status {
	case models.CompatOK:
		compat.Detail = fmt.Sprintf("targets %s %d", familyNoun(profile.Family), toc.Interface)
	case models.CompatVanilla:
		compat.Detail = fmt.Sprintf("targets Vanilla %d, expected %d", toc.Interface, profile.Interface)
	case models.CompatRetail:
		compat.Detail = fmt.Sprintf("targets Retail %d, expected %d", toc.Interface, profile.Interface)
	default:
		fam := models.FamilyLabel(models.FamilyOf(toc.Interface))
		compat.Detail = fmt.Sprintf("targets %s %d, expected %d", fam, toc.Interface, profile.Interface)
	}
	return compat
}

// ValidateAddon validates every TOC of an addon; the primary TOC drives
// the overall verdict. TOCs that parse cleanly but lack an interface
// line report as unknown rather than failing the whole addon.
func ValidateAddon(addon *models.Addon, profile *models.Profile) []Compatibility {
	out := make([]Compatibility, 0, len(addon.TOCs))
	for _, t := range addon.TOCs {
		out = append(out, ValidateTOC(t, profile))
	}
	return out
}

func familyNoun(family string) string {
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
		return "game version"
	}
}
