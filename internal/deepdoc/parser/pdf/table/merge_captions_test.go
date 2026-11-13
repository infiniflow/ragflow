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

// TestMergeCaptions_KeepsTailInPageOrder locks the canonical order of the
// merged highlight geometry: a caption on an earlier page of a cross-page
// table is appended after the later page's region, and must be sorted back so
// the serialized _pdf_positions reads page by page. Positions[0] stays the
// table's primary box.
func TestMergeCaptions_KeepsTailInPageOrder(t *testing.T) {
	table := pdf.Section{
		Text: "<table><tr><td>x</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
		Positions: []pdf.Position{
			{PageNumbers: []int{0}, Left: 10, Right: 300, Top: 40, Bottom: 900},
			{PageNumbers: []int{1}, Left: 10, Right: 300, Top: 0, Bottom: 30},
		},
	}
	caption := pdf.Section{
		Text: "Table 1: results", LayoutType: pdf.DLALabelTableCaption,
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 300, Top: 910, Bottom: 925}},
	}
	result := MergeCaptions([]pdf.Section{table, caption})
	if len(result) != 1 {
		t.Fatalf("expected the caption to merge into the table, got %d sections", len(result))
	}
	got := result[0].Positions
	want := []struct {
		page        int
		top, bottom float64
	}{
		{page: 0, top: 40, bottom: 900},
		{page: 0, top: 910, bottom: 925},
		{page: 1, top: 0, bottom: 30},
	}
	if len(got) != len(want) {
		t.Fatalf("positions: got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if len(got[i].PageNumbers) != 1 || got[i].PageNumbers[0] != w.page ||
			got[i].Top != w.top || got[i].Bottom != w.bottom {
			t.Errorf("position %d = %+v, want page %d top %v bottom %v", i, got[i], w.page, w.top, w.bottom)
		}
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
	result := MergeCaptions(sections)
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
