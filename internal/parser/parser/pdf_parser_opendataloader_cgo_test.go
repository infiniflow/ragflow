//go:build cgo

package parser

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
)

func TestPDFParser_ParseWithResult_OpenDataLoaderPositions(t *testing.T) {
	withSSRFBypass(t)
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	sizes, pageCount, err := pdfium.PageSizes(t.Context(), data, []int{0})
	if err != nil || pageCount == 0 {
		t.Fatalf("PageSizes: sizes=%v count=%d err=%v", sizes, pageCount, err)
	}
	pageHeight := sizes[0][1]

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
	if got, want := res.File["page_count"], pageCount; got != want {
		t.Errorf("page_count = %v, want %d (pdfium)", got, want)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings = %v, want none when geometry is available", res.Warnings)
	}
	item := res.JSON[0].ToMap()
	rows, ok := item["_pdf_positions"].([][]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("_pdf_positions = %#v, want one row", item["_pdf_positions"])
	}
	if page, ok := numericAny(rows[0][0]); !ok || page != 1 {
		t.Fatalf("page = %#v, want 1 (positions normalize the page to int)", rows[0][0])
	}
	want := []float64{72, 540, pageHeight - 700.2, pageHeight - 650.5}
	for i := range want {
		got, ok := rows[0][i+1].(float64)
		if !ok || math.Abs(got-want[i]) > 1e-6 {
			t.Fatalf("_pdf_positions = %#v, want %v", rows, want)
		}
	}
}
