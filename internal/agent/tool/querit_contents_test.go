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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueritContentsBuildsRequestAndPreservesResponse(t *testing.T) {
	var gotMethod, gotPath, gotAuthorization string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotMethod = request.Method
		gotPath = request.URL.Path
		gotAuthorization = request.Header.Get("Authorization")
		_ = json.NewDecoder(request.Body).Decode(&gotBody)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"error_code":0,"search_id":"crawl-1","results":[{"id":"1","url":"https://example.com","content":"# Example"}],"statuses":[{"id":"1","status":"success"}],"searchTime":1}`))
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteQueritHostTransport(server.URL)})
	contents := newQueritContentsTool(helper, func() string { return "" }, queritContentsParams{APIKey: "key-test"}, nil)
	out, err := contents.InvokableRun(t.Context(), `{"urls":["https://example.com"],"format":"html","crawl_timeout":20,"extras_meta":true}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/contents" {
		t.Fatalf("request = %s %s, want POST /v1/contents", gotMethod, gotPath)
	}
	if gotAuthorization != "Bearer key-test" {
		t.Fatalf("Authorization = %q", gotAuthorization)
	}
	if gotBody["format"] != "html" || gotBody["crawlTimeout"] != float64(20) || gotBody["extrasMeta"] != true {
		t.Fatalf("request body = %#v", gotBody)
	}
	urls, ok := gotBody["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("urls = %#v", gotBody["urls"])
	}
	if !strings.Contains(out, `"search_id":"crawl-1"`) || !strings.Contains(out, `"statuses"`) {
		t.Fatalf("complete response was not retained: %s", out)
	}
}

func TestQueritContentsMergesDefaultsAndExplicitFalse(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewDecoder(request.Body).Decode(&gotBody)
		_, _ = writer.Write([]byte(`{"results":[],"statuses":[]}`))
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteQueritHostTransport(server.URL)})
	contents := newQueritContentsTool(helper, func() string { return "" }, queritContentsParams{
		APIKey:       "stored-key",
		URLs:         []string{"https://stored.example"},
		Format:       "text",
		CrawlTimeout: 30,
		ExtrasMeta:   true,
	}, nil)
	_, err := contents.InvokableRun(t.Context(), `{"urls":"https://runtime.example","extras_meta":false}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotBody["format"] != "text" || gotBody["crawlTimeout"] != float64(30) || gotBody["extrasMeta"] != false {
		t.Fatalf("merged defaults = %#v", gotBody)
	}
	if gotBody["urls"].([]any)[0] != "https://runtime.example" {
		t.Fatalf("runtime urls = %#v", gotBody["urls"])
	}
}

func TestQueritContentsValidatesInputsAndAPIKey(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing urls", args: `{}`, want: "urls must contain"},
		{name: "too many urls", args: `{"urls":["https://1.example","https://2.example","https://3.example","https://4.example","https://5.example","https://6.example","https://7.example","https://8.example","https://9.example","https://10.example","https://11.example"]}`, want: "between 1 and 10"},
		{name: "relative url", args: `{"urls":["example.com"]}`, want: "absolute HTTP or HTTPS"},
		{name: "unsupported scheme", args: `{"urls":["file:///tmp/page"]}`, want: "absolute HTTP or HTTPS"},
		{name: "bad format", args: `{"urls":["https://example.com"],"format":"xml"}`, want: "format must be"},
		{name: "bad timeout", args: `{"urls":["https://example.com"],"crawl_timeout":61}`, want: "between 1 and 60"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contents := NewQueritContentsToolWithEnvKey(NewHTTPHelper(), func() string { return "key-test" })
			out, err := contents.InvokableRun(t.Context(), test.args)
			if err != nil || !strings.Contains(out, "_ERROR") || !strings.Contains(out, test.want) {
				t.Fatalf("result = %s, err = %v", out, err)
			}
		})
	}

	contents := NewQueritContentsToolWithEnvKey(NewHTTPHelper(), func() string { return "" })
	out, err := contents.InvokableRun(t.Context(), `{"urls":["https://example.com"]}`)
	if err != nil || !strings.Contains(out, "api_key is required") {
		t.Fatalf("missing key result = %s, err = %v", out, err)
	}
}

func TestQueritContentsRejectsMalformedResponses(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "top-level array", body: `[]`, want: "JSON object"},
		{name: "results object", body: `{"results":{}}`, want: "results must be a JSON array"},
		{name: "statuses object", body: `{"results":[],"statuses":{}}`, want: "statuses must be a JSON array"},
		{name: "trailing content", body: `{"results":[]} trailing`, want: "trailing content"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteQueritHostTransport(server.URL)})
			contents := NewQueritContentsToolWithEnvKey(helper, func() string { return "key-test" })
			out, err := contents.InvokableRun(t.Context(), `{"urls":["https://example.com"]}`)
			if err != nil || !strings.Contains(out, test.want) {
				t.Fatalf("result = %s, err = %v", out, err)
			}
		})
	}
}

func TestQueritContentsRedactsAPIKey(t *testing.T) {
	const secret = "secret-contents-key"
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: roundTripperErrorFunc(func(*http.Request) error {
		return errors.New("failed with " + secret)
	})})
	contents := NewQueritContentsToolWithEnvKey(helper, func() string { return secret })
	out, err := contents.InvokableRun(t.Context(), `{"urls":["https://example.com"]}`)
	if err != nil || strings.Contains(out, secret) || !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("result = %s, err = %v", out, err)
	}
}

func TestQueritContentsInfoAndComponentContract(t *testing.T) {
	contents := NewQueritContentsTool()
	if contents.helper.client.Timeout != 65*time.Second {
		t.Fatalf("HTTP timeout = %s, want 65s", contents.helper.client.Timeout)
	}
	info, err := contents.Info(context.Background())
	if err != nil || info.Name != queritContentsToolName || info.ParamsOneOf == nil {
		t.Fatalf("Info = %#v, %v", info, err)
	}
	encoded, _ := json.Marshal(info)
	if strings.Contains(string(encoded), "api_key") {
		t.Fatalf("Info exposed API key: %s", encoded)
	}
	spec := contents.ComponentSpec()
	if spec.Inputs["urls"] == "" || spec.Outputs["json"] == "" || !spec.PreserveJSONNumbers {
		t.Fatalf("ComponentSpec = %#v", spec)
	}
	if len(spec.InputForm) != 1 || spec.InputForm["urls"] == nil {
		t.Fatalf("InputForm = %#v, want URLs only", spec.InputForm)
	}
	response := map[string]any{"search_id": "crawl-1", "results": []any{map[string]any{"content": "page"}}}
	outputs := contents.BuildComponentOutputs(response)
	if outputs["json"].(map[string]any)["search_id"] != "crawl-1" {
		t.Fatalf("outputs = %#v", outputs)
	}
}
