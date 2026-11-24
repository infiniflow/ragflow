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
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/media"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

func TestMonkeyOCRv2ArchiveUsesNativeJSON(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("sample/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"layouts": []map[string]any{{"label": "Page-header", "content": "ignored"}, {"label": "Title", "content": "Heading"}, {"label": "Formula", "content": "x^2"}}})
	_, _ = file.Write(payload)
	_ = writer.Close()
	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buffer.Bytes(), media.NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.JSON) != 2 || res.JSON[0].ToMap()["layout"] != "title" || res.JSON[0].ToMap()["text"] != "Heading" || res.JSON[1].ToMap()["layout"] != "equation" || res.JSON[1].ToMap()["text"] != "x^2" {
		t.Fatalf("items=%v", res.JSON)
	}
}

func TestMonkeyOCRv2ArchiveEmbedsZIPImage(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	jsonFile, _ := writer.Create("sample/sample.json")
	_, _ = jsonFile.Write([]byte(`{"layouts":[{"label":"Picture","content":"![figure](images/figure.png)"}]}`))
	imageFile, _ := writer.Create("sample/images/figure.png")
	_, _ = imageFile.Write([]byte("png-data"))
	_ = writer.Close()

	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buffer.Bytes(), media.NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.JSON) != 1 || res.JSON[0].ToMap()["text"] != "figure" || res.JSON[0].ToMap()["image"] != "cG5nLWRhdGE=" {
		t.Fatalf("items=%v", res.JSON)
	}
}

func TestMonkeyOCRv2RequestTimeoutUsesSetupAndAPIConfig(t *testing.T) {
	if got := monkeyOCRv2RequestTimeout(schema.ParserSetup{"monkeyocrv2_timeout": 12}, nil); got != 12*time.Second {
		t.Fatalf("setup timeout=%v", got)
	}
	apiKey := `{"MONKEYOCRV2_TIMEOUT":"34"}`
	if got := monkeyOCRv2RequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKey}); got != 34*time.Second {
		t.Fatalf("API config timeout=%v", got)
	}
}

func TestMinerURequestTimeout(t *testing.T) {
	t.Setenv(common.EnvMineruTimeout, "")
	t.Setenv("MINERU_TIMEOUT_SECONDS", "")

	// 1. Default fallback
	if got := mineruRequestTimeout(nil, nil); got != 30*time.Minute {
		t.Fatalf("default timeout=%v, want 30m", got)
	}

	// 2. Setup mineru_timeout (int seconds)
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": 1200}, nil); got != 1200*time.Second {
		t.Fatalf("setup mineru_timeout=%v, want 1200s", got)
	}

	// 3. Setup mineru_timeout_seconds (int and float)
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout_seconds": 3600}, nil); got != 3600*time.Second {
		t.Fatalf("setup mineru_timeout_seconds=%v, want 3600s", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout_seconds": 1800.0}, nil); got != 1800*time.Second {
		t.Fatalf("setup mineru_timeout_seconds float=%v, want 1800s", got)
	}

	// 4. Setup duration strings
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "45m"}, nil); got != 45*time.Minute {
		t.Fatalf("setup mineru_timeout 45m=%v, want 45m", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "2h"}, nil); got != 2*time.Hour {
		t.Fatalf("setup mineru_timeout 2h=%v, want 2h", got)
	}

	// 5. APIConfig JSON
	apiKey := `{"mineru_timeout":"2400"}`
	if got := mineruRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKey}); got != 2400*time.Second {
		t.Fatalf("API config timeout=%v, want 2400s", got)
	}
	apiKeyEnv := `{"MINERU_TIMEOUT":"5000"}`
	if got := mineruRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKeyEnv}); got != 5000*time.Second {
		t.Fatalf("API config env key timeout=%v, want 5000s", got)
	}

	// 6. Environment variable
	t.Setenv(common.EnvMineruTimeout, "7200")
	if got := mineruRequestTimeout(nil, nil); got != 7200*time.Second {
		t.Fatalf("env MINERU_TIMEOUT=%v, want 7200s", got)
	}
	t.Setenv(common.EnvMineruTimeout, "")
	t.Setenv("MINERU_TIMEOUT_SECONDS", "3000")
	if got := mineruRequestTimeout(nil, nil); got != 3000*time.Second {
		t.Fatalf("env MINERU_TIMEOUT_SECONDS=%v, want 3000s", got)
	}

	// 7. Precedence: Setup > APIConfig > Env
	t.Setenv(common.EnvMineruTimeout, "9999")
	apiKeyPrecedence := `{"mineru_timeout":"600"}`
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": 300}, &modelModule.APIConfig{ApiKey: &apiKeyPrecedence}); got != 300*time.Second {
		t.Fatalf("precedence setup over apiConfig=%v, want 300s", got)
	}
	if got := mineruRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKeyPrecedence}); got != 600*time.Second {
		t.Fatalf("precedence apiConfig over env=%v, want 600s", got)
	}

	// 8. Invalid values fallback
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": -10}, nil); got != 30*time.Minute {
		t.Fatalf("negative timeout fallback=%v, want 30m", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "invalid"}, nil); got != 30*time.Minute {
		t.Fatalf("invalid string timeout fallback=%v, want 30m", got)
	}
	// Present setup key is authoritative: invalid values do not fall through to MINERU_TIMEOUT env or apiConfig
	t.Setenv(common.EnvMineruTimeout, "7200")
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "invalid"}, nil); got != 30*time.Minute {
		t.Fatalf("invalid setup with MINERU_TIMEOUT env fallback=%v, want 30m", got)
	}

	// 9. Duration clamping [1s, 24h]
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "500ms"}, nil); got != 1*time.Second {
		t.Fatalf("sub-second timeout clamped=%v, want 1s", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "48h"}, nil); got != 24*time.Hour {
		t.Fatalf("oversized timeout clamped=%v, want 24h", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": 10000000000}, nil); got != 24*time.Hour {
		t.Fatalf("huge int seconds clamped=%v, want 24h", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": "10000000000"}, nil); got != 24*time.Hour {
		t.Fatalf("huge str seconds clamped=%v, want 24h", got)
	}
	if got := mineruRequestTimeout(schema.ParserSetup{"mineru_timeout": 1800, "mineru_timeout_seconds": 60}, nil); got != 1800*time.Second {
		t.Fatalf("mineru_timeout precedence=%v, want 1800s", got)
	}
}

func TestDispatchMonkeyOCRv2PDFPostsNativeParseRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/parse" {
			t.Fatalf("path=%s", request.URL.Path)
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if request.FormValue("start_page_id") != "0" || request.FormValue("end_page_id") != "99999" {
			t.Fatalf("page range missing")
		}
		var output bytes.Buffer
		archive := zip.NewWriter(&output)
		file, _ := archive.Create("sample/sample.json")
		_, _ = file.Write([]byte(`{"layouts":[{"label":"Text","content":"hello"}]}`))
		_ = archive.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(output.Bytes())
	}))
	defer server.Close()
	result, err := dispatchMonkeyOCRv2PDF(context.Background(), nil, "sample.pdf", []byte("pdf"), "", schema.ParserSetup{"monkeyocrv2_server_url": server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Produced() || len(result.JSON) == 0 {
		t.Fatalf("result=%+v", result)
	}

	resultMD, err := dispatchMonkeyOCRv2PDF(context.Background(), nil, "sample.pdf", []byte("pdf"), "", schema.ParserSetup{"monkeyocrv2_server_url": server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !resultMD.Produced() || len(resultMD.JSON) == 0 {
		t.Fatalf("resultMD=%+v", resultMD)
	}
}

type monkeyOCRv2FakeDriver struct {
	modelModule.ModelDriver
}

func (d *monkeyOCRv2FakeDriver) Name() string { return "monkeyocrv2" }

func TestDispatchMonkeyOCRv2PDFUsesSelectedCompositeModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var output bytes.Buffer
		archive := zip.NewWriter(&output)
		file, _ := archive.Create("sample/all_results.json")
		_, _ = file.Write([]byte(`[{"label":"Text","content":"selected"}]`))
		_ = archive.Close()
		_, _ = w.Write(output.Bytes())
	}))
	defer server.Close()

	original := resolveModelConfig
	t.Cleanup(func() { resolveModelConfig = original })
	selected := "MonkeyOCRv2-Parsing@custom@MonkeyOCRv2"
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, tenantID string, _ entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		if tenantID != "tenant-1" || modelRef != selected {
			t.Fatalf("tenant=%q modelRef=%q", tenantID, modelRef)
		}
		return &monkeyOCRv2FakeDriver{}, "MonkeyOCRv2-Parsing", &modelModule.APIConfig{BaseURL: &server.URL}, 0, nil
	}

	result, err := dispatchMonkeyOCRv2PDF(context.Background(), nil, "sample.pdf", []byte("pdf"), "tenant-1", schema.ParserSetup{}, selected)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Produced() || len(result.JSON) == 0 {
		t.Fatalf("result=%+v", result)
	}
}

