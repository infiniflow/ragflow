//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMinerUSupportsV1(t *testing.T) {
	t.Run("v1 health", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/health" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()
		ok, err := MinerUSupportsV1(context.Background(), server.URL, "")
		if err != nil || !ok {
			t.Fatalf("SupportsV1 = %v, %v, want true", ok, err)
		}
	})
	t.Run("legacy file_parse only", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()
		ok, err := MinerUSupportsV1(context.Background(), server.URL, "")
		if err != nil {
			t.Fatalf("SupportsV1: %v", err)
		}
		if ok {
			t.Fatal("404 /v1/health should be treated as 3.x")
		}
	})
}

func TestParseMinerUV1UploadJobDownload(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	var (
		mu       sync.Mutex
		gotBytes []byte
		gotTier  string
		gotOCR   string
		polls    int
	)
	zipBuf := minerUTestZip(t, "doc.md", "# Hello\n\nFrom V1.\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-1","status":"pending","upload_url":"/v1/uploads/up-1/content","upload_method":"PUT"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/uploads/up-1/content":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			gotBytes = body
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/up-1/complete":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if tier, ok := payload["tier"].(string); ok {
				mu.Lock()
				gotTier = tier
				mu.Unlock()
			}
			if mode, ok := payload["ocr_mode"].(string); ok {
				mu.Lock()
				gotOCR = mode
				mu.Unlock()
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/parse/jobs/job-1":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if n < 2 {
				_, _ = w.Write([]byte(`{"job_id":"job-1","status":"running"}`))
				return
			}
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"zip":{"file_id":"out-zip"},"markdown":{"file_id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-zip/content":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBuf)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# Hello\n\nFrom V1.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "ocr", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# Hello\n\nFrom V1.\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	if len(result.Zip) == 0 {
		t.Fatal("expected zip artifact")
	}
	mu.Lock()
	defer mu.Unlock()
	if !bytes.Equal(gotBytes, []byte("%PDF-1.4")) {
		t.Fatalf("uploaded bytes = %q", gotBytes)
	}
	if gotTier != "basic" {
		t.Fatalf("tier = %q, want basic (mapped from pipeline)", gotTier)
	}
	if gotOCR != "ocr" {
		t.Fatalf("ocr_mode = %q, want ocr", gotOCR)
	}
}

func TestMinerUV1DownloadStripsAuthOnCrossOriginRedirect(t *testing.T) {
	var sawAuth atomic.Bool
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth.Store(true)
		}
		_, _ = w.Write([]byte("artifact"))
	}))
	defer redirect.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/files/out-1/content" {
			http.Redirect(w, r, redirect.URL, http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()

	body, err := minerUDownloadFile(context.Background(), api.URL, "secret-token", api.URL+"/v1/files/out-1/content")
	if err != nil {
		t.Fatalf("minerUDownloadFile: %v", err)
	}
	if body == nil || string(body) != "artifact" {
		t.Fatalf("body = %q", body)
	}
	if sawAuth.Load() {
		t.Fatal("Authorization must not be forwarded to a cross-origin redirect target")
	}
}

