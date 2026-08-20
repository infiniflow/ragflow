package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMergeCaptions_EmitsCaptionTag locks the fix for go_bug
// table-html-emission-format: a "table caption" section must be emitted as a
// <caption> element INSIDE the target table's HTML (matching Python's
// __html_table), and the standalone caption section removed — NOT dropped
// (its previous behavior) and NOT duplicated. This retains the caption text
// that Go used to silently lose.
func TestMergeCaptions_EmitsCaptionTag(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td >Category</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 159, Right: 393, Top: 130, Bottom: 336}}},
		{Text: "Table 1: Revenue", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 61, Right: 144, Top: 219, Bottom: 231}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)

	if len(result) != 1 {
		t.Fatalf("expected 1 section (table with caption, standalone caption removed), got %d: %v", len(result), textsOf(result))
	}
	got := result[0].Text
	if !strings.Contains(got, "<caption>Table 1: Revenue</caption>") {
		t.Errorf("table HTML missing <caption> element; got %q", got)
	}
	// Caption must sit INSIDE the table, right after <table>.
	if i := strings.Index(got, "<table>"); i < 0 || !strings.Contains(got[:i+len("<table><caption>")], "<caption>") {
		t.Errorf("expected <caption> immediately after <table>, got %q", got)
	}
	// No duplicate: the bare caption text must not also appear as a separate section
	// (it is consumed into the table).
	if strings.Count(got, "Table 1: Revenue") != 1 {
		t.Errorf("caption text should appear exactly once (inside <caption>), got %q", got)
	}
}

// TestMergeCaptions_LeftMarginCaptionAttaches locks that a caption positioned in
// the left margin (far from the table's horizontal center) still attaches to
// its table, not to a nearer non-table section. Before the fix,
// findNearestParent used pure Euclidean distance over ALL sections and returned
// -1 (nearest was another caption / non-table), so the caption was dropped.
func TestMergeCaptions_LeftMarginCaptionAttaches(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Product Analysis Report", LayoutType: pdf.LayoutTypeTitle,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 192, Right: 404, Top: 65, Bottom: 82}}},
		{Text: "<table><tr><td >Category</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 159, Right: 393, Top: 130, Bottom: 336}}},
		// Caption sits in the left margin, far from the table's horizontal center.
		{Text: "The following table summarizes the quarterly sales performance", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 61, Right: 144, Top: 150, Bottom: 170}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	if len(result) != 2 {
		t.Fatalf("expected 2 sections (title + table-with-caption), got %d: %v", len(result), textsOf(result))
	}
	found := false
	for _, s := range result {
		if strings.Contains(s.Text, "<caption>The following table summarizes the quarterly sales performance</caption>") {
			found = true
		}
	}
	if !found {
		t.Errorf("left-margin caption dropped; sections = %v", textsOf(result))
	}
}