// paddleOCRFakeDriver embeds the ModelDriver interface and only implements
// the OCRFile method needed by dispatchPaddleOCRPdf.
type paddleOCRFakeDriver struct {
	modelModule.ModelDriver
	pages     []modelModule.OCRPage
	ocrConfig *modelModule.OCRConfig
}

func (d *paddleOCRFakeDriver) Name() string { return "PaddleOCR" }

func (d *paddleOCRFakeDriver) OCRFile(_ context.Context, _ *string, _ []byte, _ *string, _ *modelModule.APIConfig, ocrConfig *modelModule.OCRConfig, _ *common.ModelUsage) (*modelModule.OCRFileResponse, error) {
	d.ocrConfig = ocrConfig
	return &modelModule.OCRFileResponse{Pages: d.pages}, nil
}

func TestPaddleOCRRequestTimeout(t *testing.T) {
	t.Setenv(common.EnvPaddleOCRTimeout, "")

	// 1. Unset: zero keeps the driver default
	if got := paddleOCRRequestTimeout(nil, nil); got != 0 {
		t.Fatalf("default timeout=%v, want 0", got)
	}

	// 2. Setup paddleocr_timeout / paddleocr_timeout_seconds (int and float seconds)
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": 3600}, nil); got != time.Hour {
		t.Fatalf("setup paddleocr_timeout=%v, want 1h", got)
	}
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout_seconds": 2400}, nil); got != 2400*time.Second {
		t.Fatalf("setup paddleocr_timeout_seconds=%v, want 2400s", got)
	}
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout_seconds": 1800.0}, nil); got != 1800*time.Second {
		t.Fatalf("setup paddleocr_timeout_seconds float=%v, want 1800s", got)
	}

	// 3. Setup duration strings
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": "45m"}, nil); got != 45*time.Minute {
		t.Fatalf("setup paddleocr_timeout 45m=%v, want 45m", got)
	}
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": " 2h "}, nil); got != 2*time.Hour {
		t.Fatalf("setup paddleocr_timeout 2h=%v, want 2h", got)
	}

	// 4. APIConfig JSON
	apiKey := `{"paddleocr_timeout":"2400"}`
	if got := paddleOCRRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKey}); got != 2400*time.Second {
		t.Fatalf("API config timeout=%v, want 2400s", got)
	}
	apiKeyEnv := `{"api_key":{"PADDLEOCR_TIMEOUT":"90m"}}`
	if got := paddleOCRRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKeyEnv}); got != 90*time.Minute {
		t.Fatalf("API config env key timeout=%v, want 90m", got)
	}
	plainToken := "plain-token"
	if got := paddleOCRRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &plainToken}); got != 0 {
		t.Fatalf("plain api_key timeout=%v, want 0", got)
	}

	// 5. Environment variable
	t.Setenv(common.EnvPaddleOCRTimeout, "7200")
	if got := paddleOCRRequestTimeout(nil, nil); got != 7200*time.Second {
		t.Fatalf("env PADDLEOCR_TIMEOUT=%v, want 7200s", got)
	}
	t.Setenv(common.EnvPaddleOCRTimeout, "1h30m")
	if got := paddleOCRRequestTimeout(nil, nil); got != 90*time.Minute {
		t.Fatalf("env PADDLEOCR_TIMEOUT duration=%v, want 1h30m", got)
	}

	// 6. Precedence: Setup > APIConfig > Env
	t.Setenv(common.EnvPaddleOCRTimeout, "9999")
	apiKeyPrecedence := `{"paddleocr_timeout":"600"}`
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": 300}, &modelModule.APIConfig{ApiKey: &apiKeyPrecedence}); got != 300*time.Second {
		t.Fatalf("precedence setup over apiConfig=%v, want 300s", got)
	}
	if got := paddleOCRRequestTimeout(nil, &modelModule.APIConfig{ApiKey: &apiKeyPrecedence}); got != 600*time.Second {
		t.Fatalf("precedence apiConfig over env=%v, want 600s", got)
	}
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": 1800, "paddleocr_timeout_seconds": 60}, nil); got != 1800*time.Second {
		t.Fatalf("paddleocr_timeout precedence=%v, want 1800s", got)
	}

	// 7. Invalid values keep the driver default. A present setup key is authoritative:
	// it does not fall through to apiConfig or the PADDLEOCR_TIMEOUT env.
	for _, invalid := range []any{-10, 0, "invalid"} {
		if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": invalid}, &modelModule.APIConfig{ApiKey: &apiKeyPrecedence}); got != 0 {
			t.Fatalf("invalid setup %v timeout=%v, want 0", invalid, got)
		}
	}
	t.Setenv(common.EnvPaddleOCRTimeout, "invalid")
	if got := paddleOCRRequestTimeout(nil, nil); got != 0 {
		t.Fatalf("invalid env timeout=%v, want 0", got)
	}

	// 8. Duration clamping [1s, 24h]
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": "500ms"}, nil); got != time.Second {
		t.Fatalf("sub-second timeout clamped=%v, want 1s", got)
	}
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": "48h"}, nil); got != 24*time.Hour {
		t.Fatalf("oversized timeout clamped=%v, want 24h", got)
	}
	if got := paddleOCRRequestTimeout(schema.ParserSetup{"paddleocr_timeout": 10000000000}, nil); got != 24*time.Hour {
		t.Fatalf("huge int seconds clamped=%v, want 24h", got)
	}
	t.Setenv(common.EnvPaddleOCRTimeout, "10000000000")
	if got := paddleOCRRequestTimeout(nil, nil); got != 24*time.Hour {
		t.Fatalf("huge env seconds clamped=%v, want 24h", got)
	}
}

