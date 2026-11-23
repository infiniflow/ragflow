package tokenizer

import (
	"testing"
	"unicode/utf8"
)

// TestTrimToRuneLimitByCount pins the shared rune-binary-search semantics used
// by every model's trimToLimit. countTokens is injected so each model keeps
// its own token counting while sharing the boundary logic.
func TestTrimToRuneLimitByCount(t *testing.T) {
	runeCount := func(s string) int { return len([]rune(s)) }

	cases := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{"non-positive limit", "abc", 0, ""},
		{"negative limit", "abc", -1, ""},
		{"within limit", "abc", 5, "abc"},
		{"exact limit", "abcd", 4, "abcd"},
		{"over limit ascii", "abcdefgh", 4, "abcd"},
		{"multibyte rune safe", "中文测试文本", 3, "中文测"},
		{"empty text", "", 4, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trimToRuneLimitByCount(tc.text, tc.limit, runeCount)
			if got != tc.want {
				t.Errorf("trimToRuneLimitByCount(%q, %d) = %q, want %q", tc.text, tc.limit, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result %q is not valid UTF-8", got)
			}
		})
	}
}

// TestTrimToRuneLimitRespectsInjectedCount ensures the injected counter, not
// the raw rune length, drives the cut.
func TestTrimToRuneLimitRespectsInjectedCount(t *testing.T) {
	// Count as if every two runes cost one token: "abcdef" with limit 2
	// should keep 4 runes.
	halfCount := func(s string) int { return (len([]rune(s)) + 1) / 2 }
	got := trimToRuneLimitByCount("abcdef", 2, halfCount)
	if got != "abcd" {
		t.Errorf("got %q, want abcd with injected half-count", got)
	}
}
