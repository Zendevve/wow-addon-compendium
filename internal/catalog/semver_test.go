package catalog

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		want Ver
	}{
		{in: "1.2.3", ok: true, want: Ver{Major: 1, Minor: 2, Patch: 3}},
		{in: "v1.2.3", ok: true, want: Ver{Major: 1, Minor: 2, Patch: 3}},
		{in: "V2", ok: true, want: Ver{Major: 2}},
		{in: "3.3.5a", ok: false},
		{in: "1.2", ok: true, want: Ver{Major: 1, Minor: 2}},
		{in: "0.0.1", ok: true, want: Ver{Major: 0, Minor: 0, Patch: 1}},
		{in: "1.2.3-beta", ok: true, want: Ver{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta"}},
		{in: "1.2.3-RC2", ok: true, want: Ver{Major: 1, Minor: 2, Patch: 3, Prerelease: "rc2"}},
		{in: "v1.0.0-alpha", ok: true, want: Ver{Major: 1, Prerelease: "alpha"}},
		{in: "1.2.3.4", ok: false}, // four parts are out of contract
		{in: "", ok: false},
		{in: "latest", ok: false},
		{in: "  ", ok: false},
		{in: "1..2", ok: false},
		{in: "1.2.3-", ok: true, want: Ver{Major: 1, Minor: 2, Patch: 3, Prerelease: ""}},
		{in: "1.2.3-hotfix-2", ok: false}, // dash inside suffix is rejected
	}
	for _, tt := range tests {
		got, ok := Parse(tt.in)
		if ok != tt.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-rc1", -1},
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		{"1.0.0-rc", "1.0.0", -1},
		{"1.0.0-beta2", "1.0.0-beta10", -1},
		{"1.0.0-beta", "1.0.0-beta2", -1},
		{"1.0.1", "1.0.0-beta", 1},
		// Fallback: unparseable sides use plain string comparison.
		{"latest", "1.0.0", 1},
		{"1.0.0", "latest", -1},
		{"main@HEAD", "1.2.3", 1},
		{"a", "b", -1},
		{"", "", 0},
		{"main@HEAD", "main@HEAD", 0},
	}
	for _, tt := range tests {
		if got := Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
