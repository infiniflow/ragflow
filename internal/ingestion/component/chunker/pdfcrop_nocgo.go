//go:build !cgo

// Package chunker — no-CGO fallback for on-demand PDF section cropping.
//
// Without the native PDF backend there is nothing to crop, so the engine is
// always nil and chunks pass through unchanged. The signatures mirror the
// cgo file so untagged callers can call them unconditionally.
package chunker

import (
	"context"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

func newPDFEngineFromUpstream(_ context.Context, _ *gorm.DB, _ schema.ChunkerFromUpstream) (deepdoctype.PDFEngine, error) {
	return nil, nil
}

func cropImageChunks(_ context.Context, _ deepdoctype.PDFEngine, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	return chunks
}

// There is no preview to make, so a chunk never needs a crop and none of its
// positions counts as a crop region. That keeps the inline image fallback
// applied immediately instead of deferring it to a crop that cannot run.
func needsCrop(_ schema.ChunkDoc) bool { return false }

func hasCropRegion(_ schema.ChunkDoc) bool { return false }
