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

// search1APITestServer records the last request and answers with the given
// status and body.
func search1APITestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *map[string]any, *atomic.Int32) {
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

func decodeSearch1APIEnvelope(t *testing.T, out string) search1APIEnvelope {
	t.Helper()
	var env search1APIEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestSearch1API_BuildRequest(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := search1APITestServer(t, http.StatusOK, `{"results":[]}`)
	tool := newSearch1APITool(helper, " key-xyz ", search1APIDefaults{TopN: 5})

	out, err := tool.InvokableRun(t.Context(), `{"query":"ragflow go","channel":"News","search_service":"HackerNews","time_range":"week"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeSearch1APIEnvelope(t, out); env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/news" {
		t.Errorf("request = %s %s, want POST /news", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow search1api-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}
	want := map[string]any{
		"query": "ragflow go", "search_service": "hackernews", "max_results": float64(5), "time_range": "week",
	}
	for k, v := range want {
		if (*body)[k] != v {
			t.Errorf("body.%s = %v, want %v", k, (*body)[k], v)
		}
	}
	if len(*body) != len(want) {
		t.Errorf("body = %v, want only %v", *body, want)
	}
}

func TestSearch1API_DefaultsOmitOptionalFields(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := search1APITestServer(t, http.StatusOK, `{"results":[]}`)
	tool := NewSearch1APIToolWith(helper, "key")

	if _, err := tool.InvokableRun(t.Context(), `{"query":"q","time_range":"any"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if req.URL.Path != "/search" {
		t.Errorf("path = %q, want /search for the general channel", req.URL.Path)
	}
	for _, key := range []string{"time_range", "search_service"} {
		if _, ok := (*body)[key]; ok {
			t.Errorf("%s sent without a value: %v", key, (*body)[key])
		}
	}
	if (*body)["max_results"] != float64(10) {
		t.Errorf("max_results = %v, want the default Top N 10", (*body)["max_results"])
	}
}

func TestSearch1API_ModelOverridesNodeDefaults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, args, wantPath string
		wantService          any
	}{
		{"node defaults", `{"query":"q"}`, "/search", "github"},
		{"model service", `{"query":"q","search_service":"arxiv"}`, "/search", "arxiv"},
		// The node's github is not a news service, so Search1API picks its default.
		{"model channel drops node service", `{"query":"q","channel":"news"}`, "/news", nil},
		{"model channel and service", `{"query":"q","channel":"news","search_service":"reuters"}`, "/news", "reuters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, req, body, _ := search1APITestServer(t, http.StatusOK, `{"results":[]}`)
			tool := newSearch1APITool(helper, "key", search1APIDefaults{Channel: "general", SearchService: "github"})
			out, err := tool.InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("InvokableRun: %v", err)
			}
			if env := decodeSearch1APIEnvelope(t, out); env.Error != "" {
				t.Fatalf("unexpected _ERROR %q", env.Error)
			}
			if req.URL.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", req.URL.Path, tc.wantPath)
			}
			if got := (*body)["search_service"]; got != tc.wantService {
				t.Errorf("search_service = %v, want %v", got, tc.wantService)
			}
		})
	}
}

