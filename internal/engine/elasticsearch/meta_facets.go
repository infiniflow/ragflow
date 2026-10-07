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
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

const (
	// metaFacetIDBatch is Elasticsearch's index.max_terms_count default: a terms
	// query naming more values than this is rejected, so the document ids go in
	// batches of it and the counts are added up across them.
	metaFacetIDBatch = 65536
	// metaFacetCarryingAgg names the aggregation that counts the documents
	// holding at least one metadata value.
	metaFacetCarryingAgg = "carrying"
)

// MetaFacets counts, among the documents docIDs of knowledge base kbID, how
// many hold each metadata value: {key: {value: count}}, plus how many documents
// hold any metadata value at all. The second number is what lets the caller
// derive the file list's empty-metadata bucket without reading a document.
//
// Scoped by document id rather than by kb_id, because the two are not the same
// set: the doc-meta index can also hold rows whose document is gone or was
// never created, and counting those would offer the file list values that
// match nothing.
//
// A document counts once per distinct value it holds. Returns
// types.ErrMetaValueSpaceIncomplete -- and no counts -- whenever the answer
// could be quietly wrong: a partial response, a value no bucket can show (see
// uncoveredValueFilters), a key with no aggregatable field at all, or a blank
// value, which the document is counted as carrying but no facet entry can
// show. The caller then counts the documents itself.
func (e *Engine) MetaFacets(ctx context.Context, tenantID, kbID string, docIDs []string) (map[string]map[string]int64, int64, error) {
	if tenantID == "" {
		return nil, 0, fmt.Errorf("tenantID cannot be empty")
	}
	if kbID == "" {
		return nil, 0, fmt.Errorf("kbID cannot be empty")
	}
	counts := map[string]map[string]int64{}
	if len(docIDs) == 0 {
		return counts, 0, nil
	}

	indexName := buildMetadataIndexName(tenantID)
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to check metadata index existence: %w", err)
	}
	if !exists {
		// No metadata has ever been written for this tenant.
		return counts, 0, nil
	}

	fields, unaggregatable, err := e.metaAggFields(ctx, indexName)
	if err != nil {
		return nil, 0, err
	}
	if len(fields) == 0 {
		if len(unaggregatable) > 0 {
			return nil, 0, fmt.Errorf("%w: no aggregatable metadata field in index=%s, only %s",
				types.ErrMetaValueSpaceIncomplete, indexName, strings.Join(unaggregatable, ", "))
		}
		return counts, 0, nil
	}

	carryingFilters := make([]interface{}, 0, len(fields))
	for _, key := range sortedKeys(fields) {
		carryingFilters = append(carryingFilters, map[string]interface{}{"exists": map[string]interface{}{"field": "meta_fields." + key}})
	}
	carryingAgg := map[string]interface{}{
		metaFacetCarryingAgg: map[string]interface{}{
			"filter": map[string]interface{}{
				"bool": map[string]interface{}{"should": carryingFilters, "minimum_should_match": 1},
			},
		},
	}

	var carrying int64
	requests := 0
	for offset := 0; offset < len(docIDs); offset += metaFacetIDBatch {
		batch := docIDs[offset:min(offset+metaFacetIDBatch, len(docIDs))]
		query := map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []interface{}{
					map[string]interface{}{"term": map[string]interface{}{"kb_id": kbID}},
					// Rows carry the document id both as _id and in the id field.
					map[string]interface{}{"bool": map[string]interface{}{
						"should": []interface{}{
							map[string]interface{}{"terms": map[string]interface{}{"id": batch}},
							map[string]interface{}{"terms": map[string]interface{}{"_id": batch}},
						},
						"minimum_should_match": 1,
					}},
				},
			},
		}

		first, n, err := e.walkMetaValueBuckets(ctx, indexName, query, fields, unaggregatable, carryingAgg,
			func(key string, field metaAggField, raw json.RawMessage, docCount int64) error {
				value := formatMetaValue(raw, field.typ)
				if strings.TrimSpace(value) == "" {
					// The carrying count sees the document, no facet entry can
					// show it, and it is not empty either: only a document scan
					// can place it.
					return fmt.Errorf("%w: blank value for metadata key %q in index=%s",
						types.ErrMetaValueSpaceIncomplete, key, indexName)
				}
				perValue, ok := counts[key]
				if !ok {
					perValue = map[string]int64{}
					counts[key] = perValue
				}
				// Batches are disjoint, and two stored values can render to one
				// label (dates do), so add rather than assign.
				perValue[value] += docCount
				return nil
			})
		requests += n
		if err != nil {
			return nil, 0, err
		}

		raw, ok := first.Aggregations[metaFacetCarryingAgg]
		if !ok {
			return nil, 0, fmt.Errorf("%w: aggregation %s missing from the response for index=%s",
				types.ErrMetaValueSpaceIncomplete, metaFacetCarryingAgg, indexName)
		}
		var agg struct {
			DocCount int64 `json:"doc_count"`
		}
		if err := json.Unmarshal(raw, &agg); err != nil {
			return nil, 0, fmt.Errorf("failed to parse aggregation %s of %s: %w", metaFacetCarryingAgg, indexName, err)
		}
		carrying += agg.DocCount
	}

	common.Debug("Elasticsearch metadata facets built",
		zap.String("index", indexName),
		zap.String("kb_id", kbID),
		zap.Int("doc_count", len(docIDs)),
		zap.Int("key_count", len(counts)),
		zap.Int("requests", requests))
	return counts, carrying, nil
}
