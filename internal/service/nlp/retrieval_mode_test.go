package nlp

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity/models"
)

func TestRetrievalModeJSON(t *testing.T) {
	for _, mode := range []RetrievalMode{RetrievalModeHybrid, RetrievalModeVector, RetrievalModeKeyword} {
		t.Run(string(mode), func(t *testing.T) {
			var got RetrievalMode
			if err := json.Unmarshal([]byte(`"`+mode+`"`), &got); err != nil || got != mode {
				t.Fatalf("解码结果 = %q，错误 = %v", got, err)
			}
			if err := mode.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, input := range []string{`null`, `""`, `"dense"`, `"VECTOR"`, `1`, `true`, `{}`, `[]`} {
		t.Run(input, func(t *testing.T) {
			got := RetrievalModeHybrid
			if err := json.Unmarshal([]byte(input), &got); err == nil || got != RetrievalModeHybrid {
				t.Fatalf("非法模式改变了原值：模式 = %q，错误 = %v", got, err)
			}
		})
	}
	var omitted struct {
		Mode RetrievalMode `json:"mode"`
	}
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil || omitted.Mode != "" {
		t.Fatalf("省略字段的模式 = %q，错误 = %v", omitted.Mode, err)
	}
	if err := RetrievalMode("").Validate(); err == nil {
		t.Fatal("显式校验接受了空模式")
	}
}

type modeEmbeddingDriver struct {
	models.ModelDriver
	calls int
}

func (d *modeEmbeddingDriver) Embed(context.Context, *string, models.EmbedRequest, *models.APIConfig, *models.EmbeddingConfig, *common.ModelUsage) ([]models.EmbeddingData, error) {
	d.calls++
	return []models.EmbeddingData{{Embedding: []float64{0.1, 0.2}}}, nil
}

type modeDocEngine struct {
	retrievalCountEngine
	engineType string
	requests   []*types.SearchRequest
	knnCalls   int
}

func (e *modeDocEngine) GetType() string { return e.engineType }

func (e *modeDocEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.requests = append(e.requests, req)
	return e.retrievalCountEngine.Search(ctx, req)
}

func (e *modeDocEngine) KNNScores(ctx context.Context, chunks []map[string]interface{}, vector []float64, count int) (map[string]interface{}, error) {
	e.knnCalls++
	return e.retrievalCountEngine.KNNScores(ctx, chunks, vector, count)
}

func TestSearchExplicitModesUseOneRecallPath(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, mode := range []RetrievalMode{RetrievalModeKeyword, RetrievalModeVector} {
		for _, threshold := range []float64{0, 0.85} {
			t.Run(string(mode)+"/"+strconv.FormatFloat(threshold, 'g', -1, 64), func(t *testing.T) {
				driver := &modeEmbeddingDriver{}
				docEngine := &modeDocEngine{engineType: "elasticsearch", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
					{"id": "semantic-neighbor", "content_ltks": "database connection refused", "_score": 0.9},
				}}}
				service := NewRetrievalService(docEngine, nil)
				result, err := service.Search(t.Context(), &RetrievalSearchRequest{
					Mode: mode, Question: "服务无法启动", TenantIDs: []string{"tenant"}, KbIDs: []string{"kb"}, DocIDs: []string{"doc"},
					EmbeddingModel: &models.EmbeddingModel{ModelDriver: driver}, SimilarityThreshold: threshold,
					Filter: map[string]interface{}{"category_kwd": "allowed"},
				})
				if err != nil {
					t.Fatal(err)
				}
				if result.Total != 1 || len(docEngine.requests) != 1 {
					t.Fatalf("召回结果 = %#v，请求次数 = %d", result, len(docEngine.requests))
				}
				request := docEngine.requests[0]
				if len(request.MatchExprs) != 1 {
					t.Fatalf("单路召回表达式 = %#v", request.MatchExprs)
				}
				if !reflect.DeepEqual(request.Filter["kb_id"], []string{"kb"}) || !reflect.DeepEqual(request.Filter["doc_id"], []string{"doc"}) || request.Filter["available_int"] != 1 || request.Filter["category_kwd"] != "allowed" {
					t.Fatalf("召回范围丢失：%#v", request.Filter)
				}
				if mode == RetrievalModeKeyword {
					if _, ok := request.MatchExprs[0].(*types.MatchTextExpr); !ok || driver.calls != 0 || len(result.QueryVector) != 0 {
						t.Fatalf("关键词模式触发了其他路径：表达式 = %T，embedding 次数 = %d", request.MatchExprs[0], driver.calls)
					}
				} else {
					dense, ok := request.MatchExprs[0].(*types.MatchDenseExpr)
					if !ok || driver.calls != 1 || dense.ExtraOptions["similarity"] != threshold || len(result.Keywords) != 0 {
						t.Fatalf("向量模式表达式 = %#v，embedding 次数 = %d，关键词 = %v", dense, driver.calls, result.Keywords)
					}
				}
			})
		}
	}
}

func TestSearchExplicitVectorRejectsMissingModelAndKeepsEmptyResult(t *testing.T) {
	service := NewRetrievalService(&modeDocEngine{engineType: "elasticsearch"}, nil)
	if _, err := service.Search(t.Context(), &RetrievalSearchRequest{Mode: RetrievalModeVector, Question: "query"}); err == nil {
		t.Fatal("向量模式接受了缺失的 embedding 模型")
	}
	docEngine := &modeDocEngine{engineType: "elasticsearch"}
	service = NewRetrievalService(docEngine, nil)
	result, err := service.Search(t.Context(), &RetrievalSearchRequest{
		Mode: RetrievalModeVector, Question: "query", SimilarityThreshold: 0.99,
		EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 || len(docEngine.requests) != 1 {
		t.Fatalf("空结果触发放宽：结果 = %#v，请求次数 = %d", result, len(docEngine.requests))
	}
}

func TestRetrievalExplicitModesKeepEngineScoresAcrossBackends(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, backend := range []string{"elasticsearch", "opensearch", "infinity", "oceanbase", "vastbase"} {
		for _, mode := range []RetrievalMode{RetrievalModeKeyword, RetrievalModeVector} {
			t.Run(backend+"/"+string(mode), func(t *testing.T) {
				docEngine := &modeDocEngine{engineType: backend, retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
					{"id": "first", "content_ltks": "unrelated text", "_score": 0.02},
					{"id": "second", "content_ltks": "query query", "_score": 0.01},
				}}}
				driver := &modeEmbeddingDriver{}
				service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
				result, err := service.Retrieval(t.Context(), &RetrievalRequest{
					Mode: mode, Question: "query", PageSize: 2, SimilarityThreshold: float64Ptr(0.9),
					EmbeddingModel: &models.EmbeddingModel{ModelDriver: driver}, VectorSimilarityWeight: float64Ptr(0.3),
				})
				if err != nil {
					t.Fatal(err)
				}
				if result.Total != 2 || result.Chunks[0]["chunk_id"] != "first" || result.Chunks[0]["similarity"] != 0.02 || docEngine.knnCalls != 0 {
					t.Fatalf("单路分数被重算或截断：结果 = %#v，KNNScores 次数 = %d", result, docEngine.knnCalls)
				}
				if mode == RetrievalModeKeyword && (driver.calls != 0 || result.Chunks[0]["vector_similarity"] != float64(0) || result.Chunks[0]["term_similarity"] != 0.02) {
					t.Fatalf("关键词分数组成错误：%#v，embedding 次数 = %d", result.Chunks[0], driver.calls)
				}
				if mode == RetrievalModeVector && (result.Chunks[0]["term_similarity"] != float64(0) || result.Chunks[0]["vector_similarity"] != 0.02) {
					t.Fatalf("向量分数组成错误：%#v", result.Chunks[0])
				}
			})
		}
	}
}

type modeRerankDriver struct{ models.ModelDriver }

func (*modeRerankDriver) Rerank(context.Context, *string, models.RerankRequest, *models.APIConfig, *models.RerankConfig, *common.ModelUsage) (*models.RerankResponse, error) {
	return &models.RerankResponse{Data: []models.RerankResult{{Index: 0, RelevanceScore: 0.1}, {Index: 1, RelevanceScore: 0.8}}}, nil
}

func TestRetrievalExplicitModesUseRerankerWithoutMixingLexicalScore(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, mode := range []RetrievalMode{RetrievalModeKeyword, RetrievalModeVector} {
		t.Run(string(mode), func(t *testing.T) {
			docEngine := &modeDocEngine{engineType: "elasticsearch", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
				{"id": "lexical-first", "content_ltks": "query", "_score": 0.9},
				{"id": "semantic-first", "content_ltks": "unrelated", "_score": 0.1, "pagerank_fea": 0.05},
			}}}
			result, err := NewRetrievalService(docEngine, &dao.DocumentDAO{}).Retrieval(t.Context(), &RetrievalRequest{
				Mode: mode, Question: "query", PageSize: 2, SimilarityThreshold: float64Ptr(0), VectorSimilarityWeight: float64Ptr(0),
				EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
				RerankModel:    models.NewRerankModel(&modeRerankDriver{}, nil, nil, 0), RankFeature: &map[string]float64{},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Chunks[0]["chunk_id"] != "semantic-first" || math.Abs(result.Chunks[0]["similarity"].(float64)-0.85) > 1e-12 || result.Chunks[0]["vector_similarity"] != 0.8 || result.Chunks[0]["term_similarity"] != float64(0) || docEngine.knnCalls != 0 {
				t.Fatalf("单路重排混入词法分数或丢失业务排序：%#v", result)
			}
		})
	}
}

func TestSearchExplicitKeywordRejectsQueryWithoutTokens(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	docEngine := &modeDocEngine{engineType: "elasticsearch"}
	_, err := NewRetrievalService(docEngine, nil).Search(t.Context(), &RetrievalSearchRequest{Mode: RetrievalModeKeyword, Question: "!!!"})
	if err == nil || len(docEngine.requests) != 0 {
		t.Fatalf("无关键词查询调用了引擎：请求次数 = %d，错误 = %v", len(docEngine.requests), err)
	}
}

func TestSearchExplicitHybridPreservesDefaultRecall(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	var defaultRequests []*types.SearchRequest
	for _, mode := range []RetrievalMode{"", RetrievalModeHybrid} {
		docEngine := &retryCaptureEngine{engineType: "elasticsearch", totals: []int64{0, 0, 1}}
		_, err := NewRetrievalService(docEngine, nil).Search(t.Context(), &RetrievalSearchRequest{
			Mode: mode, Question: "query", TenantIDs: []string{"tenant"}, KbIDs: []string{"kb"},
			DocIDs: []string{"doc"}, Filter: map[string]interface{}{"category_kwd": "allowed"}, VectorOnly: mode == RetrievalModeHybrid,
			EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(docEngine.requests) != 3 {
			t.Fatalf("混合模式请求次数 = %d，期望 3", len(docEngine.requests))
		}
		for _, request := range docEngine.requests {
			if len(request.MatchExprs) == 0 || !reflect.DeepEqual(request.Filter["doc_id"], []string{"doc"}) || request.Filter["category_kwd"] != "allowed" {
				t.Fatalf("混合模式重试丢失检索条件或范围：%#v", request)
			}
		}
		if mode == "" {
			defaultRequests = docEngine.requests
		} else if !reflect.DeepEqual(defaultRequests, docEngine.requests) {
			t.Fatalf("显式混合模式改变召回路径：默认 = %#v，显式 = %#v", defaultRequests, docEngine.requests)
		}
	}
}

func TestRetrievalExplicitModesRejectUnknownMode(t *testing.T) {
	service := NewRetrievalService(nil, nil)
	if _, err := service.Search(t.Context(), &RetrievalSearchRequest{Mode: "unknown"}); err == nil {
		t.Fatal("Search 接受了未知模式")
	}
	if _, err := service.Retrieval(t.Context(), &RetrievalRequest{Mode: "unknown"}); err == nil {
		t.Fatal("Retrieval 接受了未知模式")
	}
	if _, err := service.Retrieval(t.Context(), &RetrievalRequest{Mode: RetrievalModeVector, Question: "query"}); err == nil {
		t.Fatal("Retrieval 向量模式接受了缺失模型")
	}
}

func TestRetrievalExplicitModesPaginateAndRestrictRerankerToFirstPage(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, mode := range []RetrievalMode{RetrievalModeKeyword, RetrievalModeVector} {
		t.Run(string(mode), func(t *testing.T) {
			docEngine := &modeDocEngine{engineType: "elasticsearch", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
				{"id": "first", "content_ltks": "query", "_score": 0.9},
				{"id": "second", "content_ltks": "query", "_score": 0.8},
				{"id": "third", "content_ltks": "query", "_score": 0.7},
			}}}
			service := NewRetrievalService(docEngine, &dao.DocumentDAO{})
			req := &RetrievalRequest{
				Mode: mode, Question: "query", Page: 2, PageSize: 1,
				EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
			}
			result, err := service.Retrieval(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 3 || len(result.Chunks) != 1 || result.Chunks[0]["chunk_id"] != "second" || docEngine.knnCalls != 0 {
				t.Fatalf("单路分页结果错误：%#v", result)
			}
			req.RerankModel = models.NewRerankModel(&modeRerankDriver{}, nil, nil, 0)
			if _, err := service.Retrieval(t.Context(), req); err == nil || len(docEngine.requests) != 1 {
				t.Fatalf("重排第二页未提前拒绝：请求次数 = %d，错误 = %v", len(docEngine.requests), err)
			}
		})
	}
}

func TestSearchExplicitModeTakesPrecedenceOverVectorOnly(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, mode := range []RetrievalMode{"", RetrievalModeHybrid, RetrievalModeKeyword} {
		t.Run(string(mode), func(t *testing.T) {
			docEngine := &modeDocEngine{engineType: "elasticsearch", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{{"id": "one"}}}}
			_, err := NewRetrievalService(docEngine, nil).Search(t.Context(), &RetrievalSearchRequest{
				Mode: mode, VectorOnly: true, Question: "query", EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
			})
			if err != nil {
				t.Fatal(err)
			}
			expressions := docEngine.requests[0].MatchExprs
			switch mode {
			case "":
				if _, ok := expressions[0].(*types.MatchDenseExpr); len(expressions) != 1 || !ok {
					t.Fatalf("既有 VectorOnly 路径变化：%#v", expressions)
				}
			case RetrievalModeHybrid:
				if len(expressions) != 3 {
					t.Fatalf("显式混合模式未覆盖 VectorOnly：%#v", expressions)
				}
			case RetrievalModeKeyword:
				if _, ok := expressions[0].(*types.MatchTextExpr); len(expressions) != 1 || !ok {
					t.Fatalf("显式关键词模式未覆盖 VectorOnly：%#v", expressions)
				}
			}
		})
	}
}

func TestEngineRecallScoresReadChunkScoresInIDOrder(t *testing.T) {
	result := &RetrievalSearchResult{
		IDs: []string{"third", "first", "second", "missing"},
		Chunks: []map[string]interface{}{
			{"id": "first", "_score": 0.7, "SCORE": 1.0},
			{"_id": "second", "SCORE": 0.8},
			{"id": "third", "SIMILARITY": 0.9},
			{"_score": 100.0},
		},
		Field: map[string]map[string]interface{}{"first": {"_score": 0.01}},
	}
	if scores := engineRecallScores(result); !reflect.DeepEqual(scores, []float64{0.9, 0.7, 0.8, 0}) {
		t.Fatalf("引擎召回分数 = %v", scores)
	}
}

func TestRetrievalExplicitModesKeepEngineBusinessOrder(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, mode := range []RetrievalMode{RetrievalModeKeyword, RetrievalModeVector} {
		t.Run(string(mode), func(t *testing.T) {
			docEngine := &modeDocEngine{engineType: "vastbase", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
				{"id": "business-first", "content_ltks": "query", "_score": 0.5, "pagerank_fea": 1.0},
				{"id": "raw-score-first", "content_ltks": "query", "_score": 0.8, "pagerank_fea": 0.0},
			}}}
			result, err := NewRetrievalService(docEngine, &dao.DocumentDAO{}).Retrieval(t.Context(), &RetrievalRequest{
				Mode: mode, Question: "query", PageSize: 2,
				EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 2 || result.Chunks[0]["chunk_id"] != "business-first" || result.Chunks[0]["similarity"] != 0.5 {
				t.Fatalf("业务排序被原始分数覆盖：%#v", result)
			}
		})
	}
}

func TestRetrievalExplicitKeywordKeepsNegativeLexicalScores(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	docEngine := &modeDocEngine{engineType: "vastbase", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
		{"id": "negative", "content_ltks": "query", "_score": -0.1},
	}}}
	result, err := NewRetrievalService(docEngine, &dao.DocumentDAO{}).Retrieval(t.Context(), &RetrievalRequest{
		Mode: RetrievalModeKeyword, Question: "query", SimilarityThreshold: float64Ptr(0.9),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Chunks[0]["similarity"] != -0.1 || result.Chunks[0]["term_similarity"] != -0.1 {
		t.Fatalf("词法分数仍被相似度门槛截断：%#v", result)
	}
}

type projectedModeEngine struct{ modeDocEngine }

func (e *projectedModeEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	result, err := e.modeDocEngine.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	projected := make([]map[string]interface{}, len(result.Chunks))
	for i, chunk := range result.Chunks {
		projected[i] = map[string]interface{}{"id": chunk["id"]}
		for _, field := range req.SelectFields {
			if value, ok := chunk[field]; ok {
				projected[i][field] = value
			}
		}
	}
	return &types.SearchResult{Chunks: projected, Total: result.Total}, nil
}

func (*projectedModeEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	return (&elasticsearch.Engine{}).GetFields(chunks, fields)
}

func TestRetrievalExplicitRerankPreservesProjectedRankFeatures(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	for _, mode := range []RetrievalMode{RetrievalModeKeyword, RetrievalModeVector} {
		t.Run(string(mode), func(t *testing.T) {
			docEngine := &projectedModeEngine{modeDocEngine: modeDocEngine{engineType: "elasticsearch", retrievalCountEngine: retrievalCountEngine{rows: []map[string]interface{}{
				{"id": "business-first", "content_ltks": "query", "_score": 0.1, common.PAGERANK_FLD: 0.25, common.TAG_FLD: map[string]interface{}{"priority": 2.0}},
				{"id": "model-first", "content_ltks": "unrelated", "_score": 0.9},
			}}}}
			result, err := NewRetrievalService(docEngine, &dao.DocumentDAO{}).Retrieval(t.Context(), &RetrievalRequest{
				Mode: mode, Question: "query", PageSize: 2,
				EmbeddingModel: &models.EmbeddingModel{ModelDriver: &modeEmbeddingDriver{}},
				RerankModel:    models.NewRerankModel(&modeRerankDriver{}, nil, nil, 0), RankFeature: &map[string]float64{"priority": 1.0},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(docEngine.requests[0].SelectFields, common.PAGERANK_FLD) || !slices.Contains(docEngine.requests[0].SelectFields, common.TAG_FLD) {
				t.Fatalf("业务特征未投影：%v", docEngine.requests[0].SelectFields)
			}
			if result.Total != 2 || result.Chunks[0]["chunk_id"] != "business-first" || math.Abs(result.Chunks[0]["similarity"].(float64)-10.35) > 1e-12 || result.Chunks[0]["vector_similarity"] != 0.1 {
				t.Fatalf("真实 ES 字段投影丢失了标签或 pagerank：%#v", result)
			}
		})
	}
}
