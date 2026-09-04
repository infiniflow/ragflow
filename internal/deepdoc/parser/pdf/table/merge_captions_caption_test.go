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

// TestMergeCaptions_SingleCaptionPerTable locks the fix for invalid HTML where
// a table with MORE THAN ONE caption box (e.g. a sentence above AND below the
// table) produced multiple <caption> elements inside one <table>. The HTML
// spec allows only ONE <caption> per table, so browsers/HTML parsers/Markdown
// converters keep only the first and silently drop the rest — a real content
// loss. MergeCaptions must collapse all captions attaching to the SAME table
// into a SINGLE <caption> element (texts concatenated), retaining every
// sentence.
func TestMergeCaptions_SingleCaptionPerTable(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td >Category</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 159, Right: 393, Top: 130, Bottom: 336}}},
		// Caption above (left margin) AND caption below the table — both attach to the same table.
		{Text: "Table 1: Quarterly sales by product category (in USD)", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 61, Right: 144, Top: 219, Bottom: 231}}},
		{Text: "The following table summarizes the quarterly sales performance", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 200, Right: 380, Top: 350, Bottom: 370}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	if len(result) != 1 {
		t.Fatalf("expected 1 section (table with combined caption), got %d: %v", len(result), textsOf(result))
	}
	got := result[0].Text
	if n := strings.Count(got, "<caption>"); n != 1 {
		t.Errorf("expected exactly ONE <caption> per table (HTML allows only one), got %d: %q", n, got)
	}
	// Both caption sentences must survive inside the single <caption>.
	if !strings.Contains(got, "Table 1: Quarterly sales by product category (in USD)") {
		t.Errorf("first caption sentence lost in combined <caption>: %q", got)
	}
	if !strings.Contains(got, "The following table summarizes the quarterly sales performance") {
		t.Errorf("second caption sentence lost in combined <caption>: %q", got)
	}
}

// TestMergeCaptions_ReadingOrderByTop locks that multiple captions attached to
// the SAME table are concatenated in READING order (top→bottom), matching the
// PDF layout and Python's construct_table. Real case 06_table_content.pdf has
// "The following table summarizes..." ABOVE the table (top=105) and "Table 1:
// Quarterly sales..." BELOW it (top=248), but Go's sections list carries the
// lower one first — so a pure section-order concatenation produces a reversed
// (unnatural) caption. Sorting by the caption box's top edge restores reading
// order.
func TestMergeCaptions_ReadingOrderByTop(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td >Category</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 159, Right: 393, Top: 130, Bottom: 336}}},
		// Lower caption box FIRST in section order, despite being BELOW the
		// table's caption ("Table 1..." sits inside the table band, top=248).
		{Text: "Table 1: Quarterly sales by product category (in USD)", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 62, Right: 303, Top: 248, Bottom: 261}}},
		// Upper caption box SECOND in section order, though it is ABOVE the
		// table (top=105). Reading order must place it FIRST.
		{Text: "The following table summarizes the quarterly sales performance", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 107, Right: 501, Top: 105, Bottom: 118}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	if len(result) != 1 {
		t.Fatalf("expected 1 section (table with combined caption), got %d: %v", len(result), textsOf(result))
	}
	want := "<caption>The following table summarizes the quarterly sales performance Table 1: Quarterly sales by product category (in USD)</caption>"
	if !strings.Contains(result[0].Text, want) {
		t.Errorf("caption must be concatenated in reading order (top→bottom):\n  want %q\n  got  %q", want, result[0].Text)
	}
}

// TestMergeCaptions_TallTableCaptionNearEdgeAttaches locks the fix for the
// cross-page-table caption drop (13/14): a caption sitting just above or below
// a TALL table (a cross-page table's section is one tall merged region) was
// REJECTED by findNearestParent because the distance was measured to the
// table's CENTER — for a tall table the center is far from the edge, so
// center-distance exceeded maxCaptionGap and the caption was dropped (real
// content loss, e.g. 13's 'Extended Financial Report' and 14's 'Table 1:
// Revenue'). The distance must be measured to the table's NEAREST EDGE.
func TestMergeCaptions_TallTableCaptionNearEdgeAttaches(t *testing.T) {
	table := pdf.Section{
		Text: "<table><tr><th >Month</th><th >Revenue</th></tr></table>", LayoutType: pdf.LayoutTypeTable,
		// Cross-page merged table: table_merge.go appends one Position entry
		// PER spanned page, so a real merged table carries MULTIPLE Position
		// entries (here pages 0 and 2), each with that page's local geometry.
		// A caption on a LATER page of the same table lands inside that page's
		// band in page-local coordinates (gapY=0) and must still attach — this
		// is the 13/14 cross-page caption continuation case.
		Positions: []pdf.Position{
			{PageNumbers: []int{0}, Left: 82, Right: 513, Top: 98, Bottom: 777},
			{PageNumbers: []int{2}, Left: 82, Right: 513, Top: 98, Bottom: 777},
		},
	}
	for _, c := range []struct {
		name    string
		caption pdf.Section
	}{
		{"above", pdf.Section{
			Text: "Extended Financial Report", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 183, Right: 412, Top: 64, Bottom: 82}}}},
		{"later-page-inside-band", pdf.Section{
			Text: "Table: Monthly financial summary FY2024", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 62, Right: 250, Top: 154, Bottom: 167}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sections := []pdf.Section{table, c.caption}
			figures := pdf.CollectFigures(sections)
			result := MergeCaptions(sections, figures)
			if len(result) != 1 {
				t.Fatalf("expected 1 section (table with caption), got %d: %v", len(result), textsOf(result))
			}
			want := "<caption>" + c.caption.Text + "</caption>"
			if !strings.Contains(result[0].Text, want) {
				t.Errorf("caption near tall-table edge must attach (was dropped by center-distance): want %q, got %q", want, result[0].Text)
			}
		})
	}
}

