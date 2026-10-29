package chunker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
)

// xlsxWorkbook renders rows into an in-memory workbook so the tests run
// against the real spreadsheet parser instead of hand-written markup.
func xlsxWorkbook(t *testing.T, rows [][]string) []byte {
	t.Helper()
	return xlsxWorkbookWithMerges(t, rows, nil)
}

// xlsxWorkbookWithMerges is xlsxWorkbook plus merge rectangles expressed as
// top-left/bottom-right axis pairs (e.g. {"B1", "B2"}).
func xlsxWorkbookWithMerges(t *testing.T, rows [][]string, merges [][2]string) []byte {
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
	for _, m := range merges {
		if err := f.MergeCell(sh, m[0], m[1]); err != nil {
			t.Fatal(err)
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

	inputs := map[string]any{"name": "qa.xlsx", "file_type": "xlsx", "output_format": "json"}
	inputs["json"] = res.JSON

	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	return chunks
}

// TestXLSXQAFirstRowIsData protects the QA spreadsheet contract: workbooks
// contain question/answer rows without a header, so the leading header row
// of the segment (the first <tr>) is still the first QA pair.
func TestXLSXQAFirstRowIsData(t *testing.T) {
	chunks := qaChunksFromXLSX(t, xlsxWorkbook(t, [][]string{
		{"q1", "a1"},
		{"q2", "a2"},
	}))
	if len(chunks) != 2 {
		t.Fatalf("expected both QA rows, got %d chunks: %#v", len(chunks), chunks)
	}
	if got := chunkTexts(chunks); !strings.Contains(got[0], "q1") || !strings.Contains(got[0], "a1") {
		t.Fatalf("first row was not emitted as QA data: %#v", got)
	}
}

// A spreadsheet cell keeps the newline its author typed (Alt+Enter), so a QA
// pair whose question or answer spans lines must survive cells-first
// extraction intact. These rows used to disappear from the chunk list without
// a trace.
func TestXLSXQAMultilineCells(t *testing.T) {
	const multilineQ = "请问全国碳排放权交易市场纳入配额管理的重点排放单\n位名录，是否会公布？"
	const multilineA = "需要公布。根据《碳排放权交易管理办法（试行）》。"
	const multilineAnswer = "跨行的答案\n第二行\n第三行"

	chunks := qaChunksFromXLSX(t, xlsxWorkbook(t, [][]string{
		{multilineQ, multilineA},
		{"跨行的问句\n第二行", multilineAnswer},
	}))
	t.Logf("QA chunks=%d", len(chunks))
	if len(chunks) != 2 {
		t.Fatalf("expected 2 data-row chunks, got %d", len(chunks))
	}
	texts := strings.Join(chunkTexts(chunks), "\n")
	for _, want := range []string{multilineQ, multilineA, multilineAnswer} {
		if !strings.Contains(texts, want) {
			t.Errorf("chunk text lost the newline-bearing cell %q", want)
		}
	}
}

// TestXLSXQAMergedAnswerAtFirstRow pins the fix for a vertical merge whose
// master sits on the first row. A headerless QA sheet treats that row as data,
// so both questions must receive the merged answer; the parser must not drop
// the second row because it once mistook the master for a column label.
func TestXLSXQAMergedAnswerAtFirstRow(t *testing.T) {
	chunks := qaChunksFromXLSX(t, xlsxWorkbookWithMerges(t, [][]string{
		{"q1", "a1"},
		{"q2", ""},
	}, [][2]string{{"B1", "B2"}}))
	if len(chunks) != 2 {
		t.Fatalf("expected two QA chunks, got %d: %#v", len(chunks), chunkTexts(chunks))
	}
	texts := chunkTexts(chunks)
	if !strings.Contains(texts[0], "q1") || !strings.Contains(texts[0], "a1") {
		t.Fatalf("first pair = %q", texts[0])
	}
	if !strings.Contains(texts[1], "q2") || !strings.Contains(texts[1], "a1") {
		t.Fatalf("merged answer did not reach the second pair: %q", texts[1])
	}
	if got, _ := chunks[1]["top_int"].([]any); len(got) != 1 || got[0] != float64(1) {
		t.Fatalf("second chunk top_int = %v, want [1]", got)
	}
}

// TestXLSXQAMergedAnswerBelowFirstRow keeps the existing behavior for a merge
// whose master is already a data row: every covered row must resolve to the
// master answer.
func TestXLSXQAMergedAnswerBelowFirstRow(t *testing.T) {
	chunks := qaChunksFromXLSX(t, xlsxWorkbookWithMerges(t, [][]string{
		{"q1", "a1"},
		{"q2", "a2"},
		{"q3", ""},
		{"q4", ""},
	}, [][2]string{{"B2", "B4"}}))
	if len(chunks) != 4 {
		t.Fatalf("expected four QA chunks, got %d: %#v", len(chunks), chunkTexts(chunks))
	}
	for i, text := range chunkTexts(chunks)[1:] {
		if !strings.Contains(text, "a2") {
			t.Errorf("chunk %d did not inherit the data merge: %q", i+2, text)
		}
	}
}
