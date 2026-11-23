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
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"ragflow/internal/common"
)

const naceNDITestPDF = "%PDF-1.4\n%synthetic\n"

func withFastNaceNDI(t *testing.T) {
	t.Helper()
	withSSRFBypass(t)
	prevPoll, prevRetry, prevDelete := naceNDIPollInterval, naceNDIRetryDelay, naceNDIDeletePollDelay
	naceNDIPollInterval, naceNDIRetryDelay, naceNDIDeletePollDelay = time.Millisecond, time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		naceNDIPollInterval, naceNDIRetryDelay, naceNDIDeletePollDelay = prevPoll, prevRetry, prevDelete
	})
}

// naceNDIFake is an NDI documents API on one host: upload grants, uploads,
// one parse job and deletions. parse answers the create call, poll each job
// read; upload and deleteFile, when set, answer those calls instead of the
// default success.
type naceNDIFake struct {
	t          *testing.T
	server     *httptest.Server
	grant      func(w http.ResponseWriter) bool
	parse      func(w http.ResponseWriter, r *http.Request)
	poll       func(w http.ResponseWriter, r *http.Request, n int)
	upload     func(w http.ResponseWriter, n int) bool
	deleteFile func(w http.ResponseWriter, n int)

	mu           sync.Mutex
	calls        []string
	grantBody    map[string]any
	uploadPaths  []string
	uploadMeta   map[string]any
	uploadFile   []byte
	uploadHeader http.Header
	grantTokens  []string
	uploadTokens []string
	parseBody    map[string]any
	parseQueries []string
	idemKeys     []string
	pollQuery    []string
}

func newNaceNDIFake(t *testing.T) *naceNDIFake {
	f := &naceNDIFake{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *naceNDIFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	if r.URL.Path != "/upload/ws_1/files" {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			f.t.Errorf("%s %s: Authorization=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if !strings.HasPrefix(r.Header.Get(naceClientHeader), "ragflow/") {
			f.t.Errorf("%s %s: %s=%q", r.Method, r.URL.Path, naceClientHeader, r.Header.Get(naceClientHeader))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/documents/upload-grants":
		f.mu.Lock()
		f.grantBody = nil
		_ = json.NewDecoder(r.Body).Decode(&f.grantBody)
		f.mu.Unlock()
		if f.grant != nil && f.grant(w) {
			return
		}
		// Each grant carries its own token, which works for one upload.
		f.mu.Lock()
		token := fmt.Sprintf("upload-token-%d", len(f.grantTokens)+1)
		f.grantTokens = append(f.grantTokens, token)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"workspace_id":"ws_1","upload_url":"`+f.server.URL+`/upload/ws_1/files","token":"`+token+`","expires_at":"2026-01-01T00:00:00Z","max_bytes":null}`)
	case r.Method == http.MethodPost && r.URL.Path == "/upload/ws_1/files":
		f.serveUpload(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/documents/parse":
		f.mu.Lock()
		f.parseBody = nil
		_ = json.NewDecoder(r.Body).Decode(&f.parseBody)
		f.parseQueries = append(f.parseQueries, r.URL.RawQuery)
		f.idemKeys = append(f.idemKeys, r.Header.Get("Idempotency-Key"))
		f.mu.Unlock()
		f.parse(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/documents/jobs/job_1":
		f.mu.Lock()
		f.pollQuery = append(f.pollQuery, r.URL.RawQuery)
		n := len(f.pollQuery)
		f.mu.Unlock()
		f.poll(w, r, n)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/documents/jobs/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && r.URL.Path == "/v1/documents/workspaces/ws_1/files/file_1":
		if f.deleteFile != nil {
			f.deleteFile(w, f.called(r.Method+" "+r.URL.Path))
			return
		}
		_, _ = io.WriteString(w, `{"status":"deleted"}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *naceNDIFake) serveUpload(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	got := r.Header.Get("X-Upload-Token")
	want := ""
	if len(f.grantTokens) > 0 {
		want = f.grantTokens[len(f.grantTokens)-1]
	}
	if got != want || slices.Contains(f.uploadTokens, got) {
		f.t.Errorf("X-Upload-Token=%q, want the newest grant's unused token %q", got, want)
	}
	f.uploadTokens = append(f.uploadTokens, got)
	f.mu.Unlock()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		f.t.Errorf("ParseMultipartForm: %v", err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		f.t.Errorf("file part: %v", err)
		return
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	if header.Filename != "document.pdf" || header.Header.Get("Content-Type") != "application/pdf" {
		f.t.Errorf("file part=%q %q", header.Filename, header.Header.Get("Content-Type"))
	}
	f.mu.Lock()
	f.uploadFile = data
	f.uploadHeader = r.Header.Clone()
	f.uploadMeta = nil
	_ = json.Unmarshal([]byte(r.FormValue("metadata")), &f.uploadMeta)
	metaPath, _ := f.uploadMeta["path"].(string)
	f.uploadPaths = append(f.uploadPaths, metaPath)
	n := len(f.uploadPaths)
	f.mu.Unlock()
	if f.upload != nil && f.upload(w, n) {
		return
	}
	_, _ = io.WriteString(w, `{"job_id":"upload_1","kind":"file_upload","status":"succeeded","result":{"file":{"file_id":"file_1","workspace_id":"ws_1","path":"`+metaPath+`"},"deduplicated":false}}`)
}

func (f *naceNDIFake) called(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

func (f *naceNDIFake) driver() *NaceModel {
	return NewNaceModel(map[string]string{"default": f.server.URL + "/v1"}, URLSuffix{Decision: "systemone", Models: "models", OCR: "documents/parse"})
}

func naceNDIOCR(ctx context.Context, driver *NaceModel, model string, content []byte, url *string) (*OCRFileResponse, error) {
	key := "test-key"
	return driver.OCRFile(ctx, &model, content, url, &APIConfig{ApiKey: &key}, nil, nil)
}

// readNaceNDIFixture returns the live-recorded job, with the fake's job id.
func readNaceNDIFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/nace_ndi_parse.json")
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.Replace(string(data), "00000000-0000-0000-0000-000000000001", "job_1", 1))
}

func writeNaceNDIJob(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// naceNDIRoundTripper is an http.RoundTripper made from a function.
type naceNDIRoundTripper func(*http.Request) (*http.Response, error)

func (f naceNDIRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// withNaceNDIUploadTransport sends uploads through transport for one test.
func withNaceNDIUploadTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := naceNDIUploadTransport
	naceNDIUploadTransport = transport
	t.Cleanup(func() { naceNDIUploadTransport = previous })
}

// withNaceNDIAPITransport sends NDI API calls through transport for one test.
func withNaceNDIAPITransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := naceNDIAPITransport
	naceNDIAPITransport = transport
	t.Cleanup(func() { naceNDIAPITransport = previous })
}

// withNaceNDIHosts resolves names through hosts (an IP literal resolves to
// itself) for the upload host check, and turns ALLOW_ANY_HOST off, so the
// strict address check applies.
func withNaceNDIHosts(t *testing.T, hosts map[string]string) {
	t.Helper()
	t.Setenv(common.EnvAllowAnyHost, "")
	previousCheck := common.AssertURLSafeWithContext
	common.AssertURLSafeWithContext = func(_ context.Context, rawURL string) (string, string, error) {
		return common.AssertURLSafe(rawURL)
	}
	t.Cleanup(func() { common.AssertURLSafeWithContext = previousCheck })
	previous := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		if net.ParseIP(host) != nil {
			return []string{host}, nil
		}
		if addr, ok := hosts[host]; ok {
			return []string{addr}, nil
		}
		return nil, fmt.Errorf("no such host %s", host)
	}
	t.Cleanup(func() { common.LookupHost = previous })
}

// The shipped catalog keeps the decision models and adds one NDI OCR model per
// parse effort, with the parse endpoint as the OCR suffix.
func TestNaceNDIProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "nace.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	provider := GetProviderManager().FindProvider("Nace")
	if provider == nil {
		t.Fatal("Nace provider not found")
	}
	if provider.URLSuffix.OCR != "documents/parse" {
		t.Errorf("url_suffix.ocr=%q", provider.URLSuffix.OCR)
	}
	var ocr []string
	for _, model := range provider.Models {
		if model.ModelTypeMap["ocr"] {
			ocr = append(ocr, model.Name)
			if _, err := naceNDIParseMode(model.Name); err != nil {
				t.Errorf("%s: %v", model.Name, err)
			}
		}
	}
	if !reflect.DeepEqual(ocr, []string{"ndi-parse-high", "ndi-parse-low", "ndi-parse-medium"}) {
		t.Fatalf("ocr models=%v", ocr)
	}
}

