package reduction

import "testing"

// TestTruncateToolResultCharacterization pins the current contract:
// non-positive maxLen and already-fitting inputs are returned untouched;
// otherwise the value is cut at a maxLen-rune boundary and a byte-count
// suffix is appended.
func TestTruncateToolResultCharacterization(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty", "", 5, ""},
		{"non-positive max returns whole input", "abcdef", 0, "abcdef"},
		{"negative max returns whole input", "abcdef", -1, "abcdef"},
		{"within budget", "hello", 10, "hello"},
		{"exact budget", "abcde", 5, "abcde"},
		{"ascii truncation", "hello world", 5, "hello\n...(truncated 6 bytes)"},
		{"rune boundary", "日本語のテキスト", 3, "日本語\n...(truncated 15 bytes)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateToolResult(tc.s, tc.max); got != tc.want {
				t.Fatalf("TruncateToolResult(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
			}
		})
	}
}
