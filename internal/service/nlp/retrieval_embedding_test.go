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

package nlp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity/models"
)

type emptyEmbeddingResponseDriver struct{ models.ModelDriver }

func (*emptyEmbeddingResponseDriver) Embed(context.Context, *string, models.EmbedRequest, *models.APIConfig, *models.EmbeddingConfig, *common.ModelUsage) ([]models.EmbeddingData, error) {
	return []models.EmbeddingData{}, nil
}

func TestGetVectorRejectsEmptyEmbeddingResponse(t *testing.T) {
	t.Setenv("TOKENIZER_EMBEDDING_TOKENIZER", "")
	t.Setenv("TOKENIZER_EMBEDDING_MAX_TOKENS", "8192")
	key := t.Name()
	model := models.NewEmbeddingModel(&emptyEmbeddingResponseDriver{}, nil, &models.APIConfig{ApiKey: &key}, 8192)
	service := &RetrievalService{}
	got, err := service.GetVector(t.Context(), "query", model, 10, 10, 0.1)
	if err == nil || !strings.Contains(err.Error(), "unexpected embedding count: got 0, want 1") {
		t.Fatalf("error = %v, want an empty embedding response error", err)
	}
	if got != nil {
		t.Fatalf("invalid embedding response returned a dense expression: %#v", got)
	}
}

type guardedEmbeddingDriver struct {
	models.ModelDriver
	guarded *bool
	calls   int
}

func (d *guardedEmbeddingDriver) Embed(context.Context, *string, models.EmbedRequest, *models.APIConfig, *models.EmbeddingConfig, *common.ModelUsage) ([]models.EmbeddingData, error) {
	if *d.guarded {
		return nil, fmt.Errorf("embedding ran under search guard")
	}
	d.calls++
	return []models.EmbeddingData{{Embedding: []float64{0.1, 0.2}}}, nil
}

type guardedRetrievalEngine struct {
	retrievalCountEngine
	guarded *bool
}

func (e *guardedRetrievalEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !*e.guarded {
		return nil, fmt.Errorf("index read ran outside guard")
	}
	return e.retrievalCountEngine.Search(ctx, req)
}
func TestRetrievalEmbedsBeforeGuardedIndexRead(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}
	t.Setenv("TOKENIZER_EMBEDDING_TOKENIZER", "")
	t.Setenv("TOKENIZER_EMBEDDING_MAX_TOKENS", "8192")
	guarded := false
	driver := &guardedEmbeddingDriver{guarded: &guarded}
	key := "key"
	model := models.NewEmbeddingModel(driver, nil, &models.APIConfig{ApiKey: &key}, 8192)
	engine := &guardedRetrievalEngine{guarded: &guarded}
	guardCalls := 0
	req := &RetrievalRequest{Question: "query", Page: 1, PageSize: 1, EmbeddingModel: model, SearchGuard: func(ctx context.Context, read func(context.Context) error) error {
		guardCalls++
		guarded = true
		defer func() { guarded = false }()
		return read(ctx)
	}}
	_, err := NewRetrievalService(engine, dao.NewDocumentDAO()).Retrieval(t.Context(), req)
	if err != nil || driver.calls != 1 || guardCalls != 1 {
		t.Fatalf("retrieval: err=%v embeddings=%d guards=%d", err, driver.calls, guardCalls)
	}
}

func TestRetrievalRejectsChangedBindingBeforeIndexRead(t *testing.T) {
	t.Setenv("TOKENIZER_EMBEDDING_TOKENIZER", "")
	t.Setenv("TOKENIZER_EMBEDDING_MAX_TOKENS", "8192")
	guarded := false
	driver := &guardedEmbeddingDriver{guarded: &guarded}
	key := "key"
	model := models.NewEmbeddingModel(driver, nil, &models.APIConfig{ApiKey: &key}, 8192)
	engine := &retrievalCountEngine{}
	req := &RetrievalRequest{Question: "query", Page: 1, PageSize: 1, EmbeddingModel: model, SearchGuard: func(context.Context, func(context.Context) error) error {
		return fmt.Errorf("dataset embedding model changed")
	}}
	_, err := NewRetrievalService(engine, dao.NewDocumentDAO()).Retrieval(t.Context(), req)
	if err == nil || !strings.Contains(err.Error(), "embedding model changed") || driver.calls != 1 || len(engine.searchLimits) != 0 {
		t.Fatalf("err=%v embeddings=%d searches=%d", err, driver.calls, len(engine.searchLimits))
	}
}

func TestSecondPassScoresRejectChangedBinding(t *testing.T) {
	weight := 0.5
	features := map[string]float64{}
	req := &RetrievalRequest{VectorSimilarityWeight: &weight, RankFeature: &features, SearchGuard: func(context.Context, func(context.Context) error) error {
		return fmt.Errorf("dataset embedding model changed")
	}}
	result := &RetrievalSearchResult{Total: 1, IDs: []string{"chunk"}, Chunks: []map[string]interface{}{{"id": "chunk"}}, QueryVector: []float64{0.1}}
	_, _, _, err := NewRetrievalService(&retrievalCountEngine{}, dao.NewDocumentDAO()).scoreSearchResult(t.Context(), req, result)
	if err == nil || !strings.Contains(err.Error(), "embedding model changed") {
		t.Fatalf("error = %v", err)
	}
}
