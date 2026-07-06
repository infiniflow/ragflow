package layout

import (
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// ResolvePageSpan computes the ending page and bottom coordinate for a box
// that may span multiple pages.  When pageHeights is nil or the box fits
// within its starting page the returned (toPage, bottom) equal the inputs.
//
// Zero or negative page heights are treated as invalid: the span stops at
// the preceding page, guarding against infinite loops caused by corrupted
// page images.
func ResolvePageSpan(pageNum int, bottom float64, pageHeights map[int]float64) (toPage int, newBottom float64) {
	toPage = pageNum
	newBottom = bottom
	if pageHeights == nil {
		return
	}
	ph, ok := pageHeights[pageNum]
	if !ok || ph <= 0 || bottom <= ph {
		return
	}
	remaining := bottom
	for remaining > ph && ph > 0 {
		nextPh, ok := pageHeights[toPage+1]
		if !ok || nextPh <= 0 {
			// Unknown or invalid next page height — extend by the
			// last known height once and stop (Python: _line_tag
			// while-loop break path).
			remaining -= ph
			toPage++
			break
		}
		remaining -= ph
		ph = nextPh
		toPage++
	}
	newBottom = remaining
	return
}

// boxesToSections converts layout boxes to section format with position tags.
//
// pageHeights provides the PDF-point height of each page (image height / zoom).
// Boxes that extend beyond their page produce multi-page position tags
// (Python's _line_tag while-loop detection via resolvePageSpan).
//
// Python equivalent: output consumed by naive.py::chunk()
func BoxesToSections(boxes []pdf.TextBox, pageHeights map[int]float64) []pdf.Section {
	sections := make([]pdf.Section, 0, len(boxes))
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		toPage, bottom := ResolvePageSpan(b.PageNumber, b.Bottom, pageHeights)

		var posTag string
		var pageNums []int
		if b.PageNumber == toPage {
			posTag = util.FormatPositionTag(b.PageNumber, b.X0, b.X1, b.Top, bottom)
			pageNums = []int{b.PageNumber}
		} else {
			posTag = util.FormatPositionTagRange(b.PageNumber, toPage, b.X0, b.X1, b.Top, bottom)
			pageNums = make([]int, 0, toPage-b.PageNumber+1)
			for p := b.PageNumber; p <= toPage; p++ {
				pageNums = append(pageNums, p)
			}
		}
		sections = append(sections, pdf.Section{
			Text:        t,
			PositionTag: posTag,
			LayoutType:  b.LayoutType,
			Positions:   []pdf.Position{{PageNumbers: pageNums, Left: b.X0, Right: b.X1, Top: b.Top, Bottom: bottom}},
		})
	}
	return sections
}

// NormalizeSectionPositions ensures each Section's Positions field is populated
// by parsing PositionTag when Positions is empty. Sections that already have
// Positions populated are left unchanged.
//
// This mirrors the Python normalize_pdf_items_metadata — canonicalizing
// position metadata from the string tag format into the typed []Position form.
//
// Callers should invoke this AFTER Parse() returns, just before consuming
// Sections (e.g., before serialization to JSON or passing to the chunker).
// The normalization is intentionally NOT embedded inside the parser pipeline
// because Sections may come from multiple sources (deepdoc, MinerU, Docling,
// JSON deserialization, etc.).
func NormalizeSectionPositions(sections []pdf.Section) {
	for i := range sections {
		if len(sections[i].Positions) == 0 && sections[i].PositionTag != "" {
			sections[i].Positions = util.ExtractPositions(sections[i].PositionTag)
		}
	}
}

// SortByPageThenY sorts boxes by page → vertical key → x0.
func SortByPageThenY(boxes []pdf.TextBox, sortByTop bool) {
	key := func(b pdf.TextBox) float64 { return b.Bottom }
	if sortByTop {
		key = func(b pdf.TextBox) float64 { return b.Top }
	}
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		if key(boxes[i]) != key(boxes[j]) {
			return key(boxes[i]) < key(boxes[j])
		}
		return boxes[i].X0 < boxes[j].X0
	})
}

// SectionsToMarkdown converts Sections to a markdown string.
//
// Title sections get a "## " prefix.
// Figure sections produce an "![Image](data:image/png;base64,...)" tag.
// Text and all other sections are appended verbatim.
//
// This mirrors the Python parser.py:665-671 markdown output path.
func SectionsToMarkdown(sections []pdf.Section) string {
	var b strings.Builder
	for _, s := range sections {
		if s.LayoutType == pdf.LayoutTypeTitle {
			b.WriteString("\n## ")
		}
		if s.LayoutType == pdf.LayoutTypeFigure && s.Image != "" {
			b.WriteString("\n![Image](data:image/png;base64,")
			b.WriteString(s.Image)
			b.WriteString(")")
			continue
		}
		b.WriteString(s.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// SectionsToJSON converts Sections to a Python-compatible JSON dict format.
//
// Each dict has keys: text, layout_type, doc_type_kwd, _pdf_positions, image.
// The _pdf_positions key mirrors Python's PDF_POSITIONS_KEY constant —
// the canonical position format consumed by the chunker's extract_pdf_positions.
//
// This mirrors the Python parser.py:662 set_output("json", bboxes) path.
func SectionsToJSON(sections []pdf.Section) []map[string]any {
	result := make([]map[string]any, len(sections))
	for i, s := range sections {
		positions := make([][]any, len(s.Positions))
		for j, p := range s.Positions {
			pages := make([]any, len(p.PageNumbers))
			for k, pn := range p.PageNumbers {
				pages[k] = pn
			}
			positions[j] = []any{pages, p.Left, p.Right, p.Top, p.Bottom}
		}
		result[i] = map[string]any{
			"text":           s.Text,
			"layout_type":    s.LayoutType,
			"doc_type_kwd":   s.DocTypeKwd,
			"_pdf_positions": positions,
			"image":          s.Image,
		}
	}
	return result
}
