package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestBoxesToSections_CrossPagePositionTag(t *testing.T) {
	// Page 0: 267 PDF-points tall (800px at zoom=3).
	// Box bottom=400 > 267 → spills into page 1 by 133pt.
	boxes := []pdf.TextBox{
		{X0: 100, X1: 500, Top: 200, Bottom: 400, PageNumber: 0, Text: "跨页表格"},
	}
	pageHeights := map[int]float64{0: 267.0}

	sections := BoxesToSections(boxes, pageHeights)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	s := sections[0]

	// Python: @@1-2\t100.0\t500.0\t200.0\t133.0##
	// Page 0→1 becomes 1-indexed → pages 1-2.
	if s.PositionTag != "@@1-2\t100.0\t500.0\t200.0\t133.0##" {
		t.Errorf("PositionTag: got %q, want '@@1-2\\t100.0\\t500.0\\t200.0\\t133.0##'", s.PositionTag)
	}
	if len(s.Positions) != 1 {
		t.Fatalf("expected 1 pdf.Position, got %d", len(s.Positions))
	}
	p := s.Positions[0]
	if len(p.PageNumbers) != 2 || p.PageNumbers[0] != 0 || p.PageNumbers[1] != 1 {
		t.Errorf("PageNumbers: got %v, want [0, 1]", p.PageNumbers)
	}
	if p.Top != 200 || p.Bottom != 133 {
		t.Errorf("coords: top=%v (want 200), bottom=%v (want 133 = 400-267)", p.Top, p.Bottom)
	}
}

// TestBoxesToSections_SinglePageUnchanged verifies single-page boxes are
// unaffected by the cross-page change.
func TestBoxesToSections_SinglePageUnchanged(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 50, X1: 200, Top: 10, Bottom: 30, PageNumber: 0, Text: "普通文本"},
	}
	pageHeights := map[int]float64{0: 267.0}

	sections := BoxesToSections(boxes, pageHeights)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	// Single page: tag should be @@1, not @@1-1
	if sections[0].PositionTag != "@@1\t50.0\t200.0\t10.0\t30.0##" {
		t.Errorf("single-page PositionTag: got %q", sections[0].PositionTag)
	}
	if len(sections[0].Positions[0].PageNumbers) != 1 {
		t.Errorf("single-page PageNumbers: got %v, want [0]", sections[0].Positions[0].PageNumbers)
	}
}

func TestResolvePageSpan_SinglePage(t *testing.T) {
	// Box fits within the page → toPage unchanged, bottom unchanged.
	toPage, bottom := ResolvePageSpan(0, 30, map[int]float64{0: 267})
	if toPage != 0 || bottom != 30 {
		t.Errorf("got toPage=%d bottom=%v, want 0, 30", toPage, bottom)
	}
}

func TestResolvePageSpan_CrossPage(t *testing.T) {
	// Box bottom=400 exceeds page 0 height=267 → spans to page 1.
	toPage, bottom := ResolvePageSpan(0, 400, map[int]float64{0: 267})
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1", toPage)
	}
	if bottom != 133 {
		t.Errorf("bottom = %v, want 133 (400-267)", bottom)
	}
}

func TestResolvePageSpan_MultiPage(t *testing.T) {
	// Box bottom=600, page 0=267, page 1=200, page 2=200.
	heights := map[int]float64{0: 267, 1: 200, 2: 200}
	toPage, bottom := ResolvePageSpan(0, 600, heights)
	if toPage != 2 {
		t.Errorf("toPage = %d, want 2", toPage)
	}
	if bottom != 133 {
		t.Errorf("bottom = %v, want 133 (600-267-200)", bottom)
	}
}

func TestResolvePageSpan_NilHeights(t *testing.T) {
	toPage, bottom := ResolvePageSpan(0, 400, nil)
	if toPage != 0 || bottom != 400 {
		t.Errorf("got toPage=%d bottom=%v, want 0, 400 (nil=no cross-page)", toPage, bottom)
	}
}

func TestResolvePageSpan_ZeroHeightGuard(t *testing.T) {
	// Zero-height pages must not cause an infinite loop.
	// Page 0=200, page 1=0, page 2=0, page 3=300 — box bottom=500.
	heights := map[int]float64{0: 200, 1: 0, 2: 0, 3: 300}
	toPage, bottom := ResolvePageSpan(0, 500, heights)
	// 500-200=300 remaining; page1=0 → break at unknown/invalid; toPage=1, bottom=300.
	// (the break path treats zero/unknown as "assume same height once and stop")
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1 (stopped at first zero-height page)", toPage)
	}
	if bottom != 300 {
		t.Errorf("bottom = %v, want 300 (500-200)", bottom)
	}
}

