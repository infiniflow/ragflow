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
	"fmt"
	"strings"
	"testing"
)

// tableChunksOf drives TableChunker.Invoke and returns the emitted maps.
func tableChunksOf(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewTableChunker(nil)
	if err != nil {
		t.Fatalf("NewTableChunker: %v", err)
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
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
				"text":         "- name: Alice\n- age: 30",
				"doc_type_kwd": "table",
			},
			{
				"text":         "- name: Bob\n- age: 25",
				"doc_type_kwd": "table",
			},
		},
	})

	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0]["text"] != "- name: Alice\n- age: 30" {
		t.Errorf("chunk0 text = %v", chunks[0]["text"])
	}
	if chunks[1]["text"] != "- name: Bob\n- age: 25" {
		t.Errorf("chunk1 text = %v", chunks[1]["text"])
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

// TestTableChunkerPreservesHeaderOnlyTable: the header row only becomes a
// chunk itself when the segment has no data rows — then the whole markup is
// the table's only searchable representation.
func TestTableChunkerPreservesHeaderOnlyTable(t *testing.T) {
	segment := spreadsheetSegmentItem("Sheet1", []string{"ID", "Name"}, nil, 1, 1)
	chunks := tableChunksOf(t, map[string]any{
		"name":          "empty.xlsx",
		"output_format": "json",
		"json":          []map[string]any{segment},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want the header-only table preserved", len(chunks))
	}
	if chunks[0]["ck_type"] != "table" || chunks[0]["text"] != segment["text"] {
		t.Fatalf("chunk = %#v, want the header-only segment whole", chunks[0])
	}
}

func TestTableChunkerPreservesHeaderOnlySheetAlongsideDataSheet(t *testing.T) {
	dataSegment := spreadsheetSegmentItem("Sheet1", []string{"ID", "Name"}, [][]string{{"A-1", "paid"}}, 1, 2)
	headerSegment := spreadsheetSegmentItem("Sheet2", []string{"ID", "Status"}, nil, 2, 1)
	chunks := tableChunksOf(t, map[string]any{
		"name":          "mixed.xlsx",
		"output_format": "json",
		"json":          []map[string]any{dataSegment, headerSegment},
	})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want data row plus header-only segment", len(chunks))
	}
	// The header row of the data segment never becomes a chunk of its own;
	// its column names live in the row record text instead.
	if chunks[0]["text"] != "- ID: A-1\n- Name: paid" {
		t.Fatalf("chunk0 = %#v", chunks[0])
	}
	if chunks[1]["text"] != headerSegment["text"] {
		t.Fatalf("chunk1 = %#v, want the header-only segment whole", chunks[1])
	}
}

// TestTableChunker_ExpandsHTMLRowsWithAlignedPositions covers the P2
// expander: one HTML <table> item with a row-aligned position matrix (one
// tuple per <tr>, header included) becomes one chunk per data row, each
// carrying its own tuple and the Python "- field: value" line format.
func TestTableChunker_ExpandsHTMLRowsWithAlignedPositions(t *testing.T) {
	table := "<table><caption>orders</caption>\n" +
		"<tr><th>ID</th><th>Status</th></tr>\n" +
		"<tr><td>A-1</td><td>paid</td></tr>\n" +
		"<tr><td>A-2</td><td></td></tr>\n" +
		"</table>\n"
	chunks := tableChunksOf(t, map[string]any{
		"name":          "orders.xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":         table,
				"doc_type_kwd": "table",
				"ck_type":      "table",
				"positions":    [][]float64{{1, 1, 1, 1, 2}, {1, 2, 2, 1, 2}, {1, 3, 3, 1, 2}},
			},
		},
	})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want one per data row", len(chunks))
	}
	if chunks[0]["text"] != "- ID: A-1\n- Status: paid" {
		t.Errorf("chunk0 text = %v", chunks[0]["text"])
	}
	// The empty cell is skipped, exactly like rag/app/table.py:656.
	if chunks[1]["text"] != "- ID: A-2" {
		t.Errorf("chunk1 text = %v", chunks[1]["text"])
	}
	if got := fmt.Sprintf("%v", chunks[0]["positions"]); !strings.Contains(got, "2") || strings.Contains(got, "3") {
		t.Errorf("chunk0 positions should hold only its own tuple, got %v", chunks[0]["positions"])
	}
	if got := fmt.Sprintf("%v", chunks[1]["positions"]); !strings.Contains(got, "3") {
		t.Errorf("chunk1 positions should hold its own tuple, got %v", chunks[1]["positions"])
	}
}

// TestTableChunker_WholeTableTupleNotCopiedToRows guards R1: when the
// position matrix is not row-aligned (a single whole-table tuple), row
// chunks must not inherit it — every row would otherwise locate to the same
// first coordinate. Row chunks get no positions instead.
func TestTableChunker_WholeTableTupleNotCopiedToRows(t *testing.T) {
	table := "<table><tr><th>ID</th></tr><tr><td>a</td></tr><tr><td>b</td></tr></table>"
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":         table,
				"doc_type_kwd": "table",
				"positions":    [][]float64{{1, 1, 2, 1, 1}},
			},
		},
	})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	for i, c := range chunks {
		if p, ok := c["positions"]; ok && p != nil && fmt.Sprintf("%v", p) != "[]" && fmt.Sprintf("%v", p) != "<nil>" {
			t.Errorf("chunk %d leaked whole-table positions: %v", i, p)
		}
	}
}

// TestTableChunker_HeaderOnlyHTMLTableKeepsWholePayload: a table whose only
// row is the header has nothing to expand; the whole markup stays one chunk.
func TestTableChunker_HeaderOnlyHTMLTableKeepsWholePayload(t *testing.T) {
	table := "<table><caption>t</caption>\n<tr><th>A</th><th>B</th></tr>\n</table>\n"
	chunks := tableChunksOf(t, map[string]any{
		"name":          "template.xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": table, "doc_type_kwd": "table", "ck_type": "table"},
		},
	})
	if len(chunks) != 1 || chunks[0]["text"] != table {
		t.Fatalf("header-only table must stay a single whole-payload chunk, got %#v", chunks)
	}
}

// TestTableChunker_HeaderlessHTMLFirstRowIsHeader: with no <th> row the
// first row names the columns, matching SplitLargeHTMLTable's convention.
func TestTableChunker_HeaderlessHTMLFirstRowIsHeader(t *testing.T) {
	table := "<table><tr><td>k</td><td>v</td></tr><tr><td>a</td><td>b</td></tr></table>"
	chunks := tableChunksOf(t, map[string]any{
		"name":          "kv.csv",
		"output_format": "json",
		"json": []map[string]any{
			{"text": table, "doc_type_kwd": "table"},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want the single data row", len(chunks))
	}
	if chunks[0]["text"] != "- k: a\n- v: b" {
		t.Errorf("text = %v", chunks[0]["text"])
	}
}
