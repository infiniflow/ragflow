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
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateMetadataStore_RewriteDynamicRuntime(t *testing.T) {
	var createdIndex string
	var createdBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "ragflow_doc_meta_"):
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
	err := engine.CreateMetadataStore(ctx, "tenant_abc")
	if err != nil {
		t.Fatalf("CreateMetadataStore failed: %v", err)
	}

	if createdIndex != "ragflow_doc_meta_tenant_abc" {
		t.Errorf("Expected index name ragflow_doc_meta_tenant_abc, got %s", createdIndex)
	}

	mappings, ok := createdBody["mappings"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected mappings block in createdBody: %v", createdBody)
	}
	// "dynamic": "runtime" must have been rewritten to boolean true
	if mappings["dynamic"] != true {
		t.Errorf("Expected dynamic to be boolean true, got %v", mappings["dynamic"])
	}
}

func TestInsertMetadata_BatchesAndValidation(t *testing.T) {
	var bulkBody string
	var refreshParam string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_bulk") {
			refreshParam = r.URL.Query().Get("refresh")
			bodyBytes, _ := io.ReadAll(r.Body)
			bulkBody = string(bodyBytes)
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

	// Docs: doc1 valid, doc2 missing kb_id (should be skipped), doc3 valid
	metadata := []map[string]interface{}{
		{
			"id":          "doc_1",
			"kb_id":       "kb_1",
			"meta_fields": map[string]interface{}{"author": "Alice"},
		},
		{
			"id":          "doc_2",
			"meta_fields": map[string]interface{}{"author": "Bob"},
		},
		{
			"id":          "doc_3",
			"kb_id":       "kb_1",
			"meta_fields": map[string]interface{}{"author": "Charlie"},
		},
	}

	_, err := engine.InsertMetadata(ctx, metadata, "tenant_1")
	if err != nil {
		t.Fatalf("InsertMetadata failed: %v", err)
	}

	if refreshParam != "wait_for" {
		t.Errorf("Expected refresh=wait_for, got %s", refreshParam)
	}
	if !strings.Contains(bulkBody, `"doc_1"`) || !strings.Contains(bulkBody, `"doc_3"`) {
		t.Errorf("Expected bulk body to contain doc_1 and doc_3")
	}
	if strings.Contains(bulkBody, `"doc_2"`) {
		t.Errorf("doc_2 missing kb_id should have been skipped")
	}

	// 2. All documents invalid -> must return empty slice and nil error without issuing bulk request
	invalidOnly := []map[string]interface{}{
		{"id": "inv_1"},                 // missing kb_id
		{"kb_id": "kb_1"},               // missing id
		{"id": "", "kb_id": "kb_1"},     // empty id
		{"id": "inv_2", "kb_id": "   "}, // whitespace kb_id
		{"id": 12345, "kb_id": "kb_1"},  // non-string id
	}
	resEmpty, errEmpty := engine.InsertMetadata(ctx, invalidOnly, "tenant_1")
	if errEmpty != nil || resEmpty == nil {
		t.Errorf("Expected InsertMetadata on all-invalid docs to return empty slice, nil; got %v, %v", resEmpty, errEmpty)
	}
}

