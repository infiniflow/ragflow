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

// TestMergeCaptions_CJKBodyParagraphKept locks go_bug
// cjk-caption-false-positive end-to-end: a Chinese/Japanese body paragraph
// starting with 表/图 must survive MergeCaptions as its own section (Python
// keeps it as body text) — neither dropped when no table is nearby, nor
// swallowed into a table's <caption> when one is.
func TestMergeCaptions_CJKBodyParagraphKept(t *testing.T) {
	para := pdf.Section{Text: "表格是一种常见的数据组织形式，本文对其进行对比。", LayoutType: pdf.LayoutTypeText,
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 170, Bottom: 195}}}
	t.Run("no table nearby", func(t *testing.T) {
		sections := []pdf.Section{
			{Text: "产品分析报告", LayoutType: pdf.LayoutTypeTitle,
				Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 50, Bottom: 80}}},
			para,
		}
		result := MergeCaptions(sections, pdf.CollectFigures(sections))
		if len(result) != 2 {
			t.Errorf("body paragraph dropped; got %d sections: %v", len(result), textsOf(result))
		}
	})
	t.Run("table nearby", func(t *testing.T) {
		sections := []pdf.Section{
			{Text: "<table><tr><td>data</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
				Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 200, Bottom: 400}}},
			para,
		}
		result := MergeCaptions(sections, pdf.CollectFigures(sections))
		kept, swallowed := false, false
		for _, s := range result {
			if strings.Contains(s.Text, "表格是一种") {
				kept = true
			}
			if s.LayoutType == pdf.LayoutTypeTable && strings.Contains(s.Text, "<caption>表格是") {
				swallowed = true
			}
		}
		if !kept || swallowed {
			t.Errorf("body paragraph must stay a standalone section (not enter <caption>); kept=%v swallowed=%v sections=%v",
				kept, swallowed, textsOf(result))
		}
	})
}

// TestMergeCaptions_FigureWithCaptionMarkerTextSurvives locks go_bug
// figure-self-caption-deleted: a figure section whose OWN text starts with a
// caption marker (embedded chart title, OCR'd caption inside the figure box)
// must not be classified as its own caption and deleted. Python pops figure
// boxes before its caption scan so this cannot happen there; before the fix,
// findNearestParent matched the figure to ITSELF (CollectFigures includes it,
// distance 0) and the whole image section vanished from the output. Covers
// the figure-kind (图1/Figure 1) and table-kind (Table 2) classification
// paths — the latter would also steal the figure's text into a nearby
// table's <caption>.
func TestMergeCaptions_FigureWithCaptionMarkerTextSurvives(t *testing.T) {
	for _, text := range []string{
		"Figure 1: system architecture overview",
		"图1 系统架构总览",
		"Table 2: embedded chart title inside the figure region",
	} {
		sections := []pdf.Section{
			{Text: "Introduction", LayoutType: pdf.LayoutTypeTitle,
				Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 50, Bottom: 80}}},
			{Text: text, LayoutType: pdf.LayoutTypeFigure, Image: "img",
				Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		}
		result := MergeCaptions(sections, pdf.CollectFigures(sections))
		figureAlive := false
		for _, s := range result {
			if s.LayoutType == pdf.LayoutTypeFigure {
				figureAlive = true
				if s.Text != text {
					t.Errorf("figure text mutated: got %q, want %q", s.Text, text)
				}
			}
		}
		if !figureAlive {
			t.Errorf("figure %q removed from output (self-caption deletion); sections = %v", text, textsOf(result))
		}
	}
}

// TestMergeCaptions_FigureTextNotStolenIntoTable: same figure, but with a
// real table in range — before the fix, the figure's "Table 2: …" text was
// injected into that table's <caption> and the figure section deleted.
func TestMergeCaptions_FigureTextNotStolenIntoTable(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td>data</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		{Text: "Table 2: embedded chart title inside the figure region", LayoutType: pdf.LayoutTypeFigure, Image: "chartimg",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 120, Right: 480, Top: 420, Bottom: 600}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	figureAlive, stolen := false, false
	for _, s := range result {
		if s.LayoutType == pdf.LayoutTypeFigure {
			figureAlive = true
		}
		if s.LayoutType == pdf.LayoutTypeTable && strings.Contains(s.Text, "embedded chart title") {
			stolen = true
		}
	}
	if !figureAlive {
		t.Errorf("figure removed; sections = %v", textsOf(result))
	}
	if stolen {
		t.Errorf("figure text stolen into the table's <caption>: %v", textsOf(result))
	}
}

