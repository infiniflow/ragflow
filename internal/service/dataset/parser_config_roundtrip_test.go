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

package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

func TestParserConfigCanonicalExtractorRoundTrip(t *testing.T) {
	dsl := []byte(`{"components":{"Extractor:AutoExtractDefault":{"obj":{"component_name":"Extractor","params":{"metadata":{"enabled":true,"metadata":[],"built_in_metadata":[]},"tags":{"top_n":5,"tag_file_id":""}}}}}}`)
	raw := map[string]any{
		"chunk_token_num": float64(128),
		"Extractor:AutoExtractDefault": map[string]any{
			"tags": map[string]any{"top_n": 5, "tag_file_id": "file-round-trip"},
		},
	}
	if dropped, err := ValidateParserConfig(raw); err != nil {
		t.Fatalf("ValidateParserConfig: %v", err)
	} else if len(dropped) != 1 || dropped[0] != "chunk_token_num" {
		t.Fatalf("dropped = %#v, want [chunk_token_num]", dropped)
	}

	config, err := pipelinepkg.BuildParserConfig(dsl, raw)
	if err != nil {
		t.Fatalf("BuildParserConfig: %v", err)
	}
	if got := component.TagFileIDFromParserConfig(map[string]any(config)); got != "file-round-trip" {
		t.Fatalf("TagFileIDFromParserConfig = %q, want file-round-trip", got)
	}
	if meta, ok := common.ExtractorMetadataConfig(map[string]any(config)); !ok {
		t.Fatal("ExtractorMetadataConfig did not find the canonical Extractor node")
	} else if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("metadata enabled = %v, want true", meta["enabled"])
	}
}
