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
	"ragflow/internal/entity"
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
	config := &models.EmbeddingConfig{Dimension: 0}
	embeds, err := e.model.ModelDriver.Embed(e.model.ModelName, texts, e.model.APIConfig, config)
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
// Tokenizer component. It honors an explicit embedding-model id (from the
// Tokenizer's setups) and falls back to the dataset's configured embd_id when
// none is given. Kept as a constructor over injectable deps so the resolution
// logic stays unit-testable without a live model provider / DB.
func newEmbedderResolver(
	getEmbeddingModel func(tenantID, embdID string) (*models.EmbeddingModel, error),
	getKnowledgebaseByID func(kbID string) (*entity.Knowledgebase, error),
) componentpkg.EmbedderResolver {
	return func(tenantID, kbID, embeddingModel string) (componentpkg.Embedder, error) {
		embdID := strings.TrimSpace(embeddingModel)
		if embdID == "" {
			if strings.TrimSpace(kbID) == "" {
				return nil, fmt.Errorf("embedding requested but neither embedding_model nor kb_id provided")
			}
			kb, err := getKnowledgebaseByID(kbID)
			if err != nil {
				return nil, err
			}
			if kb == nil || strings.TrimSpace(kb.EmbdID) == "" {
				return nil, fmt.Errorf("embedding requested but dataset has no embd_id configured")
			}
			embdID = kb.EmbdID
		}
		model, err := getEmbeddingModel(tenantID, embdID)
		if err != nil {
			return nil, err
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
		service.NewModelProviderService().GetEmbeddingModel,
		dao.NewKnowledgebaseDAO().GetByID,
	)
}