func TestMinerUV1UploadStripsAuthOnCrossOriginRedirect(t *testing.T) {
	var sawAuth atomic.Bool
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer redirect.Close()

	var apiSawAuth atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			apiSawAuth.Store(true)
		}
		if r.URL.Path == "/v1/upload-slot" {
			http.Redirect(w, r, redirect.URL, http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()

	err := minerUPutUploadBytes(context.Background(), "secret-token", api.URL, http.MethodPut, api.URL+"/v1/upload-slot", nil, []byte("data"))
	if err != nil {
		t.Fatalf("minerUPutUploadBytes: %v", err)
	}
	if !apiSawAuth.Load() {
		t.Fatal("same-origin upload must carry the bearer token initially")
	}
	if sawAuth.Load() {
		t.Fatal("Authorization must not be forwarded to a cross-origin redirect target")
	}
}

func TestParseMinerUV1OutputFilesUseFileID(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"markdown":{"file_id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# From file_id\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# From file_id\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
}

func TestParseMinerUV1PartialJobIncludesFileError(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"partial","files":[{"name":"doc.pdf","status":"failed","error":{"message":"page render failed"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for partial job without artifacts")
	}
	if !strings.Contains(err.Error(), "partial") || !strings.Contains(err.Error(), "page render failed") {
		t.Fatalf("error = %q, want partial status and file error", err.Error())
	}
}

func TestParseMinerUV1ZipDownloadFailureFallsBackToMarkdown(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"zip":{"file_id":"out-zip"},"markdown":{"file_id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-zip/content":
			http.Error(w, "zip gone", http.StatusInternalServerError)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# Fallback markdown\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# Fallback markdown\n" {
		t.Fatalf("markdown = %q, want fallback artifact", result.Markdown)
	}
	if len(result.Zip) != 0 {
		t.Fatalf("zip = %d bytes, want empty after download failure", len(result.Zip))
	}
}

func TestMinerUMarkdownFromZip(t *testing.T) {
	buf := minerUTestZip(t, "out.md", "# Title\n")
	md, err := minerUMarkdownFromZip(buf)
	if err != nil {
		t.Fatalf("minerUMarkdownFromZip: %v", err)
	}
	if md != "# Title\n" {
		t.Fatalf("md = %q", md)
	}
}

func TestMinerUMarkdownFromStructuredContentZip(t *testing.T) {
	buf := minerUTestZip(t, "structured_content.json", `{"pages":[{"blocks":[
		{"type":"header","content":"HEADER"},
		{"type":"paragraph_title","content":"章节"},
		{"type":"text","content":"正文"},
		{"type":"table","content":"单元格","captions":[{"content":"表1"}]}
	]}]}`)
	md, err := minerUMarkdownFromZip(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "章节") || !strings.Contains(md, "正文") || !strings.Contains(md, "表1") || strings.Contains(md, "HEADER") {
		t.Fatalf("md = %q", md)
	}
}

func TestMinerUMarkdownFromZipFallsThroughEmptyContentList(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[{"type":"header"}]`))
	structured, err := zw.Create("structured_content.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = structured.Write([]byte(`{"pages":[{"blocks":[{"type":"text","content":"正文"}]}]}`))
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	md, err := minerUMarkdownFromZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if md != "正文" {
		t.Fatalf("md = %q", md)
	}
}

func TestMinerULocalCheckConnectionPrefersV1Health(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/health" {
			writeMinerUHealth(w)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	if err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &baseURL}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/v1/health" {
		t.Fatalf("paths = %v, want only /v1/health", paths)
	}
}

func minerUTestZip(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMinerUSameOrigin(t *testing.T) {
	if !minerUSameOrigin("http://127.0.0.1:8000", "http://127.0.0.1:8000/v1/uploads/1/content") {
		t.Fatal("same host/port should be same-origin")
	}
	if minerUSameOrigin("http://127.0.0.1:8000", "http://127.0.0.1:9000/v1/uploads/1/content") {
		t.Fatal("different port is not same-origin")
	}
	if strings.TrimSpace("") != "" {
		t.Fatal("sanity")
	}
}

type minerUErrBody struct{}

func (minerUErrBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (minerUErrBody) Close() error             { return nil }

type minerUStatusTransport struct{ code int }

func (t minerUStatusTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.code, Body: minerUErrBody{}, Header: make(http.Header)}, nil
}

type minerUFailTransport struct{}

func (minerUFailTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestMinerUProbeGETKeepsStatusWhenBodyReadFails(t *testing.T) {
	client := &http.Client{Transport: minerUStatusTransport{code: http.StatusOK}}
	code, err := minerUProbeGET(context.Background(), client, "http://mineru.example/v1/health", "")
	if err != nil {
		t.Fatalf("probe error = %v, want nil when the status is already known", err)
	}
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200", code)
	}

	client = &http.Client{Transport: minerUFailTransport{}}
	if _, err = minerUProbeGET(context.Background(), client, "http://mineru.example/v1/health", ""); err == nil {
		t.Fatal("probe error = nil, want the transport error when no status was received")
	}
}

func TestParseMinerUV1TasksFlow(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	var (
		mu         sync.Mutex
		created    map[string]any
		gotBytes   []byte
		uploadHdr  string
		uploadAuth bool
		polls      int
	)
	zipBuf := minerUTestZip(t, "markdown.md", "# Tasks\n\nFrom 4.1.\n")

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			mu.Lock()
			created = payload
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"task_id":"task-1","status":"waiting-file","files":[{"file_id":"file-1","file_name":"sample.pdf","status":"waiting-file","upload":{"url":"/v1/tasks/task-1/uploads/file-1","headers":{"X-Up":"1"},"expires_at":1800003600}}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/tasks/task-1/uploads/file-1":
			mu.Lock()
			uploadHdr = r.Header.Get("X-Up")
			uploadAuth = r.Header.Get("Authorization") != ""
			gotBytes, _ = io.ReadAll(r.Body)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/task-1":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if n < 2 {
				_, _ = w.Write([]byte(`{"task_id":"task-1","status":"pending","progress":{"completed":0,"failed":0,"total":1},"files":[{"file_id":"file-1","file_name":"sample.pdf","status":"pending"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"task_id":"task-1","status":"completed","progress":{"completed":1,"failed":0,"total":1},"files":[{"file_id":"file-1","file_name":"sample.pdf","status":"completed","parse":{"tier":"standard","duration_ms":8000},"result_url":"` + server.URL + `/v1/files/file-1/content"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-1/content":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBuf)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "ocr", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# Tasks\n\nFrom 4.1.\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	if len(result.Zip) == 0 {
		t.Fatal("expected zip artifact")
	}
	mu.Lock()
	defer mu.Unlock()
	files, _ := created["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("creation files = %v", created["files"])
	}
	file, _ := files[0].(map[string]any)
	if file["file_name"] != "sample.pdf" {
		t.Fatalf("file_name = %v", file["file_name"])
	}
	if file["bytes"] != float64(8) {
		t.Fatalf("bytes = %v, want 8", file["bytes"])
	}
	// The upload payload is asserted earlier in this test (%PDF-1.4); compare the
	// manifest digest against the exact SHA-256 of that same payload so a wrong
	// digest of the correct length cannot pass.
	wantSum := sha256.Sum256([]byte("%PDF-1.4"))
	if sum, _ := file["sha256sum"].(string); sum != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("sha256sum = %v, want %x", file["sha256sum"], wantSum)
	}
	if file["file_id"] != nil || file["client_file_id"] != nil {
		t.Fatalf("client must not send file_id/client_file_id: %v", file)
	}
	if created["tier"] != "basic" {
		t.Fatalf("tier = %v, want basic (mapped from pipeline)", created["tier"])
	}
	if created["ocr_mode"] != "ocr" {
		t.Fatalf("ocr_mode = %v, want ocr", created["ocr_mode"])
	}
	if formats, _ := created["output_formats"].([]any); len(formats) != 1 || formats[0] != "default" {
		t.Fatalf("output_formats = %v, want [default]", created["output_formats"])
	}
	if !bytes.Equal(gotBytes, []byte("%PDF-1.4")) {
		t.Fatalf("uploaded bytes = %q", gotBytes)
	}
	if uploadHdr != "1" {
		t.Fatalf("upload headers not applied: X-Up = %q", uploadHdr)
	}
	if !uploadAuth {
		t.Fatal("same-origin upload must carry the bearer token")
	}
}

func TestParseMinerUV1TasksUploadSlotArrivesOnPoll(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	var (
		mu       sync.Mutex
		uploaded bool
		polls    int
	)
	zipBuf := minerUTestZip(t, "markdown.md", "# Late slot\n")

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"task_id":"task-2","status":"waiting-file","files":[{"file_id":"file-2","file_name":"sample.pdf","status":"waiting-file"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/task-2":
			mu.Lock()
			polls++
			n := polls
			uploadedAlready := uploaded
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case n == 1:
				_, _ = w.Write([]byte(`{"task_id":"task-2","status":"waiting-file","files":[{"file_id":"file-2","status":"waiting-file","upload":{"url":"/v1/tasks/task-2/uploads/file-2","headers":{},"expires_at":1800003600}}]}`))
			case n == 2 && !uploadedAlready:
				t.Error("upload slot was offered but no PUT arrived before the next poll")
				_, _ = w.Write([]byte(`{"task_id":"task-2","status":"waiting-file","files":[{"file_id":"file-2","status":"waiting-file"}]}`))
			default:
				_, _ = w.Write([]byte(`{"task_id":"task-2","status":"completed","files":[{"file_id":"file-2","status":"completed","result_url":"` + server.URL + `/v1/files/file-2/content"}]}`))
			}
		case r.Method == http.MethodPut && r.URL.Path == "/v1/tasks/task-2/uploads/file-2":
			mu.Lock()
			uploaded = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-2/content":
			_, _ = w.Write(zipBuf)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "standard", "auto", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# Late slot\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
}

func TestParseMinerUV1TasksFailedFileSurfacesError(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"task_id":"task-3","status":"waiting-file","files":[{"file_id":"file-3","status":"waiting-file","upload":{"url":"/v1/tasks/task-3/uploads/file-3","headers":{},"expires_at":1800003600}}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/tasks/task-3/uploads/file-3":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/task-3":
			_, _ = w.Write([]byte(`{"task_id":"task-3","status":"partial","files":[{"file_id":"file-3","status":"failed","error":{"code":"render_failed","message":"page render failed"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for failed task file")
	}
	if !strings.Contains(err.Error(), "partial") || !strings.Contains(err.Error(), "page render failed") {
		t.Fatalf("error = %q, want partial status and file error", err.Error())
	}
}

func TestParseMinerUV1TasksCreationErrorIsNotMaskedByLegacy(t *testing.T) {
	var legacyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			http.Error(w, `{"error":{"code":"invalid_api_key","message":"bad key"}}`, http.StatusUnauthorized)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			legacyCalls.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err == nil {
		t.Fatal("expected the 401 creation error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %q, want the 401 status", err.Error())
	}
	if legacyCalls.Load() != 0 {
		t.Fatal("legacy cycle must not run for a non-404 creation failure")
	}
}

func TestParseMinerUV1LegacySelectedForVersion40(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	var (
		mu         sync.Mutex
		tasksCalls int
		gotBytes   []byte
		gotTier    string
		gotOCR     string
		polls      int
	)
	zipBuf := minerUTestZip(t, "doc.md", "# From 4.0 Legacy\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"4.0.12"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			mu.Lock()
			tasksCalls++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":[{"loc":["body","files",0,"source"],"msg":"field required","type":"value_error.missing"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-40","status":"pending","upload_url":"/v1/uploads/up-40/content","upload_method":"PUT"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/uploads/up-40/content":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			gotBytes = body
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/up-40/complete":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-40","status":"completed","file":{"id":"file-40"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if tier, ok := payload["tier"].(string); ok {
				mu.Lock()
				gotTier = tier
				mu.Unlock()
			}
			if mode, ok := payload["ocr_mode"].(string); ok {
				mu.Lock()
				gotOCR = mode
				mu.Unlock()
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-40","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/parse/jobs/job-40":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if n < 2 {
				_, _ = w.Write([]byte(`{"job_id":"job-40","status":"running"}`))
				return
			}
			_, _ = w.Write([]byte(`{"job_id":"job-40","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"zip":{"file_id":"out-zip"},"markdown":{"file_id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-zip/content":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBuf)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# From 4.0 Legacy\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "ocr", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# From 4.0 Legacy\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	if len(result.Zip) == 0 {
		t.Fatal("expected zip artifact")
	}
	mu.Lock()
	defer mu.Unlock()
	if tasksCalls != 0 {
		t.Fatalf("expected /v1/tasks not to be called when version is 4.0.12, got %d calls", tasksCalls)
	}
	if !bytes.Equal(gotBytes, []byte("%PDF-1.4")) {
		t.Fatalf("uploaded bytes = %q", gotBytes)
	}
	if gotTier != "basic" {
		t.Fatalf("tier = %q, want basic", gotTier)
	}
	if gotOCR != "ocr" {
		t.Fatalf("ocr_mode = %q, want ocr", gotOCR)
	}
}

func TestParseMinerUV1LegacySelectedWhenTasksReturns400(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	var (
		mu         sync.Mutex
		tasksCalls int
		polls      int
	)
	zipBuf := minerUTestZip(t, "doc.md", "# From 4.0 Legacy Via 400 Fallback\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			// Health endpoint is present but omits version (decided=false).
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			mu.Lock()
			tasksCalls++
			mu.Unlock()
			// A 4.0 server returns HTTP 400 validation error on the 4.1 manifest.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":[{"loc":["body","files",0,"source"],"msg":"field required","type":"value_error.missing"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-40","status":"pending","upload_url":"/v1/uploads/up-40/content","upload_method":"PUT"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/uploads/up-40/content":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/up-40/complete":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-40","status":"completed","file":{"id":"file-40"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-40","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/parse/jobs/job-40":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if n < 2 {
				_, _ = w.Write([]byte(`{"job_id":"job-40","status":"running"}`))
				return
			}
			_, _ = w.Write([]byte(`{"job_id":"job-40","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"zip":{"file_id":"out-zip"},"markdown":{"file_id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-zip/content":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBuf)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# From 4.0 Legacy Via 400 Fallback\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "ocr", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if result.Markdown != "# From 4.0 Legacy Via 400 Fallback\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	mu.Lock()
	defer mu.Unlock()
	if tasksCalls != 1 {
		t.Fatalf("expected /v1/tasks to be called once and return 400, got %d calls", tasksCalls)
	}
}

func TestParseMinerUV1HealthCachedBetweenSupportAndParse(t *testing.T) {
	var healthCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			healthCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"4.0.12"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"status":"completed","output_files":{"markdown":{"file_id":"md-1"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/md-1/content":
			_, _ = w.Write([]byte("# Cached\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	apiKey := "test-cache-key"
	supported, err := MinerUSupportsV1(ctx, server.URL, apiKey)
	if err != nil || !supported {
		t.Fatalf("MinerUSupportsV1: supported=%v, err=%v", supported, err)
	}
	if healthCalls != 1 {
		t.Fatalf("healthCalls after MinerUSupportsV1 = %d, want 1", healthCalls)
	}

	res, err := ParseMinerUV1(ctx, server.URL, apiKey, "test.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	if res.Markdown != "# Cached\n" {
		t.Fatalf("res.Markdown = %q", res.Markdown)
	}
	if healthCalls != 1 {
		t.Fatalf("healthCalls after ParseMinerUV1 = %d, want 1 (should be cached)", healthCalls)
	}
}

func TestParseMinerUTasksMultiFileMarkdownJoined(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	zip1 := minerUTestZip(t, "part1.md", "# Part 1\n")
	zip2 := minerUTestZip(t, "part2.md", "# Part 2\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"4.1.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{
				"task_id":"task-multi",
				"status":"completed",
				"files":[
					{"file_id":"f1","file_name":"part1.pdf","status":"completed","result_url":"/v1/download/f1.zip"},
					{"file_id":"f2","file_name":"part2.pdf","status":"completed","result_url":"/v1/download/f2.zip"}
				]
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/download/f1.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zip1)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/download/f2.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zip2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	res, err := ParseMinerUV1(context.Background(), server.URL, "secret", "multi.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err != nil {
		t.Fatalf("ParseMinerUV1: %v", err)
	}
	expected := "# Part 1\n\n# Part 2\n"
	if res.Markdown != expected {
		t.Fatalf("res.Markdown = %q, want %q", res.Markdown, expected)
	}
	if len(res.Zip) == 0 {
		t.Fatal("expected non-empty zip")
	}
}

func TestMinerUParseSemverAndTasksSupport(t *testing.T) {
	tests := []struct {
		version       string
		wantMajor     int
		wantMinor     int
		wantPatch     int
		wantOK        bool
		supportsTasks bool
	}{
		{"4.0.12", 4, 0, 12, true, false},
		{"4.0.0", 4, 0, 0, true, false},
		{"4.0", 4, 0, 0, true, false},
		{"v4.0.10", 4, 0, 10, true, false},
		{"4.1.0", 4, 1, 0, true, true},
		{"4.1.2", 4, 1, 2, true, true},
		{"v4.2.0-rc1", 4, 2, 0, true, true},
		{"5.0.0", 5, 0, 0, true, true},
		{"3.0.5", 3, 0, 5, true, false},
		{"invalid", 0, 0, 0, false, false},
		{"", 0, 0, 0, false, false},
	}
	for _, tc := range tests {
		major, minor, patch, ok := minerUParseSemver(tc.version)
		if ok != tc.wantOK {
			t.Errorf("minerUParseSemver(%q) ok = %v, want %v", tc.version, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if major != tc.wantMajor || minor != tc.wantMinor || patch != tc.wantPatch {
			t.Errorf("minerUParseSemver(%q) = (%d, %d, %d), want (%d, %d, %d)", tc.version, major, minor, patch, tc.wantMajor, tc.wantMinor, tc.wantPatch)
		}
		if got := minerUV1VersionSupportsTasks(major, minor); got != tc.supportsTasks {
			t.Errorf("minerUV1VersionSupportsTasks(%d, %d) = %v, want %v", major, minor, got, tc.supportsTasks)
		}
	}
}

func TestParseMinerUV1TasksErrorOnVersion41IsNotMaskedByLegacy(t *testing.T) {
	var legacyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"4.1.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			http.Error(w, `{"error":{"code":"invalid_request","message":"bad field"}}`, http.StatusBadRequest)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			legacyCalls.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", "auto", 5*time.Second)
	if err == nil {
		t.Fatal("expected the 400 creation error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error = %q, want the 400 status", err.Error())
	}
	if legacyCalls.Load() != 0 {
		t.Fatal("legacy cycle must not run for 4.1 server creation failure")
	}
}
