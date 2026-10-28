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
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// perplexityTestServer records the last request and its body and answers with
// the given status and body.
func perplexityTestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *[]byte, *atomic.Int32) {
	t.Helper()
	var gotReq http.Request
	var gotBody []byte
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotReq = *r.Clone(r.Context())
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)})
	return helper, &gotReq, &gotBody, calls
}

func decodePerplexityEnvelope(t *testing.T, out string) perplexityEnvelope {
	t.Helper()
	var env perplexityEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestPerplexity_Info(t *testing.T) {
	t.Parallel()
	info, err := NewPerplexityTool().Info(t.Context())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "perplexity_search" {
		t.Errorf("Name = %q, want perplexity_search", info.Name)
	}
	params, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("Info schema: %v", err)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal Info schema: %v", err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode Info schema: %v", err)
	}
	properties, ok := decoded["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Info schema properties = %#v, want object", decoded["properties"])
	}
	for _, name := range []string{"query", "search_recency_filter", "max_results"} {
		if _, ok = properties[name]; !ok {
			t.Errorf("parameter %q missing", name)
		}
	}
	if _, ok = properties["api_key"]; ok {
		t.Error("api_key must not be model-visible")
	}
}

func TestPerplexity_BuildRequest(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := perplexityTestServer(t, http.StatusOK, `{"id":"s","results":[]}`)
	tool := newPerplexityTool(helper, " pplx-test ", perplexityDefaults{
		SearchType:          "fast",
		SearchRecencyFilter: "week",
		SearchDomainFilter:  []string{"ragflow.io", "docs.perplexity.ai"},
		Country:             "US",
		TopN:                5,
	})

	out, err := tool.InvokableRun(t.Context(), `{"query":"ragflow go","search_recency_filter":"month"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodePerplexityEnvelope(t, out); env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/search" {
		t.Errorf("request = %s %s, want POST /search", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer pplx-test" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := req.Header.Get("X-Pplx-Integration"); got != "ragflow" {
		t.Errorf("X-Pplx-Integration = %q, want ragflow", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow perplexity-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}

	var sent map[string]any
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if sent["query"] != "ragflow go" {
		t.Errorf("query = %v", sent["query"])
	}
	if sent["max_results"] != float64(5) {
		t.Errorf("max_results = %v, want node Top N 5", sent["max_results"])
	}
	if sent["search_type"] != "fast" {
		t.Errorf("search_type = %v, want node default fast", sent["search_type"])
	}
	if sent["search_recency_filter"] != "month" {
		t.Errorf("search_recency_filter = %v, want model override month", sent["search_recency_filter"])
	}
	if sent["country"] != "US" {
		t.Errorf("country = %v, want US", sent["country"])
	}
	domains, _ := sent["search_domain_filter"].([]any)
	if len(domains) != 2 || domains[0] != "ragflow.io" || domains[1] != "docs.perplexity.ai" {
		t.Errorf("search_domain_filter = %v", sent["search_domain_filter"])
	}
}

func TestPerplexity_DefaultsOmitOptionalFields(t *testing.T) {
	t.Parallel()
	helper, _, body, _ := perplexityTestServer(t, http.StatusOK, `{"results":[]}`)
	tool := newPerplexityTool(helper, "key", perplexityDefaults{})

	if _, err := tool.InvokableRun(t.Context(), `{"query":"q"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if sent["search_type"] != "web" {
		t.Errorf("search_type = %v, want web", sent["search_type"])
	}
	if sent["max_results"] != float64(perplexityDefaultTopN) {
		t.Errorf("max_results = %v, want %d", sent["max_results"], perplexityDefaultTopN)
	}
	// "any" recency and empty filters must not be forwarded.
	for _, key := range []string{"search_recency_filter", "search_domain_filter", "country"} {
		if _, ok := sent[key]; ok {
			t.Errorf("%s sent as %v, want absent", key, sent[key])
		}
	}
}

func TestPerplexity_MaxResultsArg(t *testing.T) {
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
		{float64(99), 20}, // clamped to the API maximum
	}
	for _, tc := range cases {
		if got := perplexityMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("perplexityMaxResultsArg(%v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestPerplexity_DomainFilterArg(t *testing.T) {
	t.Parallel()
	// Allowlist mode: plain domains, comma or newline separated, blanks dropped.
	got, err := perplexityDomainFilterArg(" ragflow.io, example.com ,, \n docs.perplexity.ai ")
	if err != nil {
		t.Fatalf("allowlist filter: %v", err)
	}
	if len(got) != 3 || got[0] != "ragflow.io" || got[1] != "example.com" || got[2] != "docs.perplexity.ai" {
		t.Errorf("allowlist filter = %v", got)
	}
	// Denylist mode: every entry carries the "-" prefix.
	got, err = perplexityDomainFilterArg("-pinterest.com, -reddit.com")
	if err != nil {
		t.Fatalf("denylist filter: %v", err)
	}
	if len(got) != 2 || got[0] != "-pinterest.com" || got[1] != "-reddit.com" {
		t.Errorf("denylist filter = %v", got)
	}
	got, err = perplexityDomainFilterArg([]any{"a.com", " ", "b.com"})
	if err != nil || len(got) != 2 {
		t.Errorf("list filter = %v, %v", got, err)
	}
	if got, err := perplexityDomainFilterArg("  "); err != nil || got != nil {
		t.Errorf("blank filter = %v, %v, want nil", got, err)
	}
	if _, err := perplexityDomainFilterArg([]any{1}); err == nil {
		t.Error("non-string entry accepted")
	}
	// The API accepts one mode per request; a mix is rejected up front.
	for _, mixed := range []any{"ragflow.io, -example.com", []string{"-a.com", "b.com"}} {
		_, err := perplexityDomainFilterArg(mixed)
		if err == nil || !strings.Contains(err.Error(), "not both") {
			t.Errorf("mixed filter %v: err = %v, want a not-both error", mixed, err)
		}
	}
	tooMany := strings.Repeat("a.com,", perplexityMaxDomainFilters+1)
	if _, err := perplexityDomainFilterArg(tooMany); err == nil {
		t.Error("more than 20 domains accepted")
	}
}

func TestPerplexity_RejectsUnknownRecency(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := perplexityTestServer(t, http.StatusOK, `{"results":[]}`)
	tool := newPerplexityTool(helper, "key", perplexityDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"q","search_recency_filter":"decade"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodePerplexityEnvelope(t, out)
	if !strings.Contains(env.Error, "search_recency_filter") {
		t.Fatalf("_ERROR = %q, want a recency error", env.Error)
	}
	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want 0", calls.Load())
	}
}

func TestPerplexity_EmptyQuerySkipsRequest(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := perplexityTestServer(t, http.StatusOK, `{"results":[{"title":"x"}]}`)
	tool := newPerplexityTool(helper, "key", perplexityDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"   "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodePerplexityEnvelope(t, out)
	if env.Error != "" || len(env.Results) != 0 {
		t.Fatalf("envelope = %#v, want empty results", env)
	}
	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want 0", calls.Load())
	}
}

func TestPerplexity_MissingKeyIsEnvelopeError(t *testing.T) {
	t.Parallel()
	helper, _, _, calls := perplexityTestServer(t, http.StatusOK, `{"results":[]}`)
	tool := newPerplexityTool(helper, "", perplexityDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodePerplexityEnvelope(t, out); !strings.Contains(env.Error, "api_key is required") {
		t.Fatalf("_ERROR = %q, want missing key error", env.Error)
	}
	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want 0", calls.Load())
	}
}

func TestPerplexity_UpstreamErrorBecomesEnvelope(t *testing.T) {
	// Not parallel: it captures the process-wide log output to prove that
	// neither the key nor the upstream body is written anywhere.
	const keyMarker = "pplx-SECRET-KEY-MARKER-7f3a"
	const bodyMarker = "BODY-TEXT-MARKER-91c2"
	var logs bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prevOut) })

	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"` + bodyMarker + ` for key ` + keyMarker + `","type":"unauthorized","code":401}}`, "perplexity: upstream returned 401 (invalid API key)"},
		{"validation", http.StatusUnprocessableEntity, `{"detail":[{"loc":["body","max_results"],"msg":"` + bodyMarker + `","type":"value_error"}]}`, "perplexity: upstream returned 422 (request rejected, check search_domain_filter, country and max_results)"},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"` + bodyMarker + `"}}`, "perplexity: upstream returned 429 (rate limited, retry later)"},
		{"server error", http.StatusInternalServerError, bodyMarker + " " + keyMarker, "perplexity: upstream returned 500"},
		{"other", http.StatusBadGateway, `<html>` + bodyMarker + `</html>`, "perplexity: upstream returned 502"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			helper, _, _, _ := perplexityTestServer(t, tc.status, tc.body)
			tool := newPerplexityTool(helper, keyMarker, perplexityDefaults{})

			out, err := tool.InvokableRun(t.Context(), `{"query":"q"}`)
			if err != nil {
				t.Fatalf("InvokableRun: %v", err)
			}
			env := decodePerplexityEnvelope(t, out)
			if env.Error != tc.want {
				t.Fatalf("_ERROR = %q, want exactly %q", env.Error, tc.want)
			}
			for _, marker := range []string{keyMarker, bodyMarker} {
				if strings.Contains(out, marker) {
					t.Errorf("tool output leaks %q: %s", marker, out)
				}
			}
		})
	}
	for _, marker := range []string{keyMarker, bodyMarker} {
		if strings.Contains(logs.String(), marker) {
			t.Errorf("log output leaks %q: %s", marker, logs.String())
		}
	}
}

