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
// harness.RuntimeRetriever.
//
// The wrapped service is captured at construction, NOT resolved from the runtime
// singleton on every call: agenttool.SetRetrievalService and
// runtime.SetRetrievalService write the SAME registry, and the server installs
// this adapter into it (after installing the NLP adapter). Resolving the target
// at call time therefore returned this adapter itself, and Search called itself
// until the stack overflowed on every agentic retrieval.
//
// This keeps a single retrieval implementation: the harness and the canvas both
// search through the same NLPRetrievalAdapter.
type RuntimeAdapter struct {
	svc agenttool.RetrievalService
}

// NewRuntimeAdapter creates the runtime RetrievalService bridge over svc — the
// concrete NLPRetrievalAdapter installed at boot. A nil svc makes Search report
// agenttool.ErrRetrievalServiceMissing, the sentinel the registry itself used.
func NewRuntimeAdapter(svc agenttool.RetrievalService) *RuntimeAdapter {
	return &RuntimeAdapter{svc: svc}
}

// Search forwards a runtime retrieval request to the installed tool service and
// translates the result back to the runtime chunk shape.
func (a *RuntimeAdapter) Search(ctx context.Context, db *gorm.DB, req agentrunt.RetrievalRequest) ([]agentrunt.RetrievalChunk, error) {
	svc := a.svc
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
	// RetrievalRequest is the same runtime-owned type on both sides. Forward it
	// whole so new fields cannot disappear at this bridge, then apply the two
	// Agentic caller policies owned here.
	toolReq := agenttool.RetrievalRequest(req)
	toolReq.AllowDenseFallback = new(false)
	toolReq.RankFeature = rankFeatureOrNil(req.RankFeature)
	chunks, err := svc.Search(ctx, db, toolReq)
	if err != nil {
		return nil, err
	}
	// agenttool.RetrievalChunk is a type ALIAS of agentrunt.RetrievalChunk (single
	// owner: internal/agent/runtime), so the result already is the runtime slice:
	// returning it directly is exact. Hand-writing a field-by-field copy here is
	// how ChunkIndex/PageNum were silently dropped — every field the shared type
	// gains now flows through without this file having to know about it.
	return chunks, nil
}

// rankFeatureOrNil converts a rank feature into the pointer form the tool
// request expects, returning nil when empty so a missing feature is
// distinguished from an empty one (Python passes None for "no rank feature").
//
// Upstream carried the feature as a value map in the runtime request and
// converted it at this boundary; the request now carries the pointer form
// directly, so the same invariant is enforced on the pointer instead.
func rankFeatureOrNil(m *map[string]float64) *map[string]float64 {
	if m == nil || len(*m) == 0 {
		return nil
	}
	return m
}
