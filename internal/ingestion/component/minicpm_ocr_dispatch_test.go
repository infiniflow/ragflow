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

package component

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"gorm.io/gorm"
)

// A tenant model UUID picked as the PDF parser — the web UI stores it in
// parse_method; layout_recognizer, when set, takes precedence — runs a
// MiniCPM-OCR job and yields structured PDF items instead of falling through
// to the image2text VLM path.
func TestDispatch_PDFMiniCPMOCR_ModelUUID(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/document-parse/jobs" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer minicpm-secret" {
			t.Errorf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"ocr_1","status":"Success","pages":[{"metadata":{"page_index":0},` +
			`"choices":[{"message":{"content":"# MiniCPM Title\n\nMiniCPM body."}}],` +
			`"ocr":{"image_width":1240,"image_height":1754,"blocks":[` +
			`{"order":0,"label":"doc_title","bbox":[100,80,800,150],"content":"MiniCPM Title","status":"ok"},` +
			`{"order":1,"label":"text","bbox":[100,200,1000,260],"content":"MiniCPM body.","status":"ok"}]}}]}`))
	}))
	defer server.Close()

	origSelector, origResolve := isMiniCPMOCRModelSelector, resolveModelConfig
	t.Cleanup(func() { isMiniCPMOCRModelSelector, resolveModelConfig = origSelector, origResolve })
	modelID := "5b6f1d0c8a2e4f3b9c7d1e2f3a4b5c6d"
	isMiniCPMOCRModelSelector = func(_ context.Context, _ *gorm.DB, tenantID, selector string) bool {
		return tenantID == "test-tenant" && selector == modelID
	}
	baseURL, apiKey := server.URL+"/v1", "minicpm-secret"
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		if tenantID != "test-tenant" || modelType != entity.ModelTypeOCR || modelRef != modelID {
			return nil, "", nil, 0, fmt.Errorf("unexpected model resolution: tenant=%q type=%v ref=%q", tenantID, modelType, modelRef)
		}
		driver := modelModule.NewMiniCPMOCRModel(nil, modelModule.URLSuffix{OCR: "document-parse/jobs"})
		return driver, "MiniCPM-OCR-1.0", &modelModule.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, 0, nil
	}

	for _, field := range []string{"parse_method", "layout_recognizer"} {
		t.Run(field, func(t *testing.T) {
			setups := defaultSetups()
			setups["pdf"][field] = modelID
			c := &ParserComponent{setups: setups}
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"binary":    []byte("%PDF-1.4"),
				"file_type": "pdf",
				"name":      "sample.pdf",
				"tenant_id": "test-tenant",
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			requireJSONText(t, out, "MiniCPM Title")
			requireJSONText(t, out, "MiniCPM body.")
		})
	}
}

func TestIsMiniCPMOCRModelSelector(t *testing.T) {
	for _, tc := range []struct {
		selector string
		want     bool
	}{
		{"MiniCPM-OCR-1.0@default@MiniCPM-OCR", true},
		{"MiniCPM-OCR-1.0@MiniCPM-OCR", true},
		{"PaddleOCR-VL-1.6@default@PaddleOCR", false},
		// A bare model UUID is resolved within the tenant, which needs a database.
		{"5b6f1d0c8a2e4f3b9c7d1e2f3a4b5c6d", false},
	} {
		if got := defaultIsMiniCPMOCRModelSelector(t.Context(), nil, "tenant", tc.selector); got != tc.want {
			t.Errorf("selector %q: got %v, want %v", tc.selector, got, tc.want)
		}
	}
}

func TestDispatchMiniCPMOCRPDFRejectsOtherDrivers(t *testing.T) {
	original := resolveModelConfig
	t.Cleanup(func() { resolveModelConfig = original })
	resolveModelConfig = func(context.Context, *gorm.DB, string, entity.ModelType, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &paddleOCRFakeDriver{}, "PaddleOCR-VL-1.6", &modelModule.APIConfig{}, 0, nil
	}
	_, err := dispatchMiniCPMOCRPDF(t.Context(), nil, "sample.pdf", []byte("%PDF-1.4"), "tenant", "PaddleOCR-VL-1.6@default@PaddleOCR")
	if err == nil || !strings.Contains(err.Error(), "requires a MiniCPM-OCR OCR model") {
		t.Fatalf("err=%v", err)
	}
	if _, err = dispatchMiniCPMOCRPDF(t.Context(), nil, "sample.pdf", []byte("%PDF-1.4"), "", "model"); err == nil || !strings.Contains(err.Error(), "requires tenant_id") {
		t.Fatalf("missing tenant err=%v", err)
	}
}
