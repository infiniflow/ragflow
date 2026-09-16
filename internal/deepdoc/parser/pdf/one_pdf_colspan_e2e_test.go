//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Test1PdfColspanEndToEnd parses real_pdfs/1.pdf through the PRODUCTION path
// (real DLA+TSR+OCR against DEEPDOC_URL) and asserts the table HTML carries
// colspan="6" on the merged header — matching Python's golden
// (output/py/ocr_real/tables/1.pdf.json: row0 collapses to 3 text cells
// because the header spans 6 of the 8 columns).
//
// This is the end-to-end closure for the Go assembly bug fixed in
// AnnotateTableBoxes: Go was copying the spanning cell's bbox
// (H_left/H_right/H_top/H_bott) onto the overlapping box ONLY for header
// cells, not for "spanning cell" (SP) boxes. Without that propagation,
// GroupBoxesByRC rebuilt the span cell from the box's own narrow bounds and
// CalSpans covered only 5 columns, so Go emitted colspan=5 where Python
// emits colspan=6. TSR input is identical on both sides, so the divergence
// was purely a Go assembly bug, not a model issue.
//
// Requires DEEPDOC_URL (OSS DeepDoc) reachable; skips otherwise.
func Test1PdfColspanEndToEnd(t *testing.T) {
	client := mustConnectInferenceClient(t)

	pdfDir := common.GetEnv("BATCH_PARITY_PDF_DIR")
	if pdfDir == "" {
		pdfDir = "testdata/real_pdfs"
	}
	pdfPath := filepath.Join(pdfDir, "1.pdf")
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read 1.pdf: %v", err)
	}

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(t.Context(), data, client)
	if err != nil {
		t.Fatalf("Parse 1.pdf: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("1.pdf: no tables detected")
	}

	// The colspan="6" lives in the <table> HTML injected into the sections
	// by ExtractTableAndReplace. Find the header cell spanning 6 columns.
	var allHTML strings.Builder
	for _, s := range result.Sections {
		allHTML.WriteString(s.Text)
	}
	html := allHTML.String()
	if !strings.Contains(html, "colspan=6") {
		t.Errorf("1.pdf: expected <th colspan=6> in production table HTML (matching Python golden), "+
			"but colspan=6 not found. Go assembly still under-covers the span.\nHTML excerpt:\n%s",
			htmlExcerpt(html, 600))
	}

	_ = common.GetEnv // keep common import for parity with sibling tests
}

func htmlExcerpt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
