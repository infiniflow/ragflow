//go:build cgo

package pdfium

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

// onePagePDF returns a minimal valid single-page PDF with the given MediaBox
// size in points and no content.
func onePagePDF(t *testing.T, w, h float64) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	off3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g] /Resources << >> >>\nendobj\n", w, h)
	xref := buf.Len()
	buf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n", off1, off2, off3)
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return buf.Bytes()
}

func TestRenderSize(t *testing.T) {
	cases := []struct {
		name       string
		pw, ph     float64
		dpi        float64
		wantW      int
		wantH      int
		tooLarge   bool
		wantErrAny bool
	}{
		{name: "A4 at 216 DPI", pw: 595, ph: 842, dpi: 216, wantW: 1785, wantH: 2526},
		// 5102x3968 pt sheet: ~730 MB, still renderable.
		{name: "large sheet under cap", pw: 5102, ph: 3968, dpi: 216, wantW: 15306, wantH: 11904},
		// 8504x4252 pt poster: 25512x12756x4 = 1.30 GB, over the 1 GiB cap.
		{name: "poster over cap", pw: 8504, ph: 4252, dpi: 216, tooLarge: true},
		{name: "zero width", pw: 0, ph: 842, dpi: 216, wantErrAny: true},
		{name: "zero dpi", pw: 595, ph: 842, dpi: 0, wantErrAny: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h, err := renderSize(tc.pw, tc.ph, tc.dpi)
			switch {
			case tc.tooLarge:
				if !errors.Is(err, ErrRenderTooLarge) {
					t.Fatalf("err = %v, want ErrRenderTooLarge", err)
				}
			case tc.wantErrAny:
				if err == nil {
					t.Fatalf("want error, got %dx%d", w, h)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if w != tc.wantW || h != tc.wantH {
					t.Fatalf("size = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
				}
			}
		})
	}
}

// TestRenderPage_OversizedPageReturnsError guards the production crash where
// a poster page rendered at 216 DPI needed a 1.3 GB bitmap and panicked with
// "slice bounds out of range" instead of failing the page.
func TestRenderPage_OversizedPageReturnsError(t *testing.T) {
	img, err := RenderPage(onePagePDF(t, 8504, 4252), 0, 216)
	if !errors.Is(err, ErrRenderTooLarge) {
		t.Fatalf("err = %v, want ErrRenderTooLarge", err)
	}
	if img != nil {
		t.Fatal("expected nil image on error")
	}
}

func TestRenderPage_SmallPageStillRenders(t *testing.T) {
	img, err := RenderPage(onePagePDF(t, 100, 50), 0, 144)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Fatalf("size = %dx%d, want 200x100", b.Dx(), b.Dy())
	}
	if c := img.RGBAAt(10, 10); c.R != 255 || c.G != 255 || c.B != 255 || c.A != 255 {
		t.Fatalf("empty page pixel = %v, want opaque white", c)
	}
}
