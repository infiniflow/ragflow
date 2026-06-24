//go:build cgo && manual

package parser

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"ragflow/internal/pdfoxide"
)

// TestTSRBatchGo runs pdf_oxide render + DeepDoc TSR, per-PDF output.
// Set BATCH_COUNT env to limit first N PDFs (default: all).
func TestTSRBatchGo(t *testing.T) {
	url := os.Getenv("DEEPDOC_URL"); if url == "" { t.Skip("DEEPDOC_URL not set") }; client, err := NewDeepDocClient(url); if err != nil { t.Fatal(err) }
	if !client.Health() {
		t.Skip("DeepDoc not available")
	}
	dpi := 216.0

	bboxPath := filepath.Join("testdata", "tsr", "tsr_bboxes_py.json")
	bboxData, err := os.ReadFile(bboxPath)
	if err != nil {
		t.Skipf("bbox file not found: %v", err)
	}
	var bboxBatch struct {
		Results []struct {
			PDF     string `json:"pdf"`
			Results []struct {
				Page     int       `json:"page"`
				BBoxPts  []float64 `json:"bbox_pts"`
				TableIdx int       `json:"table_idx"`
			} `json:"results"`
		} `json:"results"`
	}
	if err := json.Unmarshal(bboxData, &bboxBatch); err != nil {
		t.Fatal(err)
	}

	pdfDir := filepath.Join("testdata", "real_pdfs")
	outDir := filepath.Join("testdata", "output", "go", "ocr", "tsr")
	os.MkdirAll(outDir, 0755)

	// Sort PDFs by name
	sort.Slice(bboxBatch.Results, func(i, j int) bool {
		return bboxBatch.Results[i].PDF < bboxBatch.Results[j].PDF
	})

	// BATCH_COUNT env limits N PDFs (default: all)
	count := len(bboxBatch.Results)
	if n := os.Getenv("BATCH_COUNT"); n != "" {
		c := 0
		for _, ch := range n {
			c = c*10 + int(ch-'0')
		}
		if c > 0 && c < count {
			count = c
		}
	}

	type goResult struct {
		Page     int       `json:"page"`
		TableIdx int       `json:"table_idx"`
		BBoxPts  []float64 `json:"bbox_pts"`
		Cells    int       `json:"cells"`
		TSRMs    int64     `json:"tsr_ms"`
		Error    string    `json:"error,omitempty"`
	}

	totalTables := 0
	for i := 0; i < count; i++ {
		pdfResult := bboxBatch.Results[i]
		name := pdfResult.PDF

		// Skip if already processed
		outPath := filepath.Join(outDir, name+".json")
		if _, err := os.Stat(outPath); err == nil {
			prev, _ := os.ReadFile(outPath)
			var prevR struct {
				Tables int `json:"tables"`
			}
			json.Unmarshal(prev, &prevR)
			totalTables += prevR.Tables
			t.Logf("[%d/%d] %s — SKIP (already processed, %d tables)", i+1, count, name, prevR.Tables)
			continue
		}

		if len(pdfResult.Results) == 0 {
			continue
		}

		data, err := os.ReadFile(filepath.Join(pdfDir, name))
		if err != nil {
			t.Logf("[%d/%d] %s — read error: %v", i+1, count, name, err)
			continue
		}

		renderedPages := make(map[int]image.Image)
		var results []goResult
		t0 := time.Now()

		for _, tbl := range pdfResult.Results {
			pageImg, ok := renderedPages[tbl.Page]
			if !ok {
				result, err := pdfoxide.RenderPage(data, tbl.Page, dpi)
				if err != nil {
					continue
				}
				img := image.NewRGBA(image.Rect(0, 0, result.Width, result.Height))
				copy(img.Pix, result.Data)
				renderedPages[tbl.Page] = img
				pageImg = img
			}
			gr := goResult{Page: tbl.Page, TableIdx: tbl.TableIdx, BBoxPts: tbl.BBoxPts}
			bbox := tbl.BBoxPts
			x0 := int(bbox[0] * dpi / 72)
			y0 := int(bbox[1] * dpi / 72)
			x1 := int(bbox[2] * dpi / 72)
			y1 := int(bbox[3] * dpi / 72)
			cropped := cropImage(pageImg, x0, y0, x1, y1)
			tsrStart := time.Now()
			cells, err := client.TSR(cropped)
			gr.TSRMs = time.Since(tsrStart).Milliseconds()
			if err != nil {
				gr.Error = err.Error()
			} else {
				gr.Cells = len(cells)
			}
			results = append(results, gr)
		}

		out := struct {
			PDF     string      `json:"pdf"`
			Tables  int         `json:"tables"`
			Results []goResult  `json:"results"`
			TimeS   float64     `json:"time_s"`
		}{name, len(results), results, time.Since(t0).Seconds()}

		b, _ := json.MarshalIndent(out, "", "  ")
		os.WriteFile(outPath, b, 0644)

		totalTables += len(results)
		t.Logf("[%d/%d] %s — %d tables (%.1fs)", i+1, count, name, len(results), out.TimeS)
	}

	t.Logf("Done. %d tables. Output: %s/", totalTables, outDir)
}

