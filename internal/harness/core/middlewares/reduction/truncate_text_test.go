package reduction

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateText(t *testing.T) {
	if got := truncateText("hello", 10); got != "hello" {
		t.Fatalf("short string changed: %q", got)
	}
	if got := truncateText("hello world", 5); got != "hello" {
		t.Fatalf("ascii truncation: got %q want %q", got, "hello")
	}

	// A byte slice at maxLen would land in the middle of a multi-byte rune
	// and yield invalid UTF-8. Truncating on a rune boundary must not.
	got := truncateText("日本語のテキスト", 3)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got)
	}
	if got != "日本語" {
		t.Fatalf("got %q want %q", got, "日本語")
	}
}
