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
