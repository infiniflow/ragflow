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

func memChunk(content, docID, chunkID string) map[string]any {
	return map[string]any{"content": content, "doc_id": docID, "chunk_id": chunkID}
}

// Python memory.search returns the chunks that share enough of the query's
// significant terms; unrelated queries return nothing.
func TestMemorySearchReturnsRelevant(t *testing.T) {
	kb := &Kbinfos{Memory: []map[string]any{
		memChunk("Rifampicin is an antibiotic used to treat tuberculosis.", "d1", "c1"),
		memChunk("The Eiffel Tower is a wrought-iron lattice tower in Paris.", "d2", "c2"),
	}}
	// "rifampicin" / "tuberculosis" both hit chunk c1 only.
	hits := MemorySearch(kb, "What is rifampicin used for in tuberculosis treatment?", 0, 0)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0]["chunk_id"] != "c1" {
		t.Errorf("top hit chunk_id = %v, want c1", hits[0]["chunk_id"])
	}
	if _, ok := hits[0]["similarity"]; !ok {
		t.Errorf("hit missing similarity field")
	}

	// Unrelated query clears nothing.
	if got := MemorySearch(kb, "How many moons does Jupiter have?", 0, 0); got != nil {
		t.Errorf("unrelated query should return nil, got %d hits", len(got))
	}
}

// The normalized overlap bar must drop a chunk that matches too few of a long
// query's terms (mirrors Python's (hits/_n) >= min_ratio guard).
func TestMemorySearchRatioBar(t *testing.T) {
	kb := &Kbinfos{Memory: []map[string]any{
		memChunk("Rifampicin is an antibiotic.", "d1", "c1"),
	}}
	// Query has many significant terms (9) but the chunk shares only
	// "rifampicin" -> 1/9 ~= 0.111 < the 0.12 default ratio, so it clears
	// nothing.
	q := "rifampicin airplane bicycle coffee mountain ocean planet quarter river"
	if got := MemorySearch(kb, q, 0, 0); got != nil {
		t.Errorf("below-ratio query should return nil, got %d hits", len(got))
	}

	// Lowering the ratio lets the same single-term hit through.
	if got := MemorySearch(kb, q, 0, 0.1); len(got) != 1 {
		t.Errorf("ratio 0.1 should keep the hit, got %d", len(got))
	}
}

// CJK queries match via character 3-grams, just like the Python implementation.
func TestMemorySearchCJKTrigram(t *testing.T) {
	kb := &Kbinfos{Memory: []map[string]any{
		memChunk("利福平是一种抗生素，用于治疗结核病。", "d1", "c1"),
		memChunk("巴黎的埃菲尔铁塔是著名的铁制建筑。", "d2", "c2"),
	}}
	hits := MemorySearch(kb, "利福平治疗什么疾病？", 0, 0)
	if len(hits) != 1 || hits[0]["chunk_id"] != "c1" {
		t.Errorf("CJK 3-gram search failed: got %d hits, want 1 (c1)", len(hits))
	}
}

// Ranking: more term hits ranks higher; equal hits then prefer longer text.
func TestMemorySearchRanking(t *testing.T) {
	kb := &Kbinfos{Memory: []map[string]any{
		memChunk("Rifampicin treats tuberculosis and leprosy.", "d1", "low"),
		memChunk("Rifampicin is an antibiotic for tuberculosis and also used for leprosy and meningitis.", "d2", "high"),
	}}
	hits := MemorySearch(kb, "rifampicin tuberculosis leprosy", 0, 0)
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0]["chunk_id"] != "high" {
		t.Errorf("top ranked chunk_id = %v, want high (more hits / longer)", hits[0]["chunk_id"])
	}
}

// chunkText must prefer "content_with_weight" over "content", mirroring Python
// harness/chunk_utils._chunk_text (content_with_weight or content or text), so a
// chunk carrying both yields the weighted text.
func TestChunkTextPrefersWeighted(t *testing.T) {
	c := map[string]any{
		"content":             "primary text",
		"content_with_weight": "weighted text",
	}
	if got := chunkText(c); got != "weighted text" {
		t.Errorf("chunkText = %q, want %q (Python prefers content_with_weight)", got, "weighted text")
	}

	// Falls back to content when content_with_weight is absent/empty.
	c2 := map[string]any{"content": "primary only"}
	if got := chunkText(c2); got != "primary only" {
		t.Errorf("chunkText = %q, want %q", got, "primary only")
	}
}
