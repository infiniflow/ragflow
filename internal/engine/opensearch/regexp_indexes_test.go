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
	"net/http"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
)

// TestSearchByRegexpCoversEveryIndex pins the multi-tenant contract of the
// agent's grep leg: one request per requested index, and an index that does not
// exist yet (a tenant with nothing ingested) contributes nothing instead of
// failing the whole search.
func TestSearchByRegexpCoversEveryIndex(t *testing.T) {
	var paths []string
	engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/ragflow_shared/") {
			return searchErrorResponse(404, `{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`), nil
		}
		return searchErrorResponse(200, `{"hits":{"total":{"value":1},"hits":[{"_id":"c1","_source":{"id":"c1","content_with_weight":"hit"}}]}}`), nil
	})

	result, err := engine.SearchByRegexp(t.Context(), &types.RegexpSearchRequest{
		IndexNames: []string{"ragflow_mine", "ragflow_shared"},
		KbIDs:      []string{"kb-mine", "kb-shared"},
		Pattern:    "hit",
	})
	if err != nil {
		t.Fatalf("SearchByRegexp: %v", err)
	}
	if len(paths) != 2 || !strings.HasPrefix(paths[0], "/ragflow_mine/") || !strings.HasPrefix(paths[1], "/ragflow_shared/") {
		t.Fatalf("requested paths = %v, want one search per index", paths)
	}
	if len(result.Chunks) != 1 || result.Total != 1 {
		t.Fatalf("result = %+v, want the populated index's single hit", result)
	}
}

// TestSearchByRegexpRejectsEmptyIndexNames keeps an unresolved corpus scope from
// turning into an unscoped search.
func TestSearchByRegexpRejectsEmptyIndexNames(t *testing.T) {
	engine := newSearchErrorEngine(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("an empty index scope must not reach the backend")
		return nil, nil
	})
	if _, err := engine.SearchByRegexp(t.Context(), &types.RegexpSearchRequest{Pattern: "hit"}); err == nil {
		t.Fatal("SearchByRegexp without index names must fail")
	}
}

// TestSearchByRegexpSortsTheMergedSetOnce asserts a reading-order request is
// ordered across ALL indexes, not per index: the deep read depends on one
// globally ordered result set.
func TestSearchByRegexpSortsTheMergedSetOnce(t *testing.T) {
	engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/ragflow_a/") {
			return searchErrorResponse(200, `{"hits":{"total":{"value":1},"hits":[{"_id":"second","_source":{"id":"second","chunk_order_int":9}}]}}`), nil
		}
		return searchErrorResponse(200, `{"hits":{"total":{"value":1},"hits":[{"_id":"first","_source":{"id":"first","chunk_order_int":1}}]}}`), nil
	})

	result, err := engine.SearchByRegexp(t.Context(), &types.RegexpSearchRequest{
		IndexNames: []string{"ragflow_a", "ragflow_b"},
		KbIDs:      []string{"kb-a", "kb-b"},
		Pattern:    "hit",
		Sort:       &types.OrderByExpr{Fields: []types.OrderByField{{Field: "chunk_order_int", Type: types.SortAsc}}},
	})
	if err != nil {
		t.Fatalf("SearchByRegexp: %v", err)
	}
	if len(result.Chunks) != 2 || result.Chunks[0]["id"] != "first" || result.Chunks[1]["id"] != "second" {
		t.Fatalf("chunks = %+v, want the merge sorted by chunk_order_int", result.Chunks)
	}
}
