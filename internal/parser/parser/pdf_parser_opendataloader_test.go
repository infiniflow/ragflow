package parser

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_OpenDataLoaderJSONIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/file_parse" {
			http.NotFound(w, r)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer odl-secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			return
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader: %v", err)
			return
		}
		seenHybrid := false
		seenSanitize := false
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
			case "file":
				body, _ := io.ReadAll(part)
				if !strings.HasPrefix(string(body), "%PDF") {
					t.Errorf("uploaded file = %q, want PDF bytes", string(body))
					return
				}
			case "hybrid":
				body, _ := io.ReadAll(part)
				seenHybrid = string(body) == "docling-fast"
			case "sanitize":
				body, _ := io.ReadAll(part)
				seenSanitize = string(body) == "true"
			}
		}
		if !seenHybrid || !seenSanitize {
			t.Errorf("multipart fields missing: hybrid=%v sanitize=%v", seenHybrid, seenSanitize)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json_doc":{"type":"title","content":"ODL Title","children":[{"type":"paragraph","content":"ODL Body"},{"type":"table","html":"<table><tr><td>a</td></tr></table>"}]}}`))
	}))
	defer server.Close()

	sanitize := true
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":             "OpenDataLoader",
		"output_format":            "json",
		"opendataloader_apiserver": server.URL,
		"opendataloader_api_key":   "odl-secret",
		"hybrid":                   "docling-fast",
		"sanitize":                 sanitize,
	})

	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) < 2 {
		t.Fatalf("JSON len = %d, want >=2", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_OpenDataLoaderMarkdownFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file_parse" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json_doc":null,"md_text":"# ODL Title\n\nODL Body\n"}`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":             "OpenDataLoader",
		"output_format":            "markdown",
		"opendataloader_apiserver": server.URL,
	})
	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.Markdown, "ODL Title") {
		t.Fatalf("Markdown = %q, want ODL Title", res.Markdown)
	}
}

func TestPDFParser_ParseWithResult_OpenDataLoaderRequiresAPIServer(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "OpenDataLoader"})
	res := pdf.ParseWithResult("sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "opendataloader_apiserver") {
		t.Fatalf("error = %v, want opendataloader_apiserver context", res.Err)
	}
}

func TestOpenDataLoaderItems_TableCellsFallback(t *testing.T) {
	root := map[string]any{
		"type": "table",
		"cells": []any{
			map[string]any{"row": 0, "content": "a"},
			map[string]any{"row": 0, "content": "b"},
		},
	}
	items := openDataLoaderItems(root)
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if got, _ := json.Marshal(items[0]); !strings.Contains(string(got), "a | b") {
		t.Fatalf("item = %s, want row text", string(got))
	}
}

func TestOpenDataLoaderItems_TableCellsFallbackSparseRows(t *testing.T) {
	root := map[string]any{
		"type": "table",
		"cells": []any{
			map[string]any{"row": 0, "content": "a"},
			map[string]any{"row": 5, "content": "z"},
		},
	}
	items := openDataLoaderItems(root)
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if got, _ := json.Marshal(items[0]); !strings.Contains(string(got), "z") {
		t.Fatalf("item = %s, want sparse row text", string(got))
	}
}
