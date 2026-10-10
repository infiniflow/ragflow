//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	orig := minerUV1PollInterval
	minerUV1PollInterval = time.Millisecond
	t.Cleanup(func() { minerUV1PollInterval = orig })

	var (
		mu       sync.Mutex
		gotBytes []byte
		gotTier  string
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
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"zip":{"id":"out-zip"},"markdown":{"id":"out-md"}}}]}`))
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

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", 5*time.Second)
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
}

func TestParseMinerUV1ZipDownloadFailureFallsBackToMarkdown(t *testing.T) {
	orig := minerUV1PollInterval
	minerUV1PollInterval = time.Millisecond
	t.Cleanup(func() { minerUV1PollInterval = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			_, _ = w.Write([]byte(`{"id":"up-1","status":"completed","file":{"id":"file-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/parse/jobs":
			_, _ = w.Write([]byte(`{"job_id":"job-1","status":"completed","files":[{"name":"doc.pdf","status":"completed","output_files":{"zip":{"id":"out-zip"},"markdown":{"id":"out-md"}}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-zip/content":
			http.Error(w, "zip gone", http.StatusInternalServerError)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out-md/content":
			_, _ = w.Write([]byte("# Fallback markdown\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUV1(context.Background(), server.URL, "secret", "sample.pdf", []byte("%PDF-1.4"), "pipeline", 5*time.Second)
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
	md, err := MinerUMarkdownFromZip(buf)
	if err != nil {
		t.Fatalf("MinerUMarkdownFromZip: %v", err)
	}
	if md != "# Title\n" {
		t.Fatalf("md = %q", md)
	}
}

func TestMinerULocalCheckConnectionPrefersV1Health(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/health" {
			w.WriteHeader(http.StatusOK)
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
