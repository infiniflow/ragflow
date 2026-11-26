//go:build cgo

package pdfium

import (
	"bytes"
	"fmt"
	"math"
	"testing"
)

// buildTextPDF builds a one-page PDF with Helvetica text on two lines. When
// crop is non-empty it is inserted as the page's /CropBox.
func buildTextPDF(rotate int, crop string) []byte {
	return buildTextPDFContent(rotate, crop,
		"BT /F1 12 Tf 20 40 Td (Hello) Tj 0 30 Td (World) Tj ET")
}

func buildTextPDFContent(rotate int, crop, stream string) []byte {
	var buf bytes.Buffer
	var offsets []int
	writeObj := func(s string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(s)
	}
	buf.WriteString("%PDF-1.4\n")
	writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	writeObj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	writeObj(fmt.Sprintf(
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] %s /Rotate %d "+
			"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n",
		crop, rotate))
	writeObj("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	writeObj(fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream))
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes()
}

func openTextPDF(t *testing.T, rotate int, crop string) *Document {
	t.Helper()
	doc, err := OpenDocument(buildTextPDF(rotate, crop))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	t.Cleanup(doc.Close)
	return doc
}

func findChar(t *testing.T, chars []TextChar, text string) TextChar {
	t.Helper()
	for _, c := range chars {
		if c.Text == text {
			return c
		}
	}
	t.Fatalf("no %q in %d chars", text, len(chars))
	return TextChar{}
}

func assertBBox(t *testing.T, c TextChar, x0, x1, top, bottom float64) {
	t.Helper()
	const tol = 0.02
	if math.Abs(c.X0-x0) > tol || math.Abs(c.X1-x1) > tol ||
		math.Abs(c.Top-top) > tol || math.Abs(c.Bottom-bottom) > tol {
		t.Errorf("%q bbox = (%.3f,%.3f,%.3f,%.3f), want (%.3f,%.3f,%.3f,%.3f)",
			c.Text, c.X0, c.X1, c.Top, c.Bottom, x0, x1, top, bottom)
	}
}

func TestExtractCharsRotation(t *testing.T) {
	const (
		advanceH = 8.664 // Helvetica 12pt advance width of "H"
	)
	cases := []struct {
		rotate              int
		x0, x1, top, bottom float64
	}{
		{0, 20, 20 + advanceH, 48, 60},
		{90, 40, 52, 20, 20 + advanceH},
		{180, 200 - (20 + advanceH), 180, 40, 52},
		{270, 48, 60, 200 - (20 + advanceH), 180},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("rotate_%d", tc.rotate), func(t *testing.T) {
			doc := openTextPDF(t, tc.rotate, "")
			chars, err := doc.ExtractChars(0)
			if err != nil {
				t.Fatal(err)
			}
			if len(chars) != 10 {
				t.Fatalf("got %d chars, want 10", len(chars))
			}
			for _, c := range chars {
				if c.Text == "\r" || c.Text == "\n" {
					t.Errorf("generated line-break char leaked into ExtractChars: %q", c.Text)
				}
			}
			assertBBox(t, findChar(t, chars, "H"), tc.x0, tc.x1, tc.top, tc.bottom)
		})
	}
}

// TestExtractCharsCropBox locks the CropBox transform the previous adapter
// applied, including its duplicated origin shift for unrotated pages. Existing
// extraction coordinates depend on it; changing it is a separate behavior fix,
// not part of the engine swap.
func TestExtractCharsCropBox(t *testing.T) {
	doc := openTextPDF(t, 0, "/CropBox [20 10 180 90]")
	chars, err := doc.ExtractChars(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chars) != 10 {
		t.Fatalf("got %d chars, want 10", len(chars))
	}
	h := findChar(t, chars, "H")
	assertBBox(t, h, -20, -20+8.664, 28, 40)
	w := findChar(t, chars, "W")
	assertBBox(t, w, -20, -20+11.328, -2, 10)
}

func TestPageText(t *testing.T) {
	doc := openTextPDF(t, 0, "")
	text, err := doc.PageText(0)
	if err != nil {
		t.Fatal(err)
	}
	if text != "World\nHello" {
		t.Errorf("PageText = %q, want %q", text, "World\nHello")
	}
}

func TestOpenDocumentEmpty(t *testing.T) {
	if _, err := OpenDocument(nil); err == nil {
		t.Fatal("OpenDocument(nil) returned no error")
	}
}

func TestPageGeometryAndCount(t *testing.T) {
	doc := openTextPDF(t, 90, "")
	count, err := doc.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("PageCount = %d, want 1", count)
	}
	w, h, err := doc.PageSize(0)
	if err != nil {
		t.Fatal(err)
	}
	if w != 100 || h != 200 {
		t.Errorf("PageSize = %.1fx%.1f, want 100x200", w, h)
	}
	if _, err := doc.ExtractChars(1); err == nil {
		t.Error("ExtractChars(1) on a one-page document returned no error")
	}
	doc.Close()
	doc.Close() // idempotent
	if _, err := doc.PageCount(); err == nil {
		t.Error("PageCount on a closed document returned no error")
	}
}

// TestExtractCharsRotatedGlyphs covers text rotated inside the page with a text
// matrix, as opposed to page-level /Rotate. Rotated glyphs keep pdfium's loose
// box; they must still produce positive-area, non-overlapping boxes. The
// previous adapter reported zero-width boxes for 90/270-degree glyphs; that
// artifact is intentionally not reproduced.
func TestExtractCharsRotatedGlyphs(t *testing.T) {
	cases := []struct {
		name       string
		stream     string
		horizontal bool
	}{
		{"rot90", "BT /F1 12 Tf 0 1 -1 0 50 20 Tm (Vert) Tj ET", false},
		{"rot180", "BT /F1 12 Tf -1 0 0 -1 100 50 Tm (Flip) Tj ET", true},
		{"rot270", "BT /F1 12 Tf 0 -1 1 0 100 80 Tm (Down) Tj ET", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := OpenDocument(buildTextPDFContent(0, "", tc.stream))
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			chars, err := doc.ExtractChars(0)
			if err != nil {
				t.Fatal(err)
			}
			if len(chars) != 4 {
				t.Fatalf("got %d chars, want 4", len(chars))
			}
			for _, c := range chars {
				if c.X1 <= c.X0 || c.Bottom <= c.Top {
					t.Errorf("%q has non-positive box: (%.2f,%.2f,%.2f,%.2f)",
						c.Text, c.X0, c.X1, c.Top, c.Bottom)
				}
				if c.X0 < -0.01 || c.X1 > 200.01 || c.Top < -0.01 || c.Bottom > 100.01 {
					t.Errorf("%q out of page bounds: (%.2f,%.2f,%.2f,%.2f)",
						c.Text, c.X0, c.X1, c.Top, c.Bottom)
				}
			}
			for i := 1; i < len(chars); i++ {
				prev, cur := chars[i-1], chars[i]
				if tc.horizontal {
					if overlap := math.Min(cur.X1, prev.X1) - math.Max(cur.X0, prev.X0); overlap > 0.01 {
						t.Errorf("%q and %q overlap horizontally by %.2f", prev.Text, cur.Text, overlap)
					}
					continue
				}
				if overlap := math.Min(cur.Bottom, prev.Bottom) - math.Max(cur.Top, prev.Top); overlap > 0.01 {
					t.Errorf("%q and %q overlap vertically by %.2f", prev.Text, cur.Text, overlap)
				}
			}
		})
	}
}
