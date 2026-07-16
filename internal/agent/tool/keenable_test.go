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
	"strings"
	"testing"
)

// TestKeenable_KeylessPath verifies that when no api_key is supplied the
// tool POSTs to /v1/search/public with the attribution headers but
// without an X-API-Key header.
func TestKeenable_KeylessPath(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotUA, gotTitle, gotAPIKey, gotCT string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotUA = r.Header.Get("User-Agent")
		gotTitle = r.Header.Get("X-Keenable-Title")
		gotAPIKey = r.Header.Get("X-API-Key")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewKeenableToolWithEnvBaseURL(helper, func() string { return "https://" + srv.URL[len("http://"):] })

	if _, err := tool.InvokableRun(context.Background(), `{"query":"ragflow"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/search/public" {
		t.Errorf("path = %q, want /v1/search/public (keyless endpoint)", gotPath)
	}
	if gotUA != "keenable-ragflow" {
		t.Errorf("User-Agent = %q, want keenable-ragflow", gotUA)
	}
	if gotTitle != "RAGFlow" {
		t.Errorf("X-Keenable-Title = %q, want RAGFlow", gotTitle)
	}
	if gotAPIKey != "" {
		t.Errorf("X-API-Key = %q, want empty on keyless path", gotAPIKey)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotBody["query"] != "ragflow" {
		t.Errorf("body.query = %v, want ragflow", gotBody["query"])
	}
	if gotBody["mode"] != "pro" {
		t.Errorf("body.mode = %v, want pro (default)", gotBody["mode"])
	}
}

// TestKeenable_KeyedPath verifies that a server-configured api_key
// switches the tool to the /v1/search endpoint and sets X-API-Key on
// the request.
func TestKeenable_KeyedPath(t *testing.T) {
	t.Parallel()

	var gotPath, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewKeenableToolWithAPIKey(helper, "key-xyz")
	tool.envBaseURL = func() string { return "https://" + srv.URL[len("http://"):] }

	if _, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","mode":"realtime"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	if gotPath != "/v1/search" {
		t.Errorf("path = %q, want /v1/search (keyed endpoint)", gotPath)
	}
	if gotAPIKey != "key-xyz" {
		t.Errorf("X-API-Key = %q, want key-xyz", gotAPIKey)
	}
}

// TestKeenable_SiteAndTopN verifies the site filter is forwarded and
// that the result list is truncated to top_n.
func TestKeenable_SiteAndTopN(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"A","url":"https://a","description":"alpha","custom":"preserved"},
			{"title":"B","url":"https://b","description":"beta"},
			{"title":"C","url":"https://c","description":"gamma"},
			{"title":"D","url":"https://d","description":"delta"}
		]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewKeenableToolWithEnvBaseURL(helper, func() string { return "https://" + srv.URL[len("http://"):] })

	out, err := tool.InvokableRun(context.Background(),
		`{"query":"x","site":"example.com","top_n":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	if gotBody["site"] != "example.com" {
		t.Errorf("body.site = %v, want example.com", gotBody["site"])
	}

	var env keenableEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2 (capped by top_n)", len(env.Results))
	}
	if env.Results[0]["title"] != "A" || env.Results[1]["title"] != "B" {
		t.Errorf("Results = %+v, want first 2 upstream items", env.Results)
	}
	if env.Results[0]["custom"] != "preserved" {
		t.Fatalf("raw upstream fields were lost: %#v", env.Results[0])
	}
}

// TestKeenable_DefaultTopN verifies that omitting top_n keeps up to 10
// results from the upstream response (the default in the Python tool).
func TestKeenable_DefaultTopN(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 12 results; default top_n is 10, so we expect 10 in the envelope.
		var results []map[string]string
		for range 12 {
			results = append(results, map[string]string{
				"title":       "T",
				"url":         "https://u",
				"description": "d",
			})
		}
		b, _ := json.Marshal(map[string]any{"results": results})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewKeenableToolWithEnvBaseURL(helper, func() string { return "https://" + srv.URL[len("http://"):] })

	out, err := tool.InvokableRun(context.Background(), `{"query":"x"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env keenableEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	if len(env.Results) != 10 {
		t.Errorf("Results len = %d, want 10 (default top_n)", len(env.Results))
	}
}

func TestKeenable_MissingQuery(t *testing.T) {
	t.Parallel()

	tool := NewKeenableTool()
	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var envelope keenableEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(envelope.Results) != 0 || envelope.Error != "" {
		t.Fatalf("envelope = %+v, want empty results without error", envelope)
	}
}

// TestKeenable_RealtimeRequiresAPIKey verifies the config-time rejection
// of realtime mode without a configured api_key.
func TestKeenable_RealtimeRequiresAPIKey(t *testing.T) {
	t.Parallel()

	tool := NewKeenableTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":"x","mode":"realtime"}`)
	if err == nil {
		t.Fatal("expected error for realtime mode without api_key")
	}
	if !strings.Contains(err.Error(), "configured api_key") {
		t.Errorf("err = %v, want to mention configured api_key", err)
	}
}

// TestKeenable_InvalidMode verifies that an unknown mode is rejected
// up front instead of being forwarded to the upstream.
func TestKeenable_InvalidMode(t *testing.T) {
	t.Parallel()

	tool := NewKeenableTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":"x","mode":"bogus"}`)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("err = %v, want to mention mode", err)
	}
}

