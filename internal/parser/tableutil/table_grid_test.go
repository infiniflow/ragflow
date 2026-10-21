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

package tableutil

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// gridTexts is the consumer view: row-major cell texts with covered positions
// skipped. For a 1x1 grid it must equal the legacy Rows exactly.
func gridTexts(td *entity.TableData) [][]string {
	return td.CellTexts()
}

// happy path: a plain table with no spans becomes a dense grid with no covered
// slots, and its text view is identical to the legacy Rows.
func TestGridNoSpans(t *testing.T) {
	td, err := ParseTableHTML(`<table>
<tr><th>Name</th><th>Age</th></tr>
<tr><td>Alice</td><td>30</td></tr>
<tr><td>Bob</td><td>25</td></tr>
</table>`)
	if err != nil || td == nil {
		t.Fatalf("parse: %v", err)
	}
	if td.HeaderRows != 1 {
		t.Fatalf("HeaderRows=%d want 1", td.HeaderRows)
	}
	if len(td.Grid) != 3 {
		t.Fatalf("grid rows=%d want 3", len(td.Grid))
	}
	for _, row := range td.Grid {
		if len(row) != 2 {
			t.Fatalf("row width=%d want 2: %+v", len(row), row)
		}
		for _, c := range row {
			if c.Covered || c.ColSpan != 1 || c.RowSpan != 1 {
				t.Fatalf("non-merged cell must be 1x1 uncovered: %+v", c)
			}
		}
	}
	want := [][]string{{"Name", "Age"}, {"Alice", "30"}, {"Bob", "25"}}
	if got := gridTexts(td); !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%v want %v", got, want)
	}
}

// colspan: the wide cell keeps its text on the origin slot with ColSpan=2, the
// engulfed position becomes a Covered marker, and the text view still yields
// the two real cell texts (unchanged vs legacy, which ignored colspan).
func TestGridColspan(t *testing.T) {
	td, _ := ParseTableHTML(`<table><tr><td colspan="2">wide</td><td>narrow</td></tr></table>`)
	if len(td.Grid) != 1 {
		t.Fatalf("rows=%d", len(td.Grid))
	}
	row := td.Grid[0]
	if len(row) != 3 {
		t.Fatalf("grid width=%d want 3 (origin + covered + narrow)", len(row))
	}
	if row[0].Text != "wide" || row[0].ColSpan != 2 || row[0].RowSpan != 1 {
		t.Fatalf("origin slot wrong: %+v", row[0])
	}
	if !row[1].Covered {
		t.Fatalf("engulfed slot must be Covered: %+v", row[1])
	}
	if row[1].OriginCol != 0 {
		t.Fatalf("covered slot should point at origin col 0: %+v", row[1])
	}
	if row[2].Text != "narrow" || row[2].ColSpan != 1 {
		t.Fatalf("narrow slot wrong: %+v", row[2])
	}
	if got := gridTexts(td); !reflect.DeepEqual(got, [][]string{{"wide", "narrow"}}) {
		t.Fatalf("texts=%v want [[wide narrow]]", got)
	}
}

