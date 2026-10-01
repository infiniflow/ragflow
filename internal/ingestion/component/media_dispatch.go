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

// Media dispatch runs image OCR and optional vision enhancement, audio
// transcription, and video dispatch at the component layer.

package component

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
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

// Image dispatch: OCR followed by optional IMAGE2TEXT enhancement.

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
	method := getStringOr(setup, "parse_method", "")
	useOCR := method == "" || strings.EqualFold(method, "ocr")
	release, err := parser.AcquireImageMedia(ctx)
	if err != nil {
		return parser.ParseResult{}, true, err
	}
	defer release()
	img, err := decodeDispatchImage(binary, useOCR)
	if err != nil {
		return parser.ParseResult{}, true, err
	}
	var text string
	if useOCR {
		text, err = extractImageText(ctx, img)
	}
	release()
	parsed := dispatchParse(ctx, fileType, filename, binary, setups)
	if parsed.Err != nil {
		return parsed, true, parsed.Err
	}
	if len(parsed.JSON) == 0 {
		return parsed, true, fmt.Errorf("parser: image parser returned no image item")
	}
	parsed.OutputFormat = "json"
	imageData, _ := parsed.JSON[0]["image"].(string)
	parsed.JSON[0]["text"] = text
	if err != nil {
		parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("image OCR unavailable: %v", err))
	} else if useOCR && strings.TrimSpace(text) == "" {
		parsed.Warnings = append(parsed.Warnings, "image OCR returned no text")
	}
	if err := ctx.Err(); err != nil {
		return parsed, true, err
	}
	if enableVisionEnhancement {
		description, warnings := describeImage(ctx, db, imageData, getStringOr(inputs, "tenant_id", ""), setup, inputs)
		parsed.Warnings = append(parsed.Warnings, warnings...)
		if description != "" {
			appendItemText(parsed.JSON[0], description)
		}
	}
	return parsed, true, nil
}

func decodeDispatchImage(data []byte, decodeRaster bool) (image.Image, error) {
	if len(data) == 0 || len(data) > parser.MaxImagePayloadBytes {
		return nil, fmt.Errorf("parser: image payload exceeds size limits")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parser: decode image: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > parser.MaxImageEdge || config.Height > parser.MaxImageEdge || int64(config.Width)*int64(config.Height) > parser.MaxImagePixels {
		return nil, fmt.Errorf("parser: image dimensions %dx%d exceed limits", config.Width, config.Height)
	}
	// VLM consumes the original bytes; only local OCR needs a decoded raster.
	if !decodeRaster {
		return nil, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parser: decode image: %w", err)
	}
	return img, nil
}

func extractImageText(ctx context.Context, img image.Image) (string, error) {
	analyzer, err := parser.GetDocAnalyzer()
	if err != nil {
		return "", err
	}
	if analyzer == nil || !analyzer.Health() {
		return "", fmt.Errorf("local OCR analyzer is unavailable")
	}
	boxes := deepdocpdf.OCRImage(ctx, img, analyzer, 1.0)
	sort.SliceStable(boxes, func(i, j int) bool {
		if boxes[i].Top == boxes[j].Top {
			return boxes[i].X0 < boxes[j].X0
		}
		return boxes[i].Top < boxes[j].Top
	})
	texts := make([]string, 0, len(boxes))
	for _, box := range boxes {
		if text := strings.TrimSpace(box.Text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func describeImage(
	ctx context.Context,
	db *gorm.DB,
	dataURI string,
	tenantID string,
	setup schema.ParserSetup,
	inputs map[string]any,
) (string, []string) {
	// --- Optional VLM description ---
	lang := resolveVisionLanguage(inputs, getStringOr(setup, "lang", ""))
	if tenantID == "" {
		return "", []string{"image VLM enhancement skipped: tenant ID is missing"}
	}

	// Use the configured image VLM or the tenant default.
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
		return "", []string{fmt.Sprintf("image VLM enhancement skipped: model unavailable: %v", err)}
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
	chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)
	resp, err := chatModel.ChatWithMessages(ctx, messages, &modelModule.ChatConfig{Vision: &vision}, nil)
	if err != nil {
		return "", []string{fmt.Sprintf("image VLM enhancement failed: %v", err)}
	}
	vlmText := ""
	if resp != nil && resp.Answer != nil {
		vlmText = strings.TrimSpace(*resp.Answer)
	}

	return vlmText, nil
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

	asrModel := modelModule.NewASRModel(driver, &modelName, apiConfig)
	resp, err := asrModel.Transcribe(ctx, &tmpFile, nil, nil)
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
