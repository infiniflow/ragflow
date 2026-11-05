package component

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

func TestPDFDispatchNormalizesBeforeOutputsAndEnhancement(t *testing.T) {
	image := visionTestPNGBase64(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		archive := zip.NewWriter(&buf)
		file, _ := archive.Create("sample/sample.json")
		_ = json.NewEncoder(file).Encode(map[string]any{"layouts": []any{map[string]any{"label": "Text", "content": "![figure](data:image/png;base64," + image + ")"}}})
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
	if err != nil || !handled || !result.Produced() || len(result.JSON) == 0 {
		t.Fatalf("dispatch did not emit structured JSON: %+v, %v", result, err)
	}
	for _, enabled := range []bool{false, true} {
		invoker := &visionEnhanceCaptureInvoker{}
		swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
		component, err := NewParserComponent(map[string]any{"pdf": map[string]any(setup), "enable_vision_enhancement": enabled})
		if err != nil {
			t.Fatal(err)
		}
		out, err := component.Invoke(t.Context(), nil, map[string]any{"binary": []byte("pdf"), "file_type": "pdf", "name": "sample.pdf", "tenant_id": "tenant-1"})
		if err != nil {
			t.Fatal(err)
		}
		var text string
		for _, item := range out["json"].([]parser.DocItem) {
			value, _ := item.ToMap()["text"].(string)
			text += value
		}
		if strings.Contains(text, "a diagram of a pipeline") != enabled {
			t.Fatalf("enabled=%v output=%s", enabled, text)
		}
		want := 0
		if enabled {
			want = 1
		}
		if len(invoker.images) != want {
			t.Fatalf("enabled=%v model calls=%d, want %d", enabled, len(invoker.images), want)
		}
	}
}

func TestMinerUTableRetainsCaptionAndFootnote(t *testing.T) {
	item := mineruContentItemToJSON(map[string]any{"type": "table", "table_body": "<table><tr><td>Cell</td></tr></table>", "table_caption": []any{"Caption"}, "table_footnote": []any{"Footnote"}, "page_idx": 1, "bbox": []any{10.0, 20.0, 30.0, 40.0}}, nil, nil)
	table, ok := item["table"].(*entity.TableData)
	if !ok || table == nil || table.Caption != "Caption" || len(table.CellTexts()) != 1 || len(table.CellTexts()[0]) != 1 || table.CellTexts()[0][0] != "Cell" || table.Page != 2 {
		t.Fatalf("structured table missing: %#v", item)
	}
	if item["text"] != "Footnote" {
		t.Fatalf("notes lost or table duplicated: %#v", item)
	}
}

func TestMinerUTablePreservesEmbeddedCaption(t *testing.T) {
	item := mineruContentItemToJSON(map[string]any{"type": "table", "table_body": "<table><caption>Embedded caption</caption><tr><td>Cell</td></tr></table>"}, nil, nil)
	table, ok := item["table"].(*entity.TableData)
	if !ok || table == nil || table.Caption != "Embedded caption" {
		t.Fatalf("embedded caption lost: %#v", item)
	}
}

func TestPDFTableEnhancementKeepsDescriptionOutsideTableBody(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	result := parser.ParseResult{JSON: []parser.DocItem{
		parser.NewTextJSONItem("Before"),
		parser.TableItem{Table: &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell value"}}}}, Extra: map[string]any{"image": visionTestPNGBase64(t), "page_number": 3}},
		parser.NewTextJSONItem("After"),
	}}
	result, handled, err := maybeDispatchVisionEnhancement(t.Context(), nil, utility.FileTypePDF, result, map[string]any{"tenant_id": "tenant-1"}, visionSettings{}, progress.Window{})
	if err != nil || !handled {
		t.Fatalf("enhancement failed: %v", err)
	}
	if len(result.JSON) != 4 || result.JSON[1].ToMap()["text"] != nil || result.JSON[2].ToMap()["text"] != "a diagram of a pipeline" || result.JSON[2].ToMap()["page_number"] != 3 || result.JSON[3].ToMap()["text"] != "After" {
		t.Fatalf("table description broke table contract or order: %#v", result.JSON)
	}
}

func TestMinerUContentMarksBackendDescribedImages(t *testing.T) {
	withDesc := mineruContentItemToJSON(map[string]any{
		"type": "image", "image_caption": []any{"Caption"},
		"vlm_description": "backend desc",
		"page_idx":        0, "bbox": []any{0.0, 0.0, 10.0, 10.0},
	}, nil, nil)
	if withDesc == nil {
		t.Fatal("image item dropped")
	}
	if withDesc[backendDescribedKey] != true {
		t.Fatalf("backend-described image not marked: %#v", withDesc)
	}
	if text, _ := withDesc["text"].(string); !strings.Contains(text, "backend desc") {
		t.Fatalf("backend description lost from text: %q", text)
	}
	withoutDesc := mineruContentItemToJSON(map[string]any{
		"type": "image", "image_caption": []any{"Caption"},
		"page_idx": 0, "bbox": []any{0.0, 0.0, 10.0, 10.0},
	}, nil, nil)
	if withoutDesc == nil {
		t.Fatal("image item dropped")
	}
	if _, ok := withoutDesc[backendDescribedKey]; ok {
		t.Fatalf("undescribed image wrongly marked: %#v", withoutDesc)
	}
}

func TestVisionEnhancementSkipsBackendDescribedImages(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	png := "data:image/png;base64," + visionTestPNGBase64(t)
	marked := parser.DocItemFromMap(map[string]any{
		"doc_type_kwd": "image", "layout": "figure",
		"text": "Caption\nbackend desc", "image": png,
		backendDescribedKey: true,
	})
	plain := parser.DocItemFromMap(map[string]any{
		"doc_type_kwd": "image", "layout": "figure",
		"text": "Caption", "image": png,
	})
	result := parser.ParseResult{JSON: []parser.DocItem{marked, plain}}
	enhanced, handled, err := maybeDispatchVisionEnhancement(t.Context(), nil, utility.FileTypePDF, result, map[string]any{"tenant_id": "tenant-1"}, visionSettings{}, progress.Window{})
	if err != nil || !handled {
		t.Fatalf("enhancement failed: %v", err)
	}
	if len(invoker.images) != 1 {
		t.Fatalf("backend-described image should be skipped: %d VLM calls", len(invoker.images))
	}
	if len(enhanced.JSON) != 2 {
		t.Fatalf("items lost: %#v", enhanced.JSON)
	}
	first := enhanced.JSON[0].ToMap()
	if text, _ := first["text"].(string); text != "Caption\nbackend desc" {
		t.Fatalf("backend-described text duplicated or changed: %q", text)
	}
	if _, ok := first[backendDescribedKey]; ok {
		t.Fatalf("marker leaked to output: %#v", first)
	}
	second := enhanced.JSON[1].ToMap()
	if text, _ := second["text"].(string); !strings.Contains(text, "a diagram of a pipeline") {
		t.Fatalf("undescribed image not enhanced: %q", text)
	}
	if _, ok := second[backendDescribedKey]; ok {
		t.Fatalf("marker leaked to output: %#v", second)
	}
}

func TestStripBackendDescribedResultClearsMarker(t *testing.T) {
	result := parser.ParseResult{JSON: []parser.DocItem{
		parser.DocItemFromMap(map[string]any{"doc_type_kwd": "image", "text": "t", backendDescribedKey: true}),
		parser.NewTextJSONItem("plain"),
	}}
	stripped := stripBackendDescribedResult(result)
	for _, item := range stripped.JSON {
		if _, ok := item.ToMap()[backendDescribedKey]; ok {
			t.Fatalf("marker leaked: %#v", item.ToMap())
		}
	}
	if text, _ := stripped.JSON[0].ToMap()["text"].(string); text != "t" {
		t.Fatalf("text changed: %q", text)
	}
}
