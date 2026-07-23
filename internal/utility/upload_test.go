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

package utility

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHtmlToReadableMarkdown(t *testing.T) {
	out := htmlToReadableMarkdown([]byte("<p>hello</p><div>world</div>"))
	if bytes.Contains(out, []byte("<")) {
		t.Errorf("tags not stripped: %q", out)
	}
	if !bytes.Contains(out, []byte("hello")) || !bytes.Contains(out, []byte("world")) {
		t.Errorf("text lost: %q", out)
	}
}

func TestFetchRemoteFileSafely_PDFAddsExtension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7 fake pdf"))
	}))
	defer server.Close()

	origAssert := AssertURLSafe
	origPinned := PinnedHTTPClient
	AssertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		AssertURLSafe = origAssert
		PinnedHTTPClient = origPinned
	})

	data, headers, _, err := FetchRemoteFileSafely(server.URL+"/report", 100<<20)
	if err != nil {
		t.Fatalf("FetchRemoteFileSafely failed: %v", err)
	}
	if ct := headers.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", ct)
	}
	if !bytes.Equal(data, []byte("%PDF-1.7 fake pdf")) {
		t.Fatalf("data = %q, want %%PDF-1.7 fake pdf", string(data))
	}
}

func TestFetchRemoteFileSafely_ReturnsContentAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>Hello World</body></html>`))
	}))
	defer server.Close()

	origAssert := AssertURLSafe
	origPinned := PinnedHTTPClient
	AssertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		AssertURLSafe = origAssert
		PinnedHTTPClient = origPinned
	})

	data, headers, finalURL, err := FetchRemoteFileSafely(server.URL+"/page", 100<<20)
	if err != nil {
		t.Fatalf("FetchRemoteFileSafely failed: %v", err)
	}
	if ct := headers.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	if finalURL != server.URL+"/page" {
		t.Fatalf("finalURL = %q, want %q", finalURL, server.URL+"/page")
	}
	if !bytes.Equal(data, []byte(`<html><body>Hello World</body></html>`)) {
		t.Fatalf("data = %q", string(data))
	}
}