func TestDispatchPaddleOCRPDFPassesResolvedTimeout(t *testing.T) {
	t.Setenv(common.EnvPaddleOCRTimeout, "")
	driver := &paddleOCRFakeDriver{pages: []modelModule.OCRPage{{Index: 0, Blocks: []modelModule.OCRBlock{{Text: "Body"}}}}}
	original := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = original })
	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return driver, "ocr-model", &modelModule.APIConfig{}, nil
	}

	if _, err := dispatchPaddleOCRPdf(t.Context(), nil, "sample.pdf", nil, "tenant", nil, "model"); err != nil {
		t.Fatal(err)
	}
	if driver.ocrConfig == nil || driver.ocrConfig.Timeout != 0 {
		t.Fatalf("unconfigured OCR config=%+v, want zero timeout", driver.ocrConfig)
	}

	t.Setenv(common.EnvPaddleOCRTimeout, "1h")
	if _, err := dispatchPaddleOCRPdf(t.Context(), nil, "sample.pdf", nil, "tenant", nil, "model"); err != nil {
		t.Fatal(err)
	}
	if driver.ocrConfig == nil || driver.ocrConfig.Timeout != time.Hour {
		t.Fatalf("env OCR config=%+v, want 1h timeout", driver.ocrConfig)
	}

	if _, err := dispatchPaddleOCRPdf(t.Context(), nil, "sample.pdf", nil, "tenant", schema.ParserSetup{"paddleocr_timeout": "45m"}, "model"); err != nil {
		t.Fatal(err)
	}
	if driver.ocrConfig == nil || driver.ocrConfig.Timeout != 45*time.Minute {
		t.Fatalf("setup OCR config=%+v, want 45m timeout", driver.ocrConfig)
	}
}

func TestDispatchPaddleOCRPDFUsesStructuredOutput(t *testing.T) {
	original := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = original })
	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return &paddleOCRFakeDriver{pages: []modelModule.OCRPage{{Index: 0, Blocks: []modelModule.OCRBlock{{Text: "# Heading\n\nBody"}}}}}, "ocr-model", &modelModule.APIConfig{}, nil
	}
	result, err := dispatchPaddleOCRPdf(t.Context(), nil, "sample.pdf", nil, "tenant", schema.ParserSetup{}, "model")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Produced() || len(result.JSON) == 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestDispatchPaddleOCRPDFRejectsMissingBlocks(t *testing.T) {
	orig := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = orig })

	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		baseURL := "http://localhost:9380"
		return &paddleOCRFakeDriver{}, "ocr-model", &modelModule.APIConfig{BaseURL: &baseURL}, nil
	}

	res, err := dispatchPaddleOCRPdf(t.Context(), dao.DB, "test.pdf", []byte("%PDF-1.4"), "tenant", nil, "some-uuid")
	if err == nil {
		t.Fatalf("dispatchPaddleOCRPdf without page blocks: expected error, got result %+v", res)
	}
	if res.Produced() {
		t.Errorf("expected zero-value result on error, got %+v", res)
	}
}

func TestMinerUContentItemToJSONExplicitLayoutMapping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		item        map[string]any
		wantLayout  string
		wantDocType string
	}{
		{"list", map[string]any{"type": "list", "list_items": []any{"a", "b"}}, "list", "text"},
		{"code", map[string]any{"type": "code", "code_body": "x"}, "code", "text"},
		{"title", map[string]any{"type": "title", "text": "T"}, "title", "text"},
		{"equation", map[string]any{"type": "equation", "text": "x=1"}, "equation", "text"},
		{"unknown", map[string]any{"type": "footnote", "text": "x"}, "text", "text"},
		{"image", map[string]any{"type": "image", "image_caption": []any{"cap"}}, "figure", "image"},
		{"table", map[string]any{"type": "table", "table_body": "<table></table>"}, "table", "table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mineruContentItemToJSON(tc.item, nil, nil)
			if got == nil {
				t.Fatal("item was dropped")
			}
			if got["layout"] != tc.wantLayout {
				t.Errorf("layout = %v, want %q", got["layout"], tc.wantLayout)
			}
			if got["doc_type_kwd"] != tc.wantDocType {
				t.Errorf("doc_type_kwd = %v, want %q", got["doc_type_kwd"], tc.wantDocType)
			}
		})
	}
}

