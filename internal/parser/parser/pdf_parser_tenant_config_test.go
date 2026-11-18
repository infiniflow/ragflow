// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

// TestPDFParserTenantConfigurationPolicy keeps named-engine environment behavior
// while explicit tenant instances cannot borrow an unrelated global credential.
func TestPDFParserTenantConfigurationPolicy(t *testing.T) {
	withSSRFBypass(t)
	t.Setenv(common.EnvSOMarkAPIKey, "global-fixture-key")
	t.Setenv(common.EnvOpenDataLoaderAPIKey, "global-fixture-key")
	t.Setenv(common.EnvSOMarkEnableInlineImage, "true")
	for _, engine := range []string{"somark", "opendataloader"} {
		for _, selected := range []bool{false, true} {
			t.Run(engine+map[bool]string{false: "/named", true: "/selected"}[selected], func(t *testing.T) {
				requests := make(chan string, 4)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.Header.Get("Content-Type"), "multipart") {
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Errorf("multipart: %v", err)
							return
						}
						defer r.MultipartForm.RemoveAll()
					} else {
						_ = r.ParseForm()
					}
					key := r.FormValue("api_key")
					if engine == "opendataloader" {
						key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					}
					requests <- key
					switch r.URL.Path {
					case "/parse/async":
						want := `"enable_inline_image":true`
						if selected {
							want = `"enable_inline_image":false`
						}
						if !strings.Contains(r.FormValue("feature_config"), want) {
							t.Errorf("feature environment policy not respected")
						}
						_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"local"}}`))
					case "/parse/async_check":
						_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"text","content":"fixture"}]}]}}}}}`))
					case "/file_parse":
						_, _ = w.Write([]byte(`{"md_text":"fixture"}`))
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}))
				defer server.Close()
				p := NewPDFParser()
				p.ParseMethod = engine
				p.SoMarkBaseURL, p.OpenDataLoaderAPIServer = server.URL, server.URL
				p.SoMarkEnableInlineImage = false
				p.ConfigureFromSetup(map[string]any{"use_tenant_model_config": true})
				if p.UseTenantModelConfig {
					t.Fatal("runtime-only policy accepted from saved settings")
				}
				p.UseTenantModelConfig = selected
				if result := p.ParseWithResult(t.Context(), "sample.pdf", []byte("pdf")); result.Err != nil {
					t.Fatal(result.Err)
				}
				if len(requests) == 0 {
					t.Fatal("no parser requests")
				}
				for len(requests) > 0 {
					key := <-requests
					if selected && key != "" || !selected && key != "global-fixture-key" {
						t.Fatal("incorrect environment credential policy")
					}
				}
			})
		}
	}
}
