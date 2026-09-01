//go:build cgo && integration

package infnative

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/deepdoc/native"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// TestNativeAnalyzerInProcess proves the in-process (Go) DeepDoc backend
// actually runs end-to-end through the doctype.DocAnalyzer interface: ONNX
// Runtime init + model load + DLA/TSR/OCR inference on a real page fixture,
// producing non-empty, in-bounds results. It is the caller-side analogue of the
// native equivalence suite, but exercised through the DocAnalyzer seam the
// PDF parser consumes. Requires the statically-linked ONNX Runtime
// (libonnxruntime.a) and the InfiniFlow/deepdoc model snapshot; skipped unless
// the binary was built with static ORT and MODEL_DIR is set.
//
// Run:
//
//	MODEL_DIR=/path/to/deepdoc \
//	  go test -tags "cgo integration" -run TestNativeAnalyzerInProcess \
//	  ./internal/deepdoc/parser/pdf/inference/native_analyzer/...
//
// TestNativeAnalyzerUninitializedNegative locks the fail-fast contract the
// server depends on (see registerNativeDeepDoc): before Register wires ONNX
// Runtime, the backend must report not-serving and NewAnalyzer must refuse to
// build. It exercises the negative branches of Serving/NewAnalyzer/Health that
// the happy-path test never hits. It makes no InitORT call, so it is safe to
// run first (ORT is process-global) and even when ORT_LIB/MODEL_DIR are unset.
func TestNativeAnalyzerUninitializedNegative(t *testing.T) {
	if Serving() {
		t.Fatal("Serving() reported true before any Register; backend must be inert until initialized")
	}
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		modelDir = filepath.Join("..", "..", "..", "..", "rag", "res", "deepdoc")
	}
	if _, err := NewAnalyzer(modelDir, DefaultDropScore); err == nil {
		t.Error("NewAnalyzer succeeded before ONNX Runtime init; expected error")
	}
	a := &NativeAnalyzer{modelDir: modelDir}
	if a.Health() {
		t.Error("Health() reported healthy before ONNX Runtime init; expected false")
	}
}

// deepdocNativeRequired reports whether this environment is expected to provide
// the in-process DeepDoc backend (static ORT + models). CI/runner images that
// bake both set DEEPDOC_NATIVE_REQUIRED=1 so a missing prerequisite fails loud
// instead of silently skipping and painting CI green while ORT is never tested.