// rowspan: a cell spanning two rows leaves a Covered marker directly below it;
// the text of the second row is the cell that actually lives there.
func TestGridRowspan(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td rowspan="2">A</td><td>x</td></tr>
<tr><td>y</td></tr>
</table>`)
	if len(td.Grid) != 2 {
		t.Fatalf("rows=%d want 2", len(td.Grid))
	}
	top := td.Grid[0]
	bottom := td.Grid[1]
	if top[0].Text != "A" || top[0].RowSpan != 2 {
		t.Fatalf("origin wrong: %+v", top[0])
	}
	if !bottom[0].Covered || bottom[0].OriginRow != 0 || bottom[0].OriginCol != 0 {
		t.Fatalf("covered below wrong: %+v", bottom[0])
	}
	// text view: each row keeps only its own (non-covered) cells.
	want := [][]string{{"A", "x"}, {"y"}}
	if got := gridTexts(td); !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%v want %v", got, want)
	}
}

// A rowspan that extends into a trailing row whose own cells do not reach the
// spanned column must still keep the continuation Covered marker, so the grid
// stays dense and the origin's RowSpan never exceeds the available rows. This
// guards the invariant that every claimed position is backed by a real row.
func TestGridRowspanKeepsContinuationRow(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td rowspan="3">A</td><td>B</td></tr>
<tr><td>C</td></tr>
<tr></tr>
</table>`)
	// r0: A(rs3) + B ; r1: C (with a covered continuation of A at c0) ;
	// r2: an empty <tr> must become a covered continuation of A, not be
	// dropped, otherwise the grid is no longer dense and A.RowSpan (3) would
	// exceed the available rows.
	if len(td.Grid) != 3 {
		t.Fatalf("grid rows=%d want 3 (rowspan continuation must survive)", len(td.Grid))
	}
	if td.Grid[0][0].RowSpan != 3 {
		t.Fatalf("origin RowSpan=%d want 3", td.Grid[0][0].RowSpan)
	}
	if !td.Grid[2][0].Covered || td.Grid[2][0].OriginRow != 0 || td.Grid[2][0].OriginCol != 0 {
		t.Fatalf("r2c0 must be a covered continuation of A: %+v", td.Grid[2][0])
	}
	// Density invariant: every origin's claimed rows exist.
	for r, row := range td.Grid {
		for c, cell := range row {
			if cell.Covered {
				continue
			}
			if r+cell.RowSpan > len(td.Grid) {
				t.Fatalf("cell (%d,%d) RowSpan=%d exceeds grid rows %d", r, c, cell.RowSpan, len(td.Grid))
			}
		}
	}
}

// An ancestor rowspan shifts a later row's cells to the right. The grid width
// must count the columns an ancestor rowspan occupies, otherwise the later
// row's trailing real cells are dropped (silent data loss that also regresses
// the legacy Rows text). This guards buildGrid's width computation.
func TestGridRowspanShiftsTrailingCells(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td rowspan="2">A</td><td>x</td></tr>
<tr><td>y</td><td>z</td></tr>
</table>`)
	// r0: A(rs2) + x ; r1: covered at c0, then y at c1, z at c2 -> width 3.
	if len(td.Grid) != 2 {
		t.Fatalf("rows=%d want 2", len(td.Grid))
	}
	if len(td.Grid[1]) != 3 {
		t.Fatalf("row1 width=%d want 3 (y and z both present, not dropped)", len(td.Grid[1]))
	}
	if !td.Grid[1][0].Covered || td.Grid[1][0].OriginCol != 0 {
		t.Fatalf("r1c0 must be a covered continuation of A: %+v", td.Grid[1][0])
	}
	if td.Grid[1][1].Text != "y" || td.Grid[1][2].Text != "z" {
		t.Fatalf("r1 must keep both y and z: %+v", td.Grid[1])
	}
	// Text view must reproduce the legacy Rows exactly (no cell dropped).
	want := [][]string{{"A", "x"}, {"y", "z"}}
	if got := gridTexts(td); !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%v want %v", got, want)
	}
}

// Realistic case: a single-column ID cell spans two rows while the second data
// row carries more cells than the first. The extra cells must not be dropped.
func TestGridRowspanKeepsExtraCells(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td rowspan="2">ID</td><td>a</td><td>b</td></tr>
<tr><td>c</td><td>d</td><td>e</td></tr>
</table>`)
	if len(td.Grid[1]) != 4 {
		t.Fatalf("row1 width=%d want 4 (covered ID + c,d,e)", len(td.Grid[1]))
	}
	want := [][]string{{"ID", "a", "b"}, {"c", "d", "e"}}
	if got := gridTexts(td); !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%v want %v", got, want)
	}
}