func cropImage(img image.Image, x0, y0, x1, y1 int) image.Image {
	bounds := img.Bounds()
	if x0 < bounds.Min.X { x0 = bounds.Min.X }
	if y0 < bounds.Min.Y { y0 = bounds.Min.Y }
	if x1 > bounds.Max.X { x1 = bounds.Max.X }
	if y1 > bounds.Max.Y { y1 = bounds.Max.Y }
	if x0 >= x1 || y0 >= y1 { return img }
	cropped := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			cropped.Set(x-x0, y-y0, img.At(x, y))
		}
	}
	return cropped
}


// TestTSRCompare renders a known page and crops table regions for TSR comparison.
func TestTSRCompare(t *testing.T) {
	url := os.Getenv("DEEPDOC_URL"); if url == "" { t.Skip("DEEPDOC_URL not set") }; client, err := NewDeepDocClient(url); if err != nil { t.Fatal(err) }
	if !client.Health() {
		t.Skip("DeepDoc not available")
	}

	const (
		pdfName = "RAGFlow 产品白皮书(1).pdf"
		pageNum = 10
		dpi     = 216.0
	)

	tables := []struct {
		idx  int
		bbox [4]float64
	}{
		{0, [4]float64{83.9, 95.9, 497.9, 540.4}},
		{1, [4]float64{83.9, 612.4, 497.9, 764.3}},
	}

	pdfPath := filepath.Join("testdata", "real_pdfs", pdfName)
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := pdfoxide.RenderPage(data, pageNum, dpi)
	if err != nil {
		t.Fatal(err)
	}
	pageImg := image.NewRGBA(image.Rect(0, 0, result.Width, result.Height))
	copy(pageImg.Pix, result.Data)
	t.Logf("Page image: %dx%d", result.Width, result.Height)

	scale := dpi / 72.0

	for _, tbl := range tables {
		x0 := int(tbl.bbox[0] * scale)
		y0 := int(tbl.bbox[1] * scale)
		x1 := int(tbl.bbox[2] * scale)
		y1 := int(tbl.bbox[3] * scale)

		cropped := cropImage(pageImg, x0, y0, x1, y1)

		cells, err := client.TSR(cropped)
		if err != nil {
			t.Errorf("Table %d: TSR error: %v", tbl.idx, err)
			continue
		}
		t.Logf("Table %d: %d cells", tbl.idx, len(cells))

		outPath := filepath.Join("testdata", "output", "go", "ocr", "tsr_compare",
			fmt.Sprintf("RAGFlow产品白皮书(1)_p10_t%d_go.png", tbl.idx))
		os.MkdirAll(filepath.Dir(outPath), 0755)
		data, _ := encodePNG(cropped)
		os.WriteFile(outPath, data, 0644)
	}
}
