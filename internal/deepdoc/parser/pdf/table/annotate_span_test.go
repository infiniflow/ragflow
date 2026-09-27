//go:build cgo && manual

package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestAnnotateTableBoxesPropagatesSpan verifies that AnnotateTableBoxes
// transfers the TSR "table spanning cell" recognition onto the overlapping
// OCR text box as SP>0. This is the bridge that lets GroupBoxesByRC rebuild a
// grid that still carries the span, so ConstructTable/CalSpans can emit
// colspan/rowspan.
//
// Root cause for 1.pdf (real_pdfs): AnnotateTableBoxes collected `headers`
// and `clmns` from the grid but NEVER populated the `spans` slice it later
// matched boxes against, so box.SP stayed 0 and the rebuilt grid lost every
// TSR span — Go emitted 8 independent empty <th> where Python emits
// <th colspan=6>. Python's _table_transformer_job sets b["SP"] from the
// spanning cells, so Go must too.
func TestAnnotateTableBoxesPropagatesSpan(t *testing.T) {
	// A 2-row × 2-col grid where row0 col0 spans both columns (colspan=2),
	// mirroring 1.pdf's wide header. Coordinates are in the same crop space
	// as the boxes below.
	grid := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 20, Label: "table column"},
			{X0: 100, Y0: 0, X1: 200, Y1: 20, Label: "table column"},
		},
		{
			{X0: 0, Y0: 20, X1: 100, Y1: 40, Label: "table column"},
			{X0: 100, Y0: 20, X1: 200, Y1: 40, Label: "table column"},
		},
	}
	// Overlay a spanning cell across row0 (y0=0 y1=20), x0=0 x1=200.
	grid[0][1] = pdf.TSRCell{X0: 0, Y0: 0, X1: 200, Y1: 20, Label: "table spanning cell"}

	// A text box that sits inside the spanning cell region. LayoutType must
	// be "table" — AnnotateTableBoxes skips non-table boxes, and the
	// production caller (processOneTable) always passes the table region's
	// box subset with LayoutTypeTable set.
	box := pdf.TextBox{
		X0: 10, X1: 190,
		Top: 2, Bottom: 18,
		Text:       "液化石油气储罐(区)(总容积V,m²)",
		LayoutType: pdf.LayoutTypeTable,
	}

	boxes := []pdf.TextBox{box}
	AnnotateTableBoxes(boxes, grid)

	if boxes[0].SP <= 0 {
		t.Errorf("AnnotateTableBoxes did not set SP on the box overlapping the TSR spanning cell (SP=%d). "+
			"Without SP, GroupBoxesByRC rebuilds a grid with no span label and ConstructTable/CalSpans "+
			"drops the colspan — Go emits 8 independent empty <th> instead of <th colspan=6>.",
			boxes[0].SP)
	}
}