// OCRFile uploads the PDF with the grant's token only, creates the parse job
// without waiting, maps the live-recorded blocks onto pages with normalized
// boxes, and then deletes the parse job and the upload.
func TestNaceOCRFileParsesUploadedPDF(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, http.StatusOK, string(fixture)) }

	resp, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-medium", []byte(naceNDITestPDF), nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}

	path, _ := fake.grantBody["path"].(string)
	if !strings.HasPrefix(path, "ragflow/") || !strings.HasSuffix(path, ".pdf") || fake.grantBody["ttl_seconds"] != float64(600) || fake.grantBody["total_size_bytes"] != float64(len(naceNDITestPDF)) {
		t.Errorf("grant body=%v", fake.grantBody)
	}
	if fake.uploadMeta["path"] != path || fake.uploadMeta["total_size_bytes"] != float64(len(naceNDITestPDF)) || string(fake.uploadFile) != naceNDITestPDF {
		t.Errorf("upload meta=%v file=%q", fake.uploadMeta, fake.uploadFile)
	}
	if fake.uploadHeader.Get("Authorization") != "" || fake.uploadHeader.Get(naceClientHeader) != "" {
		t.Errorf("upload headers=%v", fake.uploadHeader)
	}
	wantBody := map[string]any{
		"source":     map[string]any{"type": "workspace_file", "workspace_id": "ws_1", "file_id": "file_1"},
		"parse_mode": "medium",
		"output":     map[string]any{"formats": []any{"markdown", "blocks"}, "table_format": "markdown", "include_page_markers": false},
		"figures":    map[string]any{"mode": "include"},
	}
	if !reflect.DeepEqual(fake.parseBody, wantBody) {
		t.Errorf("parse body=%v", fake.parseBody)
	}
	if !reflect.DeepEqual(fake.parseQueries, []string{"wait_seconds=0"}) || len(fake.idemKeys) != 1 || !strings.HasPrefix(fake.idemKeys[0], "ragflow-") {
		t.Errorf("parse queries=%v idempotency keys=%v", fake.parseQueries, fake.idemKeys)
	}
	if fake.called("DELETE /v1/documents/jobs/job_1") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Errorf("calls=%v", fake.calls)
	}

	if resp.Text == nil || !strings.HasPrefix(*resp.Text, "Quarterly Widget Report\n\nThis synthetic report") || len(resp.Warnings) != 0 {
		t.Fatalf("text=%v warnings=%v", resp.Text, resp.Warnings)
	}
	if len(resp.Pages) != 2 {
		t.Fatalf("pages=%+v", resp.Pages)
	}
	var labels [][]string
	for i, page := range resp.Pages {
		if page.Index != i || !page.Normalized {
			t.Errorf("page %d=%+v", i, page)
		}
		var pageLabels []string
		for _, block := range page.Blocks {
			pageLabels = append(pageLabels, block.Label)
		}
		labels = append(labels, pageLabels)
	}
	want := [][]string{{"paragraph_title", "text", "paragraph_title", "table"}, {"paragraph_title", "figure", "figure_title", "text"}}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels=%v", labels)
	}
	table := resp.Pages[0].Blocks[3]
	if !strings.HasPrefix(table.Text, "| Region | Units | Revenue |\n| --- |") {
		t.Errorf("table text=%q", table.Text)
	}
	wantBox := []float64{0.12908496732026145, 0.24242424242424243, 0.7687908496732027, 0.3440656565656566}
	if !reflect.DeepEqual(table.BBox, wantBox) {
		t.Errorf("table box=%v", table.BBox)
	}
}

// A job that is still running after the create call is read with held reads
// until it ends.
func TestNaceOCRFileWaitsThenPolls(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		writeNaceNDIJob(w, http.StatusAccepted, `{"job_id":"job_1","status":"queued","result":null}`)
	}
	fake.poll = func(w http.ResponseWriter, r *http.Request, n int) {
		if n < 3 {
			writeNaceNDIJob(w, http.StatusOK, `{"job_id":"job_1","status":"running","result":null}`)
			return
		}
		writeNaceNDIJob(w, http.StatusOK, string(fixture))
	}

	resp, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if len(resp.Pages) != 2 || fake.parseBody["parse_mode"] != "low" {
		t.Fatalf("pages=%d parse_mode=%v", len(resp.Pages), fake.parseBody["parse_mode"])
	}
	if !reflect.DeepEqual(fake.pollQuery, []string{"wait_seconds=25", "wait_seconds=25", "wait_seconds=25"}) {
		t.Errorf("poll queries=%v", fake.pollQuery)
	}
	if fake.called("DELETE /v1/documents/jobs/job_1") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Errorf("calls=%v", fake.calls)
	}
}

// When the caller gives up, the running job is deleted, which cancels it on
// the server (a cancelled job is not billed), and the upload is still deleted.
func TestNaceOCRFileCancelsJobWhenContextEnds(t *testing.T) {
	withFastNaceNDI(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		writeNaceNDIJob(w, http.StatusAccepted, `{"job_id":"job_1","status":"running"}`)
	}
	ctx, cancel := context.WithCancel(t.Context())
	fake.poll = func(w http.ResponseWriter, r *http.Request, n int) {
		cancel()
		<-r.Context().Done()
	}

	_, err := naceNDIOCR(ctx, fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if fake.called("DELETE /v1/documents/jobs/job_1") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// A create whose answer is lost when the caller gives up is not sent again:
// the driver has no job id, so it leaves the job to NDI (a documented limit),
// and the upload is still deleted.
func TestNaceOCRFileDoesNotRepeatCreateAfterCancel(t *testing.T) {
	withFastNaceNDI(t)
	fake := newNaceNDIFake(t)
	ctx, cancel := context.WithCancel(t.Context())
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}
	_, err := naceNDIOCR(ctx, fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if fake.called("POST /v1/documents/parse") != 1 || fake.called("DELETE /v1/documents/jobs/job_1") != 0 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// A create answer cut short or not decodable may still name the job it
// started: that job is deleted, with the id read from the part that arrived.
// The create is not sent again; an answer that names no job deletes nothing.
func TestNaceOCRFileRemovesJobOfBadCreateAnswer(t *testing.T) {
	for _, tc := range []struct {
		name        string
		answer      func(w http.ResponseWriter)
		wantErr     string
		wantDeletes int
	}{
		{"cut short", func(w http.ResponseWriter) {
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"job_id":"job_1","sta`)
		}, "unexpected EOF", 1},
		{"mistyped", func(w http.ResponseWriter) {
			writeNaceNDIJob(w, http.StatusAccepted, `{"job_id":"job_1","status":7}`)
		}, "failed to parse response", 1},
		{"malformed", func(w http.ResponseWriter) {
			writeNaceNDIJob(w, http.StatusAccepted, `{"job_id":"job_1","status":"queued",}`)
		}, "failed to parse response", 1},
		{"malformed without an id", func(w http.ResponseWriter) {
			writeNaceNDIJob(w, http.StatusAccepted, `{"status":"queued",}`)
		}, "failed to parse response", 0},
		{"cut short before the id", func(w http.ResponseWriter) {
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"job_`)
		}, "unexpected EOF", 0},
		{"no job id", func(w http.ResponseWriter) {
			writeNaceNDIJob(w, http.StatusAccepted, `{"status":"queued"}`)
		}, "response carries no job id", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFastNaceNDI(t)
			fake := newNaceNDIFake(t)
			fake.parse = func(w http.ResponseWriter, r *http.Request) { tc.answer(w) }
			fake.poll = func(w http.ResponseWriter, r *http.Request, n int) { t.Error("job read") }
			_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err=%v, want %q", err, tc.wantErr)
			}
			if fake.called("POST /v1/documents/parse") != 1 || fake.called("DELETE /v1/documents/jobs/job_1") != tc.wantDeletes || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
				t.Fatalf("calls=%v", fake.calls)
			}
		})
	}
}

// A redirect answer from the API is not followed to any place: the call fails
// with the redirect status, and the other origin receives nothing.
func TestNaceOCRFileFollowsNoAPIRedirect(t *testing.T) {
	withFastNaceNDI(t)
	elsewhere := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere++ }))
	defer other.Close()
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/documents/parse", http.StatusTemporaryRedirect)
	}
	_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if err == nil || err.Error() != "nace: parse: status 307" || elsewhere != 0 {
		t.Fatalf("err=%v elsewhere=%d", err, elsewhere)
	}
	if fake.called("POST /v1/documents/parse") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// A receipt for another workspace or another path is refused, and its ids are
