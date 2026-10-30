package agentic_rag

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity/models"
)

func TestParseOverride(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  any
		want string
	}{
		{"reset", nil, ""},
		{"inherit", map[string]any{}, ""},
		{"prompt", map[string]any{"content": "custom instruction"}, ""},
		{"tools", map[string]any{"tools": []any{"grep_chunks", "list_chunks"}}, ""},
		{"structure", "prompt", "object"},
		{"unknown field", map[string]any{"temperature": 1}, "unknown"},
		{"null prompt", map[string]any{"content": nil}, "string"},
		{"blank prompt", map[string]any{"content": " \n"}, "empty"},
		{"large prompt", map[string]any{"content": strings.Repeat("界", 32769)}, "32768"},
		{"NUL", map[string]any{"content": "bad\x00prompt"}, "NUL"},
		{"UTF8", map[string]any{"content": string([]byte{0xff})}, "UTF-8"},
		{"null tools", map[string]any{"tools": nil}, "array"},
		{"tools type", map[string]any{"tools": "grep_chunks"}, "array"},
		{"tool type", map[string]any{"tools": []any{1}}, "string"},
		{"unknown tool", map[string]any{"tools": []any{"unknown"}}, "unknown"},
		{"privileged web", map[string]any{"tools": []any{"web_search"}}, "unknown"},
		{"duplicate", map[string]any{"tools": []any{"grep_chunks", "grep_chunks", "list_chunks"}}, "duplicate"},
		{"empty", map[string]any{"tools": []any{}}, "corpus search"},
		{"missing search", map[string]any{"tools": []any{"list_chunks"}}, "corpus search"},
		{"missing deep read", map[string]any{"tools": []any{"grep_chunks"}}, "list_chunks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseOverride(tc.raw)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestResolveOverrideInheritanceIsolationAndReload(t *testing.T) {
	setupTestConfig(t)
	content := "custom instruction"
	names := []string{"grep_chunks", "list_chunks"}
	override := &Override{Content: &content, Tools: &names}
	effective, err := ResolveTemplate("smart-grep", override)
	if err != nil {
		t.Fatal(err)
	}
	if effective.ID != "smart-grep" || effective.Content != content || !slices.Equal(effective.Tools, names) || !effective.strictTools {
		t.Fatalf("resolved template: %+v", effective)
	}
	effective.Tools[0] = "think"
	original, err := ResolveTemplate("smart-grep", nil)
	if err != nil || original.Content != "GREP-ONLY PROMPT\n" || original.Tools[0] != "think" || names[0] != "grep_chunks" {
		t.Fatalf("resolution mutated input/cache: %+v, %v, %v", original, names, err)
	}
	original.Tools[0] = "mutated"
	cached, err := ResolveTemplate("smart-grep", nil)
	if err != nil || cached.Tools[0] != "think" {
		t.Fatalf("returned default aliases YAML cache: %+v, %v", cached, err)
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			prompt := strings.Repeat("x", i+1)
			got, err := ResolveTemplate("smart-reasoning", &Override{Content: &prompt})
			if err != nil || got.Content != prompt || !slices.Equal(got.Tools, []string{"think", "grep_chunks", "search_chunks", "list_chunks"}) {
				t.Errorf("concurrent resolution: %+v, %v", got, err)
			}
		})
	}
	wg.Wait()
	path := os.Getenv("AGENTIC_RAG_CONFIG")
	updated := strings.ReplaceAll(twoTemplateYAML, "FULL-MODE PROMPT", "RELOADED PROMPT")
	updated = strings.ReplaceAll(updated, "      - search_chunks\n", "      - search_bm25_chunks\n")
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveTemplate("smart-reasoning", &Override{Content: &content})
	if err != nil || got.Content != content || !slices.Contains(got.Tools, "search_bm25_chunks") {
		t.Fatalf("partial override failed to inherit reload: %+v, %v", got, err)
	}
	got, err = ResolveTemplate("smart-reasoning", nil)
	if err != nil || got.Content != "RELOADED PROMPT\n" {
		t.Fatalf("reset failed to inherit reload: %+v, %v", got, err)
	}
	got, err = ResolveTemplate("smart-reasoning", &Override{Tools: &names})
	if err != nil || got.Content != "RELOADED PROMPT\n" || !slices.Equal(got.Tools, names) {
		t.Fatalf("tools-only override failed to inherit reloaded prompt: %+v, %v", got, err)
	}
}

