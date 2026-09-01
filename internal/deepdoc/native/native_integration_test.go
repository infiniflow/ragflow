//go:build cgo && integration

package native

// Integration tests run the real ONNX models on the committed test crops and
// compare the Go output against golden JSON produced by the Python reference
// scripts (ref_dla.py / ref_tsr.py / ref_ocr_rec.py). They require MODEL_DIR
// (the DeepDoc model directory: layout.onnx, tsr.onnx, rec.onnx, ocr.res) and a
// usable ONNX Runtime. ONNX Runtime is linked statically (libonnxruntime.a)
// and resolved via dlopen(NULL) from the running binary, so no ORT_LIB / .so
// is needed.
//
// Run with:
//   MODEL_DIR=... go test -tags integration ./native/...

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// deepdocNativeRequired reports whether this environment is expected to provide
// the in-process DeepDoc backend (static ORT + models). CI/runner images that
// bake both set DEEPDOC_NATIVE_REQUIRED=1 so a missing prerequisite fails loud
// instead of silently skipping and painting CI green while ORT is never tested.
func deepdocNativeRequired() bool {
	return os.Getenv("DEEPDOC_NATIVE_REQUIRED") == "1"
}

// skipIfNoModels gates the integration tests behind MODEL_DIR (always
// required) and a usable ONNX Runtime. ONNX Runtime is resolved statically via
// dlopen(NULL); if the binary was not built with static ORT, InitORT() fails
// and we skip rather than fail, so the suite stays green in environments that
// lack the static ORT archives. When DEEPDOC_NATIVE_REQUIRED=1, a missing
// prerequisite is a hard failure instead of a skip.
func skipIfNoModels(t *testing.T) {
	if os.Getenv("MODEL_DIR") == "" {
		if deepdocNativeRequired() {
			t.Fatalf("MODEL_DIR must be set: the in-process DeepDoc backend is required (DEEPDOC_NATIVE_REQUIRED=1)")
		}
		t.Skip("set MODEL_DIR to run integration tests")
	}
	if err := InitORT(); err != nil {
		if deepdocNativeRequired() {
			t.Fatalf("ONNX Runtime not statically linked but required (DEEPDOC_NATIVE_REQUIRED=1): %v", err)
		}
		t.Skipf("ORT not available (not statically linked): %v", err)
	}
}

// modelSnapshotHashes locks the DeepDoc model snapshot that generated the
// golden fixtures and that backs the equivalence proof. The goldens are only
// meaningful if produced against these exact weights; if any model file in
// MODEL_DIR drifts, the equivalence claim is void and we must fail hard rather
// than silently emit a misleading pass.
//
// Update these values ONLY when the model snapshot is intentionally upgraded,
// and regenerate every golden fixture against the new snapshot in the same
// change (see /tmp/gen_corpus.py and ref_*.py).
var modelSnapshotHashes = map[string]string{
	"det.onnx":    "30a86f5731181461d08021402766601e4302a9b9b9666be8aff402696339cdff",
	"layout.onnx": "de401c03ee30b1c120416dc06f0705237f0c36d3cdb692c9bfefe8a8f98a4b70",
	"tsr.onnx":    "1585f88015c60209f16a079a26d944afca790ab7022fe7d0574113ccb9a6f9b4",
	"rec.onnx":    "1c7cf60de2afd728d512f4190cf37455092b45f06175365c6fc58d8cd7e2a68b",
	"ocr.res":     "28b2362ad4ab2dc38769aa72feb535e3a9ddb3fd2a7585a05920e6393b1dc7f7",
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkModelSnapshotHash is reviewer follow-up P0: lock the model snapshot so
// the equivalence proof cannot pass against a different (drifted) set of model
// weights than the goldens were generated with. Fatal on any mismatch.
func checkModelSnapshotHash(t *testing.T, md string) {
	for name, want := range modelSnapshotHashes {
		got, err := sha256File(filepath.Join(md, name))
		if err != nil {
			t.Fatalf("read model %s: %v", name, err)
		}
		if got != want {
			t.Fatalf("model snapshot drift for %s: got sha256 %s, want %s — "+
				"goldens were generated against a different model; regenerate fixtures or update the lock",
				name, got, want)
		}
		t.Logf("model snapshot ok: %s %s", name, got)
	}
}

// TestModelSnapshotHash is the standalone entry point for P0; it runs as part
// of the normal integration suite so a snapshot drift fails CI before any
// golden comparison happens.
func TestModelSnapshotHash(t *testing.T) {
	skipIfNoModels(t)
	checkModelSnapshotHash(t, os.Getenv("MODEL_DIR"))
}

// dlaPages / tsrPages / ocrRecLines enumerate the fixtures the Go port is
// compared against (golden JSON produced by the Python reference). page0 /
// table0 / line0 are the original single-page baselines; the mp_* pages broaden
// DLA/DET coverage to English textbooks, CN whitepapers, equation-heavy papers,
// and ZH-TW enterprise docs. TSR is validated only on real tables (table0 plus
// two crops from table_rotation_test.pdf) — the mp_* pages are not tables and
// only produce whole-page false detections, so they are excluded from tsrPages.
// tsr_table_normal is a moderate 2.65:1 table; tsr_table_rotation is a 1:6.3
// tall rotated table. Both sit comfortably under the 3px tolerance.
var dlaPages = []string{"page0", "mp_textbook_en_p0", "mp_whitepaper_cn_p0", "mp_paper_eq_p0", "mp_zhtw_ent_p0",
	"dla_2510_figcap", "dla_bookrag_figcap", "dla_2510_eq",
	"dla_real_cn_report", "dla_real_zhtw", "dla_real_en_paper"}
var tsrPages = []string{"table0", "tsr_table_normal", "tsr_table_rotation",
	"tsr_06_table_content", "tsr_18_table_caption", "tsr_13_crosspage", "tsr_14_interleaved",
	"tsr_real_report"}

// ocrRecLines covers EN (regular text + bold/italic/serif font variants of the
// same sentence to exercise font robustness), pure CJK, mixed EN+CJK, and a
// digits/symbols/CJK line. All texts are confined to the model's vocab (basic
// Latin + Chinese + digits/symbols); scripts the model cannot read (kana,
// Cyrillic, accented Latin) are intentionally excluded — the rec dict has
// neither, so goldens would be pure garbage and add no signal.
var ocrRecLines = []string{
	"line0", "line_cn",
	"line_en_bold", "line_en_italic", "line_en_serif",
	"line_mix", "line_num", "line_cn_long",
	"line_real_cn", "line_real_zhtw", "line_real_en",
}

func TestDLAIntegration(t *testing.T) {
	skipIfNoModels(t)
	for _, stem := range dlaPages {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			img, err := Decode(filepath.Join("testdata", stem+".png"))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			res, err := RunDLA(t.Context(), os.Getenv("MODEL_DIR"), img)
			if err != nil {
				t.Fatalf("RunDLA: %v", err)
			}
			var got struct {
				Boxes [][]float64 `json:"bboxes"`
			}
			if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
				t.Fatalf("parse Go wire: %v", err)
			}
			gold := LoadGoldenBoxes(t, filepath.Join("testdata", stem+".dla.golden.json"))
			CompareBoxes(t, gold, got.Boxes)
		})
	}
}

