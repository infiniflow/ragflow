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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/engine/types"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

func setupMockEngine(t *testing.T, handler http.HandlerFunc) (*Engine, *httptest.Server) {
	server := httptest.NewServer(handler)
	osConfig := opensearch.Config{
		Addresses: []string{server.URL},
	}
	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: osConfig,
	})
	if err != nil {
		t.Fatalf("failed to create apiClient: %v", err)
	}

	engine := &Engine{
		transportClient: apiClient.Client,
		apiClient:       apiClient,
		version:         "2.19.0",
	}
	return engine, server
}

func TestCreateChunkStore(t *testing.T) {
	var createdIndex string
	var createdBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			// Index does not exist initially
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/"):
			createdIndex = strings.TrimPrefix(r.URL.Path, "/")
			_ = json.NewDecoder(r.Body).Decode(&createdBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Regular dataset index creation with vector size 1024
	err := engine.CreateChunkStore(ctx, "ragflow_tenant1", "kb1", 1024, "naive", "English")
	if err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}
	if createdIndex != "ragflow_tenant1" {
		t.Errorf("Expected index name ragflow_tenant1, got %s", createdIndex)
	}
	settings, ok := createdBody["settings"].(map[string]interface{})
	if !ok || settings["index.knn"] != true {
		t.Errorf("Expected index.knn=true in settings, got %v", settings)
	}
	mappings, ok := createdBody["mappings"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected mappings in created body, got %v", createdBody)
	}
	properties, ok := mappings["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected properties in mappings, got %v", mappings)
	}
	if _, ok := properties["tag_feas"]; !ok {
		t.Errorf("Expected tag_feas in chunk store properties")
	}
	vecProp, ok := properties["q_1024_vec"].(map[string]interface{})
	if !ok || vecProp["type"] != "knn_vector" || vecProp["dimension"] != float64(1024) {
		t.Errorf("Expected q_1024_vec knn_vector mapping, got %v", vecProp)
	}
}

func TestInsertChunks_BatchesAndFields(t *testing.T) {
	var bulkBodies []string
	var refreshParam string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "_bulk"):
			refreshParam = r.URL.Query().Get("refresh")
			bodyBytes, _ := io.ReadAll(r.Body)
			bulkBodies = append(bulkBodies, string(bodyBytes))
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": false,
				"items":  []interface{}{},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Test InsertChunks with wait_for refresh
	chunks := []map[string]interface{}{
		{
			"id":       "chunk_1",
			"doc_id":   "doc_1",
			"content":  "test chunk content",
			"tag_feas": map[string]float64{"tagA": 1.5, "tagB": 0.8},
		},
	}

	_, err := engine.InsertChunks(ctx, chunks, "ragflow_tenant1", "dataset_kb1", "English")
	if err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}
	if refreshParam != "wait_for" {
		t.Errorf("InsertChunks expected refresh=wait_for, got %s", refreshParam)
	}
	if len(bulkBodies) != 1 {
		t.Fatalf("Expected 1 bulk body, got %d", len(bulkBodies))
	}
	body := bulkBodies[0]
	if !strings.Contains(body, `{"index":{"_id":"chunk_1","_index":"ragflow_tenant1"}}`) {
		t.Errorf("Bulk action line missing or incorrect: %s", body)
	}
	if !strings.Contains(body, `"kb_id":"dataset_kb1"`) {
		t.Errorf("Bulk document missing kb_id isolation: %s", body)
	}
	if !strings.Contains(body, `"available_int":1`) {
		t.Errorf("Bulk document missing default available_int: %s", body)
	}

	// 2. Test InsertChunksNoRefresh with empty refresh param
	bulkBodies = nil
	refreshParam = "initial"
	_, err = engine.InsertChunksNoRefresh(ctx, chunks, "ragflow_tenant1", "dataset_kb1", "English")
	if err != nil {
		t.Fatalf("InsertChunksNoRefresh failed: %v", err)
	}
	if refreshParam != "" {
		t.Errorf("InsertChunksNoRefresh expected empty refresh param, got %s", refreshParam)
	}
}

