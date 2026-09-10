//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Dispatch tests pin the routing contract:
//
//   - FileTypeOTHER + missing setups → text-page mode.
//   - FileTypeMarkdown → JSON payload family on the matching output
//     key.
//   - Historical output_format values, including PDF Markdown, are accepted
//     and normalized to the single JSON payload.

package component

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/pdf/type"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

// useMockDocAnalyzer installs a test-only MockDocAnalyzer as the in-process
// DeepDoc backend via the public factory seam. MockDocAnalyzer is test
// infrastructure and must never sit in the production fallback path; it is
// injected here so the production parse path can be exercised without a real
// DeepDoc service or ONNX Runtime models. The factory is reset to nil on
// cleanup (it is nil in this test binary, which registers no real backend).
func useMockDocAnalyzer(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { doctype.SetNativeDocAnalyzerFactory(nil) })
	doctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		return &pdf.MockDocAnalyzer{Healthy: true}, true
	})
}

type captureSetupConfigurer struct {
	setup map[string]any
}

func (c *captureSetupConfigurer) ConfigureFromSetup(setup map[string]any) {
	c.setup = setup
}

func requireJSONText(t *testing.T, out map[string]any, want string) {
	t.Helper()
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	items, ok := out["json"].([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("json = %T/%v, want non-empty structured items", out["json"], out["json"])
	}
	for _, item := range items {
		if text, _ := item["text"].(string); strings.Contains(text, want) {
			return
		}
	}
	t.Fatalf("json payload does not contain %q: %#v", want, items)
}

func TestBuildParserOutputsNormalizesTextFormatsToJSON(t *testing.T) {
	tests := []struct {
		name       string
		dispatched parserDispatchResult
		wantText   string
	}{
		{
			name: "markdown",
			dispatched: parserDispatchResult{
				OutputFormat: "markdown",
				Markdown:     "# Title\n\nBody",
			},
			wantText: "Title",
		},
		{
			name: "html",
			dispatched: parserDispatchResult{
				OutputFormat: "html",
				HTML:         "<h1>Title</h1><p>Body</p>",
			},
			wantText: "Title",
		},
		{
			name: "text",
			dispatched: parserDispatchResult{
				OutputFormat: "text",
				Text:         "plain body",
			},
			wantText: "plain body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buildParserOutputs(t.Context(), tt.dispatched, "sample."+tt.name, utility.FileTypeOTHER, nil, "")

			requireJSONText(t, out, tt.wantText)
			if tt.name == "markdown" {
				items, ok := out["json"].([]map[string]any)
				if !ok {
					t.Fatalf("json = %T, want []map[string]any", out["json"])
				}
				if len(items) != 2 {
					t.Fatalf("markdown json item count = %d, want 2: %#v", len(items), items)
				}
				if got := items[0]["ck_type"]; got != "heading" {
					t.Errorf("markdown item[0].ck_type = %v, want heading", got)
				}
				if got := items[0]["text"]; got != "Title" {
					t.Errorf("markdown item[0].text = %v, want Title", got)
				}
				if got := items[1]["text"]; got != "Body" {
					t.Errorf("markdown item[1].text = %v, want Body", got)
				}
			}
			for _, key := range []string{"markdown", "html", "text"} {
				if _, ok := out[key]; ok {
					t.Errorf("output contains obsolete %q payload: %#v", key, out[key])
				}
			}
		})
	}
}

func TestBuildParserOutputsFallsBackWhenJSONIsEmpty(t *testing.T) {
	out := buildParserOutputs(t.Context(), parserDispatchResult{
		OutputFormat: "json",
		JSON:         []map[string]any{},
		Markdown:     "# Recovered title",
	}, "sample.md", utility.FileTypeMarkdown, nil, "")

	requireJSONText(t, out, "Recovered title")
}

func TestDispatch_JSONOutput(t *testing.T) {
	setups := defaultSetups()
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("# Title\n\nbody\n"),
		"doc_id":    "doc.md",
		"file_type": "md",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "json"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	if got, want := out["file_type"], "md"; got != want {
		t.Errorf("file_type = %v, want %v", got, want)
	}
	jsonItems, ok := out["json"].([]map[string]any)
	if !ok {
		t.Fatalf("json payload missing or wrong type: %T", out["json"])
	}
	if len(jsonItems) == 0 {
		t.Errorf("json payload empty; want at least 1 item")
	}
	// File metadata is carried through dispatch.
	if fm, ok := out["file"].(map[string]any); !ok || fm["name"] != "doc.md" {
		t.Errorf("file metadata missing or wrong: %+v", out["file"])
	}
}

func TestDispatchNormalizesConfiguredOutputFormatToJSON(t *testing.T) {
	setups := defaultSetups()
	setups["email"]["output_format"] = "text"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("From: sender@example.com\r\nTo: receiver@example.com\r\nSubject: Hello\r\n\r\nBody\r\n"),
		"name":      "mail.eml",
		"file_type": "eml",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	if items, ok := out["json"].([]map[string]any); !ok || len(items) == 0 {
		t.Fatalf("json = %T/%v, want non-empty structured items", out["json"], out["json"])
	}
}

