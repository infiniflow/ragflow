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

// TestTableViewInfoboxBecomesKeyValue pins the two-column shape: a Wikipedia infobox
// renders as "key: value" lines, which is the form a field lookup reads best.
func TestTableViewInfoboxBecomesKeyValue(t *testing.T) {
	raw := `<table>
		<tr><th>Born</th><td>1961</td></tr>
		<tr><th>Children</th><td>3</td></tr>
		<tr><th>Spouse</th><td>Jane Doe</td></tr>
	</table>`
	view, ok := RenderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view for an infobox")
	}
	want := "Born: 1961\nChildren: 3\nSpouse: Jane Doe"
	if view != want {
		t.Errorf("infobox view = %q, want %q", view, want)
	}
}

// TestTableViewRankedTableKeepsEveryRow pins the pipe shape: rank/order/completeness
// decide ranked-table answers, so every data row must survive verbatim.
func TestTableViewRankedTableKeepsEveryRow(t *testing.T) {
	raw := `<table>
		<tr><th>Rank</th><th>Name</th><th>Points</th></tr>
		<tr><td>1</td><td>Alice</td><td>90</td></tr>
		<tr><td>2</td><td>Bob</td><td>88</td></tr>
		<tr><td>3</td><td>Carol</td><td>71</td></tr>
	</table>`
	view, ok := RenderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view for a ranked table")
	}
	want := "| Rank | Name | Points |\n" +
		"|---|---|---|\n" +
		"| 1 | Alice | 90 |\n" +
		"| 2 | Bob | 88 |\n" +
		"| 3 | Carol | 71 |"
	if view != want {
		t.Errorf("ranked view = %q, want %q", view, want)
	}
}

// TestTableViewCaptionAndUnitsRow covers the caption header and the units row a
// Wikipedia table carries right under its header ("No. | % | No. | %"): the row holds
// no data and would only pollute the pipe body.
func TestTableViewCaptionAndUnitsRow(t *testing.T) {
	raw := `<table>
		<caption>Final standings</caption>
		<tr><th>Rank</th><th>Rider</th><th>Points</th></tr>
		<tr><td>No.</td><td>%</td><td>#</td></tr>
		<tr><td>19</td><td>Danilo</td><td>62</td></tr>
	</table>`
	view, ok := RenderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	if !strings.HasPrefix(view, "**Table: Final standings**\n") {
		t.Errorf("view = %q, want the caption as a bold header line", view)
	}
	if !strings.Contains(view, "| 19 | Danilo | 62 |") {
		t.Errorf("view = %q, want the rank-19 row kept", view)
	}
	if strings.Contains(view, "No.") || strings.Contains(view, "| % |") {
		t.Errorf("view = %q, want the units row dropped", view)
	}
}

// TestTableViewEscapesPipeAndBackslash pins the cell escaping: an unescaped "|" would
// split the row into extra columns.
func TestTableViewEscapesPipeAndBackslash(t *testing.T) {
	raw := `<table>
		<tr><th>Rank</th><th>Name</th><th>Note</th></tr>
		<tr><td>1</td><td>Alice</td><td>a|b</td></tr>
		<tr><td>2</td><td>Bob</td><td>c\d</td></tr>
	</table>`
	view, ok := RenderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	if !strings.Contains(view, `| a\|b |`) {
		t.Errorf("view = %q, want the pipe escaped", view)
	}
	if !strings.Contains(view, `| c\\d |`) {
		t.Errorf("view = %q, want the backslash escaped", view)
	}
}

// TestTableViewNoopForNonTable pins the no-op contract: plain text, markdown pipe
// tables and table-less HTML must come back untouched so a caller can route every
// chunk through TableViewOrRaw.
func TestTableViewNoopForNonTable(t *testing.T) {
	for _, raw := range []string{
		"",
		"plain prose with no markup at all",
		"| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |",
		"<p>html without a table</p>",
	} {
		if view, ok := RenderTables(raw); ok {
			t.Errorf("RenderTables(%q) = (%q, true), want no view", raw, view)
		}
		if got := TableViewOrRaw(raw); got != raw {
			t.Errorf("TableViewOrRaw(%q) = %q, want the raw text", raw, got)
		}
	}
}

// TestTableViewNoopForEmptyTable pins the all-empty path: a table whose rows carry no
// text has nothing to render, so the caller keeps the raw chunk instead of receiving an
// empty string.
func TestTableViewNoopForEmptyTable(t *testing.T) {
	for _, raw := range []string{
		"<table></table>",
		"<table><tr><td></td><td>   </td></tr></table>",
		"<table><tr><th>Rank</th></tr></table>",
	} {
		if view, ok := RenderTables(raw); ok {
			t.Errorf("RenderTables(%q) = (%q, true), want no view", raw, view)
		}
	}
}

// TestTableViewTruncatedFragmentDoesNotPanic pins the fragment case the table
// exemption exists for: a text window that cut the table mid-markup must never panic
// and must never emit a partial pipe dump.
func TestTableViewTruncatedFragmentDoesNotPanic(t *testing.T) {
	for _, raw := range []string{
		`<table><tr><td>only a fragment`,
		`row text then a stray <table`,
		`<tr><td>a</td></tr><tr><td>b</td></tr>`,
	} {
		if view, ok := RenderTables(raw); ok && strings.Contains(view, "\n") && !strings.Contains(raw, "<table") {
			t.Errorf("RenderTables(%q) = %q: a table-less fragment must not render", raw, view)
		}
		if got := TableViewOrRaw(raw); got == "" && raw != "" {
			t.Errorf("TableViewOrRaw(%q) returned an empty string", raw)
		}
	}
}

// TestTableViewJoinsMultipleTables pins the multi-table shape: a chunk that carries ten
// nominee infoboxes hands the model ten blocks, separated by a blank line.
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
	view, ok := RenderTables(raw)
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	blocks := strings.Split(view, "\n\n")
	if len(blocks) != 2 {
		t.Fatalf("view = %q, want 2 blocks", view)
	}
	for i, block := range blocks {
		if !strings.Contains(block, "Children: ") {
			t.Errorf("block %d = %q, want a key-value view", i, block)
		}
	}
}

// TestTableViewCapsRowsAndReportsOmission pins the ceiling: an oversized table keeps
// tableViewMaxRows data rows and SAYS how many were dropped, so a truncated table is
// never read as a complete one.
func TestTableViewCapsRowsAndReportsOmission(t *testing.T) {
	var b strings.Builder
	b.WriteString("<table><tr><th>Rank</th><th>Rider</th><th>Team</th></tr>")
	for i := 1; i <= tableViewMaxRows+2; i++ {
		fmt.Fprintf(&b, "<tr><td>%d</td><td>Rider %d</td><td>Team %d</td></tr>", i, i, i)
	}
	b.WriteString("</table>")

	view, ok := RenderTables(b.String())
	if !ok {
		t.Fatalf("RenderTables() reported no view")
	}
	rows := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "| ") {
			rows++
		}
	}
	// The header is a pipe line too, so the data rows are one fewer.
	if rows != tableViewMaxRows+1 {
		t.Errorf("rendered %d pipe lines, want %d data rows + 1 header", rows, tableViewMaxRows)
	}
	if !strings.HasSuffix(view, "... (2 more row(s) omitted)") {
		t.Errorf("view tail = %q, want the omission marker", view[len(view)-40:])
	}
}