func TestInsertChunks_ItemErrors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "_bulk"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": true,
				"items": []map[string]interface{}{
					{
						"index": map[string]interface{}{
							"error": map[string]interface{}{
								"type":   "mapper_parsing_exception",
								"reason": "failed to parse field [q_1024_vec] of type [knn_vector]",
							},
							"status": 400,
						},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	chunks := []map[string]interface{}{
		{"id": "chunk_err", "doc_id": "doc_1"},
	}
	_, err := engine.InsertChunks(context.Background(), chunks, "ragflow_test", "kb1", "English")
	if err == nil {
		t.Fatalf("Expected error on bulk item failure, got nil")
	}
	if !strings.Contains(err.Error(), "mapper_parsing_exception") && !strings.Contains(err.Error(), "failed to parse field") {
		t.Errorf("Error reason not surfaced: %v", err)
	}
}

func TestUpdateChunks_SingleDocAndRemove(t *testing.T) {
	var capturedSearch bool
	var capturedSearchBody map[string]interface{}
	var updateRequests []map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			if strings.Contains(r.URL.Path, "non_existent") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK) // index exists
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
			capturedSearch = true
			_ = json.NewDecoder(r.Body).Decode(&capturedSearchBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{
					"hits": []map[string]interface{}{
						{"_id": "chunk_123", "_source": map[string]interface{}{"id": "chunk_123"}},
					},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update/chunk_123"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			updateRequests = append(updateRequests, body)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "updated"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// Update single chunk with remove, add, and field change
	condition := map[string]interface{}{"id": "chunk_123"}
	newValue := map[string]interface{}{
		"remove":        "obsolete_field",
		"add":           map[string]interface{}{"tags": "new_tag"},
		"content":       "updated content",
		"available_int": 0,
	}

	err := engine.UpdateChunks(ctx, condition, newValue, "ragflow_tenant1", "kb1")
	if err != nil {
		t.Fatalf("UpdateChunks single document failed: %v", err)
	}
	if !capturedSearch {
		t.Errorf("Expected search to resolve document _id")
	}

	// Verify search enforced kb_id dataset isolation
	queryBool := capturedSearchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})
	mustList := queryBool["must"].([]interface{})
	var foundIDTerm, foundKbIDTerm bool
	for _, m := range mustList {
		clause := m.(map[string]interface{})
		if term, ok := clause["term"].(map[string]interface{}); ok {
			if term["id"] == "chunk_123" {
				foundIDTerm = true
			}
			if term["kb_id"] == "kb1" {
				foundKbIDTerm = true
			}
		}
	}
	if !foundIDTerm || !foundKbIDTerm {
		t.Errorf("Single chunk search missing ID or kb_id filter: %v", mustList)
	}

	if len(updateRequests) == 0 {
		t.Fatalf("Expected update requests to be issued, got 0")
	}

	// Verify remove script, add script, and doc update
	var foundRemove, foundAdd, foundDoc bool
	for _, req := range updateRequests {
		if script, ok := req["script"].(map[string]interface{}); ok {
			if src, ok := script["source"].(string); ok {
				if strings.Contains(src, "ctx._source.remove('obsolete_field')") {
					foundRemove = true
				}
				if strings.Contains(src, "ctx._source.tags.add(params.pp_tags)") {
					foundAdd = true
				}
			}
		}
		if doc, ok := req["doc"].(map[string]interface{}); ok {
			if doc["content"] == "updated content" {
				foundDoc = true
			}
			if _, hasAdd := doc["add"]; hasAdd {
				t.Errorf("Document update body must NOT contain 'add' field: %v", doc)
			}
			if _, hasRemove := doc["remove"]; hasRemove {
				t.Errorf("Document update body must NOT contain 'remove' field: %v", doc)
			}
		}
	}
	if !foundRemove {
		t.Errorf("Expected Painless remove script was not sent")
	}
	if !foundAdd {
		t.Errorf("Expected Painless add script was not sent")
	}
	if !foundDoc {
		t.Errorf("Expected doc update payload was not sent")
	}

	// Test index not found returns types.ErrIndexNotFound
	nonExistentErr := engine.UpdateChunks(ctx, condition, newValue, "non_existent_idx", "kb1")
	if !errors.Is(nonExistentErr, types.ErrIndexNotFound) {
		t.Errorf("Expected types.ErrIndexNotFound, got %v", nonExistentErr)
	}
}

func TestUpdateChunks_ByQuery(t *testing.T) {
	var updateByQueryBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&updateByQueryBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"updated": 10,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// Multi-doc update condition
	condition := map[string]interface{}{
		"doc_id": "doc_999",
	}
	newValue := map[string]interface{}{
		"available_int": 0,
	}

	err := engine.UpdateChunks(ctx, condition, newValue, "ragflow_tenant1", "kb_isolated")
	if err != nil {
		t.Fatalf("UpdateChunks by query failed: %v", err)
	}

	// Assert condition["kb_id"] was injected into query
	query, ok := updateByQueryBody["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected query block in update by query body: %v", updateByQueryBody)
	}
	boolBlock, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected bool block in query: %v", query)
	}
	filters, ok := boolBlock["filter"].([]interface{})
	if !ok {
		t.Fatalf("Expected filter clauses in bool query: %v", boolBlock)
	}

	var hasKbIsolation bool
	for _, f := range filters {
		fm, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if term, ok := fm["term"].(map[string]interface{}); ok {
			if term["kb_id"] == "kb_isolated" {
				hasKbIsolation = true
			}
		}
	}
	if !hasKbIsolation {
		t.Errorf("UpdateByQuery missing kb_id dataset isolation filter: %v", filters)
	}
}

