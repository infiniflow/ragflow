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
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJin10_RegisteredRESTTool(t *testing.T) {
	t.Setenv("JIN10_API_KEY", "")

	tools, err := BuildAll([]string{"Jin10"}, nil)
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	jin10, ok := tools[0].(einotool.InvokableTool)
	if !ok {
		t.Fatalf("registered tool %T is not invokable", tools[0])
	}

	ctx := t.Context()
	info, err := jin10.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "jin10" {
		t.Errorf("name = %q, want jin10", info.Name)
	}
	if !strings.Contains(info.Desc, "REST") || strings.Contains(info.Desc, "WebSocket") {
		t.Errorf("description = %q, want Jin10 REST API", info.Desc)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	for _, name := range []string{"api_key", "secret_key", "secret-key"} {
		if _, ok := schema.Properties.Get(name); ok {
			t.Errorf("schema exposes credential parameter %q", name)
		}
	}

	out, err := jin10.InvokableRun(ctx, `{"api_key":"model-key","secret_key":"model-key","secret-key":"model-key"}`)
	if err == nil || !strings.Contains(err.Error(), "JIN10_API_KEY") {
		t.Fatalf("InvokableRun error = %v, want missing JIN10_API_KEY", err)
	}
	var env jin10Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(env.Error, "JIN10_API_KEY") || strings.Contains(out, "model-key") {
		t.Errorf("unexpected error result %s", out)
	}
}

func TestJin10_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	out, err := NewJin10Tool().InvokableRun(t.Context(), `{not json`)
	if err == nil || !strings.Contains(err.Error(), "jin10: parse arguments") {
		t.Fatalf("InvokableRun error = %v, want argument parse error", err)
	}
	var env jin10Envelope
	if decodeErr := json.Unmarshal([]byte(out), &env); decodeErr != nil || env.Error != err.Error() {
		t.Errorf("InvokableRun result = %s, want JSON with parse error", out)
	}
}

func TestJin10_RESTRequests(t *testing.T) {
	t.Setenv("JIN10_API_KEY", " env-key ")
	ctx := t.Context()
	tools, err := BuildAll([]string{"Jin10"}, nil)
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	jin10 := tools[0].(*Jin10Tool)
	node, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: tools, ExecuteSequentially: true})
	if err != nil {
		t.Fatalf("NewToolNode: %v", err)
	}
	cases := []struct {
		name  string
		args  string
		path  string
		query string
		body  map[string]string
	}{
		{
			name:  "default flash",
			args:  "{}",
			path:  "/data-api/flash",
			query: "category=1",
			body:  map[string]string{"category": "1", "contain": "", "filter": ""},
		},
		{
			name:  "empty arguments",
			path:  "/data-api/flash",
			query: "category=1",
			body:  map[string]string{"category": "1", "contain": "", "filter": ""},
		},
		{
			name:  "filtered flash",
			args:  `{"type":"flash","category":"3","contain":"A & B","filter":"C","secret_key":"model-key"}`,
			path:  "/data-api/flash",
			query: "category=3",
			body:  map[string]string{"category": "3", "contain": "A & B", "filter": "C"},
		},
		{
			name:  "calendar",
			args:  `{"type":"calendar","category":"us","calendar_datatype":"event"}`,
			path:  "/data-api/calendar/event",
			query: "category=us",
			body:  map[string]string{"category": "us"},
		},
		{
			name:  "default calendar",
			args:  `{"type":"calendar"}`,
			path:  "/data-api/calendar/data",
			query: "category=cj",
			body:  map[string]string{"category": "cj"},
		},
		{
			name:  "symbols",
			args:  `{"type":"symbols"}`,
			path:  "/data-api/symbols",
			query: "type=GOODS",
			body:  map[string]string{"type": "GOODS"},
		},
		{
			name:  "quotes",
			args:  `{"type":"symbols","symbols_datatype":"quotes","symbols_type":"CRYPTO","codes":"BTCUSD,ETHUSD"}`,
			path:  "/data-api/quotes",
			query: "type=CRYPTO",
			body:  map[string]string{"type": "CRYPTO", "codes": "BTCUSD,ETHUSD"},
		},
		{
			name: "news",
			args: `{"type":"news","contain":"gold"}`,
			path: "/data-api/news",
			body: map[string]string{"contain": "gold", "filter": ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jin10.helper.client.Transport = jin10RoundTripper(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "open-data-api.jin10.com" {
					t.Errorf("unexpected request %s %s", req.Method, req.URL)
				}
				if req.URL.Path != tc.path || req.URL.RawQuery != tc.query {
					t.Errorf("got path/query %s, want path %s and query %q", req.URL.RequestURI(), tc.path, tc.query)
				}
				if req.Header.Get("secret-key") != "env-key" {
					t.Error("request did not use the environment key")
				}
				defer req.Body.Close()
				var body map[string]string
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil || !maps.Equal(body, tc.body) {
					t.Errorf("request body = %v, decode error %v, want %v", body, err, tc.body)
				}
				response := httptest.NewRecorder()
				response.WriteString(`{"data":[{"id":9007199254740993}]}`)
				return response.Result(), nil
			})
			out, err := node.Invoke(t.Context(), schema.AssistantMessage("", []schema.ToolCall{{
				ID:       "jin10-call",
				Type:     "function",
				Function: schema.FunctionCall{Name: "jin10", Arguments: tc.args},
			}}))
			if err != nil {
				t.Fatalf("ToolsNode.Invoke: %v", err)
			}
			if len(out) != 1 || out[0].Content != `{"items":[{"id":9007199254740993}]}` {
				t.Fatalf("tool messages = %v, want items from the Jin10 response", out)
			}
		})
	}
}

func TestJin10_RefusesRedirects(t *testing.T) {
	t.Setenv("JIN10_API_KEY", "env-key")
	for _, tc := range []struct{ name, location string }{
		{"same host", "/data-api/news"},
		{"different host", "https://redirect.example.invalid/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jin10 := NewJin10Tool()
			requests := 0
			jin10.helper.client.Transport = jin10RoundTripper(func(req *http.Request) (*http.Response, error) {
				requests++
				response := httptest.NewRecorder()
				if requests == 1 {
					response.Header().Set("Location", tc.location)
					response.WriteHeader(http.StatusFound)
				}
				response.WriteString(`{"data":[]}`)
				return response.Result(), nil
			})
			out, err := jin10.InvokableRun(t.Context(), "{}")
			if err == nil || !strings.Contains(out, "HTTP 302") {
				t.Errorf("InvokableRun = (%s, %v), want redirect error", out, err)
			}
			if requests != 1 {
				t.Errorf("sent %d HTTP requests, want only the original request", requests)
			}
		})
	}
}

type jin10RoundTripper func(*http.Request) (*http.Response, error)

func (f jin10RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
