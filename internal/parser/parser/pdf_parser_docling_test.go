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

func TestPDFParser_ParseWithResult_DoclingChunkedMarkdownIntegration(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := requestCount.Add(1)
		if got, want := r.Header.Get("Authorization"), "Bearer doc-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/convert/source" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Decode: %v", err)
			return
		}
		options, _ := body["options"].(map[string]any)
		if got, want := options["do_chunking"], true; got != want {
			t.Errorf("do_chunking = %#v, want %#v", got, want)
			return
		}
		chunkingOptions, _ := options["chunking_options"].(map[string]any)
		if got, want := chunkingOptions["tokenizer"], "sentencepiece"; got != want {
			t.Errorf("chunking_options.tokenizer = %#v, want %#v", got, want)
			return
		}
		sources, _ := body["sources"].([]any)
		if len(sources) != 1 {
			t.Errorf("sources len = %d, want 1", len(sources))
			return
		}
		source, _ := sources[0].(map[string]any)
		raw, err := base64.StdEncoding.DecodeString(source["base64_string"].(string))
		if err != nil {
			t.Errorf("DecodeString: %v", err)
			return
		}
		if got := string(raw); !strings.HasPrefix(got, "%PDF") {
			t.Errorf("uploaded file = %q, want PDF bytes", got)
			return
		}
		if current != 1 {
			t.Errorf("request count = %d, want first chunked request only", current)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"text":"Chunk A"},{"chunk":{"text":"Chunk B"}}]`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "Docling",
		"output_format":      "markdown",
		"docling_server_url": server.URL,
		"docling_api_key":    "doc-secret",
	})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if got, want := res.Markdown, "Chunk A\n\nChunk B"; got != want {
		t.Fatalf("Markdown = %q, want %q", got, want)
	}
}

func TestPDFParser_ParseWithResult_DoclingFallbackToStandardJSONIntegration(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := requestCount.Add(1)
		switch current {
		case 1:
			if r.URL.Path != "/v1/convert/source" {
				t.Errorf("request 1 path = %q, want /v1/convert/source", r.URL.Path)
				return
			}
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"chunking unsupported"}`))
		case 2:
			if r.URL.Path != "/v1alpha/convert/source" {
				t.Errorf("request 2 path = %q, want /v1alpha/convert/source", r.URL.Path)
				return
			}
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"chunking unsupported"}`))
		case 3:
			if r.URL.Path != "/v1/convert/source" {
				t.Errorf("request 3 path = %q, want /v1/convert/source", r.URL.Path)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Decode: %v", err)
				return
			}
			options, _ := body["options"].(map[string]any)
			if _, exists := options["do_chunking"]; exists {
				t.Errorf("standard fallback payload unexpectedly contains do_chunking: %#v", options)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"document":{"md_content":"# Docling Title\n\nDocling body.\n"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "Docling",
		"output_format":      "json",
		"docling_server_url": server.URL,
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
	if got, want := requestCount.Load(), int32(3); got != want {
		t.Fatalf("requestCount = %d, want %d", got, want)
	}
}

func TestPDFParser_ParseWithResult_DoclingRequiresServerURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "Docling"})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error when docling_server_url is missing, got nil")
	}
	if !strings.Contains(res.Err.Error(), "docling_server_url") {
		t.Fatalf("error = %q, want docling_server_url context", res.Err.Error())
	}
}
