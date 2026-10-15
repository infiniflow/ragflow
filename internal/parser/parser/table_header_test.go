package parser

import (
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

func TestSpreadsheetAutomaticHeaders(t *testing.T) {
	tests := []struct {
		name string
		rows [][]string
		want int
	}{
		{"records", [][]string{{"Alice", "18"}, {"Bob", "20"}, {"Carol", "22"}}, 0},
		{"field word in data", [][]string{{"Name", "18"}, {"Alice", "20"}, {"Bob", "22"}}, 0},
		{"ambiguous text", [][]string{{"北京", "上海"}, {"广州", "深圳"}}, 0},
		{"numbers", [][]string{{"1", "2"}, {"3", "4"}}, 0},
		{"column labels", [][]string{{"Name", "Age"}, {"Alice", "18"}, {"Bob", "20"}}, 1},
		{"single record", [][]string{{"Alice", "18"}}, 0},
		{"summary", [][]string{{"Total", "Amount"}, {"10", "20"}, {"30", "40"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := newTestXLSX(t, func(f *excelize.File) {
				for r, row := range tt.rows {
					for c, value := range row {
						mustSetCell(t, f, "Sheet1", cellAxis(r+1, c+1), value)
					}
				}
			})
			p, _ := NewXLSXParser("")
			res := p.ParseWithResult(t.Context(), "rows.xlsx", data)
			if res.Err != nil {
				t.Fatal(res.Err)
			}
			if len(res.JSON) != 1 {
				t.Fatalf("items = %v", res.JSON)
			}
			td := res.JSON[0]["table"].(*entity.TableData)
			if td.HeaderRows != tt.want {
				t.Errorf("HeaderRows = %d, want %d", td.HeaderRows, tt.want)
			}
			if !reflect.DeepEqual(td.CellTexts(), tt.rows) {
				t.Errorf("rows changed: %v", td.CellTexts())
			}
			positions := res.JSON[0]["positions"].([][]float64)
			for i, pos := range positions {
				if pos[1] != float64(i+1) {
					t.Errorf("row %d position = %v", i, pos)
				}
			}
		})
	}
}

func TestCSVAutomaticHeaders(t *testing.T) {
	for _, tt := range []struct {
		text string
		want int
	}{
		{"Alice,18\nBob,20\n", 0},
		{"北京,上海\n广州,深圳\n", 0},
		{"Name,Age\nAlice,18\nBob,20\n", 1},
		{"Alice,18\n", 0},
	} {
		res := NewCSVParser().ParseWithResult(t.Context(), "rows.csv", []byte(tt.text))
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		td := res.JSON[0]["table"].(*entity.TableData)
		if td.HeaderRows != tt.want {
			t.Errorf("%q: HeaderRows = %d, want %d", tt.text, td.HeaderRows, tt.want)
		}
	}
}

func TestXLSXExplicitNoHeader(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		for r, row := range [][]string{{"Name", "Age"}, {"Alice", "18"}, {"Bob", "20"}} {
			for c, v := range row {
				mustSetCell(t, f, "Sheet1", cellAxis(r+1, c+1), v)
			}
		}
		show := false
		mustAddTable(t, f, "Sheet1", &excelize.Table{Range: "A1:B3", Name: "NoHeader", ShowHeaderRow: &show})
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "none.xlsx", data)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	td := res.JSON[0]["table"].(*entity.TableData)
	if td.HeaderRows != 0 || len(td.Grid) != 3 {
		t.Fatalf("explicit no-header ignored: %+v", td)
	}
}

func TestXLSXMultiLevelHeader(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Sales")
		mustMergeCell(t, f, "Sheet1", "A1", "B1")
		mustSetCell(t, f, "Sheet1", "A2", "Quantity")
		mustSetCell(t, f, "Sheet1", "B2", "Amount")
		mustSetCell(t, f, "Sheet1", "A3", 2)
		mustSetCell(t, f, "Sheet1", "B3", 30)
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "multi.xlsx", data)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	td := res.JSON[0]["table"].(*entity.TableData)
	if td.HeaderRows != 2 || td.Grid[0][0].ColSpan != 2 {
		t.Fatalf("multi-level header lost: %+v", td)
	}
}

func TestPDFHeaderPredictionDoesNotPromoteData(t *testing.T) {
	item := map[string]any{"doc_type_kwd": "table", "text": `<table><tr><th>Alice</th><th>18</th></tr><tr><td>Bob</td><td>20</td></tr></table>`}
	projectPDFTable(item)
	td := item["table"].(*entity.TableData)
	if td.HeaderRows != 0 || len(td.Grid) != 2 {
		t.Fatalf("predicted data became header: %+v", td)
	}
}

