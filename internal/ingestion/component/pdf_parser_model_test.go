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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type pdfParserRequest struct {
	Path          string
	Authorization string
	Key           string
	Formats       string
	Features      string
}

// tenantPDFParserFixture uses real DAO records and local parser HTTP protocols,
// with a separate decoy instance that must never be selected.
func tenantPDFParserFixture(t *testing.T, provider string) (*gorm.DB, string, chan pdfParserRequest) {
	t.Helper()
	withSSRFBypass(t)
	requests := make(chan pdfParserRequest, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Type"), "multipart") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("multipart: %v", err)
				http.Error(w, "invalid fixture request", http.StatusBadRequest)
				return
			}
			defer r.MultipartForm.RemoveAll()
		} else {
			_ = r.ParseForm()
		}
		requests <- pdfParserRequest{
			Path: r.URL.Path, Authorization: r.Header.Get("Authorization"),
			Key: r.FormValue("api_key"), Formats: r.FormValue("element_formats"),
			Features: r.FormValue("feature_config"),
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/parse/async":
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"local-task"}}`))
		case "/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"text","content":"Selected instance B."}]}]}}}}}`))
		case "/file_parse":
			_, _ = w.Write([]byte(`{"json_doc":{"type":"text","content":"Selected instance B."}}`))
		default:
			t.Errorf("unexpected parser route %s", r.URL.Path)
			http.Error(w, "unexpected fixture route", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	decoy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("another instance was selected: %s", r.URL.Path)
		http.Error(w, "wrong instance", http.StatusBadRequest)
	}))
	t.Cleanup(decoy.Close)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.TenantModelProvider{ID: "provider", ProviderName: provider, TenantID: "tenant-1"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, inst := range []struct{ id, url string }{{"a", decoy.URL}, {"b", server.URL}} {
		key := "fixture-key-" + inst.id
		extra := fmt.Sprintf(`{"base_url":%q}`, inst.url)
		if provider == "OpenDataLoader" {
			key = fmt.Sprintf(`{"api_key":{"opendataloader_apiserver":%q,"opendataloader_api_key":%q}}`, inst.url, key)
			extra = "{}"
		}
		for _, row := range []any{
			&entity.TenantModelInstance{ID: "instance-" + inst.id, ProviderID: "provider", InstanceName: inst.id, APIKey: key, Status: "active", Extra: extra},
			&entity.TenantModel{ID: "model-" + inst.id, ProviderID: "provider", InstanceID: "instance-" + inst.id,
				ModelName: provider + "-model", ModelType: int(entity.ModelTypeOCR), Status: "active",
				Extra: `{"somark_image_format":"base64","somark_enable_inline_image":false}`},
		} {
			if err := db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return db, provider + "-model@b@" + provider, requests
}

// TestTenantPDFParserModelSelection prevents UUID and composite references from
// being ignored, substituted with another instance or sent to generic VLM chat.
func TestTenantPDFParserModelSelection(t *testing.T) {
	originalRenderer := pdfVisionPageRenderer
	pdfVisionPageRenderer = func([]byte, [][]int) ([]pdfVisionPage, error) {
		return nil, fmt.Errorf("unexpected generic VLM path")
	}
	t.Cleanup(func() { pdfVisionPageRenderer = originalRenderer })
	for _, provider := range []string{"SoMark", "OpenDataLoader"} {
		for _, field := range []string{"parse_method", "layout_recognizer"} {
			for _, form := range []string{"id", "composite"} {
				t.Run(provider+"/"+field+"/"+form, func(t *testing.T) {
					db, composite, requests := tenantPDFParserFixture(t, provider)
					ref := composite
					if form == "id" {
						ref = "model-b"
					}
					setup := schema.ParserSetup{"parse_method": "deepdoc", field: ref}
					before, _ := json.Marshal(setup)
					res, handled, err := maybeDispatchPDFVision(t.Context(), db, utility.FileTypePDF,
						"sample.pdf", []byte("%PDF local protocol fixture"), map[string]any{"tenant_id": "tenant-1"},
						map[string]schema.ParserSetup{"pdf": setup})
					if err != nil || !handled || res.Err != nil {
						t.Fatalf("selected parser was not used: handled=%v err=%v result=%v", handled, err, res.Err)
					}
					body, _ := json.Marshal(res.JSON)
					if !strings.Contains(string(body), "Selected instance B.") {
						t.Fatalf("selected parser output missing: %s", body)
					}
					after, _ := json.Marshal(setup)
					if !reflect.DeepEqual(before, after) {
						t.Fatal("runtime credentials/options mutated saved setup")
					}
					if len(requests) == 0 {
						t.Fatal("selected parser never received an HTTP request")
					}
					for len(requests) > 0 {
						request := <-requests
						if provider == "SoMark" && request.Key != "fixture-key-b" {
							t.Fatalf("wrong SoMark instance credentials")
						}
						if provider == "OpenDataLoader" && request.Authorization != "Bearer fixture-key-b" {
							t.Fatalf("wrong OpenDataLoader instance credentials")
						}
					}
				})
			}
		}
	}
}

// TestTenantPDFParserRejectsInvalidSelection checks real stored records; a bad
// explicit selection must fail without an HTTP request or a different model.
func TestTenantPDFParserRejectsInvalidSelection(t *testing.T) {
	for _, provider := range []string{"SoMark", "OpenDataLoader"} {
		for _, tc := range []struct {
			name   string
			mutate func(*gorm.DB) error
			tenant string
		}{
			{"wrong-tenant", nil, "tenant-2"},
			{"missing-tenant", nil, ""},
			{"disabled-model", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModel{}).Where("id = ?", "model-b").Update("status", "inactive").Error
			}, "tenant-1"},
			{"wrong-type", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModel{}).Where("id = ?", "model-b").Update("model_type", int(entity.ModelTypeImage2Text)).Error
			}, "tenant-1"},
			{"disabled-instance", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("status", "inactive").Error
			}, "tenant-1"},
			{"missing-instance", func(db *gorm.DB) error {
				return db.Where("id = ?", "instance-b").Delete(&entity.TenantModelInstance{}).Error
			}, "tenant-1"},
			{"wrong-provider", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("provider_id", "other-provider").Error
			}, "tenant-1"},
			{"missing-endpoint", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Updates(map[string]any{"extra": "{}", "api_key": ""}).Error
			}, "tenant-1"},
			{"malformed-instance", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("extra", "{").Error
			}, "tenant-1"},
			{"malformed-key", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("api_key", "{").Error
			}, "tenant-1"},
			{"wrong-key-type", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("api_key", `{"api_key":false}`).Error
			}, "tenant-1"},
			{"malformed-model-options", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModel{}).Where("id = ?", "model-b").Update("extra", "{").Error
			}, "tenant-1"},
			{"invalid-option-type", func(db *gorm.DB) error {
				return db.Model(&entity.TenantModel{}).Where("id = ?", "model-b").Update("extra", `{"somark_enable_inline_image":"false"}`).Error
			}, "tenant-1"},
		} {
			if provider == "OpenDataLoader" && (tc.name == "malformed-model-options" || tc.name == "invalid-option-type") {
				continue // OpenDataLoader has no SoMark-specific model options.
			}
			for _, form := range []string{"id", "composite"} {
				t.Run(provider+"/"+tc.name+"/"+form, func(t *testing.T) {
					db, ref, requests := tenantPDFParserFixture(t, provider)
					if tc.mutate != nil {
						if err := tc.mutate(db); err != nil {
							t.Fatal(err)
						}
					}
					if form == "id" {
						ref = "model-b"
					}
					_, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("pdf"),
						tc.tenant, schema.ParserSetup{"parse_method": ref})
					if !handled || err == nil {
						t.Fatalf("bad selection did not fail: handled=%v err=%v", handled, err)
					}
					if len(requests) != 0 {
						t.Fatal("invalid selection sent a parser request")
					}
				})
			}
		}
	}
}