func TestAdjustChunkPagerank(t *testing.T) {
	var capturedScript map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update/chunk_p1") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			capturedScript, _ = body["script"].(map[string]interface{})
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "updated"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	err := engine.AdjustChunkPagerank(ctx, "ragflow_tenant1", "chunk_p1", "kb1", 0.5, 0.0, 10.0)
	if err != nil {
		t.Fatalf("AdjustChunkPagerank failed: %v", err)
	}

	source, ok := capturedScript["source"].(string)
	if !ok {
		t.Fatalf("Expected script source in update body: %v", capturedScript)
	}
	// Assert rank_feature removal on <= 0.0 guard exists
	if !strings.Contains(source, "ctx._source.remove(params.field)") {
		t.Errorf("AdjustChunkPagerank missing <= 0.0 rank_features removal guard: %s", source)
	}
	// Assert dataset boundary check
	if !strings.Contains(source, "ctx._source.kb_id.equals(params.kb_id)") {
		t.Errorf("AdjustChunkPagerank missing dataset boundary check: %s", source)
	}
}

func TestDeleteChunks_IsolationAndSafety(t *testing.T) {
	var deleteByQueryBody map[string]interface{}
	var deleteCalls int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_delete_by_query"):
			atomic.AddInt32(&deleteCalls, 1)
			_ = json.NewDecoder(r.Body).Decode(&deleteByQueryBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"deleted": 5,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Normal delete with kb_id isolation
	deleted, err := engine.DeleteChunks(ctx, map[string]interface{}{"doc_id": "doc_1"}, "ragflow_tenant1", "kb_prod")
	if err != nil {
		t.Fatalf("DeleteChunks failed: %v", err)
	}
	if deleted != 5 {
		t.Errorf("Expected 5 deleted chunks, got %d", deleted)
	}

	query := deleteByQueryBody["query"].(map[string]interface{})
	boolBlock := query["bool"].(map[string]interface{})
	filters := boolBlock["filter"].([]interface{})
	var hasKbIsolation bool
	for _, f := range filters {
		fm := f.(map[string]interface{})
		if term, ok := fm["term"].(map[string]interface{}); ok && term["kb_id"] == "kb_prod" {
			hasKbIsolation = true
		}
	}
	if !hasKbIsolation {
		t.Errorf("DeleteChunks missing kb_id isolation filter: %v", filters)
	}

	// 2. Empty ID slices must immediately return 0, nil without issuing delete request
	initCalls := atomic.LoadInt32(&deleteCalls)
	deletedEmpty, errEmpty := engine.DeleteChunks(ctx, map[string]interface{}{"id": []string{}}, "ragflow_tenant1", "kb_prod")
	if errEmpty != nil || deletedEmpty != 0 {
		t.Errorf("Expected DeleteChunks on empty []string id to return 0, nil; got %d, %v", deletedEmpty, errEmpty)
	}
	deletedEmptyAny, errEmptyAny := engine.DeleteChunks(ctx, map[string]interface{}{"id": []interface{}{}}, "ragflow_tenant1", "kb_prod")
	if errEmptyAny != nil || deletedEmptyAny != 0 {
		t.Errorf("Expected DeleteChunks on empty []interface{} id to return 0, nil; got %d, %v", deletedEmptyAny, errEmptyAny)
	}
	deletedEmptyStr, errEmptyStr := engine.DeleteChunks(ctx, map[string]interface{}{"id": ""}, "ragflow_tenant1", "kb_prod")
	if errEmptyStr != nil || deletedEmptyStr != 0 {
		t.Errorf("Expected DeleteChunks on empty string id to return 0, nil; got %d, %v", deletedEmptyStr, errEmptyStr)
	}
	if calls := atomic.LoadInt32(&deleteCalls); calls != initCalls {
		t.Errorf("Expected no delete requests for empty id conditions, calls = %d, initCalls = %d", calls, initCalls)
	}

	// 3. Unfiltered delete abort safety check (condition keys dropped resulting in match_all on non-empty condition)
	_, err = engine.DeleteChunks(ctx, map[string]interface{}{"unsupported_key": nil}, "ragflow_tenant1", "")
	if err == nil {
		t.Errorf("Expected unfiltered delete abort safety error, got nil")
	}

	// 4. List-valued kb_id and empty list safety check
	deletedEmptyKb, errEmptyKb := engine.DeleteChunks(ctx, map[string]interface{}{"kb_id": []string{}}, "ragflow_tenant1", "")
	if errEmptyKb != nil || deletedEmptyKb != 0 {
		t.Errorf("Expected DeleteChunks on empty []string kb_id to return 0, nil; got %d, %v", deletedEmptyKb, errEmptyKb)
	}

	_, err = engine.DeleteChunks(ctx, map[string]interface{}{"kb_id": []string{"kb_x", "kb_y"}, "doc_id": "doc_1"}, "ragflow_tenant1", "")
	if err != nil {
		t.Fatalf("DeleteChunks with list kb_id failed: %v", err)
	}
	query = deleteByQueryBody["query"].(map[string]interface{})
	boolBlock = query["bool"].(map[string]interface{})
	filters = boolBlock["filter"].([]interface{})
	var hasTermsKb bool
	for _, f := range filters {
		fm := f.(map[string]interface{})
		if terms, ok := fm["terms"].(map[string]interface{}); ok {
			if _, hasKb := terms["kb_id"]; hasKb {
				hasTermsKb = true
			}
		}
	}
	if !hasTermsKb {
		t.Errorf("DeleteChunks with list kb_id missing terms filter: %v", filters)
	}

	// 5. Memory index condition field mapping
	_, err = engine.DeleteChunks(ctx, map[string]interface{}{"status": false, "message_type": "user"}, "memory_tenant1", "")
	if err != nil {
		t.Fatalf("DeleteChunks for memory index failed: %v", err)
	}
	query = deleteByQueryBody["query"].(map[string]interface{})
	boolBlock = query["bool"].(map[string]interface{})
	mustClauses := boolBlock["must"].([]interface{})
	var hasStatusInt, hasMsgTypeKwd bool
	for _, m := range mustClauses {
		mm := m.(map[string]interface{})
		if term, ok := mm["term"].(map[string]interface{}); ok {
			if term["status_int"] == float64(0) || term["status_int"] == 0 {
				hasStatusInt = true
			}
			if term["message_type_kwd"] == "user" {
				hasMsgTypeKwd = true
			}
		}
	}
	if !hasStatusInt || !hasMsgTypeKwd {
		t.Errorf("DeleteChunks memory index did not map condition fields: %v", mustClauses)
	}
}

func TestInsertChunks_AutoCreatesChunkStore(t *testing.T) {
	var (
		indexCreated bool
		createdIndex string
		bulkCalled   bool
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			if !indexCreated {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && !strings.Contains(r.URL.Path, "_mapping"):
			indexCreated = true
			createdIndex = strings.TrimPrefix(r.URL.Path, "/")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "_mapping"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "_bulk"):
			bulkCalled = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"errors": false, "items": []interface{}{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	chunks := []map[string]interface{}{
		{
			"id":        "c1",
			"doc_id":    "d1",
			"q_512_vec": make([]float64, 512),
		},
	}
	_, err := engine.InsertChunks(ctx, chunks, "ragflow_new_tenant", "kb1", "English")
	if err != nil {
		t.Fatalf("InsertChunks failed on non-existent index: %v", err)
	}
	if !indexCreated || createdIndex != "ragflow_new_tenant" {
		t.Fatalf("Expected index creation for ragflow_new_tenant, got created=%v, index=%s", indexCreated, createdIndex)
	}
	if !bulkCalled {
		t.Fatalf("Expected bulk index call after index creation")
	}
}

func TestDeleteChunks_PartialFailure(t *testing.T) {
	tests := []struct {
		name       string
		respBody   map[string]interface{}
		wantErrSub string
	}{
		{
			name: "timed out",
			respBody: map[string]interface{}{
				"deleted":   10,
				"total":     15,
				"timed_out": true,
			},
			wantErrSub: "timed out",
		},
		{
			name: "version conflicts",
			respBody: map[string]interface{}{
				"deleted":           5,
				"version_conflicts": 2,
			},
			wantErrSub: "version conflicts",
		},
		{
			name: "shard failures",
			respBody: map[string]interface{}{
				"deleted": 3,
				"failures": []interface{}{
					map[string]interface{}{"index": "ragflow_tenant1", "reason": "shard failed"},
				},
			},
			wantErrSub: "failures",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_delete_by_query"):
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.respBody)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			engine, server := setupMockEngine(t, handler)
			defer server.Close()

			ctx := context.Background()
			_, err := engine.DeleteChunks(ctx, map[string]interface{}{"doc_id": "doc_1"}, "ragflow_tenant1", "kb_prod")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got %v", tt.wantErrSub, err)
			}
		})
	}
}