func TestUpdateMetadata_ScriptAndUpsert(t *testing.T) {
	var updateByQueryBody map[string]interface{}
	var updatedCount int

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&updateByQueryBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total":   updatedCount,
				"updated": updatedCount,
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_bulk"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": false,
				"items":  []interface{}{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Existing doc update
	updatedCount = 1
	err := engine.UpdateMetadata(ctx, "doc_100", "kb_100", map[string]interface{}{"department": "Engineering"}, "tenant_x")
	if err != nil {
		t.Fatalf("UpdateMetadata existing doc failed: %v", err)
	}
	scriptObj := updateByQueryBody["script"].(map[string]interface{})
	src := scriptObj["source"].(string)
	if !strings.Contains(src, "ctx._source.meta_fields = params.meta_fields;") {
		t.Errorf("Expected full replacement painless script, got: %s", src)
	}

	// 2. Non-existent doc update triggers upsert via InsertMetadata
	updatedCount = 0
	err = engine.UpdateMetadata(ctx, "doc_new", "kb_new", map[string]interface{}{"department": "Sales"}, "tenant_x")
	if err != nil {
		t.Fatalf("UpdateMetadata upsert failed: %v", err)
	}
}

func TestDeleteMetadata_ConditionsAndSafety(t *testing.T) {
	var deleteByQueryBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_delete_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&deleteByQueryBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"deleted": 3,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Normal delete by condition
	deleted, err := engine.DeleteMetadata(ctx, map[string]interface{}{
		"kb_id": "kb_to_delete",
	}, "tenant_del")
	if err != nil {
		t.Fatalf("DeleteMetadata failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("Expected 3 deleted docs, got %d", deleted)
	}

	query := deleteByQueryBody["query"].(map[string]interface{})
	term := query["term"].(map[string]interface{})
	if term["kb_id"] != "kb_to_delete" {
		t.Errorf("Expected term query kb_id: kb_to_delete, got %v", term)
	}

	// 2. Non-empty condition yielding nil query safety abort
	_, err = engine.DeleteMetadata(ctx, map[string]interface{}{
		"nil_key": nil,
	}, "tenant_del")
	if err == nil {
		t.Errorf("Expected non-empty condition yielding nil query to abort, got nil")
	}
}

func TestDeleteMetadata_PartialFailure(t *testing.T) {
	tests := []struct {
		name       string
		respBody   map[string]interface{}
		wantErrSub string
	}{
		{
			name: "timed out",
			respBody: map[string]interface{}{
				"deleted":   5,
				"total":     8,
				"timed_out": true,
			},
			wantErrSub: "timed out",
		},
		{
			name: "version conflicts",
			respBody: map[string]interface{}{
				"deleted":           3,
				"version_conflicts": 1,
			},
			wantErrSub: "version conflicts",
		},
		{
			name: "failures",
			respBody: map[string]interface{}{
				"deleted": 2,
				"failures": []interface{}{
					map[string]interface{}{"index": "ragflow_doc_meta_tenant_del", "reason": "failure"},
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
			_, err := engine.DeleteMetadata(ctx, map[string]interface{}{"kb_id": "kb_to_delete"}, "tenant_del")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got %v", tt.wantErrSub, err)
			}
		})
	}
}

func TestDeleteMetadataKeys(t *testing.T) {
	var currentMeta map[string]interface{}
	var lastUpdateByQuery map[string]interface{}
	var lastDeleteByQuery map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			if strings.Contains(r.URL.Path, "missing_tenant") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
			w.WriteHeader(http.StatusOK)
			hits := []map[string]interface{}{}
			if currentMeta != nil {
				hits = append(hits, map[string]interface{}{
					"_id": "doc_meta_key",
					"_source": map[string]interface{}{
						"id":          "doc_meta_key",
						"kb_id":       "kb_key",
						"meta_fields": currentMeta,
					},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{
					"hits": hits,
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&lastUpdateByQuery)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"updated": 1})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_delete_by_query"):
			_ = json.NewDecoder(r.Body).Decode(&lastDeleteByQuery)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"deleted": 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. Partial keys removal (remaining keys > 0) -> triggers UpdateByQuery
	currentMeta = map[string]interface{}{
		"tag1": "val1",
		"tag2": "val2",
	}
	err := engine.DeleteMetadataKeys(ctx, "doc_meta_key", "kb_key", []string{"tag1"}, "tenant_k")
	if err != nil {
		t.Fatalf("DeleteMetadataKeys partial failed: %v", err)
	}
	if lastUpdateByQuery == nil {
		t.Errorf("Expected UpdateByQuery to be executed for partial keys removal")
	}

	// 2. All keys removed (remaining keys == 0) -> triggers document deletion
	currentMeta = map[string]interface{}{
		"tag1": "val1",
	}
	lastDeleteByQuery = nil
	err = engine.DeleteMetadataKeys(ctx, "doc_meta_key", "kb_key", []string{"tag1"}, "tenant_k")
	if err != nil {
		t.Fatalf("DeleteMetadataKeys all keys failed: %v", err)
	}
	if lastDeleteByQuery == nil {
		t.Errorf("Expected DeleteByQuery to delete document when all keys removed")
	}

	// 3. Document not found
	currentMeta = nil
	err = engine.DeleteMetadataKeys(ctx, "doc_not_found", "kb_key", []string{"tag1"}, "tenant_k")
	if !errors.Is(err, types.ErrDocumentNotFound) {
		t.Errorf("Expected types.ErrDocumentNotFound, got %v", err)
	}

	// 4. Index not found
	err = engine.DeleteMetadataKeys(ctx, "doc_meta_key", "kb_key", []string{"tag1"}, "missing_tenant")
	if !errors.Is(err, types.ErrIndexNotFound) {
		t.Errorf("Expected types.ErrIndexNotFound, got %v", err)
	}
}

func TestSearchMetadata(t *testing.T) {
	var searchDSL map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
			_ = json.NewDecoder(r.Body).Decode(&searchDSL)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{
						"value": 42,
					},
					"hits": []map[string]interface{}{
						{
							"_id": "meta_1",
							"_source": map[string]interface{}{
								"id":    "meta_1",
								"kb_id": "kb_search",
							},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()
	req := &types.SearchMetadataRequest{
		TenantID:     "tenant_search",
		Filter:       map[string]interface{}{"kb_id": "kb_search"},
		SelectFields: []string{"id", "kb_id"},
		Limit:        10,
		Offset:       5,
		OrderBy: &types.OrderByExpr{
			Fields: []types.OrderByField{
				{Field: "create_time_flt", Type: types.SortDesc},
			},
		},
	}

	result, err := engine.SearchMetadata(ctx, req)
	if err != nil {
		t.Fatalf("SearchMetadata failed: %v", err)
	}
	if result.Total != 42 {
		t.Errorf("Expected total 42, got %d", result.Total)
	}
	if len(result.MetadataRecords) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(result.MetadataRecords))
	}
	if result.MetadataRecords[0]["id"] != "meta_1" {
		t.Errorf("Expected record id meta_1, got %v", result.MetadataRecords[0]["id"])
	}

	// Verify DSL fields
	if searchDSL["size"] != float64(10) || searchDSL["from"] != float64(5) {
		t.Errorf("DSL pagination mismatch: size=%v, from=%v", searchDSL["size"], searchDSL["from"])
	}
	if searchDSL["track_total_hits"] != true {
		t.Errorf("Expected track_total_hits=true in search DSL")
	}
}

func TestSearchMetadata_TimeoutAndShardFailure(t *testing.T) {
	tests := []struct {
		name       string
		respBody   map[string]interface{}
		wantErrSub string
	}{
		{
			name: "timed out",
			respBody: map[string]interface{}{
				"timed_out": true,
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": 10},
					"hits":  []interface{}{},
				},
			},
			wantErrSub: "timed out",
		},
		{
			name: "shard failure",
			respBody: map[string]interface{}{
				"timed_out": false,
				"_shards": map[string]interface{}{
					"total":      2,
					"successful": 1,
					"failed":     1,
				},
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": 5},
					"hits":  []interface{}{},
				},
			},
			wantErrSub: "failed shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
					if r.URL.Query().Get("allow_partial_search_results") != "false" {
						t.Errorf("Expected allow_partial_search_results=false, got %s", r.URL.Query().Get("allow_partial_search_results"))
					}
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.respBody)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			engine, server := setupMockEngine(t, handler)
			defer server.Close()

			ctx := context.Background()
			req := &types.SearchMetadataRequest{
				TenantID: "tenant_search",
				Filter:   map[string]interface{}{"kb_id": "kb_search"},
			}
			_, err := engine.SearchMetadata(ctx, req)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got %v", tt.wantErrSub, err)
			}
		})
	}
}

