//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// mistralOCRResponseFixture mirrors a live /v1/ocr response with
// include_blocks=true: page 0 carries blocks (a header to drop, a title whose
// content already holds its Markdown heading marker, as the live API sends
// it, a text paragraph, an image and an HTML table); page 1 carries no
// blocks, so its Markdown is used with the table placeholder inlined and the
// image placeholder removed.
const mistralOCRResponseFixture = `{
  "model": "mistral-ocr-latest",
  "usage_info": {"pages_processed": 2},
  "pages": [
    {
      "index": 0,
      "dimensions": {"dpi": 87, "width": 720, "height": 1018},
      "markdown": "# Title\n\nhello world",
      "images": [{"id": "img-0.jpeg"}],
      "tables": [{"id": "tbl-0.html", "content": "<table><tr><td>a</td></tr></table>", "format": "html"}],
      "blocks": [
        {"type": "header", "content": "Co-funded by the EU"},
        {"type": "title", "content": "# Title"},
        {"type": "text", "content": "  hello world  "},
        {"type": "image", "content": ""},
        {"type": "table", "content": "", "table_id": "tbl-0.html"},
        {"type": "footer", "content": "Page 1"}
      ]
    },
    {
      "index": 1,
      "dimensions": {"dpi": 144, "width": 1021, "height": 681},
      "markdown": "## Second\n\n![img-1.jpeg](img-1.jpeg)\n\n[tbl-1.html](tbl-1.html)\n\nclosing",
      "images": [{"id": "img-1.jpeg"}],
      "tables": [{"id": "tbl-1.html", "content": "<table><tr><td>b</td></tr></table>", "format": "html"}]
    }
  ]
}`

type mistralOCRRecorder struct {
	mu       sync.Mutex
	paths    []string
	ocrBody  map[string]any
	auth     string
	deleted  []string
	upload   map[string]string
	uploadSz int
	// signedURLStatus, when set, is the status of the signed-URL request.
	signedURLStatus int
}

