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

package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"ragflow/internal/engine/types"
)

// switchProbeEngine answers the two calls the dimension check makes: a one-hit
// search and the chunk read that follows it.
type switchProbeEngine struct {
	fakeChatDocEngine
	chunk      map[string]interface{}
	searchErr  error
	searchHits int
}

func (e *switchProbeEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	e.searchHits++
	if e.searchErr != nil {
		return nil, e.searchErr
	}
	if e.chunk == nil {
		return &types.SearchResult{}, nil
	}
	id, _ := e.chunk["id"].(string)
	return &types.SearchResult{
		Chunks: []map[string]interface{}{{"id": id, "chunk_id": id}},
		Total:  1,
	}, nil
}

func (e *switchProbeEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	if e.chunk == nil {
		return nil, nil
	}
	return e.chunk, nil
}

// chunkWithVector builds a stored chunk carrying a vector of the given size.
func chunkWithVector(dim int) map[string]interface{} {
	return map[string]interface{}{
		"id":                         "chunk-1",
		"kb_id":                      "kb-1",
		fmt.Sprintf("q_%d_vec", dim): make([]float64, dim),
		"content_with_weight":        "hello",
	}
}

// stubDimensionProbe answers the candidate model's dimension without a provider.
func stubDimensionProbe(t *testing.T, dim int, err error) {
	t.Helper()
	previous := embeddingDimensionProbe
	embeddingDimensionProbe = func(context.Context, string, string) (int, error) {
		return dim, err
	}
	t.Cleanup(func() { embeddingDimensionProbe = previous })
}

func switchTestKB(chunkNum int64) *entity.Knowledgebase {
	return &entity.Knowledgebase{
		ID:       "kb-1",
		TenantID: "tenant-1",
		EmbdID:   "bge-384@Builtin",
		ChunkNum: chunkNum,
	}
}

func TestValidateEmbeddingSwitchSkipsDatasetWithoutChunks(t *testing.T) {
	engine := &switchProbeEngine{chunk: chunkWithVector(384)}
	stubDimensionProbe(t, 1024, nil)

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(0), "big-1024@Builtin")
	if err != nil {
		t.Fatalf("expected the switch to be allowed, got %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected CodeSuccess, got %v", code)
	}
	if engine.searchHits != 0 {
		t.Fatalf("expected no engine round trip for a chunkless dataset, got %d", engine.searchHits)
	}
}

func TestValidateEmbeddingSwitchSkipsUnchangedModel(t *testing.T) {
	engine := &switchProbeEngine{chunk: chunkWithVector(384)}
	stubDimensionProbe(t, 1024, nil)

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(5), "bge-384@Builtin")
	if err != nil {
		t.Fatalf("expected the same model to be allowed, got %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected CodeSuccess, got %v", code)
	}
	if engine.searchHits != 0 {
		t.Fatalf("expected no engine round trip when the model does not change, got %d", engine.searchHits)
	}
}

func TestValidateEmbeddingSwitchRejectsDimensionChange(t *testing.T) {
	engine := &switchProbeEngine{chunk: chunkWithVector(384)}
	stubDimensionProbe(t, 1024, nil)

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(5), "big-1024@Builtin")
	if err == nil {
		t.Fatal("expected the dimension change to be rejected")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %v", code)
	}
	for _, want := range []string{"384", "1024", "q_384_vec"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected the error to mention %q, got %q", want, err.Error())
		}
	}
}

func TestValidateEmbeddingSwitchAcceptsSameDimension(t *testing.T) {
	engine := &switchProbeEngine{chunk: chunkWithVector(384)}
	stubDimensionProbe(t, 384, nil)

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(5), "other-384@Builtin")
	if err != nil {
		t.Fatalf("expected a same-dimension switch to be allowed, got %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected CodeSuccess, got %v", code)
	}
}

func TestValidateEmbeddingSwitchAllowsWhenNoStoredVector(t *testing.T) {
	// The dataset counts a chunk but the sampled row holds no vector (embedding
	// disabled): there is no stored dimension to strand.
	engine := &switchProbeEngine{chunk: map[string]interface{}{"id": "chunk-1", "content_with_weight": "hello"}}
	stubDimensionProbe(t, 1024, nil)

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(5), "big-1024@Builtin")
	if err != nil {
		t.Fatalf("expected the switch to be allowed, got %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected CodeSuccess, got %v", code)
	}
}

func TestValidateEmbeddingSwitchFailsClosedWhenSampleFails(t *testing.T) {
	engine := &switchProbeEngine{searchErr: errors.New("engine unavailable")}
	stubDimensionProbe(t, 384, nil)

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(5), "other-384@Builtin")
	if err == nil {
		t.Fatal("expected an unreadable sample to reject the switch")
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
}

func TestValidateEmbeddingSwitchRejectsUnresolvableCandidate(t *testing.T) {
	engine := &switchProbeEngine{chunk: chunkWithVector(384)}
	stubDimensionProbe(t, 0, errors.New("model not found"))

	svc := &DatasetService{docEngine: engine}
	code, err := svc.validateEmbeddingSwitch(context.Background(), switchTestKB(5), "missing@Builtin")
	if err == nil {
		t.Fatal("expected an unresolvable candidate model to be rejected")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %v", code)
	}
}

// TestUpdateDatasetRejectsIncompatibleEmbeddingSwitch covers the API path: a
// request that skips the console still cannot move a dataset holding vectors to
// a model of another dimension, and nothing is persisted.
func TestUpdateDatasetRejectsIncompatibleEmbeddingSwitch(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("chunk_num", 5).Error; err != nil {
		t.Fatalf("seed chunk_num: %v", err)
	}
	stubDimensionProbe(t, 1024, nil)

	svc := testDatasetUpdateService(t)
	svc.docEngine = &switchProbeEngine{chunk: chunkWithVector(384)}

	embeddingModel := "big-1024@Builtin"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	_, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &embeddingModel})
	if err == nil {
		t.Fatal("expected the update to be rejected")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %v", code)
	}

	var persisted entity.Knowledgebase
	if err := dao.DB.Where("id = ?", "kb-1").First(&persisted).Error; err != nil {
		t.Fatalf("load dataset: %v", err)
	}
	if persisted.EmbdID != "BAAI/bge-large-zh-v1.5@Builtin" {
		t.Fatalf("expected embd_id to stay unchanged, got %q", persisted.EmbdID)
	}
}