// TestDispatch_TextPageMode_NoFileType pins the no-dispatch
// path. When the upstream inputs supply neither file_type nor
// file.name, the component degrades to text-page mode and
// emits structured JSON items. This is the documented behavior for
// canvas-bound invocations that wire the binary directly without
// a family hint.
func TestDispatch_TextPageMode_NoFileType(t *testing.T) {
	setups := defaultSetups()
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary": []byte("plain content\n"),
		"doc_id": "unknown",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "json"; got != want {
		t.Errorf("output_format = %v, want %v (raw-text mode)", got, want)
	}
	jsonItems, ok := out["json"].([]map[string]any)
	if !ok || len(jsonItems) == 0 {
		t.Fatalf("json items missing or empty: %T", out["json"])
	}
	if _, ok := out["text"]; ok {
		t.Fatalf("output contains obsolete text payload: %#v", out["text"])
	}
}

// TestDispatch_SupportedFamilyFailure_HardErrors pins the agreed
// migration rule: once a supported family is identified, parser
// resolution/execution failures must surface as errors instead of
// silently degrading to text-page mode.
func TestDispatch_SupportedFamilyFailure_HardErrors(t *testing.T) {
	setups := defaultSetups()
	c := &ParserComponent{Setups: setups}

	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("PDF payload as bytes (not a real PDF — stub test)\n"),
		"file_type": "pdf",
	})
	if err == nil {
		t.Fatal("Invoke: want error for supported family parse failure, got nil")
	}
	if !strings.Contains(err.Error(), "pdf") {
		t.Errorf("error %q must mention pdf", err.Error())
	}
}

// TestFileTypeFromInputs_ResolutionOrder pins the precedence
// rules documented on parser_dispatch.go:fileTypeFromInputs:
//
//  1. inputs["file_type"]  (explicit family hint)
//  2. inputs["file"].name  (filename in the file descriptor)
//  3. inputs["name"]       (last-resort filename)
//  4. FileTypeOTHER        (text-page mode)
func TestFileTypeFromInputs_ResolutionOrder(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"explicit pdf", map[string]any{"file_type": "pdf"}, "pdf"},
		{"explicit xls (binary)", map[string]any{"file_type": "xls"}, "xls"},
		{"explicit xlsx (OOXML)", map[string]any{"file_type": "xlsx"}, "xlsx"},
		{"explicit ppt (binary)", map[string]any{"file_type": "ppt"}, "ppt"},
		{"explicit pptx (OOXML)", map[string]any{"file_type": "pptx"}, "pptx"},
		{"explicit slides (family name)", map[string]any{"file_type": "slides"}, "pptx"},
		{"explicit spreadsheet (family name)", map[string]any{"file_type": "spreadsheet"}, "xlsx"},
		{"explicit markdown (family form)", map[string]any{"file_type": "markdown"}, "md"},
		{"file.name docx", map[string]any{"file": map[string]any{"name": "report.docx"}}, "docx"},
		{"name fallback md", map[string]any{"name": "notes.md"}, "md"},
		{"unrelated inputs", map[string]any{"binary": []byte("x"), "doc_id": "abc"}, "other"},
		{"unknown family hint", map[string]any{"file_type": "image/xyz"}, "other"},
		{"nil inputs", nil, "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fileTypeFromInputs(tc.in)
			if string(got) != tc.want {
				t.Errorf("fileTypeFromInputs(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDefaultSetups_DOCX_OutputFormatJSON(t *testing.T) {
	setups := defaultSetups()
	docx, ok := setups["docx"]
	if !ok {
		t.Fatal("defaultSetups: docx key missing")
	}
	got, ok := docx["output_format"].(string)
	if !ok {
		t.Fatal("defaultSetups: docx.output_format missing or not a string")
	}
	if got != "json" {
		t.Errorf("docx.output_format = %q, want %q", got, "json")
	}
}
func TestDefaultSetups_Audio_OutputFormatJSON(t *testing.T) {
	setups := defaultSetups()
	audio, ok := setups["audio"]
	if !ok {
		t.Fatal("defaultSetups: audio key missing")
	}
	got, ok := audio["output_format"].(string)
	if !ok {
		t.Fatal("defaultSetups: audio.output_format missing or not a string")
	}
	if got != "json" {
		t.Errorf("audio.output_format = %q, want %q", got, "json")
	}
}

func TestConfigureParserFromSetups_UsesPythonFamilySetup(t *testing.T) {
	setups := defaultSetups()
	got := &captureSetupConfigurer{}

	configureParserFromSetups(got, utility.FileTypePDF, setups)

	want := map[string]any(setups["pdf"])
	if !reflect.DeepEqual(got.setup, want) {
		t.Fatalf("ConfigureFromSetup got %+v, want %+v", got.setup, want)
	}
}

func TestDispatch_PDFLegacyMarkdownConfigurationEmitsJSON(t *testing.T) {
	useMockDocAnalyzer(t)

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	setups := defaultSetups()
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    data,
		"file_type": "pdf",
		"name":      "Doc1.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	if items, ok := out["json"].([]map[string]any); !ok || len(items) == 0 {
		t.Fatalf("json = %T/%v, want non-empty structured items", out["json"], out["json"])
	}
}

func TestDispatch_PDFPlainText_UsesConfiguredBackend(t *testing.T) {
	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "plain_text"
	setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    data,
		"file_type": "pdf",
		"name":      "Doc1.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	jsonItems, ok := out["json"].([]map[string]any)
	if !ok || len(jsonItems) == 0 {
		t.Fatalf("json payload missing or empty: %T", out["json"])
	}
	if got, _ := jsonItems[0]["text"].(string); strings.TrimSpace(got) == "" {
		t.Fatalf("json first item text = %q, want non-empty", got)
	}
}

func TestDispatch_PDFUnsupportedParseMethod_HardErrors(t *testing.T) {
	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "CustomVLM"
	c := &ParserComponent{Setups: setups}

	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "bad.pdf",
	})
	if err == nil {
		t.Fatal("Invoke: want error for unsupported PDF parse_method, got nil")
	}
	if !strings.Contains(err.Error(), "parse_method") || !strings.Contains(err.Error(), "tenant_id") {
		t.Fatalf("error = %q, want parse_method + tenant_id context", err.Error())
	}
}

