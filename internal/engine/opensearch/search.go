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

package opensearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"go.uber.org/zap"
)

// SearchResponse models the JSON payload returned by OpenSearch /_search endpoint.
type SearchResponse struct {
	Took     int64 `json:"took"`
	TimedOut bool  `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []struct {
			ID        string                 `json:"_id"`
			Index     string                 `json:"_index"`
			Score     float64                `json:"_score"`
			Source    map[string]interface{} `json:"_source"`
			Fields    map[string]interface{} `json:"fields"`
			Highlight map[string]interface{} `json:"highlight,omitempty"`
			Sort      []interface{}          `json:"sort,omitempty"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

// checkSearchResponse verifies that OpenSearch executed completely without shard failures or timeouts.
func checkSearchResponse(osResp *SearchResponse, indexName string) error {
	if osResp == nil {
		return nil
	}
	if osResp.TimedOut {
		return fmt.Errorf("opensearch search timed out on index %s", indexName)
	}
	if osResp.Shards.Failed > 0 {
		return fmt.Errorf("opensearch search encountered %d failed shards on index %s", osResp.Shards.Failed, indexName)
	}
	return nil
}

// convertOSResponse maps OpenSearch hits into RAGFlow chunk maps.
// It guarantees that:
//  1. chunk["_index"] = hit.Index is backfilled for second-stage KNNScores routing.
//  2. chunk["_score"] = hit.Score retains full float64 precision.
//  3. chunk["id"] and chunk["_id"] both reflect the document ID.
//  4. chunk["highlight"] captures matching highlight fragments.
func convertOSResponse(osResp *SearchResponse) []map[string]interface{} {
	if osResp == nil || osResp.Hits.Hits == nil {
		return []map[string]interface{}{}
	}

	chunks := make([]map[string]interface{}, len(osResp.Hits.Hits))
	for i, hit := range osResp.Hits.Hits {
		chunks[i] = hit.Source
		if chunks[i] == nil {
			chunks[i] = make(map[string]interface{})
		}
		chunks[i]["_score"] = hit.Score
		chunks[i]["_id"] = hit.ID
		chunks[i]["id"] = hit.ID
		chunks[i]["_index"] = hit.Index
		if len(hit.Highlight) > 0 {
			chunks[i]["highlight"] = hit.Highlight
		}
		for k, v := range hit.Fields {
			if _, exists := chunks[i][k]; !exists {
				chunks[i][k] = v
			}
		}
	}
	return chunks
}

func isIndexNotFoundResponse(bodyBytes []byte) bool {
	var failure struct {
		Error struct {
			Type      string `json:"type"`
			RootCause []struct {
				Type string `json:"type"`
			} `json:"root_cause"`
		} `json:"error"`
	}
	if json.Unmarshal(bodyBytes, &failure) == nil {
		if failure.Error.Type == "index_not_found_exception" {
			return true
		}
		for _, rc := range failure.Error.RootCause {
			if rc.Type == "index_not_found_exception" {
				return true
			}
		}
	}
	return false
}

// searchOneIndex sends one search query against an index or comma-separated indices.
// 404 Not Found is treated as an empty result set (matching Elasticsearch parity).
func (e *Engine) searchOneIndex(ctx context.Context, indexName string, payload []byte) ([]map[string]interface{}, int64, error) {
	endpoint := fmt.Sprintf("/%s/_search?ignore_unavailable=true&allow_partial_search_results=false", indexName)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.transportClient.Stream(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("opensearch search request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if isIndexNotFoundResponse(bodyBytes) {
			common.Debug("OpenSearch index absent during search, returning empty hits", zap.String("index", indexName))
			return []map[string]interface{}{}, 0, nil
		}
		return nil, 0, fmt.Errorf("opensearch search error status 404: %s", string(bodyBytes))
	}

	if resp.StatusCode >= 400 {
		rawBody, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("opensearch search error status %d: %s", resp.StatusCode, string(rawBody))
	}

	var osResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&osResp); err != nil {
		return nil, 0, fmt.Errorf("failed to decode search response: %w", err)
	}
	if err := checkSearchResponse(&osResp, indexName); err != nil {
		return nil, 0, err
	}

	return convertOSResponse(&osResp), osResp.Hits.Total.Value, nil
}