// combined colspan+rowspan: the big cell covers (r0c0,r0c1,r1c0,r1c1) while c
// and d are real cells, so 2 rows survive with r1's first two slots Covered.
func TestGridSpanAndTrailingTrim(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td colspan="2" rowspan="2">big</td><td>c</td></tr>
<tr><td>d</td></tr>
</table>`)
	// The big cell covers (r0c0,r0c1,r1c0,r1c1); only c and d are real in r0/r1.
	// No fully-covered trailing row here (r1 has d at c2), so 2 rows survive.
	if len(td.Grid) != 2 {
		t.Fatalf("rows=%d want 2 (trailing covered row trimmed if any)", len(td.Grid))
	}
	if td.Grid[0][0].Text != "big" || td.Grid[0][0].ColSpan != 2 || td.Grid[0][0].RowSpan != 2 {
		t.Fatalf("big origin wrong: %+v", td.Grid[0][0])
	}
	// r1 should be all covered in its first two slots and carry d at c2.
	if !td.Grid[1][0].Covered || !td.Grid[1][1].Covered {
		t.Fatalf("r1 covered slots wrong: %+v", td.Grid[1])
	}
	if td.Grid[1][2].Text != "d" {
		t.Fatalf("r1 c2 should be d: %+v", td.Grid[1][2])
	}
}

// overlapping spans must clamp instead of double-counting: a later origin that
// would land on an already-covered position is pushed past it and keeps a span
// consistent with what it owns (mirrors AnyDoc's GridBuilder clamp).
func TestGridOverlapClamps(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td rowspan="3">tall</td><td>a</td></tr>
<tr><td colspan="2">wide</td></tr>
</table>`)
	// r0: tall(rspan3) + a ; r1: covered + wide(cspan2) -> width 3
	// The wide cell at r1 would cover c1(r1) and c2(r1); that is fine (c1 is
	// covered by tall only at r0/r1/r2, c2 has nothing). Assert consistency:
	// every Covered slot points back to an origin within bounds.
	for r, row := range td.Grid {
		for c, cell := range row {
			if cell.Covered {
				// The back-pointer must name a slot that exists in the
				// grid: OriginRow is above (rowspan) or the same row
				// (colspan), and OriginCol is a real column of that row.
				if cell.OriginRow < 0 || cell.OriginRow >= len(td.Grid) ||
					cell.OriginCol < 0 || cell.OriginCol >= len(row) {
					t.Fatalf("covered slot (%d,%d) back-pointer out of bounds: %+v", r, c, cell)
				}
			}
		}
	}
}

// malformed span attributes (0, negative, non-numeric) are clamped to 1 rather
// than producing huge or negative grids.
func TestGridBadSpanAttrsClamped(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td colspan="0">a</td><td colspan="-3">b</td><td rowspan="abc">c</td></tr>
</table>`)
	row := td.Grid[0]
	if len(row) != 3 {
		t.Fatalf("width=%d want 3 (all spans clamped to 1)", len(row))
	}
	for _, c := range row {
		if c.ColSpan != 1 || c.RowSpan != 1 || c.Covered {
			t.Fatalf("clamped cell must be 1x1 uncovered: %+v", c)
		}
	}
	if got := gridTexts(td); !reflect.DeepEqual(got, [][]string{{"a", "b", "c"}}) {
		t.Fatalf("texts=%v", got)
	}
}

// A colspan origin's engulfed slot becomes a Covered marker, so the row stays
// dense (no empty cell) and its width matches rows that have no spans. Ragged
// (genuinely shorter) source rows are NOT padded with empty cells.
func TestGridColspanFillsCoveredSlot(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td colspan="2">wide</td></tr>
<tr><td>p</td><td>q</td></tr>
</table>`)
	if len(td.Grid[0]) != len(td.Grid[1]) {
		t.Fatalf("rows not uniform width: %d vs %d", len(td.Grid[0]), len(td.Grid[1]))
	}
	if len(td.Grid[0]) != 2 {
		t.Fatalf("width=%d want 2", len(td.Grid[0]))
	}
	if !td.Grid[0][1].Covered {
		t.Fatalf("r0c1 should be covered: %+v", td.Grid[0][1])
	}
}

