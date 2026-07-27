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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
)

func TestWikipedia_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		lang     string
		query    string
		max      int
		wantHost string
		wantPath string
	}{
		{
			name:     "en default",
			lang:     "",
			query:    "rag flow",
			max:      0,
			wantHost: "en.wikipedia.org",
			wantPath: "/w/api.php",
		},
		{
			name:     "de explicit",
			lang:     "de",
			query:    "Berlin",
			max:      3,
			wantHost: "de.wikipedia.org",
			wantPath: "/w/api.php",
		},
		{
			name:     "spaces encoded",
			lang:     "en",
			query:    "a b c",
			max:      1,
			wantHost: "en.wikipedia.org",
			wantPath: "/w/api.php",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildWikipediaURL(tc.lang, tc.query, tc.max)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", u.Path, tc.wantPath)
			}
			q := u.Query()
			if q.Get("action") != "query" {
				t.Errorf("action = %q, want query", q.Get("action"))
			}
			if q.Get("generator") != "search" {
				t.Errorf("generator = %q, want search", q.Get("generator"))
			}
			if q.Get("format") != "json" {
				t.Errorf("format = %q, want json", q.Get("format"))
			}
			if q.Get("gsrsearch") != tc.query {
				t.Errorf("gsrsearch = %q, want %q (raw query, not pre-encoded)", q.Get("gsrsearch"), tc.query)
			}
		})
	}
}

func TestWikipedia_ParseResults(t *testing.T) {
	t.Parallel()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"query": {
				"pages": {
					"11": {"index":2,"title":"Retrieval-augmented generation","extract":"RAG is a generation technique.","fullurl":"https://en.wikipedia.org/wiki/Retrieval-augmented_generation"},
					"10": {"index":1,"title":"RAG","extract":"RAG is an acronym.","fullurl":"https://en.wikipedia.org/wiki/RAG"}
				}
			}
		}`))
	}))
	defer srv.Close()

	// Point the hard-coded en.wikipedia.org endpoint at the test server
	// by injecting a transport that rewrites the request host.
	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewWikipediaToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"query":"RAG","lang":"en","max_results":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(gotUA, "ragflow") {
		t.Errorf("User-Agent = %q, want to contain ragflow", gotUA)
	}

	var env wikipediaEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].Title != "RAG" {
		t.Errorf("Results[0].Title = %q, want RAG", env.Results[0].Title)
	}
	if env.Results[0].Content != "RAG is an acronym." {
		t.Errorf("Results[0].Content = %q, want summary extract", env.Results[0].Content)
	}
	if !strings.Contains(env.FormalizedContent, "RAG is an acronym.") {
		t.Errorf("FormalizedContent = %q, want rendered summary", env.FormalizedContent)
	}
	if !strings.HasPrefix(env.Results[0].URL, "https://en.wikipedia.org/wiki/") {
		t.Errorf("Results[0].URL = %q, want to start with https://en.wikipedia.org/wiki/", env.Results[0].URL)
	}
}

// rewriteHostTransport returns a RoundTripper that rewrites the request
// host to the given test server URL. Used to point the hard-coded
// en.wikipedia.org endpoint at a httptest.Server without changing the
// production URL builder.
func rewriteHostTransport(srvURL string) http.RoundTripper {
	u, err := url.Parse(srvURL)
	if err != nil {
		panic("rewriteHostTransport: bad srvURL: " + err.Error())
	}
	return &hostSwapRT{inner: http.DefaultTransport, host: u.Host, scheme: u.Scheme}
}

type hostSwapRT struct {
	inner  http.RoundTripper
	host   string
	scheme string
}

func (t *hostSwapRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Scheme = t.scheme
	r2.URL.Host = t.host
	r2.Host = t.host
	return t.inner.RoundTrip(r2)
}

func TestWikipedia_ToolMeta(t *testing.T) {
	t.Parallel()

	tool := NewWikipediaTool()
	meta := tool.ToolMeta()
	if meta.Name != "wikipedia_search" {
		t.Errorf("Name = %q, want wikipedia_search", meta.Name)
	}
	if !strings.Contains(meta.Description, "Wikipedia") {
		t.Errorf("Description = %q, want to mention Wikipedia", meta.Description)
	}
}

func TestWikipedia_EmptyQuery(t *testing.T) {
	t.Parallel()

	tool := NewWikipediaTool()
	out, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun(empty): %v", err)
	}
	var envelope wikipediaEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || len(envelope.Results) != 0 {
		t.Fatalf("empty result = %s / %v", out, err)
	}
	if !strings.Contains(out, `"results":[]`) {
		t.Fatalf("empty result omitted results key: %s", out)
	}
}

func TestWikipedia_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()

	wikipedia := NewWikipediaTool()
	spec := wikipedia.ComponentSpec()
	if query, ok := spec.InputForm["query"].(map[string]any); !ok || query["name"] != "Query" || query["type"] != "line" {
		t.Fatalf("query input form = %#v", spec.InputForm["query"])
	}
	envelope := map[string]any{"results": []any{map[string]any{
		"title":   "RAG",
		"url":     "https://en.wikipedia.org/wiki/RAG",
		"content": "RAG is an acronym.",
	}}}
	chunks, docAggs := wikipedia.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 || chunks[0]["document_name"] != "RAG" || chunks[0]["similarity"] != 1 {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	outputs := wikipedia.BuildComponentOutputs(envelope)
	if results, ok := outputs["json"].([]any); !ok || len(results) != 1 {
		t.Fatalf("json output = %#v", outputs["json"])
	}
	if !strings.Contains(outputs["formalized_content"].(string), "RAG is an acronym.") {
		t.Fatalf("formalized_content = %q", outputs["formalized_content"])
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("output conversion mutated envelope: %#v", envelope)
	}
}

func TestRenderWikipediaReferencesStopsBeforeOverBudgetBlock(t *testing.T) {
	t.Parallel()

	chunks := []map[string]any{
		{"id": "1", "document_name": "First", "url": "https://first.example", "content": "first reference content"},
		{"id": "2", "document_name": "Second", "url": "https://second.example", "content": "second reference content"},
	}
	firstBlock := renderWikipediaReferences(chunks[:1], 0)
	firstTokens := tokenizer.NumTokensFromString(firstBlock)
	maxTokens := (firstTokens*100 + 96) / 97
	if got := renderWikipediaReferences(chunks, maxTokens); got != firstBlock {
		t.Fatalf("rendered = %q, want only first block %q", got, firstBlock)
	}
	if got := renderWikipediaReferences(chunks, 1); got != "" {
		t.Fatalf("over-budget first block was appended: %q", got)
	}
	if got := renderWikipediaReferences(chunks, 0); !strings.Contains(got, "Title: First") || !strings.Contains(got, "Title: Second") {
		t.Fatalf("unlimited rendering dropped blocks: %q", got)
	}
}

func TestWikipedia_BuildByNameIgnoresCanvasParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("wikipedia", map[string]any{
		"top_n":    float64(3),
		"language": "en",
		"outputs":  map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	wikipedia := built.(*WikipediaTool)
	if wikipedia.topN != 3 || wikipedia.lang != "en" {
		t.Fatalf("node defaults = %d/%q", wikipedia.topN, wikipedia.lang)
	}
}