func TestNativeAnalyzerInProcess(t *testing.T) {
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		if deepdocNativeRequired() {
			t.Fatalf("MODEL_DIR must be set: the in-process DeepDoc backend is required (DEEPDOC_NATIVE_REQUIRED=1)")
		}
		t.Skip("MODEL_DIR required (in-process backend integration)")
	}
	// ONNX Runtime is resolved statically via dlopen(NULL). If the binary was
	// not built with static ORT, Register fails and we skip rather than fail.
	if err := Register(modelDir, DefaultDropScore); err != nil {
		if deepdocNativeRequired() {
			t.Fatalf("in-process backend unavailable (not statically linked) but required (DEEPDOC_NATIVE_REQUIRED=1): %v", err)
		}
		t.Skipf("in-process backend unavailable (not statically linked?): %v", err)
	}
	if !Serving() {
		if deepdocNativeRequired() {
			t.Fatalf("in-process backend not serving (ORT/models absent) but required (DEEPDOC_NATIVE_REQUIRED=1)")
		}
		t.Skip("in-process backend not serving (ORT/models absent)")
	}
	a, err := NewAnalyzer(modelDir, DefaultDropScore)
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}

	// page0.png is a content page with a known DLA golden (see native
	// testdata). Reuse it to prove the DocAnalyzer path runs real inference.
	imgPath := filepath.Join("..", "..", "..", "..", "native", "testdata", "page0.png")
	f, err := os.Open(imgPath)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	b := src.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())

	ctx := context.Background()

	dla, err := a.DLA(ctx, src)
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(dla) == 0 {
		t.Error("DLA returned 0 regions on a content page; expected >0")
	}
	for _, r := range dla {
		if r.X1 < r.X0 || r.Y1 < r.Y0 {
			t.Errorf("DLA region has inverted bounds: %+v", r)
		}
		if r.X0 < 0 || r.Y0 < 0 || r.X1 > w || r.Y1 > h {
			t.Errorf("DLA region out of image bounds (%dx%d): %+v", int(w), int(h), r)
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			t.Errorf("DLA confidence out of [0,1]: %+v", r)
		}
	}
	t.Logf("DLA: %d regions", len(dla))

	tsr, err := a.TSR(ctx, src)
	if err != nil {
		t.Fatalf("TSR: %v", err)
	}
	t.Logf("TSR: %d cells", len(tsr))

	det, err := a.OCRDetect(ctx, src)
	if err != nil {
		t.Fatalf("OCRDetect: %v", err)
	}
	if len(det) == 0 {
		t.Error("OCRDetect returned 0 boxes on a content page; expected >0")
	}
	for _, box := range det {
		if !quadInBounds(box, w, h) {
			t.Errorf("OCRDetect quad out of bounds (%dx%d): %+v", int(w), int(h), box)
		}
	}
	t.Logf("OCRDetect: %d boxes", len(det))

	// Crop a small region around the first detected box and recognize it,
	// proving OCRRecognize runs through the DocAnalyzer seam.
	if len(det) > 0 {
		crop := cropBox(src, det[0])
		rec, err := a.OCRRecognize(ctx, crop)
		if err != nil {
			t.Fatalf("OCRRecognize: %v", err)
		}
		t.Logf("OCRRecognize: %d text run(s)", len(rec))
	}
}

func quadInBounds(b deepdoctype.OCRBox, w, h float64) bool {
	xs := []float64{b.X0, b.X1, b.X2, b.X3}
	ys := []float64{b.Y0, b.Y1, b.Y2, b.Y3}
	for _, x := range xs {
		if x < -1 || x > w+1 {
			return false
		}
	}
	for _, y := range ys {
		if y < -1 || y > h+1 {
			return false
		}
	}
	return true
}

func cropBox(src image.Image, b deepdoctype.OCRBox) image.Image {
	minX, minY := b.X0, b.Y0
	maxX, maxY := b.X0, b.Y0
	for _, x := range []float64{b.X1, b.X2, b.X3} {
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
	}
	for _, y := range []float64{b.Y1, b.Y2, b.Y3} {
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}
	// Clamp to the source bounds so a box that pokes past an edge never
	// produces an out-of-range crop. The box coords are in the source image's
	// pixel space (same space OCRDetect reported them in).
	sb := src.Bounds()
	minX = math.Max(minX, float64(sb.Min.X))
	minY = math.Max(minY, float64(sb.Min.Y))
	maxX = math.Min(maxX, float64(sb.Max.X))
	maxY = math.Min(maxY, float64(sb.Max.Y))
	if maxX <= minX || maxY <= minY {
		return src
	}
	// OCRRecognize's OCRBoxes-from-image path assumes the image origin is
	// (0,0), so draw the crop into a fresh zero-origin image rather than
	// returning a SubImage (which would keep a non-zero Min and underflow the
	// pixel index in FromImage).
	r := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)),
		int(math.Ceil(maxX)), int(math.Ceil(maxY)))
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), src, r.Min, draw.Src)
	return out
}

// === Value-level golden equivalence for the in-process DocAnalyzer seam ===
//
// The tests below prove the NativeAnalyzer (the DocAnalyzer the PDF parser
// actually consumes) produces output equivalent to the Python deepdoc
// reference, reusing the SAME Python-reference goldens as the native
// integration suite. This closes the gap noted in deepdoc_go_alignment_report.md
// (Equivalence proof section): the in-
// process backend previously only had a smoke test (non-empty, in-bounds);
// these tests assert value-level parity through the analyzer's public API.