func TestMinerUExtractJSONItemsKeepsImageAndPositions(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[
		{"type":"text","text":"Hello","page_idx":0,"bbox":[10,20,110,40]},
		{"type":"image","img_path":"images/figure.png","image_caption":["A figure"],"page_idx":1,"bbox":[1,2,3,4]},
		{"type":"header","text":"skip me","page_idx":0,"bbox":[0,0,1,1]}
	]`))
	img, err := zw.Create("images/figure.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = img.Write([]byte("png-data"))
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}

	sizes := map[int][2]float64{0: {612, 792}, 1: {612, 792}}
	items, _, err := mineruExtractJSONItems(buf.Bytes(), sizes)
	if err != nil {
		t.Fatalf("mineruExtractJSONItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d, want 2 (header dropped)", len(items))
	}
	if items[0]["text"] != "Hello" || items[0]["doc_type_kwd"] != "text" {
		t.Fatalf("text item = %#v", items[0])
	}
	pos, ok := items[0]["_pdf_positions"].([][]any)
	if !ok || len(pos) != 1 {
		t.Fatalf("text positions = %#v", items[0]["_pdf_positions"])
	}
	if page, _ := pos[0][0].(int); page != 1 {
		t.Fatalf("text page = %#v, want 1-based page 1", pos[0])
	}
	want := []float64{6.12, 67.32, 15.84, 31.68}
	for i, w := range want {
		if got, _ := pos[0][i+1].(float64); math.Abs(got-w) > 1e-6 {
			t.Fatalf("bbox positions = %#v, want %v", pos[0], want)
		}
	}

	imageItem := items[1]
	if imageItem["doc_type_kwd"] != "image" {
		t.Fatalf("image doc_type = %#v", imageItem["doc_type_kwd"])
	}
	if imageItem["text"] != "A figure" {
		t.Fatalf("image text = %#v", imageItem["text"])
	}
	if _, ok := imageItem["image"]; ok {
		t.Fatalf("zip figures must not be inlined on the MinerU JSON path, got image=%q", imageItem["image"])
	}
	imgPos, _ := imageItem["_pdf_positions"].([][]any)
	if len(imgPos) != 1 {
		t.Fatalf("image positions = %#v", imageItem["_pdf_positions"])
	}
	if page, _ := imgPos[0][0].(int); page != 2 {
		t.Fatalf("image page = %#v, want 1-based page 2", imgPos[0])
	}
}

func TestMinerUBBoxPositionScalesAndClamps(t *testing.T) {
	sizes := map[int][2]float64{0: {600, 800}}
	for _, tc := range []struct {
		name    string
		page    any
		bbox    any
		want    []float64
		wantNil bool
	}{
		{
			name: "normalized to points",
			page: 0,
			bbox: []any{100.0, 200.0, 300.0, 400.0},
			want: []float64{1, 60, 180, 160, 320},
		},
		{
			name: "inverted edges are ordered",
			page: 0,
			bbox: []any{300.0, 400.0, 100.0, 200.0},
			want: []float64{1, 60, 180, 160, 320},
		},
		{
			name: "edges clamped to the page box",
			page: 0,
			bbox: []any{-50.0, -20.0, 1100.0, 1200.0},
			want: []float64{1, 0, 600, 0, 800},
		},
		{name: "unknown page size", page: 3, bbox: []any{1.0, 2.0, 3.0, 4.0}, wantNil: true},
		{name: "negative page index", page: -1, bbox: []any{1.0, 2.0, 3.0, 4.0}, wantNil: true},
		{name: "short bbox", page: 0, bbox: []any{1.0, 2.0}, wantNil: true},
		{name: "zero width", page: 0, bbox: []any{10.0, 20.0, 10.0, 40.0}, wantNil: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mineruBBoxPosition(tc.page, tc.bbox, sizes)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("position = %#v, want nil", got)
				}
				return
			}
			if len(got) != 5 {
				t.Fatalf("position = %#v, want 5 values", got)
			}
			if page, ok := got[0].(int); !ok || page != int(tc.want[0]) {
				t.Fatalf("page = %#v, want %v", got[0], tc.want[0])
			}
			for i, w := range tc.want[1:] {
				if v, _ := got[i+1].(float64); math.Abs(v-w) > 1e-6 {
					t.Fatalf("position = %#v, want %v", got, tc.want)
				}
			}
		})
	}
}

// MinerU normalizes content_list bbox to 0..1000 against the page's point
// size (pipeline _build_bbox / vlm mkcontent). With middle.json in the zip the
// boxes must come back as PDF points: bbox/1000 * page_size.
func TestMinerUExtractJSONItemsScalesBBoxToPoints(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("output/doc/content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[
		{"type":"text","text":"Scaled","page_idx":0,"bbox":[100,200,600,400]}
	]`))
	mid, err := zw.Create("output/doc/middle.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = mid.Write([]byte(`{"pdf_info":[
		{"page_idx":0,"page_size":[500,1000],"preproc_blocks":[]},
		{"page_idx":1,"page_size":[400,800],"preproc_blocks":[]}
	]}`))
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}

	items, _, err := mineruExtractJSONItems(buf.Bytes(), nil)
	if err != nil {
		t.Fatalf("mineruExtractJSONItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d, want 1", len(items))
	}
	pos, ok := items[0]["_pdf_positions"].([][]any)
	if !ok || len(pos) != 1 {
		t.Fatalf("positions = %#v", items[0]["_pdf_positions"])
	}
	row := pos[0]
	if page, _ := row[0].(int); page != 1 {
		t.Fatalf("page = %#v, want 1", row[0])
	}
	want := []float64{50, 300, 200, 400}
	for i, key := range []int{1, 2, 3, 4} {
		v, _ := row[key].(float64)
		if v != want[i] {
			t.Fatalf("position[%d] = %v, want %v (row=%v)", key, row[key], want[i], row)
		}
	}
}

func TestMinerUExtractJSONItemsWithoutGeometryKeepsText(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[{"type":"text","text":"Hello","page_idx":1,"bbox":[10,20,110,40]}]`))
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}

	items, _, err := mineruExtractJSONItems(buf.Bytes(), nil)
	if err != nil {
		t.Fatalf("mineruExtractJSONItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d, want 1", len(items))
	}
	if items[0]["text"] != "Hello" {
		t.Errorf("text = %#v", items[0]["text"])
	}
	if items[0]["page_number"] != 2 {
		t.Errorf("page_number = %#v, want 2", items[0]["page_number"])
	}
	if _, ok := items[0]["_pdf_positions"]; ok {
		t.Errorf("_pdf_positions = %#v, want none", items[0]["_pdf_positions"])
	}
}

// MinerU V1 accepts partial jobs and preserves the source page indices in
// its artifacts, so middle.json pdf_info can be sparse (page_idx 5 and 6
// only). Sizes must be keyed by page_idx, not entry order.
func TestMinerUPageSizesKeysSparsePageIdx(t *testing.T) {
	sizes := mineruPageSizesFromMiddleJSON([]byte(`{"pdf_info":[
		{"page_idx":5,"page_size":[400,800]},
		{"page_idx":6,"page_size":[600,900]}
	]}`))
	if len(sizes) != 2 {
		t.Fatalf("len(sizes)=%d, want 2", len(sizes))
	}
	pos := mineruBBoxPosition(6, []any{500, 500, 1000, 1000}, sizes)
	if pos == nil {
		t.Fatal("expected a position, got nil")
	}
	want := []float64{300, 600, 450, 900}
	for i, key := range []int{1, 2, 3, 4} {
		v, _ := pos[key].(float64)
		if v != want[i] {
			t.Fatalf("position[%d] = %v, want %v (row=%v)", key, pos[key], want[i], pos)
		}
	}
	if mineruBBoxPosition(4, []any{100, 100, 200, 200}, sizes) != nil {
		t.Fatal("expected nil position for page missing from sparse sizes")
	}
}

// Entries with no page_idx at all fall back to their position in pdf_info.
func TestMinerUPageSizesFallsBackToEntryOrder(t *testing.T) {
	sizes := mineruPageSizesFromMiddleJSON([]byte(`{"pdf_info":[
		{"page_size":[500,1000]},
		{"page_size":[400,800]}
	]}`))
	if got, ok := sizes[1]; !ok || got != [2]float64{400, 800} {
		t.Fatalf("sizes[1] = %#v, want [400 800]", sizes)
	}
}

