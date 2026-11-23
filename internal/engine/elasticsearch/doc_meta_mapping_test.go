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
	"os"
	"testing"

	"ragflow/internal/utility"
)

// The startup PUT body must stay valid JSON declaring the numeric metadata
// fields with their intended types.
func TestDocMetaTypedFieldsBody(t *testing.T) {
	var body map[string]any
	if err := json.Unmarshal([]byte(docMetaTypedFieldsBody), &body); err != nil {
		t.Fatalf("docMetaTypedFieldsBody is not valid JSON: %v", err)
	}
	props := body["properties"].(map[string]any)["meta_fields"].(map[string]any)["properties"].(map[string]any)
	if got := props["work_experience_years"].(map[string]any)["type"]; got != "double" {
		t.Errorf("work_experience_years type = %v, want double", got)
	}
	if got := props["highest_degree_level"].(map[string]any)["type"]; got != "integer" {
		t.Errorf("highest_degree_level type = %v, want integer", got)
	}
}

// The startup PUT and conf/doc_meta_es_mapping.json govern different index
// generations (existing vs newly created); they must declare the same field
// types or a fresh index and an old one would disagree.
func TestDocMetaTypedFieldsBodyMatchesMappingFile(t *testing.T) {
	mappingPath, err := utility.FindConfFileInProject("doc_meta_es_mapping.json")
	if err != nil {
		t.Fatalf("FindConfFileInProject: %v", err)
	}
	data, err := os.ReadFile(*mappingPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var mapping struct {
		Mappings struct {
			Properties struct {
				MetaFields struct {
					Properties map[string]struct {
						Type string `json:"type"`
					} `json:"properties"`
				} `json:"meta_fields"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(data, &mapping); err != nil {
		t.Fatalf("Unmarshal mapping: %v", err)
	}
	props := mapping.Mappings.Properties.MetaFields.Properties
	if props["work_experience_years"].Type != "double" {
		t.Errorf("mapping file work_experience_years type = %q, want double", props["work_experience_years"].Type)
	}
	if props["highest_degree_level"].Type != "integer" {
		t.Errorf("mapping file highest_degree_level type = %q, want integer", props["highest_degree_level"].Type)
	}
}
