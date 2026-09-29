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

// Media dispatch: image, audio, and video branches that require model access
// (IMAGE2TEXT, SPEECH2TEXT) at the component layer.

package component

import (
	"context"
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

// Image dispatch: IMAGE2TEXT vision describe ---

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
	if !enableVisionEnhancement {
		return parser.ParseResult{}, true, fmt.Errorf("parser: image has no searchable text because vision enhancement is disabled")
	}
	parsed := dispatchParse(ctx, fileType, filename, binary, setups)
	if parsed.Err != nil {
		return parsed, true, parsed.Err
	}
	if len(parsed.JSON) == 0 {
		return parser.ParseResult{}, true, fmt.Errorf("parser: image parser returned no image item")
	}
	imageData, _ := parsed.JSON[0]["image"].(string)
	if imageData == "" {
		return parser.ParseResult{}, true, fmt.Errorf("parser: image parser returned no image payload")
	}
	result, handled, err := maybeDispatchImageVLM(ctx, db, imageData, tenantID, setup, inputs)
	result.Warnings = append(result.Warnings, parsed.Warnings...)
	result.File = parsed.File
	if err == nil && len(result.JSON) > 0 {
		text, _ := result.JSON[0]["text"].(string)
		if strings.TrimSpace(text) == "" {
			if len(result.Warnings) > 0 {
				return result, true, fmt.Errorf("parser: image has no searchable text: %s", result.Warnings[0])
			}
			return result, true, fmt.Errorf("parser: vision enhancement returned no searchable text")
		}
	}
	return result, handled, err
}

func maybeDispatchImageVLM(
	ctx context.Context,
	db *gorm.DB,
	dataURI string,
	tenantID string,
	setup schema.ParserSetup,
	inputs map[string]any,
) (parser.ParseResult, bool, error) {
	// --- Optional VLM description ---
	lang := resolveVisionLanguage(inputs, getStringOr(setup, "lang", ""))
	if tenantID == "" {
		result := imageDispatchResult("", dataURI)
		result.Warnings = append(result.Warnings, "image VLM enhancement skipped: tenant ID is missing")
		return result, true, nil
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
		result := imageDispatchResult("", dataURI)
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
		result := imageDispatchResult("", dataURI)
		result.Warnings = append(result.Warnings, fmt.Sprintf("image VLM enhancement failed: %v", err))
		return result, true, nil
	}
	vlmText := ""
	if resp != nil && resp.Answer != nil {
		vlmText = strings.TrimSpace(*resp.Answer)
	}

	return imageDispatchResult(vlmText, dataURI), true, nil
}

// imageDispatchResult builds the structured JSON payload for the image
// family: a single item carrying the VLM description, the image attachment
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