func TestTSRIntegration(t *testing.T) {
	skipIfNoModels(t)
	for _, stem := range tsrPages {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			img, err := Decode(filepath.Join("testdata", stem+".png"))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			// TSR decodes via the pure-Go path (tsr_decode.go), matching
			// deepdoc's PIL TSR decode.
			res, err := RunTSR(t.Context(), os.Getenv("MODEL_DIR"), img)
			if err != nil {
				t.Fatalf("RunTSR: %v", err)
			}
			var got struct {
				Boxes [][]float64 `json:"bboxes"`
			}
			if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
				t.Fatalf("parse Go wire: %v", err)
			}
			gold := LoadGoldenBoxes(t, filepath.Join("testdata", stem+".tsr.golden.json"))
			CompareBoxes(t, gold, got.Boxes)
		})
	}
}

// TestTSRExtremeAspect locks the known decode-floor behavior on extreme-aspect
// tables. The tsr_table_caption crop is ~4:1, so the model's 640x640 input
// squishes x by ~1.45x; the residual Go-vs-PIL JPEG decode difference is then
// amplified 1.45x on the way back to pixels, yielding ~8px box shifts (vs
// <1px on moderate tables). This is NOT a logic divergence — it is the same
// floor the 3px-tolerance real-table fixtures sit under, just scaled up by the
// aspect ratio. So this test uses a relaxed 10px tolerance and asserts only
// that the *structure* survives: the table (class 0) and all columns (class 1)
// must match, row count stays within ±1 of golden, and ONLY a near-threshold
// row (class 2, score<0.30) may be dropped — never a table or a column. That
// catches the real regression risk (hallucinated/missed table or column) while
// documenting the accepted floor amplification rather than hiding it.
func TestTSRExtremeAspect(t *testing.T) {
	skipIfNoModels(t)
	img, err := Decode(filepath.Join("testdata", "tsr_table_caption.png"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	res, err := RunTSR(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunTSR: %v", err)
	}
	var got struct {
		Boxes [][]float64 `json:"bboxes"`
	}
	if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
		t.Fatalf("parse Go wire: %v", err)
	}
	gold := LoadGoldenBoxes(t, filepath.Join("testdata", "tsr_table_caption.tsr.golden.json"))

	const (
		relaxCoord = 10.0
		relaxScore = 0.10
	)
	matched, maxd, unmatched := MatchBoxesRelaxed(t, gold, got.Boxes, relaxCoord, relaxScore)
	t.Logf("extreme-aspect: matched %d/%d, max coord diff %.3f px, unmatched=%d",
		matched, len(gold), maxd, len(unmatched))
	for _, u := range unmatched {
		// Only a near-threshold ROW may be dropped; a missing table/column is a
		// real regression (the floor should not erase a whole structural box).
		if int(u[5]) != 2 || u[4] >= 0.30 {
			t.Errorf("unmatched non-row or high-score box: class %d score %.3f", int(u[5]), u[4])
		}
	}
	// No hallucinated boxes: the near-threshold row is the only thing that can
	// go missing, so Go must never exceed the golden count.
	if len(got.Boxes) > len(gold) {
		t.Errorf("hallucinated boxes: got %d > golden %d", len(got.Boxes), len(gold))
	}
	// The table (class 0) must always be detected.
	hasTable := false
	for _, b := range got.Boxes {
		if int(b[5]) == 0 {
			hasTable = true
		}
	}
	if !hasTable {
		t.Errorf("extreme-aspect table (class 0) not detected")
	}
}

func TestOCRRecIntegration(t *testing.T) {
	skipIfNoModels(t)
	for _, stem := range ocrRecLines {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			img, err := Decode(filepath.Join("testdata", stem+".png"))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			res, err := RunOCRRec(t.Context(), os.Getenv("MODEL_DIR"), img)
			if err != nil {
				t.Fatalf("RunOCRRec: %v", err)
			}
			var got struct {
				Output [][][][]any `json:"output"`
			}
			if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
				t.Fatalf("parse Go wire: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join("testdata", stem+".ocr_rec.golden.json"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			var gold struct {
				Output [][][][]any `json:"output"`
			}
			if err := json.Unmarshal(raw, &gold); err != nil {
				t.Fatalf("parse golden: %v", err)
			}
			gotText := got.Output[0][0][0][0].(string)
			goldText := gold.Output[0][0][0][0].(string)
			if gotText != goldText {
				t.Fatalf("OCR-rec text mismatch: got %q, gold %q", gotText, goldText)
			}
			t.Logf("OCR-rec text matches: %q", gotText)
		})
	}
}

// The three recognizers below share the fixed-shape model-session pool. Each
// test runs the recognizer twice on the same crop and asserts byte-identical
// wire output, proving getModelSession hands back a clean session after release
// (no stale input tensor, no cross-call contamination) and that pooling is a
// no-behavior-change over the old per-call NewSession path.