// TestTenantPDFParserMissingModelIDIsHandled keeps a well-formed but absent
// explicit reference terminal, rather than treating it as an unselected parser.
func TestTenantPDFParserMissingModelIDIsHandled(t *testing.T) {
	const missingModelID = "7f4e7a5ce0fa4abda74cd99b6a2f89c1"
	for _, provider := range []string{"SoMark", "OpenDataLoader"} {
		for _, field := range []string{"parse_method", "layout_recognizer"} {
			t.Run(provider+"/"+field, func(t *testing.T) {
				db, _, requests := tenantPDFParserFixture(t, provider)
				setup := schema.ParserSetup{"parse_method": "deepdoc", field: missingModelID}
				before, err := json.Marshal(setup)
				if err != nil {
					t.Fatal(err)
				}
				_, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("%PDF fixture"),
					"tenant-1", setup)
				if !handled {
					t.Error("missing explicit model was left for another parser")
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) || !strings.Contains(err.Error(), "tenant PDF model") {
					t.Errorf("missing explicit model must return a PDF model lookup error, got %v", err)
				}
				if len(requests) != 0 {
					t.Error("missing explicit model sent a parser request")
				}
				after, err := json.Marshal(setup)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Error("failed lookup mutated the saved setup")
				}
			})
		}
	}
}

