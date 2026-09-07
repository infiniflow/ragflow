//go:build cgo

package native

// ocr_rec.go — OCR text recognition (PP-OCRv4 CTC) recognizer.
//
// Ports deepdoc/vision/ocr.py TextRecognizer.resize_norm_img and
// deepdoc/vision/postprocess.py CTCLabelDecode, emitting the wire format from
// deepdoc/server/adapters/ocr_adapter.py (recognize mode).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	ort "github.com/infiniflow/onnxruntime_go"
)

const (
	recH        = 48
	recW        = 320
	recSeqLen   = 40
	recVocab    = 6625
	recMaxBatch = 1
)

// OCRRecResult is the recognized text for one cropped line.
type OCRRecResult struct {
	Text  string
	Score float32
}

// RunOCRRec recognizes a single cropped text-line image. It is equivalent to a
// one-line batch (see RunOCRRecBatchReal): the line is resized against its own
// wh_ratio floored at 320/48, never the batch-max, so a caller recognizing
// lines independently gets the same result as a standalone Python
// TextRecognizer call.
func RunOCRRec(ctx context.Context, modelDir string, img *Image) (OCRRecResult, error) {
	chars, err := loadCharDict(filepath.Join(modelDir, "ocr.res"))
	if err != nil {
		return OCRRecResult{}, err
	}
	// A single image is its own batch: max_wh_ratio floors at recW/recH (matching
	// TextRecognizer.__call__'s init) but rises to the line's own ratio when
	// wider, so wide lines are NOT clamped back to 320.
	maxWhRatio := float64(recW) / float64(recH)
	if r := float64(img.W) / float64(img.H); r > maxWhRatio {
		maxWhRatio = r
	}
	return recognizeLine(ctx, modelDir, img, maxWhRatio, chars)
}

// RunOCRRecBatchReal recognizes a batch of cropped text-line images with a
// SINGLE ONNX Run, mirroring deepdoc's TextRecognizer.__call__: each line is
// resized to its own proportional width (recH * that line's wh_ratio), capped
// by the batch-shared imgW (imgW = recH * max_wh_ratio, with max_wh_ratio
// floored at 320/48), and zero-padded on the right out to imgW; all blobs are
// concatenated into one {N,3,48,imgW} tensor, and the model runs once. The
// output is split back into per-line sequences and CTC-decoded in order, so
// the result is numerically identical to calling RunOCRRec on each line (each
// line sees the same shared batch width), but amortized over one forward pass
// instead of N.
//
// The shared batch width means a line is resized against the batch max wh_ratio,
// not its own — exactly what deepdoc does inside a batch. A standalone call to
// RunOCRRec (maxWhRatio floored at the line's own ratio when wider) is the
// correct single-line equivalent and remains the unit of "one crop" inference.
func RunOCRRecBatchReal(ctx context.Context, modelDir string, imgs []*Image) ([]OCRRecResult, error) {
	n := len(imgs)
	if n == 0 {
		return nil, nil
	}
	if n == 1 {
		// Degenerate batch: fall back to the single-line path so callers get
		// the exact same result as RunOCRRec (no batch-width widening).
		res, err := RunOCRRec(ctx, modelDir, imgs[0])
		if err != nil {
			return nil, err
		}
		return []OCRRecResult{res}, nil
	}
	chars, err := loadCharDict(filepath.Join(modelDir, "ocr.res"))
	if err != nil {
		return nil, err
	}
	maxWhRatio := float64(recW) / float64(recH)
	for _, img := range imgs {
		if r := float64(img.W) / float64(img.H); r > maxWhRatio {
			maxWhRatio = r
		}
	}
	imgW := int(math.Floor(recH * maxWhRatio))
	// Per-line resized content width (<= imgW), used to place each line's
	// preprocessed blob into the shared concatenated tensor.
	resizedWs := make([]int, n)
	blobs := make([][]float32, n)
	for i, img := range imgs {
		resizedW := int(math.Ceil(recH * (float64(img.W) / float64(img.H))))
		if resizedW > imgW {
			resizedW = imgW
		}
		resizedWs[i] = resizedW
		blobs[i] = ocrRecPreprocess(img, resizedW, imgW)
	}
	// Concatenate: layout [N, 3, 48, imgW] with each line's blob at
	// offset i*3*recH*imgW. ocrRecPreprocess already zero-fills to imgW, so a
	// plain copy places it correctly at the line's N-slot.
	batch := make([]float32, n*3*recH*imgW)
	lineStride := 3 * recH * imgW
	for i, b := range blobs {
		copy(batch[i*lineStride:(i+1)*lineStride], b)
	}

	// 0 → all cores, matching deepdoc's Python onnxruntime for bit-stable
	// parity (no contour extraction in the OCR-rec Run path).
	sess, release, err := getRecSession(filepath.Join(modelDir, "rec.onnx"), "x",
		[]int64{int64(n), 3, recH, int64(imgW)}, "softmax_11.tmp_0", 0)
	if err != nil {
		return nil, err
	}
	defer release()

	out, err := sess.Run(ctx, batch)
	if err != nil {
		return nil, err
	}
	// Output layout: [N, seqLen, recVocab]; seqLen is dynamic (scales with
	// imgW), so derive it from the tensor length.
	seqLen := len(out) / (n * recVocab)
	if seqLen <= 0 {
		return nil, fmt.Errorf("recSession: unexpected batch output len %d for n=%d vocab=%d", len(out), n, recVocab)
	}
	results := make([]OCRRecResult, n)
	for i := 0; i < n; i++ {
		line := out[i*seqLen*recVocab : (i+1)*seqLen*recVocab]
		results[i] = ocrRecCTCDecode(line, chars)
	}
	return results, nil
}

