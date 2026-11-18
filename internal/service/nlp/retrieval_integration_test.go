//go:build integration

package nlp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/server/config"
	"ragflow/internal/tokenizer"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Requires ES_HOST (and optional ES_USERNAME/ES_PASSWORD), RAGFLOW_INFINITY_URI,
// TEI_BASE_URL serving BAAI/bge-small-en-v1.5, and the usual tokenizer resources.
// Missing service configuration skips the affected backend; configured service
// failures and retrieval assertion failures remain fatal.
// All three tests cover both backends when their configuration is supplied.
const retrievalIntegrationQuestion = "rocket propulsion engines exhaust thrust"

type retrievalIntegrationFixture struct {
	service *RetrievalService
	model   *models.EmbeddingModel
	tenant  string
	kb      string
	engine  string
}

// Catches a disconnected dense leg, ignored weights, or an ES second-pass score
// that no longer uses the caller's weight. No cross-engine score parity is assumed.
func TestRetrievalModesAndWeightsIntegration(t *testing.T) {
	for _, backend := range []string{"elasticsearch", "infinity"} {
		t.Run(backend, func(t *testing.T) {
			f := newRetrievalIntegrationFixture(t, backend)
			t.Run("text", func(t *testing.T) {
				req := f.request(0, 0)
				req.EmbeddingModel = nil
				mustContainRetrievalChunks(t, f.retrieve(t, req), "lexical")
			})
			t.Run("dense", func(t *testing.T) {
				req := f.request(1, 0)
				req.VectorOnly = true
				mustContainRetrievalChunks(t, f.retrieve(t, req), "semantic")
			})
			t.Run("hybrid_weights", func(t *testing.T) {
				low := f.retrieve(t, f.request(0.2, 0))
				high := f.retrieve(t, f.request(0.7, 0))
				mustContainRetrievalChunks(t, low, "lexical", "semantic")
				mustContainRetrievalChunks(t, high, "lexical", "semantic")
				gap := func(result *RetrievalResult) float64 {
					return retrievalIntegrationScore(t, result, "semantic", "similarity") - retrievalIntegrationScore(t, result, "lexical", "similarity")
				}
				if gap(high) <= gap(low)+1e-5 {
					t.Fatalf("increasing vector weight must improve semantic relative to lexical candidate: gaps %g -> %g", gap(low), gap(high))
				}
				if backend == "elasticsearch" {
					for _, tt := range []struct {
						result *RetrievalResult
						weight float64
					}{{low, 0.2}, {high, 0.7}} {
						for _, id := range []string{"lexical", "semantic"} {
							term := retrievalIntegrationScore(t, tt.result, id, "term_similarity")
							vector := retrievalIntegrationScore(t, tt.result, id, "vector_similarity")
							want := (1-tt.weight)*term + tt.weight*vector
							if got := retrievalIntegrationScore(t, tt.result, id, "similarity"); math.Abs(got-want) > 1e-6 {
								t.Fatalf("%s: similarity=%g, want blended score %g", id, got, want)
							}
						}
					}
				}
			})
		})
	}
}

// Catches a missing final threshold or a total copied from unfiltered hits.
func TestRetrievalThresholdIntegration(t *testing.T) {
	for _, backend := range []string{"elasticsearch", "infinity"} {
		t.Run(backend, func(t *testing.T) {
			f := newRetrievalIntegrationFixture(t, backend, backend != "infinity")
			low := f.scopedRequest(0)
			result := f.retrieve(t, low)
			for _, chunk := range result.Chunks {
				t.Logf("threshold fixture: id=%v similarity=%v cutoff=%g", chunk["chunk_id"], chunk["similarity"], f.threshold())
			}
			mustHaveRetrievalChunks(t, result, []string{"a1", "a2", "b1", "lexical"})
			result = f.retrieve(t, f.scopedRequest(f.threshold()))
			mustHaveRetrievalChunks(t, result, []string{"a1", "a2", "b1"})
		})
	}
}

