package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"slices"
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

// genuineBIFF8FixtureBase64 was generated with xlwt 1.3.0. It contains two
// worksheets and is an OLE compound document with a BIFF8 Workbook stream,
// rather than OOXML bytes renamed with an .xls extension.
const genuineBIFF8FixtureBase64 = `
0M8R4KGxGuEAAAAAAAAAAAAAAAAAAAAAPgADAP7/CQAGAAAAAAAAAAAAAAABAAAACQAAAAAAAAAAEAAA/v///wAAAAD+////AAAAAAgAAAD/////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////8JCBAAAAYFALsNzAcAAAAABgAAAOEAAgCwBMEA
AgAAAOIAAABcAHAATm9uZSAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg
ICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIEIAAgCwBGEBAgAAAD0BBAABAAIAnAACAA4AGQACAAAAEgACAAAAYwACAAAAEwACAAAArwECAAAA
vAECAAAAQAACAAAAjQACAAAAPQASAOABWgDPP04qOAAAAAAAAQBYAiIAAgAAAA4AAgABALcBAgAAANoAAgAAADEAFQDIAAAA/3+QAQAAAAABAAUAQXJpYWwx
ABUAyAAAAP9/kAEAAAAAAQAFAEFyaWFsMQAVAMgAAAD/f5ABAAAAAAEABQBBcmlhbDEAFQDIAAAA/3+QAQAAAAABAAUAQXJpYWwxABUAyAAAAP9/kAEAAAAA
AQAFAEFyaWFsMQAVAMgAAAD/f5ABAAAAAAEABQBBcmlhbDEAFQDIAAAA/3+QAQAAAAABAAUAQXJpYWweBAwApAAHAABHZW5lcmFs4AAUAAYApAD1/yAAAPQA
AAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1
/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAU
AAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAA
AMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQAAAAAAAAAAMAg4AAUAAYApAD1/yAAAPQA
AAAAAAAAAMAg4AAUAAYApAABACAAAPgAAAAAAAAAAMAg4AAUAAcApAABACAAAPgAAAAAAAAAAMAgkwIEAACAAP9gAQIAAQCFABEABAQAAAAACQBJbnZlbnRv
cnmFAA0AyAUAAAAABQBOb3Rlc/wASQAIAAAACAAAAAQAAE5hbWUFAABDb3VudAYAAEFwcGxlcwUAAFBlYXJzBQAAVG9waWMFAABWYWx1ZQYAAExlZ2FjeQUA
AEJJRkY4CgAAAAkIEAAABhAAuw3MBwAAAAAGAAAADQACAAEADAACAGQADwACAAEAEQACAAAAEAAIAPyp8dJNYlA/XwACAAAAgAAIAAAAAAABAAAAJQIEAAAA
/wCBAAIAAQwAAg4AAAAAAAMAAAAAAAIAAAAqAAIAAAArAAIAAACCAAIAAQAbAAIAAAAaAAIAAAAUAAUAAgAAJlAVAAUAAgAAJkaDAAIAAQCEAAIAAAAmAAgA
MzMzMzMz0z8nAAgAMzMzMzMz0z8oAAgAhetRuB6F4z8pAAgArkfhehSu1z+hACIACQBkAAEAAQABAIMALAEsAZqZmZmZmbk/mpmZmZmZuT8BABIAAgAAAN0A
AgAAABkAAgAAAGMAAgAAABMAAgAAAAgCEAAAAAAAAgD/AAAAAAAAAQ8A/QAKAAAAAAARAAAAAAD9AAoAAAABABEAAQAAAAgCEAABAAAAAgD/AAAAAAAAAQ8A
/QAKAAEAAAARAAIAAAB+AgoAAQABABEADgAAAAgCEAACAAAAAgD/AAAAAAAAAQ8A/QAKAAIAAAARAAMAAAB+AgoAAgABABEAHgAAAD4CEgC2AgAAAABAAAAA
AAAAAAAAAAAKAAAACQgQAAAGEAC7DcwHAAAAAAYAAAANAAIAAQAMAAIAZAAPAAIAAQARAAIAAAAQAAgA/Knx0k1iUD9fAAIAAACAAAgAAAAAAAEAAAAlAgQA
AAD/AIEAAgABDAACDgAAAAAAAgAAAAAAAgAAACoAAgAAACsAAgAAAIIAAgABABsAAgAAABoAAgAAABQABQACAAAmUBUABQACAAAmRoMAAgABAIQAAgAAACYA
CAAzMzMzMzPTPycACAAzMzMzMzPTPygACACF61G4HoXjPykACACuR+F6FK7XP6EAIgAJAGQAAQABAAEAgwAsASwBmpmZmZmZuT+amZmZmZm5PwEAEgACAAAA
3QACAAAAGQACAAAAYwACAAAAEwACAAAACAIQAAAAAAACAP8AAAAAAAABDwD9AAoAAAAAABEABAAAAP0ACgAAAAEAEQAFAAAACAIQAAEAAAACAP8AAAAAAAAB
DwD9AAoAAQAAABEABgAAAP0ACgABAAEAEQAHAAAAPgISALYAAAAAAEAAAAAAAAAAAAAAAAoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAQAAAAIAAAADAAAABAAAAAUAAAAGAAAABwAAAP7////9/////v//////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////////////////////////9SAG8AbwB0ACAA
RQBuAHQAcgB5AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAFgAFAf//////////AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAP7///8AAAAAAAAAAFcAbwByAGsAYgBvAG8AawAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAASAAIB////////////////AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAH///////////////8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAD+////AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAf//////////
/////wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP7///8AAAAAAAAAAA==
`

