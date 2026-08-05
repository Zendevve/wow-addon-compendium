package catalog

import (
	"strconv"
	"strings"
)

// Ver is a parsed three-part numeric version with an optional
// pre-release suffix such as "v1.2.3-beta".
type Ver struct {
	Major, Minor, Patch int
	// Prerelease is the normalized suffix without the leading dash;
	// empty means a full release.
	Prerelease string
}

// Parse parses a version string: an optional "v"/"V" prefix, one to
// three dot-separated numeric parts and an optional pre-release
// suffix (-alpha, -beta, -rc, optionally with a trailing build number
// like -rc2). Anything else ("latest", "1.2.3.4", "1.0.0-hotfix-2")
// reports ok=false.
func Parse(v string) (Ver, bool) {
	var ver Ver
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return Ver{}, false
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre := v[i+1:]
		v = v[:i]
		if !validPrerelease(pre) {
			return Ver{}, false
		}
		ver.Prerelease = normalizePrerelease(pre)
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return Ver{}, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Ver{}, false
		}
		nums[i] = n
	}
	ver.Major, ver.Minor, ver.Patch = nums[0], nums[1], nums[2]
	return ver, true
}

// Compare orders two version strings. When both parse, ordering is
// numeric with pre-releases sorting before their release
// ("1.0.0-alpha" < "1.0.0-beta" < "1.0.0-rc1" < "1.0.0" < "1.0.1").
// When either side fails to parse, a plain case-insensitive string
// comparison is used as a fallback.
func Compare(a, b string) int {
	va, oka := Parse(a)
	vb, okb := Parse(b)
	if !oka || !okb {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	}
	if va.Major != vb.Major {
		return sign(va.Major - vb.Major)
	}
	if va.Minor != vb.Minor {
		return sign(va.Minor - vb.Minor)
	}
	if va.Patch != vb.Patch {
		return sign(va.Patch - vb.Patch)
	}
	return comparePrerelease(va.Prerelease, vb.Prerelease)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// validPrerelease accepts letters with an optional trailing build
// number ("beta", "rc2"); anything else ("hotfix-2", "b.1") is not a
// version we can order reliably.
func validPrerelease(s string) bool {
	if s == "" {
		return true // trailing dash: no pre-release
	}
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	for _, r := range s[:i] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// prereleaseRank orders pre-release kinds: unknown suffixes sort
// first, then alpha, beta, rc, and finally the full release.
func prereleaseRank(s string) int {
	base, _ := splitPrerelease(s)
	switch base {
	case "":
		return 4 // full release
	case "alpha":
		return 1
	case "beta":
		return 2
	case "rc":
		return 3
	default:
		return 0
	}
}

// splitPrerelease separates a trailing build number from the kind
// name ("rc2" -> "rc", 2; "beta" -> "beta", -1).
func splitPrerelease(s string) (string, int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return s, -1
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return s, -1
	}
	return s[:i], n
}

// normalizePrerelease lowercases and trims the suffix, keeping any
// trailing build number ("-RC2" -> "rc2").
func normalizePrerelease(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func comparePrerelease(a, b string) int {
	ra, rb := prereleaseRank(a), prereleaseRank(b)
	if ra != rb {
		return sign(ra - rb)
	}
	if a == "" {
		return 0
	}
	ba, na := splitPrerelease(a)
	bb, nb := splitPrerelease(b)
	if na != nb {
		return sign(na - nb)
	}
	if ba != bb {
		return strings.Compare(ba, bb)
	}
	return 0
}
