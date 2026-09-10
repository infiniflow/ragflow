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

package task

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity/models"
	componentpkg "ragflow/internal/ingestion/component"
	"ragflow/internal/service"
)

type embedder struct {
	model *models.EmbeddingModel
}

func (e *embedder) MaxTokens() int {
	if e == nil || e.model == nil {
		return 0
	}
	return e.model.MaxTokens
}

func (e *embedder) BatchSize() int {
	if e == nil || e.model == nil {
		return models.DefaultEmbeddingBatchSize
	}
	return e.model.ResolveBatchSize()
}

func (e *embedder) Encode(ctx context.Context, texts []string) ([]componentpkg.EmbeddingResult, error) {
	if e.model.ModelDriver == nil {
		return nil, fmt.Errorf("embedder: embedding model driver is nil for model %v", e.model.ModelName)
	}
	config := &models.EmbeddingConfig{Dimension: 0}
	embeds, err := e.model.ModelDriver.Embed(ctx, e.model.ModelName, models.EmbedRequest{Texts: texts}, e.model.APIConfig, config, nil)
	if err != nil {
		return nil, err
	}
	vecs := make([]componentpkg.EmbeddingResult, len(embeds))
	for i, v := range embeds {
		vecs[i] = componentpkg.EmbeddingResult{Vector: v.Embedding, TokenCount: v.TokenCount}
	}
	return vecs, nil
}

// newEmbedderResolver builds the production embedder resolver used by the
// Tokenizer component. It always resolves the embedder from the dataset's
// configured embd_id (looked up by kbID) and returns that embd_id alongside the
// embedder, so the Tokenizer can key its per-chunk cache on the dataset-bound
// model. If the dataset has no embd_id configured, it returns a nil embedder and
// an empty embd_id (no embedding). Kept as a constructor over injectable deps so
// the resolution logic stays unit-testable without a live model provider / DB.
func newEmbedderResolver(
	getKBEmbdID func(ctx context.Context, kbID string) (string, error),
	getEmbeddingModel func(ctx context.Context, tenantID, embdID string) (*models.EmbeddingModel, error),
) componentpkg.EmbedderResolver {
	// The resolver derives the embedding model exclusively from the
	// knowledgebase's configured embd_id — never from any DSL-supplied
	// identifier. It returns embdID alongside the embedder so the tokenizer can
	// key its per-chunk cache on the dataset-bound model: when a KB's embedding
	// model changes, embdID changes, the cache key changes, and no stale vector
	// is ever served.
	return func(ctx context.Context, tenantID, kbID string) (componentpkg.Embedder, string, error) {
		embdID, err := getKBEmbdID(ctx, kbID)
		if err != nil {
			return nil, "", fmt.Errorf("embedder: resolve kb embd_id for kb_id=%s: %w", kbID, err)
		}
		embdID = strings.TrimSpace(embdID)
		if embdID == "" {
			return nil, "", nil
		}
		model, err := getEmbeddingModel(ctx, tenantID, embdID)
		if err != nil {
			return nil, "", err
		}
		if model == nil {
			return nil, "", fmt.Errorf("embedder: resolved embedding model is nil for embd_id=%s", embdID)
		}
		return &embedder{model: model}, embdID, nil
	}
}

// init wires the production embedder resolver into the component package. The
// component package must not import internal/service (dependency direction),
// so the concrete resolver is injected here - the task package is the
// composition root for ingestion runs.
func init() {
	componentpkg.DefaultEmbedderResolver = newEmbedderResolver(
		func(ctx context.Context, kbID string) (string, error) {
			kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, dao.DB, kbID)
			if err != nil {
				return "", err
			}
			if kb == nil {
				return "", nil
			}
			return kb.EmbdID, nil
		},
		service.NewModelProviderService().GetEmbeddingModel,
	)
}
