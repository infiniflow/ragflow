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

// sofyaTestServer records the last request and answers with the given
// status and body.
func sofyaTestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *map[string]any, *atomic.Int32) {
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

func decodeSofyaEnvelope(t *testing.T, out string) sofyaEnvelope {
	t.Helper()
	var env sofyaEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestSofya_BuildRequest(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := sofyaTestServer(t, http.StatusOK, `{"results":[]}`)
	tool := newSofyaTool(helper, " key-xyz ", sofyaDefaults{SearchDepth: "snippets", TopN: 5})

	out, err := tool.InvokableRun(t.Context(), `{"query":"ragflow go","topic":"News","freshness":"week"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeSofyaEnvelope(t, out); env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/v1/search" {
		t.Errorf("request = %s %s, want POST /v1/search", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow sofya-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}
	want := map[string]any{
		"query": "ragflow go", "search_depth": "snippets", "max_results": float64(5),
		"topic": "news", "freshness": "week",
	}
	for k, v := range want {
		if (*body)[k] != v {
			t.Errorf("body.%s = %v, want %v", k, (*body)[k], v)
		}
	}
}

func TestSofya_DefaultsOmitFreshness(t *testing.T) {
	t.Parallel()
	helper, _, body, _ := sofyaTestServer(t, http.StatusOK, `{"results":[]}`)
	tool := NewSofyaToolWith(helper, "key")

	if _, err := tool.InvokableRun(t.Context(), `{"query":"q","freshness":"any"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if _, ok := (*body)["freshness"]; ok {
		t.Errorf("freshness sent for \"any\": %v", (*body)["freshness"])
	}
	if (*body)["search_depth"] != "basic" || (*body)["topic"] != "general" || (*body)["max_results"] != float64(10) {
		t.Errorf("defaults = %v, want basic/general/10", *body)
	}
}

func TestSofya_MaxResultsArg(t *testing.T) {
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
		{float64(99), 20}, // clamped to Sofya's max
	}
	for _, tc := range cases {
		if got := sofyaMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("sofyaMaxResultsArg(%#v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestSofya_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, args, wantErr string
		status                   int
		wantCalls                int32
	}{
		{"no api key", "", `{"query":"q"}`, "api_key is required", http.StatusOK, 0},
		{"unknown topic", "key", `{"query":"q","topic":"sports"}`, `topic "sports" is not supported`, http.StatusOK, 0},
		{"unknown freshness", "key", `{"query":"q","freshness":"decade"}`, `freshness "decade" is not supported`, http.StatusOK, 0},
		{"bad arguments", "key", `{"query":`, "parse arguments", http.StatusOK, 0},
		{"rejected key", "key", `{"query":"q"}`, "upstream returned 401", http.StatusUnauthorized, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := sofyaTestServer(t, tc.status, `{"detail":"nope"}`)
			out, err := NewSofyaToolWith(helper, tc.key).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeSofyaEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestSofya_EmptyQueryReturnsEmptyResults(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := sofyaTestServer(t, http.StatusOK, `{"results":[]}`)
	out, err := NewSofyaToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"  "}`)
	if err != nil || out != `{"results":[]}` {
		t.Fatalf("out, err = %q, %v; want empty results", out, err)
	}
	if calls.Load() != 0 {
		t.Error("an empty query must not reach the API")
	}
}

func TestSofya_KeepsRawResultsUpToMaxResults(t *testing.T) {
	t.Parallel()
	helper, _, _, _ := sofyaTestServer(t, http.StatusOK,
		`{"results":[{"title":"A","url":"https://a","fetched":true},{"title":"B"},{"title":"C"}],"credits_used":2}`)
	out, err := NewSofyaToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"q","max_results":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeSofyaEnvelope(t, out)
	if len(env.Results) != 2 || env.Results[0]["fetched"] != true {
		t.Fatalf("results = %#v, want the first 2 results with fields intact", env.Results)
	}
}

func TestSofya_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"results": []any{
		map[string]any{"title": "Page", "url": "https://p", "content": "  page\n\n text ", "description": "snippet"},
		map[string]any{"title": "Snippet only", "url": "https://s", "content": "", "description": "just a snippet"},
		map[string]any{"title": "Empty", "url": "https://e"},
		"not a result",
	}}
	tool := NewSofyaTool()

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 || len(docAggs) != 2 {
		t.Fatalf("got %d chunks / %d docs, want 2 / 2 (empty result dropped)", len(chunks), len(docAggs))
	}
	if chunks[0]["content"] != "page text" {
		t.Errorf("chunk 0 content = %q, want page content with whitespace collapsed", chunks[0]["content"])
	}
	if chunks[1]["content"] != "just a snippet" {
		t.Errorf("chunk 1 content = %q, want the snippet fallback", chunks[1]["content"])
	}
	if chunks[0]["url"] != "https://p" || docAggs[0]["doc_name"] != "Page" {
		t.Errorf("chunk 0 = %#v / %#v", chunks[0], docAggs[0])
	}

	outputs := tool.BuildComponentOutputs(envelope)
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"├── Title: Page", "├── URL: https://p", "└── Content:\npage text", "just a snippet"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("formalized_content missing %q:\n%s", want, rendered)
		}
	}
	if raw, _ := outputs["json"].([]any); len(raw) != 4 {
		t.Errorf("json output = %#v, want the raw result list", outputs["json"])
	}
}

func TestSofya_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("SofyaSearch", map[string]any{
		"api_key": "stored-key", "search_depth": "snippets", "top_n": float64(4),
		"query": "{sys.query}", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*SofyaTool)
	if !ok {
		t.Fatalf("built %T, want *SofyaTool", built)
	}
	if tool.apiKey != "stored-key" || tool.defaults != (sofyaDefaults{SearchDepth: "snippets", TopN: 4}) {
		t.Errorf("tool = key %q defaults %+v", tool.apiKey, tool.defaults)
	}
	info, err := tool.Info(t.Context())
	if err != nil || info.Name != sofyaToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
}

func TestSofya_BuildByNameRejectsInvalidNodeConfig(t *testing.T) {
	t.Parallel()
	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"unknown search_depth": {"search_depth": "advanced"},
		"zero top_n":           {"top_n": float64(0)},
		"fractional top_n":     {"top_n": 2.5},
	} {
		if _, err := BuildByName("sofya", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