func TestDispatch_PDFVisionJSON_UsesTenantAwareModel(t *testing.T) {
	origPromptLoader := pdfVisionPromptLoader
	origRenderer := pdfVisionPageRenderer
	origResolver := pdfVisionModelResolver
	origInvoker := pdfVisionChatInvoker
	t.Cleanup(func() {
		pdfVisionPromptLoader = origPromptLoader
		pdfVisionPageRenderer = origRenderer
		pdfVisionModelResolver = origResolver
		pdfVisionChatInvoker = origInvoker
	})

	var prompts []string
	pdfVisionPromptLoader = func(name string) (string, error) {
		if name != "vision_llm_describe_prompt" {
			return "", fmt.Errorf("unexpected prompt %q", name)
		}
		return "Describe page {{ page }}.", nil
	}
	pdfVisionPageRenderer = func(_ []byte) ([]pdfVisionPage, error) {
		return []pdfVisionPage{
			{PageNumber: 1, WidthPts: 100, HeightPts: 200, ImageURL: "data:image/png;base64,aaa"},
			{PageNumber: 2, WidthPts: 120, HeightPts: 240, ImageURL: "data:image/png;base64,bbb"},
		}, nil
	}
	pdfVisionModelResolver = func(ctx context.Context, db *gorm.DB, tenantID string, modelID string) (models.ModelDriver, string, *models.APIConfig, error) {
		if tenantID != "tenant-1" || modelID != "CustomVLM" {
			return nil, "", nil, fmt.Errorf("resolver got tenant/model %q/%q", tenantID, modelID)
		}
		return nil, "resolved-vlm", nil, nil
	}
	pdfVisionChatInvoker = func(ctx context.Context, _ models.ModelDriver, modelName string, messages []models.Message, _ *models.APIConfig) (*models.ChatResponse, error) {
		if modelName != "resolved-vlm" {
			return nil, fmt.Errorf("modelName = %q, want resolved-vlm", modelName)
		}
		if len(messages) != 1 {
			return nil, fmt.Errorf("messages len = %d, want 1", len(messages))
		}
		content, ok := messages[0].Content.([]interface{})
		if !ok || len(content) != 2 {
			return nil, fmt.Errorf("content = %#v, want multimodal prompt+image", messages[0].Content)
		}
		block, ok := content[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("content[0] = %T, want map[string]any", content[0])
		}
		prompt, _ := block["text"].(string)
		prompts = append(prompts, prompt)
		answer := "Transcribed " + prompt
		return &models.ChatResponse{Answer: &answer}, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "CustomVLM"
	setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "vision.pdf",
		"tenant_id": "tenant-1",
	})
	if err == nil {
		jsonItems, ok := out["json"].([]map[string]any)
		if !ok || len(jsonItems) != 2 {
			t.Fatalf("json payload = %#v, want 2 items", out["json"])
		}
		if got, want := jsonItems[0]["page_number"], 1; got != want {
			t.Fatalf("json[0].page_number = %v, want %v", got, want)
		}
		if positions, ok := jsonItems[0]["_pdf_positions"].([][]any); !ok || len(positions) != 1 {
			t.Fatalf("json[0]._pdf_positions = %#v, want one normalized page box", jsonItems[0]["_pdf_positions"])
		}
		if file, ok := out["file"].(map[string]any); !ok || file["parse_method"] != "CustomVLM" || file["page_count"] != 2 {
			t.Fatalf("file metadata = %#v, want parse_method/page_count", out["file"])
		}
		if len(prompts) != 2 || !strings.Contains(prompts[0], "page 1") || !strings.Contains(prompts[1], "page 2") {
			t.Fatalf("prompts = %#v, want rendered page-specific prompts", prompts)
		}
		return
	}
	t.Fatalf("Invoke: %v", err)
}

