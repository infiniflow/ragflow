//go:build cgo

package parser

import (
	"context"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/entity/models"
)

func paddleOCRPageSizes(ctx context.Context, data []byte, pages []models.OCRPage) (map[int][2]float64, int, error) {
	indexes := make([]int, 0, len(pages))
	for _, page := range pages {
		if len(page.Blocks) > 0 {
			indexes = append(indexes, page.Index)
		}
	}
	return pdfium.PageSizes(ctx, data, indexes)
}
