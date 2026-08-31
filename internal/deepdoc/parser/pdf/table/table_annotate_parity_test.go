// Tests pinning the Python-equivalent behavior of AnnotateBoxLayouts.
//
// These assert the layout-annotation semantics that Python's
// LayoutRecognizer.__call__ (deepdoc/vision/layout_recognizer.py:68) produces,
// and that the Go implementation now replicates (GREEN):
//
//   - #1 layouts_cleanup: Python de-dupes overlapping same-type regions
//     (recognizer.py:124-160, called at layout_recognizer.py:100) BEFORE
//     annotation. Go matches via cleanupLayouts, so no extra synthetic
//     figure/equation boxes are emitted.
//   - #2 sort_Y_firstly: Python sorts regions top-to-bottom (recognizer.py:54,
//     layout_recognizer.py:99) before numbering them, so layoutno indices
//     follow reading order. Go matches via sortYFirstly.
//   - #3 synthetic namespace: Python numbers unmatched figure/equation regions
//     in SEPARATE counters -> "figure-N" / "equation-N"
//     (layout_recognizer.py:145-155). Go matches via a per-type typeIndex
//     keyed by the original type label.
//
// These are pure-Go unit tests (no model server, no external service): they
// feed crafted pdf.DLARegion slices and assert the annotated pdf.TextBox
// output. Coordinates use scale=1 so region pixels == PDF space.

package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// countFigureBoxes returns the number of boxes whose LayoutType is "figure".
func countFigureBoxes(boxes []pdf.TextBox) int {
	n := 0
	for _, b := range boxes {
		if b.LayoutType == pdf.LayoutTypeFigure {
			n++
		}
	}
	return n
}

// countSyntheticFigures returns the number of figure boxes with no text
// (the synthetic placeholders AnnotateBoxLayouts appends for unvisited
// figure/equation regions).
func countSyntheticFigures(boxes []pdf.TextBox) int {
	n := 0
	for _, b := range boxes {
		if b.LayoutType == pdf.LayoutTypeFigure && b.Text == "" {
			n++
		}
	}
	return n
}

// TestAnnotateBoxLayouts_DuplicateFigureRegions_Merged pins #1: two heavily
// overlapping same-type (figure) regions must be treated as ONE, so only one
// figure box (the annotated text box) results and NO synthetic figure box is
// produced. Python merges via layouts_cleanup; Go now mirrors that by fusing
// the overlapping pair into one region, so the duplicate synthetic figure is
// avoided.
func TestAnnotateBoxLayouts_DuplicateFigureRegions_Merged(t *testing.T) {
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "Some caption text", PageNumber: 0}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeFigure},
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Confidence: 0.8, Label: pdf.LayoutTypeFigure}, // identical -> IoU 1.0
	}

	out := AnnotateBoxLayouts([]pdf.TextBox{box}, regions, 1.0, 100.0)

	if got := countFigureBoxes(out); got != 1 {
		t.Errorf("#1 layouts_cleanup: expected 1 figure box after de-duplicating overlapping figure regions, got %d", got)
	}
	if got := countSyntheticFigures(out); got != 0 {
		t.Errorf("#1 layouts_cleanup: expected 0 synthetic figure boxes (duplicate region must be merged), got %d", got)
	}
}

// TestAnnotateBoxLayouts_SameTypeOutOfYOrder_LayoutNoFollowsY pins #2: when
// same-type regions arrive in non-reading order (here the bottom region is
// first in the wire list), the layoutno index must still follow top-to-bottom
// order, exactly like Python's sort_Y_firstly. The box unambiguously overlaps
// only the bottom region, so the winner is the same in both languages; only
// the assigned index differs.
func TestAnnotateBoxLayouts_SameTypeOutOfYOrder_LayoutNoFollowsY(t *testing.T) {
	// Wire order: bottom region first (NOT top-to-bottom).
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 30, X1: 100, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeTitle}, // bottom strip
		{X0: 0, Y0: 0, X1: 100, Y1: 20, Confidence: 0.9, Label: pdf.LayoutTypeTitle},  // top strip
	}
	// Box overlaps only the bottom region -> unambiguous winner = bottom region.
	box := pdf.TextBox{X0: 0, X1: 100, Top: 30, Bottom: 50, Text: "Title text", PageNumber: 0}

	out := AnnotateBoxLayouts([]pdf.TextBox{box}, regions, 1.0, 100.0)

	var annotated *pdf.TextBox
	for i := range out {
		if out[i].Text == "Title text" {
			annotated = &out[i]
		}
	}
	if annotated == nil {
		t.Fatal("#2 sort_Y_firstly: text box was not annotated as title")
	}
	// Python Y-sorts -> [top(ii=0), bottom(ii=1)] -> winner bottom => "title-1".
	if annotated.LayoutNo != "title-1" {
		t.Errorf("#2 sort_Y_firstly: expected layoutno \"title-1\" (top-to-bottom numbering), got %q", annotated.LayoutNo)
	}
}

