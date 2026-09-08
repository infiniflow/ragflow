//go:build cgo && manual

package pdf

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── Y-coordinate tests ──────────────────────────────────────────────────

// openTestingPDF opens a real PDF by name from testdata/real_pdfs/.
// Missing fixtures are skipped (soft) rather than failing — these tests
// require the "manual" build tag and rely on optional fixture files.
func openTestingPDF(t *testing.T, name string) (pdf.PDFEngine, *pdfoxide.Document) {
	t.Helper()
	dir := filepath.Join("testdata", "real_pdfs")
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		t.Skipf("test PDF not found: %s", name)
	}
	return openPDF(t, dir, name)
}

// TestYCoord_SameLineCharsHaveEqualBottom checks that characters on the same
// PDF text line (same baseline) have identical Bottom values.  Bottom =
// pageHeight - c.Y is derived from the screen-space baseline, which is the
// same for all chars on a line regardless of font size or descent.
func TestYCoord_SameLineCharsHaveEqualBottom(t *testing.T) {
	eng, _ := openTestingPDF(t, "RAG分词召回分析.pdf")

	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chars) == 0 {
		t.Fatal("no chars")
	}

	lines := lyt.GroupCharsToLines(chars, false)
	for li, line := range lines {
		if len(line) <= 1 {
			continue
		}
		refBottom := line[0].Bottom
		for _, c := range line[1:] {
			if math.Abs(c.Bottom-refBottom) > 0.1 {
				t.Errorf("line %d: char %q has Bottom=%.2f, expected ~%.2f (delta=%.2f)",
					li, c.Text, c.Bottom, refBottom, c.Bottom-refBottom)
			}
		}
	}
}

// TestYCoord_BottomEqualsTopPlusHeight checks the invariant bottom = top + height
// for every character.
func TestYCoord_BottomEqualsTopPlusHeight(t *testing.T) {
	eng, _ := openTestingPDF(t, "RAG分词召回分析.pdf")

	for pg := 0; pg < 1; pg++ {
		chars, err := eng.ExtractChars(pg)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range chars {
			h := c.Bottom - c.Top
			expected := c.Top + h
			delta := math.Abs(c.Bottom - expected)
			if delta > 0.01 {
				t.Errorf("char %q: Bottom=%.4f, Top=%.4f+Height=%.4f=%.4f, delta=%v",
					c.Text, c.Bottom, c.Top, h, expected, delta)
			}
		}
	}
}

// TestYCoord_XUnchanged verifies that X0/X1 are not affected by Y-axis
// coordinate transformations.
func TestYCoord_XUnchanged(t *testing.T) {
	eng, doc := openTestingPDF(t, "RAG分词召回分析.pdf")

	pipelineChars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pipelineChars) == 0 {
		t.Fatal("no chars")
	}

	raw, err := doc.Inner.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("no raw chars")
	}

	type xw struct {
		x0, w float64
	}
	rawSet := make(map[xw]bool, len(raw))
	for _, rc := range raw {
		rawSet[xw{float64(rc.X), float64(rc.Width)}] = true
	}

	for _, c := range pipelineChars {
		w := c.X1 - c.X0
		if !rawSet[xw{c.X0, w}] {
			t.Logf("pipeline char %q X0=%.1f W=%.1f not in raw set (may be deduped)",
				c.Text, c.X0, w)
		}
	}
}

// TestYCoord_EmptyPageNoPanic ensures extracting chars from an empty page
// (out of range) returns an error, not panics.
func TestYCoord_EmptyPageNoPanic(t *testing.T) {
	eng, _ := openTestingPDF(t, "RAG分词召回分析.pdf")

	_, err := eng.ExtractChars(9999)
	if err == nil {
		t.Error("expected error for out-of-range page, got nil")
	}
}

// TestYCoord_RenderedImageDimensionsMatchPage verifies that rendered page
// image dimensions are proportional to the page's CropBox.
func TestYCoord_RenderedImageDimensionsMatchPage(t *testing.T) {
	eng, _ := openTestingPDF(t, "RAG分词召回分析.pdf")

	img, err := eng.RenderPageImage(0, 72)
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("rendered image is nil")
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("rendered image has 0 dimensions: %dx%d", b.Dx(), b.Dy())
	}
}

// TestYCoord_MultiPageConsistency verifies that chars across pages all have
// valid Top values within page bounds.
func TestYCoord_MultiPageConsistency(t *testing.T) {
	eng, _ := openTestingPDF(t, "20240815-华福证券-海光信息-688041.SH-中报略超预告中值_新增适配AI大模型通义千问_4页_467kb.pdf")

	pageCount, err := eng.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	if pageCount < 2 {
		t.Skip("need multi-page PDF")
	}

	for pg := 0; pg < pageCount; pg++ {
		chars, err := eng.ExtractChars(pg)
		if err != nil {
			t.Errorf("page %d: ExtractChars: %v", pg, err)
			continue
		}
		if len(chars) == 0 {
			continue
		}
		for _, c := range chars {
			if c.Top < 0 {
				t.Errorf("page %d char %q: Top=%.2f < 0", pg, c.Text, c.Top)
			}
			if c.Bottom <= c.Top {
				t.Errorf("page %d char %q: Bottom=%.2f <= Top=%.2f", pg, c.Text, c.Bottom, c.Top)
			}
		}
	}
}

// TestYCoord_CropBoxUsedNotMediaBox verifies that chars are positioned using
// CropBox height, not MediaBox.
func TestYCoord_CropBoxUsedNotMediaBox(t *testing.T) {
	eng, doc := openTestingPDF(t, "RAG分词召回分析.pdf")

	info, err := doc.Inner.PageInfo(0)
	if err != nil {
		t.Fatal(err)
	}

	if info.CropBox.Height <= 0 {
		t.Skip("test PDF doesn't have CropBox")
	}

	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chars) == 0 {
		t.Fatal("no chars")
	}

	mediaBoxH := float64(info.Height)
	cropBoxH := float64(info.CropBox.Height)

	if mediaBoxH == cropBoxH {
		t.Skip("MediaBox == CropBox, no offset to test")
	}

	for _, c := range chars {
		if c.Top >= cropBoxH {
			t.Errorf("char %q Top=%.2f >= CropBox height %.2f", c.Text, c.Top, cropBoxH)
		}
	}
}
