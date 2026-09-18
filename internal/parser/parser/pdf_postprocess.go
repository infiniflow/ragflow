package parser

import (
	"math"
	"sort"
	"strings"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// pdfPostProcessOptions carries the parser backend's post-process switches.
// TOC and header/footer removal are handled earlier, at box level inside the
// DeepDoc layout pipeline (Parser.buildLayout), where the leader-dot and
// per-box geometry signals those detectors rely on are still intact; they are
// intentionally not repeated here.
type pdfPostProcessOptions struct {
	outputFormat       string
	pageWidth          float64
	zoom               float64
	enableMultiColumn  bool
	flattenMediaToText bool
}

func applyPDFPostProcess(result *deepdoctype.ParseResult, opts pdfPostProcessOptions) {
	if result == nil {
		return
	}
	sortSectionsByPosition(result)
	if opts.enableMultiColumn && opts.pageWidth > 0 {
		reorderPDFMultiColumn(result, opts.pageWidth, opts.zoom)
	}
	normalizePDFLayoutTypes(result)
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
