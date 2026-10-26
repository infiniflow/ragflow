//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

// skillSearchEngine is a minimal engine.DocEngine stub for skill search tests.
// It embeds engine.DocEngine (all methods nil) and overrides only Search and
// ChunkStoreExists, the two methods SkillSearchService touches.
type skillSearchEngine struct {
	engine.DocEngine

	indexExists bool
	// searchResults is returned by Search when searchErr is nil.
	searchResults []map[string]interface{}
	searchErr     error
	// searchCalls records every SearchRequest seen by Search.
	searchCalls []*types.SearchRequest
}

// ChunkStoreExists reports whether the skill index exists.
func (e *skillSearchEngine) ChunkStoreExists(_ context.Context, _ string, _ string) (bool, error) {
	return e.indexExists, nil
}

// Search records the request and returns the configured search results.
func (e *skillSearchEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	snapshot := *req
	e.searchCalls = append(e.searchCalls, &snapshot)
	if e.searchErr != nil {
		return nil, e.searchErr
	}
	return &types.SearchResult{Chunks: e.searchResults, Total: int64(len(e.searchResults))}, nil
}

// setupSkillSearchTenant wires an in-memory DB for user "owner" resolving to
// its own tenant via the default ("default") skill space path.
//
// The tenant ID is intentionally == the user ID ("owner"). In CE the resolver
// walks the owner-role membership below; in EE the resolver returns the user's
// personal tenant whose ID is the user ID. Keeping them equal makes the test
// pass under either policy.
func setupSkillSearchTenant(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.SkillSearchConfig{}); err != nil {
		t.Fatalf("migrate skill search config: %v", err)
	}
	pushServiceDB(t, db)

	active := string(entity.StatusValid)
	if err := db.Create(&entity.Tenant{ID: "owner", Name: sptr("owner")}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID: "skill-search-membership", UserID: "owner", TenantID: "owner",
		Role: "owner", Status: &active,
	}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
}

// saveSkillSearchConfig inserts an active config row for owner/default.
func saveSkillSearchConfig(t *testing.T, config *entity.SkillSearchConfig) {
	t.Helper()
	config.TenantID = "owner"
	config.SpaceID = normalizeSpaceID(config.SpaceID)
	config.Status = "1"
	if config.ID == "" {
		config.ID = common.GenerateUUID()
	}
	if err := dao.DB.Create(config).Error; err != nil {
		t.Fatalf("create config: %v", err)
	}
}

// lastSkillConfigID returns the active config row saved by the test.
func lastSkillConfigID(t *testing.T) string {
	t.Helper()
	var config entity.SkillSearchConfig
	if err := dao.DB.Where("tenant_id = ? AND space_id = ? AND status = ?", "owner", "default", "1").
		First(&config).Error; err != nil {
		t.Fatalf("load config: %v", err)
	}
	return config.ID
}

// esStyleChunk builds an ES-style search chunk with the given name and score.
func esStyleChunk(name string, score float64) map[string]interface{} {
	return map[string]interface{}{
		"skill_id":    "skill-" + name,
		"folder_id":   "folder-1",
		"name":        name,
		"description": name + " description",
		"tags":        "tag-a, tag-b",
		"create_time": float64(1700000000),
		"version":     "1.0.0",
		"_score":      score,
	}
}

// ---------------------------------------------------------------------------
// convertChunksToResults
// ---------------------------------------------------------------------------

// TestConvertChunksToResultsDedupesByNameKeepingHighestScore verifies that
// chunks with the same name are deduplicated, keeping the highest score.
func TestConvertChunksToResultsDedupesByNameKeepingHighestScore(t *testing.T) {
	service := &SkillSearchService{}
	chunks := []map[string]interface{}{
		esStyleChunk("web-search", 1.2),
		esStyleChunk("web-search", 2.5),
		esStyleChunk("pdf-reader", 3.0),
	}

	results := service.convertChunksToResults(chunks, 0)

	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 (dedup by name)", len(results))
	}
	if results[0].Name != "pdf-reader" || results[0].Score != 3.0 {
		t.Fatalf("top result = %+v, want pdf-reader with score 3.0", results[0])
	}
	byName := map[string]entity.SkillSearchResult{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if got := byName["web-search"].Score; got != 2.5 {
		t.Fatalf("web-search score = %v, want 2.5 (highest kept)", got)
	}
}

