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

package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

// imagePromptCaptureDriver embeds ModelDriver so it satisfies the interface
// without listing every method; only ChatWithMessages is overridden to record
// the messages the image branch selected.
type imagePromptCaptureDriver struct {
	modelModule.ModelDriver
	mu       sync.Mutex
	captured []modelModule.Message
}

func (d *imagePromptCaptureDriver) ChatWithMessages(ctx context.Context, modelName string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, chatModelConfig *modelModule.ChatConfig, usage *common.ModelUsage) (*modelModule.ChatResponse, error) {
	d.mu.Lock()
	d.captured = append(d.captured, messages...)
	d.mu.Unlock()
	ans := "captured"
	return &modelModule.ChatResponse{Answer: &ans}, nil
}

// firstUserText extracts the text of the first "text" content part from the
// first captured user message. It scans parts by the "type" discriminator so
// the test stays valid regardless of part ordering (image_url may precede text).
func firstUserText(msgs []modelModule.Message) (string, bool) {
	if len(msgs) == 0 {
		return "", false
	}
	parts, ok := msgs[0].Content.([]interface{})
	if !ok || len(parts) == 0 {
		return "", false
	}
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if part["type"] != "text" {
			continue
		}
		if txt, ok := part["text"].(string); ok {
			return txt, true
		}
	}
	return "", false
}

// TestMaybeDispatchImage_UsesSystemPrompt pins that the image branch reads the
// `system_prompt` setup key (Python/DSL contract), not `prompt`. Before the fix
// the branch read setup["prompt"] which is empty for image, so the VLM received
// the hardcoded default instead of the user-configured value.
func TestMaybeDispatchImage_UsesSystemPrompt(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	drv := &imagePromptCaptureDriver{}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	// image family's contract key is system_prompt (parser.go:295).
	// Also set a legacy `prompt` sentinel to assert system_prompt wins
	// when both keys are present (regression guard against re-reading
	// setup["prompt"], which is the video-family key, not image).
	setups["image"]["prompt"] = "legacy prompt"
	setups["image"]["system_prompt"] = "自定义视觉提示"

	ctx := t.Context()
	res, dispatched, err := maybeDispatchImage(
		ctx,
		dao.DB,
		utility.FileTypeVISUAL,
		"test.png",
		[]byte("not-a-real-image"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchImage: %v", err)
	}
	if !dispatched {
		t.Fatalf("expected dispatched=true for VISUAL file")
	}
	// After the output-shape fix the image branch returns JSON items
	// (OutputFormat=="json"), not a bare Text field. The combined text
	// now lives in JSON[0]["text"]; the legacy res.Text is no longer
	// populated for the image family.
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json (image family is always structured)", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1 (image result must be a single JSON item)", len(res.JSON))
	}
	if txt, _ := res.JSON[0]["text"].(string); txt == "" {
		t.Fatalf("expected non-empty combined text in JSON[0][\"text\"]")
	}

	got, ok := firstUserText(drv.captured)
	if !ok {
		t.Fatalf("no user text captured in VLM messages: %#v", drv.captured)
	}
	if got != "自定义视觉提示" {
		t.Fatalf("VLM user text = %q, want %q (image branch must read system_prompt)", got, "自定义视觉提示")
	}
}

