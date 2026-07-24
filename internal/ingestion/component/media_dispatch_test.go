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
	if res.Text == "" {
		t.Fatalf("expected non-empty combined text")
	}

	got, ok := firstUserText(drv.captured)
	if !ok {
		t.Fatalf("no user text captured in VLM messages: %#v", drv.captured)
	}
	if got != "自定义视觉提示" {
		t.Fatalf("VLM user text = %q, want %q (image branch must read system_prompt)", got, "自定义视觉提示")
	}
}
