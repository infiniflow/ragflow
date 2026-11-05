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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"github.com/opensearch-project/opensearch-go/v4"
)

func init() {
	_ = common.InitLogger("info", common.FileOutput{}, "opensearch_test")
}

func newMockSearchEngine(handler http.HandlerFunc, version string) (*Engine, *httptest.Server, error) {
	server := httptest.NewServer(handler)
	cfg := opensearch.Config{
		Addresses: []string{server.URL},
	}
	client, err := opensearch.NewClient(cfg)
	if err != nil {
		server.Close()
		return nil, nil, err
	}
	pm, err := newPipelineManager(version)
	if err != nil {
		server.Close()
		return nil, nil, err
	}
	engine := &Engine{
		transportClient: client,
		pipelineMgr:     pm,
	}
	return engine, server, nil
}

// TestConvertOSResponse_IndexAndScoreBackfill verifies _index, _score, and id are correctly backfilled.
func TestConvertOSResponse_IndexAndScoreBackfill(t *testing.T) {
	var resp SearchResponse
	resp.Hits.Total.Value = 1
	resp.Hits.Hits = append(resp.Hits.Hits, struct {
		ID        string                 `json:"_id"`
		Index     string                 `json:"_index"`
		Score     float64                `json:"_score"`
		Source    map[string]interface{} `json:"_source"`
		Fields    map[string]interface{} `json:"fields"`
		Highlight map[string]interface{} `json:"highlight,omitempty"`
		Sort      []interface{}          `json:"sort,omitempty"`
	}{
		ID:    "chunk-001",
		Index: "ragflow_tenant_idx",
		Score: 0.987654321,
		Source: map[string]interface{}{
			"content": "sample content",
		},
		Fields: map[string]interface{}{
			"q_1024_vec": []interface{}{0.1, 0.2},
		},
		Highlight: map[string]interface{}{
			"content": []interface{}{"sample <em>content</em>"},
		},
	})

	chunks := convertOSResponse(&resp)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	chunk := chunks[0]

	if chunk["_index"] != "ragflow_tenant_idx" {
		t.Errorf("expected _index 'ragflow_tenant_idx', got %v", chunk["_index"])
	}
	if chunk["_id"] != "chunk-001" || chunk["id"] != "chunk-001" {
		t.Errorf("expected id and _id 'chunk-001', got id=%v, _id=%v", chunk["id"], chunk["_id"])
	}
	if chunk["_score"] != 0.987654321 {
		t.Errorf("expected _score 0.987654321, got %v", chunk["_score"])
	}
	if chunk["content"] != "sample content" {
		t.Errorf("expected content 'sample content', got %v", chunk["content"])
	}
	if chunk["highlight"] == nil {
		t.Errorf("expected highlight to be populated")
	}
	if chunk["q_1024_vec"] == nil {
		t.Errorf("expected fields to be merged into chunk")
	}
}

// TestSearch_Execution_404Tolerated ensures 404 missing index is treated as empty result without error,
// while non-index 404 errors (such as missing pipeline) are preserved and returned.
func TestSearch_Execution_404Tolerated(t *testing.T) {
	// 1. index_not_found_exception -> swallowed as empty result
	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index [non_existent_idx]"},"status":404}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create test engine: %v", err)
	}
	defer server.Close()

	req := &types.SearchRequest{
		IndexNames: []string{"non_existent_idx"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "hello"},
		},
	}
	res, err := engine.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("expected 404 index_not_found_exception to be swallowed as empty result, got error: %v", err)
	}
	if len(res.Chunks) != 0 || res.Total != 0 {
		t.Errorf("expected empty chunks and 0 total, got chunks=%d, total=%d", len(res.Chunks), res.Total)
	}

	// 2. resource_not_found_exception (e.g. missing pipeline) -> returns error
	engineErr, serverErr, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"resource_not_found_exception","reason":"pipeline [missing_pipeline] does not exist"},"status":404}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create test engine: %v", err)
	}
	defer serverErr.Close()

	_, err = engineErr.Search(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error on non-index 404, got nil")
	}
}

