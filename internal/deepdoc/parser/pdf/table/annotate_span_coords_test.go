//go:build cgo && manual

package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestAnnotateTableBoxesSpanCarriesBoxCoords verifies that AnnotateTableBoxes
// copies the spanning cell's bbox (H_left/H_right/H_top/H_bott) onto the
// overlapping box, matching Python's _annotate_table_boxes
// (pdf_parser.py:632-635: b["H_left"]=spans[ii]["x0"], b["H_right"]=spans[ii]["x1"]).
//
// This is what lets GroupBoxesByRC rebuild a grid cell whose X0/X1 equals the
// span box's full extent, so CalSpans covers every column the span crosses.
// Without it, cellPosFromBox (group_boxes.go) falls back to the box's own
// narrow bounds, CalSpans under-covers by one column, and Go emits
// colspan=5 where Python emits colspan=6 (real_pdfs/1.pdf). TSR input is
// identical on both sides, so the divergence is a Go assembly bug, not a
// model issue.
func TestAnnotateTableBoxesSpanCarriesBoxCoords(t *testing.T) {
	// grid[0][1] is a spanning cell spanning columns 1..6 (x0..x1 matches
	// 1.pdf's colspan=6 header region). Coordinates are in the same crop
	// space as the box below.
	grid := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 20, Label: "table column"},
			{X0: 100, Y0: 0, X1: 500, Y1: 20, Label: "table spanning cell"},
			{X0: 500, Y0: 0, X1: 600, Y1: 20, Label: "table column"},
		},
		{
			{X0: 0, Y0: 20, X1: 100, Y1: 40, Label: "table column"},
			{X0: 100, Y0: 20, X1: 500, Y1: 40, Label: "table column"},
			{X0: 500, Y0: 20, X1: 600, Y1: 40, Label: "table column"},
		},
	}
	// A box that sits under the spanning cell (its own x0/x1 are narrow,
	// like the "名称" column in 1.pdf — only ~1 column wide).
	box := pdf.TextBox{
		X0: 110, X1: 200,
		Top: 2, Bottom: 18,
		Text:       "液化石油气储罐(区)(总容积V,m²)",
		LayoutType: pdf.LayoutTypeTable,
	}

	boxes := []pdf.TextBox{box}
	AnnotateTableBoxes(boxes, grid)

	b := boxes[0]
	if b.HLeft != 100 || b.HRight != 500 {
		t.Errorf("span box H_left/H_right not propagated from spanning cell bbox: got HLeft=%v HRight=%v, want 100/500. "+
			"This makes GroupBoxesByRC build a span cell with the box's narrow bounds, so CalSpans emits colspan=5 instead of colspan=6 (1.pdf).",
			b.HLeft, b.HRight)
	}
	if b.SP <= 0 {
		t.Errorf("SP not set on span-matching box: SP=%d", b.SP)
	}
}
