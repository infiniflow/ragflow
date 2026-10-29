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
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"

	"gorm.io/gorm"
)

func TestMonkeyOCRv2ExtractSectionsUsesNativeJSON(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("sample/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"layouts": []map[string]any{{"label": "Page-header", "content": "ignored"}, {"label": "Title", "content": "Heading"}, {"label": "Formula", "content": "x^2"}}})
	_, _ = file.Write(payload)
	_ = writer.Close()
	sections, err := monkeyOCRv2ExtractSections(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 2 || sections[0] != "Heading" || sections[1] != "x^2" {
		t.Fatalf("sections=%v", sections)
	}
}

func TestMonkeyOCRv2ExtractSectionsEmbedsZIPImage(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	jsonFile, _ := writer.Create("sample/sample.json")
	_, _ = jsonFile.Write([]byte(`{"layouts":[{"label":"Picture","content":"![figure](images/figure.png)"}]}`))
	imageFile, _ := writer.Create("sample/images/figure.png")
	_, _ = imageFile.Write([]byte("png-data"))
	_ = writer.Close()

	sections, err := monkeyOCRv2ExtractSections(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || !strings.HasPrefix(sections[0], "![figure](data:image/png;base64,") {
		t.Fatalf("sections=%v", sections)
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
	pages []modelModule.OCRPage
}

func (d *paddleOCRFakeDriver) Name() string { return "PaddleOCR" }

func (d *paddleOCRFakeDriver) OCRFile(_ context.Context, _ *string, _ []byte, _ *string, _ *modelModule.APIConfig, _ *modelModule.OCRConfig, _ *common.ModelUsage) (*modelModule.OCRFileResponse, error) {
	return &modelModule.OCRFileResponse{Pages: d.pages}, nil
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
	items, err := mineruExtractJSONItems(buf.Bytes(), sizes)
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
	uri, _ := imageItem["image"].(string)
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Fatalf("image data URI = %q", uri)
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

	items, err := mineruExtractJSONItems(buf.Bytes(), nil)
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

	items, err := mineruExtractJSONItems(buf.Bytes(), nil)
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
	items, err := mineruExtractJSONItems(buf.Bytes(), map[int][2]float64{0: {pageWidth, pageHeight}})
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