func TestGetChunk(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{
					"hits": []map[string]interface{}{
						{
							"_id": "chunk_xyz",
							"_source": map[string]interface{}{
								"id":      "chunk_xyz",
								"content": "retrieved chunk content",
								"kb_id":   "kb_target",
							},
						},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	res, err := engine.GetChunk(ctx, "ragflow_tenant1", "chunk_xyz", []string{"kb_target"})
	if err != nil {
		t.Fatalf("GetChunk failed: %v", err)
	}
	if res == nil {
		t.Fatalf("Expected chunk to be found, got nil")
	}
	source, ok := res.(map[string]interface{})
	if !ok || source["content"] != "retrieved chunk content" {
		t.Errorf("Unexpected chunk content: %v", source)
	}
}

func TestDropAndExistsChunkStore(t *testing.T) {
	var deleteByQueryCalled bool
	var indexDropCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/ragflow_existing":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead && r.URL.Path == "/ragflow_missing":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/ragflow_existing/_delete_by_query":
			deleteByQueryCalled = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"deleted": 5})
		case r.Method == http.MethodDelete && r.URL.Path == "/ragflow_existing":
			indexDropCalled = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		case r.Method == http.MethodDelete && r.URL.Path == "/ragflow_missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Exists
	exists, err := engine.ChunkStoreExists(ctx, "ragflow_existing", "kb1")
	if err != nil || !exists {
		t.Errorf("ChunkStoreExists expected true, got %v (err=%v)", exists, err)
	}
	exists, err = engine.ChunkStoreExists(ctx, "ragflow_missing", "kb1")
	if err != nil || exists {
		t.Errorf("ChunkStoreExists expected false, got %v (err=%v)", exists, err)
	}

	// 2. Drop scoped dataset (should delete chunks, not drop index)
	if err := engine.DropChunkStore(ctx, "ragflow_existing", "kb1"); err != nil {
		t.Errorf("DropChunkStore existing failed: %v", err)
	}
	if !deleteByQueryCalled {
		t.Errorf("expected DeleteChunks via _delete_by_query to be called for scoped dataset")
	}

	// 3. Drop entire store (unscoped datasetID == "")
	if err := engine.DropChunkStore(ctx, "ragflow_existing", ""); err != nil {
		t.Errorf("DropChunkStore unscoped failed: %v", err)
	}
	if !indexDropCalled {
		t.Errorf("expected dropIndex via DELETE to be called for unscoped dataset")
	}

	// 4. Drop missing (should swallow cleanly)
	if err := engine.DropChunkStore(ctx, "ragflow_missing", "kb1"); err != nil {
		t.Errorf("DropChunkStore missing should not error, got %v", err)
	}
	if err := engine.DropChunkStore(ctx, "ragflow_missing", ""); err != nil {
		t.Errorf("DropChunkStore missing unscoped should not error, got %v", err)
	}
}

