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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// hyperlinkTestSetups returns the default Parser setups with `overrides`
// applied to one family.
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

// newHyperlinkComponent builds a ParserComponent the way the canvas does, so the
// post-step reads budgets that Check validated rather than a hand-fed map.
func newHyperlinkComponent(t *testing.T, family string, overrides map[string]any) *ParserComponent {
	t.Helper()
	params := map[string]any{}
	for name, setup := range hyperlinkTestSetups(t, family, overrides) {
		params[name] = map[string]any(setup)
	}
	comp, err := NewParserComponent(params)
	if err != nil {
		t.Fatalf("NewParserComponent: %v", err)
	}
	return comp.(*ParserComponent)
}

func TestHyperlinkDisabledByDefault(t *testing.T) {
	c := newHyperlinkComponent(t, "markdown", nil)
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("see [x](https://example.com/x)")
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want %d (no fetch when disabled)", len(got), len(parent))
	}
}

func TestHyperlinkMarkdownFetchAppendsTaggedItems(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>linked page body text</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("see [page](" + srv.URL + "/p)")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
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
	withSSRFBypass(t)
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
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only %d", len(got), len(parent))
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", warnings)
	}
}

func TestHyperlinkOverSizeSkipped(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>0123456789abcdef</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[p](" + srv.URL + ")")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_bytes_per_url": 8,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", warnings)
	}
}

func TestHyperlinkTruncationWarns(t *testing.T) {
	withoutSSRFBypass(t)
	// Loopback is rejected by the SSRF guard without touching the network,
	// so both the truncation and the single kept-URL failure are hermetic.
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[a](http://127.0.0.1:1/a) [b](http://127.0.0.1:1/b)")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_urls": 1,
	})
	_, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want truncation + fetch failure", warnings)
	}
}

// A document's links are author-controlled text, so a destination that resolves
// to a non-public address must be refused before the connection is made:
// validating the host and then dialing a freshly resolved address would let one
// name answer publicly on the check and internally on the dial.
func TestHyperlinkRefusesNonPublicDestination(t *testing.T) {
	withoutSSRFBypass(t)
	var reached atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>internal page</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[p](" + srv.URL + ")")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if n := reached.Load(); n != 0 {
		t.Fatalf("the refused destination received %d requests", n)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "non-public address") {
		t.Fatalf("warnings = %v, want the SSRF rejection", warnings)
	}
}

func TestHyperlinkDocxEndToEnd(t *testing.T) {
	withSSRFBypass(t)
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
	c := newHyperlinkComponent(t, "docx", map[string]any{"fetch_links": true})

	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeDOCX,
		"doc.docx", binary, parent, "doc-42")
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
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, warnings := c.fetchHyperlinkedContent(ctx, utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 || warnings[0] != "hyperlink: run cancelled; remaining links skipped" {
		t.Fatalf("warnings = %v, want the cancellation warning", warnings)
	}
}

func TestHyperlinkRemainingBudgetCapsRead(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>body over remaining budget</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[a](" + srv.URL + ")")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
		"max_bytes_per_url": 1 << 20, "max_total_bytes": 32,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "over 32 byte limit") {
		t.Fatalf("warnings = %v, want rejection at the remaining 32-byte budget", warnings)
	}
}

func TestHyperlinkDeduplicatesRedirectTargets(t *testing.T) {
	withSSRFBypass(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>redirect target</p></body></html>"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[a](" + srv.URL + "/a) [b](" + srv.URL + "/b)")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) != len(parent)+1 {
		t.Fatalf("len = %d, want only one copy of the redirect target", len(got))
	}
}

func TestHyperlinkChargesRejectedResponsesToBudget(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>" + strings.Repeat("x", 200) + "</p></body></html>"))
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[a](" + srv.URL + "/a) [b](" + srv.URL + "/b)")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
		"max_bytes_per_url": 1 << 20, "max_total_bytes": 64,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	// First page rejected at the 64-byte cap but its bytes are charged, so
	// the second URL never gets a fresh budget.
	if len(warnings) != 2 || !strings.Contains(warnings[0], "byte limit") ||
		!strings.Contains(warnings[1], "budget exhausted") {
		t.Fatalf("warnings = %v, want reject then budget exhaustion", warnings)
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
	withSSRFBypass(t)
	started := make(chan struct{})
	var signalOnce sync.Once
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signalOnce.Do(func() { close(started) })
		<-r.Context().Done()
	}))
	defer slow.Close()

	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := c.Invoke(ctx, nil, map[string]any{
			"binary":    []byte("[p](" + slow.URL + ")"),
			"name":      "doc.md",
			"file_type": "md",
			"doc_id":    "doc1",
		})
		errCh <- err
	}()

	<-started
	cancel()

	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// The markdown contract is two switches: fetching without collecting links has
// nothing to fetch, and must say nothing rather than half-work.
func TestHyperlinkMarkdownCollectSwitchOffSkipsFetch(t *testing.T) {
	withSSRFBypass(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>unseen</p></body></html>"))
	}))
	defer srv.Close()

	c := newHyperlinkComponent(t, "markdown", map[string]any{"fetch_links": true})
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", []byte("[p]("+srv.URL+"/p)"), parent, "doc1")
	if len(got) != len(parent) || atomic.LoadInt32(&hits) != 0 || len(warnings) != 0 {
		t.Fatalf("got=%d hits=%d warnings=%v, want parent only, no request, no warning", len(got), hits, warnings)
	}
}

