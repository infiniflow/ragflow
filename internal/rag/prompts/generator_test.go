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
	if got := CitationPrompt("Cite as [n]."); got != "Cite as [n]." {
		t.Errorf("CitationPrompt(override) = %q, want the override verbatim", got)
	}
}