// used for no parse and no delete.
func TestNaceOCRFileRefusesForeignUploadReceipt(t *testing.T) {
	for name, receipt := range map[string]string{
		"workspace":  `{"job_id":"upload_1","status":"succeeded","result":{"file":{"file_id":"file_9","workspace_id":"ws_9"}}}`,
		"path":       `{"job_id":"upload_1","status":"succeeded","result":{"file":{"file_id":"file_9","workspace_id":"ws_1","path":"other/report.pdf"}}}`,
		"empty path": `{"job_id":"upload_1","status":"succeeded","result":{"file":{"file_id":"file_9","workspace_id":"ws_1","path":""}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			withFastNaceNDI(t)
			fake := newNaceNDIFake(t)
			fake.parse = func(w http.ResponseWriter, r *http.Request) { t.Error("parse called") }
			fake.upload = func(w http.ResponseWriter, n int) bool {
				_, _ = io.WriteString(w, receipt)
				return true
			}
			_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
			if err == nil || !strings.Contains(err.Error(), "upload result names a file other than the one uploaded") {
				t.Fatalf("err=%v", err)
			}
			for _, call := range fake.calls {
				if strings.HasPrefix(call, "DELETE ") {
					t.Fatalf("calls=%v", fake.calls)
				}
			}
			if len(fake.uploadPaths) != 1 {
				t.Fatalf("upload paths=%v", fake.uploadPaths)
			}
		})
	}
}

// An upload whose context ended before the attempt sent nothing, so the file
// is not sent again to learn its id.
func TestNaceOCRFileDoesNotResendUnsentUpload(t *testing.T) {
	withFastNaceNDI(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { t.Error("parse called") }
	ctx, cancel := context.WithCancel(t.Context())
	// The caller gives up while the grant answer is on its way.
	withNaceNDIAPITransport(t, naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		resp, err := naceNDITransport(nil).RoundTrip(req)
		if strings.HasSuffix(req.URL.Path, "/upload-grants") {
			cancel()
		}
		return resp, err
	}))
	withNaceNDIUploadTransport(t, naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		t.Error("upload sent")
		return nil, errors.New("unexpected upload")
	}))
	_, err := naceNDIOCR(ctx, fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if fake.called("POST /v1/documents/upload-grants") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 0 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// The upload host check honours the caller's context, so a stalled lookup
// does not hold the parse after the caller gives up.
func TestNaceOCRFileUploadHostCheckHonoursContext(t *testing.T) {
	withFastNaceNDI(t)
	previous := common.AssertURLSafeWithContext
	entered, exited := make(chan struct{}), make(chan error, 1)
	common.AssertURLSafeWithContext = func(ctx context.Context, rawURL string) (string, string, error) {
		close(entered)
		<-ctx.Done()
		exited <- ctx.Err()
		return "", "", ctx.Err()
	}
	t.Cleanup(func() { common.AssertURLSafeWithContext = previous })
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"workspace_id":"ws_1","upload_url":"https://uploads.example.com/files","token":"t"}`)
	}))
	defer api.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	driver := NewNaceModel(map[string]string{"default": api.URL + "/v1"}, URLSuffix{})
	done := make(chan error, 1)
	go func() {
		_, err := naceNDIOCR(ctx, driver, "ndi-parse-low", []byte(naceNDITestPDF), nil)
		done <- err
	}()
	// The run is cancelled only once the host check has started, so the check
	// must end because of that cancellation.
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the host check did not start")
	}
	cancel()
	select {
	case err := <-exited:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("host check ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the host check did not end on cancellation")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

// Provider text loses URL queries (relative ones too) and credentials; a
// transport error is clipped the same way and keeps its cause.
func TestNaceNDIClipMasksSecrets(t *testing.T) {
	for in, want := range map[string]string{
		"cannot read /v1/files/f.pdf?X-Signature=abc&exp=1 now":   "cannot read /v1/files/f.pdf now",
		"header Authorization: Bearer abc.def-ghi was refused":    "header Authorization: Bearer [redacted] was refused",
		"key nace_sk_live_0123456789abcdef is revoked":            "key [redacted] is revoked",
		`bad X-Upload-Token: tok_123 and "token":"tok_456" given`: `bad X-Upload-Token: [redacted] and "token":"[redacted]" given`,
		"status 401: invalid_api_key: Invalid API key.":           "status 401: invalid_api_key: Invalid API key.",
		"see access_token=abc123&upload_token=def":                "see access_token=[redacted]&upload_token=[redacted]",
		`{"api_key": "k-1", "X-UPLOAD-TOKEN":"t-2"}`:              `{"api_key": "[redacted]", "X-UPLOAD-TOKEN":"[redacted]"}`,
		"x-upload-token: t-3; client_secret=s-4":                  "x-upload-token: [redacted]; client_secret=[redacted]",
		"Is the PDF damaged? It has no pages":                     "Is the PDF damaged? It has no pages",
	} {
		if got := naceNDIClip(in); got != want {
			t.Errorf("naceNDIClip(%q)=%q, want %q", in, got, want)
		}
	}
	inner := fmt.Errorf("bad redirect to %q: %w", "/next?sig=secret-sig", context.Canceled)
	err := naceNDIRedactError(&neturl.Error{Op: "Post", URL: "https://uploads.example.com/f?sig=secret-sig", Err: inner})
	if strings.Contains(err.Error(), "secret-sig") || !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

// A failure while reading an error answer keeps its cause.
func TestNaceNDIDoKeepsReadFailureCause(t *testing.T) {
	client := &http.Client{Transport: naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(iotest.ErrReader(context.Canceled)), Header: http.Header{}}, nil
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1/documents/jobs/job_1", nil)
	err := naceNDIDo(client, req, nil)
	var apiErr *naceNDIError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway {
		t.Fatalf("err=%v", err)
	}
	client.Transport = naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(iotest.ErrReader(context.DeadlineExceeded)), Header: http.Header{}}, nil
	})
	if err := naceNDIDo(client, req, &struct{}{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

// An account with zero retention refuses a standalone upload grant (422
// invalid_request), so an uploaded PDF gets one clear error, with no retry and
// no parse. A public PDF link needs no upload: its create without a wait is
// refused, sent again held, and the held answer carries the result; a job that
// outlasts the hold is deleted (cancelled), because its result is not kept.
func TestNaceOCRFileZeroRetention(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.grant = func(w http.ResponseWriter) bool {
		writeNaceNDIJob(w, http.StatusUnprocessableEntity, `{"error":{"code":"invalid_request","message":"zero retention requires a workspace bound to your own storage","retryable":false}}`)
		return true
	}
	fake.parse = func(w http.ResponseWriter, r *http.Request) { t.Error("parse called") }
	_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if err == nil || !strings.HasPrefix(err.Error(), "nace: upload: upload grant refused; an account with zero retention cannot upload files for parsing") || !strings.Contains(err.Error(), "status 422: invalid_request") {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"POST /v1/documents/upload-grants"}) {
		t.Fatalf("calls=%v", fake.calls)
	}

	fake = newNaceNDIFake(t)
	link := "https://example.com/files/report.pdf"
	refusal := `{"error":{"code":"wait_required_for_zero_retention","message":"wait_seconds is required","retryable":false}}`
	finished := true
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("wait_seconds") == "0":
			writeNaceNDIJob(w, http.StatusUnprocessableEntity, refusal)
		case finished:
			writeNaceNDIJob(w, http.StatusOK, string(fixture))
		default:
			writeNaceNDIJob(w, http.StatusAccepted, `{"job_id":"job_1","status":"running"}`)
		}
	}
	if _, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", nil, &link); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if fake.called("POST /v1/documents/upload-grants") != 0 || !reflect.DeepEqual(fake.parseQueries, []string{"wait_seconds=0", "wait_seconds=60"}) || fake.idemKeys[0] == fake.idemKeys[1] {
		t.Fatalf("parse queries=%v keys=%v", fake.parseQueries, fake.idemKeys)
	}
	if fake.called("DELETE /v1/documents/jobs/job_1") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}

	finished = false
	_, err = naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", nil, &link)
	if err == nil || !strings.Contains(err.Error(), "zero retention") {
		t.Fatalf("err=%v", err)
	}
	if fake.called("DELETE /v1/documents/jobs/job_1") != 2 || fake.called("GET /v1/documents/jobs/job_1") != 0 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// A failed job reports the provider's code and message, never its request id,
// and the job and the upload are still deleted.
func TestNaceOCRFileReportsJobFailure(t *testing.T) {
	withFastNaceNDI(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		writeNaceNDIJob(w, http.StatusOK, `{"job_id":"job_1","status":"failed","error":{"code":"unreadable_document","message":"The PDF is damaged","retryable":false,"request_id":"req_1"}}`)
	}
	_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-high", []byte(naceNDITestPDF), nil)
	if err == nil || !strings.HasSuffix(err.Error(), "job job_1 failed: unreadable_document: The PDF is damaged") {
		t.Fatalf("err=%v", err)
	}
	if fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 || fake.called("DELETE /v1/documents/jobs/job_1") != 1 || fake.called("POST /v1/documents/parse") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}

	// A succeeded job whose result was not retained has nothing to map.
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		writeNaceNDIJob(w, http.StatusOK, `{"job_id":"job_1","status":"succeeded","result":null,"result_state":"not_retained"}`)
	}
	if _, err = naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-high", []byte(naceNDITestPDF), nil); err == nil || !strings.Contains(err.Error(), "parse job job_1 returned no result") {
		t.Fatalf("err=%v", err)
	}
}

// NDI error bodies become "status: code: message", with the message shortened,
// URLs in it cut before their query, and the request id left out; any other
// body is not quoted. Only transient failures are retried, and the
// retried create keeps its idempotency key so it cannot start a second job.
func TestNaceOCRFileErrorMapping(t *testing.T) {
	long := strings.Repeat("word ", 100)
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantErr   string
		wantCalls int
	}{
		{"bad key", 401, `{"error":{"type":"authentication_error","code":"invalid_api_key","message":"Invalid API key.","retryable":false},"request_id":"req_2"}`, "nace: parse: status 401: invalid_api_key: Invalid API key.", 1},
		{"bad source", 422, `{"error":{"code":"invalid_source","message":"no such file","retryable":false},"request_id":"req_3"}`, "nace: parse: status 422: invalid_source: no such file", 1},
		{"long message", 422, `{"error":{"code":"invalid_request","message":"` + long + `","retryable":false},"request_id":"req_4"}`, "nace: parse: status 422: invalid_request: " + strings.TrimSpace(long)[:naceNDIMaxErrorText] + "...", 1},
		{"retryable flag", 409, `{"error":{"code":"busy","message":"try again","retryable":true}}`, "nace: parse: status 409: busy: try again", naceNDIMaxAttempts},
		{"signed url in message", 422, `{"error":{"code":"invalid_source","message":"cannot read https://store.example.com/f.pdf?X-Signature=secret-sig#part","retryable":false}}`, "nace: parse: status 422: invalid_source: cannot read https://store.example.com/f.pdf", 1},
		{"server error", 503, `<html>` + strings.Repeat("x", 400) + ` https://proxy.example.com/?token=secret-token</html>`, "nace: parse: status 503", naceNDIMaxAttempts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFastNaceNDI(t)
			fake := newNaceNDIFake(t)
			fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, tc.status, tc.body) }
			_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err=%v\nwant %s", err, tc.wantErr)
			}
			if got := fake.called("POST /v1/documents/parse"); got != tc.wantCalls {
				t.Fatalf("parse calls=%d, want %d", got, tc.wantCalls)
			}
			for _, key := range fake.idemKeys {
				if key != fake.idemKeys[0] {
					t.Fatalf("idempotency keys=%v", fake.idemKeys)
				}
			}
			if fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
				t.Fatalf("calls=%v", fake.calls)
			}
		})
	}
}

func TestNaceOCRFileRetriesTransientFailure(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	attempts := 0
	fake.parse = func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			writeNaceNDIJob(w, http.StatusTooManyRequests, `{"error":{"code":"rate_limited","message":"slow down","retryable":true}}`)
			return
		}
		writeNaceNDIJob(w, http.StatusOK, string(fixture))
	}
	if _, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if len(fake.idemKeys) != 2 || fake.idemKeys[0] != fake.idemKeys[1] {
		t.Fatalf("idempotency keys=%v", fake.idemKeys)
	}
}

