package pdf

import (
	"image"
	"testing"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// spyTableBuilder wraps staticTableBuilder and records whether the
// post-grouping extension points (FinalizeGrid / AttachSpanInfo) were
// invoked by processOneTable, and with which arguments. It deliberately
// returns a distinguishable grid / mutates the item so the test can prove
// the caller actually uses the return value and the very item it returns.
type spyTableBuilder struct {
	staticTableBuilder
	finalizeCalled bool
	finalizeInGrid [][]pdf.TSRCell
	finalizeBoxes  int
	attachCalled   bool
}

// FinalizeGrid records the call and returns a sentinel grid so the test can
// prove processOneTable stores THIS return value as the table grid.
func (b *spyTableBuilder) FinalizeGrid(annotGrid [][]pdf.TSRCell, boxes []pdf.TextBox) [][]pdf.TSRCell {
	b.finalizeCalled = true
	b.finalizeInGrid = annotGrid
	b.finalizeBoxes = len(boxes)
	return [][]pdf.TSRCell{{pdf.TSRCell{Text: "FINALIZED"}}}
}

// AttachSpanInfo records the call and mutates the item so the test can prove
// the mutation survives into the item processOneTable returns.
func (b *spyTableBuilder) AttachSpanInfo(item *pdf.TableItem) {
	b.attachCalled = true
	item.Rows = append(item.Rows, []string{"ATTACHED"})
}

// TestProcessOneTableWiresFinalizeGridAndAttachSpanInfo asserts the extension
// points added to TableBuilder are actually invoked by the shared table
// assembly path. Without the call site, the production DeepDoc builder keeps
// its identity/no-op behavior but the interface methods are dead declarations
// in OSS — this test makes that gap falsifiable.
func TestProcessOneTableWiresFinalizeGridAndAttachSpanInfo(t *testing.T) {
	autoRotate := false
	cfg := pdf.DefaultParserConfig()
	cfg.AutoRotateTables = &autoRotate
	p := NewParser(cfg)

	pageImg := image.NewRGBA(image.Rect(0, 0, 320, 220))
	boxes := []pdf.TextBox{
		{X0: 10, X1: 60, Top: 10, Bottom: 30, Text: "cell", LayoutType: pdf.LayoutTypeTable},
	}
	match := tbl.TableMatch{
		Region: pdf.DLARegion{X0: 10, Y0: 10, X1: 210, Y1: 110, Label: pdf.LayoutTypeTable},
		BoxIdx: []int{0},
	}
	inner := &staticTableBuilder{
		cells: []pdf.TSRCell{{X0: 10, Y0: 20, X1: 60, Y1: 80, Label: "table row"}},
	}
	spy := &spyTableBuilder{staticTableBuilder: *inner}

	item := p.processOneTable(t.Context(), pageImg, boxes, 0, &orientationScoringDoc{}, spy, match, pdf.DlaScale)

	// Call site 1: FinalizeGrid must be invoked with the assembled grid + boxes.
	if !spy.finalizeCalled {
		t.Fatalf("processOneTable did not call TableBuilder.FinalizeGrid")
	}
	if spy.finalizeBoxes == 0 {
		t.Fatalf("FinalizeGrid received 0 boxes; expected the table's crop-space boxes")
	}
	// The assembled grid (pre-FinalizeGrid) must be the one handed to FinalizeGrid.
	if len(spy.finalizeInGrid) == 0 {
		t.Fatalf("FinalizeGrid received an empty grid; expected the assembled grid")
	}
	// The caller must store FinalizeGrid's RETURN value as the item grid, not
	// the pre-FinalizeGrid grid.
	if len(item.Grid) != 1 || len(item.Grid[0]) != 1 || item.Grid[0][0].Text != "FINALIZED" {
		t.Fatalf("item.Grid was not produced by FinalizeGrid: %#v", item.Grid)
	}

	// Call site 2: AttachSpanInfo must be invoked with the assembled item.
	if !spy.attachCalled {
		t.Fatalf("processOneTable did not call TableBuilder.AttachSpanInfo")
	}
	// The mutation to the item inside AttachSpanInfo must surface in the
	// returned item. This is the falsifiable proof that AttachSpanInfo was
	// called on the item processOneTable actually returns (not a throwaway or
	// a pre-copy), since TableItem is a value type copied on return.
	if len(item.Rows) != 1 || item.Rows[0][0] != "ATTACHED" {
		t.Fatalf("AttachSpanInfo did not mutate the assembled item: %#v", item.Rows)
	}
}
