package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func createTestExcelBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "Header1")
	_ = f.SetCellValue(sheet, "B1", "Header2")
	_ = f.SetCellValue(sheet, "A2", "Val1")
	_ = f.SetCellValue(sheet, "B2", "Val2")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write excel file failed: %v", err)
	}
	return buf.Bytes()
}

func TestXLSParser_SpreadsheetJSONOutput(t *testing.T) {
	data := createTestExcelBytes(t)
	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser failed: %v", err)
	}

	res := p.ParseWithResult(context.Background(), "test.xls", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	// Default output format for XLSParser is "json"
	if res.OutputFormat != "json" {
		t.Errorf("res.OutputFormat = %q, want 'json'", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("res.JSON should have structured table items")
	}
	item := res.JSON[0]
	if item["ck_type"] != "table" {
		t.Errorf("table item ck_type = %v, want table", item["ck_type"])
	}
	text, _ := item["text"].(string)
	if !strings.Contains(text, "<tr><th>Header1</th><th>Header2</th></tr>") ||
		!strings.Contains(text, "<tr><td>Val1</td><td>Val2</td></tr>") {
		t.Errorf("table markup = %q, want captioned header and data row", text)
	}
	if item["doc_type_kwd"] != "table" {
		t.Errorf("doc_type_kwd = %v, want 'table'", item["doc_type_kwd"])
	}

	// Configure output format to "json"
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
	})
	resJSON := p.ParseWithResult(context.Background(), "test.xls", data)
	if resJSON.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", resJSON.Err)
	}
	if resJSON.OutputFormat != "json" {
		t.Errorf("resJSON.OutputFormat = %q, want 'json'", resJSON.OutputFormat)
	}
	if len(resJSON.JSON) == 0 {
		t.Errorf("resJSON.JSON should not be empty")
	}
}

func TestXLSXParser_JSONOutput(t *testing.T) {
	data := createTestExcelBytes(t)
	p, err := NewXLSXParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSXParser failed: %v", err)
	}

	// Default output format is "json"
	res := p.ParseWithResult(context.Background(), "test.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("res.OutputFormat = %q, want 'json'", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("res.JSON = %#v, want one segmented HTML table item", res.JSON)
	}
	if res.JSON[0]["ck_type"] != "table" {
		t.Errorf("item ck_type = %v, want table", res.JSON[0]["ck_type"])
	}

	// Even if configured to legacy "html", format is unified to "json"
	p.ConfigureFromSetup(map[string]any{
		"output_format": "html",
	})
	resHTML := p.ParseWithResult(context.Background(), "test.xlsx", data)
	if resHTML.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", resHTML.Err)
	}
	if resHTML.OutputFormat != "json" {
		t.Errorf("resHTML.OutputFormat = %q, want 'json'", resHTML.OutputFormat)
	}
	if len(resHTML.JSON) == 0 {
		t.Errorf("resHTML.JSON should have items")
	}
}

func TestXLSParser_ImagesDoNotRunOCR(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	imageBytes := imageData.Bytes()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
		Extension: ".png",
		File:      imageBytes,
		Format:    &excelize.GraphicOptions{AltText: "sheet image"},
	}); err != nil {
		t.Fatalf("AddPictureFromBytes: %v", err)
	}
	var workbook bytes.Buffer
	if err := f.Write(&workbook); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	previous := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = previous })
	deepdoctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		return &deepdocpdf.MockDocAnalyzer{Healthy: true, OCRDetectErr: errors.New("xls OCR sentinel")}, true
	})

	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	result := p.ParseWithResult(t.Context(), "with-image.xls", workbook.Bytes())
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	var imageItem map[string]any
	for _, item := range result.JSON {
		if item["doc_type_kwd"] == "image" {
			imageItem = item
			break
		}
	}
	if imageItem == nil || imageItem["image"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes) {
		t.Fatalf("image item = %+v, want retained picture payload", imageItem)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for an admitted image", result.Warnings)
	}
}

func TestXLSParser_CapsEmbeddedImageCount(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	f := excelize.NewFile()
	defer f.Close()
	for i := 0; i <= maxEmbeddedMediaItems; i++ {
		cell, err := excelize.CoordinatesToCellName(3, i+1)
		if err != nil {
			t.Fatalf("cell %d: %v", i, err)
		}
		if err := f.AddPictureFromBytes("Sheet1", cell, &excelize.Picture{Extension: ".png", File: imageData.Bytes()}); err != nil {
			t.Fatalf("AddPictureFromBytes(%s): %v", cell, err)
		}
	}
	var workbook bytes.Buffer
	if err := f.Write(&workbook); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	previous := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = previous })
	deepdoctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		return &deepdocpdf.MockDocAnalyzer{Healthy: true}, true
	})
	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	result := p.ParseWithResult(t.Context(), "many-images.xls", workbook.Bytes())
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	imageCount := 0
	for _, item := range result.JSON {
		if item["doc_type_kwd"] == "image" {
			imageCount++
		}
	}
	if imageCount != maxEmbeddedMediaItems {
		t.Fatalf("image items = %d, want %d", imageCount, maxEmbeddedMediaItems)
	}
	if warnings := strings.Join(result.Warnings, "\n"); !strings.Contains(warnings, "stopped extracting embedded images after the 256-item document limit") {
		t.Fatalf("warnings = %q, want image-count limit warning", warnings)
	}
}
