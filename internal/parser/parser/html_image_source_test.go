//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

func firstHTMLNode(t *testing.T, markup, tag string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == tag {
			found = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no <%s> in %q", tag, markup)
	}
	return found
}

func TestNormalizeRemoteImageSource(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
		ok   bool
	}{
		{"protocol relative", "//cdn.example.com/a.png", "https://cdn.example.com/a.png", true},
		{"protocol relative with space", " //cdn.example.com/a.png ", "https://cdn.example.com/a.png", true},
		{"uppercase scheme", "HTTPS://cdn.example.com/a.png", "HTTPS://cdn.example.com/a.png", true},
		{"mixed case scheme", "HtTp://cdn.example.com/a.png", "HtTp://cdn.example.com/a.png", true},
		{"http", "http://cdn.example.com/a.png", "http://cdn.example.com/a.png", true},
		{"http without host", "http://", "", false},
		{"https without host", "https://", "", false},
		{"protocol relative without host", "//", "", false},
		{"http with empty authority", "http:///path", "", false},
		{"uppercase scheme without host", "HTTPS://", "", false},
		{"ftp is not fetchable", "ftp://cdn.example.com/a.png", "", false},
		{"local reference", "images/a.png", "", false},
		{"data uri", htmlMediaDataURI, "", false},
		{"empty", "   ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeRemoteImageSource(tt.src)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("normalizeRemoteImageSource(%q) = %q, %v; want %q, %v", tt.src, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestUnsupportedRemoteImageSource(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"ftp://cdn.example.com/a.png", true},
		{"ftps://cdn.example.com/a.png", true},
		{"HTTP://cdn.example.com/a.png", false},
		{"https://cdn.example.com/a.png", false},
		{htmlMediaDataURI, false},
		{"images/a.png", false},
		{"cid:chart@example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := unsupportedRemoteImageSource(tt.src); got != tt.want {
			t.Errorf("unsupportedRemoteImageSource(%q) = %v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestFirstSrcsetCandidateKeepsEmbeddedCommas(t *testing.T) {
	const first = "https://res.cloudinary.com/demo/image/upload/w_300,h_200/sample.jpg"
	got, ok := firstSrcsetCandidate(first + " 1x, https://cdn.example.com/large.png 2x")
	if !ok || got != first {
		t.Fatalf("firstSrcsetCandidate = %q, %v; want %q, true", got, ok, first)
	}
}

func TestHTMLImageSource(t *testing.T) {
	tests := []struct {
		name   string
		markup string
		want   string
	}{
		{
			name:   "remote src",
			markup: `<img src="HTTPS://cdn.example.com/a.png">`,
			want:   "HTTPS://cdn.example.com/a.png",
		},
		{
			name:   "invalid remote src falls through to data-src",
			markup: `<img src="http://" data-src="https://cdn.example.com/real.png">`,
			want:   "https://cdn.example.com/real.png",
		},
		{
			name:   "protocol relative src",
			markup: `<img src=" //cdn.example.com/a.png ">`,
			want:   "https://cdn.example.com/a.png",
		},
		{
			name:   "lazy data-src wins over placeholder",
			markup: `<img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" data-src="//cdn.example.com/real.png">`,
			want:   "https://cdn.example.com/real.png",
		},
		{
			name:   "data-original",
			markup: `<img src="/images/blank.gif" data-original="https://cdn.example.com/original.png">`,
			want:   "https://cdn.example.com/original.png",
		},
		{
			name:   "data-lazy-src",
			markup: `<img src="/images/blank.gif" data-lazy-src="//cdn.example.com/lazy-src.png">`,
			want:   "https://cdn.example.com/lazy-src.png",
		},
		{
			name:   "data-url",
			markup: `<img src="/images/blank.gif" data-url="https://cdn.example.com/url.png">`,
			want:   "https://cdn.example.com/url.png",
		},
		{
			name:   "data-image",
			markup: `<img src="/images/blank.gif" data-image="https://cdn.example.com/image.png">`,
			want:   "https://cdn.example.com/image.png",
		},
		{
			name:   "srcset only picks the first candidate",
			markup: `<img srcset="https://cdn.example.com/small.png 1x, https://cdn.example.com/large.png 2x">`,
			want:   "https://cdn.example.com/small.png",
		},
		{
			name:   "srcset keeps commas embedded in the first URL",
			markup: `<img srcset="https://res.cloudinary.com/demo/image/upload/w_300,h_200/sample.jpg 1x, https://cdn.example.com/large.png 2x">`,
			want:   "https://res.cloudinary.com/demo/image/upload/w_300,h_200/sample.jpg",
		},
		{
			name:   "srcset skips local candidates",
			markup: `<img srcset="/small.png 1x, //cdn.example.com/large.png 2x">`,
			want:   "https://cdn.example.com/large.png",
		},
		{
			name:   "srcset data uri keeps its internal comma",
			markup: `<img srcset="data:image/png;base64,aGVsbG8= 1x, https://cdn.example.com/large.png 2x">`,
			want:   "data:image/png;base64,aGVsbG8=",
		},
		{
			name:   "data-srcset only",
			markup: `<img src="/images/blank.gif" data-srcset="//cdn.example.com/data-srcset.png 1x">`,
			want:   "https://cdn.example.com/data-srcset.png",
		},
		{
			name:   "picture source srcset",
			markup: `<picture><source srcset="//cdn.example.com/pic.png 1x"><img src="/images/blank.gif" alt="pic"></picture>`,
			want:   "https://cdn.example.com/pic.png",
		},
		{
			name:   "remote img src wins over picture source",
			markup: `<picture><source srcset="//cdn.example.com/pic.png"><img src="HTTPS://cdn.example.com/main.png"></picture>`,
			want:   "HTTPS://cdn.example.com/main.png",
		},
		{
			name:   "data uri src fallback",
			markup: `<img src=" data:image/png;base64,aGVsbG8=">`,
			want:   " data:image/png;base64,aGVsbG8=",
		},
		{
			name:   "local src fallback",
			markup: `<img src="images/local.png">`,
			want:   "images/local.png",
		},
		{
			name:   "missing src",
			markup: `<img alt="no source">`,
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := firstHTMLNode(t, tt.markup, "img")
			if got := htmlImageSource(img); got != tt.want {
				t.Fatalf("htmlImageSource = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHTMLParser_FetchesRemoteImageFromUppercaseScheme(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	const body = "\x89PNG\r\n\x1a\nuppercase-bytes"
	server := imageTestServer(t, body, &hits)
	ctx, cache := recordingMediaContext()
	upper := strings.Replace(server.URL, "http://", "HTTP://", 1)

	result := NewHTMLParser().ParseWithResult(ctx, "upper.html", []byte(`<img alt="upper" src="`+upper+`/image.png">`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != body {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	if !hasImageItem(result) {
		t.Fatalf("no image item emitted: %+v", ItemsToMaps(result.JSON))
	}
}

func TestHTMLParser_FetchesSrcsetOnlyImage(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	const body = "\x89PNG\r\n\x1a\nsrcset-bytes"
	server := imageTestServer(t, body, &hits)
	ctx, cache := recordingMediaContext()
	srcset := server.URL + "/small.png 1x, " + server.URL + "/large.png 2x"

	result := NewHTMLParser().ParseWithResult(ctx, "srcset.html", []byte(`<img srcset="`+srcset+`">`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != body {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	if !hasImageItem(result) {
		t.Fatalf("no image item emitted: %+v", ItemsToMaps(result.JSON))
	}
}

func TestHTMLParser_FetchesLazyDataSourceOverPlaceholder(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	const body = "\x89PNG\r\n\x1a\nlazy-bytes"
	server := imageTestServer(t, body, &hits)
	ctx, cache := recordingMediaContext()

	input := `<img alt="lazy" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" data-src="` + server.URL + `/lazy.png">`
	result := NewHTMLParser().ParseWithResult(ctx, "lazy.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != body {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	if !hasImageItem(result) {
		t.Fatalf("no image item emitted: %+v", ItemsToMaps(result.JSON))
	}
}

func TestHTMLParser_FetchesPictureSourceSrcset(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	const body = "\x89PNG\r\n\x1a\npicture-bytes"
	server := imageTestServer(t, body, &hits)
	ctx, cache := recordingMediaContext()

	input := `<picture><source srcset="` + server.URL + `/picture.png 1x"><img src="/images/blank.gif" alt="picture"></picture>`
	result := NewHTMLParser().ParseWithResult(ctx, "picture.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != body {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	if !hasImageItem(result) {
		t.Fatalf("no image item emitted: %+v", ItemsToMaps(result.JSON))
	}
}

func TestHTMLParser_WarnsOnUnsupportedRemoteImageScheme(t *testing.T) {
	ctx, cache := recordingMediaContext()
	input := `<p>before</p><img alt="ftp chart" src="ftp://cdn.example.com/chart.png">` +
		`<table><tr><td><img alt="ftp cell" src="ftp://cdn.example.com/cell.png"></td></tr></table><p>after</p>`

	result := NewHTMLParser().ParseWithResult(ctx, "ftp.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if hasImageItem(result) {
		t.Fatalf("image item emitted for an unsupported scheme: %+v", ItemsToMaps(result.JSON))
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "unsupported remote source") {
		t.Fatalf("missing unsupported-scheme warning: %v", result.Warnings)
	}
	if !strings.Contains(warnings, "2 image(s)") {
		t.Fatalf("warning did not count both images: %v", result.Warnings)
	}
	text := itemsText(result)
	if !strings.Contains(text, "ftp chart") || !strings.Contains(text, "ftp cell") {
		t.Fatalf("skipped image alt text lost: %q", text)
	}
	if strings.Contains(text, "ftp://") {
		t.Fatalf("unsupported remote URL leaked into text: %q", text)
	}
}

func TestHTMLParser_SkippedRemoteImageKeepsAltText(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	ctx, _ := recordingMediaContext()
	input := `<p>before</p><img alt="missing chart" src="` + server.URL + `/missing.png">` +
		`<table><tr><td><img alt="missing cell" src="` + server.URL + `/cell.png"></td></tr></table>`

	result := NewHTMLParser().ParseWithResult(ctx, "missing.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 2 {
		t.Fatalf("remote fetches = %d, want 2", hits.Load())
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "could not be downloaded") {
		t.Fatalf("missing download-failure warning: %v", result.Warnings)
	}
	text := itemsText(result)
	if !strings.Contains(text, "missing chart") || !strings.Contains(text, "missing cell") {
		t.Fatalf("skipped image alt text lost: %q", text)
	}
}

func TestHTMLTextParserSkipsRemoteImageCandidatesWithoutNetwork(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := imageTestServer(t, "remote-bytes", &hits)

	p, err := GetTextOnlyParser(utility.FileTypeHTML)
	if err != nil {
		t.Fatalf("GetTextOnlyParser: %v", err)
	}
	host := strings.TrimPrefix(server.URL, "http://")
	input := `<p>before</p>` +
		`<img alt="protocol relative" src="//` + host + `/proto.png">` +
		`<img alt="uppercase" src="HTTP://` + host + `/upper.png">` +
		`<img alt="lazy" src="/placeholder.gif" data-src="` + server.URL + `/lazy.png">` +
		`<img alt="srcset" srcset="` + server.URL + `/set.png 1x">` +
		`<picture><source srcset="` + server.URL + `/picture.png"><img alt="picture" src="/blank.gif"></picture>` +
		`<table><tr><td><img alt="cell" src="` + server.URL + `/cell.png"></td></tr></table>` +
		`<p>after</p>`

	result := p.ParseWithResult(t.Context(), "text.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if hits.Load() != 0 {
		t.Fatalf("text-only HTML reached the network %d time(s)", hits.Load())
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", result.Warnings)
	}
	text := itemsText(result)
	for _, want := range []string{"before", "after", "protocol relative", "uppercase", "lazy", "srcset", "picture", "cell"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text-only output lost %q: %q", want, text)
		}
	}
	for _, item := range result.JSON {
		m := item.ToMap()
		if _, ok := m["media_ref"]; ok {
			t.Fatalf("text-only HTML emitted a media_ref: %+v", m)
		}
		if _, ok := m["image"]; ok {
			t.Fatalf("text-only HTML retained inline image bytes: %+v", m)
		}
	}
}

func hasImageItem(result ParseResult) bool {
	for _, item := range result.JSON {
		if item.ToMap()["doc_type_kwd"] == DocTypeImage &&
			entity.MediaRefFromAny(item.ToMap()["media_ref"]) != nil {
			return true
		}
	}
	return false
}
