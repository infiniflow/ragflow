package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Compile-time guarantees that every TableBuilder implemented inside the
// table package satisfies the interface after the FinalizeGrid /
// AttachSpanInfo extension. If any implementer misses a method, this file
// fails to compile (RED) and the build stays red until GREEN.
var (
	_ pdf.TableBuilder = (*DeepDocTableBuilder)(nil)
	_ pdf.TableBuilder = (*MockTableBuilder)(nil)
)

// TestDeepDocTableBuilderFinalizeGridIdentity verifies the OSS builder is
// the identity function: it returns the annotated grid unchanged and never
// allocates for a nil input.
func TestDeepDocTableBuilderFinalizeGridIdentity(t *testing.T) {
	b := NewDeepDocTableBuilder(nil)

	annotGrid := [][]pdf.TSRCell{
		{{Text: "a"}},
		{{Text: "b"}},
	}
	got := b.FinalizeGrid(annotGrid, nil)
	if len(got) != len(annotGrid) {
		t.Fatalf("FinalizeGrid changed row count: got %d want %d", len(got), len(annotGrid))
	}
	for r := range annotGrid {
		if len(got[r]) != len(annotGrid[r]) {
			t.Fatalf("row %d column count changed", r)
		}
		if got[r][0].Text != annotGrid[r][0].Text {
			t.Errorf("row %d text mutated: got %q want %q", r, got[r][0].Text, annotGrid[r][0].Text)
		}
	}

	// nil grid must round-trip as nil, not a zero-value allocation.
	if out := b.FinalizeGrid(nil, nil); out != nil {
		t.Errorf("FinalizeGrid(nil) = %#v, want nil", out)
	}
}

// TestDeepDocTableBuilderAttachSpanInfoNoop verifies the OSS builder's
// AttachSpanInfo is a safe no-op: it never panics (even on a nil item) and
// never mutates the item.
func TestDeepDocTableBuilderAttachSpanInfoNoop(t *testing.T) {
	b := NewDeepDocTableBuilder(nil)

	// nil item must not panic.
	b.AttachSpanInfo(nil)

	item := &pdf.TableItem{Rows: [][]string{{"x"}}}
	b.AttachSpanInfo(item)
	if len(item.Rows) != 1 || item.Rows[0][0] != "x" {
		t.Errorf("AttachSpanInfo mutated the item: %+v", item.Rows)
	}
}
