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
	"net/url"
	"strings"
	"testing"
)

func TestDeepL_BuildRequest(t *testing.T) {
	t.Parallel()

	var gotMethod, gotAuth, gotCT, gotPath string
	var gotForm url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"translations": [
				{"text":"Hallo Welt","detected_source_language":"EN"}
			]
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewDeepLToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"api_key":"key-xyz:fx","text":"Hello world","source_lang":"en","target_lang":"de"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/v2/translate") {
		t.Errorf("path = %q, want to end with /v2/translate", gotPath)
	}
	if gotAuth != "DeepL-Auth-Key key-xyz:fx" {
		t.Errorf("Authorization = %q, want DeepL-Auth-Key key-xyz:fx", gotAuth)
	}
	if !strings.HasPrefix(gotCT, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
	if gotForm.Get("text") != "Hello world" {
		t.Errorf("form.text = %q, want Hello world", gotForm.Get("text"))
	}
	// The endpoint routing should send :fx keys to the free endpoint.
	if gotForm.Get("source_lang") != "EN" {
		t.Errorf("form.source_lang = %q, want EN (uppercased)", gotForm.Get("source_lang"))
	}
	if gotForm.Get("target_lang") != "DE" {
		t.Errorf("form.target_lang = %q, want DE (uppercased)", gotForm.Get("target_lang"))
	}

	var env deeplEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(env.Results))
	}
	if env.Results[0].Text != "Hallo Welt" {
		t.Errorf("Results[0].Text = %q, want Hallo Welt", env.Results[0].Text)
	}
}

func TestDeepL_DefaultLanguages(t *testing.T) {
	t.Parallel()

	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"translations":[{"text":"你好","detected_source_language":"EN"}]}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewDeepLToolWith(helper)
	if _, err := tool.InvokableRun(context.Background(),
		`{"api_key":"x:fx","text":"Hello"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotForm.Get("source_lang") != "EN" {
		t.Errorf("default source_lang = %q, want EN", gotForm.Get("source_lang"))
	}
	if gotForm.Get("target_lang") != "ZH" {
		t.Errorf("default target_lang = %q, want ZH", gotForm.Get("target_lang"))
	}
}

func TestDeepL_RequiresAPIKeyAndText(t *testing.T) {
	t.Parallel()

	tool := NewDeepLTool()
	if _, err := tool.InvokableRun(context.Background(),
		`{"api_key":"","text":"Hello"}`); err == nil {
		t.Error("expected error for missing api_key")
	}
	if _, err := tool.InvokableRun(context.Background(),
		`{"api_key":"x","text":""}`); err == nil {
		t.Error("expected error for empty text")
	}
}

func TestDeepL_Info(t *testing.T) {
	t.Parallel()

	tool := NewDeepLTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "deepl" {
		t.Errorf("Name = %q, want deepl", info.Name)
	}
	if !strings.Contains(info.Desc, "DeepL") {
		t.Errorf("Desc = %q, want to mention DeepL", info.Desc)
	}
}