// goldenPath resolves a native testdata fixture from this package's test
// directory (internal/deepdoc/parser/pdf/inference/native_analyzer). Four ".." climb to
// internal/deepdoc, where the native module lives.
func goldenPath(name string) string {
	return filepath.Join("..", "..", "..", "..", "native", "testdata", name)
}

// openFixture decodes a PNG fixture the way the production path does (Go's
// image decode -> NativeAnalyzer), so the comparison exercises the real server
// code path rather than the native internal decoder.
func openFixture(t *testing.T, stem string) image.Image {
	t.Helper()
	f, err := os.Open(goldenPath(stem + ".png"))
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode fixture %s: %v", stem, err)
	}
	return src
}

// labelKey maps a layout/TSR label to a stable integer key so the analyzer's
// string Label can be matched against the golden's integer class under the
// same key space. Duplicate labels (DLA has two "table caption" entries) map to
// TestAnalyzerDLAGolden proves the analyzer's DLA output matches the Python
// reference golden (class + coordinates + confidence) within the documented
// sub-pixel floor, across the same fixtures the native suite uses.
func TestAnalyzerDLAGolden(t *testing.T) {
	a := analyzerWithModels(t)
	ctx := context.Background()
	labels := deepdoctype.DefaultDLALabels()
	stems := []string{"page0", "mp_textbook_en_p0", "mp_whitepaper_cn_p0", "mp_paper_eq_p0", "mp_zhtw_ent_p0",
		"dla_2510_figcap", "dla_bookrag_figcap", "dla_2510_eq",
		"dla_real_cn_report", "dla_real_zhtw", "dla_real_en_paper"}
	for _, stem := range stems {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			img := openFixture(t, stem)
			regions, err := a.DLA(ctx, img)
			if err != nil {
				t.Fatalf("DLA: %v", err)
			}
			got := make([][]float64, 0, len(regions))
			for _, r := range regions {
				got = append(got, []float64{r.X0, r.Y0, r.X1, r.Y1, r.Confidence, float64(labelKey(labels, r.Label))})
			}
			gold := native.LoadGoldenBoxes(t, goldenPath(stem+".dla.golden.json"))
			// Rewrite the golden's integer class to the same label key so both
			// sides match on the analyzer's label semantics.
			for i := range gold {
				gold[i][5] = float64(labelKey(labels, labels[int(gold[i][5])]))
			}
			matched, maxd, unmatched := native.MatchBoxesRelaxed(t, gold, got, 2.0, native.CmpTolScore)
			t.Logf("DLA %s: matched %d/%d, maxd %.3f px, unmatched %d", stem, matched, len(gold), maxd, len(unmatched))
			if matched != len(gold) {
				t.Errorf("DLA %s: matched %d/%d golden regions", stem, matched, len(gold))
			}
		})
	}
}

// TestAnalyzerTSRGolden proves the analyzer's TSR output matches the Python
// reference golden (structure: which cells are table/column/row) within the
// documented floor. The analyzer does not expose a TSR score, so the score
// tolerance is widened to ignore it; only class + coordinates are asserted.
func TestAnalyzerTSRGolden(t *testing.T) {
	a := analyzerWithModels(t)
	ctx := context.Background()
	labels := deepdoctype.DefaultTSRLabels()
	stems := []string{"table0", "tsr_table_normal", "tsr_table_rotation",
		"tsr_06_table_content", "tsr_18_table_caption", "tsr_13_crosspage", "tsr_14_interleaved",
		"tsr_real_report"}
	for _, stem := range stems {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			img := openFixture(t, stem)
			cells, err := a.TSR(ctx, img)
			if err != nil {
				t.Fatalf("TSR: %v", err)
			}
			got := make([][]float64, 0, len(cells))
			for _, c := range cells {
				got = append(got, []float64{c.X0, c.Y0, c.X1, c.Y1, 0, float64(labelKey(labels, c.Label))})
			}
			gold := native.LoadGoldenBoxes(t, goldenPath(stem+".tsr.golden.json"))
			for i := range gold {
				gold[i][5] = float64(labelKey(labels, labels[int(gold[i][5])]))
			}
			matched, maxd, unmatched := native.MatchBoxesRelaxed(t, gold, got, native.CmpTolCoord, 1.0)
			t.Logf("TSR %s: matched %d/%d, maxd %.3f px, unmatched %d", stem, matched, len(gold), maxd, len(unmatched))
			if matched != len(gold) {
				t.Errorf("TSR %s: matched %d/%d golden cells", stem, matched, len(gold))
			}
		})
	}
}

