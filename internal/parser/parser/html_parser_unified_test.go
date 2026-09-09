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
	_ "embed"
	"encoding/json"
	"testing"
)

// unifiedHTMLCasesJSON is the single source of truth for the browser-faithful
// HTML parsing semantics that BOTH the Go and Python parsers must converge on.
// It is embedded from testdata/unified_html_cases.json, which the Python mirror
// (test/unit_test/deepdoc/parser/test_html_parser.py) also loads — so the two
// engines share one fixture and can no longer drift.
//
// Each case wraps its content in a single block element so that
// ParseWithResult emits exactly one item and the Python merge_block_text emits
// exactly one block string, enabling a 1:1 byte comparison between engines.
//
//go:embed testdata/unified_html_cases.json
var unifiedHTMLCasesJSON []byte

type unifiedHTMLCase struct {
	Name string `json:"name"`
	HTML string `json:"html"`
	Want string `json:"want"`
}

func loadUnifiedHTMLCases(t *testing.T) []unifiedHTMLCase {
	t.Helper()
	var cases []unifiedHTMLCase
	if err := json.Unmarshal(unifiedHTMLCasesJSON, &cases); err != nil {
		t.Fatalf("unmarshal unified html cases: %v", err)
	}
	return cases
}

// TestHTMLParser_ParseWithResult_UnifiedSemantics asserts the browser-faithful
// semantics on the Go engine. The cases are loaded from the shared embedded
// fixture, so this test and its Python mirror stay in lockstep.
func TestHTMLParser_ParseWithResult_UnifiedSemantics(t *testing.T) {
	for _, tc := range loadUnifiedHTMLCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			p := NewHTMLParser()
			res := p.ParseWithResult(t.Context(), "doc.html", []byte(tc.HTML))
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if len(res.JSON) != 1 {
				t.Fatalf("block count = %d, want 1: %#v", len(res.JSON), res.JSON)
			}
			if got := res.JSON[0]["text"].(string); got != tc.Want {
				t.Errorf("got %q, want %q", got, tc.Want)
			}
		})
	}
}

// TestHTMLParser_ParseWithResult_RealisticSmoke exercises the Go HTML walker on
// a realistic multi-block document: a heading, a paragraph with an inline
// <b> and a <br> line break, a CJK paragraph with an inline element, and a
// verbatim <pre> block. It guards the leafWriter CSS-folding rewrite against
// hidden regressions specific to the Go reimplementation:
//   - <br> becomes a hard line break;
//   - inline boundaries join verbatim, with NO inserted space even for CJK;
//   - block-internal whitespace collapses to a single space and is trimmed;
//   - <pre> keeps its source whitespace verbatim (leading/trailing included).
func TestHTMLParser_ParseWithResult_RealisticSmoke(t *testing.T) {
	const html = `<h1>产品说明 Product Guide</h1>
<p>第一步：打开应用<br>第二步：点击<b>设置</b>按钮完成配置。</p>
<p>欢迎使用我们的<b>智能助手</b>，它能帮您快速处理任务。</p>
<pre>  code
  block</pre>`
	p := NewHTMLParser()
	res := p.ParseWithResult(t.Context(), "doc.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var texts []string
	for _, item := range res.JSON {
		texts = append(texts, item["text"].(string))
	}
	want := []string{
		// Heading: whitespace folded, no injected break.
		"产品说明 Product Guide",
		// <br> => hard break; inline <b> joined verbatim (无空格).
		"第一步：打开应用\n第二步：点击设置按钮完成配置。",
		// CJK inline joined verbatim: 我们的 + 智能助手, no space.
		"欢迎使用我们的智能助手，它能帮您快速处理任务。",
		// <pre> preserved verbatim, leading/trailing whitespace intact.
		"  code\n  block",
	}
	if len(texts) != len(want) {
		t.Fatalf("block count = %d, want %d; got %#v", len(texts), len(want), texts)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Errorf("block %d: got %q, want %q", i, texts[i], want[i])
		}
	}
}