func TestPerplexity_MalformedSuccessBodyIsNotEchoed(t *testing.T) {
	t.Parallel()
	const bodyMarker = "BODY-TEXT-MARKER-91c2"
	helper, _, _, _ := perplexityTestServer(t, http.StatusOK, bodyMarker+" not json")
	tool := newPerplexityTool(helper, "key", perplexityDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodePerplexityEnvelope(t, out)
	if env.Error != "perplexity: upstream returned a body that is not valid JSON" {
		t.Fatalf("_ERROR = %q", env.Error)
	}
	if strings.Contains(out, bodyMarker) {
		t.Fatalf("tool output leaks the body: %s", out)
	}
}

func TestPerplexity_ControlCharactersInKeyAreRejectedBeforeRequest(t *testing.T) {
	t.Parallel()
	const keyMarker = "pplx-SECRET-KEY-MARKER-7f3a"
	for name, key := range map[string]string{
		// Control characters sit inside the key so TrimSpace cannot remove them.
		"newline": keyMarker + "\nX-Injected: 1",
		"cr":      "pplx-\r" + keyMarker,
		"nul":     "pplx-\x00" + keyMarker,
		"tab":     "pplx-\t" + keyMarker,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := perplexityTestServer(t, http.StatusOK, `{"results":[]}`)
			tool := newPerplexityTool(helper, key, perplexityDefaults{})

			out, err := tool.InvokableRun(t.Context(), `{"query":"q"}`)
			if err != nil {
				t.Fatalf("InvokableRun: %v", err)
			}
			env := decodePerplexityEnvelope(t, out)
			if env.Error != "perplexity: api_key contains control characters" {
				t.Fatalf("_ERROR = %q", env.Error)
			}
			if strings.Contains(out, keyMarker) {
				t.Fatalf("tool output leaks the key: %s", out)
			}
			if calls.Load() != 0 {
				t.Fatalf("request was sent %d times, want 0", calls.Load())
			}
		})
	}
}