// TestSearchMissingIndexRemainsEmpty tests both isolated and mixed index queries.
func TestSearchMissingIndexRemainsEmpty(t *testing.T) {
	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "present_idx") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"hits": {
					"total": {"value": 1},
					"hits": [{"_id": "c1", "_index": "present_idx", "_score": 1.0, "_source": {"text": "found"}}]
				}
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create test engine: %v", err)
	}
	defer server.Close()

	// Mixed search: 1 present index + 1 missing index
	req := &types.SearchRequest{
		IndexNames: []string{"present_idx", "missing_idx"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "found"},
		},
	}
	res, err := engine.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("Search failed on mixed indices: %v", err)
	}
	if len(res.Chunks) != 1 || res.Total != 1 {
		t.Fatalf("expected 1 chunk and total 1, got chunks=%d, total=%d", len(res.Chunks), res.Total)
	}
	if res.Chunks[0]["_id"] != "c1" {
		t.Errorf("expected chunk c1, got %v", res.Chunks[0]["_id"])
	}
}

// TestSearch_DeepPagination_SearchAfter tests the search_after pagination loop.
func TestSearch_DeepPagination_SearchAfter(t *testing.T) {
	makeResponse := func(count int, startID int, total int64) SearchResponse {
		var resp SearchResponse
		resp.Hits.Total.Value = total
		for i := 0; i < count; i++ {
			id := fmt.Sprintf("doc-%d", startID+i)
			resp.Hits.Hits = append(resp.Hits.Hits, struct {
				ID        string                 `json:"_id"`
				Index     string                 `json:"_index"`
				Score     float64                `json:"_score"`
				Source    map[string]interface{} `json:"_source"`
				Fields    map[string]interface{} `json:"fields"`
				Highlight map[string]interface{} `json:"highlight,omitempty"`
				Sort      []interface{}          `json:"sort,omitempty"`
			}{
				ID:    id,
				Index: "idx_test",
				Score: 1.0,
				Source: map[string]interface{}{
					"title": id,
				},
				Sort: []interface{}{1.0, id},
			})
		}
		return resp
	}

	callCount := 0
	fetcher := func(ctx context.Context, baseQuery map[string]interface{}, batch int, cursor []interface{}, trackTotalHits bool) (SearchResponse, error) {
		callCount++
		startID := 0
		if len(cursor) > 1 {
			var prevNum int
			_, _ = fmt.Sscanf(cursor[1].(string), "doc-%d", &prevNum)
			startID = prevNum + 1
		}
		return makeResponse(batch, startID, 100), nil
	}

	chunks, total, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, 5, 10, common.SearchAfterBatchSize, fetcher)
	if err != nil {
		t.Fatalf("searchAfterPaginate failed: %v", err)
	}
	if total != 100 {
		t.Errorf("expected total 100, got %d", total)
	}
	if len(chunks) != 10 {
		t.Fatalf("expected 10 collected chunks, got %d", len(chunks))
	}
	if chunks[0]["id"] != "doc-5" {
		t.Errorf("expected first chunk to be doc-5, got %v", chunks[0]["id"])
	}
	if chunks[9]["id"] != "doc-14" {
		t.Errorf("expected last chunk to be doc-14, got %v", chunks[9]["id"])
	}

	// Verify small max_result_window batch capping (e.g. window=50, offset=60, limit=10)
	var maxBatchObserved int
	var batches []int
	fetcherSmall := func(ctx context.Context, baseQuery map[string]interface{}, batch int, cursor []interface{}, trackTotalHits bool) (SearchResponse, error) {
		batches = append(batches, batch)
		if batch > maxBatchObserved {
			maxBatchObserved = batch
		}
		return makeResponse(batch, 0, 200), nil
	}
	_, _, err = searchAfterPaginate(context.Background(), map[string]interface{}{}, 60, 10, 50, fetcherSmall)
	if err != nil {
		t.Fatalf("searchAfterPaginate with small window failed: %v", err)
	}
	if maxBatchObserved > 50 {
		t.Fatalf("expected all batches <= 50, got maxBatch=%d (batches: %v)", maxBatchObserved, batches)
	}
	// offset 60 should be split into batches of 50 and 10 in skip phase, then 10 in take phase
	if len(batches) != 3 || batches[0] != 50 || batches[1] != 10 || batches[2] != 10 {
		t.Errorf("expected batches [50, 10, 10], got %v", batches)
	}
}

