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
	"sync"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
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

func TestVisionEnhancement_EnhancesJSONImagesAndTables(t *testing.T) {
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	origPrompt := figureVisionPromptBuilder
	t.Cleanup(func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
		figureVisionPromptBuilder = origPrompt
	})

	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &visionEnhanceFakeDriver{}, "vision-model", &modelModule.APIConfig{}, 0, nil
	}
	invoker := &visionEnhanceCaptureInvoker{}
	visionChatInvoker = invoker.invoke
	var capturedLanguage string
	figureVisionPromptBuilder = func(_, _, language string) (string, error) {
		capturedLanguage = language
		return "describe the figure in " + language, nil
	}

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
				map[string]any{"tenant_id": "t1", "lang": "Japanese"},
			)
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
		})
	}
	if capturedLanguage != "Japanese" {
		t.Errorf("figure prompt language = %q, want Japanese", capturedLanguage)
	}
}

func TestVisionEnhancement_MarkdownOutputUntouched(t *testing.T) {
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	t.Cleanup(func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
	})

	called := false
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		called = true
		return &visionEnhanceFakeDriver{}, "m", &modelModule.APIConfig{}, 0, nil
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

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{"tenant_id": "t1"},
	)
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
		map[string]any{"tenant_id": "t1"},
	)
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
		map[string]any{},
	)
	if err != nil || handled {
		t.Errorf("handled=%v, err=%v, want false, nil for missing tenant_id", handled, err)
	}
	if res.JSON[0]["text"] != "" {
		t.Errorf("text = %q, want untouched", res.JSON[0]["text"])
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
