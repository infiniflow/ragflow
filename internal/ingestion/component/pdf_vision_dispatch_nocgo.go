//go:build !cgo

package component

import (
	"context"
	"fmt"
)

func defaultRenderPDFVisionPages(_ []byte, _ [][]int) ([]pdfVisionPage, error) {
	return nil, fmt.Errorf("tenant-aware PDF IMAGE2TEXT backend requires cgo rendering support")
}

func mineruPageSizes(ctx context.Context, _ []byte) (map[int][2]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("PDF page geometry requires cgo")
}