func TestSettingsRegistryAndDefaultIsolation(t *testing.T) {
	setupTestConfig(t)
	content := "custom"
	settings, err := GetSettings(t.Context(), &Override{Content: &content})
	if err != nil {
		t.Fatal(err)
	}
	if settings.BaseTemplate != "smart-reasoning" || settings.UsingDefaults || settings.Defaults.Content != "FULL-MODE PROMPT\n" || settings.Effective.Content != content {
		t.Fatalf("settings: %+v", settings)
	}
	if len(settings.AvailableTools) != len(toolRegistry()) {
		t.Fatalf("catalog length: %d", len(settings.AvailableTools))
	}
	var previous string
	for _, info := range settings.AvailableTools {
		if info.Name <= previous || info.Description == "" || info.Name == webSearchToolName {
			t.Fatalf("invalid registry description: %+v", info)
		}
		previous = info.Name
	}
	settings.Effective.Tools[0] = "changed"
	if settings.Defaults.Tools[0] == "changed" {
		t.Fatal("effective template aliases defaults")
	}
	defaults, err := GetSettings(t.Context(), nil)
	if err != nil || !defaults.UsingDefaults || defaults.Override != nil || defaults.Defaults.Content != defaults.Effective.Content {
		t.Fatalf("default settings: %+v, %v", defaults, err)
	}
}

func TestRunUsesOverridePromptAndExactTools(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, web := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/web=%t", stream, web), func(t *testing.T) {
				setupTestConfig(t)
				corpus := newCaptureService()
				runtime.SetRetrievalService(corpus)
				t.Cleanup(func() { runtime.SetRetrievalService(nil) })
				content := "CUSTOM SYSTEM INSTRUCTION"
				names := []string{"search_chunks", "list_chunks"}
				template, err := ResolveTemplate("smart-reasoning", &Override{Content: &content, Tools: &names})
				if err != nil {
					t.Fatal(err)
				}
				ctx := WithEvidenceRegistry(t.Context(), NewEvidenceRegistry())
				if web {
					ctx = WithWebSearch(ctx, func(context.Context, string) ([]WebResult, error) {
						return []WebResult{{Title: "web", URL: "https://example.com", Content: "evidence"}}, nil
					})
				}
				calls := 0
				driver := &retrievalToolDriver{respond: func(messages []models.Message, config *models.ChatConfig) *models.ChatResponse {
					calls++
					var system []string
					for _, message := range messages {
						if message.Role == "system" {
							system = append(system, fmt.Sprint(message.Content))
						}
					}
					if !strings.Contains(strings.Join(system, "\n"), content) || strings.Contains(strings.Join(system, "\n"), "FULL-MODE PROMPT") {
						t.Errorf("wrong executing instruction: %v", system)
					}
					advertised, _ := config.Tools.([]map[string]any)
					got := []string{}
					for _, definition := range advertised {
						function, _ := definition["function"].(map[string]any)
						got = append(got, fmt.Sprint(function["name"]))
					}
					want := slices.Clone(names)
					if web {
						want = append(want, webSearchToolName)
					}
					if !slices.Equal(got, want) {
						t.Errorf("actual tools = %v, want %v", got, want)
					}
					required := []string{"search_chunks"}
					if web {
						required = append(required, webSearchToolName)
					}
					if calls <= len(required) {
						name := required[calls-1]
						return &models.ChatResponse{ToolCalls: []map[string]any{{"id": name, "type": "function", "function": map[string]any{"name": name, "arguments": `{"queries":["evidence"]}`}}}}
					}
					answer := "custom answer [ID:0]"
					return &models.ChatResponse{Answer: &answer}
				}}
				modelName := driver.Name()
				counts := map[string]int{}
				answer, err := Run(ctx, Input{
					Model:    models.NewEinoChatModel(models.NewChatModel(driver, &modelName, &models.APIConfig{}), nil),
					Template: &template, DatasetIDs: []string{"kb1"}, TenantID: "tenant",
					// A direct caller's tools must not bypass a saved restriction.
					Tools:    []tool.BaseTool{NewThinkTool()},
					Messages: []*schema.Message{schema.UserMessage("question")},
					Stream:   stream, MaxIterations: 4, OnDelta: func(string, string) {}, ToolCallCounts: counts,
				})
				if err != nil || answer != "custom answer [ID:0]" || len(corpus.reqs) != 1 || counts["search_chunks"] != 1 {
					t.Fatalf("Run: answer=%q error=%v requests=%d counts=%v", answer, err, len(corpus.reqs), counts)
				}
			})
		}
	}
}

