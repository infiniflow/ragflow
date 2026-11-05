package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// vhDispatchGrid is a trivial 2x2 table whose OSS renderer (constructTableCore)
// produces non-empty HTML. It is used to prove the RenderVH dispatch branch is
// taken and does not silently fall through to the OSS renderer.
func vhDispatchGrid() [][]pdf.TSRCell {
	return [][]pdf.TSRCell{
		{
			{Text: "A", X0: 0, Y0: 0, X1: 10, Y1: 10},
			{Text: "B", X0: 10, Y0: 0, X1: 20, Y1: 10},
		},
		{
			{Text: "C", X0: 0, Y0: 10, X1: 10, Y1: 20},
			{Text: "D", X0: 10, Y0: 10, X1: 20, Y1: 20},
		},
	}
}

// TestConstructTableRenderVHFalseDelegatesToCore locks that the default
// (RenderVH == false) path is exactly the OSS dense-grid renderer and keeps
// its existing behavior, including the cell texts.
func TestConstructTableRenderVHFalseDelegatesToCore(t *testing.T) {
	item := &pdf.TableItem{Grid: vhDispatchGrid()}
	got := ConstructTable(nil, nil, "", item)
	if got == "" {
		t.Fatalf("RenderVH=false: expected non-empty HTML, got empty")
	}
	for _, want := range []string{"A", "B", "C", "D"} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderVH=false: output missing %q\n%s", want, got)
		}
	}
	core := constructTableCore(nil, nil, "", &pdf.TableItem{Grid: vhDispatchGrid()})
	if got != core {
		t.Errorf("RenderVH=false did not delegate to constructTableCore:\n got=%s\ncore=%s", got, core)
	}
}

// TestConstructTableRenderVHTrueRoutesToVH is the adversarial dispatch check.
// A non-empty table whose OSS renderer WOULD emit HTML must yield "" when
// RenderVH is set, because in this build ConstructTableVH is the !ee stub.
// If the dispatch were missing, ConstructTable would return the non-empty
// OSS HTML and this test would fail.
func TestConstructTableRenderVHTrueRoutesToVH(t *testing.T) {
	item := &pdf.TableItem{Grid: vhDispatchGrid(), RenderVH: true}
	got := ConstructTable(nil, nil, "", item)
	if got != "" {
		t.Fatalf("RenderVH=true must route to ConstructTableVH; expected empty stub output, got %q", got)
	}
}

// TestConstructTableVHStubReturnsEmpty guards the OSS/community stub contract:
// the shared ConstructTable must reference ConstructTableVH so both the ee and
// !ee builds compile; the !ee build's stub returns empty.
func TestConstructTableVHStubReturnsEmpty(t *testing.T) {
	if got := ConstructTableVH(nil, nil, ""); got != "" {
		t.Errorf("OSS ConstructTableVH stub must return empty; got %q", got)
	}
}

// TestTableItemRenderVHZeroByDefault ensures the new field is a no-op signal
// for the OSS pipeline (nothing sets it).
func TestTableItemRenderVHZeroByDefault(t *testing.T) {
	var it pdf.TableItem
	if it.RenderVH {
		t.Errorf("RenderVH should default to false")
	}
}

// TestTSRCellSpanFieldsZeroByDefault ensures the V/H assembler span fields are
// a zero-value extension of the shared cell type (OSS never sets them).
func TestTSRCellSpanFieldsZeroByDefault(t *testing.T) {
	var c pdf.TSRCell
	if c.ColSpan != 0 || c.RowSpan != 0 {
		t.Errorf("ColSpan/RowSpan should default to 0, got %d/%d", c.ColSpan, c.RowSpan)
	}
}
