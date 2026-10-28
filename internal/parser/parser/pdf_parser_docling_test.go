package parser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPDFParser_ParseWithResult_DoclingChunkedIntegration(t *testing.T) {
	withSSRFBypass(t)
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
		if _, isChunked := options["do_chunking"]; !isChunked {
			// Standard payloads are tried first; this server only supports chunking.
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"standard conversion unsupported"}`))
			return
		}
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
		if current != 3 {
			t.Errorf("chunked success on request %d, want 3 (after both standard endpoints 422)", current)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"text":"Chunk A"},{"chunk":{"text":"Chunk B"}}]`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "Docling",
		"docling_server_url": server.URL,
		"docling_api_key":    "doc-secret",
	})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	var got strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		got.WriteString(text)
		got.WriteString("\n")
	}
	if !strings.Contains(got.String(), "Chunk A") || !strings.Contains(got.String(), "Chunk B") {
		t.Fatalf("JSON text = %q, want both chunks", got.String())
	}
	warned := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "positions unavailable") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("chunked fallback must warn that positions are unavailable: %#v", res.Warnings)
	}
}

func TestPDFParser_ParseWithResult_DoclingStandardPreferredIntegration(t *testing.T) {
	withSSRFBypass(t)
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := requestCount.Add(1)
		if current != 1 {
			t.Errorf("request %d: standard conversion must succeed on the first request", current)
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/v1/convert/source" {
			t.Errorf("request 1 path = %q, want /v1/convert/source", r.URL.Path)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Decode: %v", err)
			return
		}
		options, _ := body["options"].(map[string]any)
		if _, exists := options["do_chunking"]; exists {
			t.Errorf("standard payload unexpectedly contains do_chunking: %#v", options)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"document":{"md_content":"# Docling Title\n\nDocling body.\n"}}`))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "Docling",
		"docling_server_url": server.URL,
	})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON is empty; want markdown-normalized items")
	}
	if got, want := requestCount.Load(), int32(1); got != want {
		t.Fatalf("requestCount = %d, want %d", got, want)
	}
}

func TestPDFParser_ParseWithResult_DoclingRequiresServerURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "Docling"})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error when docling_server_url is missing, got nil")
	}
	if !strings.Contains(res.Err.Error(), "docling_server_url") {
		t.Fatalf("error = %q, want docling_server_url context", res.Err.Error())
	}
}

func TestPDFParser_ParseWithResult_DoclingConvertOptions(t *testing.T) {
	tests := []struct {
		name  string
		setup map[string]any
		env   map[string]string
		// A nil value means the option must be left out of the request.
		wantOptions map[string]any
	}{
		{
			name:        "unset options keep the server defaults",
			wantOptions: map[string]any{"do_ocr": nil, "pdf_backend": nil},
		},
		{
			name:        "parser setup",
			setup:       map[string]any{"docling_do_ocr": false, "docling_pdf_backend": "pypdfium2"},
			wantOptions: map[string]any{"do_ocr": false, "pdf_backend": "pypdfium2"},
		},
		{
			name:        "environment",
			env:         map[string]string{"DOCLING_DO_OCR": "false", "DOCLING_PDF_BACKEND": "pypdfium2"},
			wantOptions: map[string]any{"do_ocr": false, "pdf_backend": "pypdfium2"},
		},
		{
			name:        "parser setup wins over the environment",
			setup:       map[string]any{"docling_do_ocr": true, "docling_pdf_backend": "pypdfium2"},
			env:         map[string]string{"DOCLING_DO_OCR": "false", "DOCLING_PDF_BACKEND": "dlparse_v4"},
			wantOptions: map[string]any{"do_ocr": true, "pdf_backend": "pypdfium2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSSRFBypass(t)
			t.Setenv("DOCLING_DO_OCR", tt.env["DOCLING_DO_OCR"])
			t.Setenv("DOCLING_PDF_BACKEND", tt.env["DOCLING_PDF_BACKEND"])
			var requestCount atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				current := requestCount.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("request %d: Decode: %v", current, err)
					return
				}
				options, _ := body["options"].(map[string]any)
				for key, want := range tt.wantOptions {
					got, exists := options[key]
					if want == nil {
						if exists {
							t.Errorf("request %d: options[%q] = %#v, want it left out", current, key, got)
						}
						continue
					}
					if got != want {
						t.Errorf("request %d: options[%q] = %#v, want %#v", current, key, got, want)
					}
				}
				// Reject both standard requests so the chunked payload is checked too.
				if current < 3 {
					w.WriteHeader(http.StatusUnprocessableEntity)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"text":"Chunk A"}]`))
			}))
			defer server.Close()

			setup := map[string]any{
				"parse_method":       "Docling",
				"docling_server_url": server.URL,
			}
			maps.Copy(setup, tt.setup)
			pdf := NewPDFParser()
			pdf.ConfigureFromSetup(setup)

			res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if got, want := requestCount.Load(), int32(3); got != want {
				t.Fatalf("requestCount = %d, want %d", got, want)
			}
		})
	}
}

