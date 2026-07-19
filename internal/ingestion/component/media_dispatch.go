//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Media dispatch: image, audio, video parser branches that require
// model access (OCR, IMAGE2TEXT, SPEECH2TEXT) at the component
// layer. Mirrors Python's _image / _audio / _video methods in
// rag/flow/parser/parser.py and rag/app/picture.py.
//
// These follow the maybeDispatchPDFVision pattern: they bypass
// dispatchParse and call the model directly from the component
// layer, returning a parserDispatchResult.

package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	// Import image decoders for common formats.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ragflow/internal/common"
	inference "ragflow/internal/deepdoc/parser/pdf/inference"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// Video dispatch: IMAGE2TEXT vision chat ---

func maybeDispatchVideo(
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypeVIDEO {
		return parserDispatchResult{}, false, nil
	}
	setup, ok := setups["video"]
	if !ok {
		return parserDispatchResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: video requires tenant_id")
	}

	// Resolve the tenant's IMAGE2TEXT model.
	driver, modelName, apiConfig, _, err := resolveTenantModelByType(tenantID, entity.ModelTypeImage2Text)
	if err != nil {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: video image2text model: %w", err)
	}

	videoPrompt, _ := setup["prompt"].(string)
	videoB64 := base64.StdEncoding.EncodeToString(binary)

	// Build a multimodal message with the video payload.
	// Python uses cv_mdl.async_chat(video_bytes=blob, ...);
	// Go ChatWithMessages is synchronous and uses a data URI.
	mimeType := videoMIME(filename)
	dataURI := "data:" + mimeType + ";base64," + videoB64
	messages := []modelModule.Message{{
		Role: "user",
		Content: []interface{}{
			map[string]any{"type": "text", "text": videoPrompt},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": dataURI}},
		},
	}}
	vision := true
	resp, err := driver.ChatWithMessages(modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision}, nil)
	if err != nil {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: video describe: %w", err)
	}
	txt := ""
	if resp != nil && resp.Answer != nil {
		txt = strings.TrimSpace(*resp.Answer)
	}

	outputFormat, _ := setup["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "text"
	}
	return parserDispatchResult{
		OutputFormat: outputFormat,
		DocType:      "video",
		Text:         txt,
	}, true, nil
}

// Image dispatch: OCR + IMAGE2TEXT vision describe ---
// Mirrors Python's rag/app/picture.py:chunk() image branch:
//   1. Try PaddleOCR if layout_recognize is "@PaddleOCR"
//   2. Fallback to local ONNX OCR (DeepDoc /predict/ocr endpoint)
//   3. If OCR text is short (≤32 chars or ≤32 English words),
//      also call IMAGE2TEXT VLM describe()
//   4. Returns combined text

