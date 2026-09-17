package layout

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ---------------------------------------------------------------------------
// Tunable thresholds.
// ---------------------------------------------------------------------------

const (
	// tocMaxProseRunes is the per-box length (in runes) above which a box is
	// treated as a prose paragraph. A page with substantial body text has many
	// such boxes; a TOC page has at most a title or a long chapter heading.
	// This is the primary guard against deleting content: any page with more
	// than tocMaxLongBoxes long boxes is never classified as TOC.
	tocMaxProseRunes = 60
	// tocMaxLongBoxes is the maximum number of boxes longer than
	// tocMaxProseRunes a page may carry and still be classified as TOC.
	tocMaxLongBoxes = 2
	// tocMinShortBoxes is the minimum number of short boxes a TOC page carries.
	// A TOC is a list, so its entries stay separate boxes; a page whose text
	// arrived as one merged block is not detected here and is covered by the
	// outline signal instead.
	tocMinShortBoxes = 4
	// tocMinEntries is the minimum number of confirming entry markers (chapter
	// markers plus page numbers) a page must carry to be classified as TOC.
	tocMinEntries = 3
	// tocMaxLeadPages bounds how many leading pages without body text may
	// precede the TOC. A cover, copyright page or frontispiece carries no body
	// text and is skipped, so a TOC that is not physically page one is still
	// reachable; the first page carrying body text ends the search.
	tocMaxLeadPages = 3
	// tocMaxTOCPages bounds how many consecutive pages one TOC may span.
	tocMaxTOCPages = 5
)

// ---------------------------------------------------------------------------
// Box-level patterns.
// ---------------------------------------------------------------------------

var (
	// chapterEntryPattern matches a box that opens like a TOC entry title: a
	// chapter/section/appendix marker (Chinese or Latin, digit or word form) or
	// a common front/back-matter heading.
	chapterEntryPattern = regexp.MustCompile(`(?i)^(第\s*[0-9０-９一二三四五六七八九十百千]+\s*[章节篇部册卷回讲目]|chapter\s+(\d+|[a-z]+|one|two|three|four|five|six|seven|eight|nine|ten)|appendix\s+[a-z0-9]+|section\s+\d+|part\s+(\d+|[ivxlcdm]+|[a-z]+)|前言|序言|引言|导论|绪论|后记|结语|附录|索引|参考文献|结论|致谢|跋|序|凡例|目录|acknowledgements?|acknowledgments?)`)
	// pageNumberPattern matches common page-number forms: pure digits, digits
	// with leading dots (..18), roman numerals (III, IV), or dash-wrapped
	// numbers (- 2 -). Full-width digits count as digits, matching the class
	// chapterEntryPattern uses for 第N章 numbering: the target corpus is largely
	// CJK, and a full-width page-number column still has to confirm a TOC page.
	pageNumberPattern = regexp.MustCompile(`^[.…]*[0-9０-９]+[.…]*$|^[.…]*[IVXLCDM]+[.…]*$|^\s*-\s*[0-9０-９]+\s*-\s*$`)
	// tocTitlePattern matches an outline title that names the table of contents.
	// It mirrors the section-level detector this pass replaces, which is why
	// "致谢"/"acknowledge" are accepted as end-of-TOC markers alongside the
	// heading itself.
	tocTitlePattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)
)

// ---------------------------------------------------------------------------
// Outline signal.
// ---------------------------------------------------------------------------

// TOCPageRangeFromOutlines returns the 0-based page indices the PDF bookmarks
// identify as the table of contents: from the outline entry naming the TOC up
// to — excluding — the next entry at the same level. It returns nil when the
// document carries no usable bookmark.
//
// Outline page numbers are 1-based — pdfium adds one to the 0-based destination
// index (internal/deepdoc/parser/pdf/pdfium/pdfium.go) — while
// TextBox.PageNumber is 0-based, because ParseRaw enumerates pages from zero.
// The conversion happens here, once, so no caller has to know about it.
func TOCPageRangeFromOutlines(outlines []pdf.Outline) map[int]bool {
	for i, o := range outlines {
		if !tocTitlePattern.MatchString(outlineTitle(o.Title)) {
			continue
		}
		first := o.PageNumber - 1
		if first < 0 {
			return nil
		}
		for _, next := range outlines[i+1:] {
			if next.Level != o.Level {
				continue
			}
			if tocTitlePattern.MatchString(outlineTitle(next.Title)) {
				continue
			}
			pages := make(map[int]bool, 4)
			for pg := first; pg < next.PageNumber-1; pg++ {
				pages[pg] = true
			}
			if len(pages) == 0 {
				return nil
			}
			return pages
		}
		// No following entry at the same level: the TOC page is known but its
		// extent is not, so claim only that page and leave the rest to the box
		// shape signal.
		return map[int]bool{first: true}
	}
	return nil
}

