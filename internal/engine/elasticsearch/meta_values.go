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
	"strconv"
	"strings"

	"ragflow/internal/engine/types"
)

// metaValuesScript emits one token per distinct metadata value of a doc-meta
// row, read from _source rather than from doc values.
//
// Doc values are what the index made of a value, not the value: a float is
// stored as float32, a date-looking string as epoch milliseconds in UTC, and a
// string longer than ignore_above not at all. A value read back from them does
// not match the same value read from _source, which is what the metadata scans
// read and what every metadata filter compares against. _source gives the
// aggregation the stored JSON itself.
//
// Tokens, all strings so one composite source can carry them:
//
//	"k<key>\x00s<string>"  a string value
//	"k<key>\x00b<true|false>"  a boolean value
//	"k<key>\x00n<double>"  a number, as Java's Double.toString, which parses
//	                       back to the same float64 encoding/json decodes the
//	                       stored literal to
//	"c"                    the row carries a non-null value under some key
//	"u"                    meta_fields is not an object, so the script cannot
//	                       read it; the caller must not trust the answer
//
// A list contributes its elements. Nested objects and nested lists have no
// token -- the scans render them as Go's %v of a decoded map or slice, which
// is not a value anyone filters on -- but they still count as carrying
// metadata. Keys listed in params.skip are ignored entirely.
const metaValuesScript = `
String sep = String.valueOf((char) 0);
def m = params._source.meta_fields;
List out = new ArrayList();
if (m == null) { return out; }
if (!(m instanceof Map)) { out.add('u'); return out; }
boolean carrying = false;
for (def e : m.entrySet()) {
  if (params.skip.contains(e.getKey())) { continue; }
  def v = e.getValue();
  if (v == null) { continue; }
  List items = v instanceof List ? v : [v];
  for (def item : items) {
    if (item == null) { continue; }
    carrying = true;
    String p = 'k' + e.getKey() + sep;
    if (item instanceof String) { out.add(p + 's' + item); }
    else if (item instanceof Boolean) { out.add(p + 'b' + item); }
    else if (item instanceof Number) { out.add(p + 'n' + Double.toString(((Number) item).doubleValue())); }
  }
}
if (carrying) { out.add('c'); }
return out;
`

// metaValuesPageSize is how many distinct values one composite page carries;
// it stays well below search.max_buckets.
const metaValuesPageSize = 10000

// metaValuesAgg names the composite aggregation in the request and response.
const metaValuesAgg = "meta_values"

type metaValuesResponse struct {
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Failed int `json:"failed"`
	} `json:"_shards"`
	Aggregations map[string]struct {
		AfterKey map[string]string `json:"after_key"`
		Buckets  []struct {
			Key      map[string]string `json:"key"`
			DocCount int64             `json:"doc_count"`
		} `json:"buckets"`
	} `json:"aggregations"`
}

// metaValueVisitor receives one distinct metadata value and how many of the
// matched rows hold it. value is a string, a float64 or a bool: what
// encoding/json makes of the stored value.
type metaValueVisitor func(key string, value interface{}, docs int64)

