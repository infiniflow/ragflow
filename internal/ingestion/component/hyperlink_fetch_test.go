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

package component

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

func hyperlinkTestSetups(t *testing.T, family string, overrides map[string]any) map[string]schema.ParserSetup {
	t.Helper()
	setups := defaultSetups()
	base := map[string]any{}
	for k, v := range setups[family] {
		base[k] = v
	}
	for k, v := range overrides {
		base[k] = v
	}
	setups[family] = base
	return setups
}

func allowTestHosts(t *testing.T) {
	t.Helper()
	prev := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true
	t.Cleanup(func() { common.AllowAnyHostForTest = prev })
}

func TestHyperlinkDisabledByDefault(t *testing.T) {
	parent := []parser.DocItem{parser.TextItem{Text: "see [x](https://example.com/x)"}}
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", nil, parent, defaultSetups(), "English", "doc1")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want %d (no fetch when disabled)", len(got), len(parent))
	}
}

func TestHyperlinkMarkdownFetchAppendsTaggedItems(t *testing.T) {
	allowTestHosts(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>linked page body text</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "see [page](" + srv.URL + "/p)"}}
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", nil, parent, setups, "English", "doc1")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) <= len(parent) {
		t.Fatalf("len = %d, want more than parent %d", len(got), len(parent))
	}
	// parent first, tagged children after
	if got[0].ToMap()["text"] != "see [page]("+srv.URL+"/p)" {
		t.Fatalf("parent moved: %v", got[0].ToMap())
	}
	child := got[len(got)-1].ToMap()
	if child["source_url"] == nil || child["parent_doc_id"] != "doc1" {
		t.Fatalf("child untagged: %v", child)
	}
}

func TestHyperlinkSkipsNonHTMLAndFailures(t *testing.T) {
	allowTestHosts(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bin" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0, 1, 2})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "a [b](" + srv.URL + "/bin) c [d](" + srv.URL + "/missing)"}}
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", nil, parent, setups, "English", "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only %d", len(got), len(parent))
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", warnings)
	}
}

func TestHyperlinkOverSizeSkipped(t *testing.T) {
	allowTestHosts(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>0123456789abcdef</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "[p](" + srv.URL + ")"}}
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_bytes_per_url": 8,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", nil, parent, setups, "English", "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", warnings)
	}
}

func TestHyperlinkTruncationWarns(t *testing.T) {
	// Loopback is rejected by the SSRF guard without touching the network,
	// so both the truncation and the single kept-URL failure are hermetic.
	parent := []parser.DocItem{parser.TextItem{Text: "[a](http://127.0.0.1:1/a) [b](http://127.0.0.1:1/b)"}}
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_urls": 1,
	})
	_, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", nil, parent, setups, "English", "doc1")
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want truncation + fetch failure", warnings)
	}
}
