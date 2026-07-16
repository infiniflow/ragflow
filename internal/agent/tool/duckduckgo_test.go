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
)

func TestDuckDuckGo_BuildSearchURL(t *testing.T) {
	got := buildDuckDuckGoSearchURL("rag flow", 7)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if u.Host != "duckduckgo.com" {
		t.Errorf("host = %q, want duckduckgo.com", u.Host)
	}
	if u.Path != "/html/" {
		t.Errorf("path = %q, want /html/", u.Path)
	}
	q := u.Query()
	if q.Get("q") != "rag flow" {
		t.Errorf("q = %q, want rag flow", q.Get("q"))
	}
	if q.Get("dc") != "8" {
		t.Errorf("dc = %q, want 8", q.Get("dc"))
	}
	if got := q.Get("vqd"); got != "" {
		t.Errorf("vqd = %q, want omitted", got)
	}
}

func TestDuckDuckGo_BuildNewsURL(t *testing.T) {
	got := buildDuckDuckGoNewsURL("rag flow", 3)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if u.Path != "/news.js" {
		t.Errorf("path = %q, want /news.js", u.Path)
	}
	q := u.Query()
	if q.Get("o") != "json" {
		t.Fatalf("o = %q, want json", q.Get("o"))
	}
	if q.Get("l") != "wt-wt" {
		t.Fatalf("l = %q, want wt-wt", q.Get("l"))
	}
	if q.Get("dc") != "3" {
		t.Errorf("dc = %q, want 3", q.Get("dc"))
	}
}

func TestDuckDuckGo_BuildNewsURLWithVQD(t *testing.T) {
	got := buildDuckDuckGoNewsURLWithVQD("rag flow", 3, "vqd-1")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	q := u.Query()
	if q.Get("vqd") != "vqd-1" {
		t.Fatalf("vqd = %q, want vqd-1", q.Get("vqd"))
	}
	if q.Get("u") != "bing" {
		t.Fatalf("u = %q, want bing", q.Get("u"))
	}
}

func TestDuckDuckGo_ParseGeneralResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
			<div class="results">
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fragflow.io">RAGFlow</a>
					<a class="result__snippet">Open source RAG engine</a>
				</div>
				<div class="result">
					<a class="result__a" href="https://github.com/infiniflow/ragflow">GitHub</a>
					<div class="result__snippet">Source code repository</div>
				</div>
			</div>
		</body></html>`))
	}))
	defer srv.Close()

	prevSearch := duckduckgoSearchEndpoint
	duckduckgoSearchEndpoint = srv.URL
	t.Cleanup(func() { duckduckgoSearchEndpoint = prevSearch })

	tool := NewDuckDuckGoTool()
	out, err := tool.InvokableRun(context.Background(), `{"query":"ragflow","top_n":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env duckduckgoEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].Title != "RAGFlow" {
		t.Errorf("Results[0].Title = %q, want RAGFlow", env.Results[0].Title)
	}
	if env.Results[0].URL != "https://ragflow.io" {
		t.Errorf("Results[0].URL = %q, want https://ragflow.io", env.Results[0].URL)
	}
	if env.Results[0].Body != "Open source RAG engine" {
		t.Errorf("Results[0].Body = %q, want Open source RAG engine", env.Results[0].Body)
	}
}

func TestDuckDuckGo_ParseNewsResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bootstrap":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script>var x=""; var vqd="test-vqd-1";</script></head><body></body></html>`))
		case "/news.js":
			if got := r.URL.Query().Get("vqd"); got != "test-vqd-1" {
				t.Fatalf("vqd = %q, want test-vqd-1", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results": [
					{"title":"Story One","url":"https://news.example.com/story-1","excerpt":"Breaking update one"},
					{"title":"Story Two","url":"https://news.example.com/story-2","excerpt":"Breaking &amp; update two"}
				]
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	prevNews := duckduckgoNewsEndpoint
	prevBootstrap := duckduckgoNewsBootstrapEndpoint
	duckduckgoNewsEndpoint = srv.URL + "/news.js"
	duckduckgoNewsBootstrapEndpoint = srv.URL + "/bootstrap"
	t.Cleanup(func() { duckduckgoNewsEndpoint = prevNews })
	t.Cleanup(func() { duckduckgoNewsBootstrapEndpoint = prevBootstrap })

	tool := NewDuckDuckGoTool()
	out, err := tool.InvokableRun(context.Background(), `{"query":"ragflow","channel":"news","top_n":1}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env duckduckgoEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v", jerr)
	}
	if len(env.Results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(env.Results))
	}
	if env.Results[0].Title != "Story One" {
		t.Errorf("Results[0].Title = %q, want Story One", env.Results[0].Title)
	}
	if env.Results[0].URL != "https://news.example.com/story-1" {
		t.Errorf("Results[0].URL = %q, want https://news.example.com/story-1", env.Results[0].URL)
	}
	if env.Results[0].Body != "Breaking update one" {
		t.Errorf("Results[0].Body = %q, want Breaking update one", env.Results[0].Body)
	}
}

func TestDuckDuckGo_DefaultChannelUsesGeneralSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
			<div class="result"><a class="result__a" href="https://n.example/a">A</a></div>
		</body></html>`))
	}))
	defer srv.Close()

	prevSearch := duckduckgoSearchEndpoint
	duckduckgoSearchEndpoint = srv.URL
	t.Cleanup(func() { duckduckgoSearchEndpoint = prevSearch })

	tool := NewDuckDuckGoTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":"ragflow"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
}

