package parser

import (
	"math"
	"regexp"
	"sort"
	"strings"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

var pdfHeaderFooterPattern = regexp.MustCompile(`(?i)^(header|footer|number)$`)
var pdfTOCTitlePattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)

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
	if opts.enableMultiColumn && opts.pageWidth > 0 {
		reorderPDFMultiColumn(result, opts.pageWidth, opts.zoom)
	}
	if opts.removeTOC {
		applyRemoveTOC(result)
	}
	normalizePDFLayoutTypes(result)
	if opts.removeHeaderFooter {
		filterPDFHeaderFooter(result)
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
			// markdown serialization / chunk time, so it must not
			// influence classification here (otherwise every positioned
			// text box would be mislabeled "image").
			section.DocTypeKwd = "text"
		}
	}
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
	// Detect English to mirror Python's `eng` parameter: use first-2-words
	// prefix for English documents vs first-3-characters for CJK documents.
	eng := isEnglishSections(sections)
	i := 0
	for i < len(sections) {
		text := sectionText(sections[i])
		// Collapse whitespace and strip "@@" suffix before matching, mirroring
		// Python's re.sub(r"( | |\u3000)+", "", get(i).split("@@")[0]).
		text = stripAndCollapse(text)
		if !pdfTOCTitlePattern.MatchString(strings.ToLower(text)) {
			i++
			continue
		}
		sections = append(sections[:i], sections[i+1:]...)
		if i >= len(sections) {
			break
		}
		prefix := sectionTextPrefixExpr(sections[i], eng)
		for prefix == "" {
			sections = append(sections[:i], sections[i+1:]...)
			if i >= len(sections) {
				break
			}
			prefix = sectionTextPrefixExpr(sections[i], eng)
		}
		if i >= len(sections) || prefix == "" {
			break
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
	result.Sections = sections
}

// stripAndCollapse strips the "@@" suffix and collapses consecutive
// whitespace characters (space, ideographic space) into a single empty
// string. Mirrors Python's:
//
//	re.sub(r"( | |\u3000)+", "", get(i).split("@@")[0])
func stripAndCollapse(s string) string {
	if idx := strings.Index(s, "@@"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	return spaceCollapseRe.ReplaceAllString(s, "")
}

var spaceCollapseRe = regexp.MustCompile(`[ \x{3000}]+`)

// sectionTextPrefixExpr computes the TOC prefix matching Python's:
//
//	prefix = get(i)[:3] if not eng else " ".join(get(i).split()[:2])
func sectionTextPrefixExpr(s deepdoctype.Section, eng bool) string {
	text := sectionText(s)
	if !eng {
		// Use rune slicing to match Python's character-level [:3].
		// Go's default byte-slicing would return only one
		// 3-byte CJK character instead of three characters.
		runes := []rune(text)
		if len(runes) < 3 {
			return text
		}
		return string(runes[:3])
	}
	words := strings.Fields(text)
	if len(words) >= 2 {
		return strings.Join(words[:2], " ")
	}
	return text
}

// isEnglishSections mirrors Python's is_english + random_choices
// sampling over section texts. Returns true when >80% of sampled
// section texts consist entirely of ASCII alphanumeric / whitespace /
// punctuation characters.
func isEnglishSections(sections []deepdoctype.Section) bool {
	const sampleSize = 200
	var texts []string
	for _, s := range sections {
		t := strings.TrimSpace(s.Text)
		if t == "" {
			continue
		}
		texts = append(texts, t)
		if len(texts) >= sampleSize {
			break
		}
	}
	if len(texts) == 0 {
		return false
	}
	eng := 0
	for _, t := range texts {
		if isEnglishTextPattern.MatchString(strings.TrimSpace(t)) {
			eng++
		}
	}
	return float64(eng)/float64(len(texts)) > 0.8
}

// isEnglishTextPattern mirrors Python's is_english character pattern:
//
//	r"[`a-zA-Z0-9\s.,':;/\"?<>!\(\)\-]+"
//
// The backtick is omitted as it is irrelevant for section texts.
var isEnglishTextPattern = regexp.MustCompile(`^[a-zA-Z0-9\s.,':;"/?<>!()\-]+$`)

func sectionText(s deepdoctype.Section) string {
	return strings.TrimSpace(s.Text)
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
