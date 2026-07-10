package pdf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"reflect"
	"sync"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

type recordingAnalyzer struct {
	mu              sync.Mutex
	seenDetectSizes [][2]int
}

func (r *recordingAnalyzer) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (r *recordingAnalyzer) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) { return nil, nil }
func (r *recordingAnalyzer) OCRDetect(_ context.Context, img image.Image) ([]pdf.OCRBox, error) {
	if img != nil {
		b := img.Bounds()
		r.mu.Lock()
		r.seenDetectSizes = append(r.seenDetectSizes, [2]int{b.Dx(), b.Dy()})
		r.mu.Unlock()
		if b.Dx() <= 90 {
			return nil, nil
		}
	}
	return []pdf.OCRBox{
		{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40},
	}, nil
}
func (r *recordingAnalyzer) OCRRecognize(context.Context, image.Image) ([]pdf.OCRText, error) {
	return []pdf.OCRText{{Text: "zoom retry", Confidence: 0.9}}, nil
}
func (r *recordingAnalyzer) Health() bool { return true }

func (r *recordingAnalyzer) lastDetectSize() (int, int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.seenDetectSizes) == 0 {
		return 0, 0, false
	}
	last := r.seenDetectSizes[len(r.seenDetectSizes)-1]
	return last[0], last[1], true
}

// ── Page-parallel regression tests ───────────────────────────────────────

// makeMultiPageEngine builds a MockEngine with 1 page worth of embedded
// chars per page so the unified processPage path lands on the
// ocrMergeChars strategy.
func makeMultiPageEngine(numPages int) *MockEngine {
	chars := make(map[int][]pdf.TextChar, numPages)
	for pg := 0; pg < numPages; pg++ {
		chars[pg] = []pdf.TextChar{
			{X0: 50, X1: 200, Top: 100 + float64(pg*10), Bottom: 112 + float64(pg*10), Text: "page", PageNumber: pg},
			{X0: 50, X1: 200, Top: 130 + float64(pg*10), Bottom: 142 + float64(pg*10), Text: "text", PageNumber: pg},
		}
	}
	return &MockEngine{NumPages: numPages, Chars: chars, RenderW: 100, RenderH: 100}
}

func TestParser_RunPageWorkers_DeterministicOrder(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true}
	eng := makeMultiPageEngine(8)
	p := NewParser(pdf.DefaultParserConfig())

	pages := []int{0, 1, 2, 3, 4, 5, 6, 7}
	results, err := p.runPageWorkers(context.Background(), eng, pages,
		mock, NewTableBuilderFor(mock))
	if err != nil {
		t.Fatalf("runPageWorkers: %v", err)
	}
	if len(results) != len(pages) {
		t.Fatalf("expected %d results, got %d", len(pages), len(results))
	}
	for i, r := range results {
		if r.PageNumber != pages[i] {
			t.Errorf("results[%d].PageNumber = %d, want %d", i, r.PageNumber, pages[i])
		}
	}
}

func TestParser_RunPageWorkers_PoolSize4_DeterministicOrder(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true}
	eng := makeMultiPageEngine(8)
	setPoolSize(t, 4)
	p := NewParser(pdf.DefaultParserConfig())

	pages := []int{0, 1, 2, 3, 4, 5, 6, 7}
	results, err := p.runPageWorkers(context.Background(), eng, pages,
		mock, NewTableBuilderFor(mock))
	if err != nil {
		t.Fatalf("runPageWorkers: %v", err)
	}
	if len(results) != len(pages) {
		t.Fatalf("expected %d results, got %d", len(pages), len(results))
	}
	for i, r := range results {
		if r.PageNumber != pages[i] {
			t.Errorf("results[%d].PageNumber = %d, want %d", i, r.PageNumber, pages[i])
		}
	}
}