func TestDocumentAndBulkIndex(t *testing.T) {
	var indexedDocID string
	var deletedDocID string
	var bulkIndexedCount int

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_doc/doc_single"):
			indexedDocID = "doc_single"
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "created"})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/_doc/doc_delete"):
			deletedDocID = "doc_delete"
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "deleted"})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/_doc/doc_not_found"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/_doc/doc_get"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"found": true,
				"_source": map[string]interface{}{
					"title": "Document Title",
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_bulk"):
			bulkIndexedCount = 2
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"took":   15,
				"errors": false,
				"items":  []interface{}{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. IndexDocument
	err := engine.IndexDocument(ctx, "skill_index", "doc_single", map[string]interface{}{"title": "Single Doc"})
	if err != nil {
		t.Fatalf("IndexDocument failed: %v", err)
	}
	if indexedDocID != "doc_single" {
		t.Errorf("Expected doc_single to be indexed")
	}

	// 2. DeleteDocument
	err = engine.DeleteDocument(ctx, "skill_index", "doc_delete")
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}
	if deletedDocID != "doc_delete" {
		t.Errorf("Expected doc_delete to be deleted")
	}
	err = engine.DeleteDocument(ctx, "skill_index", "doc_not_found")
	if !errors.Is(err, types.ErrDocumentNotFound) {
		t.Errorf("Expected types.ErrDocumentNotFound, got %v", err)
	}

	// 3. GetDocument
	docSource, err := engine.GetDocument(ctx, "skill_index", "doc_get")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	srcMap := docSource.(map[string]interface{})
	if srcMap["title"] != "Document Title" {
		t.Errorf("Expected title 'Document Title', got %v", srcMap["title"])
	}
	_, err = engine.GetDocument(ctx, "skill_index", "doc_not_found")
	if !errors.Is(err, types.ErrDocumentNotFound) {
		t.Errorf("Expected types.ErrDocumentNotFound for missing doc, got %v", err)
	}

	// 4. BulkIndex
	docs := []interface{}{
		map[string]interface{}{"_id": "b1", "val": 1},
		map[string]interface{}{"_id": "b2", "val": 2},
	}
	respObj, err := engine.BulkIndex(ctx, "skill_index", docs)
	if err != nil {
		t.Fatalf("BulkIndex failed: %v", err)
	}
	bulkResp, ok := respObj.(*BulkResponse)
	if !ok || bulkResp.Indexed != 2 || bulkResp.Errors {
		t.Errorf("Unexpected BulkResponse: %+v", respObj)
	}
	if bulkIndexedCount != 2 {
		t.Errorf("Expected 2 bulk indexed documents")
	}

	// 5. BulkIndex with missing / nil _id fails
	invalidDocs := []interface{}{
		map[string]interface{}{"val": 1},
	}
	if _, err := engine.BulkIndex(ctx, "skill_index", invalidDocs); err == nil {
		t.Errorf("Expected error for document missing _id, got nil")
	}
	nilIDDocs := []interface{}{
		map[string]interface{}{"_id": nil, "val": 1},
	}
	if _, err := engine.BulkIndex(ctx, "skill_index", nilIDDocs); err == nil {
		t.Errorf("Expected error for document with nil _id, got nil")
	}
}

