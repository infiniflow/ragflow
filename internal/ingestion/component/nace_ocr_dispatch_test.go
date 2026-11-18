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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"gorm.io/gorm"
)

// A Nace NDI model picked as the PDF parser uploads the PDF, parses the
// configured pages and yields structured PDF items instead of falling through
// to the image2text VLM path, whichever setup field names it. A result with
// Markdown but no layout blocks still yields its text.
func TestDispatch_PDFNaceOCR(t *testing.T) {
	withSSRFBypass(t)
	var mu sync.Mutex
	var pageRanges []any
	blocksAnswer := `{"job_id":"job","status":"succeeded","result":{"document":{"page_count":1,` +
		`"markdown":"# Nace Title\n\nNace body.","blocks":[` +
		`{"type":"heading","content":"Nace Title","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.1],[0.6,0.15]]]}},` +
		`{"type":"text","content":"Nace body.","location":{"kind":"page_region","page":1,"polygons":[[[0.1,0.2],[0.9,0.3]]]}}]},` +
		`"disclosures":[]}}`
	parseAnswer := blocksAnswer
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload" && r.Header.Get("Authorization") != "Bearer nace-secret" {
			t.Errorf("%s %s: Authorization=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/documents/upload-grants":
			_, _ = io.WriteString(w, `{"workspace_id":"ws","upload_url":"`+server.URL+`/upload","token":"tok"}`)
		case "POST /upload":
			_, _ = io.WriteString(w, `{"job_id":"up","status":"succeeded","result":{"file":{"file_id":"f","workspace_id":"ws"}}}`)
		case "POST /v1/documents/parse":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			pageRanges, _ = body["page_ranges"].([]any)
			answer := parseAnswer
			mu.Unlock()
			_, _ = io.WriteString(w, answer)
		case "DELETE /v1/documents/workspaces/ws/files/f", "DELETE /v1/documents/jobs/job":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origSelector, origResolve := isNaceOCRModelSelector, resolveModelConfig
	t.Cleanup(func() { isNaceOCRModelSelector, resolveModelConfig = origSelector, origResolve })
	modelID := "7c1d2e3f4a5b46c7d8e9f0a1b2c3d4e5"
	isNaceOCRModelSelector = func(_ context.Context, _ *gorm.DB, tenantID, selector string) bool {
		return tenantID == "test-tenant" && selector == modelID
	}
	baseURL, apiKey := server.URL+"/v1", "nace-secret"
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		if tenantID != "test-tenant" || modelType != entity.ModelTypeOCR || modelRef != modelID {
			return nil, "", nil, 0, fmt.Errorf("unexpected model resolution: tenant=%q type=%v ref=%q", tenantID, modelType, modelRef)
		}
		driver := modelModule.NewNaceModel(nil, modelModule.URLSuffix{OCR: "documents/parse"})
		return driver, "ndi-parse-low", &modelModule.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, 0, nil
	}

	for name, tc := range map[string]struct {
		pdf        map[string]any
		pageRanges []any
		answer     string
		texts      []string
	}{
		"parse_method":      {pdf: map[string]any{"parse_method": modelID}},
		"layout_recognizer": {pdf: map[string]any{"layout_recognizer": modelID}},
		"pages": {
			pdf:        map[string]any{"parse_method": modelID, "pages": []any{[]any{1, 2}, []any{5, 5}}},
			pageRanges: []any{map[string]any{"start": float64(1), "end": float64(2)}, map[string]any{"start": float64(5), "end": float64(5)}},
		},
		"markdown without blocks": {
			pdf:    map[string]any{"parse_method": modelID},
			answer: `{"job_id":"job","status":"succeeded","result":{"document":{"page_count":1,"markdown":"Scanned text only.","blocks":[]},"disclosures":[]}}`,
			texts:  []string{"Scanned text only."},
		},
	} {
		t.Run(name, func(t *testing.T) {
			mu.Lock()
			parseAnswer = blocksAnswer
			if tc.answer != "" {
				parseAnswer = tc.answer
			}
			mu.Unlock()
			texts := tc.texts
			if texts == nil {
				texts = []string{"Nace Title", "Nace body."}
			}
			setups := defaultSetups()
			for field, value := range tc.pdf {
				setups["pdf"][field] = value
			}
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
			for _, text := range texts {
				requireJSONText(t, out, text)
			}
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(pageRanges, tc.pageRanges) {
				t.Fatalf("page_ranges=%v, want %v", pageRanges, tc.pageRanges)
			}
		})
	}
}

func TestIsNaceOCRModelSelector(t *testing.T) {
	for _, tc := range []struct {
		selector string
		want     bool
	}{
		{"ndi-parse-low@default@Nace", true},
		{"ndi-parse-high@Nace", true},
		{"MiniCPM-OCR-1.0@default@MiniCPM-OCR", false},
		{"PaddleOCR-VL-1.6@default@PaddleOCR", false},
		// A bare model UUID is resolved within the tenant, which needs a database.
		{"7c1d2e3f4a5b46c7d8e9f0a1b2c3d4e5", false},
	} {
		if got := defaultIsNaceOCRModelSelector(t.Context(), nil, "tenant", tc.selector); got != tc.want {
			t.Errorf("selector %q: got %v, want %v", tc.selector, got, tc.want)
		}
	}
}

func TestDispatchNaceOCRPDFRejectsOtherDrivers(t *testing.T) {
	original := resolveModelConfig
	t.Cleanup(func() { resolveModelConfig = original })
	resolveModelConfig = func(context.Context, *gorm.DB, string, entity.ModelType, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &paddleOCRFakeDriver{}, "PaddleOCR-VL-1.6", &modelModule.APIConfig{}, 0, nil
	}
	_, err := dispatchNaceOCRPDF(t.Context(), nil, "sample.pdf", []byte("%PDF-1.4"), "tenant", "PaddleOCR-VL-1.6@default@PaddleOCR", nil)
	if err == nil || !strings.Contains(err.Error(), "requires a Nace OCR model") {
		t.Fatalf("err=%v", err)
	}
	if _, err = dispatchNaceOCRPDF(t.Context(), nil, "sample.pdf", []byte("%PDF-1.4"), "", "model", nil); err == nil || !strings.Contains(err.Error(), "requires tenant_id") {
		t.Fatalf("missing tenant err=%v", err)
	}
}
