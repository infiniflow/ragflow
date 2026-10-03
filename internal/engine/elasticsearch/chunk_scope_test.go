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
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
	"ragflow/internal/engine/types"
)

// TestSingleChunkUpdatePreservesScopeAndFieldValues checks both request boundaries
// and retains text, vector, availability, and field-removal semantics.
func TestSingleChunkUpdatePreservesScopeAndFieldValues(t *testing.T) {
	cases := []struct {
		name                       string
		withDocument, updateFields bool
		remove                     interface{}
	}{
		{"dataset", false, true, map[string]interface{}{"labels": "old"}},
		{"dataset and document", true, true, map[string]interface{}{"labels": "old"}},
		{"remove field only", false, false, "obsolete"},
		{"remove array value only", true, false, map[string]interface{}{"labels": "old"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var searchBody, updateBody map[string]interface{}
			var updateRaw string
			updates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPost && r.URL.Path == "/ragflow_tenant/_search":
					if err := json.NewDecoder(r.Body).Decode(&searchBody); err != nil {
						t.Errorf("decode search: %v", err)
					}
					_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"stored-row"}]}}`))
				case r.Method == http.MethodPost && r.URL.Path == "/ragflow_tenant/_update/stored-row":
					updates++
					var raw json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
						t.Errorf("decode update: %v", err)
					}
					updateRaw = string(raw)
					if err := json.Unmarshal(raw, &updateBody); err != nil {
						t.Errorf("parse update: %v", err)
					}
					if r.URL.Query().Get("refresh") != "wait_for" {
						t.Error("single update lost refresh behavior")
					}
					_, _ = w.Write([]byte(`{"result":"updated"}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{client: client}
			condition := map[string]interface{}{"id": "chunk-1"}
			wantScope := map[string]interface{}{"id": "chunk-1", "kb_id": "kb-1"}
			if tc.withDocument {
				condition["doc_id"] = "doc-1"
				wantScope["doc_id"] = "doc-1"
			}
			content := "Line 1's\nLine 2\r\n"
			values := map[string]interface{}{
				"id": "chunk-1", "available_int": 0, "content_with_weight": content,
				"q_2_vec": []float64{0.25, 0.75}, "tag_feas": map[string]int{"high": 9, "low": 2},
				"remove": tc.remove,
			}
			if !tc.updateFields {
				values = map[string]interface{}{"remove": tc.remove}
			}
			if err := engine.UpdateChunks(t.Context(), condition, values, "ragflow_tenant", "kb-1"); err != nil {
				t.Fatal(err)
			}
			if updates != 1 {
				t.Fatalf("mutation requests = %d, want one atomic update", updates)
			}
			query, ok := searchBody["query"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing search query: %v", searchBody)
			}
			boolQuery, ok := query["bool"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing bounded bool query: %v", query)
			}
			filters, ok := boolQuery["filter"].([]interface{})
			if !ok {
				t.Fatalf("missing search filters: %v", boolQuery)
			}
			gotScope := make(map[string]interface{})
			for _, raw := range filters {
				clause, ok := raw.(map[string]interface{})
				if !ok {
					t.Fatalf("invalid filter: %v", raw)
				}
				term, ok := clause["term"].(map[string]interface{})
				if !ok {
					t.Fatalf("invalid scope term: %v", clause)
				}
				for field, value := range term {
					gotScope[field] = value
				}
			}
			if !reflect.DeepEqual(gotScope, wantScope) {
				t.Fatalf("lookup scope = %v, want %v", gotScope, wantScope)
			}
			script, ok := updateBody["script"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing guarded script: %v", updateBody)
			}
			params, ok := script["params"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing script parameters: %v", script)
			}
			if !reflect.DeepEqual(params["scope"], wantScope) {
				t.Fatalf("mutation scope = %v, want %v", params["scope"], wantScope)
			}
			source, _ := script["source"].(string)
			guard := strings.Index(source, "ctx.op = 'noop'")
			for _, mutation := range []string{"ctx._source.remove", "values.remove", "ctx._source.putAll"} {
				if at := strings.Index(source, mutation); guard < 0 || at < guard {
					t.Fatalf("scope guard must precede %s", mutation)
				}
			}
			doc, ok := params["doc"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing field values: %v", params)
			}
			if _, ok := doc["id"]; ok {
				t.Fatal("row id must not be overwritten")
			}
			if _, ok := doc["remove"]; ok {
				t.Fatal("removal directive must not be stored")
			}
			wantRemovedFields := []interface{}{}
			wantRemovedValues := map[string]interface{}{}
			if tc.updateFields {
				wantRemovedFields = append(wantRemovedFields, "tag_feas")
			}
			switch removal := tc.remove.(type) {
			case string:
				wantRemovedFields = append(wantRemovedFields, removal)
			case map[string]interface{}:
				wantRemovedValues = removal
			}
			if !reflect.DeepEqual(params["remove_fields"], wantRemovedFields) {
				t.Fatalf("field-removal parameters changed: %v", params)
			}
			if !reflect.DeepEqual(params["remove_values"], wantRemovedValues) {
				t.Fatalf("array-removal parameters changed: %v", params)
			}
			if !tc.updateFields {
				if len(doc) != 0 {
					t.Fatalf("removal-only operation added fields: %v", doc)
				}
				return
			}
			if doc["content_with_weight"] != content {
				t.Fatalf("content was changed: %#v", doc["content_with_weight"])
			}
			if doc["available_int"] != float64(0) {
				t.Fatalf("zero availability was lost: %v", doc)
			}
			if !reflect.DeepEqual(doc["q_2_vec"], []interface{}{0.25, 0.75}) {
				t.Fatalf("vector changed: %v", doc)
			}
			if !strings.Contains(updateRaw, `"tag_feas":{"high":9,"low":2}`) {
				t.Fatalf("ordered tag scores lost: %s", updateRaw)
			}

		})
	}
}

