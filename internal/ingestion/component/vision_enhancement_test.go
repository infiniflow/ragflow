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

package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

type visionEnhanceFakeDriver struct {
	modelModule.ModelDriver
}

type visionEnhanceCaptureInvoker struct {
	mu       sync.Mutex
	images   []string
	captured []modelModule.Message
}

func (c *visionEnhanceCaptureInvoker) invoke(
	ctx context.Context,
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	c.mu.Lock()
	c.captured = append(c.captured, messages...)
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
	ans := "```markdown\na diagram of a pipeline\n```"
	return &modelModule.ChatResponse{Answer: &ans}, nil
}

// swapVisionGlobals replaces injectable vars and restores them on t.Cleanup.
func swapVisionGlobals(
	t *testing.T,
	resolver func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error),
	invoker func(context.Context, modelModule.ModelDriver, string, []modelModule.Message, *modelModule.APIConfig) (*modelModule.ChatResponse, error),
	prompt func(string) (string, error),
) {
	t.Helper()
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	origPrompt := figureVisionPromptBuilder
	t.Cleanup(func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
		figureVisionPromptBuilder = origPrompt
	})
	if resolver != nil {
		resolveTenantModelByType = resolver
	}
	if invoker != nil {
		visionChatInvoker = invoker
	}
	if prompt != nil {
		figureVisionPromptBuilder = prompt
	}
}

func fakeResolver(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	return &visionEnhanceFakeDriver{}, "vision-model", &modelModule.APIConfig{}, 0, nil
}

func fakePrompt(language string) (string, error) {
	return "describe the figure in " + language, nil
}

func TestVisionEnhancement_EnhancesJSONImagesAndTables(t *testing.T) {
	testCases := []struct {
		name     string
		fileType utility.FileType
	}{
		{"DOCX", utility.FileTypeDOCX},
		{"PDF", utility.FileTypePDF},
		{"Markdown", utility.FileTypeMarkdown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedLanguage string
			// Per-subtest invoker and prompt — avoids implicit coupling between subtests.
			invoker := &visionEnhanceCaptureInvoker{}
			swapVisionGlobals(t, fakeResolver, invoker.invoke, func(language string) (string, error) {
				capturedLanguage = language
				return "describe the figure in " + language, nil
			})

			dispatched := parserDispatchResult{
				OutputFormat: "json",
				DocType:      string(tc.fileType),
				JSON: []map[string]any{
					{"text": "Intro paragraph", "image": nil, "doc_type_kwd": "text"},
					{"text": "", "image": "aGVsbG8taW1hZ2U=", "doc_type_kwd": "image"},
					{"text": "existing table", "image": "dGFibGUtaW1hZ2U=", "doc_type_kwd": "table"},
					{"text": "<table></table>", "image": nil, "doc_type_kwd": "table"},
				},
			}

			res, handled, err := maybeDispatchVisionEnhancement(
				t.Context(),
				dao.DB,
				tc.fileType,
				dispatched,
				map[string]any{"tenant_id": "t1", "lang": "Japanese"}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatal("handled = false, want true")
			}
			if len(res.JSON) != 4 {
				t.Fatalf("JSON len = %d, want 4", len(res.JSON))
			}
			if got := res.JSON[0]["text"].(string); got != "Intro paragraph" {
				t.Errorf("text item text = %q, want unchanged", got)
			}
			// image item: VLM description cleaned of ```markdown and appended
			if got, _ := res.JSON[1]["text"].(string); got != "a diagram of a pipeline" {
				t.Errorf("image item text = %q, want 'a diagram of a pipeline'", got)
			}
			// table item with image: VLM description appended with single \n
			if got, _ := res.JSON[2]["text"].(string); got != "existing table\na diagram of a pipeline" {
				t.Errorf("table item text = %q, want 'existing table\\na diagram of a pipeline'", got)
			}
			if got, _ := res.JSON[3]["text"].(string); got != "<table></table>" {
				t.Errorf("table item without image text = %q, want unchanged", got)
			}
			if capturedLanguage != "Japanese" {
				t.Errorf("figure prompt language = %q, want Japanese", capturedLanguage)
			}
		})
	}
}

