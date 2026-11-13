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
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// gridOf builds a TableData Grid from row-major cell strings (header + data).
func gridOf(header []string, data ...[]string) [][]entity.TableCell {
	rows := make([][]entity.TableCell, 0, 1+len(data))
	rows = append(rows, cellsOf(header))
	for _, d := range data {
		rows = append(rows, cellsOf(d))
	}
	return rows
}

func cellsOf(vals []string) []entity.TableCell {
	cells := make([]entity.TableCell, len(vals))
	for i, v := range vals {
		cells[i] = entity.TableCell{Text: v}
	}
	return cells
}

// --- A: the shared decode fallback renders structured tables as HTML ---

// A structured table that arrives with only the TableData contract (no Text —
// the office/xlsx producer path) must get an HTML <table> on Text from the
// shared decode fallback, so every chunker that consumes it (token/group/
// hierarchy/one/page/qa) stores and displays a real table, not plain text.
func TestDecodeChunkerFromUpstreamRendersTableHTML(t *testing.T) {
	up, err := decodeChunkerFromUpstream(map[string]any{
		"name":          "sheet.xlsx",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><caption>Q1</caption><tr><th>Name</th><th>Age</th></tr><tr><td>Alice</td><td>30</td></tr></table>"),
		},
	})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(up.JSONResult) != 1 {
		t.Fatalf("got %d items, want 1", len(up.JSONResult))
	}
	text := up.JSONResult[0].Text
	if !strings.Contains(text, "<table>") || !strings.Contains(text, "<td>Alice</td>") {
		t.Fatalf("structured table not rendered to HTML on Text: %q", text)
	}
	if !strings.Contains(text, "<caption>Q1</caption>") {
		t.Fatalf("caption not carried into HTML: %q", text)
	}
	// The fallback fires on the "chunks" payload too (re-chunk path).
	upChunks, err := decodeChunkerFromUpstream(map[string]any{
		"name":          "sheet.xlsx",
		"output_format": "chunks",
		"chunks": []map[string]any{
			tableItem(t, "<table><tr><th>k</th></tr><tr><td>v</td></tr></table>"),
		},
	})
	if err != nil {
		t.Fatalf("decode chunks: %v", err)
	}
	if len(upChunks.Chunks) != 1 || !strings.Contains(upChunks.Chunks[0].Text, "<table>") {
		t.Fatalf("chunks payload table not rendered to HTML: %+v", upChunks.Chunks)
	}
}

// End-to-end confirmation through a real non-specialized chunker: a structured
// table (no Text) fed to the TokenChunker must surface an HTML table chunk.
func TestTokenChunkerEmitsHTMLForStructuredTable(t *testing.T) {
	component, err := NewTokenChunker(nil)
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "sheet.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><tr><th>Name</th><th>Age</th></tr><tr><td>Alice</td><td>30</td></tr></table>"),
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	found := false
	for _, c := range outputChunks(t, out) {
		if s, ok := c["text"].(string); ok && strings.Contains(s, "<table>") && strings.Contains(s, "<td>Alice</td>") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no chunk carries the HTML table text: %+v", out)
	}
}

// --- B: the split budget is measured on the HTML that is actually stored ---

// B1: the split budget is measured on the HTML that is actually stored. A
// budget below the whole table's HTML cost forces a split, and every emitted
// part's HTML must then fit the budget — which only holds when the budget is
// counted on the HTML rendering, not the smaller clean projection (under a
// clean-projection budget each part's HTML would overshoot).
func TestSplitLargeTableBudgetsOnHTML(t *testing.T) {
	data := make([][]string, 0, 10)
	for i := 0; i < 10; i++ {
		data = append(data, []string{"row-" + string(rune('a'+i)), "a moderately long padded value number " + string(rune('a'+i))})
	}
	td := &entity.TableData{Grid: gridOf([]string{"id", "value"}, data...), HeaderRows: 1}

	whole := tokenizeStr(tableutil.RenderTableHTML(td))
	budget := whole / 2

	parts, ranges, headerRows := splitLargeTable(td, budget, tokenizeStr, "")
	if len(parts) < 2 {
		t.Fatalf("table did not split under the HTML budget: whole=%d budget=%d parts=%d", whole, budget, len(parts))
	}
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1", headerRows)
	}
	// The split budget covers data rows only: every part replicates the header,
	// so a part may exceed the budget by up to the header's own HTML cost —
	// but never beyond that (which is what clean-projection counting would do).
	headerTokens := tokenizeStr(tableutil.RenderTableHTML(&entity.TableData{Grid: td.Grid[:1], HeaderRows: 1}))
	covered := 0
	for i, p := range parts {
		if n := tokenizeStr(tableutil.RenderTableHTML(&p)); n > budget+headerTokens {
			t.Fatalf("part %d HTML tokens %d exceed budget %d + header %d (budget must cover the stored HTML)", i, n, budget, headerTokens)
		}
		covered += ranges[i][1] - ranges[i][0]
	}
	if covered != len(data) {
		t.Fatalf("parts cover %d data rows, want %d (rows lost or duplicated)", covered, len(data))
	}
}

