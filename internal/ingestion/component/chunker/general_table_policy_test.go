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

package chunker

import (
	"context"
	"strings"
	"testing"
)

// TestGeneralChunkerKeepsLargeTablesWhole pins the table policy shared by the
// PDF and DOCX paths: a table is an atomic media unit, so chunk_token_size
// never splits it. Text is still merged and split by the configured budget.
func TestGeneralChunkerKeepsLargeTablesWhole(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatal(err)
	}

	table := "<table><tr><th>header</th></tr>" +
		strings.Repeat("<tr><td>cell value</td></tr>", 400) +
		"</table>"
	if tokens := tokenizeStr(table); tokens <= 512 {
		t.Fatalf("test table has %d tokens, want more than the 512 budget", tokens)
	}

	for _, fileType := range []string{"docx", "pdf"} {
		t.Run(fileType, func(t *testing.T) {
			out, err := component.Invoke(context.Background(), nil, map[string]any{
				"name":          "large." + fileType,
				"file_type":     fileType,
				"output_format": "json",
				"json": []map[string]any{{
					"text":         table,
					"doc_type_kwd": "table",
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			chunks := outputChunks(t, out)
			if len(chunks) != 1 {
				t.Fatalf("chunks = %d, want 1: a table is not split by the token budget", len(chunks))
			}
			if got := chunks[0]["doc_type_kwd"]; got != "table" {
				t.Fatalf("doc_type_kwd = %v, want table", got)
			}
			if got, _ := chunks[0]["text"].(string); got != table {
				t.Fatalf("table text changed: got %d bytes, want %d", len(got), len(table))
			}
		})
	}
}
