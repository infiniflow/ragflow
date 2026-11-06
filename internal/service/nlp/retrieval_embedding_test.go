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
	"strings"
	"testing"

	"ragflow/internal/common"
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
