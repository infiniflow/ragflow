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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"

	"ragflow/internal/utility"
)

func TestMappingFilesValidation(t *testing.T) {
	// 1. Verify conf/os_mapping.json has ignore_above <= 8191 and doc_values == false
	osMappingPath, err := utility.FindConfFileInProject("os_mapping.json")
	if err != nil {
		t.Fatalf("FindConfFileInProject os_mapping.json: %v", err)
	}
	data, err := os.ReadFile(*osMappingPath)
	if err != nil {
		t.Fatalf("ReadFile os_mapping.json: %v", err)
	}

	var osMapping struct {
		Mappings struct {
			DynamicTemplates []map[string]struct {
				Match   string `json:"match"`
				Mapping struct {
					Type        string `json:"type"`
					IgnoreAbove int    `json:"ignore_above"`
					DocValues   *bool  `json:"doc_values"`
				} `json:"mapping"`
			} `json:"dynamic_templates"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(data, &osMapping); err != nil {
		t.Fatalf("Unmarshal os_mapping.json: %v", err)
	}

	foundStringTemplate := false
	for _, dt := range osMapping.Mappings.DynamicTemplates {
		if entry, ok := dt["string"]; ok {
			foundStringTemplate = true
			if entry.Mapping.Type != "keyword" {
				t.Errorf("expected string mapping type 'keyword', got %q", entry.Mapping.Type)
			}
			if entry.Mapping.IgnoreAbove != 8191 {
				t.Errorf("expected ignore_above 8191, got %d", entry.Mapping.IgnoreAbove)
			}
			if entry.Mapping.DocValues == nil || *entry.Mapping.DocValues != false {
				t.Errorf("expected doc_values false, got %v", entry.Mapping.DocValues)
			}
		}
	}
	if !foundStringTemplate {
		t.Fatal("string dynamic template not found in os_mapping.json")
	}

	// 2. Verify conf/skill_os_mapping.json has required knn vector dimensions
	skillMappingPath, err := utility.FindConfFileInProject("skill_os_mapping.json")
	if err != nil {
		t.Fatalf("FindConfFileInProject skill_os_mapping.json: %v", err)
	}
	skillData, err := os.ReadFile(*skillMappingPath)
	if err != nil {
		t.Fatalf("ReadFile skill_os_mapping.json: %v", err)
	}

	var skillMapping struct {
		Mappings struct {
			Properties map[string]struct {
				Type      string `json:"type"`
				Dimension int    `json:"dimension"`
				SpaceType string `json:"space_type"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(skillData, &skillMapping); err != nil {
		t.Fatalf("Unmarshal skill_os_mapping.json: %v", err)
	}

	for dim, fieldName := range map[int]string{
		384:  "q_384_vec",
		2048: "q_2048_vec",
		3584: "q_3584_vec",
		4096: "q_4096_vec",
	} {
		prop, ok := skillMapping.Mappings.Properties[fieldName]
		if !ok {
			t.Errorf("field %s should exist in skill_os_mapping.json", fieldName)
			continue
		}
		if prop.Type != "knn_vector" {
			t.Errorf("field %s: expected type knn_vector, got %s", fieldName, prop.Type)
		}
		if prop.Dimension != dim {
			t.Errorf("field %s: expected dimension %d, got %d", fieldName, dim, prop.Dimension)
		}
		if prop.SpaceType != "cosinesimil" {
			t.Errorf("field %s: expected space_type cosinesimil, got %s", fieldName, prop.SpaceType)
		}
	}
}

func TestCreateIndexTemplateRegistration(t *testing.T) {
	var capturedPath string
	var capturedMethod string
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"acknowledged": true}`))
	}))
	defer server.Close()

	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{server.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewApiClient: %v", err)
	}

	eng := &Engine{
		client:    client,
		apiClient: apiClient,
	}

	err = eng.CreateIndexTemplate(context.Background(), "ragflow_doc_meta_mapping", "ragflow_doc_meta_*", "doc_meta_es_mapping.json", 2)
	if err != nil {
		t.Fatalf("CreateIndexTemplate: %v", err)
	}

	if capturedPath != "/_index_template/ragflow_doc_meta_mapping" {
		t.Errorf("expected path /_index_template/ragflow_doc_meta_mapping, got %s", capturedPath)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("expected method PUT, got %s", capturedMethod)
	}
	if p, ok := capturedBody["priority"].(float64); !ok || p != 2 {
		t.Errorf("expected priority 2, got %v", capturedBody["priority"])
	}
	patterns, ok := capturedBody["index_patterns"].([]interface{})
	if !ok || len(patterns) != 1 || patterns[0] != "ragflow_doc_meta_*" {
		t.Errorf("expected index_patterns [ragflow_doc_meta_*], got %v", capturedBody["index_patterns"])
	}
	template, ok := capturedBody["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected template object, got %v", capturedBody["template"])
	}
	mappings, ok := template["mappings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected template.mappings object, got %v", template["mappings"])
	}
	if dyn, ok := mappings["dynamic"].(bool); !ok || !dyn {
		t.Errorf("expected template.mappings.dynamic to be true (bool), got %v (%T)", mappings["dynamic"], mappings["dynamic"])
	}
	if template["settings"] == nil {
		t.Error("expected template.settings to not be nil")
	}
}

func TestEnsureSkillVectorMappingsOnExistingIndex(t *testing.T) {
	const indexName = "skill_test_tenant_space"
	var putMappingCalled bool
	var capturedPutBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/"+indexName:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/"+indexName+"/_mapping":
			// Legacy index: only has 768 and 1536 vector fields, missing 4096, 3584, 2048, 384
			resp := map[string]interface{}{
				indexName: map[string]interface{}{
					"mappings": map[string]interface{}{
						"dynamic": false,
						"properties": map[string]interface{}{
							"skill_id": map[string]interface{}{"type": "keyword"},
							"q_768_vec": map[string]interface{}{
								"type":       "knn_vector",
								"dimension":  768,
								"space_type": "cosinesimil",
							},
							"q_1536_vec": map[string]interface{}{
								"type":       "knn_vector",
								"dimension":  1536,
								"space_type": "cosinesimil",
							},
						},
					},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodPut && r.URL.Path == "/"+indexName+"/_mapping":
			putMappingCalled = true
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedPutBody)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"acknowledged": true}`))
		default:
			t.Logf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{server.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewApiClient: %v", err)
	}

	eng := &Engine{
		client:    client,
		apiClient: apiClient,
	}

	ctx := context.Background()
	err = eng.CreateChunkStore(ctx, indexName, "skill", 0, "", "")
	if err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	if !putMappingCalled {
		t.Fatal("expected PUT /<index>/_mapping to be called on legacy index")
	}

	props, ok := capturedPutBody["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties in PUT body, got %v", capturedPutBody)
	}

	// Verify missing dimensions are in the PUT body
	for _, dim := range []int{4096, 3584, 3072, 2560, 2048, 1024, 512, 384, 256} {
		fieldName := fmt.Sprintf("q_%d_vec", dim)
		field, ok := props[fieldName].(map[string]interface{})
		if !ok {
			t.Errorf("expected missing field %s in PUT mapping, but was absent", fieldName)
			continue
		}
		if field["type"] != "knn_vector" {
			t.Errorf("field %s: expected type knn_vector, got %v", fieldName, field["type"])
		}
		if int(field["dimension"].(float64)) != dim {
			t.Errorf("field %s: expected dimension %d, got %v", fieldName, dim, field["dimension"])
		}
	}

	// Verify existing dimensions were NOT re-sent
	for _, dim := range []int{768, 1536} {
		fieldName := fmt.Sprintf("q_%d_vec", dim)
		if _, exists := props[fieldName]; exists {
			t.Errorf("field %s already existed, should not be included in PUT mapping update", fieldName)
		}
	}
}

