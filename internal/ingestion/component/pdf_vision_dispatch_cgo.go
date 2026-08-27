//go:build cgo

package component

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
)

const pdfVisionZoom = 3.0

func defaultRenderPDFVisionPages(binary []byte) ([]pdfVisionPage, error) {
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