// TestConvertChunksToResultsFiltersBelowThreshold verifies that chunks whose
// score falls below the threshold are dropped.
func TestConvertChunksToResultsFiltersBelowThreshold(t *testing.T) {
	service := &SkillSearchService{}
	chunks := []map[string]interface{}{
		esStyleChunk("high", 0.9),
		esStyleChunk("low", 0.1),
	}

	results := service.convertChunksToResults(chunks, 0.2)

	if len(results) != 1 || results[0].Name != "high" {
		t.Fatalf("results = %+v, want only the chunk at/above threshold", results)
	}
}

// TestConvertChunksToResultsParsesInfinityScoreColumns verifies that the
// SCORE and SIMILARITY columns returned by Infinity are parsed, including
// float32 values that must widen to float64.
func TestConvertChunksToResultsParsesInfinityScoreColumns(t *testing.T) {
	service := &SkillSearchService{}
	// Infinity returns SCORE/SIMILARITY columns and may use float32 values.
	chunks := []map[string]interface{}{
		{
			"skill_id": "skill-1", "name": "infinity-skill", "folder_id": "f",
			"tags":        []interface{}{"alpha", "beta"},
			"create_time": int64(1700000000),
			"_score":      float64(0.8),
			"SCORE":       float32(0.5),
			"SIMILARITY":  float32(0.6),
		},
	}

	results := service.convertChunksToResults(chunks, 0)

	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	r := results[0]
	// float32 columns widen to float64, so compare with a tolerance.
	if math.Abs(r.BM25Score-0.5) > 1e-6 {
		t.Errorf("BM25Score = %v, want 0.5 from SCORE (float32)", r.BM25Score)
	}
	if math.Abs(r.VectorScore-0.6) > 1e-6 {
		t.Errorf("VectorScore = %v, want 0.6 from SIMILARITY (float32)", r.VectorScore)
	}
	if len(r.Tags) != 2 || r.Tags[0] != "alpha" || r.Tags[1] != "beta" {
		t.Errorf("Tags = %v, want [alpha beta] from array form", r.Tags)
	}
	if r.CreateTime != 1700000000 {
		t.Errorf("CreateTime = %v, want 1700000000 from int64 form", r.CreateTime)
	}
}

// TestConvertChunksToResultsTreatsScoreAsBM25WhenColumnsAbsent verifies that
// _score is used as the BM25 score when SCORE and SIMILARITY are absent.
func TestConvertChunksToResultsTreatsScoreAsBM25WhenColumnsAbsent(t *testing.T) {
	service := &SkillSearchService{}
	chunks := []map[string]interface{}{esStyleChunk("es-skill", 1.7)}

	results := service.convertChunksToResults(chunks, 0)

	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].BM25Score != 1.7 {
		t.Fatalf("BM25Score = %v, want _score fallback 1.7", results[0].BM25Score)
	}
	if results[0].VectorScore != 0 {
		t.Fatalf("VectorScore = %v, want 0", results[0].VectorScore)
	}
}

// TestConvertChunksToResultsFallsBackToSkillIDWhenNameEmpty verifies that
// deduplication falls back to skill_id when the name is empty.
func TestConvertChunksToResultsFallsBackToSkillIDWhenNameEmpty(t *testing.T) {
	service := &SkillSearchService{}
	chunks := []map[string]interface{}{
		{"skill_id": "skill-a", "_score": 0.4},
		{"skill_id": "skill-a", "_score": 0.9},
	}

	results := service.convertChunksToResults(chunks, 0)

	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (dedup falls back to skill_id)", len(results))
	}
	if results[0].Score != 0.9 {
		t.Fatalf("Score = %v, want 0.9", results[0].Score)
	}
}

