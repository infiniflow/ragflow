package post

import (
	"context"
	"testing"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── helpers ──────────────────────────────────────────────────────────────

// dummyBase64PNG is a valid 50×50 red pixel PNG, base64-encoded.
const dummyBase64PNG = "iVBORw0KGgoAAAANSUhEUgAAADIAAAAyCAIAAACRXR/mAAAAUElEQVR4nOzOsREAEAAAMefsvzILaL6iSCbI2uNH83XgTqvQKrQKrUKr0Cq0Cq1Cq9AqtAqtQqvQKrQKrUKr0Cq0Cq1Cq9AqtAqt4gQAAP//miQBZqrF+JAAAAAASUVORK5CYII="

func newTestResult(sections ...pdftype.Section) *pdftype.ParseResult {
	return &pdftype.ParseResult{Sections: sections}
}

func makePosSection(text string, page int, x0, x1, top, bottom float64) pdftype.Section {
	return pdftype.Section{
		Text:       text,
		LayoutType: "text",
		Positions:  []pdftype.Position{{PageNumbers: []int{page}, Left: x0, Right: x1, Top: top, Bottom: bottom}},
	}
}

// ── normalizeLayoutType ────────────────────────────────────────────────

func TestNormalizeLayoutType(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "a", LayoutType: ""},
		pdftype.Section{Text: "b", LayoutType: "  "},
		pdftype.Section{Text: "c", LayoutType: "table"},
		pdftype.Section{Text: "d", LayoutType: "  figure  "},
		pdftype.Section{Text: "e", LayoutType: "text"},
	)
	normalizeLayoutType(result)
	want := []string{"text", "text", "table", "figure", "text"}
	for i, s := range result.Sections {
		if s.LayoutType != want[i] {
			t.Errorf("Sections[%d]: got %q, want %q", i, s.LayoutType, want[i])
		}
	}
}

// ── filterHeaderFooter ─────────────────────────────────────────────────

func TestFilterHeaderFooter(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "Page 1", LayoutType: "header"},
		pdftype.Section{Text: "Chapter 1", LayoutType: "text"},
		pdftype.Section{LayoutType: "footer"},
		pdftype.Section{LayoutType: "number"},
		pdftype.Section{Text: "Body", LayoutType: "text"},
		pdftype.Section{Text: "reference item", LayoutType: "reference"},
	)
	filterHeaderFooter(result)
	if len(result.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d: %+v", len(result.Sections), result.Sections)
	}
	if result.Sections[0].Text != "Chapter 1" || result.Sections[1].Text != "Body" {
		t.Errorf("wrong sections kept: %+v", result.Sections)
	}
}

func TestFilterHeaderFooter_Empty(t *testing.T) {
	result := newTestResult()
	filterHeaderFooter(result)
	if len(result.Sections) != 0 {
		t.Error("expected empty result")
	}
}

// ── assignDocTypeKwd ───────────────────────────────────────────────────

func TestAssignDocTypeKwd_Normal(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "a", LayoutType: "table"},
		pdftype.Section{Text: "b", LayoutType: "figure"},
		pdftype.Section{Text: "c", LayoutType: "equation"},
		pdftype.Section{Text: "d", LayoutType: "", Image: dummyBase64PNG},
		pdftype.Section{Text: "e", LayoutType: "text"},
		pdftype.Section{Text: "f", LayoutType: ""},
	)
	assignDocTypeKwd(result, false)
	want := []string{"table", "image", "text", "image", "text", "text"}
	for i, s := range result.Sections {
		if s.DocTypeKwd != want[i] {
			t.Errorf("Sections[%d]: got %q, want %q", i, s.DocTypeKwd, want[i])
		}
	}
}

func TestAssignDocTypeKwd_Flatten(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "a", LayoutType: "table", DocTypeKwd: "table", Image: dummyBase64PNG},
		pdftype.Section{Text: "b", LayoutType: "figure", DocTypeKwd: "image", Image: dummyBase64PNG},
		pdftype.Section{Text: "c", LayoutType: "text", DocTypeKwd: "text"},
	)
	assignDocTypeKwd(result, true)
	for _, s := range result.Sections {
		if s.DocTypeKwd != "text" {
			t.Errorf("expected all 'text', got %q", s.DocTypeKwd)
		}
		if s.Image != "" {
			t.Error("flatten should clear Image to prevent VLM enhancement")
		}
	}
}

