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

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/utility"
)

// docxVisionFakeDriver satisfies modelModule.ModelDriver but never reaches the
// network: docxVisionCaptureInvoker intercepts the call before the driver.
type docxVisionFakeDriver struct {
	modelModule.ModelDriver
}

// docxVisionCaptureInvoker records the requested image and returns a fixed
// description, mirroring the markdown-vision test's capture driver.
type docxVisionCaptureInvoker struct {
	mu       sync.Mutex
	images   []string
	captured []modelModule.Message
}

func (c *docxVisionCaptureInvoker) invoke(
	ctx context.Context,
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	c.mu.Lock()
	c.captured = append(c.captured, messages...)
	// Pull the data URI out of the second content part.
	if parts, ok := messages[0].Content.([]interface{}); ok && len(parts) >= 2 {
		if img, ok := parts[1].(map[string]any); ok {
			if url, ok := img["image_url"].(map[string]any); ok {
				if u, ok := url["url"].(string); ok {
					c.images = append(c.images, u)
				}
			}
		}
	}
	c.mu.Unlock()
	ans := "a diagram of a pipeline"
	return &modelModule.ChatResponse{Answer: &ans}, nil
}

// TestMaybeDispatchDOCXVision_EnhancesJSONImages verifies Diff 2.4: DOCX vision
// enhancement must trigger on the JSON output path (like Python's
// enhance_media_sections_with_vision in parser.py:_doc) and must NOT trigger on
// the markdown path. Image items with a non-empty `image` field get their VLM
// description appended to `text`; table items (no image) and text items are
// left untouched.
func TestMaybeDispatchDOCXVision_EnhancesJSONImages(t *testing.T) {
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	origPrompt := docxVisionPromptBuilder
	defer func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
		docxVisionPromptBuilder = origPrompt
	}()

	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &docxVisionFakeDriver{}, "docx-vision-model", &modelModule.APIConfig{}, 0, nil
	}
	invoker := &docxVisionCaptureInvoker{}
	visionChatInvoker = invoker.invoke
	docxVisionPromptBuilder = func(string, string) (string, error) { return "describe the figure", nil }

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		DocType:      "docx",
		JSON: []map[string]any{
			{"text": "Intro paragraph", "image": nil, "doc_type_kwd": "text"},
			{"text": "", "image": "aGVsbG8taW1hZ2U=", "doc_type_kwd": "image"},
			{"text": "<table></table>", "image": nil, "doc_type_kwd": "table"},
		},
	}

	res, handled, err := maybeDispatchDOCXVision(
		context.Background(),
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{"tenant_id": "t1"},
		defaultSetups(),
	)
	if err != nil {
		t.Fatalf("maybeDispatchDOCXVision: unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true (JSON image item should be enhanced)")
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3", len(res.JSON))
	}
	if got := res.JSON[0]["text"].(string); got != "Intro paragraph" {
		t.Errorf("text item text = %q, want unchanged", got)
	}
	// image item: VLM description appended to (empty) text.
	if got, _ := res.JSON[1]["text"].(string); got != "a diagram of a pipeline" {
		t.Errorf("image item text = %q, want appended VLM description", got)
	}
	if got, _ := res.JSON[2]["text"].(string); got != "<table></table>" {
		t.Errorf("table item text = %q, want unchanged (no image)", got)
	}
	if len(invoker.images) != 1 {
		t.Fatalf("vision invoker called %d times, want 1 (only the image item)", len(invoker.images))
	}
	if want := "data:image/png;base64,aGVsbG8taW1hZ2U="; invoker.images[0] != want {
		t.Errorf("vision image data URI = %q, want %q", invoker.images[0], want)
	}
}

// TestMaybeDispatchDOCXVision_JSONOnly verifies Diff 2.4: the markdown output
// path must NOT be enhanced (Python's markdown/docx branch performs no vision
// enrichment). A markdown result with embedded figures is returned untouched.
func TestMaybeDispatchDOCXVision_JSONOnly(t *testing.T) {
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	defer func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
	}()

	called := false
	resolveTenantModelByType = func(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		called = true
		return &docxVisionFakeDriver{}, "m", &modelModule.APIConfig{}, 0, nil
	}
	visionChatInvoker = func(ctx context.Context, d modelModule.ModelDriver, m string, msgs []modelModule.Message, c *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
		called = true
		ans := "x"
		return &modelModule.ChatResponse{Answer: &ans}, nil
	}

	dispatched := parserDispatchResult{
		OutputFormat: "markdown",
		DocType:      "docx",
		Markdown:     "![Image](data:image/png;base64,abc)",
		File:         map[string]any{"figures": []map[string]any{{"image": "abc", "marker": "x"}}},
	}

	res, handled, err := maybeDispatchDOCXVision(
		context.Background(),
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{"tenant_id": "t1"},
		defaultSetups(),
	)
	if err != nil {
		t.Fatalf("maybeDispatchDOCXVision: unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false (markdown path must not be enhanced)")
	}
	if called {
		t.Error("vision model was resolved/invoked on the markdown path")
	}
	if res.Markdown != "![Image](data:image/png;base64,abc)" {
		t.Errorf("markdown mutated: %q", res.Markdown)
	}
}