// ---------------------------------------------------------------------------
// Search: strategy dispatch and fallbacks
// ---------------------------------------------------------------------------

// TestSearchReturnsEmptyWhenIndexMissing verifies that Search returns an empty
// result set without invoking the engine when the index does not exist.
func TestSearchReturnsEmptyWhenIndexMissing(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()
	eng := &skillSearchEngine{indexExists: false}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "anything"}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.Total != 0 || len(resp.Skills) != 0 {
		t.Fatalf("resp = %+v, want empty result set", resp)
	}
	if len(eng.searchCalls) != 0 {
		t.Fatalf("engine Search called %d times, want 0 when index is missing", len(eng.searchCalls))
	}
}

// TestSearchKeywordWhenNoEmbeddingConfigured verifies that Search takes the
// pure keyword branch when no embedding model is configured.
func TestSearchKeywordWhenNoEmbeddingConfigured(t *testing.T) {
	setupSkillSearchTenant(t)
	// No config row: defaults have EmbdID == "" so Search must take the pure
	// keyword branch without ever resolving an embedding model.
	service := NewSkillSearchService()
	eng := &skillSearchEngine{
		indexExists:   true,
		searchResults: []map[string]interface{}{esStyleChunk("kw-skill", 1.0)},
	}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "find"}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.SearchType != "keyword" {
		t.Fatalf("SearchType = %q, want keyword (no embd config)", resp.SearchType)
	}
	if resp.Total != 1 || resp.Skills[0].Name != "kw-skill" {
		t.Fatalf("resp = %+v, want the keyword hit", resp)
	}
	if len(eng.searchCalls) != 1 {
		t.Fatalf("engine Search called %d times, want 1", len(eng.searchCalls))
	}
	req := eng.searchCalls[0]
	textExpr, ok := req.MatchExprs[0].(*types.MatchTextExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] = %T, want *types.MatchTextExpr", req.MatchExprs[0])
	}
	if textExpr.MatchingText != "find" {
		t.Fatalf("MatchingText = %q, want the raw query", textExpr.MatchingText)
	}
}

// TestSearchKeywordOnZeroVectorWeight verifies that a zero vector weight
// selects the keyword branch even when an embedding model is configured.
func TestSearchKeywordOnZeroVectorWeight(t *testing.T) {
	setupSkillSearchTenant(t)
	// A struct Create would let GORM's default:0.3 swallow the zero weight,
	// so persist the config the way UpdateConfig does (map-based update).
	saveSkillSearchConfig(t, &entity.SkillSearchConfig{
		EmbdID: "embd-1", SimilarityThreshold: 0.1, TopK: 10,
	})
	if err := dao.NewSkillSearchConfigDAO().Update(t.Context(), dao.DB,
		lastSkillConfigID(t), map[string]interface{}{"vector_similarity_weight": 0}); err != nil {
		t.Fatalf("zero out vector weight: %v", err)
	}
	service := NewSkillSearchService()
	eng := &skillSearchEngine{
		indexExists:   true,
		searchResults: []map[string]interface{}{esStyleChunk("zero-weight", 0.5)},
	}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "find"}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.SearchType != "keyword" {
		t.Fatalf("SearchType = %q, want keyword when weight == 0 even with embd configured", resp.SearchType)
	}
}

