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

func decodeSpecStreamExtractEnvelope(t *testing.T, out string) specstreamExtractEnvelope {
	t.Helper()
	var env specstreamExtractEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestSpecStreamExtract_BuildRequestAndResult(t *testing.T) {
	t.Parallel()
	helper, req, body, _ := specstreamTestServer(t, http.StatusOK,
		`{"data":{"request_id":"req_ext","results":[{"url":"https://example.com/a","status":"ok","title":"A","text":"# A\n\nBody","read_from":"live"}]}}`)
	tool := NewSpecStreamExtractToolWith(helper, " key-xyz ")

	out, err := tool.InvokableRun(t.Context(), `{"url":" https://example.com/a "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/v1/extract" {
		t.Errorf("request = %s %s, want POST /v1/extract", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want trimmed bearer key", got)
	}
	if got := req.Header.Get("User-Agent"); got != "RAGFlow specstream-integration/infiniflow-ragflow" {
		t.Errorf("User-Agent = %q", got)
	}
	urls, _ := (*body)["urls"].([]any)
	if len(urls) != 1 || urls[0] != "https://example.com/a" || (*body)["format"] != "markdown" {
		t.Errorf("body = %v, want urls=[https://example.com/a] and format=markdown", *body)
	}
	env := decodeSpecStreamExtractEnvelope(t, out)
	if env.Error != "" || env.Results["text"] != "# A\n\nBody" || env.Results["status"] != "ok" {
		t.Fatalf("envelope = %#v, want extracted page with fields intact", env)
	}
}

func TestSpecStreamExtract_UsesNodeURLWhenModelGivesNone(t *testing.T) {
	t.Parallel()
	helper, _, body, _ := specstreamTestServer(t, http.StatusOK, `{"data":{"results":[{"url":"https://example.com/node","status":"ok","text":"ok"}]}}`)
	tool := newSpecStreamExtractTool(helper, "key", "https://example.com/node")

	if _, err := tool.InvokableRun(t.Context(), `{}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	urls, _ := (*body)["urls"].([]any)
	if len(urls) != 1 || urls[0] != "https://example.com/node" {
		t.Errorf("urls = %v, want node url", (*body)["urls"])
	}
}

func TestSpecStreamExtract_ErrorsReachTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key, args, respBody, wantErr string
		status                             int
		wantCalls                          int32
	}{
		{"no url", "key", `{"url":" "}`, `{}`, "url is required", http.StatusOK, 0},
		{"relative url", "key", `{"url":"example.com/a"}`, `{}`, "must be an absolute HTTP or HTTPS URL", http.StatusOK, 0},
		{"non-http url", "key", `{"url":"file:///etc/passwd"}`, `{}`, "must be an absolute HTTP or HTTPS URL", http.StatusOK, 0},
		{"unsupported format", "key", `{"url":"https://example.com","format":"html"}`, `{}`, "is not supported", http.StatusOK, 0},
		{"no api key", "", `{"url":"https://example.com"}`, `{}`, "api_key is required", http.StatusOK, 0},
		{"bad arguments", "key", `{"url":`, `{}`, "parse arguments", http.StatusOK, 0},
		{"rejected key", "key", `{"url":"https://example.com"}`, `{"error":"nope"}`, "upstream returned 401", http.StatusUnauthorized, 1},
		{"no results", "key", `{"url":"https://example.com"}`, `{"data":{"results":[]}}`, "response has no results", http.StatusOK, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _, _, calls := specstreamTestServer(t, tc.status, tc.respBody)
			out, err := NewSpecStreamExtractToolWith(helper, tc.key).InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			if env := decodeSpecStreamExtractEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestSpecStreamExtract_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	built, err := BuildByName("SpecStreamExtract", map[string]any{
		"api_key": "stored-key", "url": "https://example.com", "outputs": map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	tool, ok := built.(*SpecStreamExtractTool)
	if !ok {
		t.Fatalf("built %T, want *SpecStreamExtractTool", built)
	}
	if tool.apiKey != "stored-key" || tool.defaultURL != "https://example.com" {
		t.Errorf("tool = key %q url %q", tool.apiKey, tool.defaultURL)
	}
	info, err := tool.Info(t.Context())
	if err != nil || info.Name != specstreamExtractToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"url not a string":     {"url": []any{"https://example.com"}},
	} {
		if _, err := BuildByName("specstream_extract", params); err == nil {
			t.Errorf("%s: expected a config error", name)
		}
	}
}
