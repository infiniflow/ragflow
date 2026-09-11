package chunker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/parser/parser"
)

// xlsxWorkbook renders rows into an in-memory workbook so the tests run
// against the real spreadsheet parser instead of hand-written markup.
func xlsxWorkbook(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	sh := f.GetSheetName(0)
	for i, r := range rows {
		for j, c := range r {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellValue(sh, cell, c); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// qaChunksFromXLSX drives a workbook through the real XLSX parser and then
// the QA chunker, returning the chunks the pipeline would emit.
func qaChunksFromXLSX(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	p, err := parser.NewXLSXParser("")
	if err != nil {
		t.Fatal(err)
	}
	res := p.ParseWithResult(context.Background(), "qa.xlsx", data)
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	inputs := map[string]any{"name": "qa.xlsx", "output_format": res.OutputFormat}
	switch res.OutputFormat {
	case "json":
		inputs["json"] = res.JSON
	case "html":
		inputs["html"] = res.HTML
	}

	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

// TestXLSXQARegression is the end-to-end smoke test: every row of the sheet
// becomes one chunk. The chunker has no header concept, so the first row is
// a Q&A pair like any other.
func TestXLSXQARegression(t *testing.T) {
	chunks := qaChunksFromXLSX(t, xlsxWorkbook(t, [][]string{
		{"question", "answer"},
		{"What is RAGFlow?", "A RAG engine."},
		{"Where are the docs?", "On the website."},
	}))
	t.Logf("QA chunks=%d", len(chunks))
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
}

// A spreadsheet cell keeps the newline its author typed (Alt+Enter) and the
// parser renders it into the <tr>/<td> HTML verbatim, so a QA pair whose
// question or answer spans lines must survive extraction intact. These rows
// used to disappear from the chunk list without a trace.
func TestXLSXQAMultilineCells(t *testing.T) {
	const multilineQ = "请问全国碳排放权交易市场纳入配额管理的重点排放单\n位名录，是否会公布？"
	const multilineA = "需要公布。根据《碳排放权交易管理办法（试行）》。"
	const multilineAnswer = "跨行的答案\n第二行\n第三行"

	chunks := qaChunksFromXLSX(t, xlsxWorkbook(t, [][]string{
		{"question", "answer"},
		{multilineQ, multilineA},
		{"跨行的问句\n第二行", multilineAnswer},
	}))
	t.Logf("QA chunks=%d", len(chunks))
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	texts := strings.Join(chunkTexts(chunks), "\n")
	for _, want := range []string{multilineQ, multilineA, multilineAnswer} {
		if !strings.Contains(texts, want) {
			t.Errorf("chunk text lost the newline-bearing cell %q", want)
		}
	}
}

// Extraction must follow the table's structure rather than a tag pattern.
// The markup is rendered by several parsers and some of it comes from
// user-supplied documents, so it is not guaranteed to be well formed —
// a tag-level scan mishandles most cases below.
func TestExtractQATableFollowsStructure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		html   string
		strict bool
		want   [][2]string
	}{
		{
			name: "newline inside a cell",
			html: "<table><tr><th>问题\n跨行</th><th>答案</th></tr></table>",
			want: [][2]string{{"问题\n跨行", "答案"}},
		},
		{
			name: "colspan",
			html: `<table><tr><td colspan="2">q</td><td>a</td></tr></table>`,
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "missing </td>",
			html: "<table><tr><td>q<td>a</tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "missing </tr>",
			html: "<table><tr><td>q</td><td>a</td><tr><td>q2</td><td>a2</td></tr></table>",
			want: [][2]string{{"q", "a"}, {"q2", "a2"}},
		},
		{
			name: "attribute containing '>'",
			html: `<table><tr><td data-x="a>b">q</td><td>a</td></tr></table>`,
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "<br> inside a cell",
			html: "<table><tr><td>q<br>L2</td><td>a</td></tr></table>",
			want: [][2]string{{"q\nL2", "a"}},
		},
		{
			// The nested table's text is folded into the cell holding it, and
			// its own rows are not reported as rows of the enclosing table.
			name: "nested table",
			html: "<table><tr><td><table><tr><td>in</td><td>x</td></tr></table></td><td>a</td></tr></table>",
			want: [][2]string{{"inx", "a"}},
		},
		{
			// A bare row fragment: the HTML5 "in body" mode would discard the
			// <tr>, so it has to be parsed in a table context.
			name: "row without a table wrapper",
			html: "<tr><td>q</td><td>a</td></tr>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "rows without a table wrapper",
			html: "<tr><td>q</td><td>a</td></tr><tr><td>q2</td><td>a2</td></tr>",
			want: [][2]string{{"q", "a"}, {"q2", "a2"}},
		},
		{
			name: "rows wrapped in tbody only",
			html: "<tbody><tr><td>q</td><td>a</td></tr></tbody>",
			want: [][2]string{{"q", "a"}},
		},
		{
			// Deliberately narrow: cells without a row are not a pair, matching
			// the previous behaviour instead of inventing a row for them.
			name: "cells without a row yield nothing",
			html: "<td>q</td><td>a</td>",
			want: nil,
		},
		{
			name: "empty cell is skipped when picking the pair",
			html: "<table><tr><td>q</td><td></td><td>a</td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "unbalanced row yields nothing",
			html: "<table><tr><td>q only</td></tr></table>",
			want: nil,
		},
		{
			// The CSV contract (Python qa.py:365) is exactly two fields.
			name:   "three columns are rejected in strict mode",
			html:   "<table><tr><td>q</td><td>a</td><td>extra</td></tr></table>",
			strict: true,
			want:   nil,
		},
		{
			name:   "three columns keep the first two in lax mode",
			html:   "<table><tr><td>q</td><td>a</td><td>extra</td></tr></table>",
			strict: false,
			want:   [][2]string{{"q", "a"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pairs := extractQATable(tc.html, tc.strict)
			got := make([][2]string, 0, len(pairs))
			for _, p := range pairs {
				got = append(got, [2]string{p.Question, p.Answer})
			}
			if len(got) != len(tc.want) {
				t.Fatalf("pairs = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("pair %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