// Search executes a unified search across the requested indices.
func (e *Engine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if req == nil {
		return nil, fmt.Errorf("search request cannot be nil")
	}
	if e == nil || e.transportClient == nil {
		return nil, fmt.Errorf("opensearch engine not initialized")
	}
	types.LogSearchRequest("OpenSearch", req)

	queryBody, useSearchAfter, err := buildSearchDSL(ctx, req)
	if err != nil {
		return nil, err
	}

	isMemoryIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "memory_") {
			isMemoryIndex = true
			break
		}
	}

	offset := max(req.Offset, 0)
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}

	var (
		totalHits  int64
		allResults []map[string]interface{}
	)

	if useSearchAfter {
		allResults, totalHits, err = e.searchAfterCursor(ctx, req, queryBody, offset, limit)
		if err != nil {
			return nil, err
		}
	} else {
		payload, encErr := json.Marshal(queryBody)
		if encErr != nil {
			return nil, fmt.Errorf("error encoding query: %w", encErr)
		}

		target := strings.Join(req.IndexNames, ",")
		var osErr error
		allResults, totalHits, osErr = e.searchOneIndex(ctx, target, payload)
		if osErr != nil {
			return nil, fmt.Errorf("opensearch: search indices %q: %w", target, osErr)
		}
	}

	if isMemoryIndex {
		normalizeMemoryMessageChunks(allResults)
	}

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}

type searchAfterFetcher func(
	ctx context.Context,
	baseQuery map[string]interface{},
	batch int,
	cursor []interface{},
	trackTotalHits bool,
) (SearchResponse, error)

func (e *Engine) searchAfterCursor(
	ctx context.Context,
	req *types.SearchRequest,
	baseQuery map[string]interface{},
	offset, limit int,
) ([]map[string]interface{}, int64, error) {
	delete(baseQuery, "from")
	delete(baseQuery, "size")

	batchCap := common.SearchAfterBatchSize

	fetcher := func(ctx context.Context, baseQuery map[string]interface{}, batch int, cursor []interface{}, trackTotalHits bool) (SearchResponse, error) {
		return e.executeSearchRequest(ctx, req, baseQuery, batch, cursor, trackTotalHits)
	}

	return searchAfterPaginate(ctx, baseQuery, offset, limit, batchCap, fetcher)
}

func (e *Engine) executeSearchRequest(
	ctx context.Context,
	req *types.SearchRequest,
	baseQuery map[string]interface{},
	batch int,
	cursor []interface{},
	trackTotalHits bool,
) (SearchResponse, error) {
	reqBody := make(map[string]interface{}, len(baseQuery)+3)
	for k, v := range baseQuery {
		reqBody[k] = v
	}
	reqBody["size"] = batch
	if len(cursor) > 0 {
		reqBody["search_after"] = cursor
	}
	if trackTotalHits {
		reqBody["track_total_hits"] = true
	} else {
		delete(reqBody, "track_total_hits")
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("failed to marshal search_after body: %w", err)
	}

	indexTarget := strings.Join(req.IndexNames, ",")
	endpoint := fmt.Sprintf("/%s/_search?ignore_unavailable=true&allow_partial_search_results=false", indexTarget)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return SearchResponse{}, fmt.Errorf("failed to create search_after request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.transportClient.Stream(httpReq)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("search_after request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if isIndexNotFoundResponse(bodyBytes) {
			return SearchResponse{}, nil
		}
		return SearchResponse{}, fmt.Errorf("search_after returned error status 404: %s", string(bodyBytes))
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return SearchResponse{}, fmt.Errorf("search_after returned error status %d: %s", resp.StatusCode, string(body))
	}

	var osResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&osResp); err != nil {
		return SearchResponse{}, fmt.Errorf("failed to decode search_after response: %w", err)
	}
	if err := checkSearchResponse(&osResp, indexTarget); err != nil {
		return SearchResponse{}, err
	}

	return osResp, nil
}

