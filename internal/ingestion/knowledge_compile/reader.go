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
	"fmt"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Reader loads the per-document compiled products (available_int=0) of a KB so
// the consumer can recompute the dataset-level merged set (§11.6 step 1, §11.7
// incremental re-dedup).
type Reader interface {
	LoadCompiledProducts(ctx context.Context, tenant, kb string) ([]kccommon.Product, error)
}

type infinityReader struct{}

func (infinityReader) LoadCompiledProducts(ctx context.Context, tenant, kb string) ([]kccommon.Product, error) {
	eng := engine.Get()
	if eng == nil {
		return nil, nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	const batchSize = 5000
	var out []kccommon.Product
	offset := 0
	for {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{kb},
			Filter:     map[string]interface{}{"available_int": 0},
			SelectFields: []string{
				"id", "doc_id", "tenant_id", "compile_kwd",
				"content_with_weight", "kc_payload",
				"source_chunk_ids", "source_doc_ids",
				"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd",
				"slug_kwd", "type",
			},
			Limit:  batchSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, c := range res.Chunks {
			// Only compiled products carry compile_kwd; skip ordinary source chunks.
			if _, ok := c["compile_kwd"]; !ok {
				continue
			}
			if p, ok := productFromChunkMap(c, tenant); ok {
				out = append(out, p)
			}
		}
		// The KB-wide scan is paginated: keep fetching until a page returns
		// fewer than batchSize rows, so a KB larger than the cap merges against
		// the full compiled set instead of a truncated slice.
		if len(res.Chunks) < batchSize {
			break
		}
		offset += batchSize
	}
	return out, nil
}

// productFromChunkMap reconstructs a kccommon.Product from a stored compiled
// chunk document. It reads the payload from kc_payload (falling back to
// content_with_weight) and the embedding from the q_<dim>_vec column.
func productFromChunkMap(c map[string]interface{}, tenant string) (kccommon.Product, bool) {
	content, _ := c["kc_payload"].(string)
	if content == "" {
		content, _ = c["content_with_weight"].(string)
	}
	if content == "" {
		return kccommon.Product{}, false
	}
	id, _ := c["id"].(string)
	docID, _ := c["doc_id"].(string)
	variant, _ := c["compile_kwd"].(string)

	meta := map[string]any{}
	if v, ok := c["name_kwd"].(string); ok && v != "" {
		meta["name"] = v
	}
	if v, ok := c["entity_type_kwd"].(string); ok && v != "" {
		meta["entity_type"] = v
	}
	if v, ok := c["from_entity_kwd"].(string); ok && v != "" {
		meta["from"] = v
		meta["kind"] = "relation"
	}
	if v, ok := c["to_entity_kwd"].(string); ok && v != "" {
		meta["to"] = v
		meta["kind"] = "relation"
	}
	if v, ok := c["slug_kwd"].(string); ok && v != "" {
		meta["slug"] = v
	}
	if v, ok := c["type"].(string); ok && v != "" {
		meta["type"] = v
	}
	if _, ok := meta["kind"]; !ok {
		if _, hasName := meta["name"]; hasName {
			meta["kind"] = "entity"
		}
	}
	if v := metaStringSlice(c, "source_chunk_ids"); len(v) > 0 {
		meta["source_chunk_ids"] = v
	}
	if v := metaStringSlice(c, "source_doc_ids"); len(v) > 0 {
		meta["source_doc_ids"] = v
	}

	vec, _ := kccommon.VectorFromChunkMap(c, 0)
	return kccommon.Product{
		ID:       id,
		DocID:    docID,
		TenantID: tenant,
		Variant:  kccommon.Variant(variant),
		Content:  content,
		Vector:   vec,
		Meta:     meta,
	}, true
}
