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

	"go.uber.org/zap"

	"ragflow/internal/common"
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
// restore_pdf_text_previews). Each spanned page is
// rendered at most once. Chunks arrive in document order, so we keep only a
// sliding window of page images: once we advance past a chunk whose minimum
// page is P, no later chunk references a page < P, and we evict those entries
// from pageCache. This bounds peak memory to the pages spanned by the recent
// window (typically one page per chunk) instead of holding every rendered
// page for the whole call. The pdfsync.Mu serializer inside the engine makes
// concurrent renders safe, but we render sequentially here since the caller
// fans out across chunks.
func cropImageChunks(ctx context.Context, engine deepdoctype.PDFEngine, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	if engine == nil {
		return chunks
	}
	pageCache := make(map[int]image.Image)
	out := make([]schema.ChunkDoc, len(chunks))
	for i, ck := range chunks {
		out[i] = ck
		if !needsCrop(ck) || ck.Image != "" {
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
		// Minimum page this chunk touches; used to prune stale cache entries.
		minPage := -1
		for _, pos := range positions {
			for _, pn := range pos.PageNumbers {
				if pn < minPage || minPage < 0 {
					minPage = pn
				}
			}
		}
		// Evict page images that no later chunk can reference (all future
		// chunks start at page >= minPage).
		for pn := range pageCache {
			if pn < minPage {
				delete(pageCache, pn)
			}
		}
		single := make(map[int]image.Image, len(positions))
		for _, pos := range positions {
			for _, pn := range pos.PageNumbers {
				if _, ok := single[pn]; ok {
					continue
				}
				if img, ok := pageCache[pn]; ok {
					single[pn] = img
					continue
				}
				img, rerr := deepdocpdf.RenderPageToImage(engine, pn)
				if rerr != nil || img == nil {
					common.Warn("cropImageChunks: render failed, skipping page",
						zap.Int("page", pn), zap.Error(rerr))
					continue
				}
				pageCache[pn] = img
				single[pn] = img
			}
		}
		// Proceed whenever at least one spanned page resolved to an
		// image — whether freshly rendered or served from the page cache
		// (the latter happens for the second chunk reusing page 0).
		if len(single) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out
		}
		img := util.CropSectionPositions(positions, single, deepdoctype.DlaScale)
		if img == "" {
			continue
		}
		out[i].Image = "data:image/png;base64," + img

		// Stream the freshly cropped preview to object storage and drop the
		// in-memory base64 immediately, instead of carrying every chunk's
		// image until the later batch upload pass (imageUploadDecorator).
		// For a large PDF this bounds peak Go-heap retention during the
		// chunker stage to the in-flight set of cropped images (a bounded
		// number of chunks) rather than the whole document's worth of base64
		// previews. The batch pass is idempotent:
		// it skips any chunk whose img_id is already set, so an upload that
		// fails here simply falls through to that retry path. kb_id is empty
		// only in canvas debug (dry-run) mode, where no persist stage runs
		// and the decorator's debug branch drops the raw bytes instead.
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
	}
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