func TestResolvePageSpan_UnknownNextPage(t *testing.T) {
	// Next page not in map → assume same height once, then stop.
	heights := map[int]float64{0: 267}
	toPage, bottom := ResolvePageSpan(0, 500, heights)
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1 (one fallback extension)", toPage)
	}
	if bottom != 233 {
		t.Errorf("bottom = %v, want 233 (500-267)", bottom)
	}
}

func TestResolvePageSpan_NegativePh(t *testing.T) {
	heights := map[int]float64{0: 200, 1: -10, 2: 200}
	toPage, bottom := ResolvePageSpan(0, 500, heights)
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1 (stopped at negative-height page)", toPage)
	}
	if bottom != 300 {
		t.Errorf("bottom = %v, want 300 (500-200)", bottom)
	}
}

func TestNormalizeSectionPositions_ValidTag(t *testing.T) {
	sections := []pdf.Section{
		{Text: "test", PositionTag: "@@1\t50.0\t300.0\t200.0\t400.0##", Positions: nil},
	}
	NormalizeSectionPositions(sections)

	s := sections[0]
	if len(s.Positions) != 1 {
		t.Fatalf("expected 1 Position, got %d", len(s.Positions))
	}
	p := s.Positions[0]
	if len(p.PageNumbers) != 1 || p.PageNumbers[0] != 0 {
		t.Errorf("PageNumbers: got %v, want [0]", p.PageNumbers)
	}
	if p.Left != 50.0 || p.Right != 300.0 || p.Top != 200.0 || p.Bottom != 400.0 {
		t.Errorf("coords: got (%.1f, %.1f, %.1f, %.1f), want (50.0, 300.0, 200.0, 400.0)",
			p.Left, p.Right, p.Top, p.Bottom)
	}
}

func TestNormalizeSectionPositions_EmptyTag(t *testing.T) {
	sections := []pdf.Section{
		{Text: "test", PositionTag: "", Positions: nil},
	}
	NormalizeSectionPositions(sections)

	if len(sections[0].Positions) != 0 {
		t.Errorf("expected empty Positions, got %v", sections[0].Positions)
	}
}

func TestNormalizeSectionPositions_AlreadyPopulated(t *testing.T) {
	existing := []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 20, Top: 30, Bottom: 40}}
	sections := []pdf.Section{
		{Text: "test", PositionTag: "@@1\t50.0\t300.0\t200.0\t400.0##", Positions: existing},
	}
	NormalizeSectionPositions(sections)

	// Should NOT overwrite existing Positions
	if len(sections[0].Positions) != 1 {
		t.Fatalf("expected 1 Position, got %d", len(sections[0].Positions))
	}
	p := sections[0].Positions[0]
	if p.Left != 10 || p.Top != 30 {
		t.Errorf("Positions were overwritten: got left=%.1f top=%.1f, want left=10 top=30", p.Left, p.Top)
	}
}

func TestNormalizeSectionPositions_MultiPageTag(t *testing.T) {
	sections := []pdf.Section{
		{Text: "test", PositionTag: "@@1-2\t50.0\t300.0\t200.0\t400.0##", Positions: nil},
	}
	NormalizeSectionPositions(sections)

	p := sections[0].Positions[0]
	if len(p.PageNumbers) != 2 || p.PageNumbers[0] != 0 || p.PageNumbers[1] != 1 {
		t.Errorf("PageNumbers: got %v, want [0, 1]", p.PageNumbers)
	}
}

func TestNormalizeSectionPositions_MixedSlice(t *testing.T) {
	existing := []pdf.Position{{PageNumbers: []int{2}, Left: 1, Right: 2, Top: 3, Bottom: 4}}
	sections := []pdf.Section{
		{Text: "has_positions", PositionTag: "@@9\t1.0\t2.0\t3.0\t4.0##", Positions: existing},
		{Text: "no_positions", PositionTag: "@@1\t50.0\t300.0\t200.0\t400.0##", Positions: nil},
		{Text: "no_tag", PositionTag: "", Positions: nil},
	}
	NormalizeSectionPositions(sections)

	// has_positions: existing preserved
	if sections[0].Positions[0].PageNumbers[0] != 2 {
		t.Errorf("existing Positions were overwritten")
	}
	// no_positions: parsed from tag
	if len(sections[1].Positions) != 1 || sections[1].Positions[0].PageNumbers[0] != 0 {
		t.Errorf("no_positions not normalized: %v", sections[1].Positions)
	}
	// no_tag: left empty
	if len(sections[2].Positions) != 0 {
		t.Errorf("no_tag should remain empty: %v", sections[2].Positions)
	}
}

func TestNormalizeSectionPositions_NilInput(t *testing.T) {
	// Should not panic
	NormalizeSectionPositions(nil)
}

func TestNormalizeSectionPositions_EmptySlice(t *testing.T) {
	sections := []pdf.Section{}
	NormalizeSectionPositions(sections)
	if len(sections) != 0 {
		t.Error("empty slice should remain empty")
	}
}