func TestPerplexity_TrimsResultsAndKeepsFields(t *testing.T) {
	t.Parallel()
	helper, _, _, _ := perplexityTestServer(t, http.StatusOK, `{
		"id": "search_test",
		"results": [
			{"title": "A", "url": "https://example.com/a", "snippet": "first", "date": "2026-01-01", "last_updated": "2026-02-01"},
			{"title": "B", "url": "https://example.com/b", "snippet": "second"},
			{"title": "C", "url": "https://example.com/c", "snippet": "third"}
		],
		"server_time": "2026-03-03T00:00:00Z"
	}`)
	tool := newPerplexityTool(helper, "key", perplexityDefaults{})

	out, err := tool.InvokableRun(t.Context(), `{"query":"q","max_results":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodePerplexityEnvelope(t, out)
	if len(env.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(env.Results))
	}
	if env.Results[0]["date"] != "2026-01-01" || env.Results[0]["last_updated"] != "2026-02-01" {
		t.Errorf("date fields not preserved: %#v", env.Results[0])
	}
	if _, ok := env.Results[1]["date"]; ok {
		t.Errorf("date invented for a result without one: %#v", env.Results[1])
	}
}

func TestPerplexity_ComponentOutputsAndReferences(t *testing.T) {
	t.Parallel()
	tool := NewPerplexityTool()
	envelope := map[string]any{
		"results": []any{
			map[string]any{"title": "RAGFlow", "url": "https://ragflow.io", "snippet": "  An open-source   RAG engine. ", "date": "2026-01-01"},
			map[string]any{"title": "Blank", "url": "https://example.com/blank", "snippet": "   "},
			"not a map",
		},
	}

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("chunks = %d, docAggs = %d, want 1 and 1 (blank snippet dropped)", len(chunks), len(docAggs))
	}
	if chunks[0]["content"] != "An open-source RAG engine." {
		t.Errorf("content = %#v, want whitespace collapsed", chunks[0]["content"])
	}
	if chunks[0]["url"] != "https://ragflow.io" || chunks[0]["document_name"] != "RAGFlow" {
		t.Errorf("chunk = %#v", chunks[0])
	}
	if docAggs[0]["doc_id"] != chunks[0]["doc_id"] {
		t.Errorf("docAgg doc_id = %v, chunk doc_id = %v", docAggs[0]["doc_id"], chunks[0]["doc_id"])
	}

	outputs := tool.BuildComponentOutputs(envelope)
	if results, _ := outputs["json"].([]any); len(results) != 3 {
		t.Errorf("json output = %#v, want the raw result list", outputs["json"])
	}
	formalized, _ := outputs["formalized_content"].(string)
	if !strings.Contains(formalized, "Title: RAGFlow") || !strings.Contains(formalized, "URL: https://ragflow.io") {
		t.Errorf("formalized_content = %q", formalized)
	}
}

func TestBuildPerplexityTool_Params(t *testing.T) {
	t.Parallel()
	built, err := buildPerplexityTool(map[string]any{
		"api_key":               "k",
		"search_type":           " Fast ",
		"search_recency_filter": "Week",
		"search_domain_filter":  "-pinterest.com, -reddit.com",
		"country":               "us",
		"top_n":                 float64(20),
	})
	if err != nil {
		t.Fatalf("buildPerplexityTool: %v", err)
	}
	tool, ok := built.(*PerplexityTool)
	if !ok {
		t.Fatalf("built %T, want *PerplexityTool", built)
	}
	if tool.defaults.SearchType != "fast" || tool.defaults.SearchRecencyFilter != "week" || tool.defaults.Country != "US" || tool.defaults.TopN != 20 {
		t.Errorf("defaults = %#v", tool.defaults)
	}
	if len(tool.defaults.SearchDomainFilter) != 2 {
		t.Errorf("domain filter = %v", tool.defaults.SearchDomainFilter)
	}

	bad := []map[string]any{
		{"api_key": 1},
		{"search_type": "people"},
		{"search_recency_filter": "decade"},
		{"search_domain_filter": 3},
		{"search_domain_filter": "ragflow.io, -example.com"},
		{"country": 1},
		{"top_n": float64(0)},
		{"top_n": float64(21)},
		{"top_n": "ten"},
	}
	for _, params := range bad {
		if _, err := buildPerplexityTool(params); err == nil {
			t.Errorf("buildPerplexityTool(%v) accepted invalid params", params)
		}
	}
}

// TestBuildPerplexityTool_CanvasFixtureParams builds the same params the
// PerplexitySearch component fixture in
// internal/agent/component/tool_component_test.go uses, including the
// "outputs" key the Canvas factory forwards, so the two fixtures cannot drift.
func TestBuildPerplexityTool_CanvasFixtureParams(t *testing.T) {
	t.Parallel()
	built, err := buildPerplexityTool(map[string]any{
		"api_key":               "stored-key",
		"search_type":           "fast",
		"search_recency_filter": "week",
		"search_domain_filter":  "ragflow.io, docs.perplexity.ai",
		"country":               "us",
		"top_n":                 float64(3),
		"outputs":               map[string]any{"formalized_content": map[string]any{}, "json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("buildPerplexityTool: %v", err)
	}
	tool, ok := built.(*PerplexityTool)
	if !ok {
		t.Fatalf("built %T, want *PerplexityTool", built)
	}
	if tool.defaults.TopN != 3 || tool.defaults.Country != "US" {
		t.Errorf("defaults = %#v", tool.defaults)
	}
	if got := tool.defaults.SearchDomainFilter; len(got) != 2 || got[0] != "ragflow.io" || got[1] != "docs.perplexity.ai" {
		t.Errorf("domain filter = %v", got)
	}
}
