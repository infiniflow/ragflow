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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
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

	origAssert := common.AssertURLSafeWithContext
	origPinned := PinnedHTTPClient
	common.AssertURLSafeWithContext = func(ctx context.Context, rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		common.AssertURLSafeWithContext = origAssert
		PinnedHTTPClient = origPinned
	})

	ctx := t.Context()
	data, headers, _, err := FetchRemoteFileSafely(ctx, server.URL+"/report", 100<<20)
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

	origAssert := common.AssertURLSafeWithContext
	origPinned := PinnedHTTPClient
	common.AssertURLSafeWithContext = func(ctx context.Context, rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		common.AssertURLSafeWithContext = origAssert
		PinnedHTTPClient = origPinned
	})

	ctx := t.Context()
	data, headers, finalURL, err := FetchRemoteFileSafely(ctx, server.URL+"/page", 100<<20)
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

func TestFetchRemoteFileSafelyCancelsDNS(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprintf("redirect=%t", redirect), func(t *testing.T) {
			started := make(chan struct{}, 4)
			release := make(chan struct{})
			previousResolver := net.DefaultResolver
			net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-release:
					return nil, errors.New("DNS test released")
				}
			}}
			t.Cleanup(func() { net.DefaultResolver = previousResolver })
			defer close(release)
			t.Setenv(common.EnvAllowAnyHost, "true")
			target := "http://blocked-dns.invalid/image.png"
			if redirect {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "http://blocked-dns.invalid/image.png", http.StatusFound)
				}))
				defer server.Close()
				target = server.URL
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, _, _, err := FetchRemoteFileSafely(ctx, target, 1024); done <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("DNS lookup did not start")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("fetch error = %v, want cancellation", err)
				}
			case <-time.After(time.Second):
				t.Fatal("fetch did not stop during DNS lookup")
			}
		})
	}
}

func TestNormalizeUploadInfoContentPreservesMarkdown(t *testing.T) {
	data := []byte("# HTML lesson\n\n```html\n<div class=\"lesson\">Hello &amp; welcome</div>\n```\n")
	for _, tc := range []struct {
		name        string
		contentType string
	}{
		{"lesson.md", "text/markdown; charset=utf-8"},
		{"lesson.MD", "text/plain; charset=utf-8"},
		{"lesson.markdown", "application/octet-stream"},
		{"lesson.md", ""},
		{"lesson.mdx", "text/plain"},
	} {
		t.Run(tc.name+"/"+tc.contentType, func(t *testing.T) {
			name, contentType, got := NormalizeUploadInfoContent(tc.name, tc.contentType, data)
			if name != tc.name || contentType != "text/markdown" || !bytes.Equal(got, data) {
				t.Fatalf("got (%q, %q, %q), want (%q, text/markdown, %q)", name, contentType, got, tc.name, data)
			}
		})
	}
}

func TestNormalizeUploadInfoContentStillConvertsHTMLAndRecognizesPDF(t *testing.T) {
	html := []byte("<html><body><p>Lesson</p></body></html>")
	for _, contentType := range []string{"text/html; charset=utf-8", "application/octet-stream"} {
		name, _, got := NormalizeUploadInfoContent("lesson.html", contentType, html)
		if name != "lesson.html" || string(got) != "Lesson" {
			t.Fatalf("got (%q, %q), want (lesson.html, Lesson)", name, got)
		}
	}
	name, _, got := NormalizeUploadInfoContent("lesson.md", "text/html", html)
	if name != "lesson.md" || string(got) != "Lesson" {
		t.Fatalf("explicit HTML MIME was not honored: (%q, %q)", name, got)
	}
	name, contentType, got := NormalizeUploadInfoContent("lesson.md", "text/plain", []byte("%PDF-1.7"))
	if name != "lesson.md.pdf" || contentType != "application/pdf" || string(got) != "%PDF-1.7" {
		t.Fatalf("PDF magic was not preserved: (%q, %q, %q)", name, contentType, got)
	}
}
