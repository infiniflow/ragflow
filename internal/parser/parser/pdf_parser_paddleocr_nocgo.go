//go:build !cgo

package parser

import (
	"context"
	"fmt"

	"ragflow/internal/entity/models"
)

func paddleOCRPageSizes(ctx context.Context, _ []byte, _ []models.OCRPage) (map[int][2]float64, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	return nil, 0, fmt.Errorf("PDF page geometry requires cgo")
}
