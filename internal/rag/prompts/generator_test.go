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

package prompts

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
)

func TestGetValue(t *testing.T) {
	d := map[string]any{"chunk_id": "c1", "content_with_weight": "w", "url": nil}
	// Primary key present (nil value returned as-is, not skipped).
	if got := GetValue(d, "url", "other"); got != nil {
		t.Errorf("GetValue(url,other) = %v, want nil", got)
	}
	// Primary absent -> fallback.
	if got := GetValue(d, "id", "chunk_id"); got != "c1" {
		t.Errorf("GetValue(id,chunk_id) = %v, want c1", got)
	}
	// Both absent -> nil.
	if got := GetValue(d, "no1", "no2"); got != nil {
		t.Errorf("GetValue(no1,no2) = %v, want nil", got)
	}
	// Nil map -> nil.
	if got := GetValue(nil, "a", "b"); got != nil {
		t.Errorf("GetValue(nil) = %v, want nil", got)
	}
}

func TestChunksFormat(t *testing.T) {
	// Nil and non-dict-ish inputs yield an empty (non-nil) slice.
	if got := ChunksFormat(nil); len(got) != 0 {
		t.Errorf("ChunksFormat(nil) = %v, want empty", got)
	}
	if got := ChunksFormat(map[string]any{"chunks": "not-a-list"}); len(got) != 0 {
		t.Errorf("ChunksFormat(bad chunks) = %v, want empty", got)
	}

	ref := map[string]any{
		"chunks": []any{
			map[string]any{
				"chunk_id":          "abc",
				"content":           "body",
				"doc_id":            "d1",
				"docnm_kwd":         "doc.txt",
				"kb_id":             "kb1",
				"img_id":            "img1",
				"position_int":      3,
				"doc_type_kwd":      "pdf",
				"similarity":        0.9,
				"document_metadata": map[string]any{"page": "1"},
			},
			// Second chunk exercises the primary-name fallback path only where
			// the canonical primary name is absent.
			map[string]any{
				"id":                  "xyz",
				"content_with_weight": "alt-body",
			},
			"not-a-dict", // must be skipped
		},
	}
	got := ChunksFormat(ref)
	if len(got) != 2 {
		t.Fatalf("ChunksFormat len = %d, want 2", len(got))
	}

	first := got[0]
	wantFirst := map[string]any{
		"id":                "abc",
		"content":           "body",
		"document_id":       "d1",
		"document_name":     "doc.txt",
		"dataset_id":        "kb1",
		"image_id":          "img1",
		"positions":         3,
		"url":               nil,
		"similarity":        0.9,
		"vector_similarity": nil,
		"term_similarity":   nil,
		"row_id":            nil,
		"doc_type":          "pdf",
		"document_metadata": map[string]any{"page": "1"},
	}
	if !reflect.DeepEqual(first, wantFirst) {
		t.Errorf("first chunk = %#v, want %#v", first, wantFirst)
	}

	second := got[1]
	wantSecond := map[string]any{
		"id":                "xyz",
		"content":           "alt-body",
		"document_id":       nil,
		"document_name":     nil,
		"dataset_id":        nil,
		"image_id":          nil,
		"positions":         nil,
		"url":               nil,
		"similarity":        nil,
		"vector_similarity": nil,
		"term_similarity":   nil,
		"row_id":            nil,
		"doc_type":          nil,
		"document_metadata": nil,
	}
	if !reflect.DeepEqual(second, wantSecond) {
		t.Errorf("second chunk = %#v, want %#v", second, wantSecond)
	}
}

func TestCitationPromptCarriesIllustrativeIDCaveat(t *testing.T) {
	// rag/prompts/generator.py::citation_prompt appends this after rendering
	// CITATION_PROMPT_TEMPLATE; without it the model can echo the examples' IDs.
	got := CitationPrompt("")
	if !strings.Contains(got, "illustrative only") {
		t.Errorf("citation prompt missing the illustrative-ID caveat; got tail %q", got[len(got)-80:])
	}
	if !strings.Contains(got, "actual chunk IDs") {
		t.Errorf("citation prompt missing the chunk-ID instruction: %q", got[:120])
	}
	if strings.TrimSpace(got) != got {
		t.Errorf("citation prompt has surrounding whitespace")
	}
}

func TestCitationPromptHonoursOverride(t *testing.T) {
	// Python generator.py:227-228 renders the override (citation_guidelines)
	// template and STILL appends the illustrative-IDs caveat unconditionally —
	// the suffix is concatenated after whatever template rendered, so the
	// override replaces the template but never the caveat.
	want := "Cite as [n]." + citationIDSuffix
	if got := CitationPrompt("Cite as [n]."); got != want {
		t.Errorf("CitationPrompt(override) = %q, want override + caveat %q", got, want)
	}
}

