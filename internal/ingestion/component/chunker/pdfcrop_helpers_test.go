//go:build cgo

package chunker

import (
	"context"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
)

func mustCropImageChunks(t *testing.T, ctx context.Context, engine deepdoctype.PDFEngine, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	t.Helper()
	out, err := cropImageChunks(ctx, engine, chunks)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustCropTitleChunks(t *testing.T, ctx context.Context, engine deepdoctype.PDFEngine, chunks []map[string]any) []map[string]any {
	t.Helper()
	out, err := cropTitleChunks(ctx, engine, chunks)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