// Catches pagination before filtering, page-local totals/aggregations, and a
// lost document scope. Literal IDs/counts keep the baseline from being an oracle.
func TestRetrievalPaginationAndAggregationsIntegration(t *testing.T) {
	for _, backend := range []string{"elasticsearch", "infinity"} {
		t.Run(backend, func(t *testing.T) {
			f := newRetrievalIntegrationFixture(t, backend, backend != "infinity")
			full := f.retrieve(t, f.scopedRequest(f.threshold()))
			mustHaveRetrievalChunks(t, full, []string{"a1", "a2", "b1"})
			for i := 1; i < len(full.Chunks); i++ {
				previous := full.Chunks[i-1]["similarity"].(float64)
				current := full.Chunks[i]["similarity"].(float64)
				if previous-current < 1e-6 {
					t.Fatal("pagination fixture requires distinct scores; tied backend order cannot define page boundaries")
				}
			}
			wantAggs := []map[string]interface{}{
				{"doc_name": "alpha.txt", "doc_id": "doc-a", "count": 2},
				{"doc_name": "beta.txt", "doc_id": "doc-b", "count": 1},
			}
			if !reflect.DeepEqual(full.DocAggs, wantAggs) {
				t.Fatalf("doc_aggs=%v, want %v", full.DocAggs, wantAggs)
			}
			var all []string
			for page := 1; page <= 2; page++ {
				req := f.scopedRequest(f.threshold())
				req.Page, req.PageSize = page, 2
				result := f.retrieve(t, req)
				begin, end := (page-1)*2, min(page*2, len(full.Chunks))
				if !reflect.DeepEqual(result.Chunks, full.Chunks[begin:end]) {
					t.Fatalf("page %d differs from sorted result slice: %v", page, result.Chunks)
				}
				if result.Total != 3 || !reflect.DeepEqual(result.DocAggs, wantAggs) {
					t.Fatalf("page %d: total=%d doc_aggs=%v", page, result.Total, result.DocAggs)
				}
				all = append(all, retrievalIntegrationIDs(result)...)
			}
			slices.Sort(all)
			if !slices.Equal(all, []string{"a1", "a2", "b1"}) {
				t.Fatalf("pagination duplicated, lost, or leaked chunks: %v", all)
			}
		})
	}
}