// TestOCRRecBatchIntegration proves Go matches deepdoc's *batch* resize
// semantics: a narrow line (line_cn) recognized inside a batch that also
// contains wide lines is resized to the batch-wide max wh_ratio, not its own,
// so its text differs from the standalone recognition. Go must reproduce the
// same batch-wide text as the Python oracle (frozen in
// batch_ocr_rec.golden.json), which calls TextRecognizer on the whole list at
// once.
func TestOCRRecBatchIntegration(t *testing.T) {
	skipIfNoModels(t)
	stems := []string{"line_cn", "line_mix", "line_num", "line0"}
	imgs := make([]*Image, len(stems))
	for i, s := range stems {
		img, err := Decode(filepath.Join("testdata", s+".png"))
		if err != nil {
			t.Fatalf("decode %s: %v", s, err)
		}
		imgs[i] = img
	}
	res, err := RunOCRRecBatchReal(t.Context(), os.Getenv("MODEL_DIR"), imgs)
	if err != nil {
		t.Fatalf("RunOCRRecBatchReal: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "batch_ocr_rec.golden.json"))
	if err != nil {
		t.Fatalf("read batch golden: %v", err)
	}
	var gold []struct {
		Output [][][][]any `json:"output"`
	}
	if err := json.Unmarshal(raw, &gold); err != nil {
		t.Fatalf("parse batch golden: %v", err)
	}
	if len(gold) != len(res) {
		t.Fatalf("batch size mismatch: golden %d, go %d", len(gold), len(res))
	}
	for i, s := range stems {
		var got struct {
			Output [][][][]any `json:"output"`
		}
		if err := json.Unmarshal([]byte(res[i].Wire()), &got); err != nil {
			t.Fatalf("parse go wire %s: %v", s, err)
		}
		goldText := gold[i].Output[0][0][0][0].(string)
		gotText := got.Output[0][0][0][0].(string)
		if gotText != goldText {
			t.Errorf("batch line %s mismatch: got %q, gold %q", s, gotText, goldText)
			continue
		}
		t.Logf("batch line %s matches: %q", s, gotText)
	}

	// The batch must actually engage batch semantics: line_cn's batch text
	// must differ from its standalone recognition (it is widened to the
	// batch max), otherwise the test would pass trivially via the single-image
	// path.
	single, err := RunOCRRec(t.Context(), os.Getenv("MODEL_DIR"), imgs[0])
	if err != nil {
		t.Fatalf("RunOCRRec line_cn: %v", err)
	}
	if single.Text == res[0].Text {
		t.Errorf("batch semantics not engaged: line_cn batch text == single text (%q); expected a wider-resize result", single.Text)
	}

	// The REAL batch (single ONNX Run over the concatenated {N,3,48,imgW}
	// tensor) must produce numerically identical results to the per-line
	// RunOCRRec path: each line is resized against the same batch-max width in
	// both, so the only difference is the forward pass is shared. This guards
	// RunOCRRecBatchReal against silently diverging from the verified
	// single-line path.
	real, err := RunOCRRecBatchReal(t.Context(), os.Getenv("MODEL_DIR"), imgs)
	if err != nil {
		t.Fatalf("RunOCRRecBatchReal: %v", err)
	}
	if len(real) != len(res) {
		t.Fatalf("real batch size %d != semantic batch size %d", len(real), len(res))
	}
	for i, s := range stems {
		if real[i].Text != res[i].Text {
			t.Errorf("RunOCRRecBatchReal line %s text mismatch: real %q, per-line %q", s, real[i].Text, res[i].Text)
		}
		if math.Abs(float64(real[i].Score-res[i].Score)) > 1e-6 {
			t.Errorf("RunOCRRecBatchReal line %s score mismatch: real %v, per-line %v", s, real[i].Score, res[i].Score)
		}
	}
}

func TestDLASessionReuse(t *testing.T) {
	skipIfNoModels(t)
	img, err := Decode(filepath.Join("testdata", "page0.png"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	r1, err := RunDLA(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunDLA #1: %v", err)
	}
	r2, err := RunDLA(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunDLA #2: %v", err)
	}
	if r1.Wire() != r2.Wire() {
		t.Fatalf("DLA output changed across pooled runs (session reuse not stable)")
	}
}

func TestTSRSessionReuse(t *testing.T) {
	skipIfNoModels(t)
	img, err := Decode(filepath.Join("testdata", "table0.png"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	r1, err := RunTSR(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunTSR #1: %v", err)
	}
	r2, err := RunTSR(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunTSR #2: %v", err)
	}
	if r1.Wire() != r2.Wire() {
		t.Fatalf("TSR output changed across pooled runs (session reuse not stable)")
	}
}

func TestOCRRecSessionReuse(t *testing.T) {
	skipIfNoModels(t)
	img, err := Decode(filepath.Join("testdata", "line0.png"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	r1, err := RunOCRRec(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunOCRRec #1: %v", err)
	}
	r2, err := RunOCRRec(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunOCRRec #2: %v", err)
	}
	if r1.Wire() != r2.Wire() {
		t.Fatalf("OCR-rec output changed across pooled runs (session reuse not stable)")
	}
}

// wireRunner runs one inference pass and returns its serialized wire output.
type wireRunner func(ctx context.Context, md string, img *Image) (string, error)

// runConcurrentMatchesSerial drives `run` once serially to establish a baseline
// wire, then `workers` times concurrently from separate goroutines, and asserts
// every concurrent result is byte-identical to the serial baseline. This is
// reviewer follow-up P3: it proves the shared model-session pool is
// concurrency-safe — no cross-call tensor contamination and no data race under
// parallel load. (The pool is the only shared mutable state across calls; the
// per-call input/output tensors are owned by the call.)
func runConcurrentMatchesSerial(t *testing.T, md string, img *Image, name string, run wireRunner) {
	base, err := run(t.Context(), md, img)
	if err != nil {
		t.Fatalf("%s serial: %v", name, err)
	}
	const workers = 8
	var wg sync.WaitGroup
	results := make([]string, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e := run(t.Context(), md, img)
			results[i], errs[i] = r, e
		}(i)
	}
	wg.Wait()
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("%s concurrent worker %d: %v", name, i, errs[i])
		}
		if results[i] != base {
			t.Fatalf("%s concurrent run %d differs from serial baseline (session pool not concurrency-safe)", name, i)
		}
	}
	t.Logf("%s: %d concurrent runs == serial baseline (wire-identical)", name, workers)
}

// TestInferenceConcurrencyConsistent exercises the shared session pool under
// parallel load for every inference task (DLA / TSR / OCR-rec / Det) and proves
// numerical output is identical to a serial run. Catches data races and
// cross-call contamination that the single-flight reuse tests cannot.
func TestInferenceConcurrencyConsistent(t *testing.T) {
	skipIfNoModels(t)
	md := os.Getenv("MODEL_DIR")

	dlaImg, err := Decode(filepath.Join("testdata", "page0.png"))
	if err != nil {
		t.Fatalf("decode dla: %v", err)
	}
	runConcurrentMatchesSerial(t, md, dlaImg, "DLA",
		func(ctx context.Context, m string, im *Image) (string, error) {
			r, e := RunDLA(ctx, m, im)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		})

	tsrImg, err := Decode(filepath.Join("testdata", "table0.png"))
	if err != nil {
		t.Fatalf("decode tsr: %v", err)
	}
	runConcurrentMatchesSerial(t, md, tsrImg, "TSR",
		func(ctx context.Context, m string, im *Image) (string, error) {
			r, e := RunTSR(ctx, m, im)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		})

	ocrImg, err := Decode(filepath.Join("testdata", "line0.png"))
	if err != nil {
		t.Fatalf("decode ocr: %v", err)
	}
	runConcurrentMatchesSerial(t, md, ocrImg, "OCR-rec",
		func(ctx context.Context, m string, im *Image) (string, error) {
			r, e := RunOCRRec(ctx, m, im)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		})

	detImg, err := Decode(filepath.Join("testdata", "page0.png"))
	if err != nil {
		t.Fatalf("decode det: %v", err)
	}
	runConcurrentMatchesSerial(t, md, detImg, "Det",
		func(ctx context.Context, m string, im *Image) (string, error) {
			r, e := RunDet(ctx, m, im)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		})
}

// TestInferenceConcurrencyMixedStress reproduces the production fan-out profile
// — several simultaneous calls, perTask each of DLA / TSR / OCR-rec / Det —
// against the shared session pools. It runs under -race (see build.sh's
// run_native_integration_tests) so any data race on the pools, the per-session
// in/out tensors, or the cross-call max_wh_ratio batch state surfaces instead
// of being masked by "output happens to look right". Each task's result must
// still equal its serial baseline, proving the pools are safe under real mixed
// contention (not just homogeneous 8-way load).
//
// perTask is capped at 8 (32 concurrent total) on purpose: under -race the
// per-call ORT input tensors are shadowed ~10x, and 100 live concurrent calls
// blow the CI runner's memory budget (the whole package used to run under
// -race and the runner OOM-killed it with SIGTERM). Race detection does not
// need 100 calls — any concurrency level >= 2 exercises the shared pool — so
// 32 is plenty to catch a data race while staying within memory.
func TestInferenceConcurrencyMixedStress(t *testing.T) {
	skipIfNoModels(t)
	md := os.Getenv("MODEL_DIR")

	dlaImg, _ := Decode(filepath.Join("testdata", "page0.png"))
	tsrImg, _ := Decode(filepath.Join("testdata", "table0.png"))
	ocrImg, _ := Decode(filepath.Join("testdata", "line0.png"))
	detImg, _ := Decode(filepath.Join("testdata", "page0.png"))

	type task struct {
		name string
		run  func(ctx context.Context) (string, error)
	}
	tasks := []task{
		{"DLA", func(ctx context.Context) (string, error) {
			r, e := RunDLA(ctx, md, dlaImg)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		}},
		{"TSR", func(ctx context.Context) (string, error) {
			r, e := RunTSR(ctx, md, tsrImg)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		}},
		{"OCR-rec", func(ctx context.Context) (string, error) {
			r, e := RunOCRRec(ctx, md, ocrImg)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		}},
		{"Det", func(ctx context.Context) (string, error) {
			r, e := RunDet(ctx, md, detImg)
			if e != nil {
				return "", e
			}
			return r.Wire(), nil
		}},
	}

	// Serial baselines for each task.
	baselines := make([]string, len(tasks))
	for i, tk := range tasks {
		b, e := tk.run(t.Context())
		if e != nil {
			t.Fatalf("baseline %s: %v", tk.name, e)
		}
		baselines[i] = b
	}

	const perTask = 8
	total := perTask * len(tasks)
	var wg sync.WaitGroup
	errs := make([]error, total)
	results := make([]string, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tk := tasks[i%len(tasks)]
			r, e := tk.run(t.Context())
			results[i], errs[i] = r, e
		}(i)
	}
	wg.Wait()

	for i := 0; i < total; i++ {
		tk := tasks[i%len(tasks)]
		base := baselines[i%len(tasks)]
		if errs[i] != nil {
			t.Fatalf("%s concurrent call %d: %v", tk.name, i, errs[i])
		}
		if results[i] != base {
			t.Fatalf("%s concurrent call %d diverges from serial baseline (pool not concurrency-safe under mixed load)", tk.name, i)
		}
	}
	t.Logf("mixed stress: %d concurrent calls (25×4 tasks) == serial baselines (wire-identical)", total)
}

func TestDetIntegration(t *testing.T) {
	skipIfNoModels(t)
	imgPath := filepath.Join("testdata", "page0.png")
	img, err := Decode(imgPath)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	res, err := RunDet(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunDet: %v", err)
	}
	// Go wire: {"output": [[ [ [x,y]*4, ... ] ]]}; boxes at output[0][0].
	var got struct {
		Output [][][][][2]float64 `json:"output"`
	}
	if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
		t.Fatalf("parse Go wire: %v", err)
	}
	if len(got.Output) == 0 || len(got.Output[0]) == 0 {
		t.Fatalf("Go wire missing boxes")
	}
	goBoxes := got.Output[0][0]

	raw, err := os.ReadFile(filepath.Join("testdata", "page0.det.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var gold struct {
		Output [][][][][2]float64 `json:"output"`
	}
	if err := json.Unmarshal(raw, &gold); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if len(gold.Output) == 0 || len(gold.Output[0]) == 0 {
		t.Fatalf("golden missing boxes")
	}
	refBoxes := gold.Output[0][0]

	// The pure-Go geometry reaches the 3px hard floor (README.md §3, box#8).
	// Tolerance is CoordFloor + CoordTolMargin, just above that floor, so a
	// regression bumps it over the line.
	detCoordTol := CoordFloor + CoordTolMargin
	used := make([]bool, len(goBoxes))
	matched, maxd := 0, 0.0
	for _, rb := range refBoxes {
		rcx, rcy := quadCenter(rb)
		best, bd := -1, math.MaxFloat64
		for i, vb := range goBoxes {
			if used[i] {
				continue
			}
			vcx, vcy := quadCenter(vb)
			d := (rcx-vcx)*(rcx-vcx) + (rcy-vcy)*(rcy-vcy)
			if d < bd {
				bd, best = d, i
			}
		}
		if best < 0 {
			t.Errorf("no Go box matched golden quad at (%.0f,%.0f)", rcx, rcy)
			continue
		}
		used[best] = true
		matched++
		for j := 0; j < 4; j++ {
			for k := 0; k < 2; k++ {
				diff := math.Abs(rb[j][k] - goBoxes[best][j][k])
				if diff > detCoordTol {
					t.Errorf("quad coord diff %.3f > tol %.2f (gold=%v got=%v)",
						diff, detCoordTol, rb, goBoxes[best])
				}
				if diff > maxd {
					maxd = diff
				}
			}
		}
	}
	t.Logf("det: matched %d/%d golden quads, max coord diff %.4f px (tol %.1f)",
		matched, len(refBoxes), maxd, detCoordTol)
	if matched != len(refBoxes) {
		t.Errorf("matched %d/%d quads", matched, len(refBoxes))
	}
}

// TestDetMembershipAllFixtures quantifies the det box-membership gap (gap 3).
// For every committed det fixture it runs RunDet and compares the Go box set
// against the golden (Python) box set in TWO complementary ways:
//  1. center-distance match (tol 3.5px) — isolates coordinate drift;
//  2. IoU match (thr 0.5) — isolates true box-membership divergence (splits,
//     merges, hallucinations), independent of how far a box's center moved.
//
// The original TestDetIntegration only checked golden→Go by nearest center on
// a single fixture, so a Go box shifted >3.5px was mis-flagged and a Go box
// with no golden counterpart was invisible.
//
// This is a REGRESSION GUARD, not a zero-target. The IoU orphan counts are
// pinned to the baseline measured on 2026-08-10 (gap 3: 37 golden misses + 20
// extra Go boxes across 37 fixtures, concentrated in dense-text pages). The
// test fails ONLY if a future geometry change makes Go WORSE than that
// baseline — it does not require the gap to reach zero. A small slack absorbs
// run-to-run nondeterminism (see gap 7); it is NOT headroom for new divergence.
func TestDetMembershipAllFixtures(t *testing.T) {
	skipIfNoModels(t)
	md := os.Getenv("MODEL_DIR")
	dir := "testdata"

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	type stat struct {
		stem                             string
		nGold, nGo                       int
		centerOrphanGold, centerOrphanGo int
		iouMatchedGold, iouMatchedGo     int
		iouOrphanGold, iouOrphanGo       int
		maxd                             float64
	}
	var stats []stat
	sumGold, sumGo := 0, 0
	sumCIoG, sumCIoGo := 0, 0
	sumIIoG, sumIIoGo := 0, 0

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".det.golden.json") {
			continue
		}
		stem := strings.TrimSuffix(name, ".det.golden.json")
		img, err := Decode(filepath.Join(dir, stem+".png"))
		if err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		res, err := RunDet(t.Context(), md, img)
		if err != nil {
			t.Fatalf("RunDet %s: %v", stem, err)
		}
		var got struct {
			Output [][][][][2]float64 `json:"output"`
		}
		if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
			t.Fatalf("parse Go wire %s: %v", stem, err)
		}
		goBoxes := FlattenQuads(got.Output)

		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read golden %s: %v", stem, err)
		}
		var gold struct {
			Output [][][][][2]float64 `json:"output"`
		}
		if err := json.Unmarshal(raw, &gold); err != nil {
			t.Fatalf("parse golden %s: %v", stem, err)
		}
		goldBoxes := FlattenQuads(gold.Output)

		mG, mGo, maxd := MatchBothDirections(goldBoxes, goBoxes, CmpTolCoord)
		imG, imGo := MatchIoUBothDirections(goldBoxes, goBoxes, 0.5)
		s := stat{
			stem:             stem,
			nGold:            len(goldBoxes),
			nGo:              len(goBoxes),
			centerOrphanGold: len(goldBoxes) - mG,
			centerOrphanGo:   len(goBoxes) - mGo,
			iouMatchedGold:   imG,
			iouMatchedGo:     imGo,
			iouOrphanGold:    len(goldBoxes) - imG,
			iouOrphanGo:      len(goBoxes) - imGo,
			maxd:             maxd,
		}
		stats = append(stats, s)
		sumGold += s.nGold
		sumGo += s.nGo
		sumCIoG += s.centerOrphanGold
		sumCIoGo += s.centerOrphanGo
		sumIIoG += s.iouOrphanGold
		sumIIoGo += s.iouOrphanGo
	}

	for _, s := range stats {
		t.Logf("det %-22s gold=%d go=%d | centerOrphan(g/g)=%d/%d maxd=%.1f | iouOrphan(g/g)=%d/%d",
			s.stem, s.nGold, s.nGo, s.centerOrphanGold, s.centerOrphanGo, s.maxd, s.iouOrphanGold, s.iouOrphanGo)
	}
	t.Logf("TOTAL gold=%d go=%d", sumGold, sumGo)
	t.Logf("center orphan(gold/go)=%d/%d  |  IoU orphan(gold/go)=%d/%d",
		sumCIoG, sumCIoGo, sumIIoG, sumIIoGo)

	// Regression guard: the IoU orphan baseline is the KNOWN gap, not a target.
	// Fail only if Go gets WORSE than the baseline — a future geometry change
	// must not introduce new box-membership divergence. The slack absorbs benign
	// run-to-run nondeterminism.
	//
	// Baseline history:
	//   - 37/20 — pinned against the old non-bit-exact FillConvexPoly scanline
	//     (over-drew, kept 5 boxes at the 0.5 threshold by coincidence). Wrong.
	//   - 42/13 — after fillPoly was rewritten to bit-match cv2.fillPoly, the
	//     true gap vs the THEN-COMMITTED goldens. Those goldens were later found
	//     STALE: they were ~20 boxes denser than the current live TextDetector
	//     (generated with an older onnxruntime; the drift gate's 15%-count-only
	//     det check masked it), so 42/13 over-reported Go's divergence.
	//   - 23/9 — re-measured on 2026-08-11 AFTER the .det.golden.json fixtures
	//     were regenerated from the current live TextDetector and the contour
	//     grouping was ported to a findContours-equivalent (Suzuki-Abe / Moore-
	//     neighbour border following, RETR_LIST). This was the true Go-vs-cv2
	//     box-membership gap; the residual 23/9 was a downstream pred-map
	//     divergence (R/B channel-order swap in normalizeCHW: Go applied the
	//     ImageNet stats to BGR bytes while deepdoc's TextDetector normalizes
	//     the RGB image directly, so 0.485 landed on B in Go and on R in
	//     deepdoc — a ~3e-3 pred-map gap that box_score_fast amplified into
	//     score-crossing orphans).
	//   - 3/5 — re-measured on 2026-08-12 AFTER normalizeCHW was fixed to feed
	//     RGB bytes (detPreprocess now uses img.Pix, not img.ToBGR) with
	//     RGB-order stats, matching deepdoc. The Go det pred map now matches
	//     the live TextDetector to mean|Δ|≈4e-5 (was 3.2e-3); the only
	//     remaining IoU orphans are contour-tracer geometry (e.g. mp_cn_sm_p0
	//     gold 303 vs go 306), not pred/score. Do not raise this baseline back
	//     toward 23/9, 42/13 or 37/20 — those tracked a swapped-channel oracle
	//     or a stale golden, not Go's real divergence.
	const (
		baselineIoUOrphanGold = 3
		baselineIoUOrphanGo   = 5
		iuSlack               = 3
	)
	if sumIIoG > baselineIoUOrphanGold+iuSlack {
		t.Errorf("Go missed %d golden boxes under IoU (> baseline %d+%d): box-membership REGRESSION (gap 3)",
			sumIIoG, baselineIoUOrphanGold, iuSlack)
	}
	if sumIIoGo > baselineIoUOrphanGo+iuSlack {
		t.Errorf("Go produced %d extra boxes under IoU (> baseline %d+%d): box-membership REGRESSION (gap 3)",
			sumIIoGo, baselineIoUOrphanGo, iuSlack)
	}
	t.Logf("IoU orphan baseline(gold/go)=%d/%d (+slack %d) — current %d/%d OK",
		baselineIoUOrphanGold, baselineIoUOrphanGo, iuSlack, sumIIoG, sumIIoGo)
}