func TestMinerUBBoxPositionWithoutPageSizesReturnsNil(t *testing.T) {
	if mineruBBoxPosition(2, []any{188, 142, 875, 393}, nil) != nil {
		t.Fatal("expected nil position without page sizes")
	}
}

func TestMinerURealContentListPositionsStayInPage(t *testing.T) {
	list, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "mineru", "bmw_page_chrome_content_list.json"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(list); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}

	const (
		pageWidth  = 595.0
		pageHeight = 842.0
	)
	items, _, err := mineruExtractJSONItems(buf.Bytes(), map[int][2]float64{0: {pageWidth, pageHeight}})
	if err != nil {
		t.Fatalf("mineruExtractJSONItems: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no items converted")
	}
	positioned := 0
	for _, item := range items {
		raw, ok := item["_pdf_positions"]
		if !ok {
			continue
		}
		positioned++
		rows, ok := raw.([][]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("_pdf_positions = %#v, want one row", raw)
		}
		row := rows[0]
		left, right := row[1].(float64), row[2].(float64)
		top, bottom := row[3].(float64), row[4].(float64)
		if left < 0 || right > pageWidth || top < 0 || bottom > pageHeight || right <= left || bottom <= top {
			t.Fatalf("position %v outside the %.0fx%.0f page", row, pageWidth, pageHeight)
		}
	}
	if positioned == 0 {
		t.Fatal("fixture produced no positions")
	}
}

func TestAppendMineruGeometryWarningOnlyOnError(t *testing.T) {
	res := parser.ParseResult{}
	appendMineruGeometryWarning(&res, nil)
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none on success", res.Warnings)
	}
	appendMineruGeometryWarning(&res, errors.New("no pdfium"))
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "MinerU PDF geometry unavailable: no pdfium") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestDispatchMinerUPDFRequiresBaseURL(t *testing.T) {
	t.Setenv("MINERU_APISERVER", "")
	orig := resolveMinerUModelForDispatch
	t.Cleanup(func() { resolveMinerUModelForDispatch = orig })

	empty := ""
	resolveMinerUModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return &mineruTestDriver{}, "mineru-model", &modelModule.APIConfig{BaseURL: &empty}, nil
	}

	_, err := dispatchMinerUPDF(t.Context(), dao.DB, "sample.pdf", []byte("%PDF-1.4"), "tenant", nil, "")
	if err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("dispatchMinerUPDF empty URL error = %v", err)
	}
}

func TestDispatchPaddleOCRPDFPreservesLayout(t *testing.T) {
	withSSRFBypass(t)
	body, err := os.ReadFile("../../entity/models/testdata/paddleocr_layout.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../parser/parser/testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()
	original := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = original })
	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return modelModule.NewPaddleOCRLocalModel(nil, modelModule.URLSuffix{OCR: "layout-parsing"}), "PaddleOCR-VL-1.6", &modelModule.APIConfig{BaseURL: &server.URL}, nil
	}
	result, err := dispatchPaddleOCRPdf(t.Context(), nil, "sample.pdf", data, "tenant", nil, "model-id")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Produced() || len(result.JSON) != 7 {
		t.Fatalf("result=%+v", result)
	}
	if result.JSON[0].ToMap()["positions"] == nil || result.JSON[2].ToMap()["table"] == nil {
		t.Fatalf("items=%v", result.JSON)
	}
}

func TestDispatchPaddleOCRPDFAcceptsLocatedBlocksWithoutText(t *testing.T) {
	original := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = original })
	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return &paddleOCRFakeDriver{pages: []modelModule.OCRPage{{Index: 0, Blocks: []modelModule.OCRBlock{{Text: "body", Label: "text"}}}}}, "ocr-model", &modelModule.APIConfig{}, nil
	}
	result, err := dispatchPaddleOCRPdf(t.Context(), nil, "sample.pdf", nil, "tenant", nil, "model")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.JSON) != 1 {
		t.Fatalf("items=%v", result.JSON)
	}
}

