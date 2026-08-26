package parser

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// paddleOCRJobServer stubs the async PaddleOCR Job API
// (submit → poll → fetch JSONL), mirroring Python's PaddleOCRParser.
// wantToken is the expected Authorization bearer token; empty skips the check.
func paddleOCRJobServer(t *testing.T, called *atomic.Bool, wantToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/ocr/jobs":
			if wantToken != "" {
				if got, want := r.Header.Get("Authorization"), "Bearer "+wantToken; got != want {
					t.Errorf("Authorization = %q, want %q", got, want)
					return
				}
			}
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				t.Errorf("ParseMultipartForm: %v", err)
				return
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("FormFile: %v", err)
				return
			}
			raw, readErr := io.ReadAll(f)
			f.Close()
			if readErr != nil {
				t.Errorf("ReadAll: %v", readErr)
				return
			}
			if got := string(raw); !strings.HasPrefix(got, "%PDF") {
				t.Errorf("uploaded file = %q, want PDF bytes", got)
				return
			}
			if got, want := r.FormValue("model"), "PaddleOCR-VL"; got != want {
				t.Errorf("model = %q, want %q", got, want)
				return
			}
			if got := r.FormValue("optionalPayload"); !strings.Contains(got, "formatBlockContent") {
				t.Errorf("optionalPayload = %q, want formatBlockContent", got)
				return
			}
			if called != nil {
				called.Store(true)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"jobId":"job-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/ocr/jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			resultURL := "http://" + r.Host + "/result"
			_, _ = fmt.Fprintf(w, `{"data":{"state":"done","resultJsonUrl":%q}}`, resultURL)
		case r.Method == http.MethodGet && r.URL.Path == "/result":
			w.Header().Set("Content-Type", "application/jsonl")
			_, _ = w.Write([]byte(`{"result":{"layoutParsingResults":[{"prunedResult":{"parsing_res_list":[{"block_content":"# Paddle Title\n\nBody paragraph.\n"}]}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestPDFParser_ParseWithResult_PaddleOCRMarkdownIntegration(t *testing.T) {
	withSSRFBypass(t)
	var called atomic.Bool
	server := paddleOCRJobServer(t, &called, "paddle-secret")
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "PaddleOCR",
		"output_format":      "markdown",
		"paddleocr_base_url": server.URL,
		"paddleocr_api_key":  "paddle-secret",
	})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !called.Load() {
		t.Fatal("PaddleOCR server was not called")
	}
	if got, want := res.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if got, want := res.Markdown, "# Paddle Title\n\nBody paragraph."; got != want {
		t.Fatalf("Markdown = %q, want %q", got, want)
	}
	if got, want := res.File["name"], "sample.pdf"; got != want {
		t.Fatalf("File.name = %v, want %v", got, want)
	}
}

func TestPDFParser_ParseWithResult_PaddleOCRJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
	server := paddleOCRJobServer(t, nil, "")
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "PaddleOCR",
		"output_format":      "json",
		"paddleocr_base_url": server.URL,
	})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
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

func TestPDFParser_ParseWithResult_PaddleOCRRequiresBaseURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "PaddleOCR"})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error when paddleocr_base_url is missing, got nil")
	}
	if !strings.Contains(res.Err.Error(), "paddleocr_base_url") {
		t.Fatalf("error = %q, want paddleocr_base_url context", res.Err.Error())
	}
}