// TestTenantPDFParserMissingModelIDInvokeDoesNotFallback exercises the real
// component with reloaded canvas params. Missing selections must neither reach
// another parser nor rewrite the persisted or in-memory setup.
func TestTenantPDFParserMissingModelIDInvokeDoesNotFallback(t *testing.T) {
	const missingModelID = "7f4e7a5ce0fa4abda74cd99b6a2f89c1"
	for _, tc := range []struct {
		field  string
		method string
	}{
		{"parse_method", "deepdoc"},
		{"layout_recognizer", "deepdoc"},
		{"layout_recognizer", "somark"},
		{"layout_recognizer", "opendataloader"},
	} {
		t.Run(tc.field+"/"+tc.method, func(t *testing.T) {
			db, _, requests := tenantPDFParserFixture(t, "SoMark")
			var httpCalls, genericCalls atomic.Int64
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":1,"message":"unexpected parser fallback"}`))
			}))
			t.Cleanup(fallback.Close)
			t.Setenv(common.EnvSOMarkBaseUrl, fallback.URL+"/environment/somark")
			t.Setenv(common.EnvSOMarkAPIKey, "environment-key-must-not-be-used")
			t.Setenv(common.EnvOpenDataLoaderAPIServer, fallback.URL+"/environment/opendataloader")
			t.Setenv(common.EnvOpenDataLoaderAPIKey, "environment-key-must-not-be-used")

			originalRenderer, originalResolver := pdfVisionPageRenderer, pdfVisionModelResolver
			originalPromptLoader, originalInvoker := pdfVisionPromptLoader, pdfVisionChatInvoker
			originalAnalyzerFactory := doctype.NativeDocAnalyzerFactory
			t.Cleanup(func() {
				pdfVisionPageRenderer, pdfVisionModelResolver = originalRenderer, originalResolver
				pdfVisionPromptLoader, pdfVisionChatInvoker = originalPromptLoader, originalInvoker
				doctype.SetNativeDocAnalyzerFactory(originalAnalyzerFactory)
			})
			var renderCalls, resolveCalls, promptCalls, chatCalls int
			// Let an erroneous VLM detour reach chat without native rendering
			// or a real model; the sentinel error cannot satisfy the lookup check.
			pdfVisionPageRenderer = func([]byte, [][]int) ([]pdfVisionPage, error) {
				renderCalls++
				return []pdfVisionPage{{PageNumber: 1, WidthPts: 100, HeightPts: 200, ImageURL: "data:image/png;base64,fixture"}}, nil
			}
			pdfVisionModelResolver = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
				resolveCalls++
				return nil, "unexpected-vlm", nil, nil
			}
			pdfVisionPromptLoader = func(string) (string, error) {
				promptCalls++
				return "Transcribe page {{ page }}.", nil
			}
			pdfVisionChatInvoker = func(context.Context, modelModule.ModelDriver, string, []modelModule.Message, *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
				chatCalls++
				return nil, errors.New("unexpected generic VLM fallback")
			}
			doctype.SetNativeDocAnalyzerFactory(func() (doctype.DocAnalyzer, bool) {
				genericCalls.Add(1)
				return nil, false
			})

			setup := map[string]any{
				"parse_method": tc.method, tc.field: missingModelID,
				"somark_base_url": fallback.URL + "/setup/somark", "somark_api_key": "setup-key-must-not-be-used",
				"opendataloader_apiserver": fallback.URL + "/setup/opendataloader",
				"opendataloader_api_key":   "setup-key-must-not-be-used",
			}
			if err := db.AutoMigrate(&entity.UserCanvas{}); err != nil {
				t.Fatal(err)
			}
			canvas := entity.UserCanvas{
				ID: "parser-canvas", UserID: "tenant-1", CanvasCategory: "dataflow_canvas",
				DSL: entity.JSONMap{"components": map[string]any{
					"Parser:fixture": map[string]any{"obj": map[string]any{
						"component_name": "Parser", "params": map[string]any{"pdf": setup},
					}},
				}},
			}
			if err := db.Create(&canvas).Error; err != nil {
				t.Fatal(err)
			}
			var reloaded entity.UserCanvas
			if err := db.Where("id = ?", canvas.ID).Take(&reloaded).Error; err != nil {
				t.Fatal(err)
			}
			saved, err := json.Marshal(reloaded.DSL)
			if err != nil {
				t.Fatal(err)
			}
			node := reloaded.DSL["components"].(map[string]any)["Parser:fixture"].(map[string]any)
			params := node["obj"].(map[string]any)["params"].(map[string]any)
			component, err := NewParserComponent(params)
			if err != nil {
				t.Fatal(err)
			}
			parserComponent := component.(*ParserComponent)
			runtimeSetup, err := json.Marshal(parserComponent.setups)
			if err != nil {
				t.Fatal(err)
			}
			out, err := component.Invoke(t.Context(), db, map[string]any{
				"binary": []byte("%PDF synthetic fixture"), "file_type": "pdf",
				"name": "sample.pdf", "tenant_id": "tenant-1",
			})
			if !errors.Is(err, gorm.ErrRecordNotFound) || !strings.Contains(err.Error(), "tenant PDF model") {
				t.Errorf("Invoke must report the missing PDF model, got %v", err)
			}
			if out != nil {
				t.Error("missing explicit model produced parser output")
			}
			if renderCalls != 0 || resolveCalls != 0 || promptCalls != 0 || chatCalls != 0 || genericCalls.Load() != 0 {
				t.Errorf("missing explicit model entered fallback: render=%d resolve=%d prompt=%d chat=%d generic=%d",
					renderCalls, resolveCalls, promptCalls, chatCalls, genericCalls.Load())
			}
			if httpCalls.Load() != 0 || len(requests) != 0 {
				t.Errorf("missing explicit model sent HTTP requests: fallback=%d tenant=%d", httpCalls.Load(), len(requests))
			}
			after, err := json.Marshal(reloaded.DSL)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(saved, after) {
				t.Error("failed invocation mutated reloaded canvas params")
			}
			after, err = json.Marshal(parserComponent.setups)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(runtimeSetup, after) {
				t.Error("failed invocation mutated the component setup")
			}
			var stored entity.UserCanvas
			if err := db.Where("id = ?", canvas.ID).Take(&stored).Error; err != nil {
				t.Fatal(err)
			}
			after, err = json.Marshal(stored.DSL)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(saved, after) {
				t.Error("failed invocation changed the persisted canvas")
			}
		})
	}
}

