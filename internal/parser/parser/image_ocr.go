//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
)

const (
	// MaxImagePayloadBytes bounds compressed image data accepted by OCR and VLM.
	MaxImagePayloadBytes = 32 << 20
	// MaxImagePixels bounds decoded image dimensions before allocating a raster.
	MaxImagePixels = 40_000_000
	// MaxImageEdge bounds either decoded image dimension before raster allocation.
	MaxImageEdge = 12_000

	imageOCRTotalBudget = 60 * time.Second
	imageOCRItemBudget  = 10 * time.Second
)

var errImageOCRBudgetExpired = errors.New("local OCR document budget expired")

type imageOCRBudget struct {
	parent     context.Context
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	failures   int
	firstError string
}

func newImageOCRBudget(ctx context.Context) *imageOCRBudget {
	if ctx == nil {
		ctx = context.Background()
	}
	budgetCtx, cancel := context.WithTimeout(ctx, imageOCRTotalBudget)
	return &imageOCRBudget{parent: ctx, ctx: budgetCtx, cancel: cancel}
}

func (b *imageOCRBudget) close() {
	if b != nil {
		b.cancel()
	}
}

func (b *imageOCRBudget) recognize(data []byte) string {
	if b == nil {
		return ""
	}
	if len(data) == 0 || len(data) > MaxImagePayloadBytes {
		b.recordFailure(fmt.Errorf("local OCR: image payload size %d is outside limits", len(data)))
		return ""
	}
	text, err := b.recognizeWithReader(func() io.Reader { return bytes.NewReader(data) })
	if err != nil {
		b.recordFailure(err)
	}
	return text
}

func (b *imageOCRBudget) recognizeBase64(payload string) string {
	if b == nil {
		return ""
	}
	if err := b.parent.Err(); err != nil {
		b.recordFailure(err)
		return ""
	}
	if b.ctx.Err() != nil {
		b.recordFailure(errImageOCRBudgetExpired)
		return ""
	}
	encoded, err := visionBase64Payload(payload)
	if err != nil {
		b.recordFailure(err)
		return ""
	}
	encodedBytes := 0
	for i := 0; i < len(encoded); i++ {
		if !isBase64Whitespace(encoded[i]) {
			encodedBytes++
			if encodedBytes > base64.StdEncoding.EncodedLen(MaxImagePayloadBytes) {
				b.recordFailure(fmt.Errorf("local OCR: encoded image exceeds %d-byte limit", MaxImagePayloadBytes))
				return ""
			}
		}
	}
	if encodedBytes == 0 {
		b.recordFailure(errors.New("local OCR: empty base64 image"))
		return ""
	}

	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		text, err := b.recognizeWithReader(func() io.Reader {
			return base64.NewDecoder(encoding, &ocrBase64Reader{source: encoded})
		})
		if err == nil {
			return text
		}
		if !errors.Is(err, errInvalidImageEncoding) {
			b.recordFailure(err)
			return text
		}
	}
	b.recordFailure(errors.New("local OCR: invalid base64 image payload"))
	return ""
}

func visionBase64Payload(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		comma := strings.IndexByte(raw, ',')
		if comma < 0 || !strings.HasPrefix(strings.ToLower(raw[:comma]), "data:image/") || !strings.HasSuffix(strings.ToLower(raw[:comma]), ";base64") {
			return "", errors.New("local OCR: unsupported image data URI")
		}
		raw = raw[comma+1:]
	}
	return raw, nil
}

func isBase64Whitespace(b byte) bool {
	return b == '\r' || b == '\n' || b == ' ' || b == '\t'
}

type ocrBase64Reader struct {
	source string
	offset int
}

func (r *ocrBase64Reader) Read(dst []byte) (int, error) {
	written := 0
	for written < len(dst) && r.offset < len(r.source) {
		b := r.source[r.offset]
		r.offset++
		if isBase64Whitespace(b) {
			continue
		}
		dst[written] = b
		written++
	}
	if written > 0 {
		return written, nil
	}
	return 0, io.EOF
}

func (b *imageOCRBudget) recognizeWithReader(openReader func() io.Reader) (string, error) {
	if err := b.parent.Err(); err != nil {
		return "", err
	}
	if err := b.ctx.Err(); err != nil {
		return "", errImageOCRBudgetExpired
	}
	itemCtx, cancel := context.WithTimeout(b.ctx, imageOCRItemBudget)
	defer cancel()
	release, err := AcquireImageMedia(itemCtx)
	if err != nil {
		return "", err
	}
	defer release()
	if err := itemCtx.Err(); err != nil {
		return "", err
	}
	img, err := decodeOCRImageReader(openReader)
	if err != nil {
		return "", err
	}
	return recognizeImage(itemCtx, img)
}

func (b *imageOCRBudget) recordFailure(err error) {
	if b == nil || err == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures == 0 {
		b.firstError = err.Error()
	}
	b.failures++
}

func (b *imageOCRBudget) resetFailures() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.firstError = ""
}

func (b *imageOCRBudget) warnings() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures == 0 {
		return nil
	}
	return []string{fmt.Sprintf("local DeepDOC OCR failed for %d image(s); first failure: %s", b.failures, b.firstError)}
}

