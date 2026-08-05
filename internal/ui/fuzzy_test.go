package ui

import "testing"

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		pattern, text string
		want          bool
	}{
		{"", "AtlasLoot", true},
		{"atlas", "AtlasLoot", true},
		{"atloot", "AtlasLoot", true}, // subsequence a-t-l-o-o-t
		{"atlootx", "AtlasLoot", false},
		{"quest", "Questie", true},
		{"qq", "Questie", false},
		{"dps", "DPSMate", true},
		{"weak", "WeakAuras", true},
		{"wa", "WeakAuras", true},
		{"WeakAuras", "weakauras", true}, // case-insensitive
	}
	for _, c := range cases {
		if got := FuzzyMatch(c.pattern, c.text); got != c.want {
			t.Errorf("FuzzyMatch(%q, %q) = %v, want %v", c.pattern, c.text, got, c.want)
		}
	}
}

func TestFuzzyScoreConsistency(t *testing.T) {
	// A positive score means a match, and a match means a positive score.
	for _, c := range []struct{ pattern, text string }{
		{"atlas", "AtlasLoot"},
		{"atloot", "AtlasLoot"},
		{"weak", "WeakAuras"},
		{"dps", "DPSMate"},
	} {
		if s := FuzzyScore(c.pattern, c.text); s <= 0 {
			t.Errorf("FuzzyScore(%q, %q) = %d, want > 0", c.pattern, c.text, s)
		}
		if !FuzzyMatch(c.pattern, c.text) {
			t.Errorf("FuzzyMatch(%q, %q) = false, want true", c.pattern, c.text)
		}
	}
	for _, c := range []struct{ pattern, text string }{
		{"zzz", "AtlasLoot"},
		{"qq", "Questie"},
		{"atlootx", "AtlasLoot"},
	} {
		if s := FuzzyScore(c.pattern, c.text); s != 0 {
			t.Errorf("FuzzyScore(%q, %q) = %d, want 0", c.pattern, c.text, s)
		}
		if FuzzyMatch(c.pattern, c.text) {
			t.Errorf("FuzzyMatch(%q, %q) = true, want false", c.pattern, c.text)
		}
	}
}

func TestFuzzyScoreConsecutiveBonus(t *testing.T) {
	// Consecutive characters outrank a scattered subsequence.
	if got, want := FuzzyScore("ab", "ab"), FuzzyScore("ab", "axb"); got <= want {
		t.Errorf("consecutive match %d should beat scattered match %d", got, want)
	}
	// The best alignment wins, not the first: "ab" in "aXab" should beat
	// "ab" in "axb" because the later alignment is consecutive.
	if got, want := FuzzyScore("ab", "aXab"), FuzzyScore("ab", "axb"); got <= want {
		t.Errorf("best-alignment match %d should beat scattered match %d", got, want)
	}
}

func TestFuzzyScoreBoundaryBonus(t *testing.T) {
	// A match that starts a word scores higher than one buried mid-word.
	if got, want := FuzzyScore("ab", "a-b"), FuzzyScore("ab", "axb"); got <= want {
		t.Errorf("boundary match %d should beat mid-word match %d", got, want)
	}
	// Digit boundary: "v2" in "v2" beats "v2" in "vax2".
	if got, want := FuzzyScore("v2", "v2"), FuzzyScore("v2", "vax2"); got <= want {
		t.Errorf("digit-boundary match %d should beat scattered match %d", got, want)
	}
}

func TestFuzzyScoreEmpty(t *testing.T) {
	if s := FuzzyScore("", "AtlasLoot"); s != 0 {
		t.Errorf("empty pattern score = %d, want 0", s)
	}
	if s := FuzzyScore("atlas", ""); s != 0 {
		t.Errorf("empty text score = %d, want 0", s)
	}
}
