package parser

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_SoMarkJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
	var submitSeen bool
	var pollSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			submitSeen = true
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				return
			}
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				body, _ := io.ReadAll(part)
				fields[part.FormName()] = string(body)
			}
			if fields["api_key"] != "somark-secret" {
				t.Errorf("api_key = %q, want somark-secret", fields["api_key"])
				return
			}
			if !strings.Contains(fields["element_formats"], "image") {
				t.Errorf("element_formats = %q", fields["element_formats"])
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
		case "/parse/async_check":
			pollSeen = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "task_id=task-1") {
				t.Errorf("poll body = %q, want task_id", string(body))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"title","content":"SoMark Title","title_level":2},{"type":"figure","content":"Figure caption"},{"type":"table","content":"<table><tr><td>x</td></tr></table>"}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "SoMark",
		"somark_base_url": server.URL,
		"somark_api_key":  "somark-secret",
	})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !submitSeen || !pollSeen {
		t.Fatalf("submit/poll seen = %v/%v, want true/true", submitSeen, pollSeen)
	}
	if len(res.JSON) < 3 {
		t.Fatalf("JSON len = %d, want >=3", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_SoMarkRequiresBaseURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "SoMark"})
	pdf.SoMarkBaseURL = ""
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "somark_base_url") {
		t.Fatalf("error = %v, want somark_base_url context", res.Err)
	}
}

func TestSoMarkBlockToItem_DropsHeaderByDefault(t *testing.T) {
	if item, _ := soMarkBlockToItems(t.Context(), map[string]any{"type": "header", "content": "x"}, false, &htmlWalkState{}); item != nil {
		t.Fatalf("item = %#v, want nil", item)
	}
}

func TestSoMarkSubmitMultipartShape(t *testing.T) {
	withSSRFBypass(t)
	var form multipart.Form
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = *r.MultipartForm
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-3"}}`))
	}))
	defer server.Close()
	taskID, err := soMarkSubmit(context.Background(), server.URL, "sample.pdf", []byte("%PDF"), NewPDFParser(), "key")
	if err != nil {
		t.Fatalf("soMarkSubmit: %v", err)
	}
	if taskID != "task-3" {
		t.Fatalf("taskID = %q, want task-3", taskID)
	}
	if got := form.Value["api_key"][0]; got != "key" {
		t.Fatalf("api_key = %q, want key", got)
	}
}

func TestSoMarkItemsEmitPagePositions(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_number": float64(1), "blocks": []any{
			map[string]any{"type": "text", "content": "first", "bbox": []any{10.0, 20.0, 110.0, 40.0}},
		}},
		map[string]any{"page_number": float64(3), "blocks": []any{
			map[string]any{"type": "text", "content": "swapped", "bbox": []any{200.0, 90.0, 100.0, 50.0}},
			map[string]any{"type": "text", "content": "no bbox"},
		}},
	}}}}
	items, pageCount, _ := soMarkItems(context.Background(), result, false)
	if pageCount != 2 || len(items) != 3 {
		t.Fatalf("pageCount=%d items=%d, want 2/3", pageCount, len(items))
	}
	want := [][]any{
		{float64(1), 10.0, 110.0, 20.0, 40.0},
		{float64(3), 100.0, 200.0, 50.0, 90.0},
	}
	for i, w := range want {
		for _, key := range []string{"positions", "_pdf_positions"} {
			rows, _ := items[i][key].([][]any)
			if len(rows) != 1 || !reflect.DeepEqual(rows[0], w) {
				t.Fatalf("item %d %s = %#v, want %#v", i, key, items[i][key], w)
			}
		}
	}
	if _, ok := items[2]["positions"]; ok {
		t.Fatalf("block without bbox must not carry positions: %#v", items[2])
	}
}

func TestSoMarkItemsPreferBlockPageIdx(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": float64(0), "blocks": []any{
			map[string]any{"type": "text", "content": "own page", "page_idx": float64(4), "bbox": []any{10.0, 20.0, 110.0, 40.0}},
			map[string]any{"type": "text", "content": "page fallback", "bbox": []any{10.0, 20.0, 110.0, 40.0}},
		}},
	}}}}
	items, _ := soMarkItems(result, false)
	if len(items) != 2 {
		t.Fatalf("items=%d, want 2", len(items))
	}
	rows, _ := items[0]["_pdf_positions"].([][]any)
	if len(rows) != 1 || rows[0][0] != float64(5) {
		t.Fatalf("block page_idx must win: %#v", items[0]["_pdf_positions"])
	}
	rows, _ = items[1]["_pdf_positions"].([][]any)
	if len(rows) != 1 || rows[0][0] != float64(1) {
		t.Fatalf("missing page_idx must fall back to page_num: %#v", items[1]["_pdf_positions"])
	}
}

func TestPDFParser_ParseWithResult_SoMarkJSONCarriesPositions(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
		case "/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"page_number":2,"blocks":[{"type":"text","content":"hello","bbox":[5,6,50,16]}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "SoMark",
		"output_format":   "json",
		"somark_base_url": server.URL,
		"somark_api_key":  "k",
	})
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	rows, ok := ExtractPDFPositions(res.JSON[0].ToMap())
	if !ok || len(rows) != 1 || len(rows[0]) != 5 {
		t.Fatalf("positions = %#v ok=%v", rows, ok)
	}
	for i, w := range []float64{2, 5, 50, 6, 16} {
		if got, _ := numericAny(rows[0][i]); got != w {
			t.Fatalf("positions[0][%d] = %v, want %v (row %#v)", i, rows[0][i], w, rows[0])
		}
	}
}
