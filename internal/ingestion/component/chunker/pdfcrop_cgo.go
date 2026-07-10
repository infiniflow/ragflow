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
	"image"
	"log/slog"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
)

// newPDFEngineFromUpstream re-acquires the source PDF from storage using the
// same resolution the Parser uses, then opens a native engine. It returns
// (nil, nil) when no storage reference is present or the bytes are not a PDF,
// so callers can treat a nil engine as "no cropping".
func newPDFEngineFromUpstream(ctx context.Context, up schema.ChunkerFromUpstream) (deepdoctype.PDFEngine, error) {
	var data []byte
	var err error
	switch {
	case up.Bucket != "" && up.Path != "":
		data, err = component.FetchBinary(ctx, up.Bucket, up.Path)
	case up.DocID != "":
		var ref *component.DocumentStorageRef
		ref, err = component.ResolveDocumentStorage(up.DocID)
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

// cropImageChunks crops image/table chunks in place. Each spanned page is
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
					slog.Warn("cropImageChunks: render failed, skipping page",
						"page", pn, "err", rerr)
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
	}
	return out
}

func needsCrop(ck schema.ChunkDoc) bool {
	switch ck.CKType {
	case "image", "table":
		return len(ck.PDFPositions) > 0 || len(ck.Positions) > 0
	default:
		return false
	}
}