// TestDetOCRAdjudication answers a different question than
// TestDetMembershipAllFixtures. The membership test measures how close Go's
// boxes are to Python's (alignment). This harness measures which detector
// produces boxes that lead to BETTER parsing: for every box only one side
// found (an "IoU orphan"), it crops the original image to that box and runs
// OCR-rec, then reports whether the crop yields coherent text. The side whose
// orphan boxes more often produce real text is the side that detected genuine
// text regions the other missed. Python is NOT assumed truth here — OCR
// quality is the independent judge.
//
// Informational only: it never fails. Per-box text is logged so a human can
// adjudicate; the summary tallies non-empty, confident OCR results per side as
// a proxy for "found real text". The crop is axis-aligned (not rotated) so both
// detectors are judged by the same method.
func TestDetOCRAdjudication(t *testing.T) {
	skipIfNoModels(t)
	md := os.Getenv("MODEL_DIR")
	dir := "testdata"

	// Fixtures with non-zero IoU orphans (the only ones worth adjudicating).
	stems := []string{
		"mp_cn_sm_p0",    // dense Chinese small text — worst divergence
		"mp_arxiv_p0",    // multi-column paper
		"mp_en_dense_p0", // dense English
		"mp_physics_p5",
		"mp_sec_p0",
	}

	const (
		iouThr       = 0.5
		realScoreThr = 0.6 // OCR confidence above which a crop is "likely real text"
	)

	sumPyOnly, sumPyOnlyReal := 0, 0
	sumGoOnly, sumGoOnlyReal := 0, 0

	for _, stem := range stems {
		img, err := Decode(filepath.Join(dir, stem+".png"))
		if err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		res, err := RunDet(t.Context(), md, img)
		if err != nil {
			t.Fatalf("RunDet %s: %v", stem, err)
		}
		var got struct {
			Output [][][][][2]float64 `json:"output"`
		}
		if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
			t.Fatalf("parse Go wire %s: %v", stem, err)
		}
		goBoxes := FlattenQuads(got.Output)

		raw, err := os.ReadFile(filepath.Join(dir, stem+".det.golden.json"))
		if err != nil {
			t.Fatalf("read golden %s: %v", stem, err)
		}
		var gold struct {
			Output [][][][][2]float64 `json:"output"`
		}
		if err := json.Unmarshal(raw, &gold); err != nil {
			t.Fatalf("parse golden %s: %v", stem, err)
		}
		pyBoxes := FlattenQuads(gold.Output)

		pyOnly, goOnly := matchIoUOrphans(pyBoxes, goBoxes, iouThr)
		fPyOnly, fPyOnlyReal := 0, 0
		fGoOnly, fGoOnlyReal := 0, 0

		for _, i := range pyOnly {
			txt, score, ok := ocrCrop(t, md, img, pyBoxes[i])
			real := ok && strings.TrimSpace(txt) != "" && score >= realScoreThr
			if ok {
				fPyOnly++
				if real {
					fPyOnlyReal++
				}
			}
			t.Logf("  [PY-only] %-14s box#%d text=%q score=%.2f real=%v", stem, i, txt, score, real)
		}
		for _, i := range goOnly {
			txt, score, ok := ocrCrop(t, md, img, goBoxes[i])
			real := ok && strings.TrimSpace(txt) != "" && score >= realScoreThr
			if ok {
				fGoOnly++
				if real {
					fGoOnlyReal++
				}
			}
			t.Logf("  [GO-only] %-14s box#%d text=%q score=%.2f real=%v", stem, i, txt, score, real)
		}

		t.Logf("%-14s: PY-only=%d (real %d) | GO-only=%d (real %d)", stem, fPyOnly, fPyOnlyReal, fGoOnly, fGoOnlyReal)
		sumPyOnly += fPyOnly
		sumPyOnlyReal += fPyOnlyReal
		sumGoOnly += fGoOnly
		sumGoOnlyReal += fGoOnlyReal
	}

	t.Logf("SUMMARY — orphan boxes that yielded real text (score>=%.1f):", realScoreThr)
	t.Logf("  Python-only: %d/%d real", sumPyOnlyReal, sumPyOnly)
	t.Logf("  Go-only:     %d/%d real", sumGoOnlyReal, sumGoOnly)
	t.Logf("  (higher 'real' count on a side = that side found more genuine text the other missed)")
}