func TestMineruExtractJSONItemsStructuredContent(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entry, err := zw.Create("structured_content.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte(`{"pages":[{"page_idx":0,"blocks":[
		{"type":"paragraph_title","content":"章节","bbox":[0.1,0.2,0.4,0.3]},
		{"type":"header","content":"HEADER"},
		{"type":"text","content":"正文"},
		{"type":"table","content":"单元格","captions":[{"content":"表1"}]},
		{"type":"image","content":"示意图","image_source":"images/a.png","captions":[{"content":"图1"}]}
	]}]}`))
	img, err := zw.Create("images/a.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = img.Write([]byte{0x89, 'P', 'N', 'G'})
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	items, _, err := mineruExtractJSONItems(buf.Bytes(), map[int][2]float64{0: {1000, 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4 (header skipped)", len(items))
	}
	if items[0]["text"] != "章节" || items[0]["layout"] != "title" {
		t.Fatalf("title item = %#v", items[0])
	}
	positions, _ := items[0]["_pdf_positions"].([][]any)
	if len(positions) != 1 || positions[0][0] != 1 || positions[0][1] != float64(100) {
		t.Fatalf("positions = %#v", items[0]["_pdf_positions"])
	}
	if items[2]["doc_type_kwd"] != "table" || !strings.Contains(items[2]["text"].(string), "表1") {
		t.Fatalf("table item = %#v", items[2])
	}
	if items[3]["doc_type_kwd"] != "image" || !strings.Contains(items[3]["text"].(string), "图1") {
		t.Fatalf("image item = %#v", items[3])
	}
	if _, ok := items[3]["image"]; ok {
		t.Fatalf("structured zip image must not be inlined: %#v", items[3])
	}
}

func TestFinishMinerURemoteResultPrefersStructuredZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entry, err := zw.Create("structured_content.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte(`{"pages":[{"page_idx":0,"blocks":[
		{"type":"paragraph_title","content":"章节","bbox":[0.1,0.2,0.4,0.3]},
		{"type":"header","content":"HEADER"}
	]}]}`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	res, err := finishMinerURemoteResult(t.Context(), "sample.pdf", []byte("%PDF-1.4"), &modelModule.MinerUV1Result{
		Markdown: "# markdown only",
		Zip:      buf.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, item := range parser.ItemsToMaps(res.JSON) {
		value, _ := item["text"].(string)
		text.WriteString(value)
		text.WriteString("\n")
	}
	got := text.String()
	if !strings.Contains(got, "章节") || strings.Contains(got, "HEADER") || strings.Contains(got, "markdown only") {
		t.Fatalf("text = %q, want structured title without the header or markdown fallback", got)
	}
}

// A cancelled ingestion run must abort the MinerU 3.x /file_parse upload
// instead of holding the worker and the remote server for the whole stream
// budget.
func TestDispatchPDFMinerUStreamHonoursCancellation(t *testing.T) {
	withSSRFBypass(t)
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file_parse" {
			http.NotFound(w, r)
			return
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	apiKey := "mineru-secret"
	baseURL := server.URL
	original := resolveMinerUModelForDispatch
	t.Cleanup(func() { resolveMinerUModelForDispatch = original })
	resolveMinerUModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		return &mineruTestDriver{}, "vlm", &modelModule.APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, nil
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := dispatchMinerUPDF(ctx, nil, "scan.pdf", []byte("%PDF-1.4"), "tenant", nil, "")
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("MinerU /file_parse was never reached")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("dispatch error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch kept the /file_parse request open after cancellation")
	}
}

// readSignalBody closes signal once the client has read body bytes, which
// tells the test the response header was consumed and ReadAll is in progress.
type readSignalBody struct {
	io.ReadCloser
	once   sync.Once
	signal chan struct{}
}

func (b *readSignalBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.once.Do(func() { close(b.signal) })
	}
	return n, err
}

type readSignalTransport struct {
	base   http.RoundTripper
	signal chan struct{}
}

func (t readSignalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &readSignalBody{ReadCloser: resp.Body, signal: t.signal}
	return resp, nil
}

// A run cancelled while a non-200 error body is still streaming must still
// report context.Canceled; reporting the bare HTTP status hides the reason the
// request ended.
func TestMineruStreamKeepsCancellationWhileReadingErrorResponse(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":"upstream`))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	bodyRead := make(chan struct{})
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: readSignalTransport{base: http.DefaultTransport, signal: bodyRead}}
	t.Cleanup(func() { http.DefaultClient = originalClient })

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := mineruStreamParse(ctx, server.URL+"/file_parse", "mineru-secret", []byte("%PDF-1.4"), "auto", "en", "", "", mineruExtractionOptions{}, 0)
		done <- err
	}()

	select {
	case <-bodyRead:
	case <-time.After(5 * time.Second):
		t.Fatal("MinerU never started reading the error response body")
	}
	cancel()
	select {
	case err := <-done:
		if !strings.Contains(err.Error(), "HTTP 502") {
			t.Fatalf("stream error = %v, want the HTTP 502 error response path", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stream error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream kept reading the error response after cancellation")
	}
}
func TestMonkeyOCRv2ArchiveBudgetOmitsPayloadKeepsAltText(t *testing.T) {
	oversized := make([]byte, media.ImageMaxBytes()+1)
	for i := range oversized {
		oversized[i] = 'b'
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	jsonFile, _ := writer.Create("sample/sample.json")
	_, _ = jsonFile.Write([]byte(`{"layouts":[{"label":"Picture","content":"![big figure](images/huge.png)","page_num":2,"bbox":[1,2,3,4]}]}`))
	imageFile, _ := writer.Create("sample/images/huge.png")
	_, _ = imageFile.Write(oversized)
	_ = writer.Close()

	budget := media.NewBudget()
	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buffer.Bytes(), budget)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.JSON) != 1 || res.JSON[0].ToMap()["media_omitted"] != true || res.JSON[0].ToMap()["text"] != "big figure" {
		t.Fatalf("oversized image must stay a position carrier keeping its alt text: items=%v", res.JSON)
	}
	if !reflect.DeepEqual(res.JSON[0].ToMap()["_pdf_positions"], [][]any{{2, 1.0, 3.0, 2.0, 4.0}}) {
		t.Fatalf("omitted image must keep its layout position: %#v", res.JSON[0].ToMap())
	}
	warnings := budget.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "per-image limit") {
		t.Fatalf("warnings = %#v, want one per-image-limit warning", warnings)
	}
}

func TestMonkeyOCRv2ArchiveMissingImageKeepsPositionCarrier(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	jsonFile, _ := writer.Create("sample/sample.json")
	_, _ = jsonFile.Write([]byte(`{"layouts":[
		{"label":"Picture","content":"![](images/gone.png)","page_num":2,"bbox":[1,2,3,4]},
		{"label":"Picture","content":"![](images/also-gone.png)"}
	]}`))
	_ = writer.Close()

	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buffer.Bytes(), media.NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.JSON) != 1 || res.JSON[0].ToMap()["media_omitted"] != true {
		t.Fatalf("missing image with a position must keep a carrier, without one must drop: items=%v", res.JSON)
	}
	if !reflect.DeepEqual(res.JSON[0].ToMap()["_pdf_positions"], [][]any{{2, 1.0, 3.0, 2.0, 4.0}}) {
		t.Fatalf("carrier position = %#v, want page 2", res.JSON[0].ToMap())
	}
}

func TestMonkeyOCRv2LayoutPositionShape(t *testing.T) {
	_, got := monkeyOCRv2LayoutPosition(map[string]any{"page_num": 2, "bbox": []any{110.0, 40.0, 10.0, 20.0}})
	if len(got) != 5 || got[0] != 2 || got[1] != 10.0 || got[2] != 110.0 || got[3] != 20.0 || got[4] != 40.0 {
		t.Fatalf("position = %#v, want [2 10 110 20 40] with normalized corners", got)
	}
	_, def := monkeyOCRv2LayoutPosition(map[string]any{"bbox": []any{50.0, 60.0, 5.0, 10.0}})
	if len(def) != 5 || def[0] != 1 || def[1] != 5.0 || def[2] != 50.0 || def[3] != 10.0 || def[4] != 60.0 {
		t.Fatalf("default-page position = %#v", def)
	}
	if page, none := monkeyOCRv2LayoutPosition(map[string]any{"page_num": 1}); page != 1 || none != nil {
		t.Fatalf("missing bbox must yield nil, got %#v", none)
	}
}