func TestDropAndExistsMetadataStore(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/ragflow_doc_meta_tenant_live":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead && r.URL.Path == "/ragflow_doc_meta_tenant_absent":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodDelete && r.URL.Path == "/ragflow_doc_meta_tenant_live":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"acknowledged": true})
		case r.Method == http.MethodDelete && r.URL.Path == "/ragflow_doc_meta_tenant_absent":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	ctx := context.Background()

	// 1. MetadataStoreExists
	exists, err := engine.MetadataStoreExists(ctx, "tenant_live")
	if err != nil || !exists {
		t.Errorf("Expected MetadataStoreExists to be true, got %v, %v", exists, err)
	}
	exists, err = engine.MetadataStoreExists(ctx, "tenant_absent")
	if err != nil || exists {
		t.Errorf("Expected MetadataStoreExists to be false, got %v, %v", exists, err)
	}

	// 2. DropMetadataStore (existing index and 404 absent index both return nil)
	if err := engine.DropMetadataStore(ctx, "tenant_live"); err != nil {
		t.Errorf("Expected DropMetadataStore on existing index to succeed, got %v", err)
	}
	if err := engine.DropMetadataStore(ctx, "tenant_absent"); err != nil {
		t.Errorf("Expected DropMetadataStore on absent index to tolerate 404 and return nil, got %v", err)
	}
}

