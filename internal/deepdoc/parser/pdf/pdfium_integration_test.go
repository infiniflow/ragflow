//go:build cgo && manual

package pdf

import (
	"context"
	"image"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

func TestParse_PdfiumRender(t *testing.T) {
	// Use a small controlled test PDF from the testdata/pdfs directory.
	pdfPath := filepath.Join("testdata", "pdfs", "01_english_simple.pdf")
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	eng, err := NewEngine(data)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Verify RawData is available and correct.
	raw := eng.RawData()
	if len(raw) == 0 {
		t.Fatal("RawData() returned empty data")
	}
	if len(raw) != len(data) {
		t.Fatalf("RawData() length %d != original %d", len(raw), len(data))
	}

	// Render a page through pdfium (via the parser's RenderPageToImage).
	img, err := RenderPageToImage(eng, 0)
	if err != nil {
		t.Skipf("pdfium render not available: %v", err)
	}
	b := img.Bounds()
	t.Logf("01_english_simple.pdf page 0: %dx%d", b.Dx(), b.Dy())
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Errorf("expected non-zero dimensions from pdfium render, got %dx%d", b.Dx(), b.Dy())
	}

	// Run Parse with pdfium rendering — BATCH_SKIP_DEEPDOC=1 to avoid HTTP calls.
	t.Setenv("BATCH_SKIP_DEEPDOC", "1")
	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("Parse: %d sections, %d tables, %d page images", len(result.Sections), len(result.Tables), len(result.PageImages))

	if len(result.Sections) == 0 {
		t.Error("expected at least one section")
	}
	if len(result.PageImages) == 0 {
		t.Error("expected at least one page image")
	}
}

func TestParse_PdfiumRender_NoData(t *testing.T) {
	// When engine has no raw PDF bytes, RenderPageToImage falls back to
	// engine.RenderPageImage().  Stub returns (nil, nil) → guard converts
	// to ErrNoPDFData so callers never receive a nil image with nil error.
	img, err := RenderPageToImage(&pythonCharEngineStub{}, 0)
	if err != ErrNoPDFData {
		t.Errorf("expected ErrNoPDFData, got %v", err)
	}
	if img != nil {
		t.Error("expected nil image")
	}
}

// pythonCharEngineStub implements pdf.PDFEngine with RawData() returning nil.
type pythonCharEngineStub struct{}

func (e *pythonCharEngineStub) ExtractChars(_ int) ([]pdf.TextChar, error)  { return nil, nil }
func (e *pythonCharEngineStub) RenderPage(_ int, _ float64) ([]byte, error) { return nil, nil }
func (e *pythonCharEngineStub) RenderPageImage(_ int, _ float64) (image.Image, error) {
	return nil, nil
}
func (e *pythonCharEngineStub) RawData() []byte                  { return nil }
func (e *pythonCharEngineStub) PageCount() (int, error)          { return 0, nil }
func (e *pythonCharEngineStub) Close() error                     { return nil }
func (e *pythonCharEngineStub) Outlines() ([]pdf.Outline, error) { return nil, nil }