func TestVisionEnhancement_MarkdownOutputUntouched(t *testing.T) {
	called := false
	swapVisionGlobals(t,
		func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
			called = true
			return &visionEnhanceFakeDriver{}, "m", &modelModule.APIConfig{}, 0, nil
		},
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			called = true
			ans := "x"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		nil,
	)

	dispatched := parserDispatchResult{
		OutputFormat: "markdown",
		DocType:      "docx",
		Markdown:     "![Image](data:image/png;base64,abc)",
		File:         map[string]any{"figures": []map[string]any{{"image": "abc", "marker": "x"}}},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestVisionEnhancement_NonAllowedFileTypeSkipped(t *testing.T) {
	dispatched := parserDispatchResult{
		OutputFormat: "json",
		DocType:      "other",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeOTHER,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false for FileTypeOTHER")
	}
	if res.JSON[0]["text"] != "" {
		t.Errorf("text = %q, want empty", res.JSON[0]["text"])
	}
}

func TestVisionEnhancement_EmptyOrNoTenantSkipped(t *testing.T) {
	dispatched := parserDispatchResult{
		OutputFormat: "json",
		DocType:      "docx",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	// No tenant_id
	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{}, nil)
	if err != nil || handled {
		t.Errorf("handled=%v, err=%v, want false, nil for missing tenant_id", handled, err)
	}
	if res.JSON[0]["text"] != "" {
		t.Errorf("text = %q, want untouched", res.JSON[0]["text"])
	}
}

// TestVisionEnhancement_DispatchedErrSkipped ensures a failed parse result is
// returned unchanged — enhancement must not touch items when dispatched.Err != nil.
func TestVisionEnhancement_DispatchedErrSkipped(t *testing.T) {
	parseErr := errors.New("parse failed")
	dispatched := parserDispatchResult{
		Err:          parseErr,
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when dispatched.Err is set")
	}
	if res.Err != parseErr {
		t.Errorf("res.Err = %v, want original parse error preserved", res.Err)
	}
}

// TestVisionEnhancement_ContextCancellation verifies that a cancelled context
// propagates through visionChatInvoker and the enhancement is skipped without panic.
func TestVisionEnhancement_ContextCancellation(t *testing.T) {
	swapVisionGlobals(t, fakeResolver,
		func(ctx context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			return nil, ctx.Err()
		},
		fakePrompt,
	)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		ctx,
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if handled {
		t.Error("handled = true with cancelled context, want false")
	}
	// text field should be untouched since invoker returned no answer.
	if got, _ := res.JSON[0]["text"].(string); got != "" {
		t.Errorf("text = %q, want empty (no successful VLM response)", got)
	}
}

// TestVisionEnhancement_NonStringImageFieldFiltered verifies that items with a
// non-string image field (e.g., int) are silently skipped during target collection.
func TestVisionEnhancement_NonStringImageFieldFiltered(t *testing.T) {
	invokerCalled := false
	swapVisionGlobals(t, fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			invokerCalled = true
			ans := "description"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		fakePrompt,
	)

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			// non-string image field — filtered by target collector
			{"text": "para", "image": 12345, "doc_type_kwd": "image"},
			// nil image — also filtered
			{"text": "para2", "image": nil, "doc_type_kwd": "image"},
		},
	}

	_, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false — no valid image targets")
	}
	if invokerCalled {
		t.Error("invoker was called despite no valid image targets")
	}
}

// TestVisionEnhancement_MoreThanConcurrencyItems verifies that N > visionEnhancementConcurrency
// items are all processed without deadlock.
func TestVisionEnhancement_MoreThanConcurrencyItems(t *testing.T) {
	n := visionEnhancementConcurrency + 5
	swapVisionGlobals(t, fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			ans := "desc"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		fakePrompt,
	)

	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{
			"text":         fmt.Sprintf("item%d", i),
			"image":        "aGVsbG8=",
			"doc_type_kwd": "image",
		}
	}
	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON:         items,
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("handled = false, want true")
	}
	// All n items should have received a description appended.
	for i, item := range res.JSON {
		got, _ := item["text"].(string)
		if got == fmt.Sprintf("item%d", i) {
			t.Errorf("item[%d] text unchanged %q, expected description appended", i, got)
		}
	}
}

// TestVisionEnhancement_PlainTextResponseNotTruncated verifies that a plain-text
// (no fences) VLM response is returned verbatim — cleanMarkdownBlock must not truncate it.
func TestVisionEnhancement_PlainTextResponseNotTruncated(t *testing.T) {
	plain := "A pipeline diagram showing three stages."
	swapVisionGlobals(t, fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			return &modelModule.ChatResponse{Answer: &plain}, nil
		},
		fakePrompt,
	)

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("handled = false, want true")
	}
	if got, _ := res.JSON[0]["text"].(string); got != plain {
		t.Errorf("plain text response = %q, want %q (must not be truncated)", got, plain)
	}
}

// TestVisionEnhancement_PromptBuilderErrorSkipped verifies that a prompt build
// failure skips enhancement silently (best-effort, nilerr is intentional).
func TestVisionEnhancement_PromptBuilderErrorSkipped(t *testing.T) {
	swapVisionGlobals(t,
		fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			t.Error("invoker should not be called when prompt builder fails")
			return nil, nil
		},
		func(_ string) (string, error) {
			return "", errors.New("prompt load failed")
		},
	)

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when prompt builder fails")
	}
	if got, _ := res.JSON[0]["text"].(string); got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

