package file

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no special", "report.txt", "report.txt"},
		{"path stripped", "dir/sub/report.txt", "report.txt"},
		{"forbidden chars", "a:b*c?", "a_b_c_"},
		{"reserved device", "CON", "download"},
		{"only dots", "...", "download"},
		{"leading dots", ".hidden", "hidden"},
		{"long name keeps extension", strings.Repeat("a", 300) + ".pdf", strings.Repeat("a", 251) + ".pdf"},
		// The leading "a" puts the 255-byte boundary inside a 3-byte character.
		{"long CJK name cut on a character", "a" + strings.Repeat("报", 100), "a" + strings.Repeat("报", 84)},
	}
	for _, c := range cases {
		got := sanitizeFilename(c.in)
		if c.want != "" && got != c.want {
			t.Errorf("%s: sanitizeFilename(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
		// Safety invariant: never emit characters that are unsafe in a path.
		for _, r := range got {
			switch r {
			case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
				t.Errorf("%s: forbidden char %q in %q", c.name, r, got)
			}
			if r < 0x20 {
				t.Errorf("%s: control char in %q", c.name, got)
			}
		}
		if got == "" {
			t.Errorf("%s: empty result for %q", c.name, c.in)
		}
		if len(got) > maxUploadFilenameBytes || !utf8.ValidString(got) {
			t.Errorf("%s: %q is longer than %d bytes or not valid UTF-8", c.name, got, maxUploadFilenameBytes)
		}
	}
}