const doclingPositionsResponse = `{"document":{"md_content":"# Docling Title\n\nDocling body.\n\nUnmatched paragraph.\n","json_content":{
 "pages":{"1":{"page_no":1,"size":{"width":612,"height":792}},"2":{"page_no":2,"size":{"width":612,"height":792}},"3":{"page_no":3,"size":{"width":612,"height":792}}},
 "texts":[
  {"label":"section_header","text":"Docling Title","prov":[{"page_no":1,"bbox":{"l":10,"t":700,"r":200,"b":680,"coord_origin":"BOTTOMLEFT"}}]},
  {"label":"text","text":"Docling body.","prov":[{"page_no":2,"bbox":{"l":15,"t":100,"r":300,"b":140,"coord_origin":"TOPLEFT"}}]}
 ]}}}`

func TestPDFParser_ParseWithResult_DoclingEmitsPositionsAndPageCount(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		options, _ := body["options"].(map[string]any)
		if _, chunked := options["do_chunking"]; chunked {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"chunking unsupported"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(doclingPositionsResponse))
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":       "Docling",
		"output_format":      "json",
		"docling_server_url": server.URL,
	})
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got := res.File["page_count"]; got != 3 {
		t.Fatalf("page_count = %v, want 3 (pages, not documents)", got)
	}
	want := map[string][]any{
		// BOTTOMLEFT y=700/680 on a 792pt page convert to top-origin 92/112.
		"Docling Title": {1, 10.0, 200.0, 92.0, 112.0},
		"Docling body.": {2, 15.0, 300.0, 100.0, 140.0},
	}
	found := map[string]bool{}
	for _, item := range res.JSON {
		m := item.ToMap()
		text := strings.TrimSpace(strings.TrimLeft(stringValue(m["text"]), "# "))
		rows, ok := ExtractPDFPositions(m)
		if w, expected := want[text]; expected {
			if !ok || len(rows) != 1 || len(rows[0]) != 5 {
				t.Fatalf("%q positions = %#v ok=%v", text, rows, ok)
			}
			for i := range w {
				got, _ := numericAny(rows[0][i])
				exp, _ := numericAny(w[i])
				if got != exp {
					t.Fatalf("%q positions[%d] = %v, want %v (row %#v)", text, i, rows[0][i], w[i], rows[0])
				}
			}
			found[text] = true
		} else if ok {
			t.Fatalf("unmatched item %q must not carry positions: %#v", text, rows)
		}
	}
	if len(found) != 2 {
		t.Fatalf("matched items = %v, want both", found)
	}
}

func TestDoclingPageCountFallsBackToProvenanceThenDocuments(t *testing.T) {
	withProv := []doclingDocument{{JSONContent: map[string]any{"texts": []any{
		map[string]any{"text": "a", "prov": []any{map[string]any{"page_no": 4.0}}},
	}}}}
	if got := doclingPageCount(withProv, 0); got != 4 {
		t.Fatalf("provenance page count = %d, want 4", got)
	}
	noPages := []doclingDocument{{MDContent: "x"}}
	if got := doclingPageCount(noPages, 50); got != 50 {
		t.Fatalf("metadata-less response must report source PDF pages = %d, want 50", got)
	}
	if got := doclingPageCount(noPages, 0); got != 1 {
		t.Fatalf("unreadable source page count = %d, want 1 (document count)", got)
	}
}

