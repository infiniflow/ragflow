//go:build cgo

package parser

import (
	"context"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
)

// openDataLoaderPageGeometry returns the requested pages' sizes in PDF points
// and the document page count, which the adapter needs to flip
// OpenDataLoader's bottom-left coordinates into the screen-space rows
// positions use. One document load serves both values.
func openDataLoaderPageGeometry(ctx context.Context, data []byte, indexes []int) (map[int][2]float64, int, error) {
	return pdfium.PageSizes(ctx, data, indexes)
}