func TestMemoryIndexMapping_DynamicPut(t *testing.T) {
	var putMappingCalled bool
	var putMappingField string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/_mapping"):
			w.WriteHeader(http.StatusOK)
			// Return mapping without q_512_vec
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"memory_session1": map[string]interface{}{
					"mappings": map[string]interface{}{
						"properties": map[string]interface{}{
							"id": map[string]interface{}{"type": "keyword"},
						},
					},
				},
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_mapping"):
			putMappingCalled = true
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if props, ok := body["properties"].(map[string]interface{}); ok {
				for k := range props {
					putMappingField = k
				}
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_bulk"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"errors": false, "items": []interface{}{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// Inserting a memory message with q_512_vec triggers dynamic PutMapping
	chunks := []map[string]interface{}{
		{
			"id":        "msg_1",
			"doc_id":    "session_1",
			"content":   "hello world",
			"q_512_vec": make([]float64, 512),
		},
	}
	_, err := engine.InsertChunks(ctx, chunks, "memory_session1", "session_1", "English")
	if err != nil {
		t.Fatalf("InsertChunks memory message failed: %v", err)
	}
	if !putMappingCalled {
		t.Errorf("Expected dynamic PutMapping to be called for memory index")
	}
	if putMappingField != "q_512_vec" {
		t.Errorf("Expected PutMapping field q_512_vec, got %s", putMappingField)
	}
}

func TestChunkIndexMapping_ExistingIndexVectorEnsure(t *testing.T) {
	var putMappingCalled bool
	var putMappingField string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/ragflow_tenant1":
			w.WriteHeader(http.StatusOK) // index exists
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/_mapping"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ragflow_tenant1": map[string]interface{}{
					"mappings": map[string]interface{}{
						"properties": map[string]interface{}{
							"id": map[string]interface{}{"type": "keyword"},
						},
					},
				},
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_mapping"):
			putMappingCalled = true
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if props, ok := body["properties"].(map[string]interface{}); ok {
				for k := range props {
					putMappingField = k
				}
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// Calling CreateChunkStore on existing index with unusual vectorSize=256 ensures mapping
	err := engine.CreateChunkStore(ctx, "ragflow_tenant1", "kb2", 256, "naive", "English")
	if err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}
	if !putMappingCalled {
		t.Errorf("expected PutMapping to be called for existing chunk index with new vectorSize")
	}
	if putMappingField != "q_256_vec" {
		t.Errorf("expected PutMapping field q_256_vec, got %s", putMappingField)
	}
}

func TestCreateChunkStore_MemoryIndexVectorZero(t *testing.T) {
	var putMappingCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK) // Already exists
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_mapping"):
			putMappingCalled = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	// vectorSize = 0 on existing memory index must succeed idempotently without error and without calling putMapping
	err := engine.CreateChunkStore(ctx, "memory_session_zero", "session_zero", 0, "", "")
	if err != nil {
		t.Fatalf("CreateChunkStore with vectorSize 0 on existing memory index failed: %v", err)
	}
	if putMappingCalled {
		t.Errorf("Expected putMapping not to be called when vectorSize == 0")
	}
}