// TestVisionEnhancement_ModelResolveFailureSkipped verifies that a resolver error
// (e.g., tenant has no IMAGE2TEXT model) skips enhancement silently.
func TestVisionEnhancement_ModelResolveFailureSkipped(t *testing.T) {
	swapVisionGlobals(t,
		func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
			return nil, "", nil, 0, errors.New("no model")
		},
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			t.Error("invoker should not be called when model resolve fails")
			return nil, nil
		},
		fakePrompt,
	)

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when model resolve fails")
	}
	if got, _ := res.JSON[0]["text"].(string); got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

// TestVisionEnhancement_CancellationStopsSchedulingWithManyItems verifies that
// cancellation stops scheduling when N > visionEnhancementConcurrency, without deadlock.
func TestVisionEnhancement_CancellationStopsSchedulingWithManyItems(t *testing.T) {
	n := visionEnhancementConcurrency + 5
	swapVisionGlobals(t, fakeResolver,
		func(ctx context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			// Simulate work that respects ctx.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			ans := "desc"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		fakePrompt,
	)

	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{
			"text":         fmt.Sprintf("item%d", i),
			"image":        "aGVsbG8=",
			"doc_type_kwd": "image",
		}
	}
	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON:         items,
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel — dispatch loop should break immediately

	res, handled, err := maybeDispatchVisionEnhancement(
		ctx,
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if handled {
		t.Error("handled = true with cancelled context, want false")
	}
	// No item should be modified when dispatch is cancelled before scheduling.
	for i, item := range res.JSON {
		if got, _ := item["text"].(string); got != fmt.Sprintf("item%d", i) {
			t.Errorf("item[%d] text = %q, want untouched after cancellation", i, got)
		}
	}
}

func TestBuildVisionMessages_PreventsDoublePrefix(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantURL string
	}{
		{
			name:    "raw base64 gets png prefix",
			input:   "aGVsbG8=",
			wantURL: "data:image/png;base64,aGVsbG8=",
		},
		{
			name:    "already png data uri is preserved without double prefix",
			input:   "data:image/png;base64,aGVsbG8=",
			wantURL: "data:image/png;base64,aGVsbG8=",
		},
		{
			name:    "jpeg data uri is preserved as jpeg",
			input:   "data:image/jpeg;base64,/9j/4AAQ",
			wantURL: "data:image/jpeg;base64,/9j/4AAQ",
		},
		{
			name:    "https url is preserved",
			input:   "https://example.com/figure.png",
			wantURL: "https://example.com/figure.png",
		},
		{
			name:    "whitespace trimmed",
			input:   "  data:image/png;base64,abc  ",
			wantURL: "data:image/png;base64,abc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := buildVisionMessages("describe", tc.input)
			if len(msgs) != 1 {
				t.Fatalf("len(msgs) = %d, want 1", len(msgs))
			}
			parts, ok := msgs[0].Content.([]interface{})
			if !ok || len(parts) < 2 {
				t.Fatalf("Content parts = %+v, want >= 2", msgs[0].Content)
			}
			img, ok := parts[1].(map[string]any)
			if !ok {
				t.Fatalf("parts[1] = %+v, want map[string]any", parts[1])
			}
			imgURL, ok := img["image_url"].(map[string]any)
			if !ok {
				t.Fatalf("img[image_url] = %+v, want map[string]any", img["image_url"])
			}
			got, _ := imgURL["url"].(string)
			if got != tc.wantURL {
				t.Errorf("image_url.url = %q, want %q", got, tc.wantURL)
			}
		})
	}
}

func TestExtractVisionAnswer_CleansMarkdownBlock(t *testing.T) {
	ans := "```markdown\n| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1 | Cell 2 |\n```"
	resp := &modelModule.ChatResponse{Answer: &ans}
	got := extractVisionAnswer(resp)
	want := "| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1 | Cell 2 |"
	if got != want {
		t.Errorf("extractVisionAnswer = %q, want %q", got, want)
	}
}