// TestSearchEmptyQueryListsAllWithoutThreshold verifies that an empty query
// lists all skills with the threshold disabled and update_time desc ordering.
func TestSearchEmptyQueryListsAllWithoutThreshold(t *testing.T) {
	setupSkillSearchTenant(t)
	// Threshold 0.9 would drop everything if applied; empty query must
	// disable it so listing works.
	saveSkillSearchConfig(t, &entity.SkillSearchConfig{
		EmbdID:                 "embd-1",
		VectorSimilarityWeight: 0.5,
		SimilarityThreshold:    0.9,
		TopK:                   10,
	})
	service := NewSkillSearchService()
	eng := &skillSearchEngine{
		indexExists: true,
		searchResults: []map[string]interface{}{
			esStyleChunk("weak", 0.05),
			esStyleChunk("strong", 0.6),
		},
	}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: ""}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.SearchType != "keyword" {
		t.Fatalf("SearchType = %q, want keyword for empty query", resp.SearchType)
	}
	if resp.Total != 2 {
		t.Fatalf("Total = %d, want 2 (threshold disabled for empty query)", resp.Total)
	}
	// Empty query defaults to update_time desc ordering.
	req := eng.searchCalls[0]
	if req.OrderBy == nil || len(req.OrderBy.Fields) != 1 ||
		req.OrderBy.Fields[0].Field != "update_time" || req.OrderBy.Fields[0].Type != types.SortDesc {
		t.Fatalf("OrderBy = %+v, want update_time desc for empty query", req.OrderBy)
	}
}

// TestSearchHybridFallsBackToKeywordWhenEmbeddingUnavailable verifies that
// hybrid search falls back to keyword, reporting "keyword" and preserving the
// requested sort order.
func TestSearchHybridFallsBackToKeywordWhenEmbeddingUnavailable(t *testing.T) {
	setupSkillSearchTenant(t)
	// Weight strictly between 0 and 1 with a configured embd_id selects the
	// hybrid branch. The embedding model cannot be resolved from the test DB
	// (no tenant model rows), so Search must fall back to keyword.
	saveSkillSearchConfig(t, &entity.SkillSearchConfig{
		EmbdID:                 "embd-missing",
		VectorSimilarityWeight: 0.4,
		SimilarityThreshold:    0.1,
		TopK:                   10,
	})
	service := NewSkillSearchService()
	eng := &skillSearchEngine{
		indexExists:   true,
		searchResults: []map[string]interface{}{esStyleChunk("fallback-skill", 0.7)},
	}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "find", SortBy: "name", SortOrder: "asc"}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.SearchType != "keyword" {
		t.Fatalf("SearchType = %q, want keyword after hybrid fallback", resp.SearchType)
	}
	if resp.Total != 1 || resp.Skills[0].Name != "fallback-skill" {
		t.Fatalf("resp = %+v, want the keyword fallback hit", resp)
	}
	if len(eng.searchCalls) != 1 {
		t.Fatalf("engine Search called %d times, want 1 (fallback keyword only)", len(eng.searchCalls))
	}
	req := eng.searchCalls[0]
	if _, ok := req.MatchExprs[0].(*types.MatchTextExpr); !ok {
		t.Fatalf("fallback MatchExprs[0] = %T, want *types.MatchTextExpr", req.MatchExprs[0])
	}
	// The fallback keyword search must keep the requested sort.
	if req.OrderBy == nil || len(req.OrderBy.Fields) != 1 ||
		req.OrderBy.Fields[0].Field != "name" || req.OrderBy.Fields[0].Type != types.SortAsc {
		t.Fatalf("OrderBy = %+v, want name asc preserved on fallback", req.OrderBy)
	}
}

// TestSearchHybridFallsBackToKeywordOnEngineError verifies that an engine
// error during hybrid fallback is propagated to the caller.
func TestSearchHybridFallsBackToKeywordOnEngineError(t *testing.T) {
	setupSkillSearchTenant(t)
	saveSkillSearchConfig(t, &entity.SkillSearchConfig{
		EmbdID:                 "embd-missing",
		VectorSimilarityWeight: 0.4,
		SimilarityThreshold:    0.1,
		TopK:                   10,
	})
	service := NewSkillSearchService()
	eng := &skillSearchEngine{indexExists: true, searchErr: errors.New("engine down")}

	// Embedding resolution fails first (no model rows); Search then retries
	// with keyword search, whose engine error must surface to the caller.
	_, _, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "find"}, eng)
	if err == nil || !strings.Contains(err.Error(), "engine down") {
		t.Fatalf("err = %v, want engine error propagated from keyword fallback", err)
	}
}

