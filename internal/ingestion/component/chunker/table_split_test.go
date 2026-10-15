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
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"ragflow/internal/common"
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
	parts, ranges, headerRows := splitLargeTable(td, 1_000_000, charTokens, "")
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("small table should not split: parts=%v ranges=%v", parts, ranges)
	}
	if headerRows != 0 {
		t.Fatalf("headerRows = %d, want 0 when unsplit", headerRows)
	}
}

func TestSplitLargeTableRowGranular(t *testing.T) {
	td := makeTable(20)
	// Budget bounded so a part holds a couple of data rows (the replicated
	// header is not charged against it; rows cost ~36 tokens as HTML here).
	parts, ranges, headerRows := splitLargeTable(td, 80, charTokens, "")
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
	parts, ranges, _ := splitLargeTable(td, 5, charTokens, "")
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("over-budget single row must emit whole (ranges nil): parts=%v ranges=%v", parts, ranges)
	}
}

func TestSplitLargeTableHeaderOnly(t *testing.T) {
	td := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Name"}, {Text: "Age"}}}, HeaderRows: 1}
	parts, ranges, _ := splitLargeTable(td, 10, charTokens, "")
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
	// Budget = the HTML cost of the two rows A spans (y, z), so
	// the first part is exactly those rows and keeps A's RowSpan 3; later parts
	// carry none of them, so A degrades to a plain header cell (RowSpan 1).
	// The header cost is subtracted: the split budget covers data rows only.
	headerCost := charTokens(tableutil.RenderTableHTML(&entity.TableData{Grid: td.Grid[:1], HeaderRows: 1}))
	budget := charTokens(tableutil.RenderTableHTML(&entity.TableData{Grid: td.Grid[:3], HeaderRows: 1})) - headerCost
	parts, ranges, headerRows := splitLargeTable(td, budget, charTokens, "")
	if headerRows != 1 || len(parts) < 2 {
		t.Fatalf("parts=%d ranges=%v headerRows=%d", len(parts), ranges, headerRows)
	}
	if ranges[0] != [2]int{0, 2} {
		t.Fatalf("first range = %v, want [0 2] (A's two spanned rows)", ranges[0])
	}
	if got := parts[0].Grid[0][0].RowSpan; got != 3 {
		t.Fatalf("part 0 A RowSpan = %d, want 3 (carries both spanned rows)", got)
	}
	for i := 1; i < len(parts); i++ {
		assertPartGridContract(t, &parts[i], i)
		if got := parts[i].Grid[0][0].RowSpan; got != 1 {
			t.Fatalf("part %d A RowSpan = %d, want 1 (carries none of A's spanned rows)", i, got)
		}
	}
	covered := 0
	for _, r := range ranges {
		covered += r[1] - r[0]
	}
	if covered != 5 {
		t.Fatalf("parts cover %d data rows, want 5", covered)
	}

	// Budget = the HTML cost of only the first spanned row (y), so
	// the first part carries exactly one of A's spanned rows and A keeps
	// RowSpan 2 (header + that one row), not the full 3.
	// The header cost is subtracted: the split budget covers data rows only.
	budget = charTokens(tableutil.RenderTableHTML(&entity.TableData{Grid: td.Grid[:2], HeaderRows: 1})) - headerCost
	parts, ranges, _ = splitLargeTable(td, budget, charTokens, "")
	if len(parts) < 2 || ranges[0] != [2]int{0, 1} {
		t.Fatalf("parts=%d ranges=%v, want first part [0 1]", len(parts), ranges)
	}
	if got := parts[0].Grid[0][0].RowSpan; got != 2 {
		t.Fatalf("part 0 A RowSpan = %d, want 2 (carries exactly one spanned row)", got)
	}
	covered = 0
	for _, r := range ranges {
		covered += r[1] - r[0]
	}
	if covered != 5 {
		t.Fatalf("parts cover %d data rows, want 5", covered)
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
	// Budget = the HTML cost of B's own row only. The budget alone
	// would cut right after data row 0, inside B's span (B covers data rows
	// 0..2); span-safe cut must move the boundary to after row 2, i.e. to 3.
	// The header cost is subtracted: the split budget covers data rows only.
	headerCost := charTokens(tableutil.RenderTableHTML(&entity.TableData{Grid: td.Grid[:1], HeaderRows: 1}))
	budget := charTokens(tableutil.RenderTableHTML(&entity.TableData{Grid: td.Grid[:2], HeaderRows: 1})) - headerCost
	parts, ranges, headerRows := splitLargeTable(td, budget, charTokens, "")
	if headerRows != 1 {
		t.Fatalf("headerRows=%d want 1", headerRows)
	}
	// The first boundary must land after B's span (row 3), never inside it —
	// that is the span-safe guarantee. (How the remaining rows chunk is purely a
	// budget artifact and not what this test pins.)
	if ranges[0] != [2]int{0, 3} {
		t.Fatalf("first range = %v, want [0 3] (cut must not fall inside B's rowspan)", ranges[0])
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

// A header wider than the whole budget must not eat the data allowance. While
// the header was charged per part, every data row alone blew the budget and the
// table collapsed to one row per part (20 parts here); the budget is now spent
// on data rows alone, so a part packs as many rows as fit.
func TestSplitLargeTableHeaderDoesNotConsumeDataBudget(t *testing.T) {
	header := strings.Repeat("x", 100) // wide header: 136 tokens as HTML
	grid := [][]entity.TableCell{{{Text: header}}}
	for i := 0; i < 20; i++ {
		grid = append(grid, []entity.TableCell{{Text: strings.Repeat("y", 10)}}) // 29 tokens each as HTML
	}
	td := &entity.TableData{Grid: grid, HeaderRows: 1}

	// 150-token budget: data alone packs 5 rows per part -> 4 parts.
	// While the 136-token header was charged per part, every data row alone
	// blew the budget and the table collapsed to 20 one-row parts.
	parts, ranges, headerRows := splitLargeTable(td, 150, charTokens, "")
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1", headerRows)
	}
	if len(parts) <= 1 {
		t.Fatalf("expected the table to split, got %d parts", len(parts))
	}
	if len(parts) > 5 {
		t.Fatalf("header consumed the data budget: got %d parts, want <= 5", len(parts))
	}
	if len(parts) != len(ranges) {
		t.Fatalf("parts(%d) != ranges(%d)", len(parts), len(ranges))
	}
	seen := 0
	for i := range parts {
		part := &parts[i]
		assertPartGridContract(t, part, i)
		if part.Grid[0][0].Text != header {
			t.Fatalf("part %d missing the replicated header: %v", i, part.Grid[0])
		}
		if len(part.Grid) < 2 {
			t.Fatalf("part %d holds no data rows: %v", i, part.CellTexts())
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
}

// A header that alone meets or exceeds the budget means the table width and
// ChunkTokenSize disagree; that must be surfaced as a warning, not absorbed.
func TestSplitLargeTableWarnsWhenHeaderExceedsBudget(t *testing.T) {
	const msg = "chunker: table header alone exceeds chunk budget"

	newObservedLogs := func(t *testing.T) *observer.ObservedLogs {
		t.Helper()
		core, logs := observer.New(zap.WarnLevel)
		originalLogger := common.Logger
		common.Logger = zap.New(core)
		t.Cleanup(func() { common.Logger = originalLogger })
		return logs
	}

	t.Run("header at or over budget warns", func(t *testing.T) {
		logs := newObservedLogs(t)
		td := &entity.TableData{
			Grid:       [][]entity.TableCell{{{Text: strings.Repeat("x", 100)}}, {{Text: "cell"}}},
			HeaderRows: 1,
		}
		splitLargeTable(td, 50, charTokens, "doc-123")

		entries := logs.FilterMessage(msg).All()
		if len(entries) != 1 {
			t.Fatalf("want exactly 1 warning, got %d (logs=%v)", len(entries), logs.All())
		}
		fields := entries[0].ContextMap()
		if fields["doc_id"] != "doc-123" {
			t.Fatalf("doc_id = %v, want doc-123", fields["doc_id"])
		}
		if fields["chunk_token_size"] != int64(50) {
			t.Fatalf("chunk_token_size = %v, want 50", fields["chunk_token_size"])
		}
		if got, ok := fields["header_tokens"].(int64); !ok || got < 50 {
			t.Fatalf("header_tokens = %v, want >= 50", fields["header_tokens"])
		}
	})

	t.Run("header under budget stays quiet", func(t *testing.T) {
		logs := newObservedLogs(t)
		// makeTable's header costs 52 tokens as HTML; budget 100 stays above it.
		splitLargeTable(makeTable(3), 100, charTokens, "doc-123")

		if n := logs.FilterMessage(msg).Len(); n != 0 {
			t.Fatalf("want no warning, got %d (logs=%v)", n, logs.All())
		}
	})
}