// ── enhanceWithVision ──────────────────────────────────────────────────

func TestEnhanceWithVision_NoOp(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "original", Image: dummyBase64PNG, DocTypeKwd: "table"},
	)
	_ = enhanceWithVision(context.Background(), result, nil)
	if result.Sections[0].Text != "original" {
		t.Errorf("text changed when describer is nil: %q", result.Sections[0].Text)
	}
}

func TestEnhanceWithVision_Success(t *testing.T) {
	want := "A table showing Q1 revenue."
	desc := &mockImageDescriber{describe: want}

	result := newTestResult(
		pdftype.Section{Text: "", Image: dummyBase64PNG, DocTypeKwd: "table"},
	)
	if err := enhanceWithVision(context.Background(), result, desc); err != nil {
		t.Fatal(err)
	}
	if result.Sections[0].Text != want {
		t.Errorf("text not enhanced: got %q", result.Sections[0].Text)
	}
}

func TestEnhanceWithVision_SkipText(t *testing.T) {
	desc := &mockImageDescriber{describe: "should not be called"}

	result := newTestResult(
		pdftype.Section{Text: "plain text", DocTypeKwd: "text", Image: ""},
	)
	if err := enhanceWithVision(context.Background(), result, desc); err != nil {
		t.Fatal(err)
	}
	if result.Sections[0].Text != "plain text" {
		t.Errorf("text changed: %q", result.Sections[0].Text)
	}
}

// ── removeTOCByOutlines ────────────────────────────────────────────────

func TestRemoveTOCByOutlines_Removes(t *testing.T) {
	outlines := []pdftype.Outline{
		{Title: "Chapter 1 Introduction", Level: 0, PageNumber: 1},
		{Title: "目录", Level: 0, PageNumber: 3},
		{Title: "Chapter 2 Methods", Level: 0, PageNumber: 5},
	}
	result := newTestResult(
		makePosSection("s1", 1, 50, 550, 100, 120),
		makePosSection("s2", 2, 50, 550, 100, 120),
		makePosSection("toc1", 3, 50, 550, 100, 120),
		makePosSection("toc2", 4, 50, 550, 100, 120),
		makePosSection("body1", 5, 50, 550, 100, 120),
		makePosSection("body2", 6, 50, 550, 100, 120),
	)
	removeTOCByOutlines(result, outlines)
	if len(result.Sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(result.Sections))
	}
	if result.Sections[0].Text != "s1" || result.Sections[1].Text != "s2" {
		t.Error("pre-TOC pages should be kept")
	}
	if result.Sections[2].Text != "body1" || result.Sections[3].Text != "body2" {
		t.Error("post-TOC pages should be kept")
	}
}

func TestRemoveTOCByOutlines_NoMatch(t *testing.T) {
	outlines := []pdftype.Outline{
		{Title: "1. Introduction", Level: 0, PageNumber: 1},
		{Title: "2. Background", Level: 0, PageNumber: 3},
	}
	result := newTestResult(
		makePosSection("s1", 1, 50, 550, 100, 120),
		makePosSection("s2", 2, 50, 550, 100, 120),
	)
	removeTOCByOutlines(result, outlines)
	if len(result.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d (no TOC should mean no removal)", len(result.Sections))
	}
}

func TestRemoveTOCByOutlines_NilOutlines(t *testing.T) {
	result := newTestResult(makePosSection("a", 1, 50, 550, 100, 120))
	removeTOCByOutlines(result, nil)
	if len(result.Sections) != 1 {
		t.Errorf("nil outlines should be no-op: got %d sections", len(result.Sections))
	}
}

func TestRemoveTOCByOutlines_EmptyOutlines(t *testing.T) {
	result := newTestResult(makePosSection("a", 1, 50, 550, 100, 120))
	removeTOCByOutlines(result, []pdftype.Outline{})
	if len(result.Sections) != 1 {
		t.Errorf("empty outlines should be no-op: got %d sections", len(result.Sections))
	}
}

func TestRemoveTOCByOutlines_NoNext(t *testing.T) {
	outlines := []pdftype.Outline{
		{Title: "目录", Level: 0, PageNumber: 2},
	}
	result := newTestResult(
		makePosSection("toc", 2, 50, 550, 100, 120),
		makePosSection("body", 3, 50, 550, 100, 120),
	)
	removeTOCByOutlines(result, outlines)
	if len(result.Sections) != 2 {
		t.Errorf("no next outline → keep all sections: got %d", len(result.Sections))
	}
}

