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
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"ragflow/internal/engine/types"

	"github.com/opensearch-project/opensearch-go/v4"
)

// requestCaptureTransport records the last request and replies with a fixed
// response, so tests can assert on the exact body and query parameters the
// engine sends.
type requestCaptureTransport struct {
	status      int
	body        string
	lastURL     string
	lastRawBody string
	lastReq     map[string]interface{}
}

func (f *requestCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.lastURL = r.URL.String()
	f.lastRawBody = ""
	f.lastReq = nil
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			f.lastRawBody = string(raw)
			_ = json.Unmarshal(raw, &f.lastReq)
		}
	}
	return &http.Response{
		StatusCode: f.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

func newCaptureEngine(t *testing.T, transport *requestCaptureTransport) *Engine {
	t.Helper()
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses:    []string{"http://capture.test"},
		DisableRetry: true,
		Transport:    transport,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{client: client, hybridSearchEnabled: true}
}

// TestSearchRequestsExactTotalHits guards against OpenSearch's default cap on
// hits.total (10,000, reported as a lower bound): the ordinary search path must
// ask for an exact total.
func TestSearchRequestsExactTotalHits(t *testing.T) {
	transport := &requestCaptureTransport{
		status: http.StatusOK,
		body:   `{"took":1,"hits":{"total":{"value":5},"hits":[{"_id":"c1","_index":"ragflow_t","_score":1.0,"_source":{"id":"c1"}}]}}`,
	}
	eng := newCaptureEngine(t, transport)

	result, err := eng.Search(t.Context(), &types.SearchRequest{
		IndexNames: []string{"ragflow_t"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "hello", Fields: []string{"content_ltks"}, TopN: 10},
		},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got, ok := transport.lastReq["track_total_hits"].(bool); !ok || !got {
		t.Fatalf("track_total_hits = %#v, want true in request body %#v", transport.lastReq["track_total_hits"], transport.lastReq)
	}
	if result.Total != 5 {
		t.Fatalf("Total = %d, want 5", result.Total)
	}
}

// TestSearchAfterRequestsExactTotalHitsOnlyOnFirstPage verifies the pagination
// path forwards its trackTotalHits flag instead of ignoring it.
func TestSearchAfterRequestsExactTotalHitsOnlyOnFirstPage(t *testing.T) {
	for _, tc := range []struct {
		name            string
		trackTotalHits  bool
		wantTotalHitsIn bool
	}{
		{name: "first page", trackTotalHits: true, wantTotalHitsIn: true},
		{name: "later page", trackTotalHits: false, wantTotalHitsIn: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &requestCaptureTransport{
				status: http.StatusOK,
				body:   `{"hits":{"total":{"value":3},"hits":[]}}`,
			}
			eng := newCaptureEngine(t, transport)

			_, err := eng.executeSearchRequest(t.Context(), &types.SearchRequest{
				IndexNames: []string{"ragflow_t"},
			}, map[string]interface{}{"query": map[string]interface{}{"match_all": map[string]interface{}{}}}, 10, nil, tc.trackTotalHits)
			if err != nil {
				t.Fatalf("executeSearchRequest: %v", err)
			}
			_, present := transport.lastReq["track_total_hits"]
			if present != tc.wantTotalHitsIn {
				t.Fatalf("track_total_hits present = %v, want %v (body %#v)", present, tc.wantTotalHitsIn, transport.lastReq)
			}
		})
	}
}

// TestAdjustChunkPagerankRetriesOnConflictAndReportsMissingChunk covers the
// three behaviours copied from the Elasticsearch engine: conflict retries, 404
// mapped to ErrDocumentNotFound, and the noop result (chunk belongs to another
// dataset) surfaced as an error.
func TestAdjustChunkPagerankRetriesOnConflictAndReportsMissingChunk(t *testing.T) {
	t.Run("retry_on_conflict and success", func(t *testing.T) {
		transport := &requestCaptureTransport{
			status: http.StatusOK,
			body:   `{"result":"updated"}`,
		}
		eng := newCaptureEngine(t, transport)

		if err := eng.AdjustChunkPagerank(t.Context(), "ragflow_t", "c1", "kb1", 1, 0, 10); err != nil {
			t.Fatalf("AdjustChunkPagerank: %v", err)
		}
		if want := "retry_on_conflict=3"; !strings.Contains(transport.lastURL, want) {
			t.Fatalf("request URL %q does not contain %q", transport.lastURL, want)
		}
	})

	t.Run("missing chunk maps to ErrDocumentNotFound", func(t *testing.T) {
		transport := &requestCaptureTransport{
			status: http.StatusNotFound,
			body:   `{"error":{"type":"document_missing_exception"}}`,
		}
		eng := newCaptureEngine(t, transport)

		err := eng.AdjustChunkPagerank(t.Context(), "ragflow_t", "c1", "kb1", 1, 0, 10)
		if !errors.Is(err, types.ErrDocumentNotFound) {
			t.Fatalf("error = %v, want ErrDocumentNotFound", err)
		}
	})

	t.Run("noop reports dataset mismatch", func(t *testing.T) {
		transport := &requestCaptureTransport{
			status: http.StatusOK,
			body:   `{"result":"noop"}`,
		}
		eng := newCaptureEngine(t, transport)

		err := eng.AdjustChunkPagerank(t.Context(), "ragflow_t", "c1", "kb1", 1, 0, 10)
		if err == nil {
			t.Fatal("AdjustChunkPagerank returned nil for a noop update")
		}
	})
}

func TestHybridSearchUsesInlinePipelineWithDynamicWeights(t *testing.T) {
	for _, tc := range []struct {
		name          string
		weightsExpr   string
		includeFusion bool
		expectedBM25W float64
		expectedKNNW  float64
	}{
		{
			name:          "minimal lexical leg from retrieval path is normalized",
			weightsExpr:   "0.001,1",
			includeFusion: true,
			expectedBM25W: 0.001 / 1.001,
			expectedKNNW:  1.0 / 1.001,
		},
		{
			name:          "custom 30/70 weight",
			weightsExpr:   "0.3,0.7",
			includeFusion: true,
			expectedBM25W: 0.3,
			expectedKNNW:  0.7,
		},
		{
			name:          "custom 30/70 with whitespace",
			weightsExpr:   "0.3, 0.7",
			includeFusion: true,
			expectedBM25W: 0.3,
			expectedKNNW:  0.7,
		},
		{
			name:          "custom 80/20 weight",
			weightsExpr:   "0.8,0.2",
			includeFusion: true,
			expectedBM25W: 0.8,
			expectedKNNW:  0.2,
		},
		{
			name:          "pure vector weight boundary",
			weightsExpr:   "0.0,1.0",
			includeFusion: true,
			expectedBM25W: 0.0,
			expectedKNNW:  1.0,
		},
		{
			name:          "pure keyword weight boundary",
			weightsExpr:   "1.0,0.0",
			includeFusion: true,
			expectedBM25W: 1.0,
			expectedKNNW:  0.0,
		},
		{
			name:          "default 50/50 without FusionExpr",
			includeFusion: false,
			expectedBM25W: 0.5,
			expectedKNNW:  0.5,
		},
		{
			name:          "clamped negative vector weight",
			weightsExpr:   "1.5,-0.5",
			includeFusion: true,
			expectedBM25W: 1.0,
			expectedKNNW:  0.0,
		},
		{
			name:          "clamped negative lexical weight",
			weightsExpr:   "-0.5,1.5",
			includeFusion: true,
			expectedBM25W: 0.0,
			expectedKNNW:  1.0,
		},
		{
			name:          "zero weights fall back to default 50/50",
			weightsExpr:   "0,0",
			includeFusion: true,
			expectedBM25W: 0.5,
			expectedKNNW:  0.5,
		},
		{
			name:          "NaN weight falls back to default 50/50",
			weightsExpr:   "0.5,NaN",
			includeFusion: true,
			expectedBM25W: 0.5,
			expectedKNNW:  0.5,
		},
		{
			name:          "Inf weight falls back to default 50/50",
			weightsExpr:   "0.5,Inf",
			includeFusion: true,
			expectedBM25W: 0.5,
			expectedKNNW:  0.5,
		},
		{
			name:          "malformed single token falls back to default 50/50",
			weightsExpr:   "0.7",
			includeFusion: true,
			expectedBM25W: 0.5,
			expectedKNNW:  0.5,
		},
		{
			name:          "non-numeric weight falls back to default 50/50",
			weightsExpr:   "0.3,abc",
			includeFusion: true,
			expectedBM25W: 0.5,
			expectedKNNW:  0.5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &requestCaptureTransport{
				status: http.StatusOK,
				body:   `{"took":1,"hits":{"total":{"value":1},"hits":[{"_id":"c1","_index":"ragflow_t","_score":1.0,"_source":{"id":"c1"}}]}}`,
			}
			eng := newCaptureEngine(t, transport)
			eng.hybridSearchEnabled = true

			matchExprs := []interface{}{
				&types.MatchTextExpr{MatchingText: "hello", Fields: []string{"content_ltks"}, TopN: 10},
				&types.MatchDenseExpr{VectorColumnName: "q_768_vec", EmbeddingData: make([]float64, 768), TopN: 10},
			}
			if tc.includeFusion {
				matchExprs = append(matchExprs, &types.FusionExpr{
					Method:       "weighted_sum",
					FusionParams: map[string]interface{}{"weights": tc.weightsExpr},
				})
			}

			_, err := eng.Search(t.Context(), &types.SearchRequest{
				IndexNames: []string{"ragflow_t"},
				MatchExprs: matchExprs,
				Limit:      10,
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			// Verify query body contains inline search_pipeline
			pipeline, ok := transport.lastReq["search_pipeline"].(map[string]interface{})
			if !ok {
				t.Fatalf("search_pipeline missing from request body: %#v", transport.lastReq)
			}
			processors, ok := pipeline["phase_results_processors"].([]interface{})
			if !ok || len(processors) != 1 {
				t.Fatalf("expected 1 phase_results_processor, got %#v", pipeline["phase_results_processors"])
			}
			normProc, ok := processors[0].(map[string]interface{})["normalization-processor"].(map[string]interface{})
			if !ok {
				t.Fatalf("normalization-processor missing: %#v", processors[0])
			}
			combination, ok := normProc["combination"].(map[string]interface{})
			if !ok {
				t.Fatalf("combination missing: %#v", normProc)
			}
			params, ok := combination["parameters"].(map[string]interface{})
			if !ok {
				t.Fatalf("parameters missing: %#v", combination)
			}
			weights, ok := params["weights"].([]interface{})
			if !ok || len(weights) != 2 {
				t.Fatalf("weights missing or invalid length: %#v", params["weights"])
			}

			bm25W := weights[0].(float64)
			knnW := weights[1].(float64)
			if math.Abs(bm25W-tc.expectedBM25W) > 1e-6 || math.Abs(knnW-tc.expectedKNNW) > 1e-6 {
				t.Fatalf("got weights [%f, %f], want [%f, %f]", bm25W, knnW, tc.expectedBM25W, tc.expectedKNNW)
			}
			if math.Abs((bm25W+knnW)-1.0) > 1e-9 {
				t.Fatalf("normalized weights must sum to 1.0, got [%f, %f]", bm25W, knnW)
			}
			// Guard against Go encoding whole floats (0, 1) without decimal points,
			// which causes OpenSearch normalization-processor to throw Integer->Double ClassCastException.
			if tc.expectedBM25W == 0 && !strings.Contains(transport.lastRawBody, `"weights":[0.0,1.0]`) {
				t.Fatalf("wire weights must preserve explicit decimal floats, got raw body %s", transport.lastRawBody)
			}
			if tc.expectedKNNW == 0 && !strings.Contains(transport.lastRawBody, `"weights":[1.0,0.0]`) {
				t.Fatalf("wire weights must preserve explicit decimal floats, got raw body %s", transport.lastRawBody)
			}

			// Verify URL does NOT contain ?search_pipeline=
			if strings.Contains(transport.lastURL, "search_pipeline=") {
				t.Fatalf("search URL must not contain search_pipeline query param, got %q", transport.lastURL)
			}
		})
	}
}

func TestSupportsHybridSearch(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"2.9.0", false},
		{"2.9.1", false},
		{"2.10.0", true},
		{"2.11.0", true},
		{"2.19.7", true},
		{"3.0.0", true},
		{"3.6.0", true},
		{"3.9.0", true},
		{"7.10.2", false}, // compatibility mode
		{"invalid", false},
		{"1.3.0", false},
	}
	for _, tc := range cases {
		got := supportsHybridSearch(tc.version)
		if got != tc.want {
			t.Errorf("supportsHybridSearch(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestDeepPaginationSortContract(t *testing.T) {
	assertSortField := func(t *testing.T, entry interface{}, wantField string, wantOrder string) {
		t.Helper()
		m, ok := entry.(map[string]interface{})
		if !ok {
			t.Fatalf("expected sort entry to be map, got %#v", entry)
		}
		cfg, ok := m[wantField].(map[string]interface{})
		if !ok {
			t.Fatalf("expected sort field %q, got %#v", wantField, m)
		}
		if cfg["order"] != wantOrder {
			t.Fatalf("sort %q order = %v, want %q", wantField, cfg["order"], wantOrder)
		}
		if wantField == "_id" {
			t.Fatalf("sort must not use _id fielddata")
		}
	}

	cases := []struct {
		name      string
		req       *types.SearchRequest
		wantOrder [][2]string
	}{
		{
			name: "pure filter uses keyword id tiebreaker instead of _id",
			req: &types.SearchRequest{
				IndexNames: []string{"ragflow_t"},
				Offset:     10000,
				Limit:      10,
			},
			wantOrder: [][2]string{{"id", "asc"}},
		},
		{
			name: "text query without OrderBy preserves _score relevance before keyword id tiebreaker",
			req: &types.SearchRequest{
				IndexNames: []string{"ragflow_t"},
				MatchExprs: []interface{}{
					&types.MatchTextExpr{MatchingText: "hello", Fields: []string{"content_ltks"}, TopN: 10},
				},
				Offset: 10000,
				Limit:  10,
			},
			wantOrder: [][2]string{{"_score", "desc"}, {"id", "asc"}},
		},
		{
			name: "explicit non-unique OrderBy appends keyword id tiebreaker",
			req: &types.SearchRequest{
				IndexNames: []string{"ragflow_t"},
				OrderBy:    (&types.OrderByExpr{}).Desc("create_time"),
				Offset:     10000,
				Limit:      10,
			},
			wantOrder: [][2]string{{"create_time", "desc"}, {"id", "asc"}},
		},
		{
			name: "explicit id sort is preserved without duplication",
			req: &types.SearchRequest{
				IndexNames: []string{"ragflow_t"},
				OrderBy:    (&types.OrderByExpr{}).Desc("id"),
				Offset:     10000,
				Limit:      10,
			},
			wantOrder: [][2]string{{"id", "desc"}},
		},
		{
			name: "skill index uses mapped skill_id tiebreaker",
			req: &types.SearchRequest{
				IndexNames: []string{"skill_space1"},
				MatchExprs: []interface{}{
					&types.MatchTextExpr{MatchingText: "tool", Fields: []string{"name_tks"}, TopN: 10},
				},
				Offset: 10000,
				Limit:  10,
			},
			wantOrder: [][2]string{{"_score", "desc"}, {"skill_id", "asc"}},
		},
		{
			name: "multi-index query appends _index disambiguator",
			req: &types.SearchRequest{
				IndexNames: []string{"ragflow_a", "ragflow_b"},
				MatchExprs: []interface{}{
					&types.MatchTextExpr{MatchingText: "hello", Fields: []string{"content_ltks"}, TopN: 10},
				},
				Offset: 10000,
				Limit:  10,
			},
			wantOrder: [][2]string{{"_score", "desc"}, {"id", "asc"}, {"_index", "asc"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &requestCaptureTransport{
				status: http.StatusOK,
				body:   `{"took": 1, "timed_out": false, "hits": {"total": {"value": 15000, "relation": "eq"}, "hits": [{"_id": "c1", "_index": "ragflow_t", "_score": 1.2, "_source": {"id": "c1"}, "sort": ["c1"]}]}}`,
			}
			eng := newCaptureEngine(t, transport)

			if _, err := eng.Search(t.Context(), tc.req); err != nil {
				t.Fatalf("Search: %v", err)
			}

			sortList, ok := transport.lastReq["sort"].([]interface{})
			if !ok || len(sortList) != len(tc.wantOrder) {
				t.Fatalf("sort = %#v (len %d), want len %d", transport.lastReq["sort"], len(sortList), len(tc.wantOrder))
			}
			for i, want := range tc.wantOrder {
				assertSortField(t, sortList[i], want[0], want[1])
			}
			if fromVal, hasFrom := transport.lastReq["from"]; hasFrom {
				t.Errorf("search_after request must not have from, got %v", fromVal)
			}
		})
	}
}
