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

package component

import (
	"strings"
	"testing"
)

// TestRender_PlainDefault: empty / unknown format falls back to
// plain text, with the body returned verbatim.
func TestRender_PlainDefault(t *testing.T) {
	got := Render(RenderRequest{Format: OutputFormatEmpty, Text: "hello"})
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// TestRender_HTMLBodyAndDownloads: html output wraps the body in
// a <div class="rf-message"> and renders downloads as <a> links.
func TestRender_HTMLBodyAndDownloads(t *testing.T) {
	got := Render(RenderRequest{
		Format: OutputFormatHTML,
		Text:   "body",
		Downloads: []DownloadInfo{
			{DocID: "d1", Filename: "report.csv", MimeType: "text/csv", URL: "/dl/d1"},
		},
	})
	if !strings.Contains(got, `<div class="rf-message">body`) {
		t.Errorf("missing wrapper: %q", got)
	}
	if !strings.Contains(got, `href="/dl/d1"`) {
		t.Errorf("missing href: %q", got)
	}
	if !strings.Contains(got, `download="report.csv"`) {
		t.Errorf("missing download attr: %q", got)
	}
}

// TestRender_HTMLEscapesBody: the body is HTML-escaped so a
// user-provided "<script>" tag does not inject HTML.
func TestRender_HTMLEscapesBody(t *testing.T) {
	got := Render(RenderRequest{Format: OutputFormatHTML, Text: "<script>alert(1)</script>"})
	if strings.Contains(got, "<script>") {
		t.Errorf("body not escaped: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("missing escaped tag: %q", got)
	}
}

// TestRender_Markdown: markdown passthrough + markdown link list
// for downloads.
func TestRender_Markdown(t *testing.T) {
	got := Render(RenderRequest{
		Format: OutputFormatMarkdown,
		Text:   "**bold** body",
		Downloads: []DownloadInfo{
			{URL: "/x", Filename: "x.txt"},
		},
	})
	if !strings.Contains(got, "**bold** body") {
		t.Errorf("markdown body not preserved: %q", got)
	}
	if !strings.Contains(got, "- [x.txt](/x)") {
		t.Errorf("missing markdown link: %q", got)
	}
}

// TestRender_PlainNoDownloads: plain format with no downloads
// returns the body verbatim (no wrapper, no extra newlines).
func TestRender_PlainNoDownloads(t *testing.T) {
	got := Render(RenderRequest{Format: OutputFormatPlain, Text: "no dl"})
	if got != "no dl" {
		t.Errorf("got %q, want %q", got, "no dl")
	}
}

// TestIsDownloadInfo_True: a map carrying the three canonical
// keys is a download descriptor.
func TestIsDownloadInfo_True(t *testing.T) {
	v := map[string]any{
		"doc_id":    "d1",
		"filename":  "f.txt",
		"mime_type": "text/plain",
	}
	if !IsDownloadInfo(v) {
		t.Errorf("expected download info, got false")
	}
}

// TestIsDownloadInfo_MissingKey: missing any canonical key is
// not a download descriptor.
func TestIsDownloadInfo_MissingKey(t *testing.T) {
	cases := []map[string]any{
		{"filename": "f", "mime_type": "text/plain"},
		{"doc_id": "d", "mime_type": "text/plain"},
		{"doc_id": "d", "filename": "f"},
	}
	for i, c := range cases {
		if IsDownloadInfo(c) {
			t.Errorf("case %d should not be download info", i)
		}
	}
}

// TestIsDownloadInfo_NonMap: a non-map value is not a download.
func TestIsDownloadInfo_NonMap(t *testing.T) {
	if IsDownloadInfo("not a map") {
		t.Errorf("string should not be download info")
	}
	if IsDownloadInfo(nil) {
		t.Errorf("nil should not be download info")
	}
}

// TestExtractDownloads_FromMap: a single download descriptor map
// is returned as a one-element slice.
func TestExtractDownloads_FromMap(t *testing.T) {
	v := map[string]any{
		"doc_id":    "d1",
		"filename":  "f.txt",
		"mime_type": "text/plain",
		"url":       "/dl/d1",
	}
	dls := ExtractDownloads(v)
	if len(dls) != 1 {
		t.Fatalf("expected 1 download, got %d", len(dls))
	}
	if dls[0].DocID != "d1" || dls[0].Filename != "f.txt" || dls[0].URL != "/dl/d1" {
		t.Errorf("unexpected download: %+v", dls[0])
	}
}

// TestExtractDownloads_FromList: a list of descriptors is
// flattened into a slice.
func TestExtractDownloads_FromList(t *testing.T) {
	v := []any{
		map[string]any{"doc_id": "d1", "filename": "a", "mime_type": "x"},
		map[string]any{"doc_id": "d2", "filename": "b", "mime_type": "y"},
	}
	dls := ExtractDownloads(v)
	if len(dls) != 2 {
		t.Fatalf("expected 2 downloads, got %d", len(dls))
	}
	if dls[0].DocID != "d1" || dls[1].DocID != "d2" {
		t.Errorf("unexpected downloads: %+v", dls)
	}
}

// TestExtractDownloads_Empty: a plain string returns no downloads.
func TestExtractDownloads_Empty(t *testing.T) {
	if dls := ExtractDownloads("just text"); len(dls) != 0 {
		t.Errorf("expected 0 downloads, got %d", len(dls))
	}
}

// TestExtractDownloads_NestedList: nested list-of-lists is
// flattened (the recursive walk).
func TestExtractDownloads_NestedList(t *testing.T) {
	v := []any{
		[]any{
			map[string]any{"doc_id": "d1", "filename": "a", "mime_type": "x"},
		},
		map[string]any{"doc_id": "d2", "filename": "b", "mime_type": "y"},
	}
	dls := ExtractDownloads(v)
	if len(dls) != 2 {
		t.Fatalf("expected 2, got %d", len(dls))
	}
}

// TestStripHTMLTags: best-effort tag stripper.
func TestStripHTMLTags(t *testing.T) {
	got := StripHTMLTags(`<div class="x">hello <b>world</b></div>`)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("tags not stripped: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("body not preserved: %q", got)
	}
}
