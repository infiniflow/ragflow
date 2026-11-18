//
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

package component

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// TestMinerUExtractionSwitches exercises construction, tenant-model resolution
// and the actual multipart request, including independently omitted switches.
func TestMinerUExtractionSwitches(t *testing.T) {
	withSSRFBypass(t)
	options := []struct {
		name  string
		value any
		want  bool
	}{
		{"omitted", nil, true},
		{"enabled", true, true},
		{"disabled", false, false},
	}
	backends := []string{
		"pipeline", "vlm-engine", "vlm-auto-engine", "vlm-http-client",
		"hybrid-engine", "hybrid-auto-engine", "hybrid-http-client",
	}
	for _, backend := range backends {
		for _, formula := range options {
			for _, table := range options {
				t.Run(backend+"/"+formula.name+"/"+table.name, func(t *testing.T) {
					requests := make(chan [2]string, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/file_parse" || r.Method != http.MethodPost {
							http.NotFound(w, r)
							return
						}
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Errorf("parse multipart form: %v", err)
							http.Error(w, "bad form", http.StatusBadRequest)
							return
						}
						defer r.MultipartForm.RemoveAll()
						if got := r.FormValue("backend"); got != backend {
							t.Errorf("backend = %q, want %q", got, backend)
						}
						for key, want := range map[string]string{
							"parse_method": "ocr", "lang_list": "japan",
							"return_md": "true", "return_content_list": "true",
						} {
							if got := r.FormValue(key); got != want {
								t.Errorf("%s = %q, want %q", key, got, want)
							}
						}
						if r.Header.Get("Authorization") != "Bearer mineru-secret" {
							t.Error("tenant instance credentials were not retained")
						}
						requests <- [2]string{r.FormValue("formula_enable"), r.FormValue("table_enable")}
						writeMinerUZipResponse(w, "Extraction switches reached the selected model.")
					}))
					defer server.Close()
					db, modelID := setupMinerUVisionDispatchDB(t, server.URL)
					pdf := map[string]any{
						"parse_method": modelID, "mineru_backend": backend,
						"mineru_server_url":   "http://model.invalid",
						"mineru_parse_method": "ocr", "mineru_lang": "Japanese",
					}
					if formula.value != nil {
						pdf["mineru_formula_enable"] = formula.value
					}
					if table.value != nil {
						pdf["mineru_table_enable"] = table.value
					}
					component, err := NewParserComponent(map[string]any{"pdf": pdf})
					if err != nil {
						t.Fatal(err)
					}
					out, err := component.Invoke(t.Context(), db, map[string]any{
						"binary": []byte("%PDF-1.4"), "name": "sample.pdf",
						"file_type": "pdf", "tenant_id": "test-tenant",
					})
					if err != nil {
						t.Fatal(err)
					}
					requireJSONText(t, out, "Extraction switches reached the selected model.")
					select {
					case got := <-requests:
						want := [2]string{strconv.FormatBool(formula.want), strconv.FormatBool(table.want)}
						if got != want {
							t.Errorf("formula_enable/table_enable = %v, want %v", got, want)
						}
					default:
						t.Fatal("no /file_parse request")
					}
				})
			}
		}
	}
}

// TestMinerUExtractionSwitchTypes rejects malformed saved values instead of
// silently converting them to the enabled default.
func TestMinerUExtractionSwitchTypes(t *testing.T) {
	for _, key := range []string{"mineru_formula_enable", "mineru_table_enable"} {
		for _, value := range []any{nil, "false", 0, []any{}, map[string]any{}} {
			t.Run(fmt.Sprintf("%s/%T", key, value), func(t *testing.T) {
				_, err := NewParserComponent(map[string]any{
					"pdf": map[string]any{"parse_method": "mineru", key: value},
				})
				if err == nil || !strings.Contains(err.Error(), key+" must be a boolean") {
					t.Fatalf("invalid switch %v: error = %v", value, err)
				}
			})
		}
	}
}

