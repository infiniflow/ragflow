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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

func TestDispatchUnlimitedOCR2PDFEndToEnd(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"msg":     "Task accepted",
				"task_id": "test_task_123",
				"data": map[string]any{
					"task_id": "test_task_123",
				},
			})
		case "/tasks/test_task_123/result":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":  0,
				"msg":   "success",
				"state": "done",
				"results": map[string]any{
					"test.pdf": map[string]any{
						"md_content": "# Unlimited-OCR-2 Result\n\nRecognized text content from Baidu Unlimited-OCR-2.",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": mockServer.URL,
	}

	result, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"test.pdf",
		[]byte("%PDF-1.4 test"),
		"",
		setup,
		"",
	)
	if err != nil {
		t.Fatalf("dispatchUnlimitedOCR2PDF: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	if len(result.JSON) == 0 {
		t.Fatalf("expected non-empty JSON items in result")
	}

	found := false
	for _, it := range result.JSON {
		m := it.ToMap()
		if text, ok := m["text"].(string); ok && strings.Contains(text, "Unlimited-OCR-2 Result") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected parsed items to contain 'Unlimited-OCR-2 Result', got items: %#v", result.JSON)
	}
}

func TestDispatchUnlimitedOCR2PDF_ExplicitModelRefError(t *testing.T) {
	origResolver := resolveModelConfig
	t.Cleanup(func() { resolveModelConfig = origResolver })

	resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, _ string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return nil, "", nil, 0, fmt.Errorf("model disabled or not found")
	}

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": "http://127.0.0.1:18000",
	}

	_, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"test.pdf",
		[]byte("%PDF-1.4 test"),
		"tenant-1",
		setup,
		"explicit-model-id",
	)
	if err == nil {
		t.Fatal("expected error when explicit modelRef fails resolution, got nil")
	}
	if !strings.Contains(err.Error(), "Unlimited-OCR-2 model") {
		t.Fatalf("expected 'Unlimited-OCR-2 model' error, got: %v", err)
	}
}

func TestDispatchUnlimitedOCR2PDF_AuthFailureStopsPolling(t *testing.T) {
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"msg":     "Task accepted",
				"task_id": "test_auth_fail",
			})
		case "/tasks/test_auth_fail/result":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"msg":"unauthorized"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": mockServer.URL,
	}

	start := time.Now()
	_, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"test.pdf",
		[]byte("%PDF-1.4 test"),
		"",
		setup,
		"",
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !errors.Is(err, modelModule.ErrTaskAuthorization) {
		t.Fatalf("expected ErrTaskAuthorization, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("polling took %s, expected immediate return on 401", elapsed)
	}
	if callCount > 2 {
		t.Fatalf("ShowTask called %d times, expected immediate stop", callCount)
	}
}

