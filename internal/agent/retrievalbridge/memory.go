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

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/permission"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

// MemoryAdapter exposes MemoryService.SearchMessage through the agent tool's
// retrieval interface.
type MemoryAdapter struct {
	svc *service.MemoryService
}

// NewMemoryAdapter creates a memory retrieval adapter.
func NewMemoryAdapter(svc *service.MemoryService) *MemoryAdapter {
	return &MemoryAdapter{svc: svc}
}

// Search performs hybrid memory-message retrieval and translates messages to
// the common RetrievalChunk result shape.
func (a *MemoryAdapter) Search(
	ctx context.Context,
	db *gorm.DB,
	req agenttool.RetrievalRequest,
) ([]agenttool.RetrievalChunk, error) {
	if a == nil || a.svc == nil {
		return nil, agenttool.ErrMemoryRetrievalServiceMissing
	}
	memoryIDs := compactStrings(req.MemoryIDs)
	if len(memoryIDs) == 0 {
		return nil, fmt.Errorf("memory retrieval: memory_ids is required")
	}
	actorID := strings.TrimSpace(req.TenantID)
	if state, stateErr := runtime.GetStateFromContext(ctx); stateErr == nil && state != nil {
		if userID, _ := state.Sys["user_id"].(string); strings.TrimSpace(userID) != "" {
			actorID = strings.TrimSpace(userID)
		}
	}
	if actorID == "" {
		return nil, fmt.Errorf("memory retrieval: user id is required")
	}
	if err := agenttool.CheckCanvasDependencies(ctx, db, permission.ResourceKindMemory, memoryIDs); err != nil {
		return nil, err
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
		actorID,
		filter,
		map[string]any{
			"query":                      req.Query,
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
