package parser

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	models "ragflow/internal/entity/models"
)

func TestPDFParser_ParseWithResult_MinerUJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
	var submitCalled atomic.Bool
	var resultCalled atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/file_parse":
			submitCalled.Store(true)
			if got, want := r.Header.Get("Authorization"), "Bearer secret"; got != want {
				t.Errorf("Authorization = %q, want %q", got, want)
				return
			}
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				return
			}
			var backend string
			var fileSeen bool
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				if part.FormName() == "backend" {
					body, _ := io.ReadAll(part)
					backend = string(body)
				}
				if part.FormName() == "files" {
					fileSeen = true
					body, _ := io.ReadAll(part)
					if !strings.HasPrefix(string(body), "%PDF") {
						t.Errorf("uploaded file = %q, want PDF bytes", string(body))
						return
					}
				}
			}
			if backend != "pipeline" {
				t.Errorf("backend = %q, want pipeline", backend)
				return
			}
			if !fileSeen {
				t.Error("multipart upload missing files part")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"task_id":"task-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task-1/result":
			resultCalled.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":{"doc":{"md_content":"# Title\n\nBody paragraph.\n"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":     "MinerU",
		"mineru_apiserver": server.URL,
		"mineru_api_key":   "secret",
		"mineru_backend":   "pipeline",
	})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !submitCalled.Load() || !resultCalled.Load() {
		t.Fatalf("submit/result called = %v/%v, want true/true", submitCalled.Load(), resultCalled.Load())
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	var text strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		value, _ := item["text"].(string)
		text.WriteString(value)
		text.WriteString("\n")
	}
	if !strings.Contains(text.String(), "Title") || !strings.Contains(text.String(), "Body paragraph.") {
		t.Fatalf("JSON text = %q, want title and body", text.String())
	}
	if got, want := res.File["name"], "sample.pdf"; got != want {
		t.Fatalf("File.name = %v, want %v", got, want)
	}
}

func TestPDFParser_ParseWithResult_MinerURequiresAPIServer(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "MinerU"})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error when mineru_apiserver is missing, got nil")
	}
	if !strings.Contains(res.Err.Error(), "mineru_apiserver") {
		t.Fatalf("error = %q, want mineru_apiserver context", res.Err.Error())
	}
}

func TestMinerUUploadShape(t *testing.T) {
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("backend", "pipeline")
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strings.Contains(body.String(), "pipeline") {
		t.Fatalf("multipart body = %q, want backend field", body.String())
	}
}

func TestPDFParser_ParseWithResult_MinerUSendsServerURLForHTTPClientBackend(t *testing.T) {
	withSSRFBypass(t)
	var gotServerURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/file_parse":
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				return
			}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				if part.FormName() == "server_url" {
					body, _ := io.ReadAll(part)
					gotServerURL = string(body)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"task_id":"task-http"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task-http/result":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":{"doc":{"md_content":"# HTTP\n"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":      "MinerU",
		"mineru_apiserver":  server.URL,
		"mineru_backend":    "vlm-http-client",
		"mineru_server_url": "http://vllm-host:30000",
	})

	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := gotServerURL, "http://vllm-host:30000"; got != want {
		t.Fatalf("server_url = %q, want %q", got, want)
	}
}

func TestPDFParser_ParseWithResult_MinerUReadsBackendFromProviderJSONApiKey(t *testing.T) {
	withSSRFBypass(t)
	var gotBackend, gotServerURL, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/file_parse":
			gotAuth = r.Header.Get("Authorization")
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				switch part.FormName() {
				case "backend":
					body, _ := io.ReadAll(part)
					gotBackend = string(body)
				case "server_url":
					body, _ := io.ReadAll(part)
					gotServerURL = string(body)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"task_id":"task-json"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task-json/result":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":{"doc":{"md_content":"# JSON\n"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	apiKeyJSON := fmt.Sprintf(`{"mineru_apiserver":"%s","mineru_backend":"vlm-http-client","mineru_server_url":"http://vllm-host:30000"}`, server.URL)
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":   "MinerU",
		"mineru_api_key": apiKeyJSON,
	})
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if gotBackend != "vlm-http-client" || gotServerURL != "http://vllm-host:30000" {
		t.Fatalf("backend=%q server_url=%q", gotBackend, gotServerURL)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty for provider JSON api_key", gotAuth)
	}
}

func TestPDFParser_ParseWithResult_MinerURequiresServerURLForHTTPClientBackend(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":     "MinerU",
		"mineru_apiserver": server.URL,
		"mineru_backend":   "hybrid-http-client",
	})

	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error when mineru_server_url is missing, got nil")
	}
	if !strings.Contains(res.Err.Error(), "mineru_server_url") {
		t.Fatalf("error = %q, want mineru_server_url context", res.Err.Error())
	}
}

func TestPDFParser_ParseWithResult_MinerURejectsInvalidBackend(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":     "MinerU",
		"mineru_apiserver": "http://mineru-api:8888",
		"mineru_backend":   "vlm-transformers",
	})

	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error for invalid backend, got nil")
	}
	if !strings.Contains(res.Err.Error(), "invalid backend") {
		t.Fatalf("error = %q, want invalid backend context", res.Err.Error())
	}
}

func TestPDFParser_ParseWithResult_MinerUV1ReturnsMarkdown(t *testing.T) {
	withSSRFBypass(t)
	var fileParseCalled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","version":"4.0.12"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"name":"sample.pdf","status":"completed","output_files":{"markdown":{"file_id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# From V1\n\nParagraph.\n"))
		case r.URL.Path == "/file_parse":
			fileParseCalled.Store(true)
			http.Error(w, "file_parse", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":     "MinerU",
		"mineru_apiserver": server.URL,
		"mineru_backend":   "pipeline",
	})
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if fileParseCalled.Load() {
		t.Fatal("V1 endpoint must not call /file_parse")
	}
	var text strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		value, _ := item["text"].(string)
		text.WriteString(value)
		text.WriteString("\n")
	}
	if !strings.Contains(text.String(), "From V1") || !strings.Contains(text.String(), "Paragraph.") {
		t.Fatalf("JSON text = %q, want V1 markdown", text.String())
	}
}

func TestMinerUMarkdownResultUsesMarkdownAndRejectsEmpty(t *testing.T) {
	res := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{
		Markdown: "# Cloud title\n",
		Zip:      []byte("not-a-zip"),
	})
	if res.Err != nil {
		t.Fatalf("markdown result: %v", res.Err)
	}
	var text strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		value, _ := item["text"].(string)
		text.WriteString(value)
	}
	if !strings.Contains(text.String(), "Cloud title") {
		t.Fatalf("JSON text = %q, want markdown text", text.String())
	}

	empty := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{Zip: []byte("zip-bytes")})
	if empty.Err == nil || !strings.Contains(empty.Err.Error(), "empty") {
		t.Fatalf("empty result error = %v", empty.Err)
	}
}
