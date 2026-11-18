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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// The metadata path must honor an `id` sort like every other scan in this
// package: SearchMetadata pages with from/size, so with the sort dropped the
// pages carry no deterministic order at all. And on mappings that still type
// `id` as `text` (indices created before the keyword dynamic template covered
// it) a direct `id` sort fails with a fielddata error, so — like Search —
// SearchMetadata must resolve the sortable form of `id` per index and rewrite
// the entry (#19649, review of PR #20408).

// newMetadataSearchStub spins up an Elasticsearch stub answering the
// index-exists probe, the id-mapping lookup and the metadata search itself,
// capturing every search body it receives.
func newMetadataSearchStub(t *testing.T, index, mapping string, searchBodies *[]map[string]interface{}) *httptest.Server {
	t.Helper()
	if err := common.InitLogger("info", common.FileOutput{}, "elasticsearch_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/"+index):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/_mapping"):
			_, _ = w.Write([]byte(`{"` + index + `":` + mapping + "}"))
		case strings.HasSuffix(r.URL.Path, "/_search"):
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode search body: %v", err)
			}
			*searchBodies = append(*searchBodies, body)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{"total": map[string]int{"value": 0}, "hits": []interface{}{}},
			})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestSearchMetadataEmitsIDSort pins that an Asc("id") OrderBy reaches the
// metadata search as a real sort entry. The shared esoscommon parser this
// path once used drops `id` by design, which silently removed the only
// deterministic sort key metadata pagination had (#19649).
func TestSearchMetadataEmitsIDSort(t *testing.T) {
	const tenant = "metadata_id_sort_keyword"
	index := buildMetadataIndexName(tenant)
	t.Cleanup(func() { idSortFieldCache.Delete(index) })

	var searchBodies []map[string]interface{}
	server := newMetadataSearchStub(t, index, `{"mappings":{"properties":{"id":{"type":"keyword"}}}}`, &searchBodies)

	engine := newTestEngine(t, server.URL)
	if _, err := engine.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID: tenant,
		Offset:   0,
		Limit:    10,
		Filter:   map[string]interface{}{},
		OrderBy:  (&types.OrderByExpr{}).Asc("id"),
	}); err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}

	if len(searchBodies) != 1 {
		t.Fatalf("captured %d search bodies, want 1", len(searchBodies))
	}
	sort, ok := searchBodies[0]["sort"].([]interface{})
	if !ok || len(sort) != 1 {
		t.Fatalf("metadata search carried sort %#v, want exactly one id entry (the sort must not be dropped)", searchBodies[0]["sort"])
	}
	entry, ok := sort[0].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata search sort entry has wrong shape: %#v", sort[0])
	}
	spec, ok := entry["id"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata search sorted by %v, want the direct id field", entry)
	}
	if spec["order"] != "asc" || spec["unmapped_type"] != "keyword" {
		t.Errorf("id sort spec = %v, want order asc with unmapped_type keyword", spec)
	}
}

// TestSearchMetadataRewritesIDSortForLegacyMapping pins the legacy-mapping
// rewrite on the metadata path: an index whose `id` is still `text` with an
// `id.keyword` sub-field must sort on the sub-field — a direct `id` sort
// fails there with a fielddata error.
func TestSearchMetadataRewritesIDSortForLegacyMapping(t *testing.T) {
	const tenant = "metadata_id_sort_legacy"
	index := buildMetadataIndexName(tenant)
	t.Cleanup(func() { idSortFieldCache.Delete(index) })

	var searchBodies []map[string]interface{}
	server := newMetadataSearchStub(t, index,
		`{"mappings":{"properties":{"id":{"type":"text","fields":{"keyword":{"type":"keyword","ignore_above":256}}}}}}`,
		&searchBodies)

	engine := newTestEngine(t, server.URL)
	if _, err := engine.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID: tenant,
		Offset:   0,
		Limit:    10,
		Filter:   map[string]interface{}{},
		OrderBy:  (&types.OrderByExpr{}).Asc("id"),
	}); err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}

	if len(searchBodies) != 1 {
		t.Fatalf("captured %d search bodies, want 1", len(searchBodies))
	}
	sort, ok := searchBodies[0]["sort"].([]interface{})
	if !ok || len(sort) != 1 {
		t.Fatalf("metadata search carried sort %#v, want one id.keyword entry", searchBodies[0]["sort"])
	}
	entry, ok := sort[0].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata search sort entry has wrong shape: %#v", sort[0])
	}
	spec, ok := entry["id.keyword"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata search sorted by %v, want the sortable id.keyword sub-field", entry)
	}
	if spec["order"] != "asc" {
		t.Errorf("id.keyword sort spec = %v, want order asc", spec)
	}
}

// TestSearchMetadataWithoutOrderByLeavesSortAbsent guards the no-order path:
// without an OrderBy the metadata search must keep going out unsorted.
func TestSearchMetadataWithoutOrderByLeavesSortAbsent(t *testing.T) {
	const tenant = "metadata_id_sort_unsorted"
	index := buildMetadataIndexName(tenant)
	t.Cleanup(func() { idSortFieldCache.Delete(index) })

	var searchBodies []map[string]interface{}
	server := newMetadataSearchStub(t, index, `{"mappings":{"properties":{"id":{"type":"keyword"}}}}`, &searchBodies)

	engine := newTestEngine(t, server.URL)
	if _, err := engine.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID: tenant,
		Offset:   0,
		Limit:    10,
		Filter:   map[string]interface{}{},
	}); err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}

	if len(searchBodies) != 1 {
		t.Fatalf("captured %d search bodies, want 1", len(searchBodies))
	}
	if _, ok := searchBodies[0]["sort"]; ok {
		t.Fatalf("unsorted metadata search carried sort %#v, want none", searchBodies[0]["sort"])
	}
}
