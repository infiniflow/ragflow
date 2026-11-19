package filesystem

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateStringKeepsUTF8Valid pins that truncation on a multibyte name
// never splits a rune. The previous byte-slice cut produced invalid UTF-8.
func TestTruncateStringKeepsUTF8Valid(t *testing.T) {
	s := "中文测试文本名称"
	for _, max := range []int{4, 6, 8, 12, 20} {
		got := truncateString(s, max)
		if !utf8.ValidString(got) {
			t.Errorf("truncateString(%q, %d) = %q, invalid UTF-8", s, max, got)
		}
	}

	// A multibyte string within the byte budget is returned untouched.
	if got := truncateString("中文", 8); got != "中文" {
		t.Errorf("truncateString within budget = %q, want 中文", got)
	}
}

// TestTruncateStringKeepsEllipsis pins the visible ellipsis contract after the
// switch to rune-safe truncation.
func TestTruncateStringKeepsEllipsis(t *testing.T) {
	got := truncateString("abcdefghij", 6)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated string %q must end with ellipsis", got)
	}
}

// TestTruncateStringRuneGuard pins that the length guard compares rune count,
// not byte length. 14 CJK runes span 42 bytes: with a 40-rune budget the name
// fits and must be returned untouched (no ellipsis). The old byte-based guard
// took the truncate branch and appended "..." to a name that already fit.
func TestTruncateStringRuneGuard(t *testing.T) {
	fitting := strings.Repeat("中", 14) // 14 runes, 42 bytes, budget 40
	if got := truncateString(fitting, 40); got != fitting {
		t.Errorf("fitting multibyte name was altered: got %q (%d runes), want unchanged", got, len([]rune(got)))
	}
	if strings.HasSuffix(truncateString(fitting, 40), "...") {
		t.Error("fitting multibyte name must not get an ellipsis")
	}

	tooLong := strings.Repeat("中", 41) // 41 runes > 40
	got := truncateString(tooLong, 40)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("over-budget name %q must end with ellipsis", got)
	}
	if n := len([]rune(got)); n != 40 {
		t.Errorf("truncated name has %d runes, want 40 (37 + ...)", n)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncated name %q is not valid UTF-8", got)
	}
}