// outlineTitle strips the "@@" anchor suffix pdfium appends to bookmark titles
// and lower-cases the result, matching how the section-level detector compared.
func outlineTitle(title string) string {
	title = strings.TrimSpace(title)
	if idx := strings.Index(title, "@@"); idx >= 0 {
		title = strings.TrimSpace(title[:idx])
	}
	return strings.ToLower(title)
}

// ---------------------------------------------------------------------------
// TOC removal.
// ---------------------------------------------------------------------------

// RemoveTOCBoxes drops whole pages that are detected as tables of contents.
//
// Detection works on signals that are destroyed by the later TextMerge /
// BoxesToSections passes (leader dots get folded into adjacent boxes, per-entry
// geometry collapses into a section), so this MUST run before TextMerge (see
// Parser.buildLayout).
//
// Two signals select pages and the union is dropped:
//
//  1. outlinePages — the pages the PDF bookmarks identify as TOC (see
//     TOCPageRangeFromOutlines). Strongest, and the only one that still sees a
//     TOC whose entries were already merged into a single box.
//  2. Box shape — the leading run of pages that read as a TOC list: a page
//     carrying at least tocMinShortBoxes short boxes and tocMinEntries
//     confirming markers (chapter markers or page numbers), preceded only by
//     pages without body text and spanning at most tocMaxTOCPages pages. Kept
//     for documents that carry no usable bookmark.
//
// Both signals pass through one guard: a page carrying body text (more than
// tocMaxLongBoxes boxes longer than tocMaxProseRunes) is never dropped, and a
// document consisting only of TOC pages is left untouched. The detector stays
// deliberately conservative — missing a TOC page costs noise chunks, deleting a
// content page loses text.
func RemoveTOCBoxes(boxes []pdf.TextBox, outlinePages map[int]bool) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}

	perPage := make(map[int][]int, 64)
	for i := range boxes {
		p := boxes[i].PageNumber
		perPage[p] = append(perPage[p], i)
	}

	pages := make([]int, 0, len(perPage))
	for pg := range perPage {
		pages = append(pages, pg)
	}
	sort.Ints(pages)

	shapes := make(map[int]pageShape, len(pages))
	for _, pg := range pages {
		shapes[pg] = shapeOfPage(boxes, perPage[pg])
	}

	selected := make(map[int]bool, len(pages))
	for pg := range outlinePages {
		if _, ok := perPage[pg]; ok {
			selected[pg] = true
		}
	}

	// Heuristic fallback, for documents that carry no usable bookmark. A TOC is
	// a document prefix, so only the leading pages are candidates: pages without
	// body text (a cover, copyright page) may be skipped, one TOC may span
	// consecutive pages, and the first page carrying body text ends the search —
	// which is what keeps per-chapter pages that happen to hold several short
	// headings (the Daodejing case) out of scope.
	inTOC, lead, span := false, 0, 0
	for _, pg := range pages {
		if inTOC {
			if span >= tocMaxTOCPages || !shapes[pg].isTOC() {
				break
			}
			selected[pg] = true
			span++
			continue
		}
		if shapes[pg].isTOC() {
			inTOC, span = true, 1
			selected[pg] = true
			continue
		}
		if shapes[pg].carriesProse() || lead >= tocMaxLeadPages {
			break
		}
		lead++
	}

	drop := make(map[int]struct{}, len(boxes))
	droppedPages := 0
	for _, pg := range pages {
		if !selected[pg] || shapes[pg].carriesProse() {
			continue
		}
		droppedPages++
		for _, i := range perPage[pg] {
			drop[i] = struct{}{}
		}
	}
	if droppedPages == 0 || droppedPages == len(pages) {
		return boxes
	}

	out := make([]pdf.TextBox, 0, len(boxes)-len(drop))
	for i := range boxes {
		if _, ok := drop[i]; ok {
			continue
		}
		out = append(out, boxes[i])
	}
	return out
}

// pageShape summarises the boxes of one page.
type pageShape struct {
	prose   int // boxes longer than tocMaxProseRunes
	short   int
	markers int // short boxes opening with a chapter marker or holding a page number
}

func shapeOfPage(boxes []pdf.TextBox, indices []int) pageShape {
	var s pageShape
	for _, i := range indices {
		text := strings.TrimSpace(boxes[i].Text)
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > tocMaxProseRunes {
			s.prose++
			continue
		}
		s.short++
		if chapterEntryPattern.MatchString(text) || pageNumberPattern.MatchString(text) {
			s.markers++
		}
	}
	return s
}