// A grant works once, so a failed upload is sent again with a new grant, to
// the same path: NDI then answers with a file an earlier attempt stored.
func TestNaceOCRFileRetriesUploadWithNewGrant(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, http.StatusOK, string(fixture)) }
	fake.upload = func(w http.ResponseWriter, n int) bool {
		if n == 1 {
			writeNaceNDIJob(w, http.StatusServiceUnavailable, `{"error":{"code":"unavailable","message":"try later","retryable":true}}`)
			return true
		}
		return false
	}
	if _, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if fake.called("POST /v1/documents/upload-grants") != 2 || len(fake.uploadPaths) != 2 || fake.uploadPaths[0] != fake.uploadPaths[1] {
		t.Fatalf("grants=%d upload paths=%v", fake.called("POST /v1/documents/upload-grants"), fake.uploadPaths)
	}
	if !reflect.DeepEqual(fake.uploadTokens, []string{"upload-token-1", "upload-token-2"}) {
		t.Fatalf("upload tokens=%v", fake.uploadTokens)
	}
}

// When every upload attempt fails in a way that may still have stored the
// file, the file is sent once more to learn its id, and deleted. A failed
// deletion is sent again once.
func TestNaceOCRFileRemovesUploadOfUnknownOutcome(t *testing.T) {
	withFastNaceNDI(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { t.Error("parse called") }
	fake.upload = func(w http.ResponseWriter, n int) bool {
		if n <= naceNDIMaxAttempts {
			writeNaceNDIJob(w, http.StatusBadGateway, `upstream reset`)
			return true
		}
		return false
	}
	fake.deleteFile = func(w http.ResponseWriter, n int) {
		if n == 1 {
			writeNaceNDIJob(w, http.StatusServiceUnavailable, `{"error":{"code":"unavailable","message":"later","retryable":true}}`)
			return
		}
		_, _ = io.WriteString(w, `{"status":"deleted"}`)
	}
	_, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if err == nil || err.Error() != "nace: upload: status 502" {
		t.Fatalf("err=%v", err)
	}
	if len(fake.uploadPaths) != naceNDIMaxAttempts+1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 2 {
		t.Fatalf("upload paths=%v calls=%v", fake.uploadPaths, fake.calls)
	}

	// A refused upload stored nothing, so nothing is sent again.
	refused := newNaceNDIFake(t)
	refused.upload = func(w http.ResponseWriter, n int) bool {
		writeNaceNDIJob(w, http.StatusRequestEntityTooLarge, `{"error":{"code":"file_too_large","message":"too large","retryable":false}}`)
		return true
	}
	if _, err := naceNDIOCR(t.Context(), refused.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil); err == nil || !strings.Contains(err.Error(), "file_too_large") {
		t.Fatalf("err=%v", err)
	}
	if len(refused.uploadPaths) != 1 || refused.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 0 {
		t.Fatalf("upload paths=%v calls=%v", refused.uploadPaths, refused.calls)
	}
}

// Input that is not a PDF, an unknown model and a missing key are refused
// before any request is sent.
func TestNaceOCRFileRefusesBeforeAnyRequest(t *testing.T) {
	withFastNaceNDI(t)
	fake := newNaceNDIFake(t)
	key := "test-key"
	for _, tc := range []struct {
		name    string
		model   string
		content []byte
		url     *string
		key     *string
		wantErr string
	}{
		{"png", "ndi-parse-low", []byte("\x89PNG\r\n\x1a\n"), nil, &key, "PDF files only"},
		{"header past the first kilobyte", "ndi-parse-low", []byte(strings.Repeat(" ", naceNDIPDFHeaderWindow) + naceNDITestPDF), nil, &key, "PDF files only"},
		{"decision model", "drex-max", []byte(naceNDITestPDF), nil, &key, "not an NDI OCR model"},
		{"unknown effort", "ndi-parse-max", []byte(naceNDITestPDF), nil, &key, "not an NDI OCR model"},
		{"no key", "ndi-parse-low", []byte(naceNDITestPDF), nil, nil, "api key is required"},
		{"no input", "ndi-parse-low", nil, nil, &key, "content or url is required"},
		{"empty file", "ndi-parse-low", []byte{}, nil, &key, "nace: the file is empty (0 bytes)"},
		{"empty file, blank link", "ndi-parse-low", []byte{}, ptrTo(" "), &key, "nace: the file is empty (0 bytes)"},
		{"http link", "ndi-parse-low", nil, ptrTo("http://example.com/a.pdf"), &key, "https only"},
		{"link without pdf name", "ndi-parse-low", nil, ptrTo("https://example.com/report"), &key, "must name a .pdf file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := tc.model
			_, err := fake.driver().OCRFile(t.Context(), &model, tc.content, tc.url, &APIConfig{ApiKey: tc.key}, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err=%v, want %q", err, tc.wantErr)
			}
		})
	}
	if len(fake.calls) != 0 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

func ptrTo(s string) *string { return &s }

// A PDF header may follow other bytes within the first kilobyte.
func TestNaceNDIIsPDF(t *testing.T) {
	for content, want := range map[string]bool{
		naceNDITestPDF:         true,
		"\xef\xbb\xbf%PDF-1.7": true,
		strings.Repeat("\n", naceNDIPDFHeaderWindow-5) + "%PDF-1.4": true,
		strings.Repeat("\n", naceNDIPDFHeaderWindow-4) + "%PDF-1.4": false,
		"%PDF": false,
	} {
		if got := naceNDIIsPDF([]byte(content)); got != want {
			t.Errorf("%q: got %v", content[:min(len(content), 20)], got)
		}
	}
}

// A public https PDF link is parsed in place: nothing is uploaded, and only
// the parse job is deleted.
func TestNaceOCRFileParsesPublicLink(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, http.StatusOK, string(fixture)) }
	link := "https://example.com/files/report.pdf"
	if _, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", nil, &link); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	want := map[string]any{"type": "url", "url": link, "file_name": "report.pdf"}
	if !reflect.DeepEqual(fake.parseBody["source"], want) {
		t.Fatalf("source=%v", fake.parseBody["source"])
	}
	if !reflect.DeepEqual(fake.calls, []string{"POST /v1/documents/parse", "DELETE /v1/documents/jobs/job_1"}) {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// The configured pages go to NDI as page_ranges; blocks outside them are not
// kept.
func TestNaceOCRFileSendsPageRanges(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, http.StatusOK, string(fixture)) }
	model, key := "ndi-parse-low", "test-key"
	resp, err := fake.driver().OCRFile(t.Context(), &model, []byte(naceNDITestPDF), nil, &APIConfig{ApiKey: &key}, &OCRConfig{Pages: [][]int{{2, 2}}}, nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if want := []any{map[string]any{"start": float64(2), "end": float64(2)}}; !reflect.DeepEqual(fake.parseBody["page_ranges"], want) {
		t.Fatalf("page_ranges=%v", fake.parseBody["page_ranges"])
	}
	if len(resp.Pages) != 2 || len(resp.Pages[0].Blocks) != 0 || len(resp.Pages[1].Blocks) != 4 || strings.Contains(*resp.Text, "Quarterly Widget Report") {
		t.Fatalf("pages=%+v text=%q", resp.Pages, *resp.Text)
	}
}

func TestNaceNDIPageSelection(t *testing.T) {
	many := make([][]int, 0, naceNDIMaxRanges+1)
	for i := 0; i <= naceNDIMaxRanges; i++ {
		many = append(many, []int{2*i + 1, 2*i + 1})
	}
	whole := "the page selection is larger than one parse accepts (200 ranges, 500 pages)"
	for _, tc := range []struct {
		name  string
		pages [][]int
		want  naceNDISelection
	}{
		{"none", nil, naceNDISelection{}},
		{"one range", [][]int{{1, 2}}, naceNDISelection{Send: [][]int{{1, 2}}, Keep: [][]int{{1, 2}}}},
		{"merged", [][]int{{4, 6}, {1, 2}, {3, 3}, {9, 9}}, naceNDISelection{Send: [][]int{{1, 6}, {9, 9}}, Keep: [][]int{{1, 6}, {9, 9}}}},
		{"invalid ranges", [][]int{{0, 3}, {4, 2}, {7}}, naceNDISelection{}},
		{"500 pages", [][]int{{1, 500}}, naceNDISelection{Send: [][]int{{1, 500}}, Keep: [][]int{{1, 500}}}},
		{"default whole document", [][]int{{1, 100000000}}, naceNDISelection{Keep: [][]int{{1, 100000000}}, Whole: whole}},
		{"too many ranges", many, naceNDISelection{Keep: many, Whole: whole}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := naceNDIPageSelection(&OCRConfig{Pages: tc.pages}); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

// NDI fails a job whose page_ranges run far past the last page with
// ocr_failed "No renderable pages" (at no charge). The whole document is then
// parsed once on the same upload and filtered to the selection, and both jobs
// and the upload are deleted. Any other failure (another code, or ocr_failed
// with another message), and a failing whole-document parse, is returned as
// it is.
func TestNaceOCRFileFallsBackOnOCRFailed(t *testing.T) {
	fixture := readNaceNDIFixture(t)
	const noPages = "No renderable pages in document.pdf."
	failed := func(jobID, code, message string) string {
		return `{"job_id":"` + jobID + `","status":"failed","error":{"code":"` + code + `","message":"` + message + `"}}`
	}
	run := func(t *testing.T, reply func(n int, ranges any) string) (*naceNDIFake, *OCRFileResponse, []any, error) {
		withFastNaceNDI(t)
		fake := newNaceNDIFake(t)
		var sent []any
		fake.parse = func(w http.ResponseWriter, r *http.Request) {
			ranges, ok := fake.parseBody["page_ranges"]
			if !ok {
				ranges = nil
			}
			sent = append(sent, ranges)
			writeNaceNDIJob(w, http.StatusOK, reply(len(sent), ranges))
		}
		model, key := "ndi-parse-low", "test-key"
		resp, err := fake.driver().OCRFile(t.Context(), &model, []byte(naceNDITestPDF), nil, &APIConfig{ApiKey: &key}, &OCRConfig{Pages: [][]int{{2, 300}}}, nil)
		return fake, resp, sent, err
	}
	pageRanges := []any{map[string]any{"start": float64(2), "end": float64(300)}}

	fake, resp, sent, err := run(t, func(n int, ranges any) string {
		if n == 1 {
			return failed("job_ranges", "ocr_failed", noPages)
		}
		return string(fixture)
	})
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if !reflect.DeepEqual(sent, []any{pageRanges, nil}) {
		t.Fatalf("page_ranges sent=%v", sent)
	}
	if len(resp.Pages) != 2 || len(resp.Pages[0].Blocks) != 0 || len(resp.Pages[1].Blocks) != 4 || strings.Contains(*resp.Text, "Quarterly Widget Report") {
		t.Fatalf("pages=%+v text=%q", resp.Pages, *resp.Text)
	}
	if len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "NDI could not parse the selected pages (ocr_failed), so the whole document was parsed and billed") {
		t.Fatalf("warnings=%v", resp.Warnings)
	}
	if fake.called("DELETE /v1/documents/jobs/job_ranges") != 1 || fake.called("DELETE /v1/documents/jobs/job_1") != 1 || fake.called("POST /v1/documents/upload-grants") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}

	// A selection wholly outside the document fails with invalid_request,
	// and ocr_failed may have other causes: either is returned, with no
	// second parse.
	for code, message := range map[string]string{"invalid_request": "Pages 2-3 are outside the document, which has 1 page.", "ocr_failed": "OCR engine error."} {
		fake, _, sent, err = run(t, func(n int, ranges any) string { return failed("job_ranges", code, message) })
		if err == nil || !strings.Contains(err.Error(), "job job_ranges failed: "+code+": "+message) || len(sent) != 1 {
			t.Fatalf("%s: err=%v sent=%v", code, err, sent)
		}
		if fake.called("DELETE /v1/documents/jobs/job_ranges") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
			t.Fatalf("%s: calls=%v", code, fake.calls)
		}
	}

	// The whole-document parse fails as well: its error is returned, and
	// both jobs and the upload are deleted.
	fake, _, sent, err = run(t, func(n int, ranges any) string {
		if n == 1 {
			return failed("job_ranges", "ocr_failed", noPages)
		}
		return failed("job_1", "ocr_failed", noPages)
	})
	if err == nil || !strings.Contains(err.Error(), "job job_1 failed: ocr_failed") || len(sent) != 2 || sent[1] != nil {
		t.Fatalf("err=%v sent=%v", err, sent)
	}
	if fake.called("DELETE /v1/documents/jobs/job_ranges") != 1 || fake.called("DELETE /v1/documents/jobs/job_1") != 1 || fake.called("DELETE /v1/documents/workspaces/ws_1/files/file_1") != 1 {
		t.Fatalf("calls=%v", fake.calls)
	}
}

