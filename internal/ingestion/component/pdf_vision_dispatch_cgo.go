//go:build cgo

package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/utility"
)

const pdfVisionZoom = 3.0

// mineruPageSizes maps each zero-based page index to its effective size in PDF
// points. MinerU content_list boxes are normalized to 0-1000, so both axes
// need the real page box before they can become positions.
func mineruPageSizes(ctx context.Context, binary []byte) (map[int][2]float64, error) {
	_, count, err := pdfium.PageSizes(ctx, binary, nil)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, fmt.Errorf("PDF has no pages")
	}
	indexes := make([]int, count)
	for i := range indexes {
		indexes[i] = i
	}
	sizes, _, err := pdfium.PageSizes(ctx, binary, indexes)
	if err != nil {
		return sizes, err
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("no readable page sizes")
	}
	return sizes, nil
}

func defaultRenderPDFVisionPages(binary []byte, ranges [][]int) ([]pdfVisionPage, error) {
	engine, err := deepdocpdf.NewEngine(binary)
	if err != nil {
		return nil, err
	}
	defer engine.Close()

	pageCount, err := engine.PageCount()
	if err != nil {
		return nil, err
	}
	pages := make([]pdfVisionPage, 0, pageCount)
	for pageIdx := 0; pageIdx < pageCount; pageIdx++ {
		if !utility.PDFPageInRanges(ranges, pageIdx+1) {
			continue
		}
		img, err := deepdocpdf.RenderPageToImage(engine, pageIdx)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", pageIdx+1, err)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("page %d encode png: %w", pageIdx+1, err)
		}
		bounds := img.Bounds()
		pages = append(pages, pdfVisionPage{
			PageNumber: pageIdx + 1,
			WidthPts:   float64(bounds.Dx()) / pdfVisionZoom,
			HeightPts:  float64(bounds.Dy()) / pdfVisionZoom,
			ImageURL:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
		})
	}
	return pages, nil
}