func TestUpdateChunks_ByQuery_ParameterizedNumbers(t *testing.T) {
	var updateBody map[string]interface{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&updateBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"updated": 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	condition := map[string]interface{}{
		"status_int": int64(1),
		"float_num":  float64(3.14),
	}
	newValue := map[string]interface{}{
		"available_int": 1,
		"add":           map[string]interface{}{"categories": "tech"},
	}

	err := engine.UpdateChunks(ctx, condition, newValue, "ragflow_tenant_num", "kb_num")
	if err != nil {
		t.Fatalf("UpdateChunks by query with numeric condition failed: %v", err)
	}

	scriptObj, ok := updateBody["script"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected script object in update body: %v", updateBody)
	}
	src, _ := scriptObj["source"].(string)
	params, _ := scriptObj["params"].(map[string]interface{})

	// available_int must be parameterized: ctx._source.available_int=params.pp_available_int;
	if !strings.Contains(src, "ctx._source.available_int=params.pp_available_int;") {
		t.Errorf("Numeric update must be parameterized to avoid script cache thrashing: %s", src)
	}
	if params["pp_available_int"] != float64(1) && params["pp_available_int"] != 1 {
		t.Errorf("Expected param pp_available_int to be 1, got %v", params["pp_available_int"])
	}

	// add must have null check and ArrayList initialization
	if !strings.Contains(src, "ctx._source.categories = new ArrayList();") {
		t.Errorf("Add script must contain null check and ArrayList initialization: %s", src)
	}
}

func TestMemoryMessageStatusAndVectorProperty(t *testing.T) {
	prop := memoryMessageVectorProperty(1536)
	if prop["type"] != "knn_vector" || prop["dimension"] != 1536 {
		t.Fatalf("unexpected vector property: %v", prop)
	}
	method, ok := prop["method"].(map[string]interface{})
	if !ok || method["name"] != "hnsw" || method["space_type"] != "cosinesimil" {
		t.Fatalf("vector property must nest hnsw and cosinesimil under method for OS 2.10-2.16 compatibility, got %v", prop)
	}

	updateFields := mapMemoryMessageESUpdateFields(map[string]interface{}{
		"status": true,
	})
	if statusInt, ok := updateFields["status_int"].(int); !ok || statusInt != 1 {
		t.Fatalf("expected status_int=1, got %v", updateFields["status_int"])
	}

	condFields := mapMemoryMessageESConditionFields(map[string]interface{}{
		"status": false,
	})
	if statusInt, ok := condFields["status_int"].(int); !ok || statusInt != 0 {
		t.Fatalf("expected status_int=0, got %v", condFields["status_int"])
	}
}

func TestUpdateChunks_ScriptFailureReturnsError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK) // index exists
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{
					"hits": []map[string]interface{}{
						{"_id": "chunk_err_1", "_source": map[string]interface{}{"id": "chunk_err_1"}},
					},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update/chunk_err_1"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["script"]; ok {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"type":   "script_exception",
						"reason": "compile error in Painless script",
					},
					"status": 500,
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "updated"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	condition := map[string]interface{}{"id": "chunk_err_1"}
	newValue := map[string]interface{}{
		"remove": "field_to_remove",
	}

	err := engine.UpdateChunks(ctx, condition, newValue, "ragflow_tenant1", "kb1")
	if err == nil {
		t.Fatalf("expected error when script update fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to remove field") {
		t.Errorf("expected error message to mention failed to remove field, got: %v", err)
	}
}

func TestCreateChunkStore_SkillIndex(t *testing.T) {
	var createdIndex string
	var createdBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/"):
			createdIndex = strings.TrimPrefix(r.URL.Path, "/")
			_ = json.NewDecoder(r.Body).Decode(&createdBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := engine.CreateChunkStore(ctx, "skill_tenant", "skill", 0, "", "")
	if err != nil {
		t.Fatalf("CreateChunkStore for skill index failed: %v", err)
	}
	if createdIndex != "skill_tenant" {
		t.Errorf("Expected index name skill_tenant, got %s", createdIndex)
	}
	mappings, ok := createdBody["mappings"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected mappings in created body, got %v", createdBody)
	}
	properties, ok := mappings["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected properties in mappings, got %v", mappings)
	}
	if _, ok := properties["name_tks"]; !ok {
		t.Errorf("Expected name_tks in skill mapping properties, got %v", properties)
	}
}

