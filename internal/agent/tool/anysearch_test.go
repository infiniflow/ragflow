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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

// anySearchSeen is one request as the test server received it.
type anySearchSeen struct {
	Method, Path, RawQuery, Authorization, ContentType string
	HasAuthorization                                   bool
	Body                                               []byte
}

type anySearchRecorder struct {
	mu   sync.Mutex
	seen []anySearchSeen
}

func (r *anySearchRecorder) all() []anySearchSeen {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]anySearchSeen(nil), r.seen...)
}

// anySearchTestServer serves every request through handler and records it.
func anySearchTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body []byte)) (*HTTPHelper, *anySearchRecorder) {
	t.Helper()
	rec := &anySearchRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		_, has := r.Header["Authorization"]
		rec.mu.Lock()
		rec.seen = append(rec.seen, anySearchSeen{
			Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery,
			Authorization: r.Header.Get("Authorization"), HasAuthorization: has,
			ContentType: r.Header.Get("Content-Type"), Body: body.Bytes(),
		})
		rec.mu.Unlock()
		handler(w, r, body.Bytes())
	}))
	t.Cleanup(srv.Close)
	return NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(srv.URL)}), rec
}

func writeAnySearch(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// anySearchOK answers every request with a successful envelope holding results.
func anySearchOK(results string) func(http.ResponseWriter, *http.Request, []byte) {
	return func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeAnySearch(w, http.StatusOK, `{"code":0,"message":"success","request_id":"rid-ok","data":{"results":`+results+`}}`)
	}
}

func decodeAnySearchEnvelope(t *testing.T, out string) anySearchSearchEnvelope {
	t.Helper()
	var env anySearchSearchEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func decodeAnySearchBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode request body %q: %v", raw, err)
	}
	return body
}

func TestAnySearch_AnonymousSendsNoAuthorizationHeader(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"", "   ", "\t\n"} {
		helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
		out, err := NewAnySearchToolWith(helper, key).InvokableRun(t.Context(), `{"query":"golang"}`)
		if err != nil {
			t.Fatalf("key %q: InvokableRun: %v", key, err)
		}
		if env := decodeAnySearchEnvelope(t, out); env.Error != "" {
			t.Fatalf("key %q: unexpected _ERROR %q", key, env.Error)
		}
		seen := rec.all()
		if len(seen) != 1 {
			t.Fatalf("key %q: %d requests, want 1", key, len(seen))
		}
		if seen[0].HasAuthorization {
			t.Errorf("key %q: Authorization header present (%q), want absent", key, seen[0].Authorization)
		}
		if seen[0].Method != http.MethodPost || seen[0].Path != "/v1/search" || seen[0].ContentType != "application/json" {
			t.Errorf("key %q: request = %s %s (%s)", key, seen[0].Method, seen[0].Path, seen[0].ContentType)
		}
		body := decodeAnySearchBody(t, seen[0].Body)
		if len(body) != 2 || body["query"] != "golang" || body["max_results"] != json.Number("10") {
			t.Errorf("key %q: body = %v, want only query and the default max_results 10", key, body)
		}
	}
}

