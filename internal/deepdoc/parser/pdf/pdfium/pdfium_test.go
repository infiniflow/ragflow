//go:build cgo && manual

package pdfium

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// testdataDir points at the shared test-pdf directory.
var testdataDir = filepath.Join("..", "testdata", "pdfs")

func readPDF(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func TestRenderPage_EnglishSimple(t *testing.T) {
	data := readPDF(t, "01_english_simple.pdf")
	img, err := RenderPage(data, 0, 72)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	t.Logf("01_english_simple.pdf @ 72 DPI: %dx%d", b.Dx(), b.Dy())
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Errorf("expected non-zero dimensions, got %dx%d", b.Dx(), b.Dy())
	}
	// Must not be pure white (text should be present).
	if isPureWhite(img) {
		t.Error("rendered page is pure white — expected text content")
	}
}

func TestRenderPage_ChineseSimple(t *testing.T) {
	data := readPDF(t, "02_chinese_simple.pdf")
	img, err := RenderPage(data, 0, 72)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	t.Logf("02_chinese_simple.pdf @ 72 DPI: %dx%d", b.Dx(), b.Dy())
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Errorf("expected non-zero dimensions, got %dx%d", b.Dx(), b.Dy())
	}
	if isPureWhite(img) {
		t.Error("rendered page is pure white — expected text content")
	}
}

func TestRenderPage_MultiPage(t *testing.T) {
	data := readPDF(t, "03_multipage.pdf")
	// Render both pages.
	for pg := 0; pg < 2; pg++ {
		img, err := RenderPage(data, pg, 72)
		if err != nil {
			t.Fatalf("page %d: %v", pg, err)
		}
		b := img.Bounds()
		t.Logf("03_multipage.pdf page %d @ 72 DPI: %dx%d", pg, b.Dx(), b.Dy())
		if b.Dx() <= 0 || b.Dy() <= 0 {
			t.Errorf("page %d: expected non-zero dimensions", pg)
		}
	}
}

func TestRenderPage_OutOfRange(t *testing.T) {
	data := readPDF(t, "01_english_simple.pdf")
	_, err := RenderPage(data, 99, 72)
	if err == nil {
		t.Error("expected error for out-of-range page index")
	}
}

func TestRenderPage_InvalidPDF(t *testing.T) {
	_, err := RenderPage([]byte("not a pdf"), 0, 72)
	if err == nil {
		t.Error("expected error for invalid PDF data")
	}
}

func TestRenderPage_EmptyData(t *testing.T) {
	_, err := RenderPage(nil, 0, 72)
	if err == nil {
		t.Error("expected error for nil data")
	}
	_, err = RenderPage([]byte{}, 0, 72)
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestRenderPage_DPI(t *testing.T) {
	data := readPDF(t, "01_english_simple.pdf")

	// Higher DPI → larger image.
	low, err := RenderPage(data, 0, 72)
	if err != nil {
		t.Fatal(err)
	}
	high, err := RenderPage(data, 0, 144)
	if err != nil {
		t.Fatal(err)
	}
	lw, lh := low.Bounds().Dx(), low.Bounds().Dy()
	hw, hh := high.Bounds().Dx(), high.Bounds().Dy()
	t.Logf("72 DPI: %dx%d  144 DPI: %dx%d", lw, lh, hw, hh)

	if hw < lw*2-2 || hw > lw*2+2 {
		t.Errorf("144 DPI width %d not ≈ 2× 72 DPI width %d", hw, lw)
	}
	if hh < lh*2-2 || hh > lh*2+2 {
		t.Errorf("144 DPI height %d not ≈ 2× 72 DPI height %d", hh, lh)
	}
}

func TestRenderPage_AllTestPDFs(t *testing.T) {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Skipf("testdata dir not found: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".pdf" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(testdataDir, e.Name()))
		if err != nil {
			t.Errorf("%s: read: %v", e.Name(), err)
			continue
		}
		img, err := RenderPage(data, 0, 72)
		if err != nil {
			t.Errorf("%s: RenderPage: %v", e.Name(), err)
			continue
		}
		b := img.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			t.Errorf("%s: zero dimensions %dx%d", e.Name(), b.Dx(), b.Dy())
		}
		t.Logf("%s: %dx%d", e.Name(), b.Dx(), b.Dy())
	}
}

func isPureWhite(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// RGBA() returns premultiplied values in [0, 65535].
			if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
				return false
			}
		}
	}
	return true
}

func TestPageSize(t *testing.T) {
	// Non-rotated A4: expect ~595×842
	data := readPDF(t, "rotate_0.pdf")
	w, h, err := PageSize(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if w < 500 || w > 700 || h < 700 || h > 900 {
		t.Errorf("rotate_0.pdf: got %.1f×%.1f, want ~595×842", w, h)
	}
	t.Logf("rotate_0.pdf: %.1f×%.1f pts", w, h)

	// Rotate=90 A4: expect swapped ~842×595
	data90 := readPDF(t, "rotate_90.pdf")
	w90, h90, err := PageSize(data90, 0)
	if err != nil {
		t.Fatal(err)
	}
	if w90 < 700 || w90 > 950 || h90 < 500 || h90 > 700 {
		t.Errorf("rotate_90.pdf: got %.1f×%.1f, want ~842×595 (swapped)", w90, h90)
	}
	t.Logf("rotate_90.pdf: %.1f×%.1f pts (post-rotation)", w90, h90)

	// Verify dimensions ARE swapped relative to Rotate=0
	if math.Abs(w-w90) < 50 {
		t.Errorf("Rotate=90 width %.1f not significantly different from Rotate=0 width %.1f — rotation not reflected?", w90, w)
	}
	if math.Abs(w-h90) > 2 || math.Abs(h-w90) > 2 {
		t.Errorf("Rotate=90 dimensions (%.1f×%.1f) are not swapped from Rotate=0 (%.1f×%.1f)", w90, h90, w, h)
	}

	// Invalid page index
	_, _, err = PageSize(data, 999)
	if err == nil {
		t.Error("expected error for out-of-range page")
	}

	// Empty data
	_, _, err = PageSize([]byte{}, 0)
	if err == nil {
		t.Error("expected error for empty PDF data")
	}
}

// TestPdfiumConcurrentSafety verifies that the pdfiumMu mutex prevents
// SIGSEGV from concurrent pdfium access. Without the mutex, 10 goroutines
// calling PageSize/RenderPage simultaneously causes heap corruption within
// milliseconds (empirically proven). If this test completes without
// crashing, the mutex is working.
func TestPdfiumConcurrentSafety(t *testing.T) {
	data := readPDF(t, "01_english_simple.pdf")

	const goroutines = 10
	const iterations = 3

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if _, _, err := PageSize(data, 0); err != nil {
					t.Errorf("PageSize: %v", err)
					return
				}
				if img, err := RenderPage(data, 0, 72); err != nil {
					t.Errorf("RenderPage: %v", err)
					return
				} else if img.Bounds().Dx() <= 0 {
					t.Error("RenderPage returned zero-width image")
					return
				}
			}
		}()
	}
	wg.Wait()
	// Reaching here without SIGSEGV = mutex is effective.
}
