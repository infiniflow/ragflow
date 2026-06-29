//go:build cgo && manual

package pdfium

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── ExtractOutlines unit tests ────────────────────────────────────────

func TestExtractOutlines_Empty(t *testing.T) {
	// Create a minimal valid PDF without any outlines.
	minimal := []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R>>endobj\nxref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \ntrailer<</Size 4/Root 1 0 R>>\nstartxref\n190\n%%EOF")

	outlines := ExtractOutlines(minimal)
	if len(outlines) != 0 {
		t.Errorf("expected 0 outlines, got %d: %+v", len(outlines), outlines)
	}
}

func TestExtractOutlines_NilOrEmpty(t *testing.T) {
	if out := ExtractOutlines(nil); len(out) != 0 {
		t.Errorf("expected 0 outlines for nil, got %d", len(out))
	}
	if out := ExtractOutlines([]byte{}); len(out) != 0 {
		t.Errorf("expected 0 outlines for empty, got %d", len(out))
	}
}

// ── Integration tests (need real PDF with outlines) ────────────────────

func TestExtractOutlines_RealPDF(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pdfData, err := loadTestPDFWithOutlines()
	if err != nil {
		t.Skipf("no test PDF with outlines available: %v", err)
	}

	outlines := ExtractOutlines(pdfData)
	if len(outlines) == 0 {
		t.Log("PDF may have no outlines; not necessarily an error")
		return
	}

	t.Logf("found %d outline entries:", len(outlines))
	for i, o := range outlines {
		t.Logf("  [%d] level=%d page=%d title=%q", i, o.Level, o.PageNumber, o.Title)
	}

	for i, o := range outlines {
		if o.Title == "" {
			t.Errorf("outline[%d]: empty title", i)
		}
		if o.PageNumber < 1 {
			t.Errorf("outline[%d]: invalid PageNumber %d (<1)", i, o.PageNumber)
		}
		if o.Level < 0 {
			t.Errorf("outline[%d]: negative Level %d", i, o.Level)
		}
	}
}

func TestExtractOutlines_ChineseTitle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pdfData, err := loadTestPDFWithOutlines()
	if err != nil {
		t.Skipf("no test PDF available: %v", err)
	}
	outlines := ExtractOutlines(pdfData)
	for _, o := range outlines {
		for _, r := range o.Title {
			if r >= 0x4E00 && r <= 0x9FFF { // CJK Unified Ideograph
				if strings.ContainsRune(o.Title, '�') {
					t.Errorf("title contains U+FFFD (UTF-16LE decode error): %q", o.Title)
				}
				return // found CJK, verified no replacement chars
			}
		}
	}
	t.Log("no CJK characters found in outlines (skip)")
}

// ── helpers ────────────────────────────────────────────────────────────

var testPDFDirs = []string{
	"../../testdata/real_pdfs",
	"../testdata/real_pdfs",
	"testdata/real_pdfs",
}

func loadTestPDFWithOutlines() ([]byte, error) {
	for _, dir := range testPDFDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".pdf") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if o := ExtractOutlines(data); len(o) > 0 {
				return data, nil
			}
		}
	}
	return nil, fmt.Errorf("no PDF with outlines found in %v", testPDFDirs)
}
