package nlp

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"

	"gorm.io/gorm"
)

func TestRetrievalTotalCountsThresholdValidMatchesBeyondRerankWindow(t *testing.T) {
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

	result, err := service.Retrieval(context.Background(), &RetrievalRequest{
		Question:               "alpha",
		TenantIDs:              []string{"tenant-1"},
		Page:                   1,
		PageSize:               10,
		Top:                    &top,
		SimilarityThreshold:    &threshold,
		VectorSimilarityWeight: &vectorWeight,
		Aggs:                   &aggs,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if len(result.Chunks) != 10 {
		t.Fatalf("page chunk count = %d, want 10", len(result.Chunks))
	}
	if result.Total != 75 {
		t.Fatalf("total = %d, want 75", result.Total)
	}
	if len(engine.searchLimits) != 2 || engine.searchLimits[0] != 70 || engine.searchLimits[1] != 100 {
		t.Fatalf("search limits = %v, want [70 100]", engine.searchLimits)
	}
}

type retrievalCountEngine struct {
	rows         []map[string]interface{}
	searchLimits []int
}

func (e *retrievalCountEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searchLimits = append(e.searchLimits, req.Limit)
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