// TestSingleChunkUpdateRejectsMissingOrChangedScope ensures failures cannot be
// reported as successful chunk mutations after the lookup or final scope check.
func TestSingleChunkUpdateRejectsMissingOrChangedScope(t *testing.T) {
	cases := []struct {
		name         string
		found        bool
		status       int
		response     string
		wantNotFound bool
	}{
		{"missing scoped row", false, 200, `{"result":"updated"}`, true},
		{"scope changed before mutation", true, 200, `{"result":"noop"}`, true},
		{"row removed before mutation", true, 404, `{"error":"missing"}`, true},
		{"mutation failure", true, 500, `{"error":"failure"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.URL.Path == "/ragflow_tenant/_search":
					if tc.found {
						_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"stored-row"}]}}`))
					} else {
						_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
					}
				case r.URL.Path == "/ragflow_tenant/_update/stored-row":
					updates++
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.response))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{client: client}
			err = engine.UpdateChunks(t.Context(), map[string]interface{}{"id": "chunk-1", "doc_id": "doc-1"}, map[string]interface{}{"available_int": 1}, "ragflow_tenant", "kb-1")
			if err == nil {
				t.Fatal("expected mutation to be rejected")
			}
			if errors.Is(err, types.ErrDocumentNotFound) != tc.wantNotFound {
				t.Fatalf("unexpected error category: %v", err)
			}
			if !tc.found && updates != 0 {
				t.Fatalf("missing scoped row triggered %d mutations", updates)
			}
		})
	}
}

// TestSingleChunkUpdateRejectsInvalidScope prevents malformed explicit scope
// fields from being silently dropped before a single-chunk lookup.
func TestSingleChunkUpdateRejectsInvalidScope(t *testing.T) {
	for _, field := range []string{"id", "kb_id", "doc_id"} {
		t.Run(field, func(t *testing.T) {
			engine := &Engine{}
			condition := map[string]interface{}{"id": "chunk-1", "kb_id": "kb-1", "doc_id": "doc-1"}
			condition[field] = ""
			if err := engine.updateSingleChunk(t.Context(), "ragflow_tenant", condition, map[string]interface{}{"available_int": 1}); err == nil {
				t.Fatal("expected invalid scope to be rejected before a network call")
			}
		})
	}
}