// An upload refused with 429 waits the Retry-After it gives before the next
// attempt.
func TestNaceOCRFileUploadHonoursRetryAfter(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, http.StatusOK, string(fixture)) }
	var first time.Time
	var waited time.Duration
	fake.upload = func(w http.ResponseWriter, n int) bool {
		if n == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "1")
			writeNaceNDIJob(w, http.StatusTooManyRequests, `{"error":{"code":"rate_limited","message":"slow down"}}`)
			return true
		}
		waited = time.Since(first)
		return false
	}
	if _, err := naceNDIOCR(t.Context(), fake.driver(), "ndi-parse-low", []byte(naceNDITestPDF), nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if len(fake.uploadPaths) != 2 || waited < time.Second {
		t.Fatalf("uploads=%d waited=%v", len(fake.uploadPaths), waited)
	}
}

// The grant must not point the upload at a plain-http host other than the API.
func TestNaceOCRFileRefusesPlainHTTPUploadURL(t *testing.T) {
	withFastNaceNDI(t)
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		_, _ = io.WriteString(w, `{"workspace_id":"ws_1","upload_url":"http://upload.example.invalid/v1/workspaces/ws_1/files","token":"t"}`)
	}))
	defer server.Close()
	driver := NewNaceModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
	_, err := naceNDIOCR(t.Context(), driver, "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if err == nil || !strings.Contains(err.Error(), "refusing upload_url over http") {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(calls, []string{"POST /v1/documents/upload-grants"}) {
		t.Fatalf("calls=%v", calls)
	}
}

// An upload URL on another host must resolve to public addresses only, as for
// any cloud provider; nothing is sent to a private or internal address.
func TestNaceOCRFileRefusesPrivateUploadHost(t *testing.T) {
	withFastNaceNDI(t)
	withNaceNDIHosts(t, map[string]string{"uploads.internal.test": "10.0.0.7", "localhost": "127.0.0.1"})
	withNaceNDIUploadTransport(t, naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		t.Errorf("upload sent to %s", req.URL.Host)
		return nil, errors.New("unexpected upload")
	}))
	// The API's own hostname on another scheme or port is another origin, so
	// it is checked too.
	for _, uploadURL := range []string{"https://uploads.internal.test/files", "https://169.254.169.254/files", "https://127.0.0.1:9/files", "https://localhost:9/files"} {
		t.Run(uploadURL, func(t *testing.T) {
			grants := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				grants++
				_, _ = io.WriteString(w, `{"workspace_id":"ws_1","upload_url":"`+uploadURL+`","token":"t"}`)
			}))
			defer server.Close()
			driver := NewNaceModel(map[string]string{"default": strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/v1"}, URLSuffix{})
			_, err := naceNDIOCR(t.Context(), driver, "ndi-parse-low", []byte(naceNDITestPDF), nil)
			if err == nil || !strings.Contains(err.Error(), "refusing upload_url host") || !strings.Contains(err.Error(), "non-public address") {
				t.Fatalf("err=%v", err)
			}
			if grants != 1 {
				t.Fatalf("grants=%d", grants)
			}
		})
	}
}

// The upload host differs from the API host in production. It must resolve
// to a public address; it receives the grant's token and never the API key,
// and its redirect is not followed. A transport error quotes the upload URL
// without its query, which may carry a signature.
func TestNaceOCRFileUploadsToOtherHostWithoutKey(t *testing.T) {
	withFastNaceNDI(t)
	withNaceNDIHosts(t, map[string]string{"uploads.example.com": "93.184.215.14"})
	fixture := readNaceNDIFixture(t)
	upload := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "uploads.example.com" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Upload-Token") != "upload-token" {
			t.Errorf("upload host=%s headers=%v", r.Host, r.Header)
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://uploads.example.com/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path != "/v1/workspaces/ws_1/files" {
			t.Errorf("upload path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"job_id":"upload_1","status":"succeeded","result":{"file":{"file_id":"file_1","workspace_id":"ws_1"}}}`)
	}))
	defer upload.Close()
	// The upload transport trusts the test certificate, which names
	// example.com hosts, and dials the test server for uploads.example.com.
	transport := upload.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: transport.TLSClientConfig.RootCAs, ServerName: "example.com"}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != "uploads.example.com:443" {
			return nil, fmt.Errorf("dial %s refused by test", addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, upload.Listener.Addr().String())
	}
	withNaceNDIUploadTransport(t, transport)

	uploadURL := "https://uploads.example.com/v1/workspaces/ws_1/files"
	grants, deleted := 0, false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/documents/upload-grants":
			grants++
			_, _ = io.WriteString(w, `{"workspace_id":"ws_1","upload_url":"`+uploadURL+`","token":"upload-token"}`)
		case "POST /v1/documents/parse":
			_, _ = w.Write(fixture)
		case "DELETE /v1/documents/jobs/job_1":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /v1/documents/workspaces/ws_1/files/file_1":
			deleted = true
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	driver := NewNaceModel(map[string]string{"default": api.URL + "/v1"}, URLSuffix{})

	if _, err := naceNDIOCR(t.Context(), driver, "ndi-parse-low", []byte(naceNDITestPDF), nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if !deleted || grants != 1 {
		t.Fatalf("deleted=%v grants=%d", deleted, grants)
	}

	grants, uploadURL = 0, "https://uploads.example.com/redirect"
	if _, err := naceNDIOCR(t.Context(), driver, "ndi-parse-low", []byte(naceNDITestPDF), nil); err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("err=%v, want the redirect refused", err)
	}
	if grants != 1 {
		t.Fatalf("grants=%d, want no retry after a redirect", grants)
	}

	grants, uploadURL = 0, "https://uploads.example.com/v1/workspaces/ws_1/files?X-Signature=secret-signature"
	withNaceNDIUploadTransport(t, naceNDIRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	}))
	_, err := naceNDIOCR(t.Context(), driver, "ndi-parse-low", []byte(naceNDITestPDF), nil)
	if err == nil || !strings.Contains(err.Error(), "https://uploads.example.com/v1/workspaces/ws_1/files") || strings.Contains(err.Error(), "secret-signature") || strings.Contains(err.Error(), "X-Signature") {
		t.Fatalf("err=%v", err)
	}
	if grants != naceNDIMaxAttempts+1 {
		t.Fatalf("grants=%d, want %d attempts and one cleanup upload", grants, naceNDIMaxAttempts)
	}
}

// Verification reads the model catalog and never runs a parse. It uses the
// NDI client: not the driver's logging client, no redirect followed, and
// provider text cleaned.
func TestNaceVerifyOCRConnection(t *testing.T) {
	withSSRFBypass(t)
	var calls []string
	answer := func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":[{"name":"drex-max","alias_for":null}]}`)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		answer(w, r)
	}))
	defer server.Close()
	key := "test-key"
	driver := NewNaceModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{Models: "models"})
	client := *driver.baseModel.httpClient
	client.Transport = naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		t.Errorf("driver client used for %s", req.URL.Path)
		return nil, errors.New("unexpected")
	})
	driver.baseModel.httpClient = &client
	if err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &key}); err != nil {
		t.Fatalf("VerifyOCRConnection: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"GET /v1/models"}) {
		t.Fatalf("calls=%v", calls)
	}

	answer = func(w http.ResponseWriter, r *http.Request) {
		writeNaceNDIJob(w, http.StatusUnauthorized, `{"error":{"code":"invalid_api_key","message":"Authorization: Bearer test-key refused"}}`)
	}
	if err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &key}); err == nil || strings.Contains(err.Error(), "test-key") || !strings.Contains(err.Error(), "status 401: invalid_api_key") {
		t.Fatalf("err=%v", err)
	}
	elsewhere := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere++ }))
	defer other.Close()
	answer = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/models", http.StatusTemporaryRedirect)
	}
	if err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &key}); err == nil || !strings.Contains(err.Error(), "status 307") || elsewhere != 0 {
		t.Fatalf("err=%v elsewhere=%d", err, elsewhere)
	}
}

