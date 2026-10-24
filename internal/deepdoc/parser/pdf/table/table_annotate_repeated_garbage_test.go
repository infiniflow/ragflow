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

// headerRegion covers PDF x 0-100 and the given PDF y range; boxes inside it
// are far enough from the page edge to be discarded as garbage headers.
func headerRegion(y0, y1 float64) pdf.DLARegion {
	return pdf.DLARegion{
		X0: 0, Y0: y0 * repeatedGarbageTestScale,
		X1: 300, Y1: y1 * repeatedGarbageTestScale,
		Label:      pdf.LayoutTypeHeader,
		Confidence: 0.9,
	}
}

func TestAnnotateBoxLayouts_DropsRepeatedGarbageText(t *testing.T) {
	text := "Company Confidential"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 10, Bottom: 30, Text: text},
		{X0: 0, X1: 100, Top: 40, Bottom: 60, Text: text},
		{X0: 0, X1: 100, Top: 800, Bottom: 820, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{headerRegion(10, 60)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	for _, b := range got {
		if b.Text == text {
			t.Fatalf("surviving duplicate of discarded garbage text %q was kept", text)
		}
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
		{X0: 0, X1: 100, Top: 10, Bottom: 30, Text: text},
		{X0: 0, X1: 100, Top: 40, Bottom: 60, Text: text},
		{X0: 0, X1: 100, Top: 800, Bottom: 820, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{headerRegion(10, 60)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	found := false
	for _, b := range got {
		if b.Text == text {
			found = true
		}
	}
	if !found {
		t.Fatalf("long text matching discarded garbage was dropped; policy must protect body-length text")
	}
}

func TestAnnotateBoxLayouts_KeepsCaptionMatchingDiscardedGarbage(t *testing.T) {
	text := "Figure 1: Pipeline overview"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 10, Bottom: 30, Text: text},
		{X0: 0, X1: 100, Top: 40, Bottom: 60, Text: text},
		{X0: 0, X1: 100, Top: 800, Bottom: 820, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{headerRegion(10, 60)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	found := false
	for _, b := range got {
		if b.Text == text {
			found = true
		}
	}
	if !found {
		t.Fatalf("caption matching discarded garbage was dropped; captions must survive (#20767)")
	}
}

func TestAnnotateBoxLayouts_KeepsTextAfterSingleGarbageDrop(t *testing.T) {
	text := "Watermark"
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 10, Bottom: 30, Text: text},
		{X0: 0, X1: 100, Top: 800, Bottom: 820, Text: text},
	}

	got := AnnotateBoxLayouts(boxes, []pdf.DLARegion{headerRegion(10, 30)}, repeatedGarbageTestScale, repeatedGarbageTestPageImgHeight)
	if len(got) != 1 || got[0].Text != text {
		t.Fatalf("a single garbage drop must not cascade to other boxes: got %+v", got)
	}
}
