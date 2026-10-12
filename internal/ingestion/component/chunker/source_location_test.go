package chunker

import (
	"reflect"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/position"
)

func TestSpreadsheetChunkersKeepSourceLocationThroughIndexing(t *testing.T) {
	for _, tc := range []struct {
		name      string
		factory   func(map[string]any) (runtime.Component, error)
		positions [][][]int
		rows      []int
	}{
		{"QA", NewQAChunker, [][][]int{{{1, 1, 1, 1, 2}}, {{1, 2, 2, 1, 2}}, {{1, 3, 3, 1, 2}}}, []int{0, 1, 2}},
		{"Token", NewTokenChunker, [][][]int{{{1, 1, 1, 1, 2}, {1, 2, 2, 1, 2}, {1, 3, 3, 1, 2}}}, nil},
		{"One", NewOneChunker, [][][]int{{{1, 1, 1, 1, 2}, {1, 2, 2, 1, 2}, {1, 3, 3, 1, 2}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parser.NewCSVParser().ParseWithResult(t.Context(), "qa.csv", []byte("question,answer\nWhat is Go?,A language\nWhat is RAG?,Retrieval augmented generation\n"))
			if parsed.Err != nil {
				t.Fatal(parsed.Err)
			}
			comp, err := tc.factory(nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := comp.Invoke(t.Context(), nil, map[string]any{"name": "qa.csv", "output_format": parsed.OutputFormat, "json": parsed.JSON})
			if err != nil {
				t.Fatal(err)
			}
			chunks := indexdoc.NormalizeChunks(out)
			if len(chunks) != len(tc.positions) {
				t.Fatalf("chunks = %d, want %d", len(chunks), len(tc.positions))
			}
			for _, chunk := range chunks {
				l, err := position.FromMap(chunk)
				if err != nil || !l.IsSpreadsheet() || l.Spreadsheet.Sheet != "Data" || l.Spreadsheet.Index == nil || *l.Spreadsheet.Index != 1 {
					t.Fatalf("source identity lost: %+v err=%v", l, err)
				}
				for _, key := range []string{"positions", "_pdf_positions", "sheet", "sheet_index"} {
					if _, ok := chunk[key]; ok {
						t.Errorf("runtime emitted independent field %s", key)
					}
				}
			}
			if _, err := indexdoc.ProcessChunksForPipeline(chunks, "doc-1", "qa.csv", time.Now()); err != nil {
				t.Fatal(err)
			}
			for i, ck := range chunks {
				if !reflect.DeepEqual(ck["position_int"], tc.positions[i]) {
					t.Errorf("chunk %d: position_int = %v, want %v", i, ck["position_int"], tc.positions[i])
				}
				if _, exists := ck["page_num_int"]; exists {
					t.Errorf("chunk %d: spreadsheet acquired PDF page numbers %v", i, ck["page_num_int"])
				}
				if tc.rows != nil {
					if !reflect.DeepEqual(ck["top_int"], []any{float64(tc.rows[i])}) {
						t.Errorf("chunk %d: row = %v, want %d", i, ck["top_int"], tc.rows[i])
					}
				} else if _, exists := ck["top_int"]; exists {
					t.Errorf("chunk %d: column interpreted as PDF top %v", i, ck["top_int"])
				}
			}
		})
	}
}

func TestXLSXChunkersKeepActualRowsAndSheetBoundaries(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	if _, err := book.NewSheet("Orders"); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []struct{ sheet, axis, value string }{
		{"Sheet1", "A1", "question"}, {"Sheet1", "B1", "answer"}, {"Sheet1", "A42", "first question"}, {"Sheet1", "B42", "first answer"},
		{"Orders", "A1", "question"}, {"Orders", "B1", "answer"}, {"Orders", "A7", "second question"}, {"Orders", "B7", "second answer"},
	} {
		if err := book.SetCellValue(cell.sheet, cell.axis, cell.value); err != nil {
			t.Fatal(err)
		}
	}
	binary, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	xlsx, err := parser.NewXLSXParser("")
	if err != nil {
		t.Fatal(err)
	}
	parsed := xlsx.ParseWithResult(t.Context(), "book.xlsx", binary.Bytes())
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	for _, tc := range []struct {
		name    string
		factory func(map[string]any) (runtime.Component, error)
		want    [][][]int
		rows    []int
	}{
		{"QA", NewQAChunker, [][][]int{{{1, 1, 1, 1, 2}}, {{1, 42, 42, 1, 2}}, {{2, 1, 1, 1, 2}}, {{2, 7, 7, 1, 2}}}, []int{0, 41, 0, 6}},
		{"Token", NewTokenChunker, [][][]int{{{1, 1, 1, 1, 2}, {1, 42, 42, 1, 2}}, {{2, 1, 1, 1, 2}, {2, 7, 7, 1, 2}}}, nil},
		{"General", NewGeneralChunker, [][][]int{{{1, 1, 1, 1, 2}, {1, 42, 42, 1, 2}}, {{2, 1, 1, 1, 2}, {2, 7, 7, 1, 2}}}, nil},
		{"Table", NewTableChunker, [][][]int{{{1, 42, 42, 1, 2}}, {{2, 7, 7, 1, 2}}}, nil},
		{"One", NewOneChunker, [][][]int{nil}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := tc.factory(nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := comp.Invoke(t.Context(), nil, map[string]any{"name": "book.xlsx", "file_type": "xlsx", "output_format": parsed.OutputFormat, "json": parsed.JSON})
			if err != nil {
				t.Fatal(err)
			}
			chunks := indexdoc.NormalizeChunks(out)
			if len(chunks) != len(tc.want) {
				t.Fatalf("chunks=%d, want %d", len(chunks), len(tc.want))
			}
			if _, err := indexdoc.ProcessChunksForPipeline(chunks, "doc", "book.xlsx", time.Now()); err != nil {
				t.Fatal(err)
			}
			for i, chunk := range chunks {
				if tc.want[i] == nil {
					if _, ok := chunk["position_int"]; ok {
						t.Errorf("multi-sheet One chunk acquired a partial selection: %v", chunk["position_int"])
					}
				} else if !reflect.DeepEqual(chunk["position_int"], tc.want[i]) {
					t.Errorf("chunk %d selection=%v, want %v", i, chunk["position_int"], tc.want[i])
				}
				if _, ok := chunk["page_num_int"]; ok {
					t.Errorf("spreadsheet acquired page numbers: %v", chunk["page_num_int"])
				}
				if tc.rows != nil && !reflect.DeepEqual(chunk["top_int"], []any{float64(tc.rows[i])}) {
					t.Errorf("chunk %d row=%v, want %d", i, chunk["top_int"], tc.rows[i])
				}
			}
		})
	}
}