// walkMetaValues aggregates the metadata values of the doc-meta rows query
// matches, calling visit once per distinct (key, value), and returns how many
// of the rows carry any metadata value.
//
// One composite aggregation carries every key, paged from its after_key until
// exhausted, so the page size bounds a round trip rather than the answer.
//
// A partial answer is refused with types.ErrMetaValueSpaceIncomplete: a value
// missing because its shard failed or the search timed out is
// indistinguishable from a value that does not exist.
func (e *Engine) walkMetaValues(ctx context.Context, indexName string, query map[string]interface{}, skipKeys []string, visit metaValueVisitor) (int64, error) {
	if skipKeys == nil {
		skipKeys = []string{}
	}
	var carrying int64
	var after map[string]string
	for {
		composite := map[string]interface{}{
			"size": metaValuesPageSize,
			"sources": []interface{}{map[string]interface{}{"v": map[string]interface{}{"terms": map[string]interface{}{
				"script": map[string]interface{}{
					"lang":   "painless",
					"source": metaValuesScript,
					"params": map[string]interface{}{"skip": skipKeys},
				},
			}}}},
		}
		if after != nil {
			composite["after"] = after
		}
		resp, err := e.searchMetaValues(ctx, indexName, map[string]interface{}{
			"size":  0,
			"query": query,
			"aggs":  map[string]interface{}{metaValuesAgg: map[string]interface{}{"composite": composite}},
		})
		if err != nil {
			return 0, err
		}
		if resp.TimedOut || resp.Shards.Failed > 0 {
			return 0, fmt.Errorf("%w: timed_out=%v, failed shards=%d", types.ErrMetaValueSpaceIncomplete, resp.TimedOut, resp.Shards.Failed)
		}
		agg, ok := resp.Aggregations[metaValuesAgg]
		if !ok {
			return 0, fmt.Errorf("%w: the response carries no %s aggregation", types.ErrMetaValueSpaceIncomplete, metaValuesAgg)
		}
		for _, bucket := range agg.Buckets {
			token := bucket.Key["v"]
			switch {
			case token == "c":
				carrying += bucket.DocCount
			case token == "u":
				return 0, fmt.Errorf("%w: meta_fields is stored as something other than an object", types.ErrMetaValueSpaceIncomplete)
			case strings.HasPrefix(token, "k"):
				key, value, err := decodeMetaValueToken(token[1:])
				if err != nil {
					return 0, err
				}
				visit(key, value, bucket.DocCount)
			default:
				return 0, fmt.Errorf("unexpected metadata value token %q", token)
			}
		}
		if len(agg.Buckets) < metaValuesPageSize || len(agg.AfterKey) == 0 {
			return carrying, nil
		}
		after = agg.AfterKey
	}
}

// decodeMetaValueToken splits "<key>\x00<tag><value>" into the key and the
// value, typed the way encoding/json decodes it.
func decodeMetaValueToken(token string) (string, interface{}, error) {
	key, rest, ok := strings.Cut(token, "\x00")
	if !ok || rest == "" {
		return "", nil, fmt.Errorf("malformed metadata value token %q", token)
	}
	raw := rest[1:]
	switch rest[0] {
	case 's':
		return key, raw, nil
	case 'b':
		return key, raw == "true", nil
	case 'n':
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", nil, fmt.Errorf("malformed number in metadata value token %q: %w", token, err)
		}
		return key, number, nil
	}
	return "", nil, fmt.Errorf("malformed metadata value token %q", token)
}

func (e *Engine) searchMetaValues(ctx context.Context, indexName string, body map[string]interface{}) (*metaValuesResponse, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("error encoding metadata values query: %w", err)
	}

	// allow_partial_search_results=false: the default lets ES answer HTTP 200
	// with some shards missing.
	res, err := e.client.Search(
		e.client.Search.WithContext(ctx),
		e.client.Search.WithIndex(indexName),
		e.client.Search.WithBody(&buf),
		e.client.Search.WithAllowPartialSearchResults(false),
	)
	if err != nil {
		return nil, fmt.Errorf("metadata values aggregation failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("metadata values aggregation returned %s: %s", res.Status(), string(bodyBytes))
	}

	var parsed metaValuesResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse metadata values response: %w", err)
	}
	return &parsed, nil
}

// MetaValueSpace returns every distinct metadata value per key for the given
// knowledge bases: {key: [value, ...]}, each value a string, a float64 or a
// bool as encoding/json decodes the stored value.
//
// Deliberately not SearchMetadata: that reads the doc-meta index with from/size
// and so stops at the doc store's result window, which hands the filter
// generator the metadata of an arbitrary prefix. An aggregation is not bound by
// that window.
//
// Returns types.ErrMetaValueSpaceIncomplete when the cluster answers with
// partial results.
func (e *Engine) MetaValueSpace(ctx context.Context, tenantID string, kbIDs []string) (map[string][]interface{}, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID cannot be empty")
	}
	space := map[string][]interface{}{}
	if len(kbIDs) == 0 {
		return space, nil
	}

	indexName := buildMetadataIndexName(tenantID)
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to check metadata index existence: %w", err)
	}
	if !exists {
		return space, nil
	}

	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"filter": []interface{}{
				map[string]interface{}{"terms": map[string]interface{}{"kb_id": kbIDs}},
			},
		},
	}
	_, err = e.walkMetaValues(ctx, indexName, query, nil, func(key string, value interface{}, _ int64) {
		space[key] = append(space[key], value)
	})
	if err != nil {
		return nil, err
	}
	return space, nil
}
