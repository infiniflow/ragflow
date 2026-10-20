//go:build cgo

package chunker

import (
	"context"
	"fmt"
	"image"
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
		chunks[i] = schema.ChunkDoc{CKType: "image", Text: "cap", PDFPositions: pos}
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
		chunks[i] = schema.ChunkDoc{CKType: "table", Text: "cap", Positions: pos}
	}
	out := cropImageChunks(ctx, eng, chunks)
	for i, ck := range out {
		if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
			t.Errorf("chunk %d: image not cropped: %q", i, ck.Image)
		}
	}
}

// panicPageEngine renders page index 0 normally and panics on every other
// page, standing in for a native render that blows up on one bad page.
type panicPageEngine struct{ mockCropEngine }

func (panicPageEngine) RenderPageImage(pageNum int, _ float64) (image.Image, error) {
	if pageNum != 0 {
		panic(fmt.Sprintf("render of page %d blew up", pageNum))
	}
	return image.NewRGBA(image.Rect(0, 0, 1000, 1000)), nil
}

// TestCropImageChunks_PanicFailsOnlyThatChunk guards the production crash
// where a render panic inside a crop goroutine killed the ingestor process:
// the panic must cost only the affected chunk its preview.
func TestCropImageChunks_PanicFailsOnlyThatChunk(t *testing.T) {
	t.Setenv("RAGFLOW_CROP_CONCURRENCY", "4")
	good := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	bad := jsonPositions(t, []float64{2, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{
		{CKType: "image", Text: "ok", PDFPositions: good},
		{CKType: "image", Text: "boom", PDFPositions: bad},
		{CKType: "table", Text: "ok too", PDFPositions: good},
	}

	out := cropImageChunks(context.Background(), panicPageEngine{}, chunks)

	for _, i := range []int{0, 2} {
		if !strings.HasPrefix(out[i].Image, "data:image/png;base64,") {
			t.Errorf("chunk %d: image = %q, want a cropped preview", i, out[i].Image)
		}
	}
	if out[1].Image != "" {
		t.Errorf("chunk 1: image = %q, want none after the render panic", out[1].Image)
	}
}
