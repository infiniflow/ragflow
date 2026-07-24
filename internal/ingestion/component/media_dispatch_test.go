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
	"context"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
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
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	// image family's contract key is system_prompt (parser.go:295).
	// Also set a legacy `prompt` sentinel to assert system_prompt wins
	// when both keys are present (regression guard against re-reading
	// setup["prompt"], which is the video-family key, not image).
	setups["image"]["prompt"] = "legacy prompt"
	setups["image"]["system_prompt"] = "自定义视觉提示"

	res, dispatched, err := maybeDispatchImage(
		context.Background(),
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
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	res, dispatched, err := maybeDispatchImage(
		context.Background(),
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
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["image"]["output_format"] = "text" // legacy/override; must be ignored
	res, _, err := maybeDispatchImage(
		context.Background(),
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
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "asr-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["audio"]["output_format"] = "json"

	res, dispatched, err := maybeDispatchAudio(
		context.Background(),
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
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "asr-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["audio"]["output_format"] = "text"

	res, dispatched, err := maybeDispatchAudio(
		context.Background(),
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

// TestMaybeDispatchMarkdownVision_EnhancesTables pins diff 2.5: markdown
// vision enhancement must also process items whose doc_type_kwd is "table"
// (Python checks {"image","table"} in parser/utils.py:181), not only "image".
// Before the fix the table item was skipped and never sent to the VLM.
func TestMaybeDispatchMarkdownVision_EnhancesTables(t *testing.T) {
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()

	drv := &imagePromptCaptureDriver{}
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"doc_type_kwd": "table", "image": "base64table", "text": ""},
		},
	}

	res, handled, err := maybeDispatchMarkdownVision(
		context.Background(),
		utility.FileTypeMarkdown,
		dispatched,
		map[string]any{"tenant_id": "t1"},
	)
	if err != nil {
		t.Fatalf("maybeDispatchMarkdownVision: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true for markdown with a table image")
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	// The table item must have been sent to the VLM and its description appended.
	if got, _ := res.JSON[0]["text"].(string); got != "captured" {
		t.Fatalf("table item text = %q, want %q (table items must be vision-enhanced)", got, "captured")
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