// TestMergeCaptions_CaptionOtherPageDoesNotAttach locks the page-scope guard:
// a caption clearly ABOVE or BELOW a table's Y band on a DIFFERENT page must
// NOT attach. The edge-distance alone is page-local-coordinate blind — a
// page-N caption whose page-local Y is above a page-0 table's band gets a
// small edge gap (gapY>0) and would wrongly attach. Only captions on a
// different page that VERTICALLY OVERLAP the band (gapY==0, the cross-page
// table continuation case, see TallTableCaptionNearEdgeAttaches) may attach.
func TestMergeCaptions_CaptionOtherPageDoesNotAttach(t *testing.T) {
	table := pdf.Section{
		Text: "<table><tr><th >Month</th><th >Revenue</th></tr></table>", LayoutType: pdf.LayoutTypeTable,
		// SINGLE-page table on page 0 only.
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 82, Right: 513, Top: 98, Bottom: 777}},
	}
	// Page-1 caption ABOVE the page-0 table's Y band (page-local Y 64-82 is
	// above band top 98) — on a different page, gapY>0 must reject it.
	caption := pdf.Section{
		Text: "Table 1: Revenue", LayoutType: pdf.DLALabelTableCaption,
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 183, Right: 412, Top: 64, Bottom: 82}},
	}
	sections := []pdf.Section{table, caption}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	// No table on the caption's page -> orphaned caption (dropped), table unchanged.
	if len(result) != 1 {
		t.Fatalf("expected 1 section (table unchanged, orphaned caption dropped), got %d: %v", len(result), textsOf(result))
	}
	if strings.Contains(result[0].Text, "<caption>") {
		t.Errorf("caption above a table on a DIFFERENT page must not attach: %q", result[0].Text)
	}
}

// TestMergeCaptions_FigureCaptionRawText locks that a figure caption attaching
// to its figure section is concatenated as RAW text, NOT wrapped in a
// <caption> element. The <caption> element is table-specific (matching
// Python's __html_table); a figure section carries an image, not a table, so
// wrapping its text in <caption> would emit meaningless HTML. Figure caption
// handling is outside this PR's scope, so the pre-existing raw-text behavior
// must be preserved.
func TestMergeCaptions_FigureCaptionRawText(t *testing.T) {
	sections := []pdf.Section{
		{Text: "", LayoutType: pdf.LayoutTypeFigure,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 300, Bottom: 500}}},
		{Text: "Figure 1: Revenue trend by quarter", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 510, Bottom: 525}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	if len(result) != 1 {
		t.Fatalf("expected 1 section (figure with caption), got %d: %v", len(result), textsOf(result))
	}
	got := result[0]
	if got.Text != "Figure 1: Revenue trend by quarter" {
		t.Errorf("figure caption must be appended as raw text, got %q", got.Text)
	}
	if strings.Contains(got.Text, "<caption>") {
		t.Errorf("figure section must NOT be wrapped in a <caption> element (table-specific): %q", got.Text)
	}
}

// TestMergeCaptions_NarrowCaptionAttachesWideTable locks the fix for the
// icbccs '请求参数' caption: a NARROW caption (short label, small horizontal
// extent) sitting directly above a much WIDER table has a large horizontal
// offset dx to the table's center. Before the fix, findTables rejected the
// match because dx² alone exceeded maxCaptionGap, so MergeCaptions dropped
// the caption (no table target) — losing the text. Vertical adjacency
// (small gapY) is the real signal that the caption belongs to that table, so
// the fix attaches it when gapY <= maxCaptionVGap regardless of dx.
//
// Geometry mirrors icbccs: caption x∈[28,100] (center 64) above a full-width
// table x∈[30,565] (center ~297); dx≈233 → dx²≈54k > maxCaptionGap(40k), but
// gapY=19 ≤ maxCaptionVGap(200) → must attach.
func TestMergeCaptions_NarrowCaptionAttachesWideTable(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td >名称</td><td >位置</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 30, Right: 565, Top: 197, Bottom: 314}}},
		{Text: "请求参数", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 28, Right: 100, Top: 157, Bottom: 178}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)

	if len(result) != 1 {
		t.Fatalf("expected 1 section (table with caption, standalone caption removed), got %d: %v", len(result), textsOf(result))
	}
	got := result[0].Text
	if !strings.Contains(got, "<caption>请求参数</caption>") {
		t.Errorf("narrow caption not injected as <caption>; got %q", got)
	}
	if strings.Count(got, "请求参数") != 1 {
		t.Errorf("caption text should appear exactly once (inside <caption>), got %q", got)
	}
}