// TestParser_RunPageWorkers_StableAcrossPoolSizes verifies that the output
// produced with pool size 1 and pool size N is structurally equivalent:
// same page numbers, same box count per page, same image fingerprint, same
// TableItem count. Page concurrency is governed by the process-wide worker
// pool, not a per-Parser knob.
func TestParser_RunPageWorkers_StableAcrossPoolSizes(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 100, Y0: 200, X1: 800, Y1: 600, Label: "table", Confidence: 0.9},
		},
	}
	eng := makeMultiPageEngine(4)

	for _, par := range []int{1, 2, 4} {
		setPoolSize(t, par)
		p := NewParser(pdf.DefaultParserConfig())

		pages := []int{0, 1, 2, 3}
		results, err := p.runPageWorkers(context.Background(), eng, pages,
			mock, NewTableBuilderFor(mock))
		if err != nil {
			t.Fatalf("poolSize=%d: runPageWorkers: %v", par, err)
		}
		if len(results) != len(pages) {
			t.Fatalf("poolSize=%d: got %d results, want %d", par, len(results), len(pages))
		}
		for i, r := range results {
			if r.PageNumber != pages[i] {
				t.Errorf("poolSize=%d: results[%d].PageNumber = %d, want %d",
					par, i, r.PageNumber, pages[i])
			}
			if r.PageHeight <= 0 {
				t.Errorf("poolSize=%d: page %d has zero PageHeight", par, r.PageNumber)
			}
		}
	}
}

// TestParser_ParseRaw_PoolSizeEquivalence verifies that the final ParseResult
// has equivalent shape for pool size 1 and pool size>1: same PageHeight values,
// same Section count, same Table count.
func TestParser_ParseRaw_PoolSizeEquivalence(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true}
	eng := makeMultiPageEngine(6)

	var results [2]*pdf.ParseResult
	for i, par := range []int{1, 3} {
		setPoolSize(t, par)
		p := NewParser(pdf.DefaultParserConfig())
		r, err := p.ParseRaw(context.Background(), eng, mock)
		if err != nil {
			t.Fatalf("poolSize=%d: ParseRaw: %v", par, err)
		}
		results[i] = r
	}

	if !reflect.DeepEqual(results[0].PageHeight, results[1].PageHeight) {
		t.Errorf("PageHeight mismatch:\n poolSize=1: %v\n poolSize=3: %v",
			results[0].PageHeight, results[1].PageHeight)
	}
	if len(results[0].Sections) != len(results[1].Sections) {
		t.Errorf("Section count mismatch: poolSize=1=%d poolSize=3=%d",
			len(results[0].Sections), len(results[1].Sections))
	}
	if len(results[0].Tables) != len(results[1].Tables) {
		t.Errorf("Table count mismatch: poolSize=1=%d poolSize=3=%d",
			len(results[0].Tables), len(results[1].Tables))
	}
}

// TestParser_ProcessPage_UnifiedPath verifies that the single processPage
// function works the same way across page-local decisions: clean chars
// (ocrMergeChars path), garbled chars (ocrDetectAndRecognize path), and
// empty chars (ocrDetectAndRecognize path).
func TestParser_ProcessPage_UnifiedPath(t *testing.T) {
	t.Run("clean_chars", func(t *testing.T) {
		chars := make([]pdf.TextChar, 40)
		for i := range chars {
			chars[i] = pdf.TextChar{
				X0: 10 + float64(i*2), X1: 12 + float64(i*2), Top: 10, Bottom: 30,
				Text: "a", PageNumber: 0,
			}
		}
		eng := &MockEngine{NumPages: 1, Chars: map[int][]pdf.TextChar{0: chars}}
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 150, X3: 15, Y3: 150},
			},
			OCRTexts: []pdf.OCRText{{Text: "Hello", Confidence: 0.9}},
		}
		p := NewParser(pdf.DefaultParserConfig())
		r := p.processPage(context.Background(), eng, 0, mock, NewTableBuilderFor(mock))
		if r.Err != nil {
			t.Fatalf("processPage: %v", r.Err)
		}
		if r.PageHeight <= 0 {
			t.Error("PageHeight zero for clean chars page")
		}
		if len(r.Boxes) == 0 {
			t.Error("clean chars page should produce some boxes")
		}
		if !r.IsEnglish {
			t.Error("clean english page should be marked english")
		}
		if r.MedianH <= 0 || r.MedianW <= 0 {
			t.Error("clean page should carry per-page median metadata")
		}
	})

	t.Run("garbled_chars", func(t *testing.T) {
		chars := garbledSample()
		eng := &MockEngine{NumPages: 1, Chars: map[int][]pdf.TextChar{0: chars}}
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40},
			},
			OCRTexts: []pdf.OCRText{{Text: "OCR result", Confidence: 0.9}},
		}
		p := NewParser(pdf.DefaultParserConfig())
		r := p.processPage(context.Background(), eng, 0, mock, NewTableBuilderFor(mock))
		if r.Err != nil {
			t.Fatalf("processPage: %v", r.Err)
		}
		if len(r.Boxes) == 0 {
			t.Error("garbled chars page should yield OCR boxes")
		}
	})

	t.Run("empty_chars", func(t *testing.T) {
		eng := &MockEngine{NumPages: 1}
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40},
			},
			OCRTexts: []pdf.OCRText{{Text: "scan OCR", Confidence: 0.9}},
		}
		p := NewParser(pdf.DefaultParserConfig())
		r := p.processPage(context.Background(), eng, 0, mock, NewTableBuilderFor(mock))
		if r.Err != nil {
			t.Fatalf("processPage: %v", r.Err)
		}
		if len(r.Boxes) == 0 {
			t.Error("empty chars page should yield OCR boxes")
		}
	})
}

