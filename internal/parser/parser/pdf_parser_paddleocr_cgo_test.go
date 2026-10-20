//go:build cgo

package parser

import (
	"os"
	"reflect"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

func TestPaddleOCRPDFResultPageGeometry(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	text := "body"
	response := &models.OCRFileResponse{Text: &text, Pages: []models.OCRPage{
		{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{{Text: "first", Label: "text", BBox: []float64{174, 146, 1015, 240}}}},
		{Index: 1, Width: 900, Height: 1200},
		{Index: 2, Width: 900, Height: 800, Blocks: []models.OCRBlock{{Text: "third", Label: "text", BBox: []float64{30, 40, 300, 160}}, {Text: "<table><tr><td>42</td></tr></table>", Label: "table", BBox: []float64{30, 200, 300, 400}}}},
	}}
	result := PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.File["page_count"] != 3 || len(result.JSON) != 3 {
		t.Fatalf("result=%+v", result)
	}
	third := result.JSON[1].ToMap()
	if !reflect.DeepEqual(third["positions"], [][]any{{3, float64(10), float64(100), float64(20), float64(80)}}) {
		t.Fatalf("third positions=%v", third["positions"])
	}
	table := result.JSON[2].ToMap()
	// TableData must carry the same source page as the highlight boxes.
	if table["table"].(*entity.TableData).Page != 3 {
		t.Fatalf("table data page=%v", table["table"])
	}
	if table["page_number"] != 3 {
		t.Fatalf("table page=%v", table)
	}
}

func TestPaddleOCRPDFResultLocatesUnstructuredTableText(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{{Text: "Item: 42", Label: "table", BBox: []float64{20, 40, 100, 120}}}}}}
	result := PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil || len(result.JSON) != 1 {
		t.Fatalf("result=%+v", result)
	}
	item := result.JSON[0].ToMap()
	if item["doc_type_kwd"] != "text" || item["text"] != "Item: 42" || item["positions"] == nil {
		t.Fatalf("item=%v", item)
	}
}

func TestPaddleOCRPDFResultKeepsGeometryAfterOnePageFails(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	block := models.OCRBlock{Label: "text", Text: "body", BBox: []float64{10, 20, 100, 200}}
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{block}}, {Index: 99, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{block}}}}
	result := PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.File["page_count"] != 3 {
		t.Fatalf("source page count=%v", result.File["page_count"])
	}
	if len(result.Warnings) == 0 || result.JSON[0].ToMap()["positions"] == nil || result.JSON[1].ToMap()["positions"] != nil {
		t.Fatalf("result=%+v", result)
	}
}
