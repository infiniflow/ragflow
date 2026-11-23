package parser

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"ragflow/internal/deepdoc/parser/pdf/table"
	"ragflow/internal/entity"
)

// ExternalBlockType names what a typed element of a remote PDF service is. A
// producer maps its own labels onto these; the converter owns what each one
// becomes on the wire.
type ExternalBlockType string

const (
	// ExternalBlockText is body text and the type of every unlisted value.
	ExternalBlockText ExternalBlockType = "text"
	// ExternalBlockTitle is a title a layout model labelled. Chunkers check it
	// against their own title heuristics.
	ExternalBlockTitle ExternalBlockType = "title"
	// ExternalBlockHeading is a heading the service asserts as document
	// structure. Chunkers take it as a title without checking.
	ExternalBlockHeading ExternalBlockType = "heading"
	ExternalBlockList    ExternalBlockType = "list"
	ExternalBlockCode    ExternalBlockType = "code"
	// ExternalBlockEquation is a formula carried as text.
	ExternalBlockEquation ExternalBlockType = "equation"
	// ExternalBlockTable takes its body from HTML, then TableCells, then Text.
	ExternalBlockTable ExternalBlockType = "table"
	// ExternalBlockFigure is a picture, chart, seal or other media region.
	ExternalBlockFigure ExternalBlockType = "figure"
)

// ExternalRegion is one box a service located on a page.
type ExternalRegion struct {
	// Page is the 1-indexed page the box lies on.
	Page int
	// Box is [x0, y0, x1, y1] in the space ExternalBlockOptions declares.
	Box [4]float64
	// BottomLeft marks y as measured upward from the bottom edge of the page,
	// as in PDF user space, instead of downward from the top edge.
	BottomLeft bool
}

// ExternalBlock is one typed element of a remote PDF service's structured
// output. A producer reads its own payload into ExternalBlocks and
// ExternalBlocksToItems turns them into parser items, so the item contract is
// written once instead of once per service.
type ExternalBlock struct {
	Type ExternalBlockType
	// Text is the plain text of the block.
	Text string
	// HTML is the rendered markup of a table.
	HTML string
	// TableCells are the rows of a table that has no markup of its own. The
	// first row is the header.
	TableCells [][]string
	// Items is the body of a block the producer has already split into items,
	// because the service delivers it as a Markdown or HTML fragment. A
	// non-nil Items replaces Type, Text, HTML and TableCells; the items keep
	// their own doc_type_kwd and layout.
	Items []map[string]any
	// Page is the 1-indexed page the block belongs to; zero means unknown.
	Page int
	// Regions are the boxes the service located for the block. A block that
	// continues on another page has one region per page.
	Regions []ExternalRegion
	// Captions and Footnotes belong to a table or figure. A table keeps the
	// caption in its TableData and the footnotes as its note text; a figure
	// carries both as its text.
	Captions  []string
	Footnotes []string
	// HeadingLevel is the 1-based depth of a title or heading. A positive
	// level is written as a Markdown marker in front of the text.
	HeadingLevel int
	// PreserveOrder declares the producer's sequence authoritative, so the
	// chunker does not sort the items by geometry.
	PreserveOrder bool
}

// ExternalBlockOptions declares, once per producer, the coordinate space of
// its regions and the item policies that differ between services.
type ExternalBlockOptions struct {
	// Provider names the service in warnings.
	Provider string
	// PageSizes maps a 1-indexed page to its [width, height] in PDF points. A
	// page that is missing or has a non-positive dimension is unknown.
	PageSizes map[int][2]float64
	// SourceSizes maps a 1-indexed page to the extent of the space its boxes
	// are reported in: rendered pixels, or a normalized range. A nil map
	// declares the boxes to be in PDF points already.
	SourceSizes map[int][2]float64
	// BleedTolerance is how far past the page edge a box may reach, as a
	// fraction of that page dimension. A box further out is evidence of a
	// different coordinate space: it is dropped and reported, not clamped.
	// Zero clamps every box without reporting.
	BleedTolerance float64
	// AspectTolerance is the relative difference allowed between the
	// horizontal and vertical scale of a page. A page beyond it was rendered
	// from another crop or rotation: its boxes are dropped and reported. Zero
	// skips the check.
	AspectTolerance float64
	// ReorderEdges swaps inverted edges of a box instead of rejecting the
	// box, for services that do not fix the order of the corners.
	ReorderEdges bool
	// KeepUnscaled keeps the reported box, in the service's own units, when
	// the scale of its page is unknown, instead of dropping it. A BottomLeft
	// box is kept only when the source height is known, because that height
	// is what turns its origin.
	KeepUnscaled bool
	// FigurePlaceholder is the text given to a figure that has none. When it
	// is empty, a figure without text is kept only if it has a region to
	// crop, and carries no text.
	FigurePlaceholder string
}