func TestDispatch_PDFVisionJSON_PreservesEmptyPages(t *testing.T) {
	origPromptLoader := pdfVisionPromptLoader
	origRenderer := pdfVisionPageRenderer
	origResolver := pdfVisionModelResolver
	origInvoker := pdfVisionChatInvoker
	t.Cleanup(func() {
		pdfVisionPromptLoader = origPromptLoader
		pdfVisionPageRenderer = origRenderer
		pdfVisionModelResolver = origResolver
		pdfVisionChatInvoker = origInvoker
	})

	pdfVisionPromptLoader = func(string) (string, error) { return "Describe page {{ page }}.", nil }
	pdfVisionPageRenderer = func(_ []byte) ([]pdfVisionPage, error) {
		return []pdfVisionPage{
			{PageNumber: 1, WidthPts: 100, HeightPts: 200, ImageURL: "data:image/png;base64,aaa"},
			{PageNumber: 2, WidthPts: 120, HeightPts: 240, ImageURL: "data:image/png;base64,bbb"},
		}, nil
	}
	pdfVisionModelResolver = func(ctx context.Context, db *gorm.DB, tenantID string, modelID string) (models.ModelDriver, string, *models.APIConfig, error) {
		return nil, "resolved-vlm", nil, nil
	}
	call := 0
	pdfVisionChatInvoker = func(ctx context.Context, _ models.ModelDriver, _ string, _ []models.Message, _ *models.APIConfig) (*models.ChatResponse, error) {
		call++
		answer := ""
		if call == 1 {
			answer = "First page"
		}
		return &models.ChatResponse{Answer: &answer}, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "CustomVLM"
	setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "vision.pdf",
		"tenant_id": "tenant-1",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	jsonItems, ok := out["json"].([]map[string]any)
	if !ok || len(jsonItems) != 2 {
		t.Fatalf("json payload = %#v, want 2 items", out["json"])
	}
	if got := jsonItems[1]["text"]; got != "" {
		t.Fatalf("json[1].text = %#v, want empty string placeholder", got)
	}
}

func TestDispatch_PDFMinerUMarkdown_UsesConfiguredBackend(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/file_parse" {
			buf := new(bytes.Buffer)
			zw := zip.NewWriter(buf)
			f, _ := zw.Create("content_list.json")
			_, _ = f.Write([]byte(`[{"type":"text","text":"# Title\n\nBody\n"}]`))
			_ = zw.Close()
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buf.Bytes())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Mock resolveTenantModelByType to return a MinerU driver pointing at the test server.
	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()
	baseURL := server.URL
	apiKey := ""
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (models.ModelDriver, string, *models.APIConfig, int, error) {
		return &mineruTestDriver{}, "mineru-model", &models.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, 0, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "mineru"
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	requireJSONText(t, out, "Title")
}

func TestDispatch_PDFMinerUJSON_ParsesMarkdownToStructuredItems(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/file_parse" {
			buf := new(bytes.Buffer)
			zw := zip.NewWriter(buf)
			f, _ := zw.Create("content_list.json")
			_, _ = f.Write([]byte(`[{"type":"text","text":"# Title\n\nBody paragraph\n"}]`))
			_ = zw.Close()
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buf.Bytes())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origResolver := resolveTenantModelByType
	defer func() { resolveTenantModelByType = origResolver }()
	baseURL := server.URL
	apiKey := ""
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (models.ModelDriver, string, *models.APIConfig, int, error) {
		return &mineruTestDriver{}, "mineru-model", &models.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, 0, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "mineru"
	setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "json"; got != want {
		t.Fatalf("output_format = %v, want %v", got, want)
	}
	items, ok := out["json"].([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("json payload = %#v, want non-empty structured items", out["json"])
	}
}

func TestDispatch_PDFMonkeyOCRv2Markdown_UsesNativeParseEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/parse" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("start_page_id") != "0" || r.FormValue("end_page_id") != "99999" {
			t.Fatalf("page range = %q:%q", r.FormValue("start_page_id"), r.FormValue("end_page_id"))
		}
		file, _, err := r.FormFile("files")
		if err != nil {
			t.Fatal(err)
		}
		_ = file.Close()

		var output bytes.Buffer
		archive := zip.NewWriter(&output)
		entry, _ := archive.Create("sample/jsons/sample.json")
		_, _ = entry.Write([]byte(`[{"label":"Title","content":"MonkeyOCRv2 title"}]`))
		_ = archive.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(output.Bytes())
	}))
	defer server.Close()

	original := resolveTenantOCRModelByProvider
	t.Cleanup(func() { resolveTenantOCRModelByProvider = original })
	resolveTenantOCRModelByProvider = func(_ context.Context, _ *gorm.DB, tenantID, providerName string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		if tenantID != "test-tenant" || providerName != "MonkeyOCRv2" {
			t.Fatalf("tenant=%q provider=%q", tenantID, providerName)
		}
		return &monkeyOCRv2FakeDriver{}, "MonkeyOCRv2-Parsing", &models.APIConfig{BaseURL: &server.URL}, 0, nil
	}

	component, err := NewParserComponent(map[string]any{
		"pdf": map[string]any{"parse_method": "monkeyocrv2", "output_format": "markdown"},
	})
	if err != nil {
		t.Fatalf("NewParserComponent: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	requireJSONText(t, out, "MonkeyOCRv2 title")
}

// mineruTestDriver is a minimal ModelDriver mock whose Name() returns "mineru".
type mineruTestDriver struct{}

func (d *mineruTestDriver) NewInstance(baseURL map[string]string) models.ModelDriver { return d }
func (d *mineruTestDriver) Name() string                                             { return "mineru" }
func (d *mineruTestDriver) ChatWithMessages(ctx context.Context, modelName string, messages []models.Message, apiConfig *models.APIConfig, chatModelConfig *models.ChatConfig, usage *common.ModelUsage) (*models.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []models.Message, apiConfig *models.APIConfig, modelConfig *models.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) Embed(ctx context.Context, modelName *string, request models.EmbedRequest, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig, usage *common.ModelUsage) ([]models.EmbeddingData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *mineruTestDriver) Rerank(ctx context.Context, modelName *string, request models.RerankRequest, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig, usage *common.ModelUsage) (*models.RerankResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, usage *common.ModelUsage) (*models.ASRResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, usage *common.ModelUsage) (*models.TTSResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *models.APIConfig, ocrConfig *models.OCRConfig, usage *common.ModelUsage) (*models.OCRFileResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *models.APIConfig, parseFileConfig *models.ParseFileConfig, usage *common.ModelUsage) (*models.ParseFileResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) ListModels(ctx context.Context, apiConfig *models.APIConfig) ([]models.ListModelResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) Balance(ctx context.Context, apiConfig *models.APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) CheckConnection(ctx context.Context, apiConfig *models.APIConfig) error {
	return fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) ListTasks(ctx context.Context, apiConfig *models.APIConfig) ([]models.ListTaskStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *mineruTestDriver) ShowTask(ctx context.Context, taskID string, apiConfig *models.APIConfig) (*models.TaskResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestDispatch_PDFPaddleOCRMarkdown_UsesTenantModel(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer paddle-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Paddle Title\n\nPaddle body.\n"}}]}}`))
	}))
	defer server.Close()

	// Mock resolveTenantOCRModelByProvider to return a PaddleOCR driver
	// pointing at the test server.
	origResolver := resolveTenantOCRModelByProvider
	defer func() { resolveTenantOCRModelByProvider = origResolver }()
	baseURL := server.URL
	apiKey := "paddle-secret"
	resolveTenantOCRModelByProvider = func(ctx context.Context, db *gorm.DB, tenantID string, providerName string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		if got, want := providerName, "PaddleOCR"; got != want {
			t.Fatalf("providerName = %q, want %q", got, want)
		}
		return &paddleocrTestDriver{}, "PaddleOCR-VL", &models.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, 0, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "PaddleOCR"
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	requireJSONText(t, out, "Paddle Title")
}