// B2: row integrity is preserved under the HTML budget — a single row whose
// HTML alone exceeds the budget becomes its own whole chunk, never cut.
func TestSplitLargeTableHTMLBudgetKeepsSingleOverBudgetRowWhole(t *testing.T) {
	huge := strings.Repeat("x", 400)
	td := &entity.TableData{Grid: gridOf([]string{"id", "blob"},
		[]string{"r1", "small"},
		[]string{"r2", huge},
		[]string{"r3", "small"},
	), HeaderRows: 1}

	// Budget below the huge row's HTML but above the header + a small row.
	small := tokenizeStr(tableutil.RenderTableHTML(&entity.TableData{Grid: gridOf([]string{"id", "blob"}, []string{"r1", "small"}), HeaderRows: 1}))
	hugeRow := tokenizeStr(tableutil.RenderTableHTML(&entity.TableData{Grid: gridOf([]string{"id", "blob"}, []string{"r2", huge}), HeaderRows: 1}))
	budget := (small + hugeRow) / 2
	if budget >= hugeRow {
		t.Fatalf("test setup: budget %d should be below the huge row HTML %d", budget, hugeRow)
	}

	parts, _, _ := splitLargeTable(td, budget, tokenizeStr, "")
	if len(parts) < 2 {
		t.Fatalf("expected the huge row to isolate into its own part, got %d parts", len(parts))
	}
	// The part carrying the huge row must contain it whole (its blob intact).
	found := false
	for _, p := range parts {
		if n := tokenizeStr(tableutil.RenderTableHTML(&p)); n > budget {
			// An over-budget part is only acceptable when it is the single
			// huge row standing whole (row integrity wins over the budget).
			if len(p.Grid) != 2 { // header + the one data row
				t.Fatalf("over-budget part has %d rows, want the single huge row whole", len(p.Grid))
			}
			if p.Grid[1][1].Text != huge {
				t.Fatalf("over-budget part lost the huge row's content")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("the huge row was not emitted as its own over-budget part")
	}
}

// B3: span-safe cut is preserved under the HTML budget — a data-row cell whose
// rowspan covers the next data row keeps that group in one part (never split
// between the spanned rows).
func TestSplitLargeTableHTMLBudgetKeepsRowspanGroup(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "h1"}, {Text: "h2"}},                                         // header
			{{Text: "grp", RowSpan: 2}, {Text: "a"}},                             // data 0: grp spans data 0,1
			{{Text: "", Covered: true, OriginRow: 1, OriginCol: 0}, {Text: "b"}}, // data 1: covered by grp
			{{Text: "solo"}, {Text: "c"}},                                        // data 2
		},
	}
	// Budget = the HTML of just the header + the first (spanning) data row, so
	// the naive cut falls right after data 0; span-safe cut must pull data 1 in.
	budget := tokenizeStr(tableutil.RenderTableHTML(&entity.TableData{
		Grid:       [][]entity.TableCell{td.Grid[0], td.Grid[1]},
		HeaderRows: 1,
	}))
	parts, _, _ := splitLargeTable(td, budget, tokenizeStr, "")
	if len(parts) < 2 {
		t.Fatalf("test setup: expected a split, got %d parts", len(parts))
	}
	// Whatever the split, the spanned rows "a" and "b" must share a part.
	partOf := map[string]int{}
	for i, p := range parts {
		html := tableutil.RenderTableHTML(&p)
		if strings.Contains(html, "<td>a</td>") {
			partOf["a"] = i
		}
		if strings.Contains(html, "<td>b</td>") {
			partOf["b"] = i
		}
	}
	if _, okA := partOf["a"]; !okA {
		t.Fatalf("spanned row 'a' missing from all parts: %+v", parts)
	}
	if _, okB := partOf["b"]; !okB {
		t.Fatalf("spanned row 'b' missing from all parts: %+v", parts)
	}
	if partOf["a"] != partOf["b"] {
		t.Fatalf("rowspan group split across parts: a in %d, b in %d", partOf["a"], partOf["b"])
	}
}
