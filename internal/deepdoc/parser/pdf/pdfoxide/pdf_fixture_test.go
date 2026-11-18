//go:build cgo

package pdfoxide

import (
	"bytes"
	"fmt"
	"testing"
)

// buildTestPDF returns a minimal but valid PDF with `pages` identical pages at
// the given /Rotate. The bytes are generated in the test so the PDF-parsing
// tests run in a clean checkout, without a tracked binary fixture.
func buildTestPDF(t *testing.T, pages, rotate int) []byte {
	t.Helper()
	if pages < 1 {
		pages = 1
	}

	var buf bytes.Buffer
	var offsets []int
	writeObj := func(s string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(s)
	}

	buf.WriteString("%PDF-1.4\n")
	writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	kids := ""
	for p := 0; p < pages; p++ {
		kids += fmt.Sprintf("%d 0 R ", 3+p)
	}
	fontObj := 3 + pages
	contentObj := 4 + pages
	writeObj(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", kids, pages))
	for p := 0; p < pages; p++ {
		writeObj(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Rotate %d "+
				"/Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>\nendobj\n",
			3+p, rotate, fontObj, contentObj))
	}
	writeObj(fmt.Sprintf("%d 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n", fontObj))
	stream := "BT /F1 24 Tf 20 40 Td (Hello) Tj ET"
	writeObj(fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", contentObj, len(stream), stream))

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes()
}