func maybeDispatchImage(
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypeVISUAL {
		return parserDispatchResult{}, false, nil
	}
	setup, ok := setups["image"]
	if !ok {
		return parserDispatchResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: image requires tenant_id")
	}

	// --- Phase 1: OCR ---
	var ocrText string

	// Step 1a: Try PaddleOCR if layout_recognize is set to PaddleOCR.
	// Mirrors Python's picture.py:_try_paddleocr_image().
	layoutRecognize := getStringOr(setup, "layout_recognize", "")
	if layoutRecognize != "" {
		recognizer, _ := normalizeLayoutRecognizer(layoutRecognize)
		if recognizer == "PaddleOCR" {
			if txt, err := runPaddleOCRImage(binary, filename); err == nil && txt != "" {
				ocrText = txt
			}
		}
	}

	// Step 1b: Fallback to local ONNX OCR (DeepDoc /predict/ocr).
	// Mirrors Python's picture.py:ocr(np.array(img)) from deepdoc.vision.
	if ocrText == "" {
		if txt, err := runLocalImageOCR(binary); err == nil && txt != "" {
			ocrText = txt
		}
	}

	outputFormat, _ := setup["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "text"
	}

	// --- Phase 2: VLM description (when OCR text is short) ---
	// Mirrors Python's check: if (eng and len(txt.split()) > 32) or len(txt) > 32
	// then use OCR text only; otherwise call cv_mdl.describe().
	lang := getStringOr(setup, "lang", "")
	eng := strings.EqualFold(lang, "english")

	if ocrText != "" {
		wordCount := len(strings.Fields(ocrText))
		charCount := len(ocrText)
		if (eng && wordCount > 32) || charCount > 32 {
			// OCR returned substantial text — skip VLM.
			return parserDispatchResult{
				OutputFormat: outputFormat,
				DocType:      "image",
				Text:         ocrText,
			}, true, nil
		}
	}

	// Short OCR text (or no text): supplement with VLM describe.
	driver, modelName, apiConfig, _, err := resolveTenantModelByType(tenantID, entity.ModelTypeImage2Text)
	if err != nil {
		// If VLM is unavailable but we have OCR text, return it.
		if ocrText != "" {
			return parserDispatchResult{
				OutputFormat: outputFormat,
				DocType:      "image",
				Text:         ocrText,
			}, true, nil
		}
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: picture image2text model: %w", err)
	}

	imageB64 := base64.StdEncoding.EncodeToString(binary)
	mimeType := imageMIME(filename)
	dataURI := "data:" + mimeType + ";base64," + imageB64

	prompt := "Describe this image in detail."
	if v, ok := setup["prompt"].(string); ok && v != "" {
		prompt = v
	}
	messages := []modelModule.Message{{
		Role: "user",
		Content: []interface{}{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
		},
	}}
	vision := true
	resp, err := driver.ChatWithMessages(modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision}, nil)
	if err != nil {
		if ocrText != "" {
			return parserDispatchResult{
				OutputFormat: outputFormat,
				DocType:      "image",
				Text:         ocrText,
			}, true, nil
		}
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: picture describe: %w", err)
	}
	vlmText := ""
	if resp != nil && resp.Answer != nil {
		vlmText = strings.TrimSpace(*resp.Answer)
	}

	// Combine OCR + VLM text.
	// Mirrors Python: txt += "\n" + ans
	combined := ocrText
	if vlmText != "" {
		if combined != "" {
			combined += "\n" + vlmText
		} else {
			combined = vlmText
		}
	}
	return parserDispatchResult{
		OutputFormat: outputFormat,
		DocType:      "image",
		Text:         combined,
	}, true, nil
}

// Audio dispatch: SPEECH2TEXT transcription ---
// Mirrors Python's rag/app/audio.py:chunk():
//   - Writes the audio binary to a temp file (extension-preserving)
//   - Calls the tenant's SPEECH2TEXT model via TranscribeAudio()
//   - Returns the transcription as text

func maybeDispatchAudio(
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypeAURAL {
		return parserDispatchResult{}, false, nil
	}
	setup, ok := setups["audio"]
	if !ok {
		return parserDispatchResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: audio requires tenant_id")
	}

	driver, modelName, apiConfig, _, err := resolveTenantModelByType(tenantID, entity.ModelTypeSpeech2Text)
	if err != nil {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: audio speech2text model: %w", err)
	}

	tmpFile, err := writeTempAudioFile(filename, binary)
	if err != nil {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: audio temp file: %w", err)
	}
	defer os.Remove(tmpFile)

	resp, err := driver.TranscribeAudio(&modelName, &tmpFile, apiConfig, nil, nil)
	if err != nil {
		return parserDispatchResult{}, true,
			fmt.Errorf("Parser: audio transcription: %w", err)
	}

	transcription := ""
	if resp != nil {
		transcription = resp.Text
	}

	outputFormat, _ := setup["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "text"
	}
	return parserDispatchResult{
		OutputFormat: outputFormat,
		DocType:      "audio",
		Text:         transcription,
	}, true, nil
}