// ocrCrop crops img to the axis-aligned bbox of quad q, runs OCR-rec, and
// returns the recognized text, confidence, and whether the crop was valid.
func ocrCrop(t *testing.T, md string, img *Image, q [][2]float64) (string, float32, bool) {
	c := cropQuad(img, q)
	if c == nil {
		return "", 0, false
	}
	r, err := RunOCRRec(t.Context(), md, c)
	if err != nil {
		t.Logf("  ocrCrop rec error: %v", err)
		return "", 0, false
	}
	return r.Text, r.Score, true
}

// cropQuad returns the axis-aligned sub-image of img bounded by quad q, or nil
// if the quad is degenerate/out of bounds. AABB (not rotated) crop is used so
// both detectors are judged by the same method — any background inclusion
// affects both sides equally and the comparison stays fair.
func cropQuad(img *Image, q [][2]float64) *Image {
	x0, y0, x1, y1 := quadAABB(q)
	px0 := clampi(int(math.Floor(x0)), 0, img.W-1)
	py0 := clampi(int(math.Floor(y0)), 0, img.H-1)
	px1 := clampi(int(math.Ceil(x1)), 0, img.W)
	py1 := clampi(int(math.Ceil(y1)), 0, img.H)
	if px1 <= px0 || py1 <= py0 {
		return nil
	}
	w, h := px1-px0, py1-py0
	pix := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		copy(pix[y*w*3:(y+1)*w*3], img.Pix[((py0+y)*img.W+px0)*3:((py0+y)*img.W+px0)*3+w*3])
	}
	return &Image{W: w, H: h, Pix: pix}
}

