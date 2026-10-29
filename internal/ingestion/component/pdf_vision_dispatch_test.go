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
			got := mineruContentItemToJSON(tc.item)
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

func TestMinerUExtractJSONItemsKeepsImageTextAndPositionsWithoutPayload(t *testing.T) {
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
	// Non-markdown parse results never inline image payloads (issue #21213):
	// the chunker crops the region on demand from the source PDF using the
	// positions below.
	if payload, ok := imageItem["image"]; ok {
		t.Fatalf("image item unexpectedly carries an inlined payload: %v", payload)
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

func mineruTestZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMinerUInlineMarkdownImagesBudgetsPayloads(t *testing.T) {
	small := []byte("png-data")
	oversized := make([]byte, parser.MaxImagePayloadBytes+1)
	for i := range oversized {
		oversized[i] = 'a'
	}
	zipBytes := mineruTestZip(t, map[string][]byte{
		"images/small.png": small,
		"images/huge.png":  oversized,
	})
	md := "Intro.\n\n![A small figure](images/small.png)\n\n![A huge figure](images/huge.png)\n\n![External](https://example.com/x.png)\n\n![Missing](images/absent.png)\n\nOutro."
	inlined, warnings := mineruInlineMarkdownImages(md, zipBytes)

	if !strings.Contains(inlined, "![A small figure](data:image/png;base64,") {
		t.Fatalf("small image not inlined as data URI:\n%s", inlined)
	}
	if strings.Contains(inlined, "images/huge.png") || !strings.Contains(inlined, "A huge figure") {
		t.Fatalf("oversized image must lose its link but keep its alt text:\n%s", inlined)
	}
	if !strings.Contains(inlined, "![External](https://example.com/x.png)") {
		t.Fatalf("external reference must stay untouched:\n%s", inlined)
	}
	if !strings.Contains(inlined, "![Missing](images/absent.png)") {
		t.Fatalf("unmatched reference must stay untouched:\n%s", inlined)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "per-image limit") {
		t.Fatalf("warnings = %#v, want one per-image-limit warning", warnings)
	}
}

func TestMinerUInlineMarkdownImagesWithoutZipLeavesMarkdown(t *testing.T) {
	md := "![A figure](images/figure.png)"
	inlined, warnings := mineruInlineMarkdownImages(md, nil)
	if inlined != md || len(warnings) != 0 {
		t.Fatalf("no zip: inlined = %q warnings = %#v", inlined, warnings)
	}
}

func buildMinerUTestZip(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMinerUZipImagesAmbiguousBasenameResolvesNoWrongImage(t *testing.T) {
	zipBytes := buildMinerUTestZip(t, map[string]string{
		"page1/img.png":  "first-payload",
		"page2/img.png":  "second-payload",
		"page1/solo.png": "solo-payload",
	})
	images := mineruZipImages(zipBytes)

	// The shared basename must not silently pick one member.
	if f := mineruLookupZipImage(images, "img.png"); f != nil {
		t.Fatalf("ambiguous basename resolved to %q, want nil", f.Name)
	}
	// Full paths still resolve each member exactly.
	if f := mineruLookupZipImage(images, "page1/img.png"); f == nil || f.Name != "page1/img.png" {
		t.Fatalf("page1/img.png resolved to %v", f)
	}
	if f := mineruLookupZipImage(images, "page2/img.png"); f == nil || f.Name != "page2/img.png" {
		t.Fatalf("page2/img.png resolved to %v", f)
	}
	// An unshared basename still resolves.
	if f := mineruLookupZipImage(images, "solo.png"); f == nil || f.Name != "page1/solo.png" {
		t.Fatalf("solo.png resolved to %v", f)
	}
}

func withMinerUInlineBudget(t *testing.T, maxImageBytes, maxTotalBytes, maxItems int) {
	t.Helper()
	original := newMinerUInlineBudget
	newMinerUInlineBudget = func() *parser.EmbeddedMediaBudget {
		return parser.NewEmbeddedMediaBudgetWithLimits(maxImageBytes, maxTotalBytes, maxItems)
	}
	t.Cleanup(func() { newMinerUInlineBudget = original })
}

func TestMinerUInlineMarkdownImagesDocumentByteBudgetExhaustion(t *testing.T) {
	// Two 6-byte images against an 8-byte document budget: the second loses
	// its link, keeps its alt text, and the omission surfaces as a warning.
	withMinerUInlineBudget(t, 1024, 8, 16)
	zipBytes := buildMinerUTestZip(t, map[string]string{
		"images/a.png": "aaaaaa",
		"images/b.png": "bbbbbb",
	})
	markdown := "![first](images/a.png)\ntext\n![second](images/b.png)"

	out, warnings := mineruInlineMarkdownImages(markdown, zipBytes)
	if !strings.Contains(out, "![first](data:image/png;base64,") {
		t.Fatalf("first image not inlined: %q", out)
	}
	if strings.Contains(out, "![second](data:") || !strings.Contains(out, "second") {
		t.Fatalf("second image should drop to its alt text: %q", out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "document image budget") {
		t.Fatalf("warnings = %q, want one document-budget warning", warnings)
	}
}

func TestMinerUInlineMarkdownImagesItemBudgetExhaustion(t *testing.T) {
	// A 2-item budget inlines two images and drops the third; once exhausted
	// the walk stops charging further references.
	withMinerUInlineBudget(t, 1024, 1<<20, 2)
	zipBytes := buildMinerUTestZip(t, map[string]string{
		"images/a.png": "aa",
		"images/b.png": "bb",
		"images/c.png": "cc",
	})
	markdown := "![one](images/a.png)\n![two](images/b.png)\n![three](images/c.png)"

	out, warnings := mineruInlineMarkdownImages(markdown, zipBytes)
	if !strings.Contains(out, "![one](data:") || !strings.Contains(out, "![two](data:") {
		t.Fatalf("first two images should inline: %q", out)
	}
	if strings.Contains(out, "![three](data:") || !strings.Contains(out, "three") {
		t.Fatalf("third image should drop to its alt text: %q", out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "item document limit") {
		t.Fatalf("warnings = %q, want one item-limit warning", warnings)
	}
}
