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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
