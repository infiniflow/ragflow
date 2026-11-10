//go:build cgo

package chunker

import (
	"context"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
)

// TestCropConcurrency_UsesPageBudget pins the sizing policy: the crop pool
// reuses the process-wide PDF page budget instead of deriving its own core
// fraction or reading a second override.
func TestCropConcurrency_UsesPageBudget(t *testing.T) {
	orig := deepdocpdf.PageConcurrency()
	t.Cleanup(func() { deepdocpdf.SetPageConcurrency(orig) })

	deepdocpdf.SetPageConcurrency(7)
	if got := cropConcurrency(); got != 7 {
		t.Fatalf("cropConcurrency() = %d, want page concurrency 7", got)
	}
}

// resetCropWorkerPoolForTest replaces the process-wide crop pool with a fresh
// one sized from the PDF page budget. Resizing the existing pool is not enough
// for a concurrency test: shrinking only retires workers after they finish a
// job, so idle workers from an earlier test can still pick up new jobs. Tests
// in this package run without t.Parallel, so replacing the global pool is safe.
func resetCropWorkerPoolForTest(t *testing.T, n int) {
	t.Helper()
	orig := deepdocpdf.PageConcurrency()
	if cropPool != nil {
		cropPool.StopWait()
	}
	cropPoolOnce = sync.Once{}
	cropPool = nil
	deepdocpdf.SetPageConcurrency(n)
	cropWorkerPool()
	t.Cleanup(func() {
		if cropPool != nil {
			cropPool.StopWait()
		}
		cropPoolOnce = sync.Once{}
		cropPool = nil
		deepdocpdf.SetPageConcurrency(orig)
	})
}

// TestCropImageChunks_ConcurrentCropsAll verifies the parallel fan-out still
// crops every eligible chunk — no lost work, no missing output. Run with
// `go test -race` to catch data races in the shared pageCache / out slice.
func TestCropImageChunks_ConcurrentCropsAll(t *testing.T) {
	resetCropWorkerPoolForTest(t, 16)
	ctx := context.Background()
	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	const n = 64
	chunks := make([]schema.ChunkDoc, n)
	for i := 0; i < n; i++ {
		chunks[i] = schema.ChunkDoc{DocType: "image", Text: "cap", PDFPositions: pos}
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
	resetCropWorkerPoolForTest(t, 16)
	ctx := context.Background()
	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	const n = 200
	chunks := make([]schema.ChunkDoc, n)
	for i := 0; i < n; i++ {
		chunks[i] = schema.ChunkDoc{DocType: "table", Text: "cap", Positions: pos}
	}
	out := cropImageChunks(ctx, eng, chunks)
	for i, ck := range out {
		if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
			t.Errorf("chunk %d: image not cropped: %q", i, ck.Image)
		}
	}
}

// blockingCropEngine holds every render until gate is closed so a test can
// observe how many crop jobs enter the shared pool at once.
type blockingCropEngine struct {
	mockCropEngine
	started chan struct{}
	gate    chan struct{}

	mu      sync.Mutex
	active  int
	maxSeen int
}

func (e *blockingCropEngine) RenderPageImage(_ int, _ float64) (image.Image, error) {
	e.mu.Lock()
	e.active++
	if e.active > e.maxSeen {
		e.maxSeen = e.active
	}
	e.mu.Unlock()

	select {
	case e.started <- struct{}{}:
	default:
	}
	<-e.gate

	e.mu.Lock()
	e.active--
	e.mu.Unlock()
	return image.NewRGBA(image.Rect(0, 0, 64, 64)), nil
}

func (e *blockingCropEngine) maxConcurrent() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.maxSeen
}

// TestCropImageChunks_ProcessWidePoolLimitsConcurrency proves two independent
// cropImageChunks calls share the process-wide pool: with one worker, the
// second call cannot render while the first is blocked.
func TestCropImageChunks_ProcessWidePoolLimitsConcurrency(t *testing.T) {
	resetCropWorkerPoolForTest(t, 1)
	eng := &blockingCropEngine{
		started: make(chan struct{}, 2),
		gate:    make(chan struct{}),
	}
	var gateOnce sync.Once
	release := func() { gateOnce.Do(func() { close(eng.gate) }) }
	t.Cleanup(release)

	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{{DocType: "image", Text: "cap", PDFPositions: pos}}

	var wg sync.WaitGroup
	run := func() {
		defer wg.Done()
		_ = cropImageChunks(context.Background(), eng, chunks)
	}
	wg.Add(2)
	go run()

	select {
	case <-eng.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first crop render")
	}

	go run()

	// Wait until both calls have submitted their job. The first is blocked in
	// RenderPageImage; the second must remain queued behind the single worker.
	deadline := time.Now().Add(5 * time.Second)
	for cropWorkerPool().Stats().PendingTotal < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for both crop jobs to be submitted: %+v", cropWorkerPool().Stats())
		}
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-eng.started:
		t.Fatal("second crop render started while the first was blocked; the crop pool is not process-wide")
	case <-time.After(100 * time.Millisecond):
	}
	if got := eng.maxConcurrent(); got != 1 {
		t.Fatalf("max concurrent crop renders = %d, want 1 (pool size 1)", got)
	}

	release()
	wg.Wait()
}

// unreadableImage is a page bitmap whose pixels cannot be read, so cropping
// from it panics inside the crop task.
type unreadableImage struct{ *image.RGBA }

func (unreadableImage) At(int, int) color.Color { panic("unreadable page bitmap") }

// TestCropChunkPreview_PanicReleasesPages runs a crop that panics through the
// shared crop pool and checks that the pool reports the panic and that every
// page the crop acquired is released, so the shared cache can still evict it.
func TestCropChunkPreview_PanicReleasesPages(t *testing.T) {
	resetCropWorkerPoolForTest(t, 1)
	cache := newLRUPageCache(64, math.MaxInt64)
	render := func(int) (image.Image, bool) {
		return unreadableImage{image.NewRGBA(image.Rect(0, 0, 64, 64))}, true
	}
	positions := []deepdoctype.Position{{PageNumbers: []int{0, 1}, Left: 1, Right: 10, Top: 1, Bottom: 10}}

	ctx := context.Background()
	fut, err := cropWorkerPool().Submit(ctx, func(context.Context) {
		cropChunkPreview(cache, render, "image", positions)
	})
	if err != nil {
		t.Fatalf("submit crop job: %v", err)
	}
	res, err := fut.Wait(ctx)
	if err != nil {
		t.Fatalf("wait crop job: %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "panic") {
		t.Fatalf("crop job error = %v, want the recovered panic", res.Err)
	}
	for _, pn := range []int{0, 1} {
		el, ok := cache.items[pn]
		if !ok {
			t.Fatalf("page %d was never acquired", pn)
		}
		if ref := el.Value.(pageCacheEntry).ref; ref != 0 {
			t.Errorf("page %d ref = %d after the crop panicked, want 0", pn, ref)
		}
	}
}
