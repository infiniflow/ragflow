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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

const (
	// esMaxBuckets is Elasticsearch's search.max_buckets default: a search that
	// would build more buckets than this is rejected. One composite aggregation
	// per metadata key shares that budget, so the per-key page size is divided
	// out of it rather than fixed. This bounds a single round trip, not the
	// answer -- a key whose values fill a page is re-requested from its
	// after_key until exhausted.
	esMaxBuckets = 65536
	// metaValueSpacePageSize is the per-key ceiling on one round trip.
	metaValueSpacePageSize = 1000
	// metaValueSpaceCoverageAgg names the aggregation that counts the values
	// the composite sources cannot reach.
	metaValueSpaceCoverageAgg = "uncovered"
)

// metaAggField is the aggregatable field path for a metadata key, with the ES
// type it was mapped as. The type travels with the path because a composite
// terms source over a date field keys its buckets by epoch milliseconds, and
// only the mapping says which keys those are.
type metaAggField struct {
	path string
	typ  string
}

// esFieldMapping is the recursive shape of an index mapping's field node.
type esFieldMapping struct {
	Type       string                    `json:"type"`
	Fields     map[string]esFieldMapping `json:"fields"`
	Properties map[string]esFieldMapping `json:"properties"`
}

type esIndexMapping struct {
	Mappings esFieldMapping `json:"mappings"`
}

// compositeAggregation is one key's page of composite buckets. after_key and
// the bucket keys stay as raw JSON: after_key must go back to ES in the store's
// own representation, and a bucket key may be a string, a number or a boolean.
type compositeAggregation struct {
	AfterKey map[string]json.RawMessage `json:"after_key"`
	Buckets  []struct {
		Key map[string]json.RawMessage `json:"key"`
	} `json:"buckets"`
}

// filtersAggregation is a keyed filters aggregation: one document count per
// named filter.
type filtersAggregation struct {
	Buckets map[string]struct {
		DocCount int `json:"doc_count"`
	} `json:"buckets"`
}

type metaValueSpaceResponse struct {
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Failed     int `json:"failed"`
		Skipped    int `json:"skipped"`
	} `json:"_shards"`
	// Left raw: the round carries composite aggregations and a filters
	// aggregation, which do not share a shape.
	Aggregations map[string]json.RawMessage `json:"aggregations"`
}

// MetaValueSpace returns every distinct metadata value per key for the given
// knowledge bases: {key: [value, ...]}.
//
// Deliberately not SearchMetadata: that reads the doc-meta index with from/size
// and so stops at the doc store's result window, which hands the filter
// generator the metadata of an arbitrary prefix and asks it to pick a value
// that may not be in it. An aggregation is not bound by that window, and its
// cost scales with the metadata's cardinality rather than the document count,
// so the result is complete for a dataset of any size.
//
// Returns types.ErrMetaValueSpaceIncomplete when the cluster answers with
// partial results, or when the knowledge bases hold a value no aggregation can
// see, rather than a space that silently omits values. An empty space means the
// index or its mapping holds nothing aggregatable; the caller decides what to
// do with that.
func (e *Engine) MetaValueSpace(ctx context.Context, tenantID string, kbIDs []string) (map[string][]string, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID cannot be empty")
	}
	if len(kbIDs) == 0 {
		return map[string][]string{}, nil
	}

	indexName := buildMetadataIndexName(tenantID)
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to check metadata index existence: %w", err)
	}
	if !exists {
		return map[string][]string{}, nil
	}

	fields, unaggregatable, err := e.metaAggFields(ctx, indexName)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return map[string][]string{}, nil
	}

	// composite, not terms: a terms aggregation returns only the top `size`
	// values and drops the rest silently, which is the same class of
	// incompleteness this method exists to remove -- and its threshold moves as
	// metadata keys are added, since they share one bucket budget. composite
	// pages instead, so the page size bounds a round trip rather than the answer.
	//
	// All keys travel in one request, each as its own composite aggregation with
	// its own after_key. Only a key whose values filled an entire page is carried
	// into another round, so the common case -- every key below the page size --
	// costs exactly one search.
	pageSize := max(1, min(metaValueSpacePageSize, esMaxBuckets/len(fields)))
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"filter": []interface{}{
				map[string]interface{}{"terms": map[string]interface{}{"kb_id": kbIDs}},
			},
		},
	}

	// What the buckets cannot show, asked once, in this scope: see
	// uncoveredValueFilters.
	uncovered := uncoveredValueFilters(fields, unaggregatable)

	space := make(map[string][]string, len(fields))
	after := make(map[string]map[string]json.RawMessage, len(fields))
	pending := fields
	requests := 0
	for len(pending) > 0 {
		aggs := make(map[string]interface{}, len(pending))
		for _, key := range sortedKeys(pending) {
			composite := map[string]interface{}{
				"size":    pageSize,
				"sources": []interface{}{map[string]interface{}{key: map[string]interface{}{"terms": map[string]interface{}{"field": pending[key].path}}}},
			}
			if cursor, ok := after[key]; ok {
				composite["after"] = cursor
			}
			aggs[metaValueSpaceAggName(key)] = map[string]interface{}{"composite": composite}
		}
		if len(uncovered) > 0 {
			aggs[metaValueSpaceCoverageAgg] = map[string]interface{}{"filters": map[string]interface{}{"filters": uncovered}}
		}

		resp, err := e.searchMetaValueSpace(ctx, indexName, query, aggs)
		if err != nil {
			return nil, err
		}
		requests++

		if resp.TimedOut || resp.Shards.Failed > 0 {
			return nil, fmt.Errorf("%w: partial aggregation response for index=%s: timed_out=%t, failed_shards=%d of %d",
				types.ErrMetaValueSpaceIncomplete, indexName, resp.TimedOut, resp.Shards.Failed, resp.Shards.Total)
		}

		if len(uncovered) > 0 {
			if err := requireFullValueCoverage(resp, indexName); err != nil {
				return nil, err
			}
			uncovered = nil
		}

		unfinished := make(map[string]metaAggField, len(pending))
		for key, field := range pending {
			raw, ok := resp.Aggregations[metaValueSpaceAggName(key)]
			if !ok {
				// A requested aggregation that came back absent is not an empty
				// result; treating it as one would silently drop the whole key
				// from the value space.
				return nil, fmt.Errorf("%w: aggregation %s missing from the response for index=%s",
					types.ErrMetaValueSpaceIncomplete, metaValueSpaceAggName(key), indexName)
			}
			var agg compositeAggregation
			if err := json.Unmarshal(raw, &agg); err != nil {
				return nil, fmt.Errorf("failed to parse aggregation %s of %s: %w", metaValueSpaceAggName(key), indexName, err)
			}
			for _, bucket := range agg.Buckets {
				space[key] = append(space[key], formatMetaValue(bucket.Key[key], field.typ))
			}
			if len(agg.Buckets) == pageSize && len(agg.AfterKey) > 0 {
				after[key] = agg.AfterKey
				unfinished[key] = field
			}
		}
		pending = unfinished
	}

	common.Debug("Elasticsearch metadata value space built",
		zap.String("index", indexName),
		zap.Int("kb_count", len(kbIDs)),
		zap.Int("key_count", len(space)),
		zap.Int("requests", requests))
	return space, nil
}

