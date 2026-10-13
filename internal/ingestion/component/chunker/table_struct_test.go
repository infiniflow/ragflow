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

package chunker

import (
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

func TestExpandHTMLTableRowsFromTableData(t *testing.T) {
	td := &entity.TableData{
		Grid:       [][]entity.TableCell{{{Text: "Name"}, {Text: "Age"}}, {{Text: "Alice"}, {Text: "30"}}, {{Text: "Bob"}, {Text: "25"}}},
		HeaderRows: 1,
	}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 2 {
		t.Fatalf("got %d chunks, want 2", len(out))
	}
	if out[0].Text != "- Name: Alice\n- Age: 30" {
		t.Fatalf("row0 text = %q", out[0].Text)
	}
	if out[1].Text != "- Name: Bob\n- Age: 25" {
		t.Fatalf("row1 text = %q", out[1].Text)
	}
	for _, c := range out {
		if c.TableData != nil {
			t.Fatalf("TableData must be cleared on emitted row chunk, got %+v", c.TableData)
		}
	}
}

func TestExpandHTMLTableRowsHeaderOnlySingleChunk(t *testing.T) {
	td := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Name"}, {Text: "Age"}}}, HeaderRows: 1}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 1 {
		t.Fatalf("got %d chunks, want 1 (header-only)", len(out))
	}
	if out[0].TableData != nil {
		t.Fatalf("TableData must be cleared on the single chunk")
	}
}

func TestExpandHTMLTableRowsNonTablePassesThrough(t *testing.T) {
	item := schema.ChunkDoc{DocType: "text", Text: "just prose"}
	out := expandHTMLTableRows(item)
	if len(out) != 1 || out[0].Text != "just prose" {
		t.Fatalf("non-table not passed through: %+v", out)
	}
}

// A merged table where the first column carries a rowspan across two data rows
// must keep both data rows column-aligned with the header after expansion. The
// continuation row must not lose the spanned column (CellTexts drops Covered
// slots, which would shift its value under the wrong header). The projection
// must recover the spanned value from the grid alone, without relying on
// OriginRow/OriginCol — those fields are json:"-" and absent after a JSON
// round-trip, which is the path expandHTMLTableRows actually consumes.
func TestExpandHTMLTableRowsKeepsRowspanColumns(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "Name"}, {Text: "Age"}},
			{{Text: "Alice", RowSpan: 2}, {Text: "30"}},
			{{Covered: true, OriginRow: 0, OriginCol: 0}, {Text: "25"}},
		},
	}
	// Simulate a JSON round-trip that strips the json:"-" OriginRow/OriginCol.
	for r := range td.Grid {
		for c := range td.Grid[r] {
			td.Grid[r][c].OriginRow = 0
			td.Grid[r][c].OriginCol = 0
		}
	}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 2 {
		t.Fatalf("got %d chunks, want 2", len(out))
	}
	if out[0].Text != "- Name: Alice\n- Age: 30" {
		t.Fatalf("row0 text = %q", out[0].Text)
	}
	// The continuation row must keep the spanned "Name" column resolved to the
	// origin value and stay aligned: "Name: Alice" + "Age: 25", not a shifted
	// "Name: 25" that drops the Age column.
	if out[1].Text != "- Name: Alice\n- Age: 25" {
		t.Fatalf("row1 text = %q (rowspan column must be preserved, not dropped/misaligned)", out[1].Text)
	}
}

// A cell that spans BOTH rows and columns (RowSpan>1 && ColSpan>1) must keep
// every column it covers on the continuation rows, not just the first. The
// active-rowspan state machine in tableChunkRows used to register the span
// only at the origin's start column, so a 2x2 block left its second covered
// column blank on the row below — dropping that field's value and misaligning
// the row under the header.
func TestExpandHTMLTableRowsKeepsRowColSpanBlock(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "Name"}, {Text: "B"}, {Text: "C"}},
			{{Text: "Wide", RowSpan: 2, ColSpan: 2}, {Covered: true, OriginRow: 1, OriginCol: 0}, {Text: "x"}},
			{{Covered: true, OriginRow: 1, OriginCol: 0}, {Covered: true, OriginRow: 1, OriginCol: 0}, {Text: "y"}},
		},
	}
	// Simulate a JSON round-trip that strips the json:"-" OriginRow/OriginCol;
	// the projection must recover the spanned value from the grid alone.
	for r := range td.Grid {
		for c := range td.Grid[r] {
			td.Grid[r][c].OriginRow = 0
			td.Grid[r][c].OriginCol = 0
		}
	}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 2 {
		t.Fatalf("got %d chunks, want 2 (one per data row)", len(out))
	}
	if out[0].Text != "- Name: Wide\n- B: Wide\n- C: x" {
		t.Fatalf("row0 text = %q", out[0].Text)
	}
	// The continuation row must keep BOTH covered columns resolved to "Wide",
	// not drop the second one to a blank and misalign under the header.
	if out[1].Text != "- Name: Wide\n- B: Wide\n- C: y" {
		t.Fatalf("row1 text = %q (both spanned columns must be preserved)", out[1].Text)
	}
}

// A data row whose first cell spans two columns must keep both columns after
// expansion so the row stays aligned with the two-column header. CellTexts
// would drop the engulfed slot and leave only one value, misaligning it.
func TestExpandHTMLTableRowsKeepsColspanColumns(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "Name"}, {Text: "Age"}},
			{{Text: "Info", ColSpan: 2}, {Covered: true, OriginRow: 1, OriginCol: 0}},
		},
	}
	// Simulate a JSON round-trip that strips the json:"-" OriginRow/OriginCol.
	for r := range td.Grid {
		for c := range td.Grid[r] {
			td.Grid[r][c].OriginRow = 0
			td.Grid[r][c].OriginCol = 0
		}
	}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 1 {
		t.Fatalf("got %d chunks, want 1", len(out))
	}
	if out[0].Text != "- Name: Info\n- Age: Info" {
		t.Fatalf("row text = %q (colspan must fill both columns)", out[0].Text)
	}
}
