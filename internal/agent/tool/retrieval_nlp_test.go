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

// retrieval_nlp_test.go — NLPRetrievalAdapter tests.
//
// The adapter's Search method calls into the real
// nlp.RetrievalService, which requires a doc engine + document
// DAO. Those require network access and aren't worth standing up
// in a unit test. Instead we exercise the translation layer
// (translateChunk + helpers) directly — that's the surface that
// differs between the nlp chunk shape and the agent-tool chunk
// shape, and it's the only piece the adapter owns.

package tool

import (
	"context"
	"math"
	"testing"
)

// floatEqual compares two floats with a small epsilon so
// arithmetic like 0.4+0.8/2 == 0.6000000000000001 doesn't fail.
func floatEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestTranslateChunk_FullFields: every chunk field is present and
// maps to the expected RetrievalChunk field.
func TestTranslateChunk_FullFields(t *testing.T) {
	raw := map[string]any{
		"chunk_id":            "ck-42",
		"doc_id":              "doc-7",
		"docnm_kwd":           "report.pdf",
		"kb_id":               "kb-1",
		"content_with_weight": "the answer is 42",
		"content_ltks":        "answer 42",
		"similarity":          0.87,
		"term_similarity":     0.5,
		"vector_similarity":   0.9,
	}
	got := translateChunk(raw)
	if got.ID != "ck-42" {
		t.Errorf("ID = %q, want \"ck-42\"", got.ID)
	}
	if got.Content != "the answer is 42" {
		t.Errorf("Content = %q, want content_with_weight", got.Content)
	}
	if got.DocumentID != "doc-7" {
		t.Errorf("DocumentID = %q, want \"doc-7\"", got.DocumentID)
	}
	if got.Score != 0.87 {
		t.Errorf("Score = %v, want 0.87 (similarity preferred)", got.Score)
	}
}

// TestTranslateChunk_ContentFallback: when content_with_weight is
// empty, content_ltks is used.
func TestTranslateChunk_ContentFallback(t *testing.T) {
	raw := map[string]any{
		"chunk_id":     "ck-1",
		"content_ltks": "answer 42",
		"doc_id":       "doc-1",
		"similarity":   0.5,
	}
	got := translateChunk(raw)
	if got.Content != "answer 42" {
		t.Errorf("Content = %q, want content_ltks fallback", got.Content)
	}
}

// TestTranslateChunk_EmptyContent: both content fields empty →
// empty string (don't crash, don't synthesise).
func TestTranslateChunk_EmptyContent(t *testing.T) {
	raw := map[string]any{
		"chunk_id":   "ck-1",
		"doc_id":     "doc-1",
		"similarity": 0.5,
	}
	got := translateChunk(raw)
	if got.Content != "" {
		t.Errorf("Content = %q, want \"\"", got.Content)
	}
}

// TestTranslateChunk_ScoreFallback: when "similarity" is missing,
// the average of term_similarity + vector_similarity is used.
func TestTranslateChunk_ScoreFallback(t *testing.T) {
	raw := map[string]any{
		"chunk_id":          "ck-1",
		"term_similarity":   0.4,
		"vector_similarity": 0.8,
	}
	got := translateChunk(raw)
	if !floatEqual(got.Score, 0.6) {
		t.Errorf("Score = %v, want ~0.6 (avg of term+vec)", got.Score)
	}
}

// TestTranslateChunk_ScoreOnlyOneSub: only one of term_similarity
// or vector_similarity present → that one wins (not averaged
// against zero, which would be misleading).
func TestTranslateChunk_ScoreOnlyOneSub(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want float64
	}{
		{"term only", map[string]any{"term_similarity": 0.7}, 0.7},
		{"vector only", map[string]any{"vector_similarity": 0.3}, 0.3},
	}
	for _, tc := range cases {
		got := translateChunk(tc.raw)
		if !floatEqual(got.Score, tc.want) {
			t.Errorf("%s: Score = %v, want %v", tc.name, got.Score, tc.want)
		}
	}
}

// TestTranslateChunk_NumberTypesTolerated: numeric similarity
// fields may come back as float64, float32, int, or int64 depending
// on how the upstream serialised them. All should coerce.
func TestTranslateChunk_NumberTypesTolerated(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want float64
	}{
		{"float64", float64(0.5), 0.5},
		{"float32", float32(0.5), 0.5},
		{"int", int(1), 1},
		{"int64", int64(1), 1},
	}
	for _, tc := range cases {
		raw := map[string]any{"similarity": tc.val}
		got := translateChunk(raw)
		if !floatEqual(got.Score, tc.want) {
			t.Errorf("%s: Score = %v, want %v", tc.name, got.Score, tc.want)
		}
	}
}

// TestTranslateChunk_WrongTypesIgnored: a similarity field that's
// a string or nil must not crash. We fall back to the sub-scores
// (which are also missing in this test → zero).
func TestTranslateChunk_WrongTypesIgnored(t *testing.T) {
	cases := []map[string]any{
		{"similarity": "0.5"},
		{"similarity": nil},
		{"similarity": []any{0.5}},
	}
	for _, raw := range cases {
		got := translateChunk(raw)
		if got.Score != 0 {
			t.Errorf("wrong-type similarity: Score = %v, want 0", got.Score)
		}
	}
}

// TestTranslateChunk_WrongStringTypesIgnored: a string field
// that's actually a number must not crash.
func TestTranslateChunk_WrongStringTypesIgnored(t *testing.T) {
	raw := map[string]any{
		"chunk_id": 42,  // int, not string
		"doc_id":   nil, // nil
	}
	got := translateChunk(raw)
	if got.ID != "" {
		t.Errorf("ID = %q, want \"\"", got.ID)
	}
	if got.DocumentID != "" {
		t.Errorf("DocumentID = %q, want \"\"", got.DocumentID)
	}
}

// TestTranslateChunk_MissingAllScores: a chunk with no score
// fields at all → score 0 (don't panic).
func TestTranslateChunk_MissingAllScores(t *testing.T) {
	raw := map[string]any{"chunk_id": "ck-1"}
	got := translateChunk(raw)
	if got.Score != 0 {
		t.Errorf("Score = %v, want 0", got.Score)
	}
}

// TestNewNLPRetrievalAdapter_NilService: nil constructor inputs
// must produce an adapter whose Search returns the missing-service
// error, not a panic.
func TestNewNLPRetrievalAdapter_NilService(t *testing.T) {
	a := NewNLPRetrievalAdapter(nil)
	_, err := a.Search(context.TODO(), RetrievalRequest{Query: "hi"})
	if err == nil {
		t.Fatal("expected error from nil-service adapter")
	}
	if err != ErrRetrievalServiceMissing {
		t.Errorf("err = %v, want ErrRetrievalServiceMissing", err)
	}
}

func TestNLPRetrievalAdapter_ResolveTenantIDsStaysWithinRequestTenant(t *testing.T) {
	a := &NLPRetrievalAdapter{}
	got := a.resolveTenantIDs(RetrievalRequest{
		TenantID:   "tenant-a",
		DatasetIDs: []string{"kb-1", "kb-2", "kb-missing"},
	})

	if len(got) != 1 {
		t.Fatalf("tenantIDs len=%d want 1, got=%v", len(got), got)
	}
	if got[0] != "tenant-a" {
		t.Fatalf("tenantIDs=%v want [tenant-a]", got)
	}
}