// TestTenantPDFParserEnvironmentIsolation protects optional anonymous keys and
// explicit false model options from unrelated global environment configuration.
func TestTenantPDFParserEnvironmentIsolation(t *testing.T) {
	t.Setenv(common.EnvSOMarkAPIKey, "global-secret-must-not-be-used")
	t.Setenv(common.EnvSOMarkEnableInlineImage, "true")
	t.Setenv(common.EnvOpenDataLoaderAPIKey, "global-secret-must-not-be-used")
	for _, provider := range []string{"SoMark", "OpenDataLoader"} {
		t.Run(provider, func(t *testing.T) {
			db, ref, requests := tenantPDFParserFixture(t, provider)
			var instance entity.TenantModelInstance
			if err := db.Where("id = ?", "instance-b").Take(&instance).Error; err != nil {
				t.Fatal(err)
			}
			update := map[string]any{"api_key": ""}
			if provider == "OpenDataLoader" {
				// Move this instance's URL to ordinary base_url storage, while
				// intentionally leaving its optional credential empty.
				var stored struct {
					APIKey struct {
						Endpoint string `json:"opendataloader_apiserver"`
					} `json:"api_key"`
				}
				if err := json.Unmarshal([]byte(instance.APIKey), &stored); err != nil {
					t.Fatal(err)
				}
				update["extra"] = fmt.Sprintf(`{"base_url":%q}`, stored.APIKey.Endpoint)
			}
			if err := db.Model(&instance).Updates(update).Error; err != nil {
				t.Fatal(err)
			}
			res, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("pdf"),
				"tenant-1", schema.ParserSetup{"parse_method": ref})
			if !handled || err != nil || res.Err != nil {
				t.Fatalf("anonymous selected instance: %v", err)
			}
			for len(requests) > 0 {
				request := <-requests
				if request.Key != "" || request.Authorization != "" {
					t.Fatal("selected anonymous instance borrowed global credentials")
				}
				if request.Path == "/parse/async" && !strings.Contains(request.Features, `"enable_inline_image":false`) {
					t.Fatalf("environment overrode explicit false: %s", request.Features)
				}
			}
		})
	}
}