// metaValueSpaceAggName namespaces a metadata key inside the aggregation body.
func metaValueSpaceAggName(key string) string {
	return "vs_" + key
}

// uncoveredValueFilters names one filter per way a stored value can never reach
// a bucket.
//
// A dynamically mapped string aggregates through its .keyword subfield, which
// carries ignore_above (256 by default), so a longer value is indexed as text
// only and has no bucket. A key the mapping gives no aggregatable field at all
// -- an object, which a dict-valued metadata entry creates -- has none either.
// Either one would hand the filter generator a value space that quietly omits
// values, and a filter written from it excludes the documents holding them.
//
// Each is asked as a document count inside the caller's own scope, so a key no
// document in these knowledge bases carries costs nothing.
func uncoveredValueFilters(fields map[string]metaAggField, unaggregatable []string) map[string]interface{} {
	filters := make(map[string]interface{}, len(fields)+len(unaggregatable))
	for key, field := range fields {
		parent := "meta_fields." + key
		if field.path == parent {
			continue
		}
		filters[key] = map[string]interface{}{"bool": map[string]interface{}{
			"filter":   []interface{}{map[string]interface{}{"exists": map[string]interface{}{"field": parent}}},
			"must_not": []interface{}{map[string]interface{}{"exists": map[string]interface{}{"field": field.path}}},
		}}
	}
	for _, key := range unaggregatable {
		filters[key] = map[string]interface{}{"exists": map[string]interface{}{"field": "meta_fields." + key}}
	}
	return filters
}

// requireFullValueCoverage refuses a value space whose scope holds values no
// bucket can show.
func requireFullValueCoverage(resp *metaValueSpaceResponse, indexName string) error {
	raw, ok := resp.Aggregations[metaValueSpaceCoverageAgg]
	if !ok {
		return fmt.Errorf("%w: aggregation %s missing from the response for index=%s",
			types.ErrMetaValueSpaceIncomplete, metaValueSpaceCoverageAgg, indexName)
	}
	var agg filtersAggregation
	if err := json.Unmarshal(raw, &agg); err != nil {
		return fmt.Errorf("failed to parse aggregation %s of %s: %w", metaValueSpaceCoverageAgg, indexName, err)
	}
	var hidden []string
	for key, bucket := range agg.Buckets {
		if bucket.DocCount > 0 {
			hidden = append(hidden, fmt.Sprintf("%s=%d", key, bucket.DocCount))
		}
	}
	if len(hidden) > 0 {
		sort.Strings(hidden)
		return fmt.Errorf("%w: values no aggregation can see for index=%s: %s",
			types.ErrMetaValueSpaceIncomplete, indexName, strings.Join(hidden, ", "))
	}
	return nil
}

