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
	"reflect"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// charTokens counts runes, a cheap deterministic stand-in for a real tokenizer.
func charTokens(s string) int { return len([]rune(s)) }

func makeTable(dataRows int) *entity.TableData {
	grid := [][]entity.TableCell{{{Text: "Name"}, {Text: "Age"}}} // header
	for i := 0; i < dataRows; i++ {
		grid = append(grid, []entity.TableCell{{Text: "person"}, {Text: "30"}})
	}
	return &entity.TableData{Grid: grid, HeaderRows: 1}
}

func TestSplitLargeTableFitsWhole(t *testing.T) {
	td := makeTable(3)
	parts, ranges, headerRows := splitLargeTable(td, 1_000_000, charTokens)
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("small table should not split: parts=%v ranges=%v", parts, ranges)
	}
	if headerRows != 0 {
		t.Fatalf("headerRows = %d, want 0 when unsplit", headerRows)
	}
}

func TestSplitLargeTableRowGranular(t *testing.T) {
	td := makeTable(20)
	// Budget bounded so a part holds ~3 data rows.
	parts, ranges, headerRows := splitLargeTable(td, 22, charTokens)
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1", headerRows)
	}
	if len(parts) <= 1 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	if len(parts) != len(ranges) {
		t.Fatalf("parts(%d) != ranges(%d)", len(parts), len(ranges))
	}
	// Every part replicates the header and holds a whole number of rows; the
	// data ranges are contiguous and cover all 20 data rows exactly once.
	seen := 0
	for i, part := range parts {
		if part.HeaderRows != 1 {
			t.Fatalf("part %d headerRows = %d", i, part.HeaderRows)
		}
		if len(part.CellTexts()) < 2 {
			t.Fatalf("part %d has <1 data row: %v", i, part.CellTexts())
		}
		if part.CellTexts()[0][0] != "Name" {
			t.Fatalf("part %d missing header row", i)
		}
		r := ranges[i]
		if r[0] != seen {
			t.Fatalf("part %d range start %d != running %d", i, r[0], seen)
		}
		seen = r[1]
	}
	if seen != 20 {
		t.Fatalf("data rows covered = %d, want 20", seen)
	}
	// The concatenation of every part's data rows must equal the original rows.
	var rebuilt [][]string
	for _, part := range parts {
		rebuilt = append(rebuilt, part.CellTexts()[1:]...)
	}
	for i, row := range td.CellTexts()[1:] {
		if len(rebuilt[i]) != len(row) || rebuilt[i][0] != row[0] {
			t.Fatalf("row %d mismatch: got %v want %v", i, rebuilt[i], row)
		}
	}
}

func TestSplitLargeTableSingleRowOverBudgetKeepsRow(t *testing.T) {
	// One data row alone exceeds the budget; it must still form its own chunk
	// (row integrity wins over the budget). A single part means "not split,
	// emit whole" — the over-budget row becomes one chunk rather than being cut.
	td := &entity.TableData{
		Grid:       [][]entity.TableCell{{{Text: "H"}}, {{Text: "x" + string(make([]byte, 200))}}},
		HeaderRows: 1,
	}
	parts, ranges, _ := splitLargeTable(td, 5, charTokens)
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("over-budget single row must emit whole (ranges nil): parts=%v ranges=%v", parts, ranges)
	}
}

func TestSplitLargeTableHeaderOnly(t *testing.T) {
	td := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Name"}, {Text: "Age"}}}, HeaderRows: 1}
	parts, ranges, _ := splitLargeTable(td, 10, charTokens)
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("header-only table should not split")
	}
}

// assertPartGridContract checks that a split part is a self-contained grid:
// every Covered slot names an uncovered origin inside the part that actually
// spans over it, and every origin's stored spans stay within the rows and
// columns it owns in the part.
func assertPartGridContract(t *testing.T, part *entity.TableData, idx int) {
	t.Helper()
	for r, row := range part.Grid {
		for c, cell := range row {
			if cell.Covered {
				if cell.OriginRow < 0 || cell.OriginRow >= len(part.Grid) ||
					cell.OriginCol < 0 || cell.OriginCol >= len(part.Grid[cell.OriginRow]) {
					t.Fatalf("part %d: covered slot (%d,%d) back-pointer out of bounds: %+v", idx, r, c, cell)
				}
				origin := part.Grid[cell.OriginRow][cell.OriginCol]
				if origin.Covered || origin.RowSpan <= r-cell.OriginRow || origin.ColSpan <= c-cell.OriginCol {
					t.Fatalf("part %d: covered slot (%d,%d) not owned by origin (%d,%d): %+v", idx, r, c, cell.OriginRow, cell.OriginCol, origin)
				}
				continue
			}
			if r+cell.RowSpan > len(part.Grid) {
				t.Fatalf("part %d: cell (%d,%d) RowSpan=%d exceeds part rows %d", idx, r, c, cell.RowSpan, len(part.Grid))
			}
			if c+cell.ColSpan > len(row) {
				t.Fatalf("part %d: cell (%d,%d) ColSpan=%d exceeds row width %d", idx, r, c, cell.ColSpan, len(row))
			}
		}
	}
}

