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
	// tocMinEntries is the minimum number of confirming entry markers (chapter
	// markers plus page numbers) a page must carry to be classified as TOC.
	tocMinEntries = 3
	// tocMaxCandidatePages is the number of non-blank pages scanned from the
	// front of the document. A TOC is always the very first page, so only the
	// first non-blank page is a candidate. Kept at 1 to avoid misclassifying
	// chapter pages that happen to have multiple short chapter headings (e.g.
	// the Daodejing's per-chapter pages) as TOC.
	tocMaxCandidatePages = 1
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
	// numbers (- 2 -).
	pageNumberPattern = regexp.MustCompile(`^[.…]*\d+[.…]*$|^[.…]*[IVXLCDM]+[.…]*$|^\s*-\s*\d+\s*-\s*$`)
)

// ---------------------------------------------------------------------------
// TOC removal.
// ---------------------------------------------------------------------------

// RemoveTOCBoxes drops whole pages that are detected as tables of contents.
//
// Detection works on box-level signals that are destroyed by the later
// TextMerge / BoxesToSections passes (leader dots get folded into adjacent
// boxes, per-entry geometry collapses into a section), so this MUST run before
// TextMerge (see Parser.buildLayout).
//
// The detector is deliberately conservative: it prefers missing a TOC page to
// deleting a content page. A page is classified as TOC only when ALL of the
// following hold:
//
//  1. Position: the page is one of the first tocMaxCandidatePages non-blank
//     pages (a TOC is always a document prefix).
//  2. No prose: the page carries at most tocMaxLongBoxes boxes longer than
//     tocMaxProseRunes. This is the strongest guard — any page with body text
//     is preserved.
//  3. Many entries: the page carries at least tocMinEntries confirming markers
//     (chapter markers or page numbers).
func RemoveTOCBoxes(boxes []pdf.TextBox) []pdf.TextBox {
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

	drop := make(map[int]struct{}, len(boxes))
	candidates := 0
	for _, pg := range pages {
		indices := perPage[pg]
		if isTOCPage(boxes, indices) {
			for _, i := range indices {
				drop[i] = struct{}{}
			}
		}
		// Count non-blank pages (those with enough boxes to matter).
		if len(indices) >= 4 {
			candidates++
			if candidates >= tocMaxCandidatePages {
				break
			}
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

// isTOCPage reports whether the boxes on a single page form a table of
// contents. All three conditions must hold: no substantial body text, enough
// short boxes, and enough confirming entry markers.
func isTOCPage(boxes []pdf.TextBox, indices []int) bool {
	longBoxes := 0
	shortBoxes := 0
	entries := 0
	for _, i := range indices {
		text := strings.TrimSpace(boxes[i].Text)
		if text == "" {
			continue
		}
		n := utf8.RuneCountInString(text)
		if n > tocMaxProseRunes {
			longBoxes++
			if longBoxes > tocMaxLongBoxes {
				return false
			}
			continue
		}
		shortBoxes++
		if chapterEntryPattern.MatchString(text) || pageNumberPattern.MatchString(text) {
			entries++
		}
	}
	return shortBoxes >= 4 && entries >= tocMinEntries
}

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

// normalizeRunningText collapses whitespace and replaces digit runs with a single
// '#' and lower-cases the result, so per-page variants of the same running
// header/footer ("- 2 -", "- 3 -", "Page 4 of 9") share one comparison key.
func normalizeRunningText(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	out := make([]rune, 0, len(t))
	inDigit := false
	for _, r := range t {
		if r >= '0' && r <= '9' {
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
