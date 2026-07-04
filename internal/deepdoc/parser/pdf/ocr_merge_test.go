//go:build cgo && manual

package pdf

import (
	"context"
	"image/png"
	"os"
	inf "ragflow/internal/deepdoc/parser/pdf/inference"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	"strings"
	"testing"
)

// TestOCR_mergeChars_RealScanned tests ocrMergeChars on a real scanned
// medical PDF where pdf_oxide extracts noise (RASB@PS, random symbols)
// instead of real text. This validates that detect+merge+recognize
// produces readable English from the scan.
func TestOCR_mergeChars_RealScanned(t *testing.T) {
	url := os.Getenv("DEEPDOC_URL")
	if url == "" {
		t.Skip("DEEPDOC_URL not set")
	}
	dd, err := inf.NewClient(url)
	if err != nil {
		t.Fatal(err)
	}
	if !dd.Health() {
		t.Fatal("DeepDoc not available")
	}

	pdfPath := "testdata/real_pdfs/1例3个月喉噗合并先天性心脏病患儿气管插管的麻醉护理.pdf"
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatal(err)
	}

	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pdf_oxide Chars: %d", len(chars))

	var sample strings.Builder
	for i, c := range chars {
		if i >= 200 {
			break
		}
		sample.WriteString(c.Text)
	}
	t.Logf("pdf_oxide sample: %q", sample.String())
	t.Logf("isScanNoise: %v", util.IsScanNoise(sample.String()))
	t.Logf("isGarbledPage: %v", util.IsGarbledPage(chars))

	img, err := eng.RenderPageImage(0, 72*3)
	if err != nil {
		t.Fatal(err)
	}

	boxes := ocrMergeChars(context.Background(), img, chars, dd, 0)
	t.Logf("ocrMergeChars boxes: %d", len(boxes))
	for i, b := range boxes {
		// Save go render for comparison
		f, _ := os.Create("/tmp/_go_render.png")
		png.Encode(f, img)
		f.Close()
		t.Logf("Go render saved: %v -> /tmp/_go_render.png", img.Bounds())
		end := min(120, len(b.Text))
		t.Logf("  [%d] (%.0f,%.0f)-(%.0f,%.0f) text=%q",
			i, b.X0, b.Top, b.X1, b.Bottom, b.Text[:end])
	}

	scanBoxes := ocrDetectAndRecognize(context.Background(), img, dd, 0, "scan page")
	t.Logf("ocrScanPage boxes (no chars): %d", len(scanBoxes))
	for i, b := range scanBoxes {
		end := min(120, len(b.Text))
		t.Logf("  [%d] (%.0f,%.0f)-(%.0f,%.0f) text=%q",
			i, b.X0, b.Top, b.X1, b.Bottom, b.Text[:end])
	}
}
