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

// Package retrievalbridge connects the agent retrieval interfaces to the
// parent service package without making agent/tool import internal/service.
package retrievalbridge

import (
	"context"
	"fmt"

	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

// Enhancer delegates retrieval query and result enhancement to the existing
// service-layer implementations used by chunk and chat retrieval.
type Enhancer struct {
	docEngine   engine.DocEngine
	metadataSvc *service.MetadataService
}

// NewEnhancer creates an Enhancer for the configured document engine.
func NewEnhancer(docEngine engine.DocEngine, metadataSvc *service.MetadataService) *Enhancer {
	if metadataSvc == nil {
		metadataSvc = service.NewMetadataService()
	}
	return &Enhancer{docEngine: docEngine, metadataSvc: metadataSvc}
}

// CrossLanguages translates the query into the configured languages using the
// tenant's default chat model.
func (e *Enhancer) CrossLanguages(
	ctx context.Context,
	tenantID, query string,
	languages []string,
) (string, error) {
	return service.CrossLanguages(ctx, tenantID, "", query, languages)
}

// FilterDocuments applies auto, semi-auto, or manual metadata filtering and
// constrains the result by any document scope supplied by upstream tools.
func (e *Enhancer) FilterDocuments(
	ctx context.Context,
	filter map[string]any,
	query string,
	chatModel *modelModule.ChatModel,
	baseDocIDs []string,
	kbIDs []string,
) ([]string, error) {
	if e == nil || e.metadataSvc == nil {
		return nil, fmt.Errorf("metadata service is not configured")
	}
	metadata, err := e.metadataSvc.GetFlattedMetaByKBs(ctx, kbIDs)
	if err != nil {
		return nil, err
	}
	docIDs, noMatches := service.ApplyMetaDataFilter(
		ctx,
		filter,
		metadata,
		query,
		chatModel,
		baseDocIDs,
		kbIDs,
	)
	if noMatches {
		return []string{service.NoMatchDocIDSentinel}, nil
	}
	return docIDs, nil
}

// LabelQuestion returns tag-based rank features for NLP reranking.
func (e *Enhancer) LabelQuestion(
	ctx context.Context,
	question string,
	kbs []*entity.Knowledgebase,
) map[string]float64 {
	if e == nil || e.metadataSvc == nil {
		return nil
	}
	return e.metadataSvc.LabelQuestion(ctx, question, kbs)
}

// EnhanceTOC adds or boosts chunks selected through the document table of
// contents. The service enhancer mutates the supplied kbinfos map.
func (e *Enhancer) EnhanceTOC(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	tenantIDs, kbIDs []string,
	question string,
	topN int,
	chunks []map[string]any,
) ([]map[string]any, error) {
	if e == nil {
		return nil, fmt.Errorf("retrieval enhancer is not configured")
	}
	kbinfos := map[string]any{"chunks": chunks}
	enhancer := service.NewTOCEnhancer(
		e.docEngine,
		chatModel,
		tenantIDs,
		kbIDs,
		question,
		topN,
	)
	if _, err := enhancer.Enhance(ctx, kbinfos); err != nil {
		return nil, err
	}
	enhanced, ok := kbinfos["chunks"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("TOC enhancer returned invalid chunks type %T", kbinfos["chunks"])
	}
	return enhanced, nil
}

// RetrieveByChildren aggregates child chunks under their parent chunks.
func (e *Enhancer) RetrieveByChildren(
	ctx context.Context,
	chunks []map[string]any,
	tenantIDs []string,
) []map[string]any {
	if e == nil || e.docEngine == nil {
		return chunks
	}
	return nlp.RetrievalByChildren(chunks, tenantIDs, e.docEngine, ctx)
}
