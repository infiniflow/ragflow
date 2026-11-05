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
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	agentruntime "ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"

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

// cropJob is one chunk's crop + PNG-encode work.
type cropJob func(context.Context)

const cropPoolQueueFactor = 4

var (
	cropPoolOnce sync.Once
	cropPool     *utility.WorkerPool[cropJob, struct{}]
)

// cropWorkerPool returns the process-wide crop worker pool. Concurrent
// documents share this pool, so crop concurrency is capped globally instead of
// per cropImageChunks call. It is created lazily so its default size sees the
// page concurrency registered at server start (pdf.SetPageConcurrency); package
// initialization would freeze the pre-config default.
func cropWorkerPool() *utility.WorkerPool[cropJob, struct{}] {
	cropPoolOnce.Do(func() {
		n := cropConcurrency()
		cropPool = utility.NewWorkerPool[cropJob, struct{}](
			n, n*cropPoolQueueFactor,
			func(ctx context.Context, job cropJob) (struct{}, error) {
				job(ctx)
				return struct{}{}, nil
			},
		)
	})
	return cropPool
}

// cropConcurrency returns the process-wide crop worker count. The CGO pdfium
// render is serialized by pdfsync.Mu inside the engine, so raising this mostly
// parallelises the pure-Go crop + PNG-encode path (CropSectionImage ->
// image/png), which was the dominant cost in the serial loop. Crop reuses the
// existing process-wide PDF page budget (pdf.SetPageConcurrency, resolved from
// CLI > env > config), so there is one source of truth for PDF concurrency.
func cropConcurrency() int {
	if n := deepdocpdf.PageConcurrency(); n > 0 {
		return n
	}
	return 1
}

// cropImageChunks crops image/table chunks and renders text previews (for
// text chunks that carry PDF positions, mirroring Python
// restore_pdf_text_previews). Each spanned page is rendered at most once and
// cached; the cache is shared across chunks so two chunks on the same page
// reuse one render.
//
// Concurrency: the per-chunk crop + PNG-encode (the dominant CPU cost,
// CropSectionImage -> image/png) is dispatched to the process-wide crop worker
// pool. The pdfium CGO render is serialized by pdfsync.Mu inside the engine, so
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
	cache := newLRUPageCache(pageCacheLimit(), pageCacheBytes())
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
	var candidateIndices []int
	for i := range chunks {
		ck := chunks[i]
		out[i] = ck
		if !needsCrop(ck) || ck.Image != "" {
			continue
		}
		// A media chunk whose finalized text is empty - no caption, no vision
		// description, no media context - is not retrievable: its embedding
		// would be the empty-string vector, so retrieval can never surface it,
		// and every captionless image/table in a document would otherwise
		// collapse onto the same canonical id (ChunkID(docID, "")), each
		// overwrite the other's MinIO object. Skip the crop and upload
		// entirely; the finalizer then drops the chunk, so no object is
		// orphaned and no pdfium render / PNG encode / upload is wasted. This
		// mirrors Python's token_chunker drop of empty-text media chunks.
		// Text chunks are excluded: a text body always carries content, and the
		// Chunker-1.3 preview restore must still run for positioned text
		// regions. Region-based chunk ids still apply when context or captions
		// make the crop path run.
		if isMediaChunk(ck) && canonicalChunkText(ck) == "" {
			continue
		}
		raw := cropPositionMatrix(ck)
		if len(raw) == 0 {
			continue
		}
		candidateIndices = append(candidateIndices, i)
	}
	if len(candidateIndices) == 0 {
		return out
	}

	total := int64(len(candidateIndices))
	var settled atomic.Int64
	pool := cropWorkerPool()
	futs := make([]utility.WorkerPoolFuture[cropJob, struct{}], 0, len(candidateIndices))
	for _, idx := range candidateIndices {
		if ctx.Err() != nil {
			break
		}
		chunkIdx := idx
		fut, err := pool.Submit(ctx, func(jobCtx context.Context) {
			if jobCtx.Err() != nil {
				return
			}
			defer func() {
				if jobCtx.Err() == nil {
					done := settled.Add(1)
					agentruntime.ReportComponentFraction(jobCtx, float64(done)/float64(total))
				}
			}()

			ck := out[chunkIdx]
			raw := cropPositionMatrix(ck)
			matrix, ok := decodePositionRows(raw)
			if !ok || len(matrix) == 0 {
				return
			}
			positions := util.PositionsFromMatrix(matrix)
			if len(positions) == 0 {
				return
			}

			img := cropChunkPreview(cache, render, cropChunkType(ck), positions)
			if img == "" {
				return
			}
			out[chunkIdx].Image = "data:image/png;base64," + img

			// Stream the freshly cropped preview to object storage and drop
			// the in-memory base64 immediately, instead of carrying every
			// chunk's image until the later batch upload pass
			// (imageUploadDecorator). The batch pass is idempotent: it skips
			// any chunk whose img_id is already set, so an upload that fails
			// here simply falls through to that retry path. kb_id is empty
			// only in canvas debug (dry-run) mode, where no persist stage
			// runs and the decorator's debug branch drops the raw bytes.
			if kbID, docID := resolveImageUploadContext(jobCtx, nil); kbID != "" {
				if rawBytes, derr := base64.StdEncoding.DecodeString(img); derr == nil {
					// Key the streamed upload under the chunk's canonical id,
					// so the MinIO object is stored under exactly the key the
					// decorator (imageUploadDecorator) later exposes as
					// ck["id"] and the persist/retrieval path looks it up by.
					// Preserve a pre-stamped position-only media id; otherwise
					// derive it from canonical text (folded media context and
					// stripped position tags), matching the decorator's text.
					chunkID := canonicalChunkID(docID, out[chunkIdx])
					if imgID, uerr := uploadOneImage(jobCtx, ChunkImageUploader, kbID, chunkID, rawBytes); uerr == nil {
						out[chunkIdx].ImgID = imgID
						out[chunkIdx].Image = ""
						out[chunkIdx].ID = chunkID
					} else {
						// Note: the document text is intentionally NOT logged
						// here (CWE-532). The upload is retried at the persist
						// stage, so the error is enough to diagnose.
						common.Warn("cropImageChunks: preview upload failed; will retry at persist stage",
							zap.Error(uerr))
					}
				}
			}
		})
		if err != nil {
			break
		}
		futs = append(futs, fut)
	}
	for _, fut := range futs {
		// Wait with a non-canceled context: a canceled job is skipped by the
		// pool, but an already-running job must finish before out is returned.
		res, _ := fut.Wait(context.Background())
		if res.Err != nil && ctx.Err() == nil {
			common.Warn("cropImageChunks: crop worker failed", zap.Error(res.Err))
		}
	}
	if ctx.Err() == nil {
		agentruntime.ReportComponentFraction(ctx, 1.0)
	}
	return out
}