// TestKBPromptSkipsChunksWithoutContent pins Python's `if not c: continue`: a
// chunk whose content fields are absent (or explicitly null) must be skipped,
// not rendered as "\n└── Content:\n<nil>".
func TestKBPromptSkipsChunksWithoutContent(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "a"},                              // neither content field
		{"chunk_id": "b", "content": nil},              // explicit null
		{"chunk_id": "c", "content_with_weight": "hi"}, // the only usable chunk
	}
	got := KBPrompt(chunks, 1000)
	if len(got) != 1 {
		t.Fatalf("blocks = %d, want 1 (content-less chunks must be skipped): %v", len(got), got)
	}
	if strings.Contains(got[0], "<nil>") {
		t.Errorf("block rendered a nil field: %q", got[0])
	}
	if !strings.Contains(got[0], "hi") {
		t.Errorf("block lost its content: %q", got[0])
	}
}

// TestKBPromptSkipsNilMetadataValues pins Python draw_node's `if not line: return
// ""` for None: a null metadata value must not render as "<nil>".
func TestKBPromptSkipsNilMetadataValues(t *testing.T) {
	got := KBPrompt([]map[string]any{{
		"content":           "body",
		"document_metadata": map[string]any{"author": nil, "page": "3"},
	}}, 1000)
	if len(got) != 1 {
		t.Fatalf("blocks = %d, want 1", len(got))
	}
	if strings.Contains(got[0], "<nil>") {
		t.Errorf("nil metadata rendered literally: %q", got[0])
	}
	if !strings.Contains(got[0], "page: 3") {
		t.Errorf("non-nil metadata lost: %q", got[0])
	}
}

// TestKBPromptBudgetsCompleteBlock pins that the budget covers the title / url /
// metadata decoration, not just the content: a tiny-content chunk whose block is
// dominated by metadata must be dropped when it cannot fit the token budget.
func TestKBPromptBudgetsCompleteBlock(t *testing.T) {
	lean := map[string]any{"content": "hi"}
	fat := map[string]any{
		"content":           "hi",
		"document_metadata": map[string]any{"blob": strings.Repeat("x", 500)},
	}
	leanBlock, _ := kbpBlock(lean, 1)
	fatBlock, _ := kbpBlock(fat, 1)
	leanTokens := tokenizer.NumTokensFromString(leanBlock)
	fatTokens := tokenizer.NumTokensFromString(fatBlock)
	if leanTokens == 0 || fatTokens <= leanTokens {
		t.Fatalf("cl100k tokenizer unavailable: lean=%d fat=%d tokens", leanTokens, fatTokens)
	}
	// A budget that just fits the lean block must reject the metadata-heavy one.
	maxTokens := int(float64(leanTokens)/0.97) + 1
	if got := KBPrompt([]map[string]any{fat}, maxTokens); len(got) != 0 {
		t.Errorf("metadata-heavy block (%d tokens) must not fit a ~%d-token budget; got %d block(s)",
			fatTokens, maxTokens, len(got))
	}
	if got := KBPrompt([]map[string]any{lean}, maxTokens); len(got) != 1 {
		t.Errorf("lean block (%d tokens) should fit a ~%d-token budget; got %d block(s)",
			leanTokens, maxTokens, len(got))
	}
}

// TestKBPromptWithSourceIndicesTracksRenderedChunks locks the contract callers
// need to slice blocks safely: every rendered block reports the index of the
// chunk it came from, and a chunk with no content contributes no block. A caller
// that slices by a chunk count instead would shift by the skipped chunks.
func TestKBPromptWithSourceIndicesTracksRenderedChunks(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "c0", "content": ""}, // skipped: no content
		{"chunk_id": "c1", "content": "first body"},
		nil, // skipped: not a chunk
		{"chunk_id": "c2", "content": "second body"},
	}

	blocks, sources := KBPromptWithSourceIndices(chunks, 100000)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (only the chunks with content)", len(blocks))
	}
	want := []int{1, 3}
	if !reflect.DeepEqual(sources, want) {
		t.Fatalf("sources = %v, want %v", sources, want)
	}
	for i, src := range sources {
		if got := chunks[src]["chunk_id"]; got != nil && !strings.Contains(blocks[i], "Content:") {
			t.Errorf("block %d (source %v) is not a rendered chunk: %q", i, got, blocks[i])
		}
	}
	// KBPrompt keeps the same rendering, just without the source indices.
	if plain := KBPrompt(chunks, 100000); !reflect.DeepEqual(plain, blocks) {
		t.Fatalf("KBPrompt = %v, want the same blocks as KBPromptWithSourceIndices", plain)
	}
}