func TestDispatchMonkeyOCRv2PDFEmitsPositionsAndPageCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var output bytes.Buffer
		archive := zip.NewWriter(&output)
		file, _ := archive.Create("sample/sample.json")
		_, _ = file.Write([]byte(`{"layouts":[
			{"label":"Title","content":"Heading","page_num":2,"bbox":[10,20,110,40]},
			{"label":"Text","content":"Body.","page_num":3,"bbox":[50,60,300,80]}
		]}`))
		_ = archive.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(output.Bytes())
	})))
	defer server.Close()
	res, err := dispatchMonkeyOCRv2PDF(context.Background(), nil, "sample.pdf", []byte("pdf"), "", schema.ParserSetup{"monkeyocrv2_server_url": server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := res.File["page_count"]; got != 3 {
		t.Fatalf("page_count = %v, want max layout page 3", got)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("items = %v", res.JSON)
	}
	first := res.JSON[0].ToMap()
	rows, ok := first["_pdf_positions"].([][]any)
	if !ok || len(rows) != 1 || rows[0][0] != 2 {
		t.Fatalf("heading positions = %#v, want page 2", first)
	}
	if first["layout"] != "title" {
		t.Fatalf("heading layout = %v, want title from the layout label", first["layout"])
	}
}

func TestMonkeyOCRv2EmbeddedImageDataURI(t *testing.T) {
	// A service that already inlines the payload: no ZIP entry can ever
	// match, so the payload must be budget-checked and kept, not lost.
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	encoded := monkeyOCRv2EmbeddedImage("data:image/png;base64,"+png, map[string]*zip.File{}, media.NewBudget())
	if encoded != png {
		t.Fatalf("inline data URI must be kept: encoded=%q", encoded)
	}
	// Undecodable payload: position-only carrier with the alt text.
	encoded = monkeyOCRv2EmbeddedImage("data:image/png;base64,%%%", map[string]*zip.File{}, media.NewBudget())
	if encoded != "" {
		t.Fatalf("undecodable data URI must fall back to position-only: encoded=%q", encoded)
	}
}

func TestMonkeyOCRv2ImageOnlyLastPageCountsAndFileContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		archive := zip.NewWriter(&buf)
		file, _ := archive.Create("sample/sample.json")
		_ = json.NewEncoder(file).Encode(map[string]any{"layouts": []any{
			map[string]any{"label": "Text", "content": "Body.", "page_num": 1, "bbox": []any{1.0, 2.0, 3.0, 4.0}},
			// Last page holds only an image missing from the ZIP: the carrier
			// is position-only, but its page must still count.
			map[string]any{"label": "Picture", "content": "![fig](images/missing.png)", "page_num": 3, "bbox": []any{5.0, 6.0, 7.0, 8.0}},
		}})
		_ = archive.Close()
		_, _ = w.Write(buf.Bytes())
	}))
	defer server.Close()
	previousResolver := resolveTenantOCRModelByProvider
	t.Cleanup(func() { resolveTenantOCRModelByProvider = previousResolver })
	resolveTenantOCRModelByProvider = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &monkeyOCRv2FakeDriver{}, "monkey", &modelModule.APIConfig{}, 0, nil
	}
	setup := schema.ParserSetup{"parse_method": "monkeyocrv2", "monkeyocrv2_server_url": server.URL}
	result, handled, err := maybeDispatchPDFVision(t.Context(), nil, utility.FileTypePDF, "sample.pdf", []byte("pdf"), nil, map[string]schema.ParserSetup{"pdf": setup})
	if err != nil || !handled {
		t.Fatalf("dispatch: %v", err)
	}
	if got := result.File["page_count"]; got != 3 {
		t.Fatalf("page_count = %v, want 3 (image-only last page must count)", got)
	}
	if got := result.File["parse_method"]; got != "monkeyocrv2" {
		t.Fatalf("parse_method = %v, want monkeyocrv2 (sibling MinerU contract)", got)
	}
	if _, ok := result.File["outline"]; !ok {
		t.Fatalf("outline missing from File: %#v", result.File)
	}
}

func TestMonkeyOCRv2EmbeddedImageOversizedDataURIWarns(t *testing.T) {
	// An oversized inline data: payload must be charged to the shared
	// budget like the ZIP path: omitted with a per-image warning, keeping
	// the alt text as the position carrier instead of dropping both.
	oversized := base64.StdEncoding.EncodeToString(make([]byte, media.ImageMaxBytes()+1))
	budget := media.NewBudget()
	encoded := monkeyOCRv2EmbeddedImage("data:image/png;base64,"+oversized, map[string]*zip.File{}, budget)
	if encoded != "" {
		t.Fatalf("oversized inline data URI: encoded=%q", encoded)
	}
	warnings := budget.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "per-image limit") {
		t.Fatalf("warnings = %#v, want one per-image-limit warning", warnings)
	}
}

func TestDispatchMonkeyOCRv2PreservesLayoutBoundaries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archive := zip.NewWriter(w)
		layout, _ := archive.Create("sample/sample.json")
		_, _ = io.WriteString(layout, `{"layouts":[
			{"label":"Text","content":"Repeated","page_num":1},
			{"label":"Title","content":"Repeated","page_num":2,"bbox":[11,12,40,50]},
			{"label":"Code","content":"first line\n\nsecond line","page_num":2,"bbox":[10,55,50,65]},
			{"label":"Picture","content":"![](images/missing.png)","page_num":3,"bbox":[15,20,45,60]}
		]}`)
		archive.Close()
	}))
	defer server.Close()

	result, err := dispatchMonkeyOCRv2PDF(t.Context(), nil, "sample.pdf", nil, "", schema.ParserSetup{"monkeyocrv2_server_url": server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.JSON) != 4 || result.File["page_count"] != 3 {
		t.Fatalf("want four source layouts on three pages, got %+v", result)
	}
	for i, want := range []string{"text", "title", "code", "figure"} {
		if got := result.JSON[i].ToMap()["layout"]; got != want {
			t.Fatalf("layout %d = %v, want %s", i, got, want)
		}
	}
	first, second := result.JSON[0].ToMap(), result.JSON[1].ToMap()
	if first["page_number"] != 1 || first["positions"] != nil || first["text"] != "Repeated" {
		t.Fatalf("first occurrence acquired another occurrence's box: %#v", first)
	}
	wantBox := [][]any{{2, 11.0, 40.0, 12.0, 50.0}}
	if second["text"] != "Repeated" || !reflect.DeepEqual(second["_pdf_positions"], wantBox) || !reflect.DeepEqual(second["positions"], wantBox) {
		t.Fatalf("second occurrence lost its own box: %#v", second)
	}
	if got := result.JSON[2].ToMap()["text"]; got != "first line\n\nsecond line" {
		t.Fatalf("code block was split or rewritten: %q", got)
	}
	last := result.JSON[3].ToMap()
	if last["page_number"] != 3 || last["media_omitted"] != true || last["text"] != "" {
		t.Fatalf("missing source image lost its place: %#v", last)
	}
}

func TestMonkeyOCRv2ArchiveTablesAndImagesKeepTheirSourceOrder(t *testing.T) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	layouts, _ := archive.Create("sample/sample.json")
	_, _ = io.WriteString(layouts, `{"layouts":[
		{"label":"Table","content":"| A |\n|---|\n| 1 |"},
		{"label":"Picture","content":"![a](data:image/png;base64,YQ==)"},
		{"label":"Table","table_body":"<table><tr><th>B</th></tr><tr><td>2</td></tr></table>","page_num":2,"bbox":[2,3,4,5]},
		{"label":"Picture","content":"![b](data:image/png;base64,Yg==)","page_num":3,"bbox":[5,6,7,8]},
		{"label":"Section-header","content":"Heading"},
		{"label":"List-item","content":"List item"}
	]}`)
	md, _ := archive.Create("sample/sample.md")
	_, _ = io.WriteString(md, "stale Markdown")
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buf.Bytes(), media.NewBudget())
	if err != nil || len(res.JSON) != 6 || len(res.Warnings) != 0 {
		t.Fatalf("unexpected archive result: %+v, %v", res, err)
	}
	for i, want := range []string{"table", "figure", "table", "figure", "title", "list"} {
		fields := res.JSON[i].ToMap()
		if fields["layout"] != want || fields["_preserve_order"] != true {
			t.Fatalf("item %d = %#v, want %s in source order", i, fields, want)
		}
		if i < 2 && fields["_pdf_positions"] != nil {
			t.Fatalf("item %d acquired a later item's position: %#v", i, fields)
		}
	}
	for _, tc := range []struct {
		index, page int
		cell        string
	}{{0, 0, "1"}, {2, 2, "2"}} {
		td, ok := res.JSON[tc.index].ToMap()["table"].(*entity.TableData)
		if !ok || td.Page != tc.page || len(td.Grid) != 2 || len(td.Grid[1]) != 1 || td.Grid[1][0].Text != tc.cell {
			t.Fatalf("table %d = %#v", tc.index, td)
		}
	}
	picture := res.JSON[3].ToMap()
	if picture["page_number"] != 3 || picture["image"] != "Yg==" {
		t.Fatalf("positioned image = %#v", picture)
	}
}