func TestDispatchUnlimitedOCR2PDF_WithContentListVisualGrounding(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"msg":     "Task accepted",
				"task_id": "task_grounding",
				"data": map[string]any{
					"task_id": "task_grounding",
				},
			})
		case "/tasks/task_grounding/result":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":  0,
				"msg":   "success",
				"state": "done",
				"results": map[string]any{
					"grounding.pdf": map[string]any{
						"md_content": "# Grounding Title\n\nGrounding paragraph.",
						"content_list": []map[string]any{
							{
								"type":     "title",
								"text":     "Grounding Title",
								"page_idx": 0,
								"bbox":     []float64{100, 100, 800, 200},
							},
							{
								"type":     "text",
								"text":     "Grounding paragraph.",
								"page_idx": 0,
								"bbox":     []float64{100, 220, 800, 400},
							},
							{
								"type":       "table",
								"table_body": "<table><tr><td>Col 1</td><td>Col 2</td></tr></table>",
								"page_idx":   0,
								"bbox":       []float64{100, 420, 800, 700},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": mockServer.URL,
	}

	pdfPath := filepath.Join("..", "..", "deepdoc", "parser", "pdf", "pdfium", "testdata", "page_sizes.pdf")
	pdfBinary, err := os.ReadFile(pdfPath)
	if err != nil {
		pdfBinary = []byte("%PDF-1.4 test")
	}

	result, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"grounding.pdf",
		pdfBinary,
		"",
		setup,
		"",
	)
	if err != nil {
		t.Fatalf("dispatchUnlimitedOCR2PDF with visual grounding: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	if len(result.JSON) < 3 {
		t.Fatalf("expected at least 3 structured items, got %d", len(result.JSON))
	}

	sizes, pageSizesErr := mineruPageSizes(context.Background(), pdfBinary)
	titleFound := false
	tableFound := false
	for _, it := range result.JSON {
		m := it.ToMap()
		layout, _ := m["layout"].(string)
		if layout == "title" {
			titleFound = true
			if text, _ := m["text"].(string); text != "Grounding Title" {
				t.Errorf("expected title text 'Grounding Title', got %q", text)
			}
			if pageNo, _ := m["page_number"].(int); pageNo != 1 {
				t.Errorf("expected page_number 1, got %v", pageNo)
			}
			if pageSizesErr == nil && len(sizes) > 0 {
				posRaw, ok := m["positions"].([][]any)
				if !ok || len(posRaw) == 0 {
					t.Fatalf("expected positions in title under CGO, got %#v", m["positions"])
				}
				row := posRaw[0]
				if len(row) != 5 {
					t.Fatalf("expected 5-element position row, got %#v", row)
				}
				if page, _ := row[0].(int); page != 1 {
					t.Errorf("expected position page 1, got %v", page)
				}
				left, _ := row[1].(float64)
				right, _ := row[2].(float64)
				top, _ := row[3].(float64)
				bottom, _ := row[4].(float64)
				if math.Abs(left-59.55) > 1e-2 || math.Abs(right-476.4) > 1e-2 || math.Abs(top-84.2) > 1e-2 || math.Abs(bottom-168.4) > 1e-2 {
					t.Errorf("unexpected visual grounding positions: %v, want approx [1 59.55 476.4 84.2 168.4]", row)
				}
			} else {
				// Non-CGO environment: verify positions are cleanly omitted without failure
				if _, ok := m["positions"]; ok {
					t.Errorf("expected no positions without CGO page sizes, got %#v", m["positions"])
				}
			}
		}
		if layout == "table" {
			tableFound = true
		}
	}

	if !titleFound {
		t.Errorf("expected title item in parsed JSON")
	}
	if !tableFound {
		t.Errorf("expected table item in parsed JSON")
	}
}

func TestDispatchUnlimitedOCR2PDF_TenantCredential_NoCallerOverride(t *testing.T) {
	callerHit := false
	callerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callerHit = true
		http.Error(w, "should not be called", http.StatusForbidden)
	}))
	defer callerServer.Close()

	envHit := false
	envServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envHit = true
		switch r.URL.Path {
		case "/file_parse":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer secret-tenant-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"msg":     "success",
				"task_id": "env_task_123",
			})
		case "/tasks/env_task_123/result":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":  0,
				"msg":   "success",
				"state": "done",
				"results": map[string]any{
					"test.pdf": map[string]any{
						"md_content": "# Clean Secure Content",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer envServer.Close()

	origEnv := os.Getenv(common.EnvUnlimitedOCRServerURL)
	t.Setenv(common.EnvUnlimitedOCRServerURL, envServer.URL)
	defer os.Setenv(common.EnvUnlimitedOCRServerURL, origEnv)

	origResolver := resolveTenantOCRModelByProvider
	t.Cleanup(func() { resolveTenantOCRModelByProvider = origResolver })

	token := "secret-tenant-token"
	resolveTenantOCRModelByProvider = func(ctx context.Context, db *gorm.DB, tenantID, providerName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		driver := modelModule.NewUnlimitedOCRModel(
			map[string]string{"default": envServer.URL},
			modelModule.URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
		)
		return driver, "Unlimited-OCR-2", &modelModule.APIConfig{
			ApiKey: &token,
		}, 0, nil
	}

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": callerServer.URL,
	}

	result, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"test.pdf",
		[]byte("%PDF-1.4 test"),
		"tenant-sec-1",
		setup,
		"",
	)
	if err != nil {
		t.Fatalf("dispatchUnlimitedOCR2PDF unexpected error: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}

	if callerHit {
		t.Errorf("security violation: caller-supplied server was contacted with tenant credential")
	}
	if !envHit {
		t.Errorf("expected deployment environment server to be contacted")
	}
}

func TestDispatchUnlimitedOCR2PDF_TenantCredentialFailsClosedWithoutURL(t *testing.T) {
	callerHit := false
	callerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callerHit = true
		http.Error(w, "should not be reached", http.StatusInternalServerError)
	}))
	defer callerServer.Close()

	t.Setenv(common.EnvUnlimitedOCRServerURL, "")
	t.Setenv(common.EnvUnlimitedOCRAPIServer, "")

	origResolver := resolveTenantOCRModelByProvider
	t.Cleanup(func() { resolveTenantOCRModelByProvider = origResolver })

	token := "secret-tenant-token"
	resolveTenantOCRModelByProvider = func(ctx context.Context, db *gorm.DB, tenantID, providerName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		driver := modelModule.NewUnlimitedOCRModel(
			map[string]string{"default": "http://placeholder"},
			modelModule.URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
		)
		return driver, "Unlimited-OCR-2", &modelModule.APIConfig{
			ApiKey: &token,
		}, 0, nil
	}

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": callerServer.URL,
	}

	_, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"test.pdf",
		[]byte("%PDF-1.4 test"),
		"tenant-sec-fail-closed",
		setup,
		"",
	)
	if err == nil {
		t.Fatalf("expected error when tenant credential has no model URL and environmentURL is empty, got nil")
	}
	expectedMsg := "Unlimited-OCR-2 tenant credential requires a server URL on the model or " + common.EnvUnlimitedOCRServerURL
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Fatalf("expected error containing %q, got: %v", expectedMsg, err)
	}
	if callerHit {
		t.Errorf("security violation: caller-supplied server was contacted")
	}
}

func TestUnlimitedOCR2RequestTimeout(t *testing.T) {
	// 1. Default fallback is 30 minutes
	t.Setenv(common.EnvUnlimitedOCRTimeout, "")
	if got := unlimitedOCR2RequestTimeout(schema.ParserSetup{}, nil); got != 30*time.Minute {
		t.Fatalf("expected default 30m, got %v", got)
	}

	// 2. Environment variable override
	t.Setenv(common.EnvUnlimitedOCRTimeout, "900")
	if got := unlimitedOCR2RequestTimeout(schema.ParserSetup{}, nil); got != 15*time.Minute {
		t.Fatalf("expected 15m from env, got %v", got)
	}

	// 3. APIConfig override takes precedence over environment variable
	apiKeyJSON := `{"unlimited_ocr_timeout": 600}`
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKeyJSON}
	if got := unlimitedOCR2RequestTimeout(schema.ParserSetup{}, apiConfig); got != 10*time.Minute {
		t.Fatalf("expected 10m from apiConfig, got %v", got)
	}

	// 4. Setup parameter takes precedence over APIConfig and environment variable
	setup := schema.ParserSetup{"unlimited_ocr_timeout": 120}
	if got := unlimitedOCR2RequestTimeout(setup, apiConfig); got != 2*time.Minute {
		t.Fatalf("expected 2m from setup, got %v", got)
	}
}