func searchAfterPaginate(
	ctx context.Context,
	baseQuery map[string]interface{},
	offset, limit, batchCap int,
	fetch searchAfterFetcher,
) ([]map[string]interface{}, int64, error) {
	if batchCap <= 0 {
		batchCap = common.SearchAfterBatchSize
	}
	var (
		cursor        []interface{}
		totalHits     int64
		collected     []map[string]interface{}
		collectedTake int
		firstCall     = true
	)

	// Skip phase
	remainingSkip := offset
	for remainingSkip > 0 {
		batch := min(remainingSkip, batchCap)

		resp, err := fetch(ctx, baseQuery, batch, cursor, firstCall)
		firstCall = false
		if err != nil {
			return nil, 0, err
		}
		if totalHits == 0 {
			totalHits = resp.Hits.Total.Value
		}
		if len(resp.Hits.Hits) == 0 {
			break
		}
		nextCursor := resp.Hits.Hits[len(resp.Hits.Hits)-1].Sort
		if len(nextCursor) == 0 || sortValuesEqual(nextCursor, cursor) {
			break
		}
		cursor = nextCursor
		remainingSkip -= len(resp.Hits.Hits)
		if len(resp.Hits.Hits) < batch {
			break
		}
	}

	// Take phase
	for collectedTake < limit {
		want := limit - collectedTake
		batch := min(want, batchCap)

		resp, err := fetch(ctx, baseQuery, batch, cursor, firstCall)
		firstCall = false
		if err != nil {
			return nil, 0, err
		}
		if totalHits == 0 {
			totalHits = resp.Hits.Total.Value
		}
		if len(resp.Hits.Hits) == 0 {
			break
		}

		for _, hit := range resp.Hits.Hits {
			if collectedTake >= limit {
				break
			}
			chunk := hit.Source
			if chunk == nil {
				chunk = map[string]interface{}{}
			}
			chunk["_score"] = hit.Score
			chunk["_id"] = hit.ID
			chunk["id"] = hit.ID
			chunk["_index"] = hit.Index
			if len(hit.Highlight) > 0 {
				chunk["highlight"] = hit.Highlight
			}
			for k, v := range hit.Fields {
				if _, exists := chunk[k]; !exists {
					chunk[k] = v
				}
			}
			collected = append(collected, chunk)
			collectedTake++
		}

		if collectedTake >= limit {
			break
		}

		nextCursor := resp.Hits.Hits[len(resp.Hits.Hits)-1].Sort
		if len(nextCursor) == 0 || sortValuesEqual(nextCursor, cursor) {
			break
		}
		cursor = nextCursor
		if len(resp.Hits.Hits) < batch {
			break
		}
	}

	if firstCall {
		resp, err := fetch(ctx, baseQuery, 0, nil, true)
		if err != nil {
			return nil, 0, err
		}
		totalHits = resp.Hits.Total.Value
	}

	return collected, totalHits, nil
}

func sortValuesEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

