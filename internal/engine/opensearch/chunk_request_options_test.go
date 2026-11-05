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
	status  int
	body    string
	lastURL string
	lastReq map[string]interface{}
}

func (f *requestCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.lastURL = r.URL.String()
	f.lastReq = nil
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
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
	return &Engine{client: client, hybridPipelineName: "ragflow_hybrid_pipeline"}
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
