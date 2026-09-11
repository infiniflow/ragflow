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
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	knowledge_compile "ragflow/internal/ingestion/knowledge_compile"

	"go.uber.org/zap"
)

// publishKnowledgeCompileStatusChange notifies the dataset-level consumer when
// a document's retrieval availability changes. Documents without compiled
// products do not wake the consumer because they cannot contribute to any
// dataset-level knowledge artifact.
func (s *DocumentService) publishKnowledgeCompileStatusChange(ctx context.Context, tenantID, datasetID, documentID string, status int) {
	variants, err := s.documentKnowledgeCompileVariants(ctx, tenantID, datasetID, documentID)
	if err != nil {
		common.Warn("document mutation: failed to resolve knowledge compile variants",
			zap.String("document_id", documentID), zap.Error(err))
		return
	}
	if len(variants) == 0 {
		return
	}
	var publishErr error
	if status == 0 {
		publishErr = knowledge_compile.PublishDisabled(ctx, tenantID, datasetID, documentID, variants)
	} else {
		publishErr = knowledge_compile.PublishEnabled(ctx, tenantID, datasetID, documentID, variants)
	}
	if publishErr != nil {
		common.Warn("document mutation: failed to publish knowledge compile status change",
			zap.String("document_id", documentID), zap.Int("status", status), zap.Error(publishErr))
	}
}

func (s *DocumentService) documentKnowledgeCompileVariants(ctx context.Context, tenantID, datasetID, documentID string) ([]string, error) {
	if s.docEngine == nil {
		return nil, nil
	}
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	seen := make(map[string]struct{})
	for offset := 0; ; offset += 1000 {
		result, err := s.docEngine.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{datasetID},
			Offset:       offset,
			Limit:        1000,
			SelectFields: []string{"compile_kwd", "compilation_template_kind_kwd"},
			Filter:       map[string]any{"doc_id": []string{documentID}},
		})
		if err != nil {
			return nil, err
		}
		if result == nil || len(result.Chunks) == 0 {
			break
		}
		for _, row := range result.Chunks {
			kind := strings.TrimSpace(documentStoreString(row["compilation_template_kind_kwd"]))
			variant, variantErr := kccommon.KindToVariant(kind)
			if variantErr != nil {
				variant, variantErr = knowledge_compile.KwdToVariant(strings.TrimSpace(documentStoreString(row["compile_kwd"])))
			}
			if variantErr == nil {
				seen[string(variant)] = struct{}{}
			}
		}
		if int64(offset+len(result.Chunks)) >= result.Total {
			break
		}
	}
	variants := make([]string, 0, len(seen))
	for variant := range seen {
		variants = append(variants, variant)
	}
	sort.Strings(variants)
	return variants, nil
}
