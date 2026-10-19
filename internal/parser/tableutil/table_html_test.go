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
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// mustParse is a test helper that fails the test on parse error.
func mustParse(t *testing.T, html string) *entity.TableData {
	t.Helper()
	td, err := ParseTableHTML(html)
	if err != nil {
		t.Fatalf("ParseTableHTML(%q) error = %v", html, err)
	}
	return td
}

func TestParseTableHTMLHappyPath(t *testing.T) {
	html := `<table><tr><th>Name</th><th>Age</th></tr>` +
		`<tr><td>Alice</td><td>30</td></tr>` +
		`<tr><td>Bob</td><td>25</td></tr></table>`
	td := mustParse(t, html)
	if td.HeaderRows != 1 {
		t.Fatalf("HeaderRows = %d, want 1", td.HeaderRows)
	}
	want := [][]string{
		{"Name", "Age"},
		{"Alice", "30"},
		{"Bob", "25"},
	}
	assertRowsEqual(t, td.CellTexts(), want)
}

func TestParseTableHTMLMultipleHeaderRows(t *testing.T) {
	html := `<table><tr><th>H1</th></tr><tr><th>H2</th></tr>` +
		`<tr><td>a</td></tr></table>`
	td := mustParse(t, html)
	if td.HeaderRows != 2 {
		t.Fatalf("HeaderRows = %d, want 2", td.HeaderRows)
	}
}

func TestParseTableHTMLNoThTreatsFirstRowAsHeader(t *testing.T) {
	html := `<table><tr><td>Name</td><td>Age</td></tr>` +
		`<tr><td>Alice</td><td>30</td></tr></table>`
	td := mustParse(t, html)
	if td.HeaderRows != 1 {
		t.Fatalf("HeaderRows = %d, want 1 (no <th> => first row is header)", td.HeaderRows)
	}
}

func TestParseTableHTMLEmpty(t *testing.T) {
	td := mustParse(t, `<table></table>`)
	if len(td.CellTexts()) != 0 {
		t.Fatalf("Rows = %v, want empty", td.CellTexts())
	}
	if td.HeaderRows != 0 {
		t.Fatalf("HeaderRows = %d, want 0", td.HeaderRows)
	}
}

func TestParseTableHTMLTrimsCellWhitespace(t *testing.T) {
	html := `<table><tr><td>  spaced  </td></tr></table>`
	td := mustParse(t, html)
	if got := td.CellTexts()[0][0]; got != "spaced" {
		t.Fatalf("cell = %q, want %q", got, "spaced")
	}
}

func TestParseTableHTMLBrBecomesNewline(t *testing.T) {
	html := `<table><tr><td>line1<br>line2</td></tr></table>`
	td := mustParse(t, html)
	if got := td.CellTexts()[0][0]; got != "line1\nline2" {
		t.Fatalf("cell = %q, want %q", got, "line1\nline2")
	}
}

func TestParseTableHTMLInertElementsIgnored(t *testing.T) {
	// script/style/template content must not appear in any cell.
	html := `<table><tr><td>keep<script>alert(1)</script>middle` +
		`<style>.x{}</style>end</td></tr></table>`
	td := mustParse(t, html)
	got := td.CellTexts()[0][0]
	if strings.Contains(got, "alert") || strings.Contains(got, ".x{}") {
		t.Fatalf("inert content leaked into cell: %q", got)
	}
	if !strings.Contains(got, "keep") || !strings.Contains(got, "middle") || !strings.Contains(got, "end") {
		t.Fatalf("visible text lost: %q", got)
	}
}

func TestParseTableHTMLNestedTableFoldedIntoCell(t *testing.T) {
	html := `<table><tr><td>outer<table><tr><td>inner</td></tr></table></td></tr></table>`
	td := mustParse(t, html)
	// Outer <tr> yields exactly one row with one cell; the nested <tr> must
	// NOT become a separate row.
	if len(td.CellTexts()) != 1 {
		t.Fatalf("len(Rows) = %d, want 1 (nested tr not a row)", len(td.CellTexts()))
	}
	got := td.CellTexts()[0][0]
	if !strings.Contains(got, "outer") || !strings.Contains(got, "inner") {
		t.Fatalf("nested table text not folded: %q", got)
	}
	if strings.Contains(got, "<table") {
		t.Fatalf("nested markup not stripped: %q", got)
	}
}

