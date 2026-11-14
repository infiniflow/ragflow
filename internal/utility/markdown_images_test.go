package utility

import "testing"

func TestStripMarkdownImages(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
	}{
		{"plain image", "![a](https://example.invalid/a.png)", ""},
		{"parenthesized filename", "![a](http://x/img_(1).png)", ""},
		{"multiple parentheses", "![a](http://x/(folder)/img_(1).png)", ""},
		{"surrounding text", "before ![a](http://x/img_(1).png) after", "before  after"},
		{"multiple images", "![a](a(1).png) ![b](b(2).png)", " "},
		{"html image", `<div><img src="a(1).png"></div>`, ""},
		{"regular link", "[a](http://x/img_(1).png)", "[a](http://x/img_(1).png)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripMarkdownImages(tc.text); got != tc.want {
				t.Fatalf("got=%q want=%q", got, tc.want)
			}
		})
	}
}
