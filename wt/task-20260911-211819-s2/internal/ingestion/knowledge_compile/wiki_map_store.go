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

package knowledge_compile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const (
	wikiMapExtractCompileKWD = "wiki_map_extract"
	wikiMapActiveCompileKWD  = "wiki_map_active"
	wikiMapStoreBatchSize    = 500
)

type wikiMapVersionStore struct {
	engine engine.DocEngine
}

func (s *wikiMapVersionStore) GetWikiMapActiveState(ctx context.Context, tenantID, datasetID, key string) ([]byte, error) {
	if s == nil || s.engine == nil {
		return nil, fmt.Errorf("wiki MAP active-state DocStore is not initialized")
	}
	result, err := s.engine.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        []string{datasetID},
		Limit:        1,
		SelectFields: []string{"id", "compile_kwd", "content_with_weight"},
		Filter: map[string]interface{}{
			"id":            []string{key},
			"compile_kwd":   wikiMapActiveCompileKWD,
			"available_int": 0,
		},
	})
	if err != nil || result == nil || len(result.Chunks) == 0 {
		return nil, err
	}
	payload := mapStoreString(result.Chunks[0]["content_with_weight"])
	if payload == "" {
		return nil, nil
	}
	return []byte(payload), nil
}

func (s *wikiMapVersionStore) PutWikiMapActiveState(ctx context.Context, state kccommon.WikiMapActiveState) error {
	if s == nil || s.engine == nil {
		return fmt.Errorf("wiki MAP active-state DocStore is not initialized")
	}
	if state.Key == "" || state.TenantID == "" || state.DatasetID == "" || state.DocumentID == "" {
		return fmt.Errorf("save Wiki MAP active state: key and scope are required")
	}
	row := map[string]interface{}{
		"id":                  state.Key,
		"doc_id":              "wiki_map_active:" + state.DocumentID,
		"tenant_id":           state.TenantID,
		"kb_id":               state.DatasetID,
		"compile_kwd":         wikiMapActiveCompileKWD,
		"scope_kwd":           "doc",
		"source_doc_ids":      []string{state.DocumentID},
		"content_with_weight": string(state.Payload),
		"available_int":       0,
	}
	_, err := s.engine.InsertChunks(ctx, []map[string]interface{}{row}, fmt.Sprintf("ragflow_%s", state.TenantID), state.DatasetID)
	return err
}

// NewWikiMapVersionStore returns the DocStore-backed immutable Wiki MAP cache.
// Its rows remain non-searchable through available_int=0 and the compile
// discriminator, while preserving every chunk/hash version for reuse.
func NewWikiMapVersionStore(docEngine engine.DocEngine) kccommon.WikiMapVersionStore {
	return &wikiMapVersionStore{engine: docEngine}
}

func (s *wikiMapVersionStore) GetWikiMapVersions(ctx context.Context, tenantID, datasetID string, keys []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	if s == nil || s.engine == nil {
		return nil, fmt.Errorf("wiki MAP version DocStore is not initialized")
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(datasetID) == "" {
		return nil, fmt.Errorf("load Wiki MAP versions: tenant_id and dataset_id are required")
	}
	for start := 0; start < len(keys); start += wikiMapStoreBatchSize {
		end := min(start+wikiMapStoreBatchSize, len(keys))
		result, err := s.engine.Search(ctx, &types.SearchRequest{
			IndexNames: []string{fmt.Sprintf("ragflow_%s", tenantID)},
			KbIDs:      []string{datasetID},
			Limit:      end - start,
			SelectFields: []string{
				"id", "compile_kwd", "content_with_weight",
			},
			Filter: map[string]interface{}{
				"id":            keys[start:end],
				"compile_kwd":   wikiMapExtractCompileKWD,
				"available_int": 0,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("load Wiki MAP versions: %w", err)
		}
		if result == nil {
			continue
		}
		for _, row := range result.Chunks {
			if mapStoreString(row["compile_kwd"]) != wikiMapExtractCompileKWD {
				continue
			}
			id := mapStoreString(row["id"])
			payload := mapStoreString(row["content_with_weight"])
			if id != "" && payload != "" {
				out[id] = []byte(payload)
			}
		}
	}
	return out, nil
}

func (s *wikiMapVersionStore) PutWikiMapVersions(ctx context.Context, versions []kccommon.WikiMapVersion) error {
	if len(versions) == 0 {
		return nil
	}
	if s == nil || s.engine == nil {
		return fmt.Errorf("wiki MAP version DocStore is not initialized")
	}
	byScope := make(map[string][]kccommon.WikiMapVersion)
	for _, version := range versions {
		if version.Key == "" || version.DatasetID == "" || version.TenantID == "" {
			return fmt.Errorf("save Wiki MAP version: key, tenant_id, and dataset_id are required")
		}
		scope := version.TenantID + "\x00" + version.DatasetID
		byScope[scope] = append(byScope[scope], version)
	}
	for _, scopedVersions := range byScope {
		for start := 0; start < len(scopedVersions); start += wikiMapStoreBatchSize {
			end := min(start+wikiMapStoreBatchSize, len(scopedVersions))
			batch := scopedVersions[start:end]
			keys := make([]string, len(batch))
			for i := range batch {
				keys[i] = batch[i].Key
			}
			existing, err := s.GetWikiMapVersions(ctx, batch[0].TenantID, batch[0].DatasetID, keys)
			if err != nil {
				return err
			}
			rows := make([]map[string]interface{}, 0, len(batch))
			for _, version := range batch {
				if _, exists := existing[version.Key]; !exists {
					rows = append(rows, wikiMapVersionRow(version))
				}
			}
			if len(rows) == 0 {
				continue
			}
			if _, err := s.engine.InsertChunks(ctx, rows, fmt.Sprintf("ragflow_%s", batch[0].TenantID), batch[0].DatasetID); err != nil {
				return fmt.Errorf("save Wiki MAP versions: %w", err)
			}
		}
	}
	return nil
}

func wikiMapVersionRow(version kccommon.WikiMapVersion) map[string]interface{} {
	return map[string]interface{}{
		"id": version.Key,
		// Keep immutable MAP history in a separate document namespace so source
		// document deletion cannot remove a reusable chunk/hash version.
		"doc_id":              wikiMapCacheDocID(version.DocumentID),
		"tenant_id":           version.TenantID,
		"kb_id":               version.DatasetID,
		"compile_kwd":         wikiMapExtractCompileKWD,
		"scope_kwd":           "doc",
		"source_chunk_ids":    []string{version.ChunkID},
		"source_doc_ids":      []string{version.DocumentID},
		"chunk_hash_kwd":      version.ContentHash,
		"input_hash_kwd":      wikiMapInputFingerprint(version),
		"content_with_weight": string(version.Payload),
		"available_int":       0,
	}
}

func wikiMapCacheDocID(documentID string) string {
	return "wiki_map_cache:" + documentID
}

func wikiMapInputFingerprint(version kccommon.WikiMapVersion) string {
	sum := sha256.Sum256([]byte(version.TemplateFingerprint + "\x00" + version.LLMFingerprint))
	return hex.EncodeToString(sum[:])
}

func mapStoreString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		if len(typed) == 1 {
			return strings.TrimSpace(typed[0])
		}
	case []interface{}:
		if len(typed) == 1 {
			if value, ok := typed[0].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}