// KNNScores performs a second-pass KNN vector search to calculate clean cosine similarities.
func (e *Engine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	if len(chunks) == 0 || len(queryVector) == 0 {
		return nil, nil
	}

	chunkIDs := make([]string, 0, len(chunks))
	indexNames := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := opensearchChunkID(chunk); ok && id != "" {
			indexName, ok := chunk["_index"].(string)
			if !ok || indexName == "" {
				return nil, fmt.Errorf("KNN candidate %s has no index", id)
			}
			chunkIDs = append(chunkIDs, id)
			indexNames = append(indexNames, indexName)
		}
	}
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	chunkIDs = common.Deduplicate(chunkIDs)
	indexNames = common.Deduplicate(indexNames)

	common.Info("KNNScores starting in OpenSearch", zap.Int("chunkCount", len(chunkIDs)), zap.Int("indexCount", len(indexNames)), zap.Int("vectorSize", len(queryVector)))

	vectorSize := len(queryVector)
	k := len(chunkIDs)
	knnParams := map[string]interface{}{
		"vector": queryVector,
		"k":      k,
		"filter": map[string]interface{}{
			"terms": map[string]interface{}{"id": chunkIDs},
		},
	}

	queryBody := map[string]interface{}{
		"query": map[string]interface{}{
			"knn": map[string]interface{}{
				fmt.Sprintf("q_%d_vec", vectorSize): knnParams,
			},
		},
		"size":    k,
		"_source": false,
	}

	body, err := json.Marshal(queryBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal KNN query: %w", err)
	}

	endpoint := fmt.Sprintf("/%s/_search", strings.Join(indexNames, ","))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create KNNScores request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.transportClient.Stream(httpReq)
	if err != nil {
		return nil, fmt.Errorf("KNNScores search failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("KNNScores search returned error status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var osResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&osResp); err != nil {
		return nil, fmt.Errorf("failed to parse KNNScores response: %w", err)
	}
	if err := checkSearchResponse(&osResp, strings.Join(indexNames, ",")); err != nil {
		return nil, err
	}

	knnHits := make([]map[string]interface{}, len(osResp.Hits.Hits))
	for i, hit := range osResp.Hits.Hits {
		knnHits[i] = map[string]interface{}{
			"_id":    hit.ID,
			"_score": hit.Score,
			"_index": hit.Index,
		}
	}
	return map[string]interface{}{
		"hits": map[string]interface{}{
			"hits": knnHits,
		},
	}, nil
}

// GetScores extracts similarity scores from KNN search result.
// OpenSearch 2.19 cosine similarity scores are clamped to [0, 1], matching Elasticsearch.
func (e *Engine) GetScores(knnResult map[string]interface{}) map[string]float64 {
	scores := make(map[string]float64)
	if knnResult == nil {
		return scores
	}
	hits, ok := knnResult["hits"].(map[string]interface{})
	if !ok {
		return scores
	}
	hitsList, ok := hits["hits"]
	if !ok {
		return scores
	}

	recordScore := func(docID string, score float64) {
		if math.IsNaN(score) {
			scores[docID] = 0.0
			return
		}
		if score < 0.0 {
			score = 0.0
		} else if score > 1.0 {
			score = 1.0
		}
		scores[docID] = score
	}

	switch v := hitsList.(type) {
	case []interface{}:
		for _, h := range v {
			if hit, ok := h.(map[string]interface{}); ok {
				if docID, ok := hit["_id"].(string); ok && docID != "" {
					if scoreVal := hit["_score"]; scoreVal != nil {
						if score, ok := toFloat64(scoreVal); ok {
							recordScore(docID, score)
						}
					}
				}
			}
		}
	case []map[string]interface{}:
		for _, hit := range v {
			if docID, ok := hit["_id"].(string); ok && docID != "" {
				if scoreVal := hit["_score"]; scoreVal != nil {
					if score, ok := toFloat64(scoreVal); ok {
						recordScore(docID, score)
					}
				}
			}
		}
	}

	return scores
}

// GetFields extracts the requested fields from OpenSearch search response chunks.
func (e *Engine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	if len(fields) == 0 || len(chunks) == 0 {
		return result
	}

	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[f] = true
	}

	for _, chunk := range chunks {
		docID, ok := opensearchChunkID(chunk)
		if !ok {
			continue
		}
		if id, ok := chunk["id"].(string); !ok || id == "" {
			chunk["id"] = docID
		}

		m := make(map[string]interface{})
		for field := range fieldSet {
			val := chunk[field]
			if val == nil {
				continue
			}

			if mapVal, ok := val.(map[string]interface{}); ok {
				if raw, err := json.Marshal(mapVal); err == nil {
					m[field] = string(raw)
					continue
				}
			}

			if listVal, ok := val.([]interface{}); ok {
				if len(listVal) == 1 {
					if _, isArray := listVal[0].([]interface{}); !isArray {
						val = listVal[0]
					}
				}
			}

			if _, ok = val.([]interface{}); ok {
				m[field] = val
				continue
			}

			if field == "available_int" {
				if _, ok = val.(int); ok {
					m[field] = val
					continue
				}
				if _, ok = val.(float64); ok {
					m[field] = val
					continue
				}
			}

			if _, ok = val.(string); !ok {
				val = fmt.Sprintf("%v", val)
			}
			m[field] = val
		}

		if len(m) > 0 {
			result[docID] = m
		}
	}

	return result
}