func (r *mistralOCRRecorder) record(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

// newMistralOCRServer serves the Mistral OCR and Files endpoints under
// /v1, recording what the driver sent.
func newMistralOCRServer(t *testing.T, rec *mistralOCRRecorder, ocrStatus int, ocrResponse string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method + " " + r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ocr":
			rec.mu.Lock()
			rec.auth = r.Header.Get("Authorization")
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &rec.ocrBody); err != nil {
				t.Errorf("invalid OCR JSON body: %v", err)
			}
			rec.mu.Unlock()
			w.WriteHeader(ocrStatus)
			_, _ = w.Write([]byte(ocrResponse))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			if err := r.ParseMultipartForm(64 << 20); err != nil {
				t.Errorf("parse upload: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Errorf("upload file part: %v", err)
				return
			}
			data, _ := io.ReadAll(file)
			rec.mu.Lock()
			rec.upload = map[string]string{"purpose": r.FormValue("purpose"), "filename": header.Filename}
			rec.uploadSz = len(data)
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"id":"file-1","purpose":"ocr"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-1/url":
			if rec.signedURLStatus != 0 {
				w.WriteHeader(rec.signedURLStatus)
				_, _ = w.Write([]byte(`{"message":"signed url unavailable"}`))
				return
			}
			_, _ = w.Write([]byte(`{"url":"https://signed.example/doc.pdf"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/file-1":
			rec.mu.Lock()
			rec.deleted = append(rec.deleted, "file-1")
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"id":"file-1","deleted":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func newMistralOCRDriverForTest() *MistralModel {
	return NewMistralModel(
		map[string]string{"default": "https://api.mistral.ai"},
		URLSuffix{OCR: "v1/ocr", Files: "v1/files"},
	)
}

func mistralOCRAPIConfig(apiKey, baseURL string) *APIConfig {
	return &APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}
}

func TestMistralOCRFileSendsInlinePDFRequest(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusOK, mistralOCRResponseFixture)
	defer srv.Close()

	model := "mistral-ocr-latest"
	pdf := []byte("%PDF-1.4 tiny")
	// A base URL that already ends in /v1 (the documented form) must not yield /v1/v1/ocr.
	resp, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, pdf, nil,
		mistralOCRAPIConfig("sk-test", srv.URL+"/v1/"), nil, nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if resp == nil || resp.Text == nil {
		t.Fatal("OCRFile returned no text")
	}
	if rec.auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", rec.auth)
	}
	if got, want := rec.paths, []string{"POST /v1/ocr"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	body := rec.ocrBody
	if body["model"] != model {
		t.Errorf("model = %v, want %s", body["model"], model)
	}
	if body["table_format"] != "html" {
		t.Errorf("table_format = %v, want html", body["table_format"])
	}
	if body["include_blocks"] != true {
		t.Errorf("include_blocks = %v, want true", body["include_blocks"])
	}
	if body["include_image_base64"] != false {
		t.Errorf("include_image_base64 = %v, want false", body["include_image_base64"])
	}
	if _, ok := body["pages"]; ok {
		t.Errorf("pages = %v, want absent without a page selection", body["pages"])
	}
	doc, _ := body["document"].(map[string]any)
	if doc["type"] != "document_url" {
		t.Errorf("document.type = %v, want document_url", doc["type"])
	}
	if url, _ := doc["document_url"].(string); !strings.HasPrefix(url, "data:application/pdf;base64,") {
		t.Errorf("document_url = %.60q, want a PDF data URL", url)
	}
}

func TestMistralOCRFileDetectsPDFWithLeadingBytes(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusOK, mistralOCRResponseFixture)
	defer srv.Close()

	model := "mistral-ocr-latest"
	// Real-world PDFs often carry bytes before the header; content sniffing
	// alone labels them text/plain, which the OCR API rejects with 422.
	pdf := []byte("\r\n%PDF-1.4\r\n%\xfb\xfc\r\n1 0 obj\r\n<<>>\r\nendobj")
	if _, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, pdf, nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	doc, _ := rec.ocrBody["document"].(map[string]any)
	if url, _ := doc["document_url"].(string); !strings.HasPrefix(url, "data:application/pdf;base64,") {
		t.Errorf("document_url = %.60q, want a PDF data URL", url)
	}
}

func TestMistralOCRMimeTypeDropsParameters(t *testing.T) {
	if got := mistralOCRMimeType([]byte("plain text, not a document")); got != "text/plain" {
		t.Errorf("mistralOCRMimeType(text) = %q, want text/plain", got)
	}
}

// An image whose first kilobyte happens to contain "%PDF-" (metadata, an
// embedded thumbnail) is still an image: its signature wins over the PDF
// header window.
func TestMistralOCRMimeTypePrefersImageSignature(t *testing.T) {
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0x0D, 0x49, 0x48, 0x44, 0x52}, []byte("tEXt%PDF-1.4")...)
	if got := mistralOCRMimeType(png); got != "image/png" {
		t.Errorf("mistralOCRMimeType(png with %%PDF-) = %q, want image/png", got)
	}
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("JFIF %PDF-1.7")...)
	if got := mistralOCRMimeType(jpeg); got != "image/jpeg" {
		t.Errorf("mistralOCRMimeType(jpeg with %%PDF-) = %q, want image/jpeg", got)
	}
}

// Title blocks render as Markdown headings so block-built pages match pages
// that fall back to the API Markdown. The live API already prefixes the
// content with its heading marker ("## Section"); a bare title gets "# ".
func TestMistralOCRMarkdownRendersTitleBlocksAsHeadings(t *testing.T) {
	var result mistralOCRResponse
	if err := json.Unmarshal([]byte(`{"pages":[{"index":0,"blocks":[
		{"type":"title","content":"# Annual Report"},
		{"type":"title","content":"## Summary"},
		{"type":"title","content":"Plain Title"},
		{"type":"text","content":"Body text."}
	]}]}`), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "# Annual Report\n\n## Summary\n\n# Plain Title\n\nBody text."
	if got := result.markdown(); got != want {
		t.Fatalf("markdown =\n%q\nwant\n%q", got, want)
	}
}

func TestMistralOCRSendRejectsOversizedResponse(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 2048))
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = newMistralOCRDriverForTest().mistralOCRSend(req, 1024)
	if err == nil || !strings.Contains(err.Error(), "exceeds 1024 bytes") {
		t.Fatalf("error = %v, want the response size limit", err)
	}
}

func TestMistralOCRFileJoinsPagesAndInlinesTables(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusOK, mistralOCRResponseFixture)
	defer srv.Close()

	model := "mistral-ocr-latest"
	resp, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, []byte("%PDF-1.4"), nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	want := strings.Join([]string{
		"# Title",
		"hello world",
		"<table><tr><td>a</td></tr></table>",
		"## Second",
		"<table><tr><td>b</td></tr></table>",
		"closing",
	}, "\n\n")
	if got := *resp.Text; got != want {
		t.Fatalf("text =\n%s\nwant\n%s", got, want)
	}
}

func TestMistralOCRFileForwardsPageSelection(t *testing.T) {
	withSSRFBypass(t)
	cases := []struct {
		name   string
		ranges [][]int
		want   []any
	}{
		{"one-indexed ranges become zero-based indexes", [][]int{{2, 3}, {5, 5}}, []any{1.0, 2.0, 4.0}},
		{"selection is bounded by the 1000-page API limit", [][]int{{999, 100000}}, []any{998.0, 999.0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &mistralOCRRecorder{}
			srv := newMistralOCRServer(t, rec, http.StatusOK, mistralOCRResponseFixture)
			defer srv.Close()

			model := "mistral-ocr-latest"
			if _, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, []byte("%PDF-1.4"), nil,
				mistralOCRAPIConfig("sk-test", srv.URL), &OCRConfig{Pages: tc.ranges}, nil); err != nil {
				t.Fatalf("OCRFile: %v", err)
			}
			if got := rec.ocrBody["pages"]; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("pages = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMistralOCRFileSendsImagesAsImageURL(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusOK, `{"pages":[]}`)
	defer srv.Close()

	model := "mistral-ocr-latest"
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0x0D, 0x49, 0x48, 0x44, 0x52}
	if _, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, png, nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	doc, _ := rec.ocrBody["document"].(map[string]any)
	if doc["type"] != "image_url" {
		t.Fatalf("document.type = %v, want image_url", doc["type"])
	}
	if url, _ := doc["image_url"].(string); !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("image_url = %.40q, want a PNG data URL", url)
	}
}

func TestMistralOCRFileSendsRemoteDocumentURL(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusOK, `{"pages":[]}`)
	defer srv.Close()

	model := "mistral-ocr-latest"
	remote := "https://example.com/report.pdf"
	if _, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, nil, &remote,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	doc, _ := rec.ocrBody["document"].(map[string]any)
	if doc["type"] != "document_url" || doc["document_url"] != remote {
		t.Fatalf("document = %v, want document_url %s", doc, remote)
	}
}

func TestMistralOCRFileUploadsLargeDocuments(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusOK, mistralOCRResponseFixture)
	defer srv.Close()

	model := "mistral-ocr-latest"
	large := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), mistralOCRInlineMaxBytes)...)
	if _, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, large, nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	want := []string{
		"POST /v1/files",
		"GET /v1/files/file-1/url?expiry=24",
		"POST /v1/ocr",
		"DELETE /v1/files/file-1",
	}
	if !reflect.DeepEqual(rec.paths, want) {
		t.Fatalf("requests = %v, want %v", rec.paths, want)
	}
	if rec.upload["purpose"] != "ocr" || rec.upload["filename"] != "document.pdf" || rec.uploadSz != len(large) {
		t.Errorf("upload = %v (%d bytes), want purpose=ocr document.pdf %d bytes", rec.upload, rec.uploadSz, len(large))
	}
	doc, _ := rec.ocrBody["document"].(map[string]any)
	if doc["document_url"] != "https://signed.example/doc.pdf" {
		t.Errorf("document_url = %v, want the signed URL", doc["document_url"])
	}
}

func TestMistralOCRFileDeletesUploadWhenOCRFails(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusInternalServerError, `{"message":"boom"}`)
	defer srv.Close()

	model := "mistral-ocr-latest"
	large := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), mistralOCRInlineMaxBytes)...)
	if _, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, large, nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil); err == nil {
		t.Fatal("OCRFile: want error, got nil")
	}
	if !reflect.DeepEqual(rec.deleted, []string{"file-1"}) {
		t.Fatalf("deleted = %v, want the uploaded file removed", rec.deleted)
	}
}

func TestMistralOCRFileDeletesUploadWhenSignedURLFails(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{signedURLStatus: http.StatusInternalServerError}
	srv := newMistralOCRServer(t, rec, http.StatusOK, mistralOCRResponseFixture)
	defer srv.Close()

	model := "mistral-ocr-latest"
	large := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), mistralOCRInlineMaxBytes)...)
	_, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, large, nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "signed url") {
		t.Fatalf("OCRFile error = %v, want the signed url failure", err)
	}
	want := []string{
		"POST /v1/files",
		"GET /v1/files/file-1/url?expiry=24",
		"DELETE /v1/files/file-1",
	}
	if !reflect.DeepEqual(rec.paths, want) {
		t.Fatalf("requests = %v, want %v (no OCR call, upload deleted)", rec.paths, want)
	}
}

func TestMistralOCRFileReportsHTTPErrors(t *testing.T) {
	withSSRFBypass(t)
	rec := &mistralOCRRecorder{}
	srv := newMistralOCRServer(t, rec, http.StatusUnprocessableEntity, `{"detail":"invalid document"}`)
	defer srv.Close()

	model := "mistral-ocr-latest"
	_, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, []byte("%PDF-1.4"), nil,
		mistralOCRAPIConfig("sk-test", srv.URL), nil, nil)
	if err == nil {
		t.Fatal("OCRFile: want error, got nil")
	}
	if !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "invalid document") {
		t.Fatalf("error = %v, want status and body", err)
	}
}

func TestMistralOCRFileRequiresAPIKey(t *testing.T) {
	model := "mistral-ocr-latest"
	_, err := newMistralOCRDriverForTest().OCRFile(context.Background(), &model, []byte("%PDF-1.4"), nil,
		mistralOCRAPIConfig("  ", ""), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("error = %v, want api key is required", err)
	}
}

// mistral-ocr-latest is listed with the "ocr" type under the Mistral
// provider, so a tenant can add it to a Mistral instance from the model
// settings and pick it as a PDF layout recognizer.
func TestMistralProviderListsOCRModels(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "mistral.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	provider := GetProviderManager().FindProvider("Mistral")
	if provider == nil {
		t.Fatal("Mistral provider not found")
	}
	if provider.URLSuffix.OCR != "v1/ocr" || provider.URLSuffix.Files != "v1/files" {
		t.Fatalf("URLSuffix = %+v, want ocr=v1/ocr files=v1/files", provider.URLSuffix)
	}
	for _, name := range []string{"mistral-ocr-latest", "mistral-ocr-2512"} {
		model, err := GetProviderManager().GetModelByName("Mistral", name)
		if err != nil {
			t.Fatalf("GetModelByName(%q): %v", name, err)
		}
		if !model.ModelTypeMap["ocr"] {
			t.Fatalf("%s model types = %v, want ocr", name, model.ModelTypes)
		}
	}
}
