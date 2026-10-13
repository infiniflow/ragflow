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
)

// NavEmbedder is the production implementation of nlp.NavEmbedder. It resolves
// the tenant's embedding model on each call and returns float32 vectors (the
// dataset-nav index stores q_<dim>_vec as float). It lives in the service
// package (not nlp) so it can import model_service without an import cycle.
type NavEmbedder struct {
	modelFactory *ModelFactory
	// embdModelName is the composite embedding model name (e.g.
	// "embedding_model@..." ). Empty falls back to resolving the tenant default.
	embdModelName string
}

// NewNavEmbedder builds the production embedder used by NavService.
func NewNavEmbedder(modelFactory *ModelFactory, embdModelName string) *NavEmbedder {
	return &NavEmbedder{modelFactory: modelFactory, embdModelName: embdModelName}
}

// Encode embeds texts as DOCUMENTS for the tenant and returns float32 vectors.
func (e *NavEmbedder) Encode(ctx context.Context, tenantID string, texts []string) ([][]float32, error) {
	return e.encode(ctx, tenantID, texts, false)
}

// EncodeQueries is the query-side counterpart of Encode (Python
// LLMBundle.encode_queries). Providers that embed queries and documents
// asymmetrically — Cohere search_query, Voyage query, Jina retrieval.query,
// NVIDIA query — only apply their query encoding when this method is used, so
// every caller embedding a SEARCH QUERY must prefer it over Encode.
func (e *NavEmbedder) EncodeQueries(ctx context.Context, tenantID string, texts []string) ([][]float32, error) {
	return e.encode(ctx, tenantID, texts, true)
}

func (e *NavEmbedder) encode(ctx context.Context, tenantID string, texts []string, query bool) ([][]float32, error) {
	if e.modelFactory == nil {
		return nil, fmt.Errorf("datasetnav: model factory not initialized")
	}
	name := e.embdModelName
	var model *modelModule.EmbeddingModel
	access := ModelAccess{TenantID: tenantID}
	var err error
	if name == "" {
		model, err = e.modelFactory.NewDefaultEmbeddingModel(ctx, access)
	} else {
		model, err = e.modelFactory.NewEmbeddingModel(ctx, access, name)
	}
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
	request := modelModule.EmbedRequest{Texts: nonEmpty, Query: query}
	embeds, err := model.Embed(ctx, request, nil, nil)
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
