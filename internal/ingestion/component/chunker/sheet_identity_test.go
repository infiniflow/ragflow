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
			params  map[string]any
			factory func(map[string]any) (runtime.Component, error)
		}{
			{"QA", nil, NewQAChunker},
			{"Token", nil, NewTokenChunker},
			{"One", nil, NewOneChunker},
			// The title-family chunkers share extractLineRecords; a
			// spreadsheet item must keep its sheet identity there too, or its
			// selections would be decoded as PDF boxes.
			{"Group", map[string]any{"levels": [][]string{{`^# `}}}, NewGroupTitleChunker},
			{"Hierarchy", map[string]any{"hierarchy": 1, "levels": [][]string{{`^# `}}}, NewHierarchyTitleChunker},
			{"Manual", map[string]any{"levels": [][]string{{`^# `}}}, NewManualChunker},
		} {
			t.Run(source.name+"/"+tc.name, func(t *testing.T) {
				batches := [][]map[string]any{parser.ItemsToMaps(source.result.JSON)}
				// One preserves positions only for a single source item.
				if tc.name == "One" {
					batches = nil
					for _, item := range source.result.JSON {
						batches = append(batches, []map[string]any{item.ToMap()})
					}
				}
				for _, items := range batches {
					comp, err := tc.factory(tc.params)
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

func TestQAChunkerSheetNameOnlyPreservesSourceRows(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	for cell, value := range map[string]string{"A1": "question", "B1": "answer", "A42": "source question", "B42": "source answer"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
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
	parsed := xlsx.ParseWithResult(t.Context(), "rows.xlsx", data.Bytes())
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	if len(parsed.JSON) != 1 {
		t.Fatalf("parser items=%d, want 1", len(parsed.JSON))
	}
	// Keep real parser selections, but exercise the supported name-only identity.
	m := parsed.JSON[0].ToMap()
	delete(m, "sheet_index")
	parsed.JSON[0] = parser.RawItem{M: m}
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{"name": "rows.xlsx", "output_format": parsed.OutputFormat, "json": parsed.JSON})
	if err != nil {
		t.Fatal(err)
	}
	chunks := indexdoc.NormalizeChunks(out)
	if len(chunks) != 2 {
		t.Fatalf("chunks=%d, want 2", len(chunks))
	}
	for i, row := range []int{1, 42} {
		chunk := chunks[i]
		if chunk["sheet"] != "Sheet1" {
			t.Fatalf("sheet=%v, want Sheet1", chunk["sheet"])
		}
		if _, ok := chunk["sheet_index"]; ok {
			t.Fatal("name-only identity acquired sheet_index")
		}
		wantTop := []any{float64(row - 1)}
		if !reflect.DeepEqual(chunk["top_int"], wantTop) {
			t.Errorf("chunk %d row=%v, want %v", i, chunk["top_int"], wantTop)
		}
		if want := [][]float64{{1, float64(row), float64(row), 1, 2}}; !reflect.DeepEqual(chunk["positions"], want) {
			t.Errorf("chunk %d selection=%v, want %v", i, chunk["positions"], want)
		}
		if _, err = indexdoc.ProcessChunksForPipeline([]map[string]any{chunk}, "doc", "rows.xlsx", time.Now()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(chunk["top_int"], wantTop) || !reflect.DeepEqual(chunk["position_int"], [][]int{{1, row, row, 1, 2}}) {
			t.Errorf("chunk %d indexed row/selection=%v", i, chunk)
		}
		if _, ok := chunk["page_num_int"]; ok {
			t.Fatal("sheet interpreted as PDF")
		}
	}
}
