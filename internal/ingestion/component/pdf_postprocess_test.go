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
	if err != nil || !handled || result.OutputFormat != "json" || len(result.JSON) == 0 {
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
	item := mineruContentItemToJSON(map[string]any{"type": "table", "table_body": "<table><tr><td>Cell</td></tr></table>", "table_caption": []any{"Caption"}, "table_footnote": []any{"Footnote"}, "page_idx": 1, "bbox": []any{10.0, 20.0, 30.0, 40.0}}, nil)
	table, ok := item["table"].(*entity.TableData)
	if !ok || table == nil || table.Caption != "Caption" || len(table.CellTexts()) != 1 || len(table.CellTexts()[0]) != 1 || table.CellTexts()[0][0] != "Cell" || table.Page != 2 {
		t.Fatalf("structured table missing: %#v", item)
	}
	if item["text"] != "Footnote" {
		t.Fatalf("notes lost or table duplicated: %#v", item)
	}
}

func TestMinerUTablePreservesEmbeddedCaption(t *testing.T) {
	item := mineruContentItemToJSON(map[string]any{"type": "table", "table_body": "<table><caption>Embedded caption</caption><tr><td>Cell</td></tr></table>"}, nil)
	table, ok := item["table"].(*entity.TableData)
	if !ok || table == nil || table.Caption != "Embedded caption" {
		t.Fatalf("embedded caption lost: %#v", item)
	}
}

func TestPDFTableEnhancementKeepsDescriptionOutsideTableBody(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	result := parser.ParseResult{OutputFormat: "json", JSON: []parser.DocItem{
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
