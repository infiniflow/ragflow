package infnative

import (
	"testing"

	"ragflow/internal/deepdoc/native"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// TestTSRCellsFnOverridable guards two contracts of the TSR mapping seam:
//
//  1. tsrCellsFn is a reassignable package-level variable (the override seam);
//     a downstream build variant can swap the mapping at init/startup.
//  2. The default mapping clamps out-of-bounds coordinates into the source
//     image bounds [0, W] x [0, H] — parity-critical behaviour that must hold
//     without loading a model.
func TestTSRCellsFnOverridable(t *testing.T) {
	// (1) The seam must be reassignable. Swap in a stub, confirm it is invoked,
	// then restore the default so other tests see the real mapping.
	saved := tsrCellsFn
	defer func() { tsrCellsFn = saved }()

	stubCalled := false
	tsrCellsFn = func(res native.TSRResult) []deepdoctype.TSRCell {
		stubCalled = true
		return nil
	}
	if tsrCellsFn == nil {
		t.Fatal("tsrCellsFn must remain assignable")
	}
	_ = tsrCellsFn(native.TSRResult{})
	if !stubCalled {
		t.Fatal("reassigned tsrCellsFn was not invoked")
	}

	// (2) The default mapping clamps each coordinate independently into bounds.
	W, H := 100, 200
	res := native.TSRResult{
		W: W, H: H,
		Boxes: []native.TSRBox{{
			Label:  "table",
			Score:  0.9,
			X0:     -5,
			X1:     float32(W) + 10,
			Top:    -3,
			Bottom: float32(H) + 7,
		}},
	}
	cells := saved(res)
	if len(cells) != 1 {
		t.Fatalf("expected 1 cell from default mapping, got %d", len(cells))
	}
	c := cells[0]
	if c.X0 != 0 || c.Y0 != 0 {
		t.Errorf("low coordinates must clamp to 0, got X0=%v Y0=%v", c.X0, c.Y0)
	}
	if c.X1 != float64(W) || c.Y1 != float64(H) {
		t.Errorf("high coordinates must clamp to bounds, got X1=%v Y1=%v", c.X1, c.Y1)
	}
	if c.Label != "table" {
		t.Errorf("unexpected label %q", c.Label)
	}
}
