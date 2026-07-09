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
//     key, with the pages slice preserved.
//   - FileTypePDF + setups["pdf"].output_format set to a value not
//     in allowed_output_format["pdf"] → component errors with the
//     format-mismatch message (matches the Python check() behavior).

package component

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

type captureSetupConfigurer struct {
	setup map[string]any
}

func (c *captureSetupConfigurer) ConfigureFromSetup(setup map[string]any) {
	c.setup = setup
}

// TestDispatch_OutputFormatValidation_Allowed is the happy-path
// pin: a Markdown file with output_format=json passes the
// allowed_output_format check and runs the structured dispatch.
func TestDispatch_OutputFormatValidation_Allowed(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	// Defaults already include markdown → {text, json}.
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
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
	jsonItems, ok := out["json"].([]map[string]any)
	if !ok {
		t.Fatalf("json payload missing or wrong type: %T", out["json"])
	}
	if len(jsonItems) == 0 {
		t.Errorf("json payload empty; want at least 1 item")
	}
	// Pages must still exist for chunker-side consumers.
	pages, ok := out["pages"].([]schema.Page)
	if !ok || len(pages) == 0 {
		t.Errorf("pages slice missing or empty: %T", out["pages"])
	}
	if ok && len(pages) > 0 {
		if got, _ := pages[0]["text"].(string); !strings.Contains(got, "Title") {
			t.Errorf("pages[0].text = %q, want content containing Title", got)
		}
	}
	// File metadata is carried through dispatch.
	if fm, ok := out["file"].(map[string]any); !ok || fm["name"] != "doc.md" {
		t.Errorf("file metadata missing or wrong: %+v", out["file"])
	}
}

// TestDispatch_OutputFormatValidation_Rejection pins the
// whitelist enforcement: a request for output_format=html on the
// markdown family is rejected because markdown's allowed list is
// {text, json}. The component must surface this as a hard error
// before any fallback so a misconfigured template cannot silently
// degrade.
func TestDispatch_OutputFormatValidation_Rejection(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	// Override the markdown setup to ask for an unsupported format.
	// The key is "markdown" (the python-side family identifier),
	// NOT "md" — utility.FileTypeMarkdown happens to be the string
	// "md" but the setup key is the family name. resolveOutputFormat
	// looks up setups[string(fileType)], so the fileType passed in
	// here must match the setup key.
	param.Setups["markdown"] = schema.ParserSetup{"output_format": "html"}
	// inputs["file_type"] must also be "markdown" so fileTypeFromInputs
	// returns a FileType whose string form matches the setup key.
	c := &ParserComponent{Param: param}

	_, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("# Title\n"),
		"file_type": "md",
	})
	if err == nil {
		t.Fatal("Invoke: want error for unsupported output_format, got nil")
	}
	if !strings.Contains(err.Error(), "output_format") {
		t.Errorf("error %q must mention output_format", err.Error())
	}
	if !strings.Contains(err.Error(), "markdown") && !strings.Contains(err.Error(), "md") {
		t.Errorf("error %q must mention the family", err.Error())
	}
}