// TestSearchVectorFallsBackToKeywordWhenEmbeddingUnavailable verifies that
// pure vector search falls back to keyword when the embedding is unavailable.
func TestSearchVectorFallsBackToKeywordWhenEmbeddingUnavailable(t *testing.T) {
	setupSkillSearchTenant(t)
	saveSkillSearchConfig(t, &entity.SkillSearchConfig{
		EmbdID:                 "embd-missing",
		VectorSimilarityWeight: 1,
		SimilarityThreshold:    0.1,
		TopK:                   10,
	})
	service := NewSkillSearchService()
	eng := &skillSearchEngine{
		indexExists:   true,
		searchResults: []map[string]interface{}{esStyleChunk("vector-fallback", 0.55)},
	}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "find"}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.SearchType != "keyword" {
		t.Fatalf("SearchType = %q, want keyword after vector fallback", resp.SearchType)
	}
	if resp.Total != 1 || resp.Skills[0].Name != "vector-fallback" {
		t.Fatalf("resp = %+v, want the keyword fallback hit", resp)
	}
}

// TestSearchPropagatesKeywordSearchError verifies that a keyword search error
// is propagated as CodeOperatingError with a nil response.
func TestSearchPropagatesKeywordSearchError(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()
	eng := &skillSearchEngine{indexExists: true, searchErr: errors.New("boom")}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "find"}, eng)
	if err == nil || code == common.CodeSuccess {
		t.Fatalf("code=%d err=%v, want error propagated from keyword search", code, err)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil on error", resp)
	}
}

// TestSearchPaginationWindow verifies that pagination slices the sorted results
// correctly and that an out-of-range page returns an empty slice.
func TestSearchPaginationWindow(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()
	results := make([]map[string]interface{}, 0, 25)
	for i := 0; i < 25; i++ {
		results = append(results, esStyleChunk(fmt.Sprintf("skill-%02d", i), float64(i)))
	}
	eng := &skillSearchEngine{indexExists: true, searchResults: results}

	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "", Page: 2, PageSize: 10}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if resp.Total != 25 {
		t.Fatalf("Total = %d, want 25", resp.Total)
	}
	if len(resp.Skills) != 10 {
		t.Fatalf("page size = %d, want 10", len(resp.Skills))
	}
	// Descending score order: index 0 is skill-24; page 2 starts at
	// offset 10, i.e. skill-14.
	if resp.Skills[0].Name != "skill-14" {
		t.Fatalf("first skill on page 2 = %q, want skill-14 (sorted desc, offset 10)", resp.Skills[0].Name)
	}

	// Page beyond the result set must return an empty slice, not panic.
	resp, _, err = service.Search(t.Context(), "owner", &SearchRequest{Query: "", Page: 9, PageSize: 10}, eng)
	if err != nil {
		t.Fatalf("out-of-range page: %v", err)
	}
	if len(resp.Skills) != 0 {
		t.Fatalf("out-of-range page returned %d skills, want 0", len(resp.Skills))
	}
}

// TestSearchDefaultsPageAndPageSize verifies that non-positive page and page
// size default to 1 and 10 respectively.
func TestSearchDefaultsPageAndPageSize(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()
	eng := &skillSearchEngine{
		indexExists:   true,
		searchResults: []map[string]interface{}{esStyleChunk("only", 1.0)},
	}

	// Page/page_size default to 1/10 when non-positive.
	resp, code, err := service.Search(t.Context(), "owner", &SearchRequest{Query: "", Page: -1, PageSize: -1}, eng)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Search: code=%d err=%v", code, err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(resp.Skills))
	}
}

// ---------------------------------------------------------------------------
// UpdateConfig validation
// ---------------------------------------------------------------------------