// recognizeLine runs the resize + session + CTC decode for one line at the
// given batch max wh_ratio, mirroring deepdoc TextRecognizer.resize_norm_img
// exactly: the tensor width is imgW = int(48 * max_wh_ratio) (floored at
// 320/48 for narrow batches); the content is resized to resized_w =
// min(ceil(48*ratio), imgW) and zero-padded on the right to imgW. Feeding the
// unpadded own-width (no floor, no pad) — the naive resize — changes
// recognition for narrow lines because the model sees a different width than
// deepdoc.
func recognizeLine(ctx context.Context, modelDir string, img *Image, maxWhRatio float64, chars []string) (OCRRecResult, error) {
	ratio := float64(img.W) / float64(img.H)
	imgW := int(math.Floor(recH * maxWhRatio))
	resizedW := int(math.Ceil(recH * ratio))
	if resizedW > imgW {
		resizedW = imgW
	}
	blob := ocrRecPreprocess(img, resizedW, imgW)
	// 0 → all cores, matching deepdoc's Python onnxruntime for bit-stable
	// parity (no contour extraction in the OCR-rec Run path).
	sess, release, err := getRecSession(filepath.Join(modelDir, "rec.onnx"), "x",
		[]int64{recMaxBatch, 3, recH, int64(imgW)}, "softmax_11.tmp_0", 0)
	if err != nil {
		return OCRRecResult{}, err
	}
	defer release()

	out, err := sess.Run(ctx, blob)
	if err != nil {
		return OCRRecResult{}, err
	}
	return ocrRecCTCDecode(out, chars), nil
}

// ocrRecPreprocess builds the CHW float blob (/255, standardized) for a
// text-line image resized to (resizedW, recH) and zero-padded on the right to
// the full tensor width imgW. The session runs at imgW; padding mirrors
// deepdoc's resize_norm_img (padding_im[:, :, 0:resized_w] = resized_image).
func ocrRecPreprocess(img *Image, resizedW, imgW int) []float32 {
	bgr := img.ToBGR()
	w, h := img.W, img.H
	resized := bilinearResize(bgr, w, h, resizedW, recH)
	blob := make([]float32, 3*recH*imgW) // zero-filled (padded right)
	for y := 0; y < recH; y++ {
		for x := 0; x < resizedW; x++ {
			for c := 0; c < 3; c++ {
				v := float32(resized[(y*resizedW+x)*3+c]) / 255.0
				v = (v - 0.5) / 0.5
				blob[c*recH*imgW+y*imgW+x] = v
			}
		}
	}
	return blob
}

// charDictCache memoises loadCharDict by the ocr.res path. RunOCRRec is called
// once per cropped text line, so without caching every line would re-read and
// re-parse the same vocabulary file from disk. The decoded slice is only ever
// read (by ocrRecCTCDecode), never mutated, so sharing it across goroutines is
// safe.
var charDictCache sync.Map // map[string][]string, keyed by ocr.res path

// loadCharDict returns the decode vocabulary: ["blank"] + <ocr.res lines> + " ".
func loadCharDict(path string) ([]string, error) {
	if v, ok := charDictCache.Load(path); ok {
		return v.([]string), nil
	}
	chars, err := readCharDict(path)
	if err != nil {
		return nil, err
	}
	charDictCache.Store(path, chars)
	return chars, nil
}

func readCharDict(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	chars := make([]string, 0, len(lines)+2)
	chars = append(chars, "blank")
	chars = append(chars, lines...)
	chars = append(chars, " ") // use_space_char
	return chars, nil
}