// TestMergeCaptions_FigureCaptionKeptWithMarkerFigure locks the cascade form
// of figure-self-caption-deleted: a genuine DLA figure caption attaching to a
// figure whose own text starts with a caption marker must keep BOTH the
// image and the real caption text (before the fix, the figure became its own
// target, was deleted, and the attached caption text vanished with it).
func TestMergeCaptions_FigureCaptionKeptWithMarkerFigure(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Figure 4: deployment topology", LayoutType: pdf.LayoutTypeFigure, Image: "img4",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		{Text: "图4 部署拓扑结构", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 410, Bottom: 430}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	if len(result) != 1 || result[0].LayoutType != pdf.LayoutTypeFigure {
		t.Fatalf("want the single figure section, got %d sections: %v", len(result), textsOf(result))
	}
	if !strings.Contains(result[0].Text, "部署拓扑") {
		t.Errorf("real figure caption text lost with the deleted figure: %q", result[0].Text)
	}
}

// TestMergeCaptions_TableCaptionFallsBackToFigure locks go_bug
// table-caption-orphan-dropped: Python attaches a caption to the nearest
// table OR figure (nearest(tables)/nearest(figures), `elif fk`) and drops it
// only when BOTH searches fail. Go used to search tables only and delete the
// caption when none was reachable — losing text Python keeps, for English
// body text AND for correctly-DLA-labeled "table caption" sections alike
// (language-independent; the figure here is deliberately neutral-text so the
// figure-self-deletion bug cannot mask this path).
func TestMergeCaptions_TableCaptionFallsBackToFigure(t *testing.T) {
	for _, caption := range []pdf.Section{
		{Text: "Table 1 shows revenue by category.", LayoutType: pdf.LayoutTypeText,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 510, Bottom: 530}}},
		{Text: "Quarterly revenue breakdown", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 510, Bottom: 530}}},
	} {
		sections := []pdf.Section{
			{Text: "market overview chart", LayoutType: pdf.LayoutTypeFigure, Image: "img",
				Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 300, Bottom: 500}}},
			caption,
		}
		result := MergeCaptions(sections, pdf.CollectFigures(sections))
		found := false
		for _, s := range result {
			if strings.Contains(s.Text, "revenue") {
				found = true
			}
		}
		if !found {
			t.Errorf("table caption %q dropped although a figure exists (Python attaches it); sections = %v",
				caption.Text, textsOf(result))
		}
	}
}

// TestMergeCaptions_EnglishTable1OpeningParagraphSwallowed pins the SHARED
// (Python-identical) heuristic boundary of the CJK fix: a body paragraph that
// STARTS with "Table N" is classified a caption by BOTH implementations
// (Python's re.match is start-anchored too) and consumed into the table's
// <caption>. The text is retained — misplacement parity with Python, not a
// Go regression; only the CJK false-positive class was fixed.
func TestMergeCaptions_EnglishTable1OpeningParagraphSwallowed(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td>data</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		{Text: "Table 1 shows revenue by category for the fiscal year.", LayoutType: pdf.LayoutTypeText,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 170, Bottom: 195}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	for _, s := range result {
		if s.LayoutType == pdf.LayoutTypeTable && strings.Contains(s.Text, "<caption>Table 1 shows revenue") {
			return
		}
	}
	t.Errorf("expected the start-anchored English paragraph consumed into <caption> (Python parity); sections = %v",
		textsOf(result))
}

