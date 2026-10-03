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
	"fmt"
	"strings"

	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/service"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// memoryMessageSearcher is the MemoryService surface the adapter searches.
type memoryMessageSearcher interface {
	SearchMessage(
		ctx context.Context,
		userID string,
		filterDict, params map[string]any,
	) ([]map[string]any, common.ErrorCode, error)
}

// queryTranslator translates the query into the tool's cross_languages.
type queryTranslator interface {
	CrossLanguages(
		ctx context.Context,
		tenantID, query string,
		languages []string,
	) (string, error)
}

// MemoryAdapter exposes MemoryService.SearchMessage through the agent tool's
// retrieval interface.
type MemoryAdapter struct {
	svc        memoryMessageSearcher
	translator queryTranslator
}

// NewMemoryAdapter creates a memory retrieval adapter. The translator applies
// the tool's cross_languages to the query, as the dataset path does.
func NewMemoryAdapter(svc *service.MemoryService, translator queryTranslator) *MemoryAdapter {
	adapter := &MemoryAdapter{translator: translator}
	if svc != nil {
		adapter.svc = svc
	}
	return adapter
}

// Search performs hybrid memory-message retrieval and translates messages to
// the common RetrievalChunk result shape.
func (a *MemoryAdapter) Search(
	ctx context.Context,
	_ *gorm.DB,
	req agenttool.RetrievalRequest,
) ([]agenttool.RetrievalChunk, error) {
	if a == nil || a.svc == nil {
		return nil, agenttool.ErrMemoryRetrievalServiceMissing
	}
	if strings.TrimSpace(req.TenantID) == "" {
		return nil, fmt.Errorf("memory retrieval: tenant id is required")
	}
	memoryIDs := compactStrings(req.MemoryIDs)
	if len(memoryIDs) == 0 {
		return nil, fmt.Errorf("memory retrieval: memory_ids is required")
	}
	query := req.Query
	if len(req.CrossLanguages) > 0 {
		if a.translator == nil {
			return nil, fmt.Errorf("memory retrieval: cross-language service is not configured")
		}
		translated, err := a.translator.CrossLanguages(ctx, req.TenantID, query, req.CrossLanguages)
		if err != nil {
			common.Warn("agent memory retrieval: cross-language query failed; using original query", zap.Error(err))
		} else if strings.TrimSpace(translated) != "" {
			query = translated
		}
	}
	keywordWeight := 0.7
	if req.KeywordsSimilarityWeight != nil {
		keywordWeight = *req.KeywordsSimilarityWeight
	}
	filter := map[string]any{"memory_id": memoryIDs}
	if user := strings.TrimSpace(req.UserID); user != "" {
		filter["user_id"] = user
	}
	messages, _, err := a.svc.SearchMessage(
		ctx,
		req.TenantID,
		filter,
		map[string]any{
			"query":                      query,
			"similarity_threshold":       req.SimilarityThreshold,
			"keywords_similarity_weight": keywordWeight,
			"top_n":                      req.TopN,
		},
	)
	if err != nil {
		return nil, err
	}
	chunks := make([]agenttool.RetrievalChunk, 0, len(messages))
	for _, message := range messages {
		memoryID := fmt.Sprint(message["memory_id"])
		chunks = append(chunks, agenttool.RetrievalChunk{
			ID:         fmt.Sprint(message["message_id"]),
			Content:    fmt.Sprint(message["content"]),
			DocumentID: memoryID,
			DatasetID:  memoryID,
		})
	}
	return chunks, nil
}

func compactStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
