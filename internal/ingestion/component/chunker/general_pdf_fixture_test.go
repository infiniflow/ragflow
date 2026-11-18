//go:build cgo

package chunker

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"testing"
)

// newGeneralPDFChildPreviewFixture builds a one-page PDF with three solid
// markers so preview tests do not need an opaque binary fixture.
func newGeneralPDFChildPreviewFixture(t *testing.T) []byte {
	t.Helper()

	content := []byte(
		"1 0 0 rg\n110 582 20 20 re f\n" +
			"0 1 0 rg\n110 472 20 20 re f\n" +
			"0 0 1 rg\n110 372 20 20 re f\n",
	)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, 5)
	writeObject := func(number int, body string) {
		t.Helper()
		offsets[number] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", number, body)
	}
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 842] /Resources << >> /Contents 4 0 R >>")
	writeObject(4, fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", compressed.Len(), compressed.Bytes()))

	xref := pdf.Len()
	pdf.WriteString("xref\n0 5\n")
	pdf.WriteString("0000000000 65535 f \n")
	for number := 1; number < len(offsets); number++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[number])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return pdf.Bytes()
}