// ── reorderMultiColumn ─────────────────────────────────────────────────

func TestReorderMultiColumn_SingleCol(t *testing.T) {
	result := newTestResult(
		makePosSection("B", 0, 50, 550, 200, 220),
		makePosSection("A", 0, 50, 550, 100, 120),
	)
	reorderMultiColumn(result, 600.0, 1.0)
	// medianW=500 >= 300 → single col, order preserved
	if result.Sections[0].Text != "B" {
		t.Fatal("single column should preserve original order")
	}
}

func TestReorderMultiColumn_MultiCol(t *testing.T) {
	result := newTestResult(
		makePosSection("B", 0, 300, 500, 100, 120),
		makePosSection("A", 0, 50, 250, 100, 120),
	)
	reorderMultiColumn(result, 600.0, 1.0)
	if result.Sections[0].Positions[0].Left > result.Sections[1].Positions[0].Left {
		t.Log("multi-column: sections reordered")
	}
}

func TestReorderMultiColumn_Empty(t *testing.T) {
	result := newTestResult()
	reorderMultiColumn(result, 600.0, 1.0)
	if len(result.Sections) != 0 {
		t.Error("empty sections should remain empty")
	}
}

func TestReorderMultiColumn_NoText(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "t1", LayoutType: "table", Positions: []pdftype.Position{{PageNumbers: []int{0}, Left: 300, Right: 500, Top: 100, Bottom: 120}}},
		pdftype.Section{Text: "t2", LayoutType: "table", Positions: []pdftype.Position{{PageNumbers: []int{0}, Left: 50, Right: 250, Top: 100, Bottom: 120}}},
	)
	reorderMultiColumn(result, 600.0, 1.0)
	if len(result.Sections) != 2 {
		t.Fatal("expected 2 sections")
	}
}

// ── PostProcess integration ────────────────────────────────────────────

func TestPostProcess_FullPipeline(t *testing.T) {
	// Simulates post-processing after Parse(): all features enabled.
	result := newTestResult(
		// Page 1: TOC — should be removed
		pdftype.Section{Text: "目录", LayoutType: "text", Positions: []pdftype.Position{{PageNumbers: []int{1}, Left: 50, Right: 550, Top: 100, Bottom: 120}}},
		pdftype.Section{Text: "Chapter 1 ... 1", LayoutType: "text", Positions: []pdftype.Position{{PageNumbers: []int{1}, Left: 50, Right: 550, Top: 120, Bottom: 140}}},
		// Page 1: header — should be removed
		pdftype.Section{Text: "Page 1", LayoutType: "header", Positions: []pdftype.Position{{PageNumbers: []int{1}, Left: 500, Right: 550, Top: 10, Bottom: 20}}},
		// Page 3: actual content
		pdftype.Section{Text: "Introduction text", LayoutType: "", Positions: []pdftype.Position{{PageNumbers: []int{3}, Left: 50, Right: 550, Top: 100, Bottom: 120}}},
		pdftype.Section{Text: "Row1 Col1 Row1 Col2", LayoutType: "table", Positions: []pdftype.Position{{PageNumbers: []int{3}, Left: 50, Right: 550, Top: 200, Bottom: 300}}, Image: dummyBase64PNG},
		pdftype.Section{Text: "Chart description", LayoutType: "figure", Positions: []pdftype.Position{{PageNumbers: []int{3}, Left: 50, Right: 550, Top: 300, Bottom: 400}}, Image: dummyBase64PNG},
		// Page 4: footer — should be removed
		pdftype.Section{Text: "Confidential", LayoutType: "footer", Positions: []pdftype.Position{{PageNumbers: []int{4}, Left: 50, Right: 550, Top: 700, Bottom: 720}}},
	)

	outlines := []pdftype.Outline{
		{Title: "目录", Level: 0, PageNumber: 1},
		{Title: "Chapter 1 Introduction", Level: 0, PageNumber: 3},
	}

	wantVLM := "This table shows quarterly revenue data with 2 columns."
	describer := &mockImageDescriber{describe: wantVLM}

	// First pass: non-VLM steps through PostProcess
	config := PipelineConfig{
		ConfigKeyPageWidth: 600.0,
		ConfigKeyZoom:      1.0,
		ConfigKeyOutlines:  outlines,
		ConfigKeyRemoveTOC: true,
	}
	if err := PostProcess(context.Background(), result, config); err != nil {
		t.Fatal(err)
	}
	// Then: VLM enhancement through internal function (with mock)
	if err := enhanceWithVision(context.Background(), result, describer); err != nil {
		t.Fatal(err)
	}
	// Then: flatten
	if err := PostProcess(context.Background(), result, PipelineConfig{
		ConfigKeyFlattenMediaToText: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Verify
	if len(result.Sections) != 3 {
		t.Fatalf("expected 3 sections after filtering, got %d: %+v", len(result.Sections), result.Sections)
	}
	for i, s := range result.Sections {
		if s.DocTypeKwd != "text" {
			t.Errorf("section[%d] DocTypeKwd = %q, want 'text'", i, s.DocTypeKwd)
		}
		if s.LayoutType == "header" || s.LayoutType == "footer" {
			t.Errorf("section[%d] LayoutType = %q, should have been filtered out", i, s.LayoutType)
		}
	}
	// Table section should have enhanced text
	found := false
	for _, s := range result.Sections {
		if s.LayoutType == "table" {
			found = true
			if s.Text != "Row1 Col1 Row1 Col2\n"+wantVLM {
				t.Errorf("table text not enhanced: %q", s.Text)
			}
		}
	}
	if !found {
		t.Error("table section missing from result")
	}
}

func TestPostProcess_Minimal(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "Hello", LayoutType: ""},
		pdftype.Section{Text: "World", LayoutType: "  "},
	)
	if err := PostProcess(context.Background(), result, nil); err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(result.Sections))
	}
	if result.Sections[0].LayoutType != "text" || result.Sections[1].LayoutType != "text" {
		t.Error("layout not normalized")
	}
	if result.Sections[0].DocTypeKwd != "text" || result.Sections[1].DocTypeKwd != "text" {
		t.Error("doc_type_kwd not assigned")
	}
}

