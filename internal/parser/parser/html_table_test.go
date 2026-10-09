//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package parser

import "testing"

// TestIsTableOpeningTag pins the predicate's prefix semantics: it reports
// whether the block opens with the "<table" text, exactly as the markdown
// parser has always classified its HTML blocks. It is deliberately loose (a
// "<tableau>" token passes), which is why routing never treats it as the final
// word — the row walker decides.
func TestIsTableOpeningTag(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"<table>", true},
		{"  <TABLE class=\"x\">", true},
		{"<table-x>", true}, // historical prefix semantics: "<table" prefix only
		{"<tableau>", true},
		{"<div><table>", false},
		{"<tr><td>q</td><td>a</td></tr>", false},
		{"<tbody><tr><td>q</td></tr></tbody>", false},
		{"plain text", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsTableOpeningTag(tc.text); got != tc.want {
			t.Errorf("IsTableOpeningTag(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

// TestLooksLikeTableHTMLCoversWhatTheWalkerReads guards the routing invariant: the
// candidate filter must never deny a payload the row walker can read, or a
// readable table would silently fall through to the prose extractor.
func TestLooksLikeTableHTMLCoversWhatTheWalkerReads(t *testing.T) {
	texts := []string{
		"<table><tr><th>q</th><th>a</th></tr></table>",
		"<table><caption>c</caption>\n<tr><td>q</td><td>a</td></tr>\n</table>\n",
		"<tr><td>q</td><td>a</td></tr>",                                            // bare fragment
		"<tr><td>q</td><td>a</td></tr><tr><td>q2</td><td>a2</td></tr>",             // bare rows
		"<tbody><tr><td>q</td><td>a</td></tr></tbody>",                             // rows wrapped in tbody
		"<table><tr><td>q<table><tr><td>inner</td></tr></table></td></tr></table>", // nested
		"<table><template><tr><td>x</td></tr></template></table>",                  // template rows are inert
		"<table></table>", // nothing to read
		"plain text",
		"",
	}
	for _, text := range texts {
		if len(HTMLTableRows(text)) == 0 {
			continue
		}
		if !LooksLikeTableHTML(text) {
			t.Errorf("HTMLTableRows can read %q but LooksLikeTableHTML denies it", text)
		}
	}
}