// TestKeenable_ResolveBaseURL exercises the HTTPS-only / loopback-http
// guard around KEENABLE_API_URL.
func TestKeenable_ResolveBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		raw       string
		wantOK    bool
		wantValue string
	}{
		{"https default", "https://api.keenable.ai", true, "https://api.keenable.ai"},
		{"https trailing slash", "https://api.keenable.ai/", true, "https://api.keenable.ai"},
		{"http loopback ok", "http://localhost:8080", true, "http://localhost:8080"},
		{"http 127 ok", "http://127.0.0.1:8080", true, "http://127.0.0.1:8080"},
		{"http ::1 ok", "http://[::1]:8080", true, "http://[::1]:8080"},
		{"http non-loopback rejected", "http://example.com", false, ""},
		{"ftp rejected", "ftp://api.keenable.ai", false, ""},
		{"query rejected", "https://api.keenable.ai?x=1", false, ""},
		{"fragment rejected", "https://api.keenable.ai#frag", false, ""},
		{"no host rejected", "https:///path", false, ""},
		{"empty rejected", "", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveKeenableBaseURL(tc.raw)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				if got != tc.wantValue {
					t.Errorf("got = %q, want %q", got, tc.wantValue)
				}
				return
			}
			if err == nil {
				t.Fatalf("got = %q, want error", got)
			}
		})
	}
}

// TestKeenable_BaseURLFromEnv verifies that the KEENABLE_API_URL env var
// is honored. We use a fake resolver that does NOT touch os.Getenv so
// the test does not depend on the host environment.
func TestKeenable_BaseURLFromEnv(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewKeenableToolWithEnvBaseURL(helper, func() string {
		return "https://" + srv.URL[len("http://"):]
	})

	if _, err := tool.InvokableRun(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotPath != "/v1/search/public" {
		t.Errorf("path = %q, want /v1/search/public", gotPath)
	}
}

// TestKeenable_BadBaseURL verifies that an invalid KEENABLE_API_URL is
// reported back to the caller instead of being silently sent.
func TestKeenable_BadBaseURL(t *testing.T) {
	t.Parallel()

	tool := NewKeenableToolWithEnvBaseURL(NewHTTPHelper(), func() string { return "http://example.com" })
	_, err := tool.InvokableRun(context.Background(), `{"query":"x"}`)
	if err == nil {
		t.Fatal("expected error for non-https non-loopback base URL")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("err = %v, want to mention https requirement", err)
	}
}

// TestKeenable_UpstreamError verifies that a non-2xx upstream response
// is surfaced as an error and an _ERROR envelope.
func TestKeenable_UpstreamError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewKeenableToolWithEnvBaseURL(helper, func() string { return "https://" + srv.URL[len("http://"):] })

	out, err := tool.InvokableRun(context.Background(), `{"query":"x"}`)
	if err == nil {
		t.Fatal("expected error for 5xx response")
	}
	var env keenableEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error == "" {
		t.Errorf("envelope Error = %q, want non-empty", env.Error)
	}
}

