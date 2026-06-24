//go:build cgo && manual
//
// TODO: switch to cgo && integration once the TSR Docker model is fixed
// (see TSR_MODEL_ISSUE.md — wrong model deployed, 2-class line detector
// instead of 6-class table structure recognizer).
//
// Usage:
//
//	go test -v -run TestTSR_DumpTableImage -tags=manual -count=1 ./internal/parser/

package parser

import (
	"encoding/json"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestTSR_DumpTableImage(t *testing.T) {
	pdfPath := filepath.Join("testdata", "real_pdfs", "icbccs deployment.pdf")
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := NewEngine(data)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	// Render page 2 (0-indexed), which has 2 table regions from DLA.
	pageNum := 2
	pageImg, err := renderPageToImage(engine, pageNum)
	if err != nil {
		t.Fatal(err)
	}

	// Save full page image for reference.
	pp, _ := os.Create("/tmp/tsr_debug_page.png")
	png.Encode(pp, pageImg)
	pp.Close()
	t.Log("Page image saved to /tmp/tsr_debug_page.png")

	// Get DLA regions.
	url := os.Getenv("DEEPDOC_URL"); if url == "" { t.Skip("DEEPDOC_URL not set") }; dd, err := NewDeepDocClient(url); if err != nil { t.Fatal(err) }
	if !dd.Health() {
		t.Fatal("DeepDoc not available")
	}
	regions, err := dd.DLA(pageImg)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("DLA returned %d regions", len(regions))

	// Find first table region.
	var tableRegion *DLARegion
	for i := range regions {
		if regions[i].Label == "table" {
			tableRegion = &regions[i]
			break
		}
	}
	if tableRegion == nil {
		t.Fatal("No table region found")
	}
	t.Logf("Table region: [%.0f,%.0f,%.0f,%.0f]",
		tableRegion.X0, tableRegion.Y0, tableRegion.X1, tableRegion.Y1)

	// Crop table image.
	cropped, err := cropImageRegion(pageImg, *tableRegion)
	if err != nil {
		t.Fatal(err)
	}

	// Save cropped image as JPEG (same as what Go sends to TSR).
	var tmpJPG = "/tmp/tsr_debug_table.jpg"
	f, _ := os.Create(tmpJPG)
	jpeg.Encode(f, cropped, &jpeg.Options{Quality: 90})
	f.Close()
	t.Logf("Table image saved to %s (%dx%d)", tmpJPG, cropped.Bounds().Dx(), cropped.Bounds().Dy())

	// Call TSR API.
	cells, err := dd.TSR(cropped)
	if err != nil {
		t.Fatal(err)
	}

	// Print label distribution.
	labelCounts := map[string]int{}
	for _, c := range cells {
		labelCounts[c.Label]++
	}
	t.Logf("TSR returned %d cells:", len(cells))
	b, _ := json.MarshalIndent(labelCounts, "", "  ")
	t.Logf("Labels: %s", string(b))

	if len(labelCounts) == 1 && labelCounts["table"] == len(cells) {
		t.Errorf("All %d cells have label 'table' — TSR model can't identify structure in Go's image", len(cells))
	} else {
		t.Log("✅ TSR model produced proper structure labels")
	}

	// Also print first 5 cell details.
	for i, c := range cells {
		if i >= 5 {
			break
		}
		w, h := c.X1-c.X0, c.Y1-c.Y0
		fmt.Printf("  [%d] label=%q bbox=[%.0f,%.0f,%.0f,%.0f] %.0fx%.0f\n",
			i, c.Label, c.X0, c.Y0, c.X1, c.Y1, w, h)
	}
}
