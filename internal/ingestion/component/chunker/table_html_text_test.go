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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// generalTableChunksOf drives GeneralChunker.Invoke over structured TableData
// items and returns the emitted chunk maps.
func generalTableChunksOf(t *testing.T, params map[string]any, items ...map[string]any) []map[string]any {
	t.Helper()
	component, err := NewGeneralChunker(params)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("output chunks type = %T, want []map[string]any", out["chunks"])
	}
	return chunks
}

// --- Table chunker: structured TableData renders to HTML text ---

func TestTableChunkerEmitsHTMLTextPerRow(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><tr><th>Name</th><th>Age</th></tr><tr><td>Alice</td><td>30</td></tr><tr><td>Bob</td><td>25</td></tr></table>"),
		},
	})
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (one per data row)", len(chunks))
	}
	for i, want := range []string{"Alice", "Bob"} {
		text, ok := chunks[i]["text"].(string)
		if !ok {
			t.Fatalf("chunk %d text missing", i)
		}
		if !strings.Contains(text, "<table>") || !strings.Contains(text, "<td>"+want+"</td>") {
			t.Fatalf("chunk %d text not HTML with its row: %q", i, text)
		}
		// The header must ride along so the rendered row is a real table.
		if !strings.Contains(text, "<th>Name</th>") || !strings.Contains(text, "<th>Age</th>") {
			t.Fatalf("chunk %d HTML missing header cells: %q", i, text)
		}
		// The structured payload must not survive onto the persisted chunk.
		if _, exists := chunks[i]["table"]; exists {
			t.Fatalf("chunk %d still carries structured table: %#v", i, chunks[i]["table"])
		}
	}
}

func TestTableChunkerEmitsHTMLTextHeaderOnly(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><tr><th>Name</th><th>Age</th></tr></table>"),
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (header-only)", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	if !strings.Contains(text, "<table>") || !strings.Contains(text, "<th>Name</th>") {
		t.Fatalf("header-only chunk text not HTML: %q", text)
	}
	if _, exists := chunks[0]["table"]; exists {
		t.Fatalf("header-only chunk still carries structured table")
	}
}

func TestTableChunkerEmitsHTMLTextKeepsAllHeaderRows(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><tr><th>Group</th><th>Metric</th></tr><tr><th>A</th><th>B</th></tr><tr><td>x</td><td>1</td></tr></table>"),
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	// Both projected header rows must survive into the rendered HTML.
	for _, want := range []string{"<th>Group</th>", "<th>Metric</th>", "<th>A</th>", "<th>B</th>", "<td>x</td>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("multi-header HTML missing %q: %q", want, text)
		}
	}
}

func TestTableChunkerEmitsHTMLTextEscapesSpecialChars(t *testing.T) {
	// Build the structured TableData directly so the cell bodies literally
	// contain <, >, & and " — characters the HTML parser would otherwise treat
	// as markup. The rendered chunk text must escape every one of them so the
	// persisted HTML stays well-formed and no cell body leaks as a live tag.
	td := &entity.TableData{
		Grid: [][]entity.TableCell{
			{{Text: "k<>&\""}},
			{{Text: "a<b>&\"c\""}},
		},
		HeaderRows: 1,
	}
	b, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var tbl map[string]any
	if err := json.Unmarshal(b, &tbl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json":          []map[string]any{{"table": tbl, "doc_type_kwd": "table"}},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	for _, want := range []string{"&lt;", "&gt;", "&amp;", "&#34;"} {
		if !strings.Contains(text, want) {
			t.Fatalf("cell body not HTML-escaped (missing %q): %q", want, text)
		}
	}
	if strings.Contains(text, "<b>") {
		t.Fatalf("cell markup leaked unescaped into HTML: %q", text)
	}
}

func TestTableChunkerEmitsHTMLTextCaption(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><caption>Q1 Sales</caption><tr><th>k</th></tr><tr><td>v</td></tr></table>"),
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	if !strings.Contains(text, "<caption>Q1 Sales</caption>") {
		t.Fatalf("caption not rendered into HTML: %q", text)
	}
}

func TestTableChunkerEmitsHTMLTextNilTableDataPassthrough(t *testing.T) {
	// An item without structured TableData is not a table payload: it must
	// pass through untouched (text preserved), never rendered.
	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "plain body", "doc_type_kwd": "text"},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if got := chunks[0]["text"]; got != "plain body" {
		t.Fatalf("non-table text mutated: %q", got)
	}
}

