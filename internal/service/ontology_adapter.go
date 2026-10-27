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
	// traversal is built from the same wiring as svc: both read the same snapshots
	// and the same rows, and a walk that resolved its own model separately could
	// prune by a different version than the lookup reported.
	traversal *OntologyTraversalService
}

// NewOntologyAdapter wraps an ABox retrieval service for install into the agent
// runtime.
func NewOntologyAdapter(svc *OntologyABoxRetrievalService) *OntologyAdapter {
	return &OntologyAdapter{svc: svc, traversal: NewOntologyTraversalService()}
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

// Traverse walks the compiled assertions outward from the named entities.
//
// The agent gets paths rather than assertions because that is what a multi-hop
// answer needs: the middle entity of a chain usually appears in no sentence the
// reader wrote, so no ranking can reach it — the chain has to be followed, and
// the result has to say which hops succeeded and which were refused.
func (a *OntologyAdapter) Traverse(ctx context.Context, req agentic_rag.OntologyTraverseRequest) (*agentic_rag.OntologyTraversal, error) {
	if a == nil || a.traversal == nil {
		return nil, agentic_rag.ErrOntologyServiceMissing
	}
	result, err := a.traversal.Traverse(ctx, &OntologyTraversalRequest{
		UserID:             req.UserID,
		DatasetIDs:         req.KnowledgeDatasetIDs,
		OntologyTemplateID: req.ModelID,
		OntologyVersion:    req.ModelVersion,
		Starts:             req.Starts,
		Hops:               req.Hops,
		AllowProperties:    req.AllowProperties,
		AllowTypes:         req.AllowTypes,
		MaxPaths:           req.MaxPaths,
		DocumentScope:      req.DocumentScope,
	})
	if err != nil {
		return nil, err
	}
	out := &agentic_rag.OntologyTraversal{
		Paths: make([]agentic_rag.OntologyTraversePath, 0, len(result.Paths)),
		Hops:  make([]agentic_rag.OntologyTraverseHop, 0, len(result.Hops)),
		Model: agentic_rag.OntologyModelRef{
			ID:            result.Model.ID,
			Name:          result.Model.Name,
			Version:       result.Model.Version,
			PublishStatus: result.Model.PublishStatus,
		},
		DegradeCode:   result.DegradeCode,
		DegradeReason: result.DegradeReason,
		Truncated:     result.Truncated,
	}
	for _, path := range result.Paths {
		converted := agentic_rag.OntologyTraversePath{
			Start: path.Start,
			Nodes: path.Nodes,
			Steps: make([]agentic_rag.OntologyTraverseStep, 0, len(path.Steps)),
		}
		for _, step := range path.Steps {
			converted.Steps = append(converted.Steps, agentic_rag.OntologyTraverseStep{
				From:           step.From,
				Property:       step.Property,
				To:             step.To,
				FromType:       step.FromType,
				ToType:         step.ToType,
				Direction:      step.Direction,
				SourceChunkIDs: step.SourceChunkIDs,
			})
		}
		out.Paths = append(out.Paths, converted)
	}
	for _, hop := range result.Hops {
		out.Hops = append(out.Hops, agentic_rag.OntologyTraverseHop{
			Hop:                   hop.Hop,
			Frontier:              hop.Frontier,
			Rows:                  hop.Rows,
			Matched:               hop.Matched,
			RejectedByDeclaration: hop.RejectedByDeclaration,
			RejectedByAllowList:   hop.RejectedByAllowList,
			Truncated:             hop.Truncated,
		})
	}
	return out, nil
}

// compile-time proof that the adapter satisfies the agent-side contract.
var _ agentic_rag.OntologyService = (*OntologyAdapter)(nil)
