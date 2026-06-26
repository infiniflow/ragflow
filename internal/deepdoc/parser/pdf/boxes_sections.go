package parser

import (
	"sort"
	"strings"

	util "ragflow/internal/deepdoc/parser/pdf/util"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// resolvePageSpan computes the ending page and bottom coordinate for a box
// that may span multiple pages.  When pageHeights is nil or the box fits
// within its starting page the returned (toPage, bottom) equal the inputs.
//
// Zero or negative page heights are treated as invalid: the span stops at
// the preceding page, guarding against infinite loops caused by corrupted
// page images.
func resolvePageSpan(pageNum int, bottom float64, pageHeights map[int]float64) (toPage int, newBottom float64) {
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
func boxesToSections(boxes []pdf.TextBox, pageHeights map[int]float64) []pdf.Section {
	sections := make([]pdf.Section, 0, len(boxes))
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		toPage, bottom := resolvePageSpan(b.PageNumber, b.Bottom, pageHeights)

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

// mergeCaptions finds "figure caption" and "table caption" sections,
// appends their text to the nearest figure/table, then removes the
// caption sections.  Matches Python _extract_table_figure caption
// matching (pdf_parser.py:1196-1232).
// Also uses isCaptionBox to detect captions that DLA mislabeled as
// "text" — matching Python's is_caption(text) pattern matching.

// sortByPageThenY sorts boxes by page → vertical key → x0.
func sortByPageThenY(boxes []pdf.TextBox, sortByTop bool) {
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
