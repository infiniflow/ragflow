package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMergeCaptions_Unit verifies mergeCaptions directly without full pipeline.
func TestMergeCaptions_Unit(t *testing.T) {
	sections := []pdf.Section{
		{Text: "F", LayoutType: "figure", Positions: []pdf.Position{{PageNumbers: []int{0, 0}, Left: 40, Right: 60, Top: 30, Bottom: 45}}},
		{Text: "C", LayoutType: "figure caption", Positions: []pdf.Position{{PageNumbers: []int{0, 0}, Left: 40, Right: 60, Top: 80, Bottom: 95}}},
	}

	result := MergeCaptions(sections)

	// Caption removed.
	if len(result) != 1 {
		t.Fatalf("expected 1 section after merge, got %d", len(result))
	}
	// Figure text includes caption.
	if !strings.Contains(result[0].Text, "C") {
		t.Errorf("expected figure Text to contain caption 'C', got %q", result[0].Text)
	}
	if result[0].LayoutType != "figure" {
		t.Errorf("expected figure LayoutType, got %q", result[0].LayoutType)
	}
}

// TestMergeCaptions_TableCaption verifies table caption merging directly.
func TestMergeCaptions_TableCaption(t *testing.T) {
	sections := []pdf.Section{
		{Text: "T", LayoutType: "table", Positions: []pdf.Position{{PageNumbers: []int{0, 0}, Left: 40, Right: 60, Top: 30, Bottom: 45}}},
		{Text: "C", LayoutType: "table caption", Positions: []pdf.Position{{PageNumbers: []int{0, 0}, Left: 40, Right: 60, Top: 80, Bottom: 95}}},
	}

	result := MergeCaptions(sections)

	if len(result) != 1 {
		t.Fatalf("expected 1 section after merge, got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "C") {
		t.Errorf("expected table Text to contain caption 'C', got %q", result[0].Text)
	}
}

// TestMergeCaptions_NearbyCaptionAttaches verifies that a nearby figure
// caption is merged into its figure section.
func TestMergeCaptions_NearbyCaptionAttaches(t *testing.T) {
	sections := []pdf.Section{
		{Text: "F", LayoutType: "figure", Positions: []pdf.Position{
			{PageNumbers: []int{0, 0}, Left: 0, Right: 100, Top: 0, Bottom: 50},
		}},
		// Caption sits directly below the figure.
		{Text: "close", LayoutType: "figure caption", Positions: []pdf.Position{
			{PageNumbers: []int{0, 0}, Left: 0, Right: 100, Top: 70, Bottom: 80},
		}},
	}
	result := MergeCaptions(sections)
	// Caption merged into figure — verified by figure Text containing caption.
	if len(result) != 1 {
		t.Fatalf("expected 1 section after merge, got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "close") {
		t.Errorf("figure Text should contain caption 'close', got %q", result[0].Text)
	}
}

// TestMergeCaptions_FigureCaptionPrefersAdjacentEdge reproduces #21389. A
// small equation below the caption has a slightly nearer center than the
// actual diagram above it, even though the caption is visibly adjacent to the
// diagram's bottom edge.
func TestMergeCaptions_FigureCaptionPrefersAdjacentEdge(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Bus diagram", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 260, Right: 420, Top: 132, Bottom: 201}}},
		{Text: "Rp(min) = ...", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 279, Right: 399, Top: 257, Bottom: 283}}},
		{Text: "Figure 12. I2C bus", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 280, Right: 388, Top: 213, Bottom: 226}}},
	}

	result := MergeCaptions(sections)

	if len(result) != 2 {
		t.Fatalf("expected two figure sections after caption merge, got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "Figure 12. I2C bus") {
		t.Errorf("caption should attach to the adjacent bus diagram, got %q", result[0].Text)
	}
	if strings.Contains(result[1].Text, "Figure 12. I2C bus") {
		t.Errorf("caption attached to the nearby equation instead: %q", result[1].Text)
	}
}

