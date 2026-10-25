package parser

import (
	"context"
	"fmt"
	"math"
	"strings"

	"ragflow/internal/entity"
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

// PaddleOCRPDFResult projects OCR regions onto the PDF's page geometry and
// emits the structured items consumed by the existing PDF chunk pipeline.
func PaddleOCRPDFResult(ctx context.Context, filename string, data []byte, response *models.OCRFileResponse) ParseResult {
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if response == nil || len(response.Pages) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: PaddleOCR returned no page blocks")}
	}
	sizes, pageCount, geometryErr := paddleOCRPageSizes(ctx, data, response.Pages)
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	result := ParseResult{Warnings: append([]string(nil), response.Warnings...)}
	hasPageCount := pageCount > 0
	if geometryErr != nil {
		result.Warnings = append(result.Warnings, "PaddleOCR PDF geometry unavailable: "+geometryErr.Error())
	}
	markdown, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		return ParseResult{Err: err}
	}
	markdown.FetchRemoteImages = false
	mediaOrdinal := 0
	for _, page := range response.Pages {
		if !hasPageCount && page.Index+1 > pageCount {
			pageCount = page.Index + 1
		}
		size := sizes[page.Index]
		if paddleOCRPositive(page.Width) && paddleOCRPositive(page.Height) && paddleOCRPositive(size[0]) && paddleOCRPositive(size[1]) && !paddleOCRAspectRatioMatches(page, size) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("PaddleOCR page %d: OCR dimensions %gx%g do not match PDF aspect ratio %gx%g; coordinates omitted", page.Index+1, page.Width, page.Height, size[0], size[1]))
			size = [2]float64{}
		}
		for _, block := range page.Blocks {
			if err := ctx.Err(); err != nil {
				return ParseResult{Err: err}
			}
			label := strings.ToLower(strings.TrimSpace(block.Label))
			if models.IsOCRMediaLabel(label) || models.IsOCRImageOnlyText(block.Text) {
				mediaOrdinal++
				result.JSON = append(result.JSON, paddleOCRMediaItem(page, block, size, mediaOrdinal))
				continue
			}
			content := strings.TrimSpace(utility.StripMarkdownImages(block.Text))
			if content == "" {
				continue
			}
			parsed := markdown.ParseWithResult(ctx, filename, []byte(content+"\n\n"))
			if parsed.Err != nil {
				return parsed
			}
			result.Warnings = append(result.Warnings, parsed.Warnings...)
			position := paddleOCRBlockPosition(page, block.BBox, size)
			isTitle := label == "doc_title" || label == "paragraph_title"
			if label == "figure_title" || label == "table_title" {
				label = "caption"
			}
			for _, item := range parsed.JSON {
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
				// Page identity is retained even when the block has no usable geometry.
				fields["page_number"] = page.Index + 1
				fields["_preserve_order"] = true
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
				if len(position) > 0 {
					matrix := [][]any{position}
					fields["positions"], fields["_pdf_positions"] = matrix, matrix
				}
				if table, ok := fields["table"].(*entity.TableData); ok && table != nil {
					table.Page = page.Index + 1
				}
				result.JSON = append(result.JSON, DocItemFromMap(fields))
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if len(result.JSON) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: PaddleOCR returned empty text")}
	}
	result.File = pdfFileMeta(filename, pageCount)
	return result
}

// paddleOCRMediaItem keeps a figure-like block as a media item. The service
// may return only Markdown image markup or no body text at all; either way the
// block must still reach the chunker with its region so the on-demand crop and
// vision enhancement can act on it, instead of disappearing from the document.
func paddleOCRMediaItem(page models.OCRPage, block models.OCRBlock, size [2]float64, ordinal int) DocItem {
	text := strings.TrimSpace(utility.StripMarkdownImages(block.Text))
	if text == "" {
		// The ordinal keeps captionless figures distinct: the chunk id hashes
		// the body, so a shared placeholder would make every one of them
		// overwrite the same preview object and index entry.
		text = fmt.Sprintf("[Image %d]", ordinal)
	}
	fields := map[string]any{
		"text":            text,
		"doc_type_kwd":    "image",
		"layout":          "figure",
		"page_number":     page.Index + 1,
		"_preserve_order": true,
	}
	if position := paddleOCRBlockPosition(page, block.BBox, size); len(position) > 0 {
		matrix := [][]any{position}
		fields["positions"], fields["_pdf_positions"] = matrix, matrix
	}
	return DocItemFromMap(fields)
}

func paddleOCRBlockPosition(page models.OCRPage, bbox []float64, size [2]float64) []any {
	if page.Index < 0 || !paddleOCRPositive(page.Width) || !paddleOCRPositive(page.Height) || !paddleOCRPositive(size[0]) || !paddleOCRPositive(size[1]) {
		return nil
	}
	if !paddleOCRAspectRatioMatches(page, size) || len(bbox) != 4 {
		return nil
	}
	for _, v := range bbox {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
	}
	left, top, right, bottom := bbox[0], bbox[1], bbox[2], bbox[3]
	if right <= left || bottom <= top {
		return nil
	}
	left = math.Max(0, math.Min(size[0], left*size[0]/page.Width))
	right = math.Max(0, math.Min(size[0], right*size[0]/page.Width))
	top = math.Max(0, math.Min(size[1], top*size[1]/page.Height))
	bottom = math.Max(0, math.Min(size[1], bottom*size[1]/page.Height))
	if right <= left || bottom <= top {
		return nil
	}
	return []any{page.Index + 1, left, right, top, bottom}
}

// Allow 2% variation for renderer pixel rounding, but reject anisotropic scaling.
func paddleOCRAspectRatioMatches(page models.OCRPage, size [2]float64) bool {
	xScale, yScale := size[0]/page.Width, size[1]/page.Height
	return math.Max(xScale, yScale) <= 1.02*math.Min(xScale, yScale)
}

func paddleOCRPositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