// ExternalBlocksToItems converts the blocks of one document into parser items
// and returns the warnings about regions it could not place.
//
// An item built from a block's Type carries doc_type_kwd and layout.
// page_number is set when the page is known, positions and _pdf_positions when
// a region could be placed in PDF points with a top-left origin. Page identity
// never depends on geometry: a block whose region is unusable keeps its text
// and page.
func ExternalBlocksToItems(blocks []ExternalBlock, opts ExternalBlockOptions) ([]DocItem, []string) {
	items, warnings := externalBlockMaps(blocks, opts)
	return MapsToItems(items), warnings
}

// externalBlockMaps is ExternalBlocksToItems for the producers in this package
// that assemble their result from item maps.
func externalBlockMaps(blocks []ExternalBlock, opts ExternalBlockOptions) ([]map[string]any, []string) {
	space := newExternalBlockSpace(opts)
	items := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		positions := space.positions(block.Regions)
		body := block.Items
		if body == nil {
			if item := externalBlockBody(block, opts.FigurePlaceholder, len(positions) > 0); item != nil {
				body = []map[string]any{item}
			}
		}
		for _, item := range body {
			if item == nil {
				continue
			}
			if block.Page > 0 {
				item["page_number"] = block.Page
				if td, ok := item["table"].(*entity.TableData); ok && td != nil {
					td.Page = block.Page
				}
			}
			if block.PreserveOrder {
				item["_preserve_order"] = true
			}
			if len(positions) > 0 {
				// One matrix per item: items of the same block must not share
				// rows that a later stage rewrites in place.
				matrix := make([][]any, len(positions))
				for i, row := range positions {
					matrix[i] = slices.Clone(row)
				}
				item["positions"], item["_pdf_positions"] = matrix, matrix
			}
			items = append(items, item)
		}
	}
	return items, space.warnings()
}

// externalBlockBody builds the item of one block from its type and content, or
// returns nil for a block with nothing to index or crop.
func externalBlockBody(block ExternalBlock, figurePlaceholder string, hasRegion bool) map[string]any {
	text := strings.TrimSpace(block.Text)
	switch block.Type {
	case ExternalBlockTable:
		return externalTableItem(block, text)
	case ExternalBlockFigure:
		text = joinExternalText(text, block.Captions, block.Footnotes)
		if text == "" {
			text = figurePlaceholder
		}
		// A figure with neither text nor a region has nothing a crop preview
		// or the vision pass can act on.
		if text == "" && !hasRegion {
			return nil
		}
		item := map[string]any{"doc_type_kwd": "image", "layout": "figure"}
		if text != "" {
			item["text"] = text
		}
		return item
	}
	if text == "" {
		return nil
	}
	layout := "text"
	switch block.Type {
	case ExternalBlockTitle, ExternalBlockHeading, ExternalBlockList, ExternalBlockCode, ExternalBlockEquation:
		layout = string(block.Type)
	}
	item := map[string]any{"doc_type_kwd": "text", "layout": layout}
	if block.HeadingLevel > 0 && (block.Type == ExternalBlockTitle || block.Type == ExternalBlockHeading) {
		text = strings.Repeat("#", block.HeadingLevel) + " " + text
		// heading_level accompanies the asserted layout, as the Markdown
		// parser emits it; a model-labelled title keeps only the text marker.
		if block.Type == ExternalBlockHeading {
			item["heading_level"] = block.HeadingLevel
		}
	}
	item["text"] = text
	return item
}

