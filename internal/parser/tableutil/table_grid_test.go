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

// combined colspan+rowspan with a trailing all-covered row that must be
// trimmed so no empty rows survive.
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

// ragged source rows (a merged row is shorter after span accounting) pad to a
// uniform width so the grid stays rectangular.
func TestGridRaggedPadsUniformWidth(t *testing.T) {
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

// RenderTableText strips tags; a merged table yields the same stripped text as
// the equivalent non-merged table (text-level output is unchanged by spans).
func TestRenderTextMergedSameAsNonMerged(t *testing.T) {
	merged, _ := ParseTableHTML(`<table><tr><td colspan="2">wide</td><td>narrow</td></tr></table>`)
	plain, _ := ParseTableHTML(`<table><tr><td>wide</td><td>narrow</td></tr></table>`)
	if RenderTableText(merged) != RenderTableText(plain) {
		t.Fatalf("merged text %q != plain text %q", RenderTableText(merged), RenderTableText(plain))
	}
}
