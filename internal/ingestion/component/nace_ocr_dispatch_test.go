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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

// A Nace NDI model picked as the PDF parser uploads the PDF, parses the
// configured pages and yields structured PDF items instead of falling through
// to the image2text VLM path, whichever setup field names it. A composite
// selector on an instance named like another parser (MinerU, PaddleOCR,
// SoMark, Unlimited-OCR) still reaches Nace. A result with Markdown but no
// layout blocks still yields its text.
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
	composites := map[string]bool{}
	for _, instance := range []string{"mineru-prod", "paddleocr-prod", "somark-prod", "unlimited-ocr-prod"} {
		composites["ndi-parse-low@"+instance+"@Nace"] = true
	}
	isNaceOCRModelSelector = func(_ context.Context, _ *gorm.DB, tenantID, selector string) bool {
		return tenantID == "test-tenant" && (selector == modelID || composites[selector])
	}
	baseURL, apiKey := server.URL+"/v1", "nace-secret"
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		if tenantID != "test-tenant" || modelType != entity.ModelTypeOCR || (modelRef != modelID && !composites[modelRef]) {
			return nil, "", nil, 0, fmt.Errorf("unexpected model resolution: tenant=%q type=%v ref=%q", tenantID, modelType, modelRef)
		}
		driver := modelModule.NewNaceModel(nil, modelModule.URLSuffix{OCR: "documents/parse"})
		return driver, "ndi-parse-low", &modelModule.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, 0, nil
	}

	cases := map[string]struct {
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
	}
	for composite := range composites {
		cases["layout_recognizer "+composite] = struct {
			pdf        map[string]any
			pageRanges []any
			answer     string
			texts      []string
		}{pdf: map[string]any{"layout_recognizer": composite}}
	}
	for name, tc := range cases {
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

// A parser is named by the model or the provider segment of a composite
// selector, never by its instance; a bare parse_method or layout_recognizer
// keeps its own rule (a layout_recognizer may start with the name).
func TestSelectorNamesParser(t *testing.T) {
	for _, tc := range []struct {
		value string
		model bool
		want  bool
	}{
		{"mineru", false, false},
		{"mineru", true, true},
		{"mineru-pipeline", true, true},
		{"mineru-pipeline", false, false},
		{"deepdoc", true, false},
		{"mineru-model@default@mineru", false, true},
		{"mineru-model@default@mineru", true, true},
		{"mineru-model@mineru.net", true, true},
		{"mineru-model@default@giteeai", true, true},
		{"mineru-model@default@giteeai", false, false},
		{"ndi-parse-low@mineru-prod@nace", true, false},
		{"ndi-parse-low@mineru-prod@nace", false, false},
		{"@b@mineru", false, true},
	} {
		if got := selectorNamesParser(tc.value, tc.model, parserPrefix("mineru")); got != tc.want {
			t.Errorf("selectorNamesParser(%q, %v)=%v, want %v", tc.value, tc.model, got, tc.want)
		}
	}
}

// dispatchPDFBackend picks a parser from every form of its name that it took
// before: bare name, tenant model UUID, model@provider,
// model@instance@provider in any case, and a parser-named model served by
// another provider. An NDI model on an instance named like a parser goes to
// NDI.
func TestDispatchPDFBackendParserNameForms(t *testing.T) {
	errProbe := errors.New("probe")
	var called string
	record := func(name string) {
		if called == "" {
			called = name
		}
	}
	resolver := func(name string) func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
			record(name)
			return nil, "", nil, errProbe
		}
	}
	uuid := func(name string) func(context.Context, *gorm.DB, string, string) bool {
		return func(_ context.Context, _ *gorm.DB, _, selector string) bool { return selector == "uuid-"+name }
	}
	origConfig, origProvider := resolveModelConfig, resolveTenantOCRModelByProvider
	origMinerU, origPaddle, origSoMark, origMonkey := resolveMinerUModelForDispatch, resolvePaddleOCRModelForDispatch, resolveSoMarkModelForDispatch, resolveMonkeyOCRModelForDispatch
	origIDs := []func(context.Context, *gorm.DB, string, string) bool{isMinerULayoutModelID, isPaddleOCRLayoutModelID, isSoMarkLayoutModelID, isUnlimitedOCRLayoutModelID, isMonkeyOCRLayoutModelID, isMonkeyOCRv2LayoutModelID, isNaceOCRModelSelector, isMiniCPMOCRModelSelector}
	t.Cleanup(func() {
		resolveModelConfig, resolveTenantOCRModelByProvider = origConfig, origProvider
		resolveMinerUModelForDispatch, resolvePaddleOCRModelForDispatch, resolveSoMarkModelForDispatch, resolveMonkeyOCRModelForDispatch = origMinerU, origPaddle, origSoMark, origMonkey
		isMinerULayoutModelID, isPaddleOCRLayoutModelID, isSoMarkLayoutModelID, isUnlimitedOCRLayoutModelID = origIDs[0], origIDs[1], origIDs[2], origIDs[3]
		isMonkeyOCRLayoutModelID, isMonkeyOCRv2LayoutModelID, isNaceOCRModelSelector, isMiniCPMOCRModelSelector = origIDs[4], origIDs[5], origIDs[6], origIDs[7]
	})
	resolveMinerUModelForDispatch = resolver("mineru")
	resolvePaddleOCRModelForDispatch = resolver("paddleocr")
	resolveSoMarkModelForDispatch = resolver("somark")
	resolveMonkeyOCRModelForDispatch = resolver("monkeyocr")
	resolveTenantOCRModelByProvider = func(_ context.Context, _ *gorm.DB, _ string, provider string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		record(strings.ToLower(provider))
		return nil, "", nil, 0, errProbe
	}
	resolveModelConfig = func(context.Context, *gorm.DB, string, entity.ModelType, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return nil, "", nil, 0, errProbe
	}
	isMinerULayoutModelID, isPaddleOCRLayoutModelID, isSoMarkLayoutModelID = uuid("mineru"), uuid("paddleocr"), uuid("somark")
	isUnlimitedOCRLayoutModelID, isMonkeyOCRLayoutModelID, isMonkeyOCRv2LayoutModelID = uuid("unlimited-ocr-2"), uuid("monkeyocr"), uuid("monkeyocrv2")
	isNaceOCRModelSelector = func(_ context.Context, _ *gorm.DB, _, selector string) bool {
		return strings.HasSuffix(strings.ToLower(selector), "@nace")
	}
	isMiniCPMOCRModelSelector = func(context.Context, *gorm.DB, string, string) bool { return false }

	// A parser branch falls back to its service URL from the environment when
	// the resolver fails, so clear those URLs: no probe may send a request.
	for _, name := range []string{
		common.EnvMonkeyOCRv2ServerURL, common.EnvMonkeyOCRServerURL, common.EnvMonkeyOCRAPIServer,
		common.EnvUnlimitedOCRServerURL, common.EnvUnlimitedOCRAPIServer,
	} {
		t.Setenv(name, "")
	}

	// route names the branch that took the selector: the resolver it called
	// first, else the error text of the branch.
	route := func(field, value string) string {
		called = ""
		_, handled, err := dispatchPDFBackend(t.Context(), nil, "sample.pdf", []byte("%PDF-1.4"), "tenant", schema.ParserSetup{field: value})
		text := ""
		if err != nil {
			text = err.Error()
		}
		switch {
		case called != "":
			return called
		case strings.Contains(text, "Unlimited-OCR-2"):
			return "unlimited-ocr-2"
		case strings.Contains(text, "MonkeyOCRv2"):
			return "monkeyocrv2"
		case strings.Contains(text, "Nace model"):
			return "nace"
		case strings.Contains(text, "pdf vision render"):
			return "vlm"
		}
		return fmt.Sprintf("handled=%v err=%v", handled, err)
	}
	type form struct{ field, value, want string }
	var forms []form
	for _, p := range []struct{ route, bare, model, provider, providerAlias string }{
		{"mineru", "mineru", "MinerU-pipeline", "MinerU", "MinerU.Net"},
		{"paddleocr", "paddleocr", "PaddleOCR-VL-1.5", "PaddleOCR", "PaddleOCR.local"},
		{"somark", "somark", "SoMark-v1", "SoMark", "SoMark"},
		{"unlimited-ocr-2", "unlimited-ocr-2", "Unlimited-OCR-2", "Unlimited-OCR-2", "unlimited_ocr"},
		{"monkeyocr", "monkeyocr", "MonkeyOCR-pro", "MonkeyOCR", "MonkeyOCR"},
		{"monkeyocrv2", "monkeyocrv2", "MonkeyOCRv2-3B", "MonkeyOCRv2", "MonkeyOCRv2"},
	} {
		forms = append(forms,
			form{"parse_method", p.bare, p.route},
			form{"parse_method", strings.ToUpper(p.bare), p.route},
			form{"parse_method", "uuid-" + p.route, p.route},
			form{"layout_recognizer", "uuid-" + p.route, p.route},
			form{"layout_recognizer", p.model + "@" + p.provider, p.route},
			form{"layout_recognizer", p.model + "@default@" + p.provider, p.route},
			form{"layout_recognizer", p.model + "@default@" + p.providerAlias, p.route},
			form{"layout_recognizer", strings.ToUpper(p.model + "@default@" + p.provider), p.route},
			form{"layout_recognizer", p.model + "@default@GiteeAI", p.route},
			form{"layout_recognizer", p.model + "@default@OpenAI-API-Compatible", p.route},
			form{"layout_recognizer", "ndi-parse-low@" + p.bare + "-prod@Nace", "nace"},
			form{"parse_method", "ndi-parse-low@" + p.bare + "-prod@Nace", "nace"},
		)
	}
	forms = append(forms,
		form{"parse_method", "unlimited_ocr", "unlimited-ocr-2"},
		form{"layout_recognizer", "unlimited_ocr", "unlimited-ocr-2"},
		form{"layout_recognizer", "other-model@unlimited-ocr-prod@OpenAI", "vlm"},
		form{"layout_recognizer", "other-model@paddleocr-prod@OpenAI", "vlm"},
	)
	for _, f := range forms {
		if got := route(f.field, f.value); got != f.want {
			t.Errorf("%s=%q: route %s, want %s", f.field, f.value, got, f.want)
		}
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