func TestFilterDocIdsByMetaPushdown_Contract(t *testing.T) {
	ctx := context.Background()

	// 1. Nil DB -> must return nil
	engineNoDB, serverNoDB := setupMockEngine(t, func(w http.ResponseWriter, r *http.Request) {})
	defer serverNoDB.Close()
	if res := engineNoDB.FilterDocIdsByMetaPushdown(ctx, nil, []string{"kb1"}, []map[string]interface{}{{"key": "tag", "op": "=", "value": "v"}}, "and"); res != nil {
		t.Errorf("Nil DB must return nil, got %v", res)
	}

	// 2. Empty conditions -> must return nil
	if res := engineNoDB.FilterDocIdsByMetaPushdown(ctx, nil, []string{"kb1"}, []map[string]interface{}{}, "and"); res != nil {
		t.Errorf("Empty conditions must return nil, got %v", res)
	}

	// 3. Empty kbIDs -> must return nil
	if res := engineNoDB.FilterDocIdsByMetaPushdown(ctx, nil, []string{}, []map[string]interface{}{{"key": "tag", "op": "=", "value": "v"}}, "and"); res != nil {
		t.Errorf("Empty kbIDs must return nil, got %v", res)
	}

	// 4. Unsupported operator -> must return nil
	if res := engineNoDB.FilterDocIdsByMetaPushdown(ctx, nil, []string{"kb1"}, []map[string]interface{}{{"key": "tag", "op": "unsupported_op", "value": "v"}}, "and"); res != nil {
		t.Errorf("Unsupported operator must return nil, got %v", res)
	}

	// Set up SQLite memory DB with knowledge bases
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	status := string(entity.StatusValid)
	kbs := []entity.Knowledgebase{
		{ID: "kb-a", TenantID: "tenant-a", Status: &status},
		{ID: "kb-b", TenantID: "tenant-b", Status: &status},
	}
	if err := db.Create(&kbs).Error; err != nil {
		t.Fatal(err)
	}

	var totalHits int64
	var returnedHits []map[string]interface{}
	var missingIndex bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			if missingIndex && strings.Contains(r.URL.Path, "tenant-b") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{
						"value": totalHits,
					},
					"hits": returnedHits,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	engine, server := setupMockEngine(t, handler)
	defer server.Close()

	conditions := []map[string]interface{}{{"key": "author", "op": "=", "value": "Alice"}}

	// 5. Successful pushdown returning doc IDs
	totalHits = 2
	returnedHits = []map[string]interface{}{
		{"_id": "doc-1", "_source": map[string]interface{}{"id": "doc-1"}},
		{"_id": "doc-2", "_source": map[string]interface{}{"id": "doc-2"}},
	}
	missingIndex = false
	got := engine.FilterDocIdsByMetaPushdown(ctx, db, []string{"kb-a", "kb-b"}, conditions, "and")
	if !reflect.DeepEqual(got, []string{"doc-1", "doc-2"}) {
		t.Errorf("Expected [doc-1 doc-2], got %v", got)
	}

	// 6. Pushdown succeeds but found 0 matching docs -> must return []string{} (not nil!)
	totalHits = 0
	returnedHits = []map[string]interface{}{}
	gotZero := engine.FilterDocIdsByMetaPushdown(ctx, db, []string{"kb-a", "kb-b"}, conditions, "and")
	if gotZero == nil {
		t.Errorf("Pushdown with 0 hits must return empty slice []string{}, not nil")
	}
	if len(gotZero) != 0 {
		t.Errorf("Expected 0 elements, got %d", len(gotZero))
	}

	// 7. Overflow exceeds pushdown cap (10000) -> must return nil (fallback)
	totalHits = 10001
	returnedHits = []map[string]interface{}{}
	gotOverflow := engine.FilterDocIdsByMetaPushdown(ctx, db, []string{"kb-a", "kb-b"}, conditions, "and")
	if gotOverflow != nil {
		t.Errorf("Result exceeding pushdown cap must return nil (fallback), got %v", gotOverflow)
	}

	// 8. Missing tenant metadata index -> must return nil (fallback)
	missingIndex = true
	totalHits = 1
	gotMissing := engine.FilterDocIdsByMetaPushdown(ctx, db, []string{"kb-a", "kb-b"}, conditions, "and")
	if gotMissing != nil {
		t.Errorf("Missing index must return nil (fallback), got %v", gotMissing)
	}
}

