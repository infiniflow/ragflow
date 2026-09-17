package pdf

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// outlineEngine is a MockEngine that also reports bookmarks; the base stub
// hardcodes Outlines to nil.
type outlineEngine struct {
	MockEngine
	outlines []pdf.Outline
}

func (e *outlineEngine) Outlines() ([]pdf.Outline, error) { return e.outlines, nil }

// tocDocEngine builds a two-page document: page 0 is a TOC carrying two short
// lines, page 1 is body text. Two short boxes stay below tocMinShortBoxes, so
// the box signal cannot select page 0 and the bookmark is the only signal that
// can.
func tocDocEngine() *outlineEngine {
	return &outlineEngine{
		MockEngine: MockEngine{
			NumPages: 2,
			Chars: map[int][]pdf.TextChar{
				0: {
					{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "目录", PageNumber: 0},
					{X0: 50, X1: 550, Top: 300, Bottom: 312, Text: "第一章 道可道", PageNumber: 0},
				},
				1: {
					{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "body text", PageNumber: 1},
				},
			},
		},
		outlines: []pdf.Outline{
			{Title: "目录", Level: 0, PageNumber: 1},
			{Title: "第一章", Level: 0, PageNumber: 2},
		},
	}
}

func sectionsOnPage(result *pdf.ParseResult, pg int) int {
	n := 0
	for _, s := range result.Sections {
		for _, p := range s.Positions {
			for _, pn := range p.PageNumbers {
				if pn == pg {
					n++
				}
			}
		}
	}
	return n
}

// TestParseRaw_RemoveTOCDropsPageSelectedByOutline drives the option end to end:
// the bookmark names the TOC page, so its boxes must never reach the sections.
func TestParseRaw_RemoveTOCDropsPageSelectedByOutline(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.RemoveTOC = true

	result, err := NewParser(cfg).ParseRaw(t.Context(), tocDocEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if result.Metrics.BoxesTOCRemoved == 0 {
		t.Fatal("expected the bookmark-selected TOC page boxes to be removed")
	}
	if n := sectionsOnPage(result, 0); n != 0 {
		t.Fatalf("page 0 is the TOC and must be gone, %d sections still reference it", n)
	}
	if n := sectionsOnPage(result, 1); n == 0 {
		t.Fatal("body page 1 must survive TOC removal")
	}
}

// TestParseRaw_RemoveTOCOffKeepsPage is the control: the same document keeps its
// TOC page when the option is off.
func TestParseRaw_RemoveTOCOffKeepsPage(t *testing.T) {
	result, err := NewParser(pdf.DefaultParserConfig()).ParseRaw(t.Context(), tocDocEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if result.Metrics.BoxesTOCRemoved != 0 {
		t.Fatalf("RemoveTOC is off, got %d boxes removed", result.Metrics.BoxesTOCRemoved)
	}
	if n := sectionsOnPage(result, 0); n == 0 {
		t.Fatal("with RemoveTOC off the TOC page must stay")
	}
}