// TestAnalyzerOCRRecGolden proves the analyzer's OCR text recognition matches
// the Python reference golden exactly (EN / CJK / mixed / digits).
func TestAnalyzerOCRRecGolden(t *testing.T) {
	a := analyzerWithModels(t)
	ctx := context.Background()
	stems := []string{"line0", "line_cn", "line_mix", "line_num",
		"line_real_cn", "line_real_zhtw", "line_real_en"}
	for _, stem := range stems {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			img := openFixture(t, stem)
			rec, err := a.OCRRecognize(ctx, img)
			if err != nil {
				t.Fatalf("OCRRecognize: %v", err)
			}
			gold := ocrRecGoldText(t, goldenPath(stem+".ocr_rec.golden.json"))
			got := ""
			if len(rec) > 0 {
				got = rec[0].Text
			}
			if got != gold {
				t.Errorf("OCR-rec %s: got %q, gold %q", stem, got, gold)
			}
		})
	}
}

// TestAnalyzerDetGolden proves the analyzer's text detection matches the Python
// reference golden on page0: every Python box has a Go twin by center within the
// documented floor, and Go does not hallucinate beyond the accepted 3/5 orphan
// floor.
func TestAnalyzerDetGolden(t *testing.T) {
	a := analyzerWithModels(t)
	ctx := context.Background()
	img := openFixture(t, "page0")
	boxes, err := a.OCRDetect(ctx, img)
	if err != nil {
		t.Fatalf("OCRDetect: %v", err)
	}
	got := make([][][2]float64, 0, len(boxes))
	for _, b := range boxes {
		got = append(got, [][2]float64{{b.X0, b.Y0}, {b.X1, b.Y1}, {b.X2, b.Y2}, {b.X3, b.Y3}})
	}
	raw, err := os.ReadFile(goldenPath("page0.det.golden.json"))
	if err != nil {
		t.Skipf("golden unavailable: %v", err)
	}
	var gold struct {
		Output [][][][][2]float64 `json:"output"`
	}
	if err := json.Unmarshal(raw, &gold); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	goldBoxes := native.FlattenQuads(gold.Output)
	mG, mGo, maxd := native.MatchBothDirections(goldBoxes, got, native.CmpTolCoord)
	imG, imGo := native.MatchIoUBothDirections(goldBoxes, got, 0.5)
	t.Logf("Det page0: center matched(g/g)=%d/%d maxd=%.3f px | IoU orphan(g/g)=%d/%d",
		mG, mGo, maxd, len(goldBoxes)-imG, len(got)-imGo)
	if mG != len(goldBoxes) {
		t.Errorf("Det page0: %d/%d golden boxes matched by center (missing %d)", mG, len(goldBoxes), len(goldBoxes)-mG)
	}
	const detOrphanSlack = 5 // accepted IoU floor (3/5) + slack
	if len(got)-mGo > detOrphanSlack {
		t.Errorf("Det page0: %d unmatched Go boxes (got %d) exceeds accepted floor %d", len(got)-mGo, len(got), detOrphanSlack)
	}
}

