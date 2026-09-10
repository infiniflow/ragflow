//go:build cgo

package native

import (
	"runtime"
	"testing"
)

func TestPlanOCRRecBatchesKeepsNormalBatchTogether(t *testing.T) {
	imgs := make([]*Image, 16)
	for i := range imgs {
		imgs[i] = &Image{W: 320, H: 48, Pix: make([]byte, 320*48*3)}
	}

	batches, err := planOCRRecBatches(imgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0]) != 16 {
		t.Fatalf("got batch sizes %v, want [16]", batchSizes(batches))
	}
}

func TestPlanOCRRecBatchesBoundsWideBatchMemory(t *testing.T) {
	imgs := make([]*Image, 16)
	for i := range imgs {
		imgs[i] = &Image{W: 4096, H: 48, Pix: make([]byte, 4096*48*3)}
	}

	batches, err := planOCRRecBatches(imgs)
	if err != nil {
		t.Fatal(err)
	}
	for _, batch := range batches {
		if got := estimateOCRRecBatchBytes(len(batch), normalizedOCRRecWidth(batch[len(batch)-1].img)); got > recBatchMemoryLimit {
			t.Fatalf("estimated batch memory %d exceeds limit %d", got, recBatchMemoryLimit)
		}
	}
}

func TestPlanOCRRecBatchesSplitsUnsafeWidth(t *testing.T) {
	img := &Image{W: maxOCRRecWidth + 1, H: recH, Pix: make([]byte, (maxOCRRecWidth+1)*recH*3)}
	batches, err := planOCRRecBatches([]*Image{img})
	if err != nil {
		t.Fatal(err)
	}
	var parts int
	for _, batch := range batches {
		for _, item := range batch {
			parts++
			if width := normalizedOCRRecWidth(item.img); width > maxOCRRecWidth {
				t.Fatalf("split width %d exceeds limit %d", width, maxOCRRecWidth)
			}
		}
	}
	if parts != 2 {
		t.Fatalf("got %d parts, want 2", parts)
	}
}

func TestDefaultIntraOpThreads(t *testing.T) {
	t.Setenv("DEEPDOC_ORT_NUM_THREADS", "3")
	if got := defaultIntraOpThreads(); got != 3 {
		t.Fatalf("got %d threads, want 3", got)
	}
}

func TestDefaultIntraOpThreadsDoesNotUseEveryCorePerInference(t *testing.T) {
	t.Setenv("DEEPDOC_ORT_NUM_THREADS", "")
	got := defaultIntraOpThreads()
	if got < 1 || got > runtime.GOMAXPROCS(0) {
		t.Fatalf("got %d threads for GOMAXPROCS=%d", got, runtime.GOMAXPROCS(0))
	}
}

func batchSizes(batches []ocrRecBatch) []int {
	sizes := make([]int, len(batches))
	for i := range batches {
		sizes[i] = len(batches[i])
	}
	return sizes
}
