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

package runtime

import (
	"fmt"
	"strings"
	"testing"
)

// TestTableViewInfoboxBecomesFieldObjects pins the two-column shape: a Wikipedia infobox row
// renders as one field object per row, with the CELL ITSELF as the field name.
func TestTableViewInfoboxBecomesFieldObjects(t *testing.T) {
	raw := `<table>
		<tr><th>Born</th><td>1961</td></tr>
		<tr><th>Children</th><td>3</td></tr>
		<tr><th>Spouse</th><td>Jane Doe</td></tr>
	</table>`
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view for an infobox")
	}
	want := `{"Born": "1961"}` + "\n" +
		`{"Children": "3"}` + "\n" +
		`{"Spouse": "Jane Doe"}`
	if view != want {
		t.Errorf("infobox view = %q, want %q", view, want)
	}
}

// TestTableViewRankedTableIsKeyedByColumns pins the multi-column shape: the header is carried
// ONCE as "columns", and every data row is keyed by those column names (not by its first cell),
// in column order.
func TestTableViewRankedTableIsKeyedByColumns(t *testing.T) {
	raw := `<table>
		<tr><th>Rank</th><th>Name</th><th>Points</th></tr>
		<tr><td>1</td><td>Alice</td><td>90</td></tr>
		<tr><td>2</td><td>Bob</td><td>88</td></tr>
		<tr><td>3</td><td>Carol</td><td>71</td></tr>
	</table>`
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view for a ranked table")
	}
	want := `{"columns": ["Rank","Name","Points"]}` + "\n" +
		`{"Rank": "1", "Name": "Alice", "Points": "90"}` + "\n" +
		`{"Rank": "2", "Name": "Bob", "Points": "88"}` + "\n" +
		`{"Rank": "3", "Name": "Carol", "Points": "71"}`
	if view != want {
		t.Errorf("ranked view = %q, want %q", view, want)
	}
}

// TestTableViewSingleTHNameRowIsNotAHeader pins the shape a Wikipedia infobox actually starts
// with: a one-cell all-<th> row holding the subject's NAME. It is not a list of columns, so it
// stays a text line and the fields that follow keep their own key.
func TestTableViewSingleTHNameRowIsNotAHeader(t *testing.T) {
	raw := `<table>
		<tr><th colspan="2">Brendan Fraser</th></tr>
		<tr><th>Born</th><td>1968</td></tr>
		<tr><th>Children</th><td>3</td></tr>
	</table>`
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	if strings.Contains(view, `"columns"`) {
		t.Errorf("view = %q, want no column header taken from the name row", view)
	}
	if !strings.HasPrefix(view, "Brendan Fraser\n") {
		t.Errorf("view = %q, want the name row kept as text", view)
	}
	if !strings.Contains(view, `{"Children": "3"}`) {
		t.Errorf("view = %q, want the field rows keyed by their own cells", view)
	}
}

// TestTableViewCaptionAndUnitsRow covers the caption object and the units row a Wikipedia table
// carries right under its header ("No. | % | No. | %"): the row holds no data and must not become
// a field.
func TestTableViewCaptionAndUnitsRow(t *testing.T) {
	raw := `<table>
		<caption>Final standings</caption>
		<tr><th>Rank</th><th>Rider</th><th>Points</th></tr>
		<tr><td>No.</td><td>%</td><td>#</td></tr>
		<tr><td>19</td><td>Danilo</td><td>62</td></tr>
	</table>`
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	if !strings.HasPrefix(view, `{"caption": "Final standings"}`+"\n") {
		t.Errorf("view = %q, want the caption as its own object", view)
	}
	if !strings.Contains(view, `{"Rank": "19", "Rider": "Danilo", "Points": "62"}`) {
		t.Errorf("view = %q, want the rank-19 row kept", view)
	}
	if strings.Contains(view, "No.") || strings.Contains(view, "%") {
		t.Errorf("view = %q, want the units row dropped", view)
	}
}

// TestTableViewEscapesValuesAsJSON pins the escaping: a value with a quote, a pipe or a backslash
// must come back as valid JSON, since the model reads the object as JSON.
func TestTableViewEscapesValuesAsJSON(t *testing.T) {
	raw := `<table>
		<tr><th>Rank</th><th>Name</th><th>Note</th></tr>
		<tr><td>1</td><td>Alice</td><td>a|b</td></tr>
		<tr><td>2</td><td>Bob</td><td>c\d</td></tr>
	</table>`
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	if !strings.Contains(view, `{"Rank": "1", "Name": "Alice", "Note": "a|b"}`) {
		t.Errorf("view = %q, want the pipe kept as data", view)
	}
	if !strings.Contains(view, `"Note": "c\\d"`) {
		t.Errorf("view = %q, want the backslash escaped", view)
	}
	if !strings.Contains(view, `{"Rank": "2", "Name": "Bob", "Note": "c\\d"}`) {
		t.Errorf("view = %q, want the second row keyed by its columns", view)
	}
}

