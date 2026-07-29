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
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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

func TestChunkTypesDecrementChunkStats(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })

	status := string(entity.StatusValid)
	if err := dao.DB.Create(&entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		Name:         "kb-1",
		EmbdID:       "embed",
		Permission:   string(entity.TenantPermissionMe),
		CreatedBy:    "user-1",
		ParserConfig: entity.JSONMap{},
		TokenNum:     20,
		ChunkNum:     5,
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{},
		SourceType:   string(entity.FileSourceLocal),
		Type:         "txt",
		CreatedBy:    "user-1",
		TokenNum:     10,
		ChunkNum:     3,
		Suffix:       ".txt",
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	svc := &ChunkService{}
	if err := svc.decrementChunkStats("doc-1", "kb-1", 0, 2, 0); err != nil {
		t.Fatalf("decrementChunkStats() error = %v", err)
	}

	var doc entity.Document
	if err := dao.DB.First(&doc, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc.TokenNum != 10 || doc.ChunkNum != 1 {
		t.Fatalf("document stats token=%d chunk=%d, want token=10 chunk=1", doc.TokenNum, doc.ChunkNum)
	}
	var kb entity.Knowledgebase
	if err := dao.DB.First(&kb, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("get kb: %v", err)
	}
	if kb.TokenNum != 20 || kb.ChunkNum != 3 {
		t.Fatalf("knowledgebase stats token=%d chunk=%d, want token=20 chunk=3", kb.TokenNum, kb.ChunkNum)
	}

	if err := svc.decrementChunkStats("doc-1", "kb-1", 30, 30, -1); err != nil {
		t.Fatalf("decrementChunkStats() clamp error = %v", err)
	}
	if err := dao.DB.First(&doc, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("get doc after clamp: %v", err)
	}
	if doc.TokenNum != 0 || doc.ChunkNum != 0 || doc.ProcessDuration != 0 {
		t.Fatalf("document clamped stats token=%d chunk=%d duration=%v, want zeros", doc.TokenNum, doc.ChunkNum, doc.ProcessDuration)
	}
	if err := dao.DB.First(&kb, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("get kb after clamp: %v", err)
	}
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("knowledgebase clamped stats token=%d chunk=%d, want zeros", kb.TokenNum, kb.ChunkNum)
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

func TestGetPageNum_Nil(t *testing.T) {
	if got := common.CoalesceInt(nil, 10); got != 10 {
		t.Errorf("CoalesceInt(nil, 10) = %d, want 10", got)
	}
}

func TestGetPageNum_ZeroReturnsDefault(t *testing.T) {
	val := 0
	if got := common.CoalesceInt(&val, 5); got != 5 {
		t.Errorf("CoalesceInt(&0, 5) = %d, want 5", got)
	}
}

func TestGetPageNum_NegativeReturnsDefault(t *testing.T) {
	val := -1
	if got := common.CoalesceInt(&val, 5); got != 5 {
		t.Errorf("CoalesceInt(&-1, 5) = %d, want 5", got)
	}
}

func TestGetPageNum_Valid(t *testing.T) {
	val := 3
	if got := common.CoalesceInt(&val, 5); got != 3 {
		t.Errorf("CoalesceInt(&3, 5) = %d, want 3", got)
	}
}

func TestGetPageSize_Nil(t *testing.T) {
	if got := common.CoalesceInt(nil, 20); got != 20 {
		t.Errorf("CoalesceInt(nil, 20) = %d, want 20", got)
	}
}

func TestGetPageSize_ZeroReturnsDefault(t *testing.T) {
	val := 0
	if got := common.CoalesceInt(&val, 20); got != 20 {
		t.Errorf("CoalesceInt(&0, 20) = %d, want 20", got)
	}
}

func TestGetPageSize_Valid(t *testing.T) {
	val := 50
	if got := common.CoalesceInt(&val, 20); got != 50 {
		t.Errorf("CoalesceInt(&50, 20) = %d, want 50", got)
	}
}

func TestIsInternalField(t *testing.T) {
	tests := []struct {
		field string
		want  bool
	}{
		{"vector_vec", true},
		{"content_sm_ltks", true},
		{"content_ltks", true},
		{"content", false},
		{"docnm_kwd", false},
		{"important_kwd", false},
		{"knowledge_graph_kwd", false},
	}
	for _, tt := range tests {
		if got := isInternalField(tt.field); got != tt.want {
			t.Errorf("isInternalField(%q) = %v, want %v", tt.field, got, tt.want)
		}
	}
}

func TestSplitKwdHash(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"non-string int", 42},
		{"no hash", "hello world"},
		{"simple hash", "a###b###c"},
		{"hash with empty", "a######b"},
		{"hash only", "###"},
	}
	for _, tt := range tests {
		_ = splitKwdHash(tt.input)
	}

	// Verify actual output
	result := splitKwdHash("a###b###c")
	slice, ok := result.([]interface{})
	if !ok {
		t.Errorf("expected []interface{}, got %T", result)
		return
	}
	if len(slice) != 3 {
		t.Errorf("expected 3 elements, got %d", len(slice))
	}

	// Non-string returns unchanged
	if splitKwdHash(42) != 42 {
		t.Error("non-string should return unchanged")
	}
}

func TestApplyCommonChunkMapping(t *testing.T) {
	result := make(map[string]interface{})

	// content -> content_with_weight
	if !applyCommonChunkMapping(result, "content", "hello") {
		t.Error("content should be handled")
	}
	if result["content_with_weight"] != "hello" {
		t.Errorf("content_with_weight = %v, want hello", result["content_with_weight"])
	}

	// docnm -> docnm_kwd
	result = make(map[string]interface{})
	applyCommonChunkMapping(result, "docnm", "mydoc")
	if result["docnm_kwd"] != "mydoc" {
		t.Errorf("docnm_kwd = %v, want mydoc", result["docnm_kwd"])
	}

	// important_keywords -> important_kwd
	result = make(map[string]interface{})
	applyCommonChunkMapping(result, "important_keywords", []interface{}{"kw1"})
	if _, ok := result["important_kwd"]; !ok {
		t.Error("important_kwd should be set")
	}

	// *_kwd empty handling
	result = make(map[string]interface{})
	applyCommonChunkMapping(result, "entity_kwd", "")
	if _, ok := result["entity_kwd"]; !ok {
		t.Error("entity_kwd should be set even for empty")
	}

	// Unknown field should not be handled
	result = make(map[string]interface{})
	if applyCommonChunkMapping(result, "unknown_field", "val") {
		t.Error("unknown_field should not be handled")
	}
	if len(result) != 0 {
		t.Error("result should be empty for unhandled field")
	}
}