func TestAnySearch_KeyedSendsTrimmedBearer(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
	if _, err := NewAnySearchToolWith(helper, " key-xyz ").InvokableRun(t.Context(), `{"query":"q"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	seen := rec.all()
	if len(seen) != 1 || seen[0].Authorization != "Bearer key-xyz" {
		t.Fatalf("requests = %+v, want one with Authorization %q", seen, "Bearer key-xyz")
	}
}

func TestAnySearch_FlightStatusParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, params, departure, wantErr string
	}{
		{"airport omitted", `{"flight_number":"CA981"}`, "", ""},
		{"airport empty", `{"flight_number":"CA981","departure":""}`, "", ""},
		{"airport supplied", `{"flight_number":"CA981","departure":"PEK"}`, "PEK", ""},
		{"flight missing", `{"departure":"PEK"}`, "", "flight_number"},
		{"flight blank", `{"flight_number":"  "}`, "", "flight_number"},
		{"flight null", `{"flight_number":null}`, "", "flight_number"},
		{"airport null", `{"flight_number":"CA981","departure":null}`, "", "departure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
			out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), `{"query":"CA981 flight status","tag":"travel.flight_status","params":`+tc.params+`}`)
			if err != nil {
				t.Fatal(err)
			}
			env := decodeAnySearchEnvelope(t, out)
			if tc.wantErr != "" {
				if !strings.Contains(env.Error, tc.wantErr) || len(rec.all()) != 0 {
					t.Fatalf("want a %s parameter error without a request; result=%s requests=%d", tc.wantErr, out, len(rec.all()))
				}
				return
			}
			if env.Error != "" || len(rec.all()) != 1 {
				t.Fatalf("result=%s requests=%d", out, len(rec.all()))
			}
			params := decodeAnySearchBody(t, rec.all()[0].Body)["params"].(map[string]any)
			if params["flight_number"] != "CA981" || params["departure"] != tc.departure {
				t.Fatalf("request params=%#v, want flight_number CA981 and departure %q", params, tc.departure)
			}
		})
	}
}

func TestAnySearch_ReferencesUseSharedCitationIDs(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"queries": []any{map[string]any{"query": "RAGFlow", "results": []any{
		map[string]any{"title": "RAGFlow", "url": "https://ragflow.io/", "content": "Native agent references."},
	}}}}
	search := NewAnySearchTool()
	chunks, docs := search.BuildReferences(t.Context(), envelope)
	if len(chunks) != 1 || len(docs) != 1 {
		t.Fatalf("references=%#v docs=%#v", chunks, docs)
	}
	documentID, displayID := referenceIDs("https://ragflow.io/", "Native agent references.")
	if chunks[0]["id"] != displayID || chunks[0]["chunk_id"] != documentID || chunks[0]["doc_id"] != documentID || docs[0]["doc_id"] != documentID {
		t.Fatalf("reference IDs do not follow the shared citation contract: chunk=%#v doc=%#v", chunks[0], docs[0])
	}
	rendered := search.BuildComponentOutputs(envelope)["formalized_content"].(string)
	if !strings.Contains(rendered, "ID: "+displayID) {
		t.Fatalf("prompt has no shared display ID: %q", rendered)
	}
}

func TestAnySearch_BodyMappingAndNodeDefaults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, args string
		want       map[string]any
	}{
		{"node defaults", `{"query":"q"}`,
			map[string]any{"query": "q", "max_results": json.Number("7"), "zone": "intl", "language": "en"}},
		{"model overrides node", `{"query":"q","zone":"cn","language":"zh-CN","max_results":3}`,
			map[string]any{"query": "q", "max_results": json.Number("3"), "zone": "cn", "language": "zh-CN"}},
		{"vertical with params", `{"query":"AAPL","tag":" finance.quote ","params":{"type":"stock","symbol":"AAPL","cn_code":""}}`,
			map[string]any{"query": "AAPL", "tag": "finance.quote", "max_results": json.Number("7"), "zone": "intl", "language": "en",
				"params": map[string]any{"type": "stock", "symbol": "AAPL", "cn_code": ""}}},
		{"large integers stay exact", `{"query":"q","tag":"x.y","params":{"id":9007199254740993}}`,
			map[string]any{"query": "q", "tag": "x.y", "max_results": json.Number("7"), "zone": "intl", "language": "en",
				"params": map[string]any{"id": json.Number("9007199254740993")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
			tool := newAnySearchTool(helper, "", anySearchDefaults{TopN: 7, Zone: "intl", Language: "en"})
			out, err := tool.InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("InvokableRun: %v", err)
			}
			if env := decodeAnySearchEnvelope(t, out); env.Error != "" {
				t.Fatalf("unexpected _ERROR %q", env.Error)
			}
			seen := rec.all()
			if len(seen) != 1 {
				t.Fatalf("%d requests, want 1", len(seen))
			}
			got := decodeAnySearchBody(t, seen[0].Body)
			if gotJSON, wantJSON := mustJSON(t, got), mustJSON(t, tc.want); gotJSON != wantJSON {
				t.Errorf("body = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestAnySearch_MaxResultsArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value any
		want  int
	}{
		{nil, 7},
		{"", 7},
		{"lots", 7},
		{json.Number("3"), 3},
		{"5", 5},
		{json.Number("0"), 1},
		{json.Number("-5"), 1},
		{json.Number("99"), 10},
		{json.Number("2.9"), 2},
		{json.Number("1e30"), 10},
		{json.Number("-1e30"), 1},
	}
	for _, tc := range cases {
		if got := anySearchMaxResultsArg(tc.value, 7); got != tc.want {
			t.Errorf("anySearchMaxResultsArg(%#v, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestAnySearch_ArgumentErrorsNeverReachTheAPI(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, args, wantErr string }{
		{"both query and queries", `{"query":"a","queries":[{"query":"b"}]}`, "either query or queries"},
		{"too many queries", `{"queries":[{"query":"1"},{"query":"2"},{"query":"3"},{"query":"4"},{"query":"5"},{"query":"6"}]}`, "at most 5"},
		{"params not an object", `{"query":"q","tag":"a.b","params":"library=react"}`, "params must be an object"},
		{"params without tag", `{"query":"q","params":{"a":"b"}}`, "params require a tag"},
		{"bad zone", `{"query":"q","zone":"mars"}`, `zone "mars"`},
		{"bad zone inside queries", `{"queries":[{"query":"q","zone":"mars"}]}`, `zone "mars"`},
		{"unparseable arguments", `{"query":`, "parse arguments"},
		{"trailing data", `{"query":"a"}{"query":"b"}`, "trailing data"},
		{"empty queries array", `{"queries":[]}`, "at least one item"},
		{"tag without a query", `{"tag":"code.doc"}`, "query is required"},
		{"blank query with a tag", `{"query":"","tag":"code.doc","params":{"library":"react"}}`, "query is required"},
		{"max_results without a query", `{"max_results":3}`, "query is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
			out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeAnySearchEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if n := len(rec.all()); n != 0 {
				t.Errorf("%d upstream requests, want 0", n)
			}
		})
	}
}

func TestAnySearch_EmptyQueryMakesNoRequest(t *testing.T) {
	t.Parallel()
	for _, args := range []string{``, `  `, `{}`, `null`, `{"query":"  "}`} {
		helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
		out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), args)
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		if env := decodeAnySearchEnvelope(t, out); env.Error != "" || len(env.Queries) != 0 {
			t.Errorf("%s: envelope = %+v, want empty and no error", args, env)
		}
		if n := len(rec.all()); n != 0 {
			t.Errorf("%s: %d requests, want 0", args, n)
		}
	}
}

func TestAnySearch_ResponseStrictness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string // substring of the per-query error; empty means success
		calls   int
	}{
		{"genuine empty result", 200, `{"code":0,"request_id":"r","data":{"results":[]}}`, "", 1},
		{"vendor 400 with message", 400, `{"code":-1,"message":"Invalid tag: nope.nope.","request_id":"rid-400"}`, "HTTP 400: Invalid tag: nope.nope. (request_id rid-400)", 1},
		{"401", 401, `{"code":401,"message":"invalid api key","request_id":"rid-401"}`, "HTTP 401", 1},
		{"429", 429, `{"code":429,"message":"quota exceeded","request_id":"rid-429"}`, "HTTP 429", 1},
		{"500 is not retried", 500, `{"message":"boom"}`, "HTTP 500", 1},
		{"non-JSON error body", 502, `<html>bad gateway</html>`, "HTTP 502", 1},
		{"missing code", 200, `{"data":{"results":[]}}`, "missing business status", 1},
		{"string code", 200, `{"code":"0","data":{"results":[]}}`, "missing business status", 1},
		{"fractional code", 200, `{"code":0.5,"data":{"results":[]}}`, "missing business status", 1},
		{"business failure on 200", 200, `{"code":1002,"message":"quota","request_id":"rid-b","data":{"results":[]}}`, "business error 1002: quota (request_id rid-b)", 1},
		{"null data", 200, `{"code":0,"data":null}`, "missing data", 1},
		{"array data", 200, `{"code":0,"data":[]}`, "missing data", 1},
		{"missing results", 200, `{"code":0,"data":{}}`, "invalid results", 1},
		{"results not an array", 200, `{"code":0,"data":{"results":{}}}`, "invalid results", 1},
		{"result item not an object", 200, `{"code":0,"data":{"results":["x"]}}`, "invalid results", 1},
		{"invalid JSON", 200, `{"code":0,`, "invalid JSON", 1},
		{"trailing data", 200, `{"code":0,"data":{"results":[]}} {}`, "invalid JSON", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
				writeAnySearch(w, tc.status, tc.body)
			})
			out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"q"}`)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			env := decodeAnySearchEnvelope(t, out)
			if tc.wantErr == "" {
				if env.Error != "" || len(env.Queries) != 1 || env.Queries[0].Error != "" || env.Queries[0].Results == nil || len(env.Queries[0].Results) != 0 {
					t.Fatalf("envelope = %+v, want one successful query with an empty (non-null) result list", env)
				}
			} else {
				// A single failed query means every query failed, so the whole call is an error.
				if len(env.Queries) != 1 || !strings.Contains(env.Queries[0].Error, tc.wantErr) {
					t.Fatalf("query error = %+v, want it to contain %q", env.Queries, tc.wantErr)
				}
				if env.Error == "" {
					t.Errorf("top-level _ERROR empty although every query failed: %s", out)
				}
			}
			if n := len(rec.all()); n != tc.calls {
				t.Errorf("%d upstream requests, want %d (no retry of a POST)", n, tc.calls)
			}
		})
	}
}

func TestAnySearch_OversizedResponseIsRejected(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"results":[]},"pad":"`))
		_, _ = w.Write(bytes.Repeat([]byte("a"), anySearchMaxResponseBytes+16))
		_, _ = w.Write([]byte(`"}`))
	})
	out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeAnySearchEnvelope(t, out); len(env.Queries) != 1 || !strings.Contains(env.Queries[0].Error, "4 MiB") {
		t.Fatalf("envelope = %s, want a 4 MiB limit error", out)
	}
}

func TestAnySearch_RedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		http.Redirect(w, r, "https://elsewhere.invalid/steal", http.StatusFound)
	})
	out, err := NewAnySearchToolWith(helper, "secret-key").InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeAnySearchEnvelope(t, out); len(env.Queries) != 1 || !strings.Contains(env.Queries[0].Error, "HTTP 302") {
		t.Fatalf("envelope = %s, want an HTTP 302 error", out)
	}
	if n := len(rec.all()); n != 1 {
		t.Errorf("%d requests, want 1: the redirect must not be followed", n)
	}
}

