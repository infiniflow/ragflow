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

package service

import (

	"testing"
)

func TestNewSourcedChunks_Empty(t *testing.T) {
	result := NewSourcedChunks(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
	result = NewSourcedChunks([]map[string]interface{}{})
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestNewSourcedChunks_NilEntry(t *testing.T) {
	result := NewSourcedChunks([]map[string]interface{}{nil, {"id": "c1"}})
	if len(result) != 1 {
		t.Errorf("expected 1 (nil skipped), got %d", len(result))
	}
}

func TestNewSourcedChunks_PrimaryKeys(t *testing.T) {
	raw := []map[string]interface{}{{
		"chunk_id":            "abc",
		"content_with_weight": "hello world",
		"doc_id":              "doc1",
		"docnm_kwd":           "My Doc",
		"kb_id":               "kb1",
		"image_id":            "img1",
		"positions":           "1-10",
		"url":                 "http://example.com",
		"similarity":          0.95,
		"vector_similarity":   0.87,
		"term_similarity":     0.03,
		"doc_type_kwd":        "pdf",
		"document_metadata":   map[string]interface{}{"author": "test"},
	}}
	result := NewSourcedChunks(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	r := result[0]
	if r.ID != "abc" {
		t.Errorf("ID = %q, want abc", r.ID)
	}
	if r.Content != "hello world" {
		t.Errorf("Content = %q", r.Content)
	}
	if r.DocName != "My Doc" {
		t.Errorf("DocName = %q", r.DocName)
	}
	if r.Similarity != 0.95 {
		t.Errorf("Similarity = %f", r.Similarity)
	}
	if r.DocumentMetadata == nil {
		t.Error("DocumentMetadata should not be nil")
	}
}

func TestNewSourcedChunks_FallbackKeys(t *testing.T) {
	raw := []map[string]interface{}{{
		"id":            "fallback-id",
		"content":       "fallback content",
		"document_id":   "fallback-doc",
		"document_name": "fallback-name",
		"dataset_id":    "fallback-kb",
		"img_id":        "fallback-img",
		"position_int":  "11-20",
		"doc_type":      "markdown",
	}}
	result := NewSourcedChunks(raw)
	r := result[0]
	if r.ID != "fallback-id" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Content != "fallback content" {
		t.Errorf("Content = %q", r.Content)
	}
	if r.DocType != "markdown" {
		t.Errorf("DocType = %q", r.DocType)
	}
}

func TestNewSourcedChunks_EmptyStringSkipped(t *testing.T) {
	raw := []map[string]interface{}{{
		"chunk_id":            "",
		"id":                  "",
		"content_with_weight": "",
	}}
	result := NewSourcedChunks(raw)
	r := result[0]
	if r.ID != "" {
		t.Errorf("expected empty ID for empty primary+fallback, got %q", r.ID)
	}
}

func TestChunksFormat_Empty(t *testing.T) {
	if got := ChunksFormat(nil); len(got) != 0 {
		t.Errorf("expected empty for nil, got %d", len(got))
	}
	if got := ChunksFormat([]SourcedChunk{}); len(got) != 0 {
		t.Errorf("expected empty for empty slice, got %d", len(got))
	}
}

func TestChunksFormat_FieldMapping(t *testing.T) {
	ck := SourcedChunk{
		ID:               "abc",
		Content:          "hello world",
		DocID:            "doc1",
		DocName:          "My Doc",
		DatasetID:        "kb1",
		ImageID:          "img1",
		Positions:        "1-10",
		URL:              "http://x.com",
		Similarity:       0.95,
		VectorSimilarity: 0.87,
		TermSimilarity:   0.03,
		DocType:          "pdf",
		DocumentMetadata: map[string]interface{}{"author": "test"},
	}
	result := ChunksFormat([]SourcedChunk{ck})
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	r := result[0]
	if r["id"] != "abc" {
		t.Errorf("id = %v", r["id"])
	}
	if r["content"] != "hello world" {
		t.Errorf("content = %v", r["content"])
	}
	if r["document_name"] != "My Doc" {
		t.Errorf("document_name = %v", r["document_name"])
	}
	if r["row_id"] != "abc" {
		t.Errorf("row_id = %v", r["row_id"])
	}
}

func TestKbPrompt_Empty(t *testing.T) {
	if got := KbPrompt(nil, 100); got != "" {
		t.Errorf("expected empty for nil chunks")
	}
	if got := KbPrompt([]SourcedChunk{}, 100); got != "" {
		t.Errorf("expected empty for empty chunks")
	}
	if got := KbPrompt([]SourcedChunk{{Content: "x"}}, 0); got != "" {
		t.Errorf("expected empty for maxTokens=0")
	}
	if got := KbPrompt([]SourcedChunk{{Content: "x"}}, -1); got != "" {
		t.Errorf("expected empty for maxTokens=-1")
	}
}

func TestKbPrompt_Format(t *testing.T) {
	chunks := []SourcedChunk{{
		ID:      "abc",
		Content: "chunk content here",
		DocName: "Test Document",
		URL:     "http://example.com",
	}}
	result := KbPrompt(chunks, 10000)
	if result == "" {
		t.Fatal("expected non-empty prompt")
	}
	// Verify ID appears
	if !contains(result, "ID: abc") {
		t.Errorf("missing ID line: %s", result)
	}
	// Verify title
	if !contains(result, "Title: Test Document") {
		t.Errorf("missing title: %s", result)
	}
	// Verify URL
	if !contains(result, "URL: http://example.com") {
		t.Errorf("missing URL: %s", result)
	}
	// Verify content
	if !contains(result, "chunk content here") {
		t.Errorf("missing content: %s", result)
	}
	// Verify unicode box-drawing chars
	if !contains(result, "├──") {
		t.Errorf("missing tree drawing: %s", result)
	}
}

func TestKbPrompt_TokenLimit(t *testing.T) {
	chunks := []SourcedChunk{
		{ID: "1", Content: "a very long content that takes many tokens "},
		{ID: "2", Content: "second chunk content here"},
	}
	// Tight limit: first chunk ~31 tokens (limit=48 tokens at 0.97 ratio).
	// Second chunk ~25 tokens — excluded.
	result := KbPrompt(chunks, 50)
	if !contains(result, "ID: 1") {
		t.Error("first chunk should be included")
	}
	if contains(result, "ID: 2") {
		t.Error("second chunk should be excluded under tight limit")
	}
}

func TestKbPrompt_DocMetadata(t *testing.T) {
	chunks := []SourcedChunk{{
		ID:      "abc",
		Content: "content",
		DocumentMetadata: map[string]interface{}{
			"author": "test author",
			"year":   "2024",
		},
	}}
	result := KbPrompt(chunks, 10000)
	if !contains(result, "author: test author") {
		t.Errorf("missing metadata author: %s", result)
	}
	if !contains(result, "year: 2024") {
		t.Errorf("missing metadata year: %s", result)
	}
}

func TestKbPrompt_NoDocNameOrURL(t *testing.T) {
	chunks := []SourcedChunk{{
		ID:      "simple",
		Content: "plain content",
	}}
	result := KbPrompt(chunks, 10000)
	if contains(result, "Title:") {
		t.Error("should not have title when empty")
	}
	if contains(result, "URL:") {
		t.Error("should not have URL when empty")
	}
}

func TestGetStr_MultipleKeys(t *testing.T) {
	m := map[string]interface{}{"b": "value"}
	if getStr(m, "a", "b") != "value" {
		t.Error("should prefer primary, fall back to secondary")
	}
	if getStr(m, "x", "y") != "" {
		t.Error("should return empty for missing keys")
	}
	emptyMap := map[string]interface{}{"e": ""}
	if getStr(emptyMap, "e") != "" {
		t.Error("should return empty for empty string")
	}
}

func TestGetFloat_Types(t *testing.T) {
	m := map[string]interface{}{
		"f64": float64(3.14),
		"f32": float32(1.5),
		"i":   42,
		"i64": int64(100),
	}
	if getFloat(m, "f64") != 3.14 {
		t.Error("float64 failed")
	}
	if getFloat(m, "f32") != 1.5 {
		t.Error("float32 failed")
	}
	if getFloat(m, "i") != 42 {
		t.Error("int failed")
	}
	if getFloat(m, "i64") != 100 {
		t.Error("int64 failed")
	}
	if getFloat(m, "missing") != 0 {
		t.Error("missing should return 0")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSourcedChunk_ZeroValue(t *testing.T) {
	var ck SourcedChunk
	if ck.ID != "" {
		t.Error("zero SourcedChunk should have empty ID")
	}
	if ck.Similarity != 0 {
		t.Error("zero SourcedChunk should have zero Similarity")
	}
	if ck.DocumentMetadata != nil {
		t.Error("zero SourcedChunk should have nil DocumentMetadata")
	}
}

func TestNewSourcedChunks_RoundTrip(t *testing.T) {
	original := []SourcedChunk{{
		ID:               "id1",
		Content:          "content1",
		DocID:            "doc1",
		DocName:          "doc name",
		DatasetID:        "kb1",
		ImageID:          "img1",
		Positions:        "1-5",
		URL:              "http://x",
		Similarity:       0.9,
		VectorSimilarity: 0.8,
		TermSimilarity:   0.1,
		DocType:          "pdf",
		DocumentMetadata: map[string]interface{}{"k": "v"},
	}}
	formatted := ChunksFormat(original)
	roundTripped := NewSourcedChunks(formatted)
	if len(roundTripped) != len(original) {
		t.Fatalf("round trip length mismatch: %d vs %d", len(roundTripped), len(original))
	}
	r := roundTripped[0]
	if r.ID != original[0].ID {
		t.Error("ID mismatch after round trip")
	}
	if r.Content != original[0].Content {
		t.Error("Content mismatch after round trip")
	}
	if r.DocName != original[0].DocName {
		t.Error("DocName mismatch after round trip")
	}
}

func TestNumTokensFromString_Empty(t *testing.T) {
	if got := NumTokensFromString(""); got != 0 {
		t.Errorf("expected 0 for empty string, got %d", got)
	}
}

func TestNumTokensFromString_Fallback(t *testing.T) {
	// With tokenizer unavailable, fallback to rune count / 2.
	s := "hello world"
	got := NumTokensFromString(s)
	expected := len([]rune(s)) / 2 // 5
	if got != expected {
		t.Errorf("got %d, want %d (fallback)", got, expected)
	}
}

func TestNumTokensFromString_Chinese(t *testing.T) {
	s := "你好世界"
	got := NumTokensFromString(s)
	expected := len([]rune(s)) / 2 // 2
	if got != expected {
		t.Errorf("got %d, want %d (Chinese fallback)", got, expected)
	}
}

func TestKbPrompt_TokenLimitAccurate(t *testing.T) {
	// Verify truncation uses NumTokensFromString, not byte length.
	chunks := []SourcedChunk{
		{ID: "1", Content: "hello"},   // ~10 runes → 5 tokens + overhead
		{ID: "2", Content: "world"},   // ~5 tokens
	}
	// With maxTokens=20, limit=19→ first fits, second doesn't.
	result := KbPrompt(chunks, 20)
	if !contains(result, "ID: 1") {
		t.Error("first chunk should fit under 20 token limit")
	}
	if contains(result, "ID: 2") {
		t.Errorf("second chunk should be excluded: result = %q", result)
	}
}

func TestKbPrompt_AllFit(t *testing.T) {
	chunks := []SourcedChunk{
		{ID: "1", Content: "a"},
		{ID: "2", Content: "b"},
	}
	result := KbPrompt(chunks, 1000)
	if !contains(result, "ID: 1") && !contains(result, "ID: 2") {
		t.Error("both chunks should fit under generous limit")
	}
}

func TestGetMap_NilAndMissing(t *testing.T) {
	if getMap(nil, "x") != nil {
		t.Error("nil map should return nil")
	}
	if getMap(map[string]interface{}{}, "x") != nil {
		t.Error("missing key should return nil")
	}
	if getMap(map[string]interface{}{"x": "not a map"}, "x") != nil {
		t.Error("wrong type should return nil")
	}
}
