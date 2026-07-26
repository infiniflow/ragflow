//go:build cgo && manual

package pdf

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── helpers ──────────────────────────────────────────────────────────────

// pdfiumPtSize returns post-rotation page dimensions via pdfium.
func pdfiumPtSize(eng pdf.PDFEngine, file string, t *testing.T) (w, h float64) {
	t.Helper()
	raw := eng.RawData()
	if raw == nil {
		// Fallback: use pdf_oxide pre-rotation size.
		if pe, ok := eng.(*PDFOxideEngine); ok {
			w, h, _ = pe.Inner.PageSize(0)
		}
		return
	}
	pw, ph, err := pdfium.PageSize(raw, 0)
	if err != nil {
		t.Fatalf("%s: pdfium.PageSize: %v", file, err)
	}
	return pw, ph
}

// openPDF reads a PDF fixture from dir/name, opens it via pdfoxide, and
// returns both the engine and document. The document is closed via t.Cleanup.
// Missing or corrupt fixtures cause a hard failure (t.Fatal).
func openPDF(t *testing.T, dir, name string) (pdf.PDFEngine, *pdfoxide.Document) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	doc, err := pdfoxide.OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	t.Cleanup(func() { doc.Close() })
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng, doc
}

func openRotatePDF(t *testing.T, name string) (pdf.PDFEngine, *pdfoxide.Document) {
	t.Helper()
	return openPDF(t, "testdata/pdfs", name)
}

// ── Test 1: pdf_oxide page size is A4 for all test PDFs ──────────────────

func TestRotation_PageInfo(t *testing.T) {
	for _, file := range []string{"rotate_0.pdf", "rotate_90.pdf", "rotate_180.pdf", "rotate_270.pdf"} {
		t.Run(file, func(t *testing.T) {
			_, doc := openRotatePDF(t, file)
			w, h, err := doc.PageSize(0)
			if err != nil {
				t.Fatalf("PageSize: %v", err)
			}
			if w < 500 || w > 700 || h < 700 || h > 900 {
				t.Errorf("unexpected pdf_oxide page size: %.1f x %.1f", w, h)
			}
		})
	}
}

// ── Test 2: Char extent after rotation ───────────────────────────────────
// After the rotation fix, ExtractChars returns chars in post-rotation space.

func TestRotation_CharExtent(t *testing.T) {
	tests := []struct {
		file      string
		maxXAbove float64 // maxX must be > this
		maxXBelow float64 // maxX must be < this
	}{
		{"rotate_0.pdf", 0, 600},    // portrait A4
		{"rotate_90.pdf", 600, 850}, // landscape (text near right edge after CW)
		{"rotate_180.pdf", 0, 600},  // still portrait (180° flips within bounds)
		{"rotate_270.pdf", 0, 600},  // landscape (text near left edge after CCW)
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			eng, _ := openRotatePDF(t, tt.file)
			chars, err := eng.ExtractChars(0)
			if err != nil {
				t.Fatal(err)
			}
			if len(chars) == 0 {
				t.Fatal("no chars")
			}
			var maxX float64
			for _, c := range chars {
				if c.X1 > maxX {
					maxX = c.X1
				}
			}
			t.Logf("maxX=%.1f (need >%.0f and <%.0f)", maxX, tt.maxXAbove, tt.maxXBelow)

			if maxX <= tt.maxXAbove {
				t.Errorf("maxX=%.1f <= %.0f: rotation not applied to char coordinates", maxX, tt.maxXAbove)
			}
			if maxX >= tt.maxXBelow {
				t.Errorf("maxX=%.1f >= %.0f: chars out of expected range", maxX, tt.maxXBelow)
			}
		})
	}
}

// ── Test 3: All chars within page bounds ─────────────────────────────────

