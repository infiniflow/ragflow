//go:build cgo

// Package chunker — on-demand PDF section cropping.
//
// When the upstream Parser forwards storage references (doc_id / bucket /
// path) for a PDF, the chunker re-acquires the source bytes and crops
// image/table sections on demand, instead of carrying the rendered images
// (or the raw binary) across the component boundary. This matches the
// Python pipeline, where pdf_parser.crop() runs at tokenize time, and keeps
// peak memory bounded to one page render per cropped section.
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log/slog"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

// newPDFEngineFromUpstream re-acquires the source PDF from storage using the
// same resolution the Parser uses, then opens a native engine. It returns
// (nil, nil) when no storage reference is present or the bytes are not a PDF,
// so callers can treat a nil engine as "no cropping".
func newPDFEngineFromUpstream(ctx context.Context, db *gorm.DB, up schema.ChunkerFromUpstream) (deepdoctype.PDFEngine, error) {
	var data []byte
	var err error
	switch {
	case up.Bucket != "" && up.Path != "":
		data, err = component.FetchBinary(ctx, up.Bucket, up.Path)
	case up.DocID != "":
		var ref *component.DocumentStorageRef
		ref, err = component.ResolveDocumentStorage(ctx, db, up.DocID)
		if err == nil && ref != nil {
			data, err = component.FetchBinary(ctx, ref.Bucket, ref.Path)
		}
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Only PDFs can be cropped. Guard against other binary types so a
	// non-PDF pipeline that happens to forward doc_id stays a no-op.
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return nil, nil
	}
	return deepdocpdf.NewEngine(data)
}

// cropImageChunks crops image/table chunks and renders text previews (for
// text chunks that carry PDF positions, mirroring Python
// restore_pdf_text_previews). Each spanned page is rendered at most once and
// cached, then the crop runs concurrently across chunks.
//
// Parallelism: every chunk's crop (page render + CropSectionPositions + PNG
// encode) is independent. Page rendering is serialized internally by the
// engine's pdfsync.Mu, so concurrent renders are safe; the shared pageCache is
// guarded by a mutex so each page is rendered at most once. The crop itself
// only reads the (read-only) cached page images and writes a fresh output
// image, so it runs off the lock. This bounds per-document wall time to the
// slowest single chunk instead of the sum of all chunks — a large win now that
// text chunks also take this path (see needsCrop).
//
// Because chunks are no longer processed in document order, the sequential
// sliding-window eviction is replaced by a bounded, concurrency-safe cache
// (pageImageCache). Its footprint is capped at a fixed number of rendered
// pages regardless of document length, so a large PDF cannot make ingestion
// memory grow without bound; pages evicted under pressure are re-rendered on
// demand (the engine serializes renders, so this is safe — merely redundant
// work for a cold page), while the hot working set stays cached.
func cropImageChunks(ctx context.Context, engine deepdoctype.PDFEngine, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	if engine == nil {
		return chunks
	}
	out := make([]schema.ChunkDoc, len(chunks))
	// pageCache is shared across goroutines and bounds peak memory to
	// cacheCap rendered pages. Cached images are read-only during crop, so
	// no lock is needed there.
	cache := newPageImageCache(2 * (runtime.NumCPU() + 1))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU() + 1)
	for i := range chunks {
		i := i
		ck := chunks[i]
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			out[i] = ck
			if !needsCrop(ck) || ck.Image != "" {
				return nil
			}
			raw := ck.PDFPositions
			if len(raw) == 0 {
				raw = ck.Positions
			}
			var matrix [][]any
			if err := json.Unmarshal(raw, &matrix); err != nil || len(matrix) == 0 {
				return nil
			}
			positions := util.PositionsFromMatrix(matrix)
			if len(positions) == 0 {
				return nil
			}
			single := make(map[int]image.Image, len(positions))
			for _, pos := range positions {
				for _, pn := range pos.PageNumbers {
					if _, ok := single[pn]; ok {
						continue
					}
					img, rerr := cache.render(engine, pn)
					if rerr != nil {
						slog.Warn("cropImageChunks: render failed, skipping page",
							"page", pn, "err", rerr)
						continue
					}
					single[pn] = img
				}
			}
			// Proceed whenever at least one spanned page resolved to an
			// image — whether freshly rendered or served from the page cache.
			if len(single) == 0 {
				return nil
			}
			img := util.CropSectionPositions(positions, single, deepdoctype.DlaScale)
			if img == "" {
				return nil
			}
			out[i].Image = "data:image/png;base64," + img
			return nil
		})
	}
	g.Wait()
	return out
}

// pageImageCache is a concurrency-safe, size-bounded cache of rendered PDF
// pages keyed by page number. It caps peak memory at cacheCap full-page images
// regardless of document length. When the cache is full, storing a new page
// evicts one existing entry; an evicted page that is still needed later is
// simply re-rendered (the engine serializes renders, so this is safe — only
// redundant work). The cap is sized to hold the typical working set of a
// parallel crop pass; bounded memory matters more than avoiding rare
// re-renders on very large documents.
type pageImageCache struct {
	mu  sync.Mutex
	m   map[int]image.Image
	cap int
}

func newPageImageCache(capacity int) *pageImageCache {
	if capacity < 1 {
		capacity = 1
	}
	return &pageImageCache{m: make(map[int]image.Image, capacity), cap: capacity}
}

func (c *pageImageCache) get(pn int) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	img, ok := c.m[pn]
	return img, ok
}

// put stores img for pn, evicting one arbitrary entry (under the lock) when
// the cache is at capacity. A no-op if pn is already cached.
func (c *pageImageCache) put(pn int, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[pn]; ok {
		return
	}
	if len(c.m) >= c.cap {
		for k := range c.m {
			delete(c.m, k)
			break
		}
	}
	c.m[pn] = img
}

// render returns the cached image for pn, rendering and caching it on a miss.
// Two goroutines can race to render the same missing page; both results are
// valid and the second put is a no-op, so the redundant render is harmless.
func (c *pageImageCache) render(engine deepdoctype.PDFEngine, pn int) (image.Image, error) {
	if img, ok := c.get(pn); ok {
		return img, nil
	}
	img, rerr := deepdocpdf.RenderPageToImage(engine, pn)
	if rerr != nil {
		return nil, rerr
	}
	if img == nil {
		return nil, fmt.Errorf("nil image for page %d", pn)
	}
	c.put(pn, img)
	return img, nil
}

// needsCrop reports whether a chunk should be cropped to a page-region
// preview from its PDF positions. Image/table chunks get their media region
// cropped; text chunks with positions get a rendered preview of the text
// region (Python restore_pdf_text_previews). A pre-existing Image is never
// re-cropped — cropImageChunks honors that separately.
func needsCrop(ck schema.ChunkDoc) bool {
	switch ck.CKType {
	case "image", "table", "text":
		return len(ck.PDFPositions) > 0 || len(ck.Positions) > 0
	default:
		return false
	}
}
