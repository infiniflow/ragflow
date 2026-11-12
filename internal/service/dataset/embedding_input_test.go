package dataset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"ragflow/internal/common"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type embeddingFixtureEngine struct {
	fakeChatDocEngine
}

func (embeddingFixtureEngine) Search(context.Context, *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	return &enginetypes.SearchResult{Total: 1, Chunks: []map[string]any{{"id": "chunk-fixture"}}}, nil
}

func (embeddingFixtureEngine) GetChunk(context.Context, string, string, []string) (any, error) {
	return map[string]any{
		"doc_id": "doc-fixture", "docnm_kwd": "原始标题",
		"content_with_weight": "<table><tr><td>原始正文</td></tr></table>",
		"title_tks":           "原 始 标 题", "content_ltks": "原 始 正 文",
		"q_2_vec": []float64{1, 0},
	}, nil
}

func TestVerifyEmbeddingCompatibilityInput(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	for _, tc := range []struct {
		name   string
		vector []float64
		want   common.ErrorCode
	}{
		{"compatible", []float64{1, 0}, common.CodeSuccess},
		{"dimension", []float64{1, 0, 0}, common.CodeDataError},
		{"low similarity", []float64{0, 1}, common.CodeNotEffective},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var inputs []string
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input []string `json:"input"`
				}
				if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Input) != 1 {
					http.Error(w, "unexpected synthetic embedding request", http.StatusBadRequest)
					return
				}
				mu.Lock()
				inputs = append(inputs, body.Input...)
				mu.Unlock()
				vector := tc.vector
				if body.Input[0] != "原始标题" && body.Input[0] != "原始正文" {
					vector = []float64{0, 1}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data":  []map[string]any{{"index": 0, "embedding": vector}},
					"usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1},
				})
			}))
			defer endpoint.Close()

			db := setupDatasetUpdateTestDB(t)
			pushServiceDB(t, db)
			insertDatasetUpdateKB(t, "kb-fixture", "tenant-fixture", "Indexed")
			insertDatasetUpdateModelProvider(t, "provider-fixture", "tenant-fixture", "OpenAI-API-Compatible")
			insertDatasetUpdateModelInstance(t, "instance-fixture", "provider-fixture", "fixture")
			extra, err := json.Marshal(map[string]string{"base_url": endpoint.URL + "/v1"})
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-fixture").Update("extra", string(extra)).Error; err != nil {
				t.Fatal(err)
			}
			const modelID = "aabbccdd11223344aabbccdd11223346"
			insertDatasetUpdateTenantModel(t, modelID, "provider-fixture", "instance-fixture", "fixture-embedding", int(entity.ModelTypeEmbedding))

			svc := testDatasetUpdateService(t)
			svc.docEngine = embeddingFixtureEngine{}
			ctx := withDatasetTestContext(t.Context(), "tenant-fixture", "tenant-fixture")
			_, code, err := svc.CheckEmbedding(ctx, "kb-fixture", &service.CheckEmbeddingRequest{EmbeddingID: modelID})
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(inputs, []string{"原始标题", "原始正文"}) {
				t.Fatalf("provider inputs = %q, expected original title and cleaned content; code=%v err=%v", inputs, code, err)
			}
			if code != tc.want || (err == nil) != (tc.want == common.CodeSuccess) {
				t.Fatalf("check: code=%v err=%v, want code=%v", code, err, tc.want)
			}
		})
	}
}
