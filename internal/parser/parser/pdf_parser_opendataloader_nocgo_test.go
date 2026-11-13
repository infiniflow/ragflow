//go:build !cgo

package parser

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_OpenDataLoaderWithoutGeometry(t *testing.T) {
	withSSRFBypass(t)
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("drain body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json_doc":{"type":"paragraph","content":"ODL Body","page number":1,"bounding box":[72,650.5,540,700.2]}}`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":             "OpenDataLoader",
		"opendataloader_apiserver": server.URL,
	})
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	item := res.JSON[0].ToMap()
	if item["text"] != "ODL Body" {
		t.Errorf("text = %v, want ODL Body", item["text"])
	}
	if item["_pdf_positions"] != nil {
		t.Errorf("_pdf_positions = %#v, want none without page geometry", item["_pdf_positions"])
	}
	if page, ok := numericAny(item["page_number"]); !ok || page != 1 {
		t.Errorf("page_number = %#v, want 1 (page identity survives without geometry)", item["page_number"])
	}
	if got, want := res.File["page_count"], 1; got != want {
		t.Errorf("page_count = %v, want %d (pages reported by the service)", got, want)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "requires cgo") {
		t.Errorf("warnings = %v, want one geometry warning", res.Warnings)
	}

	// A response without page keys still yields items; the document must not
	// report zero pages because nothing pins the count down.
	pageLess := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("drain body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json_doc":{"type":"document","number of pages":3,"children":[{"type":"paragraph","content":"No page"}]}}`))
	}))
	defer pageLess.Close()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":             "OpenDataLoader",
		"opendataloader_apiserver": pageLess.URL,
	})
	pageLessRes := pdf.ParseWithResult(t.Context(), "sample.pdf", data)
	if pageLessRes.Err != nil {
		t.Fatalf("ParseWithResult (page-less): %v", pageLessRes.Err)
	}
	if got, want := pageLessRes.File["page_count"], 3; got != want {
		t.Errorf("page_count = %v, want %d from the service's page total", got, want)
	}
}
