//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIsMinerUCloudBase(t *testing.T) {
	if !IsMinerUCloudBase("https://mineru.net") {
		t.Fatal("mineru.net should be the cloud host")
	}
	if !IsMinerUCloudBase("https://mineru.net/api/v4") {
		t.Fatal("path on mineru.net should still be the cloud host")
	}
	if IsMinerUCloudBase("http://127.0.0.1:8000") {
		t.Fatal("local MinerU is not the cloud host")
	}
}

func TestMinerUCloudModelVersion(t *testing.T) {
	if got := minerUCloudModelVersion("pipeline"); got != "pipeline" {
		t.Fatalf("pipeline = %q", got)
	}
	if got := minerUCloudModelVersion("vlm-http-client"); got != "vlm" {
		t.Fatalf("vlm-http-client = %q", got)
	}
}

func TestParseMinerUCloudV4(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	zipBytes := minerUTestZip(t, "full.md", "# Cloud\n")
	var mu sync.Mutex
	var uploaded []byte
	var polls int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/file-urls/batch":
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Errorf("auth = %q", got)
			}
			var raw []byte
			raw, _ = io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"model_version":"vlm"`) {
				t.Errorf("body = %s", raw)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"batch_id":"b1","file_urls":["`+server.URL+`/upload"]}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/upload":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			uploaded = append([]byte(nil), body...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/extract-results/batch/b1":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if n == 1 {
				_, _ = w.Write([]byte(`{"code":0,"data":{"extract_result":[{"state":"running"}]}}`))
				return
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"extract_result":[{"state":"done","full_zip_url":"`+server.URL+`/zip"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/zip":
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUCloud(context.Background(), server.URL, "secret", "doc.pdf", []byte("%PDF"), "vlm-http-client", "ocr", "ch")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Zip) == 0 || !strings.Contains(result.Markdown, "Cloud") {
		t.Fatalf("result zip=%d md=%q", len(result.Zip), result.Markdown)
	}
	mu.Lock()
	defer mu.Unlock()
	if string(uploaded) != "%PDF" {
		t.Fatalf("uploaded %q", uploaded)
	}
}

func TestParseMinerUCloudAgent(t *testing.T) {
	orig := minerUV1PollInitial
	minerUV1PollInitial = time.Millisecond
	t.Cleanup(func() { minerUV1PollInitial = orig })

	var polls int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/parse/file":
			if r.Header.Get("Authorization") != "" {
				t.Errorf("agent API must not send Authorization, got %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"task_id":"t1","file_url":"`+server.URL+`/upload"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/upload":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/parse/t1":
			polls++
			if polls == 1 {
				_, _ = w.Write([]byte(`{"code":0,"data":{"state":"running"}}`))
				return
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"state":"done","markdown_url":"`+server.URL+`/full.md"}}`)
		case r.URL.Path == "/full.md":
			_, _ = w.Write([]byte("# Agent\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ParseMinerUCloud(context.Background(), server.URL, "", "doc.pdf", []byte("%PDF"), "pipeline", "auto", "en")
	if err != nil {
		t.Fatal(err)
	}
	if result.Markdown != "# Agent\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
}

func TestMinerUCloudReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := minerUCloudReachable(context.Background(), server.Client(), u); err != nil {
		t.Fatal(err)
	}
}
