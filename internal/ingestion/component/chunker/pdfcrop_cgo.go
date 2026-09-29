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
	"encoding/base64"
	"encoding/json"
	"image"
	"os"
	"runtime"
	"strconv"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/common"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	pdfpos "ragflow/internal/deepdoc/parser/pdf/type"
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

// cropConcurrency bounds how many chunks are cropped concurrently. The CGO
// pdfium render is serialized by pdfsync.Mu inside the engine, so raising this
// mostly parallelises the pure-Go crop + PNG-encode path (CropSectionImage →
// image/png), which was the dominant cost in the serial loop. Overridable via
// RAGFLOW_CROP_CONCURRENCY; defaults to GOMAXPROCS/3 (the crop path competes
// with the rest of the ingestor — tokenizer, uploads, extractions — so we use
// a third of the cores rather than saturating them), never below 1.
func cropConcurrency() int {
	if v := os.Getenv("RAGFLOW_CROP_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	// Use a third of the available cores so the crop fan-out does not starve
	// the rest of the ingestor pipeline; never drop below 1.
	n := runtime.GOMAXPROCS(0) / 3
	if n < 1 {
		n = 1
	}
	return n
}

// cropImageChunks crops image/table chunks and renders text previews (for
// text chunks that carry PDF positions, mirroring Python
// restore_pdf_text_previews). Each spanned page is rendered at most once and
// cached; the cache is shared across chunks so two chunks on the same page
// reuse one render.
//
// Concurrency: the per-chunk crop + PNG-encode (the dominant CPU cost,
// CropSectionImage → image/png) is fanned out across a bounded worker pool.
// The pdfium CGO render is serialized by pdfsync.Mu inside the engine, so
// concurrent renders are safe — only the Go-side encode runs in parallel.
func cropImageChunks(ctx context.Context, engine deepdoctype.PDFEngine, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	if engine == nil {
		return chunks
	}

	// cache bounds peak memory (reference counting + LRU, see lruPageCache)
	// so a thousands-of-pages PDF no longer spills every rendered page into
	// RAM. This replaces the serial code's sliding-window eviction, which
	// relied on in-order processing and is unsafe under fan-out. render is
	// called outside the lock; the warning on failure is logged here so the
	// cache stays render-agnostic and unit-testable.
	cache := newLRUPageCache(pageCacheLimit())
	render := func(pn int) (image.Image, bool) {
		img, rerr := deepdocpdf.RenderPageToImage(engine, pn)
		if rerr != nil || img == nil {
			common.Warn("cropImageChunks: render failed, skipping page",
				zap.Int("page", pn), zap.Error(rerr))
			return nil, false
		}
		return img, true
	}

	out := make([]schema.ChunkDoc, len(chunks))
	sem := make(chan struct{}, cropConcurrency())
	var wg sync.WaitGroup
	for i := range chunks {
		ck := chunks[i]
		out[i] = ck
		if !needsCrop(ck) || ck.Image != "" {
			continue
		}
		// A media chunk whose finalized text is empty — no caption, no vision
		// description, no media context — is not retrievable: its embedding
		// would be the empty-string vector, so retrieval can never surface it,
		// and every captionless image/table in a document would otherwise
		// collapse onto the same canonical id (ChunkID(docID, "")), each
		// overwrite the other's MinIO object. Skip the crop and upload
		// entirely; the finalizer then drops the chunk, so no object is
		// orphaned and no pdfium render / PNG encode / upload is wasted. This
		// mirrors Python's token_chunker drop of empty-text media chunks.
		// Text chunks are excluded: a text body always carries content, and the
		// Chunker-1.3 preview restore must still run for positioned text
		// regions.
		if isMediaChunk(ck) && canonicalChunkText(ck) == "" {
			continue
		}
		raw := ck.PDFPositions
		if len(raw) == 0 {
			raw = ck.Positions
		}
		var matrix [][]any
		if err := json.Unmarshal(raw, &matrix); err != nil || len(matrix) == 0 {
			continue
		}
		positions := util.PositionsFromMatrix(matrix)
		if len(positions) == 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, ck schema.ChunkDoc, positions []pdfpos.Position) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				return
			}
			single := make(map[int]image.Image, len(positions))
			acquired := make([]int, 0, len(positions))
			for _, pos := range positions {
				for _, pn := range pos.PageNumbers {
					if _, ok := single[pn]; ok {
						continue
					}
					if img, ok := cache.Acquire(render, pn); ok {
						single[pn] = img
						acquired = append(acquired, pn)
					}
				}
			}
			// Proceed whenever at least one spanned page resolved to an
			// image — freshly rendered or served from the page cache.
			if len(single) == 0 {
				return
			}
			img := util.CropSectionPositions(positions, single, deepdoctype.DlaScale)
			// Release the page bitmaps now that cropping is done. They are no
			// longer needed and may be evicted under the LRU cap; the image
			// below is encoded from single and does not depend on the pages.
			for _, pn := range acquired {
				cache.Release(pn)
			}
			if img == "" {
				return
			}
			out[i].Image = "data:image/png;base64," + img

			// Stream the freshly cropped preview to object storage and drop
			// the in-memory base64 immediately, instead of carrying every
			// chunk's image until the later batch upload pass
			// (imageUploadDecorator). The batch pass is idempotent: it skips
			// any chunk whose img_id is already set, so an upload that fails
			// here simply falls through to that retry path. kb_id is empty
			// only in canvas debug (dry-run) mode, where no persist stage
			// runs and the decorator's debug branch drops the raw bytes.
			if kbID, docID := resolveImageUploadContext(ctx, nil); kbID != "" {
				if raw, derr := base64.StdEncoding.DecodeString(img); derr == nil {
					// Key the streamed upload under the chunk's canonical id,
					// so the MinIO object is stored under exactly the key the
					// decorator (imageUploadDecorator) later exposes as
					// ck["id"] and the persist/retrieval path looks it up by.
					// canonicalChunkText is the single source for that id text:
					// it folds media context and strips position tags, so the
					// value equals the tag-stripped (finalized) text the
					// decorator derives — for every chunk type, including
					// image/table chunks whose text still carries position tags
					// when there is no media context. crop depends only on
					// positions, so reading the canonical text here does not
					// alter the cropped image or the chunker's later output
					// text.
					chunkID := canonicalChunkID(docID, out[i])
					if imgID, uerr := uploadOneImage(ctx, ChunkImageUploader, kbID, chunkID, raw); uerr == nil {
						out[i].ImgID = imgID
						out[i].Image = ""
						out[i].ID = chunkID
					} else {
						// Note: the document text is intentionally NOT logged
						// here (CWE-532). The upload is retried at the persist
						// stage, so the error is enough to diagnose.
						common.Warn("cropImageChunks: preview upload failed; will retry at persist stage",
							zap.Error(uerr))
					}
				}
			}
		}(i, ck, positions)
	}
	wg.Wait()
	return out
}

// needsCrop reports whether a chunk should be cropped to a page-region
// preview from its PDF positions. Image/table chunks get their media region
// cropped; text chunks with positions get a rendered preview of the text
// region (Python restore_pdf_text_previews). A pre-existing Image is never
// re-cropped — cropImageChunks honors that separately.
//
// CKType is the chunker-layer refinement (heading/table_header/table_row/text/
// image/table). General and token chunkers always set it before cropping, so
// they are handled by the CKType branch above. Group and hierarchy chunkers
// forward the parser's output verbatim, which carries only the coarser
// doc_type_kwd (no ck_type). For those, fall back to DocType so image/table/
// text regions are still cropped on demand — otherwise every figure/table/
// text preview in group/hierarchy would be silently skipped. When CKType is
// set (general/token) it takes priority, so headings (CKType "heading") stay
// excluded from preview cropping.
func needsCrop(ck schema.ChunkDoc) bool {
	typ := ck.CKType
	if typ == "" {
		typ = ck.DocType
	}
	switch typ {
	case "image", "table", "text":
		return len(ck.PDFPositions) > 0 || len(ck.Positions) > 0
	default:
		return false
	}
}
