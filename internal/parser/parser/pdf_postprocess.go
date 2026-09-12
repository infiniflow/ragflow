package parser

import (
	"math"
	"regexp"
	"sort"
	"strings"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// Substring match to mirror Python's remove_header_footer:
// re.search(r"(header|footer|number)", raw_layout, re.I) (rag/flow/parser/parser.py:754).
// Python matches any layout type CONTAINING one of these words, not just the
// exact token, so a composite label like "page-footer" is also stripped.
var pdfHeaderFooterPattern = regexp.MustCompile(`(?i)header|footer|number`)
var pdfTOCTitlePattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)

// pdfTOCEntryPattern matches table-of-contents entry lines: a leader run
// (dots, ellipses, midline ellipses, or middots) followed by a
// page number, optionally with a trailing roman-numeral fragment, e.g.
// "第1章 道可道…………3", "Introduction ....... 12", "……18 I". It extends the
// leader heuristic Python already uses in remove_toc_word
// (rag/flow/parser/utils.py) to the PDF path on both sides, so a TOC without
// a "目录"/"contents" heading is still dropped. Wide-space runs are NOT a
// leader here, mirroring pdfTOCEntryAnywherePattern: they are too common in
// prose, so a line merely ending in "  12" is not a TOC entry.
var pdfTOCEntryPattern = regexp.MustCompile(`(\.{2,}|…{2,}|⋯{2,}|·{2,})\s*\d{1,4}\s*[IVXLCivxlc]{0,5}\s*$`)

// pdfTOCEntryAnywherePattern is the non-anchored form of pdfTOCEntryPattern.
// DeepDoc merges the lines of a text block into ONE section whose text joins
// the lines with spaces, so a TOC page usually becomes a single section whose
// trailing text may be a page marker or a footer watermark instead of an
// entry — the anchored pattern alone never fires for such a section. Counting
// explicit leader runs (wide-space leaders excluded, they are too common in
// prose) identifies those merged TOC blocks.
var pdfTOCEntryAnywherePattern = regexp.MustCompile(`(\.{2,}|…{2,}|⋯{2,}|·{2,})\s*\d{1,4}\s*[IVXLCivxlc]{0,5}`)

// pdfTOCBarePageRefPattern matches a line that consists solely of a leader
// run plus a page number (and optional roman-numeral fragment), e.g.
// "…………………39", "………18 I". In wrapped TOCs the entry title and its leader/page
// number are split onto separate lines, leaving such a bare reference behind
// the title line.
var pdfTOCBarePageRefPattern = regexp.MustCompile(`^[\s.·…⋯]*\d{1,4}\s*[IVXLCivxlc]{0,5}[\s.。·…⋯]*$`)

// pdfTOCTitlePrefixPattern matches lines that open like a TOC entry title:
// a chapter/section marker or a common front/back-matter heading. Used only
// to pair a title line with the bare leader+page-number line that follows it,
// so regular prose is never dropped.
var pdfTOCTitlePrefixPattern = regexp.MustCompile(`(?i)^(第\s*[0-9０-９一二三四五六七八九十百千]+\s*[章节篇部册卷回讲]|chapter\s+[0-9]+|appendix\s+[a-z0-9]+|section\s+[0-9]+|前言|序言|引言|导论|绪论|后记|结语|附录|索引|参考文献|结论|致谢|acknowledgements?|acknowledgments?)`)

type pdfPostProcessOptions struct {
	outputFormat       string
	pageWidth          float64
	zoom               float64
	enableMultiColumn  bool
	flattenMediaToText bool
	removeTOC          bool
	removeHeaderFooter bool
}

func applyPDFPostProcess(result *deepdoctype.ParseResult, opts pdfPostProcessOptions) {
	if result == nil {
		return
	}
	sortSectionsByPosition(result)
	if opts.enableMultiColumn && opts.pageWidth > 0 {
		reorderPDFMultiColumn(result, opts.pageWidth, opts.zoom)
	}
	if opts.removeTOC {
		applyRemoveTOC(result)
	}
	normalizePDFLayoutTypes(result)
	if opts.removeHeaderFooter {
		filterPDFHeaderFooter(result)
		removePDFRunningHeaderFooter(result)
	}
	assignPDFDocTypeKeywords(result, opts.flattenMediaToText)
}

