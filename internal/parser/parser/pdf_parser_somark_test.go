package parser

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
		"output_format":   "json",
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

func TestPDFParser_ParseWithResult_SoMarkMarkdownIntegration(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-2"}}`))
		case "/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"title","content":"SoMark Title","title_level":1},{"type":"text","content":"Body"}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "SoMark",
		"output_format":   "markdown",
		"somark_base_url": server.URL,
	})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.Markdown, "SoMark Title") {
		t.Fatalf("Markdown = %q, want title", res.Markdown)
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
	if item := soMarkBlockToItem(map[string]any{"type": "header", "content": "x"}, false); item != nil {
		t.Fatalf("item = %#v, want nil", item)
	}
}

// TestSoMarkBlockToItem_TableWithoutMarkupDowngradesToText pins the
// producer-side fix for issue #20143: when SoMark labels a block
// `type=="table"` but the content carries no `<table>` / `<tr>`
// markup, the resulting item must come back as `doc_type_kwd: "text"`
// so the QA chunker does not silently return zero pairs.
func TestSoMarkBlockToItem_TableWithoutMarkupDowngradesToText(t *testing.T) {
	item := soMarkBlockToItem(map[string]any{"type": "table", "content": "col1 | col2\ncol3 | col4"}, false)
	if item == nil {
		t.Fatal("item = nil, want non-nil (content non-empty)")
	}
	if got := item["doc_type_kwd"]; got != "text" {
		t.Fatalf("doc_type_kwd = %v, want text", got)
	}
	if got := item["layout"]; got != "text" {
		t.Fatalf("layout = %v, want text", got)
	}
}

// TestSoMarkBlockToItem_TableWithMarkupKeepsTableLabel pins the
// positive case: when SoMark's content does carry `<table>` / `<tr>`
// markup, the producer keeps the table label.
func TestSoMarkBlockToItem_TableWithMarkupKeepsTableLabel(t *testing.T) {
	item := soMarkBlockToItem(map[string]any{"type": "table", "content": "<table><tr><td>a</td></tr></table>"}, false)
	if item == nil {
		t.Fatal("item = nil, want non-nil")
	}
	if got := item["doc_type_kwd"]; got != "table" {
		t.Fatalf("doc_type_kwd = %v, want table", got)
	}
	if got := item["layout"]; got != "table" {
		t.Fatalf("layout = %v, want table", got)
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
	taskID, err := soMarkSubmit(server.URL, "sample.pdf", []byte("%PDF"), NewPDFParser(), "key")
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