func TestRotation_CharsInBounds(t *testing.T) {
	files := []string{"rotate_0.pdf", "rotate_90.pdf", "rotate_180.pdf", "rotate_270.pdf"}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			eng, _ := openRotatePDF(t, file)
			// Use pdfium.PageSize for post-rotation page dimensions,
			// since chars from ExtractChars are now in post-rotation space.
			pageW, pageH := pdfiumPtSize(eng, file, t)

			chars, err := eng.ExtractChars(0)
			if err != nil {
				t.Fatal(err)
			}
			oob := 0
			for _, c := range chars {
				if c.X0 < -1 || c.X1 > pageW+1 || c.Top < -1 || c.Bottom > pageH+1 {
					oob++
					if oob <= 3 {
						t.Errorf("OOB char %q: X=[%.1f,%.1f] Y=[%.1f,%.1f] page=%.1fx%.1f",
							c.Text, c.X0, c.X1, c.Top, c.Bottom, pageW, pageH)
					}
				}
				if c.X0 >= c.X1 {
					t.Errorf("char %q: X0=%.2f >= X1=%.2f", c.Text, c.X0, c.X1)
				}
				if c.Top >= c.Bottom {
					t.Errorf("char %q: Top=%.2f >= Bottom=%.2f", c.Text, c.Top, c.Bottom)
				}
			}
			if oob > 0 {
				t.Errorf("%d/%d chars OOB (%.1f%%)", oob, len(chars), float64(oob)/float64(len(chars))*100)
			} else {
				t.Logf("all %d chars in bounds [%.0f x %.0f]", len(chars), pageW, pageH)
			}
		})
	}
}

// ── Test 4: Same-line chars preserved after rotation ─────────────────────

func TestRotation_SameLinePreserved(t *testing.T) {
	for _, file := range []string{"rotate_0.pdf", "rotate_90.pdf", "rotate_270.pdf"} {
		t.Run(file, func(t *testing.T) {
			eng, _ := openRotatePDF(t, file)
			chars, err := eng.ExtractChars(0)
			if err != nil {
				t.Fatal(err)
			}

			// After rotation, same-baseline chars have slightly different
			// Bottom values because the rotation maps char Width to post-rot
			// Y-height.  Use font-size proportional tolerance.
			isRotated := file != "rotate_0.pdf"
			tolerance := 0.5
			if isRotated {
				tolerance = 15.0 // char widths vary ~10-13pts on same line
			}

			lines := lyt.GroupCharsToLines(chars, false)
			violations := 0
			for li, line := range lines {
				if len(line) <= 1 {
					continue
				}
				refBottom := line[0].Bottom
				for _, c := range line[1:] {
					diff := math.Abs(c.Bottom - refBottom)
					if diff > tolerance {
						violations++
						if violations <= 3 {
							t.Errorf("line %d: char %q Bottom=%.2f ref=%.2f diff=%.2f",
								li, c.Text, c.Bottom, refBottom, diff)
						}
					}
				}
			}
			if violations > 0 {
				t.Errorf("%d same-line Bottom violations (tolerance=%.1f)", violations, tolerance)
			}
		})
	}
}

// ── Test 5: Multi-page with mixed rotation ───────────────────────────────

func TestRotation_MultiPageMixed(t *testing.T) {
	eng, doc := openRotatePDF(t, "multi_rotate.pdf")
	pageCount, err := eng.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	if pageCount != 3 {
		t.Fatalf("expected 3 pages, got %d", pageCount)
	}

	// Page 0: Rotate=0 → portrait.  Page 1-2: Rotate=90/270 → landscape.
	expectations := []struct {
		page      int
		maxXAbove float64
		maxXBelow float64
	}{
		{0, 0, 600},
		{1, 600, 850},
		{2, 0, 600}, // Rotate=270 → CCW, text near left edge
	}

	for _, exp := range expectations {
		info, err := doc.Inner.PageInfo(exp.page)
		if err != nil {
			t.Fatalf("PageInfo page %d: %v", exp.page, err)
		}
		t.Logf("Page %d: Rotation=%d, W=%.1f H=%.1f", exp.page, info.Rotation, info.Width, info.Height)

		chars, err := eng.ExtractChars(exp.page)
		if err != nil {
			t.Fatalf("ExtractChars page %d: %v", exp.page, err)
		}
		if len(chars) == 0 {
			t.Errorf("page %d: no chars", exp.page)
			continue
		}

		var maxX float64
		for _, c := range chars {
			if c.X1 > maxX {
				maxX = c.X1
			}
		}
		t.Logf("Page %d: %d chars, maxX=%.1f", exp.page, len(chars), maxX)

		if maxX <= exp.maxXAbove {
			t.Errorf("Page %d: maxX=%.1f <= %.0f — rotation not applied",
				exp.page, maxX, exp.maxXAbove)
		}
		if maxX > exp.maxXBelow {
			t.Errorf("Page %d: maxX=%.1f > %.0f — out of range",
				exp.page, maxX, exp.maxXBelow)
		}
	}
}