// The NDI transport waits for answer headers as long as
// LLM_HTTP_RESPONSE_HEADER_TIMEOUT says, like the shared provider clients,
// and 5 minutes when it is unset or not a positive duration. A value below
// the longest hold asked of NDI is raised, so a held call is not cut off and a
// healthy job is not deleted.
func TestNaceNDITransportResponseHeaderTimeout(t *testing.T) {
	if naceNDIMinResponseHeaderTimeout <= time.Duration(max(naceNDIPollWaitSeconds, naceNDIZeroRetentionWaitSeconds))*time.Second {
		t.Fatalf("floor %v does not exceed the longest hold", naceNDIMinResponseHeaderTimeout)
	}
	for value, want := range map[string]time.Duration{"15s": naceNDIMinResponseHeaderTimeout, "2m": 2 * time.Minute, "": naceNDIResponseHeaderTimeout, "soon": naceNDIResponseHeaderTimeout, "-1s": naceNDIResponseHeaderTimeout} {
		t.Setenv(common.EnvLLMHTTPResponseHeaderTimeout, value)
		transport, ok := naceNDINewTransport().(*http.Transport)
		if !ok || transport.ResponseHeaderTimeout != want {
			t.Errorf("%q: timeout=%v, want %v", value, transport.ResponseHeaderTimeout, want)
		}
	}
}

// A result without blocks keeps its Markdown as one text block, so ingestion
// gets the text: on page 1, on the first page sent, or not at all (with a
// warning) when the selection applied here leaves out pages.
func TestNaceNDIResponseMarkdownOnly(t *testing.T) {
	var result naceNDIParseResult
	if err := json.Unmarshal([]byte(`{"document":{"page_count":3,"markdown":"# Scan\n\nBody.","blocks":[]},"disclosures":[]}`), &result); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		selection naceNDISelection
		wantPage  int
	}{
		{"whole document", naceNDISelection{}, 1},
		{"sent selection", naceNDISelection{Send: [][]int{{3, 4}}, Keep: [][]int{{3, 4}}}, 3},
		{"selection covering every page", naceNDISelection{Keep: [][]int{{1, 100000000}}}, 1},
		{"selection leaving out pages", naceNDISelection{Keep: [][]int{{2, 3}}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := naceNDIResponse(&result, tc.selection)
			var blocks []string
			page := 0
			for _, p := range resp.Pages {
				for _, block := range p.Blocks {
					blocks, page = append(blocks, block.Label+":"+block.Text), p.Index+1
				}
			}
			if tc.wantPage == 0 {
				if len(blocks) != 0 || *resp.Text != "" || len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "cannot be limited") {
					t.Fatalf("blocks=%v text=%q warnings=%v", blocks, *resp.Text, resp.Warnings)
				}
				return
			}
			if !reflect.DeepEqual(blocks, []string{"text:# Scan\n\nBody."}) || page != tc.wantPage || *resp.Text != "# Scan\n\nBody." || len(resp.Warnings) != 1 {
				t.Fatalf("blocks=%v page=%d text=%q warnings=%v", blocks, page, *resp.Text, resp.Warnings)
			}
			for i, p := range resp.Pages {
				if p.Index != i || !p.Normalized {
					t.Fatalf("pages=%+v", resp.Pages)
				}
			}
		})
	}
}

func TestNaceNDIBlockLabel(t *testing.T) {
	for blockType, want := range map[string]string{
		"heading": "paragraph_title", "text": "text", "paragraph": "text", "ref_text": "text",
		"table": "table", "figure": "figure", "image_caption": "figure_title",
		"table_caption": "table_title", "equation": "formula", "formula_number": "formula_number",
	} {
		if label, keep, known := naceNDIBlockLabel(blockType); label != want || !keep || !known {
			t.Errorf("%s -> %q keep=%v known=%v, want %q", blockType, label, keep, known, want)
		}
	}
	for _, blockType := range []string{"page_number", "footer", "page_header", "aside_text", "page_footnote"} {
		if _, keep, _ := naceNDIBlockLabel(blockType); keep {
			t.Errorf("%s kept", blockType)
		}
	}
	if label, keep, known := naceNDIBlockLabel("sidebar_note"); label != "text" || !keep || known {
		t.Errorf("unknown type -> %q keep=%v known=%v", label, keep, known)
	}
}