// TestUpdateConfigRejectsInvalidParameters verifies that UpdateConfig rejects
// out-of-range weights, thresholds, and non-positive top_k.
func TestUpdateConfigRejectsInvalidParameters(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()
	base := func() *UpdateConfigRequest {
		return &UpdateConfigRequest{
			SpaceID: "default", EmbdID: "embd-1",
			VectorSimilarityWeight: 0.5, SimilarityThreshold: 0.2, TopK: 10,
		}
	}

	cases := []struct {
		name string
		mut  func(*UpdateConfigRequest)
	}{
		{"negative vector weight", func(r *UpdateConfigRequest) { r.VectorSimilarityWeight = -0.1 }},
		{"vector weight above one", func(r *UpdateConfigRequest) { r.VectorSimilarityWeight = 1.1 }},
		{"negative threshold", func(r *UpdateConfigRequest) { r.SimilarityThreshold = -0.1 }},
		{"threshold above one", func(r *UpdateConfigRequest) { r.SimilarityThreshold = 1.5 }},
		{"zero top_k", func(r *UpdateConfigRequest) { r.TopK = 0 }},
		{"negative top_k", func(r *UpdateConfigRequest) { r.TopK = -5 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base()
			tc.mut(req)
			resp, code, err := service.UpdateConfig(t.Context(), "owner", req)
			if err == nil || code != common.CodeDataError {
				t.Fatalf("code=%d err=%v, want CodeDataError", code, err)
			}
			if resp != nil {
				t.Fatalf("resp = %+v, want nil on validation error", resp)
			}
		})
	}
}

