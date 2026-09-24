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
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/knowledge_compile"

	"go.uber.org/zap"
)

const sourceChunkAvailabilityBatchSize = 1000

// updateDocumentChunkAvailability changes retrieval visibility for every row a
// disabled/enabled document is allowed to expose: its ordinary source chunks and
// its per-document final compiled products (tree / structure / mindmap — the wiki
// variant is hidden at write time, see markCompiledProductsHidden). Wiki staging
// rows and unknown-kwd rows are skipped; see loadAvailabilityToggleChunkIDs.
// Parent-child parent rows stay available_int=0 on enable (citation-only).
func (s *DocumentService) updateDocumentChunkAvailability(ctx context.Context, tenantID, datasetID, documentID string, available int) error {
	if s.docEngine == nil {
		return fmt.Errorf("document engine not initialized")
	}
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	ids, err := s.loadAvailabilityToggleChunkIDs(ctx, indexName, datasetID, documentID, available)
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

type availabilityToggleRow struct {
	id       string
	compiled bool
	momID    string
}

// loadAvailabilityToggleChunkIDs returns the document's own rows whose
// availability follows the document status: source chunks (no compile_kwd) and
// final compiled products (a compile_kwd mapping to a non-wiki variant). Skipped:
// the wiki variant (wiki_page / wiki_section), whose per-document rows are
// staging for the dataset-level merge, and unknown-kwd rows such as the
// wiki_map_active state row or the legacy, KB-scoped wiki_page_graph blob.
// When enabling (available==1) a parent-child document, source rows without
// mom_id are also skipped — those are hidden parent passages.
func (s *DocumentService) loadAvailabilityToggleChunkIDs(ctx context.Context, indexName, datasetID, documentID string, available int) ([]string, error) {
	rows := make([]availabilityToggleRow, 0)
	hasChild := false
	for offset := 0; ; offset += sourceChunkAvailabilityBatchSize {
		searchCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
		result, err := s.docEngine.Search(searchCtx, &enginetypes.SearchRequest{
			IndexNames:         []string{indexName},
			KbIDs:              []string{datasetID},
			Offset:             offset,
			Limit:              sourceChunkAvailabilityBatchSize,
			SelectFields:       []string{"id", "compile_kwd", "mom_id"},
			Filter:             map[string]any{"doc_id": []string{documentID}},
			IncludeUnavailable: true,
		})
		cancel()
		if err != nil {
			return nil, err
		}
		if result == nil || len(result.Chunks) == 0 {
			break
		}
		for _, row := range result.Chunks {
			id := strings.TrimSpace(documentStoreString(row["id"]))
			if id == "" {
				continue
			}
			if kwd := strings.TrimSpace(documentStoreString(row["compile_kwd"])); kwd != "" {
				variant, variantErr := knowledge_compile.KwdToVariant(kwd)
				if variantErr != nil || variant == kccommon.VariantWiki {
					continue
				}
				rows = append(rows, availabilityToggleRow{id: id, compiled: true})
				continue
			}
			momID := strings.TrimSpace(documentStoreString(row["mom_id"]))
			if momID != "" {
				hasChild = true
			}
			rows = append(rows, availabilityToggleRow{id: id, momID: momID})
		}
		if int64(offset+len(result.Chunks)) >= result.Total {
			break
		}
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if available == 1 && hasChild && !row.compiled && row.momID == "" {
			continue
		}
		ids = append(ids, row.id)
	}
	return ids, nil
}

func (s *DocumentService) loadSourceChunkIDs(ctx context.Context, indexName, datasetID, documentID string, includeUnavailable bool) ([]string, error) {
	ids := make([]string, 0)
	for offset := 0; ; offset += sourceChunkAvailabilityBatchSize {
		searchCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
		result, err := s.docEngine.Search(searchCtx, &enginetypes.SearchRequest{
			IndexNames:         []string{indexName},
			KbIDs:              []string{datasetID},
			Offset:             offset,
			Limit:              sourceChunkAvailabilityBatchSize,
			SelectFields:       []string{"id", "compile_kwd"},
			Filter:             map[string]any{"doc_id": []string{documentID}},
			IncludeUnavailable: includeUnavailable,
		})
		cancel()
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
	// Reparse is the migration boundary for parent-child IDs. Include hidden
	// parent rows so old hash(mom) rows are removed with their children before
	// new document-scoped rows are written.
	ids, err := s.loadSourceChunkIDs(ctx, indexName, datasetID, documentID, true)
	if err != nil || len(ids) == 0 {
		return err
	}
	for start := 0; start < len(ids); start += sourceChunkAvailabilityBatchSize {
		end := min(start+sourceChunkAvailabilityBatchSize, len(ids))
		batchCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
		_, err = s.docEngine.DeleteChunks(batchCtx, map[string]any{
			"id":    ids[start:end],
			"kb_id": datasetID,
		}, indexName, datasetID)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

// deleteDocumentGeneratedChunks removes document-scoped compiler products
// without touching source chunks or dataset-level merged products.
func (s *DocumentService) deleteDocumentGeneratedChunks(ctx context.Context, tenantID, datasetID, documentID string) error {
	if s.docEngine == nil {
		return nil
	}
	batchCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
	_, err := s.docEngine.DeleteChunks(batchCtx, map[string]any{
		"doc_id":        documentID,
		"kb_id":         datasetID,
		"available_int": 0,
	}, fmt.Sprintf("ragflow_%s", tenantID), datasetID)
	cancel()
	return err
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
