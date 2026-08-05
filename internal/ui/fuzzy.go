package ui

import "strings"

// FuzzyMatch reports whether pattern occurs in text as a case-insensitive
// subsequence. An empty pattern matches everything.
func FuzzyMatch(pattern, text string) bool {
	pl := []rune(strings.ToLower(pattern))
	if len(pl) == 0 {
		return true
	}
	tl := []rune(strings.ToLower(text))
	pi := 0
	for _, r := range tl {
		if r == pl[pi] {
			pi++
			if pi == len(pl) {
				return true
			}
		}
	}
	return false
}

// FuzzyScore ranks how well pattern matches text as a case-insensitive
// subsequence. It returns 0 when pattern is empty or not a subsequence of
// text. Matches score higher when their characters are consecutive in text
// and when a match begins a word (after a separator or a digit).
//
// The score is computed with dynamic programming so the best-scoring
// alignment wins, not merely the earliest one.
func FuzzyScore(pattern, text string) int {
	pl := []rune(strings.ToLower(pattern))
	tl := []rune(strings.ToLower(text))
	if len(pl) == 0 || len(tl) == 0 {
		return 0
	}
	// dp[j] is the best score for matching pl[0..j-1] in the text prefix
	// processed so far; -1 means unreachable.
	dp := make([]int, len(pl)+1)
	for i := 1; i <= len(pl); i++ {
		dp[i] = -1
	}
	for ti, r := range tl {
		// Descending j so each text character is consumed at most once.
		for j := len(pl) - 1; j >= 0; j-- {
			if dp[j] < 0 || r != pl[j] {
				continue
			}
			s := dp[j] + 1
			if j > 0 && ti > 0 && tl[ti-1] == pl[j-1] {
				s += 2 // consecutive characters
			}
			if ti > 0 && isWordBoundary(tl[ti-1]) {
				s += 3 // match starts a word
			}
			if s > dp[j+1] {
				dp[j+1] = s
			}
		}
	}
	if dp[len(pl)] < 0 {
		return 0
	}
	return dp[len(pl)]
}

// isWordBoundary reports whether r separates words: common separators or a
// digit (so "v2" matches "v2" better than "vax2").
func isWordBoundary(r rune) bool {
	switch r {
	case ' ', '-', '_', '.', '/', '\\', '(', '[', '{', ':', '+':
		return true
	}
	return r >= '0' && r <= '9'
}