func TestParseTableHTMLBareTrWrapped(t *testing.T) {
	// Legacy behaviour: a bare <tr> fragment without <table> still parses.
	html := `<tr><td>a</td><td>b</td></tr>`
	td := mustParse(t, html)
	assertRowsEqual(t, td.CellTexts(), [][]string{{"a", "b"}})
}

// A huge colspan on one row drives the grid width up, but each row's backing
// slice must be sized to its OWN column count, not the whole-table width.
// buildGrid used to pre-allocate every row to the full table width, so a small
// HTML input with one wide row could allocate megabytes for every short row
// that followed. The dense grid stays ragged (short rows are not padded), so a
// short row's capacity must equal its actual length.
func TestParseTableHTMLRowCapacityNotFullWidth(t *testing.T) {
	td := mustParse(t, "<table><tr><td colspan=\"1000\">WIDE</td></tr><tr><td>a</td><td>b</td></tr></table>")
	if len(td.Grid) != 2 {
		t.Fatalf("got %d rows, want 2", len(td.Grid))
	}
	// Row 1 is short (two 1x1 cells, no ancestor rowspan): its length is 2 and
	// its backing capacity must equal that, not the full table width (1000).
	if len(td.Grid[1]) != 2 {
		t.Fatalf("row1 len = %d, want 2", len(td.Grid[1]))
	}
	if cap(td.Grid[1]) != len(td.Grid[1]) {
		t.Fatalf("row1 cap = %d, want %d (no full-width pre-allocation)", cap(td.Grid[1]), len(td.Grid[1]))
	}
}

func TestParseTableHTMLCapturesColspan(t *testing.T) {
	// A colspan is captured, not discarded: the wide cell keeps its text on the
	// origin slot (ColSpan=2) and the engulfed position becomes a Covered
	// marker, but the text view still yields the two real cell texts.
	html := `<table><tr><td colspan="2">wide</td><td>narrow</td></tr></table>`
	td := mustParse(t, html)
	row := td.Grid[0]
	if len(row) != 3 {
		t.Fatalf("grid width = %d, want 3 (origin + covered + narrow)", len(row))
	}
	if row[0].Text != "wide" || row[0].ColSpan != 2 || row[0].RowSpan != 1 {
		t.Fatalf("origin slot wrong: %+v", row[0])
	}
	if !row[1].Covered || row[1].OriginCol != 0 {
		t.Fatalf("engulfed slot must be Covered pointing at col 0: %+v", row[1])
	}
	if row[2].Text != "narrow" || row[2].ColSpan != 1 {
		t.Fatalf("narrow slot wrong: %+v", row[2])
	}
	assertRowsEqual(t, td.CellTexts(), [][]string{{"wide", "narrow"}})
}

func TestRenderTableHTMLRoundTrip(t *testing.T) {
	td := &entity.TableData{
		Grid: [][]entity.TableCell{
			{{Text: "Name"}, {Text: "Age"}},
			{{Text: "Alice"}, {Text: "30"}},
		},
		HeaderRows: 1,
		Caption:    "People",
	}
	rendered := RenderTableHTML(td)
	back := mustParse(t, rendered)
	assertRowsEqual(t, back.CellTexts(), td.CellTexts())
	if back.HeaderRows != td.HeaderRows {
		t.Fatalf("HeaderRows round-trip = %d, want %d", back.HeaderRows, td.HeaderRows)
	}
	if back.Caption != td.Caption {
		t.Fatalf("Caption round-trip = %q, want %q", back.Caption, td.Caption)
	}
}

func assertRowsEqual(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("row count = %d, want %d\n got=%v\nwant=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("row %d cell count = %d, want %d\n got=%v\nwant=%v", i, len(got[i]), len(want[i]), got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("cell [%d][%d] = %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}