// TestAnalyzerConcurrentBatchAndSingle exercises the NativeAnalyzer under the
// production fan-out profile that previously had NO race coverage: the
// PDF parser calls a single *NativeAnalyzer from page-parallel goroutines, and
// the batchRecognizer fast path mixes OCRRecognizeBatch (a single ONNX Run over
// every line, resized against the shared batch-max width) with standalone
// OCRRecognize (fixed recW) on the same instance. Under --test-native this
// package was run WITHOUT -race, so the only on-instance concurrency that was
// race-instrumented lived in the native package (which cannot import infnative
// due to the circular dependency, and thus cannot exercise the analyzer seam at
// all). This test closes that gap: it must be run with -race (see
// run_native_integration_tests in build.sh, which now also covers this package)
// so any data race on the analyzer's per-call tensors, the batch resize state,
// or the shared native session pool surfaces instead of being masked by
// "output happens to look right".
//
// It asserts three things:
//  1. No method returns an error and every result is non-nil (the instance is
//     usable from many goroutines at once).
//  2. Homogeneous concurrent calls of the SAME method are byte-identical to a
//     serial baseline (the shared pool / batch state is not contaminated).
//  3. The batch fast path and the single path can run concurrently on the same
//     instance without racing — the explicit gap this test exists to cover.
func TestAnalyzerConcurrentBatchAndSingle(t *testing.T) {
	a := analyzerWithModels(t)
	ctx := context.Background()

	// A content page exercises DLA/TSR/Det; the line crops exercise OCRRec and
	// the batch path. Reuse the in-process fixtures the other analyzer tests
	// already resolve through goldenPath.
	page := openFixture(t, "page0")
	lines := []string{"line0", "line_cn", "line_mix", "line_num",
		"line_real_cn", "line_real_zhtw", "line_real_en"}
	lineImgs := make([]image.Image, len(lines))
	for i, s := range lines {
		lineImgs[i] = openFixture(t, s)
	}

	// Serial baselines (must succeed; used for both non-nil checks and
	// homogeneous-concurrency equality).
	baseDLA, err := a.DLA(ctx, page)
	mustOK(t, "DLA", err)
	baseTSR, err := a.TSR(ctx, page)
	mustOK(t, "TSR", err)
	baseDet, err := a.OCRDetect(ctx, page)
	mustOK(t, "OCRDetect", err)
	baseRec, err := a.OCRRecognize(ctx, lineImgs[0])
	mustOK(t, "OCRRecognize", err)
	baseBatch, err := a.OCRRecognizeBatch(ctx, lineImgs)
	mustOK(t, "OCRRecognizeBatch", err)
	if len(baseBatch) != len(lineImgs) {
		t.Fatalf("OCRRecognizeBatch baseline size %d != %d", len(baseBatch), len(lineImgs))
	}

	const workers = 8

	// Homogeneous concurrency: each method is hammered from N goroutines on the
	// SAME instance. The result must equal the serial baseline wire-for-wire —
	// this catches session-pool / batch-state contamination that a single
	// flight would never surface.
	runHomogeneous(t, workers, "DLA", func() ([]deepdoctype.DLARegion, error) {
		return a.DLA(ctx, page)
	}, func(got []deepdoctype.DLARegion) string { return dlaWire(got) }, baseDLA, dlaWire)

	runHomogeneous(t, workers, "TSR", func() ([]deepdoctype.TSRCell, error) {
		return a.TSR(ctx, page)
	}, func(got []deepdoctype.TSRCell) string { return tsrWire(got) }, baseTSR, tsrWire)

	runHomogeneous(t, workers, "OCRDetect", func() ([]deepdoctype.OCRBox, error) {
		return a.OCRDetect(ctx, page)
	}, func(got []deepdoctype.OCRBox) string { return detWire(got) }, baseDet, detWire)

	runHomogeneous(t, workers, "OCRRecognize", func() ([]deepdoctype.OCRText, error) {
		return a.OCRRecognize(ctx, lineImgs[0])
	}, func(got []deepdoctype.OCRText) string { return ocrTextWire(got) }, baseRec, ocrTextWire)

	runHomogeneous(t, workers, "OCRRecognizeBatch", func() ([][]deepdoctype.OCRText, error) {
		return a.OCRRecognizeBatch(ctx, lineImgs)
	}, func(got [][]deepdoctype.OCRText) string { return batchWire(got) }, baseBatch, batchWire)

	// The explicit gap: mix the batch fast path with the single-crop path on
	// the same instance across many goroutines. Page-parallel parsing does
	// exactly this — some pages call OCRRecognizeBatch (the batchRecognizer
	// path), others fall through to standalone OCRRecognize — concurrently.
	var wg sync.WaitGroup
	errs := make([]error, workers*2)
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_, e := a.OCRRecognizeBatch(ctx, lineImgs)
			errs[i] = e
		}(i)
		go func(i int) {
			defer wg.Done()
			_, e := a.OCRRecognize(ctx, lineImgs[i%len(lineImgs)])
			errs[workers+i] = e
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("mixed batch+single worker %d: %v", i, e)
		}
	}
	t.Logf("concurrent batch+single mix: %d calls clean on one instance (with -race)", workers*2)
}

