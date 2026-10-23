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

	"ragflow/internal/agentic_rag"
)

// OntologyAdapter exposes the ABox retrieval service to the agent loop.
//
// The agent package must not import service (service imports it), so the
// knowledge layer crosses that boundary through the interface agentic_rag
// declares — the same shape the compiled-graph walk uses. The two entry points
// share one implementation, which is what keeps the REST preview and the agent's
// prefetch narrowing by the same structure under the same model version.
type OntologyAdapter struct {
	svc *OntologyABoxRetrievalService
}

// NewOntologyAdapter wraps an ABox retrieval service for install into the agent
// runtime.
func NewOntologyAdapter(svc *OntologyABoxRetrievalService) *OntologyAdapter {
	return &OntologyAdapter{svc: svc}
}

// Retrieve runs one knowledge-layer lookup and returns the digest.
//
// The result's rows are dropped here on purpose: the agent consumes the digest,
// and carrying the raw rows too would invite one of the two shapes to be used
// as the answer, which is exactly the confusion the digest exists to prevent.
func (a *OntologyAdapter) Retrieve(ctx context.Context, req agentic_rag.OntologyRetrieveRequest) (*agentic_rag.OntologyDigest, error) {
	if a == nil || a.svc == nil {
		return nil, agentic_rag.ErrOntologyServiceMissing
	}
	result, err := a.svc.Retrieve(ctx, &OntologyABoxRetrievalRequest{
		UserID:              req.UserID,
		DatasetIDs:          req.KnowledgeDatasetIDs,
		DocumentIDs:         req.DocumentIDs,
		Question:            req.Question,
		OntologyTemplateID:  req.ModelID,
		OntologyVersion:     req.ModelVersion,
		EntityTypes:         req.EntityTypes,
		Properties:          req.Properties,
		FromTypes:           req.FromTypes,
		ToTypes:             req.ToTypes,
		TopK:                req.TopK,
		SimilarityThreshold: req.SimilarityThreshold,
		DocumentScope:       req.DocumentScope,
		SkipUnreviewed:      req.SkipUnreviewed,
		HypergraphRecall:    req.HypergraphRecall,
	})
	if err != nil {
		return nil, err
	}
	return result.Digest, nil
}

// compile-time proof that the adapter satisfies the agent-side contract.
var _ agentic_rag.OntologyService = (*OntologyAdapter)(nil)