// TestDispatch_TextPageMode_NoFileType pins the no-dispatch
// path. When the upstream inputs supply neither file_type nor
// file.name, the component degrades to text-page mode and
// emits output_format=text. This is the documented behavior for
// canvas-bound invocations that wire the binary directly without
// a family hint.
func TestDispatch_TextPageMode_NoFileType(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary": []byte("plain content\n"),
		"doc_id": "unknown",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "text"; got != want {
		t.Errorf("output_format = %v, want %v (text-page mode)", got, want)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok || len(pages) == 0 {
		t.Fatalf("pages slice missing or empty: %T", out["pages"])
	}
}

// TestDispatch_SupportedFamilyFailure_HardErrors pins the agreed
// migration rule: once a supported family is identified, parser
// resolution/execution failures must surface as errors instead of
// silently degrading to text-page mode.
func TestDispatch_SupportedFamilyFailure_HardErrors(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	c := &ParserComponent{Param: param}

	_, err := c.Invoke(context.Background(), map[string]any{
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

// TestResolveOutputFormat_DefaultsAndWhitelist pins the two-layer
// behavior of resolveOutputFormat: it returns the setup's
// output_format when present (or "text" when absent), and
// rejects values not in the allowed_output_format list.
func TestResolveOutputFormat_DefaultsAndWhitelist(t *testing.T) {
	allowed := map[string][]string{
		"pdf":      {"json", "markdown"},
		"markdown": {"text", "json"},
	}
	cases := []struct {
		name    string
		setups  map[string]schema.ParserSetup
		family  string
		want    string
		wantErr bool
	}{
		{
			name:   "no setup → empty (text-page mode)",
			setups: nil,
			family: "pdf",
			want:   "",
		},
		{
			name:   "setup with output_format=json → json",
			setups: map[string]schema.ParserSetup{"pdf": {"output_format": "json"}},
			family: "pdf",
			want:   "json",
		},
		{
			name:   "setup with output_format=markdown → markdown",
			setups: map[string]schema.ParserSetup{"pdf": {"output_format": "markdown"}},
			family: "pdf",
			want:   "markdown",
		},
		{
			name:   "setup without output_format → default text",
			setups: map[string]schema.ParserSetup{"markdown": {}},
			family: "markdown",
			want:   "text",
		},
		{
			name:    "pdf asking for html (not allowed) → reject",
			setups:  map[string]schema.ParserSetup{"pdf": {"output_format": "html"}},
			family:  "pdf",
			wantErr: true,
		},
		{
			name:   "family with no whitelist → accept setup value",
			setups: map[string]schema.ParserSetup{"video": {"output_format": "json"}},
			family: "video",
			want:   "json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveOutputFormat(tc.family, tc.setups, allowed)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConfigureParserFromSetups_UsesPythonFamilySetup(t *testing.T) {
	setups := schema.ParserParam{}.Defaults().Setups
	got := &captureSetupConfigurer{}

	configureParserFromSetups(got, utility.FileTypePDF, setups)

	want := map[string]any(setups["pdf"])
	if !reflect.DeepEqual(got.setup, want) {
		t.Fatalf("ConfigureFromSetup got %+v, want %+v", got.setup, want)
	}
}

func TestDispatch_PDFMarkdown_UsesConfiguredOutputFormat(t *testing.T) {
	t.Setenv("DEEPDOC_URL", "")
	t.Setenv("OSSDEEPDOC_URL", "")

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["output_format"] = "markdown"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    data,
		"file_type": "pdf",
		"name":      "Doc1.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "markdown"; got != want {
		t.Fatalf("output_format = %v, want %v", got, want)
	}
	md, ok := out["markdown"].(string)
	if !ok || md == "" {
		t.Fatalf("markdown payload missing or empty: %T", out["markdown"])
	}
	if _, ok := out["json"]; ok {
		t.Fatalf("json payload must be absent for markdown output: %+v", out["json"])
	}
}

func TestDispatch_PDFPlainText_UsesConfiguredBackend(t *testing.T) {
	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "plain_text"
	param.Setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
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
	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "CustomVLM"
	c := &ParserComponent{Param: param}

	_, err := c.Invoke(context.Background(), map[string]any{
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
	pdfVisionModelResolver = func(tenantID string, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		if tenantID != "tenant-1" || modelID != "CustomVLM" {
			return nil, "", nil, fmt.Errorf("resolver got tenant/model %q/%q", tenantID, modelID)
		}
		return nil, "resolved-vlm", nil, nil
	}
	pdfVisionChatInvoker = func(_ modelModule.ModelDriver, modelName string, messages []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
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
		return &modelModule.ChatResponse{Answer: &answer}, nil
	}

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "CustomVLM"
	param.Setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
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
	pdfVisionModelResolver = func(string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return nil, "resolved-vlm", nil, nil
	}
	call := 0
	pdfVisionChatInvoker = func(_ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
		call++
		answer := ""
		if call == 1 {
			answer = "First page"
		}
		return &modelModule.ChatResponse{Answer: &answer}, nil
	}

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "CustomVLM"
	param.Setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"task_id":"task-3"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task-3/result":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":{"doc":{"md_content":"# Title\n\nBody\n"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "MinerU"
	param.Setups["pdf"]["output_format"] = "markdown"
	param.Setups["pdf"]["mineru_apiserver"] = server.URL
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "markdown"; got != want {
		t.Fatalf("output_format = %v, want %v", got, want)
	}
	md, ok := out["markdown"].(string)
	if !ok || !strings.Contains(md, "Title") {
		t.Fatalf("markdown payload = %#v, want Title content", out["markdown"])
	}
}

func TestDispatch_PDFPaddleOCRMarkdown_UsesConfiguredBackend(t *testing.T) {
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

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "PaddleOCR"
	param.Setups["pdf"]["output_format"] = "markdown"
	param.Setups["pdf"]["paddleocr_base_url"] = server.URL
	param.Setups["pdf"]["paddleocr_api_key"] = "paddle-secret"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "markdown"; got != want {
		t.Fatalf("output_format = %v, want %v", got, want)
	}
	md, ok := out["markdown"].(string)
	if !ok || !strings.Contains(md, "Paddle Title") {
		t.Fatalf("markdown payload = %#v, want Paddle Title content", out["markdown"])
	}
}

func TestDispatch_PDFDoclingMarkdown_UsesConfiguredBackend(t *testing.T) {
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

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "Docling"
	param.Setups["pdf"]["output_format"] = "markdown"
	param.Setups["pdf"]["docling_server_url"] = server.URL
	param.Setups["pdf"]["docling_api_key"] = "doc-secret"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "markdown"; got != want {
		t.Fatalf("output_format = %v, want %v", got, want)
	}
	md, ok := out["markdown"].(string)
	if !ok || !strings.Contains(md, "Docling Title") {
		t.Fatalf("markdown payload = %#v, want Docling Title content", out["markdown"])
	}
	if got, want := requestCount, 3; got != want {
		t.Fatalf("requestCount = %d, want %d", got, want)
	}
}

func TestDispatch_PDFOpenDataLoaderMarkdown_UsesConfiguredBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/file_parse" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json_doc":null,"md_text":"# ODL Title\n\nODL body.\n"}`))
	}))
	defer server.Close()

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "OpenDataLoader"
	param.Setups["pdf"]["output_format"] = "markdown"
	param.Setups["pdf"]["opendataloader_apiserver"] = server.URL
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	md, ok := out["markdown"].(string)
	if !ok || !strings.Contains(md, "ODL Title") {
		t.Fatalf("markdown payload = %#v, want ODL Title", out["markdown"])
	}
}

func TestDispatch_PDFSoMarkMarkdown_UsesConfiguredBackend(t *testing.T) {
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

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "SoMark"
	param.Setups["pdf"]["output_format"] = "markdown"
	param.Setups["pdf"]["somark_base_url"] = server.URL
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	md, ok := out["markdown"].(string)
	if !ok || !strings.Contains(md, "SoMark Title") {
		t.Fatalf("markdown payload = %#v, want SoMark Title", out["markdown"])
	}
}

func TestDispatch_PDFTCADPMarkdown_UsesConfiguredBackend(t *testing.T) {
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

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "TCADP parser"
	param.Setups["pdf"]["output_format"] = "markdown"
	param.Setups["pdf"]["tcadp_apiserver"] = server.URL
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("%PDF-1.4"),
		"file_type": "pdf",
		"name":      "sample.pdf",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	md, ok := out["markdown"].(string)
	if !ok || !strings.Contains(md, "Hello TCADP") {
		t.Fatalf("markdown payload = %#v, want Hello TCADP", out["markdown"])
	}
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

func TestResolveLibType_UsesOwningFamilySetup(t *testing.T) {
	setups := schema.ParserParam{}.Defaults().Setups
	setups["slides"]["lib_type"] = "office_oxide"
	setups["slides"]["parse_method"] = "deepdoc"
	setups["spreadsheet"]["lib_type"] = "office_oxide"
	setups["spreadsheet"]["parse_method"] = "deepdoc"

	cases := []struct {
		name            string
		fileType        utility.FileType
		wantLibType     string
		wantParseMethod string
	}{
		{
			name:            "pptx resolves from slides family",
			fileType:        utility.FileTypePPTX,
			wantLibType:     "office_oxide",
			wantParseMethod: "deepdoc",
		},
		{
			name:            "xlsx resolves from spreadsheet family",
			fileType:        utility.FileTypeXLSX,
			wantLibType:     "office_oxide",
			wantParseMethod: "deepdoc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLibType, gotParseMethod := resolveLibType(tc.fileType, setups)
			if gotLibType != tc.wantLibType || gotParseMethod != tc.wantParseMethod {
				t.Fatalf("resolveLibType(%q) = (%q, %q), want (%q, %q)",
					tc.fileType, gotLibType, gotParseMethod, tc.wantLibType, tc.wantParseMethod)
			}
		})
	}
}
