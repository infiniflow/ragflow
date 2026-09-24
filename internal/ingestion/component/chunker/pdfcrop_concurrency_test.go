//go:build cgo

package chunker

import (
	"context"
	"strings"
	"testing"

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
		chunks[i] = schema.ChunkDoc{CKType: "image", PDFPositions: pos}
	}
	out := cropImageChunks(ctx, eng, chunks)
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
		chunks[i] = schema.ChunkDoc{CKType: "table", Positions: pos}
	}
	out := cropImageChunks(ctx, eng, chunks)
	for i, ck := range out {
		if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
			t.Errorf("chunk %d: image not cropped: %q", i, ck.Image)
		}
	}
}
