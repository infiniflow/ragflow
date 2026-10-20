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
	"reflect"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/chunker"
	"ragflow/internal/parser/tableutil"
)

// TestBuiltinTitleMediaContext verifies saved configuration and mixed-media output
// using the actual book/paper/manual templates, without external parser services.
func TestBuiltinTitleMediaContext(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	table, err := tableutil.ParseTableHTML("<table><tr><td>Value</td></tr></table>")
	if err != nil {
		t.Fatal(err)
	}
	tableHTML := tableutil.RenderTableHTML(table)
	for _, ref := range []string{"book", "paper", "manual"} {
		tpl, ok := registry.Get(ref)
		if !ok {
			t.Fatalf("missing builtin %s", ref)
		}
		dsl, err := json.Marshal(tpl.DSL)
		if err != nil {
			t.Fatal(err)
		}
		components, err := ExtractAllComponentParams(dsl)
		if err != nil {
			t.Fatal(err)
		}
		tested := false
		for _, spec := range components {
			if spec.ComponentName != "TitleChunker" && spec.ComponentName != "ManualChunker" {
				continue
			}
			tested = true
			for _, pair := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}, {64, 32}} {
				t.Run(fmt.Sprintf("%s/table=%d/image=%d", ref, pair[0], pair[1]), func(t *testing.T) {
					config := BuildParserConfig(dsl, map[string]any{spec.CpnID: map[string]any{
						"table_context_size": pair[0], "image_context_size": pair[1],
					}})
					raw, err := json.Marshal(config)
					if err != nil {
						t.Fatal(err)
					}
					var saved map[string]map[string]any
					if err := json.Unmarshal(raw, &saved); err != nil {
						t.Fatal(err)
					}
					params := saved[spec.CpnID]
					for key, want := range map[string]int{"table_context_size": pair[0], "image_context_size": pair[1]} {
						if got := params[key]; got != float64(want) {
							t.Fatalf("%s lost %s=%d: %#v", ref, key, want, params)
						}
					}
					var comp runtime.Component
					if spec.ComponentName == "ManualChunker" {
						comp, err = chunker.NewManualChunker(params)
					} else {
						comp, err = chunker.NewTitleChunker(params)
					}
					if err != nil {
						t.Fatal(err)
					}
					out, err := comp.Invoke(t.Context(), nil, map[string]any{
						"name": ref + ".docx", "output_format": "json",
						"json": []map[string]any{
							{"text": "Alpha.", "doc_type_kwd": "text"},
							{"table": table, "doc_type_kwd": "table", "img_id": "table-preview"},
							{"text": "Beta.", "doc_type_kwd": "text"},
							{"text": "Caption.", "doc_type_kwd": "image", "img_id": "figure"},
							{"text": "Gamma.", "doc_type_kwd": "text"},
						},
					})
					if err != nil {
						t.Fatal(err)
					}
					if problem := out["_ERROR"]; problem != nil {
						t.Fatal(problem)
					}
					chunks, ok := out["chunks"].([]map[string]any)
					if !ok || len(chunks) != 5 {
						t.Fatalf("unexpected chunks: %#v", out)
					}
					wantTable, wantImage := tableHTML, "Caption."
					if pair[0] > 0 {
						wantTable = "Alpha.\n" + tableHTML + "\nBeta."
						if pair[0] > 1 {
							wantTable += "\nGamma."
						}
					}
					if pair[1] > 0 {
						wantImage = "Beta.\nCaption.\nGamma."
						if pair[1] > 1 {
							wantImage = "Alpha.\n" + wantImage
						}
					}
					if chunks[1]["text"] != wantTable || chunks[3]["text"] != wantImage {
						t.Fatalf("wrong media context: table=%q image=%q", chunks[1]["text"], chunks[3]["text"])
					}
					if chunks[1]["img_id"] != "table-preview" || chunks[3]["img_id"] != "figure" {
						t.Fatalf("media reference changed: %#v", chunks)
					}
					if !reflect.DeepEqual([]any{chunks[0]["text"], chunks[2]["text"], chunks[4]["text"]}, []any{"Alpha.\n", "Beta.\n", "Gamma.\n"}) {
						t.Fatalf("text order changed: %#v", chunks)
					}
				})
			}
		}
		if !tested {
			t.Fatalf("%s has no title/manual component", ref)
		}
	}
}
