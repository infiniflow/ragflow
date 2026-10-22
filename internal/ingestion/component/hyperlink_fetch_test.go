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
	"strings"
	"testing"
	"time"

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
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("see [x](https://example.com/x)")
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, defaultSetups(), "doc1")
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

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("see [page](" + srv.URL + "/p)")
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, setups, "doc1")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) <= len(parent) {
		t.Fatalf("len = %d, want more than parent %d", len(got), len(parent))
	}
	// parent first, tagged children after
	if got[0].ToMap()["text"] != "parent body" {
		t.Fatalf("parent moved: %v", got[0].ToMap())
	}
	child := got[len(got)-1].ToMap()
	extra, _ := child["extra"].(map[string]any)
	if extra[hyperlinkSourceURLKey] == nil || extra[hyperlinkParentDocKey] != "doc1" {
		t.Fatalf("child provenance missing from extra: %v", child)
	}
	if _, topLevel := child[hyperlinkSourceURLKey]; topLevel {
		t.Fatalf("provenance must not be a top-level key: %v", child)
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

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("a [b](" + srv.URL + "/bin) c [d](" + srv.URL + "/missing)")
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, setups, "doc1")
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

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[p](" + srv.URL + ")")
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_bytes_per_url": 8,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, setups, "doc1")
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
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[a](http://127.0.0.1:1/a) [b](http://127.0.0.1:1/b)")
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_urls": 1,
	})
	_, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, setups, "doc1")
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want truncation + fetch failure", warnings)
	}
}

func TestCheckRejectsMisplacedHyperlinkKey(t *testing.T) {
	if _, err := NewParserComponent(map[string]any{"html": map[string]any{"fetch_links": true}}); err == nil {
		t.Fatal("want error when a family without hyperlink support carries fetch_links")
	}
	if _, err := NewParserComponent(map[string]any{"pdf": map[string]any{"collect_markdown_links": true}}); err == nil {
		t.Fatal("want error for collect_markdown_links outside markdown")
	}
}

func TestCheckRejectsInvalidHyperlinkBudget(t *testing.T) {
	if _, err := NewParserComponent(map[string]any{"pdf": map[string]any{"fetch_links": true, "max_urls": 0}}); err == nil {
		t.Fatal("want error for out-of-range max_urls")
	}
	if _, err := NewParserComponent(map[string]any{"pdf": map[string]any{"fetch_links": "yes"}}); err == nil {
		t.Fatal("want error for non-boolean fetch_links")
	}
}

func TestHyperlinkDocxEndToEnd(t *testing.T) {
	allowTestHosts(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><p>docx linked page</p></body></html>"))
	}))
	defer srv.Close()

	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="` + srv.URL + `"/>` +
		`</Relationships>`
	binary := buildDocxWithRels(t, rels)
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	setups := hyperlinkTestSetups(t, "docx", map[string]any{"fetch_links": true})

	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeDOCX,
		"doc.docx", binary, parent, setups, "doc-42")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want parent + 1 linked item", len(got))
	}
	child := got[1].ToMap()
	extra, _ := child["extra"].(map[string]any)
	if extra[hyperlinkParentDocKey] != "doc-42" || extra[hyperlinkSourceURLKey] != srv.URL {
		t.Fatalf("child provenance missing from extra: %v", child)
	}
}

func TestHyperlinkCancelledContextSkipsFetch(t *testing.T) {
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[p](https://example.com/p)")
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, warnings := maybeFetchHyperlinkContents(ctx, utility.FileTypeMarkdown,
		"doc.md", binary, parent, setups, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 || warnings[0] != "hyperlink: fetch cancelled; remaining links skipped" {
		t.Fatalf("warnings = %v, want cancellation warning", warnings)
	}
}

func TestHyperlinkRemainingBudgetCapsRead(t *testing.T) {
	allowTestHosts(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>body over remaining budget</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[a](" + srv.URL + ")")
	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
		"max_bytes_per_url": 1 << 20, "max_total_bytes": 32,
	})
	got, warnings := maybeFetchHyperlinkContents(context.Background(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, setups, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "over 32 byte limit") {
		t.Fatalf("warnings = %v, want rejection at the remaining 32-byte budget", warnings)
	}
}

func TestHyperlinkOverallDeadline(t *testing.T) {
	base := schema.HyperlinkFetchParam{MaxURLs: 5, TimeoutSeconds: 10}
	if got := hyperlinkOverallDeadline(base); got != 50*time.Second {
		t.Fatalf("deadline = %v, want 50s", got)
	}
	maxed := schema.HyperlinkFetchParam{MaxURLs: 50, TimeoutSeconds: 120}
	if got := hyperlinkOverallDeadline(maxed); got != 5*time.Minute {
		t.Fatalf("deadline = %v, want the 5m cap", got)
	}
}

func TestParserInvokeFailsWhenCancelledDuringFetch(t *testing.T) {
	allowTestHosts(t)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>late</p></body></html>"))
	}))
	defer slow.Close()

	setups := hyperlinkTestSetups(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	params := map[string]any{}
	for family, setup := range setups {
		params[family] = map[string]any(setup)
	}
	comp, err := NewParserComponent(params)
	if err != nil {
		t.Fatalf("NewParserComponent: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	out, err := comp.Invoke(ctx, nil, map[string]any{
		"binary":    []byte("[p](" + slow.URL + ")"),
		"name":      "doc.md",
		"file_type": "md",
		"doc_id":    "doc1",
	})
	if err == nil {
		t.Fatalf("want cancellation error from Invoke, out=%v ctxErr=%v setup=%v", out, ctx.Err(), comp.(*ParserComponent).setups["markdown"])
	}
}
