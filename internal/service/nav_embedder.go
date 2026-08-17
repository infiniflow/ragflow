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

package service

import (
	"context"
	"fmt"
	modelModule "ragflow/internal/entity/models"
	"strings"

	"ragflow/internal/entity"
)

// NavEmbedder is the production implementation of nlp.NavEmbedder. It resolves
// the tenant's embedding model on each call and returns float32 vectors (the
// dataset-nav index stores q_<dim>_vec as float). It lives in the service
// package (not nlp) so it can import model_service without an import cycle.
type NavEmbedder struct {
	modelSvc *ModelProviderService
	// embdModelName is the composite embedding model name (e.g.
	// "embedding_model@..." ). Empty falls back to resolving the tenant default.
	embdModelName string
}

// NewNavEmbedder builds the production embedder used by NavService.
func NewNavEmbedder(modelSvc *ModelProviderService, embdModelName string) *NavEmbedder {
	return &NavEmbedder{modelSvc: modelSvc, embdModelName: embdModelName}
}

// Encode embeds texts for the tenant and returns float32 vectors.
func (e *NavEmbedder) Encode(ctx context.Context, tenantID string, texts []string) ([][]float32, error) {
	if e.modelSvc == nil {
		return nil, fmt.Errorf("datasetnav: embedding model service not initialized")
	}
	name := e.embdModelName
	if name == "" {
		// Resolve the tenant's default embedding model composite reference
		// ("<model>@<instance>@<provider>") — NOT the tenant id. Passing the
		// tenant id as the model ref makes ResolveModelConfig parse it as a
		// "model@provider" key, which fails with "provider name missing in model
		// name: <tenant_id>". Mirrors knowledge_compiler_wiring.go's chat-model
		// default resolution.
		ref, err := e.modelSvc.GetTenantDefaultModelRef(ctx, tenantID, entity.ModelTypeEmbedding)
		if err != nil {
			return nil, fmt.Errorf("datasetnav: resolve embedding model for tenant %s: %w", tenantID, err)
		}
		name = ref
	}
	model, err := e.modelSvc.GetEmbeddingModel(ctx, tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("datasetnav: resolve embedding model for tenant %s: %w", tenantID, err)
	}
	nonEmpty := make([]string, 0, len(texts))
	for _, t := range texts {
		if strings.TrimSpace(t) != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	if len(nonEmpty) == 0 {
		return nil, nil
	}
	embeds, err := model.ModelDriver.Embed(ctx, model.ModelName, modelModule.EmbedRequest{Texts: nonEmpty}, model.APIConfig, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([][]float32, 0, len(embeds))
	for _, e := range embeds {
		out = append(out, toF32(e.Embedding))
	}
	return out, nil
}

func toF32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}