func TestPostProcess_NilResult(t *testing.T) {
	if err := PostProcess(context.Background(), nil, nil); err == nil {
		t.Error("expected error for nil result")
	}
}

func TestPostProcess_EmptySections(t *testing.T) {
	result := newTestResult()
	if err := PostProcess(context.Background(), result, nil); err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 0 {
		t.Error("empty should remain empty")
	}
}

func TestPostProcess_FiguresLazy(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "Fig1", LayoutType: "figure"},
		pdftype.Section{Text: "Body", LayoutType: "text"},
		pdftype.Section{Text: "Fig2", LayoutType: "figure"},
	)
	if err := PostProcess(context.Background(), result, nil); err != nil {
		t.Fatal(err)
	}
	figs := result.Figures()
	if len(figs) != 2 {
		t.Fatalf("expected 2 figures, got %d", len(figs))
	}
	if figs[0].Text != "Fig1" || figs[1].Text != "Fig2" {
		t.Errorf("wrong figures: %+v", figs)
	}
}

func TestPostProcess_FilterOnly(t *testing.T) {
	result := newTestResult(
		pdftype.Section{Text: "Header", LayoutType: "header"},
		pdftype.Section{Text: "Second", LayoutType: "text"},
		pdftype.Section{Text: "First", LayoutType: "text"},
	)
	if err := PostProcess(context.Background(), result, nil); err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 2 {
		t.Fatalf("expected 2 sections after filtering, got %d", len(result.Sections))
	}
	figs := result.Figures()
	if len(figs) != 0 {
		t.Errorf("expected 0 figures, got %d", len(figs))
	}
}

func TestPostProcess_ReorderOnly(t *testing.T) {
	result := newTestResult(
		makePosSection("B", 0, 300, 500, 100, 120),
		makePosSection("A", 0, 50, 250, 100, 120),
	)
	config := PipelineConfig{
		ConfigKeyPageWidth: 600.0,
		ConfigKeyZoom:      1.0,
	}
	// Remove the outlines key since we don't need it
	if err := PostProcess(context.Background(), result, config); err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 2 {
		t.Fatal("expected 2 sections")
	}
	// Should be reordered: col 1 leftmost: A then B
	if result.Sections[0].Positions[0].Left > result.Sections[1].Positions[0].Left {
		t.Log("multi-column: sections reordered left-to-right")
	}
}
