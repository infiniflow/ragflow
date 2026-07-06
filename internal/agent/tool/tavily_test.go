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

func TestTavily_BuildRequest(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth, gotCT, gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	// Point the hard-coded tavily endpoint at the test server via a
	// transport that rewrites the host. Avoids the global-package-var
	// race that breaks parallel tests.
	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTavilyToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"ragflow","api_key":"key-xyz","max_results":3,"search_depth":"advanced"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if out == "" {
		t.Fatal("InvokableRun returned empty string")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "search") {
		t.Errorf("path = %q, want .../search", gotPath)
	}
	if gotAuth != "Bearer key-xyz" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer key-xyz")
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotBody["query"] != "ragflow" {
		t.Errorf("body.query = %v, want ragflow", gotBody["query"])
	}
	if n, ok := gotBody["max_results"].(float64); !ok || int(n) != 3 {
		t.Errorf("body.max_results = %v, want 3", gotBody["max_results"])
	}
	if gotBody["search_depth"] != "advanced" {
		t.Errorf("body.search_depth = %v, want advanced", gotBody["search_depth"])
	}
}

func TestTavily_ParseResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"url":"https://a.example/","title":"A","content":"alpha"},
				{"url":"https://b.example/","title":"B","content":"beta"}
			],
			"answer": "ignored"
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTavilyToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"x","api_key":"k"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env tavilyEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].URL != "https://a.example/" || env.Results[0].Title != "A" {
		t.Errorf("Results[0] = %+v, want url=https://a.example/ title=A", env.Results[0])
	}
	if env.Results[1].Content != "beta" {
		t.Errorf("Results[1].Content = %q, want beta", env.Results[1].Content)
	}
}

func TestTavily_RequiresAPIKey(t *testing.T) {
	t.Parallel()

	// envKey always returns "" so we know the failure is from the
	// missing api_key, not from a stray process env var.
	tool := NewTavilyToolWithEnvKey(NewHTTPHelper(), func() string { return "" })
	_, err := tool.InvokableRun(context.Background(), `{"query":"x"}`)
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("err = %v, want to mention api_key", err)
	}
}

func TestTavily_APIKeyFromEnv(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTavilyToolWithEnvKey(helper, func() string { return "from-env" })
	if _, err := tool.InvokableRun(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotAuth != "Bearer from-env" {
		t.Errorf("Authorization = %q, want Bearer from-env", gotAuth)
	}
}

func TestTavily_Info(t *testing.T) {
	t.Parallel()

	tool := NewTavilyTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "tavily" {
		t.Errorf("Name = %q, want tavily", info.Name)
	}
	if !strings.Contains(info.Desc, "Tavily") {
		t.Errorf("Desc = %q, want to mention Tavily", info.Desc)
	}
}