func TestCreateChunkStore_ConcurrentExisting(t *testing.T) {
	var headCount int
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			headCount++
			if headCount == 1 {
				// First check: does not exist
				w.WriteHeader(http.StatusNotFound)
				return
			}
			// Second check after create conflict: now exists!
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut:
			if strings.HasSuffix(r.URL.Path, "/_mapping") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"acknowledged": true}`))
				return
			}
			// Concurrent creator won the race and created it first
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"type":   "resource_already_exists_exception",
					"reason": "index already exists",
				},
				"status": 400,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := engine.CreateChunkStore(ctx, "concurrent_idx", "kb1", 1024, "naive", "English")
	if err != nil {
		t.Fatalf("expected concurrent creation to succeed idempotently, got: %v", err)
	}
}

func TestUpdateChunks_UnsupportedConditionType(t *testing.T) {
	engine, server := setupMockEngine(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	condition := map[string]interface{}{
		"bad_field": make(chan int),
	}
	err := engine.UpdateChunks(ctx, condition, map[string]interface{}{"content": "abc"}, "tenant1", "kb1")
	if err == nil {
		t.Fatalf("expected error on unsupported condition type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported condition value type") {
		t.Errorf("expected unsupported condition error, got: %v", err)
	}
}

func TestDeleteChunks_UnsupportedConditionType(t *testing.T) {
	engine, server := setupMockEngine(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	condition := map[string]interface{}{
		"bad_field": make(chan int),
	}
	_, err := engine.DeleteChunks(ctx, condition, "tenant1", "kb1")
	if err == nil {
		t.Fatalf("expected error on unsupported condition type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported condition value type") {
		t.Errorf("expected unsupported condition error, got: %v", err)
	}
}

func TestCreateChunkStore_SkillCustomVectorDimension(t *testing.T) {
	var createdIndex string
	var createdBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			// Skill index does not exist initially
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut:
			createdIndex = strings.TrimPrefix(r.URL.Path, "/")
			_ = json.NewDecoder(r.Body).Decode(&createdBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	// Create skill index with custom dimension 8
	err := engine.CreateChunkStore(ctx, "ragflow_tenant_skill", "skill", 8, "naive", "English")
	if err != nil {
		t.Fatalf("CreateChunkStore skill failed: %v", err)
	}
	if createdIndex != "ragflow_tenant_skill" {
		t.Errorf("Expected created index ragflow_tenant_skill, got %s", createdIndex)
	}
	mappings, ok := createdBody["mappings"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected mappings in created body: %v", createdBody)
	}
	props, ok := mappings["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected properties in mappings: %v", mappings)
	}
	vecProp, ok := props["q_8_vec"].(map[string]interface{})
	if !ok || vecProp["type"] != "knn_vector" || vecProp["dimension"] != float64(8) {
		t.Fatalf("Expected q_8_vec knn_vector with dimension 8, got %v", vecProp)
	}
}

func TestUpdateChunks_PartialFailureAndConflicts(t *testing.T) {
	cases := []struct {
		name      string
		respJSON  string
		expectErr string
	}{
		{
			name:      "timed out",
			respJSON:  `{"took":100,"timed_out":true,"total":10,"updated":5,"batches":1,"version_conflicts":0,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[]}`,
			expectErr: "timed out",
		},
		{
			name:      "has failures",
			respJSON:  `{"took":100,"timed_out":false,"total":10,"updated":5,"batches":1,"version_conflicts":0,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[{"index":"test","type":"_doc","id":"1","cause":{"type":"version_conflict_engine_exception","reason":"conflict"},"status":409}]}`,
			expectErr: "failures",
		},
		{
			name:      "version conflicts",
			respJSON:  `{"took":100,"timed_out":false,"total":10,"updated":5,"batches":1,"version_conflicts":2,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[]}`,
			expectErr: "version conflicts",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "_update_by_query"):
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tc.respJSON))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			engine, server := setupMockEngine(t, handler)
			defer server.Close()

			err := engine.UpdateChunks(context.Background(), map[string]interface{}{"doc_id": "doc1"}, map[string]interface{}{"available_int": 0}, "ragflow_tenant1", "kb1")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.expectErr)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got: %v", tc.expectErr, err)
			}
		})
	}
}

func TestDeleteChunks_KnowledgeGraphSemantics(t *testing.T) {
	var capturedQuery map[string]interface{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "_delete_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&capturedQuery)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"deleted":           5,
				"timed_out":         false,
				"version_conflicts": 0,
				"failures":          []interface{}{},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	_, err := engine.DeleteChunks(ctx, map[string]interface{}{
		"knowledge_graph_kwd": "entity",
	}, "ragflow_tenant", "kb1")
	if err != nil {
		t.Fatalf("DeleteChunks failed: %v", err)
	}

	queryBytes, _ := json.Marshal(capturedQuery)
	queryStr := string(queryBytes)
	if !strings.Contains(queryStr, "type_kwd") || !strings.Contains(queryStr, "knowledge_graph_kwd") {
		t.Errorf("DeleteChunks query should match both type_kwd and knowledge_graph_kwd, got: %s", queryStr)
	}

	// Test empty knowledge_graph_kwd condition produces match_none to prevent scope expansion
	_, err = engine.DeleteChunks(ctx, map[string]interface{}{
		"knowledge_graph_kwd": []string{},
	}, "ragflow_tenant", "kb1")
	if err != nil {
		t.Fatalf("DeleteChunks with empty knowledge_graph_kwd failed: %v", err)
	}
	emptyQueryBytes, _ := json.Marshal(capturedQuery)
	emptyQueryStr := string(emptyQueryBytes)
	if !strings.Contains(emptyQueryStr, "match_none") {
		t.Errorf("DeleteChunks with empty knowledge_graph_kwd should contain match_none, got: %s", emptyQueryStr)
	}
}

func TestUpdateChunks_KnowledgeGraphSemantics(t *testing.T) {
	var capturedQuery map[string]interface{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "_update_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&capturedQuery)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"updated":           3,
				"timed_out":         false,
				"version_conflicts": 0,
				"failures":          []interface{}{},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := engine.updateChunksByQuery(ctx, "ragflow_tenant", map[string]interface{}{
		"knowledge_graph_kwd": []string{"entity", "relation"},
	}, map[string]interface{}{"available_int": 0})
	if err != nil {
		t.Fatalf("updateChunksByQuery failed: %v", err)
	}

	queryBytes, _ := json.Marshal(capturedQuery)
	queryStr := string(queryBytes)
	if !strings.Contains(queryStr, "type_kwd") || !strings.Contains(queryStr, "knowledge_graph_kwd") {
		t.Errorf("updateChunksByQuery should match both type_kwd and knowledge_graph_kwd, got: %s", queryStr)
	}

	// Test empty knowledge_graph_kwd condition produces match_none to prevent scope expansion
	err = engine.updateChunksByQuery(ctx, "ragflow_tenant", map[string]interface{}{
		"knowledge_graph_kwd": []string{},
	}, map[string]interface{}{"available_int": 0})
	if err != nil {
		t.Fatalf("updateChunksByQuery with empty knowledge_graph_kwd failed: %v", err)
	}
	emptyQueryBytes, _ := json.Marshal(capturedQuery)
	emptyQueryStr := string(emptyQueryBytes)
	if !strings.Contains(emptyQueryStr, "match_none") {
		t.Errorf("updateChunksByQuery with empty knowledge_graph_kwd should contain match_none, got: %s", emptyQueryStr)
	}
}

func TestSearchChunk_TimedOutAndShardFailures(t *testing.T) {
	t.Run("searchChunkInDataset timed_out returns error", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"timed_out": true,
				"_shards":   map[string]interface{}{"total": 5, "successful": 3, "failed": 2},
				"hits":      map[string]interface{}{"total": map[string]interface{}{"value": 0}, "hits": []interface{}{}},
			})
		})
		engine, server := setupMockEngine(t, handler)
		defer server.Close()

		_, _, err := engine.searchChunkInDataset(context.Background(), "ragflow_tenant", "chunk_1", "kb_1")
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected timed out error, got: %v", err)
		}
	})

	t.Run("searchChunkInDataset shard failure returns error", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"timed_out": false,
				"_shards":   map[string]interface{}{"total": 5, "successful": 4, "failed": 1},
				"hits":      map[string]interface{}{"total": map[string]interface{}{"value": 0}, "hits": []interface{}{}},
			})
		})
		engine, server := setupMockEngine(t, handler)
		defer server.Close()

		_, _, err := engine.searchChunkInDataset(context.Background(), "ragflow_tenant", "chunk_1", "kb_1")
		if err == nil || !strings.Contains(err.Error(), "shard failures") {
			t.Fatalf("expected shard failures error, got: %v", err)
		}
	})

	t.Run("updateSingleChunk timed_out returns error", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"timed_out": true,
				"_shards":   map[string]interface{}{"total": 5, "successful": 3, "failed": 2},
				"hits":      map[string]interface{}{"total": map[string]interface{}{"value": 0}, "hits": []interface{}{}},
			})
		})
		engine, server := setupMockEngine(t, handler)
		defer server.Close()

		err := engine.updateSingleChunk(context.Background(), "ragflow_tenant", "chunk_1", "kb_1", map[string]interface{}{"available_int": 1})
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected timed out error, got: %v", err)
		}
	})
}

// Compile-time checks
var (
	_ noRefreshChunkInserter = (*Engine)(nil)
	_ chunkPagerankAdjuster  = (*Engine)(nil)
)

type (
	noRefreshChunkInserter interface {
		InsertChunksNoRefresh(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string, language string) ([]string, error)
	}
	chunkPagerankAdjuster interface {
		AdjustChunkPagerank(ctx context.Context, indexName, chunkID, kbID string, delta, minWeight, maxWeight float64) error
	}
)

var _ = errors.New
