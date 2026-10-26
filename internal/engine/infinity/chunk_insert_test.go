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

import "testing"

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

// TestBatchNeedsMediaOrdinalColumns pins the gate for the media ordinal
// reconcile on the insert path: only a batch that actually carries
// slide_number or media_order (the PPTX slide/image rows) should reconcile
// the columns onto a legacy table, in any batch position.
func TestBatchNeedsMediaOrdinalColumns(t *testing.T) {
	tests := []struct {
		name   string
		chunks []map[string]interface{}
		want   bool
	}{
		{name: "no chunks", chunks: nil, want: false},
		{name: "plain chunk", chunks: []map[string]interface{}{{"id": "c1"}}, want: false},
		{name: "slide row", chunks: []map[string]interface{}{{"slide_number": 3}}, want: true},
		{name: "image row", chunks: []map[string]interface{}{{"media_order": 2}}, want: true},
		{
			name: "slide row after a plain chunk",
			chunks: []map[string]interface{}{
				{"id": "c1"},
				{"id": "c2", "slide_number": 1},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := batchNeedsMediaOrdinalColumns(tc.chunks); got != tc.want {
				t.Fatalf("batchNeedsMediaOrdinalColumns() = %v, want %v", got, tc.want)
			}
		})
	}
}
