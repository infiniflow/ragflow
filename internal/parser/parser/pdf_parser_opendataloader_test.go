package parser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestPDFParser_ParseWithResult_OpenDataLoaderJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
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
		"opendataloader_apiserver": server.URL,
		"opendataloader_api_key":   "odl-secret",
		"hybrid":                   "docling-fast",
		"sanitize":                 sanitize,
	})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	if len(res.JSON) < 2 {
		t.Fatalf("JSON len = %d, want >=2", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_OpenDataLoaderMarkdownFallbackToJSON(t *testing.T) {
	withSSRFBypass(t)
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
		"opendataloader_apiserver": server.URL,
	})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var text strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		value, _ := item["text"].(string)
		text.WriteString(value)
		text.WriteString("\n")
	}
	if !strings.Contains(text.String(), "ODL Title") {
		t.Fatalf("JSON text = %q, want ODL Title", text.String())
	}
}

func TestPDFParser_ParseWithResult_OpenDataLoaderRequiresAPIServer(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "OpenDataLoader"})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "opendataloader_apiserver") {
		t.Fatalf("error = %v, want opendataloader_apiserver context", res.Err)
	}
}

// opendataloader_timeout is the only bound a synchronous OpenDataLoader parse
// gets, so a stalled server has to be cut off at the configured value instead
// of at the transport's 20-minute header timeout.
func TestPDFParser_ParseWithResult_OpenDataLoaderAppliesConfiguredTimeout(t *testing.T) {
	withSSRFBypass(t)
	server := openDataLoaderStallServer(t, 3*time.Second)
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":             "OpenDataLoader",
		"opendataloader_apiserver": server.URL,
		"opendataloader_timeout":   1,
	})

	start := time.Now()
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	elapsed := time.Since(start)
	if !errors.Is(res.Err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", res.Err)
	}
	// The configured value is in seconds: neither firing early (milliseconds)
	// nor never firing (the setting ignored) is a pass.
	if elapsed < 900*time.Millisecond || elapsed >= 2500*time.Millisecond {
		t.Fatalf("elapsed = %s, want the request cut off at the configured 1s", elapsed)
	}
}

// A pipeline cancellation must reach the in-flight request even when the
// parser carries no configured timeout.
func TestPDFParser_ParseWithResult_OpenDataLoaderAbortsOnCancelledContext(t *testing.T) {
	withSSRFBypass(t)
	server := openDataLoaderStallServer(t, 3*time.Second)
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":             "OpenDataLoader",
		"opendataloader_apiserver": server.URL,
	})
	pdf.OpenDataLoaderTimeout = 0

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", res.Err)
	}
}

// openDataLoaderStallServer answers /file_parse only after stall has passed,
// which stands in for an OpenDataLoader server that is still parsing.
func openDataLoaderStallServer(t *testing.T, stall time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file_parse" {
			http.NotFound(w, r)
			return
		}
		select {
		case <-time.After(stall):
			_, _ = w.Write([]byte(`{"json_doc":null,"md_text":"late"}`))
		case <-r.Context().Done():
		}
	}))
}

func TestOpenDataLoaderItems_TableCellsFallback(t *testing.T) {
	root := map[string]any{
		"type": "table",
		"cells": []any{
			map[string]any{"row": 0, "content": "a"},
			map[string]any{"row": 0, "content": "b"},
		},
	}
	items, _ := openDataLoaderItemsWithGeometry(root, nil)
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	td, ok := items[0]["table"].(*entity.TableData)
	if !ok || td == nil {
		t.Fatalf("table field missing: %#v", items[0])
	}
	if items[0]["text"] != nil {
		t.Fatalf("table item must not carry legacy HTML text: %#v", items[0])
	}
	rows := td.CellTexts()
	if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0] != "a" || rows[0][1] != "b" {
		t.Fatalf("table rows = %#v, want [[a b]]", rows)
	}
	if td.HeaderRows != 1 {
		t.Fatalf("HeaderRows = %d, want 1", td.HeaderRows)
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
	items, _ := openDataLoaderItemsWithGeometry(root, nil)
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if got, _ := json.Marshal(items[0]); !strings.Contains(string(got), "z") {
		t.Fatalf("item = %s, want sparse row text", string(got))
	}
}