func TestDoclingPageCountMaxAcrossDuplicateDocuments(t *testing.T) {
	// A single source converts to one logical document; payload shapes that
	// repeat it (document + documents + results) must not multiply the page
	// count.
	pageful := doclingDocument{JSONContent: map[string]any{
		"pages": map[string]any{"1": map[string]any{"page_no": 1.0}, "2": map[string]any{"page_no": 2.0}, "3": map[string]any{"page_no": 3.0}},
	}}
	if got := doclingPageCount([]doclingDocument{pageful, pageful}, 0); got != 3 {
		t.Fatalf("duplicated documents must not sum pages: got %v, want 3", got)
	}
}

func TestDoclingSourcePageCount(t *testing.T) {
	if got := doclingSourcePageCount(buildDoclingPDFWithCount(t, 50)); got != 50 {
		t.Fatalf("source page count = %d, want 50", got)
	}
	if got := doclingSourcePageCount([]byte("%PDF-1.4\nmock")); got != 0 {
		t.Fatalf("unopenable PDF must yield 0, got %d", got)
	}
	if got := doclingSourcePageCount(buildDoclingPDFWithCount(t, 1<<30)); got != 0 {
		t.Fatalf("implausible /Count must be ignored, got %d", got)
	}
}

// buildDoclingPDFWithCount emits a minimal valid PDF whose /Pages /Count is
// the supplied value, mirroring the deepdoc plaintext test fixture.
func buildDoclingPDFWithCount(t *testing.T, count int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	off1 := buf.Len()
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count %d >>\nendobj\n", count)
	off3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>\nendobj\n")
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", off1)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off2)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off3)
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefStart)
	return buf.Bytes()
}

func TestDoclingAttachPositionsMatchesOccurrencesInOrder(t *testing.T) {
	content := map[string]any{"texts": []any{
		map[string]any{"text": "Repeated paragraph.", "prov": []any{
			map[string]any{"page_no": 2.0, "bbox": map[string]any{"l": 10.0, "t": 20.0, "r": 100.0, "b": 40.0}},
		}},
		map[string]any{"text": "Repeated paragraph.", "prov": []any{
			map[string]any{"page_no": 5.0, "bbox": map[string]any{"l": 10.0, "t": 20.0, "r": 100.0, "b": 40.0}},
		}},
	}}
	res := ParseResult{JSON: MapsToItems([]map[string]any{
		{"text": "Repeated paragraph."},
		{"text": "Repeated paragraph."},
	})}
	res = doclingAttachPositions(res, content)
	first, _ := res.JSON[0].ToMap()["_pdf_positions"].([][]any)
	second, _ := res.JSON[1].ToMap()["_pdf_positions"].([][]any)
	if len(first) != 1 || first[0][0] != 2 {
		t.Fatalf("first occurrence must take page 2: %#v", first)
	}
	if len(second) != 1 || second[0][0] != 5 {
		t.Fatalf("second occurrence must take page 5: %#v", second)
	}
}

func TestDoclingAttachPositionsKeepsLeadingDigits(t *testing.T) {
	content := map[string]any{"texts": []any{
		map[string]any{"text": "2026 Outlook", "prov": []any{
			map[string]any{"page_no": 3.0, "bbox": map[string]any{"l": 10.0, "t": 20.0, "r": 100.0, "b": 40.0}},
		}},
		map[string]any{"text": "Introduction", "prov": []any{
			map[string]any{"page_no": 4.0, "bbox": map[string]any{"l": 10.0, "t": 20.0, "r": 100.0, "b": 40.0}},
		}},
	}}
	res := ParseResult{JSON: MapsToItems([]map[string]any{
		{"text": "2026 Outlook"},
		{"text": "1. Introduction"},
	})}
	res = doclingAttachPositions(res, content)
	digits, ok := res.JSON[0].ToMap()["_pdf_positions"].([][]any)
	if !ok || len(digits) != 1 || digits[0][0] != 3 {
		t.Fatalf("leading digits are content and must match: %#v", res.JSON[0].ToMap())
	}
	numbered, ok := res.JSON[1].ToMap()["_pdf_positions"].([][]any)
	if !ok || len(numbered) != 1 || numbered[0][0] != 4 {
		t.Fatalf("numbered list prefix must strip for the match: %#v", res.JSON[1].ToMap())
	}
}