func TestNaceNDIBox(t *testing.T) {
	block := func(location string) naceNDIBlock {
		var b naceNDIBlock
		if err := json.Unmarshal([]byte(`{"type":"text","location":`+location+`}`), &b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, tc := range []struct {
		name     string
		location string
		want     []float64
	}{
		{"rectangle", `{"kind":"page_region","page":1,"polygons":[[[0.25,0.5],[0.75,0.5],[0.75,0.625],[0.25,0.625]]]}`, []float64{0.25, 0.5, 0.75, 0.625}},
		{"two polygons", `{"kind":"page_region","page":1,"polygons":[[[0.5,0.25],[0.75,0.25],[0.75,0.5]],[[0.125,0.5],[0.25,0.75]]]}`, []float64{0.125, 0.25, 0.75, 0.75}},
		{"no polygons", `{"kind":"page_region","page":1,"polygons":[]}`, nil},
		{"zero area", `{"kind":"page_region","page":1,"polygons":[[[0.5,0.5],[0.5,0.75]]]}`, nil},
		{"bad point", `{"kind":"page_region","page":1,"polygons":[[[0.5],[0.75,0.75]]]}`, nil},
		{"sheet range", `{"kind":"sheet_range","page":1,"polygons":[[[0.25,0.5],[0.75,0.625]]]}`, nil},
		{"no location", `null`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := naceNDIBox(block(tc.location)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

// Blocks land on their page in reading order; page furniture is dropped; a
// block without content takes its text from the Markdown span (counted in
// characters, not bytes); blank pages are kept; an unknown type warns once.
func TestNaceNDIResponseMapsBlocks(t *testing.T) {
	var result naceNDIParseResult
	raw := `{"document":{"page_count":3,"markdown":"# Café menu\n\nCrème brûlée 5\n\n7","blocks":[
		{"type":"heading","content":"","markdown_span":{"page":1,"char_start":2,"char_end":11},"location":{"kind":"page_region","page":1,"polygons":[[[0.25,0.125],[0.75,0.25]]]}},
		{"type":"sidebar_note","content":"Crème brûlée 5","markdown_span":{"page":1,"char_start":13,"char_end":27}},
		{"type":"sidebar_note","content":"again","markdown_span":{"page":3,"char_start":0,"char_end":0}},
		{"type":"page_number","content":"7","location":{"kind":"page_region","page":1,"polygons":[[[0.5,0.9],[0.55,0.95]]]}},
		{"type":"text","content":"lost"}
	]},"disclosures":[]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	resp := naceNDIResponse(&result, naceNDISelection{})
	if len(resp.Pages) != 3 || len(resp.Pages[0].Blocks) != 2 || len(resp.Pages[1].Blocks) != 0 || len(resp.Pages[2].Blocks) != 1 {
		t.Fatalf("pages=%+v", resp.Pages)
	}
	heading := resp.Pages[0].Blocks[0]
	if heading.Text != "Café menu" || heading.Label != "paragraph_title" || !reflect.DeepEqual(heading.BBox, []float64{0.25, 0.125, 0.75, 0.25}) {
		t.Fatalf("heading=%+v", heading)
	}
	if body := resp.Pages[0].Blocks[1]; body.Label != "text" || body.BBox != nil {
		t.Fatalf("body=%+v", body)
	}
	if !resp.Pages[1].Normalized || resp.Pages[1].Index != 1 {
		t.Fatalf("blank page=%+v", resp.Pages[1])
	}
	want := []string{`Nace NDI: unknown block type "sidebar_note" kept as text`, "Nace NDI: text block without a page dropped"}
	if !reflect.DeepEqual(resp.Warnings, want) {
		t.Fatalf("warnings=%v", resp.Warnings)
	}
	// The text comes from the kept blocks: the dropped page number is not in it.
	if *resp.Text != "Café menu\n\nCrème brûlée 5\n\nagain" {
		t.Fatalf("text=%q", *resp.Text)
	}
	// Only a result without blocks returns the document Markdown.
	result.Document.Blocks = nil
	if resp := naceNDIResponse(&result, naceNDISelection{}); *resp.Text != result.Document.Markdown {
		t.Fatalf("text=%q", *resp.Text)
	}
}

// A result whose Markdown was cut short keeps the blocks, which carry their
// own content or a span into the preview, and rebuilds the text from them.
func TestNaceNDIResponseTruncatedMarkdown(t *testing.T) {
	var result naceNDIParseResult
	raw := `{"document":{"page_count":1,"markdown":"# Ti","blocks":[
		{"type":"heading","content":"Title","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"text","content":"Body text.","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.3],[0.9,0.4]]]}},
		{"type":"text","content":"","markdown_span":{"page":1,"char_start":2,"char_end":4},"location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.5],[0.9,0.6]]]}},
		{"type":"text","content":"","markdown_span":{"page":1,"char_start":2,"char_end":40},"location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.7],[0.9,0.8]]]}}
	]},"disclosures":[{"code":"content_truncated","target":"document_content"}]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	// The Markdown is a preview: a span inside it still gives its block text,
	// and a span past it gives none.
	resp := naceNDIResponse(&result, naceNDISelection{})
	if *resp.Text != "Title\n\nBody text.\n\nTi" || len(resp.Pages[0].Blocks) != 4 || resp.Pages[0].Blocks[3].Text != "" {
		t.Fatalf("text=%q pages=%+v", *resp.Text, resp.Pages)
	}
	if !reflect.DeepEqual(resp.Warnings, []string{"Nace NDI: content_truncated (document_content)"}) {
		t.Fatalf("warnings=%v", resp.Warnings)
	}
}

// A page number in the response cannot size an arbitrary page list: pages
// stop at naceNDIMaxPages, blocks past it are dropped, and one warning says so.
func TestNaceNDIResponseCapsPages(t *testing.T) {
	var result naceNDIParseResult
	raw := `{"document":{"page_count":4000000000,"markdown":"# T","blocks":[
		{"type":"heading","content":"Kept","location":{"kind":"page_region","page":2,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"text","content":"Far","location":{"kind":"page_region","page":2000000000,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"text","content":"Also far","markdown_span":{"page":2147483647,"char_start":0,"char_end":0}}
	]},"disclosures":[]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	resp := naceNDIResponse(&result, naceNDISelection{})
	if len(resp.Pages) != naceNDIMaxPages || len(resp.Pages[1].Blocks) != 1 || *resp.Text != "Kept" {
		t.Fatalf("pages=%d text=%q", len(resp.Pages), *resp.Text)
	}
	if !reflect.DeepEqual(resp.Warnings, []string{"Nace NDI: only the first 10000 pages were kept"}) {
		t.Fatalf("warnings=%v", resp.Warnings)
	}

	result.Document.PageCount = 2
	result.Document.Blocks = result.Document.Blocks[1:2]
	if resp := naceNDIResponse(&result, naceNDISelection{}); len(resp.Pages) != 2 || len(resp.Warnings) != 1 {
		t.Fatalf("pages=%d warnings=%v", len(resp.Pages), resp.Warnings)
	}
}

// A selection too large to send is applied to the whole-document result, with
// a warning that the whole document was billed; a selection that covers
// every parsed page warns about nothing. The text always comes from the kept
// blocks.
func TestNaceNDIResponseFiltersUnsentSelection(t *testing.T) {
	var result naceNDIParseResult
	raw := `{"document":{"page_count":3,"markdown":"# One\n\n# Two\n\n# Three","blocks":[
		{"type":"heading","content":"One","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"heading","content":"Two","location":{"kind":"page_region","page":2,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"heading","content":"Three","location":{"kind":"page_region","page":3,"polygons":[[[0.1,0.1],[0.9,0.2]]]}}
	]},"disclosures":[]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	resp := naceNDIResponse(&result, naceNDISelection{Keep: [][]int{{2, 1000}}})
	if *resp.Text != "Two\n\nThree" || len(resp.Pages) != 3 || len(resp.Pages[0].Blocks) != 0 || len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "whole document was parsed and billed") {
		t.Fatalf("text=%q pages=%+v warnings=%v", *resp.Text, resp.Pages, resp.Warnings)
	}
	resp = naceNDIResponse(&result, naceNDISelection{Keep: [][]int{{1, 100000000}}})
	if *resp.Text != "One\n\nTwo\n\nThree" || len(resp.Warnings) != 0 {
		t.Fatalf("text=%q warnings=%v", *resp.Text, resp.Warnings)
	}

	// An excluded page that holds only a dropped block type adds no block,
	// and its Markdown stays out of the text.
	raw = `{"document":{"page_count":2,"markdown":"# One\n\nPage 2 of 2","blocks":[
		{"type":"heading","content":"One","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"page_footer","content":"Page 2 of 2","location":{"kind":"page_region","page":2,"polygons":[[[0.1,0.9],[0.9,0.95]]]}}
	]},"disclosures":[]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	resp = naceNDIResponse(&result, naceNDISelection{Keep: [][]int{{1, 1}, {3, 1000}}})
	if *resp.Text != "One" || len(resp.Pages[1].Blocks) != 0 {
		t.Fatalf("text=%q pages=%+v", *resp.Text, resp.Pages)
	}
}

// A page whose text NDI labels aside_text only (a rotated page) keeps those
// blocks as text, in reading order; on a page with other kept blocks,
// aside_text is still dropped. Other page furniture stays dropped everywhere.
func TestNaceNDIResponseKeepsAsideTextOfOtherwiseEmptyPage(t *testing.T) {
	var result naceNDIParseResult
	raw := `{"document":{"page_count":3,"markdown":"Body\n\nMargin\n\nRotated one\n\nRotated two\n\nSide","blocks":[
		{"type":"text","content":"Body","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.1],[0.9,0.2]]]}},
		{"type":"aside_text","content":"Margin","location":{"kind":"page_region","page":1,"polygons":[[[0.0,0.1],[0.05,0.9]]]}},
		{"type":"page_header","content":"Header","location":{"kind":"page_region","page":2,"polygons":[[[0.1,0.0],[0.9,0.05]]]}},
		{"type":"aside_text","content":"Rotated one","location":{"kind":"page_region","page":2,"polygons":[[[0.1,0.1],[0.2,0.9]]]}},
		{"type":"aside_text","content":"","markdown_span":{"page":2,"char_start":27,"char_end":38}},
		{"type":"text","content":" ","location":{"kind":"page_region","page":3,"polygons":[[[0.3,0.1],[0.9,0.2]]]}},
		{"type":"figure","content":"![](figure.png)","location":{"kind":"page_region","page":3,"polygons":[[[0.3,0.3],[0.9,0.6]]]}},
		{"type":"Aside_Text","content":"Side","location":{"kind":"page_region","page":3,"polygons":[[[0.1,0.1],[0.2,0.9]]]}}
	]},"disclosures":[]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	resp := naceNDIResponse(&result, naceNDISelection{})
	var got [][]string
	for _, page := range resp.Pages {
		var blocks []string
		for _, block := range page.Blocks {
			blocks = append(blocks, block.Label+":"+block.Text)
		}
		got = append(got, blocks)
	}
	// Page 3 holds an empty text block and a figure without text, neither with
	// a body the projection indexes, so its aside_text is kept too.
	want := [][]string{{"text:Body"}, {"text:Rotated one", "text:Rotated two"}, {"text: ", "figure:![](figure.png)", "text:Side"}}
	if !reflect.DeepEqual(got, want) || len(resp.Warnings) != 0 {
		t.Fatalf("blocks=%v warnings=%v", got, resp.Warnings)
	}
	if box := resp.Pages[1].Blocks[0].BBox; !reflect.DeepEqual(box, []float64{0.1, 0.1, 0.2, 0.9}) {
		t.Fatalf("box=%v", box)
	}
	// The dropped margin text of page 1 is not in the text either.
	if *resp.Text != "Body\n\nRotated one\n\nRotated two\n\n![](figure.png)\n\nSide" {
		t.Fatalf("text=%q", *resp.Text)
	}

	// Text rebuilt from the kept blocks carries the kept aside_text too.
	resp = naceNDIResponse(&result, naceNDISelection{Keep: [][]int{{2, 2}}})
	if *resp.Text != "Rotated one\n\nRotated two" {
		t.Fatalf("text=%q", *resp.Text)
	}
}

// NDI requests and answers carry the upload URL and token, signed source links
// and the parsed text, so no NDI call goes through the driver's client, whose
// transport logs payloads under LLM_DEBUG; every API call goes through the
// NDI transport, without that log.
func TestNaceOCRFileKeepsNDICallsOutOfProviderLog(t *testing.T) {
	withFastNaceNDI(t)
	fixture := readNaceNDIFixture(t)
	fake := newNaceNDIFake(t)
	fake.parse = func(w http.ResponseWriter, r *http.Request) { writeNaceNDIJob(w, http.StatusOK, string(fixture)) }
	driver := fake.driver()
	var mu sync.Mutex
	var logged, direct []string
	record := func(calls *[]string, base http.RoundTripper) http.RoundTripper {
		return naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			*calls = append(*calls, req.Method+" "+req.URL.Path)
			mu.Unlock()
			return base.RoundTrip(req)
		})
	}
	client := *driver.baseModel.httpClient
	client.Transport = record(&logged, client.Transport)
	driver.baseModel.httpClient = &client
	withNaceNDIAPITransport(t, record(&direct, naceNDITransport(nil)))
	if _, err := naceNDIOCR(t.Context(), driver, "ndi-parse-low", []byte(naceNDITestPDF), nil); err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if len(logged) != 0 {
		t.Fatalf("driver client calls=%v, want none", logged)
	}
	want := []string{"POST /v1/documents/upload-grants", "POST /v1/documents/parse", "DELETE /v1/documents/jobs/job_1", "DELETE /v1/documents/workspaces/ws_1/files/file_1"}
	if !reflect.DeepEqual(direct, want) {
		t.Fatalf("NDI transport calls=%v, want %v", direct, want)
	}
}

// Every Markdown span in the recorded fixture slices, in characters, to its
// block's content.
func TestNaceNDIFixtureSpans(t *testing.T) {
	var job struct {
		Result naceNDIParseResult `json:"result"`
	}
	if err := json.Unmarshal(readNaceNDIFixture(t), &job); err != nil {
		t.Fatal(err)
	}
	markdown := []rune(job.Result.Document.Markdown)
	spans := 0
	for i, block := range job.Result.Document.Blocks {
		if block.MarkdownSpan == nil {
			continue
		}
		spans++
		span := block.MarkdownSpan
		if span.CharStart < 0 || span.CharStart > span.CharEnd || span.CharEnd > len(markdown) || string(markdown[span.CharStart:span.CharEnd]) != block.Content {
			t.Errorf("block %d (%s): span %d-%d does not slice to its content %q", i, block.Type, span.CharStart, span.CharEnd, block.Content)
		}
	}
	if spans == 0 {
		t.Fatal("the fixture has no spans")
	}
}

// A 429, 503 or 529 waits the Retry-After it gives (seconds or an HTTP date) when
// that is at most naceNDIMaxRetryAfter and ends before the run's deadline;
// otherwise the usual backoff applies.
func TestNaceNDIRetryWait(t *testing.T) {
	withFastNaceNDI(t)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for value, want := range map[string]time.Duration{
		"10": 10 * time.Second, " 3 ": 3 * time.Second, "0": 0, "-5": 0, "soon": 0, "": 0,
		"Wed, 18 Nov 2026 12:00:20 GMT": 20 * time.Second, "Wed, 18 Nov 2026 11:59:00 GMT": 0,
	} {
		if got := naceNDIRetryAfter(value, now); got != want {
			t.Errorf("Retry-After %q: got %v, want %v", value, got, want)
		}
	}
	throttled := &naceNDIError{Status: http.StatusTooManyRequests, RetryAfter: 10 * time.Second}
	if got := naceNDIRetryWait(t.Context(), throttled, 1); got != 10*time.Second {
		t.Errorf("no deadline: %v", got)
	}
	short, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if got := naceNDIRetryWait(short, throttled, 2); got != 2*naceNDIRetryDelay {
		t.Errorf("past the deadline: %v", got)
	}
	if got := naceNDIRetryWait(t.Context(), &naceNDIError{Status: http.StatusTooManyRequests, RetryAfter: time.Hour}, 1); got != naceNDIRetryDelay {
		t.Errorf("over the cap: %v", got)
	}
	if got := naceNDIRetryWait(t.Context(), &naceNDIError{Status: http.StatusBadGateway}, 3); got != 3*naceNDIRetryDelay {
		t.Errorf("no Retry-After: %v", got)
	}

	// The refused call keeps the Retry-After of a 429, 503 or 529
	// (service_disabled) only, and each of them is sent again.
	for status, want := range map[int]time.Duration{http.StatusTooManyRequests: 7 * time.Second, http.StatusServiceUnavailable: 7 * time.Second, 529: 7 * time.Second, http.StatusBadGateway: 0} {
		client := &http.Client{Transport: naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"busy"}}`)), Header: http.Header{"Retry-After": {"7"}}}, nil
		})}
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1/documents/jobs/job_1", nil)
		var apiErr *naceNDIError
		if err := naceNDIDo(client, req, &struct{}{}); !errors.As(err, &apiErr) || apiErr.RetryAfter != want || !naceNDITransient(err) {
			t.Errorf("status %d: err=%v", status, err)
		}
	}
}

// naceNDIDeleteTarget is the workspace file the deletion tests remove.
const naceNDIDeleteTarget = "https://ndi.example.com/v1/documents/workspaces/ws_1/files/file_1"

// naceNDIDeleteAnswer builds one DELETE answer: the status, the JSON body NDI
// serves with it, and the Retry-After a 202 or 503 carries when retryAfter is
// set.
func naceNDIDeleteAnswer(status int, retryAfter string) *http.Response {
	body := `{"status":"deleted"}`
	switch status {
	case http.StatusAccepted:
		body = `{"status":"deleting"}`
	case http.StatusNotFound:
		body = `{"error":{"code":"file_not_found","message":"no such file"}}`
	case http.StatusServiceUnavailable, http.StatusForbidden:
		body = `{"error":{"code":"refused","message":"refused"}}`
	case http.StatusNoContent:
		body = ""
	}
	header := http.Header{}
	if retryAfter != "" && (status == http.StatusAccepted || status == http.StatusServiceUnavailable) {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

// withNaceNDIFileDeleteTransport answers the file deletion through an
// in-memory transport, so it uses no network: the nth
// DELETE of target is counted in deletes and answered by answer, and it
// carries the API key and the client header. Any other request fails the
// call; wrong DELETE headers are reported when the test ends.
func withNaceNDIFileDeleteTransport(t *testing.T, target, key string, deletes *int, answer func(n int, req *http.Request) (*http.Response, error)) {
	t.Helper()
	var badHeaders []string
	withNaceNDIAPITransport(t, naceNDIRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete || req.URL.String() != target {
			return nil, fmt.Errorf("unexpected %s %s", req.Method, req.URL)
		}
		*deletes++
		if got := req.Header.Get("Authorization"); got != "Bearer "+key {
			badHeaders = append(badHeaders, "Authorization="+got)
		}
		if got := req.Header.Get(naceClientHeader); !strings.HasPrefix(got, "ragflow/") {
			badHeaders = append(badHeaders, naceClientHeader+"="+got)
		}
		return answer(*deletes, req)
	}))
	t.Cleanup(func() {
		for _, h := range badHeaders {
			t.Errorf("DELETE %s", h)
		}
	})
}

// A file deletion answered 202 (still deleting) is sent again until NDI
// confirms it with 200, 204 or a 404. A check that fails transiently does
// not end the checks; one that fails otherwise does. Each check is sent once,
// and the checks end naceNDIDeleteConfirmTimeout after the first DELETE: a
// Retry-After past that point gives one last check halfway to it, and a
// deletion still in progress then, or a first answer that arrives past it,
// is reported. Every case runs in a synctest bubble against the in-memory
// transport, so the DELETE count and the elapsed time it asserts are exact.
func TestNaceNDIDeleteFileWaitsForDeletion(t *testing.T) {
	withFastNaceNDI(t)
	key := "test-key"
	driver := NewNaceModel(map[string]string{"default": "https://ndi.example.com/v1"}, URLSuffix{})
	for name, tc := range map[string]struct {
		answers []int
		// retryAfter is sent with each 202 and 503.
		retryAfter string
		// window replaces naceNDIDeleteConfirmTimeout when set.
		window time.Duration
		// firstDelay holds the first DELETE's answer past the window.
		firstDelay time.Duration
		want       int
		wantErr    string
		// wantElapsed is the exact time one run takes on the fake clock.
		wantElapsed time.Duration
	}{
		"deleted after two checks":             {answers: []int{202, 202, 200}, want: 3, wantElapsed: 2 * naceNDIDeletePollDelay},
		"gone after a check":                   {answers: []int{202, 404}, want: 2, wantElapsed: naceNDIDeletePollDelay},
		"no content":                           {answers: []int{204}, want: 1},
		"503 twice while deleting":             {answers: []int{202, 503, 503, 200}, want: 4, wantElapsed: 3 * naceNDIDeletePollDelay},
		"refused while deleting":               {answers: []int{202, 403}, want: 2, wantErr: "status 403", wantElapsed: naceNDIDeletePollDelay},
		"Retry-After past the window":          {answers: []int{202, 200}, retryAfter: "3600", window: 200 * time.Millisecond, want: 2, wantElapsed: 100 * time.Millisecond},
		"503 with Retry-After past the window": {answers: []int{202, 503, 200}, retryAfter: "20", window: 200 * time.Millisecond, want: 2, wantErr: "status 503", wantElapsed: 100 * time.Millisecond},
		"still deleting at the last check":     {answers: []int{202, 202}, retryAfter: "3600", window: 200 * time.Millisecond, want: 2, wantErr: "deletion not confirmed: still in progress", wantElapsed: 100 * time.Millisecond},
		"first answer past the window":         {answers: []int{202}, window: 200 * time.Millisecond, firstDelay: 300 * time.Millisecond, want: 1, wantErr: "deletion not confirmed: still in progress", wantElapsed: 300 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.window > 0 {
				previous := naceNDIDeleteConfirmTimeout
				naceNDIDeleteConfirmTimeout = tc.window
				t.Cleanup(func() { naceNDIDeleteConfirmTimeout = previous })
			}
			deletes := 0
			withNaceNDIFileDeleteTransport(t, naceNDIDeleteTarget, key, &deletes, func(n int, _ *http.Request) (*http.Response, error) {
				if n == 1 {
					time.Sleep(tc.firstDelay)
				}
				return naceNDIDeleteAnswer(tc.answers[min(n, len(tc.answers))-1], tc.retryAfter), nil
			})
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				err := driver.ndiDelete(t.Context(), &APIConfig{ApiKey: &key}, naceNDIDeleteTarget, true)
				elapsed := time.Since(start)
				if (tc.wantErr == "" && err != nil) || (tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr))) {
					t.Fatalf("ndiDelete: err=%v, want %q", err, tc.wantErr)
				}
				if deletes != tc.want {
					t.Fatalf("file deletes=%d, want %d", deletes, tc.want)
				}
				if elapsed != tc.wantElapsed {
					t.Fatalf("ndiDelete took %v, want %v", elapsed, tc.wantElapsed)
				}
			})
		})
	}
}

// The checks of a file deletion also end with the cleanup context: a check
// that never answers gives "not confirmed" with the deadline cause, and the
// delete returns exactly at naceNDICleanupTimeout. The in-memory transport
// holds the check until its context ends, in a synctest bubble, so the clock
// and the DELETE count are exact.
func TestNaceNDIDeleteFileStopsAtCleanupTimeout(t *testing.T) {
	withFastNaceNDI(t)
	key := "test-key"
	previous := naceNDICleanupTimeout
	naceNDICleanupTimeout = 200 * time.Millisecond
	t.Cleanup(func() { naceNDICleanupTimeout = previous })
	deletes := 0
	withNaceNDIFileDeleteTransport(t, naceNDIDeleteTarget, key, &deletes, func(n int, req *http.Request) (*http.Response, error) {
		if n > 1 {
			// A later DELETE never answers: it ends with its context.
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		return naceNDIDeleteAnswer(http.StatusAccepted, ""), nil
	})
	driver := NewNaceModel(map[string]string{"default": "https://ndi.example.com/v1"}, URLSuffix{})
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		err := driver.ndiDelete(t.Context(), &APIConfig{ApiKey: &key}, naceNDIDeleteTarget, true)
		elapsed := time.Since(start)
		if err == nil || !strings.Contains(err.Error(), "deletion not confirmed") || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ndiDelete: err=%v, want not confirmed with a deadline error", err)
		}
		if deletes != 2 {
			t.Fatalf("file deletes=%d, want 2", deletes)
		}
		if elapsed != naceNDICleanupTimeout {
			t.Fatalf("ndiDelete took %v, want %v", elapsed, naceNDICleanupTimeout)
		}
	})
}