// TestTableViewKeepsTextOutsideTheTable pins the surrounding text: a chunk carries its own
// metadata lines before the table (Title/Source/Prepared-Format), and rendering must not throw
// them away — the Source URL is what a citation resolves through.
func TestTableViewKeepsTextOutsideTheTable(t *testing.T) {
	raw := "Title: Brendan Fraser\nSource: https://en.wikipedia.org/wiki/Brendan_Fraser\n\n" +
		`<table><tr><th>Born</th><td>1968</td></tr><tr><th>Children</th><td>3</td></tr></table>` +
		"\n\nNot to be confused with Brandon Frazier."
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	for _, want := range []string{
		"Title: Brendan Fraser",
		"https://en.wikipedia.org/wiki/Brendan_Fraser",
		`{"Children": "3"}`,
		"Not to be confused",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view = %q, want %q kept", view, want)
		}
	}
}

// TestTableViewNoopForNonTable pins the no-op contract: plain text, markdown pipe tables and
// table-less HTML must come back untouched so a caller can route every chunk through
// tableViewOrRaw.
func TestTableViewNoopForNonTable(t *testing.T) {
	for _, raw := range []string{
		"",
		"plain prose with no markup at all",
		"| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |",
		"<p>html without a table</p>",
	} {
		if view, ok := renderTables(raw); ok {
			t.Errorf("RenderTables(%q) = (%q, true), want no view", raw, view)
		}
		if got := tableViewOrRaw(raw); got != raw {
			t.Errorf("TableViewOrRaw(%q) = %q, want the raw text", raw, got)
		}
	}
}

// TestTableViewNoopForEmptyTable pins the all-empty path: a table whose rows carry no text has
// nothing to render, so the caller keeps the raw chunk instead of receiving an empty string.
func TestTableViewNoopForEmptyTable(t *testing.T) {
	for _, raw := range []string{
		"<table></table>",
		"<table><tr><td></td><td>   </td></tr></table>",
		"<table><tr><th>Rank</th></tr></table>",
	} {
		if view, ok := renderTables(raw); ok {
			t.Errorf("RenderTables(%q) = (%q, true), want no view", raw, view)
		}
	}
}

// TestTableViewTruncatedFragmentDoesNotPanic pins the fragment case the table exemption exists
// for: a text window that cut the table mid-markup must never panic and must never emit a partial
// dump.
func TestTableViewTruncatedFragmentDoesNotPanic(t *testing.T) {
	for _, raw := range []string{
		`<table><tr><td>only a fragment`,
		`row text then a stray <table`,
		`<tr><td>a</td></tr><tr><td>b</td></tr>`,
	} {
		if view, ok := renderTables(raw); ok && strings.Contains(view, "\n") && !strings.Contains(raw, "<table") {
			t.Errorf("RenderTables(%q) = %q: a table-less fragment must not render", raw, view)
		}
		if got := tableViewOrRaw(raw); got == "" && raw != "" {
			t.Errorf("TableViewOrRaw(%q) returned an empty string", raw)
		}
	}
}

// TestTableViewJoinsMultipleTables pins the multi-table shape: a chunk that carries two nominee
// infoboxes hands the model two blocks, separated by a blank line, each with its own field objects.
func TestTableViewJoinsMultipleTables(t *testing.T) {
	raw := `<table>
		<tr><th>Born</th><td>1961</td></tr>
		<tr><th>Children</th><td>3</td></tr>
		<tr><th>Spouse</th><td>Jane</td></tr>
	</table>
	<table>
		<tr><th>Born</th><td>1954</td></tr>
		<tr><th>Children</th><td>2</td></tr>
		<tr><th>Spouse</th><td>John</td></tr>
	</table>`
	view, ok := renderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	blocks := strings.Split(view, "\n\n")
	if len(blocks) != 2 {
		t.Fatalf("view = %q, want 2 blocks", view)
	}
	for i, block := range blocks {
		if !strings.Contains(block, `"Children"`) {
			t.Errorf("block %d = %q, want a field object", i, block)
		}
	}
}

// TestTableViewCapsRowsAndReportsOmission pins the ceiling: an oversized table keeps
// tableViewMaxRows data rows and SAYS how many were dropped, so a truncated table is never read
// as a complete one. The row cells are deliberately short so the character ceiling does not cut
// the omission marker away before the row ceiling is reached.
func TestTableViewCapsRowsAndReportsOmission(t *testing.T) {
	var b strings.Builder
	b.WriteString("<table><tr><th>Rank</th><th>Rider</th><th>Team</th></tr>")
	for i := 1; i <= tableViewMaxRows+2; i++ {
		fmt.Fprintf(&b, "<tr><td>%d</td><td>R%d</td><td>T%d</td></tr>", i, i, i)
	}
	b.WriteString("</table>")

	view, ok := renderTables(b.String())
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	rows := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, `"Rank": `) {
			rows++
		}
	}
	if rows != tableViewMaxRows {
		t.Errorf("rendered %d row object(s), want the cap %d", rows, tableViewMaxRows)
	}
	if !strings.HasSuffix(view, "... (2 more row(s) omitted)") {
		t.Errorf("view tail = %q, want the omission marker", view[len(view)-40:])
	}
}