// matchIoUOrphans returns the indices (into pyBoxes / goBoxes) of boxes that
// have no counterpart on the other side under greedy best-IoU >= thr. These
// are the "orphan" boxes found by only one detector. Greedy matching is run in
// both directions so a box with no stable twin on either side is reported.
func matchIoUOrphans(pyBoxes, goBoxes [][][2]float64, thr float64) (pyOnly, goOnly []int) {
	usedGo := make([]bool, len(goBoxes))
	pyMatched := make([]bool, len(pyBoxes))
	for i, gb := range pyBoxes {
		best, bestI := -1, 0.0
		for j, vb := range goBoxes {
			if usedGo[j] {
				continue
			}
			if v := iou(gb, vb); v > bestI {
				bestI, best = v, j
			}
		}
		if best >= 0 && bestI >= thr {
			usedGo[best] = true
			pyMatched[i] = true
		}
	}
	for i := range pyBoxes {
		if !pyMatched[i] {
			pyOnly = append(pyOnly, i)
		}
	}
	usedPy := make([]bool, len(pyBoxes))
	goMatched := make([]bool, len(goBoxes))
	for j, vb := range goBoxes {
		best, bestI := -1, 0.0
		for i, gb := range pyBoxes {
			if usedPy[i] {
				continue
			}
			if v := iou(gb, vb); v > bestI {
				bestI, best = v, i
			}
		}
		if best >= 0 && bestI >= thr {
			usedPy[best] = true
			goMatched[j] = true
		}
	}
	for j := range goBoxes {
		if !goMatched[j] {
			goOnly = append(goOnly, j)
		}
	}
	return pyOnly, goOnly
}

// TestDetSessionPoolBounded is the regression guard for the unbounded
// sync.Map leak: a long-running server ingesting many differently-sized pages
// used to pin one pool + cached tensors per unique (modelPath, rh, rw) forever.
// It now bounds the pool set (detMaxShapePools). We drive far more distinct
// page sizes than the cap and assert the live pool count never exceeds it.
func TestDetSessionPoolBounded(t *testing.T) {
	skipIfNoModels(t)
	modelDir := os.Getenv("MODEL_DIR")

	const n = 80
	for i := 0; i < n; i++ {
		w := 64 + i*14
		h := 128 + (i*9)%500
		if w > 952 {
			w = 952
		}
		if h > 952 {
			h = 952
		}
		// Synthetic gray raster; we only exercise the pool lifecycle here, not
		// detection quality, so the content is irrelevant.
		img := &Image{W: w, H: h, Pix: make([]byte, w*h*3)}
		for j := 0; j < len(img.Pix); j += 3 {
			img.Pix[j], img.Pix[j+1], img.Pix[j+2] = 200, 200, 200
		}
		if _, err := RunDet(t.Context(), modelDir, img); err != nil {
			t.Fatalf("RunDet #%d (%dx%d): %v", i, w, h, err)
		}
	}

	got := detSessions.KeyCount()
	if got > detMaxShapePools {
		t.Fatalf("det session pool set grew unbounded: %d pools (cap %d) after %d distinct sizes",
			got, detMaxShapePools, n)
	}
	t.Logf("det session pool set bounded: %d pools (cap %d) after %d distinct sizes", got, detMaxShapePools, n)
}

// jsonTemplate reduces a decoded JSON value to a structural signature where
// every number becomes "#", every string "$", every bool "?", and object keys
// are sorted. Two values with identical nesting/keys/leaf-types produce the
// same template even if their values differ — so it isolates schema from
// content.
func jsonTemplate(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(k)
			b.WriteString(":")
			b.WriteString(jsonTemplate(x[k]))
		}
		b.WriteString("}")
		return b.String()
	case []any:
		if len(x) == 0 {
			return "[]"
		}
		// Structures here have uniform inner shape, so the first element's
		// template represents the whole array.
		return "[" + jsonTemplate(x[0]) + "]"
	case float64:
		return "#"
	case string:
		return "$"
	case bool:
		return "?"
	case nil:
		return "null"
	default:
		return "?"
	}
}

// TestWireSchemaMatchesGolden is the schema half of the migration contract: a
// caller parsing Go's Wire() output must see the exact same JSON structure
// (top-level key, nesting depth, leaf types) as the deepdoc reference golden.
// The per-task integration tests already check values; this guard catches a
// shape regression (e.g. {"bboxes":...} vs a bare array, or a changed nesting)
// that value comparison would otherwise paper over.
func TestWireSchemaMatchesGolden(t *testing.T) {
	skipIfNoModels(t)
	md := os.Getenv("MODEL_DIR")
	cases := []struct {
		name   string
		stem   string
		golden string
		wire   func(t *testing.T, img *Image) string
	}{
		{"DLA", "page0", "page0.dla.golden.json", func(t *testing.T, img *Image) string {
			res, err := RunDLA(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunDLA: %v", err)
			}
			return res.Wire()
		}},
		{"TSR", "table0", "table0.tsr.golden.json", func(t *testing.T, img *Image) string {
			res, err := RunTSR(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunTSR: %v", err)
			}
			return res.Wire()
		}},
		{"DET", "page0", "page0.det.golden.json", func(t *testing.T, img *Image) string {
			res, err := RunDet(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunDet: %v", err)
			}
			return res.Wire()
		}},
		{"OCR_REC", "line_mix", "line_mix.ocr_rec.golden.json", func(t *testing.T, img *Image) string {
			res, err := RunOCRRec(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunOCRRec: %v", err)
			}
			return res.Wire()
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			img, err := Decode(filepath.Join("testdata", c.stem+".png"))
			if err != nil {
				t.Fatalf("decode %s: %v", c.stem, err)
			}
			goWire := c.wire(t, img)
			goldRaw, err := os.ReadFile(filepath.Join("testdata", c.golden))
			if err != nil {
				t.Fatalf("read golden %s: %v", c.golden, err)
			}
			var gv, gv2 any
			if err := json.Unmarshal([]byte(goWire), &gv); err != nil {
				t.Fatalf("parse go wire: %v", err)
			}
			if err := json.Unmarshal(goldRaw, &gv2); err != nil {
				t.Fatalf("parse golden: %v", err)
			}
			if got, want := jsonTemplate(gv), jsonTemplate(gv2); got != want {
				t.Errorf("wire schema mismatch:\n got  %s\n want %s", got, want)
			} else {
				t.Logf("%s wire schema: %s", c.name, got)
			}
		})
	}
}

