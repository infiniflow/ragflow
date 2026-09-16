//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package elasticsearch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
)

func newMetadataTestEngine(t *testing.T, handler http.HandlerFunc) *Engine {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	return &Engine{client: client}
}

func writeMetadataOK(w http.ResponseWriter) {
	w.Header().Set("X-Elastic-Product", "Elasticsearch")
	w.Header().Set("Content-Type", "application/json")
}

func TestInsertMetadataWaitsForRefresh(t *testing.T) {
	var gotRefresh string
	engine := newMetadataTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/_bulk" {
			t.Errorf("request = %s %s, want POST /_bulk", r.Method, r.URL.Path)
		}
		gotRefresh = r.URL.Query().Get("refresh")
		writeMetadataOK(w)
		_, _ = w.Write([]byte(`{"errors":false}`))
	})
	if _, err := engine.InsertMetadata(t.Context(), []map[string]interface{}{{
		"id": "doc1", "kb_id": "kb1", "meta_fields": map[string]interface{}{"author": "Alice"},
	}}, "tenant1"); err != nil {
		t.Fatalf("InsertMetadata: %v", err)
	}
	if gotRefresh != "wait_for" {
		t.Fatalf("refresh=%q, want wait_for", gotRefresh)
	}
}

func TestInsertMetadataDeferredSkipsRefresh(t *testing.T) {
	var gotRefresh string
	engine := newMetadataTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/_bulk" {
			t.Errorf("request = %s %s, want POST /_bulk", r.Method, r.URL.Path)
		}
		gotRefresh = r.URL.Query().Get("refresh")
		writeMetadataOK(w)
		_, _ = w.Write([]byte(`{"errors":false}`))
	})
	if _, err := engine.InsertMetadataDeferred(t.Context(), []map[string]interface{}{{
		"id": "doc1", "kb_id": "kb1", "meta_fields": map[string]interface{}{"author": "Alice"},
	}}, "tenant1"); err != nil {
		t.Fatalf("InsertMetadataDeferred: %v", err)
	}
	if gotRefresh != "" {
		t.Fatalf("refresh=%q, want no refresh param", gotRefresh)
	}
}

func TestUpdateMetadataRefreshesAfterWrite(t *testing.T) {
	var gotRefresh string
	engine := newMetadataTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/ragflow_doc_meta_tenant1/_update_by_query" {
			t.Errorf("request = %s %s, want POST /ragflow_doc_meta_tenant1/_update_by_query", r.Method, r.URL.Path)
		}
		gotRefresh = r.URL.Query().Get("refresh")
		writeMetadataOK(w)
		_, _ = w.Write([]byte(`{"total":1}`))
	})
	if err := engine.UpdateMetadata(t.Context(), "doc1", "kb1", map[string]interface{}{"author": "Alice"}, "tenant1"); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	if gotRefresh != "true" {
		t.Fatalf("refresh=%q, want true", gotRefresh)
	}
}

func TestUpdateMetadataDeferredSkipsRefresh(t *testing.T) {
	var gotRefresh string
	engine := newMetadataTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/ragflow_doc_meta_tenant1/_update_by_query" {
			t.Errorf("request = %s %s, want POST /ragflow_doc_meta_tenant1/_update_by_query", r.Method, r.URL.Path)
		}
		gotRefresh = r.URL.Query().Get("refresh")
		writeMetadataOK(w)
		_, _ = w.Write([]byte(`{"total":1}`))
	})
	if err := engine.UpdateMetadataDeferred(t.Context(), "doc1", "kb1", map[string]interface{}{"author": "Alice"}, "tenant1"); err != nil {
		t.Fatalf("UpdateMetadataDeferred: %v", err)
	}
	if gotRefresh != "false" {
		t.Fatalf("refresh=%q, want false", gotRefresh)
	}
}

func TestUpdateMetadataDeferredInsertsWithoutRefreshWhenMissing(t *testing.T) {
	paths := []string{}
	refreshes := []string{}
	engine := newMetadataTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		refreshes = append(refreshes, r.URL.Query().Get("refresh"))
		writeMetadataOK(w)
		if r.URL.Path == "/ragflow_doc_meta_tenant1/_update_by_query" {
			_, _ = w.Write([]byte(`{"total":0}`)) // no existing row -> insert branch
			return
		}
		_, _ = w.Write([]byte(`{"errors":false}`))
	})
	if err := engine.UpdateMetadataDeferred(t.Context(), "doc1", "kb1", map[string]interface{}{"author": "Alice"}, "tenant1"); err != nil {
		t.Fatalf("UpdateMetadataDeferred: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want update_by_query then bulk", paths)
	}
	if refreshes[0] != "false" {
		t.Fatalf("update_by_query refresh=%q, want false", refreshes[0])
	}
	if refreshes[1] != "" {
		t.Fatalf("insert refresh=%q, want no refresh param", refreshes[1])
	}
}

func TestRefreshMetadataIndexTargetsTenantIndex(t *testing.T) {
	var gotPath string
	engine := newMetadataTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeMetadataOK(w)
		_, _ = w.Write([]byte(`{"_shards":{"successful":1,"failed":0}}`))
	})
	if err := engine.RefreshMetadataIndex(t.Context(), "tenant1"); err != nil {
		t.Fatalf("RefreshMetadataIndex: %v", err)
	}
	if gotPath != "/ragflow_doc_meta_tenant1/_refresh" {
		t.Fatalf("path = %q, want /ragflow_doc_meta_tenant1/_refresh", gotPath)
	}
}
