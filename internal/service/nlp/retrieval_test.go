package nlp

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"

	"gorm.io/gorm"
)

func TestRetrievalUsesRerankCandidatesCountAsCandidateSet(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	rows := make([]map[string]interface{}, 75)
	for i := range rows {
		rows[i] = map[string]interface{}{
			"id":                  fmt.Sprintf("chunk-%02d", i),
			"content_ltks":        "alpha",
			"content_with_weight": "alpha",
			"_score":              0.9,
		}
	}
	engine := &retrievalCountEngine{rows: rows}
	service := NewRetrievalService(engine, &dao.DocumentDAO{})
	top := 100
	threshold := 0.5
	vectorWeight := 1.0
	aggs := false
	rerankCandidatesCount := 70

	result, err := service.Retrieval(t.Context(), &RetrievalRequest{
		Question:               "alpha",
		TenantIDs:              []string{"tenant-1"},
		Page:                   1,
		PageSize:               10,
		KNNTopK:                &top,
		SimilarityThreshold:    &threshold,
		VectorSimilarityWeight: &vectorWeight,
		Aggs:                   &aggs,
		Filter:                 map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}},
		RerankCandidatesCount:  &rerankCandidatesCount,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if len(result.Chunks) != 10 {
		t.Fatalf("page chunk count = %d, want 10", len(result.Chunks))
	}
	if result.Total != 70 {
		t.Fatalf("total = %d, want 70", result.Total)
	}
	if len(engine.searchLimits) != 1 || engine.searchLimits[0] != rerankCandidatesCount {
		t.Fatalf("search limits = %v, want [%d]", engine.searchLimits, rerankCandidatesCount)
	}
	for _, filters := range engine.searchFilters {
		mustNot, ok := filters["must_not"].(map[string]interface{})
		if !ok || mustNot["exists"] != "compile_kwd" {
			t.Fatalf("must_not filter = %#v", filters["must_not"])
		}
	}
}

type retrievalCountEngine struct {
	rows          []map[string]interface{}
	searchLimits  []int
	searchFilters []map[string]interface{}
}

func (e *retrievalCountEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searchLimits = append(e.searchLimits, req.Limit)
	e.searchFilters = append(e.searchFilters, req.Filter)
	offset := req.Offset
	if offset > len(e.rows) {
		offset = len(e.rows)
	}
	end := offset + req.Limit
	if req.Limit <= 0 || end > len(e.rows) {
		end = len(e.rows)
	}
	return &types.SearchResult{Chunks: e.rows[offset:end], Total: int64(len(e.rows))}, nil
}

func (e *retrievalCountEngine) GetChunkIDs(chunks []map[string]interface{}) []string {
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (e *retrievalCountEngine) GetFields(chunks []map[string]interface{}, _ []string) map[string]map[string]interface{} {
	fields := make(map[string]map[string]interface{}, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			fields[id] = chunk
		}
	}
	return fields
}

func (e *retrievalCountEngine) KNNScores(_ context.Context, chunks []map[string]interface{}, _ []float64, _ int) (map[string]interface{}, error) {
	scores := make(map[string]interface{}, len(chunks))
	for _, chunk := range chunks {
		id, _ := chunk["id"].(string)
		score, _ := chunk["_score"].(float64)
		scores[id] = score
	}
	return scores, nil
}

func (e *retrievalCountEngine) GetScores(result map[string]interface{}) map[string]float64 {
	scores := make(map[string]float64, len(result))
	for id, raw := range result {
		if score, ok := raw.(float64); ok {
			scores[id] = score
		}
	}
	return scores
}