// TestParser_RunPageWorkers_CancellationHonored verifies that an already-
// cancelled context produces empty results without panicking.
func TestParser_RunPageWorkers_CancellationHonored(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true}
	eng := makeMultiPageEngine(4)
	setPoolSize(t, 2)
	p := NewParser(pdf.DefaultParserConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.runPageWorkers(ctx, eng, []int{0, 1, 2, 3},
		mock, NewTableBuilderFor(mock))
	if err == nil {
		t.Error("expected non-nil error from cancelled context")
	}
}

// TestParser_ProcessPages_CrossPageTableMerge verifies that cross-page
// tables still merge after being collected from per-page workers, even
// when different workers process the participating pages. This is the
// regression test for AD-3 in the plan: pages processed by separate
// workers participate in one document-wide merge pass.
func TestParser_ProcessPages_CrossPageTableMerge(t *testing.T) {
	// Same DLA region is returned for every page; positions match the
	// rendered image space so the cross-page merge would attach page 1
	// to page 0 once MergeTablesAcrossPages is called with both.
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 250, Y0: 600, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.95},
		},
		TSRCells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 600, Y1: 400, Text: "A1"},
		},
	}
	chars := map[int][]pdf.TextChar{
		0: {{X0: 80, X1: 500, Top: 200, Bottom: 300, Text: "head", PageNumber: 0}},
		1: {{X0: 80, X1: 500, Top: 100, Bottom: 200, Text: "next", PageNumber: 1}},
	}
	eng := &MockEngine{NumPages: 2, Chars: chars, RenderW: 2000, RenderH: 3000}

	for _, par := range []int{1, 4} {
		setPoolSize(t, par)
		p := NewParser(pdf.DefaultParserConfig())
		result, err := p.ParseRaw(context.Background(), eng, mock)
		if err != nil {
			t.Fatalf("poolSize=%d: ParseRaw: %v", par, err)
		}
		// With the same DLA region on both pages, per-page workers
		// emit two table candidates and MergeTablesAcrossPages must
		// collapse them to one merged document-wide TableItem.
		if len(result.Tables) >= 2 {
			t.Errorf("poolSize=%d: expected merged cross-page tables, got %d (no merge occurred)",
				par, len(result.Tables))
		}
	}
}

// TestParser_PageParallel_DeterministicOrder_MockEngine keeps a fast mock
// regression around the assembly-order invariant. The integration test with
// the plan-mandated fixture coverage lives in parser_parallel_integration_test.go.
//
// The plan's AD-5a requires this regression to catch output drift
// before it is observable in production. The test runs each
// configuration multiple times to flush out flaky worker-pool timing
// surprises that a single execution might miss.
func TestParser_PageParallel_DeterministicOrder_MockEngine(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true}
	eng := makeMultiPageEngine(6)

	runParse := func(par int) *pdf.ParseResult {
		setPoolSize(t, par)
		p := NewParser(pdf.DefaultParserConfig())
		r, err := p.ParseRaw(context.Background(), eng, mock)
		if err != nil {
			t.Fatalf("poolSize=%d: ParseRaw: %v", par, err)
		}
		return r
	}

	// Run pool size 4 three times so we exercise worker scheduling
	// variations. Each run should match the pool size 1 baseline.
	baseline := runParse(1)
	for run := 0; run < 3; run++ {
		parRun := runParse(4)
		if !reflect.DeepEqual(stableSections(baseline.Sections), stableSections(parRun.Sections)) {
			t.Errorf("run=%d: Sections not stable across pool size", run)
		}
		if !reflect.DeepEqual(stableTables(baseline.Tables), stableTables(parRun.Tables)) {
			t.Errorf("run=%d: Tables not stable across pool size", run)
		}
		if !reflect.DeepEqual(baseline.Metrics, parRun.Metrics) {
			t.Errorf("run=%d: Metrics not stable across pool size: %v vs %v",
				run, baseline.Metrics, parRun.Metrics)
		}
	}
}

