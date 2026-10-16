package nlp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"github.com/glebarez/sqlite"
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
	if engine.highlightCalls != 0 {
		t.Fatalf("GetHighlight calls = %d with highlighting disabled, want 0", engine.highlightCalls)
	}
}

func TestRetrievalPreservesCompilationKindWhenPresent(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	docEngine := &retrievalCountEngine{rows: []map[string]interface{}{
		{"id": "compiled", "content_ltks": "alpha", "content_with_weight": "alpha", "compile_kwd": "page_index", "_score": 0.9},
		{"id": "ordinary", "content_ltks": "alpha", "content_with_weight": "alpha", "_score": 0.8},
	}}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
	threshold := 0.0
	vectorWeight := 1.0
	aggs := false
	result, err := service.Retrieval(t.Context(), &RetrievalRequest{
		Question:               "alpha",
		TenantIDs:              []string{"tenant-1"},
		Page:                   1,
		PageSize:               2,
		SimilarityThreshold:    &threshold,
		VectorSimilarityWeight: &vectorWeight,
		Aggs:                   &aggs,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	for _, fields := range docEngine.selectedFields {
		if !slices.Contains(fields, "compile_kwd") {
			t.Fatalf("search fields = %v, want compile_kwd", fields)
		}
	}
	if len(result.Chunks) != 2 {
		t.Fatalf("chunks = %#v, want compiled and ordinary chunks", result.Chunks)
	}
	if got := result.Chunks[0]["compile_kwd"]; got != "page_index" {
		t.Fatalf("compiled chunk compile_kwd = %v, want page_index", got)
	}
	if _, ok := result.Chunks[1]["compile_kwd"]; ok {
		t.Fatalf("ordinary chunk unexpectedly has compile_kwd: %#v", result.Chunks[1])
	}
}

func TestRetrievalReturnsRegexHighlightWithoutFallbackOrCleanup(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	rows := []map[string]interface{}{
		{"id": "highlighted", "content_ltks": "alpha", "content_with_weight": "alpha élève", "_score": 0.9},
		{"id": "unmatched", "content_ltks": "alpha", "content_with_weight": "plain content", "_score": 0.8},
	}
	docEngine := &retrievalCountEngine{
		rows:       rows,
		highlights: map[string]string{"highlighted": "<em>alpha</em> élève"},
	}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
	top := 10
	threshold := 0.5
	vectorWeight := 1.0
	highlight := true

	result, err := service.Retrieval(t.Context(), &RetrievalRequest{
		Question:               "alpha",
		TenantIDs:              []string{"tenant-1"},
		Page:                   1,
		PageSize:               2,
		KNNTopK:                &top,
		SimilarityThreshold:    &threshold,
		VectorSimilarityWeight: &vectorWeight,
		Highlight:              &highlight,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if docEngine.highlightCalls != 1 {
		t.Fatalf("GetHighlight calls = %d, want 1", docEngine.highlightCalls)
	}
	if got := result.Chunks[0]["highlight"]; got != "<em>alpha</em> élève" {
		t.Fatalf("highlight = %q, want spacing preserved", got)
	}
	if _, ok := result.Chunks[1]["highlight"]; ok {
		t.Fatalf("unmatched chunk received fallback highlight: %#v", result.Chunks[1])
	}
}

func TestRetrievalPreservesDatasetCompilationChunks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&entity.Document{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Document{ID: "doc-1", KbID: "kb-1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldQueryBuilder := dao.DB, globalQueryBuilder
	dao.DB, globalQueryBuilder = db, NewQueryBuilder()
	defer func() { dao.DB, globalQueryBuilder = oldDB, oldQueryBuilder }()

	artifact := map[string]interface{}{"id": "artifact", "doc_id": "kb-1", "kb_id": "kb-1", "compile_kwd": "graph", "scope_kwd": "dataset", "available_int": 1, "content_ltks": "alpha", "content_with_weight": "alpha", "_score": 0.9}
	for _, tt := range []struct {
		name string
		rows []map[string]interface{}
		want []string
	}{
		{name: "dataset only", rows: []map[string]interface{}{artifact}, want: []string{"artifact"}},
		{name: "mixed with deleted documents", rows: []map[string]interface{}{
			artifact,
			{"id": "document", "doc_id": "doc-1", "kb_id": "kb-1", "compile_kwd": "graph", "scope_kwd": "doc", "content_ltks": "alpha", "_score": 0.8},
			{"id": "deleted", "doc_id": "deleted-doc", "kb_id": "kb-1", "compile_kwd": "graph", "scope_kwd": "doc", "content_ltks": "alpha", "_score": 0.7},
			{"id": "wrong identity", "doc_id": "deleted-doc", "kb_id": "kb-1", "compile_kwd": "graph", "scope_kwd": "dataset", "content_ltks": "alpha", "_score": 0.6},
		}, want: []string{"artifact", "document"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			eng := &projectedRetrievalEngine{retrievalCountEngine: retrievalCountEngine{rows: tt.rows}}
			result, err := NewRetrievalService(eng, &dao.DocumentDAO{}).Retrieval(t.Context(), &RetrievalRequest{
				Question: "alpha", TenantIDs: []string{"tenant-1"}, KbIDs: []string{"kb-1"}, Page: 1, PageSize: 10,
				VectorSimilarityWeight: new(1.0), SimilarityThreshold: new(0.0), Aggs: new(true),
			})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, chunk := range result.Chunks {
				got = append(got, chunk["chunk_id"].(string))
				if _, ok := chunk["available_int"]; ok {
					t.Fatal("available_int should not be returned in retrieval results")
				}
			}
			if !slices.Equal(got, tt.want) || result.Total != int64(len(tt.want)) {
				t.Fatalf("chunks=%v total=%d want %v", got, result.Total, tt.want)
			}
			if len(result.DocAggs) != len(tt.want)-1 {
				t.Fatalf("doc_aggs=%v, want only source documents", result.DocAggs)
			}
			if len(result.DocAggs) > 0 && (result.DocAggs[0]["doc_id"] != "doc-1" || result.DocAggs[0]["count"] != 1) {
				t.Fatalf("doc_aggs=%v, want doc-1 with count 1", result.DocAggs)
			}
		})
	}
}

type projectedRetrievalEngine struct{ retrievalCountEngine }

func (e *projectedRetrievalEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{}, len(chunks))
	for _, chunk := range chunks {
		projected := make(map[string]interface{}, len(fields))
		for _, field := range fields {
			if value, ok := chunk[field]; ok {
				projected[field] = value
			}
		}
		result[chunk["id"].(string)] = projected
	}
	return result
}

type retrievalCountEngine struct {
	rows           []map[string]interface{}
	searchLimits   []int
	searchFilters  []map[string]interface{}
	selectedFields [][]string
	highlights     map[string]string
	highlightCalls int
}

func (e *retrievalCountEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searchLimits = append(e.searchLimits, req.Limit)
	e.searchFilters = append(e.searchFilters, req.Filter)
	e.selectedFields = append(e.selectedFields, slices.Clone(req.SelectFields))
	offset := req.Offset
	if offset > len(e.rows) {
		offset = len(e.rows)
	}
	end := offset + req.Limit
	if req.Limit <= 0 || end > len(e.rows) {
		end = len(e.rows)
	}
	chunks := make([]map[string]interface{}, 0, end-offset)
	for _, row := range e.rows[offset:end] {
		if len(req.SelectFields) == 0 {
			chunks = append(chunks, maps.Clone(row))
			continue
		}
		projected := map[string]interface{}{"id": row["id"]}
		for _, field := range req.SelectFields {
			if value, ok := row[field]; ok {
				projected[field] = value
			}
		}
		chunks = append(chunks, projected)
	}
	return &types.SearchResult{Chunks: chunks, Total: int64(len(e.rows))}, nil
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

func (e *retrievalCountEngine) GetFields(chunks []map[string]interface{}, selected []string) map[string]map[string]interface{} {
	fields := make(map[string]map[string]interface{}, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			projected := make(map[string]interface{}, len(selected))
			for _, field := range selected {
				if value, ok := chunk[field]; ok {
					projected[field] = value
				}
			}
			fields[id] = projected
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
func (e *retrievalCountEngine) CreateChunkStore(context.Context, string, string, int, string, string) error {
	return nil
}
func (e *retrievalCountEngine) InsertChunks(context.Context, []map[string]interface{}, string, string, string) ([]string, error) {
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
	e.highlightCalls++
	return e.highlights
}
func (e *retrievalCountEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (e *retrievalCountEngine) FilterDocIdsByMetaPushdown(context.Context, *gorm.DB, []string, []map[string]interface{}, string) []string {
	return nil
}

type parentChunkMissingEngine struct{ engine.DocEngine }

func (parentChunkMissingEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return nil, errors.New("parent chunk missing")
}

type parentChunkScopeEngine struct {
	engine.DocEngine
	parentSearch *types.SearchRequest
	parentChunk  map[string]interface{}
}

func (e *parentChunkScopeEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.parentSearch = req
	if e.parentChunk != nil {
		return &types.SearchResult{Chunks: []map[string]interface{}{e.parentChunk}}, nil
	}
	return &types.SearchResult{}, nil
}

func TestRetrievalByChildrenScopesParentLookupToChildDocument(t *testing.T) {
	engine := &parentChunkScopeEngine{}
	child := map[string]interface{}{
		"chunk_id":            "child-a",
		"mom_id":              "legacy-shared-parent-id",
		"doc_id":              "doc-a",
		"kb_id":               "kb-1",
		"content_with_weight": "matching child text",
		"similarity":          0.8,
	}

	got := RetrievalByChildren([]map[string]interface{}{child}, []string{"tenant-1", "tenant-2"}, engine, t.Context())

	if len(got) != 1 || got[0]["chunk_id"] != "child-a" {
		t.Fatalf("retrieval result = %#v, want fallback child", got)
	}
	if engine.parentSearch == nil {
		t.Fatal("parent lookup did not use a document-scoped search")
	}
	if engine.parentSearch.Filter["doc_id"] != "doc-a" {
		t.Fatalf("parent lookup filter = %#v, want doc_id doc-a", engine.parentSearch.Filter)
	}
	if !slices.Equal(engine.parentSearch.IndexNames, buildIndexNames([]string{"tenant-1", "tenant-2"})) {
		t.Fatalf("parent lookup indices = %v, want both tenant indices", engine.parentSearch.IndexNames)
	}
	if !slices.Equal(engine.parentSearch.KbIDs, []string{"kb-1"}) {
		t.Fatalf("parent lookup KBs = %v, want the child's dataset", engine.parentSearch.KbIDs)
	}
	if !engine.parentSearch.IncludeUnavailable {
		t.Fatal("parent lookup must include hidden parent chunks")
	}
	engine.parentChunk = map[string]interface{}{
		"id": "legacy-shared-parent-id", "doc_id": "doc-a", "kb_id": "kb-1",
		"available_int": 0, "content_with_weight": "parent text",
	}
	got = RetrievalByChildren([]map[string]interface{}{child}, []string{"tenant-1", "tenant-2"}, engine, t.Context())
	if len(got) != 1 || got[0]["chunk_id"] != "legacy-shared-parent-id" || got[0]["content_with_weight"] != "parent text" {
		t.Fatalf("retrieval result = %#v, want aggregated parent", got)
	}
	if _, ok := got[0]["available_int"]; ok {
		t.Fatal("available_int should not be returned in aggregated retrieval results")
	}
	if engine.parentChunk["available_int"] != 0 {
		t.Fatal("stored parent availability should remain unchanged")
	}
}

// TestRetrievalByChildrenKeepsChildWhenParentIsMissing verifies a partial
// parent-child write never turns a relevant child hit into an empty result.
// Removing the fallback is a retrieval data-loss bug.
func TestRetrievalByChildrenKeepsChildWhenParentIsMissing(t *testing.T) {
	child := map[string]interface{}{
		"chunk_id":            "child-1",
		"mom_id":              "parent-1",
		"doc_id":              "doc-1",
		"kb_id":               "kb-1",
		"content_with_weight": "matching child text",
		"similarity":          0.8,
	}

	got := RetrievalByChildren(
		[]map[string]interface{}{child},
		[]string{"tenant-1"},
		parentChunkMissingEngine{},
		t.Context(),
	)
	if len(got) != 1 {
		t.Fatalf("retrieval result count = %d, want fallback child", len(got))
	}
	if got[0]["chunk_id"] != "child-1" {
		t.Fatalf("fallback chunk_id = %#v, want child-1", got[0]["chunk_id"])
	}
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
func (e *captureSearchDocEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (e *captureSearchDocEngine) GetScores(map[string]interface{}) map[string]float64 {
	return map[string]float64{}
}

type captureEmbeddingDriver struct{ modelModule.ModelDriver }

func (d *captureEmbeddingDriver) Embed(_ context.Context, _ *string, _ modelModule.EmbedRequest, _ *modelModule.APIConfig, _ *modelModule.EmbeddingConfig, _ *common.ModelUsage) ([]modelModule.EmbeddingData, error) {
	return []modelModule.EmbeddingData{{Embedding: []float64{0.1, 0.2}}}, nil
}

type retryCaptureEngine struct {
	engine.DocEngine
	engineType string
	totals     []int64
	requests   []*types.SearchRequest
	mutate     bool
}

func (e *retryCaptureEngine) GetType() string { return e.engineType }
func (e *retryCaptureEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	requestCopy := *req
	requestCopy.Filter = maps.Clone(req.Filter)
	requestCopy.MatchExprs = slices.Clone(req.MatchExprs)
	for i, expression := range requestCopy.MatchExprs {
		if dense, ok := expression.(*types.MatchDenseExpr); ok {
			requestCopy.MatchExprs[i] = cloneDenseExpr(dense)
		}
	}
	e.requests = append(e.requests, &requestCopy)
	if e.mutate {
		for _, expression := range req.MatchExprs {
			if dense, ok := expression.(*types.MatchDenseExpr); ok {
				delete(dense.ExtraOptions, "num_candidates")
				dense.ExtraOptions["filter"] = "connector-added"
			}
		}
	}
	total := e.totals[0]
	e.totals = e.totals[1:]
	return &types.SearchResult{Total: total}, nil
}
func (e *retryCaptureEngine) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (e *retryCaptureEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (e *retryCaptureEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (e *retryCaptureEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}

func TestSearchDenseFallbackContract(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}

	for _, engineType := range []string{string(engine.EngineElasticsearch), string(engine.EngineInfinity)} {
		for _, test := range []struct {
			name      string
			totals    []int64
			wantCalls int
			wantExprs int
		}{
			{name: "one weak lexical hit", totals: []int64{1}, wantCalls: 1, wantExprs: 3},
			{name: "relaxed hit", totals: []int64{0, 1}, wantCalls: 2, wantExprs: 3},
			{name: "dense recovery", totals: []int64{0, 0, 1}, wantCalls: 3, wantExprs: 1},
			{name: "dense recovery empty", totals: []int64{0, 0, 0}, wantCalls: 3, wantExprs: 1},
		} {
			t.Run(engineType+"/"+test.name, func(t *testing.T) {
				docEngine := &retryCaptureEngine{engineType: engineType, totals: slices.Clone(test.totals), mutate: true}
				service := NewRetrievalService(docEngine, nil)
				_, err := service.Search(t.Context(), &RetrievalSearchRequest{
					Question: "ปัญหาแจ้งระบบภาษี", TenantIDs: []string{"tenant-1"}, KbIDs: []string{"kb-1"}, Page: 1, PageSize: 10,
					KNNTopK: 7, KNNNumCandidates: 19, Filter: map[string]interface{}{"category_kwd": "allowed"},
					EmbeddingModel: &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}},
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(docEngine.requests) != test.wantCalls {
					t.Fatalf("search calls = %d, want %d", len(docEngine.requests), test.wantCalls)
				}
				last := docEngine.requests[len(docEngine.requests)-1]
				if len(last.MatchExprs) != test.wantExprs {
					t.Fatalf("final expressions = %d, want %d", len(last.MatchExprs), test.wantExprs)
				}
				if test.wantCalls == 3 {
					dense := last.MatchExprs[0].(*types.MatchDenseExpr)
					if dense.TopN != 7 || dense.ExtraOptions["num_candidates"] != 19 || dense.ExtraOptions["similarity"] != 0.17 {
						t.Fatalf("dense fallback changed options: %#v", dense)
					}
					if _, contaminated := dense.ExtraOptions["filter"]; contaminated {
						t.Fatalf("dense fallback inherited connector mutation: %#v", dense.ExtraOptions)
					}
					if fmt.Sprint(last.Filter["kb_id"]) != "[kb-1]" || last.Filter["available_int"] != 1 || last.Filter["category_kwd"] != "allowed" {
						t.Fatalf("dense fallback lost scope filters: %#v", last.Filter)
					}
				}
			})
		}
	}
}

func TestSearchWithoutLexicalExpressionRetries(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}

	docEngine := &retryCaptureEngine{engineType: string(engine.EngineElasticsearch), totals: []int64{0, 1}, mutate: true}
	service := NewRetrievalService(docEngine, nil)
	_, err := service.Search(t.Context(), &RetrievalSearchRequest{
		Question: "!!!", TenantIDs: []string{"tenant-1"}, KbIDs: []string{"kb-1"},
		EmbeddingModel: &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(docEngine.requests) != 2 || len(docEngine.requests[1].MatchExprs) != 1 {
		t.Fatalf("dense-only request did not retry: %#v", docEngine.requests)
	}
	dense := docEngine.requests[1].MatchExprs[0].(*types.MatchDenseExpr)
	if dense.ExtraOptions["similarity"] != 0.17 || dense.ExtraOptions["num_candidates"] != 2048 || dense.ExtraOptions["filter"] != nil {
		t.Fatalf("fallback options = %#v", dense.ExtraOptions)
	}
}

func TestRetrievalDisablesDenseFallback(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}
	for _, query := range []string{"!!!", "weak lexical query"} {
		t.Run(query, func(t *testing.T) {
			wantCalls := 2
			if query == "!!!" {
				wantCalls = 1
			}
			docEngine := &retryCaptureEngine{engineType: string(engine.EngineElasticsearch), totals: []int64{0, 0, 0}}
			service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
			_, err := service.Retrieval(t.Context(), &RetrievalRequest{
				Question: query, TenantIDs: []string{"tenant-1"}, KbIDs: []string{"kb-1"},
				EmbeddingModel: &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}}, AllowDenseFallback: new(false),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(docEngine.requests) != wantCalls {
				t.Fatalf("calls = %d, want %d", len(docEngine.requests), wantCalls)
			}
		})
	}
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
		result:     &types.SearchResult{Chunks: []map[string]interface{}{}, Total: 1},
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

func TestBuildRetrievalFusionExprMatchesPythonEngineWeights(t *testing.T) {
	// Elasticsearch takes the reference's fixed pair regardless of the caller
	// weight: its first search is a vector recall pass, and the caller weight is
	// applied afterwards by RerankWithKNN.
	for _, callerWeight := range []*float64{float64Ptr(0.8), float64Ptr(0.3), nil} {
		expr := buildRetrievalFusionExpr(string(engine.EngineElasticsearch), 10, callerWeight)
		if got := expr.FusionParams["weights"]; got != esFusionWeights {
			t.Fatalf("expected Elasticsearch weights=%s, got %v", esFusionWeights, got)
		}
	}

	// Infinity fuses the two legs itself, so it keeps the caller weight.
	expr := buildRetrievalFusionExpr(string(engine.EngineInfinity), 10, float64Ptr(0.8))
	if got := expr.FusionParams["weights"]; got != "0.2,0.8" {
		t.Fatalf("expected Infinity weights=0.2,0.8, got %v", got)
	}
}

func TestSearchKeepsPythonFusionWeightForElasticsearch(t *testing.T) {
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
	if got := fusionExpr.FusionParams["weights"]; got != esFusionWeights {
		t.Fatalf("expected Elasticsearch weights=%s, got %v", esFusionWeights, got)
	}
}

// perIndexEngine searches each listed index separately, like Elasticsearch and Infinity.
type perIndexEngine struct {
	retrievalCountEngine
	indexNames []string
}

func (e *perIndexEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.indexNames = req.IndexNames
	merged := &types.SearchResult{}
	for range req.IndexNames {
		result, err := e.retrievalCountEngine.Search(ctx, req)
		if err != nil {
			return nil, err
		}
		merged.Chunks = append(merged.Chunks, result.Chunks...)
		merged.Total += result.Total
	}
	return merged, nil
}

func TestRetrievalSearchesEachTenantIndexOnce(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	docEngine := &perIndexEngine{retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
		{"id": "chunk-a", "content_ltks": "alpha", "content_with_weight": "alpha", "_score": 0.9},
		{"id": "chunk-b", "content_ltks": "alpha", "content_with_weight": "alpha", "_score": 0.8},
	}}}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
	threshold := 0.0
	aggs := false

	// Two datasets owned by the same tenant.
	result, err := service.Retrieval(t.Context(), &RetrievalRequest{
		Question:            "alpha",
		TenantIDs:           []string{"tenant-1", "tenant-1"},
		KbIDs:               []string{"kb-1", "kb-2"},
		Page:                1,
		PageSize:            10,
		SimilarityThreshold: &threshold,
		Aggs:                &aggs,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	var ids []string
	for _, chunk := range result.Chunks {
		ids = append(ids, chunk["chunk_id"].(string))
	}
	if !slices.Equal(ids, []string{"chunk-a", "chunk-b"}) || result.Total != 2 {
		t.Fatalf("chunks = %v (total %d), want [chunk-a chunk-b] (total 2)", ids, result.Total)
	}
	if !slices.Equal(docEngine.indexNames, []string{"ragflow_tenant-1"}) {
		t.Fatalf("index names = %v, want [ragflow_tenant-1]", docEngine.indexNames)
	}
}
