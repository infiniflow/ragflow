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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func specstreamTestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *map[string]any, *atomic.Int32) {
	t.Helper()
	var gotReq http.Request
	gotBody := map[string]any{}
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotReq = *r.Clone(r.Context())
		gotBody = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	return helper, &gotReq, &gotBody, calls
}

func decodeSpecStreamEnvelope(t *testing.T, out string) specstreamEnvelope {
	t.Helper()
	var env specstreamEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestSpecStream_BuildRequestAndMergePassages(t *testing.T) {
	t.Parallel()
	helper, req, _, _ := specstreamTestServer(t, http.StatusOK, `{
		"data": {
			"id": "req_1",
			"kind": "fact",
			"sources": [
				{"url": "https://ragflow.io", "title": "RAGFlow", "site": "ragflow.io", "snippet": "Open-source RAG engine.", "cited": true}
			],
			"passages": [
				{"url": "https://ragflow.io", "title": "RAGFlow", "text": "Full extracted passage."}
			]
		}
	}`)
	tool := newSpecStreamTool(helper, " key-xyz ", specstreamDefaults{TopN: 5})

	out, err := tool.InvokableRun(t.Context(), `{"query":"ragflow go","passages":true}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeSpecStreamEnvelope(t, out)
	if env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	if req.Method != http.MethodGet || req.URL.Path != "/v1/web-search" {
		t.Errorf("request = %s %s, want GET /v1/web-search", req.Method, req.URL.Path)
	}
	if got := req.URL.Query().Get("q"); got != "ragflow go" {
		t.Errorf("q = %q, want ragflow go", got)
	}
	if got := req.URL.Query().Get("passages"); got != "true" {
		t.Errorf("passages = %q, want true", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow specstream-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}
	if len(env.Results) != 1 || env.Results[0]["content"] != "Full extracted passage." {
		t.Fatalf("results = %#v, want merged passage content", env.Results)
	}
}

func TestSpecStream_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, args, wantErr string
		status                   int
		wantCalls                int32
	}{
		{"no api key", "", `{"query":"q"}`, "api_key is required", http.StatusOK, 0},
		{"bad arguments", "key", `{"query":`, "parse arguments", http.StatusOK, 0},
		{"rejected key", "key", `{"query":"q"}`, "upstream returned 401", http.StatusUnauthorized, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := specstreamTestServer(t, tc.status, `{"error":"nope"}`)
			out, err := NewSpecStreamToolWith(helper, tc.key).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeSpecStreamEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestSpecStream_EmptyQueryReturnsEmptyResults(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := specstreamTestServer(t, http.StatusOK, `{"data":{"sources":[]}}`)
	out, err := NewSpecStreamToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"  "}`)
	if err != nil || out != `{"results":[]}` {
		t.Fatalf("out, err = %q, %v; want empty results", out, err)
	}
	if calls.Load() != 0 {
		t.Error("an empty query must not reach the API")
	}
}

func TestSpecStream_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"results": []any{
		map[string]any{"title": "Page", "url": "https://p", "content": "  page\n\n text ", "snippet": "snippet"},
		map[string]any{"title": "Snippet only", "url": "https://s", "snippet": "just a snippet"},
		map[string]any{"title": "Empty", "url": "https://e"},
	}}
	tool := NewSpecStreamTool()

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 || len(docAggs) != 2 {
		t.Fatalf("got %d chunks / %d docs, want 2 / 2", len(chunks), len(docAggs))
	}
	outputs := tool.BuildComponentOutputs(envelope)
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"├── Title: Page", "├── URL: https://p", "└── Content:\npage text", "just a snippet"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("formalized_content missing %q:\n%s", want, rendered)
		}
	}
}

func TestSpecStream_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("SpecStreamSearch", map[string]any{
		"api_key": "stored-key", "top_n": float64(4), "passages": true,
		"query": "{sys.query}", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*SpecStreamTool)
	if !ok {
		t.Fatalf("built %T, want *SpecStreamTool", built)
	}
	want := specstreamDefaults{Passages: true, TopN: 4}
	if tool.apiKey != "stored-key" || tool.defaults != want {
		t.Errorf("tool = key %q defaults %+v, want %+v", tool.apiKey, tool.defaults, want)
	}
	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"zero top_n":           {"top_n": float64(0)},
		"top_n over the limit": {"top_n": float64(21)},
	} {
		if _, err := BuildByName("specstream", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
