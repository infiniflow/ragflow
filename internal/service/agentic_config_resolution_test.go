package service

import (
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/entity"
)

func TestChatAgenticTemplateResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentic.yaml")
	if err := os.WriteFile(path, []byte(`templates:
  - id: smart-reasoning
    content: default prompt
    temperature: 0.5
    tools: [grep_chunks, list_chunks, think]
  - id: alternate
    content: alternate prompt
    temperature: 0.2
    tools: [search_chunks, list_chunks]
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTIC_RAG_CONFIG", path)
	chat := &entity.Chat{PromptConfig: entity.JSONMap{
		"system":      "standard RAG prompt",
		"agentic_rag": map[string]any{"content": "saved prompt"},
	}}
	for _, tc := range []struct {
		mode        string
		temperature float64
	}{{"", 0.5}, {"alternate", 0.2}} {
		t.Run(tc.mode, func(t *testing.T) {
			tmpl, err := resolveChatAgenticTemplate(chat, map[string]any{
				"agent_mode":  tc.mode,
				"agentic_rag": map[string]any{"content": "unsaved injection", "tools": []any{"web_search"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if tmpl.Content != "saved prompt" || tmpl.Temperature == nil || *tmpl.Temperature != tc.temperature {
				t.Fatalf("unexpected resolved template: %+v", tmpl)
			}
		})
	}
	chat.PromptConfig["agentic_rag"] = nil
	tmpl, err := resolveChatAgenticTemplate(chat, nil)
	if err != nil || tmpl.Content != "default prompt" {
		t.Fatalf("reset: %+v, %v", tmpl, err)
	}
	if _, err := resolveChatAgenticTemplate(chat, map[string]any{"agent_mode": "missing"}); err == nil {
		t.Fatal("unknown mode accepted")
	}
	chat.PromptConfig["agentic_rag"] = map[string]any{"tools": []any{"web_search"}}
	if _, err := resolveChatAgenticTemplate(chat, nil); err == nil {
		t.Fatal("invalid stored override accepted")
	}
}
