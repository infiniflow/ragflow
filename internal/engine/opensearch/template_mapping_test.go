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
	"net/http/httptest"
	"os"
	"reflect"
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

func TestDefaultSkillMappingMatchesConfigFile(t *testing.T) {
	skillMappingPath, err := utility.FindConfFileInProject("skill_os_mapping.json")
	if err != nil {
		t.Fatalf("FindConfFileInProject skill_os_mapping.json: %v", err)
	}
	raw, err := os.ReadFile(*skillMappingPath)
	if err != nil {
		t.Fatalf("ReadFile skill_os_mapping.json: %v", err)
	}

	var want interface{}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("Unmarshal skill_os_mapping.json: %v", err)
	}

	fallbackJSON, err := json.Marshal(getDefaultSkillMapping())
	if err != nil {
		t.Fatalf("Marshal getDefaultSkillMapping: %v", err)
	}
	var got interface{}
	if err := json.Unmarshal(fallbackJSON, &got); err != nil {
		t.Fatalf("Unmarshal fallback mapping: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getDefaultSkillMapping() drifted from conf/skill_os_mapping.json:\ngot:  %#v\nwant: %#v", got, want)
	}
}