// cropChunkPreview crops the preview of one chunk from the pages its positions
// span and returns it PNG-encoded in base64, or "" when no page resolved.
// The pages are released when cropping ends, even if it panics: the crop pool
// recovers the panic, and a page left referenced would stay pinned in cache.
func cropChunkPreview(cache *lruPageCache, render func(int) (image.Image, bool), chunkType string, positions []deepdoctype.Position) string {
	pages := make(map[int]image.Image, len(positions))
	defer func() {
		for pn := range pages {
			cache.Release(pn)
		}
	}()
	for _, pos := range positions {
		for _, pn := range pos.PageNumbers {
			if _, ok := pages[pn]; ok {
				continue
			}
			if img, ok := cache.Acquire(render, pn); ok {
				pages[pn] = img
			}
		}
	}
	// Proceed whenever at least one spanned page resolved to an image —
	// freshly rendered or served from the page cache.
	if len(pages) == 0 {
		return ""
	}
	if chunkType == "text" {
		return util.CropTextSectionPositions(positions, pages, deepdoctype.DlaScale)
	}
	// Media previews are clean position-only crops (Python cropout): no 120pt
	// context bands, no edge dimming. The text path above is the only one that
	// keeps context.
	return util.CropMediaSectionPositions(positions, pages, deepdoctype.DlaScale)
}

// needsCrop reports whether a chunk should be cropped to a page-region
// preview from its PDF positions. Image/table chunks get their media region
// cropped; text chunks with positions get a rendered preview of the text
// region (Python restore_pdf_text_previews). A pre-existing Image is never
// re-cropped — cropImageChunks honors that separately.
//
// The content type on doc_type_kwd alone decides the crop kind. Headings are
// text chunks and get the same text preview as any other body text, matching
// the historical behavior.
func needsCrop(ck schema.ChunkDoc) bool {
	switch cropChunkType(ck) {
	case "image", "table", "text":
		return hasCropRegion(ck)
	default:
		return false
	}
}

// cropPositionMatrix selects _pdf_positions when it contains a positive-area
// box, otherwise positions. Inspect every row so a page-only first row does
// not hide a crop region in a later row. Return nil if neither has a region.
func cropPositionMatrix(ck schema.ChunkDoc) json.RawMessage {
	for _, raw := range []json.RawMessage{ck.PDFPositions, ck.Positions} {
		for _, row := range parser.PDFPositionRowsFromJSON(raw) {
			if row[2] > row[1] && row[4] > row[3] {
				return raw
			}
		}
	}
	return nil
}

// hasCropRegion reports whether the chunk carries a position with a
// positive-area box. A page-only position (page number, no geometry) carries
// page identity for the indexer but no region, so it must not trigger a crop.
func hasCropRegion(ck schema.ChunkDoc) bool {
	return len(cropPositionMatrix(ck)) > 0
}

// cropChunkType returns the content type that selects the crop. Callers must
// pass normalized chunks: General/Token clear structural layout before
// cropping, so passing raw parser items would lose their block-kind
// distinction here.
func cropChunkType(ck schema.ChunkDoc) string {
	return itemDocType(ck)
}
