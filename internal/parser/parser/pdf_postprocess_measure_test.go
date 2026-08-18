package parser

// Measurement / characterization tests for Borrow B (image-anchored reading
// order). These pin the CURRENT behavior of the adapter layer
// (pdf_postprocess.go) on pages where a figure spans one or both columns, so
// we can decide WHERE a figure-anchored reading-order fix (if any) must live.
//
// Key fact established during planning: AssignColumn's FinalReadingOrderMerge
// (column-major) is OVERWRITTEN by sortSectionsByPosition (page -> top -> left,
// run unconditionally at pdf_postprocess.go:29) and, when enableMultiColumn, by
// reorderPDFMultiColumn (left -> top). Both derive order from section geometry,
// not from ColID, and are already figure-aware. These tests verify that and
// surface the one residual limitation (a merged section whose vertical span
// straddles a figure is placed by its FIRST top).

import (
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func orderTexts(s []deepdoctype.Section) []string {
	out := make([]string, len(s))
	for i, sec := range s {
		out[i] = sec.Text
	}
	return out
}

func measureLayout() []deepdoctype.Section {
	// Page 0, two columns. A figure spans BOTH columns in the vertical middle.
	return []deepdoctype.Section{
		makePDFSection("col0-upper", "text", 0, 10, 290, 100, 120),
		makePDFSection("col1-upper", "text", 0, 310, 590, 100, 120),
		makePDFSection("figure", "figure", 0, 10, 590, 300, 500),
		makePDFSection("col0-lower", "text", 0, 10, 290, 700, 720),
		makePDFSection("col1-lower", "text", 0, 310, 590, 700, 720),
	}
}

// TestMeasure_FigureSpanningBothColumns_DefaultAdapter checks the unconditional
// sortSectionsByPosition path (enableMultiColumn=false). Expected: row-by-row
// interleave with the figure sitting between the upper and lower text bands.
func TestMeasure_FigureSpanningBothColumns_DefaultAdapter(t *testing.T) {
	result := &deepdoctype.ParseResult{Sections: measureLayout()}
	applyPDFPostProcess(result, pdfPostProcessOptions{})
	got := orderTexts(result.Sections)
	t.Logf("default adapter order: %v", got)

	figIdx := -1
	for i, s := range result.Sections {
		if s.Text == "figure" {
			figIdx = i
		}
	}
	if figIdx < 0 {
		t.Fatal("figure section missing")
	}
	// Every upper text must precede the figure; every lower text must follow.
	for _, name := range []string{"col0-upper", "col1-upper"} {
		for i, s := range result.Sections {
			if s.Text == name && i > figIdx {
				t.Errorf("%s (idx %d) is after figure (idx %d); want before", name, i, figIdx)
			}
		}
	}
	for _, name := range []string{"col0-lower", "col1-lower"} {
		for i, s := range result.Sections {
			if s.Text == name && i < figIdx {
				t.Errorf("%s (idx %d) is before figure (idx %d); want after", name, i, figIdx)
			}
		}
	}
}

// TestMeasure_FigureSpanningBothColumns_MultiColumnAdapter checks the
// enableMultiColumn path (reorderPDFMultiColumn: left -> top). Expected:
// column-major, with the figure correctly wrapped INSIDE column 0 (between
// col0-upper and col0-lower).
func TestMeasure_FigureSpanningBothColumns_MultiColumnAdapter(t *testing.T) {
	result := &deepdoctype.ParseResult{Sections: measureLayout()}
	applyPDFPostProcess(result, pdfPostProcessOptions{enableMultiColumn: true, pageWidth: 600, zoom: 1})
	got := orderTexts(result.Sections)
	t.Logf("multiColumn adapter order: %v", got)

	idx := func(name string) int {
		for i, s := range result.Sections {
			if s.Text == name {
				return i
			}
		}
		return -1
	}
	f := idx("figure")
	c0u, c0l := idx("col0-upper"), idx("col0-lower")
	if f < 0 || c0u < 0 || c0l < 0 {
		t.Fatal("expected sections missing")
	}
	if !(c0u < f && f < c0l) {
		t.Errorf("figure not wrapped inside col0: want col0-upper(%d) < figure(%d) < col0-lower(%d)", c0u, f, c0l)
	}
}

// TestMeasure_FigureStraddledByParagraph documents the ONE residual limitation:
// a single merged text section whose vertical span crosses the figure is sorted
// by its FIRST top, so it is placed entirely ABOVE the figure even though its
// body extends below. This is inherent to position-based sorting of already
// merged sections and is the main candidate a figure-anchored fix would target.
func TestMeasure_FigureStraddledByParagraph(t *testing.T) {
	// One long paragraph (left col) spanning top=100..bottom=800, a figure in
	// the middle (top 300..500), and the right column upper/lower.
	result := &deepdoctype.ParseResult{Sections: []deepdoctype.Section{
		makePDFSection("para-col0", "text", 0, 10, 290, 100, 800),
		makePDFSection("col1-upper", "text", 0, 310, 590, 100, 120),
		makePDFSection("figure", "figure", 0, 10, 590, 300, 500),
		makePDFSection("col1-lower", "text", 0, 310, 590, 700, 720),
	}}
	applyPDFPostProcess(result, pdfPostProcessOptions{})
	got := orderTexts(result.Sections)
	t.Logf("straddle case order: %v", got)
	// Current behavior: para-col0 (first top=100) precedes figure (top=300).
	idx := func(name string) int {
		for i, s := range result.Sections {
			if s.Text == name {
				return i
			}
		}
		return -1
	}
	if idx("para-col0") > idx("figure") {
		t.Errorf("paragraph not placed above figure as current sort does")
	}
}
