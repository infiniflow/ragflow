// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pipeline

import (
	"encoding/json"
	"testing"

	"ragflow/internal/ingestion/component"
)

// TestTenantPDFParserReferenceCleaning retains a reference, not resolved
// credentials, through the existing parser_config cleaning/save/load boundary.
func TestTenantPDFParserReferenceCleaning(t *testing.T) {
	for _, ref := range []string{"model-b", "SoMark-model@b@SoMark", "OpenDataLoader-model@b@OpenDataLoader"} {
		for _, field := range []string{"parse_method", "layout_recognizer"} {
			t.Run(ref+"/"+field, func(t *testing.T) {
				cleaned := CleanComponentParams(generalDSL(t), map[string]any{
					"Parser:HipSignsRhyme": map[string]any{
						"pdf": map[string]any{"parse_method": "deepdoc", field: ref},
					},
				})
				saved, err := json.Marshal(cleaned)
				if err != nil {
					t.Fatal(err)
				}
				var reloaded map[string]any
				if err := json.Unmarshal(saved, &reloaded); err != nil {
					t.Fatal(err)
				}
				params := reloaded["Parser:HipSignsRhyme"].(map[string]any)
				if got := params["pdf"].(map[string]any)[field]; got != ref {
					t.Fatalf("saved reference = %v, want %s", got, ref)
				}
				if _, err := component.NewParserComponent(params); err != nil {
					t.Fatalf("reloaded component: %v", err)
				}
			})
		}
	}
}
