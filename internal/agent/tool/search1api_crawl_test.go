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
	"strings"
	"testing"
)

func decodeSearch1APICrawlEnvelope(t *testing.T, out string) search1APICrawlEnvelope {
	t.Helper()
	var env search1APICrawlEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestSearch1APICrawl_BuildRequestAndResult(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := search1APITestServer(t, http.StatusOK,
		`{"crawlParameters":{"url":"https://example.com/a"},"results":{"title":"A","link":"https://example.com/a","content":"# A\n\nBody","metadata":{"lang":"en"}}}`)
	tool := NewSearch1APICrawlToolWith(helper, " key-xyz ")

	out, err := tool.InvokableRun(t.Context(), `{"url":" https://example.com/a "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/crawl" {
		t.Errorf("request = %s %s, want POST /crawl", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow search1api-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}
	if len(*body) != 1 || (*body)["url"] != "https://example.com/a" {
		t.Errorf("body = %v, want only the trimmed url", *body)
	}
	env := decodeSearch1APICrawlEnvelope(t, out)
	if env.Error != "" || env.Results["content"] != "# A\n\nBody" || env.Results["metadata"] == nil {
		t.Fatalf("envelope = %#v, want the page with its fields intact", env)
	}
}

func TestSearch1APICrawl_UsesNodeURLWhenModelGivesNone(t *testing.T) {
	t.Parallel()
	helper, _, body, _ := search1APITestServer(t, http.StatusOK, `{"results":{"title":"N"}}`)
	tool := newSearch1APICrawlTool(helper, "key", "https://example.com/node")

	if _, err := tool.InvokableRun(t.Context(), `{}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if (*body)["url"] != "https://example.com/node" {
		t.Errorf("url = %v, want the node url", (*body)["url"])
	}
}

func TestSearch1APICrawl_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, args, respBody, wantErr string
		status                             int
		wantCalls                          int32
	}{
		{"no url", "key", `{"url":" "}`, `{}`, "url is required", http.StatusOK, 0},
		{"relative url", "key", `{"url":"example.com/a"}`, `{}`, "must be an absolute HTTP or HTTPS URL", http.StatusOK, 0},
		{"non-http url", "key", `{"url":"file:///etc/passwd"}`, `{}`, "must be an absolute HTTP or HTTPS URL", http.StatusOK, 0},
		{"no api key", "", `{"url":"https://example.com"}`, `{}`, "api_key is required", http.StatusOK, 0},
		{"bad arguments", "key", `{"url":`, `{}`, "parse arguments", http.StatusOK, 0},
		{"rejected key", "key", `{"url":"https://example.com"}`, `{"error":"nope"}`, "upstream returned 401", http.StatusUnauthorized, 1},
		{"no results", "key", `{"url":"https://example.com"}`, `{"crawlParameters":{}}`, "response has no results", http.StatusOK, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := search1APITestServer(t, tc.status, tc.respBody)
			out, err := NewSearch1APICrawlToolWith(helper, tc.key).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeSearch1APICrawlEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestSearch1APICrawl_ComponentOutputs(t *testing.T) {
	t.Parallel()
	tool := NewSearch1APICrawlTool()
	page := map[string]any{"title": "A", "content": "Body"}
	outputs := tool.BuildComponentOutputs(map[string]any{"results": page})
	if got, _ := outputs["json"].(map[string]any); got["content"] != "Body" {
		t.Errorf("json output = %#v, want the crawled page", outputs["json"])
	}
	if got, _ := tool.BuildComponentOutputs(map[string]any{"_ERROR": "x"})["json"].(map[string]any); got == nil {
		t.Error("json output must be an empty object on error, not nil")
	}
	spec := tool.ComponentSpec()
	if _, ok := spec.Inputs["url"]; !ok {
		t.Errorf("Inputs = %#v, want url", spec.Inputs)
	}
}

func TestSearch1APICrawl_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("Search1APICrawl", map[string]any{
		"api_key": "stored-key", "url": "https://example.com", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*Search1APICrawlTool)
	if !ok {
		t.Fatalf("built %T, want *Search1APICrawlTool", built)
	}
	if tool.apiKey != "stored-key" || tool.defaultURL != "https://example.com" {
		t.Errorf("tool = key %q url %q", tool.apiKey, tool.defaultURL)
	}
	info, err := tool.Info(t.Context())
	if err != nil || info.Name != search1APICrawlToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}

	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"url not a string":     {"url": []any{"https://example.com"}},
	} {
		if _, err := BuildByName("search1api_crawl", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
