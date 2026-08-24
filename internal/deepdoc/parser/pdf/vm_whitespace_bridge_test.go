package pdf

import (
	"testing"

	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestVMWhitespaceGapBridge locks production lyt.NaiveVerticalMerge's inline
// whitespace gap bridge on the exact RAG PDF divergence:
//
//	A (339.35-382.39)  content
//	WS (396.39-406.79) " " U+00A0 non-breaking space, non-zero width
//	B  (420.16-431.19) content
//	C  (436.16-447.20) content
//
// mh=9.361 so the merge threshold is mh*1.5=14.04. The whitespace box sits
// 14.00 below A — just under the threshold — so mergeColumnBoxes bridges it
// into A (extending A.Bottom to 406.79, OverlapX(A,WS)=1.0). That flips B's
// gap (420.16-406.79=13.37 < 14.04) from reject to merge, then C cascades
// (gap 4.97): the whole column collapses to ONE section. Without the bridge
// (whitespace ignored) A→B gap is 37.77 > 14.04 and B→C still merge, yielding
// two sections — so the count 1 specifically proves the bridge fired.
//
// The literal count is asserted directly (no hand-written replica of the merge
// algorithm) so the test locks production behavior itself, not
// production-vs-a-copy. Moved out of the cgo && manual parity file so it runs
// in CI: it uses only synthetic boxes and the production merge, no Python dump.
func TestVMWhitespaceGapBridge(t *testing.T) {
	boxes := []pdf.TextBox{
		// Content A: merged result of 3 preceding lines
		{X0: 37.6, X1: 491.0, Top: 339.35, Bottom: 382.39,
			Text: "生成文本再用standard分词建立索引", PageNumber: 1},
		// Whitespace: U+00A0 non-breaking space, has non-zero width
		{X0: 37.6, X1: 40.3, Top: 396.39, Bottom: 406.79,
			Text: " ", PageNumber: 1},
		// Content B: would be rejected without whitespace gap bridge
		{X0: 37.6, X1: 543.3, Top: 420.16, Bottom: 431.19,
			Text: "直接用rag分词建立索引", PageNumber: 1},
		// Content C: cascades after B merges
		{X0: 37.6, X1: 526.4, Top: 436.16, Bottom: 447.20,
			Text: "是在原文中并没有这样的文字", PageNumber: 1},
	}

	mhMap := map[int]float64{1: 9.361}
	mwMap := map[int]float64{1: 5}
	vmResult := lyt.NaiveVerticalMerge(boxes, mhMap, mwMap, nil)
	if len(vmResult) != 1 {
		t.Fatalf("whitespace gap bridge must collapse the 4-box column to 1 section, got %d", len(vmResult))
	}
	if vmResult[0].Bottom < 447.20 {
		t.Fatalf("bridged box must cascade-merge content C (bottom 447.20), got bottom %.2f", vmResult[0].Bottom)
	}
}