func (e *retrievalCountEngine) DropChunkStore(context.Context, string, string) error { return nil }
func (e *retrievalCountEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (e *retrievalCountEngine) Close() error               { return nil }
func (e *retrievalCountEngine) Ping(context.Context) error { return nil }
func (e *retrievalCountEngine) GetType() string            { return "elasticsearch" }
func (e *retrievalCountEngine) SupportsPageRank() bool     { return false }
func (e *retrievalCountEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (e *retrievalCountEngine) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
	return nil, nil
}
func (e *retrievalCountEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	return nil
}
func (e *retrievalCountEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	return 0, nil
}
func (e *retrievalCountEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (e *retrievalCountEngine) CreateMetadataStore(context.Context, string) error { return nil }
func (e *retrievalCountEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (e *retrievalCountEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (e *retrievalCountEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (e *retrievalCountEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (e *retrievalCountEngine) DropMetadataStore(context.Context, string) error { return nil }
func (e *retrievalCountEngine) MetadataStoreExists(context.Context, string) (bool, error) {
	return true, nil
}
func (e *retrievalCountEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (e *retrievalCountEngine) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}
func (e *retrievalCountEngine) DeleteDocument(context.Context, string, string) error { return nil }
func (e *retrievalCountEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (e *retrievalCountEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (e *retrievalCountEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (e *retrievalCountEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (e *retrievalCountEngine) FilterDocIdsByMetaPushdown(context.Context, *gorm.DB, []string, []map[string]interface{}, string) []string {
	return nil
}

func TestBuildInfinityFusionExprUsesVectorSimilarityWeight(t *testing.T) {
	tests := []struct {
		name                   string
		vectorSimilarityWeight *float64
		expectedWeights        string
	}{
		{name: "default", vectorSimilarityWeight: nil, expectedWeights: "0.7,0.3"},
		{name: "text only", vectorSimilarityWeight: float64Ptr(0), expectedWeights: "1,0"},
		{name: "balanced", vectorSimilarityWeight: float64Ptr(0.5), expectedWeights: "0.5,0.5"},
		{name: "vector only", vectorSimilarityWeight: float64Ptr(1), expectedWeights: "0,1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := buildInfinityFusionExpr(10, tt.vectorSimilarityWeight)
			if expr.Method != "weighted_sum" {
				t.Fatalf("expected weighted_sum, got %q", expr.Method)
			}
			if expr.TopN != 10 {
				t.Fatalf("expected TopN=10, got %d", expr.TopN)
			}
			weights, ok := expr.FusionParams["weights"].(string)
			if !ok || weights != tt.expectedWeights {
				t.Fatalf("expected weights=%q, got %v", tt.expectedWeights, expr.FusionParams["weights"])
			}
		})
	}
}

func float64Ptr(value float64) *float64 { return &value }
func intPtr(value int) *int             { return &value }

type captureSearchDocEngine struct {
	engine.DocEngine
	engineType    string
	searchRequest *types.SearchRequest
	result        *types.SearchResult
}

func (e *captureSearchDocEngine) GetType() string {
	return e.engineType
}

func (e *captureSearchDocEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searchRequest = req
	if e.result != nil {
		return e.result, nil
	}
	return &types.SearchResult{Chunks: []map[string]interface{}{{"id": "chunk-1"}}, Total: 1}, nil
}
func (e *captureSearchDocEngine) GetChunkIDs(_ []map[string]interface{}) []string {
	return []string{"chunk-1"}
}
func (e *captureSearchDocEngine) GetFields(_ []map[string]interface{}, _ []string) map[string]map[string]interface{} {
	return map[string]map[string]interface{}{}
}
func (e *captureSearchDocEngine) GetAggregation(_ []map[string]interface{}, _ string) []map[string]interface{} {
	return []map[string]interface{}{}
}
func (e *captureSearchDocEngine) GetHighlight(_ []map[string]interface{}, _ []string, _ string) map[string]string {
	return nil
}

type captureEmbeddingDriver struct{ modelModule.ModelDriver }

func (d *captureEmbeddingDriver) Embed(_ context.Context, _ *string, _ modelModule.EmbedRequest, _ *modelModule.APIConfig, _ *modelModule.EmbeddingConfig, _ *common.ModelUsage) ([]modelModule.EmbeddingData, error) {
	return []modelModule.EmbeddingData{{Embedding: []float64{0.1, 0.2}}}, nil
}

func TestSearchPassesVectorSimilarityWeightToFusionExpr(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}
	vectorWeight := 0.8
	docEngine := &captureSearchDocEngine{engineType: string(engine.EngineInfinity)}
	service := NewRetrievalService(docEngine, nil)
	_, err := service.Search(t.Context(), &RetrievalSearchRequest{
		Question: "test question", TenantIDs: []string{"tenant-1"}, KbIDs: []string{"kb-1"}, Page: 1, PageSize: 10, KNNTopK: 10,
		RankFeature: map[string]float64{}, EmbeddingModel: &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}}, VectorSimilarityWeight: &vectorWeight,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	assertFusionWeights(t, docEngine.searchRequest, "0.2,0.8")
}

func TestRetrievalPassesVectorSimilarityWeightToSearch(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}
	vectorWeight := 0.8
	top := 10
	docEngine := &captureSearchDocEngine{
		engineType: string(engine.EngineInfinity),
		result:     &types.SearchResult{Chunks: []map[string]interface{}{}, Total: 0},
	}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
	_, err := service.Retrieval(t.Context(), &RetrievalRequest{
		Question: "test question", TenantIDs: []string{"tenant-1"}, KbIDs: []string{"kb-1"}, Page: 1, PageSize: 10, KNNTopK: &top,
		RankFeature: &map[string]float64{}, EmbeddingModel: &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}}, VectorSimilarityWeight: &vectorWeight,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	assertFusionWeights(t, docEngine.searchRequest, "0.2,0.8")
}

func assertFusionWeights(t *testing.T, request *types.SearchRequest, want string) {
	t.Helper()
	if request == nil || len(request.MatchExprs) != 3 {
		t.Fatalf("expected three match expressions, got %#v", request)
	}
	fusionExpr, ok := request.MatchExprs[2].(*types.FusionExpr)
	if !ok {
		t.Fatalf("expected third match expression to be FusionExpr, got %T", request.MatchExprs[2])
	}
	if got := fusionExpr.FusionParams["weights"]; got != want {
		t.Fatalf("expected weights=%s, got %v", want, got)
	}
}

func TestBuildRetrievalFusionExprKeepsLegacyWeightsOutsideInfinity(t *testing.T) {
	expr := buildRetrievalFusionExpr(string(engine.EngineElasticsearch), 10, float64Ptr(0.8))

	if got := expr.FusionParams["weights"]; got != "0.05,0.95" {
		t.Fatalf("expected Elasticsearch weights=0.05,0.95, got %v", got)
	}
}

func TestSearchKeepsLegacyFusionWeightForElasticsearch(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}

	vectorWeight := 0.8
	docEngine := &captureSearchDocEngine{engineType: string(engine.EngineElasticsearch)}
	service := NewRetrievalService(docEngine, nil)

	_, err := service.Search(t.Context(), &RetrievalSearchRequest{
		Question:               "test question",
		TenantIDs:              []string{"tenant-1"},
		KbIDs:                  []string{"kb-1"},
		Page:                   1,
		PageSize:               10,
		KNNTopK:                10,
		RankFeature:            map[string]float64{},
		EmbeddingModel:         &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}},
		VectorSimilarityWeight: &vectorWeight,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if docEngine.searchRequest == nil || len(docEngine.searchRequest.MatchExprs) != 3 {
		t.Fatalf("expected three match expressions, got %#v", docEngine.searchRequest)
	}
	fusionExpr, ok := docEngine.searchRequest.MatchExprs[2].(*types.FusionExpr)
	if !ok {
		t.Fatalf("expected third match expression to be FusionExpr, got %T", docEngine.searchRequest.MatchExprs[2])
	}
	if got := fusionExpr.FusionParams["weights"]; got != "0.05,0.95" {
		t.Fatalf("expected Elasticsearch weights=0.05,0.95, got %v", got)
	}
}
