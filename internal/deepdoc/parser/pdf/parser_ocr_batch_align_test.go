package pdf

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sort"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// ── ocrRecognizeBatchAligned: Python parity (sort by W/H, ≤recBatchNum) ──
//
// Python's TextRecognizer.__call__ (deepdoc/vision/ocr.py) argsorts crops by
// aspect ratio (W/H) ascending and recognizes sub-batches of at most
// rec_batch_num=16, padding each sub-batch only to its own local max width.
// The Go OCR-rec fast path must do the same so CTC confidence is not diluted
// by over-padding narrow lines to the whole page's widest line. These tests
// pin that behavior.

// recordingBatchAnalyzer implements batchRecognizer and records how crops were
// grouped into batches so the test can assert sorting + chunking.
type recordingBatchAnalyzer struct {
	healthy    bool
	failOnCall int // 1-based call index that returns an error; <1 = never
	callCount  int

	batches    [][]float64 // per call: aspect ratios (W/H) of imgs in call order
	batchSizes []int
}

func (a *recordingBatchAnalyzer) Health() bool { return a.healthy }
func (a *recordingBatchAnalyzer) OCRDetect(context.Context, image.Image) ([]pdf.OCRBox, error) {
	return nil, nil
}
func (a *recordingBatchAnalyzer) OCRRecognize(context.Context, image.Image) ([]pdf.OCRText, error) {
	return nil, nil
}
func (a *recordingBatchAnalyzer) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (a *recordingBatchAnalyzer) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (a *recordingBatchAnalyzer) OCRRecognizeBatch(_ context.Context, imgs []image.Image) ([][]pdf.OCRText, error) {
	a.callCount++
	if a.failOnCall > 0 && a.callCount == a.failOnCall {
		return nil, errors.New("injected batch failure")
	}
	ratios := make([]float64, len(imgs))
	out := make([][]pdf.OCRText, len(imgs))
	for i, im := range imgs {
		b := im.Bounds()
		ratios[i] = float64(b.Dx()) / float64(b.Dy())
		// Echo the crop's own ratio so the caller can verify round-trip mapping.
		out[i] = []pdf.OCRText{{Text: fmt.Sprintf("%.4f", ratios[i])}}
	}
	a.batches = append(a.batches, ratios)
	a.batchSizes = append(a.batchSizes, len(imgs))
	return out, nil
}

// nonBatchAnalyzer implements only the base DocAnalyzer interface (no batch).
type nonBatchAnalyzer struct{ healthy bool }

func (a *nonBatchAnalyzer) Health() bool { return a.healthy }
func (a *nonBatchAnalyzer) OCRDetect(context.Context, image.Image) ([]pdf.OCRBox, error) {
	return nil, nil
}
func (a *nonBatchAnalyzer) OCRRecognize(context.Context, image.Image) ([]pdf.OCRText, error) {
	return nil, nil
}
func (a *nonBatchAnalyzer) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (a *nonBatchAnalyzer) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) { return nil, nil }