// TestKeenable_Info verifies the model-facing metadata.
func TestKeenable_Info(t *testing.T) {
	t.Parallel()

	tool := NewKeenableTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "keenable_search" {
		t.Errorf("Name = %q, want keenable_search", info.Name)
	}
	if !strings.Contains(info.Desc, "Keenable") {
		t.Errorf("Desc = %q, want to mention Keenable", info.Desc)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("ParamsOneOf = nil, want schema definition")
	}
	paramsJSON, err := json.Marshal(info.ParamsOneOf)
	if err != nil {
		t.Fatalf("marshal ParamsOneOf: %v", err)
	}
	if strings.Contains(string(paramsJSON), "api_key") {
		t.Fatalf("Info ParamsOneOf unexpectedly exposes api_key: %s", string(paramsJSON))
	}
	if strings.Contains(string(paramsJSON), "mode") || strings.Contains(string(paramsJSON), "top_n") {
		t.Fatalf("Info ParamsOneOf leaked node configuration: %s", string(paramsJSON))
	}
	form := tool.ComponentSpec().InputForm
	for _, key := range []string{"query", "site"} {
		field, ok := form[key].(map[string]any)
		if !ok {
			t.Fatalf("GetInputForm()[%s] = %#v, want field map", key, form[key])
		}
		if field["type"] != "line" {
			t.Fatalf("GetInputForm()[%s][type] = %v, want line", key, field["type"])
		}
	}
}

func TestKeenable_ComponentContractReferencesAndOutputs(t *testing.T) {
	t.Parallel()

	tool := NewKeenableTool()
	spec := tool.ComponentSpec()
	for _, input := range []string{"query", "site"} {
		if _, ok := spec.Inputs[input]; !ok {
			t.Fatalf("component inputs missing %s: %#v", input, spec.Inputs)
		}
	}
	for _, output := range []string{"formalized_content", "json"} {
		if _, ok := spec.Outputs[output]; !ok {
			t.Fatalf("component outputs missing %s: %#v", output, spec.Outputs)
		}
	}

	envelope := map[string]any{"results": []any{map[string]any{
		"title":       "Keenable result",
		"url":         "https://example.com/item",
		"description": "Fresh search result",
	}}}
	chunks, docAggs := tool.BuildReferences(context.Background(), envelope)
	if len(chunks) != 1 || len(docAggs) != 1 {
		t.Fatalf("references = %#v / %#v", chunks, docAggs)
	}
	if chunks[0]["document_name"] != "Keenable result" || chunks[0]["url"] != "https://example.com/item" {
		t.Fatalf("reference metadata = %#v", chunks[0])
	}
	outputs := tool.BuildComponentOutputs(envelope)
	formalized, _ := outputs["formalized_content"].(string)
	for _, want := range []string{"Keenable result", "https://example.com/item", "Fresh search result"} {
		if !strings.Contains(formalized, want) {
			t.Fatalf("formalized_content missing %q: %s", want, formalized)
		}
	}
	results, ok := outputs["json"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("json output = %#v", outputs["json"])
	}
	if _, exists := envelope["chunks"]; exists {
		t.Fatalf("component conversion mutated envelope: %#v", envelope)
	}
}

func TestKeenable_BuildByNameAcceptsCanvasParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("keenable", map[string]any{
		"api_key": "stored-key",
		"mode":    "realtime",
		"top_n":   float64(3),
		"site":    "example.com",
		"outputs": map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool := built.(*KeenableTool)
	if tool.apiKey != "stored-key" || tool.defaults.Mode != "realtime" || tool.defaults.TopN != 3 || tool.defaults.Site != "example.com" {
		t.Fatalf("tool config = apiKey=%q defaults=%+v", tool.apiKey, tool.defaults)
	}
}

func TestKeenable_BuildByNameRejectsInvalidCanvasParams(t *testing.T) {
	t.Parallel()

	invalid := []map[string]any{
		{"api_key": 1},
		{"mode": "fast"},
		{"mode": "realtime"},
		{"top_n": 0},
		{"top_n": 1.5},
		{"site": 1},
	}
	for _, params := range invalid {
		if _, err := BuildByName("keenable", params); err == nil {
			t.Fatalf("BuildByName(%#v) succeeded, want validation error", params)
		}
	}
}
