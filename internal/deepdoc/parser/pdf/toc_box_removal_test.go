package pdf

import (
	"strconv"
	"strings"
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

// fixtureHeader is the running header the combined fixture repeats. It carries
// no digits, so every page normalizes to the same comparison key.
const fixtureHeader = "BOOK TITLE"

// combinedRemovalEngine builds a document where both passes have something to
// do: pages 0 and 1 are TOC-shaped (five short boxes, four of them markers),
// pages 2-5 carry prose, and the same header repeats in the top zone on pages
// 0-3. Four of six pages is above the half-the-pages bar, so the header is
// removable only while the pages carrying it are still in the box set. Render
// height 1000 at DlaScale gives a 333pt page, which puts the header (bottom 5)
// inside the top 10% and the body lines (top 100) outside it.
func combinedRemovalEngine() *MockEngine {
	line := func(pg int, top float64, text string) pdf.TextChar {
		return pdf.TextChar{X0: 50, X1: 900, Top: top, Bottom: top + 12, Text: text, PageNumber: pg}
	}
	header := func(pg int) pdf.TextChar {
		return pdf.TextChar{X0: 50, X1: 200, Top: 1, Bottom: 5, Text: fixtureHeader, PageNumber: pg}
	}
	// Long enough to count as prose (> tocMaxProseRunes runes).
	const body = "This paragraph of body text is comfortably longer than sixty runes, so the page it sits on counts as prose."
	tocLine := func(pg int, top float64, text string) pdf.TextChar { return line(pg, top, text) }

	chars := map[int][]pdf.TextChar{}
	for pg := 0; pg < 6; pg++ {
		if pg < 2 {
			chars[pg] = []pdf.TextChar{
				header(pg),
				tocLine(pg, 100, "目录"),
				tocLine(pg, 130, "第一章 道可道"),
				tocLine(pg, 160, "第二章 天下皆知"),
				tocLine(pg, 190, "1"),
			}
			continue
		}
		page := []pdf.TextChar{line(pg, 100, body), line(pg, 130, body), line(pg, 160, body)}
		if pg < 4 {
			page = append([]pdf.TextChar{header(pg)}, page...)
		}
		chars[pg] = page
	}
	return &MockEngine{NumPages: 6, RenderW: 1000, RenderH: 1000, Chars: chars}
}

// TestParseRaw_HeaderFooterRemovesHeaderAlone is the control for the test below:
// the fixture's header really is removable, so a combined run that keeps it is
// reporting an interaction and not a header no run could remove.
func TestParseRaw_HeaderFooterRemovesHeaderAlone(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.RemoveHeaderFooter = true

	result, err := NewParser(cfg).ParseRaw(t.Context(), combinedRemovalEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if result.Metrics.BoxesHeaderFooterRemoved == 0 {
		t.Fatal("the repeated header must be removed when RemoveHeaderFooter is on")
	}
	for _, s := range result.Sections {
		if strings.Contains(s.Text, fixtureHeader) {
			t.Fatalf("header %q survived on its own", s.Text)
		}
	}
}

// TestParseRaw_TOCAndHeaderFooterBothApply pins the order the two passes run in.
// Header/footer removal counts cross-page repetition, and TOC removal deletes
// whole pages — so if TOC ran first it would take the header's evidence with it
// and leave the header on the pages that survive, making the two options
// together weaker than either one alone.
func TestParseRaw_TOCAndHeaderFooterBothApply(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.RemoveTOC = true
	cfg.RemoveHeaderFooter = true

	result, err := NewParser(cfg).ParseRaw(t.Context(), combinedRemovalEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if result.Metrics.BoxesTOCRemoved == 0 {
		t.Fatal("the TOC pages must still be dropped when both options are on")
	}
	if result.Metrics.BoxesHeaderFooterRemoved == 0 {
		t.Fatal("the running header must still be removed when both options are on")
	}
	for pg := 0; pg < 2; pg++ {
		if n := sectionsOnPage(result, pg); n != 0 {
			t.Fatalf("TOC page %d must be gone, %d sections still reference it", pg, n)
		}
	}
	for _, s := range result.Sections {
		if strings.Contains(s.Text, fixtureHeader) {
			t.Fatalf("header %q survived the combined run", s.Text)
		}
	}
	if n := sectionsOnPage(result, 4); n == 0 {
		t.Fatal("body page 4 must survive both passes")
	}
}

// rangeFooterEngine builds a 100-page document whose parsed range (pages 31-41,
// 1-based) carries a tight bare-number footer: Top is inside the sequence
// track's 80%+ band but above the 86% footer zone, so only the sequence track
// can remove it. DlaScale 3 turns a 1000px render into a 333pt page, putting
// the footer at 280pt.
func rangeFooterEngine() *MockEngine {
	const footerTop = 280.0
	chars := map[int][]pdf.TextChar{}
	for pg := 30; pg <= 40; pg++ {
		chars[pg] = []pdf.TextChar{
			{X0: 50, X1: 550, Top: 100, Bottom: 140, Text: "body text", PageNumber: pg},
			{X0: 280, X1: 340, Top: footerTop, Bottom: footerTop + 12, Text: strconv.Itoa(pg + 1), PageNumber: pg},
		}
	}
	return &MockEngine{NumPages: 100, RenderW: 1000, RenderH: 1000, Chars: chars}
}

// TestParseRaw_PageRangeHeaderFooterUsesDocumentPageCount pins that the
// page-number sequence track judges plausibility against the document's full
// page count, not the parsed subset. Before the fix len(pageHeights)==11 gave
// pageNumberCeiling==20, so the 31..41 footers were rejected as implausible and
// survived; with the engine page count (100) the +1 run is recognised.
func TestParseRaw_PageRangeHeaderFooterUsesDocumentPageCount(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.RemoveHeaderFooter = true
	cfg.Pages = [][]int{{31, 41}}

	result, err := NewParser(cfg).ParseRaw(t.Context(), rangeFooterEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if result.Metrics.BoxesHeaderFooterRemoved == 0 {
		t.Fatal("the tight page-number footer run must be removed when totalPages is the document size")
	}
	for _, s := range result.Sections {
		if strings.ContainsAny(s.Text, "0123456789") {
			t.Fatalf("a page number survived into section %q", s.Text)
		}
	}
}

// rangeTOCEngine builds a 20-page bookmarkless document whose TOC sits on pages
// 3-7 (1-based), i.e. 0-based 2-6: each TOC page carries a heading plus chapter
// markers and a page number, the layout the box-shape signal removes. The other
// pages carry prose, which no signal may drop.
func rangeTOCEngine() *MockEngine {
	line := func(pg int, top float64, text string) pdf.TextChar {
		return pdf.TextChar{X0: 50, X1: 550, Top: top, Bottom: top + 12, Text: text, PageNumber: pg}
	}
	const body = "This paragraph of body text is comfortably longer than sixty runes, so the page it sits on counts as prose."
	chars := map[int][]pdf.TextChar{}
	for pg := 0; pg < 20; pg++ {
		if pg >= 2 && pg <= 6 {
			chars[pg] = []pdf.TextChar{
				line(pg, 100, "目录"),
				line(pg, 160, "第一章 道可道"),
				line(pg, 190, "第二章 天下皆知"),
				line(pg, 220, "第三章 不尚贤"),
				line(pg, 250, "1"),
			}
			continue
		}
		chars[pg] = []pdf.TextChar{
			line(pg, 300, body),
			line(pg, 330, body),
			line(pg, 360, body),
		}
	}
	return &MockEngine{NumPages: 20, RenderW: 1000, RenderH: 1000, Chars: chars}
}

// TestParseRaw_PageRangeRemoveTOCDropsTOC pins that a no-bookmark TOC is removed
// when the selected page range contains it, even though the range does not start
// at the document's first page. Before the fix the box-shape signal was gated on
// pages[0]==0, so page 3's range start disabled it and the TOC survived.
func TestParseRaw_PageRangeRemoveTOCDropsTOC(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.RemoveTOC = true
	cfg.Pages = [][]int{{3, 7}}

	result, err := NewParser(cfg).ParseRaw(t.Context(), rangeTOCEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if result.Metrics.BoxesTOCRemoved == 0 {
		t.Fatal("a no-bookmark TOC inside the selected range must be removed")
	}
	for _, s := range result.Sections {
		if strings.Contains(s.Text, "目录") || strings.Contains(s.Text, "第一章") {
			t.Fatalf("TOC text survived into section %q", s.Text)
		}
	}

	// Control: a range that does not contain the TOC removes nothing.
	outside := pdf.DefaultParserConfig()
	outside.RemoveTOC = true
	outside.Pages = [][]int{{8, 10}}
	got, err := NewParser(outside).ParseRaw(t.Context(), rangeTOCEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw (outside range): %v", err)
	}
	if got.Metrics.BoxesTOCRemoved != 0 {
		t.Fatalf("a range excluding the TOC must remove nothing, removed %d", got.Metrics.BoxesTOCRemoved)
	}

	// Control: the option off keeps the TOC page.
	off := pdf.DefaultParserConfig()
	off.Pages = [][]int{{3, 7}}
	kept, err := NewParser(off).ParseRaw(t.Context(), rangeTOCEngine(), &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw (option off): %v", err)
	}
	if kept.Metrics.BoxesTOCRemoved != 0 {
		t.Fatalf("RemoveTOC is off, got %d boxes removed", kept.Metrics.BoxesTOCRemoved)
	}
}
