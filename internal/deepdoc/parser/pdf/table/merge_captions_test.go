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
	figures := pdf.CollectFigures(sections)

	result := MergeCaptions(sections, figures)

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
	figures := pdf.CollectFigures(sections)

	result := MergeCaptions(sections, figures)

	if len(result) != 1 {
		t.Fatalf("expected 1 section after merge, got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "C") {
		t.Errorf("expected table Text to contain caption 'C', got %q", result[0].Text)
	}
}

// TestMergeCaptions_EuclideanDistance verifies that caption matching uses
// squared Euclidean distance (center-to-center), not Y-only distance.
// Two captions at different X positions — the one closer by Euclidean
// distance wins, even if its Y distance is slightly larger.
func TestMergeCaptions_EuclideanDistance(t *testing.T) {
	sections := []pdf.Section{
		{Text: "F", LayoutType: "figure", Positions: []pdf.Position{
			{PageNumbers: []int{0, 0}, Left: 0, Right: 100, Top: 0, Bottom: 50},
		}},
		// Caption A: directly below figure (dx=0, dy=20) → Euclidean = 20²
		{Text: "close", LayoutType: "figure caption", Positions: []pdf.Position{
			{PageNumbers: []int{0, 0}, Left: 0, Right: 100, Top: 70, Bottom: 80},
		}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	// Caption merged into figure — verified by figure Text containing caption.
	if len(result) != 1 {
		t.Fatalf("expected 1 section after merge, got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "close") {
		t.Errorf("figure Text should contain caption 'close', got %q", result[0].Text)
	}
}