// TestMergeCaptions_FigureCaptionPrefersHorizontalOverlap verifies the X-axis
// half of box-edge distance. The small equation has the nearer center, but the
// caption overlaps the diagram horizontally and is separated from the
// equation by whitespace.
func TestMergeCaptions_FigureCaptionPrefersHorizontalOverlap(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Wide diagram", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{4}, Left: 0, Right: 400, Top: 100, Bottom: 200}}},
		{Text: "x = 1", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{4}, Left: 130, Right: 150, Top: 210, Bottom: 230}}},
		{Text: "Figure 8. Wide diagram", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{4}, Left: 0, Right: 100, Top: 210, Bottom: 225}}},
	}

	result := MergeCaptions(sections)

	if len(result) != 2 {
		t.Fatalf("expected two figure sections after caption merge, got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "Figure 8. Wide diagram") {
		t.Errorf("caption should attach to the horizontally overlapping diagram, got %q", result[0].Text)
	}
	if strings.Contains(result[1].Text, "Figure 8. Wide diagram") {
		t.Errorf("caption attached across horizontal whitespace to the equation: %q", result[1].Text)
	}
}

// TestMergeCaptions_FigureCaptionCanStillAttachToEquation guards the other
// side of #21389: equations remain ordinary figure candidates when their edge
// is genuinely the closest one.
func TestMergeCaptions_FigureCaptionCanStillAttachToEquation(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Bus diagram", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 260, Right: 420, Top: 132, Bottom: 201}}},
		{Text: "Rp(min) = ...", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 279, Right: 399, Top: 257, Bottom: 283}}},
		{Text: "Figure 13. Rp minimum", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{10}, Left: 280, Right: 388, Top: 235, Bottom: 245}}},
	}

	result := MergeCaptions(sections)

	if len(result) != 2 {
		t.Fatalf("expected two figure sections after caption merge, got %d", len(result))
	}
	if strings.Contains(result[0].Text, "Figure 13. Rp minimum") {
		t.Errorf("equation caption attached to the farther bus diagram: %q", result[0].Text)
	}
	if !strings.Contains(result[1].Text, "Figure 13. Rp minimum") {
		t.Errorf("caption should attach to the adjacent equation, got %q", result[1].Text)
	}
}

// TestMergeCaptions_FigureCaptionFarAwayStaysStandalone verifies that edge
// ranking does not expand the existing center-distance eligibility boundary.
func TestMergeCaptions_FigureCaptionFarAwayStaysStandalone(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Remote diagram", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{3}, Left: 100, Right: 500, Top: 100, Bottom: 500}}},
		{Text: "Figure 20. Remote detail", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{3}, Left: 100, Right: 500, Top: 800, Bottom: 815}}},
	}

	result := MergeCaptions(sections)

	if len(result) != 2 {
		t.Fatalf("far caption should remain standalone, got %d sections", len(result))
	}
	if strings.Contains(result[0].Text, "Figure 20. Remote detail") {
		t.Errorf("far caption should not attach to the diagram: %q", result[0].Text)
	}
}

// TestMergeCaptions_FigureCaptionNoTargetKept locks the production behavior
// change flagged as untested in review: a figure caption with NO nearby figure
// section is KEPT as its own section. A pure-image figure has no text section
// to merge into, so removing the caption (the old behavior) would silently drop
// caption text that Python keeps (07_mixed_content 'Figure 1/2').
func TestMergeCaptions_FigureCaptionNoTargetKept(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Figure 2: system architecture overview", LayoutType: "figure caption",
			Positions: []pdf.Position{{PageNumbers: []int{0, 0}, Left: 40, Right: 160, Top: 300, Bottom: 315}}},
	}
	result := MergeCaptions(sections)
	if len(result) != 1 {
		t.Fatalf("figure caption with no parent must be kept, got %d sections", len(result))
	}
	if result[0].LayoutType != "figure caption" {
		t.Errorf("expected figure caption kept as its own section, got LayoutType %q", result[0].LayoutType)
	}
	if result[0].Text != "Figure 2: system architecture overview" {
		t.Errorf("caption text must be preserved, got %q", result[0].Text)
	}
}
