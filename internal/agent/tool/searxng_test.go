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
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/tokenizer"
)

func TestSearXNGBuildURLMatchesPythonQuery(t *testing.T) {
	t.Parallel()

	got := buildSearXNGURL("https://searx.example.com", "rag flow")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if parsed.Path != "/search" {
		t.Fatalf("path = %q, want /search", parsed.Path)
	}
	want := map[string]string{
		"q":          "rag flow",
		"format":     "json",
		"categories": "general",
		"language":   "auto",
		"safesearch": "1",
		"pageno":     "1",
	}
	for key, value := range want {
		if actual := parsed.Query().Get(key); actual != value {
			t.Errorf("query[%s] = %q, want %q", key, actual, value)
		}
	}
}

func TestSearXNGInvokableRunPreservesRawResultsAndTopN(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/search" {
			t.Errorf("path = %q, want /search", request.URL.Path)
		}
		if request.URL.Query().Get("q") != "  ragflow search  " {
			t.Errorf("q = %q, want original query whitespace", request.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"title":"RAGFlow","url":"https://ragflow.io","content":"Open source RAG engine","engine":"bing","score":0.9},
				{"title":"GitHub","url":"https://github.com/infiniflow/ragflow","content":"Source code","engines":["google","bing"]},
				{"title":"Dropped","url":"https://example.com","content":"top_n applies"}
			]
		}`))
	}))
	defer server.Close()

	defaults := defaultSearXNGParams()
	defaults.SearXNGURL = server.URL
	defaults.TopN = 2
	searchTool := newLocalSearXNGTool(t, defaults)
	out, err := searchTool.InvokableRun(context.Background(), `{"query":"  ragflow search  ","searxng_url":"http://127.0.0.1:1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var envelope searxngEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", err, out)
	}
	if len(envelope.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(envelope.Results))
	}
	first := envelope.Results[0].(map[string]any)
	if first["engine"] != "bing" || first["score"] != float64(0.9) {
		t.Fatalf("first result lost upstream fields: %#v", first)
	}
	second := envelope.Results[1].(map[string]any)
	if _, ok := second["engines"].([]any); !ok {
		t.Fatalf("second result lost engines: %#v", second)
	}
}

func TestSearXNGInfoMatchesPythonModelSchema(t *testing.T) {
	t.Parallel()

	tool := NewSearXNGTool()
	meta := tool.ToolMeta()
	if meta.Name != "searxng_search" {
		t.Errorf("Name = %q, want searxng_search", meta.Name)
	}
	if !strings.Contains(meta.Description, "SearXNG") {
		t.Errorf("Desc = %q, want to mention SearXNG", meta.Description)
	}
}

func TestSearXNGEmptyTryRunInputsSkipRequest(t *testing.T) {
	t.Parallel()

	searchTool := NewSearXNGTool()
	var resolves atomic.Int32
	searchTool.resolve = func(string) (string, net.IP, error) {
		resolves.Add(1)
		return "", nil, errors.New("must not resolve")
	}
	for _, args := range []string{
		`{"query":""}`,
		`{"query":"   ","searxng_url":"https://example.com"}`,
		`{"query":"ragflow"}`,
	} {
		out, err := searchTool.InvokableRun(context.Background(), args)
		if err != nil {
			t.Fatalf("InvokableRun(%s): %v", args, err)
		}
		var envelope searxngEnvelope
		if err := json.Unmarshal([]byte(out), &envelope); err != nil || len(envelope.Results) != 0 || envelope.Error != "" {
			t.Fatalf("InvokableRun(%s) = %s, want empty results", args, out)
		}
	}
	if resolves.Load() != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolves.Load())
	}
}