// TestDispatch_PDFPaddleOCRMarkdown_UsesAPIKeyPayload pins the cloud
// PaddleOCR configuration contract: the tenant api_key is a JSON payload
// (paddleocr_api_url / paddleocr_access_token / paddleocr_algorithm) and the
// instance base_url field stays empty, mirroring Python's PaddleOCROcrModel.
// Dispatch must unwrap that payload into a concrete base url, bearer token and
// algorithm before handing the driver its API config.
func TestDispatch_PDFPaddleOCRMarkdown_UsesAPIKeyPayload(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer tok-123"; got != want {
			t.Errorf("Authorization = %q, want %q (must unwrap api_key payload)", got, want)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if got, want := body["algorithm"], "PaddleOCR-VL"; got != want {
			t.Errorf("algorithm = %v, want %v (must unwrap api_key payload)", got, want)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Unwrapped Title\n\nUnwrapped body.\n"}}]}}`))
	}))
	defer server.Close()

	origResolver := resolveTenantOCRModelByProvider
	defer func() { resolveTenantOCRModelByProvider = origResolver }()
	apiKey := fmt.Sprintf(
		`{"paddleocr_api_url":%q,"paddleocr_access_token":"tok-123","paddleocr_algorithm":"PaddleOCR-VL"}`,
		server.URL+"/api")
	emptyBaseURL := ""
	resolveTenantOCRModelByProvider = func(ctx context.Context, db *gorm.DB, tenantID string, providerName string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		if got, want := providerName, "PaddleOCR"; got != want {
			t.Fatalf("providerName = %q, want %q", got, want)
		}
		return &paddleocrTestDriver{}, "PaddleOCR-VL", &models.APIConfig{ApiKey: &apiKey, BaseURL: &emptyBaseURL}, 0, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "PaddleOCR"
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	requireJSONText(t, out, "Unwrapped Title")
}

