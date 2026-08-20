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

package document

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/ingestion/knowledge_compile"

	"go.uber.org/zap"
)

const sourceChunkAvailabilityBatchSize = 1000

// updateSourceChunkAvailability changes retrieval visibility for source chunks
// only. Document-level compiler products must remain hidden (available_int=0)
// until the dataset-level compiler has merged them.
func (s *DocumentService) updateSourceChunkAvailability(ctx context.Context, tenantID, datasetID, documentID string, available int) error {
	if s.docEngine == nil {
		return fmt.Errorf("document engine not initialized")
	}
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	ids, err := s.loadSourceChunkIDs(ctx, indexName, datasetID, documentID)
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += sourceChunkAvailabilityBatchSize {
		end := min(start+sourceChunkAvailabilityBatchSize, len(ids))
		if err := s.docEngine.UpdateChunks(ctx,
			map[string]any{"id": ids[start:end]},
			map[string]any{"available_int": available},
			indexName,
			datasetID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *DocumentService) loadSourceChunkIDs(ctx context.Context, indexName, datasetID, documentID string) ([]string, error) {
	ids := make([]string, 0)
	for offset := 0; ; offset += sourceChunkAvailabilityBatchSize {
		result, err := s.docEngine.Search(ctx, &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{datasetID},
			Offset:       offset,
			Limit:        sourceChunkAvailabilityBatchSize,
			SelectFields: []string{"id", "compile_kwd"},
			Filter:       map[string]any{"doc_id": []string{documentID}},
		})
		if err != nil {
			return nil, err
		}
		if result == nil || len(result.Chunks) == 0 {
			break
		}
		for _, row := range result.Chunks {
			if strings.TrimSpace(documentStoreString(row["compile_kwd"])) != "" {
				continue
			}
			if id := strings.TrimSpace(documentStoreString(row["id"])); id != "" {
				ids = append(ids, id)
			}
		}
		if int64(offset+len(result.Chunks)) >= result.Total {
			break
		}
	}
	return ids, nil
}

func (s *DocumentService) deleteSourceChunks(ctx context.Context, tenantID, datasetID, documentID string) error {
	if s.docEngine == nil {
		return nil
	}
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	ids, err := s.loadSourceChunkIDs(ctx, indexName, datasetID, documentID)
	if err != nil || len(ids) == 0 {
		return err
	}
	for start := 0; start < len(ids); start += sourceChunkAvailabilityBatchSize {
		end := min(start+sourceChunkAvailabilityBatchSize, len(ids))
		if _, err := s.docEngine.DeleteChunks(ctx, map[string]any{
			"id":    ids[start:end],
			"kb_id": datasetID,
		}, indexName, datasetID); err != nil {
			return err
		}
	}
	return nil
}

func (s *DocumentService) markDocumentWikiDirty(ctx context.Context, tenantID, datasetID, documentID string) {
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := knowledge_compile.MarkWikiDocumentDirty(markCtx, tenantID, datasetID, documentID, nil); err != nil {
		common.Warn("document mutation: failed to schedule Wiki refresh",
			zap.String("document_id", documentID), zap.Error(err))
	}
}

func documentStoreString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []string:
		if len(typed) == 1 {
			return typed[0]
		}
	case []any:
		if len(typed) == 1 {
			if text, ok := typed[0].(string); ok {
				return text
			}
		}
	}
	return ""
}
