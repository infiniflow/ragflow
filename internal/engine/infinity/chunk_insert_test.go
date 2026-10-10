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

package infinity

import (
	"testing"
)

// TestParserIDFromChunks pins the table-parser detection that gates the
// chunk_data column reconcile: only a batch that carries chunk_data needs the
// parser-specific column, and any position in the batch counts (a mixed batch
// still writes chunk_data on the rows that have it).
func TestParserIDFromChunks(t *testing.T) {
	var nilMap map[string]interface{}

	tests := []struct {
		name   string
		chunks []map[string]interface{}
		want   string
	}{
		{name: "no chunks", chunks: nil, want: ""},
		{name: "plain chunk", chunks: []map[string]interface{}{{"id": "c1"}}, want: ""},
		{name: "nil chunk_data", chunks: []map[string]interface{}{{"chunk_data": nil}}, want: ""},
		{name: "typed nil chunk_data", chunks: []map[string]interface{}{{"chunk_data": nilMap}}, want: ""},
		{name: "empty chunk_data", chunks: []map[string]interface{}{{"chunk_data": map[string]interface{}{}}}, want: "table"},
		{
			name:   "table chunk after a plain chunk",
			chunks: []map[string]interface{}{{"id": "c1"}, {"chunk_data": map[string]interface{}{"rows": []interface{}{}}}},
			want:   "table",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parserIDFromChunks(tc.chunks); got != tc.want {
				t.Fatalf("parserIDFromChunks() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestApplyTableRowMarkers covers both directions of the column check: a batch
// that mixes table rows with ordinary chunks has to state the marker on every
// row, and a table without the column must not be sent one at all.
func TestApplyTableRowMarkers(t *testing.T) {
	batch := []map[string]interface{}{
		{"id": "row-1", "table_row_int": 1},
		{"id": "chunk-2", "content_with_weight": "prose"},
	}
	applyTableRowMarkers(batch, true)

	if batch[1]["table_row_int"] != 0 {
		t.Errorf("a non-row chunk must state 0, got %v", batch[1]["table_row_int"])
	}
	if batch[0]["table_row_int"] != 1 {
		t.Errorf("the row's own markers were overwritten: %v", batch[0])
	}
}

func TestApplyTableRowMarkersWithoutTheColumns(t *testing.T) {
	batch := []map[string]interface{}{{"id": "row-1", "table_row_int": 1}}
	applyTableRowMarkers(batch, false)

	if _, ok := batch[0]["table_row_int"]; ok {
		t.Error("Infinity rejects an insert naming a column the table lacks")
	}
	if batch[0]["id"] != "row-1" {
		t.Errorf("unrelated fields changed: %v", batch[0])
	}
}