func TestTableChunkerEmitsHTMLTextWideRowStaysWhole(t *testing.T) {
	// A single very wide row must become exactly one chunk whose HTML carries
	// every cell — the row is never cut across chunks.
	var b strings.Builder
	b.WriteString("<table><tr>")
	for i := 0; i < 40; i++ {
		b.WriteString("<th>h")
		b.WriteString(strings.Repeat("x", i%5))
		b.WriteString("</th>")
	}
	b.WriteString("</tr><tr>")
	for i := 0; i < 40; i++ {
		b.WriteString("<td>cell")
		b.WriteByte(byte('a' + i%26))
		b.WriteString("</td>")
	}
	b.WriteString("</tr></table>")

	chunks := tableChunksOf(t, map[string]any{
		"name":          "sheet.csv",
		"output_format": "json",
		"json":          []map[string]any{tableItem(t, b.String())},
	})
	if len(chunks) != 1 {
		t.Fatalf("wide row produced %d chunks, want 1 (row never cut)", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	for _, want := range []string{"<td>cella</td>", "<td>cells</td>", "<td>cellz</td>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("wide-row HTML missing %q", want)
		}
	}
}

// --- General chunker: structured spreadsheet table renders to HTML text ---

func TestGeneralChunkerEmitsHTMLTextSingleChunk(t *testing.T) {
	// A small table that fits the budget becomes a single chunk whose text is
	// the whole table rendered as HTML (not the plain-text projection).
	chunks := generalTableChunksOf(t, nil, tableItem(t,
		"<table><caption>People</caption><tr><th>Name</th><th>Age</th></tr><tr><td>Alice</td><td>30</td></tr></table>"))
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	text, ok := chunks[0]["text"].(string)
	if !ok {
		t.Fatalf("text missing")
	}
	if !strings.Contains(text, "<table>") || !strings.Contains(text, "<td>Alice</td>") {
		t.Fatalf("single-chunk table text not HTML: %q", text)
	}
	if !strings.Contains(text, "<caption>People</caption>") {
		t.Fatalf("caption not rendered: %q", text)
	}
	if _, exists := chunks[0]["table"]; exists {
		t.Fatalf("structured table leaked onto chunk: %#v", chunks[0]["table"])
	}
}

func TestGeneralChunkerEmitsHTMLTextLargeTableRowGranular(t *testing.T) {
	// Force a split with a tiny budget. The table must break on row boundaries
	// (never mid-row), and every emitted part must be HTML carrying the header.
	const nRows = 30
	var b strings.Builder
	b.WriteString("<table><tr><th>id</th><th>val</th></tr>")
	for i := 0; i < nRows; i++ {
		b.WriteString(fmt.Sprintf("<tr><td>r%02d</td><td>%s</td></tr>", i, strings.Repeat("v", 8)))
	}
	b.WriteString("</table>")

	chunks := generalTableChunksOf(t, map[string]any{"chunk_token_size": 40}, tableItem(t, b.String()))
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want >=2 (budget forces a split)", len(chunks))
	}
	seen := map[string]bool{}
	for i, ck := range chunks {
		text, ok := ck["text"].(string)
		if !ok || !strings.Contains(text, "<table>") {
			t.Fatalf("part %d not HTML: %q", i, text)
		}
		if !strings.Contains(text, "<th>id</th>") || !strings.Contains(text, "<th>val</th>") {
			t.Fatalf("part %d missing replicated header: %q", i, text)
		}
		// Each part is whole rows only: every <tr> opens and closes.
		if strings.Count(text, "<tr>") != strings.Count(text, "</tr>") {
			t.Fatalf("part %d has unbalanced rows (a row was cut): %q", i, text)
		}
		// Collect the unique data-row ids that landed in this part; a row id
		// appearing in two parts would mean it was split or duplicated.
		rest := text
		for {
			idx := strings.Index(rest, "<td>r")
			if idx < 0 {
				break
			}
			id := rest[idx+4 : idx+7] // "r%02d"
			rest = rest[idx+7:]
			if seen[id] {
				t.Fatalf("row id %q appears in two parts (split/duplicated)", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != nRows {
		t.Fatalf("saw %d distinct rows across parts, want %d (rows lost or merged)", len(seen), nRows)
	}
}

// --- Row-span group integrity across the HTML rendering path ---

func TestGeneralChunkerEmitsHTMLTextRowspanGroupNotSplit(t *testing.T) {
	// A header cell spanning two data rows must keep its group in one part; the
	// rendered HTML must show the colspan/rowspan, and no part may cut the group.
	td := &entity.TableData{
		Grid: [][]entity.TableCell{
			{{Text: "g", RowSpan: 2}, {Text: "m"}},
			{{Text: "", Covered: true, OriginRow: 0, OriginCol: 0}, {Text: "a"}},
			{{Text: "", Covered: true, OriginRow: 0, OriginCol: 0}, {Text: "b"}},
		},
		HeaderRows: 1,
	}
	b, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var tbl map[string]any
	if err := json.Unmarshal(b, &tbl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	chunks := generalTableChunksOf(t, map[string]any{"chunk_token_size": 40},
		map[string]any{"table": tbl, "doc_type_kwd": "table"})
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		// The origin cell and every row it covers must be in the same part:
		// if a part contains "b" (the last spanned data row) it must also
		// contain the origin "g" with its rowspan.
		if strings.Contains(text, "<td>b</td>") {
			if !strings.Contains(text, "rowspan=\"2\"") {
				t.Fatalf("part %d split the rowspan group: %q", i, text)
			}
		}
	}
}
