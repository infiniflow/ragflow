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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// newMetadataIndexServer serves a metadata index of n rows sorted on id. Like
// Elasticsearch, it rejects from+size past the result window and pages with
// search_after.
func newMetadataIndexServer(t *testing.T, n int) *httptest.Server {
	t.Helper()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("doc-%05d", i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			From        *int          `json:"from"`
			Size        *int          `json:"size"`
			SearchAfter []interface{} `json:"search_after"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode search body: %v", err)
		}
		from, size := 0, 10
		if body.From != nil {
			from = *body.From
		}
		if body.Size != nil {
			size = *body.Size
		}
		if from+size > common.MAX_RESULT_WINDOW {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"illegal_argument_exception","reason":"Result window is too large"}}`))
			return
		}
		start := from
		if len(body.SearchAfter) > 0 {
			cursor, _ := body.SearchAfter[0].(string)
			start = 0
			for start < n && ids[start] <= cursor {
				start++
			}
		}
		hits := []map[string]interface{}{}
		for i := start; i < n && len(hits) < size; i++ {
			hits = append(hits, map[string]interface{}{
				"_id":     ids[i],
				"_source": map[string]interface{}{"id": ids[i], "kb_id": "kb-1"},
				"sort":    []interface{}{ids[i]},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hits": map[string]interface{}{
				"total": map[string]interface{}{"value": n, "relation": "eq"},
				"hits":  hits,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSearchMetadataPastTheResultWindow pins how a metadata page beyond
// index.max_result_window is read: sorted, it is walked with search_after;
// unsorted, it is cut at the window instead of failing.
func TestSearchMetadataPastTheResultWindow(t *testing.T) {
	const n = 12345
	e := newTestEngine(t, newMetadataIndexServer(t, n).URL)
	read := func(offset, limit int, sorted bool) *types.SearchMetadataResult {
		t.Helper()
		req := &types.SearchMetadataRequest{TenantID: "tenant-1", Offset: offset, Limit: limit, Filter: map[string]interface{}{"kb_id": []string{"kb-1"}}}
		if sorted {
			req.OrderBy = (&types.OrderByExpr{}).Asc("id")
		}
		res, err := e.SearchMetadata(t.Context(), req)
		if err != nil {
			t.Fatalf("SearchMetadata(offset=%d, limit=%d, sorted=%v): %v", offset, limit, sorted, err)
		}
		return res
	}

	page := read(10000, 5000, true)
	if got := len(page.MetadataRecords); got != n-10000 {
		t.Fatalf("sorted page past the window returned %d rows, want %d", got, n-10000)
	}
	if first := page.MetadataRecords[0]["id"]; first != "doc-10000" {
		t.Fatalf("sorted page past the window starts at %v, want doc-10000", first)
	}
	if page.Total != n {
		t.Fatalf("total = %d, want %d", page.Total, n)
	}

	if got := len(read(8000, 5000, false).MetadataRecords); got != 2000 {
		t.Fatalf("unsorted page across the window returned %d rows, want the 2000 below it", got)
	}
	if got := len(read(10000, 5000, false).MetadataRecords); got != 0 {
		t.Fatalf("unsorted page past the window returned %d rows, want none", got)
	}
}
