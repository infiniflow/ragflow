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
	figures := pdf.CollectFigures(sections) // empty: no "figure" section present
	result := MergeCaptions(sections, figures)
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

// TestMergeCaptions_DeduplicateRepeatedCaptions verifies that running/repeated
// captions from multi-page tables do not get duplicated in the <caption> element.
func TestMergeCaptions_DeduplicateRepeatedCaptions(t *testing.T) {
	sections := []pdf.Section{
		{
			Text:       "<table><tr><td>1</td></tr></table>",
			LayoutType: "table",
			Positions:  []pdf.Position{{PageNumbers: []int{0, 1}, Left: 40, Right: 500, Top: 50, Bottom: 800}},
		},
		{
			Text:       "全省各设区市价格信息",
			LayoutType: "table caption",
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 40, Right: 300, Top: 30, Bottom: 45}},
		},
		{
			Text:       "全省各设区市价格信息",
			LayoutType: "table caption",
			Positions:  []pdf.Position{{PageNumbers: []int{1}, Left: 40, Right: 300, Top: 30, Bottom: 45}},
		},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)

	if len(result) != 1 {
		t.Fatalf("expected 1 section after merge, got %d", len(result))
	}
	expected := "<table><caption>全省各设区市价格信息</caption><tr><td>1</td></tr></table>"
	if result[0].Text != expected {
		t.Errorf("expected %q, got %q", expected, result[0].Text)
	}
}

// TestInjectCaption_SubsumingCaptions verifies containment is honored in both
// directions: a caption that extends an earlier one replaces it instead of
// being concatenated next to it ("Table 1" + "Table 1 Results").
func TestInjectCaption_SubsumingCaptions(t *testing.T) {
	section := pdf.Section{Text: "<table><tr><td>1</td></tr></table>", LayoutType: "table"}
	injectCaption(&section, []string{"Table 1", "Table 1 Results"})
	want := "<table><caption>Table 1 Results</caption><tr><td>1</td></tr></table>"
	if section.Text != want {
		t.Errorf("caption = %q, want %q", section.Text, want)
	}
	// Reverse order keeps the same (longer) result.
	rev := pdf.Section{Text: "<table><tr><td>1</td></tr></table>", LayoutType: "table"}
	injectCaption(&rev, []string{"Table 1 Results", "Table 1"})
	if rev.Text != want {
		t.Errorf("reverse-order caption = %q, want %q", rev.Text, want)
	}
}

func TestDedupCaptions_ReplacementKeepsPositionOfFirstReplaced(t *testing.T) {
	got := dedupCaptions([]string{"Table 1", "Notes", "Table 1 Results"})
	want := []string{"Table 1 Results", "Notes"}
	if len(got) != len(want) {
		t.Fatalf("dedupCaptions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupCaptions = %v, want %v (replacement must take the position of the first caption it replaces)", got, want)
		}
	}
}