// isTOC reports whether the page reads as a table of contents: a list of short
// boxes carrying entry markers, and no body text.
func (s pageShape) isTOC() bool {
	return !s.carriesProse() && s.short >= tocMinShortBoxes && s.markers >= tocMinEntries
}

// carriesProse reports whether the page carries body text. No signal may drop
// such a page: this is the single guard that bounds every removal.
func (s pageShape) carriesProse() bool { return s.prose > tocMaxLongBoxes }

// ---------------------------------------------------------------------------
// Running header / footer removal.
// ---------------------------------------------------------------------------

const (
	// headerZoneRatio / footerZoneRatio bound the top/bottom page zones where a
	// running header / footer lives (fractions of the page height).
	headerZoneRatio = 0.10
	footerZoneRatio = 0.90
	// minHeaderFooterPages guards against false positives on short documents.
	minHeaderFooterPages = 3
)

// RemoveHeaderFooterBoxes drops boxes that are running headers or footers.
//
// A box is a candidate when it sits entirely in the top headerZoneRatio or
// bottom footerZoneRatio of its page and carries text-like content. Among
// candidates, a (zone, normalized-text) pair that recurs on at least half of
// the document's pages is treated as a running header / footer and removed.
//
// Like RemoveTOCBoxes this works on intact box geometry and must run before
// TextMerge (which can fold a header box into the first body section).
func RemoveHeaderFooterBoxes(boxes []pdf.TextBox, pageHeights map[int]float64) []pdf.TextBox {
	if len(pageHeights) < minHeaderFooterPages || len(boxes) == 0 {
		return boxes
	}

	type zoneKey struct {
		zone string
		text string
	}
	keyPages := make(map[zoneKey]map[int]struct{})
	keyIdx := make(map[zoneKey][]int)

	for i := range boxes {
		b := boxes[i]
		// Only plain-text boxes participate; tables / figures in the margin are
		// not running headers/footers.
		if lt := strings.TrimSpace(b.LayoutType); lt != "" && lt != "text" {
			continue
		}
		h, ok := pageHeights[b.PageNumber]
		if !ok || h <= 0 {
			continue
		}
		top, bottom := b.Top, b.Bottom
		var zone string
		switch {
		case bottom <= h*headerZoneRatio:
			zone = "header"
		case top >= h*footerZoneRatio:
			zone = "footer"
		default:
			continue
		}
		norm := normalizeRunningText(b.Text)
		if norm == "" {
			continue
		}
		key := zoneKey{zone: zone, text: norm}
		if keyPages[key] == nil {
			keyPages[key] = make(map[int]struct{})
		}
		keyPages[key][b.PageNumber] = struct{}{}
		keyIdx[key] = append(keyIdx[key], i)
	}

	numPages := len(pageHeights)
	minPages := numPages / 2
	if minPages < 2 {
		minPages = 2
	}

	drop := make(map[int]struct{})
	for key, pages := range keyPages {
		if len(pages) < minPages {
			continue
		}
		for _, i := range keyIdx[key] {
			drop[i] = struct{}{}
		}
	}
	if len(drop) == 0 {
		return boxes
	}

	out := make([]pdf.TextBox, 0, len(boxes)-len(drop))
	for i := range boxes {
		if _, ok := drop[i]; ok {
			continue
		}
		out = append(out, boxes[i])
	}
	return out
}

// normalizeRunningText collapses whitespace, replaces digit runs with a single
// '#' and lower-cases the result, so per-page variants of the same running
// header/footer ("- 2 -", "- 3 -", "Page 4 of 9", "第 １ 页") share one
// comparison key.
func normalizeRunningText(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	out := make([]rune, 0, len(t))
	inDigit := false
	for _, r := range t {
		if isMaskableDigit(r) {
			inDigit = true
			continue
		}
		if inDigit {
			out = append(out, '#')
			inDigit = false
		}
		out = append(out, r)
	}
	if inDigit {
		out = append(out, '#')
	}
	return strings.ToLower(strings.TrimSpace(string(out)))
}

// isMaskableDigit reports whether r is a digit whose per-page value must be
// masked before running headers/footers are compared: ASCII and full-width
// decimal digits. Full width is required for the same reason pageNumberPattern
// accepts it — otherwise every page of "第 １ 页" / "第 ２ 页" gets its own key
// and the footer never reaches minPages.
func isMaskableDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= '０' && r <= '９')
}
