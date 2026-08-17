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

package harness

import "testing"

// TestExtractCJKSubstrings_NoPanic guards the RE2-compatible CJK regex: the
// previous `\u4e00` escape would panic at compile time (review fix R14).
func TestExtractCJKSubstrings_NoPanic(t *testing.T) {
	got := extractCJKSubstrings("巴黎是法国的首都，位于欧洲。")
	if len(got) == 0 {
		t.Fatal("CJK substrings must be extracted (and must not panic)")
	}
	for _, s := range got {
		if !isCJK(s) {
			t.Errorf("extracted %q is not CJK", s)
		}
	}
}

// TestExtractNamedEntities_LatinNil asserts Latin text yields nil (NER delegated
// to the LLM grounded review).
func TestExtractNamedEntities_LatinNil(t *testing.T) {
	if got := extractNamedEntities("Paris is the capital"); got != nil {
		t.Errorf("Latin text must yield nil (NER delegated), got %v", got)
	}
}

// TestParseToolCall_Multiline asserts a multi-line generate_report body parses
// (review fix R13: the (?s) flag makes `.` span newlines).
func TestParseToolCall_Multiline(t *testing.T) {
	text := `<tool_call>{"name": "generate_report", "arguments": {
		"report": "Paris has 2 million people",
		"is_verified": true,
		"confidence": 0.9,
		"evidence_ids": [0, 3],
		"gaps": []
	}}</tool_call>`
	call := parseToolCall(text)
	if call == nil {
		t.Fatal("multi-line tool call must parse")
	}
	if call["name"] != "generate_report" {
		t.Errorf("name = %v, want generate_report", call["name"])
	}
	args, _ := call["arguments"].(map[string]interface{})
	if args["report"] != "Paris has 2 million people" {
		t.Errorf("report = %v", args["report"])
	}
}

// TestParseToolCall_Fence asserts the fenced-JSON path still works.
func TestParseToolCall_Fence(t *testing.T) {
	text := "```json\n{\"name\":\"hybrid_search\",\"arguments\":{\"query\":\"rocket\"}}\n```"
	call := parseToolCall(text)
	if call == nil || call["name"] != "hybrid_search" {
		t.Fatalf("fenced tool call = %v", call)
	}
}

// TestNormalizeWebResults_UniqueChunkID asserts several snippets from the SAME
// URL get distinct chunk_id values so Kbinfos.Merge does not collapse them
// (review fix R11).
func TestNormalizeWebResults_UniqueChunkID(t *testing.T) {
	raw := []byte(`{"results":[
		{"url":"https://a","content":"snippet one","title":"A"},
		{"url":"https://a","content":"snippet two","title":"A"}
	]}`)
	out := normalizeWebResults(raw)
	if len(out) != 2 {
		t.Fatalf("normalizeWebResults = %d results, want 2 (distinct snippets)", len(out))
	}
	if chunkIDOf(out[0]) == chunkIDOf(out[1]) {
		t.Errorf("chunk_id collision: %q == %q", chunkIDOf(out[0]), chunkIDOf(out[1]))
	}
	// doc_id stays the URL.
	if docIDOf(out[0]) != "https://a" || docIDOf(out[1]) != "https://a" {
		t.Errorf("doc_id must stay the URL, got %q / %q", docIDOf(out[0]), docIDOf(out[1]))
	}
}

// TestChunkText_TextFallback asserts the "text" field is used when neither
// content_with_weight nor content is present (review fix: _evidence_md parity).
func TestChunkText_TextFallback(t *testing.T) {
	if got := chunkText(map[string]interface{}{"text": "plain text field"}); got != "plain text field" {
		t.Errorf("chunkText(text) = %q, want the text field", got)
	}
	// content_with_weight still wins over text.
	if got := chunkText(map[string]interface{}{"content_with_weight": "cw", "text": "plain"}); got != "cw" {
		t.Errorf("chunkText must prefer content_with_weight, got %q", got)
	}
}
