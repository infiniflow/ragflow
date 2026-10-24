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
	"net/http"
	"net/http/httptest"
	"os"
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
	sections, err := monkeyOCRv2ExtractSections(buffer.Bytes(), parser.NewEmbeddedMediaBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 2 || sections[0].text != "Heading" || sections[1].text != "x^2" {
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

	sections, err := monkeyOCRv2ExtractSections(buffer.Bytes(), parser.NewEmbeddedMediaBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || !strings.HasPrefix(sections[0].text, "![figure](data:image/png;base64,") {
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
			got := mineruContentItemToJSON(tc.item, nil)
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

	items, err := mineruExtractJSONItems(buf.Bytes())
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
	if left, _ := pos[0][1].(float64); left != 10 {
		t.Fatalf("bbox left = %#v, want 10 (x0)", pos[0])
	}
	if right, _ := pos[0][2].(float64); right != 110 {
		t.Fatalf("bbox right = %#v, want 110 (x1)", pos[0])
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
func TestMonkeyOCRv2ExtractSectionsBudgetOmitsPayloadKeepsAltText(t *testing.T) {
	oversized := make([]byte, parser.MaxImagePayloadBytes+1)
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

	budget := parser.NewEmbeddedMediaBudget()
	sections, err := monkeyOCRv2ExtractSections(buffer.Bytes(), budget)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || !sections[0].imageOnly || sections[0].text != "big figure" {
		t.Fatalf("oversized image must stay a position carrier keeping its alt text: sections=%v", sections)
	}
	if len(sections[0].position) != 5 || sections[0].position[0] != 2 {
		t.Fatalf("omitted image must keep its layout position: %#v", sections[0].position)
	}
	warnings := budget.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "per-image limit") {
		t.Fatalf("warnings = %#v, want one per-image-limit warning", warnings)
	}
}

func TestMonkeyOCRv2AttachPositionsMatchesMarkdownItems(t *testing.T) {
	sections := []monkeyOCRv2Section{
		{text: "# Heading", position: []any{1, 10.0, 200.0, 680.0, 700.0}},
		{text: "Body text.", position: []any{2, 15.0, 300.0, 100.0, 140.0}},
		{text: "No geometry here"},
	}
	res := parser.ParseResult{JSON: parser.MapsToItems([]map[string]any{
		{"text": "Heading", "doc_type_kwd": "text"},
		{"text": "Body text.", "doc_type_kwd": "text"},
		{"text": "No geometry here", "doc_type_kwd": "text"},
	})}
	res = monkeyOCRv2AttachPositions(res, sections)
	first := res.JSON[0].ToMap()
	rows, ok := first["_pdf_positions"].([][]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("heading positions = %#v", first["_pdf_positions"])
	}
	if page, _ := rows[0][0].(int); page != 1 {
		t.Fatalf("heading page = %#v, want 1", rows[0])
	}
	second := res.JSON[1].ToMap()
	if page, _ := second["page_number"].(int); page != 2 {
		t.Fatalf("body page_number = %#v, want 2", second["page_number"])
	}
	third := res.JSON[2].ToMap()
	if _, ok := third["_pdf_positions"]; ok {
		t.Fatalf("ungeometried section must stay position-free: %#v", third)
	}
}

func TestMonkeyOCRv2AttachPositionsMapsImageItemsOrdinally(t *testing.T) {
	sections := []monkeyOCRv2Section{
		{text: "![first](data:image/png;base64,AAAA)", position: []any{1, 1.0, 2.0, 3.0, 4.0}},
		{text: "![second](data:image/png;base64,BBBB)", position: []any{2, 5.0, 6.0, 7.0, 8.0}},
		{text: "![third](data:image/png;base64,CCCC)"}, // no geometry: skipped
	}
	res := parser.ParseResult{JSON: parser.MapsToItems([]map[string]any{
		{"doc_type_kwd": "image", "image": "data:image/png;base64,AAAA"},
		{"doc_type_kwd": "image", "image": "data:image/png;base64,BBBB"},
		{"doc_type_kwd": "image", "image": "data:image/png;base64,DDDD"},
	})}
	res = monkeyOCRv2AttachPositions(res, sections)
	first := res.JSON[0].ToMap()
	if page, _ := first["page_number"].(int); page != 1 {
		t.Fatalf("first image page = %#v, want 1", first)
	}
	second := res.JSON[1].ToMap()
	rows, ok := second["_pdf_positions"].([][]any)
	if !ok || len(rows) != 1 || rows[0][0] != 2 {
		t.Fatalf("second image positions = %#v, want page 2", second)
	}
	third := res.JSON[2].ToMap()
	if _, ok := third["_pdf_positions"]; ok {
		t.Fatalf("image item without a positioned section must stay position-free: %#v", third)
	}
}

func TestMonkeyOCRv2AttachPositionsInsertsOmittedImagesAsPositionOnlyItems(t *testing.T) {
	sections := []monkeyOCRv2Section{
		{text: "Before.", position: []any{1, 1.0, 2.0, 3.0, 4.0}, label: "text"},
		{text: "", position: []any{2, 5.0, 6.0, 7.0, 8.0}, label: "picture", imageOnly: true},
		{text: "After.", position: []any{3, 9.0, 10.0, 11.0, 12.0}, label: "text"},
	}
	res := parser.ParseResult{JSON: parser.MapsToItems([]map[string]any{
		{"text": "Before.", "doc_type_kwd": "text"},
		{"text": "After.", "doc_type_kwd": "text"},
	})}
	res = monkeyOCRv2AttachPositions(res, sections)
	if len(res.JSON) != 3 {
		t.Fatalf("items = %d, want 3 with the inserted position-only image item", len(res.JSON))
	}
	mid := res.JSON[1].ToMap()
	if mid["doc_type_kwd"] != "image" || mid["media_omitted"] != true {
		t.Fatalf("inserted item = %#v, want media_omitted image", mid)
	}
	if text, _ := mid["text"].(string); text != "" {
		t.Fatalf("no visible text may be invented, got %q", text)
	}
	rows, ok := mid["_pdf_positions"].([][]any)
	if !ok || len(rows) != 1 || rows[0][0] != 2 {
		t.Fatalf("inserted positions = %#v, want page 2", mid["_pdf_positions"])
	}
	last := res.JSON[2].ToMap()
	if last["text"] != "After." {
		t.Fatalf("insertion broke document order: %#v", last)
	}
	if page, _ := last["page_number"].(int); page != 3 {
		t.Fatalf("trailing item positions = %#v, want page 3", last)
	}
}

func TestMonkeyOCRv2ExtractSectionsMissingImageKeepsPositionCarrier(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	jsonFile, _ := writer.Create("sample/sample.json")
	_, _ = jsonFile.Write([]byte(`{"layouts":[
		{"label":"Picture","content":"![](images/gone.png)","page_num":2,"bbox":[1,2,3,4]},
		{"label":"Picture","content":"![](images/also-gone.png)"}
	]}`))
	_ = writer.Close()

	sections, err := monkeyOCRv2ExtractSections(buffer.Bytes(), parser.NewEmbeddedMediaBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || !sections[0].imageOnly {
		t.Fatalf("missing image with a position must keep a carrier, without one must drop: sections=%v", sections)
	}
	if len(sections[0].position) != 5 || sections[0].position[0] != 2 {
		t.Fatalf("carrier position = %#v, want page 2", sections[0].position)
	}
}

func TestMonkeyOCRv2LayoutPositionShape(t *testing.T) {
	got := monkeyOCRv2LayoutPosition(map[string]any{"page_num": 2, "bbox": []any{110.0, 40.0, 10.0, 20.0}})
	if len(got) != 5 || got[0] != 2 || got[1] != 10.0 || got[2] != 110.0 || got[3] != 20.0 || got[4] != 40.0 {
		t.Fatalf("position = %#v, want [2 10 110 20 40] with normalized corners", got)
	}
	def := monkeyOCRv2LayoutPosition(map[string]any{"bbox": []any{50.0, 60.0, 5.0, 10.0}})
	if len(def) != 5 || def[0] != 1 || def[1] != 5.0 || def[2] != 50.0 || def[3] != 10.0 || def[4] != 60.0 {
		t.Fatalf("default-page position = %#v", def)
	}
	if none := monkeyOCRv2LayoutPosition(map[string]any{"page_num": 1}); none != nil {
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
