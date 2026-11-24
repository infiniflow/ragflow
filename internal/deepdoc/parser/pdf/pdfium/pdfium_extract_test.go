//go:build cgo

package pdfium

import (
	"fmt"
	"testing"
)

func makeMultiPagePDF(pages int) []byte {
	// Simple minimal valid PDF generator for N pages
	var buf []byte
	buf = append(buf, []byte("%PDF-1.4\n")...)

	// Obj 1: Catalog
	buf = append(buf, []byte("1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n")...)

	// Obj 2: Pages
	kids := ""
	for i := 0; i < pages; i++ {
		kids += fmt.Sprintf("%d 0 R ", 3+i)
	}
	buf = append(buf, []byte(fmt.Sprintf("2 0 obj\n<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n", kids, pages))...)

	// Pages objects
	for i := 0; i < pages; i++ {
		buf = append(buf, []byte(fmt.Sprintf("%d 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n", 3+i))...)
	}

	// xref
	startxref := len(buf)
	buf = append(buf, []byte("xref\n0 1\n0000000000 65535 f \n")...)
	buf = append(buf, []byte(fmt.Sprintf("trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF", 3+pages, startxref))...)
	return buf
}

func TestExtractPages_Basic(t *testing.T) {
	threePagePDF := makeMultiPagePDF(3)

	// Extract page 1 (middle page)
	sliced, err := ExtractPages(threePagePDF, []int{1})
	if err != nil {
		t.Fatalf("ExtractPages failed: %v", err)
	}
	if len(sliced) == 0 {
		t.Fatal("extracted PDF is empty")
	}

	// Extract range [0, 1]
	slicedRange, err := ExtractPageRange(threePagePDF, 0, 1)
	if err != nil {
		t.Fatalf("ExtractPageRange failed: %v", err)
	}
	if len(slicedRange) == 0 {
		t.Fatal("extracted range PDF is empty")
	}

	// Out of bounds page
	_, err = ExtractPages(threePagePDF, []int{5})
	if err == nil {
		t.Error("expected error for out of bounds page index, got nil")
	}

	// Invalid range
	_, err = ExtractPageRange(threePagePDF, 2, 1)
	if err == nil {
		t.Error("expected error for invalid page range, got nil")
	}
}