// The PDF family has to reach the fetch stage through its own collector, not
// just the markdown/docx ones.
func TestHyperlinkPDFFetchEndToEnd(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>pdf linked page</p></body></html>"))
	}))
	defer srv.Close()

	c := newHyperlinkComponent(t, "pdf", map[string]any{"fetch_links": true})
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("%PDF-1.7 <</S/URI/URI (" + srv.URL + "/p)>>")
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypePDF,
		"doc.pdf", binary, parent, "doc1")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(got) <= len(parent) {
		t.Fatalf("len = %d, want linked items appended after the parent", len(got))
	}
	extra, _ := got[len(got)-1].ToMap()["extra"].(map[string]any)
	if extra[hyperlinkSourceURLKey] != srv.URL+"/p" {
		t.Fatalf("source_url = %#v, want the fetched page", extra[hyperlinkSourceURLKey])
	}
}

// Without a document id the provenance falls back to the filename, so an item
// is never attributed to the empty string.
func TestHyperlinkParentIDFallsBackToFilename(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>linked</p></body></html>"))
	}))
	defer srv.Close()

	c := newHyperlinkComponent(t, "markdown", map[string]any{"fetch_links": true, "collect_markdown_links": true})
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", []byte("[p]("+srv.URL+"/p)"), parent, "")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	extra, _ := got[len(got)-1].ToMap()["extra"].(map[string]any)
	if extra[hyperlinkParentDocKey] != "doc.md" {
		t.Fatalf("parent_doc_id = %#v, want the filename fallback", extra[hyperlinkParentDocKey])
	}
}

// A server that omits Content-Type still yields a page: the guard only rejects
// content types it can positively identify as non-HTML.
func TestHyperlinkFetchesResponseWithoutContentType(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>typeless page</p></body></html>"))
	}))
	defer srv.Close()

	c := newHyperlinkComponent(t, "markdown", map[string]any{"fetch_links": true, "collect_markdown_links": true})
	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", []byte("[p]("+srv.URL+"/p)"), parent, "doc1")
	if len(warnings) != 0 || len(got) <= len(parent) {
		t.Fatalf("warnings = %v len = %d, want the page indexed", warnings, len(got))
	}
}

// Invoke is the production entry point: the post-step's warnings and its run
// summary have to reach the progress sink or an operator sees nothing.
func TestParserInvokeReportsLinkedContentProgress(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not a page"))
	}))
	defer srv.Close()

	c := newHyperlinkComponent(t, "markdown", map[string]any{"fetch_links": true, "collect_markdown_links": true})
	var messages []string
	ctx := runtime.WithProgressMessageCallback(t.Context(), func(_, message string) {
		messages = append(messages, message)
	})
	if _, err := c.Invoke(ctx, nil, map[string]any{
		"binary":    []byte("[a](" + srv.URL + "/a) [b](" + srv.URL + "/b)"),
		"name":      "doc.md",
		"file_type": "md",
		"doc_id":    "doc1",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var sawSkip, sawSummary bool
	for _, m := range messages {
		if strings.Contains(m, "is not a page") {
			sawSkip = true
		}
		if strings.Contains(m, "linked content: 0 of 2 links fetched") {
			sawSummary = true
		}
	}
	if !sawSkip || !sawSummary {
		t.Fatalf("messages = %v, want the per-URL skip warning and the run summary", messages)
	}
}

// The redirect chain is followed by hand so every hop is validated, which means
// the loop needs its own bound: a server that redirects to itself must stop with
// a warning instead of spinning until the document deadline.
func TestHyperlinkStopsAtTheRedirectLimit(t *testing.T) {
	withSSRFBypass(t)
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[loop](" + srv.URL + "/loop)")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if n := hits.Load(); n != hyperlinkMaxRedirects {
		t.Fatalf("requests = %d, want the chain stopped at %d", n, hyperlinkMaxRedirects)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "too many redirects") {
		t.Fatalf("warnings = %v, want the redirect-limit warning", warnings)
	}
}

// A redirect target is a new destination, not a continuation of the one already
// cleared: it has to pass the same guard, or a public page could hand the worker
// a URL the guard would never accept.
func TestHyperlinkValidatesEachRedirectHop(t *testing.T) {
	withSSRFBypass(t)
	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/jump", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[jump](" + srv.URL + "/jump)")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
	})
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "disallowed URL scheme") {
		t.Fatalf("warnings = %v, want the per-hop scheme rejection", warnings)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("requests = %d, want only the hop that redirected", n)
	}
}

// timeout_s is per link, not per document: one unresponsive host must not be
// able to occupy the run until the document deadline.
func TestHyperlinkPerURLTimeoutExpires(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	parent := []parser.DocItem{parser.TextItem{Text: "parent body"}}
	binary := []byte("[slow](" + srv.URL + ")")
	c := newHyperlinkComponent(t, "markdown", map[string]any{
		"fetch_links": true, "collect_markdown_links": true,
		"timeout_s": 1, "max_urls": 5,
	})
	started := time.Now()
	got, warnings := c.fetchHyperlinkedContent(t.Context(), utility.FileTypeMarkdown,
		"doc.md", binary, parent, "doc1")
	if len(got) != len(parent) {
		t.Fatalf("len = %d, want parent only", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "fetch") {
		t.Fatalf("warnings = %v, want the per-URL timeout warning", warnings)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("elapsed = %v, want the link abandoned at its 1s timeout", elapsed)
	}
}
