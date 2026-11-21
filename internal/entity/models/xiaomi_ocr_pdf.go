//go:build cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/utility"
)

func xiaomiOCRPDF(ctx context.Context, content []byte, options *OCRConfig, recognize func(image.Image) (string, error)) (*OCRFileResponse, error) {
	_, count, err := pdfium.PageSizes(ctx, content, nil)
	if err != nil {
		return nil, err
	}
	var indexes []int
	for i := 0; i < count; i++ {
		if options == nil || utility.PDFPageInRanges(options.Pages, i+1) {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) == 0 {
		return nil, fmt.Errorf("XiaomiOCR: no PDF pages selected")
	}
	sizes, _, err := pdfium.PageSizes(ctx, content, indexes)
	if err != nil {
		return nil, err
	}
	var pages []OCRPage
	var texts []string
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		size := sizes[i]
		// Bound the bitmap at render time, before allocating memory for large sheets.
		dpi := math.Min(150, 72*xiaomiOCRMaxEdge/math.Max(size[0], size[1]))
		page, err := pdfium.RenderPage(content, i, dpi)
		if err != nil {
			return nil, fmt.Errorf("XiaomiOCR render page %d: %w", i+1, err)
		}
		text, err := recognize(page)
		if err != nil {
			return nil, fmt.Errorf("XiaomiOCR page %d: %w", i+1, err)
		}
		texts = append(texts, text)
		// The model gives reading order for the page, without region coordinates.
		pages = append(pages, OCRPage{Index: i, Blocks: []OCRBlock{{Text: text}}})
	}
	markdown := strings.Join(texts, "\n\n")
	return &OCRFileResponse{Text: &markdown, Pages: pages}, nil
}
