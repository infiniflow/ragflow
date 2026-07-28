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
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
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
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
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
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "tcadp_apiserver") {
		t.Fatalf("error = %v, want tcadp_apiserver context", res.Err)
	}
}

// TestTCADPAnyToItems_PropagatesPageNumber verifies the fix for migration
// : per-slide page numbers present in the raw TCADP response
// must be attached to each chunk item as "positions" (a 1-indexed 5-tuple
// [page, 0, 0, 0, 0]). The shared ingestion pipeline
// (processChunkPositions -> AddPositions, a passthrough) derives
// top_int=[0], position_int=[[page,0,0,0,0]] and page_num_int=[page] from
// this field. Elements without a page number (e.g. spreadsheet TCADP)
// must NOT receive a positions field.
func TestTCADPAnyToItems_PropagatesPageNumber(t *testing.T) {
	cases := []struct {
		name    string
		raw     any
		wantPos []float64 // nil => positions must be absent
	}{
		{
			name:    "text element with page_number",
			raw:     map[string]any{"content": "slide text", "type": "text", "page_number": 3},
			wantPos: []float64{3, 0, 0, 0, 0}, // 1-indexed
		},
		{
			name:    "image element with page_number",
			raw:     map[string]any{"caption": "fig", "type": "image", "page_number": 5},
			wantPos: []float64{5, 0, 0, 0, 0},
		},
		{
			name:    "table element with page_number",
			raw:     map[string]any{"type": "table", "table_data": map[string]any{"rows": []any{[]any{"a", "b"}}}, "page_number": 2},
			wantPos: []float64{2, 0, 0, 0, 0},
		},
		{
			name:    "element without page number gets no positions",
			raw:     map[string]any{"content": "no page", "type": "text"},
			wantPos: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := tcadpAnyToItems(tc.raw)
			if len(items) == 0 {
				t.Fatalf("tcadpAnyToItems returned no items")
			}
			item := items[0]
			got, ok := item["positions"].([]float64)
			if tc.wantPos == nil {
				if ok {
					t.Errorf("positions present = %v, want absent (element without page number)", got)
				}
				return
			}
			if !ok {
				t.Fatalf("positions missing, want %v", tc.wantPos)
			}
			if len(got) != len(tc.wantPos) {
				t.Fatalf("positions len = %d, want %d (%v)", len(got), len(tc.wantPos), tc.wantPos)
			}
			for i := range tc.wantPos {
				if got[i] != tc.wantPos[i] {
					t.Errorf("positions[%d] = %v, want %v (full: %v)", i, got[i], tc.wantPos[i], got)
				}
			}
		})
	}
}

// TestTCADPAnyToItems_NestedArrayKeepsEachPage verifies that a nested
// array of TCADP elements produces one item per element, each carrying
// its own page-derived positions — not just the first element.
func TestTCADPAnyToItems_NestedArrayKeepsEachPage(t *testing.T) {
	raw := []any{
		map[string]any{"content": "p1", "type": "text", "page_number": 1},
		map[string]any{"content": "p7", "type": "text", "page_number": 7},
	}
	items := tcadpAnyToItems(raw)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	want := [][]float64{{1, 0, 0, 0, 0}, {7, 0, 0, 0, 0}} // page 1 -> [1,...], page 7 -> [7,...] (1-indexed)
	for i, w := range want {
		got, ok := items[i]["positions"].([]float64)
		if !ok {
			t.Errorf("items[%d].positions missing, want %v", i, w)
			continue
		}
		if len(got) != len(w) {
			t.Errorf("items[%d].positions = %v, want len %d", i, got, len(w))
			continue
		}
		for j := range w {
			if got[j] != w[j] {
				t.Errorf("items[%d].positions[%d] = %v, want %v", i, j, got[j], w[j])
			}
		}
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