// TestUpdateConfigCreatesThenUpdatesSingleActiveConfig verifies that updating
// an existing config keeps a single active row while switching embd_id.
func TestUpdateConfigCreatesThenUpdatesSingleActiveConfig(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()

	created, code, err := service.UpdateConfig(t.Context(), "owner", &UpdateConfigRequest{
		SpaceID: "default", EmbdID: "embd-1",
		VectorSimilarityWeight: 0.5, SimilarityThreshold: 0.2, TopK: 10,
		FieldConfig: entity.DefaultFieldConfig(),
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("create: code=%d err=%v", code, err)
	}
	if created["embd_id"] != "embd-1" {
		t.Fatalf("embd_id = %v, want embd-1", created["embd_id"])
	}

	updated, code, err := service.UpdateConfig(t.Context(), "owner", &UpdateConfigRequest{
		SpaceID: "default", EmbdID: "embd-2",
		VectorSimilarityWeight: 0.7, SimilarityThreshold: 0.3, TopK: 20,
		FieldConfig: entity.DefaultFieldConfig(),
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("update: code=%d err=%v", code, err)
	}
	if updated["embd_id"] != "embd-2" {
		t.Fatalf("embd_id = %v, want embd-2 (always updated to new value)", updated["embd_id"])
	}

	var activeCount int64
	if err := dao.DB.Model(&entity.SkillSearchConfig{}).
		Where("tenant_id = ? AND space_id = ? AND status = ?", "owner", "default", "1").
		Count(&activeCount).Error; err != nil {
		t.Fatalf("count active configs: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active configs = %d, want exactly 1 after embd switch", activeCount)
	}
}

// TestGetConfigReturnsDefaultsWhenNoConfigExists verifies that GetConfig
// returns the default configuration when no config row exists.
func TestGetConfigReturnsDefaultsWhenNoConfigExists(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()

	config, code, err := service.GetConfig(t.Context(), "owner", "default", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetConfig: code=%d err=%v", code, err)
	}
	if config["vector_similarity_weight"] != 0.3 {
		t.Errorf("vector_similarity_weight = %v, want default 0.3", config["vector_similarity_weight"])
	}
	if config["similarity_threshold"] != 0.2 {
		t.Errorf("similarity_threshold = %v, want default 0.2", config["similarity_threshold"])
	}
	if config["top_k"] != int64(10) {
		t.Errorf("top_k = %v (%T), want 10", config["top_k"], config["top_k"])
	}
}

// TestGetConfigWithEmbdIDCreatesMissingConfig verifies that GetConfig creates
// a config when an embd_id is supplied and none exists.
func TestGetConfigWithEmbdIDCreatesMissingConfig(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()

	config, code, err := service.GetConfig(t.Context(), "owner", "default", "embd-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetConfig: code=%d err=%v", code, err)
	}
	if config["embd_id"] != "embd-1" {
		t.Fatalf("embd_id = %v, want embd-1 (auto-created)", config["embd_id"])
	}

	var stored entity.SkillSearchConfig
	if err := dao.DB.Where("tenant_id = ? AND space_id = ?", "owner", "default").First(&stored).Error; err != nil {
		t.Fatalf("config row missing after GetOrCreate: %v", err)
	}
	if stored.EmbdID != "embd-1" {
		t.Fatalf("stored embd_id = %q, want embd-1", stored.EmbdID)
	}
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

// TestSkillIndexNameSanitizesAndStaysDeterministic verifies that SkillIndexName
// sanitizes special characters, is deterministic, and truncates long inputs.
func TestSkillIndexNameSanitizesAndStaysDeterministic(t *testing.T) {
	first := SkillIndexName("tenant-1", "My Space/With:Special.Chars")
	second := SkillIndexName("tenant-1", "My Space/With:Special.Chars")

	if first != second {
		t.Fatalf("SkillIndexName not deterministic: %q vs %q", first, second)
	}
	if !strings.HasPrefix(first, "skill_tenant-1_") {
		t.Fatalf("index name = %q, want skill_tenant-1_ prefix", first)
	}
	for _, banned := range []string{"/", ":", ".", " "} {
		if strings.Contains(first, banned) {
			t.Fatalf("index name %q contains banned rune %q", first, banned)
		}
	}

	// Empty space normalizes to "default".
	if got := SkillIndexName("tenant-1", ""); !strings.Contains(got, "default") {
		t.Fatalf("empty space id index = %q, want default space segment", got)
	}

	// Overlong components are truncated with a hash suffix, not overflowing.
	long := strings.Repeat("a", 80)
	hashed := SkillIndexName(long, long)
	if !strings.HasPrefix(hashed, "skill_") {
		t.Fatalf("long id index = %q, want skill_ prefix", hashed)
	}
	if len(hashed) > len("skill_")+32+1+8+1+32+1+8 {
		t.Fatalf("long id index %q longer than expected truncation bound", hashed)
	}

	// Distinct tenant/space pairs must not collide.
	if SkillIndexName("t1", "s1") == SkillIndexName("t2", "s2") {
		t.Fatal("distinct tenant/space pairs collided")
	}
}

func TestBuildVectorTextRespectsFieldConfig(t *testing.T) {
	all := entity.FieldConfig{
		Name:        entity.FieldWeight{Enabled: true, Weight: 3},
		Tags:        entity.FieldWeight{Enabled: true, Weight: 2},
		Description: entity.FieldWeight{Enabled: true, Weight: 1},
		Content:     entity.FieldWeight{Enabled: true, Weight: 0.5},
	}
	if got, want := BuildVectorText("name", "desc", []string{"t1", "t2"}, "content", all), "name\n\nt1 t2\n\ndesc\n\ncontent"; got != want {
		t.Fatalf("all fields: got %q, want %q", got, want)
	}

	none := entity.FieldConfig{
		Name:        entity.FieldWeight{Enabled: false},
		Tags:        entity.FieldWeight{Enabled: false},
		Description: entity.FieldWeight{Enabled: false},
		Content:     entity.FieldWeight{Enabled: false},
	}
	if got := BuildVectorText("name", "desc", []string{"t1"}, "content", none); got != "" {
		t.Fatalf("no fields enabled: got %q, want empty", got)
	}

	// Disabled content is the default config: name/tags/description only.
	defaults := entity.DefaultFieldConfig()
	if got, want := BuildVectorText("name", "desc", []string{"t1"}, "content", defaults), "name\n\nt1\n\ndesc"; got != want {
		t.Fatalf("default config: got %q, want %q", got, want)
	}
}

func TestCalculateContentHash(t *testing.T) {
	// Tags are hashed in slice order, so reordering them changes the hash.
	a := CalculateContentHash("name", "desc", []string{"x", "y"}, "content")
	b := CalculateContentHash("name", "desc", []string{"y", "x"}, "content")
	if a == b {
		t.Fatal("tag order did not change the hash, but the hash is written tag-by-tag in order")
	}
	if c := CalculateContentHash("name", "desc", []string{"x", "y"}, "content"); c != a {
		t.Fatal("identical inputs produced different hashes")
	}
	if c := CalculateContentHash("name2", "desc", []string{"x", "y"}, "content"); c == a {
		t.Fatal("different names produced the same hash")
	}
}

func TestBuildOrderByExpr(t *testing.T) {
	cases := []struct {
		name      string
		sortBy    string
		sortOrder string
		isEmpty   bool
		wantNil   bool
		wantField string
		wantDir   types.OrderByType
	}{
		{"empty query defaults to update_time desc", "", "", true, false, "update_time", types.SortDesc},
		{"search query defaults to relevance", "", "", false, true, "", 0},
		{"explicit relevance is nil", "relevance", "", false, true, "", 0},
		{"name defaults to asc", "name", "", false, false, "name", types.SortAsc},
		{"time fields default to desc", "create_time", "", true, false, "create_time", types.SortDesc},
		{"explicit desc kept", "name", "desc", false, false, "name", types.SortDesc},
		{"camelCase mapped", "updateTime", "ASC", true, false, "update_time", types.SortAsc},
		{"underscore alias mapped", "created_at", "", true, false, "create_time", types.SortDesc},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildOrderByExpr(tc.sortBy, tc.sortOrder, tc.isEmpty)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("orderBy = %+v, want nil", got)
				}
				return
			}
			if got == nil || len(got.Fields) != 1 {
				t.Fatalf("orderBy = %+v, want one field", got)
			}
			if got.Fields[0].Field != tc.wantField || got.Fields[0].Type != tc.wantDir {
				t.Fatalf("orderBy = %+v, want %s type %d", got, tc.wantField, tc.wantDir)
			}
		})
	}
}

