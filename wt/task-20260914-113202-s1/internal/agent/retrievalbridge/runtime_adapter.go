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

package retrievalbridge

import (
	"context"

	agentrunt "ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/dao"

	"gorm.io/gorm"
)

// RuntimeAdapter exposes the agent tool's NLPRetrievalAdapter (the production
// hybrid-retrieval backend installed by the server) through the runtime
// RetrievalService singleton that the agentic-RAG harness reads via
// harness.RuntimeRetriever. The wrapped tool service is read on every call (not
// captured at construction) because the server installs it during boot.
//
// This keeps a single retrieval implementation: the harness and the canvas both
// search through the same NLPRetrievalAdapter.
type RuntimeAdapter struct{}

// NewRuntimeAdapter creates the runtime RetrievalService bridge.
func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{}
}

// Search forwards a runtime retrieval request to the installed tool service and
// translates the result back to the runtime chunk shape.
func (a *RuntimeAdapter) Search(ctx context.Context, db *gorm.DB, req agentrunt.RetrievalRequest) ([]agentrunt.RetrievalChunk, error) {
	svc := agenttool.GetRetrievalService()
	if svc == nil {
		return nil, agenttool.ErrRetrievalServiceMissing
	}
	// The harness calls the runtime service with a nil *gorm.DB (see
	// harness.RuntimeRetriever.Retrieve), while the underlying NLPRetrievalAdapter
	// needs a real connection to resolve knowledge bases. Fall back to the
	// process-wide DB handle used by the rest of the chat pipeline.
	if db == nil {
		db = dao.DB
	}
	toolReq := agenttool.RetrievalRequest{
		Query:                 req.Query,
		DatasetIDs:            req.DatasetIDs,
		MemoryIDs:             req.MemoryIDs,
		TopN:                  req.TopN,
		RerankCandidatesCount: req.RerankCandidatesCount,
		TopK:                  req.TopK,
		// VectorSimilarityWeight is the VECTOR weight (Python
		// vector_similarity_weight) and is forwarded to the nlp layer
		// UN-inverted. KeywordsSimilarityWeight (canvas semantics, keyword
		// weight) stays on its own inversion path in nlpRequestFromRetrieval —
		// conflating the two turned the agentic hybrid leg vector-dominant and
		// the BM25 legs into pure-vector searches.
		VectorSimilarityWeight:   req.VectorSimilarityWeight,
		DisableVectorLeg:         req.DisableVectorLeg,
		KeywordsSimilarityWeight: req.KeywordsSimilarityWeight,
		UseKG:                    req.UseKG,
		SimilarityThreshold:      req.SimilarityThreshold,
		CrossLanguages:           req.CrossLanguages,
		RetrievalFrom:            req.RetrievalFrom,
		DocScope:                 req.DocScope,
		TenantID:                 req.TenantID,
		// rank_feature (Python retrieve: rank_feature=label_question(question,
		// self.kbs)) — forwarded from the RAGTools-computed value so the agentic
		// tool stays authoritative; the adapter falls back to its own resolution.
		RankFeature: rankFeatureOrNil(req.RankFeature),
		// OnlyOriginalText restricts retrieval to ordinary document chunks (no
		// compile_kwd) — Python hybrid_search's must_not={"exists":"compile_kwd"}.
		ExcludeCompiled: req.OnlyOriginalText,
	}
	chunks, err := svc.Search(ctx, db, toolReq)
	if err != nil {
		return nil, err
	}
	out := make([]agentrunt.RetrievalChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, agentrunt.RetrievalChunk{
			ID:               c.ID,
			Content:          c.Content,
			DocumentID:       c.DocumentID,
			DocumentName:     c.DocumentName,
			DatasetID:        c.DatasetID,
			ImageID:          c.ImageID,
			URL:              c.URL,
			Positions:        c.Positions,
			Score:            c.Score,
			TermSimilarity:   c.TermSimilarity,
			VectorSimilarity: c.VectorSimilarity,
			MomID:            c.MomID,
		})
	}
	return out, nil
}

// rankFeatureOrNil converts a map-valued rank feature into the pointer form the
// tool request expects, returning nil when empty so a missing feature is
// distinguished from an empty one (Python passes None for "no rank feature").
func rankFeatureOrNil(m map[string]float64) *map[string]float64 {
	if len(m) == 0 {
		return nil
	}
	return &m
}
