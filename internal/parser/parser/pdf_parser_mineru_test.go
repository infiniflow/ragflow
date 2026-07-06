package parser

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPDFParser_ParseWithResult_MinerUMarkdownIntegration(t *testing.T) {
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
		"output_format":    "markdown",
		"mineru_apiserver": server.URL,
		"mineru_api_key":   "secret",
		"mineru_backend":   "pipeline",
	})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !submitCalled.Load() || !resultCalled.Load() {
		t.Fatalf("submit/result called = %v/%v, want true/true", submitCalled.Load(), resultCalled.Load())
	}
	if got, want := res.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if got, want := res.Markdown, "# Title\n\nBody paragraph.\n"; got != want {
		t.Fatalf("Markdown = %q, want %q", got, want)
	}
	if got, want := res.File["name"], "sample.pdf"; got != want {
		t.Fatalf("File.name = %v, want %v", got, want)
	}
}

func TestPDFParser_ParseWithResult_MinerUJSONIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"task_id":"task-2"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task-2/result":
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
		"output_format":    "json",
		"mineru_apiserver": server.URL,
	})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON is empty; want markdown-normalized items")
	}
	if got := res.JSON[0]["text"]; got == nil {
		t.Fatal("JSON[0].text missing")
	}
}

func TestPDFParser_ParseWithResult_MinerURequiresAPIServer(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "MinerU"})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
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
