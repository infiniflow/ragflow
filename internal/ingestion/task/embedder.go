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

func (e *embedder) Encode(texts []string) ([]componentpkg.EmbeddingResult, error) {
	if e.model.ModelDriver == nil {
		return nil, fmt.Errorf("embedder: embedding model driver is nil for model %v", e.model.ModelName)
	}
	config := &models.EmbeddingConfig{Dimension: 0}
	embeds, err := e.model.ModelDriver.Embed(e.model.ModelName, texts, e.model.APIConfig, config, nil)
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
// configured embd_id (looked up by kbID). If the dataset has no embd_id
// configured, it returns nil (no embedding). Kept as a constructor over
// injectable deps so the resolution logic stays unit-testable without a live
// model provider / DB.
func newEmbedderResolver(
	getKBEmbdID func(kbID string) (string, error),
	getEmbeddingModel func(tenantID, embdID string) (*models.EmbeddingModel, error),
) componentpkg.EmbedderResolver {
	return func(tenantID, kbID, _ string) (componentpkg.Embedder, error) {
		embdID, err := getKBEmbdID(kbID)
		if err != nil {
			return nil, fmt.Errorf("embedder: resolve kb embd_id for kb_id=%s: %w", kbID, err)
		}
		embdID = strings.TrimSpace(embdID)
		if embdID == "" {
			return nil, nil
		}
		model, err := getEmbeddingModel(tenantID, embdID)
		if err != nil {
			return nil, err
		}
		if model == nil {
			return nil, fmt.Errorf("embedder: resolved embedding model is nil for embd_id=%s", embdID)
		}
		return &embedder{model: model}, nil
	}
}

// init wires the production embedder resolver into the component package. The
// component package must not import internal/service (dependency direction),
// so the concrete resolver is injected here - the task package is the
// composition root for ingestion runs.
func init() {
	componentpkg.DefaultEmbedderResolver = newEmbedderResolver(
		func(kbID string) (string, error) {
			kb, err := dao.NewKnowledgebaseDAO().GetByID(kbID)
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
