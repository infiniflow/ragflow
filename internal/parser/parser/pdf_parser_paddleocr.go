package parser

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/entity/models"
	"ragflow/internal/utility"
)

// isStructuralOCRTextLayout reports whether an item already carries a
// structural block kind produced by markdown normalization. Those values are
// finer than an OCR block label and must win over it.
func isStructuralOCRTextLayout(fields map[string]any) bool {
	layout, _ := fields["layout"].(string)
	switch layout {
	case "heading", "paragraph", "list", "code", "quote":
		return true
	}
	return false
}

// ocrAspectTolerance allows 2% variation between the horizontal and vertical
// scale of an OCR page raster for renderer pixel rounding. Anything beyond it
// is anisotropic scaling: the raster was cropped or rotated differently from
// the PDF page.
const ocrAspectTolerance = 0.02

// OCRPDFResult projects the page blocks of an OCR provider onto the PDF's page
// geometry and emits the structured items consumed by the existing PDF chunk
// pipeline. provider names the OCR service in errors and warnings.
//
// A block's BBox is [left, top, right, bottom] in the pixels of the page
// raster the provider recognized, and its text is a Markdown fragment.
func OCRPDFResult(ctx context.Context, provider, filename string, data []byte, response *models.OCRFileResponse) ParseResult {
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if response == nil || len(response.Pages) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: %s returned no page blocks", provider)}
	}
	sizes, pageCount, geometryErr := paddleOCRPageSizes(ctx, data, response.Pages)
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	result := ParseResult{Warnings: append([]string(nil), response.Warnings...)}
	hasPageCount := pageCount > 0
	if geometryErr != nil {
		result.Warnings = append(result.Warnings, provider+" PDF geometry unavailable: "+geometryErr.Error())
	}
	markdown, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		return ParseResult{Err: err}
	}
	markdown.FetchRemoteImages = false
	opts := ExternalBlockOptions{
		Provider:        provider,
		PageSizes:       externalPageSizes(sizes),
		SourceSizes:     make(map[int][2]float64, len(response.Pages)),
		AspectTolerance: ocrAspectTolerance,
	}
	var blocks []ExternalBlock
	var markdownWarnings []string
	for _, page := range response.Pages {
		pageNumber := page.Index + 1
		if !hasPageCount && pageNumber > pageCount {
			pageCount = pageNumber
		}
		opts.SourceSizes[pageNumber] = [2]float64{page.Width, page.Height}
		for _, ocrBlock := range page.Blocks {
			if err := ctx.Err(); err != nil {
				return ParseResult{Err: err}
			}
			// Page identity is retained even when the block has no usable geometry.
			block := ExternalBlock{Page: pageNumber, PreserveOrder: true}
			if len(ocrBlock.BBox) == 4 {
				block.Regions = []ExternalRegion{{Page: pageNumber, Box: [4]float64(ocrBlock.BBox)}}
			}
			label := strings.ToLower(strings.TrimSpace(ocrBlock.Label))
			content := strings.TrimSpace(utility.StripMarkdownImages(ocrBlock.Text))
			if models.IsOCRMediaLabel(label) || models.IsOCRImageOnlyText(ocrBlock.Text) {
				// A figure-like block stays a media item. Its region is kept for
				// the vision pass and the on-demand crop; a synthetic placeholder
				// would index as retrievable body text (#21218).
				block.Type, block.Text = ExternalBlockFigure, content
				blocks = append(blocks, block)
				continue
			}
			if content == "" {
				continue
			}
			parsed := markdown.ParseWithResult(ctx, filename, []byte(content+"\n\n"))
			if parsed.Err != nil {
				return parsed
			}
			markdownWarnings = append(markdownWarnings, parsed.Warnings...)
			if block.Items = ocrBlockItems(label, parsed.JSON); len(block.Items) > 0 {
				blocks = append(blocks, block)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	items, placementWarnings := ExternalBlocksToItems(blocks, opts)
	if len(items) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: %s returned empty text", provider)}
	}
	result.JSON = items
	result.Warnings = append(append(result.Warnings, placementWarnings...), markdownWarnings...)
	result.File = pdfFileMeta(filename, pageCount)
	return result
}

// ocrBlockItems applies an OCR block label to the items the Markdown parser
// produced from the block's text. A title label promotes plain text to a
// heading; any other label fills only the layout Markdown did not assert.
func ocrBlockItems(label string, parsed []DocItem) []map[string]any {
	isTitle := label == "doc_title" || label == "paragraph_title"
	if label == "figure_title" || label == "table_title" {
		label = "caption"
	}
	items := make([]map[string]any, 0, len(parsed))
	for _, item := range parsed {
		_, isTextItem := item.(TextItem)
		if text, ok := item.(TextItem); ok {
			if strings.TrimSpace(text.Text) == "" {
				continue
			}
			layout, _ := text.Extra["layout"].(string)
			if isTitle && layout != "heading" && layout != "list" && layout != "code" {
				prefix := "## "
				if label == "doc_title" {
					prefix = "# "
				}
				text.Text = prefix + text.Text
				if text.Extra == nil {
					text.Extra = map[string]any{}
				}
				text.Extra["layout"] = "heading"
				item = text
			}
		}
		fields := item.ToMap()
		// Markdown normalization already asserted a structural layout
		// (heading/list/code). The OCR block label is coarser and must
		// not overwrite it; only fill an absent layout.
		if isTitle && isTextItem && !isStructuralOCRTextLayout(fields) {
			// The asserted-title marker the Title chunker reads without
			// the notTitle heuristic guard. PDF model labels that merely
			// look title-like stay on "title".
			fields["layout"] = "heading"
		} else if label != "" {
			if existing, _ := fields["layout"].(string); existing == "" {
				fields["layout"] = label
			}
		}
		items = append(items, fields)
	}
	return items
}