// TestMinerUV1RejectsDisabledExtraction verifies that unsupported preferences
// fail before creating an upload or job on the V1 protocol.
func TestMinerUV1RejectsDisabledExtraction(t *testing.T) {
	withSSRFBypass(t)
	for _, pdf := range []map[string]any{
		{"mineru_formula_enable": false},
		{"mineru_table_enable": false},
		{"mineru_formula_enable": false, "mineru_table_enable": false},
		{"mineru_formula_enable": false, "mineru_table_enable": true},
		{"mineru_formula_enable": true, "mineru_table_enable": false},
	} {
		t.Run(fmt.Sprint(pdf), func(t *testing.T) {
			var submissions atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/health" && r.Method == http.MethodGet {
					w.WriteHeader(http.StatusOK)
					return
				}
				submissions.Add(1)
				http.Error(w, "must reject before upload or job creation", http.StatusBadRequest)
			}))
			defer server.Close()
			db, modelID := setupMinerUVisionDispatchDB(t, server.URL)
			pdf["parse_method"] = modelID
			component, err := NewParserComponent(map[string]any{"pdf": pdf})
			if err != nil {
				t.Fatal(err)
			}
			_, err = component.Invoke(t.Context(), db, map[string]any{
				"binary": []byte("%PDF-1.4"), "name": "sample.pdf",
				"file_type": "pdf", "tenant_id": "test-tenant",
			})
			if err == nil || !strings.Contains(err.Error(), "V1 does not support disabling formula or table extraction") {
				t.Errorf("unsupported V1 preference: error = %v", err)
			}
			if got := submissions.Load(); got != 0 {
				t.Errorf("created %d requests after V1 probe; want no upload or job", got)
			}
		})
	}
}

// TestMinerUV1EnabledExtractionPreservesJob ensures default/enabled switches do
// not add unsupported fields to V1's strictly validated job request.
func TestMinerUV1EnabledExtractionPreservesJob(t *testing.T) {
	withSSRFBypass(t)
	for _, enabled := range []map[string]any{
		{},
		{"mineru_formula_enable": true},
		{"mineru_table_enable": true},
		{"mineru_formula_enable": true, "mineru_table_enable": true},
	} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			var jobs atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/health":
					_, _ = w.Write([]byte(`{}`))
				case "/v1/uploads":
					_, _ = w.Write([]byte(`{"status":"completed","file":{"id":"fixture-file"}}`))
				case "/v1/parse/jobs":
					var job map[string]any
					if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
						t.Error(err)
						http.Error(w, "invalid job", http.StatusBadRequest)
						return
					}
					for key := range job {
						switch key {
						case "files", "tier", "output_formats", "ocr_mode":
						default:
							t.Errorf("unsupported V1 job field %q", key)
						}
					}
					jobs.Add(1)
					_, _ = w.Write([]byte(`{"job_id":"fixture-job","status":"completed","files":[{"status":"completed","output_files":{"markdown":{"file_id":"fixture-output"}}}]}`))
				case "/v1/files/fixture-output/content":
					_, _ = w.Write([]byte("V1 extraction defaults are unchanged."))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			db, modelID := setupMinerUVisionDispatchDB(t, server.URL)
			pdf := map[string]any{"parse_method": modelID}
			for key, value := range enabled {
				pdf[key] = value
			}
			component, err := NewParserComponent(map[string]any{"pdf": pdf})
			if err != nil {
				t.Fatal(err)
			}
			out, err := component.Invoke(t.Context(), db, map[string]any{
				"binary": []byte("%PDF-1.4"), "name": "sample.pdf",
				"file_type": "pdf", "tenant_id": "test-tenant",
			})
			if err != nil {
				t.Fatal(err)
			}
			requireJSONText(t, out, "V1 extraction defaults are unchanged.")
			if jobs.Load() != 1 {
				t.Fatalf("V1 parse jobs = %d, want 1", jobs.Load())
			}
		})
	}
}