// TestKNNScores_RoutingAndQueryConstruction verifies second-pass KNNScores logic.
func TestKNNScores_RoutingAndQueryConstruction(t *testing.T) {
	var requestedPath string
	var requestedBody map[string]interface{}

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &requestedBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"hits": {
				"total": {"value": 2},
				"hits": [
					{"_id": "c1", "_index": "idx_1", "_score": 0.95},
					{"_id": "c2", "_index": "idx_2", "_score": 0.85}
				]
			}
		}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create mock engine: %v", err)
	}
	defer server.Close()

	chunks := []map[string]interface{}{
		{"id": "c1", "_id": "c1", "_index": "idx_1"},
		{"id": "c2", "_id": "c2", "_index": "idx_2"},
	}

	res, err := engine.KNNScores(context.Background(), chunks, []float64{0.1, 0.2, 0.3}, 2)
	if err != nil {
		t.Fatalf("KNNScores failed: %v", err)
	}

	if requestedPath != "/idx_1,idx_2/_search" && requestedPath != "/idx_2,idx_1/_search" {
		t.Errorf("expected search across candidate indices, got path %q", requestedPath)
	}

	query, ok := requestedBody["query"].(map[string]interface{})
	if !ok || query["knn"] == nil {
		t.Fatalf("expected query.knn block in request body, got: %v", requestedBody)
	}

	scores := engine.GetScores(res)
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	if math.Abs(scores["c1"]-0.95) > 1e-4 {
		t.Errorf("expected score 0.95 for c1, got %f", scores["c1"])
	}
	if math.Abs(scores["c2"]-0.85) > 1e-4 {
		t.Errorf("expected score 0.85 for c2, got %f", scores["c2"])
	}

	// Missing _index error test
	badChunks := []map[string]interface{}{
		{"id": "c3", "_id": "c3"},
	}
	_, err = engine.KNNScores(context.Background(), badChunks, []float64{0.1}, 1)
	if err == nil {
		t.Errorf("expected error when chunk lacks _index, got nil")
	}
}

// TestGetScores_ClampingAndExtraction verifies score extraction and [0, 1] clamping for OpenSearch 2.19.
func TestGetScores_ClampingAndExtraction(t *testing.T) {
	rawResult := map[string]interface{}{
		"hits": map[string]interface{}{
			"hits": []interface{}{
				map[string]interface{}{
					"_id":    "doc-1",
					"_score": 0.85,
				},
				map[string]interface{}{
					"_id":    "doc-2",
					"_score": 1.5,
				},
				map[string]interface{}{
					"_id":    "doc-3",
					"_score": -0.5,
				},
			},
		},
	}

	pm219, err := newPipelineManager("2.19.0")
	if err != nil {
		t.Fatalf("newPipelineManager failed: %v", err)
	}
	eng := &Engine{
		pipelineMgr: pm219,
	}
	scores := eng.GetScores(rawResult)
	if math.Abs(scores["doc-1"]-0.85) > 1e-6 {
		t.Errorf("expected 0.85 for doc-1, got %f", scores["doc-1"])
	}
	if math.Abs(scores["doc-2"]-1.0) > 1e-6 {
		t.Errorf("expected 1.0 clamped for doc-2, got %f", scores["doc-2"])
	}
	if math.Abs(scores["doc-3"]-0.0) > 1e-6 {
		t.Errorf("expected 0.0 clamped for doc-3, got %f", scores["doc-3"])
	}
}

