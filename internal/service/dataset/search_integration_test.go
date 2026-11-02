//go:build integration

package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/server/config"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

// Requires ES_HOST (optional ES_USERNAME/ES_PASSWORD), TEI_BASE_URL serving
// BAAI/bge-small-en-v1.5, and the production tokenizer resources. No parallel
// tests: SQLite temporarily replaces the process-wide dao.DB.
// Missing ES_HOST or TEI_BASE_URL skips these tests; configured service failures
// and retrieval assertion failures remain fatal.
const searchIntegrationQuestion = "rocket propulsion engines exhaust thrust"

type searchIntegrationFixture struct {
	service *DatasetService
	ctx     context.Context
	owner   string
}

// Catches a saved config that is loaded but ignored when preparing retrieval.
func TestSearchDatasetsSearchIDOverridesIntegration(t *testing.T) {
	f := newSearchIntegrationFixture(t)
	for _, tt := range []struct {
		name                        string
		weight, threshold           float64
		wrongWeight, wrongThreshold float64
	}{
		{"weight", 0.7, 0, 0.2, 0},
		{"threshold", 0.7, 0.85, 0.7, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			searches := service.NewSearchService()
			created, err := searches.CreateSearch(f.ctx, f.owner, "retrieval_"+tt.name, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = searches.UpdateSearch(f.ctx, f.owner, created.SearchID, &service.UpdateSearchRequest{
				Name: "retrieval_" + tt.name,
				SearchConfig: map[string]interface{}{
					"keywords_similarity_weight": 1 - tt.weight,
					"similarity_threshold":       tt.threshold,
					"top_k":                      16, "rerank_candidates_count": 16,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			expected := f.search(t, f.request(tt.weight, tt.threshold))
			conflicting := f.request(tt.wrongWeight, tt.wrongThreshold)
			control := f.search(t, conflicting)
			conflicting.SearchID = &created.SearchID
			actual := f.search(t, conflicting)
			if expected.Total == 0 || actual.Total != expected.Total || !reflect.DeepEqual(actual.Chunks, expected.Chunks) {
				t.Fatalf("saved parameters must determine results: expected=%+v actual=%+v", expected, actual)
			}
			if tt.name == "weight" {
				mustHaveSearchIntegrationChunks(t, expected, "high-a", "low")
				mustHaveSearchIntegrationChunks(t, control, "high-a", "low")
				if math.Abs(searchIntegrationScore(t, expected, "low")-searchIntegrationScore(t, control, "low")) < 1e-5 {
					t.Fatal("weight fixture must distinguish saved and conflicting request weights")
				}
			} else {
				mustHaveSearchIntegrationChunks(t, expected, "high-a")
				mustHaveSearchIntegrationChunks(t, control, "high-a", "low")
			}
		})
	}
}

// Catches comparing model row IDs instead of base names, or bypassing the
// consistency guard before loading a model and executing retrieval.
func TestSearchDatasetsEmbeddingConsistencyIntegration(t *testing.T) {
	f := newSearchIntegrationFixture(t)
	t.Run("same_base_different_records", func(t *testing.T) {
		req := f.request(0.7, 0)
		req.DatasetIDs = []string{"kb-a", "kb-b"}
		result := f.search(t, req)
		mustHaveSearchIntegrationChunks(t, result, "high-a", "high-b", "low")
		for _, chunk := range result.Chunks {
			wantKB := "kb-a"
			if chunk["id"] == "high-b" {
				wantKB = "kb-b"
			}
			if chunk["dataset_id"] != wantKB {
				t.Fatalf("incorrect dataset ownership: %v", chunk)
			}
		}
	})
	t.Run("different_bases", func(t *testing.T) {
		if err := dao.DB.Model(&entity.TenantModel{}).Where("id = ?", "22222222222222222222222222222222").Update("model_name", "BAAI/bge-large-en-v1.5").Error; err != nil {
			t.Fatal(err)
		}
		if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-b").Update("embd_id", "BAAI/bge-large-en-v1.5@Builtin").Error; err != nil {
			t.Fatal(err)
		}
		req := f.request(0.7, 0)
		req.DatasetIDs = []string{"kb-a", "kb-b"}
		result, err := f.service.SearchDatasets(f.ctx, req, f.owner)
		if err == nil || !strings.Contains(err.Error(), "datasets use different embedding models") || result != nil {
			t.Fatalf("different base models must be rejected before retrieval: result=%+v err=%v", result, err)
		}
	})
}

func newSearchIntegrationFixture(t *testing.T) *searchIntegrationFixture {
	t.Helper()
	if os.Getenv("ES_HOST") == "" || os.Getenv("TEI_BASE_URL") == "" {
		t.Skip("set ES_HOST and TEI_BASE_URL to run real SearchDatasets integration tests")
	}
	if err := common.InitLogger("error", common.FileOutput{}, ""); err != nil {
		t.Fatal(err)
	}
	if !tokenizer.IsInitialized() {
		if err := tokenizer.Init(&tokenizer.PoolConfig{MinSize: 1, MaxSize: 2}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tokenizer.Close)
	}
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		t.Fatal(err)
	}
	if err := nlp.InitQueryBuilder(""); err != nil {
		t.Fatal(err)
	}
	// Verify the deployed model rather than accepting any model with 384 dimensions.
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(strings.TrimRight(os.Getenv("TEI_BASE_URL"), "/") + "/info")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		ModelID string `json:"model_id"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&info)
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusOK || decodeErr != nil || closeErr != nil || info.ModelID != "BAAI/bge-small-en-v1.5" {
		t.Fatalf("TEI must serve BAAI/bge-small-en-v1.5: status=%d model=%q decode=%v close=%v", response.StatusCode, info.ModelID, decodeErr, closeErr)
	}
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Search{}); err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("search_%d", time.Now().UnixNano())
	if err := db.Create(&entity.Tenant{ID: owner, ParserIDs: "general"}).Error; err != nil {
		t.Fatal(err)
	}
	ensureDatasetTestMembership(t, owner, owner, string(permission.RoleOwner))
	ctx, cancel := context.WithTimeout(withDatasetTestContext(t.Context(), owner, owner), 3*time.Minute)
	t.Cleanup(cancel)
	de, err := elasticsearch.NewEngine(ctx, config.ElasticsearchConfig{Hosts: os.Getenv("ES_HOST"), Username: os.Getenv("ES_USERNAME"), Password: os.Getenv("ES_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := de.Close(); err != nil {
			t.Errorf("close Elasticsearch: %v", err)
		}
	})
	f := &searchIntegrationFixture{ctx: ctx, owner: owner, service: &DatasetService{kbDAO: dao.NewKnowledgebaseDAO(), documentDAO: dao.NewDocumentDAO(), searchService: service.NewSearchService(), docEngine: de}}
	// Different model records resolve to the same base. The ordinary Builtin
	// reference still goes through ModelFactory to the real TEI driver.
	if err := db.Create(&entity.TenantModelProvider{ID: "provider", ProviderName: "Builtin", UserID: owner, TenantID: owner}).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"11111111111111111111111111111111", "22222222222222222222222222222222"} {
		instanceID := fmt.Sprintf("instance-%d", i)
		if err := db.Create(&entity.TenantModelInstance{ID: instanceID, InstanceName: instanceID, ProviderID: "provider", Status: "active", Extra: "{}"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&entity.TenantModel{ID: id, ModelName: info.ModelID, ProviderID: "provider", InstanceID: instanceID, ModelType: int(entity.ModelTypeEmbedding), Status: "active"}).Error; err != nil {
			t.Fatal(err)
		}
		kb := fmt.Sprintf("kb-%c", 'a'+i)
		if err := db.Create(&entity.Knowledgebase{ID: kb, Name: kb, TenantID: owner, CreatedBy: owner, Permission: "me", EmbdID: info.ModelID + "@Builtin", TenantEmbdID: &id, ParserConfig: entity.JSONMap{}}).Error; err != nil {
			t.Fatal(err)
		}
		grantDatasetTestResource(t, owner, "kb", kb, owner, "owner")
	}
	model, err := service.NewModelFactory().NewEmbeddingModel(ctx, service.ModelAccess{TenantID: owner}, info.ModelID+"@Builtin")
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct{ id, kb, content string }{
		{"high-a", "kb-a", searchIntegrationQuestion},
		{"low", "kb-a", "Rocket Engines is a restaurant serving peppery salad leaves, fresh tomatoes, garden vegetables and homemade bread."},
		{"high-b", "kb-b", searchIntegrationQuestion + " spacecraft"},
	}
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = row.content
	}
	embeddings, err := model.Embed(ctx, models.EmbedRequest{Texts: texts}, &models.EmbeddingConfig{}, nil)
	if err != nil || len(embeddings) != len(rows) {
		t.Fatalf("embed fixture: count=%d err=%v", len(embeddings), err)
	}
	// Both datasets share an isolated tenant index, just as production does.
	index := "ragflow_" + owner
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := de.DropChunkStore(cleanupCtx, index, "kb-a"); err != nil {
			t.Errorf("drop fixture index: %v", err)
		}
	})
	for i, row := range rows {
		if len(embeddings[i].Embedding) != 384 {
			t.Fatal("expected 384-dimensional BGE embeddings")
		}
		if err := db.Create(&entity.Document{ID: row.id, KbID: row.kb, ParserConfig: entity.JSONMap{}}).Error; err != nil {
			t.Fatal(err)
		}
		if err := de.CreateChunkStore(ctx, index, row.kb, 384, "naive", "English"); err != nil {
			t.Fatal(err)
		}
		tokens, err := tokenizer.Tokenize(row.content)
		if err != nil {
			t.Fatal(err)
		}
		fine, err := tokenizer.FineGrainedTokenize(tokens)
		if err != nil {
			t.Fatal(err)
		}
		chunk := map[string]interface{}{"id": row.id, "doc_id": row.id, "kb_id": row.kb, "docnm_kwd": row.id + ".txt", "content_with_weight": row.content, "content_ltks": tokens, "content_sm_ltks": fine, "available_int": 1, "q_384_vec": embeddings[i].Embedding}
		if row.id == "low" {
			chunk["important_kwd"] = strings.Fields(searchIntegrationQuestion)
		}
		if _, err := de.InsertChunks(ctx, []map[string]interface{}{chunk}, index, row.kb, "English"); err != nil {
			t.Fatal(err)
		}
	}
	visibilityCtx, visibilityCancel := context.WithTimeout(ctx, 30*time.Second)
	defer visibilityCancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, err := de.Search(visibilityCtx, &types.SearchRequest{IndexNames: []string{index}, KbIDs: []string{"kb-a", "kb-b"}, Limit: 16, SelectFields: []string{"id"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Chunks) == len(rows) {
			break
		}
		select {
		case <-visibilityCtx.Done():
			t.Fatal("fixture index visibility timeout")
		case <-ticker.C:
		}
	}
	return f
}

func (f *searchIntegrationFixture) request(weight, threshold float64) *service.SearchDatasetsRequest {
	return &service.SearchDatasetsRequest{Question: searchIntegrationQuestion, DatasetIDs: []string{"kb-a"}, Page: new(1), PageSize: new(16), RerankCandidatesCount: new(16), KNNTopK: new(16), KNNNumCandidates: new(32), VectorSimilarityWeight: &weight, SimilarityThreshold: &threshold, IncludeCompiledChunks: new(false)}
}

func (f *searchIntegrationFixture) search(t *testing.T, req *service.SearchDatasetsRequest) *service.SearchDatasetsResponse {
	t.Helper()
	result, err := f.service.SearchDatasets(f.ctx, req, f.owner)
	if err != nil {
		t.Fatalf("SearchDatasets: %v", err)
	}
	return result
}

func mustHaveSearchIntegrationChunks(t *testing.T, result *service.SearchDatasetsResponse, want ...string) {
	t.Helper()
	got := make([]string, 0, len(result.Chunks))
	for _, chunk := range result.Chunks {
		id, _ := chunk["id"].(string)
		got = append(got, id)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) || result.Total != int64(len(want)) {
		t.Fatalf("chunks=%v total=%d, want %v", got, result.Total, want)
	}
}

func searchIntegrationScore(t *testing.T, result *service.SearchDatasetsResponse, id string) float64 {
	t.Helper()
	for _, chunk := range result.Chunks {
		if chunk["id"] == id {
			score, ok := chunk["similarity"].(common.PyFloat64)
			if !ok || math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
				t.Fatalf("invalid similarity: %v", chunk)
			}
			return float64(score)
		}
	}
	t.Fatalf("missing chunk %s", id)
	return 0
}
