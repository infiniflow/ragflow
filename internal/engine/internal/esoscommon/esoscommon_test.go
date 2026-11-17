package esoscommon

import (
	"strings"
	"testing"
)

func TestSanitizeStringGolden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"spaces_only", "   ", ""},
		{"plain", "hello", "hello"},
		{"quote_newline", "it's a\ntest", "it s a test"},
		{"trim_surrounding", "  hello  ", "hello"},
		{"newline", "a\nb", "a b"},
		{"cr", "a\rb", "a b"},
		{"newline_cr", "a\n\rb", "a  b"},
		{"apostrophe", "don't", "don t"},
		{"tab_preserved", "tab\there", "tab	here"},
		{"unicode", "héllo\nwörld", "héllo wörld"},
		{"only_quotes", "'''", ""},
		{"consecutive_specials", "''\n\n\r\r", ""},
		{"leading_specials", "  ' \n ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SanitizeString(c.in); got != c.want {
				t.Errorf("SanitizeString(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeStringProperties(t *testing.T) {
	inputs := []string{
		"", "a", "'", "\n", "\r", " ' \n\r ", "it's\na\ntest\r",
		"mixed ' quotes\nand\rreturns", "héllo wörld", "tab\there",
		"   ", "a   b", "'\n'\r'",
	}
	for _, in := range inputs {
		out := SanitizeString(in)
		if strings.ContainsAny(out, "'\n\r") {
			t.Errorf("SanitizeString(%q) = %q contains unsanitized char", in, out)
		}
		if out != strings.TrimSpace(out) {
			t.Errorf("SanitizeString(%q) = %q is not trimmed", in, out)
		}
	}
}