// GetAggregation aggregates chunk values by field name.
func (e *Engine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	if len(chunks) == 0 || fieldName == "" {
		return []map[string]interface{}{}
	}

	tagCounts := make(map[string]int)
	for _, chunk := range chunks {
		value, ok := chunk[fieldName]
		if !ok || value == nil {
			continue
		}

		if valueStr, ok := value.(string); ok {
			if valueStr == "" {
				continue
			}
			separator := ","
			if fieldName == "tag_kwd" && strings.Contains(valueStr, "###") {
				separator = "###"
			}
			for tag := range strings.SplitSeq(valueStr, separator) {
				countOpenSearchAggregationTag(tagCounts, tag)
			}
			continue
		}

		if valueList, ok := value.([]interface{}); ok {
			for _, item := range valueList {
				if itemStr, ok := item.(string); ok {
					countOpenSearchAggregationTag(tagCounts, itemStr)
				}
			}
		}
	}

	if len(tagCounts) == 0 {
		return []map[string]interface{}{}
	}

	tags := make([]string, 0, len(tagCounts))
	for tag := range tagCounts {
		tags = append(tags, tag)
	}
	slices.SortFunc(tags, func(a, b string) int {
		if byCount := cmp.Compare(tagCounts[b], tagCounts[a]); byCount != 0 {
			return byCount
		}
		return cmp.Compare(a, b)
	})

	result := make([]map[string]interface{}, len(tags))
	for i, tag := range tags {
		result[i] = map[string]interface{}{"key": tag, "count": tagCounts[tag]}
	}

	return result
}

// GetChunkIDs extracts chunk IDs from OpenSearch response chunks.
func (e *Engine) GetChunkIDs(chunks []map[string]interface{}) []string {
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := opensearchChunkID(chunk); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetHighlight returns highlighted text for matching keywords.
func (e *Engine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	result := make(map[string]string)
	pattern := compileOpenSearchHighlightPattern(keywords)

	for _, chunk := range chunks {
		docID, ok := opensearchChunkID(chunk)
		if !ok {
			continue
		}

		txt, ok := chunk[fieldName].(string)
		if !ok {
			continue
		}
		if pattern != nil {
			txt = pattern.ReplaceAllStringFunc(txt, func(match string) string {
				return "<em>" + match + "</em>"
			})
		}
		result[docID] = txt
	}
	return result
}

func opensearchChunkID(chunk map[string]interface{}) (string, bool) {
	if id, ok := chunk["id"].(string); ok && id != "" {
		return id, true
	}
	if id, ok := chunk["_id"].(string); ok && id != "" {
		return id, true
	}
	return "", false
}

func countOpenSearchAggregationTag(counts map[string]int, tag string) {
	if tag = strings.TrimSpace(tag); tag != "" {
		counts[tag]++
	}
}

func compileOpenSearchHighlightPattern(keywords []string) *regexp.Regexp {
	nonEmpty := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		if kw != "" {
			nonEmpty = append(nonEmpty, kw)
		}
	}
	if len(nonEmpty) == 0 {
		return nil
	}
	slices.SortStableFunc(nonEmpty, func(a, b string) int {
		return cmp.Compare(utf8.RuneCountInString(b), utf8.RuneCountInString(a))
	})
	parts := make([]string, len(nonEmpty))
	for i, keyword := range nonEmpty {
		parts[i] = regexp.QuoteMeta(keyword)
	}
	pattern := strings.Join(parts, "|")
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	return re
}