// mustOK fails the test when an analyzer call returns an error. The helper
// keeps the baseline block above flat instead of nesting in if-err checks.
func mustOK(t *testing.T, name string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s baseline: %v", name, err)
	}
}

// runHomogeneous drives `run` once per worker from separate goroutines on the
// SAME analyzer instance and asserts every concurrent result equals the serial
// baseline (base). It mirrors runConcurrentMatchesSerial in the native suite but
// operates through the NativeAnalyzer public API (which the native package
// cannot import). nonNil is asserted by the caller-supplied wire fn returning a
// string rather than erroring on nil input.
func runHomogeneous[T any](t *testing.T, workers int, name string, run func() (T, error), wire func(T) string, base T, _ func(T) string) {
	t.Helper()
	baseWire := wire(base)
	results := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e := run()
			if e != nil {
				errs[i] = e
				return
			}
			results[i] = wire(r)
		}(i)
	}
	wg.Wait()
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("%s concurrent worker %d: %v", name, i, errs[i])
		}
		if results[i] != baseWire {
			t.Fatalf("%s concurrent run %d differs from serial baseline (instance not concurrency-safe)", name, i)
		}
	}
	t.Logf("%s: %d concurrent runs == serial baseline (wire-identical)", name, workers)
}

// --- wire serializers: cheap, order-stable string form for equality checks ---

func dlaWire(rs []deepdoctype.DLARegion) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "%.4f,%.4f,%.4f,%.4f,%.4f,%s|", r.X0, r.Y0, r.X1, r.Y1, r.Confidence, r.Label)
	}
	return b.String()
}

func tsrWire(cs []deepdoctype.TSRCell) string {
	var b strings.Builder
	for _, c := range cs {
		fmt.Fprintf(&b, "%.4f,%.4f,%.4f,%.4f,%s|", c.X0, c.Y0, c.X1, c.Y1, c.Label)
	}
	return b.String()
}

func detWire(bs []deepdoctype.OCRBox) string {
	var b strings.Builder
	for _, bx := range bs {
		fmt.Fprintf(&b, "%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f|",
			bx.X0, bx.Y0, bx.X1, bx.Y1, bx.X2, bx.Y2, bx.X3, bx.Y3)
	}
	return b.String()
}

func ocrTextWire(ts []deepdoctype.OCRText) string {
	var b strings.Builder
	for _, x := range ts {
		fmt.Fprintf(&b, "%q,%.4f|", x.Text, x.Confidence)
	}
	return b.String()
}

func batchWire(b [][]deepdoctype.OCRText) string {
	var sb strings.Builder
	for _, ts := range b {
		sb.WriteString(ocrTextWire(ts))
		sb.WriteByte(';')
	}
	return sb.String()
}

