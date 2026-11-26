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
	pageWidth         float64
	zoom              float64
	enableMultiColumn bool
	// parseMethod carries the PDFParser.ParseMethod so the DeepDoc analyzer can
	// be resolved per request (in-process vs a registered external backend).
	// Empty means the historical in-process default.
	parseMethod string
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
	assignPDFDocTypeKeywords(result)
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

func assignPDFDocTypeKeywords(result *deepdoctype.ParseResult) {
	for i := range result.Sections {
		section := &result.Sections[i]
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

// sectionGeometry caches the geometry used by the multi-column ordering so
// the O(n^2) comparison loop never rescans every position of a section.
type sectionGeometry struct {
	page int
	left float64
	top  float64
}

func sectionGeometryOf(s deepdoctype.Section) sectionGeometry {
	return sectionGeometry{
		page: firstSectionPage(s),
		left: firstSectionLeft(s),
		top:  firstSectionTop(s),
	}
}

// sortedSections keeps sections and their cached geometry in lockstep while
// sorting, mirroring the original sort.Slice comparator exactly.
type sortedSections struct {
	sections []deepdoctype.Section
	keys     []sectionGeometry
}

func (s sortedSections) Len() int { return len(s.sections) }

func (s sortedSections) Less(i, j int) bool {
	if s.keys[i].page != s.keys[j].page {
		return s.keys[i].page < s.keys[j].page
	}
	if math.Abs(s.keys[i].left-s.keys[j].left) > 1e-6 {
		return s.keys[i].left < s.keys[j].left
	}
	return s.keys[i].top < s.keys[j].top
}

func (s sortedSections) Swap(i, j int) {
	s.sections[i], s.sections[j] = s.sections[j], s.sections[i]
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
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

	keys := make([]sectionGeometry, len(result.Sections))
	for i := range result.Sections {
		keys[i] = sectionGeometryOf(result.Sections[i])
	}
	sort.Sort(sortedSections{sections: result.Sections, keys: keys})

	threshold := medianWidth / 2
	for i := len(result.Sections) - 1; i >= 1; i-- {
		for j := i - 1; j >= 0; j-- {
			if keys[j].page != keys[j+1].page {
				continue
			}
			if math.Abs(keys[j].left-keys[j+1].left) >= threshold {
				continue
			}
			if keys[j+1].top < keys[j].top {
				result.Sections[j], result.Sections[j+1] = result.Sections[j+1], result.Sections[j]
				keys[j], keys[j+1] = keys[j+1], keys[j]
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
