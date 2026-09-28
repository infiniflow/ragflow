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
// model access (IMAGE2TEXT, SPEECH2TEXT) at the component layer.
//
// Audio and video branches dispatch their models directly. Image OCR is
// supplied by PictureParser; the component retains PaddleOCR selection and
// optional IMAGE2TEXT enrichment.

package component

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

// Video dispatch: IMAGE2TEXT vision chat ---

func maybeDispatchVideo(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parser.ParseResult, bool, error) {
	if fileType != utility.FileTypeVIDEO {
		return parser.ParseResult{}, false, nil
	}
	if _, ok := setups["video"]; !ok {
		return parser.ParseResult{}, false, nil
	}

	// Video parsing is intentionally not implemented yet: the underlying
	// video-analysis capability is pending. The previously-shipped path sent a
	// video_url data URI, but no model driver honors it — OpenAI-compatible
	// drivers only accept image_url (the block is ignored), and Gemini's
	// googleMessageParts only accepts text/image_url and silently drops
	// video_url. Returning an explicit error is safer than silently producing
	// a description from the prompt text alone. The real implementation must
	// be provider-specific (OpenAI-compatible: frame extraction -> image_url;
	// Gemini: raw-bytes inline_data; Qwen: file://).
	// When video analysis is implemented, it emits standard Text JSON items:
	// [{"text": transcript, "doc_type_kwd": "text"}] with output_format "json".
	return parser.ParseResult{}, true,
		fmt.Errorf("Parser: video parsing is not yet supported; underlying video analysis capability is pending")
}

// Image dispatch: optional PaddleOCR plus IMAGE2TEXT vision describe ---
//   1. Try PaddleOCR if layout_recognize is "@PaddleOCR"
//   2. Fall back to local DeepDOC OCR in PictureParser
//   3. If image enhancement is enabled, call IMAGE2TEXT VLM describe()
//   4. Returns combined text