// TestMergeCaptions_FigureCaptionOtherPageDoesNotAttach locks the figure-side
// page-scope guard (go_bug figure-caption-cross-page-attached). Page-local
// coordinates repeat on every page, so the figure search used to match on
// page-local distance alone: here the page-2 caption's page-local centre is
// CLOSER to the page-0 figure (dist²=13225) than to the page-2 figure it
// belongs to (dist²=21025). The caption was therefore glued to the page-0
// figure — pages away — and its own figure was left with no caption at all.
func TestMergeCaptions_FigureCaptionOtherPageDoesNotAttach(t *testing.T) {
	sections := []pdf.Section{
		{Text: "page zero chart", LayoutType: pdf.LayoutTypeFigure, Image: "img0",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 250, Right: 350, Top: 250, Bottom: 350}}},
		{Text: "page two chart", LayoutType: pdf.LayoutTypeFigure, Image: "img2",
			Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 250, Right: 350, Top: 510, Bottom: 610}}},
		{Text: "Figure 9: latency by shard count", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 250, Right: 350, Top: 400, Bottom: 430}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))

	var onZero, onTwo bool
	for _, s := range result {
		if s.LayoutType != pdf.LayoutTypeFigure {
			continue
		}
		hasCap := strings.Contains(s.Text, "latency by shard count")
		switch s.Positions[0].PageNumbers[0] {
		case 0:
			onZero = hasCap
		case 2:
			onTwo = hasCap
		}
	}
	if !onTwo {
		t.Errorf("caption did not attach to its own (page 2) figure; sections = %v", textsOf(result))
	}
	if onZero {
		t.Errorf("caption wrongly attached to the page-0 figure (page-local distance only): %v", textsOf(result))
	}
}

