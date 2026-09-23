//go:build cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"context"
	"image"
	"math"
	"sync"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/parser/parser"

	"gorm.io/gorm"
)

// visionSourceFetcher and visionEngineOpener are the injection seams for the
// on-demand crop path. They default to the production storage/engine resolvers
// but are overridable in tests so the cropper can be exercised without a real
// PDF or storage backend.
var (
	visionSourceFetcher func(ctx context.Context, bucket, path string) ([]byte, error)
	visionEngineOpener  func(data []byte) (deepdoctype.PDFEngine, error)
)

func init() {
	visionSourceFetcher = FetchBinary
	visionEngineOpener = deepdocpdf.NewEngine
}

// visionPDFCropper crops section images on demand from the source PDF. Parser
// output for PDF no longer carries an inlined base64 image (that is what this
// change removes to bound parser-phase memory); instead each item carries PDF
// positions, and the VLM path re-acquires the source bytes, opens a fresh
// engine, renders the spanned page(s), and crops the region — exactly what the
// chunker does at index time. The engine is opened lazily and at most once per
// vision-enhancement call, then released via Close.
type visionPDFCropper struct {
	ctx    context.Context
	db     *gorm.DB
	inputs map[string]any

	once   sync.Once
	engine deepdoctype.PDFEngine
	engErr error
}

// newVisionImageCropper builds the on-demand cropper. It never touches storage
// here; the source PDF is re-acquired lazily on the first Crop call that needs
// it.
func newVisionImageCropper(ctx context.Context, db *gorm.DB, inputs map[string]any) (visionImageCropper, error) {
	return &visionPDFCropper{ctx: ctx, db: db, inputs: inputs}, nil
}

func (c *visionPDFCropper) Crop(item map[string]any) (*visionImage, error) {
	// Fast path: an inlined image (docx/markdown, or any pre-inlined source)
	// is used directly — no storage access, no engine.
	if img, _ := item["image"].(string); img != "" {
		return materializeInlineVisionImage(img)
	}
	matrix, ok := parser.ExtractPDFPositions(item)
	if !ok {
		return nil, nil
	}
	positions := util.PositionsFromMatrix(matrix)
	if len(positions) == 0 {
		return nil, nil
	}
	if err := c.ensureEngine(); err != nil {
		// Best-effort: a missing/unreadable source PDF means no vision
		// description for this item, not a hard failure.
		return nil, nil
	}
	if c.engine == nil {
		return nil, nil
	}
	// Render each distinct page the positions span (1-based → 0-based is
	// handled by PositionsFromMatrix). Reuse the chunker's sliding-window
	// idea but scope it to this single item: typically one page.
	pages := make(map[int]struct{}, len(positions))
	for _, pos := range positions {
		for _, pn := range pos.PageNumbers {
			pages[pn] = struct{}{}
		}
	}
	single := make(map[int]image.Image, len(pages))
	for pn := range pages {
		if !pdfPageRasterWithinOCRLimits(c.engine, pn) {
			return nil, nil
		}
		img, rerr := deepdocpdf.RenderPageToImage(c.engine, pn)
		if rerr != nil || img == nil {
			continue
		}
		single[pn] = img
	}
	if len(single) == 0 {
		return nil, nil
	}
	raster := util.CropSectionPositionsRaster(positions, single, deepdoctype.DlaScale)
	if raster == nil {
		return nil, nil
	}
	return &visionImage{Raster: raster}, nil
}

func pdfPageRasterWithinOCRLimits(engine deepdoctype.PDFEngine, pageNum int) bool {
	sizer, ok := engine.(interface {
		PageSize(int) (float64, float64, error)
	})
	if !ok {
		return true
	}
	widthPoints, heightPoints, err := sizer.PageSize(pageNum)
	if err != nil {
		return false
	}
	widthPixels := math.Ceil(widthPoints * deepdoctype.DlaScale)
	heightPixels := math.Ceil(heightPoints * deepdoctype.DlaScale)
	return widthPixels > 0 && heightPixels > 0 &&
		widthPixels <= maxOCRImageEdge && heightPixels <= maxOCRImageEdge &&
		widthPixels*heightPixels <= maxOCRImagePixels
}

func (c *visionPDFCropper) ensureEngine() error {
	c.once.Do(func() {
		data, err := c.acquireSource()
		if err != nil || len(data) == 0 {
			c.engErr = err
			return
		}
		// Only PDFs can be cropped. Guard against other binary types so a
		// docx/markdown item that happens to reach here stays a no-op.
		if len(data) < 5 || string(data[:5]) != "%PDF-" {
			return
		}
		eng, oerr := visionEngineOpener(data)
		if oerr != nil {
			c.engErr = oerr
			return
		}
		c.engine = eng
	})
	return c.engErr
}

func (c *visionPDFCropper) acquireSource() ([]byte, error) {
	if bucket, _ := getString(c.inputs, "bucket"); bucket != "" {
		if path, _ := getString(c.inputs, "path"); path != "" {
			return visionSourceFetcher(c.ctx, bucket, path)
		}
	}
	if docID, _ := getString(c.inputs, "doc_id"); docID != "" {
		ref, err := ResolveDocumentStorage(c.ctx, c.db, docID)
		if err != nil || ref == nil {
			return nil, err
		}
		return visionSourceFetcher(c.ctx, ref.Bucket, ref.Path)
	}
	return nil, nil
}

func (c *visionPDFCropper) Close() error {
	if c.engine != nil {
		return c.engine.Close()
	}
	return nil
}
