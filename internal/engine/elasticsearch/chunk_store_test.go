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

	"github.com/elastic/go-elasticsearch/v8"

	"ragflow/internal/common"
)

// vectorMappingServer answers the three requests CreateChunkStore makes for a
// tenant index: the existence probe, the mapping read, and the mapping write.
func vectorMappingServer(t *testing.T, indexExists bool, existing map[string]interface{}) (*httptest.Server, *map[string]interface{}) {
	t.Helper()
	putBody := map[string]interface{}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodHead:
			if indexExists {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/_mapping"):
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("decode mapping body: %v", err)
			}
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		case r.Method == http.MethodPut:
			// Index creation. The default mapping carries no vector field.
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/_mapping"):
			mapping := map[string]interface{}{
				"ragflow_tenant": map[string]interface{}{
					"mappings": map[string]interface{}{
						"properties": map[string]interface{}{},
					},
				},
			}
			if existing != nil {
				mapping["ragflow_tenant"].(map[string]interface{})["mappings"].(map[string]interface{})["properties"] = existing
			}
			_ = json.NewEncoder(w).Encode(mapping)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, &putBody
}

func newVectorMappingEngine(t *testing.T, server *httptest.Server) *Engine {
	t.Helper()
	if err := common.InitLogger("info", common.FileOutput{}, "elasticsearch_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("new elasticsearch client: %v", err)
	}
	return &Engine{client: client}
}

func mappedVectorField(t *testing.T, body *map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	properties, ok := (*body)["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("mapping body has no properties: %#v", *body)
	}
	property, ok := properties[field].(map[string]interface{})
	if !ok {
		t.Fatalf("mapping body has no %s field: %#v", field, properties)
	}
	return property
}

// Elasticsearch infers dense_vector only for float arrays of 128 values or
// more. An existing tenant index must therefore map the new model's field
// explicitly, or a smaller model would be stored as a numeric field and every
// kNN search on it would fail.
func TestCreateChunkStoreMapsNewVectorFieldOnExistingIndex(t *testing.T) {
	server, putBody := vectorMappingServer(t, true, map[string]interface{}{
		"tag_feas": map[string]interface{}{"type": "rank_features"},
	})
	engine := newVectorMappingEngine(t, server)

	if err := engine.CreateChunkStore(t.Context(), "ragflow_tenant", "kb-1", 64, "naive", "English"); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}
	property := mappedVectorField(t, putBody, "q_64_vec")
	if property["type"] != "dense_vector" {
		t.Fatalf("type = %v, want dense_vector", property["type"])
	}
	if dims, ok := common.GetInt(property["dims"]); !ok || dims != 64 {
		t.Fatalf("dims = %v, want 64", property["dims"])
	}
	if property["similarity"] != "cosine" {
		t.Fatalf("similarity = %v, want cosine", property["similarity"])
	}
}

// A field that already holds the right dense_vector mapping needs no write.
func TestCreateChunkStoreKeepsCompatibleVectorMapping(t *testing.T) {
	server, putBody := vectorMappingServer(t, true, map[string]interface{}{
		"q_64_vec": map[string]interface{}{"type": "dense_vector", "dims": 64, "index": true, "similarity": "cosine"},
	})
	engine := newVectorMappingEngine(t, server)

	if err := engine.CreateChunkStore(t.Context(), "ragflow_tenant", "kb-1", 64, "naive", "English"); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}
	if len(*putBody) != 0 {
		t.Fatalf("unexpected mapping update: %#v", *putBody)
	}
}

// A field mapped as a plain number cannot be searched as a vector. Report that
// while the model is being switched instead of at the first kNN search.
func TestCreateChunkStoreRejectsIncompatibleVectorMapping(t *testing.T) {
	server, putBody := vectorMappingServer(t, true, map[string]interface{}{
		"q_64_vec": map[string]interface{}{"type": "float"},
	})
	engine := newVectorMappingEngine(t, server)

	err := engine.CreateChunkStore(t.Context(), "ragflow_tenant", "kb-1", 64, "naive", "English")
	if err == nil || !strings.Contains(err.Error(), "not a 64-dimension dense_vector") {
		t.Fatalf("CreateChunkStore = %v, want an incompatible-mapping error", err)
	}
	if len(*putBody) != 0 {
		t.Fatalf("unexpected mapping update: %#v", *putBody)
	}
}

// A new index is created from a mapping without vector fields, so the first
// model's dimension must be mapped as well.
func TestCreateChunkStoreMapsVectorFieldOnNewIndex(t *testing.T) {
	server, putBody := vectorMappingServer(t, false, nil)
	engine := newVectorMappingEngine(t, server)

	if err := engine.CreateChunkStore(t.Context(), "ragflow_tenant", "kb-1", 1024, "naive", "English"); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}
	property := mappedVectorField(t, putBody, "q_1024_vec")
	if property["type"] != "dense_vector" {
		t.Fatalf("type = %v, want dense_vector", property["type"])
	}
}