// TestMergeCaptions_FigureCaptionUnknownPageStillAttaches pins the deliberate
// asymmetry with findTables: a figure whose positions carry NO page metadata is
// kept as a candidate. This search is also the fallback target for orphaned
// table captions, so rejecting on missing metadata would DELETE their text.
func TestMergeCaptions_FigureCaptionUnknownPageStillAttaches(t *testing.T) {
	sections := []pdf.Section{
		{Text: "metadata-less chart", LayoutType: pdf.LayoutTypeFigure, Image: "imgnp",
			Positions: []pdf.Position{{Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		{Text: "Table 3: yearly totals", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{4}, Left: 100, Right: 500, Top: 410, Bottom: 430}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	for _, s := range result {
		if s.LayoutType == pdf.LayoutTypeFigure && strings.Contains(s.Text, "yearly totals") {
			return
		}
	}
	t.Errorf("caption dropped instead of attaching to the page-less figure: %v", textsOf(result))
}

// TestMergeCaptions_TableFallbackSurvivesPageGuard locks the interaction
// between the figure-side page guard and the orphaned-table-caption fallback
// (go_bug table-caption-orphan-dropped). A table caption with no table in
// range falls back to the figure search; the page guard must not then reject
// every candidate and hand the caller a section to delete. Here the only
// figure is on another page, so the page-scoped pass finds nothing — the
// fallback must retry without the page filter and keep the text. Before that
// retry existed, this caption was deleted outright.
func TestMergeCaptions_TableFallbackSurvivesPageGuard(t *testing.T) {
	sections := []pdf.Section{
		{Text: "far away chart", LayoutType: pdf.LayoutTypeFigure, Image: "img0",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 300, Bottom: 500}}},
		{Text: "Table 5 shows the answer quality and retrieval performance across benchmarks.",
			LayoutType: pdf.LayoutTypeText,
			Positions:  []pdf.Position{{PageNumbers: []int{7}, Left: 100, Right: 400, Top: 320, Bottom: 340}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	for _, s := range result {
		if strings.Contains(s.Text, "answer quality and retrieval performance") {
			return
		}
	}
	t.Errorf("table caption text deleted instead of preserved on the fallback figure: %v", textsOf(result))
}

// TestMergeCaptions_FigureCaptionPositionMerged locks the highlight geometry:
// a caption merged into a figure carries its TEXT into the figure's chunk, so
// its box must be merged into the figure's Positions too — otherwise the UI
// highlights the figure region only and the caption line is never highlighted
// with the image. Positions[0] must stay the figure's own box (reading-order
// sorting and the proximity searches use it).
func TestMergeCaptions_FigureCaptionPositionMerged(t *testing.T) {
	sections := []pdf.Section{
		{Text: "revenue chart", LayoutType: pdf.LayoutTypeFigure, Image: "img",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 300, Bottom: 500}}},
		{Text: "Figure 1: revenue by quarter", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 510, Bottom: 525}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	if len(result) != 1 {
		t.Fatalf("expected 1 section (figure with caption), got %d: %v", len(result), textsOf(result))
	}
	got := result[0]
	if len(got.Positions) != 2 {
		t.Fatalf("caption box not merged into the figure's positions: %+v", got.Positions)
	}
	if got.Positions[0].Top != 300 || got.Positions[0].Bottom != 500 {
		t.Errorf("primary box must stay first, got %+v", got.Positions[0])
	}
	found := false
	for _, p := range got.Positions {
		if p.Top == 510 && p.Bottom == 525 {
			found = true
		}
	}
	if !found {
		t.Errorf("caption box missing from the merged positions: %+v", got.Positions)
	}
}

// TestMergeCaptions_CrossPageFallbackDoesNotClaimPage pins the guard on the
// above: a table caption rescued by the page-blind fallback can land on a
// figure pages away. Its text is still preserved, but its box must NOT be
// merged into the figure's positions — Position pages drive the section's page
// set, its render/eviction window and its crop plan, so claiming a page the
// figure does not occupy would corrupt all three.
func TestMergeCaptions_CrossPageFallbackDoesNotClaimPage(t *testing.T) {
	sections := []pdf.Section{
		{Text: "far chart", LayoutType: pdf.LayoutTypeFigure, Image: "img0",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 300, Bottom: 500}}},
		{Text: "Table 9: yearly totals", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{5}, Left: 100, Right: 400, Top: 320, Bottom: 340}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	for _, s := range result {
		if s.LayoutType != pdf.LayoutTypeFigure {
			continue
		}
		if len(s.Positions) != 1 {
			t.Errorf("cross-page caption must not extend the figure's positions: %+v", s.Positions)
		}
		if !strings.Contains(s.Text, "yearly totals") {
			t.Errorf("caption text must still be preserved: %q", s.Text)
		}
	}
}

// TestMergeCaptions_IdenticalBoxOtherPageNotChosen locks the page identity of
// the figure the caption lands on. Two figures on DIFFERENT pages can share a
// page-local box (page-local coordinates repeat per page — the very reason the
// page filter exists). The search used to map its hit back to a section by
// comparing position boxes alone, which returned the earliest section with an
// equal box: the caption was then attached to the other page's figure.
func TestMergeCaptions_IdenticalBoxOtherPageNotChosen(t *testing.T) {
	shared := func(pg int) []pdf.Position {
		return []pdf.Position{{PageNumbers: []int{pg}, Left: 100, Right: 400, Top: 200, Bottom: 400}}
	}
	sections := []pdf.Section{
		{Text: "page zero chart", LayoutType: pdf.LayoutTypeFigure, Image: "i0", Positions: shared(0)},
		{Text: "page two chart", LayoutType: pdf.LayoutTypeFigure, Image: "i2", Positions: shared(2)},
		{Text: "Figure 7: numbers", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 100, Right: 400, Top: 410, Bottom: 425}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	var zero, two bool
	for _, s := range result {
		has := strings.Contains(s.Text, "numbers")
		switch s.Positions[0].PageNumbers[0] {
		case 0:
			zero = has
		case 2:
			two = has
		}
	}
	if !two {
		t.Errorf("caption did not attach to the page-2 figure; sections = %v", textsOf(result))
	}
	if zero {
		t.Errorf("caption attached to the page-0 figure that merely shares a box: %v", textsOf(result))
	}
}

// TestMergeCaptions_SamePageFigureBeatsPageLessNearer locks the candidate
// ordering: a figure with no page metadata stays eligible so that a caption the
// table fallback must not drop can still land somewhere, but it must never
// outbid a figure we can actually place on the caption's page.
func TestMergeCaptions_SamePageFigureBeatsPageLessNearer(t *testing.T) {
	sections := []pdf.Section{
		// page-less figure, much CLOSER to the caption than the same-page one:
		// dist² 56 against the same-page figure's 6006, so distance alone would
		// pick it.
		{Text: "page-less chart", LayoutType: pdf.LayoutTypeFigure, Image: "np",
			Positions: []pdf.Position{{Left: 100, Right: 400, Top: 500, Bottom: 540}}},
		{Text: "same-page chart", LayoutType: pdf.LayoutTypeFigure, Image: "sp",
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 100, Right: 400, Top: 400, Bottom: 500}}},
		{Text: "Figure 3: detail", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 100, Right: 400, Top: 520, Bottom: 535}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	for _, s := range result {
		if !strings.Contains(s.Text, "detail") {
			continue
		}
		if !strings.Contains(s.Text, "same-page chart") {
			t.Errorf("closer page-less figure outbid the caption's own-page figure: %v", textsOf(result))
		}
	}
}

// TestMergeCaptions_TableCaptionPositionMerged is the table half of the
// highlight-geometry fix: injectCaption puts the caption text inside the
// table's HTML, but a table's caption box routinely sits a few points ABOVE the
// table box rather than overlapping it, so without merging the boxes the UI
// highlights the table and never the caption line above it.
func TestMergeCaptions_TableCaptionPositionMerged(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td>x</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 113, Right: 484, Top: 161, Bottom: 236}}},
		{Text: "Table 1: results", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 210, Right: 385, Top: 138, Bottom: 149}}},
	}
	result := MergeCaptions(sections, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 section (table with caption), got %d: %v", len(result), textsOf(result))
	}
	got := result[0]
	if !strings.Contains(got.Text, "<caption>") {
		t.Fatalf("caption not injected into the table HTML: %q", got.Text)
	}
	if len(got.Positions) != 2 {
		t.Fatalf("caption box not merged into the table's positions: %+v", got.Positions)
	}
	if got.Positions[0].Top != 161 || got.Positions[0].Bottom != 236 {
		t.Errorf("primary box must stay first, got %+v", got.Positions[0])
	}
	found := false
	for _, p := range got.Positions {
		if p.Top == 138 && p.Bottom == 149 {
			found = true
		}
	}
	if !found {
		t.Errorf("caption box missing from the merged positions: %+v", got.Positions)
	}
}