func TestMonkeyOCRv2ArchiveMarkdownFallback(t *testing.T) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	for name, body := range map[string]string{"sample/sample.json": `{"layouts":[]}`, "sample/sample.md": "# Fallback heading", "README.md": "not the document"} {
		f, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buf.Bytes(), media.NewBudget())
	if err != nil || len(res.JSON) != 1 || res.File["parse_method"] != "monkeyocrv2" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "without layout positions") {
		t.Fatalf("unexpected fallback: %+v, %v", res, err)
	}
	item := res.JSON[0].ToMap()
	if item["text"] != "# Fallback heading" || item["_pdf_positions"] != nil {
		t.Fatalf("unexpected Markdown item: %#v", item)
	}
}

func TestMonkeyOCRv2PositionedPicturesWithoutReferences(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("sample/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(f, `{"layouts":[
		{"label":"Text","content":"  ","page_num":1,"bbox":[1,2,10,20]},
		{"label":"Picture","content":"plain caption","page_num":2,"bbox":[2,3,12,18]},
		{"label":"Picture","page_num":3,"bbox":[4,5,14,15]},
		{"label":"Picture","page_num":4},
		{"label":"Picture","page_num":5,"bbox":[1,2,1,2]}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	res, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buf.Bytes(), media.NewBudget())
	if err != nil || len(res.JSON) != 2 || res.File["page_count"] != 3 {
		t.Fatalf("crop-only figures: %+v, %v", res, err)
	}
	for i, wantText := range []string{"plain caption", ""} {
		item := res.JSON[i].ToMap()
		if item["text"] != wantText || item["page_number"] != i+2 || item["doc_type_kwd"] != "image" || item["media_omitted"] != true || !parser.HasPDFCropRegion(item) {
			t.Fatalf("item %d lost its caption or crop: %#v", i, item)
		}
	}
}

func TestMonkeyOCRv2TextWithMultipleImageReferences(t *testing.T) {
	block := map[string]any{"label": "Text", "content": "before ![one](data:image/png;base64,YQ==) between ![two](data:image/png;base64,Yg==) after", "page_num": 2, "bbox": []any{1, 2, 10, 20}}
	for _, limit := range []int{2, 1} {
		budget := media.NewBudgetWithLimits(media.Limits{MaxImagesPerRun: limit, MaxBytesPerRun: 10, ImageMaxBytes: 10})
		sections := monkeyOCRv2Sections(block, nil, budget)
		if len(sections) != 2 {
			t.Fatalf("limit %d: want two figures, got %+v", limit, sections)
		}
		if got := sections[0].text + sections[1].text; got != "before one between two after" {
			t.Fatalf("surrounding text or alt text changed: %q", got)
		}
		wantSecond := "Yg=="
		if limit == 1 {
			wantSecond = ""
		}
		for i, want := range []string{"YQ==", wantSecond} {
			s := sections[i]
			if s.layout != "figure" || s.image != want || s.page != 2 || !reflect.DeepEqual(s.position, []any{2, 1.0, 10.0, 2.0, 20.0}) {
				t.Fatalf("limit %d: figure %d = %+v", limit, i, s)
			}
		}
		if budget.Count() != limit || len(budget.Warnings()) != 2-limit {
			t.Fatalf("limit %d: charged %d, warnings %v", limit, budget.Count(), budget.Warnings())
		}
	}
}

func TestMonkeyOCRv2UnpositionedPictureKeepsTextForMissingImage(t *testing.T) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	err := archive.AddFS(fstest.MapFS{
		"sample/sample.json": {Data: []byte(`{"layouts":[
			{"label":"Picture","content":"before ![one](a.png) between ![two](missing.png) after"},
			{"label":"Picture","content":"before ![one](a.png) between ![two](b.png) after"}
		]}`)},
		"sample/images/a.png": {Data: []byte("a")},
		"sample/images/b.png": {Data: []byte("b")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := monkeyOCRv2ParseArchive(t.Context(), "sample.pdf", buf.Bytes(), media.NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ layout, text, image string }{
		{"figure", "before one", "YQ=="},
		{"text", " between two after", ""},
		{"figure", "before one", "YQ=="},
		{"figure", " between two after", "Yg=="},
	}
	if len(result.JSON) != len(want) {
		t.Fatalf("got %d items, want %d: %#v", len(result.JSON), len(want), result.JSON)
	}
	for i, expected := range want {
		item := result.JSON[i].ToMap()
		image, _ := item["image"].(string)
		if item["layout"] != expected.layout || item["text"] != expected.text || image != expected.image {
			t.Errorf("item %d = %#v, want %+v", i, item, expected)
		}
	}
}

func TestMonkeyOCRv2ReservesZIPImageBeforeOpening(t *testing.T) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	member, err := archive.CreateHeader(&zip.FileHeader{Name: "figure.png", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(member, "ab"); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	opens := 0
	reader.RegisterDecompressor(zip.Store, func(r io.Reader) io.ReadCloser { opens++; return io.NopCloser(r) })
	images := map[string]*zip.File{"figure.png": reader.File[0]}
	for _, limits := range []media.Limits{
		{MaxImagesPerRun: 1, MaxBytesPerRun: 10, ImageMaxBytes: 10},
		{MaxImagesPerRun: 3, MaxBytesPerRun: 3, ImageMaxBytes: 10},
	} {
		opens = 0
		budget := media.NewBudgetWithLimits(limits)
		for _, want := range []string{"YWI=", ""} {
			image := monkeyOCRv2EmbeddedImage("figure.png", images, budget)
			if image != want {
				t.Fatalf("image = %q, want %q", image, want)
			}
		}
		if opens != 1 || len(budget.Warnings()) != 1 {
			t.Fatalf("opened %d times, warnings %v", opens, budget.Warnings())
		}
	}
}