func ocrRecCTCDecode(out []float32, chars []string) OCRRecResult {
	// out layout: [recMaxBatch, seqLen, recVocab]; take batch 0. The sequence
	// length is dynamic (scales with the input width), so derive it from the
	// tensor length rather than a fixed constant.
	seqLen := len(out) / recVocab
	var text strings.Builder
	var probs []float32
	prev := -1
	var meanAcc float32
	for t := 0; t < seqLen; t++ {
		base := t * recVocab
		bestIdx, bestProb := 0, float32(-1e9)
		for v := 0; v < recVocab; v++ {
			if out[base+v] > bestProb {
				bestProb = out[base+v]
				bestIdx = v
			}
		}
		if bestIdx == 0 { // blank
			prev = 0
			continue
		}
		if bestIdx != prev {
			if bestIdx < len(chars) {
				text.WriteString(chars[bestIdx])
				probs = append(probs, bestProb)
			}
		}
		prev = bestIdx
	}
	for _, p := range probs {
		meanAcc += p
	}
	// Match deepdoc/vision/postprocess.py CTCLabelDecode: an empty decode
	// (no characters recognized) yields confidence 0.0, not 1.0. The old 1.0
	// sentinel made an unreadable/blank orientation outscore a real reading,
	// which corrupted rotation selection and any max-confidence picker.
	score := float32(0.0)
	if len(probs) > 0 {
		score = meanAcc / float32(len(probs))
	}
	return OCRRecResult{Text: text.String(), Score: round4(score)}
}

// Wire emits the Go DocAnalyzer OCR-rec format: {"output": [[[text, score]]]}.
func (r OCRRecResult) Wire() string {
	// Emit the real recognition confidence (mean per-char softmax prob from
	// ocrRecCTCDecode) so the wire schema matches ocr.py's
	// recognize_batch_with_score, 4-level nesting.
	pair := []any{r.Text, r.Score}
	arr1 := []any{pair}
	arr2 := []any{arr1}
	arr3 := []any{arr2}
	out, _ := json.Marshal(map[string]any{"output": arr3})
	return string(out)
}

// recSession runs rec.onnx, whose output sequence length is dynamic: it scales
// with the input width (≈ width/8), so a fixed-shape AdvancedSession cannot be
// pre-sized per width and even a width-matched session would still emit a
// varying seq length. Instead we use a DynamicAdvancedSession and pass a nil
// output on every Run: onnxruntime allocates the correctly-shaped output
// tensor, which we copy out before destroying it. The input tensor is
// fixed-shape per (model, width), so one recSession is reused per width.
type recSession struct {
	inName   string
	outName  string
	sess     *ort.DynamicAdvancedSession
	in       *ort.Tensor[float32]
	poisoned bool
}

func newRecSession(modelPath, inName string, inShape []int64, outName string, intraOpThreads int) (*recSession, error) {
	in := make([]float32, prod(inShape))
	inT, err := ort.NewTensor(ort.NewShape(inShape...), in)
	if err != nil {
		return nil, err
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		inT.Destroy()
		return nil, err
	}
	// 0 → all cores (mirrors Python's onnxruntime default); OCR-rec does no
	// contour extraction in the Run path, so parallelism is safe and matches
	// deepdoc's reduction order for bit-stable parity.
	if err := opts.SetIntraOpNumThreads(intraOpThreads); err != nil {
		opts.Destroy()
		inT.Destroy()
		return nil, err
	}
	sess, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{inName}, []string{outName}, opts)
	if err != nil {
		opts.Destroy()
		inT.Destroy()
		return nil, err
	}
	return &recSession{inName: inName, outName: outName, sess: sess, in: inT}, nil
}

// Run copies input into the input tensor, executes with an auto-allocated
// (dynamic) output, and returns the output data. The allocated output tensor
// is destroyed before returning; out is a fresh copy the caller owns.
func (s *recSession) Run(ctx context.Context, input []float32) ([]float32, error) {
	if len(input) != len(s.in.GetData()) {
		return nil, fmt.Errorf("recSession %s: input len %d != tensor len %d",
			s.outName, len(input), len(s.in.GetData()))
	}
	opts, err := ort.NewRunOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	// Cancel an in-flight Run when the context is done. done closes once Run
	// returns so the watcher exits even on the success path.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = opts.Terminate()
		case <-done:
		}
	}()

	copy(s.in.GetData(), input)
	// nil output → onnxruntime allocates the actual-shaped tensor.
	outputs := []ort.Value{nil}
	if err := s.sess.RunWithOptions([]ort.Value{s.in}, outputs, opts); err != nil {
		if ctx.Err() != nil {
			s.poisoned = true
		}
		return nil, err
	}
	outVal := outputs[0]
	if outVal == nil {
		return nil, fmt.Errorf("recSession %s: nil output tensor", s.outName)
	}
	defer outVal.Destroy()
	t, ok := outVal.(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("recSession %s: unexpected output type %T", s.outName, outVal)
	}
	data := t.GetData()
	out := make([]float32, len(data))
	copy(out, data)
	return out, nil
}

// Destroy releases the dynamic session and input tensor.
func (s *recSession) Destroy() {
	if s.sess != nil {
		s.sess.Destroy()
	}
	if s.in != nil {
		s.in.Destroy()
	}
}

func (s *recSession) isPoisoned() bool { return s.poisoned }

func (s *recSession) markPoisoned() { s.poisoned = true }