func TestSearXNGBuildByNameAcceptsPythonNodeParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("searxng", map[string]any{
		"top_n":       "8",
		"searxng_url": "https://searx.example.com",
		"outputs":     map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	searchTool, ok := built.(*SearXNGTool)
	if !ok {
		t.Fatalf("built type = %T, want *SearXNGTool", built)
	}
	if searchTool.defaults.TopN != 8 || searchTool.defaults.SearXNGURL != "https://searx.example.com" {
		t.Fatalf("defaults = %+v", searchTool.defaults)
	}
}

func TestSearXNGBuildByNameRejectsInvalidNodeParams(t *testing.T) {
	t.Parallel()

	invalid := []map[string]any{
		{"top_n": "abc"},
		{"top_n": 0},
		{"top_n": 1.5},
	}
	for _, params := range invalid {
		if _, err := BuildByName("searxng", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded, want validation error", params)
		}
	}
}

func TestSearXNGComponentContractReferencesAndOutputs(t *testing.T) {
	t.Parallel()

	tool := NewSearXNGTool()
	spec := tool.ComponentSpec()
	for _, input := range []string{"query", "searxng_url"} {
		if _, ok := spec.Inputs[input]; !ok {
			t.Fatalf("component inputs missing %s: %#v", input, spec.Inputs)
		}
	}
	for _, output := range []string{"formalized_content", "json"} {
		if _, ok := spec.Outputs[output]; !ok {
			t.Fatalf("component outputs missing %s: %#v", output, spec.Outputs)
		}
	}
	serverURL := spec.InputForm["searxng_url"].(map[string]any)
	if serverURL["placeholder"] != "http://localhost:4000" {
		t.Fatalf("searxng_url input form = %#v", serverURL)
	}

	content := "RAGFlow content ![img](data:image/png;base64,AAAA) remains"
	envelope := map[string]any{"results": []any{
		map[string]any{"title": "RAGFlow\nDocs", "url": "https://ragflow.io", "content": content, "engine": "bing", "score": 0.9},
		map[string]any{"title": "Empty", "url": "https://example.com", "content": ""},
	}}
	chunks, docAggs := tool.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	cleaned := "RAGFlow content  remains"
	documentID := hashSearXNGString(cleaned, 100000000)
	referenceID := hashSearXNGString(strconv.Itoa(documentID), 500)
	if documentID != 93760153 || referenceID != 491 {
		t.Fatalf("hash parity = %d/%d, want Python 93760153/491", documentID, referenceID)
	}
	if chunks[0]["document_id"] != strconv.Itoa(documentID) || chunks[0]["id"] != strconv.Itoa(referenceID) || chunks[0]["content"] != cleaned {
		t.Fatalf("reference chunk = %#v", chunks[0])
	}

	outputs := tool.BuildComponentOutputs(envelope)
	results, ok := outputs["json"].([]any)
	if !ok || len(results) != 2 || results[0].(map[string]any)["engine"] != "bing" {
		t.Fatalf("json output lost raw fields: %#v", outputs["json"])
	}
	formalized := outputs["formalized_content"].(string)
	for _, want := range []string{
		"ID: " + strconv.Itoa(referenceID),
		"Title: RAGFlow Docs",
		"URL: https://ragflow.io",
		"Content:\n" + cleaned,
	} {
		if !strings.Contains(formalized, want) {
			t.Fatalf("formalized_content missing %q: %s", want, formalized)
		}
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("component conversion mutated envelope: %#v", envelope)
	}
}

func TestRenderSearXNGReferencesStopsBeforeOverBudgetBlock(t *testing.T) {
	t.Parallel()

	chunks := []map[string]any{
		{"id": "1", "document_name": "First", "url": "https://first.example", "content": "first reference content"},
		{"id": "2", "document_name": "Second", "url": "https://second.example", "content": "second reference content"},
	}
	firstBlock := renderSearXNGReferences(chunks[:1], 0)
	firstTokens := tokenizer.NumTokensFromString(firstBlock)
	maxTokens := (firstTokens*100 + 96) / 97
	if got := renderSearXNGReferences(chunks, maxTokens); got != firstBlock {
		t.Fatalf("rendered = %q, want only first block %q", got, firstBlock)
	}
	if got := renderSearXNGReferences(chunks, 1); got != "" {
		t.Fatalf("over-budget first block was appended: %q", got)
	}
	if got := renderSearXNGReferences(chunks, 0); !strings.Contains(got, "Title: First") || !strings.Contains(got, "Title: Second") {
		t.Fatalf("unlimited rendering dropped blocks: %q", got)
	}
}

func TestSearXNGDoesNotRetryFailedRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	defaults := defaultSearXNGParams()
	defaults.SearXNGURL = server.URL
	searchTool := newLocalSearXNGTool(t, defaults)
	if _, err := searchTool.InvokableRun(context.Background(), `{"query":"single attempt"}`); err == nil {
		t.Fatal("InvokableRun succeeded, want upstream error")
	}
	if calls.Load() != 1 {
		t.Fatalf("request calls = %d, want 1", calls.Load())
	}
}

func TestSearXNGSSRFGuardRejectsLoopback(t *testing.T) {
	t.Parallel()

	defaults := defaultSearXNGParams()
	defaults.SearXNGURL = "http://127.0.0.1:4000"
	searchTool := newSearXNGToolWithDefaults(nil, defaults)
	out, err := searchTool.InvokableRun(context.Background(), `{"query":"metadata"}`)
	if err == nil || !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("err = %v, want ErrSSRFBlocked", err)
	}
	if !strings.Contains(out, `"_ERROR"`) {
		t.Fatalf("output = %s, want _ERROR envelope", out)
	}
}

func newLocalSearXNGTool(t *testing.T, defaults searxngParams) *SearXNGTool {
	t.Helper()
	searchTool := newSearXNGToolWithDefaults(nil, defaults)
	searchTool.resolve = func(rawURL string) (string, net.IP, error) {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return "", nil, err
		}
		ip := net.ParseIP(parsed.Hostname())
		if ip == nil {
			return "", nil, errors.New("test URL must use a literal IP")
		}
		return parsed.Hostname(), ip, nil
	}
	return searchTool
}