// TestAnnotateBoxLayouts_SyntheticFigureEquation_SeparateNamespaces pins #3:
// an unmatched figure region and an unmatched equation region must yield
// distinct synthetic layoutnos "figure-0" and "equation-0". Go now keeps
// per-type synthetic counters so figures and equations are numbered
// independently instead of sharing a single figure-N counter.
func TestAnnotateBoxLayouts_SyntheticFigureEquation_SeparateNamespaces(t *testing.T) {
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 50, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeFigure},
		{X0: 60, Y0: 0, X1: 110, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeEquation},
	}

	out := AnnotateBoxLayouts([]pdf.TextBox{}, regions, 1.0, 100.0)

	layoutNos := map[string]bool{}
	for _, b := range out {
		layoutNos[b.LayoutNo] = true
	}
	if !layoutNos["figure-0"] {
		t.Errorf("#3 synthetic namespace: expected a synthetic box with layoutno \"figure-0\"")
	}
	if !layoutNos["equation-0"] {
		t.Errorf("#3 synthetic namespace: expected a synthetic box with layoutno \"equation-0\" (separate counter from figure); equation was folded into the shared figure counter")
	}
}

// TestAnnotateBoxLayouts_TieBreakRegionCoverage pins #4: when two same-type
// regions cover the box by the SAME fraction (ov tie), Python's
// find_overlapped_with_threshold (recognizer.py:255-269) breaks the tie by the
// region-coverage ratio (_ov = box∩region / region area), preferring the region
// the box sits more "inside" of. Go mirrors the (ov, _ov) tuple comparison so
// the higher-_ov region wins the tie instead of the first Y-sorted candidate.
//
// Layout: wide top region A and narrow bottom region B. The box spans both
// vertically with EQUAL absolute intersection, so ov_A == ov_B. B is smaller,
// so _ov_B > _ov_A and Python picks B. In Y-sorted order [A, B], B is per-type
// index 1, so the expected layoutno is "table-1".
func TestAnnotateBoxLayouts_TieBreakRegionCoverage(t *testing.T) {
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 200, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeTable},  // A: wide, top
		{X0: 0, Y0: 60, X1: 50, Y1: 110, Confidence: 0.9, Label: pdf.LayoutTypeTable}, // B: narrow, bottom
	}
	box := pdf.TextBox{X0: 0, X1: 50, Top: 0, Bottom: 110, Text: "tbl", PageNumber: 0}

	out := AnnotateBoxLayouts([]pdf.TextBox{box}, regions, 1.0, 0)

	var annotated *pdf.TextBox
	for i := range out {
		if out[i].Text == "tbl" {
			annotated = &out[i]
		}
	}
	if annotated == nil {
		t.Fatal("#4 tie-break: box not annotated as table")
	}
	if annotated.LayoutNo != "table-1" {
		t.Errorf("#4 tie-break: expected layoutno \"table-1\" (smaller region B wins the _ov tie), got %q", annotated.LayoutNo)
	}
}

// TestAnnotateBoxLayouts_NMSDedupOverlappingRegions pins #6: Python's layout
// model postprocess applies per-class NMS with IoU 0.45 (layout_recognizer.py:246,
// operators.py:667) on the RAW detections BEFORE annotation. Go must do the same
// on the raw regions; otherwise same-label detections overlapping between 0.45
// and 0.7 survive (cleanupLayouts only merges at thr=0.7) and emit extra
// synthetic figure boxes.
//
// Two same-label figure regions with IoU ~0.54: >0.45 so NMS suppresses the
// lower-score one; <0.7 so cleanupLayouts would NOT merge them. With NMS there
// is exactly one figure region -> one synthetic figure box.
func TestAnnotateBoxLayouts_NMSDedupOverlappingRegions(t *testing.T) {
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 100, Confidence: 0.9, Label: pdf.LayoutTypeFigure},  // A
		{X0: 35, Y0: 0, X1: 135, Y1: 100, Confidence: 0.8, Label: pdf.LayoutTypeFigure}, // B overlaps A (shift 35)
	}
	// No text box overlaps -> without NMS both become synthetic figures.
	// Overlap is ~0.65 (no-+1 IoU) so cleanupLayouts (thr=0.7) does NOT merge,
	// but Python's NMS uses +1 IoU (~0.50) > 0.45 and suppresses the lower-score B.
	out := AnnotateBoxLayouts([]pdf.TextBox{}, regions, 1.0, 100.0)

	if got := countFigureBoxes(out); got != 1 {
		t.Errorf("#6 NMS: expected 1 figure box after per-class NMS merges overlapping detections (IoU ~0.5), got %d", got)
	}
}