func TestMaybeDispatchImage_DefaultPromptUsesDatasetLanguage(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	drv := &imagePromptCaptureDriver{}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["image"]["lang"] = "Chinese"
	setups["image"]["system_prompt"] = ""

	_, _, err := maybeDispatchImage(
		t.Context(),
		dao.DB,
		utility.FileTypeVISUAL,
		"test.png",
		[]byte("not-a-real-image"),
		map[string]any{"tenant_id": "t1", "lang": "Japanese"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchImage: %v", err)
	}

	got, ok := firstUserText(drv.captured)
	if !ok {
		t.Fatalf("no user text captured in VLM messages: %#v", drv.captured)
	}
	if !strings.Contains(got, "Respond in Japanese.") {
		t.Fatalf("VLM user text = %q, want dataset language instruction", got)
	}
	if strings.Contains(got, "Respond in Chinese.") {
		t.Fatalf("VLM user text = %q, setup fallback overrode dataset language", got)
	}
}

// TestMaybeDispatchImage_ReturnsJSONWithImage pins the output-shape fix:
// the image branch must return a JSON item carrying the `image` attachment
// (data URI) and `doc_type_kwd:"image"`, mirroring Python
// rag/app/picture.py:71-72. Before the fix the branch returned a bare Text
// string with JSON=nil, dropping the image attachment and (on the default
// json path) causing OneChunker/TokenChunker to reject the payload.
func TestMaybeDispatchImage_ReturnsJSONWithImage(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	drv := &imagePromptCaptureDriver{}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	ctx := t.Context()
	res, dispatched, err := maybeDispatchImage(
		ctx,
		dao.DB,
		utility.FileTypeVISUAL,
		"test.png",
		[]byte("not-a-real-image"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchImage: %v", err)
	}
	if !dispatched {
		t.Fatalf("expected dispatched=true")
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	item := res.JSON[0]
	if got, _ := item["doc_type_kwd"].(string); got != "image" {
		t.Errorf("doc_type_kwd = %q, want \"image\"", got)
	}
	img, _ := item["image"].(string)
	if !strings.HasPrefix(img, "data:") || !strings.Contains(img, ";base64,") {
		t.Errorf("image = %q, want a data URI (data:<mime>;base64,<b64>)", img)
	}
	if txt, _ := item["text"].(string); txt == "" {
		t.Errorf("text field empty; want non-empty combined OCR+VLM text")
	}
}

// TestMaybeDispatchImage_HardcodesJSONOutput verifies the image family
// always emits json regardless of setup["output_format"]. Python
// rag/app/picture.py:chunk() has no output_format concept — it always
// returns a structured doc. Honoring a "text" override produced a bare
// Text payload that lost the image attachment and set doc_type to "text".
func TestMaybeDispatchImage_HardcodesJSONOutput(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	drv := &imagePromptCaptureDriver{}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["image"]["output_format"] = "text" // legacy/override; must be ignored
	ctx := t.Context()
	res, _, err := maybeDispatchImage(
		ctx,
		dao.DB,
		utility.FileTypeVISUAL,
		"test.png",
		[]byte("not-a-real-image"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchImage: %v", err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json (image family must ignore output_format override)", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1 even when setup says text", len(res.JSON))
	}
}

// audioTranscribeDriver is a mock ModelDriver whose TranscribeAudio returns a
// fixed transcription, so maybeDispatchAudio can be exercised without a real
// ASR provider.
type audioTranscribeDriver struct {
	modelModule.ModelDriver
	transcription string
}

func (d *audioTranscribeDriver) TranscribeAudio(ctx context.Context, _ *string, _ *string, _ *modelModule.APIConfig, _ *modelModule.ASRConfig, _ *common.ModelUsage) (*modelModule.ASRResponse, error) {
	return &modelModule.ASRResponse{Text: d.transcription}, nil
}

// TestMaybeDispatchAudio_JSONCarriesTranscription pins diff 2.11: when the
// audio family's output_format is "json", the ASR transcription must be
// carried in the JSON items (not only in the Text field). Before the fix the
// branch returned Text only with an empty JSON slice, and the Invoke switch
// silently dropped the transcription because it has no "json" branch.
func TestMaybeDispatchAudio_JSONCarriesTranscription(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	const want = "hello world"
	drv := &audioTranscribeDriver{transcription: want}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "asr-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["audio"]["output_format"] = "json"

	ctx := t.Context()
	res, dispatched, err := maybeDispatchAudio(
		ctx,
		dao.DB,
		utility.FileTypeAURAL,
		"test.mp3",
		[]byte("fake-audio"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchAudio: %v", err)
	}
	if !dispatched {
		t.Fatalf("expected dispatched=true for AURAL file")
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1 (transcription must be carried as a JSON item)", len(res.JSON))
	}
	if got, _ := res.JSON[0]["text"].(string); got != want {
		t.Fatalf("JSON[0].text = %q, want %q", got, want)
	}
	if got, _ := res.JSON[0]["doc_type_kwd"].(string); got != "audio" {
		t.Fatalf("JSON[0].doc_type_kwd = %q, want audio", got)
	}
}

// TestMaybeDispatchAudio_TextCarriesTranscription guards the text path: with
// output_format "text" the transcription stays in the Text field and JSON is
// empty (current default after aligning with Python parser.py:232).
func TestMaybeDispatchAudio_TextCarriesTranscription(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	const want = "hello world"
	drv := &audioTranscribeDriver{transcription: want}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "asr-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["audio"]["output_format"] = "text"

	ctx := t.Context()
	res, dispatched, err := maybeDispatchAudio(
		ctx,
		dao.DB,
		utility.FileTypeAURAL,
		"test.mp3",
		[]byte("fake-audio"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchAudio: %v", err)
	}
	if !dispatched {
		t.Fatalf("expected dispatched=true for AURAL file")
	}
	if res.OutputFormat != "text" {
		t.Fatalf("OutputFormat = %q, want text", res.OutputFormat)
	}
	if res.Text != want {
		t.Fatalf("Text = %q, want %q", res.Text, want)
	}
	if len(res.JSON) != 0 {
		t.Fatalf("JSON len = %d, want 0 for text output", len(res.JSON))
	}
}

// TestMaybeDispatchAudio_DefaultOutputFormatJson covers the
// maybeDispatchAudio fallback: when an audio setup omits
// output_format entirely, the dispatch defaults to "json" and wraps
// the transcription as a JSON item. (The defaultSetups value is
// "text" to mirror the Python audio setup in parser.py; this test
// deliberately supplies an empty setup to exercise the fallback
// inside the dispatch itself.)
func TestMaybeDispatchAudio_DefaultOutputFormatJson(t *testing.T) {
	const want = "hello world"
	drv := &audioTranscribeDriver{transcription: want}
	orig := resolveTenantModelByType
	defer func() { resolveTenantModelByType = orig }()
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "asr-model", &modelModule.APIConfig{}, 0, nil
	}
	// No output_format key — exercise the default path inside
	// maybeDispatchAudio.
	setups := map[string]schema.ParserSetup{"audio": {}}
	res, dispatched, err := maybeDispatchAudio(
		context.Background(),
		nil,
		utility.FileTypeAURAL,
		"test.mp3",
		[]byte("fake-audio"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchAudio: %v", err)
	}
	if !dispatched {
		t.Fatal("expected dispatched=true for AURAL file")
	}
	if res.OutputFormat != "json" {
		t.Fatalf("default OutputFormat = %q, want json", res.OutputFormat)
	}
}

// TestDefaultEmailOutputFormatIsJSON pins diff 2.2: the email family default
// output_format must be "json" (matching Python parser.py:212), not "text".
// With "text" the structured email fields (from/to/subject/attachments/...) are
// flattened into a blob and lost downstream.
func TestDefaultEmailOutputFormatIsJSON(t *testing.T) {
	got, _ := defaultSetups()["email"]["output_format"].(string)
	if got != "json" {
		t.Fatalf("email default output_format = %q, want json", got)
	}
}

func TestMaybeDispatchImage_UsesConfiguredVLMModel(t *testing.T) {
	origTenantResolver := resolveTenantModelByType
	origModelResolver := resolveModelConfig
	t.Cleanup(func() {
		resolveTenantModelByType = origTenantResolver
		resolveModelConfig = origModelResolver
	})

	tenantResolverCalled := false
	resolveTenantModelByType = func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		tenantResolverCalled = true
		return nil, "", nil, 0, nil
	}

	var gotRef string
	var gotType entity.ModelType
	drv := &imagePromptCaptureDriver{}
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, modelType entity.ModelType, ref string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		gotRef = ref
		gotType = modelType
		return drv, "custom-vlm", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["image"]["parse_method"] = "custom-vlm@provider"

	_, dispatched, err := maybeDispatchImage(
		t.Context(),
		dao.DB,
		utility.FileTypeVISUAL,
		"test.png",
		[]byte("not-a-real-image"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchImage: %v", err)
	}
	if !dispatched {
		t.Fatal("expected dispatched=true for VISUAL file")
	}
	if gotRef != "custom-vlm@provider" {
		t.Errorf("model ref = %q, want %q", gotRef, "custom-vlm@provider")
	}
	if gotType != entity.ModelTypeImage2Text {
		t.Errorf("model type = %q, want %q", gotType, entity.ModelTypeImage2Text)
	}
	if tenantResolverCalled {
		t.Error("tenant default resolver must not be called when image parse_method names a VLM model")
	}
}

func TestMaybeDispatchAudio_UsesConfiguredModel(t *testing.T) {
	origTenantResolver := resolveTenantModelByType
	origModelResolver := resolveModelConfig
	t.Cleanup(func() {
		resolveTenantModelByType = origTenantResolver
		resolveModelConfig = origModelResolver
	})

	tenantResolverCalled := false
	resolveTenantModelByType = func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		tenantResolverCalled = true
		return nil, "", nil, 0, nil
	}

	var gotRef string
	var gotType entity.ModelType
	drv := &audioTranscribeDriver{transcription: "transcribed"}
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, modelType entity.ModelType, ref string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		gotRef = ref
		gotType = modelType
		return drv, "custom-asr", &modelModule.APIConfig{}, 0, nil
	}

	setups := map[string]schema.ParserSetup{
		"audio": {"vlm": map[string]any{"llm_id": "custom-asr@provider"}},
	}
	res, dispatched, err := maybeDispatchAudio(
		t.Context(),
		dao.DB,
		utility.FileTypeAURAL,
		"test.mp3",
		[]byte("fake-audio"),
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchAudio: %v", err)
	}
	if !dispatched {
		t.Fatal("expected dispatched=true for AURAL file")
	}
	if gotRef != "custom-asr@provider" {
		t.Errorf("model ref = %q, want %q", gotRef, "custom-asr@provider")
	}
	if gotType != entity.ModelTypeSpeech2Text {
		t.Errorf("model type = %q, want %q", gotType, entity.ModelTypeSpeech2Text)
	}
	if tenantResolverCalled {
		t.Error("tenant default resolver must not be called when audio setup vlm.llm_id is set")
	}
	if len(res.JSON) != 1 || res.JSON[0]["text"] != "transcribed" {
		t.Fatalf("unexpected audio result: %#v", res.JSON)
	}
}

// TestImageDecoders_RegisteredFormats validates that image decoders for WebP, BMP,
// TIFF, PNG, JPEG, and GIF are registered by media_dispatch.go and can decode
// their respective binary payloads via image.Decode without importing the decoder
// packages directly in the test file.
func TestImageDecoders_RegisteredFormats(t *testing.T) {
	// Fixed binary fixtures for image formats decoded via decoders registered in
	// media_dispatch.go (neither standard library nor x/image decoders are imported
	// in this test file).
	const (
		webpB64 = "UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoBAAEAD8D+JaQAA3AA/ua1AAA="
		bmpB64  = "Qk1GAAAAAAAAADYAAAAoAAAAAgAAAAIAAAABACAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD/AP8AAP//AAAAAA=="
		tiffB64 = "SUkqABgAAAD/AAD/AAAAAAAAAAAA/wD/DQAAAQMAAQAAAAIAAAABAQMAAQAAAAIAAAACAQMABAAAALoAAAADAQMAAQAAAAEAAAAGAQMAAQAAAAIAAAARAQQAAQAAAAgAAAAVAQMAAQAAAAQAAAAWAQMAAQAAAAIAAAAXAQQAAQAAABAAAAAaAQUAAQAAAMIAAAAbAQUAAQAAAMoAAAAoAQMAAQAAAAIAAABSAQMAAQAAAAEAAAAAAAAACAAIAAgACABIAAAAAQAAAEgAAAABAAAA"
		pngB64  = "iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFElEQVR4nGP8z8Dwn4GBgYGJAQoAHxcCAk+Uzr4AAAAASUVORK5CYII="
		jpegB64 = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQgJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCAACAAIDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDi6KKK+UP38//Z"
		gifB64  = "R0lGODdhAgACAIEAAP8AAAAAAAAAAAAAACwAAAAAAgACAAAIBgABCAQQEAA7"
	)

	tests := []struct {
		format string
		b64    string
	}{
		{"webp", webpB64},
		{"bmp", bmpB64},
		{"tiff", tiffB64},
		{"png", pngB64},
		{"jpeg", jpegB64},
		{"gif", gifB64},
	}

	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			raw, err := base64.StdEncoding.DecodeString(tc.b64)
			if err != nil {
				t.Fatalf("failed to decode test %s base64: %v", tc.format, err)
			}
			decoded, format, err := image.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("image.Decode failed for %s: %v", tc.format, err)
			}
			if format != tc.format {
				t.Errorf("image.Decode format = %q, want %q", format, tc.format)
			}
			if decoded == nil {
				t.Errorf("image.Decode returned nil image for %s", tc.format)
			}
		})
	}
}