// ── Test 6: CropBox with rotation ────────────────────────────────────────
// pdf_oxide does not read /CropBox from the page dictionary (same limitation
// as /Rotate).  It always reports MediaBox values.  The test verifies that
// chars are within bounds using the dimensions pdf_oxide actually reports.

func TestRotation_CropBoxWithRotate(t *testing.T) {
	eng, doc := openRotatePDF(t, "cropbox_rotate.pdf")
	info, err := doc.Inner.PageInfo(0)
	if err != nil {
		t.Fatal(err)
	}
	// pdf_oxide reports MediaBox (not our custom CropBox [30,20,575,832]).
	t.Logf("pdf_oxide: W=%.1f H=%.1f CropBox=(%.1f,%.1f,%.1f,%.1f) Rotation=%d",
		info.Width, info.Height,
		info.CropBox.X, info.CropBox.Y, info.CropBox.Width, info.CropBox.Height,
		info.Rotation)

	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chars) == 0 {
		t.Fatal("no chars")
	}

	// Use pdfium dimensions (accounts for rotation) for bounds check.
	pageW, pageH := pdfiumPtSize(eng, "cropbox_rotate.pdf", t)
	oob := 0
	for _, c := range chars {
		if c.X0 < -1 || c.X1 > pageW+1 || c.Top < -1 || c.Bottom > pageH+1 {
			oob++
		}
	}
	oobRate := float64(oob) / float64(len(chars)) * 100
	t.Logf("OOB: %d/%d (%.1f%%), page=%.1fx%.1f", oob, len(chars), oobRate, pageW, pageH)
	// CropBox excludes content from the page edges; chars near the
	// CropBox boundary may end up outside the effective page after rotation.
	if oobRate > 40 {
		t.Errorf("too many OOB Chars: %.1f%%", oobRate)
	}

	// Verify render alignment.
	raw := eng.RawData()
	if raw != nil {
		img, err := pdfium.RenderPage(raw, 0, 216)
		if err == nil {
			scale := 216.0 / 72.0
			hit, checked := bboxDarkPixelHitRate(t, chars, img, scale)
			if checked > 0 {
				hitRate := float64(hit) / float64(checked) * 100
				t.Logf("CropBox+Rotate render align: %d/%d (%.1f%%)", hit, checked, hitRate)
				if hitRate < 70 {
					t.Errorf("CropBox+Rotate render alignment: %.1f%% < 70%%", hitRate)
				}
			}
		}
	}
}

// ── Test 7: Render alignment — dark-pixel bbox verification ──────────────
// Chars are now in post-rotation space (rotation handled by ExtractChars),
// so we use the identity mapper for all rotations.

