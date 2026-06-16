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

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

func TestDuckDuckGo_BuildURL(t *testing.T) {
	t.Parallel()

	got := buildDuckDuckGoURL("rag flow")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if u.Host != "api.duckduckgo.com" {
		t.Errorf("host = %q, want api.duckduckgo.com", u.Host)
	}
	q := u.Query()
	if q.Get("q") != "rag flow" {
		t.Errorf("q = %q, want 'rag flow' (no pre-encoding)", q.Get("q"))
	}
	if q.Get("format") != "json" {
		t.Errorf("format = %q, want json", q.Get("format"))
	}
	if q.Get("no_html") != "1" {
		t.Errorf("no_html = %q, want 1", q.Get("no_html"))
	}
}

func TestDuckDuckGo_ParseTopics(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// upstream returns a recursive tree; include a category
		// (`Topics` non-empty, no `FirstURL`) and a leaf hit.
		_, _ = w.Write([]byte(`{
			"abstract_text": "RAGFlow is an open-source RAG engine.",
			"abstract_url": "https://ragflow.io",
			"related_topics": [
				{
					"text": "Category: Technology",
					"topics": [
						{"text": "Open source", "first_url": "https://example.com/os"},
						{"text": "Search engines", "first_url": "https://example.com/se"}
					]
				},
				{"text": "GitHub", "first_url": "https://github.com/infiniflow/ragflow"}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewDuckDuckGoToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"query":"ragflow","max_results":10}`)
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
	if env.AbstractText != "RAGFlow is an open-source RAG engine." {
		t.Errorf("AbstractText = %q, want the upstream abstract", env.AbstractText)
	}
	if env.AbstractURL != "https://ragflow.io" {
		t.Errorf("AbstractURL = %q, want https://ragflow.io", env.AbstractURL)
	}
	if len(env.RelatedTopics) != 3 {
		t.Fatalf("RelatedTopics len = %d, want 3 (category child leaves + direct hit)", len(env.RelatedTopics))
	}
	wantURLs := map[string]bool{
		"https://example.com/os":                false,
		"https://example.com/se":                false,
		"https://github.com/infiniflow/ragflow": false,
	}
	for _, t2 := range env.RelatedTopics {
		if _, ok := wantURLs[t2.FirstURL]; ok {
			wantURLs[t2.FirstURL] = true
		}
	}
	for u, seen := range wantURLs {
		if !seen {
			t.Errorf("missing topic URL %q in flattened result", u)
		}
	}
}

func TestDuckDuckGo_RespectsMaxResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"related_topics": [
				{"text":"A","first_url":"https://a"},
				{"text":"B","first_url":"https://b"},
				{"text":"C","first_url":"https://c"},
				{"text":"D","first_url":"https://d"}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewDuckDuckGoToolWith(helper)
	out, err := tool.InvokableRun(context.Background(), `{"query":"x","max_results":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env duckduckgoEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v", jerr)
	}
	if len(env.RelatedTopics) != 2 {
		t.Errorf("RelatedTopics len = %d, want 2 (capped by max_results)", len(env.RelatedTopics))
	}
}

func TestDuckDuckGo_Info(t *testing.T) {
	t.Parallel()

	tool := NewDuckDuckGoTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "duckduckgo" {
		t.Errorf("Name = %q, want duckduckgo", info.Name)
	}
	if !strings.Contains(info.Desc, "DuckDuckGo") {
		t.Errorf("Desc = %q, want to mention DuckDuckGo", info.Desc)
	}
}

// TestDuckDuckGo_RealReactAgent_ExecutesTool drives a real eino
// react.NewAgent with the real DuckDuckGoTool (httptest-stubbed
// upstream) and a scripted chat model. Proves the HTTP-based tool is
// actually invoked by eino, its JSON envelope is fed back as a
// ToolMessage, and the model can ground a final answer in the
// retrieved abstract. This is the HTTP-side counterpart to
// TestExeSQL_RealReactAgent_ExecutesTool — together they cover the
// two distinct wiring patterns (DB-backed vs HTTP-backed) the agent
// needs to handle.
func TestDuckDuckGo_RealReactAgent_ExecutesTool(t *testing.T) {
	t.Parallel()

	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"abstract_text": "RAGFlow is an open-source RAG engine.",
			"abstract_url":  "https://ragflow.io",
			"related_topics": [
				{"text": "GitHub", "first_url": "https://github.com/infiniflow/ragflow"}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	realTool := NewDuckDuckGoToolWith(helper)

	mdl := newReactScriptedModel(
		"duckduckgo",
		`{"query":"ragflow"}`,
		"RAGFlow is an open-source RAG engine.",
	)

	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: mdl,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []einotool.BaseTool{realTool},
		},
		MaxStep: 5,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}

	out, err := agent.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("What is RAGFlow?"),
	})
	if err != nil {
		t.Fatalf("agent.Generate: %v", err)
	}
	if got, want := out.Content, "RAGFlow is an open-source RAG engine."; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
	if mdl.turn != 2 {
		t.Errorf("Generate called %d times, want 2 (tool_call + final)", mdl.turn)
	}
	if len(mdl.boundTools) != 1 || mdl.boundTools[0].Name != "duckduckgo" {
		names := make([]string, 0, len(mdl.boundTools))
		for _, ti := range mdl.boundTools {
			names = append(names, ti.Name)
		}
		t.Errorf("tools bound to model = %v, want [duckduckgo]", names)
	}
	if len(mdl.rounds) < 2 {
		t.Fatalf("only %d rounds captured, want >= 2", len(mdl.rounds))
	}
	var sawToolResult bool
	for _, msg := range mdl.rounds[1] {
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "RAGFlow is an open-source RAG engine") {
			sawToolResult = true
			break
		}
	}
	if !sawToolResult {
		t.Errorf("round 2 input did not contain a ToolMessage carrying the upstream abstract")
	}
	if hitCount == 0 {
		t.Error("test server was never hit; the tool did not actually call the upstream")
	}
}
