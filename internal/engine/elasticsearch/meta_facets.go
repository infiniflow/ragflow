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

package elasticsearch

import (
	"context"
	"fmt"

	"ragflow/internal/engine/types"
)

// metaFacetIDBatch caps how many document ids one search scopes to, staying
// within index.max_terms_count.
const metaFacetIDBatch = 65536

// metaFacetSkipKeys are the meta_fields keys the file list never shows.
var metaFacetSkipKeys = []string{"id", "kb_id"}

// MetaFacets counts, among docIDs of knowledge base kbID, how many documents
// hold each metadata value, and how many hold any metadata value at all.
//
// Scoped by document id as well as kb_id: the doc-meta index can hold rows
// whose document is gone or was never created, and a facet built from the
// index alone would offer values no listed document has. The ids go in
// batches; a document is in one batch only, so the counts add up.
//
// Values come from walkMetaValues, so they are read from _source like the
// file list's scan reads them. Returns types.ErrMetaValueSpaceIncomplete when
// the cluster answers with partial results.
func (e *Engine) MetaFacets(ctx context.Context, tenantID, kbID string, docIDs []string) ([]types.MetaValueCount, int64, error) {
	if tenantID == "" {
		return nil, 0, fmt.Errorf("tenantID cannot be empty")
	}
	if len(docIDs) == 0 {
		return nil, 0, nil
	}

	indexName := buildMetadataIndexName(tenantID)
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to check metadata index existence: %w", err)
	}
	if !exists {
		return nil, 0, nil
	}

	type valueKey struct {
		key   string
		value interface{}
	}
	counts := map[valueKey]int64{}
	var order []valueKey
	var carrying int64
	for start := 0; start < len(docIDs); start += metaFacetIDBatch {
		batch := docIDs[start:min(start+metaFacetIDBatch, len(docIDs))]
		query := map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []interface{}{
					map[string]interface{}{"term": map[string]interface{}{"kb_id": kbID}},
					map[string]interface{}{"ids": map[string]interface{}{"values": batch}},
				},
			},
		}
		n, err := e.walkMetaValues(ctx, indexName, query, metaFacetSkipKeys, func(key string, value interface{}, docs int64) {
			k := valueKey{key: key, value: value}
			if _, seen := counts[k]; !seen {
				order = append(order, k)
			}
			counts[k] += docs
		})
		if err != nil {
			return nil, 0, err
		}
		carrying += n
	}

	facets := make([]types.MetaValueCount, 0, len(order))
	for _, k := range order {
		facets = append(facets, types.MetaValueCount{Key: k.key, Value: k.value, Docs: counts[k]})
	}
	return facets, carrying, nil
}