// TestDumpGoCandidates is a diagnostic: run RunDet on one fixture with
// DLA_DUMP_CANDIDATES set so dbPostProcess writes /tmp/go_candidates.json
// (post-geometry, pre-score-filter quads + scores) for offline analysis of
// Go/cv2 det divergence. Not a regression test.
func TestDumpGoCandidates(t *testing.T) {
	skipIfNoModels(t)
	fixture := os.Getenv("FIXTURE")
	if fixture == "" {
		fixture = "mp_cn_sm_p0"
	}
	t.Setenv("DLA_DUMP_CANDIDATES", "1")
	imgPath := filepath.Join("testdata", fixture+".png")
	img, err := Decode(imgPath)
	if err != nil {
		t.Fatalf("decode %s: %v", imgPath, err)
	}
	if _, err := RunDet(t.Context(), os.Getenv("MODEL_DIR"), img); err != nil {
		t.Fatalf("RunDet: %v", err)
	}
	t.Logf("wrote /tmp/go_candidates.json for %s", fixture)
}

// TestDumpStages is a diagnostic: run RunDet on one fixture (FIXTURE env) with
// the stage-dump env vars set, writing the Go-side intermediates that the
// Python oracle (cmp_stages.py) is diffed against:
//
//	/tmp/go_pred.json       — raw pred map (post-sigmoid, pre-threshold)
//	/tmp/go_quads_pre.json  — pre-unclip min-area rect per component (resized)
//	/tmp/go_candidates.json — post-geometry, pre-score-filter quad+score
//
// Not a regression test. Pair with: python cmp_stages.py <img> <model_dir>;
// then python diff_stages.py for the per-stage comparison.
func TestDumpStages(t *testing.T) {
	skipIfNoModels(t)
	fixture := os.Getenv("FIXTURE")
	if fixture == "" {
		fixture = "mp_cn_sm_p0"
	}
	t.Setenv("DLA_DUMP_STAGES", "1")
	t.Setenv("DLA_DUMP_QUADS", "1")
	t.Setenv("DLA_DUMP_CANDIDATES", "1")
	imgPath := filepath.Join("testdata", fixture+".png")
	img, err := Decode(imgPath)
	if err != nil {
		t.Fatalf("decode %s: %v", imgPath, err)
	}
	res, err := RunDet(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunDet: %v", err)
	}
	// Final boxes (post score-filter + filter_tag_det_res), for direct IoU
	// comparison against the Python oracle's final output (ref_det.py).
	fb := make([]map[string]any, 0, len(res.Boxes))
	for _, b := range res.Boxes {
		pts := make([][2]float64, 4)
		for i := range b.Pts {
			pts[i] = [2]float64{float64(b.Pts[i][0]), float64(b.Pts[i][1])}
		}
		fb = append(fb, map[string]any{"pts": pts, "score": float64(b.Score)})
	}
	if b, e := json.Marshal(map[string]any{"boxes": fb}); e == nil {
		_ = os.WriteFile("/tmp/go_final.json", b, 0o644)
	}
	t.Logf("wrote /tmp/go_pred.json, /tmp/go_quads_pre.json, /tmp/go_candidates.json, /tmp/go_final.json for %s", fixture)
}

// TestEquivalenceReport runs every native recognizer against the Python
// reference goldens and prints one consolidated equivalence summary to the test
// log (visible in CI). It is the human-readable counterpart to the per-task
// integration tests: with a single command it shows that Go's det / DLA / TSR /
// OCR outputs match the Python deepdoc inference service to within the
// documented, bounded floors. It also acts as a light regression guard (DLA /
// TSR boxes and OCR text must match exactly; the full-fixture det floor is
// guarded separately by TestDetMembershipAllFixtures).
//
// Requires MODEL_DIR (//go:build integration).
func TestEquivalenceReport(t *testing.T) {
	skipIfNoModels(t)
	md := os.Getenv("MODEL_DIR")
	// P0: refuse to produce an equivalence report against a drifted model
	// snapshot — the goldens would be meaningless.
	checkModelSnapshotHash(t, md)
	dir := "testdata"

	type row struct {
		task  string
		fix   int
		match int
		total int
		maxD  float64
		note  string
	}
	var rows []row

	// DLA — layout detection: every golden box matched within 2px.
	{
		m, tot, mx := 0, 0, 0.0
		for _, stem := range dlaPages {
			img, err := Decode(filepath.Join(dir, stem+".png"))
			if err != nil {
				t.Fatalf("decode %s: %v", stem, err)
			}
			res, err := RunDLA(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunDLA %s: %v", stem, err)
			}
			var got struct {
				Boxes [][]float64 `json:"bboxes"`
			}
			if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
				t.Fatalf("wire %s: %v", stem, err)
			}
			gold := LoadGoldenBoxes(t, filepath.Join(dir, stem+".dla.golden.json"))
			mm, mx2, _ := MatchBoxesRelaxed(t, gold, got.Boxes, CmpTolCoord, CmpTolScore)
			m += mm
			tot += len(gold)
			mx = math.Max(mx, mx2)
		}
		rows = append(rows, row{"DLA (layout)", len(dlaPages), m, tot, mx, "sub-pixel"})
	}

	// TSR — table structure: every golden box matched within 2px.
	{
		m, tot, mx := 0, 0, 0.0
		for _, stem := range tsrPages {
			img, err := Decode(filepath.Join(dir, stem+".png"))
			if err != nil {
				t.Fatalf("decode %s: %v", stem, err)
			}
			res, err := RunTSR(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunTSR %s: %v", stem, err)
			}
			var got struct {
				Boxes [][]float64 `json:"bboxes"`
			}
			if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
				t.Fatalf("wire %s: %v", stem, err)
			}
			gold := LoadGoldenBoxes(t, filepath.Join(dir, stem+".tsr.golden.json"))
			mm, mx2, _ := MatchBoxesRelaxed(t, gold, got.Boxes, CmpTolCoord, CmpTolScore)
			m += mm
			tot += len(gold)
			mx = math.Max(mx, mx2)
		}
		rows = append(rows, row{"TSR (table)", len(tsrPages), m, tot, mx, "<1px (<=10px @4:1 aspect)"})
	}

	// OCR — text recognition: exact text match.
	{
		m, tot := 0, 0
		for _, stem := range ocrRecLines {
			img, err := Decode(filepath.Join(dir, stem+".png"))
			if err != nil {
				t.Fatalf("decode %s: %v", stem, err)
			}
			res, err := RunOCRRec(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunOCRRec %s: %v", stem, err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, stem+".ocr_rec.golden.json"))
			if err != nil {
				t.Fatalf("read golden %s: %v", stem, err)
			}
			var gold struct {
				Output [][][][]any `json:"output"`
			}
			if err := json.Unmarshal(raw, &gold); err != nil {
				t.Fatalf("parse golden %s: %v", stem, err)
			}
			gotText := res.Text
			goldText := gold.Output[0][0][0][0].(string)
			tot++
			if gotText == goldText {
				m++
			} else {
				t.Logf("OCR mismatch %s: got %q gold %q", stem, gotText, goldText)
			}
		}
		rows = append(rows, row{"OCR (text)", len(ocrRecLines), m, tot, 0, "exact text"})
	}

	// Det — text detection IoU box-membership floor (curated dense subset).
	{
		const iouThr = 0.5
		detStems := []string{"mp_arxiv_p0", "mp_cn_sm_p0", "mp_en_dense_p0", "mp_sec_p0", "mp_physics_p5"}
		orphanG, orphanGo, tot := 0, 0, 0
		for _, stem := range detStems {
			img, err := Decode(filepath.Join(dir, stem+".png"))
			if err != nil {
				t.Fatalf("decode %s: %v", stem, err)
			}
			res, err := RunDet(t.Context(), md, img)
			if err != nil {
				t.Fatalf("RunDet %s: %v", stem, err)
			}
			var got struct {
				Output [][][][][2]float64 `json:"output"`
			}
			if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
				t.Fatalf("wire %s: %v", stem, err)
			}
			goBoxes := FlattenQuads(got.Output)
			raw, err := os.ReadFile(filepath.Join(dir, stem+".det.golden.json"))
			if err != nil {
				t.Fatalf("read golden %s: %v", stem, err)
			}
			var gold struct {
				Output [][][][][2]float64 `json:"output"`
			}
			if err := json.Unmarshal(raw, &gold); err != nil {
				t.Fatalf("parse golden %s: %v", stem, err)
			}
			goldBoxes := FlattenQuads(gold.Output)
			imG, imGo := MatchIoUBothDirections(goldBoxes, goBoxes, iouThr)
			orphanG += len(goldBoxes) - imG
			orphanGo += len(goBoxes) - imGo
			tot += len(goldBoxes)
		}
		rows = append(rows, row{"Det (text boxes)", len(detStems), tot - orphanG, tot, 0,
			fmt.Sprintf("IoU orphan(gold/go)=%d/%d (accepted floor 3/5)", orphanG, orphanGo)})
	}

	// Consolidated summary — visible in CI logs.
	t.Logf("===== DLA-NATIVE vs PYTHON DEEP DOC: EQUIVALENCE SUMMARY =====")
	t.Logf("%-18s %4s %9s %10s  %s", "TASK", "FIX", "MATCH/TOT", "MAXΔ(px)", "NOTE")
	for _, r := range rows {
		t.Logf("%-18s %4d %4d/%-4d %10.3f  %s", r.task, r.fix, r.match, r.total, r.maxD, r.note)
	}
	t.Logf("==================================================================")

	// Light regression guard: DLA/TSR boxes and OCR text must match exactly.
	// The full-fixture det floor is guarded by TestDetMembershipAllFixtures.
	for _, r := range rows {
		if r.task == "DLA (layout)" || r.task == "TSR (table)" || r.task == "OCR (text)" {
			if r.match != r.total {
				t.Errorf("%s: %d/%d matched — equivalence REGRESSION", r.task, r.match, r.total)
			}
		}
	}
}

