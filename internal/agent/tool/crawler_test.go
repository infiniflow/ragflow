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

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head><title>RAGFlow Docs</title></head>
<body>
<h1>Welcome</h1>
<p>This is a paragraph with <a href="/docs">docs link</a> and <a href="https://example.com">external</a>.</p>
<script>var secret = "ignored";</script>
<style>body { color: red; }</style>
</body>
</html>`

func TestCrawler_FetchesAndExtractsText(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	// httptest.NewServer binds to 127.0.0.1 which the production
	// SSRF guard correctly blocks. Bypass it for this test only by
	// installing a resolver that returns the literal host/IP. This
	// mirrors the previous WithSSRFValidator behaviour and keeps the
	// test exercising the pinned-connect path (DoPinned), which is the
	// whole point of the M1-rebinding fix.
	loopbackResolver := func(rawURL string) (string, net.IP, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", nil, err
		}
		host := u.Hostname()
		return host, net.ParseIP(host), nil
	}
	c := NewCrawlerTool().WithResolver(loopbackResolver)
	out, err := c.InvokableRun(context.Background(),
		`{"url":`+jsonString(srv.URL)+`,"max_depth":0}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var got crawlerResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if got.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", got.Status)
	}
	if !strings.Contains(got.Title, "RAGFlow Docs") {
		t.Errorf("Title = %q, want to contain 'RAGFlow Docs'", got.Title)
	}
	if !strings.Contains(got.Content, "Welcome") {
		t.Errorf("Content = %q, want to contain 'Welcome'", got.Content)
	}
	// <script> and <style> contents must be stripped.
	if strings.Contains(got.Content, "secret") {
		t.Errorf("Content leaked <script> text: %q", got.Content)
	}
	if strings.Contains(got.Content, "color: red") {
		t.Errorf("Content leaked <style> text: %q", got.Content)
	}
	// Both links should be captured.
	wantLinks := map[string]bool{"/docs": false, "https://example.com": false}
	for _, l := range got.Links {
		if _, ok := wantLinks[l]; ok {
			wantLinks[l] = true
		}
	}
	for href, seen := range wantLinks {
		if !seen {
			t.Errorf("missing link %q in result", href)
		}
	}
}

func TestCrawler_RejectsMaxDepthGreaterThanZero(t *testing.T) {
	t.Parallel()

	c := NewCrawlerTool()
	_, err := c.InvokableRun(context.Background(), `{"url":"https://example.com","max_depth":1}`)
	if !errors.Is(err, ErrCrawlerDepthUnsupported) {
		t.Fatalf("err = %v, want ErrCrawlerDepthUnsupported", err)
	}
}

func TestCrawler_RejectsMissingURL(t *testing.T) {
	t.Parallel()

	c := NewCrawlerTool()
	_, err := c.InvokableRun(context.Background(), `{"url":""}`)
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestCrawler_RejectsNonHTTPScheme(t *testing.T) {
	t.Parallel()

	c := NewCrawlerTool()
	_, err := c.InvokableRun(context.Background(), `{"url":"file:///etc/passwd"}`)
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("err = %v, want to reject file:// scheme", err)
	}
}

func TestCrawler_Info(t *testing.T) {
	t.Parallel()

	c := NewCrawlerTool()
	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "crawler" {
		t.Errorf("Name = %q, want crawler", info.Name)
	}
	if !strings.Contains(info.Desc, "text") {
		t.Errorf("Desc = %q, want to mention text extraction", info.Desc)
	}
}

// jsonString is defined in exesql_test.go (same package).
