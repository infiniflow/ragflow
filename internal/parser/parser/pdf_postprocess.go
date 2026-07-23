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
