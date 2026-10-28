package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

const (
	repeatedGarbageTestScale         = 3.0
	repeatedGarbageTestPageImgHeight = 3000.0 // 1000 PDF units after scale
)

// footerRegion covers PDF x 0-300 and the given PDF y range near the page
// bottom; boxes inside it are in the footer band, so the layout pass discards
// them as garbage footers.
func footerRegion(y0, y1 float64) pdf.DLARegion {
	return pdf.DLARegion{
		X0: 0, Y0: y0 * repeatedGarbageTestScale,
		X1: 300, Y1: y1 * repeatedGarbageTestScale,
		Label:      pdf.LayoutTypeFooter,
		Confidence: 0.9,
	}
}

func keptText(boxes []pdf.TextBox, text string) bool {
	for _, b := range boxes {
		if b.Text == text {
			return true
		}
	}
	return false
}

func TestAnnotateBoxLayouts_DropsRepeatedGarbageText(t *testing.T) {
	text := "Company Confidential"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 0, X1: 100, Top: 965, Bottom: 985, Text: text},
		// Same footer band, outside the region's x range, so it survives the
		// layout pass and is removed by the decoration cleanup.
		{X0: 400, X1: 500, Top: 950, Bottom: 970, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{footerRegion(940, 990)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if keptText(got, text) {
		t.Fatalf("surviving copy of discarded footer text %q was kept", text)
	}
}

func TestAnnotateBoxLayouts_KeepsBodyTextMatchingDiscardedGarbage(t *testing.T) {
	text := "Company Confidential"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 0, X1: 100, Top: 965, Bottom: 985, Text: text},
		// Body text that happens to equal the footer string. Text equality is
		// not evidence of decoration, so this box must survive.
		{X0: 0, X1: 400, Top: 500, Bottom: 520, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{footerRegion(940, 990)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if !keptText(got, text) {
		t.Fatalf("body text %q outside the decoration band was dropped", text)
	}
}

func TestAnnotateBoxLayouts_KeepsRepeatedNonGarbageText(t *testing.T) {
	text := "Chapter 1"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 10, Bottom: 30, Text: text},
		{X0: 0, X1: 100, Top: 40, Bottom: 60, Text: text},
		{X0: 0, X1: 100, Top: 800, Bottom: 820, Text: text},
	}
	regions := []pdf.DLARegion{{
		X0: 0, Y0: 0, X1: 300, Y1: 3000,
		Label:      pdf.LayoutTypeText,
		Confidence: 0.9,
	}}

	got := AnnotateBoxLayouts(boxes, regions, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if len(got) != len(boxes) {
		t.Fatalf("repeated non-garbage text was dropped: got %d boxes, want %d", len(got), len(boxes))
	}
}

func TestAnnotateBoxLayouts_KeepsLongTextMatchingDiscardedGarbage(t *testing.T) {
	text := strings.TrimSpace(strings.Repeat("long body sentence ", 5))
	if n := len([]rune(text)); n <= repeatedGarbageMaxRunes {
		t.Fatalf("test text has %d runes, want more than %d", n, repeatedGarbageMaxRunes)
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 0, X1: 100, Top: 965, Bottom: 985, Text: text},
		{X0: 400, X1: 500, Top: 950, Bottom: 970, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{footerRegion(940, 990)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if !keptText(got, text) {
		t.Fatalf("long text matching discarded garbage was dropped; policy must protect body-length text")
	}
}

func TestAnnotateBoxLayouts_KeepsCaptionMatchingDiscardedGarbage(t *testing.T) {
	text := "Figure 1: Pipeline overview"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 0, X1: 100, Top: 965, Bottom: 985, Text: text},
		{X0: 400, X1: 500, Top: 950, Bottom: 970, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{footerRegion(940, 990)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if !keptText(got, text) {
		t.Fatalf("caption matching discarded garbage was dropped; captions must survive (#20767)")
	}
}

func TestAnnotateBoxLayouts_KeepsTableCellMatchingDiscardedGarbage(t *testing.T) {
	// A page number "1" discarded twice as footer garbage must not delete the
	// same text when it is a table cell: table matching needs every cell.
	text := "1"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 0, X1: 100, Top: 965, Bottom: 985, Text: text},
		{X0: 420, X1: 450, Top: 930, Bottom: 960, Text: text},
	}
	regions := []pdf.DLARegion{
		footerRegion(940, 990),
		{
			X0: 400 * repeatedGarbageTestScale, Y0: 900 * repeatedGarbageTestScale,
			X1: 700 * repeatedGarbageTestScale, Y1: 990 * repeatedGarbageTestScale,
			Label:      pdf.LayoutTypeTable,
			Confidence: 0.9,
		},
	}

	got := AnnotateBoxLayouts(boxes, regions, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if !keptText(got, text) {
		t.Fatalf("table cell %q matching discarded footer text was dropped", text)
	}
}

func TestAnnotateBoxLayouts_KeepsTextAfterSingleGarbageDrop(t *testing.T) {
	text := "Watermark"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 400, X1: 500, Top: 950, Bottom: 970, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{footerRegion(940, 990)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if len(got) != 1 || got[0].Text != text {
		t.Fatalf("a single garbage drop must not cascade to other boxes: got %+v", got)
	}
}

func TestAnnotateBoxLayouts_KeepsNonTableBoxOverlappingTable(t *testing.T) {
	// A box inside a table region can lose the layout pass's table label to a
	// higher-priority label (title) when both regions cover it.
	// MatchTableRegions still consumes every overlapping box, so cleanup must
	// not remove it: dropping it loses a cell value.
	text := "1"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 940, Bottom: 960, Text: text},
		{X0: 0, X1: 100, Top: 965, Bottom: 985, Text: text},
		{X0: 420, X1: 450, Top: 950, Bottom: 970, Text: text},
	}
	regions := []pdf.DLARegion{
		footerRegion(940, 990),
		{
			X0: 420 * repeatedGarbageTestScale, Y0: 950 * repeatedGarbageTestScale,
			X1: 450 * repeatedGarbageTestScale, Y1: 970 * repeatedGarbageTestScale,
			Label:      pdf.LayoutTypeTitle,
			Confidence: 0.9,
		},
		{
			X0: 400 * repeatedGarbageTestScale, Y0: 900 * repeatedGarbageTestScale,
			X1: 700 * repeatedGarbageTestScale, Y1: 990 * repeatedGarbageTestScale,
			Label:      pdf.LayoutTypeTable,
			Confidence: 0.9,
		},
	}

	got := AnnotateBoxLayouts(boxes, regions, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if !keptText(got, text) {
		t.Fatalf("table-overlapping box %q matching discarded footer text was dropped", text)
	}
}