func TestCleanMarkdownBlock_EdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard markdown block",
			input: "```markdown\nHello world\n```",
			want:  "Hello world",
		},
		{
			name:  "windows CRLF endings",
			input: "```markdown\r\nHello world\r\n```",
			want:  "Hello world",
		},
		{
			name:  "nested code blocks preserved",
			input: "```markdown\nText with ```nested``` blocks\n```",
			want:  "Text with ```nested``` blocks",
		},
		{
			name:  "mixed whitespace tabs",
			input: "\t```markdown\t\n\tContent with tabs\n\t```\t",
			want:  "Content with tabs",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only markdown tags",
			input: "```markdown```",
			want:  "",
		},
		{
			name:  "unwrapped text unchanged",
			input: "Just plain text without markdown block",
			want:  "Just plain text without markdown block",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanMarkdownBlock(tc.input)
			if got != tc.want {
				t.Errorf("cleanMarkdownBlock(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPromptDirState_FailureIsSticky(t *testing.T) {
	dir := t.TempDir()
	var state promptDirState
	for i := 0; i < 2; i++ {
		if _, err := state.resolve(dir); err == nil {
			t.Fatalf("call %d: resolve() = nil error, want sticky init error", i+1)
		}
	}
}

func TestPromptDirState_SuccessIsCached(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rag", "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	var state promptDirState
	for i := 0; i < 2; i++ {
		got, err := state.resolve(dir)
		if err != nil {
			t.Fatalf("call %d: resolve() error: %v", i+1, err)
		}
		if got != dir {
			t.Fatalf("call %d: resolve() = %q, want %q", i+1, got, dir)
		}
	}
}

// TestVisionEnhancement_PerCallModelPreferred verifies that setups vlm.llm_id
// takes precedence over the tenant default IMAGE2TEXT model.
func TestVisionEnhancement_PerCallModelPreferred(t *testing.T) {
	origTenantResolver := resolveTenantModelByType
	origModelResolver := resolveModelConfig
	origInvoker := visionChatInvoker
	origPrompt := figureVisionPromptBuilder
	t.Cleanup(func() {
		resolveTenantModelByType = origTenantResolver
		resolveModelConfig = origModelResolver
		visionChatInvoker = origInvoker
		figureVisionPromptBuilder = origPrompt
	})

	tenantResolverCalled := false
	resolveTenantModelByType = func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		tenantResolverCalled = true
		return &visionEnhanceFakeDriver{}, "tenant-model", &modelModule.APIConfig{}, 0, nil
	}

	var gotRef string
	var gotType entity.ModelType
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, modelType entity.ModelType, ref string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		gotRef = ref
		gotType = modelType
		return &visionEnhanceFakeDriver{}, "custom-model", &modelModule.APIConfig{}, 0, nil
	}

	invoker := &visionEnhanceCaptureInvoker{}
	visionChatInvoker = invoker.invoke
	figureVisionPromptBuilder = fakePrompt

	setups := map[string]schema.ParserSetup{
		"pdf": {"vlm": map[string]any{"llm_id": "custom-vlm@provider"}},
	}
	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	_, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if gotRef != "custom-vlm@provider" {
		t.Errorf("resolveModelConfig modelRef = %q, want %q", gotRef, "custom-vlm@provider")
	}
	if gotType != entity.ModelTypeImage2Text {
		t.Errorf("resolveModelConfig modelType = %q, want %q", gotType, entity.ModelTypeImage2Text)
	}
	if tenantResolverCalled {
		t.Error("tenant default resolver must not be called when setup vlm.llm_id is set")
	}
}

func TestVisionEnhancement_InvalidImageDataSkipped(t *testing.T) {
	invokerCalled := false
	swapVisionGlobals(t,
		fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			invokerCalled = true
			return nil, nil
		},
		fakePrompt,
	)

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "keep", "image": "!!!not-base64!!!", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when invalid image data is skipped")
	}
	if invokerCalled {
		t.Error("invoker must not be called for invalid image data")
	}
	if got, _ := res.JSON[0]["text"].(string); got != "keep" {
		t.Errorf("text = %q, want unchanged %q", got, "keep")
	}
}

type deadlineCaptureDriver struct {
	modelModule.ModelDriver
	hasDeadline bool
	remaining   time.Duration
}

func (d *deadlineCaptureDriver) ChatWithMessages(
	ctx context.Context,
	_ string,
	_ []modelModule.Message,
	_ *modelModule.APIConfig,
	_ *modelModule.ChatConfig,
	_ *common.ModelUsage,
) (*modelModule.ChatResponse, error) {
	deadline, ok := ctx.Deadline()
	d.hasDeadline = ok
	if ok {
		d.remaining = time.Until(deadline)
	}
	ans := "ok"
	return &modelModule.ChatResponse{Answer: &ans}, nil
}

func TestDefaultVisionChatInvoker_AppliesDeadline(t *testing.T) {
	drv := &deadlineCaptureDriver{}
	if _, err := defaultVisionChatInvoker(context.Background(), drv, "m", nil, nil); err != nil {
		t.Fatalf("defaultVisionChatInvoker: %v", err)
	}
	if !drv.hasDeadline {
		t.Fatal("vision chat context must have a deadline")
	}
	if drv.remaining <= 0 || drv.remaining > visionChatTimeout+time.Second {
		t.Fatalf("deadline remaining = %v, want ~%v", drv.remaining, visionChatTimeout)
	}
}