// compareBBoxesJSON compares a server {"bboxes":[...]} response against the Go
// Wire() {"bboxes":[...]} output. The server response is the reference ("gold");
// Go is "got". Matching is class-aware with the documented coordinate/score
// tolerances.
func compareBBoxesJSON(t *testing.T, serverJSON, goJSON []byte, kind string, scoreTol float64) {
	t.Helper()
	var s struct {
		Bboxes [][]float64 `json:"bboxes"`
	}
	if err := json.Unmarshal(serverJSON, &s); err != nil {
		t.Fatalf("parse server %s json: %v", kind, err)
	}
	var g struct {
		Bboxes [][]float64 `json:"bboxes"`
	}
	if err := json.Unmarshal(goJSON, &g); err != nil {
		t.Fatalf("parse go %s json: %v", kind, err)
	}
	matched, maxd, _ := MatchBoxesRelaxed(t, s.Bboxes, g.Bboxes, CmpTolCoord, scoreTol)
	t.Logf("%s: server=%d go=%d matched=%d maxd=%.3f px", kind, len(s.Bboxes), len(g.Bboxes), matched, maxd)
	if matched != len(s.Bboxes) {
		t.Errorf("%s: %d/%d server boxes matched by Go (missing %d)", kind, matched, len(s.Bboxes), len(s.Bboxes)-matched)
	}
}

// compareDetJSON compares a server det {"output":[[quads]]} response against the
// Go Wire() det output. The only accepted divergence between Go and the live
// cv2-backed server is the documented Det 3/5 contour-tracer floor (+slack);
// anything larger is a real Go-vs-service regression.
func compareDetJSON(t *testing.T, serverJSON, goJSON []byte) {
	t.Helper()
	srvBoxes := flattenOutput(t, serverJSON, "det")
	goBoxes := flattenOutput(t, goJSON, "det")
	imG, imGo := MatchIoUBothDirections(srvBoxes, goBoxes, 0.5)
	orphanG := len(srvBoxes) - imG
	orphanGo := len(goBoxes) - imGo
	t.Logf("det: server=%d go=%d | IoU orphan(g/g)=%d/%d", len(srvBoxes), len(goBoxes), orphanG, orphanGo)
	const (
		baselineG  = 3
		baselineGo = 5
		slack      = 3
	)
	if orphanG > baselineG+slack {
		t.Errorf("det: %d/%d server boxes unmatched under IoU (> baseline %d+%d)", imG, len(srvBoxes), baselineG, slack)
	}
	if orphanGo > baselineGo+slack {
		t.Errorf("det: Go produced %d extra boxes under IoU (> baseline %d+%d)", orphanGo, baselineGo, slack)
	}
}

// compareRecJSON compares a server rec {"output":[[[text,1.0]]]} response against
// the Go Wire() rec output, exactly (text must be identical).
func compareRecJSON(t *testing.T, serverJSON, goJSON []byte) {
	t.Helper()
	srvTexts := flattenTexts(t, serverJSON)
	goTexts := flattenTexts(t, goJSON)
	if len(srvTexts) != len(goTexts) {
		t.Errorf("rec: count mismatch server=%d go=%d", len(srvTexts), len(goTexts))
	}
	n := len(srvTexts)
	if len(goTexts) < n {
		n = len(goTexts)
	}
	for i := 0; i < n; i++ {
		if srvTexts[i] != goTexts[i] {
			t.Errorf("rec[%d]: server %q != go %q", i, srvTexts[i], goTexts[i])
		}
	}
}

// flattenOutput extracts the quad list from a det Wire()/server payload
// (boxes live under output[0][0]).
func flattenOutput(t *testing.T, raw []byte, kind string) [][][2]float64 {
	t.Helper()
	var v struct {
		Output [][][][][2]float64 `json:"output"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse %s json: %v", kind, err)
	}
	return FlattenQuads(v.Output)
}

// flattenTexts extracts the recognized strings from a rec Wire()/server payload
// (pairs live under output[0][0], text is pair[0]).
func flattenTexts(t *testing.T, raw []byte) []string {
	t.Helper()
	var v struct {
		Output [][][]any `json:"output"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse rec json: %v", err)
	}
	if len(v.Output) == 0 || len(v.Output[0]) == 0 {
		return nil
	}
	var out []string
	for _, item := range v.Output[0][0] {
		pair, ok := item.([]any)
		if !ok || len(pair) < 1 {
			continue
		}
		if s, ok := pair[0].(string); ok {
			out = append(out, s)
		}
	}
	return out
}
