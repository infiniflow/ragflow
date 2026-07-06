//go:build cgo

package component

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
)

func TestDispatch_PDFVisionJSON_RealPDFFixture(t *testing.T) {
	origPromptLoader := pdfVisionPromptLoader
	origResolver := pdfVisionModelResolver
	origInvoker := pdfVisionChatInvoker
	t.Cleanup(func() {
		pdfVisionPromptLoader = origPromptLoader
		pdfVisionModelResolver = origResolver
		pdfVisionChatInvoker = origInvoker
	})

	pdfVisionPromptLoader = func(name string) (string, error) {
		return "Describe page {{ page }}.", nil
	}
	pdfVisionModelResolver = func(tenantID string, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		if tenantID != "tenant-vision" || modelID != "CustomVLM" {
			t.Fatalf("resolver got tenant/model %q/%q", tenantID, modelID)
		}
		return nil, "fixture-vlm", nil, nil
	}

	var callCount atomic.Int32
	pdfVisionChatInvoker = func(_ modelModule.ModelDriver, modelName string, messages []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
		if modelName != "fixture-vlm" {
			t.Fatalf("modelName = %q, want fixture-vlm", modelName)
		}
		callCount.Add(1)
		content, ok := messages[0].Content.([]interface{})
		if !ok || len(content) != 2 {
			t.Fatalf("messages[0].Content = %#v, want multimodal payload", messages[0].Content)
		}
		promptBlock, ok := content[0].(map[string]any)
		if !ok {
			t.Fatalf("prompt block = %T, want map[string]any", content[0])
		}
		prompt, _ := promptBlock["text"].(string)
		answer := "Recognized " + prompt + "\n\n--- Page ---"
		return &modelModule.ChatResponse{Answer: &answer}, nil
	}

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	param := schema.ParserParam{}.Defaults()
	param.Setups["pdf"]["parse_method"] = "CustomVLM"
	param.Setups["pdf"]["output_format"] = "json"
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    data,
		"file_type": "pdf",
		"name":      "Doc1.pdf",
		"tenant_id": "tenant-vision",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	file, ok := out["file"].(map[string]any)
	if !ok {
		t.Fatalf("file metadata missing: %T", out["file"])
	}
	pageCount, ok := file["page_count"].(int)
	if !ok || pageCount < 1 {
		t.Fatalf("file.page_count = %#v, want positive int", file["page_count"])
	}
	if got := int(callCount.Load()); got != pageCount {
		t.Fatalf("vision call count = %d, want %d", got, pageCount)
	}

	jsonItems, ok := out["json"].([]map[string]any)
	if !ok || len(jsonItems) == 0 {
		t.Fatalf("json payload missing or empty: %T", out["json"])
	}
	if got, _ := jsonItems[0]["text"].(string); !strings.Contains(got, "Recognized Describe page 1.") {
		t.Fatalf("json[0].text = %q, want rendered vision answer", got)
	}
	if positions, ok := jsonItems[0]["_pdf_positions"].([][]any); !ok || len(positions) == 0 {
		t.Fatalf("json[0]._pdf_positions = %#v, want normalized page positions", jsonItems[0]["_pdf_positions"])
	}
}
