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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

// imageOCRModelLookupCounts observes GORM query executions after fixture setup,
// excluding DryRun subquery construction. The returned counts are read after
// the synchronous resolver call, not concurrently with queries.
func imageOCRModelLookupCounts(t *testing.T, db *gorm.DB) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	if err := db.Callback().Query().Before("gorm:query").Register("test:image_ocr_model_lookups", func(tx *gorm.DB) {
		if tx.DryRun {
			return
		}
		switch tx.Statement.Table {
		case "tenant_model", "tenant_model_instance", "tenant_model_provider":
			counts[tx.Statement.Table]++
		}
	}); err != nil {
		t.Fatal(err)
	}
	return counts
}

// imageOCRInstanceBaseURL returns instance B's fixture endpoint, allowing tests
// to clear Extra and redirect cloud defaults without contacting a public service.
func imageOCRInstanceBaseURL(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var instance entity.TenantModelInstance
	if err := db.Where("id = ?", "instance-b").Take(&instance).Error; err != nil {
		t.Fatal(err)
	}
	var extra struct {
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil || extra.BaseURL == "" {
		t.Fatalf("fixture base URL missing: %v", err)
	}
	return extra.BaseURL
}

// setImageOCRCloudDefault keeps fallback requests on a local fixture and
// restores the shared catalog before the fixture is closed.
// Callers must not run in parallel because the catalog is process-wide.
func setImageOCRCloudDefault(t *testing.T, baseURL string) {
	t.Helper()
	provider := dao.GetModelProviderManager().FindProvider("PaddleOCR")
	if provider == nil || provider.URL == nil {
		t.Fatal("PaddleOCR catalog URL missing")
	}
	previous, existed := provider.URL["default"]
	provider.URL["default"] = baseURL
	t.Cleanup(func() {
		if existed {
			provider.URL["default"] = previous
		} else {
			delete(provider.URL, "default")
		}
	})
}

// observeImageOCRWarnings captures Warn-and-higher logs after initializing the
// shared log level needed by error paths. Cleanup restores the previous logger,
// sugar logger, and any saved level; callers must not run in parallel.
func observeImageOCRWarnings(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	previousLogger, previousSugar := common.Logger, common.Sugar
	var previousLevel string
	if previousLogger != nil {
		previousLevel = common.GetLogLevel()
	}
	t.Cleanup(func() {
		common.Logger, common.Sugar = previousLogger, previousSugar
		if previousLevel != "" {
			if err := common.SetLogLevel(previousLevel); err != nil {
				t.Errorf("restore log level: %v", err)
			}
		}
	})
	// Error logging also reads the shared AtomicLevel, which a zap observer
	// alone does not initialize. Empty FileOutput keeps this fixture file-free.
	if err := common.InitLogger("warn", common.FileOutput{}, ""); err != nil {
		t.Fatalf("initialize OCR test logger: %v", err)
	}
	core, logs := observer.New(zap.WarnLevel)
	common.Logger = zap.New(core)
	common.Sugar = common.Logger.Sugar()
	return logs
}

// TestImageOCRSelectedModelLookupOnce checks that cloud and local image OCR
// each issue one top-level GORM query for the selected model, instance and
// provider. The model query's provider subquery is not counted separately;
// neither is the cloud driver's HTTP polling.
func TestImageOCRSelectedModelLookupOnce(t *testing.T) {
	for _, provider := range []string{"PaddleOCR.local", "PaddleOCR"} {
		t.Run(provider, func(t *testing.T) {
			db, _ := imageOCRFixture(t, provider, false)
			counts := imageOCRModelLookupCounts(t, db)
			text, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
			if err != nil || text != "Selected instance B." {
				t.Fatalf("selected OCR failed: text=%q err=%v", text, err)
			}
			for _, table := range []string{"tenant_model", "tenant_model_instance", "tenant_model_provider"} {
				if counts[table] != 1 {
					t.Errorf("%s lookups=%d, want exactly one; counts=%v", table, counts[table], counts)
				}
			}
		})
	}
}

// TestImageOCRInaccessibleSelectionProgressDoesNotRevealModelState compares
// actual Parser progress warnings for missing and foreign selections. Foreign
// record states must not alter that message, reveal the owner, or send an OCR
// request; rejection must still preserve the image attachment.
func TestImageOCRInaccessibleSelectionProgressDoesNotRevealModelState(t *testing.T) {
	for _, tc := range []struct {
		name, table, id, field string
		value                  any
	}{
		{name: "active foreign model"},
		{name: "disabled foreign model", table: "tenant_model", id: "model-b", field: "status", value: "inactive"},
		{name: "wrong foreign model type", table: "tenant_model", id: "model-b", field: "model_type", value: int(entity.ModelTypeChat)},
		{name: "missing foreign instance", table: "tenant_model", id: "model-b", field: "instance_id", value: "missing-instance"},
		{name: "disabled foreign instance", table: "tenant_model_instance", id: "instance-b", field: "status", value: "inactive"},
		{name: "mismatched foreign instance provider", table: "tenant_model_instance", id: "instance-b", field: "provider_id", value: "other-provider"},
		{name: "missing foreign provider", table: "tenant_model", id: "model-b", field: "provider_id", value: "missing-provider"},
		{name: "unknown foreign driver", table: "tenant_model_provider", id: "provider", field: "provider_name", value: "missing-driver"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, requests := imageOCRFixture(t, "PaddleOCR.local", false)
			if tc.table != "" {
				if err := db.Table(tc.table).Where("id = ?", tc.id).Update(tc.field, tc.value).Error; err != nil {
					t.Fatal(err)
				}
			}
			data := picturePNG(t)
			warningFor := func(ref string) string {
				t.Helper()
				component, err := NewParserComponent(map[string]any{
					"image": map[string]any{"ocr_enabled": true, "ocr_model_id": ref},
				})
				if err != nil {
					t.Fatal(err)
				}
				var warnings []string
				ctx := runtime.WithProgressMessageCallback(t.Context(), func(_ string, message string) {
					if strings.HasPrefix(message, "WARNING: image OCR unavailable:") {
						warnings = append(warnings, message)
					}
				})
				out, err := component.Invoke(ctx, db, map[string]any{
					"name": "invoice.png", "binary": data, "tenant_id": "tenant-2",
				})
				if err != nil {
					t.Fatal(err)
				}
				item := out["json"].([]parser.DocItem)[0].ToMap()
				if item["text"] != "" || item["image"] == "" || item["image"] == nil {
					t.Fatalf("rejection lost the image or substituted OCR text: %+v", item)
				}
				if len(warnings) != 1 {
					t.Fatalf("expected one actual Parser warning, got %q", warnings)
				}
				return warnings[0]
			}
			want := warningFor("missing-model")
			got := warningFor("model-b")
			if got != want || !strings.Contains(strings.ToLower(got), "not found") {
				t.Errorf("foreign model is distinguishable: got %q, missing %q", got, want)
			}
			if strings.Contains(got, "tenant-1") {
				t.Errorf("owner tenant ID leaked in progress: %q", got)
			}
			if len(requests.list()) != 0 {
				t.Fatalf("inaccessible selection sent an OCR request: %v", requests.list())
			}
		})
	}
}

