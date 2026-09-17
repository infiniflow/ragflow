//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"strings"
	"testing"
)

// convertMarkdownForTest runs the shared converter and trims the surrounding
// whitespace html-to-markdown adds for a paragraph.
func convertMarkdownForTest(t *testing.T, html string) string {
	t.Helper()
	converter := newMarkdownConverter()
	out, err := converter.ConvertString(html)
	if err != nil {
		t.Fatalf("ConvertString(%q): %v", html, err)
	}
	return strings.TrimSpace(out)
}

func TestHTMLToMarkdownKeepsWhitespaceOnlyElementBoundary(t *testing.T) {
	for _, tag := range []string{"a", "b", "strong", "em", "i", "u", "s", "del", "strike", "sub", "sup"} {
		t.Run(tag, func(t *testing.T) {
			attributes := ""
			if tag == "a" {
				attributes = ` href="https://example.com"`
			}
			got := convertMarkdownForTest(t, "<p>First<"+tag+attributes+"> </"+tag+">Last</p>")
			if got != "First Last" {
				t.Fatalf("got %q, want %q", got, "First Last")
			}
		})
	}
}

func TestHTMLToMarkdownSpaceBetweenStyledRuns(t *testing.T) {
	got := convertMarkdownForTest(t, "<p><b>First</b><b> </b><b>Last</b></p>")
	if got != "**First** **Last**" {
		t.Fatalf("got %q, want %q", got, "**First** **Last**")
	}
}

func TestHTMLToMarkdownNonBreakingSpaceStaysItself(t *testing.T) {
	got := convertMarkdownForTest(t, "<p>First<b>&#160;</b>Last</p>")
	if got != "First\u00a0Last" {
		t.Fatalf("got %q, want %q", got, "First\u00a0Last")
	}
}

func TestHTMLToMarkdownRealContentUnchanged(t *testing.T) {
	cases := []struct{ html, want string }{
		{"<p>First <b>bold</b> Last</p>", "First **bold** Last"},
		{"<p>First <em>italic</em> Last</p>", "First *italic* Last"},
		{"<p>First <code>code</code> Last</p>", "First `code` Last"},
		{`<p>First <a href="https://example.com">link</a> Last</p>`, "First [link](https://example.com) Last"},
		{"<p>First<b></b>Last</p>", "FirstLast"},
	}
	for _, tc := range cases {
		if got := convertMarkdownForTest(t, tc.html); got != tc.want {
			t.Errorf("convertMarkdownForTest(%q) = %q, want %q", tc.html, got, tc.want)
		}
	}
}

func TestMoodleHTMLToMarkdownPreservesBoundary(t *testing.T) {
	out, err := moodleHTMLToMarkdown("<p>First<strong> </strong>Last</p>")
	if err != nil {
		t.Fatalf("moodleHTMLToMarkdown: %v", err)
	}
	if strings.TrimSpace(out) != "First Last" {
		t.Fatalf("got %q, want %q", strings.TrimSpace(out), "First Last")
	}
}

func TestSitemapHTMLToMarkdownPreservesBoundary(t *testing.T) {
	out, err := sitemapHTMLToMarkdown([]byte("<p>First<strong> </strong>Last</p>"))
	if err != nil {
		t.Fatalf("sitemapHTMLToMarkdown: %v", err)
	}
	if out != "First Last" {
		t.Fatalf("got %q, want %q", out, "First Last")
	}
}
