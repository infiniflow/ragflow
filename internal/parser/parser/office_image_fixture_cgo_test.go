//go:build cgo

package parser

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

func TestDOCXParser_RealTableImage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "docx_table_image.docx"))
	if err != nil {
		t.Fatal(err)
	}
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})
	result := p.ParseWithResult(t.Context(), "docx_table_image.docx", data)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var tableID string
	for _, item := range result.JSON {
		if item["doc_type_kwd"] == "table" {
			tableID, _ = item["source_table_id"].(string)
		}
	}
	if tableID == "" {
		t.Fatalf("table with image not found: %+v", result.JSON)
	}
	for _, item := range result.JSON {
		if item["doc_type_kwd"] != "image" || item["parent_table_id"] != tableID {
			continue
		}
		encoded, _ := item["image"].(string)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("table image payload: %v", err)
		}
		if _, err := png.Decode(bytes.NewReader(decoded)); err != nil {
			t.Fatalf("table image is not PNG: %v", err)
		}
		if item["row_index"] != 1 || item["column_index"] != 1 || item["media_order"] != 1 {
			t.Fatalf("table image location = %+v", item)
		}
		return
	}
	t.Fatalf("no image linked to table %q: %+v", tableID, result.JSON)
}

func TestPPTXParser_RealSlideImage(t *testing.T) {
	raster := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			raster.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, raster); err != nil {
		t.Fatal(err)
	}
	w := officeOxide.NewPptxWriter()
	defer w.Close()
	slide := w.AddSlide()
	w.AddSlideText(slide, "slide with image")
	w.AddSlideImage(slide, imageData.Bytes(), "png", 914400, 914400, 914400, 457200)
	data, err := w.ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	result := NewPPTXParser().ParseWithResult(t.Context(), "slide_image.pptx", data)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for _, item := range result.JSON {
		if item["doc_type_kwd"] != "image" {
			continue
		}
		encoded, _ := item["image"].(string)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("slide image payload: %v", err)
		}
		if _, err := png.Decode(bytes.NewReader(decoded)); err != nil {
			t.Fatalf("slide image is not PNG: %v", err)
		}
		if item["slide_number"] != 1 || item["media_order"] != 1 {
			t.Fatalf("slide image location = %+v", item)
		}
		return
	}
	t.Fatalf("slide image not emitted: %+v", result.JSON)
}
