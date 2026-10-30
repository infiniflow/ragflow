//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

// allowLoopbackImages lets one test reach a loopback httptest server through
// the shared SSRF guard. Every other URL still goes through the production
// guard, so a direct or redirected reference to a private address stays
// rejected.
func allowLoopbackImages(t *testing.T) {
	t.Helper()
	previous := common.AssertURLSafe
	common.AssertURLSafe = func(rawURL string) (string, string, error) {
		parsed, err := url.Parse(strings.TrimSpace(rawURL))
		if err == nil {
			if host := parsed.Hostname(); host != "" {
				if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
					return host, ip.String(), nil
				}
			}
		}
		return previous(rawURL)
	}
	t.Cleanup(func() { common.AssertURLSafe = previous })
}

func imageTestServer(t *testing.T, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestHTMLParser_FetchesRemoteImageIntoRunCache(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	const body = "\x89PNG\r\n\x1a\nremote-bytes"
	server := imageTestServer(t, body, &hits)
	ctx, cache := recordingMediaContext()

	result := NewHTMLParser().ParseWithResult(ctx, "remote.html", []byte(`<p>before <img alt="remote alt" src="`+server.URL+`/image.png"> after</p>`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 1 || cache.puts[0].format != "html" || cache.puts[0].mime != "image/png" || string(cache.puts[0].data) != body {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	var imageItem map[string]any
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] == DocTypeImage {
			imageItem = m
		}
	}
	if imageItem == nil {
		t.Fatalf("no image item emitted: %+v", ItemsToMaps(result.JSON))
	}
	ref := entity.MediaRefFromAny(imageItem["media_ref"])
	if ref == nil || ref.Format != "html" || ref.MIME != "image/png" || ref.Size != int64(len(body)) {
		t.Fatalf("media_ref = %+v", ref)
	}
	if _, ok := imageItem["image"]; ok {
		t.Fatalf("image bytes retained: %+v", imageItem)
	}
	if strings.Join(result.Warnings, "\n") != "" {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
}

func TestHTMLParser_RemoteImageDownloadFailureWarns(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	ctx, cache := recordingMediaContext()

	result := NewHTMLParser().ParseWithResult(ctx, "missing.html", []byte(`<p>before</p><img alt="gone" src="`+server.URL+`/missing.png"><p>after</p>`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	for _, item := range result.JSON {
		if item.ToMap()["doc_type_kwd"] == DocTypeImage {
			t.Fatalf("image item emitted for a failed download: %+v", item.ToMap())
		}
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "remote image(s) that could not be downloaded") {
		t.Fatalf("missing remote download warning: %v", result.Warnings)
	}
}

func TestHTMLParser_RepeatedRemoteImagesFetchOnce(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := imageTestServer(t, "shared-bytes", &hits)
	img := `<img src="` + server.URL + `/same.png">`
	ctx, cache := recordingMediaContext()

	result := NewHTMLParser().ParseWithResult(ctx, "repeat.html", []byte("<p>"+img+img+"</p>"))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1 for a repeated source", hits.Load())
	}
	if len(cache.puts) != 1 {
		t.Fatalf("cache puts = %d, want 1", len(cache.puts))
	}
	images := 0
	var firstRef *entity.MediaRef
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] != DocTypeImage {
			continue
		}
		images++
		ref := entity.MediaRefFromAny(m["media_ref"])
		if ref == nil {
			t.Fatalf("image item without media_ref: %+v", m)
		}
		if firstRef == nil {
			firstRef = ref
		} else if *ref != *firstRef {
			t.Fatalf("repeated source produced different refs: %+v vs %+v", ref, firstRef)
		}
	}
	if images != 2 {
		t.Fatalf("image items = %d, want 2", images)
	}
}

func TestHTMLParser_RejectsRemoteImageToInternalAddress(t *testing.T) {
	ctx, cache := recordingMediaContext()
	// Private and loopback literals are rejected before any connection is
	// attempted, so this test needs no network.
	for _, source := range []string{"http://10.0.0.1/image.png", "http://127.0.0.1/image.png"} {
		result := NewHTMLParser().ParseWithResult(ctx, "ssrf.html", []byte(`<img alt="blocked" src="`+source+`">`))
		if result.Err != nil {
			t.Fatalf("ParseWithResult(%s): %v", source, result.Err)
		}
		if len(cache.puts) != 0 {
			t.Fatalf("cache puts = %+v, want none", cache.puts)
		}
		if !strings.Contains(strings.Join(result.Warnings, "\n"), "could not be downloaded") {
			t.Fatalf("missing SSRF rejection warning for %s: %v", source, result.Warnings)
		}
	}
}

func TestHTMLParser_RemoteRedirectToPrivateAddressIsRejected(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "http://10.0.0.1/image.png", http.StatusFound)
	}))
	defer server.Close()
	ctx, cache := recordingMediaContext()

	result := NewHTMLParser().ParseWithResult(ctx, "redirect.html", []byte(`<img src="`+server.URL+`/redirect.png">`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("first-hop fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "could not be downloaded") {
		t.Fatalf("missing redirect rejection warning: %v", result.Warnings)
	}
}

func TestHTMLParser_BoundsRemoteImageFetchingPerDocument(t *testing.T) {
	allowLoopbackImages(t)
	previous := htmlImageProcessingBudget
	htmlImageProcessingBudget = 50 * time.Millisecond
	t.Cleanup(func() { htmlImageProcessingBudget = previous })

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("slow-image"))
	}))
	defer server.Close()
	ctx, _ := recordingMediaContext()

	input := `<p>before</p><img src="` + server.URL + `/slow.png"><p>after</p>` +
		`<img src="` + server.URL + `/second.png"><p>done</p>`
	result := NewHTMLParser().ParseWithResult(ctx, "slow.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want only the first image to reach the network", hits.Load())
	}
	texts := make([]string, 0, len(result.JSON))
	for _, item := range result.JSON {
		if text, ok := item.ToMap()["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "after") || !strings.Contains(joined, "done") {
		t.Fatalf("text after the timed-out image was lost: %q", joined)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "document time budget") {
		t.Fatalf("missing document time-budget warning: %v", result.Warnings)
	}
}