func TestDuckDuckGo_ToolMeta(t *testing.T) {
	tool := NewDuckDuckGoTool()
	meta := tool.ToolMeta()
	if meta.Name != "duckduckgo" {
		t.Errorf("Name = %q, want duckduckgo", meta.Name)
	}
	if !strings.Contains(meta.Description, "DuckDuckGo") {
		t.Errorf("Description = %q, want to mention DuckDuckGo", meta.Description)
	}
	if _, ok := meta.Parameters["query"]; !ok {
		t.Fatalf("parameters missing 'query'")
	}
	if _, ok := meta.Parameters["channel"]; !ok {
		t.Fatalf("parameters missing 'channel'")
	}
	if _, ok := meta.Parameters["top_n"]; ok {
		t.Fatalf("parameters should not expose top_n")
	}
}

func TestDuckDuckGo_EmptyQuery(t *testing.T) {
	tool := NewDuckDuckGoTool()
	out, err := tool.InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun(empty): %v", err)
	}
	var envelope duckduckgoEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || len(envelope.Results) != 0 {
		t.Fatalf("empty result = %s / %v", out, err)
	}
}

func TestDuckDuckGo_RealReactAgent_ExecutesTool(t *testing.T) {
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
			<div class="result">
				<a class="result__a" href="https://ragflow.io">RAGFlow</a>
				<div class="result__snippet">RAGFlow is an open-source RAG engine.</div>
			</div>
		</body></html>`))
	}))
	defer srv.Close()

	prevSearch := duckduckgoSearchEndpoint
	duckduckgoSearchEndpoint = srv.URL
	t.Cleanup(func() { duckduckgoSearchEndpoint = prevSearch })

	realTool := NewDuckDuckGoTool()
	// Verify the tool actually calls the upstream and returns results.
	result, err := realTool.InvokableRun(context.Background(), `{"query":"ragflow"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	var env duckduckgoEnvelope
	if jerr := json.Unmarshal([]byte(result), &env); jerr != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if len(env.Results) == 0 {
		t.Fatal("expected at least one result")
	}
	if env.Results[0].Title != "RAGFlow" {
		t.Errorf("Results[0].Title = %q, want RAGFlow", env.Results[0].Title)
	}
	if hitCount == 0 {
		t.Error("test server was never hit; the tool did not actually call the upstream")
	}
}

func TestDuckDuckGo_ComponentReferencesAndDefaults(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("duckduckgo", map[string]any{
		"top_n":   float64(4),
		"channel": "news",
		"outputs": map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	duck := built.(*DuckDuckGoTool)
	if duck.defaults.TopN != 4 || duck.defaults.Channel != "news" {
		t.Fatalf("defaults = %+v", duck.defaults)
	}
	for _, params := range []map[string]any{{"top_n": 0}, {"top_n": 1.5}, {"channel": "images"}} {
		if _, err := BuildByName("duckduckgo", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded", params)
		}
	}
	spec := duck.ComponentSpec()
	if channel, ok := spec.InputForm["channel"].(map[string]any); !ok || channel["value"] != "general" {
		t.Fatalf("channel input form = %#v", spec.InputForm["channel"])
	}
	envelope := map[string]any{"results": []any{map[string]any{
		"title": "Story", "url": "https://news.example/story", "body": "Breaking update",
	}}}
	chunks, docAggs := duck.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 || chunks[0]["content"] != "Breaking update" {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	outputs := duck.BuildComponentOutputs(envelope)
	if results, ok := outputs["json"].([]any); !ok || len(results) != 1 {
		t.Fatalf("json output = %#v", outputs["json"])
	}
	if !strings.Contains(outputs["formalized_content"].(string), "Breaking update") {
		t.Fatalf("formalized_content = %q", outputs["formalized_content"])
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("output conversion mutated envelope: %#v", envelope)
	}
}
