package parser

import (
	"context"
	"math"
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

func TestPaddleOCRBlockPosition(t *testing.T) {
	page := models.OCRPage{Index: 2, Width: 900, Height: 800}
	for _, tc := range []struct {
		name string
		box  []float64
		want []any
	}{
		{"rectangle", []float64{30, 40, 300, 160}, []any{3, float64(10), float64(100), float64(20), float64(80)}},
		{"polygon", []float64{30, 40, 300, 40, 300, 160, 30, 160}, []any{3, float64(10), float64(100), float64(20), float64(80)}},
		{"clip", []float64{-30, -40, 1200, 1000}, []any{3, float64(0), float64(300), float64(0), float64(400)}},
		{"zero", []float64{1, 1, 1, 2}, nil},
		{"outside", []float64{1000, 900, 1100, 1000}, nil},
		{"nonfinite", []float64{0, 0, math.Inf(1), 100}, nil},
		{"missing", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := paddleOCRBlockPosition(page, tc.box, [2]float64{300, 400}); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestPaddleOCRPDFResultKeepsUnlocatedText(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Blocks: []models.OCRBlock{{Text: "keep me", Label: "text", BBox: []float64{10, 20, 30, 40}}, {Text: "![figure](https://example.invalid/a.png)", Label: "image"}}}}}
	result := PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil || len(result.JSON) != 1 {
		t.Fatalf("result=%+v", result)
	}
	item := result.JSON[0].ToMap()
	if item["text"] != "keep me" || item["positions"] != nil {
		t.Fatalf("item=%v", item)
	}
}

func TestPaddleOCRPDFResultKeepsUnstructuredTableText(t *testing.T) {
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

func TestPaddleOCRMarkdownTable(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{{Label: "table", Text: "| Item | Value |\n| --- | --- |\n| A | 42 |", BBox: []float64{10, 20, 100, 200}}}}}}
	result := PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.JSON) != 1 || result.JSON[0].ToMap()["doc_type_kwd"] != "table" {
		t.Fatalf("GFM table lost structure: %v", ItemsToMaps(result.JSON))
	}
}

func TestPaddleOCRCanceledContext(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{{Label: "text", Text: "body", BBox: []float64{10, 20, 100, 200}}}}}}
	result := PaddleOCRPDFResult(ctx, "sample.pdf", data, response)
	if result.Err == nil {
		t.Fatal("canceled parse returned success")
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

func TestPaddleOCRTitleLabels(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ label, text, want, ckType string }{
		{"doc_title", "Document title", "# Document title", "heading"},
		{"paragraph_title", "Section title", "## Section title", "heading"},
		{"doc_title", "### Existing title", "### Existing title", "heading"},
		{"figure_title", "Figure caption", "Figure caption", "text"},
		{"table_title", "Table caption", "Table caption", "text"},
	} {
		t.Run(tc.label+tc.text, func(t *testing.T) {
			response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Blocks: []models.OCRBlock{{Text: tc.text, Label: tc.label}}}}}
			result := PaddleOCRPDFResult(t.Context(), "title.pdf", data, response)
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			item := result.JSON[0].ToMap()
			if item["ck_type"] != tc.ckType || item["text"] != tc.want {
				t.Fatalf("title semantics: %v", item)
			}
			formatted := pdfItemsToResult("title.pdf", ItemsToMaps(result.JSON), "markdown", 1)
			if formatted.Markdown != tc.want {
				t.Fatalf("markdown=%q; want %q", formatted.Markdown, tc.want)
			}
		})
	}
}