func TestDispatch_PDFPaddleOCR_NoTenantModel_HardErrors(t *testing.T) {
	withSSRFBypass(t)
	origResolver := resolveTenantOCRModelByProvider
	defer func() { resolveTenantOCRModelByProvider = origResolver }()
	resolveTenantOCRModelByProvider = func(ctx context.Context, db *gorm.DB, tenantID string, providerName string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		return nil, "", nil, 0, fmt.Errorf("no active PaddleOCR OCR model")
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "paddleocr"
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err == nil || !strings.Contains(err.Error(), "parser: PaddleOCR model") {
		t.Fatalf("Invoke error = %v, want PaddleOCR model error", err)
	}
}

// TestDispatch_PDFPaddleOCR_BareModelUUID_UsesExactModel pins the routing of
// a bare tenant model UUID in layout_recognizer — the value the web UI writes
// when a user picks an OCR model for PDF parsing — to the PaddleOCR dispatch
// path. The raw UUID carries no "@provider" hint in the string, so it must be
// resolved first (mirroring Python's get_composite_model_name_by_id before
// normalize_layout_recognizer). Previously the UUID fell through to the
// image2text VLM path and failed with "cannot be used as image2text model"
// for OCR-typed models such as the cloud "PaddleOCR" provider's.
func TestDispatch_PDFPaddleOCR_BareModelUUID_UsesExactModel(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer paddle-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Cloud Paddle Title\n\nCloud body.\n"}}]}}`))
	}))
	defer server.Close()

	origIsLayout := isPaddleOCRLayoutModelID
	origResolve := resolvePaddleOCRModelForDispatch
	defer func() {
		isPaddleOCRLayoutModelID = origIsLayout
		resolvePaddleOCRModelForDispatch = origResolve
	}()

	modelID := "d13ffec6c1e34b1abc30e540b692d83d"
	isPaddleOCRLayoutModelID = func(ctx context.Context, db *gorm.DB, tenantID, layout string) bool {
		if got, want := layout, modelID; got != want {
			t.Fatalf("layout = %q, want %q", got, want)
		}
		return true
	}
	baseURL := server.URL
	apiKey := "paddle-secret"
	resolvePaddleOCRModelForDispatch = func(ctx context.Context, db *gorm.DB, tenantID, mid string) (models.ModelDriver, string, *models.APIConfig, error) {
		if got, want := mid, modelID; got != want {
			t.Fatalf("modelID = %q, want %q", got, want)
		}
		return &paddleocrTestDriver{}, "PaddleOCR-VL", &models.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, nil
	}

	setups := defaultSetups()
	setups["pdf"]["layout_recognizer"] = modelID
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	requireJSONText(t, out, "Cloud Paddle Title")
}

// TestDispatch_PDFPaddleOCR_BareModelUUID_InParseMethod pins the routing of a
// bare tenant model UUID carried in parse_method (with layout_recognizer
// empty) to the PaddleOCR dispatch path. Previously only layout_recognizer
// was probed for a UUID, so a UUID in parse_method fell through to the
// image2text VLM path and failed with "cannot be used as image2text model".
func TestDispatch_PDFPaddleOCR_BareModelUUID_InParseMethod(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer paddle-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Cloud Paddle Title\n\nCloud body.\n"}}]}}`))
	}))
	defer server.Close()

	origIsLayout := isPaddleOCRLayoutModelID
	origResolve := resolvePaddleOCRModelForDispatch
	defer func() {
		isPaddleOCRLayoutModelID = origIsLayout
		resolvePaddleOCRModelForDispatch = origResolve
	}()

	modelID := "d13ffec6c1e34b1abc30e540b692d83d"
	isPaddleOCRLayoutModelID = func(ctx context.Context, db *gorm.DB, tenantID, selector string) bool {
		if got, want := selector, modelID; got != want {
			t.Fatalf("selector = %q, want %q", got, want)
		}
		return true
	}
	baseURL := server.URL
	apiKey := "paddle-secret"
	resolvePaddleOCRModelForDispatch = func(ctx context.Context, db *gorm.DB, tenantID, mid string) (models.ModelDriver, string, *models.APIConfig, error) {
		if got, want := mid, modelID; got != want {
			t.Fatalf("modelID = %q, want %q", got, want)
		}
		return &paddleocrTestDriver{}, "PaddleOCR-VL", &models.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, nil
	}

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = modelID
	setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
		"tenant_id": "test-tenant",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	requireJSONText(t, out, "Cloud Paddle Title")
}