func TestEnsureSkillVectorMappingsNoOpWhenUpToDate(t *testing.T) {
	const indexName = "skill_test_tenant_uptodate"
	var putMappingCalled bool

	allProperties := map[string]interface{}{
		"skill_id": map[string]interface{}{"type": "keyword"},
	}
	for _, dim := range []int{4096, 3584, 3072, 2560, 2048, 1536, 1024, 768, 512, 384, 256} {
		allProperties[fmt.Sprintf("q_%d_vec", dim)] = map[string]interface{}{
			"type":       "knn_vector",
			"dimension":  dim,
			"space_type": "cosinesimil",
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/"+indexName:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/"+indexName+"/_mapping":
			resp := map[string]interface{}{
				indexName: map[string]interface{}{
					"mappings": map[string]interface{}{
						"dynamic":    false,
						"properties": allProperties,
					},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodPut && r.URL.Path == "/"+indexName+"/_mapping":
			putMappingCalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"acknowledged": true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{server.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewApiClient: %v", err)
	}

	eng := &Engine{
		client:    client,
		apiClient: apiClient,
	}

	ctx := context.Background()
	err = eng.CreateChunkStore(ctx, indexName, "skill", 0, "", "")
	if err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	if putMappingCalled {
		t.Error("PUT /<index>/_mapping should NOT be called when index already has all vector mappings")
	}
}

func TestEnsureSkillVectorMappingOnIndexDocument(t *testing.T) {
	const indexName = "skill_test_tenant_indexdoc"
	var putMappingCalled bool
	var indexDocCalled bool
	var capturedPutBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/"+indexName+"/_mapping":
			resp := map[string]interface{}{
				indexName: map[string]interface{}{
					"mappings": map[string]interface{}{
						"dynamic": false,
						"properties": map[string]interface{}{
							"skill_id": map[string]interface{}{"type": "keyword"},
						},
					},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodPut && r.URL.Path == "/"+indexName+"/_mapping":
			putMappingCalled = true
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedPutBody)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"acknowledged": true}`))
		case r.Method == http.MethodPut && r.URL.Path == "/"+indexName+"/_doc/test_skill":
			indexDocCalled = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"result": "created"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{server.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewApiClient: %v", err)
	}

	eng := &Engine{
		client:    client,
		apiClient: apiClient,
	}

	ctx := context.Background()
	doc := map[string]interface{}{
		"skill_id":   "test_skill",
		"q_4096_vec": []float64{0.1, 0.2},
	}
	err = eng.IndexDocument(ctx, indexName, "test_skill", doc)
	if err != nil {
		t.Fatalf("IndexDocument failed: %v", err)
	}

	if !putMappingCalled {
		t.Fatal("expected PUT /<index>/_mapping to be called before IndexDocument")
	}
	if !indexDocCalled {
		t.Fatal("expected document to be indexed")
	}

	props, ok := capturedPutBody["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties in PUT mapping, got %v", capturedPutBody)
	}
	q4096, ok := props["q_4096_vec"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected q_4096_vec in PUT mapping, got %v", props)
	}
	if q4096["type"] != "knn_vector" {
		t.Errorf("expected type knn_vector, got %v", q4096["type"])
	}
	if int(q4096["dimension"].(float64)) != 4096 {
		t.Errorf("expected dimension 4096, got %v", q4096["dimension"])
	}
}

func TestEnsureSkillSearchVectorMappings(t *testing.T) {
	const indexName = "skill_test_tenant_search"
	var putMappingCalled bool
	var capturedPutBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/"+indexName:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/"+indexName+"/_mapping":
			resp := map[string]interface{}{
				indexName: map[string]interface{}{
					"mappings": map[string]interface{}{
						"dynamic": false,
						"properties": map[string]interface{}{
							"skill_id": map[string]interface{}{"type": "keyword"},
						},
					},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodPut && r.URL.Path == "/"+indexName+"/_mapping":
			putMappingCalled = true
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedPutBody)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"acknowledged": true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{server.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewApiClient: %v", err)
	}

	eng := &Engine{
		client:    client,
		apiClient: apiClient,
	}

	ctx := context.Background()
	err = eng.ensureSkillSearchVectorMappings(ctx, []string{indexName}, "q_2048_vec", 0)
	if err != nil {
		t.Fatalf("ensureSkillSearchVectorMappings failed: %v", err)
	}

	if !putMappingCalled {
		t.Fatal("expected PUT /<index>/_mapping to be called for search vector mapping")
	}

	props, ok := capturedPutBody["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties in PUT mapping, got %v", capturedPutBody)
	}
	q2048, ok := props["q_2048_vec"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected q_2048_vec in PUT mapping, got %v", props)
	}
	if q2048["type"] != "knn_vector" {
		t.Errorf("expected type knn_vector, got %v", q2048["type"])
	}
	if int(q2048["dimension"].(float64)) != 2048 {
		t.Errorf("expected dimension 2048, got %v", q2048["dimension"])
	}
}
