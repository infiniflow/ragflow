package parser

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_TCADPJSONIntegration(t *testing.T) {
	zipPayload := tcadpZipFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			if got, want := r.Header.Get("Authorization"), "Bearer tcadp-secret"; got != want {
				t.Errorf("Authorization = %q, want %q", got, want)
				return
			}
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "TCADP parser",
		"output_format":   "json",
		"tcadp_apiserver": server.URL,
		"tcadp_api_key":   "tcadp-secret",
	})
	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) < 2 {
		t.Fatalf("JSON len = %d, want >=2", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_TCADPMarkdownIntegration(t *testing.T) {
	zipPayload := tcadpZipFixture(t)
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
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "TCADP parser",
		"output_format":   "markdown",
		"tcadp_apiserver": server.URL,
	})
	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.Markdown, "Hello TCADP") {
		t.Fatalf("Markdown = %q, want fixture text", res.Markdown)
	}
}

func TestPDFParser_ParseWithResult_TCADPRequiresAPIServer(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "TCADP parser"})
	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "tcadp_apiserver") {
		t.Fatalf("error = %v, want tcadp_apiserver context", res.Err)
	}
}

func tcadpZipFixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	f1, err := writer.Create("result.md")
	if err != nil {
		t.Fatalf("Create md: %v", err)
	}
	_, _ = f1.Write([]byte("Hello TCADP"))
	f2, err := writer.Create("blocks.json")
	if err != nil {
		t.Fatalf("Create json: %v", err)
	}
	_, _ = f2.Write([]byte(`[{"type":"table","table_data":{"rows":[["a","b"]]}}]`))
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}
	return buf.Bytes()
}
