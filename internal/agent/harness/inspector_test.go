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

func sampleChunks() []map[string]interface{} {
	return []map[string]interface{}{
		{"chunk_id": "c0", "content_with_weight": "alpha", "doc_id": "d1"},
		{"chunk_id": "c1", "content_with_weight": "beta", "doc_id": "d1"},
		{"chunk_id": "c2", "content_with_weight": "gamma", "doc_id": "d1"},
		{"chunk_id": "c3", "content_with_weight": "delta", "doc_id": "d2"},
		{"chunk_id": "c4", "content_with_weight": "epsilon", "doc_id": "d2"},
	}
}

// TestInspectorOpenContext asserts the chunk plus 2 neighbours on each side
// (Python slice chunks[idx-2:idx+2] is right-open, so 4 chunks: idx-2..idx+1).
func TestInspectorOpenContext(t *testing.T) {
	out := InspectorOpenContext(sampleChunks(), "c2")
	if len(out) != 4 { // c0..c3 (idx-2..idx+1)
		t.Fatalf("open_context around c2 = %d chunks, want 4", len(out))
	}
	if chunkIDOf(out[2]) != "c2" {
		t.Errorf("middle chunk = %s, want c2", chunkIDOf(out[2]))
	}
	// Near the boundary, clamp.
	if out := InspectorOpenContext(sampleChunks(), "c0"); len(out) != 2 {
		t.Errorf("open_context around c0 = %d, want 2 (clamped)", len(out))
	}
}

// TestInspectorCompareSources asserts only requested ids are returned.
func TestInspectorCompareSources(t *testing.T) {
	out := InspectorCompareSources(sampleChunks(), []string{"c1", "c3"})
	if len(out) != 2 || chunkIDOf(out[0]) != "c1" || chunkIDOf(out[1]) != "c3" {
		t.Errorf("compare_sources = %v", out)
	}
}

// TestInspectorGrepWithin asserts chunks are narrowed to keyword sentences and
// copies never mutate the original. Keywords are comma-separated (≥3 to avoid
// the space-bigram fallback).
func TestInspectorGrepWithin(t *testing.T) {
	chunks := []map[string]interface{}{
		{"chunk_id": "c0", "content_with_weight": "Paris has 2 million people. It is in France. The Eiffel Tower is famous.", "doc_id": "d1"},
		{"chunk_id": "c1", "content_with_weight": "Tokyo has 14 million people. It is in Japan.", "doc_id": "d1"},
	}
	orig := chunks[0]["content_with_weight"].(string)
	out := InspectorGrepWithin(chunks, "d1", "france, eiffel, tower")
	if len(out) != 1 {
		t.Fatalf("grep_within = %d chunks, want 1 (only c0 has France/Eiffel)", len(out))
	}
	if got := chunkText(out[0]); !contains(got, "*France*") {
		t.Errorf("narrowed content must highlight France, got %q", got)
	}
	// Original must be untouched.
	if chunks[0]["content_with_weight"].(string) != orig {
		t.Error("grep_within must not mutate the shared chunk pool")
	}
}

// TestInspectorRequestAdjacent asserts next/prev direction and count clamp.
func TestInspectorRequestAdjacent(t *testing.T) {
	next := InspectorRequestAdjacent(sampleChunks(), "c1", "next", 2)
	if len(next) != 2 || chunkIDOf(next[0]) != "c2" || chunkIDOf(next[1]) != "c3" {
		t.Errorf("request_adjacent next = %v", next)
	}
	prev := InspectorRequestAdjacent(sampleChunks(), "c4", "prev", 10)
	if len(prev) != 4 { // c0..c3, clamped
		t.Errorf("request_adjacent prev = %d, want 4 (clamped)", len(prev))
	}
}

// TestKeywordList asserts comma vs space-bigram parsing.
func TestKeywordList(t *testing.T) {
	// >=3 comma terms → kept as-is.
	if got := keywordList("a, b, c"); len(got) != 3 {
		t.Errorf("comma list = %v", got)
	}
	// <3 comma terms → space bigrams.
	if got := keywordList("alpha beta"); len(got) != 1 || got[0] != "alpha beta" {
		t.Errorf("space bigram = %v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
