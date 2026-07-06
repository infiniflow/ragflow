package parser

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPDFParser_ParseWithResult_PaddleOCRMarkdownIntegration(t *testing.T) {
	var called atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		called.Store(true)
		if got, want := r.Header.Get("Authorization"), "Bearer paddle-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		var body struct {
			File      string `json:"file"`
			FileType  int    `json:"fileType"`
			Algorithm string `json:"algorithm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Decode: %v", err)
			return
		}
		if got, want := body.FileType, 0; got != want {
			t.Errorf("fileType = %d, want %d for PDF", got, want)
			return
		}
		if got, want := body.Algorithm, "PaddleOCR-VL"; got != want {
			t.Errorf("algorithm = %q, want %q", got, want)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(body.File)
		if err != nil {
			t.Errorf("DecodeString: %v", err)
			return
		}
		if got := string(raw); !strings.HasPrefix(got, "%PDF") {
			t.Errorf("uploaded file = %q, want PDF bytes", got)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Paddle Title\n\nBody paragraph.\n"}}]}}`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "PaddleOCR",
		"output_format":      "markdown",
		"paddleocr_base_url": server.URL,
		"paddleocr_api_key":  "paddle-secret",
	})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Paddle Title\n\nBody paragraph.\n"}}]}}`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "PaddleOCR",
		"output_format":      "json",
		"paddleocr_base_url": server.URL,
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

func TestPDFParser_ParseWithResult_PaddleOCRRequiresBaseURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "PaddleOCR"})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error when paddleocr_base_url is missing, got nil")
	}
	if !strings.Contains(res.Err.Error(), "paddleocr_base_url") {
		t.Fatalf("error = %q, want paddleocr_base_url context", res.Err.Error())
	}
}