func TestDoclingChunkedResultPrefersSourcePageCount(t *testing.T) {
	body := []byte(`[{"text":"chunk one"},{"text":"chunk two"}]`)
	res, ok := parseDoclingChunkedResult("a.pdf", body, 9)
	if !ok {
		t.Fatal("chunked parse failed")
	}
	if got := res.File["page_count"]; got != 9 {
		t.Fatalf("chunked page_count = %v, want source PDF count 9", got)
	}
	res, ok = parseDoclingChunkedResult("a.pdf", body, 0)
	if !ok {
		t.Fatal("chunked parse failed")
	}
	if got := res.File["page_count"]; got != 2 {
		t.Fatalf("without source count, page_count = %v, want chunk count 2", got)
	}
}

func TestDoclingPageCountSourceLowerBound(t *testing.T) {
	shortProv := []doclingDocument{{JSONContent: map[string]any{"texts": []any{
		map[string]any{"text": "a", "prov": []any{map[string]any{"page_no": 3.0}}},
	}}}}
	if got := doclingPageCount(shortProv, 9); got != 9 {
		t.Fatalf("provenance ending early must not underreport: got %d, want 9", got)
	}
	fullProv := []doclingDocument{{JSONContent: map[string]any{"texts": []any{
		map[string]any{"text": "a", "prov": []any{map[string]any{"page_no": 12.0}}},
	}}}}
	if got := doclingPageCount(fullProv, 9); got != 12 {
		t.Fatalf("metadata above source count still wins: got %d, want 12", got)
	}
}

func TestDoclingProvPositionConvertsBottomLeftOrigin(t *testing.T) {
	heights := map[int]float64{1: 200}
	prov := map[string]any{"page_no": 1.0, "bbox": map[string]any{
		"l": 10.0, "t": 180.0, "r": 50.0, "b": 170.0, "coord_origin": "BOTTOMLEFT",
	}}
	pos := doclingProvPosition(prov, heights)
	if pos == nil {
		t.Fatal("position dropped")
	}
	// BOTTOMLEFT y=180/170 from the bottom become 20/30 from the top.
	if pos[3] != 20.0 || pos[4] != 30.0 {
		t.Fatalf("BOTTOMLEFT not converted: %#v", pos)
	}
	// Unknown page height: the bottom-origin Y cannot meet the top-origin
	// contract, so the position is dropped rather than emitted wrong.
	if pos = doclingProvPosition(prov, map[int]float64{}); pos != nil {
		t.Fatalf("missing height must drop the position, got %#v", pos)
	}
	// TOPLEFT boxes are untouched even with a known height.
	provTL := map[string]any{"page_no": 1.0, "bbox": map[string]any{
		"l": 10.0, "t": 20.0, "r": 50.0, "b": 30.0, "coord_origin": "TOPLEFT",
	}}
	pos = doclingProvPosition(provTL, heights)
	if pos == nil || pos[3] != 20.0 || pos[4] != 30.0 {
		t.Fatalf("TOPLEFT must pass through unchanged: %#v", pos)
	}
}

func TestDoclingPageHeightsFromPagesMetadata(t *testing.T) {
	asMap := map[string]any{"pages": map[string]any{
		"1": map[string]any{"size": map[string]any{"width": 100.0, "height": 200.0}},
	}}
	if h := doclingPageHeights(asMap)[1]; h != 200.0 {
		t.Fatalf("map-form pages height = %v, want 200", h)
	}
	asList := map[string]any{"pages": []any{
		map[string]any{"page_no": 2.0, "size": map[string]any{"height": 300.0}},
	}}
	if h := doclingPageHeights(asList)[2]; h != 300.0 {
		t.Fatalf("list-form pages height = %v, want 300", h)
	}
}