// ocrRecGoldText reads the recognized text from an OCR-rec golden JSON
// ({"output": [[[["<text>"]]]]}).
func ocrRecGoldText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("golden unavailable: %v", err)
	}
	var gold struct {
		Output [][][][]any `json:"output"`
	}
	if err := json.Unmarshal(raw, &gold); err != nil {
		t.Fatalf("parse golden %s: %v", path, err)
	}
	return gold.Output[0][0][0][0].(string)
}

// TestAnalyzerOCRRecBatchIntegration exercises the caller-side batched OCR
// recognition path: NativeAnalyzer.OCRRecognizeBatch (the batchRecognizer fast
// path the PDF parser opts into from ocrDetectAndRecognize / buildTextBoxes)
// must produce, per crop, the SAME text and confidence as the underlying
// native.RunOCRRecBatchReal — it is a thin seam over that single-ONNX-Run
// batch, not a precision-changing reimplementation.
//
// NOTE: the batch is NOT expected to equal the per-crop OCRRecognize result.
// The batch resizes every line against the shared batch-max width, while a
// standalone OCRRecognize uses the fixed recW=320; so batch text legitimately
// differs from single (e.g. "PDF 1:Purpose..." vs "PDF 1: Purpose..."). That
// divergence is the whole point of batching and is asserted separately below.
func TestAnalyzerOCRRecBatchIntegration(t *testing.T) {
	a := analyzerWithModels(t)
	ctx := context.Background()
	stems := []string{"line0", "line_cn", "line_mix", "line_num",
		"line_real_cn", "line_real_zhtw", "line_real_en"}

	imgs := make([]image.Image, len(stems))
	singles := make([][]deepdoctype.OCRText, len(stems))
	for i, s := range stems {
		imgs[i] = openFixture(t, s)
		rec, err := a.OCRRecognize(ctx, imgs[i])
		if err != nil {
			t.Fatalf("OCRRecognize %s: %v", s, err)
		}
		singles[i] = rec
	}

	batch, err := a.OCRRecognizeBatch(ctx, imgs)
	if err != nil {
		t.Fatalf("OCRRecognizeBatch: %v", err)
	}
	if len(batch) != len(stems) {
		t.Fatalf("batch size %d != %d", len(batch), len(stems))
	}

	// Oracle: the underlying real-batch primitive the analyzer delegates to.
	nis, err := native.FromImages(imgs)
	if err != nil {
		t.Fatalf("FromImages: %v", err)
	}
	real, err := native.RunOCRRecBatchReal(ctx, a.modelDir, nis)
	if err != nil {
		t.Fatalf("RunOCRRecBatchReal: %v", err)
	}
	if len(real) != len(stems) {
		t.Fatalf("real batch size %d != %d", len(real), len(stems))
	}

	diffCount := 0
	for i, s := range stems {
		batchText := ""
		if len(batch[i]) > 0 {
			batchText = batch[i][0].Text
		}
		// Analyzer batch must match the underlying real batch exactly.
		if batchText != real[i].Text {
			t.Errorf("OCRRecognizeBatch %s text != RunOCRRecBatchReal: batch %q, real %q",
				s, batchText, real[i].Text)
		}
		if len(batch[i]) > 0 &&
			math.Abs(batch[i][0].Confidence-float64(real[i].Score)) > 1e-6 {
			t.Errorf("OCRRecognizeBatch %s confidence != RunOCRRecBatchReal: batch %.6f, real %.6f",
				s, batch[i][0].Confidence, float64(real[i].Score))
		}
		// The batch must actually engage batch semantics: at least some lines
		// differ from their standalone recognition (widened to the batch max).
		singleText := ""
		if len(singles[i]) > 0 {
			singleText = singles[i][0].Text
		}
		if batchText != singleText {
			diffCount++
		}
	}
	if diffCount == 0 {
		t.Errorf("batch semantics not engaged: OCRRecognizeBatch text == single-rec for ALL lines; expected batch-width widening to diverge at least some lines")
	} else {
		t.Logf("batch semantics engaged: %d/%d lines differ from standalone rec (batch-max width)", diffCount, len(stems))
	}
}