func TestSectionsToMarkdown_Title(t *testing.T) {
	sections := []pdf.Section{
		{LayoutType: pdf.LayoutTypeTitle, Text: "标题", Image: ""},
	}
	got := SectionsToMarkdown(sections)
	want := "\n## 标题\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSectionsToMarkdown_FigureWithImage(t *testing.T) {
	sections := []pdf.Section{
		{LayoutType: pdf.LayoutTypeFigure, Text: "图1", Image: "abc123"},
	}
	got := SectionsToMarkdown(sections)
	want := "\n![Image](data:image/png;base64,abc123)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSectionsToMarkdown_FigureWithoutImage(t *testing.T) {
	sections := []pdf.Section{
		{LayoutType: pdf.LayoutTypeFigure, Text: "图1", Image: ""},
	}
	got := SectionsToMarkdown(sections)
	want := "图1\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSectionsToMarkdown_Text(t *testing.T) {
	sections := []pdf.Section{
		{LayoutType: pdf.LayoutTypeText, Text: "普通内容", Image: ""},
	}
	got := SectionsToMarkdown(sections)
	want := "普通内容\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSectionsToMarkdown_Table(t *testing.T) {
	sections := []pdf.Section{
		{LayoutType: pdf.LayoutTypeTable, Text: "表格内容", Image: ""},
	}
	got := SectionsToMarkdown(sections)
	want := "表格内容\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSectionsToMarkdown_Mixed(t *testing.T) {
	sections := []pdf.Section{
		{LayoutType: pdf.LayoutTypeTitle, Text: "标题", Image: ""},
		{LayoutType: pdf.LayoutTypeText, Text: "内容", Image: ""},
		{LayoutType: pdf.LayoutTypeFigure, Text: "图", Image: "img"},
		{LayoutType: pdf.LayoutTypeTable, Text: "表格", Image: ""},
	}
	got := SectionsToMarkdown(sections)
	want := "\n## 标题\n内容\n\n![Image](data:image/png;base64,img)表格\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSectionsToMarkdown_EmptySlice(t *testing.T) {
	got := SectionsToMarkdown([]pdf.Section{})
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestSectionsToMarkdown_NilSlice(t *testing.T) {
	got := SectionsToMarkdown(nil)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestSectionsToJSON_SingleSection(t *testing.T) {
	sections := []pdf.Section{
		{
			Text:       "测试内容",
			LayoutType: pdf.LayoutTypeText,
			DocTypeKwd: "text",
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 300, Top: 200, Bottom: 400}},
			Image:      "abc123",
		},
	}
	got := SectionsToJSON(sections)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	m := got[0]
	if m["text"] != "测试内容" {
		t.Errorf("text: got %v", m["text"])
	}
	if m["layout_type"] != "text" {
		t.Errorf("layout_type: got %v", m["layout_type"])
	}
	if m["doc_type_kwd"] != "text" {
		t.Errorf("doc_type_kwd: got %v", m["doc_type_kwd"])
	}
	if m["image"] != "abc123" {
		t.Errorf("image: got %v", m["image"])
	}
	positions, ok := m["_pdf_positions"].([][]any)
	if !ok || len(positions) != 1 {
		t.Fatalf("_pdf_positions not correct type/length: %T %v", m["_pdf_positions"], m["_pdf_positions"])
	}
	pos := positions[0]
	pages := pos[0].([]any)
	if len(pages) != 1 || pages[0] != 0 {
		t.Errorf("pages: got %v", pages)
	}
	if pos[1] != float64(50) || pos[2] != float64(300) || pos[3] != float64(200) || pos[4] != float64(400) {
		t.Errorf("coords: got %v", pos[1:])
	}
}

func TestSectionsToJSON_MultiPage(t *testing.T) {
	sections := []pdf.Section{
		{
			Text:      "跨页内容",
			Positions: []pdf.Position{{PageNumbers: []int{0, 1}, Left: 100, Right: 200, Top: 50, Bottom: 300}},
		},
	}
	got := SectionsToJSON(sections)
	positions := got[0]["_pdf_positions"].([][]any)
	pages := positions[0][0].([]any)
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 1 {
		t.Errorf("multi-page pages: got %v, want [0, 1]", pages)
	}
}

func TestSectionsToJSON_EmptySlice(t *testing.T) {
	got := SectionsToJSON([]pdf.Section{})
	if len(got) != 0 {
		t.Errorf("got %d items, want 0", len(got))
	}
}

func TestSectionsToJSON_NilSlice(t *testing.T) {
	got := SectionsToJSON(nil)
	if got == nil {
		t.Error("expected non-nil slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("got %d items, want 0", len(got))
	}
}

// TestCrossPageTableMerge verifies that mergeTablesAcrossPages merges
// two TableItems on consecutive pages with overlapping X positions.
// Python: _extract_table_figure merges cross-page tables by matching layoutno.
// Spanning cells should be annotated with colspan/rowspan in the HTML output.
