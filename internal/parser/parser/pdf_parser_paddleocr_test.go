package parser

import (
	"context"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity/models"
)

func TestPaddleOCRBlockPosition(t *testing.T) {
	page := models.OCRPage{Index: 2, Width: 600, Height: 800}
	for _, tc := range []struct {
		name string
		box  []float64
		want []any
	}{
		{"rectangle", []float64{30, 40, 300, 160}, []any{3, float64(15), float64(150), float64(20), float64(80)}},
		{"noncanonical polygon", []float64{30, 40, 300, 40, 300, 160, 30, 160}, nil},
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
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Blocks: []models.OCRBlock{{Text: "keep me", Label: "text", BBox: []float64{10, 20, 30, 40}}, {Text: "![figure](https://example.invalid/a(1).png)", Label: "image"}}}}}
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
	if item["doc_type_kwd"] != "text" || item["text"] != "Item: 42" {
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

func TestPaddleOCRPDFResultRejectsRenderedEmptyText(t *testing.T) {
	for _, label := range []string{"text", "doc_title", "paragraph_title"} {
		t.Run(label, func(t *testing.T) {
			response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Blocks: []models.OCRBlock{{Label: label, Text: "---"}}}}}
			result := PaddleOCRPDFResult(t.Context(), "separator.pdf", nil, response)
			if result.Err == nil || !strings.Contains(result.Err.Error(), "empty text") {
				t.Fatalf("rendered-empty document accepted: %+v", result)
			}
			response.Pages[0].Blocks = append(response.Pages[0].Blocks, models.OCRBlock{Text: "body"})
			result = PaddleOCRPDFResult(t.Context(), "mixed.pdf", nil, response)
			if result.Err != nil || len(result.JSON) != 1 || result.JSON[0].ToMap()["text"] != "body" {
				t.Fatalf("empty placeholder survived beside body: %+v", result)
			}
		})
	}
}
