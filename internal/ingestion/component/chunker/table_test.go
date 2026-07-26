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
	"testing"
)

// tableChunksOf drives TableChunker.Invoke and returns the emitted maps.
func tableChunksOf(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewTableChunker(nil)
	if err != nil {
		t.Fatalf("NewTableChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("TableChunker.Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// TestTableChunker_OneChunkPerRow ports rag/app/table.py's contract:
// "Every row in table will be treated as a chunk." Two upstream rows must
// yield exactly two chunks, each preserving its own content + metadata.
func TestTableChunker_OneChunkPerRow(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			{
				"content_with_weight": "- name: Alice\n- age: 30",
				"doc_type_kwd":        "table",
			},
			{
				"content_with_weight": "- name: Bob\n- age: 25",
				"doc_type_kwd":        "table",
			},
		},
	})

	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0]["content_with_weight"] != "- name: Alice\n- age: 30" {
		t.Errorf("chunk0 content = %v", chunks[0]["content_with_weight"])
	}
	if chunks[1]["content_with_weight"] != "- name: Bob\n- age: 25" {
		t.Errorf("chunk1 content = %v", chunks[1]["content_with_weight"])
	}
}

// TestTableChunker_EmptyRows yields no chunks when there is no data.
func TestTableChunker_EmptyRows(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json":          []map[string]any{},
	})
	if len(chunks) != 0 {
		t.Errorf("got %d chunks, want 0", len(chunks))
	}
}
