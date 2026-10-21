//go:build !cgo

package parser

import (
	"os"
	"strings"
	"testing"

	"ragflow/internal/entity/models"
)

func TestPaddleOCRPDFResultWithoutCGOKeepsTextAndPageIdentity(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 2, Width: 900, Height: 800, Blocks: []models.OCRBlock{{Text: "body", Label: "text", BBox: []float64{30, 40, 300, 160}}}}}}
	result := PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.JSON) != 1 || result.File["page_count"] != 3 {
		t.Fatalf("result=%+v", result)
	}
	item := result.JSON[0].ToMap()
	if item["text"] != "body" || item["page_number"] != 3 || item["positions"] != nil || item["_pdf_positions"] != nil {
		t.Fatalf("degraded item=%v", item)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "requires cgo") {
		t.Fatalf("warnings=%v", result.Warnings)
	}
}