// TestImageOCRCloudDefaultEndpointWarning checks that choosing a catalog
// endpoint emits one ID-only warning even if submission fails. The catalog
// points to a local server, while the real driver supplies the selected key,
// model, and image bytes.
func TestImageOCRCloudDefaultEndpointWarning(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote_failure=%v", fail), func(t *testing.T) {
			db, requests := imageOCRFixture(t, "PaddleOCR", fail)
			baseURL := imageOCRInstanceBaseURL(t, db)
			setImageOCRCloudDefault(t, baseURL)
			if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").
				Update("extra", "{}").Error; err != nil {
				t.Fatal(err)
			}
			logs := observeImageOCRWarnings(t)
			data := picturePNG(t)
			ctx := common.WithSessionID(t.Context(), "image-ocr-fallback")
			text, err := extractSelectedImageOCR(ctx, db, "tenant-1", "model-b", data)
			if fail {
				if err == nil || !strings.Contains(err.Error(), "fixture OCR unavailable") {
					t.Fatalf("remote error was lost: %v", err)
				}
			} else if err != nil || text != "Selected instance B." {
				t.Fatalf("cloud fallback failed: text=%q err=%v", text, err)
			}
			got := requests.list()
			if len(got) == 0 {
				t.Fatal("cloud catalog endpoint was not used")
			}
			req := got[0]
			if req.path != "/v2/ocr/jobs" || req.auth != "Bearer fixture-key-b" ||
				req.model != "PaddleOCR-VL-1.5" || req.filename != "document.png" || !bytes.Equal(req.content, data) {
				t.Fatalf("fallback changed the selected instance or image upload: %+v", req)
			}
			warnings := logs.FilterLevelExact(zap.WarnLevel).All()
			if len(warnings) != 1 {
				t.Fatalf("fallback warnings=%d, want exactly one; logs=%v", len(warnings), logs.All())
			}
			entry := warnings[0]
			wantFields := map[string]string{
				"tenant_id": "tenant-1", "model_id": "model-b", "instance_id": "instance-b",
				"session_id": "image-ocr-fallback",
			}
			fields := entry.ContextMap()
			if len(fields) != len(wantFields) {
				t.Errorf("fallback warning must contain only IDs, got %v", fields)
			}
			for key, value := range wantFields {
				if fields[key] != value {
					t.Errorf("warning %s=%v, want %q", key, fields[key], value)
				}
			}
			if !strings.Contains(strings.ToLower(entry.Message), "default") ||
				strings.Contains(entry.Message, baseURL) || strings.Contains(entry.Message, "fixture-key-b") {
				t.Errorf("fallback warning must describe the default without URL or key: %q", entry.Message)
			}
		})
	}
}

