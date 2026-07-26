//go:build cgo && integration

package pdf

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	"testing"
)

// TestDLATSRResponseCompare calls DeepDoc DLA/TSR from Go and saves the
// parsed results as JSON. A companion Python script sends the same image
// and saves its results. Comparing the two JSONs verifies that both sides
// parse the DeepDoc response identically.
//
// Usage:
//  1. Run this test:  go test -v -tags=integration -run TestDLATSRResponseCompare
//  2. Run Python:     python3 tools/dla_tsr_compare.py
//  3. Diff the JSON:  diff testdata/output/render_compare/go_dla.json testdata/output/render_compare/py_dla.json
func TestDLATSRResponseCompare(t *testing.T) {
	client := mustConnectInferenceClient(t)
	eng := mustOpenEngine(t, "06_table_content.pdf")
	defer eng.Close()

	pageImg, err := RenderPageToImage(eng, 0)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	outDir := filepath.Join("testdata", "output", "render_compare")
	os.MkdirAll(outDir, 0755)

	// Save rendered image as JPEG (matching what DLA/TSR actually send).
	jpegData, err := util.EncodeJPEG(pageImg)
	if err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	imgPath := filepath.Join(outDir, "dla_input.jpeg")
	os.WriteFile(imgPath, jpegData, 0644)
	t.Logf("Input image saved: %s (%dx%d, %d bytes JPEG)", imgPath, pageImg.Bounds().Dx(), pageImg.Bounds().Dy(), len(jpegData))

	// ── DLA ──
	regions, err := client.DLA(context.Background(), pageImg)
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	dlaJSON := filepath.Join(outDir, "go_dla.json")
	writeJSON(t, dlaJSON, regions)
	t.Logf("DLA: %d regions → %s", len(regions), dlaJSON)
	for i, r := range regions {
		t.Logf("  region[%d]: label=%s conf=%.3f bbox=[%.1f, %.1f, %.1f, %.1f]",
			i, r.Label, r.Confidence, r.X0, r.Y0, r.X1, r.Y1)
	}

	// ── TSR (crop first table region) ──
	var tableRegion *pdf.DLARegion
	for i := range regions {
		if regions[i].Label == "table" {
			tableRegion = &regions[i]
			break
		}
	}
	if tableRegion == nil {
		t.Log("No table region found — skipping TSR comparison")
	} else {
		cropped := cropImageRect(pageImg,
			int(tableRegion.X0), int(tableRegion.Y0),
			int(tableRegion.X1), int(tableRegion.Y1))

		cropPath := filepath.Join(outDir, "tsr_input.jpeg")
		cropJPEG, _ := util.EncodeJPEG(cropped)
		os.WriteFile(cropPath, cropJPEG, 0644)

		cells, err := client.TSR(context.Background(), cropped)
		if err != nil {
			t.Fatalf("TSR: %v", err)
		}
		tsrJSON := filepath.Join(outDir, "go_tsr.json")
		writeJSON(t, tsrJSON, cells)
		t.Logf("TSR: %d cells → %s", len(cells), tsrJSON)
		for i, c := range cells {
			t.Logf("  cell[%d]: [%.1f, %.1f, %.1f, %.1f]", i, c.X0, c.Y0, c.X1, c.Y1)
		}
	}

	// ── OCR Detect ──
	detectBoxes, err := client.OCRDetect(context.Background(), pageImg)
	if err != nil {
		t.Fatalf("OCRDetect: %v", err)
	}
	detectJSON := filepath.Join(outDir, "go_ocr_detect.json")
	writeJSON(t, detectJSON, detectBoxes)
	t.Logf("OCR Detect: %d boxes → %s", len(detectBoxes), detectJSON)

	// ── OCR Recognize (crop a text region from the page) ──
	if len(detectBoxes) > 0 {
		// Use the first detected text box as crop region.
		b := detectBoxes[0]
		cropped := cropImageRect(pageImg,
			int(b.X0), int(b.Y0), int(b.X2), int(b.Y2))

		cropPath := filepath.Join(outDir, "ocr_rec_input.jpeg")
		recJPEG, _ := util.EncodeJPEG(cropped)
		os.WriteFile(cropPath, recJPEG, 0644)

		texts, err := client.OCRRecognize(context.Background(), cropped)
		if err != nil {
			t.Fatalf("OCRRecognize: %v", err)
		}
		recJSON := filepath.Join(outDir, "go_ocr_rec.json")
		writeJSON(t, recJSON, texts)
		t.Logf("OCR Recognize: %d texts → %s", len(texts), recJSON)
		for i, tx := range texts {
			t.Logf("  text[%d]: %q conf=%.3f", i, tx.Text, tx.Confidence)
		}
	} else {
		t.Log("OCR Detect returned 0 boxes — skipping OCR Recognize")
	}
}

func savePNGFile(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}
