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
	"errors"
	"image"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
	"ragflow/internal/common"
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

const visionPDFSourceBudget = 90 * time.Second

type visionPageSizer interface {
	PageSize(pageNum int) (float64, float64, error)
}

var _ visionPageSizer = (*deepdocpdf.PDFOxideEngine)(nil)

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
	invocationCtx context.Context
	sourceCtx     context.Context
	cancelSource  context.CancelFunc
	db            *gorm.DB
	inputs        map[string]any

	mu          sync.Mutex
	initialized bool
	engine      deepdoctype.PDFEngine
	engErr      error
	sourceErr   error
}

// newVisionImageCropper builds the on-demand cropper. It never touches storage
// here; the source PDF is re-acquired lazily on the first Crop call that needs
// it.
func newVisionImageCropper(ctx context.Context, db *gorm.DB, inputs map[string]any) (visionImageCropper, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceCtx, cancelSource := context.WithTimeout(ctx, visionPDFSourceBudget)
	return &visionPDFCropper{
		invocationCtx: ctx,
		sourceCtx:     sourceCtx,
		cancelSource:  cancelSource,
		db:            db,
		inputs:        inputs,
	}, nil
}

func (c *visionPDFCropper) Crop(ctx context.Context, item map[string]any) (*visionImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	if err := c.invocationCtx.Err(); err != nil {
		return nil, err
	}
	// Source acquisition has its own invocation-level budget and can outlive
	// this item's materialization deadline. Once the shared source is ready,
	// give rendering a fresh per-item budget while still honoring invocation
	// cancellation.
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	renderCtx, cancelRender := context.WithTimeout(c.invocationCtx, visionMediaItemBudget)
	defer cancelRender()
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
	if !pdfPagesRasterWithinVisionLimits(c.engine, pages) {
		return nil, nil
	}
	single := make(map[int]image.Image, len(pages))
	for pn := range pages {
		if err := renderCtx.Err(); err != nil {
			return nil, err
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
	raster := util.CropSectionPositionsRasterLimited(positions, single, deepdoctype.DlaScale, maxVisionImagePixels)
	if raster == nil {
		return nil, nil
	}
	return &visionImage{Raster: raster}, nil
}

func pdfPagesRasterWithinVisionLimits(engine deepdoctype.PDFEngine, pageNums map[int]struct{}) bool {
	sizer, ok := engine.(visionPageSizer)
	if !ok {
		return false
	}
	var totalPixels int64
	for pageNum := range pageNums {
		widthPoints, heightPoints, err := sizer.PageSize(pageNum)
		if err != nil {
			return false
		}
		widthPixels := math.Ceil(widthPoints * deepdoctype.DlaScale)
		heightPixels := math.Ceil(heightPoints * deepdoctype.DlaScale)
		if math.IsNaN(widthPixels) || math.IsInf(widthPixels, 0) ||
			math.IsNaN(heightPixels) || math.IsInf(heightPixels, 0) ||
			widthPixels <= 0 || heightPixels <= 0 ||
			widthPixels > maxVisionImageEdge || heightPixels > maxVisionImageEdge ||
			widthPixels*heightPixels > float64(maxVisionImagePixels) {
			return false
		}
		pagePixels := int64(widthPixels * heightPixels)
		if pagePixels > maxVisionImagePixels-totalPixels {
			return false
		}
		totalPixels += pagePixels
	}
	return true
}

func (c *visionPDFCropper) ensureEngine() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initialized {
		return c.engErr
	}
	if err := c.sourceCtx.Err(); err != nil {
		c.initialized = true
		c.engErr = err
		c.sourceErr = err
		return err
	}
	data, err := c.acquireSource(c.sourceCtx)
	if err != nil {
		c.sourceErr = err
		if c.sourceCtx.Err() != nil {
			c.initialized = true
			c.engErr = err
		} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			c.initialized = true
			c.engErr = err
		}
		return err
	}
	if err := c.sourceCtx.Err(); err != nil {
		c.initialized = true
		c.engErr = err
		c.sourceErr = err
		return err
	}
	c.sourceErr = nil
	// Empty and non-PDF sources are deterministic no-op results for this
	// invocation. A transient fetch error can be retried by a later item while
	// the shared source context remains active.
	c.initialized = true
	if len(data) == 0 || len(data) < 5 || string(data[:5]) != "%PDF-" {
		return nil
	}
	eng, err := visionEngineOpener(data)
	if err != nil {
		c.engErr = err
		c.sourceErr = err
		return err
	}
	c.engine = eng
	return c.engErr
}

func (c *visionPDFCropper) acquireSource(ctx context.Context) ([]byte, error) {
	if bucket, _ := getString(c.inputs, "bucket"); bucket != "" {
		if path, _ := getString(c.inputs, "path"); path != "" {
			return visionSourceFetcher(ctx, bucket, path)
		}
	}
	if docID, _ := getString(c.inputs, "doc_id"); docID != "" {
		ref, err := ResolveDocumentStorage(ctx, c.db, docID)
		if err != nil || ref == nil {
			return nil, err
		}
		return visionSourceFetcher(ctx, ref.Bucket, ref.Path)
	}
	return nil, nil
}

func (c *visionPDFCropper) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelSource()
	if c.sourceErr != nil && c.invocationCtx.Err() == nil {
		common.Warn("vision enhancement: source PDF unavailable for VLM crop", zap.Error(c.sourceErr))
		c.sourceErr = nil
	}
	if c.engine != nil {
		return c.engine.Close()
	}
	return nil
}