// TestTenantPDFParserKeepsSSRFGuard proves explicit instance selection does not
// bypass the repository's production transport policy.
func TestTenantPDFParserKeepsSSRFGuard(t *testing.T) {
	db, ref, requests := tenantPDFParserFixture(t, "SoMark")
	common.AllowAnyHostForTest = false
	_, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("pdf"),
		"tenant-1", schema.ParserSetup{"parse_method": ref})
	if !handled || err == nil || !strings.Contains(err.Error(), "non-public address") || len(requests) != 0 {
		t.Fatalf("SSRF policy not preserved: handled=%v err=%v", handled, err)
	}
}

// TestTenantPDFParserConfigRoundTripAndRotation exercises normal component
// construction and output after a persisted reference is reloaded. Credentials
// are looked up again on each invocation, not cached or written into params.
func TestTenantPDFParserConfigRoundTripAndRotation(t *testing.T) {
	for _, provider := range []string{"SoMark", "OpenDataLoader"} {
		t.Run(provider, func(t *testing.T) {
			db, ref, requests := tenantPDFParserFixture(t, provider)
			params := map[string]any{"pdf": map[string]any{"parse_method": ref}}
			saved, _ := json.Marshal(params)
			var reloaded map[string]any
			if err := json.Unmarshal(saved, &reloaded); err != nil {
				t.Fatal(err)
			}
			component, err := NewParserComponent(reloaded)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"fixture-key-b", "rotated-fixture-key"} {
				if key == "rotated-fixture-key" {
					var instance entity.TenantModelInstance
					if err := db.Where("id = ?", "instance-b").Take(&instance).Error; err != nil {
						t.Fatal(err)
					}
					baseURL, _, err := tenantPDFParserCredentials(tenantPDFParserEngine(provider), &instance)
					if err != nil {
						t.Fatal(err)
					}
					if err := db.Model(&instance).Updates(map[string]any{
						"api_key": key, "extra": fmt.Sprintf(`{"base_url":%q}`, baseURL),
					}).Error; err != nil {
						t.Fatal(err)
					}
				}
				out, err := component.Invoke(t.Context(), db, map[string]any{
					"binary": []byte("%PDF local fixture"), "file_type": "pdf",
					"name": "sample.pdf", "tenant_id": "tenant-1",
				})
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(out)
				if !strings.Contains(string(body), "Selected instance B.") || strings.Contains(string(body), key) {
					t.Fatalf("component output missing text or exposing credentials: %s", body)
				}
				if len(requests) == 0 {
					t.Fatal("missing runtime parser request")
				}
				for len(requests) > 0 {
					request := <-requests
					if provider == "SoMark" && request.Key != key ||
						provider == "OpenDataLoader" && request.Authorization != "Bearer "+key {
						t.Fatal("runtime used stale or wrong credentials")
					}
				}
			}
			after, _ := json.Marshal(reloaded)
			if !reflect.DeepEqual(saved, after) {
				t.Fatal("component mutated persisted configuration")
			}
		})
	}
}

