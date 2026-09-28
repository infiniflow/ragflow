//go:build cgo

// Package infnative provides an in-process DeepDoc DocAnalyzer backend.
//
// It wraps the ONNX Runtime inference library in the standalone native
// module (import path native) so the PDF parser can run DLA/TSR/OCR
// locally on CPU, with no Python service in the loop. It is the SOLE
// production DeepDoc backend: the external Python HTTP service (DEEPDOC_URL)
// has been removed entirely from both the production path and the test
// suite, so all DeepDoc regression tests now exercise this in-process
// backend directly.
//
// The package imports onnxruntime_go (cgo), so it is build-tag gated
// (cgo) and only the server binary built with that tag opts into it.
// The parser package itself stays free of the onnxruntime dependency for its
// unit-test build path.
package infnative

import (
	"context"
	"fmt"
	"image"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/native"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// registeredModelDir is the model directory recorded by Register, used by
// Serving for startup diagnostics.
var registeredModelDir string

// NativeAnalyzer runs DeepDoc vision inference in-process. It satisfies
// doctype.DocAnalyzer, so the PDF parser consumes it through the exact same
// interface as the HTTP-backed Client.
// DefaultDropScore mirrors deepdoc/vision/ocr.py's Recognizer.drop_score
// (0.5). OCRRecognize blanks text whose score is below this threshold while
// preserving the real score, so the in-process backend honours the exact same
// text-blanking contract as the Python inference service.
const DefaultDropScore = 0.5

type NativeAnalyzer struct {
	modelDir  string
	dropScore float64
}

var _ deepdoctype.DocAnalyzer = (*NativeAnalyzer)(nil)

// NewAnalyzer builds a NativeAnalyzer after verifying ONNX Runtime is
// initialized and every required model file exists. It returns an error when
// the in-process backend cannot serve, letting the caller (the registration
// factory) fall back to the empty analyzer instead of panicking on an
// uninitialized ONNX environment. dropScore is the confidence threshold below
// which recognized text is blanked (see DefaultDropScore and the Python
// service contract).
func NewAnalyzer(modelDir string, dropScore float64) (*NativeAnalyzer, error) {
	if !native.Initialized() {
		return nil, fmt.Errorf("deepdoc native: onnxruntime not initialized")
	}
	if !common.HasModelFiles(modelDir) {
		return nil, fmt.Errorf("deepdoc native: missing required model files in %s", modelDir)
	}
	return &NativeAnalyzer{modelDir: modelDir, dropScore: dropScore}, nil
}

// Register wires this backend into the parser as the local in-process
// backend. Call it once at process start (the server binary) after resolving
// modelDir/dropScore. This fork links ONNX Runtime
// statically (libonnxruntime.a), so native.InitORT always resolves ORT from
// the running binary itself via dlopen(NULL) — no external libonnxruntime.so
// is required. InitORT is a sync.Once, so re-entry (e.g. tests calling it
// directly) is a no-op. dropScore is the confidence threshold used by
// OCRRecognize to blank low-confidence text, mirroring the Python service's
// Recognizer.drop_score. The factory returns false when the backend cannot
// serve, so the parser degrades to the empty analyzer rather than crashing.
func Register(modelDir string, dropScore float64) error {
	registeredModelDir = modelDir
	if err := native.InitORT(); err != nil {
		return fmt.Errorf("deepdoc native: init onnxruntime: %w", err)
	}
	deepdoctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		a, err := NewAnalyzer(modelDir, dropScore)
		if err != nil {
			return nil, false
		}
		return a, true
	})
	return nil
}

// canServe reports whether the backend can serve from modelDir: ONNX Runtime
// is initialized and every required model file is present. Serving and
// NativeAnalyzer.Health share this exact check; they differ only in which
// model directory they probe (the process-registered one vs the instance's).
func canServe(modelDir string) bool {
	if !native.Initialized() {
		return false
	}
	return common.HasModelFiles(modelDir)
}

// Serving reports whether the backend can currently serve from the
// process-registered model directory. Used for startup logging only; the
// parser's factory already gates on the same check via canServe.
func Serving() bool {
	return canServe(registeredModelDir)
}

// DLA runs layout detection on a page image.
func (a *NativeAnalyzer) DLA(ctx context.Context, img image.Image) ([]deepdoctype.DLARegion, error) {
	ni, err := native.FromImage(img)
	if err != nil {
		return nil, err
	}
	res, err := native.RunDLA(ctx, a.modelDir, ni)
	if err != nil {
		return nil, err
	}
	labels := deepdoctype.DefaultDLALabels()
	out := make([]deepdoctype.DLARegion, 0, len(res.Boxes))
	for _, b := range res.Boxes {
		label := ""
		if b.Class >= 0 && int(b.Class) < len(labels) {
			label = labels[b.Class]
		}
		out = append(out, deepdoctype.DLARegion{
			X0: float64(b.X0), Y0: float64(b.Y0),
			X1: float64(b.X1), Y1: float64(b.Y1),
			Label:      label,
			Confidence: float64(b.Score),
		})
	}
	return out, nil
}