// partDataTexts concatenates the data-row (below-header) cell texts of every
// part; splitting must preserve the source's data rows exactly once each.
func partDataTexts(parts []entity.TableData, headerRows int) [][]string {
	var out [][]string
	for _, part := range parts {
		out = append(out, part.CellTexts()[headerRows:]...)
	}
	return out
}

// A header origin whose rowspan reaches into the data rows is replicated
// into every part. Each part must re-derive that span from the data rows it
// actually contains: keep it when all spanned rows travel with the part,
// shrink it when only some do, and degrade it to a plain header cell when
// none do — otherwise a later part's header claims rows it does not have.
func TestSplitLargeTableHeaderRowSpanClippedPerPart(t *testing.T) {
	td, err := tableutil.ParseTableHTML(`<table>
<tr><th rowspan="3">A</th><th>x</th></tr>
<tr><td>y</td></tr>
<tr><td>z</td></tr>
<tr><td>p</td><td>q</td></tr>
<tr><td>r</td><td>s</td></tr>
<tr><td>t</td><td>u</td></tr>
</table>`)
	if err != nil || td == nil {
		t.Fatalf("parse: %v", err)
	}
	// Budget 6: parts hold data rows [0,2), [2,3), [3,4), [4,5). The first
	// part carries both rows A spans, so A keeps RowSpan 3; the later parts
	// carry none of them, so A degrades to a plain header cell.
	parts, ranges, headerRows := splitLargeTable(td, 6, charTokens)
	if headerRows != 1 || len(parts) != 4 {
		t.Fatalf("parts=%d ranges=%v headerRows=%d", len(parts), ranges, headerRows)
	}
	wantRanges := [][2]int{{0, 2}, {2, 3}, {3, 4}, {4, 5}}
	if !reflect.DeepEqual(ranges, wantRanges) {
		t.Fatalf("ranges=%v want %v", ranges, wantRanges)
	}
	wantSpans := []int{3, 1, 1, 1}
	for i, part := range parts {
		assertPartGridContract(t, &part, i)
		if got := part.Grid[0][0].RowSpan; got != wantSpans[i] {
			t.Fatalf("part %d header A RowSpan=%d want %d", i, got, wantSpans[i])
		}
	}
	// Budget 4: every part holds a single data row, so A keeps exactly the
	// covered rows that travel with the part.
	parts, _, _ = splitLargeTable(td, 4, charTokens)
	wantSpans = []int{2, 2, 1, 1, 1}
	if len(parts) != len(wantSpans) {
		t.Fatalf("parts=%d want %d", len(parts), len(wantSpans))
	}
	for i, part := range parts {
		assertPartGridContract(t, &part, i)
		if got := part.Grid[0][0].RowSpan; got != wantSpans[i] {
			t.Fatalf("part %d header A RowSpan=%d want %d", i, got, wantSpans[i])
		}
	}
	wantTexts := [][]string{{"y"}, {"z"}, {"p", "q"}, {"r", "s"}, {"t", "u"}}
	if got := partDataTexts(parts, headerRows); !reflect.DeepEqual(got, wantTexts) {
		t.Fatalf("data texts=%v want %v", got, wantTexts)
	}
}

// A cut must never fall inside a data row's rowspan: the origin and every
// row it covers belong to the same part. Otherwise the first part's origin
// claims rows outside the part and the next part starts with a Covered slot
// whose origin is gone.
func TestSplitLargeTableNeverSplitsInsideRowSpan(t *testing.T) {
	td, err := tableutil.ParseTableHTML(`<table>
<tr><th>H</th><th>I</th></tr>
<tr><td rowspan="3">B</td><td>b1</td></tr>
<tr><td>b2</td></tr>
<tr><td>b3</td></tr>
<tr><td>p</td><td>q</td></tr>
<tr><td>r</td><td>s</td></tr>
</table>`)
	if err != nil || td == nil {
		t.Fatalf("parse: %v", err)
	}
	// The budget alone would cut after data row 2, inside B's span (B covers
	// data rows 0..2); the cut must move to after row 2, i.e. to 3.
	parts, ranges, headerRows := splitLargeTable(td, 11, charTokens)
	if headerRows != 1 {
		t.Fatalf("headerRows=%d want 1", headerRows)
	}
	wantRanges := [][2]int{{0, 3}, {3, 5}}
	if !reflect.DeepEqual(ranges, wantRanges) {
		t.Fatalf("ranges=%v want %v (cut must not fall inside B's rowspan)", ranges, wantRanges)
	}
	for i, part := range parts {
		assertPartGridContract(t, &part, i)
	}
	wantTexts := [][]string{{"B", "b1"}, {"b2"}, {"b3"}, {"p", "q"}, {"r", "s"}}
	if got := partDataTexts(parts, headerRows); !reflect.DeepEqual(got, wantTexts) {
		t.Fatalf("data texts=%v want %v", got, wantTexts)
	}
	// B's merged value survives in exactly one part.
	seen := 0
	for _, part := range parts {
		for _, row := range part.CellTexts()[headerRows:] {
			for _, text := range row {
				if text == "B" {
					seen++
				}
			}
		}
	}
	if seen != 1 {
		t.Fatalf("merged value B appears %d times across parts, want 1", seen)
	}
}