// AcquireImageMedia limits concurrent image materialization across Parser OCR
// and ingestion VLM crop paths. The native analyzer keeps its own inference limit.
func AcquireImageMedia(ctx context.Context) (func(), error) {
	return sharedImageMediaAdmission().acquire(ctx)
}

type imageMediaAdmission struct {
	slots chan struct{}
}

func newImageMediaAdmission(limit int) *imageMediaAdmission {
	if limit < 1 {
		limit = 1
	}
	return &imageMediaAdmission{slots: make(chan struct{}, limit)}
}

func (a *imageMediaAdmission) acquire(ctx context.Context) (func(), error) {
	select {
	case a.slots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-a.slots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var (
	imageMediaAdmissionOnce sync.Once
	imageMediaAdmissionPool *imageMediaAdmission
)

func sharedImageMediaAdmission() *imageMediaAdmission {
	imageMediaAdmissionOnce.Do(func() {
		imageMediaAdmissionPool = newImageMediaAdmission(deepdocpdf.DeepDocConcurrency())
	})
	return imageMediaAdmissionPool
}

func recognizeImage(ctx context.Context, img image.Image) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	analyzer, err := GetDocAnalyzer()
	if err != nil {
		return "", fmt.Errorf("local OCR: %w", err)
	}
	if img == nil {
		return "", fmt.Errorf("local OCR: nil image")
	}
	boxes, err := analyzer.OCRDetect(ctx, img)
	if err != nil {
		return "", fmt.Errorf("local OCR: detect: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(boxes) == 0 {
		return "", nil
	}
	sort.Slice(boxes, func(i, j int) bool {
		yi := (boxes[i].Y0 + boxes[i].Y2) / 2
		yj := (boxes[j].Y0 + boxes[j].Y2) / 2
		if yi != yj {
			return yi < yj
		}
		return boxes[i].X0 < boxes[j].X0
	})

	bounds := img.Bounds()
	texts := make([]string, 0, len(boxes))
	failedBoxes := 0
	for _, box := range boxes {
		if err := ctx.Err(); err != nil {
			return strings.Join(texts, "\n"), err
		}
		x0 := maxInt(bounds.Min.X, int(min4(box.X0, box.X1, box.X2, box.X3)))
		y0 := maxInt(bounds.Min.Y, int(min4(box.Y0, box.Y1, box.Y2, box.Y3)))
		x1 := minInt(bounds.Max.X, int(max4(box.X0, box.X1, box.X2, box.X3)))
		y1 := minInt(bounds.Max.Y, int(max4(box.Y0, box.Y1, box.Y2, box.Y3)))
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		crop := cropImage(img, x0, y0, x1, y1)
		if crop == nil {
			continue
		}
		recTexts, err := analyzer.OCRRecognize(ctx, crop)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return strings.Join(texts, "\n"), ctxErr
		}
		if err != nil {
			failedBoxes++
			continue
		}
		for _, recognized := range recTexts {
			if text := strings.TrimSpace(recognized.Text); text != "" {
				texts = append(texts, text)
			}
		}
	}
	if failedBoxes > 0 {
		return strings.Join(texts, "\n"), fmt.Errorf("local OCR: recognition failed for %d of %d text regions", failedBoxes, len(boxes))
	}
	return strings.Join(texts, "\n"), nil
}

var errInvalidImageEncoding = errors.New("local OCR: invalid image encoding")

func decodeOCRImageReader(openReader func() io.Reader) (image.Image, error) {
	config, _, err := image.DecodeConfig(io.LimitReader(openReader(), MaxImagePayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: decode image config: %v", errInvalidImageEncoding, err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > MaxImageEdge || config.Height > MaxImageEdge || int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return nil, fmt.Errorf("local OCR: image dimensions %dx%d exceed limits", config.Width, config.Height)
	}
	img, _, err := image.Decode(io.LimitReader(openReader(), MaxImagePayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: decode image: %v", errInvalidImageEncoding, err)
	}
	return img, nil
}

func appendOCRText(item map[string]any, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	existing, _ := item["text"].(string)
	if strings.TrimSpace(existing) == "" {
		item["text"] = text
		return
	}
	item["text"] = existing + "\n" + text
}

func cropImage(img image.Image, x0, y0, x1, y1 int) image.Image {
	rect := image.Rect(x0, y0, x1, y1)
	switch src := img.(type) {
	case *image.RGBA:
		return src.SubImage(rect)
	case *image.NRGBA:
		return src.SubImage(rect)
	case *image.RGBA64:
		return src.SubImage(rect)
	case *image.NRGBA64:
		return src.SubImage(rect)
	case *image.Gray:
		return src.SubImage(rect)
	case *image.Gray16:
		return src.SubImage(rect)
	case *image.YCbCr:
		return src.SubImage(rect)
	case *image.Paletted:
		return src.SubImage(rect)
	default:
		rgba := image.NewRGBA(rect)
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				rgba.Set(x, y, img.At(x, y))
			}
		}
		return rgba
	}
}

func min4(a, b, c, d float64) float64 {
	return minFloat(a, minFloat(b, minFloat(c, d)))
}

func max4(a, b, c, d float64) float64 {
	return maxFloat(a, maxFloat(b, maxFloat(c, d)))
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
