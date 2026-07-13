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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	agenttool "ragflow/internal/agent/tool"
)

type arxivRoundTripper func(*http.Request) (*http.Response, error)

func (fn arxivRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestArXiv_RegisteredFactoryAndInputForm(t *testing.T) {
	t.Parallel()

	c, err := New("ArXiv", map[string]any{
		"top_n":   float64(7),
		"sort_by": "relevance",
	})
	if err != nil {
		t.Fatalf("New(ArXiv): %v", err)
	}
	if got := c.Name(); got != "ArXiv" {
		t.Fatalf("Name() = %q, want ArXiv", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("ArXiv component does not expose GetInputForm")
	}
	query, ok := formGetter.GetInputForm()["query"].(map[string]any)
	if !ok || query["type"] != "line" {
		t.Fatalf("GetInputForm()[query] = %#v, want Query line input", query)
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestArXiv_InvalidNodeParams(t *testing.T) {
	t.Parallel()

	for _, params := range []map[string]any{
		{"top_n": 0},
		{"sort_by": "newest"},
	} {
		if _, err := New("ArXiv", params); err == nil {
			t.Fatalf("New(ArXiv, %#v) succeeded, want validation error", params)
		}
	}
}

func TestArXiv_InvokeSendsOnlyQueryAndFormatsPythonFields(t *testing.T) {
	t.Parallel()

	const atom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2501.12345v1</id>
    <title>Test Paper</title>
    <summary>Paper summary.</summary>
    <author><name>Author One</name></author>
    <link href="http://arxiv.org/pdf/2501.12345v1" rel="related" type="application/pdf"/>
  </entry>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("search_query"); got != "all:retrieval augmented generation" {
			t.Errorf("search_query = %q", got)
		}
		if got := query.Get("max_results"); got != "7" {
			t.Errorf("max_results = %q, want 7", got)
		}
		if got := query.Get("sortBy"); got != "relevance" {
			t.Errorf("sortBy = %q, want relevance", got)
		}
		_, _ = w.Write([]byte(atom))
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(server.URL): %v", err)
	}
	component := &arxivComponent{inner: agenttool.NewArxivToolWithParams(agenttool.NewHTTPHelper().WithClient(&http.Client{
		Transport: arxivRoundTripper(func(request *http.Request) (*http.Response, error) {
			request.URL.Scheme = serverURL.Scheme
			request.URL.Host = serverURL.Host
			return http.DefaultTransport.RoundTrip(request)
		}),
	}), 7, "relevance")}

	out, err := component.Invoke(context.Background(), map[string]any{"query": "  retrieval augmented generation  "})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	formalized, _ := out["formalized_content"].(string)
	for _, want := range []string{"Test Paper", "http://arxiv.org/pdf/2501.12345v1", "Paper summary."} {
		if !strings.Contains(formalized, want) {
			t.Errorf("formalized_content missing %q: %s", want, formalized)
		}
	}
	if results, ok := out["json"].([]any); !ok || len(results) != 1 {
		t.Fatalf("json output = %#v, want one paper", out["json"])
	}
}

func TestArXiv_InvokeEmptyQueryReturnsEmptyPayload(t *testing.T) {
	t.Parallel()

	c, err := newArxivComponent(nil)
	if err != nil {
		t.Fatalf("newArxivComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{"query": "  "})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["formalized_content"]; got != "" {
		t.Errorf("formalized_content = %v, want empty string", got)
	}
	if results, ok := out["json"].([]any); !ok || len(results) != 0 {
		t.Fatalf("json output = %#v, want empty []any", out["json"])
	}
}