func TestAnySearch_ErrorsNeverLeakTheKeyOrTheTransport(t *testing.T) {
	t.Parallel()
	const key = "secret-key-123"
	t.Run("vendor message is redacted and truncated", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("é", 500)
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			writeAnySearch(w, 400, `{"code":-1,"message":"bad key `+key+` `+long+`","request_id":"rid-1"}`)
		})
		out, _ := NewAnySearchToolWith(helper, key).InvokableRun(t.Context(), `{"query":"q"}`)
		if strings.Contains(out, key) {
			t.Fatalf("the key leaked into the result: %s", out)
		}
		env := decodeAnySearchEnvelope(t, out)
		msg := env.Queries[0].Error
		if !strings.Contains(msg, "[redacted]") || !strings.Contains(msg, "rid-1") || !utf8.ValidString(msg) {
			t.Errorf("error = %q, want a redacted, valid UTF-8 message with the request id", msg)
		}
		if utf8.RuneCountInString(msg) > 400 {
			t.Errorf("error has %d runes, want the vendor message truncated", utf8.RuneCountInString(msg))
		}
	})
	t.Run("transport failure hides the address", func(t *testing.T) {
		t.Parallel()
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("recorder cannot hijack")
				return
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		})
		out, _ := NewAnySearchToolWith(helper, key).InvokableRun(t.Context(), `{"query":"find me"}`)
		env := decodeAnySearchEnvelope(t, out)
		if len(env.Queries) != 1 || !strings.Contains(env.Queries[0].Error, "request failed") {
			t.Fatalf("envelope = %s, want a generic request failure", out)
		}
		// The record echoes its query so results stay associated; the error text must not.
		for _, text := range []string{env.Error, env.Queries[0].Error} {
			for _, leak := range []string{key, "127.0.0.1", "find me", "http://"} {
				if strings.Contains(text, leak) {
					t.Errorf("error %q leaks %q", text, leak)
				}
			}
		}
	})
}