// TSR recognises table structure from a cropped image.
func (a *NativeAnalyzer) TSR(ctx context.Context, img image.Image) ([]deepdoctype.TSRCell, error) {
	ni, err := native.FromImage(img)
	if err != nil {
		return nil, err
	}
	res, err := native.RunTSR(ctx, a.modelDir, ni)
	if err != nil {
		return nil, err
	}
	out := make([]deepdoctype.TSRCell, 0, len(res.Boxes))
	for _, b := range res.Boxes {
		out = append(out, deepdoctype.TSRCell{
			X0: float64(b.X0), Y0: float64(b.Top),
			X1: float64(b.X1), Y1: float64(b.Bottom),
			Label: b.Label,
		})
	}
	return out, nil
}

// OCRDetect detects text regions (quad boxes) in a cropped image.
func (a *NativeAnalyzer) OCRDetect(ctx context.Context, img image.Image) ([]deepdoctype.OCRBox, error) {
	ni, err := native.FromImage(img)
	if err != nil {
		return nil, err
	}
	res, err := native.RunDet(ctx, a.modelDir, ni)
	if err != nil {
		return nil, err
	}
	out := make([]deepdoctype.OCRBox, 0, len(res.Boxes))
	for _, b := range res.Boxes {
		out = append(out, deepdoctype.OCRBox{
			X0: float64(b.Pts[0][0]), Y0: float64(b.Pts[0][1]),
			X1: float64(b.Pts[1][0]), Y1: float64(b.Pts[1][1]),
			X2: float64(b.Pts[2][0]), Y2: float64(b.Pts[2][1]),
			X3: float64(b.Pts[3][0]), Y3: float64(b.Pts[3][1]),
		})
	}
	return out, nil
}

// OCRRecognize recognizes text in a cropped image region.
func (a *NativeAnalyzer) OCRRecognize(ctx context.Context, img image.Image) ([]deepdoctype.OCRText, error) {
	ni, err := native.FromImage(img)
	if err != nil {
		return nil, err
	}
	res, err := native.RunOCRRec(ctx, a.modelDir, ni)
	if err != nil {
		return nil, err
	}
	// Mirror the Python inference service contract: blank text whose score is
	// below drop_score but preserve the real confidence, so callers consume an
	// identical OCRText regardless of which backend produced it.
	if float64(res.Score) < a.dropScore {
		return []deepdoctype.OCRText{{Text: "", Confidence: float64(res.Score)}}, nil
	}
	return []deepdoctype.OCRText{{Text: res.Text, Confidence: float64(res.Score)}}, nil
}

// OCRRecognizeBatch recognizes text in a batch of cropped image regions with
// a SINGLE ONNX Run, mirroring deepdoc's TextRecognizer.__call__ over a page's
// lines. It is the batched analogue of OCRRecognize: the recognizer under the
// hood concatenates every crop's preprocessed blob into one {N,3,48,imgW}
// tensor and runs the model once, which is numerically identical to calling
// OCRRecognize per crop (each line sees the same shared batch width) but
// amortizes N forward passes into one. The drop_score contract is applied
// per line, so callers consume an identical []OCRText per crop regardless of
// whether batching was used.
//
// A degenerate batch (len(imgs) <= 1) falls back to the single-crop path so
// callers get the exact same result as OCRRecognize (no batch-width widening).
// This is the production fast path the caller opts into by implementing
// batchRecognizer — strictly more efficient than N sequential OCRRecognize
// calls and numerically identical.
func (a *NativeAnalyzer) OCRRecognizeBatch(ctx context.Context, imgs []image.Image) ([][]deepdoctype.OCRText, error) {
	n := len(imgs)
	if n == 0 {
		return nil, nil
	}
	if n == 1 {
		res, err := a.OCRRecognize(ctx, imgs[0])
		if err != nil {
			return nil, err
		}
		return [][]deepdoctype.OCRText{res}, nil
	}
	nis, err := native.FromImages(imgs)
	if err != nil {
		return nil, err
	}
	recs, err := native.RunOCRRecBatchReal(ctx, a.modelDir, nis)
	if err != nil {
		return nil, err
	}
	out := make([][]deepdoctype.OCRText, n)
	for i, r := range recs {
		if float64(r.Score) < a.dropScore {
			out[i] = []deepdoctype.OCRText{{Text: "", Confidence: float64(r.Score)}}
			continue
		}
		out[i] = []deepdoctype.OCRText{{Text: r.Text, Confidence: float64(r.Score)}}
	}
	return out, nil
}

// Health reports whether the backend can serve from this analyzer's model
// directory: ONNX Runtime is initialized and every required model file is
// present. It delegates to canServe.
func (a *NativeAnalyzer) Health() bool {
	return canServe(a.modelDir)
}
