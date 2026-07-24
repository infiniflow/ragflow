//go:build cgo && manual

// Package pagespdfiumtest exercises Config.Pages against a real PDF via the
// pdfium CGO engine. It lives in a separate subpackage so that it does not
// share the build of the parent package's manual-tag test files (some of
// which do not currently compile on main).
package pagespdfiumtest

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/pdf/type"
)

// noopDocAnalyzer reports unhealthy and returns empty results, forcing the
// parser onto the charsToBoxes path so no DeepDoc model service is required.
type noopDocAnalyzer struct{}

func (noopDocAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return nil, nil
}
func (noopDocAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return nil, nil
}
func (noopDocAnalyzer) OCRDetect(context.Context, image.Image) ([]deepdoctype.OCRBox, error) {
	return nil, nil
}
func (noopDocAnalyzer) OCRRecognize(context.Context, image.Image) ([]deepdoctype.OCRText, error) {
	return nil, nil
}
func (noopDocAnalyzer) Health() bool { return false }

// pageKeys extracts the set of processed page numbers from PageHeight.
func pageKeys(m map[int]float64) map[int]struct{} {
	out := make(map[int]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}

// TestPagesRealPdf_PdfiumFilter verifies Config.Pages end-to-end against a
// real multi-page PDF through the pdfium CGO engine (NewEngine ->
// PageCount/ExtractChars/RenderPage call the pdfium C library). DeepDoc
// DLA/TSR/OCR are stubbed by noopDocAnalyzer.
//
// Run with:
//
//	./build.sh --test -tags manual -v -run TestPagesRealPdf_PdfiumFilter ./internal/deepdoc/parser/pdf/pagespdfiumtest/
func TestPagesRealPdf_PdfiumFilter(t *testing.T) {
	t.Setenv("BATCH_SKIP_DEEPDOC", "1")

	pdfPath := filepath.Join("..", "testdata", "pdfs", "03_multipage.pdf")
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read %s: %v", pdfPath, err)
	}

	// NewEngine calls into pdfium (CGO). Failure here means the real C path
	// was reached but pdfium could not open the document.
	eng, err := deepdocpdf.NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine (pdfium CGO): %v", err)
	}
	defer eng.Close()

	total, err := eng.PageCount()
	if err != nil {
		t.Fatalf("eng.PageCount: %v", err)
	}
	t.Logf("03_multipage.pdf: pdfium PageCount() = %d (real CGO call)", total)
	if total < 2 {
		t.Skipf("need >=2 pages to verify filtering, got %d", total)
	}

	mock := noopDocAnalyzer{}

	// Group 1: restrict to page 1 (1-indexed) -> only 0-based page 0.
	t.Run("filter to page 1 only", func(t *testing.T) {
		cfg := deepdoctype.DefaultParserConfig()
		cfg.Pages = [][]int{{1, 1}}
		p := deepdocpdf.NewParser(cfg)
		result, err := p.ParseRaw(context.Background(), eng, mock)
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}
		if len(result.PageHeight) != 1 {
			t.Errorf("expected 1 page parsed, got %d (keys=%v)",
				len(result.PageHeight), pageKeys(result.PageHeight))
		}
		if _, ok := result.PageHeight[0]; !ok {
			t.Errorf("expected page 0 in PageHeight, got keys %v",
				pageKeys(result.PageHeight))
		}
	})

	// Group 2: control — no Pages restriction -> all pages.
	t.Run("no pages -> all pages (control)", func(t *testing.T) {
		p := deepdocpdf.NewParser(deepdoctype.DefaultParserConfig())
		result, err := p.ParseRaw(context.Background(), eng, mock)
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}
		if len(result.PageHeight) != total {
			t.Errorf("expected %d pages (all), got %d (keys=%v)",
				total, len(result.PageHeight), pageKeys(result.PageHeight))
		}
	})

	// Group 3: multi-range — first and last page only.
	if total >= 3 {
		t.Run("multi-range first and last page", func(t *testing.T) {
			cfg := deepdoctype.DefaultParserConfig()
			cfg.Pages = [][]int{{1, 1}, {total, total}}
			p := deepdocpdf.NewParser(cfg)
			result, err := p.ParseRaw(context.Background(), eng, mock)
			if err != nil {
				t.Fatalf("ParseRaw: %v", err)
			}
			want := map[int]struct{}{0: {}, total - 1: {}}
			if got := pageKeys(result.PageHeight); !reflect.DeepEqual(got, want) {
				t.Errorf("PageHeight keys = %v, want %v", got, want)
			}
		})
	}
}