func newRetrievalIntegrationFixture(t *testing.T, backend string, lexicalBoost ...bool) *retrievalIntegrationFixture {
	t.Helper()
	if os.Getenv("TEI_BASE_URL") == "" {
		t.Skip("set TEI_BASE_URL to run real retrieval integration tests with BAAI/bge-small-en-v1.5")
	}
	switch backend {
	case "elasticsearch":
		if os.Getenv("ES_HOST") == "" {
			t.Skip("set ES_HOST to run Elasticsearch retrieval integration tests")
		}
	case "infinity":
		if os.Getenv("RAGFLOW_INFINITY_URI") == "" {
			t.Skip("set RAGFLOW_INFINITY_URI to run Infinity retrieval integration tests")
		}
	default:
		t.Fatalf("unknown backend %q", backend)
	}
	// Weight competition needs a strong lexical candidate. Infinity threshold
	// and pagination fixtures opt out so unrelated content is not boosted by
	// important_kwd^30/important_tks^20. Existing ES and reranker fixtures retain it.
	boostLexical := len(lexicalBoost) == 0 || lexicalBoost[0]
	if err := common.InitLogger("error", common.FileOutput{}, ""); err != nil {
		t.Fatal(err)
	}
	if !tokenizer.IsInitialized() {
		if err := tokenizer.Init(&tokenizer.PoolConfig{MinSize: 1, MaxSize: 2}); err != nil {
			t.Fatalf("initialize production tokenizer: %v", err)
		}
		t.Cleanup(tokenizer.Close)
	}
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		t.Fatal(err)
	}
	previousQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	t.Cleanup(func() { globalQueryBuilder = previousQueryBuilder })

	baseURL := strings.TrimRight(os.Getenv("TEI_BASE_URL"), "/")
	if baseURL == "" {
		t.Fatal("TEI_BASE_URL must point to BAAI/bge-small-en-v1.5")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(baseURL + "/info")
	if err != nil {
		t.Fatalf("TEI /info: %v", err)
	}
	var info struct {
		ModelID string `json:"model_id"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&info)
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusOK || decodeErr != nil || closeErr != nil || info.ModelID != "BAAI/bge-small-en-v1.5" {
		t.Fatalf("TEI must serve BAAI/bge-small-en-v1.5: status=%d model=%q decode=%v close=%v", response.StatusCode, info.ModelID, decodeErr, closeErr)
	}
	modelName := info.ModelID
	model := models.NewEmbeddingModel(models.NewBuiltinModel(baseURL, modelName), &modelName, &models.APIConfig{}, 512)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "documents.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite: %v", err)
		}
	})
	if err := db.AutoMigrate(&entity.Document{}); err != nil {
		t.Fatal(err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	var de engine.DocEngine
	switch backend {
	case "elasticsearch":
		de, err = elasticsearch.NewEngine(ctx, config.ElasticsearchConfig{Hosts: os.Getenv("ES_HOST"), Username: os.Getenv("ES_USERNAME"), Password: os.Getenv("ES_PASSWORD")})
	case "infinity":
		de, err = infinity.NewEngine(ctx, config.InfinityConfig{URI: os.Getenv("RAGFLOW_INFINITY_URI"), DBName: "ragflow_retrieval_test", PostgresPort: 5432})
	}
	if err != nil {
		t.Fatalf("connect to %s: %v", backend, err)
	}
	t.Cleanup(func() {
		if err := de.Close(); err != nil {
			t.Errorf("close %s: %v", backend, err)
		}
	})
	f := &retrievalIntegrationFixture{service: NewRetrievalService(de, dao.NewDocumentDAO()), model: model, tenant: fmt.Sprintf("retrieval_%d", time.Now().UnixNano()), kb: "retrieval", engine: backend}
	index := "ragflow_" + f.tenant
	rows := []struct{ id, doc, name, content string }{
		{"a1", "doc-a", "alpha.txt", retrievalIntegrationQuestion},
		{"a2", "doc-a", "alpha.txt", retrievalIntegrationQuestion + " vacuum"},
		{"b1", "doc-b", "beta.txt", retrievalIntegrationQuestion + " spacecraft"},
		{"lexical", "doc-low", "garden.txt", "Rocket Engines is a restaurant serving peppery salad leaves, fresh tomatoes, garden vegetables and homemade bread."},
		{"semantic", "doc-semantic", "science.txt", "A rocket engine produces thrust by ejecting hot gas into space."},
		{"excluded", "doc-excluded", "excluded.txt", retrievalIntegrationQuestion},
	}
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = row.content
	}
	embeddings, err := model.Embed(ctx, models.EmbedRequest{Texts: texts}, &models.EmbeddingConfig{}, nil)
	if err != nil {
		t.Fatalf("embed fixture with real model: %v", err)
	}
	if len(embeddings) != len(rows) || len(embeddings[0].Embedding) != 384 {
		t.Fatal("fixture requires all six 384-dimensional bge-small-en-v1.5 embeddings")
	}
	if err := de.CreateChunkStore(ctx, index, f.kb, 384, "naive", "English"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := de.DropChunkStore(cleanupCtx, index, f.kb); err != nil {
			t.Errorf("drop fixture store: %v", err)
		}
	})
	chunks := make([]map[string]interface{}, 0, len(rows))
	documents := map[string]bool{}
	for i, row := range rows {
		if !documents[row.doc] {
			if err := db.Create(&entity.Document{ID: row.doc, KbID: f.kb, Name: &row.name, ParserConfig: entity.JSONMap{}}).Error; err != nil {
				t.Fatal(err)
			}
			documents[row.doc] = true
		}
		tokens, err := tokenizer.Tokenize(row.content)
		if err != nil {
			t.Fatal(err)
		}
		fine, err := tokenizer.FineGrainedTokenize(tokens)
		if err != nil {
			t.Fatal(err)
		}
		chunk := map[string]interface{}{"id": row.id, "doc_id": row.doc, "kb_id": f.kb, "docnm_kwd": row.name, "content_with_weight": row.content, "content_ltks": tokens, "content_sm_ltks": fine, "available_int": 1, "q_384_vec": embeddings[i].Embedding}
		if row.id == "lexical" && boostLexical {
			chunk["important_kwd"] = strings.Fields(retrievalIntegrationQuestion)
			chunk["important_tks"], err = tokenizer.Tokenize(retrievalIntegrationQuestion)
			if err != nil {
				t.Fatal(err)
			}
		}
		chunks = append(chunks, chunk)
	}
	if _, err := de.InsertChunks(ctx, chunks, index, f.kb, "English"); err != nil {
		t.Fatal(err)
	}
	// Retry visibility only. Authentication, transport and query errors fail now.
	visibilityCtx, visibilityCancel := context.WithTimeout(ctx, 30*time.Second)
	defer visibilityCancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		visible, err := de.Search(visibilityCtx, &types.SearchRequest{IndexNames: []string{index}, KbIDs: []string{f.kb}, Limit: 16, SelectFields: []string{"id"}})
		if err != nil {
			t.Fatalf("fixture visibility query: %v", err)
		}
		if len(visible.Chunks) == len(rows) {
			break
		}
		select {
		case <-visibilityCtx.Done():
			t.Fatalf("index visibility timeout: got %d chunks, want %d", len(visible.Chunks), len(rows))
		case <-ticker.C:
		}
	}
	return f
}

func (f *retrievalIntegrationFixture) request(weight, threshold float64) *RetrievalRequest {
	return &RetrievalRequest{Question: retrievalIntegrationQuestion, TenantIDs: []string{f.tenant}, KbIDs: []string{f.kb}, Page: 1, PageSize: 16, RerankCandidatesCount: new(16), KNNTopK: new(16), KNNNumCandidates: new(32), VectorSimilarityWeight: &weight, SimilarityThreshold: &threshold, RankFeature: &map[string]float64{}, EmbeddingModel: f.model, AllowDenseFallback: new(false), Language: "English"}
}

func (f *retrievalIntegrationFixture) scopedRequest(threshold float64) *RetrievalRequest {
	req := f.request(0.7, threshold)
	req.DocIDs = []string{"doc-a", "doc-b", "doc-low"}
	return req
}

func (f *retrievalIntegrationFixture) threshold() float64 {
	// ES returns its clean kNN score on a different scale from Infinity's atan
	// fusion. These are fixed cutoffs, not thresholds fitted to retrieved scores.
	if f.engine == "elasticsearch" {
		return 0.85
	}
	return 0.70
}

func (f *retrievalIntegrationFixture) retrieve(t *testing.T, req *RetrievalRequest) *RetrievalResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := f.service.Retrieval(ctx, req)
	if err != nil {
		t.Fatalf("Retrieval: %v", err)
	}
	previousScore := math.Inf(1)
	for _, chunk := range result.Chunks {
		score, ok := chunk["similarity"].(float64)
		if !ok || math.IsNaN(score) || math.IsInf(score, 0) || score > previousScore {
			t.Fatalf("invalid or unsorted similarity: %v", chunk)
		}
		if chunk["kb_id"] != f.kb {
			t.Fatalf("chunk escaped fixture dataset: %v", chunk)
		}
		previousScore = score
	}
	return result
}

func retrievalIntegrationIDs(result *RetrievalResult) []string {
	ids := make([]string, 0, len(result.Chunks))
	for _, chunk := range result.Chunks {
		id, _ := chunk["chunk_id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func mustContainRetrievalChunks(t *testing.T, result *RetrievalResult, ids ...string) {
	t.Helper()
	got := retrievalIntegrationIDs(result)
	for _, id := range ids {
		if !slices.Contains(got, id) {
			t.Fatalf("missing target chunk %q: got %v", id, got)
		}
	}
}

func mustHaveRetrievalChunks(t *testing.T, result *RetrievalResult, want []string) {
	t.Helper()
	got := retrievalIntegrationIDs(result)
	slices.Sort(got)
	if !slices.Equal(got, want) || result.Total != int64(len(want)) {
		t.Fatalf("chunks=%v total=%d, want chunks=%v total=%d", got, result.Total, want, len(want))
	}
}

func retrievalIntegrationScore(t *testing.T, result *RetrievalResult, id, field string) float64 {
	t.Helper()
	for _, chunk := range result.Chunks {
		if chunk["chunk_id"] == id {
			score, ok := chunk[field].(float64)
			if !ok || math.IsNaN(score) || math.IsInf(score, 0) {
				t.Fatalf("%s has invalid %s: %v", id, field, chunk[field])
			}
			return score
		}
	}
	t.Fatalf("chunk %s missing", id)
	return 0
}