// externalTableItem renders a table once: markup wins, cell rows are rendered
// when there is none, and plain text is what remains. The markup is projected
// into TableData here, so no later stage parses it again.
func externalTableItem(block ExternalBlock, text string) map[string]any {
	markup := strings.TrimSpace(block.HTML)
	if markup == "" && rowsHaveText(block.TableCells) {
		markup = table.SimpleRowsToHTML(block.TableCells)
	}
	if markup != "" {
		text = markup
	}
	if text == "" {
		// No cells were read: the caption and footnotes are all the region has.
		if text = joinExternalText("", block.Captions, block.Footnotes); text == "" {
			return nil
		}
		return map[string]any{"text": text, "doc_type_kwd": "text", "layout": "table"}
	}
	item := map[string]any{"text": text, "doc_type_kwd": pdfTableDocType(text), "layout": "table"}
	projectPDFTable(item)
	td, _ := item["table"].(*entity.TableData)
	if td == nil {
		// Free text in a table region: the caption and footnotes are all the
		// structure it has, so they stay with it.
		item["text"] = joinExternalText(text, block.Captions, block.Footnotes)
		return item
	}
	if caption := joinExternalText("", block.Captions, nil); caption != "" && caption != td.Caption {
		td.Caption = strings.TrimSpace(td.Caption + "\n" + caption)
	}
	// Text left beside a projected table is its note. NormalizeResult emits it
	// as the text item that follows the table.
	note, _ := item["text"].(string)
	if note = joinExternalText(note, block.Footnotes, nil); note != "" {
		item["text"] = note
	}
	return item
}