// TestImageOCRExplicitEndpointDoesNotWarn checks Extra-over-APIKey-JSON endpoint
// precedence for both drivers. Explicit endpoints must neither contact the
// catalog fallback server nor emit its warning.
func TestImageOCRExplicitEndpointDoesNotWarn(t *testing.T) {
	for _, provider := range []string{"PaddleOCR.local", "PaddleOCR"} {
		for _, source := range []string{"extra", "api_key_json", "extra_over_api_key_json"} {
			t.Run(provider+"/"+source, func(t *testing.T) {
				db, requests := imageOCRFixture(t, provider, false)
				baseURL := imageOCRInstanceBaseURL(t, db)
				fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("explicit endpoint unexpectedly used the cloud catalog default")
					http.Error(w, "unexpected catalog fallback", http.StatusBadRequest)
				}))
				t.Cleanup(fallback.Close)
				setImageOCRCloudDefault(t, fallback.URL)
				if source == "api_key_json" {
					key := fmt.Sprintf(`{"api_key":{"PADDLEOCR_BASE_URL":%q,"PADDLEOCR_ACCESS_TOKEN":"fixture-key-b"}}`, baseURL)
					if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").
						Updates(map[string]any{"api_key": key, "extra": "{}"}).Error; err != nil {
						t.Fatal(err)
					}
				} else if source == "extra_over_api_key_json" {
					key := fmt.Sprintf(`{"api_key":{"PADDLEOCR_BASE_URL":%q,"PADDLEOCR_ACCESS_TOKEN":"fixture-key-b"}}`, fallback.URL)
					if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").
						Update("api_key", key).Error; err != nil {
						t.Fatal(err)
					}
				}
				logs := observeImageOCRWarnings(t)
				text, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
				if err != nil || text != "Selected instance B." {
					t.Fatalf("explicit endpoint failed: text=%q err=%v", text, err)
				}
				got := requests.list()
				if len(got) == 0 || got[0].auth != "Bearer fixture-key-b" {
					t.Fatalf("selected explicit endpoint or key was not used: %v", got)
				}
				if warnings := logs.FilterLevelExact(zap.WarnLevel).All(); len(warnings) != 0 {
					t.Fatalf("explicit endpoint emitted fallback warnings: %v", warnings)
				}
			})
		}
	}
}

// TestImageOCRMissingEndpointDoesNotUseGlobalEndpoint rejects incomplete
// selected instances for both drivers when the cloud catalog URL is also empty.
// Process-global endpoints must not fill the gap or produce a fallback warning.
func TestImageOCRMissingEndpointDoesNotUseGlobalEndpoint(t *testing.T) {
	for _, provider := range []string{"PaddleOCR.local", "PaddleOCR"} {
		t.Run(provider, func(t *testing.T) {
			db, requests := imageOCRFixture(t, provider, false)
			setImageOCRCloudDefault(t, "")
			if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").
				Update("extra", "{}").Error; err != nil {
				t.Fatal(err)
			}
			global := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("missing selected endpoint borrowed the process-global OCR server")
				http.Error(w, "unexpected global endpoint", http.StatusBadRequest)
			}))
			t.Cleanup(global.Close)
			t.Setenv("PADDLEOCR_BASE_URL", global.URL)
			t.Setenv("PADDLEOCR_API_URL", global.URL)
			logs := observeImageOCRWarnings(t)
			_, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
			if err == nil || !strings.Contains(err.Error(), "no base URL") || len(requests.list()) != 0 {
				t.Fatalf("missing selected endpoint must fail without a request: err=%v requests=%v", err, requests.list())
			}
			if warnings := logs.FilterLevelExact(zap.WarnLevel).All(); len(warnings) != 0 {
				t.Fatalf("no endpoint was chosen, but a fallback warning was emitted: %v", warnings)
			}
		})
	}
}
