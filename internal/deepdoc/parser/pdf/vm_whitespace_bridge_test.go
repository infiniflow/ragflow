package pdf

import (
	"sort"
	"strings"
	"testing"

	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestVMWhitespaceGapBridge reproduces the exact RAG PDF divergence
// with synthetic boxes.  A whitespace box (width > 0, gap just below
// threshold) gets merged into a content box, extending its bottom by
// the whitespace height.  This flips the next gap from reject to merge,
// creating a cascade that reduces the section count by 1.
//
// Go's whitespace pre-filter removes this box before VM, so the
// bottom extension never happens and the cascade fails to start.
//
// Moved out of the cgo && manual parity file (pipeline_parity_test.go) so it
// runs in CI: it uses only synthetic boxes and the production
// lyt.NaiveVerticalMerge, no Python dump.
func TestVMWhitespaceGapBridge(t *testing.T) {
	// Coordinates extracted from RAG PDF charspy data, "服务体系" region.
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

	mh := 9.361 // RAG PDF char median
	thr := mh * 1.5

	// Run VM with whitespace PRESENT (Python-like, no pre-filter).
	// Python's while/pop merges whitespace at b_ position into b
	// (extending b.bottom), then compares same b against next content.
	// We simulate this by letting whitespace through gap/xov checks
	// and absorbing it into prev when the checks pass.
	vWithWS := func() int {
		bxs := make([]pdf.TextBox, len(boxes))
		copy(bxs, boxes)
		sort.Slice(bxs, func(i, j int) bool {
			if bxs[i].Top != bxs[j].Top {
				return bxs[i].Top < bxs[j].Top
			}
			return bxs[i].X0 < bxs[j].X0
		})
		out := make([]pdf.TextBox, 0, len(bxs))
		for i := 0; i < len(bxs); i++ {
			b := bxs[i]
			isWS := strings.TrimSpace(b.Text) == ""
			// Whitespace in b position (current box): pop (skip).
			// In Python: bxs.pop(i); continue; i stays.
			if isWS && len(out) == 0 {
				continue // nothing to extend
			}
			if isWS && len(out) > 0 {
				prev := &out[len(out)-1]
				gap := b.Top - prev.Bottom
				ov := util.OverlapX(prev, &b)
				// Python: gap passes AND xov passes → whitespace merged
				// into prev, extending bottom.  i advances (Go for-loop).
				if gap <= thr && ov >= 0.3 {
					prev.Bottom = b.Bottom
				}
				continue
			}
			if len(out) == 0 {
				out = append(out, b)
				continue
			}
			prev := &out[len(out)-1]
			if prev.LayoutNo != b.LayoutNo {
				out = append(out, b)
				continue
			}
			gap := b.Top - prev.Bottom
			ov := util.OverlapX(prev, &b)
			if gap > thr {
				out = append(out, b)
				continue
			}
			if ov < 0.3 {
				out = append(out, b)
				continue
			}
			pt := strings.TrimSpace(prev.Text)
			bt := strings.TrimSpace(b.Text)
			prev.Text = strings.TrimSpace(strings.TrimRight(pt, " \t") + " " + strings.TrimLeft(bt, " \t"))
			prev.Bottom = b.Bottom
			if prev.X0 > b.X0 {
				prev.X0 = b.X0
			}
			if prev.X1 < b.X1 {
				prev.X1 = b.X1
			}
		}
		return len(out)
	}

	// Run VM with whitespace PRE-FILTERED (Go current behavior).
	vNoWS := func() int {
		bxs := make([]pdf.TextBox, 0, len(boxes))
		for _, b := range boxes {
			if strings.TrimSpace(b.Text) != "" {
				bxs = append(bxs, b)
			}
		}
		sort.Slice(bxs, func(i, j int) bool {
			if bxs[i].Top != bxs[j].Top {
				return bxs[i].Top < bxs[j].Top
			}
			return bxs[i].X0 < bxs[j].X0
		})
		out := make([]pdf.TextBox, 0, len(bxs))
		for i := 0; i < len(bxs); i++ {
			b := bxs[i]
			if len(out) == 0 {
				out = append(out, b)
				continue
			}
			prev := &out[len(out)-1]
			if prev.LayoutNo != b.LayoutNo {
				out = append(out, b)
				continue
			}
			gap := b.Top - prev.Bottom
			ov := util.OverlapX(prev, &b)
			if gap > thr {
				out = append(out, b)
				continue
			}
			if ov < 0.3 {
				out = append(out, b)
				continue
			}
			pt := strings.TrimSpace(prev.Text)
			bt := strings.TrimSpace(b.Text)
			prev.Text = strings.TrimSpace(strings.TrimRight(pt, " \t") + " " + strings.TrimLeft(bt, " \t"))
			prev.Bottom = b.Bottom
			if prev.X0 > b.X0 {
				prev.X0 = b.X0
			}
			if prev.X1 < b.X1 {
				prev.X1 = b.X1
			}
		}
		return len(out)
	}

	nWS := vWithWS()
	nNoWS := vNoWS()
	t.Logf("With whitespace (Python-like): %d sections", nWS)
	t.Logf("Without whitespace (Go pre-filter): %d sections", nNoWS)
	t.Logf("Gap without bridge: 420.16 - 382.39 = %.2f > %.2f = REJECT", 420.16-382.39, thr)
	t.Logf("Gap with bridge:    420.16 - 406.79 = %.2f < %.2f = MERGE", 420.16-406.79, thr)

	// The manual vWithWS (Python-like) and vNoWS (old Go pre-filter) still
	// differ — the mechanism is real.  But production lyt.NaiveVerticalMerge now
	// handles whitespace inline (gap bridge), matching Python.
	if nWS == nNoWS {
		t.Error("Manual implementations should differ — the gap bridge mechanism is real")
	}

	// Verify production lyt.NaiveVerticalMerge matches vWithWS (Python behavior).
	mhMap := map[int]float64{1: mh}
	mwMap := map[int]float64{1: 5}
	vmResult := lyt.NaiveVerticalMerge(boxes, mhMap, mwMap, nil)
	t.Logf("lyt.NaiveVerticalMerge (production): %d sections", len(vmResult))
	if len(vmResult) != nWS {
		t.Errorf("lyt.NaiveVerticalMerge produced %d sections, want %d (Python-like with gap bridge)", len(vmResult), nWS)
	}
}