// TestAnnotateBoxLayouts_NMSDeterministic pins that equal-confidence
// same-label detections collapse to the SAME region on every run. Before the
// sort.SliceStable + original-index tie-break, sort.Slice ordered equal scores
// nondeterministically, so the survivor (and its synthetic LayoutNo) could flip
// between otherwise-identical runs — breaking reproducibility and the golden
// comparison.
func TestAnnotateBoxLayouts_NMSDeterministic(t *testing.T) {
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 100, Confidence: 0.9, Label: pdf.LayoutTypeFigure},  // A (lower original index)
		{X0: 35, Y0: 0, X1: 135, Y1: 100, Confidence: 0.9, Label: pdf.LayoutTypeFigure}, // B overlaps A, equal score
	}
	var baseline []pdf.TextBox
	for run := 0; run < 30; run++ {
		out := AnnotateBoxLayouts([]pdf.TextBox{}, regions, 1.0, 100.0)
		if got := countFigureBoxes(out); got != 1 {
			t.Fatalf("run %d: expected exactly 1 figure box after NMS, got %d", run, got)
		}
		if run == 0 {
			baseline = out
			continue
		}
		if len(out) != len(baseline) {
			t.Fatalf("run %d: box count changed across runs (%d vs %d)", run, len(out), len(baseline))
		}
		for i := range out {
			if out[i].X0 != baseline[i].X0 || out[i].X1 != baseline[i].X1 ||
				out[i].Top != baseline[i].Top || out[i].Bottom != baseline[i].Bottom ||
				out[i].LayoutNo != baseline[i].LayoutNo {
				t.Fatalf("run %d: nondeterministic output (box %d differs from run 0)", run, i)
			}
		}
	}
	// Lower-index tie-break keeps region A.
	if baseline[0].X0 != 0 || baseline[0].X1 != 100 {
		t.Errorf("expected the lower-index region A (x0=0,x1=100) to survive, got x0=%v,x1=%v", baseline[0].X0, baseline[0].X1)
	}
}

// TestAnnotateBoxLayouts_CleanupEqualScoreKeepsLater pins that when two
// same-type regions overlap beyond cleanup's 0.7 threshold with EQUAL
// confidence, Go keeps the LATER one (j) — matching Python layouts_cleanup
// (pop(i) on equal scores). Before the fix Go kept the earlier region.
//
// NMS (IoU 0.45) must not pre-remove either: A sits fully inside B, so their
// +1 IoU is tiny (<0.45) while their overlap ratio (inter/area of B) exceeds
// 0.7, so only cleanup is exercised.
func TestAnnotateBoxLayouts_CleanupEqualScoreKeepsLater(t *testing.T) {
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 400, Y1: 400, Confidence: 0.9, Label: pdf.LayoutTypeFigure},     // B: large, on top (Y0=0)
		{X0: 100, Y0: 200, X1: 200, Y1: 300, Confidence: 0.9, Label: pdf.LayoutTypeFigure}, // A: small, inside B, lower (Y0=200)
	}
	// No text box overlaps -> both unvisited -> cleanup collapses to one synthetic.
	out := AnnotateBoxLayouts([]pdf.TextBox{}, regions, 1.0, 100.0)
	if got := countFigureBoxes(out); got != 1 {
		t.Fatalf("expected 1 figure box after cleanup merge, got %d", got)
	}
	// Y-sort puts B (Y0=0) first, A (Y0=200) second; equal score -> keep later (A).
	if out[0].X0 != 100 || out[0].X1 != 200 || out[0].Top != 200 || out[0].Bottom != 300 {
		t.Errorf("cleanup equal-score: expected later region A (x0=100,x1=200,top=200,bottom=300) to survive, got x0=%v,x1=%v,top=%v,bottom=%v",
			out[0].X0, out[0].X1, out[0].Top, out[0].Bottom)
	}
}

// TestAnnotateBoxLayouts_SyntheticVisitedInterleaved pins that an unmatched
// figure region is numbered by its index in the per-type list that ALSO
// includes already-visited figure regions (Python's enumerate over the full
// type-filtered list). Before the fix Go used a separate unvisited-only
// counter, so the unmatched figure became figure-0 instead of figure-1.
func TestAnnotateBoxLayouts_SyntheticVisitedInterleaved(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "caption for A"},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeFigure},   // A: matched to text box -> visited
		{X0: 200, Y0: 0, X1: 300, Y1: 50, Confidence: 0.9, Label: pdf.LayoutTypeFigure}, // B: unmatched -> synthetic
	}
	out := AnnotateBoxLayouts(boxes, regions, 1.0, 100.0)

	var annotated, synthetic *pdf.TextBox
	for i := range out {
		if out[i].Text == "caption for A" {
			annotated = &out[i]
		}
		if out[i].LayoutType == pdf.LayoutTypeFigure && out[i].Text == "" {
			synthetic = &out[i]
		}
	}
	if annotated == nil {
		t.Fatal("visited figure A: text box was not annotated as figure")
	}
	if annotated.LayoutNo != "figure-0" {
		t.Errorf("visited figure A: expected LayoutNo figure-0, got %q", annotated.LayoutNo)
	}
	if synthetic == nil {
		t.Fatal("expected a synthetic figure box for unmatched B")
	}
	// B is the 2nd figure in Y order (after visited A), so Python numbers it figure-1.
	if synthetic.LayoutNo != "figure-1" {
		t.Errorf("unmatched figure B: expected figure-1 (index in full per-type list), got %q", synthetic.LayoutNo)
	}
	if synthetic.X0 != 200 || synthetic.X1 != 300 {
		t.Errorf("unmatched figure B: expected x0=200,x1=300, got x0=%v,x1=%v", synthetic.X0, synthetic.X1)
	}
}
