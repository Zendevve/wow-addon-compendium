package catalog

import (
	"strconv"
	"strings"
)

// gameFamily maps a WoW game-version string to a client family name,
// mirroring the classification used for TOC interface numbers:
// "1.14.0" -> vanilla, "2.4.3" -> tbc, "3.3.5" -> wrath,
// "4.3.4" -> cata, "11.0.2" -> retail. Unknown or empty strings map
// to "".
func gameFamily(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return ""
	}
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == ',' || r == ' '
	})
	if len(parts) == 0 {
		return ""
	}
	// Tolerate suffixes such as the "a" in "3.3.5a".
	major := strings.TrimRight(parts[0], "abcde")
	n, err := strconv.Atoi(major)
	if err != nil {
		return ""
	}
	switch {
	case n < 2:
		return "vanilla"
	case n == 2:
		return "tbc"
	case n == 3:
		return "wrath"
	case n == 4:
		return "cata"
	default:
		return "retail"
	}
}
