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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/parser/tableutil"
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

// tableItem builds a chunker JSON input item carrying the structured TableData
// contract a post-migration producer emits (derived from markup), plus any
// extra fields (positions, sheet_index, ck_type, ...). It deliberately does
// NOT set "text": the chunker derives a plain text body from TableData.
func tableItem(t *testing.T, html string, extra ...map[string]any) map[string]any {
	t.Helper()
	td, err := tableutil.ParseTableHTML(html)
	if err != nil {
		t.Fatalf("ParseTableHTML(%q): %v", html, err)
	}
	b, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal TableData: %v", err)
	}
	var tbl map[string]any
	if err := json.Unmarshal(b, &tbl); err != nil {
		t.Fatalf("unmarshal TableData: %v", err)
	}
	item := map[string]any{"table": tbl, "doc_type_kwd": "table"}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			item[k] = v
		}
	}
	return item
}

// TestTableChunker_OneChunkPerRow ports the Python table chunker's contract:
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
// chunk itself when the segment has no data rows — then the whole table's
// plain-text rendering is the table's only searchable representation.
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
	if chunks[0]["ck_type"] != "table" {
		t.Fatalf("chunk = %#v, want a table chunk", chunks[0])
	}
	if got, ok := chunks[0]["text"].(string); !ok || got == "" {
		t.Fatalf("header-only chunk must carry a plain-text body, got %#v", chunks[0]["text"])
	}
	if chunks[0]["table"] != nil {
		t.Fatalf("structured TableData must be cleared on the emitted chunk: %#v", chunks[0]["table"])
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
	if chunks[1]["ck_type"] != "table" || chunks[1]["table"] != nil {
		t.Fatalf("chunk1 = %#v, want the header-only segment whole with TableData cleared", chunks[1])
	}
}

// TestTableChunker_ExpandsHTMLRowsWithAlignedPositions covers the P2
// expander: one table item with a row-aligned position matrix (one
// tuple per row, header included) becomes one chunk per data row, each
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
			tableItem(t, table, map[string]any{
				"ck_type":     "table",
				"sheet_index": 1,
				"positions":   [][]float64{{1, 1, 1, 1, 2}, {1, 2, 2, 1, 2}, {1, 3, 3, 1, 2}},
			}),
		},
	})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want one per data row", len(chunks))
	}
	if chunks[0]["text"] != "- ID: A-1\n- Status: paid" {
		t.Errorf("chunk0 text = %v", chunks[0]["text"])
	}
	// The empty cell is skipped, matching the Python table chunker's per-cell skip.
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
			tableItem(t, table, map[string]any{
				"sheet_index": 1,
				"positions":   [][]float64{{1, 1, 2, 1, 1}},
			}),
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

// TestTableChunker_NonSpreadsheetPositionsNotAttached: a table item without
// spreadsheet identity must not have its positions read as per-row tuples —
// PDF items write layout boxes into the same field, so a five-field matrix
// that happens to align must still be ignored.
func TestTableChunker_NonSpreadsheetPositionsNotAttached(t *testing.T) {
	table := "<table><tr><th>ID</th></tr><tr><td>a</td></tr><tr><td>b</td></tr></table>"
	chunks := tableChunksOf(t, map[string]any{
		"name":          "document.docx",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, table, map[string]any{
				"ck_type":   "table",
				"positions": [][]float64{{1, 1, 1, 1, 1}, {1, 2, 2, 1, 1}, {1, 3, 3, 1, 1}},
			}),
		},
	})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want one per data row", len(chunks))
	}
	for i, c := range chunks {
		if p, ok := c["positions"]; ok && p != nil && fmt.Sprintf("%v", p) != "[]" && fmt.Sprintf("%v", p) != "<nil>" {
			t.Errorf("chunk %d kept positions without spreadsheet identity: %v", i, p)
		}
	}
}

// TestTableChunker_HeaderOnlyHTMLTableKeepsWholePayload: a table whose only
// row is the header has nothing to expand; the whole markup stays one chunk
// carrying the plain-text rendering of the header.
func TestTableChunker_HeaderOnlyHTMLTableKeepsWholePayload(t *testing.T) {
	table := "<table><caption>t</caption>\n<tr><th>A</th><th>B</th></tr>\n</table>\n"
	td, err := tableutil.ParseTableHTML(table)
	if err != nil {
		t.Fatalf("ParseTableHTML: %v", err)
	}
	want := tableutil.RenderTableText(td)
	chunks := tableChunksOf(t, map[string]any{
		"name":          "template.xlsx",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, table, map[string]any{"ck_type": "table"}),
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want the single header-only chunk", len(chunks))
	}
	if chunks[0]["text"] != want {
		t.Fatalf("header-only chunk text = %q, want %q", chunks[0]["text"], want)
	}
}

// TestTableChunker_HeaderlessHTMLFirstRowIsHeader: with no <th> row the
// first row names the columns, matching splitLargeTable's convention.
func TestTableChunker_HeaderlessHTMLFirstRowIsHeader(t *testing.T) {
	table := "<table><tr><td>k</td><td>v</td></tr><tr><td>a</td><td>b</td></tr></table>"
	chunks := tableChunksOf(t, map[string]any{
		"name":          "kv.csv",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, table),
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want the single data row", len(chunks))
	}
	if chunks[0]["text"] != "- k: a\n- v: b" {
		t.Errorf("text = %v", chunks[0]["text"])
	}
}