func TestXLSParser_GenuineBIFF8SpreadsheetJSONOutput(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(genuineBIFF8FixtureBase64)
	if err != nil {
		t.Fatalf("decode BIFF8 fixture: %v", err)
	}
	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser failed: %v", err)
	}

	res := p.ParseWithResult(t.Context(), "inventory.xls", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if res.OutputFormat != spreadsheetOutputFormat {
		t.Fatalf("res.OutputFormat = %q, want %q", res.OutputFormat, spreadsheetOutputFormat)
	}
	if got := res.File["format"]; got != "xls" {
		t.Fatalf("res.File[format] = %v, want xls", got)
	}
	if got := res.File["sheets"]; got != 2 {
		t.Fatalf("res.File[sheets] = %v, want 2", got)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("res.JSON len = %d, want one table for each sheet", len(res.JSON))
	}

	first := res.JSON[0]
	text, _ := first["text"].(string)
	if !strings.Contains(text, "<caption>Inventory</caption>") ||
		!strings.Contains(text, "<tr><th>Name</th><th>Count</th></tr>") ||
		!strings.Contains(text, "<tr><td>Apples</td><td>3</td></tr>") {
		t.Fatalf("first table markup = %q, want BIFF8 sheet data", text)
	}
	if first["sheet_index"] != 1 {
		t.Fatalf("first sheet_index = %v, want 1", first["sheet_index"])
	}
	positions, ok := first["positions"].([][]float64)
	if !ok || len(positions) != 3 {
		t.Fatalf("first positions = %#v, want header plus two data rows", first["positions"])
	}
	if got, want := positions[2], []float64{1, 3, 3, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("last row position = %v, want %v", got, want)
	}

	second := res.JSON[1]
	secondText, _ := second["text"].(string)
	if !strings.Contains(secondText, "<caption>Notes</caption>") ||
		!strings.Contains(secondText, "<tr><td>Legacy</td><td>BIFF8</td></tr>") {
		t.Fatalf("second table markup = %q, want second BIFF8 sheet data", secondText)
	}
	if second["sheet_index"] != 2 {
		t.Fatalf("second sheet_index = %v, want 2", second["sheet_index"])
	}
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