func TestRunSynthesisUsesOverrideAndCannotExecuteDisabledTool(t *testing.T) {
	setupTestConfig(t)
	corpus := newCaptureService()
	runtime.SetRetrievalService(corpus)
	t.Cleanup(func() { runtime.SetRetrievalService(nil) })
	content := "CUSTOM SYNTHESIS INSTRUCTION"
	names := []string{"grep_chunks", "list_chunks"}
	template, err := ResolveTemplate("smart-reasoning", &Override{Content: &content, Tools: &names})
	if err != nil {
		t.Fatal(err)
	}
	driver := &retrievalToolDriver{respond: func([]models.Message, *models.ChatConfig) *models.ChatResponse {
		return &models.ChatResponse{ToolCalls: []map[string]any{{"id": "forbidden", "type": "function", "function": map[string]any{"name": "search_chunks", "arguments": `{"queries":["forbidden"]}`}}}}
	}}
	synthCalls := 0
	synthDriver := &retrievalToolDriver{respond: func(messages []models.Message, config *models.ChatConfig) *models.ChatResponse {
		synthCalls++
		if len(messages) == 0 || messages[0].Role != "system" || fmt.Sprint(messages[0].Content) != content {
			t.Errorf("synthesis lost custom instruction: %+v", messages)
		}
		if config.Tools != nil {
			t.Errorf("synthesis advertised tools: %+v", config.Tools)
		}
		answer := "custom fallback"
		return &models.ChatResponse{Answer: &answer}
	}}
	modelName := driver.Name()
	answer, err := Run(t.Context(), Input{
		Model:      models.NewEinoChatModel(models.NewChatModel(driver, &modelName, &models.APIConfig{}), nil),
		SynthModel: models.NewEinoChatModel(models.NewChatModel(synthDriver, &modelName, &models.APIConfig{}), nil),
		Template:   &template, DatasetIDs: []string{"kb1"}, TenantID: "tenant",
		Messages:      []*schema.Message{schema.UserMessage("question")},
		MaxIterations: 2, OnDelta: func(string, string) {},
	})
	if err != nil || answer != "custom fallback" || synthCalls != 1 || len(corpus.reqs) != 0 {
		t.Fatalf("Run: answer=%q err=%v synth=%d forbidden executions=%d", answer, err, synthCalls, len(corpus.reqs))
	}
}

func TestStrictEmptyToolsNeverExpandToRegistry(t *testing.T) {
	got := toolsFor(t.Context(), Template{strictTools: true}, "tenant", nil, nil)
	if len(got) != 0 {
		t.Fatalf("empty explicit tools expanded to %d tools", len(got))
	}
}

func TestStrictToolsPreventCompiledInjection(t *testing.T) {
	setupTestConfig(t)
	de := &kgProbeFakeEngine{entityRows: []map[string]any{{"id": "compiled"}}}
	if !datasetsHaveCompiledKnowledge(t.Context(), de, "tenant", []string{"kb1"}) {
		t.Fatal("mock corpus must have compiled knowledge")
	}
	in := Input{TenantID: "tenant", DatasetIDs: []string{"kb1"}}
	defaults, err := ResolveTemplate("smart-reasoning", nil)
	if err != nil {
		t.Fatal(err)
	}
	defaultTools := toolsForRun(t.Context(), defaults, in, de)
	if len(defaultTools) != len(defaults.Tools)+len(compiledKnowledgeToolNames) {
		t.Fatalf("default tools lost automatic compilation capabilities: %d", len(defaultTools))
	}
	names := []string{"grep_chunks", "list_chunks"}
	strict, err := ResolveTemplate("smart-reasoning", &Override{Tools: &names})
	if err != nil {
		t.Fatal(err)
	}
	de.probeCalled = false
	selected := toolsForRun(t.Context(), strict, in, de)
	if de.probeCalled {
		t.Error("restricted configuration should not probe compiled capability")
	}
	var got []string
	for _, selectedTool := range selected {
		info, err := selectedTool.Info(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, info.Name)
	}
	if !slices.Equal(got, names) {
		t.Fatalf("compiled tools bypassed restriction: %v", got)
	}
}