func TestUpdateMetadata_VersionConflictDoesNotOverwrite(t *testing.T) {
	var bulkCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update_by_query"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return version conflict: total=1, updated=0, version_conflicts=1
			_, _ = w.Write([]byte(`{"took":50,"timed_out":false,"total":1,"updated":0,"batches":1,"version_conflicts":1,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[]}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_bulk"):
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
	err := engine.UpdateMetadata(ctx, "doc_conflict", "kb_1", map[string]interface{}{"status": "pending"}, "tenant_x")
	if err == nil {
		t.Fatalf("expected error on version conflict, got nil")
	}
	if !strings.Contains(err.Error(), "version conflict") {
		t.Errorf("expected version conflict error, got: %v", err)
	}
	if bulkCalled {
		t.Errorf("UpdateMetadata must NOT call bulk insert/upsert on version conflict")
	}
}

func TestDeleteMetadataKeys_FailuresAndConflicts(t *testing.T) {
	cases := []struct {
		name      string
		respJSON  string
		expectErr string
	}{
		{
			name:      "timed out",
			respJSON:  `{"took":50,"timed_out":true,"total":1,"updated":0,"batches":1,"version_conflicts":0,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[]}`,
			expectErr: "timed out",
		},
		{
			name:      "failures",
			respJSON:  `{"took":50,"timed_out":false,"total":1,"updated":0,"batches":1,"version_conflicts":0,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[{"index":"test","type":"_doc","id":"1","cause":{"type":"circuit_breaking_exception","reason":"data too large"},"status":429}]}`,
			expectErr: "failures",
		},
		{
			name:      "version conflicts",
			respJSON:  `{"took":50,"timed_out":false,"total":1,"updated":0,"batches":1,"version_conflicts":1,"noops":0,"retries":{"bulk":0,"search":0},"throttled_millis":0,"requests_per_second":-1.0,"throttled_until_millis":0,"failures":[]}`,
			expectErr: "version conflicts",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_search"):
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
						"took": 1,
						"timed_out": false,
						"_shards": {"total": 1, "successful": 1, "skipped": 0, "failed": 0},
						"hits": {
							"total": {"value": 1, "relation": "eq"},
							"hits": [{
								"_id": "doc1",
								"_source": {
									"id": "doc1",
									"kb_id": "kb1",
									"meta_fields": {"temp_key": "v1", "keep_key": "v2"}
								}
							}]
						}
					}`))
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/_update_by_query"):
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tc.respJSON))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			engine, server := setupMockEngine(t, handler)
			defer server.Close()

			err := engine.DeleteMetadataKeys(context.Background(), "doc1", "kb1", []string{"temp_key"}, "tenant_x")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.expectErr)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got: %v", tc.expectErr, err)
			}
		})
	}
}
