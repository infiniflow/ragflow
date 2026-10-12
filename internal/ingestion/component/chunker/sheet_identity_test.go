package chunker

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/parser/parser"
)

func TestSpreadsheetChunkersPreserveSheetIdentityThroughIndexing(t *testing.T) {
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
	data, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	xlsx, err := parser.NewXLSXParser("")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct {
		name   string
		result parser.ParseResult
	}{
		{"rows.csv", parser.NewCSVParser().ParseWithResult(t.Context(), "rows.csv", []byte("question,answer\nhello,world\n"))},
		{"book.xlsx", xlsx.ParseWithResult(t.Context(), "book.xlsx", data.Bytes())},
	} {
		if source.result.Err != nil {
			t.Fatal(source.result.Err)
		}
		for _, tc := range []struct {
			name    string
			factory func(map[string]any) (runtime.Component, error)
		}{{"QA", NewQAChunker}, {"Token", NewTokenChunker}, {"One", NewOneChunker}} {
			t.Run(source.name+"/"+tc.name, func(t *testing.T) {
				batches := [][]map[string]any{source.result.JSON}
				// One preserves positions only for a single source item.
				if tc.name == "One" {
					batches = nil
					for _, item := range source.result.JSON {
						batches = append(batches, []map[string]any{item})
					}
				}
				for _, items := range batches {
					comp, err := tc.factory(nil)
					if err != nil {
						t.Fatal(err)
					}
					out, err := comp.Invoke(t.Context(), nil, map[string]any{"name": source.name, "output_format": source.result.OutputFormat, "json": items})
					if err != nil {
						t.Fatal(err)
					}
					chunks := indexdoc.NormalizeChunks(out)
					if len(chunks) == 0 {
						t.Fatal("no chunks")
					}
					var want [][]int
					for _, item := range items {
						raw, err := json.Marshal(item["positions"])
						if err != nil {
							t.Fatal(err)
						}
						var ranges [][]int
						if err = json.Unmarshal(raw, &ranges); err != nil {
							t.Fatal(err)
						}
						want = append(want, ranges...)
					}
					var got [][]int
					for _, chunk := range chunks {
						var sourceItem map[string]any
						for _, item := range items {
							if chunk["sheet_index"] == float64(item["sheet_index"].(int)) {
								sourceItem = item
								break
							}
						}
						if sourceItem == nil || chunk["sheet"] != sourceItem["sheet"] {
							t.Fatalf("sheet identity lost: chunk=%v", chunk)
						}
						beforeTop := chunk["top_int"]
						if _, err = indexdoc.ProcessChunksForPipeline([]map[string]any{chunk}, "doc", source.name, time.Now()); err != nil {
							t.Fatal(err)
						}
						if _, ok := chunk["page_num_int"]; ok {
							t.Fatalf("spreadsheet interpreted as PDF: %v", chunk)
						}
						positions, ok := chunk["position_int"].([][]int)
						if !ok {
							t.Fatalf("missing spreadsheet positions: %v", chunk)
						}
						got = append(got, positions...)
						if tc.name == "QA" {
							if !reflect.DeepEqual(chunk["top_int"], beforeTop) || !reflect.DeepEqual(beforeTop, []any{float64(positions[0][1] - 1)}) {
								t.Fatalf("QA source row overwritten: %v", chunk)
							}
						} else if _, ok := chunk["top_int"]; ok {
							t.Fatalf("spreadsheet column became PDF top: %v", chunk)
						}
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("selections=%v, want %v", got, want)
					}
				}
			})
		}
	}
}