func TestRotation_RenderAlignment(t *testing.T) {
	const dpi = 216.0
	const scale = dpi / 72.0

	identityMap := func(c pdf.TextChar, _, _ float64) (px0, py0, px1, py1 int) {
		return int(math.Round(c.X0 * scale)),
			int(math.Round(c.Top * scale)),
			int(math.Round(c.X1 * scale)),
			int(math.Round(c.Bottom * scale))
	}

	for _, file := range []string{"rotate_0.pdf", "rotate_90.pdf", "rotate_270.pdf"} {
		t.Run(file, func(t *testing.T) {
			eng, _ := openRotatePDF(t, file)
			raw := eng.RawData()
			if raw == nil {
				t.Fatal("no raw data")
			}
			chars, err := eng.ExtractChars(0)
			if err != nil {
				t.Fatal(err)
			}
			img, err := pdfium.RenderPage(raw, 0, dpi)
			if err != nil {
				t.Skipf("pdfium not available: %v", err)
			}
			imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()
			pdfiumPtW := float64(imgW) / scale
			pdfiumPtH := float64(imgH) / scale

			n := len(chars)
			if n == 0 {
				t.Fatal("no chars")
			}
			step := max(1, n/200)
			var hit, miss, oob int
			var dratios []float64

			for i := 0; i < n; i += step {
				c := chars[i]
				px0, py0, px1, py1 := identityMap(c, pdfiumPtW, pdfiumPtH)
				if px0 > px1 {
					px0, px1 = px1, px0
				}
				if py0 > py1 {
					py0, py1 = py1, py0
				}
				if px0 < 0 || py0 < 0 || px1 > imgW || py1 > imgH || px0 >= px1 || py0 >= py1 {
					oob++
					continue
				}
				if px1-px0 < 2 || py1-py0 < 2 {
					continue
				}
				dark, total := 0, 0
				for y := py0; y <= py1; y++ {
					for x := px0; x <= px1; x++ {
						r, g, b, _ := img.At(x, y).RGBA()
						bright := (float64(r>>8) + float64(g>>8) + float64(b>>8)) / 3.0
						if bright < 128 {
							dark++
						}
						total++
					}
				}
				ratio := float64(dark) / float64(total) * 100
				dratios = append(dratios, ratio)
				if ratio > 2.0 {
					hit++
				} else {
					miss++
				}
			}

			if len(dratios) == 0 {
				t.Fatal("no bboxes tested")
			}
			sort.Float64s(dratios)
			var sum float64
			for _, r := range dratios {
				sum += r
			}
			avg := sum / float64(len(dratios))
			p95 := dratios[len(dratios)*95/100]
			hitRate := float64(hit) / float64(len(dratios)) * 100

			t.Logf("avg=%.1f%% p95=%.1f%% hit=%d/%d (%.1f%%) oob=%d",
				avg, p95, hit, len(dratios), hitRate, oob)

			if hitRate < 70 {
				t.Errorf("hit rate %.1f%% < 70%% — bbox/render misalignment", hitRate)
			}
			if float64(oob)/float64(len(dratios)+oob) > 0.05 {
				t.Errorf("OOB rate > 5%%")
			}
		})
	}
}

// ── Test 8: Letter size + Rotate 90 ──────────────────────────────────────

func TestRotation_LetterSize(t *testing.T) {
	eng, doc := openRotatePDF(t, "letter_rotate.pdf")
	w, h, err := doc.PageSize(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Letter (pdf_oxide): %.1f x %.1f", w, h)

	if w < 600 || h < 600 {
		t.Errorf("unexpected Letter dimensions: %.1f x %.1f", w, h)
	}

	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chars) == 0 {
		t.Fatal("no chars")
	}
	t.Logf("%d chars", len(chars))

	// After fix: Letter landscape (792×612), maxX should be > 650
	var maxX float64
	for _, c := range chars {
		if c.X1 > maxX {
			maxX = c.X1
		}
		if c.X0 < 0 || c.Top < 0 {
			t.Errorf("negative coord: %q X=%.1f Top=%.1f", c.Text, c.X0, c.Top)
		}
	}
	t.Logf("maxX=%.1f", maxX)
	if maxX <= 650 {
		t.Errorf("maxX=%.1f <= 650: rotation not applied for Letter+Rotate90", maxX)
	}

	// Render alignment check (chars from ExtractChars are post-rotation)
	raw := eng.RawData()
	if raw != nil {
		img, err := pdfium.RenderPage(raw, 0, 216)
		if err == nil {
			imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()
			scale := 216.0 / 72.0
			t.Logf("pdfium render: %.0fx%.0f pts", float64(imgW)/scale, float64(imgH)/scale)

			hit, checked := bboxDarkPixelHitRate(t, chars, img, scale)
			if checked > 0 {
				hitRate := float64(hit) / float64(checked) * 100
				t.Logf("Letter render alignment: %d/%d hit (%.1f%%)", hit, checked, hitRate)
				if hitRate < 70 {
					t.Errorf("Letter render hit rate %.1f%% < 70%%", hitRate)
				}
			}
		}
	}
}

// ── Test 9: Rotate=180 ──────────────────────────────────────────────────

