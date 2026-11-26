//go:build cgo

package pdfium

import (
	"bytes"
	"fmt"
	"math"
	"testing"
)

func buildVectorPDF(rotate int, crop, stream string) []byte {
	return buildTextPDFContent(rotate, crop, stream)
}

func buildFormVectorPDF(formStream, pageStream string) []byte {
	var buf bytes.Buffer
	var offsets []int
	writeObj := func(s string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(s)
	}
	buf.WriteString("%PDF-1.4\n")
	writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	writeObj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	writeObj("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] " +
		"/Resources << /Font << /F1 4 0 R >> /XObject << /Fm1 6 0 R >> >> /Contents 5 0 R >>\nendobj\n")
	writeObj("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	writeObj(fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(pageStream), pageStream))
	writeObj(fmt.Sprintf("6 0 obj\n<< /Type /XObject /Subtype /Form /BBox [0 0 200 100] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(formStream), formStream))
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes()
}

func TestExtractPageVectorLayoutWordsAndPaths(t *testing.T) {
	// Page is 200x100 (PDF bottom-left origin):
	// - Invisible clipping rect (should be ignored)
	// - Filled box at x=[20,80], y=[40,70] -> display top=[30,60]
	// - Open L-shaped stroke connector from (80,55) -> (120,55) -> (120,25)
	//   -> display (80,45) -> (120,45) -> (120,75)
	// - Filled triangle arrowhead pointing down at (120,20)
	// - Horizontal text "Step A" inside box and 90-degree vertical text "Lane1"
	stream := "" +
		"0 0 200 100 re W n\n" +
		"20 40 60 30 re B\n" +
		"1.5 w 80 55 m 120 55 l 120 25 l S\n" +
		"116 25 m 124 25 l 120 20 l h f\n" +
		"BT /F1 10 Tf 25 52 Td (Step A) Tj ET\n" +
		"BT /F1 10 Tf 0 1 -1 0 12 20 Tm (Lane1) Tj ET\n"

	doc, err := OpenDocument(buildVectorPDF(0, "", stream))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	defer doc.Close()

	geom, words, paths, err := doc.ExtractPageVectorLayout(0)
	if err != nil {
		t.Fatalf("ExtractPageVectorLayout: %v", err)
	}
	if geom.Width != 200 || geom.Height != 100 {
		t.Fatalf("geom = %+v, want 200x100", geom)
	}

	// Words: "Step", "A", "Lane1"
	var foundStep, foundLane bool
	for _, w := range words {
		if w.Text == "Step" {
			foundStep = true
			if math.Abs(w.RotationDegrees) > 5 {
				t.Errorf("Step rotation = %.1f, want ~0", w.RotationDegrees)
			}
			if w.X0 < 20 || w.X1 > 80 || w.Top < 30 || w.Bottom > 60 {
				t.Errorf("Step box %+v outside filled rect [20,80]x[30,60]", w)
			}
		}
		if w.Text == "Lane1" {
			foundLane = true
			if math.Abs(math.Abs(w.RotationDegrees)-90) > 5 {
				t.Errorf("Lane1 rotation = %.1f, want +/-90", w.RotationDegrees)
			}
		}
	}
	if !foundStep || !foundLane {
		t.Fatalf("words = %+v, want Step and Lane1", words)
	}

	// Paths: 3 visible paths (clipping path must be excluded).
	if len(paths) != 3 {
		t.Fatalf("paths = %d, want 3 (clipping path excluded); paths=%+v", len(paths), paths)
	}

	var foundBox, foundConnector, foundArrow bool
	for _, p := range paths {
		if p.HasFill && p.HasStroke && math.Abs(p.X0-20) < 0.5 && math.Abs(p.X1-80) < 0.5 && math.Abs(p.Top-30) < 0.5 && math.Abs(p.Bottom-60) < 0.5 {
			foundBox = true
		}
		if p.HasStroke && !p.HasFill && len(p.Segments) == 3 {
			start := p.Segments[0].Point
			end := p.Segments[len(p.Segments)-1].Point
			if math.Abs(start.X-80) < 0.5 && math.Abs(start.Y-45) < 0.5 &&
				math.Abs(end.X-120) < 0.5 && math.Abs(end.Y-75) < 0.5 {
				foundConnector = true
				if math.Abs(p.StrokeWidth-1.5) > 0.1 {
					t.Errorf("connector strokeWidth = %.2f, want 1.5", p.StrokeWidth)
				}
			}
		}
		if p.HasFill && !p.HasStroke && len(p.Segments) >= 3 && math.Abs(p.X1-p.X0-8) < 0.5 && math.Abs(p.Bottom-p.Top-5) < 0.5 {
			foundArrow = true
		}
	}
	if !foundBox || !foundConnector || !foundArrow {
		t.Fatalf("missing expected paths: box=%v connector=%v arrow=%v paths=%+v", foundBox, foundConnector, foundArrow, paths)
	}
}

func TestExtractPageVectorLayoutRotationAndCropBox(t *testing.T) {
	// MediaBox [0 0 200 100], CropBox [20 10 180 90] (160x80), Rotate 90 CW -> display 80x160.
	// Draw a line from (30, 80) to (30, 20) in PDF user space:
	// CropBox-relative unrotated: rx=10, ry=10 to rx=10, ry=70.
	// After 90 CW (ch=80): x = ch - ry, y = rx -> (70, 10) to (10, 10).
	stream := "30 80 m 30 20 l S\nBT /F1 10 Tf 40 50 Td (Hi) Tj ET\n"
	doc, err := OpenDocument(buildVectorPDF(90, "/CropBox [20 10 180 90]", stream))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	defer doc.Close()

	geom, words, paths, err := doc.ExtractPageVectorLayout(0)
	if err != nil {
		t.Fatalf("ExtractPageVectorLayout: %v", err)
	}
	if geom.Width != 80 || geom.Height != 160 {
		t.Fatalf("geom = %+v, want 80x160", geom)
	}
	if len(paths) != 1 || len(paths[0].Segments) != 2 {
		t.Fatalf("paths = %+v, want 1 two-segment line", paths)
	}
	s0, s1 := paths[0].Segments[0].Point, paths[0].Segments[1].Point
	if math.Abs(s0.X-70) > 0.5 || math.Abs(s0.Y-10) > 0.5 || math.Abs(s1.X-10) > 0.5 || math.Abs(s1.Y-10) > 0.5 {
		t.Fatalf("rotated line endpoints = %+v -> %+v, want (70,10)->(10,10)", s0, s1)
	}
	if len(words) != 1 || words[0].Text != "Hi" {
		t.Fatalf("words = %+v, want [Hi]", words)
	}
	if math.Abs(words[0].RotationDegrees+90) > 5 {
		t.Errorf("rotated word angle = %.1f, want -90", words[0].RotationDegrees)
	}
}

func TestExtractPageVectorLayoutFormXObject(t *testing.T) {
	// Form XObject draws a line (0,0)->(20,0); page translates it by (50,30).
	// User space: (50,30)->(70,30) -> display (50,70)->(70,70).
	data := buildFormVectorPDF("0 0 m 20 0 l S\n", "q 1 0 0 1 50 30 cm /Fm1 Do Q\n")
	doc, err := OpenDocument(data)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	defer doc.Close()

	_, _, paths, err := doc.ExtractPageVectorLayout(0)
	if err != nil {
		t.Fatalf("ExtractPageVectorLayout: %v", err)
	}
	if len(paths) != 1 || len(paths[0].Segments) != 2 {
		t.Fatalf("paths = %+v, want 1 transformed path from Form XObject", paths)
	}
	s0, s1 := paths[0].Segments[0].Point, paths[0].Segments[1].Point
	if math.Abs(s0.X-50) > 0.5 || math.Abs(s0.Y-70) > 0.5 || math.Abs(s1.X-70) > 0.5 || math.Abs(s1.Y-70) > 0.5 {
		t.Fatalf("form path endpoints = %+v -> %+v, want (50,70)->(70,70)", s0, s1)
	}
}