// RenderTableHTML for a merged table emits colspan/rowspan on the origin and
// skips covered slots; the markup must round-trip back to the same grid.
func TestRenderMergedEmitsSpanAttrs(t *testing.T) {
	td, _ := ParseTableHTML(`<table><tr><td colspan="2">wide</td><td>narrow</td></tr></table>`)
	h := RenderTableHTML(td)
	if !strings.Contains(h, `colspan="2"`) {
		t.Fatalf("merged render missing colspan: %s", h)
	}
	if strings.Contains(h, `colspan="1"`) {
		t.Fatalf("1x1 cell must not emit colspan attr: %s", h)
	}
	// round-trip
	back, err := ParseTableHTML(h)
	if err != nil || back == nil {
		t.Fatalf("round-trip: %v", err)
	}
	if !reflect.DeepEqual(gridTexts(back), gridTexts(td)) {
		t.Fatalf("round-trip texts differ: %v vs %v", gridTexts(back), gridTexts(td))
	}
	if back.Grid[0][0].ColSpan != 2 {
		t.Fatalf("round-trip lost colspan: %+v", back.Grid[0][0])
	}
}

// RenderTableHTML -> ParseTableHTML round-trip must preserve both colspan and
// rowspan geometry (not just the text view), so a downstream consumer that
// renders or indexes the grid from the re-serialized markup sees the same
// merge geometry.
func TestRenderRoundTripPreservesSpans(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td colspan="2" rowspan="2">big</td><td>c</td></tr>
<tr><td>d</td></tr>
</table>`)
	h := RenderTableHTML(td)
	back, err := ParseTableHTML(h)
	if err != nil || back == nil {
		t.Fatalf("round-trip: %v", err)
	}
	if back.Grid[0][0].ColSpan != 2 || back.Grid[0][0].RowSpan != 2 {
		t.Fatalf("round-trip lost spans: %+v", back.Grid[0][0])
	}
	if !back.Grid[1][0].Covered || !back.Grid[1][1].Covered {
		t.Fatalf("round-trip lost covered continuation: %+v", back.Grid[1])
	}
	if got := gridTexts(back); !reflect.DeepEqual(got, gridTexts(td)) {
		t.Fatalf("round-trip texts differ: %v vs %v", gridTexts(back), gridTexts(td))
	}
}

// A non-merged table renders byte-for-byte identical to the legacy markup so
// existing downstream consumers see no change.
func TestRenderNonMergedByteIdentical(t *testing.T) {
	td := &entity.TableData{
		Grid: [][]entity.TableCell{
			{{Text: "Name"}, {Text: "Age"}},
			{{Text: "Alice"}, {Text: "30"}},
		},
		HeaderRows: 1,
	}
	got := RenderTableHTML(td)
	want := "<table>\n<tr><th>Name</th><th>Age</th></tr>\n" +
		"<tr><td>Alice</td><td>30</td></tr>\n</table>\n"
	if got != want {
		t.Fatalf("render mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// CellTexts is span-agnostic: a merged table yields the same cell texts as the
// equivalent non-merged table (the Covered slot contributes no text).
func TestCellTextsMergedSameAsNonMerged(t *testing.T) {
	merged, _ := ParseTableHTML(`<table><tr><td colspan="2">wide</td><td>narrow</td></tr></table>`)
	plain, _ := ParseTableHTML(`<table><tr><td>wide</td><td>narrow</td></tr></table>`)
	if !reflect.DeepEqual(merged.CellTexts(), plain.CellTexts()) {
		t.Fatalf("merged %v != plain %v", merged.CellTexts(), plain.CellTexts())
	}
}

// A colspan far beyond any real table must be clamped to the HTML-standard
// maximum (1000, the limit browsers enforce) before buildGrid sees it, so a
// malformed or hostile attribute can never drive a multi-gigabyte allocation
// or hang the parser. The grid still stays dense: the origin claims 1000
// columns, the engulfed positions become Covered markers, and the text view
// is unchanged.
func TestGridHugeColSpanClampedToSpecMax(t *testing.T) {
	td, _ := ParseTableHTML(`<table><tr><td colspan="100000000">wide</td><td>narrow</td></tr></table>`)
	if len(td.Grid) != 1 {
		t.Fatalf("rows=%d want 1", len(td.Grid))
	}
	row := td.Grid[0]
	if row[0].Text != "wide" || row[0].ColSpan != 1000 {
		t.Fatalf("origin ColSpan=%d want 1000 (HTML standard max): %+v", row[0].ColSpan, row[0])
	}
	if len(row) != 1001 {
		t.Fatalf("row width=%d want 1001 (origin + 999 covered + narrow)", len(row))
	}
	for c := 1; c <= 999; c++ {
		if !row[c].Covered || row[c].OriginCol != 0 {
			t.Fatalf("engulfed slot %d must be Covered pointing at origin col 0: %+v", c, row[c])
		}
	}
	if row[1000].Text != "narrow" || row[1000].ColSpan != 1 {
		t.Fatalf("trailing cell wrong: %+v", row[1000])
	}
	if got := gridTexts(td); !reflect.DeepEqual(got, [][]string{{"wide", "narrow"}}) {
		t.Fatalf("texts=%v want [[wide narrow]]", got)
	}
}

// A rowspan beyond the HTML-standard maximum (65534) must be clamped too; in a
// small table the surviving extent is additionally bounded by the number of
// rows below the origin, so the stored span always names rows that exist.
func TestGridHugeRowSpanClamped(t *testing.T) {
	td, _ := ParseTableHTML(`<table>
<tr><td rowspan="999999">A</td><td>x</td></tr>
<tr><td>y</td></tr>
<tr><td>z</td></tr>
</table>`)
	if len(td.Grid) != 3 {
		t.Fatalf("rows=%d want 3", len(td.Grid))
	}
	if td.Grid[0][0].RowSpan != 3 {
		t.Fatalf("RowSpan=%d want 3 (clamped to 65534, then to grid rows)", td.Grid[0][0].RowSpan)
	}
	if !td.Grid[1][0].Covered || !td.Grid[2][0].Covered {
		t.Fatalf("continuation rows must stay Covered: %+v / %+v", td.Grid[1][0], td.Grid[2][0])
	}
	if got := gridTexts(td); !reflect.DeepEqual(got, [][]string{{"A", "x"}, {"y"}, {"z"}}) {
		t.Fatalf("texts=%v", got)
	}
}

// The renderer must emit the clamped span, never the raw attribute value, so a
// pathological table round-trips without leaking a huge span attribute.
func TestRenderHugeSpanRoundTripBounded(t *testing.T) {
	td, _ := ParseTableHTML(`<table><tr><td colspan="100000000">wide</td><td>narrow</td></tr></table>`)
	h := RenderTableHTML(td)
	if strings.Contains(h, "100000000") {
		t.Fatalf("render leaked the raw span attribute: %s", h)
	}
	if !strings.Contains(h, `colspan="1000"`) {
		t.Fatalf("render must emit the clamped colspan: %s", h)
	}
	back, err := ParseTableHTML(h)
	if err != nil || back == nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if back.Grid[0][0].ColSpan != 1000 {
		t.Fatalf("round-trip lost the clamped span: %+v", back.Grid[0][0])
	}
	if !reflect.DeepEqual(gridTexts(back), gridTexts(td)) {
		t.Fatalf("round-trip texts differ: %v vs %v", gridTexts(back), gridTexts(td))
	}
}