// writeTempAudioFile writes binary to a temp file preserving the
// original extension so the ASR provider can detect the format.
func writeTempAudioFile(filename string, binary []byte) (string, error) {
	ext := filepath.Ext(filename)
	tmp, err := os.CreateTemp("", "ragflow_audio_*"+ext)
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := tmp.Write(binary); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// normalizeLayoutRecognizer parses layout_recognize strings like
// "model@PaddleOCR" → ("PaddleOCR", "model@PaddleOCR").
// Mirrors Python's common/parser_config_utils.py:normalize_layout_recognizer().
func normalizeLayoutRecognizer(raw string) (recognizer, modelName string) {
	lowered := strings.ToLower(raw)
	if strings.HasSuffix(lowered, "@paddleocr") {
		return "PaddleOCR", raw
	}
	if strings.HasSuffix(lowered, "@mineru") {
		return "MinerU", raw
	}
	if strings.HasSuffix(lowered, "@somark") {
		return "SoMark", raw
	}
	if strings.HasSuffix(lowered, "@opendataloader") {
		return "OpenDataLoader", raw
	}
	return raw, ""
}

// imageMIME maps common image filename extensions to MIME types
// for constructing base64 data URIs.
func imageMIME(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot == -1 {
		return "image/png"
	}
	switch strings.ToLower(filename[dot+1:]) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "tiff", "tif":
		return "image/tiff"
	case "ico":
		return "image/x-icon"
	case "avif":
		return "image/avif"
	case "heic":
		return "image/heic"
	default:
		return "image/png"
	}
}

// videoMIME maps common video filename extensions to MIME types
// for constructing base64 data URIs.
func videoMIME(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot == -1 {
		return "video/mp4"
	}
	switch strings.ToLower(filename[dot+1:]) {
	case "mp4":
		return "video/mp4"
	case "avi":
		return "video/x-msvideo"
	case "mkv":
		return "video/x-matroska"
	case "mov":
		return "video/quicktime"
	case "wmv":
		return "video/x-ms-wmv"
	case "flv":
		return "video/x-flv"
	case "webm":
		return "video/webm"
	case "mpeg", "mpg":
		return "video/mpeg"
	case "3gp":
		return "video/3gpp"
	default:
		return "video/mp4"
	}
}

// --- OCR helpers for picture dispatch ---

// runPaddleOCRImage tries PaddleOCR remote API for image text extraction.
// Mirrors Python's picture.py:_try_paddleocr_image() which creates a
// PaddleOCRParser and calls parse_image().
func runPaddleOCRImage(binary []byte, filename string) (string, error) {
	client := parser.NewPaddleOCRClientFromEnv()
	if !client.Enabled() {
		return "", fmt.Errorf("paddleocr: not configured (set PADDLEOCR_ACCESS_TOKEN)")
	}
	return client.ParseImage(binary, filename)
}

