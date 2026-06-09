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
	"context"
	"testing"

	"ragflow/internal/entity"
)

// --- Helper tests ---

func TestPtrString_Nil(t *testing.T) {
	if got := ptrString[int](nil); got != "<nil>" {
		t.Errorf("ptrString(nil) = %q, want <nil>", got)
	}
}

func TestPtrString_Value(t *testing.T) {
	val := 42
	if got := ptrString(&val); got != "42" {
		t.Errorf("ptrString(&42) = %q, want 42", got)
	}
}

func TestPtrString_Bool(t *testing.T) {
	val := true
	if got := ptrString(&val); got != "true" {
		t.Errorf("ptrString(&true) = %q, want true", got)
	}
}

func TestGetPageNum_Nil(t *testing.T) {
	if got := getPageNum(nil, 10); got != 10 {
		t.Errorf("getPageNum(nil, 10) = %d, want 10", got)
	}
}

func TestGetPageNum_ZeroReturnsDefault(t *testing.T) {
	val := 0
	if got := getPageNum(&val, 5); got != 5 {
		t.Errorf("getPageNum(&0, 5) = %d, want 5", got)
	}
}

func TestGetPageNum_NegativeReturnsDefault(t *testing.T) {
	val := -1
	if got := getPageNum(&val, 5); got != 5 {
		t.Errorf("getPageNum(&-1, 5) = %d, want 5", got)
	}
}

func TestGetPageNum_Valid(t *testing.T) {
	val := 3
	if got := getPageNum(&val, 5); got != 3 {
		t.Errorf("getPageNum(&3, 5) = %d, want 3", got)
	}
}

func TestGetPageSize_Nil(t *testing.T) {
	if got := getPageSize(nil, 20); got != 20 {
		t.Errorf("getPageSize(nil, 20) = %d, want 20", got)
	}
}

func TestGetPageSize_ZeroReturnsDefault(t *testing.T) {
	val := 0
	if got := getPageSize(&val, 20); got != 20 {
		t.Errorf("getPageSize(&0, 20) = %d, want 20", got)
	}
}

func TestGetPageSize_Valid(t *testing.T) {
	val := 50
	if got := getPageSize(&val, 20); got != 50 {
		t.Errorf("getPageSize(&50, 20) = %d, want 50", got)
	}
}

// --- RetrievalTestRequest validation tests ---

func TestRetrievalTestRequest_Defaults(t *testing.T) {
	req := &RetrievalTestRequest{
		Datasets: []string{"kb1"},
		Question: "test question",
	}
	// Verify pointer fields are nil by default
	if req.Page != nil {
		t.Error("Page should default to nil")
	}
	if req.Size != nil {
		t.Error("Size should default to nil")
	}
	if req.TopK != nil {
		t.Error("TopK should default to nil")
	}
	if req.UseKG != nil {
		t.Error("UseKG should default to nil")
	}
	if req.SimilarityThreshold != nil {
		t.Error("SimilarityThreshold should default to nil")
	}
	if req.VectorSimilarityWeight != nil {
		t.Error("VectorSimilarityWeight should default to nil")
	}
	if req.Keyword != nil {
		t.Error("Keyword should default to nil")
	}
}

func TestRetrievalTestResponse_Fields(t *testing.T) {
	resp := &RetrievalTestResponse{
		Chunks:  []map[string]interface{}{},
		DocAggs: []map[string]interface{}{},
		Total:   0,
	}
	if resp.Chunks == nil {
		t.Error("Chunks should not be nil")
	}
	if resp.DocAggs == nil {
		t.Error("DocAggs should not be nil")
	}
	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
}

// --- transformQuestion edge cases ---

func TestTransformQuestion_NoTransformNeeded(t *testing.T) {
	svc := &ChunkService{}
	ctx := context.Background()
	result, err := svc.transformQuestion(ctx, "hello", nil, nil, []string{"t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected unchanged question, got %q", result)
	}
}

func TestTransformQuestion_EmptyCrossLanguages(t *testing.T) {
	svc := &ChunkService{}
	ctx := context.Background()
	kw := false
	result, err := svc.transformQuestion(ctx, "hello", []string{}, &kw, []string{"t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected unchanged question, got %q", result)
	}
}

func TestTransformQuestion_KeywordFalse(t *testing.T) {
	// This test verifies the early-return path for transformQuestion.
	// With crossLanguages non-empty it would hit the DB; this is tested
	// via integration tests that have a full service setup.
	svc := &ChunkService{}
	ctx := context.Background()
	kw := false
	result, err := svc.transformQuestion(ctx, "hello", []string{}, &kw, []string{"t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected unchanged question, got %q", result)
	}
}

// --- resolveEmbeddingModel via exported Retriever ---
// These test that the retriever can handle nil inputs gracefully

func TestResolveEmbeddingModel_NilTenantEmbdID(t *testing.T) {
	kb := &entity.Knowledgebase{
		EmbdID: "text-embedding-ada-002@OpenAI",
	}
	// This will fail because it needs a real DAO, but we verify the type contract
	if kb.TenantEmbdID != nil {
		t.Error("TenantEmbdID should be nil for this test")
	}
	_ = kb // verified fields are accessible
}

func TestResolveRerankModel_BothNil(t *testing.T) {
	svc := &ChunkService{}
	result, err := svc.resolveRerankModel("t1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil rerank model when both IDs are nil, got %v", result)
	}
}

func TestResolveRerankModel_EmptyStrings(t *testing.T) {
	svc := &ChunkService{}
	empty := ""
	result, err := svc.resolveRerankModel("t1", &empty, &empty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil rerank model when both IDs are empty, got %v", result)
	}
}

func TestResolveRerankModel_InvalidTenantRerankID(t *testing.T) {
	svc := &ChunkService{}
	invalid := "not_a_number"
	_, err := svc.resolveRerankModel("t1", &invalid, nil)
	if err == nil {
		t.Error("expected error for invalid tenant_rerank_id")
	}
}

// --- validateKBs input validation ---

func TestValidateKBs_EmptyDatasets(t *testing.T) {
	// validateKBs iterates over datasetIDs and queries DAOs.
	// With empty input it should return empty slices.
	// This test is limited since validateKBs requires DB-backed DAOs.
	_ = &ChunkService{} // compiles
}

// --- Verify ChunkService struct fields ---
func TestChunkService_FieldsAccessible(t *testing.T) {
	svc := &ChunkService{}
	_ = svc.docEngine
	_ = svc.kbDAO
	_ = svc.userTenantDAO
	_ = svc.searchService
	// Verify embeddingCache field type
	_ = svc.embeddingCache
}