// TestGetFields_FilteringAndFallback tests field extraction and serialization.
func TestGetFields_FilteringAndFallback(t *testing.T) {
	eng := &Engine{}
	chunks := []map[string]interface{}{
		{
			"_id":           "c1",
			"docnm_kwd":     "document.pdf",
			"available_int": 1,
			"tag_feas": map[string]interface{}{
				"keyword": 5,
			},
			"unrequested": "skip_me",
		},
		{
			"id":        "c2",
			"docnm_kwd": "notes.txt",
		},
	}

	fields := []string{"docnm_kwd", "available_int", "tag_feas"}
	result := eng.GetFields(chunks, fields)

	if len(result) != 2 {
		t.Fatalf("expected 2 chunk entries in result, got %d", len(result))
	}

	c1 := result["c1"]
	if c1["docnm_kwd"] != "document.pdf" {
		t.Errorf("expected 'document.pdf', got %v", c1["docnm_kwd"])
	}
	if c1["available_int"] != 1 {
		t.Errorf("expected available_int 1, got %v", c1["available_int"])
	}
	// Nested object should be marshaled as JSON string
	tagFeas, ok := c1["tag_feas"].(string)
	if !ok || !contains(tagFeas, `"keyword":5`) {
		t.Errorf("expected JSON string for tag_feas, got %v", c1["tag_feas"])
	}
	if _, ok := c1["unrequested"]; ok {
		t.Errorf("unrequested field should not be present")
	}

	c2 := result["c2"]
	if c2["docnm_kwd"] != "notes.txt" {
		t.Errorf("expected 'notes.txt', got %v", c2["docnm_kwd"])
	}
}

// TestGetAggregation_TagCounts tests tag frequency aggregation.
func TestGetAggregation_TagCounts(t *testing.T) {
	eng := &Engine{}
	chunks := []map[string]interface{}{
		{"docnm_kwd": "docA"},
		{"docnm_kwd": "docA"},
		{"docnm_kwd": "docB"},
		{"tag_kwd": "ai###search###ai"},
	}

	aggsDoc := eng.GetAggregation(chunks, "docnm_kwd")
	if len(aggsDoc) != 2 {
		t.Fatalf("expected 2 agg items, got %d", len(aggsDoc))
	}
	if aggsDoc[0]["key"] != "docA" || aggsDoc[0]["count"] != 2 {
		t.Errorf("expected docA with count 2, got %v", aggsDoc[0])
	}
	if aggsDoc[1]["key"] != "docB" || aggsDoc[1]["count"] != 1 {
		t.Errorf("expected docB with count 1, got %v", aggsDoc[1])
	}

	aggsTag := eng.GetAggregation(chunks, "tag_kwd")
	if len(aggsTag) != 2 {
		t.Fatalf("expected 2 tags (ai, search), got %d", len(aggsTag))
	}
	if aggsTag[0]["key"] != "ai" || aggsTag[0]["count"] != 2 {
		t.Errorf("expected ai with count 2, got %v", aggsTag[0])
	}
}

// TestGetHighlight_TagsAndCaseInsensitivity tests highlighting.
func TestGetHighlight_TagsAndCaseInsensitivity(t *testing.T) {
	eng := &Engine{}
	chunks := []map[string]interface{}{
		{
			"id":      "c1",
			"content": "Deep Learning and Neural Network systems",
		},
	}

	highlights := eng.GetHighlight(chunks, []string{"learning", "neural network"}, "content")
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight entry, got %d", len(highlights))
	}
	txt := highlights["c1"]
	if !contains(txt, "Deep <em>Learning</em> and <em>Neural Network</em> systems") {
		t.Errorf("expected wrapped highlight tags, got: %q", txt)
	}
}

// TestGetChunkIDs tests extracting IDs from chunks.
func TestGetChunkIDs(t *testing.T) {
	eng := &Engine{}
	chunks := []map[string]interface{}{
		{"id": "c1"},
		{"_id": "c2"},
		{"empty": "c3"},
	}
	ids := eng.GetChunkIDs(chunks)
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Errorf("expected ['c1', 'c2'], got %v", ids)
	}
}

func TestCheckSearchResponse_ShardsAndTimeout(t *testing.T) {
	// Normal response
	var okResp SearchResponse
	okResp.Shards.Total = 5
	okResp.Shards.Successful = 5
	if err := checkSearchResponse(&okResp, "test_idx"); err != nil {
		t.Fatalf("expected no error for successful search, got: %v", err)
	}

	// Timed out response
	var timeoutResp SearchResponse
	timeoutResp.TimedOut = true
	if err := checkSearchResponse(&timeoutResp, "test_idx"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}

	// Shard failure response
	var failResp SearchResponse
	failResp.Shards.Total = 5
	failResp.Shards.Successful = 4
	failResp.Shards.Failed = 1
	if err := checkSearchResponse(&failResp, "test_idx"); err == nil || !strings.Contains(err.Error(), "failed shards") {
		t.Fatalf("expected failed shards error, got: %v", err)
	}
}

