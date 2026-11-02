//go:build cgo

package chunker

import (
	"context"
	"errors"
	"image"
	"strings"
	"sync/atomic"
	"testing"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
)

// TestCropImageChunks_ConcurrentCropsAll verifies the parallel fan-out still
// crops every eligible chunk — no lost work, no missing output. Run with
// `go test -race` to catch data races in the shared pageCache / out slice.
func TestCropImageChunks_ConcurrentCropsAll(t *testing.T) {
	t.Setenv("RAGFLOW_CROP_CONCURRENCY", "32")
	ctx := context.Background()
	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	const n = 64
	chunks := make([]schema.ChunkDoc, n)
	for i := 0; i < n; i++ {
		chunks[i] = schema.ChunkDoc{DocType: "image", Text: "cap", PDFPositions: pos}
	}
	out := mustCropImageChunks(t, ctx, eng, chunks)
	if len(out) != n {
		t.Fatalf("len(out) = %d, want %d", len(out), n)
	}
	for i, ck := range out {
		if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
			t.Errorf("chunk %d: image = %q, want data:image/png;base64, prefix", i, ck.Image)
		}
	}
}

// TestCropImageChunks_ConcurrentPageCacheReuse hammers renderPage from many
// goroutines against one shared page to prove the pageCache map is race-free
// (run with -race). It also confirms every chunk on the same page still gets a
// cropped preview under heavy contention.
func TestCropImageChunks_ConcurrentPageCacheReuse(t *testing.T) {
	t.Setenv("RAGFLOW_CROP_CONCURRENCY", "16")
	ctx := context.Background()
	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	const n = 200
	chunks := make([]schema.ChunkDoc, n)
	for i := 0; i < n; i++ {
		chunks[i] = schema.ChunkDoc{DocType: "table", Text: "cap", Positions: pos}
	}
	out := mustCropImageChunks(t, ctx, eng, chunks)
	for i, ck := range out {
		if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
			t.Errorf("chunk %d: image not cropped: %q", i, ck.Image)
		}
	}
}

type cropPageCache struct {
	calls atomic.Int64
	err   error
}

func (c *cropPageCache) Put(context.Context, doctype.PageRef, image.Image) error { return nil }
func (c *cropPageCache) Get(context.Context, doctype.PageRef) (image.Image, error) {
	c.calls.Add(1)
	if c.err != nil {
		return nil, c.err
	}
	return image.NewRGBA(image.Rect(0, 0, 1000, 1000)), nil
}
func TestCropImageChunksUsesCachedPagesAndReturnsCacheError(t *testing.T) {
	cache := &cropPageCache{}
	source := strings.Repeat("a", 64)
	document := doctype.PageDocument{Source: source, Pages: map[int]doctype.PageSize{
		0: {Width: 1000, Height: 1000},
		1: {Width: 1000, Height: 1000},
	}}
	ctx := deepdocpdf.WithPageDocument(deepdocpdf.WithPageCache(t.Context(), cache), document)
	chunks := []schema.ChunkDoc{
		{Text: "first", DocType: "text", Positions: jsonPositions(t, []float64{1, 10, 100, 10, 100})},
		{Text: "cross page", DocType: "image", Positions: jsonPositions(t, []float64{1, 10, 100, 200, 300}, []float64{2, 10, 100, 10, 100})},
	}
	baseline := mustCropImageChunks(t, t.Context(), mockCropEngine{}, chunks)
	got, err := cropImageChunks(ctx, nil, chunks)
	if err != nil {
		t.Fatal(err)
	}
	for index := range got {
		if got[index].Image == "" || got[index].Image != baseline[index].Image {
			t.Fatalf("preview differs at %d", index)
		}
	}
	if cache.calls.Load() != 2 {
		t.Fatalf("page loads = %d; want 2", cache.calls.Load())
	}
	cache.err = errors.New("page expired")
	if _, err := cropImageChunks(ctx, nil, chunks); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("cache failure swallowed: %v", err)
	}
	chunker, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = chunker.Invoke(deepdocpdf.WithPageCache(t.Context(), cache), nil, map[string]any{
		"name": "cached.pdf", "file_type": "pdf", "output_format": "json", "json": chunks, "pdf_pages": document,
		"bucket": "must-not-fetch", "path": "missing.pdf",
	})
	if !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("GeneralChunker swallowed cache failure: %v", err)
	}
}