func makeCrops(ratios []float64) []image.Image {
	// Fixed height 48 (recH); width = round(48*ratio) so W/H == ratio.
	const h = 48
	crops := make([]image.Image, len(ratios))
	for i, r := range ratios {
		w := int(r * h)
		if w < 1 {
			w = 1
		}
		crops[i] = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	return crops
}

func parseRatio(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func TestOCRRecognizeBatchAligned_SortsAndChunks(t *testing.T) {
	// 40 crops with shuffled aspect ratios spanning 0.5..8.0.
	rnd := []float64{3.2, 0.8, 5.1, 1.0, 7.3, 2.0, 0.5, 8.0, 4.4, 1.5,
		6.2, 0.9, 3.7, 2.9, 1.2, 5.8, 0.6, 4.0, 7.0, 1.8,
		2.4, 6.6, 0.7, 3.0, 5.5, 1.1, 8.0, 4.7, 2.2, 0.55,
		6.0, 1.6, 3.9, 9.0, 0.75, 5.0, 2.7, 1.3, 7.6, 4.2}
	if len(rnd) != 40 {
		t.Fatalf("setup: expected 40 crops, got %d", len(rnd))
	}
	crops := makeCrops(rnd)

	analyzer := &recordingBatchAnalyzer{healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	// Fallback must never be used when the batch path succeeds.
	fallbackUsed := false
	results := p.ocrRecognizeBatchAligned(t.Context(), analyzer, 0, crops, func(ci int, _ image.Image) ([]pdf.OCRText, error) {
		fallbackUsed = true
		return []pdf.OCRText{{Text: fmt.Sprintf("fallback:%d", ci)}}, nil
	})

	if fallbackUsed {
		t.Fatal("fallback should not be used when batching succeeds")
	}
	if len(results) != len(crops) {
		t.Fatalf("results len %d, want %d", len(results), len(crops))
	}

	// Chunking: 40 crops / 16 == 3 batches (16,16,8).
	wantBatches := (len(crops) + recBatchNum - 1) / recBatchNum
	if len(analyzer.batches) != wantBatches {
		t.Fatalf("batch calls = %d, want %d", len(analyzer.batches), wantBatches)
	}
	for _, sz := range analyzer.batchSizes {
		if sz > recBatchNum {
			t.Fatalf("sub-batch size %d exceeds recBatchNum=%d", sz, recBatchNum)
		}
	}
	if analyzer.batchSizes[0] != recBatchNum || analyzer.batchSizes[2] != 8 {
		t.Fatalf("unexpected chunk sizes %v", analyzer.batchSizes)
	}

	// Within every batch the ratios must be non-decreasing (ascending sort).
	for bi, b := range analyzer.batches {
		if !sort.Float64sAreSorted(b) {
			t.Fatalf("batch %d not sorted ascending: %v", bi, b)
		}
	}

	// Round-trip mapping: results[i] must correspond to the original crop i.
	for i, c := range crops {
		cb := c.Bounds()
		want := float64(cb.Dx()) / float64(cb.Dy())
		got := parseRatio(results[i][0].Text)
		if absf(got-want) > 1e-3 {
			t.Fatalf("crop %d mapping wrong: got %.4f want %.4f (ratio drift)", i, got, want)
		}
	}
}

func TestOCRRecognizeBatchAligned_FallbackOnSubBatchError(t *testing.T) {
	// 20 crops; force the batch path to fail on its 2nd call. The failed
	// sub-batch is the LAST recBatchNum-window of crops in aspect-ratio-sorted
	// order, not the last original indices — so compute the expected set from
	// the same sort the implementation uses.
	ratios := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 0.5, 9.0,
		1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 0.6, 7.5, 8.5, 2.2}
	crops := makeCrops(ratios)

	order := make([]int, len(ratios))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return ratios[order[a]] < ratios[order[b]] })
	wantFallback := map[int]bool{}
	for _, ci := range order[recBatchNum:] { // 2nd sub-batch (crops 16..19 in order)
		wantFallback[ci] = true
	}

	analyzer := &recordingBatchAnalyzer{healthy: true, failOnCall: 2}
	p := NewParser(pdf.DefaultParserConfig())

	var fallbackIdx []int
	results := p.ocrRecognizeBatchAligned(t.Context(), analyzer, 0, crops, func(ci int, _ image.Image) ([]pdf.OCRText, error) {
		fallbackIdx = append(fallbackIdx, ci)
		return []pdf.OCRText{{Text: fmt.Sprintf("fallback:%d", ci)}}, nil
	})

	if len(results) != len(crops) {
		t.Fatalf("results len %d, want %d", len(results), len(crops))
	}
	if len(fallbackIdx) != len(wantFallback) {
		t.Fatalf("fallback count = %d, want %d", len(fallbackIdx), len(wantFallback))
	}
	for _, ci := range fallbackIdx {
		if !wantFallback[ci] {
			t.Fatalf("crop %d fell back but is not in the failed sub-batch %v", ci, wantFallback)
		}
		if results[ci][0].Text != fmt.Sprintf("fallback:%d", ci) {
			t.Fatalf("crop %d not filled by fallback: %q", ci, results[ci][0].Text)
		}
	}
	// Crops outside the failed sub-batch keep their batch result.
	for i := range crops {
		if !wantFallback[i] && results[i][0].Text == fmt.Sprintf("fallback:%d", i) {
			t.Fatalf("crop %d wrongly fell back", i)
		}
	}
}

func TestOCRRecognizeBatchAligned_NonBatchAnalyzerFallsBackAll(t *testing.T) {
	crops := makeCrops([]float64{1.0, 2.0, 3.0, 4.0, 5.0})
	analyzer := &nonBatchAnalyzer{healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	var fallbackIdx []int
	results := p.ocrRecognizeBatchAligned(t.Context(), analyzer, 0, crops, func(ci int, _ image.Image) ([]pdf.OCRText, error) {
		fallbackIdx = append(fallbackIdx, ci)
		return []pdf.OCRText{{Text: fmt.Sprintf("fallback:%d", ci)}}, nil
	})

	if len(results) != len(crops) {
		t.Fatalf("results len %d, want %d", len(results), len(crops))
	}
	if len(fallbackIdx) != len(crops) {
		t.Fatalf("non-batch analyzer should fall back for every crop, got %d/%d", len(fallbackIdx), len(crops))
	}
}

func TestOCRRecognizeBatchAligned_UnhealthyBatchAnalyzerFallsBack(t *testing.T) {
	crops := makeCrops([]float64{1.0, 2.0, 3.0})
	analyzer := &recordingBatchAnalyzer{healthy: false}
	p := NewParser(pdf.DefaultParserConfig())

	var fallbackIdx []int
	results := p.ocrRecognizeBatchAligned(t.Context(), analyzer, 0, crops, func(ci int, _ image.Image) ([]pdf.OCRText, error) {
		fallbackIdx = append(fallbackIdx, ci)
		return []pdf.OCRText{{Text: fmt.Sprintf("fallback:%d", ci)}}, nil
	})

	if len(results) != len(crops) {
		t.Fatalf("results len %d, want %d", len(results), len(crops))
	}
	if len(fallbackIdx) != len(crops) {
		t.Fatalf("unhealthy batch analyzer should fall back for every crop, got %d/%d", len(fallbackIdx), len(crops))
	}
	// inferOCRRecognizeBatch short-circuits on !Health() before invoking the
	// analyzer (returns nil,nil), so the helper falls back via its count
	// mismatch branch without ever calling OCRRecognizeBatch.
	if analyzer.callCount != 0 {
		t.Fatalf("unhealthy batch analyzer should not invoke OCRRecognizeBatch, got %d calls", analyzer.callCount)
	}
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

var _ doctype.DocAnalyzer = (*recordingBatchAnalyzer)(nil)