func TestHTMLTextParserSkipsRemoteImages(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := imageTestServer(t, "remote-bytes", &hits)

	p, err := GetTextOnlyParser(utility.FileTypeHTML)
	if err != nil {
		t.Fatalf("GetTextOnlyParser: %v", err)
	}
	input := `<p>before <img alt="alt text" src="` + server.URL + `/image.png"> after</p>` +
		`<table><tr><td><img alt="cell alt" src="` + server.URL + `/cell.png"></td></tr></table>`
	result := p.ParseWithResult(context.Background(), "text.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 0 {
		t.Fatalf("text-only HTML reached the network %d time(s)", hits.Load())
	}
	var texts []string
	for _, item := range result.JSON {
		m := item.ToMap()
		if _, ok := m["image"]; ok {
			t.Fatalf("text-only HTML retained inline image bytes: %+v", m)
		}
		if _, ok := m["media_ref"]; ok {
			t.Fatalf("text-only HTML emitted a media_ref: %+v", m)
		}
		if text, ok := m["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
		if table, ok := m["table"].(*entity.TableData); ok {
			for _, row := range table.CellTexts() {
				texts = append(texts, strings.Join(row, " "))
			}
		}
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "before") || !strings.Contains(joined, "after") {
		t.Fatalf("surrounding prose lost: %q", joined)
	}
	if !strings.Contains(joined, "alt text") {
		t.Fatalf("inline image alt text lost: %q", joined)
	}
	if !strings.Contains(joined, "cell alt") {
		t.Fatalf("table image alt text lost: %q", joined)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", result.Warnings)
	}
}
