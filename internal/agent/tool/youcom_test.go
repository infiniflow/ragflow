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

	"ragflow/internal/agent/runtime"
)

// youComTestServer records the last request and answers with the given status
// and body.
func youComTestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *atomic.Int32) {
	t.Helper()
	var gotReq http.Request
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotReq = *r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	return helper, &gotReq, calls
}

func decodeYouComEnvelope(t *testing.T, out string) youComEnvelope {
	t.Helper()
	var env youComEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestYouCom_BuildRequest(t *testing.T) {
	t.Parallel()
	helper, req, _ := youComTestServer(t, http.StatusOK, `{"results":{"web":[]}}`)
	tool := newYouComTool(helper, " ydc-test ", youComDefaults{Freshness: "week", TopN: 5})

	out, err := tool.InvokableRun(t.Context(), `{"query":"ragflow go","freshness":"month"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeYouComEnvelope(t, out); env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	if req.Method != http.MethodGet || req.URL.Path != "/v1/search" {
		t.Errorf("request = %s %s, want GET /v1/search", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("X-API-Key"); got != "ydc-test" {
		t.Errorf("X-API-Key = %q, want trimmed key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow youdotcom-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}
	params := req.URL.Query()
	if params.Get("query") != "ragflow go" {
		t.Errorf("query = %q", params.Get("query"))
	}
	if params.Get("count") != "5" {
		t.Errorf("count = %q, want node Top N", params.Get("count"))
	}
	if params.Get("freshness") != "month" {
		t.Errorf("freshness = %q, want model override", params.Get("freshness"))
	}
}

func TestYouCom_KeylessOmitsAuth(t *testing.T) {
	t.Parallel()
	helper, req, _ := youComTestServer(t, http.StatusOK, `{"results":{"web":[]}}`)
	tool := newYouComTool(helper, "", youComDefaults{})

	if _, err := tool.InvokableRun(t.Context(), `{"query":"q"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if req.URL.Path != "/v1/agents/search" {
		t.Errorf("path = %q, want keyless endpoint", req.URL.Path)
	}
	// The keyless endpoint rejects an auth header, so none may be sent.
	if got, ok := req.Header["X-Api-Key"]; ok {
		t.Errorf("X-API-Key = %v, want absent", got)
	}
	// The node default "any" must not be forwarded.
	if _, ok := req.URL.Query()["freshness"]; ok {
		t.Errorf("freshness sent for \"any\": %v", req.URL.Query().Get("freshness"))
	}
}

func TestYouCom_MaxResultsArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value any
		want  int
	}{
		{nil, 7},          // not given: node Top N
		{"", 7},           // blank: node Top N
		{"lots", 7},       // unparseable: node Top N
		{float64(3), 3},   // model's value wins
		{"4", 4},          // numeric string
		{float64(0), 1},   // clamped low
		{float64(99), 20}, // clamped to the max
	}
	for _, tc := range cases {
		if got := youComMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("youComMaxResultsArg(%v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestYouCom_EmptyQuerySkipsRequest(t *testing.T) {
	t.Parallel()
	helper, _, calls := youComTestServer(t, http.StatusOK, `{"results":{"web":[{"title":"x"}]}}`)
	tool := newYouComTool(helper, "key", youComDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"   "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeYouComEnvelope(t, out)
	if env.Error != "" || len(env.Results) != 0 {
		t.Fatalf("envelope = %#v, want empty results", env)
	}
	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want 0", calls.Load())
	}
}

func TestYouCom_UpstreamErrorBecomesEnvelope(t *testing.T) {
	t.Parallel()
	helper, _, _ := youComTestServer(t, http.StatusPaymentRequired, ``)
	tool := newYouComTool(helper, "key", youComDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeYouComEnvelope(t, out)
	if !strings.HasPrefix(env.Error, "youcom: ") {
		t.Fatalf("_ERROR = %q, want a youcom-prefixed error", env.Error)
	}
	// The leaf adds the prefix; the tool's transport errors must not add one again.
	if strings.Contains(env.Error, "youcom: youcom:") {
		t.Fatalf("_ERROR = %q, want a single youcom prefix", env.Error)
	}
}

func TestYouCom_TrimsMergedResultsToMaxResults(t *testing.T) {
	t.Parallel()
	web := make([]map[string]string, 0, 6)
	news := make([]map[string]string, 0, 6)
	for i := 0; i < 6; i++ {
		web = append(web, map[string]string{"url": "https://example.com/w", "title": "w", "description": "d"})
		news = append(news, map[string]string{"url": "https://example.com/n", "title": "n", "description": "d"})
	}
	payload, err := json.Marshal(map[string]any{
		"results": map[string]any{"web": web, "news": news},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	helper, _, _ := youComTestServer(t, http.StatusOK, string(payload))
	tool := newYouComTool(helper, "key", youComDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"q","max_results":3}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeYouComEnvelope(t, out)
	if len(env.Results) != 3 {
		t.Fatalf("results = %d, want 3 (\"count\" is per section, so the merge must be trimmed)", len(env.Results))
	}
}

func TestYouCom_ComponentOutputsAndReferences(t *testing.T) {
	t.Parallel()
	tool := NewYouComTool()
	envelope := map[string]any{
		"results": []any{
			map[string]any{"title": "RAGFlow", "url": "https://ragflow.io", "content": "RAG engine"},
			map[string]any{"title": "Blank", "url": "https://example.com/blank", "content": "   "},
		},
	}

	outputs := tool.BuildComponentOutputs(envelope)
	if _, ok := outputs["json"]; !ok {
		t.Fatalf("outputs missing json: %#v", outputs)
	}
	formalized, _ := outputs["formalized_content"].(string)
	if !strings.Contains(formalized, "https://ragflow.io") {
		t.Fatalf("formalized_content = %q, want the usable hit", formalized)
	}

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("references = %d chunks / %d docAggs, want 1/1 (blank content dropped)", len(chunks), len(docAggs))
	}
	if chunks[0]["url"] != "https://ragflow.io" {
		t.Errorf("chunk url = %v", chunks[0]["url"])
	}
	form := tool.ComponentSpec().InputForm
	if _, ok := form["query"]; !ok {
		t.Errorf("InputForm missing query: %#v", form)
	}
}

// TestYouCom_ReferencesDistinguishSameContentFromDifferentURLs guards the
// citation path: identical snippets served from different URLs must keep
// distinct ids instead of collapsing into one reference.
func TestYouCom_ReferencesDistinguishSameContentFromDifferentURLs(t *testing.T) {
	t.Parallel()
	tool := NewYouComTool()
	content := "identical snippet text"
	envelope := map[string]any{
		"results": []any{
			map[string]any{"title": "First", "url": "https://first.example/page", "content": content},
			map[string]any{"title": "Second", "url": "https://second.example/page", "content": content},
		},
	}

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 || len(docAggs) != 2 {
		t.Fatalf("references = %d chunks / %d docAggs, want 2/2", len(chunks), len(docAggs))
	}
	if chunks[0]["id"] == chunks[1]["id"] {
		t.Fatalf("both chunks share display id %v", chunks[0]["id"])
	}
	if chunks[0]["document_id"] == chunks[1]["document_id"] {
		t.Fatalf("both chunks share document id %v", chunks[0]["document_id"])
	}

	state := runtime.NewCanvasState("run-youcom", "task-youcom")
	state.SetRetrievalReferences(chunks, docAggs)
	if got := state.GetRetrievalChunks(); len(got) != 2 {
		t.Fatalf("retrieval chunks = %d, want 2 (distinct URLs must survive dedupe)", len(got))
	}
}
