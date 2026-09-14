package service

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// Regression lock for the first-turn empty dataset binding: the agentic
// (reasoning) system prompt used to render {knowledge} as "" — the web UI's
// default template then read "derived solely from this dataset: ``" and the
// outer model answered the canned "not found in the dataset!" line without
// calling the terminal `rag` tool (logs/api_server_9384.log 2026-09-14
// 13:50:14: answer_chars=59, zero graph LLM calls).

func TestHarnessBoundDatasetNames(t *testing.T) {
	cases := []struct {
		name string
		kbs  []*entity.Knowledgebase
		want string
	}{
		{"nil", nil, ""},
		{"empty", []*entity.Knowledgebase{}, ""},
		{
			"filters nil and unnamed",
			[]*entity.Knowledgebase{nil, {ID: "kb1"}, {ID: "kb2", Name: "三国演义"}},
			"三国演义",
		},
		{
			"joins multiple names",
			[]*entity.Knowledgebase{{Name: "三国演义"}, {Name: "三国志"}},
			"三国演义, 三国志",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := harnessBoundDatasetNames(c.kbs); got != c.want {
				t.Fatalf("harnessBoundDatasetNames(%v) = %q, want %q", c.kbs, got, c.want)
			}
		})
	}
}

// TestHarnessSystemPromptFirstTurnNonEmpty pins the end-to-end first-turn
// render: with datasets bound, the dialog system prompt's {knowledge}
// placeholder (backtick-wrapped in the web UI default) must carry the bound
// dataset names — never an empty “ binding.
func TestHarnessSystemPromptFirstTurnNonEmpty(t *testing.T) {
	// The web UI default template (web/src/locales en.ts systemInitialValue):
	// the placeholder is wrapped in backticks.
	template := "You are an intelligent assistant. Your primary function is to answer questions based strictly on the provided knowledge base.\n\n**Essential Rules:**\n  - Your answer must be derived **solely** from this dataset: `{knowledge}`.\n"

	s := &ChatPipelineService{}
	kws := map[string]interface{}{
		"date": "2026-09-14 06:00:00",
	}
	if _, ok := kws["knowledge"]; !ok {
		kws["knowledge"] = harnessBoundDatasetNames([]*entity.Knowledgebase{
			{ID: "kb1", Name: "三国演义"},
		})
	}
	rendered := s.formatPrompt(template, kws)

	if strings.Contains(rendered, "``") {
		t.Fatalf("first-turn system prompt rendered an empty dataset binding:\n%s", rendered)
	}
	if !strings.Contains(rendered, "三国演义") {
		t.Fatalf("first-turn system prompt does not carry the bound dataset name:\n%s", rendered)
	}
	if strings.Contains(rendered, "{knowledge}") {
		t.Fatalf("{knowledge} placeholder was not substituted:\n%s", rendered)
	}
}

// TestHarnessSystemPromptCallerSuppliedKnowledgeWins: a caller-supplied
// knowledge value must not be overwritten by the dataset-name default.
func TestHarnessSystemPromptCallerSuppliedKnowledgeWins(t *testing.T) {
	s := &ChatPipelineService{}
	kws := map[string]interface{}{"knowledge": "caller evidence"}
	if _, ok := kws["knowledge"]; !ok {
		kws["knowledge"] = harnessBoundDatasetNames([]*entity.Knowledgebase{{Name: "三国演义"}})
	}
	rendered := s.formatPrompt("Context: {knowledge}", kws)
	if !strings.Contains(rendered, "caller evidence") {
		t.Fatalf("caller-supplied knowledge was overwritten:\n%s", rendered)
	}
	if strings.Contains(rendered, "三国演义") {
		t.Fatalf("dataset-name default leaked over the caller-supplied value:\n%s", rendered)
	}
}
