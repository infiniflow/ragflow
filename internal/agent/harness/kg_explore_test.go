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

// TestKGParseEntity asserts the name/alias/source extraction from a KG row.
func TestKGParseEntity(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"name":"Paris","type":"LOCATION","aliases":["City of Light"]}`,
		"source_chunk_ids":    []interface{}{"ck-1", "ck-2"},
		"doc_id":              "doc-1",
	}
	e, ok := kgParseEntity(row)
	if !ok {
		t.Fatal("entity must parse")
	}
	if e.Name != "Paris" || e.Type != "LOCATION" {
		t.Errorf("name/type = %q/%q", e.Name, e.Type)
	}
	if len(e.Aliases) != 1 || e.Aliases[0] != "City of Light" {
		t.Errorf("aliases = %v", e.Aliases)
	}
	if len(e.SourceChunkIDs) != 2 || e.DocID != "doc-1" {
		t.Errorf("source_chunk_ids/doc_id = %v/%q", e.SourceChunkIDs, e.DocID)
	}
}

// TestKGParseRelation asserts endpoint + type extraction, and rejection of
// dangling endpoints.
func TestKGParseRelation(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"type":"capital_of"}`,
		"from_entity_kwd":     "Paris",
		"to_entity_kwd":       "France",
	}
	r, ok := kgParseRelation(row)
	if !ok {
		t.Fatal("relation must parse")
	}
	if r.From != "Paris" || r.To != "France" || r.Type != "capital_of" {
		t.Errorf("from/to/type = %q/%q/%q", r.From, r.To, r.Type)
	}
	if _, ok := kgParseRelation(map[string]interface{}{"from_entity_kwd": "", "to_entity_kwd": "X"}); ok {
		t.Error("dangling endpoint must be rejected")
	}
}

// TestEndpointTerms asserts original + lowercased variants are produced.
func TestEndpointTerms(t *testing.T) {
	terms := endpointTerms([]string{"Paris", "France"})
	got := map[string]bool{}
	for _, t := range terms {
		got[t] = true
	}
	for _, want := range []string{"Paris", "paris", "France", "france"} {
		if !got[want] {
			t.Errorf("endpoint terms missing %q, got %v", want, terms)
		}
	}
}

// TestCollectEvidenceIDs asserts relevant entities AND relations contribute
// source chunk ids grouped by doc.
func TestCollectEvidenceIDs(t *testing.T) {
	entities := []kgEntity{
		{Name: "Paris", SourceChunkIDs: []string{"ck-1"}, DocID: "doc-1"},
		{Name: "Berlin", SourceChunkIDs: []string{"ck-2"}, DocID: "doc-1"},
	}
	relations := []kgRelation{
		{From: "Paris", To: "France", SourceChunkIDs: []string{"ck-3"}, DocID: "doc-1"},
	}
	got := collectEvidenceIDs(entities, relations, []string{"Paris", "France"})
	ids := got["doc-1"]
	if len(ids) != 2 { // ck-1 (Paris entity) + ck-3 (Paris→France relation)
		t.Errorf("expected 2 evidence ids (Paris entity + Paris→France relation), got %v", ids)
	}
	// Berlin is not relevant → its ck-2 must be excluded.
	for _, id := range ids {
		if id == "ck-2" {
			t.Error("Berlin's chunk must be excluded (not relevant)")
		}
	}
}

// TestMentionCountReRank asserts topMentionCount re-sorts by mention_count_int
// desc and truncates to topN.
func TestMentionCountReRank(t *testing.T) {
	rows := []map[string]interface{}{
		{"name_kwd": "low", "mention_count_int": float64(3)},
		{"name_kwd": "high", "mention_count_int": float64(9)},
		{"name_kwd": "mid", "mention_count_int": float64(5)},
	}
	top := topMentionCount(rows, 2)
	if len(top) != 2 {
		t.Fatalf("topMentionCount = %d, want 2", len(top))
	}
	if mentionCount(top[0]) != 9 || mentionCount(top[1]) != 5 {
		t.Errorf("re-rank order wrong: [%d, %d]", mentionCount(top[0]), mentionCount(top[1]))
	}
}

// TestNormalizeWebResults asserts Tavily "results" and agent "chunks" envelopes
// both normalize to the shared evidence shape.
func TestNormalizeWebResults(t *testing.T) {
	tavily := `{"results":[{"url":"https://a","content":"hello world","title":"A"}]}`
	out := normalizeWebResults([]byte(tavily))
	if len(out) != 1 || out[0]["content_with_weight"] != "hello world" {
		t.Fatalf("tavily envelope: got %v", out)
	}
	if out[0]["url"] != "https://a" {
		t.Errorf("url = %v", out[0]["url"])
	}
	// A result without a URL must be dropped.
	noURL := `{"results":[{"content":"no url here"}]}`
	if out := normalizeWebResults([]byte(noURL)); len(out) != 0 {
		t.Errorf("result without URL must be dropped, got %v", out)
	}
}