// paddleocrTestDriver is a minimal ModelDriver mock whose Name() returns "paddleocr".
type paddleocrTestDriver struct{}

func (d *paddleocrTestDriver) NewInstance(baseURL map[string]string) models.ModelDriver { return d }
func (d *paddleocrTestDriver) Name() string                                             { return "paddleocr" }
func (d *paddleocrTestDriver) ChatWithMessages(ctx context.Context, modelName string, messages []models.Message, apiConfig *models.APIConfig, chatModelConfig *models.ChatConfig, usage *common.ModelUsage) (*models.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []models.Message, apiConfig *models.APIConfig, modelConfig *models.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) Embed(ctx context.Context, modelName *string, request models.EmbedRequest, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig, usage *common.ModelUsage) ([]models.EmbeddingData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *paddleocrTestDriver) Rerank(ctx context.Context, modelName *string, request models.RerankRequest, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig, usage *common.ModelUsage) (*models.RerankResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, usage *common.ModelUsage) (*models.ASRResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, usage *common.ModelUsage) (*models.TTSResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}

// OCRFile mimics the local PaddleOCRLocalModel protocol: a synchronous
// JSON POST to {baseURL}/layout-parsing carrying the file as base64, with
// Bearer auth when the API config provides a key.
func (d *paddleocrTestDriver) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *models.APIConfig, ocrConfig *models.OCRConfig, usage *common.ModelUsage) (*models.OCRFileResponse, error) {
	if apiConfig == nil || apiConfig.BaseURL == nil || *apiConfig.BaseURL == "" {
		return nil, fmt.Errorf("missing base url")
	}
	endpoint := strings.TrimRight(*apiConfig.BaseURL, "/") + "/layout-parsing"
	reqData := map[string]any{
		"file":     base64.StdEncoding.EncodeToString(content),
		"fileType": 0,
	}
	if ocrConfig != nil && strings.TrimSpace(ocrConfig.Algorithm) != "" {
		reqData["algorithm"] = ocrConfig.Algorithm
	}
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := models.BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var ocrResp struct {
		Result struct {
			LayoutParsingResults []struct {
				Markdown struct {
					Text string `json:"text"`
				} `json:"markdown"`
			} `json:"layoutParsingResults"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &ocrResp); err != nil {
		return nil, err
	}
	var md strings.Builder
	for _, lr := range ocrResp.Result.LayoutParsingResults {
		if lr.Markdown.Text != "" {
			md.WriteString(lr.Markdown.Text)
			md.WriteString("\n\n")
		}
	}
	text := strings.TrimSpace(md.String())
	return &models.OCRFileResponse{Text: &text}, nil
}
func (d *paddleocrTestDriver) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *models.APIConfig, parseFileConfig *models.ParseFileConfig, usage *common.ModelUsage) (*models.ParseFileResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) ListModels(ctx context.Context, apiConfig *models.APIConfig) ([]models.ListModelResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) Balance(ctx context.Context, apiConfig *models.APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) CheckConnection(ctx context.Context, apiConfig *models.APIConfig) error {
	return fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) ListTasks(ctx context.Context, apiConfig *models.APIConfig) ([]models.ListTaskStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *paddleocrTestDriver) ShowTask(ctx context.Context, taskID string, apiConfig *models.APIConfig) (*models.TaskResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestIsPaddleOCRDriver(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    models.ModelDriver
		want bool
	}{
		{"local", &paddleocrTestDriver{}, true},
		{"remote", &models.PaddleOCRModel{}, true},
		{"dummy", &models.DummyModel{}, false},
	} {
		if got := isPaddleOCRDriver(tc.d); got != tc.want {
			t.Errorf("isPaddleOCRDriver(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDispatch_PDFDoclingMarkdown_UsesConfiguredBackend(t *testing.T) {
	withSSRFBypass(t)
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got, want := r.Header.Get("Authorization"), "Bearer doc-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/convert/source" && requestCount == 1:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"chunking unsupported"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1alpha/convert/source" && requestCount == 2:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"chunking unsupported"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/convert/source" && requestCount == 3:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"document":{"md_content":"# Docling Title\n\nDocling body.\n"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "Docling"
	setups["pdf"]["output_format"] = "markdown"
	setups["pdf"]["docling_server_url"] = server.URL
	setups["pdf"]["docling_api_key"] = "doc-secret"
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}
	requireJSONText(t, out, "Docling Title")
	if got, want := requestCount, 3; got != want {
		t.Fatalf("requestCount = %d, want %d", got, want)
	}
}

func TestDispatch_PDFOpenDataLoaderLegacyMarkdownEmitsJSON(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/file_parse" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json_doc":null,"md_text":"# ODL Title\n\nODL body.\n"}`))
	}))
	defer server.Close()

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "OpenDataLoader"
	setups["pdf"]["output_format"] = "markdown"
	setups["pdf"]["opendataloader_apiserver"] = server.URL
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	requireJSONText(t, out, "ODL Title")
}

