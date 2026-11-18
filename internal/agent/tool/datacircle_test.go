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

// datacircleTestServer records the last request and answers with the given
// status and body.
func datacircleTestServer(t *testing.T, status int, respBody string) (*HTTPHelper, *http.Request, *atomic.Int32) {
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

func decodeDatacircleEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

const datacircleTestPlans = `{"total":2,"offset":0,"plans":[
	{"provider":"Acme","plan":"Growth","api":"Employee email","call":"Find email","rank":1,"score":76.3,"perThousand":4.99,
	 "billing":"monthly","pay":49,"currency":"USD","calls":10000,"strengths":["under $5 per 1,000","found live"],
	 "use":"long terms of use","fields":["email"],"page":"https://datacircle.dev/find-email-apis/acme/growth"},
	{"provider":"Beta","plan":"Free","api":"Employee email","rank":null,"score":null,"perThousand":0,"units":15000000,
	 "page":"https://datacircle.dev/find-email-apis/beta/free"}]}`

func TestDatacircle_SearchesPlans(t *testing.T) {
	t.Parallel()
	helper, req, _ := datacircleTestServer(t, http.StatusOK, datacircleTestPlans)
	tool := newDatacircleTool(helper, datacircleDefaults{Sort: "price", TopN: 5})

	out, err := tool.InvokableRun(t.Context(), `{"api":" Employee email ","query":"acme"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if req.Method != http.MethodGet || req.URL.Path != "/v1/plans" {
		t.Errorf("request = %s %s, want GET /v1/plans", req.Method, req.URL.Path)
	}
	query := req.URL.Query()
	want := map[string]string{"api": "Employee email", "query": "acme", "sort": "price", "limit": "5"}
	for k, v := range want {
		if got := query.Get(k); got != v {
			t.Errorf("query %s = %q, want %q", k, got, v)
		}
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, the API takes no key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow datacircle-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}

	env := decodeDatacircleEnvelope(t, out)
	plans, _ := env["plans"].([]any)
	if env["total"] != float64(2) || len(plans) != 2 {
		t.Fatalf("envelope = %#v, want total 2 and 2 plans", env)
	}
	first, _ := plans[0].(map[string]any)
	if first["provider"] != "Acme" || first["page"] != "https://datacircle.dev/find-email-apis/acme/growth" {
		t.Errorf("plan 0 = %#v", first)
	}
	if _, kept := first["use"]; kept {
		t.Error("plan kept its terms of use; only the fields that compare plans go to the model")
	}
	second, _ := plans[1].(map[string]any)
	if _, kept := second["rank"]; kept {
		t.Error("a null rank must be left out")
	}
}

func TestDatacircle_ModelArgsOverrideNodeDefaults(t *testing.T) {
	t.Parallel()
	helper, req, _ := datacircleTestServer(t, http.StatusOK, `{"total":0,"plans":[]}`)
	tool := NewDatacircleToolWith(helper)

	out, err := tool.InvokableRun(t.Context(), `{"api":"email-finder","sort":"Provider","max_results":"7"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if query := req.URL.Query(); query.Get("sort") != "provider" || query.Get("limit") != "7" || query.Has("query") {
		t.Errorf("query = %v, want sort=provider, limit=7 and no query", query)
	}
	env := decodeDatacircleEnvelope(t, out)
	if plans, ok := env["plans"].([]any); !ok || len(plans) != 0 {
		t.Errorf("plans = %#v, want an empty list", env["plans"])
	}
}

func TestDatacircle_NodeAPIIsTheDefault(t *testing.T) {
	t.Parallel()
	helper, req, _ := datacircleTestServer(t, http.StatusOK, `{"total":0,"plans":[]}`)
	tool := newDatacircleTool(helper, datacircleDefaults{API: "email-finder"})

	if _, err := tool.InvokableRun(t.Context(), `{"query":"hunter"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if query := req.URL.Query(); req.URL.Path != "/v1/plans" || query.Get("api") != "email-finder" || query.Get("query") != "hunter" {
		t.Errorf("request = %s?%v, want the node's API and the query", req.URL.Path, query)
	}
	if _, err := tool.InvokableRun(t.Context(), `{"api":"phone-finder"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got := req.URL.Query().Get("api"); got != "phone-finder" {
		t.Errorf("api = %q, want the call's own API over the node's", got)
	}
}

func TestDatacircle_ListsAPIsWithoutAPIOrQuery(t *testing.T) {
	t.Parallel()
	helper, req, _ := datacircleTestServer(t, http.StatusOK,
		`[{"api":"Employee email","slug":"email-finder","plans":5489,"providers":402,"sheetPage":"https://datacircle.dev/email-finder-api-pricing","bestPage":"https://datacircle.dev/best-email-finder-api","best":{"provider":"Acme","plan":"Growth","score":76.3,"perThousand":4.99}}]`)

	out, err := NewDatacircleToolWith(helper).InvokableRun(t.Context(), `{"api":"  ","sort":"price"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if req.URL.Path != "/v1/apis" || req.URL.RawQuery != "" {
		t.Errorf("request = %s?%s, want /v1/apis", req.URL.Path, req.URL.RawQuery)
	}
	env := decodeDatacircleEnvelope(t, out)
	apis, _ := env["apis"].([]any)
	if len(apis) != 1 {
		t.Fatalf("envelope = %#v, want the API list", env)
	}
}

func TestDatacircle_MaxResultsArg(t *testing.T) {
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
		{float64(99), 50}, // clamped to the API's max
	}
	for _, tc := range cases {
		if got := datacircleMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("datacircleMaxResultsArg(%#v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestDatacircle_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, args, body, wantErr string
		status                    int
		wantCalls                 int32
	}{
		{"unknown sort", `{"api":"x","sort":"cheapest"}`, `{}`, `sort "cheapest" is not supported`, http.StatusOK, 0},
		{"bad arguments", `{"api":`, `{}`, "parse arguments", http.StatusOK, 0},
		{"unknown api", `{"api":"email finder"}`, `{"error":"api must be one of: Employee profile, Employee email"}`,
			"upstream returned 400: api must be one of: Employee profile, Employee email", http.StatusBadRequest, 1},
		{"rate limited", `{}`, `not json`, "upstream returned 429", http.StatusTooManyRequests, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, calls := datacircleTestServer(t, tc.status, tc.body)
			out, err := NewDatacircleToolWith(helper).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if got, _ := decodeDatacircleEnvelope(t, out)["_ERROR"].(string); !strings.Contains(got, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", got, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestDatacircle_ComponentReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	helper, _, _ := datacircleTestServer(t, http.StatusOK, datacircleTestPlans)
	tool := NewDatacircleToolWith(helper)
	out, err := tool.InvokableRun(t.Context(), `{"api":"Employee email"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	envelope := decodeDatacircleEnvelope(t, out)

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 || len(docAggs) != 2 {
		t.Fatalf("got %d chunks / %d docs, want 2 / 2", len(chunks), len(docAggs))
	}
	if chunks[0]["url"] != "https://datacircle.dev/find-email-apis/acme/growth" || docAggs[0]["doc_name"] != "Acme, Growth (Employee email)" {
		t.Errorf("chunk 0 = %#v / %#v", chunks[0], docAggs[0])
	}
	content, _ := chunks[0]["content"].(string)
	for _, want := range []string{"Provider: Acme", "Rank: 1", "Score out of 100: 76.3", "Price per 1,000 in USD: 4.99", "Pay: 49 USD", "Strengths: under $5 per 1,000, found live"} {
		if !strings.Contains(content, want) {
			t.Errorf("chunk 0 content missing %q:\n%s", want, content)
		}
	}
	if second, _ := chunks[1]["content"].(string); !strings.Contains(second, "Price per 1,000 in USD: 0") || strings.Contains(second, "Rank:") {
		t.Errorf("chunk 1 content = %q, want a 0 price and no rank", second)
	}

	outputs := tool.BuildComponentOutputs(envelope)
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"├── Title: Acme, Growth (Employee email)", "├── URL: https://datacircle.dev/find-email-apis/beta/free"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("formalized_content missing %q:\n%s", want, rendered)
		}
	}
	if raw, _ := outputs["json"].([]any); len(raw) != 2 {
		t.Errorf("json output = %#v, want the plan list", outputs["json"])
	}
}

func TestDatacircle_APIReferencesLinkTheirPages(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"apis": []any{
		map[string]any{"api": "Employee email", "slug": "email-finder", "plans": float64(5489), "providers": float64(402),
			"sheetPage": "https://datacircle.dev/email-finder-api-pricing", "bestPage": "https://datacircle.dev/best-email-finder-api",
			"best": map[string]any{"provider": "Acme", "plan": "Growth", "score": 76.3, "perThousand": 4.99}},
		map[string]any{"api": "Domain WHOIS", "slug": "domain-whois", "plans": float64(12),
			"sheetPage": "https://datacircle.dev/domain-whois-api-pricing", "bestPage": nil},
		"not an api",
	}}
	tool := NewDatacircleTool()

	chunks, _ := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0]["url"] != "https://datacircle.dev/best-email-finder-api" || chunks[1]["url"] != "https://datacircle.dev/domain-whois-api-pricing" {
		t.Errorf("urls = %v, %v; want the best page, else the sheet", chunks[0]["url"], chunks[1]["url"])
	}
	if content, _ := chunks[0]["content"].(string); !strings.Contains(content, "Plans: 5489") || !strings.Contains(content, "Best provider: Acme") {
		t.Errorf("chunk 0 content = %q", content)
	}
	if raw, _ := tool.BuildComponentOutputs(envelope)["json"].([]any); len(raw) != 3 {
		t.Errorf("json output = %#v, want the API list", raw)
	}
}

func TestDatacircle_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("Datacircle", map[string]any{
		"sort": "price", "top_n": float64(4),
		"api": "Employee email", "query": "{sys.query}", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*DatacircleTool)
	if !ok {
		t.Fatalf("built %T, want *DatacircleTool", built)
	}
	if tool.defaults != (datacircleDefaults{API: "Employee email", Sort: "price", TopN: 4}) {
		t.Errorf("defaults = %+v", tool.defaults)
	}
	info, err := tool.Info(t.Context())
	if err != nil || info.Name != datacircleToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
}

func TestDatacircle_BuildByNameRejectsInvalidNodeConfig(t *testing.T) {
	t.Parallel()
	for name, params := range map[string]map[string]any{
		"api not string":   {"api": 1},
		"unknown sort":     {"sort": "cheapest"},
		"sort not string":  {"sort": 1},
		"zero top_n":       {"top_n": float64(0)},
		"top_n over 50":    {"top_n": float64(51)},
		"fractional top_n": {"top_n": 2.5},
	} {
		if _, err := BuildByName("datacircle", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