// TestSearch_DeepPagination_SearchAfter_FullRetrieval verifies that when text searches
// exceed max_result_window (10,000), search_after cursor pagination is automatically used.
func TestSearch_DeepPagination_SearchAfter_FullRetrieval(t *testing.T) {
	settingsCalls := 0
	searchCalls := 0

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "_settings") {
			settingsCalls++
			w.WriteHeader(http.StatusOK)
			return
		}

		if strings.Contains(r.URL.Path, "_search") {
			searchCalls++
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)

			size := int(body["size"].(float64))
			startID := 0
			if cursor, ok := body["search_after"].([]interface{}); ok && len(cursor) > 1 {
				var prevNum int
				_, _ = fmt.Sscanf(cursor[1].(string), "doc-%d", &prevNum)
				startID = prevNum + 1
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			hits := make([]map[string]interface{}, size)
			for i := 0; i < size; i++ {
				id := fmt.Sprintf("doc-%d", startID+i)
				hits[i] = map[string]interface{}{
					"_id":    id,
					"_score": 1.0,
					"_source": map[string]interface{}{
						"content": "sample",
					},
					"sort": []interface{}{1.0, id},
				}
			}
			respBytes, _ := json.Marshal(map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": 20000},
					"hits":  hits,
				},
			})
			_, _ = w.Write(respBytes)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create mock engine: %v", err)
	}
	defer server.Close()

	req := &types.SearchRequest{
		IndexNames: []string{"test_deep_idx"},
		Offset:     10000,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "database"},
		},
	}

	res, err := engine.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("Search failed on deep pagination: %v", err)
	}
	if settingsCalls != 0 {
		t.Errorf("expected 0 settings queries, got %d", settingsCalls)
	}
	if searchCalls < 2 {
		t.Errorf("expected search_after pagination batches (got %d search calls)", searchCalls)
	}
	if len(res.Chunks) != 10 {
		t.Errorf("expected 10 chunks, got %d", len(res.Chunks))
	}
}

// TestSearch_VectorOutOfBounds_ImmediateError_NoNetworkRequests verifies that when a vector
// or hybrid search exceeds max_result_window (10,000), it fails immediately without sending any network requests.
func TestSearch_VectorOutOfBounds_ImmediateError_NoNetworkRequests(t *testing.T) {
	searchCalls := 0

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		searchCalls++
		w.WriteHeader(http.StatusOK)
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create mock engine: %v", err)
	}
	defer server.Close()

	// 1. Vector search with offset + limit > 10000 (e.g. 9995 + 10 = 10005)
	vecReq := &types.SearchRequest{
		IndexNames: []string{"test_vec_idx"},
		Offset:     9995,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{EmbeddingData: []float64{0.1, 0.2, 0.3}},
		},
	}
	_, vecErr := engine.Search(context.Background(), vecReq)
	if vecErr == nil {
		t.Fatalf("expected vector search beyond window to fail immediately, got nil")
	}
	if !strings.Contains(vecErr.Error(), "vector search does not support offset + limit 10005 beyond max_result_window (10000)") {
		t.Errorf("unexpected error: %v", vecErr)
	}
	if searchCalls != 0 {
		t.Errorf("expected 0 HTTP requests for out-of-bounds vector search, got %d", searchCalls)
	}

	// 2. Hybrid search with offset + limit > 10000 (e.g. 9995 + 10 = 10005)
	hybridReq := &types.SearchRequest{
		IndexNames: []string{"test_hybrid_idx"},
		Offset:     9995,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "database"},
			&types.MatchDenseExpr{EmbeddingData: []float64{0.1, 0.2, 0.3}},
		},
	}
	_, hybridErr := engine.Search(context.Background(), hybridReq)
	if hybridErr == nil {
		t.Fatalf("expected hybrid search beyond window to fail immediately, got nil")
	}
	if !strings.Contains(hybridErr.Error(), "vector search does not support offset + limit 10005 beyond max_result_window (10000)") {
		t.Errorf("unexpected error: %v", hybridErr)
	}
	if searchCalls != 0 {
		t.Errorf("expected 0 HTTP requests for out-of-bounds hybrid search, got %d", searchCalls)
	}
}