func TestDispatch_PDFSoMarkLegacyMarkdownEmitsJSON(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-4"}}`))
		case "/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"title","content":"SoMark Title","title_level":1},{"type":"text","content":"Body"}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "SoMark"
	setups["pdf"]["output_format"] = "markdown"
	setups["pdf"]["somark_base_url"] = server.URL
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	requireJSONText(t, out, "SoMark Title")
}

func TestDispatch_PDFTCADPLegacyMarkdownEmitsJSON(t *testing.T) {
	withSSRFBypass(t)
	zipPayload := tcadpZipFixtureForComponent(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setups := defaultSetups()
	setups["pdf"]["parse_method"] = "TCADP parser"
	setups["pdf"]["output_format"] = "markdown"
	setups["pdf"]["tcadp_apiserver"] = server.URL
	c := &ParserComponent{Setups: setups}

	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	requireJSONText(t, out, "Hello TCADP")
}

func tcadpZipFixtureForComponent(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	f1, err := writer.Create("result.md")
	if err != nil {
		t.Fatalf("Create md: %v", err)
	}
	_, _ = f1.Write([]byte("Hello TCADP"))
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}
	return buf.Bytes()
}

// TestPythonFamilyName_FileTypeConstants pins the contract that every
// utility.FileType semantic constant resolves to the setups key used by
// defaultSetups. Before the fix, FileTypeVISUAL mapped
// to "picture" (mismatching the "image" setups key) and FileTypeAURAL matched
// no case at all (returning ""), so output_format validation and
// configureParserFromSetups were silently skipped for image/audio files.
func TestPythonFamilyName_FileTypeConstants(t *testing.T) {
	cases := []struct {
		ft   utility.FileType
		want string
	}{
		{utility.FileTypePDF, "pdf"},
		{utility.FileTypeDOC, "doc"},
		{utility.FileTypeDOCX, "docx"},
		{utility.FileTypePPT, "slides"},
		{utility.FileTypePPTX, "slides"},
		{utility.FileTypeXLS, "spreadsheet"},
		{utility.FileTypeXLSX, "spreadsheet"},
		{utility.FileTypeCSV, "spreadsheet"},
		{utility.FileTypeHTML, "html"},
		{utility.FileTypeMarkdown, "markdown"},
		{utility.FileTypeTXT, "text&code"},
		{utility.FileTypeEPUB, "epub"},
		{utility.FileTypeJSON, "json"},
		{utility.FileTypeVISUAL, "image"},
		{utility.FileTypeAURAL, "audio"},
		{utility.FileTypeVIDEO, "video"},
		{utility.FileTypeEMAIL, "email"},
	}
	for _, c := range cases {
		got := pythonFamilyName(string(c.ft))
		if got != c.want {
			t.Errorf("pythonFamilyName(%q) = %q, want %q", c.ft, got, c.want)
		}
		// Every mapped family must have a matching setup so its parser receives
		// the canonical output format and family-specific configuration.
		if _, ok := defaultSetups()[got]; !ok {
			t.Errorf("pythonFamilyName(%q) → %q has no defaultSetups entry", c.ft, got)
		}
	}
}

// TestConfigureParserFromSetups_VisualAural pins that image and audio
// files pick up their setup (parse_method / lang / vlm) via the family
// mapping. Before the fix, configureParserFromSetups silently skipped
// configuration because resolveParserFamily returned "picture" / "aural",
// neither of which existed in defaultSetups.
func TestConfigureParserFromSetups_VisualAural(t *testing.T) {
	setups := defaultSetups()

	t.Run("visual resolves to image setup", func(t *testing.T) {
		got := &captureSetupConfigurer{}
		configureParserFromSetups(got, utility.FileTypeVISUAL, setups)
		want := map[string]any(setups["image"])
		if got.setup == nil {
			t.Fatal("ConfigureFromSetup not called for FileTypeVISUAL")
		}
		if v, _ := got.setup["parse_method"].(string); v != want["parse_method"] {
			t.Errorf("FileTypeVISUAL parse_method = %v, want %v", v, want["parse_method"])
		}
	})

	t.Run("aural resolves to audio setup", func(t *testing.T) {
		got := &captureSetupConfigurer{}
		configureParserFromSetups(got, utility.FileTypeAURAL, setups)
		want := map[string]any(setups["audio"])
		if got.setup == nil {
			t.Fatal("ConfigureFromSetup not called for FileTypeAURAL")
		}
		if v, _ := got.setup["output_format"].(string); v != want["output_format"] {
			t.Errorf("FileTypeAURAL output_format = %v, want %v", v, want["output_format"])
		}
	})
}