// searchMetaValueSpace runs one aggregation round.
func (e *Engine) searchMetaValueSpace(ctx context.Context, indexName string, query map[string]interface{}, aggs map[string]interface{}) (*metaValueSpaceResponse, error) {
	body := map[string]interface{}{"size": 0, "query": query, "aggs": aggs}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("error encoding metadata value space query: %w", err)
	}

	// allow_partial_search_results=false: the default lets ES answer HTTP 200
	// with some shards missing, and a value absent because its shard failed is
	// indistinguishable here from a value that does not exist. Filtering on that
	// silently drops matching documents, so refuse the answer instead.
	res, err := e.client.Search(
		e.client.Search.WithContext(ctx),
		e.client.Search.WithIndex(indexName),
		e.client.Search.WithBody(&buf),
		e.client.Search.WithAllowPartialSearchResults(false),
	)
	if err != nil {
		return nil, fmt.Errorf("metadata value space aggregation failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("metadata value space aggregation returned %s: %s", res.Status(), string(bodyBytes))
	}

	var parsed metaValueSpaceResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse metadata value space response: %w", err)
	}
	return &parsed, nil
}

// metaAggFields maps each metadata key to the aggregatable field path and the
// type it was mapped as, and names the keys that have no aggregatable field at
// all -- an object mapping, which a dict-valued metadata entry creates. The
// second list is not a detail to drop: a space built from the remaining keys
// would be missing a whole key without saying so.
//
// meta_fields is mapped dynamically, so the index mapping already enumerates
// every key ever indexed for the tenant. Reading it costs one cheap metadata
// call and -- unlike scanning documents -- cannot miss a key just because the
// documents carrying it happen to sort late.
func (e *Engine) metaAggFields(ctx context.Context, indexName string) (map[string]metaAggField, []string, error) {
	req := esapi.IndicesGetMappingRequest{Index: []string{indexName}}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read mapping of %s: %w", indexName, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, nil, fmt.Errorf("failed to read mapping of %s: %s: %s", indexName, res.Status(), string(bodyBytes))
	}

	var mappings map[string]esIndexMapping
	if err := json.NewDecoder(res.Body).Decode(&mappings); err != nil {
		return nil, nil, fmt.Errorf("failed to parse mapping of %s: %w", indexName, err)
	}

	var props map[string]esFieldMapping
	for _, index := range mappings {
		if candidate := index.Mappings.Properties["meta_fields"].Properties; len(candidate) > 0 {
			props = candidate
			break
		}
	}

	fields := make(map[string]metaAggField, len(props))
	var unaggregatable []string
	for key, spec := range props {
		switch {
		case spec.Type == "text":
			// text is analysed and not aggregatable; its keyword subfield is.
			if sub, ok := spec.Fields["keyword"]; ok && sub.Type == "keyword" {
				fields[key] = metaAggField{path: fmt.Sprintf("meta_fields.%s.keyword", key), typ: spec.Type}
				continue
			}
			unaggregatable = append(unaggregatable, key)
		case isAggregatableType(spec.Type):
			fields[key] = metaAggField{path: fmt.Sprintf("meta_fields.%s", key), typ: spec.Type}
		default:
			unaggregatable = append(unaggregatable, key)
		}
	}
	sort.Strings(unaggregatable)
	return fields, unaggregatable, nil
}

func isAggregatableType(typ string) bool {
	switch typ {
	case "keyword", "long", "integer", "short", "byte", "double", "float", "boolean", "date":
		return true
	}
	return false
}

// formatMetaValue renders a bucket key the way a filter value for that field is
// written.
//
// A composite terms source over a date field keys its buckets by epoch
// milliseconds and, unlike date_histogram, takes no format option to change
// that. Left as they arrive, the value space offers the filter generator
// 1784851200000 where the document holds 2026-07-23, so the model cannot relate
// a date in the question to anything it is shown and every condition it writes
// against a date field is meaningless.
//
// Midnight UTC renders as a plain date, which is what such a field holds in
// practice and what the filter translators recognise as a date; a value carrying
// a time of day keeps it rather than being rounded away.
func formatMetaValue(raw json.RawMessage, esType string) string {
	if len(raw) == 0 {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return string(raw)
	}

	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		if esType == "date" {
			if millis, err := v.Int64(); err == nil {
				return formatEpochMillis(millis)
			}
		}
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatEpochMillis(millis int64) string {
	moment := time.UnixMilli(millis).UTC()
	if moment.Hour() == 0 && moment.Minute() == 0 && moment.Second() == 0 && moment.Nanosecond() == 0 {
		return moment.Format("2006-01-02")
	}
	if moment.Nanosecond() == 0 {
		return moment.Format("2006-01-02T15:04:05Z")
	}
	return moment.Format("2006-01-02T15:04:05.000000Z")
}

// sortedKeys keeps the request body stable across rounds and processes.
func sortedKeys(fields map[string]metaAggField) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
