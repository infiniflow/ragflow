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

package chunker

import (
	"context"
	"testing"
)

// tagChunksOf drives TagChunker.Invoke and returns the emitted chunk maps.
func tagChunksOf(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewTagChunker(nil)
	if err != nil {
		t.Fatalf("NewTagChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("TagChunker.Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// tagKwdOf reads the tag_kwd field from a chunk map. After the
// ChunkDoc -> JSON -> map round-trip, the string slice arrives as
// []any, so we normalise it here.
func tagKwdOf(t *testing.T, m map[string]any) []string {
	t.Helper()
	raw, ok := m["tag_kwd"]
	if !ok {
		t.Fatalf("tag_kwd missing: %v", m)
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("tag_kwd not []any: %T", raw)
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("tag_kwd entry not string: %T", v)
		}
		out = append(out, s)
	}
	return out
}

// TestTagChunker_TextTabDelimited ports tag.py's txt path: one chunk per
// content+tags row, content_with_weight = content, tag_kwd = comma-split
// (dots sanitsed) tags.
func TestTagChunker_TextTabDelimited(t *testing.T) {
	chunks := tagChunksOf(t, map[string]any{
		"name":          "tags.txt",
		"output_format": "text",
		"text":          "what is ragflow\tRAG,LLM\nhow to deploy\tGUIDE,ops.example",
	})
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}

	c0 := chunks[0]
	if got := c0["content_with_weight"]; got != "what is ragflow" {
		t.Errorf("chunk0 content_with_weight = %q", got)
	}
	tags0 := tagKwdOf(t, c0)
	if len(tags0) != 2 || tags0[0] != "RAG" || tags0[1] != "LLM" {
		t.Errorf("chunk0 tag_kwd = %v, want [RAG LLM]", tags0)
	}

	c1 := chunks[1]
	tags1 := tagKwdOf(t, c1)
	// "ops.example" -> "ops_example": dots are sanitsed for keyword storage.
	if len(tags1) != 2 || tags1[0] != "GUIDE" || tags1[1] != "ops_example" {
		t.Errorf("chunk1 tag_kwd = %v, want [GUIDE ops_example]", tags1)
	}
}

// TestTagChunker_TextCommaDelimited verifies comma wins when the file has
// no tab-structured rows (mirrors tag.py:detectDelimiter).
func TestTagChunker_TextCommaDelimited(t *testing.T) {
	chunks := tagChunksOf(t, map[string]any{
		"name":          "tags.csv",
		"output_format": "text",
		"text":          "what is ragflow,RAG\nhow to deploy,GUIDE",
	})
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}
	if got := chunks[0]["content_with_weight"]; got != "what is ragflow" {
		t.Errorf("chunk0 content_with_weight = %q", got)
	}
	tags0 := tagKwdOf(t, chunks[0])
	if len(tags0) != 1 || tags0[0] != "RAG" {
		t.Errorf("chunk0 tag_kwd = %v, want [RAG]", tags0)
	}
}

// TestTagChunker_HTMLTable ports the spreadsheet (xlsx/csv -> html table)
// path: first two <td> columns become content + tags.
func TestTagChunker_HTMLTable(t *testing.T) {
	html := "<table><tr><td>q1</td><td>a1,b1</td></tr><tr><td>q2</td><td>a2</td></tr></table>"
	chunks := tagChunksOf(t, map[string]any{
		"name":          "tags.xlsx",
		"output_format": "html",
		"html":          html,
	})
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}
	if got := chunks[0]["content_with_weight"]; got != "q1" {
		t.Errorf("chunk0 content_with_weight = %q", got)
	}
	tags0 := tagKwdOf(t, chunks[0])
	if len(tags0) != 2 || tags0[0] != "a1" || tags0[1] != "b1" {
		t.Errorf("chunk0 tag_kwd = %v, want [a1 b1]", tags0)
	}
}