func normalizePDFLayoutTypes(result *deepdoctype.ParseResult) {
	for i := range result.Sections {
		layoutType := strings.TrimSpace(result.Sections[i].LayoutType)
		if layoutType == "" {
			layoutType = deepdoctype.LayoutTypeText
		}
		result.Sections[i].LayoutType = layoutType
	}
}

func filterPDFHeaderFooter(result *deepdoctype.ParseResult) {
	filtered := result.Sections[:0]
	for _, s := range result.Sections {
		if pdfHeaderFooterPattern.MatchString(strings.TrimSpace(s.LayoutType)) {
			continue
		}
		filtered = append(filtered, s)
	}
	result.Sections = filtered
}

const (
	// pdfHeaderZoneRatio / pdfFooterZoneRatio bound the page zones where
	// running headers/footers live: sections sitting entirely in the top
	// or bottom fraction of a page are candidates.
	pdfHeaderZoneRatio = 0.10
	pdfFooterZoneRatio = 0.90
	// pdfMinPagesForHeaderFooter guards against false positives on short
	// documents: cross-page repetition only means "running header/footer"
	// when the document has enough pages.
	pdfMinPagesForHeaderFooter = 3
)

var pdfRunningDigitPattern = regexp.MustCompile(`\d+`)

// normalizePDFRunningText collapses whitespace and replaces digit runs so
// that per-page variants of the same running header/footer ("- 2 -",
// "- 3 -", "Page 4 of 9") share one comparison key.
func normalizePDFRunningText(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	t = pdfRunningDigitPattern.ReplaceAllString(t, "#")
	return strings.ToLower(strings.TrimSpace(t))
}

// removePDFRunningHeaderFooter drops sections whose normalized text repeats
// in the same page zone (top 10% or bottom 10%) on at least half of the
// document's pages — running headers, footers, and page numbers. Every
// rendered page counts toward the total, including blank or image-only
// pages that produced no sections. The
// layout-type filter above only works when a DeepDoc inference service
// (DEEPDOC_URL) types header/footer regions; in default deployments there
// is none, every section stays "text", and 页眉页脚 removal silently does
// nothing. This positional frequency pass makes the option work without an
// inference service. Repetition — not position alone — is the guard that
// keeps genuine body text: a section must recur on >= max(2, pages/2) pages
// in the same zone to be removed.
func removePDFRunningHeaderFooter(result *deepdoctype.ParseResult) {
	sections := result.Sections
	if len(sections) == 0 || len(result.PageHeight) == 0 {
		return
	}
	numPages := len(result.PageHeight)
	if numPages < pdfMinPagesForHeaderFooter {
		return
	}
	type zoneKey struct {
		header bool
		text   string
	}
	keyPages := make(map[zoneKey]map[int]struct{})
	keySections := make(map[zoneKey][]int)
	for i := range sections {
		s := &sections[i]
		if layout := strings.TrimSpace(s.LayoutType); layout != "" && layout != deepdoctype.LayoutTypeText {
			continue
		}
		if len(s.Positions) == 0 || len(s.Positions[0].PageNumbers) == 0 {
			continue
		}
		page := s.Positions[0].PageNumbers[0]
		pageHeight := result.PageHeight[page]
		if pageHeight <= 0 {
			continue
		}
		top, bottom := s.Positions[0].Top, s.Positions[0].Bottom
		if bottom <= top {
			continue
		}
		var isHeader, isFooter bool
		switch {
		case bottom <= pageHeight*pdfHeaderZoneRatio:
			isHeader = true
		case top >= pageHeight*pdfFooterZoneRatio:
			isFooter = true
		}
		if !isHeader && !isFooter {
			continue
		}
		norm := normalizePDFRunningText(s.Text)
		if norm == "" {
			continue
		}
		key := zoneKey{header: isHeader, text: norm}
		if keyPages[key] == nil {
			keyPages[key] = make(map[int]struct{})
		}
		keyPages[key][page] = struct{}{}
		keySections[key] = append(keySections[key], i)
	}
	drop := make(map[int]struct{})
	for key, pages := range keyPages {
		n := len(pages)
		if n < 2 || n*2 < numPages {
			continue
		}
		for _, i := range keySections[key] {
			drop[i] = struct{}{}
		}
	}
	if len(drop) == 0 {
		return
	}
	filtered := sections[:0]
	for i, s := range sections {
		if _, ok := drop[i]; ok {
			continue
		}
		filtered = append(filtered, s)
	}
	result.Sections = filtered
}

