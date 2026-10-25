package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func figureSection(text string, left, right, top, bottom float64) pdf.Section {
	return pdf.Section{
		Text: text, LayoutType: pdf.LayoutTypeFigure, Image: "img",
		Positions: []pdf.Position{{PageNumbers: []int{10}, Left: left, Right: right, Top: top, Bottom: bottom}},
	}
}

// TestMergeCaptions_FigureCaptionAttachesToAdjacentDiagram locks issue #21389:
// a diagram caption sits BETWEEN the diagram above it (12pt above the
// caption) and an equation below it (31pt below). Both are figure media, and
// the equation is shorter, so its CENTRE is nearer to the caption's centre than
// the taller diagram's centre is — centre distance handed the caption to the
// equation. Adjacency is the real signal: the caption belongs to the diagram it
// is vertically closest to.
func TestMergeCaptions_FigureCaptionAttachesToAdjacentDiagram(t *testing.T) {
	sections := []pdf.Section{
		figureSection("Bus diagram", 260, 420, 132, 201),
		figureSection("Rp(min) = ...", 279, 399, 257, 283),
		{Text: "Figure 12. I2C bus", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 280, Right: 388, Top: 213, Bottom: 226}}},
	}
	result := MergeCaptions(sections)

	diagram, equation := -1, -1
	for i, s := range result {
		switch {
		case strings.Contains(s.Text, "Bus diagram"):
			diagram = i
		case strings.Contains(s.Text, "Rp(min)"):
			equation = i
		}
	}
	if diagram < 0 || equation < 0 {
		t.Fatalf("figure sections lost; got %v", textsOf(result))
	}
	if !strings.Contains(result[diagram].Text, "Figure 12. I2C bus") {
		t.Errorf("caption did not attach to the adjacent diagram: %q", result[diagram].Text)
	}
	if strings.Contains(result[equation].Text, "Figure 12.") {
		t.Errorf("caption wrongly attached to the equation below it: %q", result[equation].Text)
	}
}

// TestMergeCaptions_TallFigureCaptionNearEdgeAttaches pins the metric itself:
// a caption 120pt above a 400pt-tall figure is 327.5pt from its CENTRE (beyond
// maxCaptionGap) but only 120pt from its top EDGE, so it must still attach —
// the same tall-parent case findTables already handles for cross-page tables.
func TestMergeCaptions_TallFigureCaptionNearEdgeAttaches(t *testing.T) {
	sections := []pdf.Section{
		figureSection("tall chart", 100, 400, 300, 700),
		{Text: "Figure 6: tall chart", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 100, Right: 400, Top: 165, Bottom: 180}}},
	}
	result := MergeCaptions(sections)

	if len(result) != 1 {
		t.Fatalf("expected 1 section (figure with caption), got %d: %v", len(result), textsOf(result))
	}
	if !strings.Contains(result[0].Text, "Figure 6: tall chart") {
		t.Errorf("caption near a tall figure's edge was dropped: %q", result[0].Text)
	}
}

// TestMergeCaptions_DistantFigureCaptionStaysStandalone bounds the tolerance in
// the other direction: a caption 201pt from every figure's edge is outside
// maxCaptionGap and must not be glued to a figure it does not belong to. The
// caption section survives on its own — an unattached figure caption is kept,
// not dropped.
func TestMergeCaptions_DistantFigureCaptionStaysStandalone(t *testing.T) {
	sections := []pdf.Section{
		figureSection("unrelated chart", 100, 400, 300, 400),
		{Text: "Figure 7: somewhere else", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 100, Right: 400, Top: 601, Bottom: 617}}},
	}
	result := MergeCaptions(sections)

	if len(result) != 2 {
		t.Fatalf("caption outside maxCaptionGap must stay standalone, got %d sections: %v", len(result), textsOf(result))
	}
	if strings.Contains(result[0].Text, "Figure 7") {
		t.Errorf("distant caption was glued to the figure: %q", result[0].Text)
	}
}
