// Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package parser

import (
	"testing"
)

// unifiedHTMLCases encodes the browser-faithful HTML parsing semantics that
// BOTH the Go and Python parsers must converge on. Each case wraps its content
// in a single block element so that ParseWithResult emits exactly one item and
// the Python merge_block_text emits exactly one block string, enabling a 1:1
// byte comparison between engines.
//
// This is the RED phase: every case currently FAILS on at least one engine,
// exposing the divergences (Go drops <br> and preserves raw whitespace; Python
// inserts an inline space and strips <pre>). The Python mirror lives in
// test/unit_test/deepdoc/parser/test_html_parser.py::test_unified_html_semantics_parity
// and MUST stay in sync with this table.
var unifiedHTMLCases = []struct {
	name string
	html string
	want string
}{
	// <br> is a hard line break; surrounding whitespace is collapsed away.
	{"br_basic", "<p>line1<br>line2</p>", "line1\nline2"},
	{"br_surrounding_space", "<p>Hello <br> World</p>", "Hello\nWorld"},
	{"br_double", "<p>A<br><br>B</p>", "A\n\nB"},
	{"br_before_inline", "<p>Line1<br><span>Line2</span></p>", "Line1\nLine2"},
	// Inline boundaries are joined verbatim: no separator is inserted, even
	// when the source has no whitespace (Latin or CJK).
	{"inline_no_space_latin", "<p>Hello<b>World</b></p>", "HelloWorld"},
	{"inline_no_space_cjk", "<p>你好<b>世界</b></p>", "你好世界"},
	{"inline_with_space", "<p>Hello <b>World</b></p>", "Hello World"},
	{"inline_three", "<p>First<b>Second</b>Third</p>", "FirstSecondThird"},
	// Source whitespace sequences collapse to a single space and are trimmed
	// at block edges (CSS whitespace folding).
	{"whitespace_collapse", "<p>\n  Hello\n  <b>World</b>\n</p>", "Hello World"},
	// <pre> preserves its whitespace verbatim, including leading/trailing.
	{"pre_preserved", "<pre>  code\n  block</pre>", "  code\n  block"},
}

// TestHTMLParser_ParseWithResult_UnifiedSemantics asserts the browser-faithful
// semantics on the Go engine. It is expected to FAIL on the current code
// (Go drops <br>, keeps raw whitespace, and TrimSpace's <pre>).
func TestHTMLParser_ParseWithResult_UnifiedSemantics(t *testing.T) {
	for _, tc := range unifiedHTMLCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewHTMLParser()
			res := p.ParseWithResult(t.Context(), "doc.html", []byte(tc.html))
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if len(res.JSON) != 1 {
				t.Fatalf("block count = %d, want 1: %#v", len(res.JSON), res.JSON)
			}
			if got := res.JSON[0]["text"].(string); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