// runLocalImageOCR uses the DeepDoc inference service (/predict/ocr) to
// detect and recognize text in an image. Mirrors Python's
// deepdoc.vision.OCR (local ONNX pipeline), but routed through the
// DeepDoc HTTP service which wraps the same ONNX models.
//
// Pipeline:
//  1. Decode image bytes → image.Image
//  2. OCRDetect → find text region boxes
//  3. For each box: crop → OCRRecognize → text
//  4. Sort boxes by Y, then X (reading order)
//  5. Join all recognized text with newlines
func runLocalImageOCR(binary []byte) (string, error) {
	deepdocURL := common.GetEnv(common.EnvDeepDocURL)
	if deepdocURL == "" {
		deepdocURL = common.GetEnv(common.EnvTensorrtDLAServer)
	}
	if deepdocURL == "" {
		return "", fmt.Errorf("local OCR: DEEPDOC_URL not configured")
	}

	client, err := inference.NewClient(deepdocURL)
	if err != nil {
		return "", fmt.Errorf("local OCR: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(binary))
	if err != nil {
		return "", fmt.Errorf("local OCR: decode image: %w", err)
	}

	// Step 1: Detect text regions.
	ctx := context.Background()
	boxes, err := client.OCRDetect(ctx, img)
	if err != nil {
		return "", fmt.Errorf("local OCR: detect: %w", err)
	}
	if len(boxes) == 0 {
		return "", nil
	}

	// Step 2: Sort boxes by Y (top to bottom), then X (left to right)
	// for reading-order text assembly.
	sort.Slice(boxes, func(i, j int) bool {
		yi := (boxes[i].Y0 + boxes[i].Y2) / 2
		yj := (boxes[j].Y0 + boxes[j].Y2) / 2
		if yi < yj {
			return true
		}
		if yi > yj {
			return false
		}
		return boxes[i].X0 < boxes[j].X0
	})

	// Step 3: Recognize text per box.
	var texts []string
	bounds := img.Bounds()
	for _, box := range boxes {
		// Convert quad box to axis-aligned crop rect.
		x0 := int(min4(box.X0, box.X1, box.X2, box.X3))
		y0 := int(min4(box.Y0, box.Y1, box.Y2, box.Y3))
		x1 := int(max4(box.X0, box.X1, box.X2, box.X3))
		y1 := int(max4(box.Y0, box.Y1, box.Y2, box.Y3))

		// Clamp to image bounds.
		if x0 < bounds.Min.X {
			x0 = bounds.Min.X
		}
		if y0 < bounds.Min.Y {
			y0 = bounds.Min.Y
		}
		if x1 > bounds.Max.X {
			x1 = bounds.Max.X
		}
		if y1 > bounds.Max.Y {
			y1 = bounds.Max.Y
		}
		if x1 <= x0 || y1 <= y0 {
			continue
		}

		// Crop the region. This requires an image type that supports
		// cropping; for simplicity we recode through a sub-image.
		crop := cropImage(img, x0, y0, x1, y1)
		if crop == nil {
			continue
		}

		recTexts, err := client.OCRRecognize(ctx, crop)
		if err != nil {
			continue // skip boxes that fail recognition
		}
		for _, t := range recTexts {
			s := strings.TrimSpace(t.Text)
			if s != "" {
				texts = append(texts, s)
			}
		}
	}

	if len(texts) == 0 {
		return "", nil
	}
	return strings.Join(texts, "\n"), nil
}

// cropImage extracts a sub-rectangle from img. Works with any image.Image
// by converting to RGBA if needed, then cropping.
func cropImage(img image.Image, x0, y0, x1, y1 int) image.Image {
	bounds := img.Bounds()
	cropRect := image.Rect(
		bounds.Min.X+x0, bounds.Min.Y+y0,
		bounds.Min.X+x1, bounds.Min.Y+y1,
	)
	switch src := img.(type) {
	case *image.RGBA:
		return src.SubImage(cropRect)
	case *image.NRGBA:
		return src.SubImage(cropRect)
	case *image.RGBA64:
		return src.SubImage(cropRect)
	case *image.NRGBA64:
		return src.SubImage(cropRect)
	case *image.Gray:
		return src.SubImage(cropRect)
	case *image.Gray16:
		return src.SubImage(cropRect)
	case *image.YCbCr:
		return src.SubImage(cropRect)
	case *image.Paletted:
		return src.SubImage(cropRect)
	default:
		// Convert to RGBA for cropping.
		rgba := image.NewRGBA(cropRect)
		for y := cropRect.Min.Y; y < cropRect.Max.Y; y++ {
			for x := cropRect.Min.X; x < cropRect.Max.X; x++ {
				rgba.Set(x, y, img.At(x, y))
			}
		}
		return rgba
	}
}

func min4(a, b, c, d float64) float64 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	if d < m {
		m = d
	}
	return m
}

func max4(a, b, c, d float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	if d > m {
		m = d
	}
	return m
}