func TestKeywordSearchBuildsWeightedMatchExprAndOrderBy(t *testing.T) {
	setupSkillSearchTenant(t)
	service := NewSkillSearchService()
	eng := &skillSearchEngine{indexExists: true}

	config := &entity.SkillSearchConfig{
		EmbdID: "embd-1", VectorSimilarityWeight: 0, SimilarityThreshold: 0.1, TopK: 10,
	}
	if _, err := service.keywordSearch(t.Context(), eng, "skill_idx", "hello world", config, 0.1, "name", "asc"); err != nil {
		t.Fatalf("keywordSearch: %v", err)
	}

	if len(eng.searchCalls) != 1 {
		t.Fatalf("engine Search called %d times, want 1", len(eng.searchCalls))
	}
	req := eng.searchCalls[0]
	expr, ok := req.MatchExprs[0].(*types.MatchTextExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] = %T, want *types.MatchTextExpr", req.MatchExprs[0])
	}
	if expr.MatchingText != "hello world" {
		t.Errorf("MatchingText = %q, want raw query", expr.MatchingText)
	}
	wantFields := []string{"name^10", "tags^5", "description^3", "content^1"}
	if len(expr.Fields) != len(wantFields) {
		t.Fatalf("Fields = %v, want %v", expr.Fields, wantFields)
	}
	for i, f := range wantFields {
		if expr.Fields[i] != f {
			t.Errorf("Fields[%d] = %q, want %q", i, expr.Fields[i], f)
		}
	}
	if req.OrderBy == nil || len(req.OrderBy.Fields) != 1 ||
		req.OrderBy.Fields[0].Field != "name" || req.OrderBy.Fields[0].Type != types.SortAsc {
		t.Fatalf("OrderBy = %+v, want name asc", req.OrderBy)
	}
}
