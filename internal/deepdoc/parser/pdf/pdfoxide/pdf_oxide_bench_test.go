//go:build cgo && manual

package pdfoxide

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPDFPlumber_Basic(t *testing.T) {
	pdfDir := filepath.Join("..", "testdata", "pdfs")
	path := filepath.Join(pdfDir, "01_english_simple.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PDF: %v", err)
	}

	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	pc, _ := eng.PageCount()
	t.Logf("Pages: %d", pc)

	chars, err := eng.ExtractChars(0)
	if err != nil {
		t.Fatalf("ExtractChars: %v", err)
	}
	t.Logf("Page 0: %d chars extracted", len(chars))
	if len(chars) == 0 {
		t.Error("got 0 chars")
	}

	// Show first few chars
	for i := 0; i < min(5, len(chars)); i++ {
		t.Logf("  char[%d]: text=%q x0=%.1f x1=%.1f top=%.1f bottom=%.1f font=%q",
			i, chars[i].Text, chars[i].X0, chars[i].X1, chars[i].Top, chars[i].Bottom, chars[i].FontName)
	}
}

func BenchmarkPDFPlumber_ExtractChars(b *testing.B) {
	pdfDir := filepath.Join("..", "testdata", "pdfs")
	path := filepath.Join(pdfDir, "01_english_simple.pdf")
	data, _ := os.ReadFile(path)

	eng, _ := NewEngine(data)
	defer eng.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.ExtractChars(0)
	}
}