func TestSpreadsheetImageSegmentationKeepsMergedRows(t *testing.T) {
	td, err := tableutil.ParseTableHTML(`<table><tr><th>Name</th></tr><tr><td rowspan="2">Alice</td></tr><tr></tr><tr><td>Bob</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	items := buildSheetItems(spreadsheetTable{data: td, rowNumbers: []int{1, 2, 3, 4}}, "Sheet1", 1, []map[string]any{{"doc_type_kwd": "image", "row_start": 2, "col_start": 1}})
	if len(items) != 3 {
		t.Fatalf("items = %v", items)
	}
	first := items[0]["table"].(*entity.TableData)
	last := items[2]["table"].(*entity.TableData)
	if len(first.Grid) != 3 || len(last.Grid) != 2 {
		t.Fatalf("merged rows split or lost: first=%+v last=%+v", first, last)
	}
	if first.Grid[1][0].RowSpan != 2 || !first.Grid[2][0].Covered || last.Grid[1][0].Text != "Bob" {
		t.Fatalf("merge geometry changed: first=%+v last=%+v", first, last)
	}
	positions := items[0]["positions"].([][]float64)
	if len(positions) != 3 || positions[2][1] != 3 {
		t.Fatalf("covered row position missing: %v", positions)
	}
}

func TestSpreadsheetHeaderlessImageHasNoPhantomHeader(t *testing.T) {
	source := newSpreadsheetTable([][]string{{"Alice", "18"}, {"Bob", "20"}}, nil, tableutil.HeaderEvidence{})
	items := buildSheetItems(source, "Sheet1", 1, []map[string]any{{"doc_type_kwd": "image", "row_start": 1, "col_start": 0}})
	if len(items) != 2 || items[0]["doc_type_kwd"] != "image" {
		t.Fatalf("invented header segment: %v", items)
	}
	td := items[1]["table"].(*entity.TableData)
	if td.HeaderRows != 0 || len(td.Grid) != 2 {
		t.Fatalf("data row lost: %+v", td)
	}
}

func TestSpreadsheetPreamblePreservesTextAndRebasesMerges(t *testing.T) {
	grid := [][]entity.TableCell{
		{{Text: "Report title"}}, {},
		{{Text: "Name"}, {Text: "Age"}},
		{{Text: "Alice", RowSpan: 2}, {Text: "18"}},
		{{Covered: true, OriginRow: 3, OriginCol: 0}, {Text: "20"}},
	}
	source := resolveSpreadsheetTable(grid, []int{1, 2, 3, 4, 5}, tableutil.HeaderEvidence{Explicit: &tableutil.Header{Start: 2, Rows: 1}})
	items := buildSheetItems(source, "Sheet1", 1, nil)
	if len(items) != 2 || items[0]["text"] != "Report title" {
		t.Fatalf("preamble lost or reordered: %v", items)
	}
	td := items[1]["table"].(*entity.TableData)
	if td.Grid[2][0].OriginRow != 1 || grid[4][0].OriginRow != 3 {
		t.Fatalf("merge origin wrong or source mutated: %+v", td.Grid)
	}
	positions := items[1]["positions"].([][]float64)
	if len(positions) != 3 || positions[0][1] != 3 || positions[2][1] != 5 {
		t.Fatalf("source rows changed: %v", positions)
	}
}

func TestPDFGeneratedMarkdownHeaderIsPrediction(t *testing.T) {
	for _, format := range []string{"json", "markdown"} {
		res := parseMinerUMarkdownResult(t.Context(), "data.pdf", "| Alice | 18 |\n| --- | --- |\n| Bob | 20 |\n", format, 1)
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		items := res.JSON
		if format == "markdown" {
			md, _ := NewMarkdownParser(GoMarkdown)
			converted := md.ParseWithResult(t.Context(), "data.pdf", []byte(res.Markdown))
			if converted.Err != nil {
				t.Fatal(converted.Err)
			}
			items = res.NormalizeTableHeaders(converted.JSON)
		}
		td := items[0]["table"].(*entity.TableData)
		if td.HeaderRows != 0 || len(td.Grid) != 2 {
			t.Fatalf("%s conversion promoted a data row: %+v", format, td)
		}
	}
}