func TestAnySearch_CancelledContextReturnsPromptly(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan string, 1)
	go func() {
		out, _ := NewAnySearchToolWith(helper, "key").InvokableRun(ctx, `{"query":"q"}`)
		done <- out
	}()
	select {
	case out := <-done:
		if env := decodeAnySearchEnvelope(t, out); len(env.Queries) != 1 || !strings.Contains(env.Queries[0].Error, "canceled") {
			t.Errorf("envelope = %s, want a canceled error", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled context did not return")
	}
}

func TestAnySearch_BatchRunsInParallelKeepsOrderAndPartialFailure(t *testing.T) {
	t.Parallel()
	var arrived sync.WaitGroup
	arrived.Add(3)
	release := make(chan struct{})
	go func() { arrived.Wait(); close(release) }()
	helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, body []byte) {
		arrived.Done()
		select {
		case <-release: // all three requests are in flight at once
		case <-time.After(5 * time.Second):
			writeAnySearch(w, 504, `{}`)
			return
		}
		switch decodeAnySearchBodyQuery(body) {
		case "second":
			writeAnySearch(w, 400, `{"code":-1,"message":"Invalid tag: bad.tag.","request_id":"rid-2"}`)
		case "first":
			writeAnySearch(w, 200, `{"code":0,"request_id":"rid-1","data":{"results":[{"title":"One","url":"https://one.example","content":"first text"}]}}`)
		default:
			writeAnySearch(w, 200, `{"code":0,"request_id":"rid-3","data":{"results":[{"title":"Three","url":"https://three.example","snippet":"third snippet"}]}}`)
		}
	})
	out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(),
		`{"queries":[{"query":"first"},{"query":"second","tag":"bad.tag"},{"query":"third"}]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if env.Error != "" {
		t.Errorf("top-level _ERROR %q: a partial failure must not hide the successful results", env.Error)
	}
	if len(env.Queries) != 3 {
		t.Fatalf("%d query records, want 3: %s", len(env.Queries), out)
	}
	for i, want := range []string{"first", "second", "third"} {
		if env.Queries[i].Index != i || env.Queries[i].Query != want {
			t.Errorf("record %d = %+v, want index %d query %q (input order)", i, env.Queries[i], i, want)
		}
	}
	if env.Queries[0].Error != "" || len(env.Queries[0].Results) != 1 || env.Queries[0].RequestID != "rid-1" {
		t.Errorf("record 0 = %+v", env.Queries[0])
	}
	if !strings.Contains(env.Queries[1].Error, "Invalid tag: bad.tag.") || env.Queries[1].Tag != "bad.tag" {
		t.Errorf("record 1 = %+v, want its own vendor error and tag", env.Queries[1])
	}
	if env.Queries[2].Error != "" || len(env.Queries[2].Results) != 1 {
		t.Errorf("record 2 = %+v, want the third query to survive the second's failure", env.Queries[2])
	}
	if n := len(rec.all()); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
}

type anySearchObservedBody struct {
	io.ReadCloser
	closed chan struct{}
}

func (b *anySearchObservedBody) Close() error {
	err := b.ReadCloser.Close()
	close(b.closed)
	return err
}

func TestAnySearchBatchItemIsolation(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		waiting := make(chan struct{})
		finished := make(chan struct{})
		helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
			if decodeAnySearchBodyQuery(body) == "done" {
				anySearchOK(`[{"url":"https://example.com/done","content":"completed"}]`)(w, r, body)
				return
			}
			close(waiting)
			<-r.Context().Done()
			close(finished)
		})
		completed := make(chan struct{})
		transport := helper.client.Transport
		helper.client.Transport = anySearchRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp, err := transport.RoundTrip(r)
			if err == nil {
				resp.Body = &anySearchObservedBody{ReadCloser: resp.Body, closed: completed}
			}
			return resp, err
		})
		go func() {
			for _, signal := range []chan struct{}{waiting, completed} {
				select {
				case <-signal:
				case <-ctx.Done():
					return
				}
			}
			cancel()
		}()
		out, err := NewAnySearchToolWith(helper, "key").InvokableRun(ctx, `{"queries":[{"query":""},{"query":"done"},{"query":"waiting"}]}`)
		if err != nil {
			t.Fatal(err)
		}
		env := decodeAnySearchEnvelope(t, out)
		if env.Error != "" || len(env.Queries) != 3 || len(rec.all()) != 2 {
			t.Fatalf("output = %s, requests = %d", out, len(rec.all()))
		}
		if env.Queries[0].Index != 0 || !strings.Contains(env.Queries[0].Error, "query is required") || env.Queries[0].RequestID != "" ||
			env.Queries[1].Index != 1 || env.Queries[1].Error != "" || len(env.Queries[1].Results) != 1 ||
			env.Queries[2].Index != 2 || !strings.Contains(env.Queries[2].Error, "canceled") {
			t.Errorf("records = %+v", env.Queries)
		}
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("canceled HTTP handler did not finish")
		}
	})
	t.Run("order", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		arrived := make(chan struct{}, 3)
		release := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
		finished := make(chan int, 3)
		helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, body []byte) {
			query := decodeAnySearchBodyQuery(body)
			i := int(query[0] - '0')
			arrived <- struct{}{}
			select {
			case <-release[i]:
				anySearchOK(`[{"url":"https://example.com/`+query+`","content":"content `+query+`"}]`)(w, nil, body)
				finished <- i
			case <-ctx.Done():
			}
		})
		go func() {
			for range 3 {
				select {
				case <-arrived:
				case <-ctx.Done():
					return
				}
			}
			for _, i := range []int{2, 0, 1} {
				close(release[i])
				select {
				case <-finished:
				case <-ctx.Done():
					return
				}
			}
		}()
		search := NewAnySearchToolWith(helper, "key")
		out, err := search.InvokableRun(ctx, `{"queries":[{"query":"0"},{"query":"1"},{"query":"2"}]}`)
		if err != nil {
			t.Fatal(err)
		}
		env := decodeAnySearchEnvelope(t, out)
		if env.Error != "" || len(env.Queries) != 3 || len(rec.all()) != 3 {
			t.Fatalf("output = %s, requests = %d", out, len(rec.all()))
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		chunks, _ := search.BuildReferences(t.Context(), envelope)
		if len(chunks) != 3 {
			t.Fatalf("references = %v", chunks)
		}
		for i, query := range []string{"0", "1", "2"} {
			record := env.Queries[i]
			url := "https://example.com/" + query
			if record.Index != i || record.Query != query || record.Error != "" || len(record.Results) != 1 || record.Results[0]["url"] != url || chunks[i]["url"] != url {
				t.Errorf("record %d = %+v, reference = %v", i, record, chunks[i])
			}
		}
	})
	for _, args := range []string{
		`{"queries":[]}`, `{"queries":{}}`, `{"queries":`,
		`{"queries":[{"query":"a"}]} {}`, `{"query":"a","queries":[{"query":"b"}]}`,
		`{"queries":[{},{},{},{},{},{}]}`,
	} {
		t.Run("envelope_"+args, func(t *testing.T) {
			helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
			out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), args)
			if err != nil {
				t.Fatal(err)
			}
			if env := decodeAnySearchEnvelope(t, out); env.Error == "" || len(env.Queries) != 0 || len(rec.all()) != 0 {
				t.Fatalf("output = %s, requests = %d; want envelope rejection without dispatch", out, len(rec.all()))
			}
		})
	}
	t.Run("all_fail", func(t *testing.T) {
		helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, body []byte) {
			if decodeAnySearchBodyQuery(body) == "unauthorized" {
				writeAnySearch(w, 401, `{"code":401,"request_id":"unauthorized"}`)
			} else {
				writeAnySearch(w, 429, `{"code":429,"request_id":"limited"}`)
			}
		})
		out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), `{"queries":[{"query":"unauthorized"},{"query":"limited"}]}`)
		if err != nil {
			t.Fatal(err)
		}
		env := decodeAnySearchEnvelope(t, out)
		if len(env.Queries) != 2 || !strings.Contains(env.Error, "all 2 queries failed") || len(rec.all()) != 2 {
			t.Fatalf("output = %s, requests = %d", out, len(rec.all()))
		}
		for i, record := range env.Queries {
			if record.Index != i || record.Error == "" || record.RequestID != record.Query || len(record.Results) != 0 {
				t.Errorf("record %d = %+v", i, record)
			}
		}
	})
	for _, tc := range []struct {
		name, args, wantError string
		validIndex            int
	}{
		{"local_type", `{"queries":[{"query":"RAGFlow"},{"query":5}]}`, "invalid search fields", 0},
		{"local_null", `{"queries":[null,{"query":"RAGFlow"}]}`, "must be an object", 1},
		{"local_scalar", `{"queries":[5,{"query":"RAGFlow"}]}`, "must be an object", 1},
		{"local_array", `{"queries":[[],{"query":"RAGFlow"}]}`, "must be an object", 1},
		{"local_query", `{"queries":[{"query":""},{"query":"RAGFlow"}]}`, "query is required", 1},
		{"local_zone", `{"queries":[{"query":"bad","zone":"invalid"},{"query":"RAGFlow"}]}`, "zone", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper, rec := anySearchTestServer(t, anySearchOK(`[{"url":"https://ragflow.io","content":"result"}]`))
			out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatal(err)
			}
			env := decodeAnySearchEnvelope(t, out)
			if env.Error != "" || len(env.Queries) != 2 || len(rec.all()) != 1 {
				t.Fatalf("output = %s, requests = %d; want one request and two records without envelope error", out, len(rec.all()))
			}
			for i, record := range env.Queries {
				if record.Index != i {
					t.Errorf("index = %d, want %d", record.Index, i)
				}
				if i == tc.validIndex {
					if record.Query != "RAGFlow" || record.Error != "" || len(record.Results) != 1 {
						t.Errorf("successful sibling = %+v", record)
					}
				} else if !strings.Contains(record.Error, tc.wantError) || record.RequestID != "" || record.Results == nil || len(record.Results) != 0 {
					t.Errorf("local failure = %+v, want %q and empty results without request ID", record, tc.wantError)
				}
			}
		})
	}
	t.Run("all_invalid", func(t *testing.T) {
		helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
		out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), `{"queries":[{"query":""},{"query":5}]}`)
		if err != nil {
			t.Fatal(err)
		}
		env := decodeAnySearchEnvelope(t, out)
		if len(env.Queries) != 2 || !strings.Contains(env.Error, "all 2 queries failed") || len(rec.all()) != 0 {
			t.Fatalf("output = %s, requests = %d", out, len(rec.all()))
		}
		for i, record := range env.Queries {
			if record.Index != i || record.Error == "" || record.Results == nil || len(record.Results) != 0 || record.RequestID != "" {
				t.Errorf("record %d = %+v", i, record)
			}
		}
	})
	t.Run("empty_plus_fail", func(t *testing.T) {
		helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, body []byte) {
			if decodeAnySearchBodyQuery(body) == "empty" {
				anySearchOK(`[]`)(w, nil, body)
			} else {
				writeAnySearch(w, 429, `{"code":429,"request_id":"limited"}`)
			}
		})
		search := NewAnySearchToolWith(helper, "key")
		out, err := search.InvokableRun(t.Context(), `{"queries":[{"query":"empty"},{"query":"limited"}]}`)
		if err != nil {
			t.Fatal(err)
		}
		env := decodeAnySearchEnvelope(t, out)
		if env.Error != "" || len(env.Queries) != 2 || len(rec.all()) != 2 {
			t.Fatalf("output = %s, requests = %d", out, len(rec.all()))
		}
		if env.Queries[0].Error != "" || len(env.Queries[0].Results) != 0 || env.Queries[1].Error == "" || env.Queries[1].RequestID != "limited" {
			t.Errorf("records = %+v", env.Queries)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		chunks, docs := search.BuildReferences(t.Context(), envelope)
		if len(chunks) != 0 || len(docs) != 0 {
			t.Errorf("invented references: %v, %v", chunks, docs)
		}
	})
	t.Run("local_flight", func(t *testing.T) {
		helper, rec := anySearchTestServer(t, anySearchOK(`[{"title":"RAGFlow","url":"https://ragflow.io","content":"RAGFlow search result"}]`))
		search := NewAnySearchToolWith(helper, "key")
		out, err := search.InvokableRun(t.Context(), `{"queries":[{"query":"RAGFlow"},{"query":"flight status","tag":"travel.flight_status","params":{"departure":""}}]}`)
		if err != nil {
			t.Fatalf("InvokableRun: %v", err)
		}
		if n := len(rec.all()); n != 1 {
			t.Errorf("%d upstream requests, want only the valid sibling's request", n)
		}
		env := decodeAnySearchEnvelope(t, out)
		if env.Error != "" || len(env.Queries) != 2 {
			t.Fatalf("envelope = %s, want two records without a top-level error", out)
		}
		if first := env.Queries[0]; first.Index != 0 || first.Query != "RAGFlow" || first.Error != "" || len(first.Results) != 1 {
			t.Errorf("successful sibling = %+v", first)
		}
		if second := env.Queries[1]; second.Index != 1 || !strings.Contains(second.Error, "flight_number") || second.RequestID != "" || len(second.Results) != 0 {
			t.Errorf("invalid flight = %+v", second)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		chunks, docs := search.BuildReferences(t.Context(), envelope)
		if len(chunks) != 1 || len(docs) != 1 || chunks[0]["url"] != "https://ragflow.io" {
			t.Errorf("references = %v, %v, want only the successful sibling's reference", chunks, docs)
		}
	})
}

func decodeAnySearchBodyQuery(body []byte) string {
	var b struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(body, &b)
	return b.Query
}

func TestAnySearch_ResultTextIsCappedAndNormalized(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("é", 5000)
	helper, _ := anySearchTestServer(t, anySearchOK(`[
		{"title":"Both","url":"https://a.example","content":"`+huge+`","snippet":"short","published_date":"2026-10-01"},
		{"title":"Snippet only","url":"https://b.example","snippet":"  just a snippet  "},
		{"title":"Neither","url":"https://c.example"}
	]`))
	out, err := NewAnySearchToolWith(helper, "key").InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	results := decodeAnySearchEnvelope(t, out).Queries[0].Results
	if len(results) != 3 {
		t.Fatalf("%d results, want 3 (unusable ones stay in the list)", len(results))
	}
	content, _ := results[0]["content"].(string)
	if utf8.RuneCountInString(content) != anySearchResultTextRunes || !utf8.ValidString(content) {
		t.Errorf("content has %d runes (valid=%v), want %d valid runes", utf8.RuneCountInString(content), utf8.ValidString(content), anySearchResultTextRunes)
	}
	if results[0]["snippet"] != "short" {
		t.Errorf("snippet = %#v, want it kept next to content: for vertical results it is a separate summary", results[0]["snippet"])
	}
	if results[0]["published_date"] != "2026-10-01" {
		t.Errorf("extra upstream field lost: %#v", results[0])
	}
	if results[1]["snippet"] != "just a snippet" {
		t.Errorf("snippet-only result = %#v, want the trimmed snippet kept", results[1])
	}
}

func TestAnySearch_ReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	envelope := map[string]any{"queries": []any{
		map[string]any{"index": float64(0), "query": "alpha", "tag": "code.doc", "results": []any{
			map[string]any{"title": "Page", "url": "https://p.example/x", "content": "  page\n\n text ", "snippet": "snippet"},
			map[string]any{"title": "Snippet only", "url": "https://s.example", "snippet": "just a snippet"},
			map[string]any{"title": "Blank", "url": "https://e.example"},
			map[string]any{"title": "Script", "url": "javascript:alert(1)", "content": "bad scheme"},
			map[string]any{"title": "Relative", "url": "/local", "content": "relative"},
			map[string]any{"title": "No host", "url": "https://", "content": "no host"},
			"not a result",
		}},
		map[string]any{"index": float64(1), "query": "beta", "_ERROR": "anysearch: HTTP 400: Invalid tag: x.y. (request_id r2)", "results": []any{}},
		map[string]any{"index": float64(2), "query": "gamma", "results": []any{
			map[string]any{"title": "Third", "url": "http://t.example", "content": "third text"},
		}},
	}}
	tool := NewAnySearchTool()

	chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
	if len(chunks) != 3 || len(docAggs) != 3 {
		t.Fatalf("got %d chunks / %d docs, want 3 / 3 (blank text and non-http(s) URLs dropped): %#v", len(chunks), len(docAggs), chunks)
	}
	if chunks[0]["content"] != "page text" || chunks[0]["url"] != "https://p.example/x" || chunks[0]["docnm_kwd"] != "Page" {
		t.Errorf("chunk 0 = %#v, want content with whitespace collapsed, url and title", chunks[0])
	}
	if chunks[1]["content"] != "just a snippet" {
		t.Errorf("chunk 1 content = %q, want the snippet fallback", chunks[1]["content"])
	}
	if chunks[2]["content"] != "third text" || docAggs[2]["doc_name"] != "Third" || docAggs[2]["url"] != "http://t.example" {
		t.Errorf("chunk 2 / doc 2 = %#v / %#v, want the third query's hit to survive the second's failure", chunks[2], docAggs[2])
	}

	outputs := tool.BuildComponentOutputs(envelope)
	rendered, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"Query: alpha", "├── Title: Page", "├── URL: https://p.example/x", "└── Content:\npage text", "just a snippet", "Query: beta", "Error: anysearch: HTTP 400", "Query: gamma", "third text"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("formalized_content missing %q:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"javascript:", "relative", "no host"} {
		if strings.Contains(rendered, unwanted) {
			t.Errorf("formalized_content contains %q:\n%s", unwanted, rendered)
		}
	}
	records, _ := outputs["json"].([]any)
	if len(records) != 3 {
		t.Fatalf("json output = %#v, want the 3 ordered per-query records", outputs["json"])
	}
	if first, _ := records[0].(map[string]any); first["query"] != "alpha" {
		t.Errorf("json[0] = %#v, want query alpha first", records[0])
	}
}

func TestAnySearch_RenderCountsTheWholeBlockBeforeAppending(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("alpha ", 200)
	chunk := func(id string) map[string]any {
		return map[string]any{"id": id, "document_name": "T" + id, "docnm_kwd": "T" + id, "url": "https://x.example/" + id, "content": big}
	}
	queries := []anySearchRenderQuery{{Query: "q", Chunks: []map[string]any{chunk("1"), chunk("2")}}}

	unlimited := renderAnySearchReferences(queries, 0)
	if !strings.Contains(unlimited, "ID: 1") || !strings.Contains(unlimited, "ID: 2") {
		t.Fatalf("no budget should render everything:\n%s", unlimited)
	}
	limited := renderAnySearchReferences(queries, 300)
	if !strings.Contains(limited, "ID: 1") || strings.Contains(limited, "ID: 2") {
		t.Errorf("budget 300 should keep only the first block (a second would exceed it):\n%s", limited)
	}
	if tiny := renderAnySearchReferences(queries, 10); strings.Contains(tiny, "ID: 1") {
		t.Errorf("a block larger than the whole budget must not be appended:\n%s", tiny)
	}
}

func TestAnySearch_InfoSchema(t *testing.T) {
	t.Parallel()
	info, err := NewAnySearchTool().Info(t.Context())
	if err != nil || info.Name != anySearchToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, _ := json.Marshal(schema)
	for _, want := range []string{`"query"`, `"queries"`, `"minItems":1`, `"maxItems":5`, `"tag"`, `"params"`, `"zone"`, `"language"`, `"max_results"`, anySearchDiscoverToolName} {
		if !strings.Contains(string(raw), want) && !strings.Contains(info.Desc, want) {
			t.Errorf("schema and description lack %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "api_key") || strings.Contains(info.Desc, "api_key") {
		t.Errorf("api_key must not be model-visible: %s / %s", raw, info.Desc)
	}
	if strings.Contains(string(raw), `"anyOf"`) || strings.Contains(string(raw), `"oneOf"`) {
		t.Errorf("top-level anyOf/oneOf is rejected by several providers: %s", raw)
	}
}

func TestAnySearch_ComponentSpec(t *testing.T) {
	t.Parallel()
	spec := NewAnySearchTool().ComponentSpec()
	if !spec.PreserveJSONNumbers {
		t.Error("PreserveJSONNumbers = false, want exact integers from vertical results")
	}
	for _, key := range []string{"query", "tag", "params", "max_results"} {
		if _, ok := spec.Inputs[key]; !ok {
			t.Errorf("Inputs lacks %q", key)
		}
	}
	for key, want := range map[string]string{"query": "line", "tag": "line", "params": "object", "max_results": "integer"} {
		form, _ := spec.InputForm[key].(map[string]any)
		if form["type"] != want {
			t.Errorf("InputForm[%s] = %#v, want type %q", key, form, want)
		}
		if optional, _ := form["optional"].(bool); (key == "query") == optional {
			t.Errorf("InputForm[%s].optional = %v, only query is required", key, optional)
		}
	}
	for _, key := range []string{"formalized_content", "json"} {
		if _, ok := spec.Outputs[key]; !ok {
			t.Errorf("Outputs lacks %q", key)
		}
	}
}

func TestAnySearch_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"AnySearchSearch", "anysearch_search", "anysearchsearch"} {
		built, err := BuildByName(name, map[string]any{
			"api_key": " stored-key ", "top_n": float64(4), "zone": "cn", "language": "zh-CN",
			"query": "{sys.query}", "outputs": map[string]any{}, "function_name": "x",
		})
		if err != nil {
			t.Fatalf("BuildByName(%q): %v", name, err)
		}
		tool, ok := built.(*AnySearchTool)
		if !ok {
			t.Fatalf("BuildByName(%q) = %T, want *AnySearchTool", name, built)
		}
		want := anySearchDefaults{TopN: 4, Zone: "cn", Language: "zh-CN"}
		if tool.client.apiKey != "stored-key" || tool.defaults != want {
			t.Errorf("BuildByName(%q) = key %q defaults %+v, want %+v", name, tool.client.apiKey, tool.defaults, want)
		}
	}
	for _, params := range []map[string]any{nil, {}, {"api_key": nil}, {"api_key": ""}, {"api_key": "  "}} {
		built, err := BuildByName("anysearch_search", params)
		if err != nil {
			t.Errorf("anonymous config %v rejected: %v", params, err)
			continue
		}
		if tool := built.(*AnySearchTool); tool.client.apiKey != "" || tool.defaults.TopN != 10 {
			t.Errorf("anonymous config %v = key %q top_n %d", params, tool.client.apiKey, tool.defaults.TopN)
		}
	}
}

func TestAnySearch_BuildByNameRejectsInvalidNodeConfig(t *testing.T) {
	t.Parallel()
	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"key with newline":     {"api_key": "abc\ndef"},
		"key with CR":          {"api_key": "abc\rdef"},
	} {
		for _, tool := range []string{"anysearch_search", "anysearch_discover"} {
			if _, err := BuildByName(tool, params); err == nil {
				t.Errorf("%s: %s accepted the config, want an error", name, tool)
			}
		}
	}
	for name, params := range map[string]map[string]any{
		"zero top_n":           {"top_n": float64(0)},
		"top_n over the limit": {"top_n": float64(11)},
		"fractional top_n":     {"top_n": 2.5},
		"unknown zone":         {"zone": "mars"},
		"zone not a string":    {"zone": 1},
		"language not string":  {"language": 1},
	} {
		if _, err := BuildByName("anysearch_search", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}

// TestAnySearch_RealReactAgentRunsBatchAndDiscovery drives a real eino ReAct
// agent with both tools bound and a scripted model, so attaching the tools, the
// schema the model sees, argument decoding and the result fed back are all
// exercised without depending on a real model's tool-calling quality.
func TestAnySearch_RealReactAgentRunsBatchAndDiscovery(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if r.URL.Path == "/v1/domains" {
			writeAnySearch(w, 200, anySearchCatalogBody)
			return
		}
		switch decodeAnySearchBodyQuery(body) {
		case "golang":
			writeAnySearch(w, 200, `{"code":0,"request_id":"rid-go","data":{"results":[{"title":"Go","url":"https://go.dev","content":"Go context docs"}]}}`)
		default:
			writeAnySearch(w, 200, `{"code":0,"request_id":"rid-react","data":{"results":[{"title":"React","url":"https://react.dev","snippet":"useEffect docs"}]}}`)
		}
	})
	search := NewAnySearchToolWith(helper, "")
	discover := NewAnySearchDiscoverToolWith(helper, "")
	mdl := newReactScriptedModel(
		anySearchToolName,
		`{"queries":[{"query":"golang"},{"query":"useEffect","tag":"code.doc","params":{"library":"react"}}]}`,
		"done",
	)
	agent, err := react.NewAgent(t.Context(), &react.AgentConfig{
		ToolCallingModel: mdl,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: []einotool.BaseTool{search, discover}},
		MaxStep:          5,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}
	out, err := agent.Generate(t.Context(), []*schema.Message{schema.UserMessage("research go and react")})
	if err != nil {
		t.Fatalf("agent.Generate: %v", err)
	}
	if out.Content != "done" || mdl.turn != 2 {
		t.Fatalf("content = %q after %d model turns, want %q after 2", out.Content, mdl.turn, "done")
	}

	bound := map[string]bool{}
	for _, info := range mdl.boundTools {
		bound[info.Name] = true
		jsonSchema, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil {
			t.Fatalf("tool %s: ToJSONSchema: %v", info.Name, err)
		}
		raw, _ := json.Marshal(jsonSchema)
		if strings.Contains(string(raw), "api_key") {
			t.Errorf("tool %s exposes api_key to the model: %s", info.Name, raw)
		}
		if info.Name == anySearchToolName && !strings.Contains(string(raw), `"maxItems":5`) {
			t.Errorf("the model-visible search schema lacks the queries bound: %s", raw)
		}
	}
	if !bound[anySearchToolName] || !bound[anySearchDiscoverToolName] || len(bound) != 2 {
		t.Errorf("tools bound to the model = %v, want both AnySearch tools", bound)
	}

	var toolMessage string
	for _, msg := range mdl.rounds[1] {
		if msg.Role == schema.Tool {
			toolMessage = msg.Content
		}
	}
	env := decodeAnySearchEnvelope(t, toolMessage)
	if env.Error != "" || len(env.Queries) != 2 || env.Queries[0].RequestID != "rid-go" || env.Queries[1].RequestID != "rid-react" {
		t.Fatalf("tool message = %s, want both queries with their own request ids", toolMessage)
	}
	if env.Queries[0].Results[0]["url"] != "https://go.dev" || env.Queries[1].Results[0]["snippet"] != "useEffect docs" {
		t.Errorf("results = %+v / %+v", env.Queries[0].Results, env.Queries[1].Results)
	}

	seen := rec.all()
	if len(seen) != 2 {
		t.Fatalf("%d upstream requests, want 2", len(seen))
	}
	for _, request := range seen {
		if request.HasAuthorization || request.Path != "/v1/search" {
			t.Errorf("request = %+v, want anonymous POST /v1/search", request)
		}
		body := decodeAnySearchBody(t, request.Body)
		if body["query"] == "useEffect" {
			params, _ := body["params"].(map[string]any)
			if body["tag"] != "code.doc" || params["library"] != "react" {
				t.Errorf("vertical body = %v, want tag code.doc and params.library react", body)
			}
		}
	}
}

func TestAnySearch_ModelResultBoundsEveryFieldButKeepsSmallOnes(t *testing.T) {
	t.Parallel()
	bigNested := map[string]any{"rows": []any{strings.Repeat("x", 2049)}}
	small := map[string]any{"currency": "USD"}
	longURL := "https://example.com/" + strings.Repeat("u", 5000)
	out, _ := anySearchModelResult(map[string]any{
		"title":     "  " + strings.Repeat("t", 5000) + "  ",
		"url":       longURL,
		"snippet":   "summary",
		"content":   "",
		"price":     json.Number("9007199254740993"),
		"listed":    true,
		"missing":   nil,
		"small":     small,
		"too_large": bigNested,
	})
	if got := utf8.RuneCountInString(out["title"].(string)); got != 5004 {
		t.Errorf("title has %d runes, want the original 5004", got)
	}
	if out["snippet"] != "summary" || out["content"] != "" {
		t.Errorf("snippet/content = %#v / %#v, want both kept as they are", out["snippet"], out["content"])
	}
	if out["price"] != json.Number("9007199254740993") || out["listed"] != true {
		t.Errorf("scalars changed: %#v", out)
	}
	if _, has := out["too_large"]; !has || out["url"] != longURL {
		t.Error("structured fields or the URL were not preserved")
	}
	if _, has := out["small"]; !has {
		t.Error("a small nested value was dropped")
	}
	if _, has := out["missing"]; !has {
		t.Error("a null field was dropped")
	}
}

func TestAnySearch_LongURLAndStructuredResultFidelity(t *testing.T) {
	t.Parallel()
	longURL := "https://example.com/" + strings.Repeat("x", 1800)
	largeValue := strings.Repeat("a", 2148)
	resultJSON, err := json.Marshal(map[string]any{"title": "Market", "url": longURL, "content": "Market result", "data": map[string]any{"details": largeValue}})
	if err != nil {
		t.Fatal(err)
	}
	helper, _ := anySearchTestServer(t, anySearchOK("["+string(resultJSON)+"]"))
	out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), `{"query":"market","tag":"finance.quote","params":{"type":"stock"}}`)
	if err != nil {
		t.Fatal(err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if env.Error != "" || len(env.Queries) != 1 || len(env.Queries[0].Results) != 1 {
		t.Fatalf("search result = %s", out)
	}
	result := env.Queries[0].Results[0]
	if result["url"] != longURL {
		t.Fatalf("URL was changed: got %d bytes, want %d", len(anySearchText(result["url"])), len(longURL))
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["details"] != largeValue {
		t.Fatalf("structured field was lost or changed: %#v", result["data"])
	}
}

func TestAnySearch_OversizedStructuredResultReturnsError(t *testing.T) {
	t.Parallel()
	resultJSON, err := json.Marshal(map[string]any{"url": "https://example.com/", "content": "market", "data": map[string]any{"details": strings.Repeat("x", anySearchMaxQueryResultBytes)}})
	if err != nil {
		t.Fatal(err)
	}
	helper, _ := anySearchTestServer(t, anySearchOK("["+string(resultJSON)+"]"))
	out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), `{"query":"market"}`)
	if err != nil {
		t.Fatal(err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if env.Error == "" || len(env.Queries) != 1 || env.Queries[0].Error == "" || len(env.Queries[0].Results) != 0 {
		t.Fatalf("oversized data must report an error instead of returning a partial success: %s", out)
	}
}

func TestAnySearch_ReportsTruncatedTextFields(t *testing.T) {
	t.Parallel()
	resultJSON, err := json.Marshal(map[string]any{"url": "https://example.com/", "title": strings.Repeat("t", 1600), "content": strings.Repeat("c", 1600), "snippet": strings.Repeat("s", 1600)})
	if err != nil {
		t.Fatal(err)
	}
	helper, _ := anySearchTestServer(t, anySearchOK("["+string(resultJSON)+"]"))
	out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	query := decoded["queries"].([]any)[0].(map[string]any)
	result := query["results"].([]any)[0].(map[string]any)
	if len(result["title"].(string)) != 1600 || len(result["content"].(string)) != 1500 || len(result["snippet"].(string)) != 1500 {
		t.Fatalf("non-text values or text bounds were not preserved: %s", out)
	}
	fields, ok := query["truncated_fields"].(map[string]any)
	fieldList, hasFields := fields["0"].([]any)
	if !ok || !hasFields || len(fieldList) != 2 || fieldList[0] != "content" || fieldList[1] != "snippet" {
		t.Fatalf("missing truncation metadata: %s", out)
	}
}

func TestAnySearch_BatchPreservesResultsWhenOneExceedsOutputLimit(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, body []byte) {
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request body: %v", err)
			return
		}
		result := map[string]any{"url": "https://example.com/", "content": "market"}
		if request["query"] == "large" {
			result["data"] = map[string]any{"details": strings.Repeat("x", anySearchMaxQueryResultBytes)}
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			t.Error(err)
			return
		}
		anySearchOK("["+string(resultJSON)+"]")(w, nil, nil)
	})
	out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), `{"queries":[{"query":"large"},{"query":"small"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if env.Error != "" || len(env.Queries) != 2 || env.Queries[0].Error == "" || len(env.Queries[0].Results) != 0 || env.Queries[1].Error != "" || len(env.Queries[1].Results) != 1 {
		t.Fatalf("partial batch result = %s", out)
	}
}

type anySearchTimeoutError struct{}

func (anySearchTimeoutError) Error() string {
	return "i/o timeout to https://api.anysearch.com/v1/search?key=secret"
}
func (anySearchTimeoutError) Timeout() bool   { return true }
func (anySearchTimeoutError) Temporary() bool { return true }

type anySearchRoundTripper func(*http.Request) (*http.Response, error)

func (f anySearchRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAnySearch_TransportTimeoutIsReportedWithoutDetails(t *testing.T) {
	t.Parallel()
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: anySearchRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, anySearchTimeoutError{}
	})})
	out, err := NewAnySearchToolWith(helper, "secret").InvokableRun(t.Context(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if len(env.Queries) != 1 || env.Queries[0].Error != "anysearch: request timed out" {
		t.Fatalf("envelope = %s, want the generic timeout error", out)
	}
	if strings.Contains(out, "secret") || strings.Contains(env.Queries[0].Error, "api.anysearch.com") {
		t.Errorf("timeout error leaks transport details: %s", out)
	}
}

func TestAnySearch_BatchCancelledMidFlightKeepsEveryRecord(t *testing.T) {
	t.Parallel()
	arrived := make(chan struct{}, 3)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		arrived <- struct{}{}
		<-r.Context().Done()
	})
	go func() {
		for range 3 {
			select {
			case <-arrived:
			case <-ctx.Done():
				return
			}
		}
		cancel()
	}()
	out, err := NewAnySearchToolWith(helper, "key").InvokableRun(ctx, `{"queries":[{"query":"a"},{"query":"b"},{"query":"c"}]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if len(env.Queries) != 3 || env.Error == "" {
		t.Fatalf("envelope = %s, want 3 records and a top-level error because every query failed", out)
	}
	for i, record := range env.Queries {
		if record.Index != i || !strings.Contains(record.Error, "canceled") {
			t.Errorf("record %d = %+v, want its own canceled error in input order", i, record)
		}
	}
}

func TestAnySearch_RenderSkipsOversizedError(t *testing.T) {
	queries := []anySearchRenderQuery{
		{Query: "failed", Error: strings.Repeat("upstream failure ", 500)},
		{Query: "working", Chunks: []map[string]any{{"id": "1", "url": "https://example.com/", "content": "usable later evidence"}}},
	}
	out := renderAnySearchReferences(queries, 200)
	if !strings.Contains(out, "usable later evidence") || strings.Contains(out, "upstream failure") {
		t.Fatalf("oversized error suppressed usable later evidence: %q", out)
	}
	if tokens := tokenizer.NumTokensFromString(out); float64(tokens) > 200*0.97 {
		t.Fatalf("rendered output exceeds budget: %d tokens", tokens)
	}
}

func TestAnySearch_RenderSkipsOversizedHit(t *testing.T) {
	queries := []anySearchRenderQuery{{Query: "q", Chunks: []map[string]any{
		{"id": "1", "document_name": strings.Repeat("large ", 1000), "content": "large"},
		{"id": "2", "document_name": "small", "content": "usable later hit"},
	}}}
	if out := renderAnySearchReferences(queries, 200); !strings.Contains(out, "usable later hit") || !strings.Contains(out, "Query: q") {
		t.Fatalf("later hit and its query header were lost: %s", out)
	}
}

func TestAnySearch_SuccessDoesNotExposeConfiguredKey(t *testing.T) {
	for _, discover := range []bool{false, true} {
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			writeAnySearch(w, 200, `{"code":0,"request_id":"rid-ok","data":{"results":[{"content":"private-test-key"}],"domains":[{"description":"private-test-key"}]}}`)
		})
		var out string
		if discover {
			out, _ = NewAnySearchDiscoverToolWith(helper, "private-test-key").InvokableRun(t.Context(), `{}`)
		} else {
			out, _ = NewAnySearchToolWith(helper, "private-test-key").InvokableRun(t.Context(), `{"query":"q"}`)
		}
		if strings.Contains(out, "private-test-key") || !strings.Contains(out, "_ERROR") {
			t.Fatalf("credential-bearing response was forwarded: %s", out)
		}
	}
}

func TestAnySearch_ReferencesUseFullTextIdentity(t *testing.T) {
	content := strings.Repeat("prefix ", 300)
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		results, _ := json.Marshal([]map[string]any{
			{"url": "https://example.com/page", "content": content + "first ending"},
			{"url": "https://example.com/page", "content": content + "different ending"},
		})
		anySearchOK(string(results))(w, nil, nil)
	})
	search := NewAnySearchToolWith(helper, "")
	out, _ := search.InvokableRun(t.Context(), `{"query":"q"}`)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatal(err)
	}
	chunks, _ := search.BuildReferences(t.Context(), envelope)
	if len(chunks) != 2 || chunks[0]["chunk_id"] == chunks[1]["chunk_id"] {
		t.Fatalf("distinct full pages share citation identity: %#v", chunks)
	}
}

func TestAnySearch_KeyWithSurroundingWhitespaceIsTrimmedBeforeTheNewlineCheck(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("anysearch_search", map[string]any{"api_key": "  abc \n"})
	if err != nil {
		t.Fatalf("a key pasted with a trailing newline was rejected: %v", err)
	}
	if tool := built.(*AnySearchTool); tool.client.apiKey != "abc" {
		t.Errorf("key = %q, want %q", tool.client.apiKey, "abc")
	}
}

func TestAnySearch_BlankOptionalFieldsAreNotAMistake(t *testing.T) {
	t.Parallel()
	noOps := []string{
		`{"query":"","params":{},"max_results":""}`,
		`{"query":" ","tag":" ","zone":"","language":"  ","params":null,"max_results":null}`,
		`{"queries":null}`,
	}
	for _, args := range noOps {
		helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
		out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), args)
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		if env := decodeAnySearchEnvelope(t, out); env.Error != "" || len(env.Queries) != 0 {
			t.Errorf("%s: envelope = %+v, want an empty no-op", args, env)
		}
		if n := len(rec.all()); n != 0 {
			t.Errorf("%s: %d requests, want 0", args, n)
		}
	}
	for _, args := range []string{`{"query":"x","queries":[]}`, `{"query":"x","params":{},"max_results":"","queries":null}`} {
		helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
		out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), args)
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		if env := decodeAnySearchEnvelope(t, out); env.Error != "" || len(env.Queries) != 1 {
			t.Errorf("%s: envelope = %+v, want one search", args, env)
		}
		if n := len(rec.all()); n != 1 {
			t.Errorf("%s: %d requests, want 1: a valid query next to an empty queries array must still run", args, n)
		}
	}
}

func TestAnySearch_TopLevelFieldsBesideQueriesAreRejected(t *testing.T) {
	t.Parallel()
	for _, args := range []string{
		`{"tag":"code.doc","queries":[{"query":"x"}]}`,
		`{"zone":"cn","queries":[{"query":"x"}]}`,
		`{"params":{"library":"react"},"queries":[{"query":"x"}]}`,
		`{"max_results":3,"queries":[{"query":"x"}]}`,
	} {
		helper, rec := anySearchTestServer(t, anySearchOK(`[]`))
		out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), args)
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		if env := decodeAnySearchEnvelope(t, out); !strings.Contains(env.Error, "inside each item") {
			t.Errorf("%s: _ERROR = %q, want it to say the fields belong inside each item", args, env.Error)
		}
		if n := len(rec.all()); n != 0 {
			t.Errorf("%s: %d requests, want 0: a silently ignored tag would run a general search", args, n)
		}
	}
}

func TestAnySearch_PanicInOneQueryBecomesThatQuerysError(t *testing.T) {
	t.Parallel()
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: anySearchRoundTripper(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"boom"`) {
			panic("simulated transport failure")
		}
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"request_id":"rid-ok","data":{"results":[]}}`)),
			Request:    r,
		}, nil
	})})
	out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), `{"queries":[{"query":"fine"},{"query":"boom"},{"query":"also fine"}]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchEnvelope(t, out)
	if len(env.Queries) != 3 || env.Error != "" {
		t.Fatalf("envelope = %s, want 3 records and no top-level error", out)
	}
	if env.Queries[1].Error != "anysearch: internal error" || env.Queries[1].Query != "boom" || env.Queries[1].Index != 1 {
		t.Errorf("record 1 = %+v, want the panic reported as that query's own error", env.Queries[1])
	}
	if env.Queries[0].Error != "" || env.Queries[2].Error != "" || env.Queries[0].RequestID != "rid-ok" {
		t.Errorf("neighbouring queries were affected: %+v / %+v", env.Queries[0], env.Queries[2])
	}
	if strings.Contains(out, "simulated transport failure") {
		t.Errorf("panic text leaked into the result: %s", out)
	}
}
