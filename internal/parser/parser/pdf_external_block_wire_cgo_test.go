//go:build cgo

package parser

import (
	"os"
	"testing"

	"ragflow/internal/entity/models"
)

// TestExternalBlockWire_PaddleOCR locks the OCR page-block projection: title
// promotion, Markdown bodies, caption labels, media regions with and without
// text, tables, a page whose OCR raster does not match the PDF page, a page
// with no OCR dimensions, and boxes that cannot be located.
func TestExternalBlockWire_PaddleOCR(t *testing.T) {
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	response := &models.OCRFileResponse{
		Warnings: []string{"provider warning"},
		Pages: []models.OCRPage{
			{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{
				{Label: "doc_title", Text: "Annual Report", BBox: []float64{174, 146, 1015, 240}},
				{Label: "paragraph_title", Text: "Scope", BBox: []float64{174, 260, 1015, 300}},
				{Label: "paragraph_title", Text: "### Already a heading", BBox: []float64{174, 320, 1015, 360}},
				{Label: "text", Text: "Body paragraph.", BBox: []float64{174, 380, 1015, 460}},
				{Label: "text", Text: "- first item\n- second item", BBox: []float64{174, 480, 1015, 560}},
				{Label: "figure_title", Text: "Figure 1: pipeline", BBox: []float64{174, 580, 1015, 620}},
				{Label: "figure", Text: "![pipeline](https://example.invalid/p.png)", BBox: []float64{174, 640, 1015, 900}},
				{Label: "image", Text: "Captioned region", BBox: []float64{174, 920, 1015, 1000}},
				{Text: "![unlabelled](https://example.invalid/u.png)", BBox: []float64{174, 1020, 1015, 1100}},
				{Label: "chart", Text: "![no region](https://example.invalid/c.png)"},
				{Label: "table", Text: "<table><tr><th>Item</th><th>Value</th></tr><tr><td>A</td><td>42</td></tr></table>", BBox: []float64{174, 1120, 1015, 1300}},
				{Label: "table", Text: "| K | V |\n| --- | --- |\n| a | 1 |", BBox: []float64{174, 1320, 1015, 1400}},
				{Label: "table", Text: "Item: 42", BBox: []float64{174, 1420, 1015, 1460}},
				{Label: "formula", Text: "$$E = mc^2$$", BBox: []float64{174, 1480, 1015, 1520}},
				{Label: "text", Text: "clipped to the page", BBox: []float64{-30, 1540, 2000, 1600}},
				{Label: "text", Text: "zero area", BBox: []float64{10, 10, 10, 20}},
				{Label: "text", Text: "polygon box", BBox: []float64{30, 40, 300, 40, 300, 160, 30, 160}},
				{Label: "text", Text: "no box"},
				{Label: "text", Text: "   "},
			}},
			{Index: 1, Width: 600, Height: 600, Blocks: []models.OCRBlock{
				{Label: "text", Text: "raster does not match the page", BBox: []float64{30, 40, 300, 160}},
				{Label: "figure", Text: "![lost](https://example.invalid/l.png)", BBox: []float64{30, 200, 300, 400}},
			}},
			{Index: 2, Blocks: []models.OCRBlock{
				{Label: "text", Text: "page without OCR dimensions", BBox: []float64{30, 40, 300, 160}},
			}},
		},
	}
	assertExternalBlockWire(t, "pdf_external_block.paddleocr", OCRPDFResult(t.Context(), "PaddleOCR", "sample.pdf", data, response))
}