func assignPDFDocTypeKeywords(result *deepdoctype.ParseResult, flatten bool) {
	for i := range result.Sections {
		section := &result.Sections[i]
		if flatten {
			section.DocTypeKwd = "text"
			continue
		}
		switch strings.TrimSpace(section.LayoutType) {
		case deepdoctype.LayoutTypeTable:
			section.DocTypeKwd = "table"
		case deepdoctype.LayoutTypeFigure:
			section.DocTypeKwd = "image"
		default:
			// doc_type_kwd is derived from layout, not from whether a
			// section image was cropped. Cropping happens lazily at
			// Markdown serialization / chunk time, so it must not
			// influence classification here (otherwise every positioned
			// text box would be mislabeled "image").
			section.DocTypeKwd = "text"
		}
	}
}

// sortSectionsByPosition reorders sections into reading order: page number,
// then vertical position (top), then horizontal position (left). The DeepDoc
// layout engine does not guarantee reading order in its output, so this sort
// ensures the downstream chunker receives items in document order regardless
// of the engine's internal extraction sequence.
func sortSectionsByPosition(result *deepdoctype.ParseResult) {
	if result == nil || len(result.Sections) < 2 {
		return
	}
	sort.SliceStable(result.Sections, func(i, j int) bool {
		pi, pj := firstSectionPage(result.Sections[i]), firstSectionPage(result.Sections[j])
		if pi != pj {
			return pi < pj
		}
		ti, tj := firstSectionTop(result.Sections[i]), firstSectionTop(result.Sections[j])
		if math.Abs(ti-tj) > 1e-6 {
			return ti < tj
		}
		return firstSectionLeft(result.Sections[i]) < firstSectionLeft(result.Sections[j])
	})
}

// applyRemoveTOC mirrors Python parser.py:663-681 three-way dispatch:
//   - No outlines → pattern-based remove_toc on all sections
//   - First outline on page 1 → outline-based remove_toc_pdf, then the
//     fragment-page pass and the entry filter: the outline pass only drops
//     pages when an outline title names the TOC ("目录"/"contents"), so a
//     headingless TOC must still run through both instead of being left
//     in place
//   - First outline after page 1 → pattern-based on pages before the first outline
func applyRemoveTOC(result *deepdoctype.ParseResult) {
	if result == nil {
		return
	}
	outlines := result.Outlines
	if len(outlines) == 0 {
		removePDFTOC(result)
		return
	}
	firstOutlinePage := outlines[0].PageNumber
	if firstOutlinePage <= 1 {
		removePDFTOCByOutlines(result, outlines)
		repeated := pdfTOCRepeatedLines(result.Sections)
		result.Sections = filterPDFTOCFragmentPagesWithRepeated(result.Sections, repeated)
		result.Sections = filterPDFTOCEntries(result.Sections)
		return
	}
	splitAt := len(result.Sections)
	for i, s := range result.Sections {
		if firstSectionPage(s) >= firstOutlinePage {
			splitAt = i
			break
		}
	}
	beforeSplit := &deepdoctype.ParseResult{Sections: result.Sections[:splitAt]}
	removePDFTOC(beforeSplit)
	result.Sections = append(beforeSplit.Sections, result.Sections[splitAt:]...)
}