func TestSearch1API_MaxResultsArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value any
		want  int
	}{
		{nil, 7},            // not given: node Top N
		{"", 7},             // blank: node Top N
		{"lots", 7},         // unparseable: node Top N
		{float64(3), 3},     // model's value wins
		{"4", 4},            // numeric string
		{float64(0), 1},     // clamped low
		{float64(99), 50},   // clamped to Search1API's max
		{float64(-5), 1},    // negative
		{float64(50), 50},   // upper bound kept
		{float64(20.9), 20}, // fractional truncated
	}
	for _, tc := range cases {
		if got := search1APIMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("search1APIMaxResultsArg(%#v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestSearch1API_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, args, wantErr string
		status                   int
		wantCalls                int32
	}{
		{"no api key", "", `{"query":"q"}`, "api_key is required", http.StatusOK, 0},
		{"unknown channel", "key", `{"query":"q","channel":"images"}`, `channel "images" is not supported`, http.StatusOK, 0},
		{"unknown service", "key", `{"query":"q","search_service":"altavista"}`, `search_service "altavista" is not supported by the general channel`, http.StatusOK, 0},
		{"service not in channel", "key", `{"query":"q","channel":"news","search_service":"github"}`, `search_service "github" is not supported by the news channel`, http.StatusOK, 0},
		{"unknown time range", "key", `{"query":"q","time_range":"decade"}`, `time_range "decade" is not supported`, http.StatusOK, 0},
		{"bad arguments", "key", `{"query":`, "parse arguments", http.StatusOK, 0},
		{"rejected key", "key", `{"query":"q"}`, "upstream returned 401", http.StatusUnauthorized, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := search1APITestServer(t, tc.status, `{"error":"nope"}`)
			out, err := NewSearch1APIToolWith(helper, tc.key).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeSearch1APIEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestSearch1API_EmptyQueryReturnsEmptyResults(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := search1APITestServer(t, http.StatusOK, `{"results":[]}`)
	out, err := NewSearch1APIToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"  "}`)
	if err != nil || out != `{"results":[]}` {
		t.Fatalf("out, err = %q, %v; want empty results", out, err)
	}
	if calls.Load() != 0 {
		t.Error("an empty query must not reach the API")
	}
}

func TestSearch1API_KeepsRawResultsUpToMaxResults(t *testing.T) {
	t.Parallel()
	helper, _, _, _ := search1APITestServer(t, http.StatusOK,
		`{"searchParameters":{"query":"q"},"results":[{"title":"A","link":"https://a","published_date":"2026-10-01"},{"title":"B"},{"title":"C"}]}`)
	out, err := NewSearch1APIToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"q","max_results":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeSearch1APIEnvelope(t, out)
	if len(env.Results) != 2 || env.Results[0]["published_date"] != "2026-10-01" {
		t.Fatalf("results = %#v, want the first 2 results with fields intact", env.Results)
	}
}

func TestSearch1API_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"results": []any{
		map[string]any{"title": "Page", "link": "https://p", "content": "  page\n\n text ", "snippet": "snippet"},
		map[string]any{"title": "Snippet only", "link": "https://s", "snippet": "just a snippet"},
		map[string]any{"title": "Empty", "link": "https://e"},
		"not a result",
	}}
	tool := NewSearch1APITool()

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
	if chunks[0]["url"] != "https://p" || docAggs[0]["doc_name"] != "Page" || docAggs[0]["url"] != "https://p" {
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

func TestSearch1API_InfoExposesServiceChoice(t *testing.T) {
	t.Parallel()
	info, err := NewSearch1APITool().Info(t.Context())
	if err != nil || info.Name != search1APIToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	params, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, _ := json.Marshal(params)
	for _, want := range []string{`"search_service"`, `"hackernews"`, `"arxiv"`, `"channel"`, `"time_range"`, `"max_results"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("schema missing %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "api_key") {
		t.Errorf("api_key must not be model-visible: %s", raw)
	}
}

func TestSearch1API_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("Search1APISearch", map[string]any{
		"api_key": "stored-key", "channel": "news", "search_service": "reuters", "top_n": float64(4),
		"query": "{sys.query}", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*Search1APITool)
	if !ok {
		t.Fatalf("built %T, want *Search1APITool", built)
	}
	want := search1APIDefaults{Channel: "news", SearchService: "reuters", TopN: 4}
	if tool.apiKey != "stored-key" || tool.defaults != want {
		t.Errorf("tool = key %q defaults %+v, want %+v", tool.apiKey, tool.defaults, want)
	}
}

func TestSearch1API_BuildByNameRejectsInvalidNodeConfig(t *testing.T) {
	t.Parallel()
	for name, params := range map[string]map[string]any{
		"api_key not a string":     {"api_key": 42},
		"unknown channel":          {"channel": "images"},
		"unknown search_service":   {"search_service": "altavista"},
		"service not in channel":   {"channel": "news", "search_service": "github"},
		"news service for general": {"search_service": "reuters"},
		"zero top_n":               {"top_n": float64(0)},
		"top_n over the limit":     {"top_n": float64(51)},
		"fractional top_n":         {"top_n": 2.5},
	} {
		if _, err := BuildByName("search1api", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