func maybeDispatchImage(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
	enableVisionEnhancement bool,
) (parser.ParseResult, bool, error) {
	if fileType != utility.FileTypeVISUAL {
		return parser.ParseResult{}, false, nil
	}
	setup, ok := setups["image"]
	if !ok {
		return parser.ParseResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
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

	// PictureParser owns the local DeepDOC fallback and returns its OCR text
	// and warnings as part of the parser result.
	var parsed parser.ParseResult
	if strings.TrimSpace(ocrText) == "" {
		parsed = dispatchParse(ctx, fileType, filename, binary, setups)
		if parsed.Err != nil {
			return parsed, true, parsed.Err
		}
		if len(parsed.JSON) > 0 {
			ocrText, _ = parsed.JSON[0]["text"].(string)
		}
	}

	release, err := parser.AcquireImageMedia(ctx)
	if err != nil {
		return parser.ParseResult{}, true, err
	}
	var dataURI string
	func() {
		defer release()
		imageB64 := base64.StdEncoding.EncodeToString(binary)
		dataURI = "data:" + imageMIME(filename) + ";base64," + imageB64
	}()
	if !enableVisionEnhancement {
		result := imageDispatchResult(ocrText, dataURI)
		result.Warnings = append(result.Warnings, parsed.Warnings...)
		return result, true, nil
	}
	result, handled, err := maybeDispatchImageVLM(ctx, db, dataURI, ocrText, tenantID, setup, inputs)
	result.Warnings = append(result.Warnings, parsed.Warnings...)
	return result, handled, err
}

func maybeDispatchImageVLM(
	ctx context.Context,
	db *gorm.DB,
	dataURI string,
	ocrText string,
	tenantID string,
	setup schema.ParserSetup,
	inputs map[string]any,
) (parser.ParseResult, bool, error) {
	// --- Phase 2: optional VLM description ---
	lang := resolveVisionLanguage(inputs, getStringOr(setup, "lang", ""))
	if tenantID == "" {
		result := imageDispatchResult(ocrText, dataURI)
		result.Warnings = append(result.Warnings, "image VLM enhancement skipped: tenant ID is missing")
		return result, true, nil
	}

	// Supplement parser-provided OCR text with a VLM description.
	modelRef := configuredMediaModelID(setup, "image")
	var driver modelModule.ModelDriver
	var modelName string
	var apiConfig *modelModule.APIConfig
	var err error
	if modelRef != "" {
		driver, modelName, apiConfig, _, err = resolveModelConfig(ctx, db, tenantID, entity.ModelTypeImage2Text, modelRef)
		if err != nil {
			common.Warn("media dispatch image: per-call VLM resolve failed, falling back to tenant default",
				zap.String("modelRef", modelRef), zap.String("tenant", tenantID), zap.Error(err))
			driver, modelName, apiConfig, _, err = resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
		}
	} else {
		driver, modelName, apiConfig, _, err = resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
	}
	if err == nil && driver == nil {
		err = fmt.Errorf("no usable vision model")
	}
	if err != nil {
		result := imageDispatchResult(ocrText, dataURI)
		result.Warnings = append(result.Warnings, fmt.Sprintf("image VLM enhancement skipped: model unavailable: %v", err))
		return result, true, nil
	}

	prompt := defaultImageVisionPrompt(lang)
	// image family's contract key is system_prompt (parser.go:295),
	// mirroring Python parser.py:1119. Do NOT read setup["prompt"]
	// here — that key is for the video family, not image.
	if v, ok := setup["system_prompt"].(string); ok && v != "" {
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
	resp, err := driver.ChatWithMessages(ctx, modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision}, nil)
	if err != nil {
		result := imageDispatchResult(ocrText, dataURI)
		result.Warnings = append(result.Warnings, fmt.Sprintf("image VLM enhancement failed: %v", err))
		return result, true, nil
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
	return imageDispatchResult(combined, dataURI), true, nil
}

// imageDispatchResult builds the structured JSON payload for the image
// family: a single item carrying the combined text, the image attachment
// (data URI), and doc_type_kwd "image". Mirrors Python
// rag/app/picture.py:71-72.
func imageDispatchResult(text, dataURI string) parser.ParseResult {
	return parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{{
			"text":         text,
			"image":        dataURI,
			"doc_type_kwd": "image",
		}},
	}
}

// Audio dispatch: SPEECH2TEXT transcription ---
// Mirrors Python's rag/app/audio.py:chunk():
//   - Writes the audio binary to a temp file (extension-preserving)
//   - Calls the tenant's SPEECH2TEXT model via TranscribeAudio()
//   - Returns the transcription as text

func maybeDispatchAudio(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parser.ParseResult, bool, error) {
	if fileType != utility.FileTypeAURAL {
		return parser.ParseResult{}, false, nil
	}
	setup, ok := setups["audio"]
	if !ok {
		return parser.ParseResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return parser.ParseResult{}, true,
			fmt.Errorf("parser: audio requires tenant_id")
	}

	modelRef := configuredMediaModelID(setup, "audio")
	var driver modelModule.ModelDriver
	var modelName string
	var apiConfig *modelModule.APIConfig
	var err error
	if modelRef != "" {
		driver, modelName, apiConfig, _, err = resolveModelConfig(ctx, db, tenantID, entity.ModelTypeSpeech2Text, modelRef)
		if err != nil {
			common.Warn("media dispatch audio: per-call VLM resolve failed, falling back to tenant default",
				zap.String("modelRef", modelRef), zap.String("tenant", tenantID), zap.Error(err))
			driver, modelName, apiConfig, _, err = resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeSpeech2Text)
		}
	} else {
		driver, modelName, apiConfig, _, err = resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeSpeech2Text)
	}
	if err != nil {
		return parser.ParseResult{}, true,
			fmt.Errorf("parser: audio speech2text model: %w", err)
	}

	tmpFile, err := writeTempAudioFile(filename, binary)
	if err != nil {
		return parser.ParseResult{}, true,
			fmt.Errorf("parser: audio temp file: %w", err)
	}
	defer os.Remove(tmpFile)

	resp, err := driver.TranscribeAudio(ctx, &modelName, &tmpFile, apiConfig, nil, nil)
	if err != nil {
		return parser.ParseResult{}, true,
			fmt.Errorf("Parser: audio transcription: %w", err)
	}

	transcription := ""
	if resp != nil {
		transcription = resp.Text
	}

	return parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{{
			"text":         transcription,
			"doc_type_kwd": "text",
		}},
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
// for constructing base64 data URIs. Retained as a reference for the
// future real video-parsing implementation (provider-specific frame
// extraction / inline_data / file://); not currently used because
// maybeDispatchVideo returns an explicit unsupported error.
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
