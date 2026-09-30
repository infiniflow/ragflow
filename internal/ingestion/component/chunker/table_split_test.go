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
)

// charTokens approximates a tokenizer for tests: one token per byte.
func charTokens(s string) int { return len(s) }

func buildTable(header string, data []string) string {
	var b strings.Builder
	b.WriteString("<table><caption>cap</caption>\n")
	b.WriteString("<tr>" + header + "</tr>\n")
	for _, row := range data {
		b.WriteString("<tr>" + row + "</tr>\n")
	}
	b.WriteString("</table>\n")
	return b.String()
}

func TestSplitLargeHTMLTableNoSplit(t *testing.T) {
	small := buildTable("<th>a</th><th>b</th>", []string{"<td>1</td><td>2</td>"})
	parts, ranges, headerRows := splitLargeHTMLTable(small, len(small)+100, charTokens)
	if len(parts) != 1 || parts[0] != small || ranges != nil || headerRows != 0 {
		t.Fatalf("small table must return unchanged with nil ranges, got %d parts ranges=%v headerRows=%d", len(parts), ranges, headerRows)
	}
	parts, ranges, headerRows = splitLargeHTMLTable("no table here", 1, charTokens)
	if len(parts) != 1 || parts[0] != "no table here" || ranges != nil || headerRows != 0 {
		t.Fatalf("non-table text must pass through unchanged")
	}
}

func TestSplitLargeHTMLTableReplicatesHeaderAndSlicesRanges(t *testing.T) {
	data := make([]string, 6)
	for i := range data {
		data[i] = "<td>d" + string(rune('0'+i)) + "</td><td>x</td>"
	}
	text := buildTable("<th>h1</th><th>h2</th>", data)
	// Budget that holds header + 2 data rows at most.
	rowLen := len("<td>d0</td><td>x</td>")
	budget := len("<table><caption>cap</caption><tr><th>h1</th><th>h2</th></tr></table>") + 2*rowLen
	parts, ranges, headerRows := splitLargeHTMLTable(text, budget, charTokens)
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1 (the single <th> row)", headerRows)
	}
	if len(parts) < 3 {
		t.Fatalf("expected several sub-tables, got %d", len(parts))
	}
	if len(ranges) != len(parts) {
		t.Fatalf("ranges count %d != parts count %d", len(ranges), len(parts))
	}
	seen := 0
	for i, p := range parts {
		if !strings.HasPrefix(p, "<table><caption>cap</caption>") {
			t.Errorf("part %d lost its caption: %q", i, p)
		}
		if !strings.Contains(p, "<th>h1</th>") {
			t.Errorf("part %d lost the header row", i)
		}
		lo, hi := ranges[i][0], ranges[i][1]
		if hi <= lo || hi > len(data) {
			t.Fatalf("part %d range [%d,%d) out of bounds", i, lo, hi)
		}
		for r := lo; r < hi; r++ {
			if !strings.Contains(p, data[r]) {
				t.Errorf("part %d missing row %d", i, r)
			}
			seen++
		}
		// Rows outside the range must not leak into this part.
		for r := 0; r < len(data); r++ {
			if (r < lo || r >= hi) && strings.Contains(p, data[r]) {
				t.Errorf("part %d leaked row %d outside [%d,%d)", i, r, lo, hi)
			}
		}
	}
	if seen != len(data) {
		t.Errorf("rows covered %d, want %d", seen, len(data))
	}
}

func TestSplitLargeHTMLTableHeaderlessReplicatesFirstRow(t *testing.T) {
	// Original behaviour: with no <th> row the first row is treated as the
	// repeating header.
	var b strings.Builder
	b.WriteString("<table>")
	b.WriteString("<tr><td>r0</td></tr>")
	for i := 1; i < 5; i++ {
		b.WriteString("<tr><td>row" + string(rune('0'+i)) + "</td></tr>")
	}
	b.WriteString("</table>")
	parts, ranges, headerRows := splitLargeHTMLTable(b.String(), len("<table><tr><td>r0</td></tr><tr><td>row1</td></tr></table>")+10, charTokens)
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1 (first row treated as header)", headerRows)
	}
	if len(parts) < 2 {
		t.Fatalf("expected split, got %d parts", len(parts))
	}
	for i, p := range parts {
		if !strings.Contains(p, "<td>r0</td>") {
			t.Errorf("part %d lost replicated first row", i)
		}
	}
	if ranges[0][0] != 0 {
		t.Errorf("ranges index DATA rows below the treated header, want first range to start at 0, got %v", ranges[0])
	}
}

// TestSplitLargeHTMLTableRefusesUncuttableMarkup: an open tag that never
// closes, a longer tag name, or a nested table is markup this splitter must
// not cut at a guessed boundary — the text comes back unchanged. The
// "<table </table>" case used to invert the slice bounds.
func TestSplitLargeHTMLTableRefusesUncuttableMarkup(t *testing.T) {
	unchanged := []string{
		"<table </table>",
		"<tableau><tr><td>a</td></tr><tr><td>b</td></tr></table>",
		"<table><tr><td>a</td></tr><table><tr><td>b</td></tr></table></table>",
	}
	for _, text := range unchanged {
		parts, ranges, headerRows := splitLargeHTMLTable(text, 1, charTokens)
		if len(parts) != 1 || parts[0] != text || ranges != nil || headerRows != 0 {
			t.Errorf("text %q must pass through unchanged, got %d parts ranges=%v headerRows=%d", text, len(parts), ranges, headerRows)
		}
	}
}

// TestSplitLargeHTMLTableQuotedAngleBracketStaysInOpenTag: a ">" inside a
// quoted attribute value must not end the open tag — every part repeats the
// complete tag instead of being cut at the attribute.
func TestSplitLargeHTMLTableQuotedAngleBracketStaysInOpenTag(t *testing.T) {
	text := "<table data-meta=\"a>b\">" +
		"<tr><td>r0</td></tr><tr><td>r1</td></tr><tr><td>r2</td></tr></table>"
	parts, ranges, headerRows := splitLargeHTMLTable(text, 60, charTokens)
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1 (first row treated as header)", headerRows)
	}
	if len(parts) < 2 {
		t.Fatalf("expected a split, got %d parts", len(parts))
	}
	if len(ranges) != len(parts) {
		t.Fatalf("ranges count %d != parts count %d", len(ranges), len(parts))
	}
	for i, p := range parts {
		if !strings.HasPrefix(p, "<table data-meta=\"a>b\">") {
			t.Errorf("part %d lost the complete open tag: %q", i, p)
		}
	}
}