func removePDFTOC(result *deepdoctype.ParseResult) {
	sections := result.Sections
	i := 0
	for i < len(sections) {
		text := sectionText(sections[i])
		if !pdfTOCTitlePattern.MatchString(strings.ToLower(strings.TrimSpace(text))) {
			i++
			continue
		}
		sections = append(sections[:i], sections[i+1:]...)
		if i >= len(sections) {
			break
		}
		prefix := sectionTextPrefix(sections[i], 3)
		for prefix == "" {
			sections = append(sections[:i], sections[i+1:]...)
			if i >= len(sections) {
				break
			}
			prefix = sectionTextPrefix(sections[i], 3)
		}
		if i >= len(sections) || prefix == "" {
			break
		}
		// The first entry is formatted (leader run plus page number): the
		// table is entry-shaped and the prefix scan below must not run —
		// with a single formatted entry that scan would delete body text up
		// to the next line sharing the entry's prefix. Drop the entry and
		// resume the title scan; filterPDFTOCEntries removes the remaining
		// entry lines.
		if isPDFTOCEntrySection(sectionText(sections[i])) {
			sections = append(sections[:i], sections[i+1:]...)
			continue
		}
		sections = append(sections[:i], sections[i+1:]...)
		if i >= len(sections) || prefix == "" {
			break
		}
		for j := i; j < len(sections) && j < i+128; j++ {
			if !strings.HasPrefix(sectionText(sections[j]), prefix) {
				continue
			}
			sections = append(sections[:i], sections[j:]...)
			break
		}
	}
	// The anchored pass above needs a heading section; a TOC whose pages
	// carry no such heading survives it entirely. Build the repetition
	// table from the raw stream first: the passes below delete in place
	// and would otherwise strip the page fragments some keys need to
	// reach threshold. Classify fragmented pages before entry-line
	// filtering for the same reason: the entry filter can strip the
	// page-number fragments the classifier counts on.
	repeated := pdfTOCRepeatedLines(sections)
	sections = filterPDFTOCFragmentPagesWithRepeated(sections, repeated)
	// Drop any remaining leader+page-number entry lines, mirroring
	// remove_toc in rag/flow/parser/utils.py.
	sections = filterPDFTOCEntries(sections)
	result.Sections = sections
}

// filterPDFTOCEntries removes table-of-contents sections from the stream,
// reusing the backing array. It goes beyond the per-item anchored match that
// Python's remove_toc (rag/flow/parser/utils.py) applies, because the Go
// DeepDoc layout merges a whole TOC block into one section and wrapped TOCs
// split an entry's leader/page number onto its own line:
//
//   - a section ending in a leader+page-number run is an entry (anchored
//     pattern), as before;
//   - a section containing two or more leader+page-number runs is a merged
//     TOC block even when its trailing text (page marker, roman numeral or
//     footer watermark) is not an entry itself;
//   - when a dropped section is a bare leader+page-number reference, the
//     preceding kept section is dropped too if it opens like a TOC entry
//     title ("第28章 ………" followed by "………39"), pairing the wrapped entry.
//
// Regular prose is kept: a lone leader+number run inside a paragraph matches
// neither the anchored pattern nor the two-run threshold, and title pairing
// only fires on a dropped bare page reference.
func filterPDFTOCEntries(sections []deepdoctype.Section) []deepdoctype.Section {
	filtered := make([]deepdoctype.Section, 0, len(sections))
	for _, s := range sections {
		text := sectionText(s)
		if !isPDFTOCEntrySection(text) {
			filtered = append(filtered, s)
			continue
		}
		if pdfTOCBarePageRefPattern.MatchString(text) && len(filtered) > 0 {
			if isPDFTOCTitleCandidate(sectionText(filtered[len(filtered)-1])) {
				filtered = filtered[:len(filtered)-1]
			}
		}
	}
	return filtered
}

// pdfTOCFragmentThresholds bound page-level TOC detection: a page is a
// fragmented TOC page only when it carries at least this many title-like
// and page-number-like fragments and no body-length prose.
const (
	pdfTOCTitleFragmentsMin   = 5
	pdfTOCPageRefFragmentsMin = 3
	pdfTOCBodyTextRunesMin    = 40
	pdfTOCShortTextRunesMax   = 40
	// pdfTOCRepeatPagesMin is the cross-page repetition threshold for
	// boilerplate within one vertical band: a long line repeating verbatim
	// at the same height on this many pages is a running watermark or
	// storefront promo, never body prose.
	pdfTOCRepeatPagesMin = 3
	// pdfTOCTopBucketPoints groups section tops into vertical bands:
	// watermarks are page templates sitting at the same height on every
	// page (within a few points of jitter), while body prose drifts. Ten
	// points is wide enough for render jitter and narrow enough to
	// separate header, body and footer bands.
	pdfTOCTopBucketPoints = 10
)

// pdfTOCRomanMarkerPattern matches standalone roman-numeral page markers
// ("I", "II", "D", "M"). They carry no content but are kept as page anchors,
// matching the existing entry-filter behavior.
var pdfTOCRomanMarkerPattern = regexp.MustCompile(`(?i)^[mdclxvi]+$`)