// TestTenantPDFParserBareEnginesAndOtherModels leaves existing named engines
// and other providers to their current dispatch paths.
func TestTenantPDFParserBareEnginesAndOtherModels(t *testing.T) {
	for _, selector := range []string{"", "somark", "OpenDataLoader", "deepdoc", "plain_text", "vision@b@OpenAI"} {
		_, handled, err := maybeDispatchTenantPDFParser(t.Context(), nil, "sample.pdf", []byte("pdf"),
			"tenant-1", schema.ParserSetup{"parse_method": selector})
		if handled || err != nil {
			t.Fatalf("unrelated/bare selector %q intercepted: %v", selector, err)
		}
	}
	for _, selector := range []string{"SoMark-model@b@SoMark", "OpenDataLoader-model@b@OpenDataLoader", "@b@SoMark"} {
		_, handled, err := maybeDispatchTenantPDFParser(t.Context(), nil, "sample.pdf", []byte("pdf"),
			"tenant-1", schema.ParserSetup{"parse_method": selector})
		if !handled || err == nil {
			t.Fatalf("explicit selection %q fell back without database: %v", selector, err)
		}
	}
}

// TestTenantPDFParserSelectorPrecedence shares the existing model-selector rule:
// a model in parse_method wins, otherwise layout_recognizer supplies the model.
func TestTenantPDFParserSelectorPrecedence(t *testing.T) {
	db, ref, requests := tenantPDFParserFixture(t, "SoMark")
	_, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("pdf"),
		"tenant-1", schema.ParserSetup{"parse_method": ref, "layout_recognizer": "model-a"})
	if !handled || err != nil || len(requests) != 2 {
		t.Fatalf("parse_method model did not take precedence: handled=%v err=%v", handled, err)
	}
}

// TestTenantPDFParserExplicitOptions keeps non-secret overrides while ignoring
// pipeline endpoint/key values in favor of the explicitly selected instance.
func TestTenantPDFParserExplicitOptions(t *testing.T) {
	db, ref, requests := tenantPDFParserFixture(t, "SoMark")
	setup := schema.ParserSetup{
		"parse_method": ref, "somark_image_format": "none", "somark_enable_inline_image": true,
		"somark_base_url": "https://not-the-selected-instance.invalid", "somark_api_key": "not-the-selected-key",
	}
	_, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("pdf"), "tenant-1", setup)
	if !handled || err != nil {
		t.Fatal(err)
	}
	request := <-requests
	if request.Key != "fixture-key-b" || !strings.Contains(request.Formats, `"image":"none"`) ||
		!strings.Contains(request.Features, `"enable_inline_image":true`) {
		t.Fatalf("instance credentials / pipeline option precedence mismatch: %#v", request)
	}
}

// TestTenantPDFParserFailureDoesNotFallback returns the selected backend's
// failure rather than silently trying another instance or a VLM.
func TestTenantPDFParserFailureDoesNotFallback(t *testing.T) {
	for _, provider := range []string{"SoMark", "OpenDataLoader"} {
		t.Run(provider, func(t *testing.T) {
			db, ref, otherRequests := tenantPDFParserFixture(t, provider)
			calls := make(chan struct{}, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls <- struct{}{}
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":1,"message":"fixture backend unavailable"}`))
			}))
			defer server.Close()
			if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Updates(map[string]any{
				"extra": fmt.Sprintf(`{"base_url":%q}`, server.URL), "api_key": "fixture-key-b",
			}).Error; err != nil {
				t.Fatal(err)
			}
			_, handled, err := maybeDispatchTenantPDFParser(t.Context(), db, "sample.pdf", []byte("pdf"), "tenant-1",
				schema.ParserSetup{"parse_method": ref})
			if !handled || err == nil || len(calls) != 1 || len(otherRequests) != 0 {
				t.Fatalf("selected backend failure fell back: handled=%v err=%v", handled, err)
			}
		})
	}
}
