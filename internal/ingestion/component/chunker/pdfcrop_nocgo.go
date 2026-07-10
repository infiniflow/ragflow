//go:build !cgo

// Package chunker — no-CGO fallback for on-demand PDF section cropping.
//
// Without the native PDF backend there is nothing to crop, so the engine is
// always nil and chunks pass through unchanged. The signatures mirror the
// cgo file so token.go can call them unconditionally.
package chunker

import (
	"context"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
)

func newPDFEngineFromUpstream(_ context.Context, _ schema.ChunkerFromUpstream) (deepdoctype.PDFEngine, error) {
	return nil, nil
}

func cropImageChunks(_ context.Context, _ deepdoctype.PDFEngine, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	return chunks
}
