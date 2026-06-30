package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestCellToPageSpace(t *testing.T) {
	cell := pdf.TSRCell{X0: 100, Y0: 200, X1: 300, Y1: 400, Text: "hello", Label: "table"}
	got := CellToPageSpace(cell, 15, 25, 3.0)

	// (100+15)/3 = 38.33..., (200+25)/3 = 75
	if got.X0 != 38.333333333333336 || got.Y0 != 75 || got.X1 != 105 || got.Y1 != 141.66666666666666 {
		t.Errorf("cellToPageSpace: got (%f,%f,%f,%f), want (38.33,75,105,141.67)", got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.Text != "hello" || got.Label != "table" {
		t.Error("cellToPageSpace should preserve Text and Label")
	}
}

func TestCellAddOffset(t *testing.T) {
	cell := pdf.TSRCell{X0: 100, Y0: 200, X1: 300, Y1: 400, Text: "hello"}
	got := CellAddOffset(cell, 15, 25)
	if got.X0 != 115 || got.Y0 != 225 || got.X1 != 315 || got.Y1 != 425 {
		t.Errorf("cellAddOffset: got (%f,%f,%f,%f)", got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.Text != "hello" {
		t.Error("cellAddOffset should preserve Text")
	}
}

func TestBoxToCropSpace(t *testing.T) {
	box := pdf.TextBox{X0: 50, X1: 200, Top: 100, Bottom: 300, Text: "text"}
	got := BoxToCropSpace(box, 3.0, 10, 20)
	if got.X0 != 140 || got.Top != 280 || got.X1 != 590 || got.Bottom != 880 {
		t.Errorf("boxToCropSpace: got (%f,%f,%f,%f)", got.X0, got.Top, got.X1, got.Bottom)
	}
	if got.Text != "text" {
		t.Error("boxToCropSpace should preserve Text")
	}
}

func TestCopyBoxAnnotations(t *testing.T) {
	src := &pdf.TextBox{R: 1, C: 2, RTop: 10, RBott: 20, H: 3, HTop: 30, HBott: 40,
		HLeft: 50, HRight: 60, CLeft: 70, CRight: 80, SP: 4}
	dst := &pdf.TextBox{}
	CopyBoxAnnotations(dst, src)
	if dst.R != 1 || dst.C != 2 || dst.RTop != 10 || dst.RBott != 20 {
		t.Error("R/C fields not copied")
	}
	if dst.H != 3 || dst.HTop != 30 || dst.HBott != 40 {
		t.Error("H fields not copied")
	}
	if dst.HLeft != 50 || dst.HRight != 60 || dst.CLeft != 70 || dst.CRight != 80 {
		t.Error("spanning fields not copied")
	}
	if dst.SP != 4 {
		t.Error("SP not copied")
	}
}

func TestCellSliceToPageSpace(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 100, Y0: 200, X1: 300, Y1: 400},
		{X0: 400, Y0: 200, X1: 600, Y1: 400},
	}
	got := CellSliceToPageSpace(cells, 15, 25, 3)
	if len(got) != 2 {
		t.Fatal("expected 2 cells")
	}
	if got[0].X0 != 38.333333333333336 || got[1].X0 != 138.33333333333334 {
		t.Error("wrong conversion")
	}
}