// TestMergeCaptions_MergedCaptionKeepsOnlySharedPages locks the page trimming
// on the highlight merge: a caption box that itself spans pages must not make
// the target claim a page it does not occupy. Position pages drive the
// section's page set, its render/eviction window and its crop plan, so merging
// a {0,5} caption box into a page-0 figure would corrupt all three.
func TestMergeCaptions_MergedCaptionKeepsOnlySharedPages(t *testing.T) {
	sections := []pdf.Section{
		{Text: "spread chart", LayoutType: pdf.LayoutTypeFigure, Image: "img",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 400, Top: 300, Bottom: 500}}},
		{Text: "Figure 5: spread", LayoutType: pdf.DLALabelFigureCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0, 5}, Left: 100, Right: 400, Top: 510, Bottom: 525}}},
	}
	result := MergeCaptions(sections, pdf.CollectFigures(sections))
	if len(result) != 1 {
		t.Fatalf("expected 1 section (figure with caption), got %d: %v", len(result), textsOf(result))
	}
	got := result[0]
	if len(got.Positions) != 2 {
		t.Fatalf("caption box not merged: %+v", got.Positions)
	}
	if pn := got.Positions[1].PageNumbers; len(pn) != 1 || pn[0] != 0 {
		t.Errorf("merged caption must keep only the pages the figure occupies, got %v", pn)
	}
	if pn := got.Positions[0].PageNumbers; len(pn) != 1 || pn[0] != 0 {
		t.Errorf("primary box changed: %v", pn)
	}
}