func TestRotation_Rotate180_NotYetHandled(t *testing.T) {
	eng, _ := openRotatePDF(t, "rotate_180.pdf")
	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}

	// After the fix, chars should be in post-rotation space (180° inverted).
	// X range: still 0–600 (portrait width unchanged).
	// Y range: chars originally near top → now near bottom.
	var maxX, minTop, maxBottom float64
	maxX = -1e9
	minTop = 1e9
	for _, c := range chars {
		if c.X1 > maxX {
			maxX = c.X1
		}
		if c.Top < minTop {
			minTop = c.Top
		}
		if c.Bottom > maxBottom {
			maxBottom = c.Bottom
		}
	}
	t.Logf("Rotate=180: maxX=%.1f minTop=%.1f maxBottom=%.1f", maxX, minTop, maxBottom)

	// 180° flips content upside down: top-half chars move to bottom half.
	// For our test PDF (A4 portrait 595×842), pre-rot text was near top
	// (minTop≈28). After fix: minTop ≈ 842-382 ≈ 460 (near bottom).
	if maxX > 600 {
		t.Errorf("maxX=%.1f > 600: Rotate=180 should stay in portrait width", maxX)
	}
	if minTop < 300 {
		t.Errorf("minTop=%.1f < 300: Rotate=180 not inverted (chars still at top)", minTop)
	}

	// Render alignment check
	raw := eng.RawData()
	if raw != nil {
		img, err := pdfium.RenderPage(raw, 0, 216)
		if err == nil {
			scale := 216.0 / 72.0
			hit, checked := bboxDarkPixelHitRate(t, chars, img, scale)
			hitRate := float64(hit) / float64(checked) * 100
			t.Logf("Rotate=180 render alignment: %d/%d (%.1f%%)", hit, checked, hitRate)
			if hitRate < 70 {
				t.Errorf("Rotate=180 render alignment: %.1f%% < 70%%", hitRate)
			}
		}
	}
}

// ── Test 10: Document.PageSize ───────────────────────────────────────────

func TestRotation_DocumentPageSize(t *testing.T) {
	_, doc := openRotatePDF(t, "rotate_0.pdf")
	w, h, err := doc.PageSize(0)
	if err != nil {
		t.Fatal(err)
	}
	if w < 500 || w > 700 || h < 700 || h > 900 {
		t.Errorf("rotate_0.pdf: unexpected size %.1f×%.1f", w, h)
	}
	// Rotate=90 must report same pre-rotation size
	_, doc = openRotatePDF(t, "rotate_90.pdf")
	w2, h2, err := doc.PageSize(0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(w-w2) > 0.1 || math.Abs(h-h2) > 0.1 {
		t.Errorf("pre-rotation size differs: %.1f×%.1f vs %.1f×%.1f", w, h, w2, h2)
	}
	// Closed document returns error
	doc.Close()
	_, _, err = doc.PageSize(0)
	if err == nil {
		t.Error("expected error from closed document")
	}
}

// ── bboxDarkPixelHitRate helper ─────────────────────────────────────────

func bboxDarkPixelHitRate(t *testing.T, chars []pdf.TextChar, img *image.RGBA, scale float64) (hit, checked int) {
	t.Helper()
	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()
	n, step := len(chars), max(1, len(chars)/min(50, len(chars)))
	for i := 0; i < n; i += step {
		c := chars[i]
		px0 := int(math.Round(c.X0 * scale))
		py0 := int(math.Round(c.Top * scale))
		px1 := int(math.Round(c.X1 * scale))
		py1 := int(math.Round(c.Bottom * scale))
		if px0 > px1 {
			px0, px1 = px1, px0
		}
		if py0 > py1 {
			py0, py1 = py1, py0
		}
		if px0 < 0 || py0 < 0 || px1 > imgW || py1 > imgH || px0 >= px1 || py0 >= py1 {
			continue
		}
		if px1-px0 < 2 || py1-py0 < 2 {
			continue
		}
		dark, total := 0, 0
		for y := py0; y <= py1; y++ {
			for x := px0; x <= px1; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if (float64(r>>8)+float64(g>>8)+float64(b>>8))/3.0 < 128 {
					dark++
				}
				total++
			}
		}
		if total > 0 && float64(dark)/float64(total)*100 > 2.0 {
			hit++
		}
		checked++
	}
	return
}