// filterPDFTOCFragmentPages drops fragmented table-of-contents pages from the
// stream. DeepDoc layout splits one TOC line (chapter title + subtitle +
// leader dots + page number) into unadjacent sections — a bare chapter title,
// a subtitle line and a bare page number — that neither the anchored title
// pass nor the entry-line filter recognizes.
//
// Classification counts title-like and page-number-like fragments and
// requires zero body-length prose outside title-layout sections. Long lines
// repeating verbatim on every page (running watermarks, storefront promos)
// are boilerplate, not prose, and do not veto: body prose never repeats
// verbatim on all pages. Body chapter pages (one or two headings plus long
// prose) never satisfy the thresholds, so a mixed page degrades to untouched
// instead of deleting prose. Sections without positions take no part in the
// classification and are never deleted; table and figure sections are
// excluded on both sides.
func filterPDFTOCFragmentPages(sections []deepdoctype.Section) []deepdoctype.Section {
	return filterPDFTOCFragmentPagesWithRepeated(sections, pdfTOCRepeatedLines(sections))
}

// filterPDFTOCFragmentPagesWithRepeated is the injectable core of
// filterPDFTOCFragmentPages: production builds the repetition table from
// the raw stream before any deletion pass (see removePDFTOC); tests call
// the wrapper above. Splitting the two keeps the table lifetime explicit.
func filterPDFTOCFragmentPagesWithRepeated(sections []deepdoctype.Section, repeated map[string]bool) []deepdoctype.Section {
	pages := map[int][]int{}
	for i, s := range sections {
		if page, ok := pdfTOCFragmentPage(s); ok {
			pages[page] = append(pages[page], i)
		}
	}
	drop := map[int]struct{}{}
	for _, indices := range pages {
		var titles, refs, body int
		for _, i := range indices {
			text := sectionText(sections[i])
			entry := isPDFTOCEntrySection(text)
			if entry || isPDFTOCTitleCandidate(text) {
				titles++
			}
			if entry || pdfTOCBarePageRefPattern.MatchString(text) {
				refs++
			}
			if entry || isPDFTOCTitleCandidate(text) || len([]rune(text)) < pdfTOCBodyTextRunesMin {
				continue
			}
			if repeated[pdfTOCRepeatKey(text, firstSectionTop(sections[i]))] {
				continue
			}
			if strings.TrimSpace(sections[i].LayoutType) == deepdoctype.LayoutTypeTitle {
				continue
			}
			body++
		}
		if titles < pdfTOCTitleFragmentsMin || refs < pdfTOCPageRefFragmentsMin || body > 0 {
			continue
		}
		for _, i := range indices {
			if !pdfTOCFragmentDeletable(sections, i, repeated) {
				continue
			}
			drop[i] = struct{}{}
		}
	}
	if len(drop) == 0 {
		return sections
	}
	filtered := make([]deepdoctype.Section, 0, len(sections)-len(drop))
	for i, s := range sections {
		if _, ok := drop[i]; ok {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

// pdfTOCFragmentPage reports the page a section belongs to. It returns false
// for non-text sections (tables and figures are never TOC fragments) and for
// sections without positions (they take no part in page-level
// classification). Only positions carrying actual page numbers count: a
// position entry with an empty PageNumbers slice is not a page, and
// firstSectionPage alone would report it as the real page 0.
func pdfTOCFragmentPage(s deepdoctype.Section) (int, bool) {
	switch strings.TrimSpace(s.LayoutType) {
	case "", deepdoctype.LayoutTypeText, deepdoctype.LayoutTypeTitle:
	default:
		return 0, false
	}
	for _, pos := range s.Positions {
		if len(pos.PageNumbers) > 0 {
			return pos.PageNumbers[0], true
		}
	}
	return 0, false
}

// pdfTOCRepeatedLines reports the set of long lines repeating verbatim at
// the same height on at least pdfTOCRepeatPagesMin pages. Only text/title
// sections with page positions vote; title candidates and entry lines
// always count as TOC signals first and never as boilerplate, so chapter
// headings repeating across pages cannot be misread. Short text needs no
// table: it never vetoes classification on its own length. The key couples
// normalized text with a vertical band: watermarks are page templates at a
// fixed height, so glue/space jitter across pages still lands in one
// bucket while drifting body prose never gathers. The table is built from
// the raw section stream before any deletion pass runs: entry filtering
// deletes in place and would otherwise strip the page fragments some keys
// need to reach threshold.
func pdfTOCRepeatedLines(sections []deepdoctype.Section) map[string]bool {
	pages := map[string]map[int]struct{}{}
	for _, s := range sections {
		page, ok := pdfTOCFragmentPage(s)
		if !ok {
			continue
		}
		text := sectionText(s)
		if isPDFTOCTitleCandidate(text) || isPDFTOCEntrySection(text) || len([]rune(text)) < pdfTOCBodyTextRunesMin {
			continue
		}
		key := pdfTOCRepeatKey(text, firstSectionTop(s))
		if pages[key] == nil {
			pages[key] = map[int]struct{}{}
		}
		pages[key][page] = struct{}{}
	}
	repeated := map[string]bool{}
	for key, set := range pages {
		if len(set) >= pdfTOCRepeatPagesMin {
			repeated[key] = true
		}
	}
	return repeated
}

// pdfTOCRepeatKey normalizes a line for repetition comparison: all
// whitespace stripped, coupled with the vertical band of its top. Template
// watermarks gluing or spacing the URL differently across pages still share
// text and height, so they land in one bucket. No digit masking and no case
// folding: page numbers inside boilerplate would otherwise merge distinct
// lines, and title candidates never reach this table.
func pdfTOCRepeatKey(text string, top float64) string {
	stripped := strings.Join(strings.Fields(text), "")
	return stripped + "\x00" + itoa(int(top/pdfTOCTopBucketPoints))
}

// itoa formats a non-negative bucket index without pulling in strconv for
// one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// pdfTOCFragmentDeletable reports whether the section at idx on a classified
// TOC page is TOC debris: a title candidate, an entry line, a bare page
// reference, a page-number-like title, repeated boilerplate, or short text
// (subtitles and debris) anchored by a neighboring TOC signal. Book titles
// (non-title headings), roman-numeral page markers, long prose and non-text
// layouts are kept.
func pdfTOCFragmentDeletable(sections []deepdoctype.Section, idx int, repeated map[string]bool) bool {
	s := sections[idx]
	switch strings.TrimSpace(s.LayoutType) {
	case "", deepdoctype.LayoutTypeText:
	case deepdoctype.LayoutTypeTitle:
		t := sectionText(s)
		if !isPDFTOCTitleCandidate(t) && !isPDFTOCEntrySection(t) && !pdfTOCBarePageRefPattern.MatchString(t) && !repeated[pdfTOCRepeatKey(t, firstSectionTop(s))] {
			return false
		}
	default:
		return false
	}
	text := sectionText(s)
	top := firstSectionTop(s)
	switch {
	case isPDFTOCTitleCandidate(text), isPDFTOCEntrySection(text):
		return true
	case pdfTOCRomanMarkerPattern.MatchString(text), strings.Contains(text, "《"):
		return false
	case repeated[pdfTOCRepeatKey(text, top)]:
		return true
	case pdfTOCBarePageRefPattern.MatchString(text):
		return true
	case len([]rune(text)) < pdfTOCShortTextRunesMax && pdfTOCFragmentAnchorText(sections, idx):
		return true
	default:
		return false
	}
}

// pdfTOCFragmentAnchorText reports whether the short text at idx sits next
// to a TOC signal (a title candidate, entry line or page reference within
// one neighbor on each side). It gates short-text deletion so a classified
// page keeps short lines that read as standalone content. Both sides go
// through pdfTOCFragmentPage so positionless sections and table/figure
// sections can neither be anchored nor anchor: firstSectionPage alone
// returns 0 for them, which collides with the real page 0.
func pdfTOCFragmentAnchorText(sections []deepdoctype.Section, idx int) bool {
	page, ok := pdfTOCFragmentPage(sections[idx])
	if !ok {
		return false
	}
	for _, j := range []int{idx - 1, idx + 1} {
		if j < 0 || j >= len(sections) {
			continue
		}
		neighborPage, ok := pdfTOCFragmentPage(sections[j])
		if !ok || neighborPage != page {
			continue
		}
		t := sectionText(sections[j])
		if isPDFTOCTitleCandidate(t) || isPDFTOCEntrySection(t) || pdfTOCBarePageRefPattern.MatchString(t) {
			return true
		}
	}
	return false
}

func isPDFTOCEntrySection(text string) bool {
	if pdfTOCEntryPattern.MatchString(text) {
		return true
	}
	return len(pdfTOCEntryAnywherePattern.FindAllString(text, -1)) >= 2
}

// isPDFTOCTitleCandidate reports whether a section reads like the title line
// of a TOC entry whose leader/page number lives on the following line: it
// must open with a chapter/section marker and must not contain a leader run
// itself.
func isPDFTOCTitleCandidate(text string) bool {
	if !pdfTOCTitlePrefixPattern.MatchString(text) {
		return false
	}
	return !pdfTOCEntryAnywherePattern.MatchString(text)
}

func sectionText(s deepdoctype.Section) string {
	return strings.TrimSpace(s.Text)
}

func sectionTextPrefix(s deepdoctype.Section, n int) string {
	text := sectionText(s)
	if len(text) < n {
		return text
	}
	return text[:n]
}
func removePDFTOCByOutlines(result *deepdoctype.ParseResult, outlines []deepdoctype.Outline) {
	if result == nil || len(outlines) == 0 {
		return
	}
	tocPage, contentPage := findPDFTOCPageRange(outlines)
	if contentPage <= tocPage {
		return
	}
	filtered := result.Sections[:0]
	for _, s := range result.Sections {
		page := firstSectionPage(s)
		if page >= tocPage && page < contentPage {
			continue
		}
		filtered = append(filtered, s)
	}
	result.Sections = filtered
}

func findPDFTOCPageRange(outlines []deepdoctype.Outline) (tocPage, contentPage int) {
outer:
	for i, o := range outlines {
		title := strings.TrimSpace(o.Title)
		if idx := strings.Index(title, "@@"); idx >= 0 {
			title = strings.TrimSpace(title[:idx])
		}
		if !pdfTOCTitlePattern.MatchString(strings.ToLower(title)) {
			continue
		}
		tocPage = o.PageNumber
		for _, next := range outlines[i+1:] {
			if next.Level != o.Level {
				continue
			}
			nextTitle := strings.TrimSpace(next.Title)
			if idx := strings.Index(nextTitle, "@@"); idx >= 0 {
				nextTitle = strings.TrimSpace(nextTitle[:idx])
			}
			if pdfTOCTitlePattern.MatchString(strings.ToLower(nextTitle)) {
				continue
			}
			contentPage = next.PageNumber
			break outer
		}
		break
	}
	return
}

func reorderPDFMultiColumn(result *deepdoctype.ParseResult, pageWidth, _ float64) {
	if result == nil || len(result.Sections) < 2 {
		return
	}

	var widths []float64
	for _, s := range result.Sections {
		if strings.TrimSpace(s.LayoutType) != deepdoctype.LayoutTypeText || len(s.Positions) == 0 {
			continue
		}
		width := s.Positions[0].Right - s.Positions[0].Left
		if width > 0 {
			widths = append(widths, width)
		}
	}
	if len(widths) == 0 {
		return
	}
	sort.Float64s(widths)
	medianWidth := widths[len(widths)/2]
	if medianWidth >= pageWidth/2 {
		return
	}

	sort.Slice(result.Sections, func(i, j int) bool {
		pi, pj := firstSectionPage(result.Sections[i]), firstSectionPage(result.Sections[j])
		if pi != pj {
			return pi < pj
		}
		xi, xj := firstSectionLeft(result.Sections[i]), firstSectionLeft(result.Sections[j])
		if math.Abs(xi-xj) > 1e-6 {
			return xi < xj
		}
		return firstSectionTop(result.Sections[i]) < firstSectionTop(result.Sections[j])
	})

	threshold := medianWidth / 2
	for i := len(result.Sections) - 1; i >= 1; i-- {
		for j := i - 1; j >= 0; j-- {
			if firstSectionPage(result.Sections[j]) != firstSectionPage(result.Sections[j+1]) {
				continue
			}
			if math.Abs(firstSectionLeft(result.Sections[j])-firstSectionLeft(result.Sections[j+1])) >= threshold {
				continue
			}
			if firstSectionTop(result.Sections[j+1]) < firstSectionTop(result.Sections[j]) {
				result.Sections[j], result.Sections[j+1] = result.Sections[j+1], result.Sections[j]
			}
		}
	}
}

func firstSectionPage(s deepdoctype.Section) int {
	for _, p := range s.Positions {
		for _, pn := range p.PageNumbers {
			return pn
		}
	}
	return 0
}

func firstSectionLeft(s deepdoctype.Section) float64 {
	for _, p := range s.Positions {
		return p.Left
	}
	return 0
}

func firstSectionTop(s deepdoctype.Section) float64 {
	for _, p := range s.Positions {
		return p.Top
	}
	return 0
}
