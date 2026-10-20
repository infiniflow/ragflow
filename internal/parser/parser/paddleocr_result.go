package parser

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

var paddleOCRImageMarkup = regexp.MustCompile(`(?is)!\[[^\]]*\]\([^)]*\)|<div[^>]*>\s*<img[^>]*>\s*</div>|<img[^>]*>`)

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
	result := ParseResult{OutputFormat: "json", Warnings: append([]string(nil), response.Warnings...)}
	hasPageCount := pageCount > 0
	if geometryErr != nil {
		result.Warnings = append(result.Warnings, "PaddleOCR PDF geometry unavailable: "+geometryErr.Error())
	}
	markdown, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		return ParseResult{Err: err}
	}
	markdown.FetchRemoteImages = false
	for _, page := range response.Pages {
		if !hasPageCount && page.Index+1 > pageCount {
			pageCount = page.Index + 1
		}
		for _, block := range page.Blocks {
			if err := ctx.Err(); err != nil {
				return ParseResult{Err: err}
			}
			content := strings.TrimSpace(paddleOCRImageMarkup.ReplaceAllString(block.Text, ""))
			if content == "" {
				continue
			}
			parsed := markdown.ParseWithResult(ctx, filename, []byte(content+"\n\n"))
			if parsed.Err != nil {
				return parsed
			}
			result.Warnings = append(result.Warnings, parsed.Warnings...)
			position := paddleOCRBlockPosition(page, block.BBox, sizes[page.Index])
			for _, item := range parsed.JSON {
				fields := item.ToMap()
				fields["page_number"] = page.Index + 1
				fields["_preserve_order"] = true
				label := strings.ToLower(strings.TrimSpace(block.Label))
				if label != "" {
					fields["layout"] = label
				}
				if label == "doc_title" || label == "paragraph_title" {
					fields["layout"] = "title"
					if fields["ck_type"] == "text" {
						prefix := "## "
						if label == "doc_title" {
							prefix = "# "
						}
						fields["text"] = prefix + fields["text"].(string)
						fields["ck_type"] = "heading"
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

func paddleOCRBlockPosition(page models.OCRPage, bbox []float64, size [2]float64) []any {
	if page.Index < 0 || !paddleOCRPositive(page.Width) || !paddleOCRPositive(page.Height) || !paddleOCRPositive(size[0]) || !paddleOCRPositive(size[1]) {
		return nil
	}
	if len(bbox) != 4 && len(bbox) != 8 {
		return nil
	}
	for _, v := range bbox {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
	}
	left, top, right, bottom := bbox[0], bbox[1], bbox[2], bbox[3]
	if len(bbox) == 8 {
		left, right, top, bottom = bbox[0], bbox[0], bbox[1], bbox[1]
		for i := 2; i < len(bbox); i += 2 {
			left = math.Min(left, bbox[i])
			right = math.Max(right, bbox[i])
			top = math.Min(top, bbox[i+1])
			bottom = math.Max(bottom, bbox[i+1])
		}
	}
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

func paddleOCRPositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