// TestVLMGateShouldSkip verifies the rune vs word count threshold
// for skipping VLM description. Specifically:
//   - Zero-allocation rune counting via utf8.RuneCountInString correctly handles multi-byte UTF-8.
//   - Whitespace trimming matches Python txt.strip(): surrounding whitespace is trimmed before counting.
//   - CJK text is measured in unicode runes: 12 CJK characters occupy 36 bytes (>32 bytes)
//     but only 12 runes (<=32 runes), so VLM must NOT be skipped.
//   - CJK text >32 runes (e.g. 33 runes) skips VLM.
//   - CJK exact boundary text (32 runes) triggers VLM.
//   - English text >32 words skips VLM.
//   - English short text (<=32 words and <=32 chars) triggers VLM.
//   - English text with <=32 words but >32 chars skips VLM.
func TestVLMGateShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		ocrText  string
		wantSkip bool
	}{
		{
			name:     "empty text does not skip",
			lang:     "Chinese",
			ocrText:  "",
			wantSkip: false,
		},
		{
			name:     "whitespace only text does not skip",
			lang:     "Chinese",
			ocrText:  "   \n\t  ",
			wantSkip: false,
		},
		{
			name:     "CJK substantial text (>32 runes, >32 bytes) skips VLM",
			lang:     "Chinese",
			ocrText:  strings.Repeat("中", 33), // 33 runes, 99 bytes
			wantSkip: true,
		},
		{
			name:     "CJK short text with >32 bytes but <=32 runes triggers VLM",
			lang:     "Chinese",
			ocrText:  strings.Repeat("中", 12), // 12 runes, 36 bytes (>32 bytes)
			wantSkip: false,
		},
		{
			name:     "CJK exact boundary text (32 runes, 96 bytes) triggers VLM",
			lang:     "Chinese",
			ocrText:  strings.Repeat("中", 32), // 32 runes, 96 bytes (32 is not > 32)
			wantSkip: false,
		},
		{
			name:     "CJK exact boundary text with whitespace padding trims to <=32 runes and triggers VLM",
			lang:     "Chinese",
			ocrText:  "  " + strings.Repeat("中", 32) + "  ", // 32 runes after trim
			wantSkip: false,
		},
		{
			name:     "CJK substantial text with whitespace padding trims to >32 runes and skips VLM",
			lang:     "Chinese",
			ocrText:  "  " + strings.Repeat("中", 33) + "  ", // 33 runes after trim
			wantSkip: true,
		},
		{
			name:     "English substantial text (>32 words) skips VLM",
			lang:     "English",
			ocrText:  strings.Repeat("word ", 33), // 33 words
			wantSkip: true,
		},
		{
			name:     "English short text (<=32 words and <=32 chars) triggers VLM",
			lang:     "English",
			ocrText:  "hello world", // 2 words, 11 chars
			wantSkip: false,
		},
		{
			name:     "English text with <=32 words but >32 chars skips VLM",
			lang:     "English",
			ocrText:  "abcdefghijklmnopqrstuvwxyz01234567", // 1 word, 34 chars (>32 chars)
			wantSkip: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := vlmGateShouldSkip(tc.ocrText, tc.lang)
			if got != tc.wantSkip {
				t.Errorf("vlmGateShouldSkip(%q, %q) = %v, want %v", tc.ocrText, tc.lang, got, tc.wantSkip)
			}
		})
	}
}
