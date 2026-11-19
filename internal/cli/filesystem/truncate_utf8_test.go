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
