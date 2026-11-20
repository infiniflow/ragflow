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

	"ragflow/internal/entity"
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

// The Markdown export deliberately disagrees with json_content here: the
// body must come from the typed elements, not from md_content.
const doclingPositionsResponse = `{"document":{"md_content":"# Docling Title\n\nDocling body.\n\nOnly in the Markdown export.\n","json_content":{
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
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none for a response with provenance", res.Warnings)
	}
	want := []struct {
		text, layout string
		page         int
		row          []float64
	}{
		// BOTTOMLEFT y=700/680 on a 792pt page convert to top-origin 92/112.
		{"## Docling Title", "heading", 1, []float64{1, 10, 200, 92, 112}},
		{"Docling body.", "text", 2, []float64{2, 15, 300, 100, 140}},
	}
	if len(res.JSON) != len(want) {
		t.Fatalf("items = %v, want the two json_content elements", ItemsToMaps(res.JSON))
	}
	for i, w := range want {
		m := res.JSON[i].ToMap()
		if m["text"] != w.text || m["layout"] != w.layout || m["page_number"] != w.page {
			t.Fatalf("item %d = %v, want text %q layout %q page %d", i, m, w.text, w.layout, w.page)
		}
		rows, ok := ExtractPDFPositions(m)
		if !ok || len(rows) != 1 || len(rows[0]) != 5 {
			t.Fatalf("%q positions = %#v ok=%v", w.text, rows, ok)
		}
		for j := range w.row {
			if got, _ := numericAny(rows[0][j]); got != w.row[j] {
				t.Fatalf("%q positions[%d] = %v, want %v (row %#v)", w.text, j, rows[0][j], w.row[j], rows[0])
			}
		}
	}
	if level := res.JSON[0].ToMap()["heading_level"]; level != 2 {
		t.Fatalf("heading_level = %v, want 2 (a section header sits below the title)", level)
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

// doclingStandardItems parses a standard response built around jsonContent
// and returns the normalized items with the parse warnings.
func doclingStandardItems(t *testing.T, markdown, jsonContent string) ([]map[string]any, []string) {
	t.Helper()
	doc := map[string]any{"md_content": markdown}
	if jsonContent != "" {
		var content map[string]any
		if err := json.Unmarshal([]byte(jsonContent), &content); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		doc["json_content"] = content
	}
	body, err := json.Marshal(map[string]any{"document": doc})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := parseDoclingStandardResult(t.Context(), "sample.pdf", body, 0)
	if !ok || res.Err != nil {
		t.Fatalf("parseDoclingStandardResult ok=%v err=%v", ok, res.Err)
	}
	res = NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", res)
	if res.Err != nil {
		t.Fatalf("NormalizeResult: %v", res.Err)
	}
	return ItemsToMaps(res.JSON), res.Warnings
}

func doclingItemPage(t *testing.T, item map[string]any) int {
	t.Helper()
	rows, ok := ExtractPDFPositions(item)
	if !ok || len(rows) != 1 {
		t.Fatalf("item %v: want one position row, got %#v", item["text"], rows)
	}
	page, _ := numericAny(rows[0][0])
	return int(page)
}

// Each element carries its own provenance, so identical text on two pages
// cannot trade positions the way matching Markdown text back to it could.
func TestDoclingStructuredKeepsRepeatedTextProvenance(t *testing.T) {
	items, _ := doclingStandardItems(t, "Repeated paragraph.\n\nRepeated paragraph.\n", `{"texts":[
		{"label":"text","text":"Repeated paragraph.","prov":[{"page_no":5,"bbox":{"l":10,"t":20,"r":100,"b":40}}]},
		{"label":"text","text":"Repeated paragraph.","prov":[{"page_no":2,"bbox":{"l":10,"t":20,"r":100,"b":40}}]}
	]}`)
	if len(items) != 2 {
		t.Fatalf("items = %v, want 2", items)
	}
	if first, second := doclingItemPage(t, items[0]), doclingItemPage(t, items[1]); first != 5 || second != 2 {
		t.Fatalf("pages = %d, %d; want each occurrence on its own page (5, 2)", first, second)
	}
}

// Headings and list items are typed by their label. Their text is never
// stripped of a Markdown marker to find a match, so leading digits survive.
func TestDoclingStructuredLabelsDecideLayout(t *testing.T) {
	prov := `"prov":[{"page_no":1,"bbox":{"l":10,"t":20,"r":100,"b":40}}]`
	items, _ := doclingStandardItems(t, "", `{"texts":[
		{"label":"title","text":"2026 Outlook",`+prov+`},
		{"label":"section_header","level":2,"text":"1. Introduction",`+prov+`},
		{"label":"list_item","text":"plain bullet",`+prov+`},
		{"label":"list_item","enumerated":true,"marker":"3.","text":"numbered",`+prov+`},
		{"label":"code","text":"x := 1",`+prov+`},
		{"label":"formula","text":"E = mc^2",`+prov+`},
		{"label":"footnote","text":"1 A footnote.",`+prov+`},
		{"label":"text","text":"","orig":"kept from orig",`+prov+`},
		{"label":"page_header","text":"Running header",`+prov+`},
		{"label":"page_footer","text":"7",`+prov+`},
		{"label":"text","content_layer":"furniture","text":"Watermark",`+prov+`}
	]}`)
	want := []struct {
		text, layout string
		level        any
	}{
		{"# 2026 Outlook", "heading", 1},
		{"### 1. Introduction", "heading", 3},
		{"- plain bullet", "list", nil},
		{"3. numbered", "list", nil},
		{"x := 1", "code", nil},
		{"E = mc^2", "equation", nil},
		{"1 A footnote.", "text", nil},
		{"kept from orig", "text", nil},
	}
	if len(items) != len(want) {
		t.Fatalf("items = %v, want %d (running header, footer and furniture dropped)", items, len(want))
	}
	for i, w := range want {
		if items[i]["text"] != w.text || items[i]["layout"] != w.layout || items[i]["heading_level"] != w.level {
			t.Errorf("item %d = %v, want text %q layout %q heading_level %v", i, items[i], w.text, w.layout, w.level)
		}
		if items[i]["doc_type_kwd"] != "text" || items[i]["page_number"] != 1 {
			t.Errorf("item %d = %v, want a text item on page 1", i, items[i])
		}
	}
}

// The body tree is the reading order. Captions and footnotes travel with
// their table or picture instead of becoming loose paragraphs, a merged cell
// keeps its span, and text recognized inside a picture follows the picture.
func TestDoclingStructuredFollowsBodyTree(t *testing.T) {
	items, warnings := doclingStandardItems(t, "", `{
	 "pages":{"1":{"size":{"width":612,"height":792}}},
	 "body":{"children":[{"$ref":"#/texts/0"},{"$ref":"#/tables/0"},{"$ref":"#/groups/0"},{"$ref":"#/pictures/0"},{"$ref":"#/groups/1"},{"$ref":"#/texts/9"}]},
	 "groups":[
	  {"label":"list","children":[{"$ref":"#/texts/3"},{"$ref":"#/texts/4"}]},
	  {"label":"inline","children":[{"$ref":"#/texts/7"},{"$ref":"#/texts/8"}]}
	 ],
	 "texts":[
	  {"label":"text","text":"Before the table.","prov":[{"page_no":1,"bbox":{"l":72,"t":80,"r":540,"b":100,"coord_origin":"TOPLEFT"}}]},
	  {"label":"caption","text":"Table 1: totals","prov":[{"page_no":1,"bbox":{"l":72,"t":110,"r":540,"b":125,"coord_origin":"TOPLEFT"}}]},
	  {"label":"footnote","text":"a Preliminary figures.","prov":[{"page_no":1,"bbox":{"l":72,"t":260,"r":540,"b":275,"coord_origin":"TOPLEFT"}}]},
	  {"label":"list_item","text":"first","prov":[{"page_no":1,"bbox":{"l":90,"t":290,"r":540,"b":305,"coord_origin":"TOPLEFT"}}]},
	  {"label":"list_item","text":"second","prov":[{"page_no":1,"bbox":{"l":90,"t":310,"r":540,"b":325,"coord_origin":"TOPLEFT"}}]},
	  {"label":"caption","text":"Figure 1: pipeline","prov":[{"page_no":1,"bbox":{"l":72,"t":520,"r":540,"b":535,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"600 sec","prov":[{"page_no":1,"bbox":{"l":200,"t":400,"r":260,"b":415,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"Inline","prov":[{"page_no":1,"bbox":{"l":72,"t":560,"r":120,"b":575,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"paragraph.","prov":[{"page_no":1,"bbox":{"l":125,"t":560,"r":200,"b":575,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"After everything.","prov":[{"page_no":1,"bbox":{"l":72,"t":600,"r":540,"b":620,"coord_origin":"TOPLEFT"}}]}
	 ],
	 "tables":[{"label":"table","captions":[{"$ref":"#/texts/1"}],"footnotes":[{"$ref":"#/texts/2"}],
	  "children":[{"$ref":"#/texts/1"},{"$ref":"#/texts/2"}],
	  "prov":[{"page_no":1,"bbox":{"l":72,"t":130,"r":540,"b":250,"coord_origin":"TOPLEFT"}}],
	  "data":{"num_rows":2,"num_cols":2,"grid":[
	   [{"text":"Region & total","column_header":true,"col_span":2,"row_span":1,"start_row_offset_idx":0,"start_col_offset_idx":0},
	    {"text":"Region & total","column_header":true,"col_span":2,"row_span":1,"start_row_offset_idx":0,"start_col_offset_idx":0}],
	   [{"text":"North","start_row_offset_idx":1,"start_col_offset_idx":0},{"text":"42","start_row_offset_idx":1,"start_col_offset_idx":1}]
	  ]}}],
	 "pictures":[{"label":"picture","captions":[{"$ref":"#/texts/5"}],"children":[{"$ref":"#/texts/5"},{"$ref":"#/texts/6"}],
	  "prov":[{"page_no":1,"bbox":{"l":72,"t":340,"r":540,"b":510,"coord_origin":"TOPLEFT"}}]}]
	}`)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %q, want none", warnings)
	}
	type shape struct{ docType, layout, text string }
	want := []shape{
		{"text", "text", "Before the table."},
		{"table", "table", ""},
		{"text", "text", "a Preliminary figures."},
		{"text", "list", "- first"},
		{"text", "list", "- second"},
		{"image", "figure", "Figure 1: pipeline"},
		{"text", "text", "600 sec"},
		{"text", "text", "Inline paragraph."},
		{"text", "text", "After everything."},
	}
	if len(items) != len(want) {
		t.Fatalf("items = %v, want %d", items, len(want))
	}
	for i, w := range want {
		text, _ := items[i]["text"].(string)
		if items[i]["doc_type_kwd"] != w.docType || items[i]["layout"] != w.layout || text != w.text {
			t.Errorf("item %d = %v, want %+v", i, items[i], w)
		}
		if items[i]["page_number"] != 1 {
			t.Errorf("item %d page_number = %v, want 1", i, items[i]["page_number"])
		}
		if items[i]["_preserve_order"] != true {
			t.Errorf("item %d = %v, want the body order declared authoritative", i, items[i])
		}
	}
	td, ok := items[1]["table"].(*entity.TableData)
	if !ok || td == nil {
		t.Fatalf("table item carries no TableData: %v", items[1])
	}
	if td.Caption != "Table 1: totals" || td.Page != 1 || td.HeaderRows != 1 {
		t.Errorf("table caption/page/header rows = %q/%d/%d, want the caption on page 1 with one header row", td.Caption, td.Page, td.HeaderRows)
	}
	if rows := td.CellTexts(); len(rows) != 2 || len(rows[0]) != 1 || rows[0][0] != "Region & total" || len(rows[1]) != 2 || rows[1][1] != "42" {
		t.Errorf("table rows = %v, want the merged header written once", rows)
	}
	if !HasPDFCropRegion(items[5]) {
		t.Errorf("picture item lost its crop region: %v", items[5])
	}
	// The inline paragraph spans both of its fragments.
	if rows, _ := ExtractPDFPositions(items[7]); len(rows) != 2 {
		t.Errorf("inline paragraph positions = %v, want one row per fragment", rows)
	}
}

// The body tree is Docling's reading order. On a two-column page it differs
// from top-to-bottom order, which is how the chunker sorts positioned items
// unless their order is declared authoritative. The flat element lists carry
// no reading order, so there the chunker's own order stands.
func TestDoclingStructuredDeclaresBodyOrderAuthoritative(t *testing.T) {
	const texts = `"texts":[
	  {"label":"text","text":"left column, top","prov":[{"page_no":1,"bbox":{"l":72,"t":100,"r":290,"b":120,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"left column, bottom","prov":[{"page_no":1,"bbox":{"l":72,"t":600,"r":290,"b":620,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"right column, top","prov":[{"page_no":1,"bbox":{"l":320,"t":90,"r":540,"b":110,"coord_origin":"TOPLEFT"}}]}
	 ]`
	items, _ := doclingStandardItems(t, "", `{"body":{"children":[{"$ref":"#/texts/0"},{"$ref":"#/texts/1"},{"$ref":"#/texts/2"}]},`+texts+`}`)
	if len(items) != 3 || items[1]["text"] != "left column, bottom" || items[2]["text"] != "right column, top" {
		t.Fatalf("items = %v, want the body order", items)
	}
	for i, item := range items {
		if item["_preserve_order"] != true {
			t.Errorf("item %d = %v, want _preserve_order so the right column is not sorted above the left", i, item)
		}
	}
	items, _ = doclingStandardItems(t, "", `{`+texts+`}`)
	for i, item := range items {
		if _, ok := item["_preserve_order"]; ok {
			t.Errorf("flat item %d = %v, want no order claim for element lists", i, item)
		}
	}
}

// An inline group is one paragraph. Its text may sit under a nested group, a
// fragment that is page furniture or a running footer is not part of it, and
// a picture among the fragments follows the paragraph as its own item.
func TestDoclingStructuredInlineGroup(t *testing.T) {
	prov := func(left int) string {
		return fmt.Sprintf(`"prov":[{"page_no":1,"bbox":{"l":%d,"t":100,"r":%d,"b":115,"coord_origin":"TOPLEFT"}}]`, left, left+40)
	}
	items, _ := doclingStandardItems(t, "", `{
	 "body":{"children":[{"$ref":"#/groups/0"},{"$ref":"#/texts/6"}]},
	 "groups":[
	  {"label":"inline","children":[{"$ref":"#/texts/0"},{"$ref":"#/groups/1"},{"$ref":"#/pictures/0"},{"$ref":"#/texts/3"},{"$ref":"#/texts/4"},{"$ref":"#/texts/5"}]},
	  {"label":"unspecified","children":[{"$ref":"#/texts/1"},{"$ref":"#/texts/2"}]}
	 ],
	 "texts":[
	  {"label":"text","text":"Energy is",`+prov(72)+`},
	  {"label":"formula","text":"E = mc^2",`+prov(120)+`},
	  {"label":"text","text":"exactly",`+prov(170)+`},
	  {"label":"text","content_layer":"furniture","text":"DRAFT",`+prov(220)+`},
	  {"label":"page_footer","text":"7",`+prov(270)+`},
	  {"label":"text","text":"as shown.",`+prov(320)+`},
	  {"label":"text","text":"Next paragraph.","prov":[{"page_no":1,"bbox":{"l":72,"t":300,"r":540,"b":320,"coord_origin":"TOPLEFT"}}]}
	 ],
	 "pictures":[{"label":"picture","prov":[{"page_no":1,"bbox":{"l":72,"t":130,"r":200,"b":200,"coord_origin":"TOPLEFT"}}]}]
	}`)
	if len(items) != 3 {
		t.Fatalf("items = %v, want the paragraph, the picture and the next paragraph", items)
	}
	if items[0]["text"] != "Energy is E = mc^2 exactly as shown." || items[0]["layout"] != "text" {
		t.Fatalf("paragraph = %v, want the nested fragments joined and the furniture left out", items[0])
	}
	rows, _ := ExtractPDFPositions(items[0])
	if len(rows) != 4 {
		t.Fatalf("paragraph positions = %v, want one row per body fragment", rows)
	}
	for _, row := range rows {
		if left, _ := numericAny(row[1]); left == 220 || left == 270 {
			t.Fatalf("paragraph positions = %v, want no region from the furniture or the footer", rows)
		}
	}
	if items[1]["doc_type_kwd"] != "image" || !HasPDFCropRegion(items[1]) {
		t.Fatalf("item 1 = %v, want the picture after the paragraph", items[1])
	}
	if items[2]["text"] != "Next paragraph." {
		t.Fatalf("item 2 = %v, want the next paragraph", items[2])
	}
}

// A caption or footnote belongs to its table or picture wherever it sits in
// the document: listed before its owner in the flat element lists, or placed
// ahead of it in the body tree. It is never emitted as a paragraph as well.
func TestDoclingStructuredCaptionsStayWithTheirOwner(t *testing.T) {
	const elements = `
	 "texts":[
	  {"label":"caption","text":"Table 1: totals","prov":[{"page_no":1,"bbox":{"l":72,"t":100,"r":540,"b":115,"coord_origin":"TOPLEFT"}}]},
	  {"label":"text","text":"Body.","prov":[{"page_no":1,"bbox":{"l":72,"t":60,"r":540,"b":80,"coord_origin":"TOPLEFT"}}]},
	  {"label":"footnote","text":"a Preliminary.","prov":[{"page_no":1,"bbox":{"l":72,"t":260,"r":540,"b":275,"coord_origin":"TOPLEFT"}}]},
	  {"label":"caption","text":"Figure 1: pipeline","prov":[{"page_no":1,"bbox":{"l":72,"t":480,"r":540,"b":495,"coord_origin":"TOPLEFT"}}]}
	 ],
	 "tables":[{"label":"table","captions":[{"$ref":"#/texts/0"}],"footnotes":[{"$ref":"#/texts/2"}],
	  "prov":[{"page_no":1,"bbox":{"l":72,"t":120,"r":540,"b":250,"coord_origin":"TOPLEFT"}}],
	  "data":{"grid":[[{"text":"Region"},{"text":"Total"}],[{"text":"North"},{"text":"42"}]]}}],
	 "pictures":[{"label":"picture","captions":[{"$ref":"#/texts/3"}],
	  "prov":[{"page_no":1,"bbox":{"l":72,"t":300,"r":540,"b":470,"coord_origin":"TOPLEFT"}}]}]`
	for name, body := range map[string]string{
		"flat element lists":             ``,
		"caption ahead of its owner":     `"body":{"children":[{"$ref":"#/texts/1"},{"$ref":"#/texts/0"},{"$ref":"#/tables/0"},{"$ref":"#/texts/2"},{"$ref":"#/texts/3"},{"$ref":"#/pictures/0"}]},`,
		"caption nested under its owner": `"body":{"children":[{"$ref":"#/texts/1"},{"$ref":"#/tables/0"},{"$ref":"#/pictures/0"}]},`,
	} {
		t.Run(name, func(t *testing.T) {
			items, _ := doclingStandardItems(t, "", `{`+body+elements+`}`)
			type shape struct{ docType, text string }
			want := []shape{{"text", "Body."}, {"table", ""}, {"text", "a Preliminary."}, {"image", "Figure 1: pipeline"}}
			if len(items) != len(want) {
				t.Fatalf("items = %v, want %d (no caption as a paragraph of its own)", items, len(want))
			}
			for i, w := range want {
				text, _ := items[i]["text"].(string)
				if items[i]["doc_type_kwd"] != w.docType || text != w.text {
					t.Errorf("item %d = %v, want %+v", i, items[i], w)
				}
			}
			if td, _ := items[1]["table"].(*entity.TableData); td == nil || td.Caption != "Table 1: totals" {
				t.Errorf("table = %v, want its caption attached", items[1])
			}
		})
	}
}

// A table without a grid is read from its cell list, in row and column order.
func TestDoclingTableHTMLFromCellList(t *testing.T) {
	data := map[string]any{"table_cells": []any{
		map[string]any{"text": "42", "start_row_offset_idx": 1.0, "start_col_offset_idx": 1.0},
		map[string]any{"text": "Total", "column_header": true, "col_span": 2.0, "start_row_offset_idx": 0.0, "start_col_offset_idx": 0.0},
		map[string]any{"text": "North", "start_row_offset_idx": 1.0, "start_col_offset_idx": 0.0},
	}}
	const want = `<table><tr><th colspan="2">Total</th></tr><tr><td>North</td><td>42</td></tr></table>`
	if got := doclingTableHTML(data); got != want {
		t.Fatalf("markup = %s, want %s", got, want)
	}
	if got := doclingTableHTML(map[string]any{"grid": []any{[]any{map[string]any{"text": " "}}}}); got != "" {
		t.Fatalf("a table with no cell text rendered %q", got)
	}
}

// A picture with no caption is kept for its crop region and carries no
// placeholder text; one with neither text nor a region is dropped.
func TestDoclingStructuredPictureWithoutText(t *testing.T) {
	items, _ := doclingStandardItems(t, "", `{
	 "texts":[{"label":"text","text":"Body.","prov":[{"page_no":1,"bbox":{"l":10,"t":20,"r":100,"b":40}}]}],
	 "pictures":[
	  {"label":"picture","prov":[{"page_no":1,"bbox":{"l":10,"t":50,"r":100,"b":90}}]},
	  {"label":"picture"}
	 ]}`)
	if len(items) != 2 {
		t.Fatalf("items = %v, want the body text and the located picture", items)
	}
	picture := items[1]
	if picture["doc_type_kwd"] != "image" || picture["layout"] != "figure" || !HasPDFCropRegion(picture) {
		t.Fatalf("picture = %v, want an image item with a crop region", picture)
	}
	if text, _ := picture["text"].(string); text != "" {
		t.Fatalf("picture text = %q, want none (a placeholder would be indexed as body text)", text)
	}
}

// Without provenance the typed elements cannot be placed on a page, so the
// Markdown export is used and the loss of positions is reported.
func TestDoclingStandardFallsBackToMarkdownWithoutProvenance(t *testing.T) {
	for name, jsonContent := range map[string]string{
		"no json_content":    "",
		"no texts":           `{"texts":[]}`,
		"no prov":            `{"texts":[{"label":"text","text":"From json."}]}`,
		"unconvertible prov": `{"texts":[{"label":"text","text":"From json.","prov":[{"page_no":1,"bbox":{"l":10,"t":700,"r":200,"b":680,"coord_origin":"BOTTOMLEFT"}}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			items, warnings := doclingStandardItems(t, "# From Markdown\n\nBody.\n", jsonContent)
			if len(items) != 2 || items[0]["text"] != "# From Markdown" {
				t.Fatalf("items = %v, want the Markdown export", items)
			}
			for _, item := range items {
				if _, ok := ExtractPDFPositions(item); ok {
					t.Fatalf("fallback item carries positions: %v", item)
				}
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], "positions are unavailable") {
				t.Fatalf("warnings = %q, want the missing-provenance warning", warnings)
			}
		})
	}
	// With no Markdown either, the typed elements are still the document.
	items, warnings := doclingStandardItems(t, "", `{"texts":[{"label":"title","text":"Only json"}]}`)
	if len(items) != 1 || items[0]["text"] != "# Only json" || len(warnings) != 0 {
		t.Fatalf("items = %v warnings = %q, want the json element", items, warnings)
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

func TestDoclingRegionsConvertBottomLeftOrigin(t *testing.T) {
	bottomLeft := `{"label":"text","text":"bottom","prov":[{"page_no":1,"bbox":{"l":10,"t":180,"r":50,"b":170,"coord_origin":"BOTTOMLEFT"}}]}`
	topLeft := `{"label":"text","text":"top","prov":[{"page_no":1,"bbox":{"l":10,"t":20,"r":50,"b":30,"coord_origin":"TOPLEFT"}}]}`
	row := func(item map[string]any) []any {
		rows, _ := ExtractPDFPositions(item)
		if len(rows) != 1 {
			return nil
		}
		return rows[0]
	}
	items, _ := doclingStandardItems(t, "", `{"pages":{"1":{"size":{"width":100,"height":200}}},"texts":[`+bottomLeft+`,`+topLeft+`]}`)
	// BOTTOMLEFT y=180/170 from the bottom become 20/30 from the top.
	if pos := row(items[0]); pos == nil || pos[3] != 20.0 || pos[4] != 30.0 {
		t.Fatalf("BOTTOMLEFT not converted: %#v", pos)
	}
	if pos := row(items[1]); pos == nil || pos[3] != 20.0 || pos[4] != 30.0 {
		t.Fatalf("TOPLEFT must pass through unchanged: %#v", pos)
	}
	// Unknown page height: the bottom-origin Y cannot meet the top-origin
	// contract, so the position is dropped rather than emitted wrong. A
	// TOPLEFT box needs no page size.
	items, _ = doclingStandardItems(t, "", `{"texts":[`+bottomLeft+`,`+topLeft+`]}`)
	if pos := row(items[0]); pos != nil {
		t.Fatalf("missing height must drop the position, got %#v", pos)
	}
	if items[0]["page_number"] != 1 {
		t.Fatalf("page identity lost with the position: %v", items[0])
	}
	if pos := row(items[1]); pos == nil || pos[3] != 20.0 || pos[4] != 30.0 {
		t.Fatalf("TOPLEFT without a page size must be kept: %#v", pos)
	}
}

func TestDoclingPageSizesFromPagesMetadata(t *testing.T) {
	asMap := map[string]any{"pages": map[string]any{
		"1": map[string]any{"size": map[string]any{"width": 100.0, "height": 200.0}},
	}}
	if size := doclingPageSizes(asMap)[1]; size != [2]float64{100, 200} {
		t.Fatalf("map-form page size = %v, want [100 200]", size)
	}
	asList := map[string]any{"pages": []any{
		map[string]any{"page_no": 2.0, "size": map[string]any{"width": 150.0, "height": 300.0}},
	}}
	if size := doclingPageSizes(asList)[2]; size != [2]float64{150, 300} {
		t.Fatalf("list-form page size = %v, want [150 300]", size)
	}
}
