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

// contextFetchTestServer records the last request and answers with the given
// status and body.
func contextFetchTestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *map[string]any, *atomic.Int32) {
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

func decodeContextFetchEnvelope(t *testing.T, out string) contextFetchEnvelope {
	t.Helper()
	var env contextFetchEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestContextFetch_BuildRequest(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := contextFetchTestServer(t, http.StatusOK, `{"results":[]}`)
	tool := newContextFetchTool(helper, " key-xyz ", 5)

	out, err := tool.InvokableRun(t.Context(), `{"query":"ragflow go"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeContextFetchEnvelope(t, out); env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/tavily/search" {
		t.Errorf("request = %s %s, want POST /tavily/search", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if (*body)["query"] != "ragflow go" || (*body)["max_results"] != float64(5) {
		t.Errorf("body = %v, want query and the node Top N", *body)
	}
}

func TestContextFetch_MaxResultsArg(t *testing.T) {
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
		{float64(99), 20}, // clamped to the API max
	}
	for _, tc := range cases {
		if got := contextFetchMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("contextFetchMaxResultsArg(%#v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestContextFetch_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, args, wantErr string
		status                   int
		wantCalls                int32
	}{
		{"no api key", "", `{"query":"q"}`, "api_key is required", http.StatusOK, 0},
		{"bad arguments", "key", `{"query":`, "parse arguments", http.StatusOK, 0},
		{"rejected key", "key", `{"query":"q"}`, "upstream returned 401", http.StatusUnauthorized, 1},
		{"out of credits", "key", `{"query":"q"}`, "upstream returned 432", 432, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := contextFetchTestServer(t, tc.status, `{"detail":{"error":"nope"}}`)
			out, err := NewContextFetchToolWith(helper, tc.key).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeContextFetchEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestContextFetch_EmptyQueryReturnsEmptyResults(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := contextFetchTestServer(t, http.StatusOK, `{"results":[]}`)
	out, err := NewContextFetchToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"  "}`)
	if err != nil || out != `{"results":[]}` {
		t.Fatalf("out, err = %q, %v; want empty results", out, err)
	}
	if calls.Load() != 0 {
		t.Error("an empty query must not reach the API")
	}
}

func TestContextFetch_KeepsRawResultsUpToMaxResults(t *testing.T) {
	t.Parallel()
	helper, _, _, _ := contextFetchTestServer(t, http.StatusOK,
		`{"query":"q","results":[{"title":"A","url":"https://a","score":0.9},{"title":"B"},{"title":"C"}],"response_time":0.4}`)
	out, err := NewContextFetchToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"q","max_results":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeContextFetchEnvelope(t, out)
	if len(env.Results) != 2 || env.Results[0]["score"] != 0.9 {
		t.Fatalf("results = %#v, want the first 2 results with fields intact", env.Results)
	}
}

func TestContextFetch_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"results": []any{
		map[string]any{"title": "Page", "url": "https://p", "content": "passage", "raw_content": nil, "score": 0.8},
		map[string]any{"title": "Empty", "url": "https://e", "content": ""},
		"not a result",
	}}
	tool := NewContextFetchToolWith(nil, "")

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("got %d chunks / %d docs, want 1 / 1 (empty result dropped)", len(chunks), len(docAggs))
	}
	if chunks[0]["content"] != "passage" || chunks[0]["url"] != "https://p" || chunks[0]["score"] != 0.8 {
		t.Errorf("chunk 0 = %#v", chunks[0])
	}

	outputs := tool.BuildComponentOutputs(envelope)
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"├── Title: Page", "├── URL: https://p", "└── Content:\npassage"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("formalized_content missing %q:\n%s", want, rendered)
		}
	}
	if raw, _ := outputs["json"].([]any); len(raw) != 3 {
		t.Errorf("json output = %#v, want the raw result list", outputs["json"])
	}
}

func TestContextFetch_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("ContextFetchSearch", map[string]any{
		"api_key": "stored-key", "top_n": float64(4),
		"query": "{sys.query}", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*ContextFetchTool)
	if !ok {
		t.Fatalf("built %T, want *ContextFetchTool", built)
	}
	if tool.apiKey != "stored-key" || tool.topN != 4 {
		t.Errorf("tool = key %q top_n %d", tool.apiKey, tool.topN)
	}
	info, err := tool.Info(t.Context())
	if err != nil || info.Name != contextFetchToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
}

func TestContextFetch_BuildByNameRejectsInvalidNodeConfig(t *testing.T) {
	t.Parallel()
	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"zero top_n":           {"top_n": float64(0)},
		"fractional top_n":     {"top_n": 2.5},
	} {
		if _, err := BuildByName("contextfetch", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
