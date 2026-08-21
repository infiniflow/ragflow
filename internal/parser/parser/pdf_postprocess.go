package parser

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// Substring match to mirror Python's remove_header_footer:
// re.search(r"(header|footer|number)", raw_layout, re.I) (rag/flow/parser/parser.py:754).
// Python matches any layout type CONTAINING one of these words, not just the
// exact token, so a composite label like "page-footer" is also stripped.
var pdfHeaderFooterPattern = regexp.MustCompile(`(?i)header|footer|number`)
var pdfTOCTitlePattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)

// TOC entry patterns: dot leaders or wide whitespace followed by a trailing
// page number, e.g. "Chapter One .............. 2" or "Chapter One    2".
var (
	pdfTOCDotLeaderPattern    = regexp.MustCompile(`(\.|…|·){3,}\s*\d{1,5}\s*$`)
	pdfTOCTrailingPagePattern = regexp.MustCompile(`\s{2,}\d{1,5}\s*$`)
	pdfTOCEntryNumberPattern  = regexp.MustCompile(`^\d+(\.\d+)*[\s\.、]`)
)

// isPDFTOCEntry reports whether a line still looks like a table-of-contents
// entry (title text plus a trailing page number). Used to consume the whole
// contiguous run of entries after the TOC title instead of only the first.
func isPDFTOCEntry(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) < 4 || len(t) > 200 {
		return false
	}
	if !pdfTOCDotLeaderPattern.MatchString(t) && !pdfTOCTrailingPagePattern.MatchString(t) {
		return false
	}
	for _, r := range t {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return pdfTOCEntryNumberPattern.MatchString(t)
}

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
// document's pages — running headers, footers, and page numbers. The
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
	pageSet := make(map[int]struct{}, len(result.PageHeight))
	for i := range sections {
		if len(sections[i].Positions) == 0 || len(sections[i].Positions[0].PageNumbers) == 0 {
			continue
		}
		pageSet[sections[i].Positions[0].PageNumbers[0]] = struct{}{}
	}
	numPages := len(pageSet)
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
		case top <= pageHeight*pdfHeaderZoneRatio && bottom <= pageHeight*0.5:
			isHeader = true
		case bottom >= pageHeight*pdfFooterZoneRatio && top >= pageHeight*0.5:
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
//   - First outline on page 1 → outline-based remove_toc_pdf
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
		sections = append(sections[:i], sections[i+1:]...)
		if i >= len(sections) || prefix == "" {
			break
		}
		// Consume the remaining contiguous TOC entries (dot leaders or a
		// trailing page number). Python's remove_contents_table stops at the
		// first line sharing the first entry's 3-byte prefix, which leaves
		// most of the table behind whenever every entry shares that prefix
		// (e.g. "Chapter One ...", "Chapter Two ...", ...). When entries were
		// consumed this way, skip the legacy prefix scan and resume the title
		// scan; otherwise fall back to it for pattern-less TOCs.
		removedEntries := 0
		for i < len(sections) && isPDFTOCEntry(sections[i].Text) {
			sections = append(sections[:i], sections[i+1:]...)
			removedEntries++
		}
		if removedEntries > 0 {
			continue
		}
		for j := i; j < len(sections) && j < i+128; j++ {
			if !strings.HasPrefix(sectionText(sections[j]), prefix) {
				continue
			}
			sections = append(sections[:i], sections[j:]...)
			break
		}
	}
	result.Sections = sections
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
