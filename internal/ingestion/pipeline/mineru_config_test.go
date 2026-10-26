//
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
	"fmt"
	"testing"

	"ragflow/internal/ingestion/component"
)

// TestBuildParserConfigPreservesMinerUExtractionSwitches verifies that cleaning,
// serialization and Parser construction retain independent PDF choices.
func TestBuildParserConfigPreservesMinerUExtractionSwitches(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, formula := range []any{nil, true, false} {
			for _, table := range []any{nil, true, false} {
				t.Run(fmt.Sprintf("nested=%t/formula=%v/table=%v", nested, formula, table), func(t *testing.T) {
					pdf := map[string]any{"parse_method": "mineru"}
					if formula != nil {
						pdf["mineru_formula_enable"] = formula
					}
					if table != nil {
						pdf["mineru_table_enable"] = table
					}
					params := map[string]any{"pdf": pdf}
					if nested {
						params = map[string]any{"setups": params}
					}
					config := buildTestParserConfig(t, generalDSL(t), map[string]any{
						"Parser:HipSignsRhyme": params,
					})
					// Exercise the serialized shape stored in parser_config, not
					// just the in-memory map before normalization.
					encoded, err := json.Marshal(config)
					if err != nil {
						t.Fatal(err)
					}
					var saved map[string]any
					if err := json.Unmarshal(encoded, &saved); err != nil {
						t.Fatal(err)
					}
					parser := saved["Parser:HipSignsRhyme"].(map[string]any)
					if _, exists := parser["setups"]; exists {
						t.Fatal("nested setups were not normalized")
					}
					savedPDF := parser["pdf"].(map[string]any)
					for key, want := range map[string]any{
						"mineru_formula_enable": formula,
						"mineru_table_enable":   table,
					} {
						got, exists := savedPDF[key]
						if want == nil {
							if exists {
								t.Errorf("%s was materialized as %v", key, got)
							}
						} else if !exists || got != want {
							t.Errorf("%s = %v (present=%t), want %v", key, got, exists, want)
						}
					}
					if _, err := component.NewParserComponent(parser); err != nil {
						t.Fatalf("saved config cannot construct Parser: %v", err)
					}
				})
			}
		}
	}
}