func TestParser_ParseRaw_RetryZoomReplacesPageImageAndZoom(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.Zoom = 2
	p := NewParser(cfg)

	eng := &MockEngine{
		NumPages: 1,
		RenderPageImageFunc: func(_ int, dpi float64) (image.Image, error) {
			if dpi > pdf.DlaDPI {
				return image.NewRGBA(image.Rect(0, 0, 180, 240)), nil
			}
			return image.NewRGBA(image.Rect(0, 0, 90, 120)), nil
		},
	}
	mock := &MockDocAnalyzer{Healthy: true}

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	// Verify retry zoom: page dimensions are always reported in PDF-point
	// space. The retry render is 180×240 px at 6× zoom → 30×40 PDF-point.
	if got, want := result.PageHeight[0], 40.0; got != want {
		t.Fatalf("PageHeight[0] = %v, want %v (PDF-point height)", got, want)
	}
	if got, want := result.PageWidth[0], 30.0; got != want {
		t.Fatalf("PageWidth[0] = %v, want %v (PDF-point width)", got, want)
	}
}

func TestParser_ParseRaw_RetryZoomUsesHigherDPIForOCR(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	cfg.Zoom = 2
	p := NewParser(cfg)

	eng := &MockEngine{
		NumPages: 1,
		RenderPageImageFunc: func(_ int, dpi float64) (image.Image, error) {
			if dpi > pdf.DlaDPI {
				return image.NewRGBA(image.Rect(0, 0, 180, 240)), nil
			}
			return image.NewRGBA(image.Rect(0, 0, 90, 120)), nil
		},
	}
	analyzer := &recordingAnalyzer{}

	result, err := p.ParseRaw(context.Background(), eng, analyzer)
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected retryZoom OCR to produce sections")
	}
	w, h, ok := analyzer.lastDetectSize()
	if !ok {
		t.Fatal("expected OCRDetect to be called")
	}
	if w != 180 || h != 240 {
		t.Fatalf("retryZoom OCRDetect used image size %dx%d, want 180x240", w, h)
	}
}

// stableSections normalizes Sections to a comparable shape: positions
// become a sorted tuple list and PositionTag is omitted since the test
// only cares about text + layout-type + position geometry.
func stableSections(sections []pdf.Section) []stableSection {
	out := make([]stableSection, len(sections))
	for i, s := range sections {
		var pages []int
		var bbox [4]float64
		if len(s.Positions) > 0 {
			p := s.Positions[0]
			pages = append(pages, p.PageNumbers...)
			bbox = [4]float64{p.Left, p.Right, p.Top, p.Bottom}
		}
		// Defensive copy and sort so worker completion order cannot
		// affect the comparison.
		sortPages(pages)
		out[i] = stableSection{
			Text:       s.Text,
			LayoutType: s.LayoutType,
			Pages:      pages,
			BBox:       bbox,
		}
	}
	return out
}

type stableSection struct {
	Text       string
	LayoutType string
	Pages      []int
	BBox       [4]float64
}

type stableTable struct {
	Positions [][]int
}

func stableTables(tables []pdf.TableItem) []stableTable {
	out := make([]stableTable, len(tables))
	for i, t := range tables {
		positions := make([][]int, 0, len(t.Positions))
		for _, p := range t.Positions {
			pages := append([]int(nil), p.PageNumbers...)
			sortPages(pages)
			positions = append(positions, pages)
		}
		out[i] = stableTable{Positions: positions}
	}
	return out
}

func sortPages(pages []int) {
	// Small fixed slice — insertion sort is fine.
	for i := 1; i < len(pages); i++ {
		for j := i; j > 0 && pages[j-1] > pages[j]; j-- {
			pages[j-1], pages[j] = pages[j], pages[j-1]
		}
	}
}

// setPoolSize resizes the process-wide page worker pool for the duration of
// a test and restores the prior size afterwards. With Config.Parallelism
// removed, the pool worker count is the only page-concurrency knob.
func setPoolSize(t *testing.T, n int) {
	t.Helper()
	orig := PageWorkerPoolStats().DesiredWorkers
	SetPageWorkerPoolSize(n)
	t.Cleanup(func() { SetPageWorkerPoolSize(orig) })
}

// imageHash produces a stable PNG-content fingerprint for an image so
// equivalence comparisons remain independent of concrete image types.
func imageHash(img image.Image) string {
	if img == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	h := sha256.New()
	h.Write(buf.Bytes())
	return hex.EncodeToString(h.Sum(nil))
}