// joinExternalText joins the non-blank parts on separate lines.
func joinExternalText(text string, groups ...[]string) string {
	parts := make([]string, 0, 1+len(groups))
	if text = strings.TrimSpace(text); text != "" {
		parts = append(parts, text)
	}
	for _, group := range groups {
		for _, part := range group {
			if part = strings.TrimSpace(part); part != "" {
				parts = append(parts, part)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// externalBlockSpace places regions on PDF pages under one producer's declared
// coordinate space.
type externalBlockSpace struct {
	opts ExternalBlockOptions
	// mismatched holds the pages whose source extent failed the aspect check.
	mismatched map[int]bool
	// reported holds one warning per page: a page in the wrong space is worth
	// surfacing, and repeating it per block would bury the other warnings.
	reported map[int]string
}

func newExternalBlockSpace(opts ExternalBlockOptions) *externalBlockSpace {
	space := &externalBlockSpace{opts: opts, mismatched: map[int]bool{}, reported: map[int]string{}}
	if opts.AspectTolerance <= 0 {
		return space
	}
	for page := range opts.SourceSizes {
		source, sourced := space.sourceSize(page)
		size, sized := space.pageSize(page)
		if !sourced || !sized {
			continue
		}
		xScale, yScale := size[0]/source[0], size[1]/source[1]
		if math.Max(xScale, yScale) > (1+opts.AspectTolerance)*math.Min(xScale, yScale) {
			space.mismatched[page] = true
			space.reported[page] = fmt.Sprintf("%s page %d: OCR dimensions %gx%g do not match PDF aspect ratio %gx%g; coordinates omitted",
				opts.Provider, page, source[0], source[1], size[0], size[1])
		}
	}
	return space
}

func (s *externalBlockSpace) pageSize(page int) ([2]float64, bool) {
	size, ok := s.opts.PageSizes[page]
	return size, ok && positiveFinite(size[0]) && positiveFinite(size[1])
}

func (s *externalBlockSpace) sourceSize(page int) ([2]float64, bool) {
	size, ok := s.opts.SourceSizes[page]
	return size, ok && positiveFinite(size[0]) && positiveFinite(size[1])
}

func positiveFinite(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}

// positions returns the canonical rows of the regions that could be placed.
func (s *externalBlockSpace) positions(regions []ExternalRegion) [][]any {
	var rows [][]any
	for _, region := range regions {
		if row := s.position(region); row != nil {
			rows = append(rows, row)
		}
	}
	return rows
}

// position converts one region into the canonical [page, left, right, top,
// bottom] row: PDF points, origin at the top-left corner of the page. It
// returns nil when the region cannot be placed, because a highlight in the
// wrong place is worse than none.
func (s *externalBlockSpace) position(region ExternalRegion) []any {
	page := region.Page
	if page < 1 {
		return nil
	}
	x0, y0, x1, y1 := region.Box[0], region.Box[1], region.Box[2], region.Box[3]
	for _, v := range region.Box {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
	}
	if s.opts.ReorderEdges {
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
	}
	if x1 <= x0 || y1 <= y0 {
		return nil
	}
	size, sized := s.pageSize(page)
	if s.opts.SourceSizes != nil {
		source, sourced := s.sourceSize(page)
		switch {
		case sourced && sized:
			if s.mismatched[page] {
				return nil
			}
			if tolerance := s.opts.BleedTolerance; tolerance > 0 && s.boxExceedsBleed(x0, y0, x1, y1, source, tolerance) {
				s.reportBleedMismatch(page, region.Box, size)
				return nil
			}
			x0, x1 = x0*size[0]/source[0], x1*size[0]/source[0]
			y0, y1 = y0*size[1]/source[1], y1*size[1]/source[1]
		case s.opts.KeepUnscaled:
			if region.BottomLeft {
				// The box stays in the service's space, so only that space's
				// height can turn its origin.
				if !sourced {
					return nil
				}
				y0, y1 = source[1]-y1, source[1]-y0
			}
			return []any{page, x0, x1, y0, y1}
		default:
			if s.opts.Provider == "SoMark" && s.reported[page] == "" {
				s.reported[page] = fmt.Sprintf("%s page %d: positions omitted because the %s pixel size or PDF point size is unavailable", s.opts.Provider, page, s.opts.Provider)
			}
			return nil
		}
	} else if sized {
		if tolerance := s.opts.BleedTolerance; tolerance > 0 && s.boxExceedsBleed(x0, y0, x1, y1, size, tolerance) {
			s.reportBleedMismatch(page, region.Box, size)
			return nil
		}
	}
	if region.BottomLeft {
		// The flip needs the page height. It assumes both sides describe the
		// same page box after rotation.
		if !sized {
			return nil
		}
		y0, y1 = size[1]-y1, size[1]-y0
	}
	if !sized {
		return []any{page, x0, x1, y0, y1}
	}
	x0, x1 = math.Max(0, math.Min(size[0], x0)), math.Max(0, math.Min(size[0], x1))
	y0, y1 = math.Max(0, math.Min(size[1], y0)), math.Max(0, math.Min(size[1], y1))
	if x1 <= x0 || y1 <= y0 {
		return nil
	}
	return []any{page, x0, x1, y0, y1}
}

// boxExceedsBleed reports whether any edge of an unscaled box extends beyond
// the page bleed tolerance in its native coordinate space. It preserves boxes
// at exactly the tolerance and drops only boxes that extend beyond it.
func (s *externalBlockSpace) boxExceedsBleed(x0, y0, x1, y1 float64, extent [2]float64, tolerance float64) bool {
	if tolerance <= 0 || extent[0] <= 0 || extent[1] <= 0 {
		return false
	}
	overflowX := math.Max(-x0, x1-extent[0])
	overflowY := math.Max(-y0, y1-extent[1])
	limitX := extent[0] * tolerance
	limitY := extent[1] * tolerance
	const eps = 1e-9
	return overflowX > limitX*(1+eps) || overflowY > limitY*(1+eps)
}

func (s *externalBlockSpace) reportBleedMismatch(page int, box [4]float64, size [2]float64) {
	if s.reported[page] == "" {
		s.reported[page] = fmt.Sprintf("%s page %d: element box [%g %g %g %g] does not fit page size %gx%g; positions omitted because the service coordinates do not match the PDF point space",
			s.opts.Provider, page, box[0], box[1], box[2], box[3], size[0], size[1])
	}
}

// warnings returns the reported pages in page order.
func (s *externalBlockSpace) warnings() []string {
	warnings := make([]string, 0, len(s.reported))
	for _, page := range slices.Sorted(maps.Keys(s.reported)) {
		warnings = append(warnings, s.reported[page])
	}
	return warnings
}

// externalPageSizes rekeys page sizes read by zero-based page index, the form
// pdfium reports, onto the 1-indexed pages ExternalBlockOptions uses.
func externalPageSizes(byIndex map[int][2]float64) map[int][2]float64 {
	sizes := make(map[int][2]float64, len(byIndex))
	for index, size := range byIndex {
		sizes[index+1] = size
	}
	return sizes
}
