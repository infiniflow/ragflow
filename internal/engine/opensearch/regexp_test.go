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
	"io"
	"net/http"
	"testing"

	"ragflow/internal/engine/types"
)

type regexpSearchable interface {
	SearchByRegexp(ctx context.Context, req *types.RegexpSearchRequest) (*types.SearchResult, error)
}

func TestEngine_SatisfiesRegexpSearchable(t *testing.T) {
	var e *Engine
	var _ regexpSearchable = e
}

func TestSearchByRegexp_Validation(t *testing.T) {
	ctx := context.Background()

	var nilEngine *Engine
	if _, err := nilEngine.SearchByRegexp(ctx, &types.RegexpSearchRequest{}); err == nil {
		t.Errorf("expected error on nil engine, got nil")
	}

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	// Nil request
	if _, err := engine.SearchByRegexp(ctx, nil); err == nil {
		t.Errorf("expected error on nil request, got nil")
	}

	// Empty tenant ID
	if _, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{TenantID: "", Pattern: "test"}); err == nil {
		t.Errorf("expected error on empty tenant ID, got nil")
	}

	// Multi-tenant ID with comma
	if _, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{TenantID: "t1,t2", Pattern: "test"}); err == nil {
		t.Errorf("expected error on multi-tenant ID, got nil")
	}

	// Empty pattern
	if _, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{TenantID: "t1", Pattern: "   "}); err == nil {
		t.Errorf("expected error on empty pattern, got nil")
	}
}

func TestSearchByRegexp_Execution_QueryPayload(t *testing.T) {
	var capturedPath string
	var capturedBody string

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"hits": {
				"total": {"value": 1},
				"hits": [
					{
						"_id": "chunk_1",
						"_index": "ragflow_tenant_abc",
						"_score": 1.0,
						"_source": {
							"id": "chunk_1",
							"content_with_weight": "regex target text"
						}
					}
				]
			}
		}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	req := &types.RegexpSearchRequest{
		TenantID:     "tenant_abc",
		Pattern:      "target.*text",
		KbIDs:        []string{"kb_1"},
		SelectFields: []string{"id", "content_with_weight"},
	}

	res, err := engine.SearchByRegexp(ctx, req)
	if err != nil {
		t.Fatalf("SearchByRegexp failed: %v", err)
	}

	if capturedPath != "/ragflow_tenant_abc/_search" {
		t.Errorf("expected path /ragflow_tenant_abc/_search, got %s", capturedPath)
	}

	var dsl map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &dsl); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	// Check query.bool.must contains regexp on content_with_weight
	query, _ := dsl["query"].(map[string]interface{})
	boolQ, _ := query["bool"].(map[string]interface{})
	mustList, _ := boolQ["must"].([]interface{})

	foundRegexp := false
	for _, m := range mustList {
		clause, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if regexpClause, ok := clause["regexp"].(map[string]interface{}); ok {
			if cww, ok := regexpClause["content_with_weight"].(map[string]interface{}); ok {
				foundRegexp = true
				if cww["value"] != ".*(target.*text).*" {
					t.Errorf("expected value '.*(target.*text).*', got %v", cww["value"])
				}
				if cww["case_insensitive"] != true {
					t.Errorf("expected case_insensitive=true, got %v", cww["case_insensitive"])
				}
				if cww["flags"] != "NONE" {
					t.Errorf("expected flags='NONE', got %v", cww["flags"])
				}
			}
		}
	}
	if !foundRegexp {
		t.Errorf("expected regexp clause in bool.must, got %v", capturedBody)
	}

	if res.Total != 1 || len(res.Chunks) != 1 {
		t.Fatalf("expected 1 result, got total=%d chunks=%d", res.Total, len(res.Chunks))
	}
	if res.Chunks[0]["_id"] != "chunk_1" {
		t.Errorf("chunk ID mismatch: %v", res.Chunks[0]["_id"])
	}
}

func TestSearchByRegexp_SortPreserved(t *testing.T) {
	var capturedBody string
	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"hits": {
				"total": {"value": 2},
				"hits": [
					{
						"_id": "chunk_b",
						"_index": "ragflow_tenant_abc",
						"_source": {"id": "chunk_b", "page_num_int": [10.0]}
					},
					{
						"_id": "chunk_a",
						"_index": "ragflow_tenant_abc",
						"_source": {"id": "chunk_a", "page_num_int": [2.0]}
					}
				]
			}
		}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	req := &types.RegexpSearchRequest{
		TenantID: "tenant_abc",
		Pattern:  "test",
		Sort: &types.OrderByExpr{
			Fields: []types.OrderByField{
				{Field: "page_num_int", Type: types.SortAsc},
			},
		},
	}

	res, err := engine.SearchByRegexp(ctx, req)
	if err != nil {
		t.Fatalf("SearchByRegexp failed: %v", err)
	}

	var dsl map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &dsl); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}
	if _, ok := dsl["sort"]; !ok {
		t.Errorf("expected sort clause in request payload, got %v", capturedBody)
	}

	// Verify hits retain the exact order returned by OpenSearch
	if len(res.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(res.Chunks))
	}
	if res.Chunks[0]["id"] != "chunk_b" || res.Chunks[1]["id"] != "chunk_a" {
		t.Errorf("expected order [chunk_b, chunk_a] preserved, got [%v, %v]",
			res.Chunks[0]["id"], res.Chunks[1]["id"])
	}
}
