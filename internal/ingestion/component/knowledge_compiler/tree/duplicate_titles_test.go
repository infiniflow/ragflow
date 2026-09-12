package tree

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// renameChat replies with a canned rename JSON for any prompt.
type renameChat struct {
	response string
	prompts  []string
}

func (f *renameChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	f.prompts = append(f.prompts, req.UserPrompt)
	return &common.ChatResponse{Content: f.response}, nil
}

func summaryProduct(title, body string) common.Product {
	content := title
	if body != "" {
		content = title + "\n" + body
	}
	return common.Product{
		ID:      "node-" + title,
		Content: content,
		Meta:    map[string]any{"title": title, "kind": "summary", "level": 0},
	}
}

// Group with two members and two distinct descriptions: LLM renames both.
func TestRewriteDuplicatesLLMRenames(t *testing.T) {
	chat := &renameChat{response: `[{"id":"0","name":"Alpha Plan"},{"id":"1","name":"Alpha Report"}]`}
	products := []common.Product{
		summaryProduct("Alpha", "quarterly planning notes"),
		summaryProduct("Beta", "unrelated"),
		summaryProduct("Alpha", "financial report summary"),
	}
	rewriteDuplicateTreeNames(context.Background(), common.Deps{Chat: chat}, "llm-1", products)

	if len(chat.prompts) != 1 {
		t.Fatalf("LLM calls = %d, want 1 (only the Alpha group qualifies)", len(chat.prompts))
	}
	if got := products[0].Meta["title"]; got != "Alpha Plan" {
		t.Errorf("products[0] title = %v, want Alpha Plan", got)
	}
	if got := products[2].Meta["title"]; got != "Alpha Report" {
		t.Errorf("products[2] title = %v, want Alpha Report", got)
	}
	// Content first line must track the meta title.
	if !strings.HasPrefix(products[0].Content, "Alpha Plan\n") {
		t.Errorf("products[0] content = %q, want first line replaced", products[0].Content)
	}
	if products[1].Meta["title"] != "Beta" {
		t.Errorf("untouched product title changed: %v", products[1].Meta["title"])
	}
}

// Same title but identical descriptions: no LLM call, deterministic suffix only.
func TestRewriteDuplicatesIdenticalDescriptionsNoLLM(t *testing.T) {
	chat := &renameChat{response: `[]`}
	products := []common.Product{
		summaryProduct("Alpha", "same body"),
		summaryProduct("Alpha", "same body"),
	}
	rewriteDuplicateTreeNames(context.Background(), common.Deps{Chat: chat}, "llm-1", products)

	if len(chat.prompts) != 0 {
		t.Fatalf("LLM calls = %d, want 0 (descriptions identical)", len(chat.prompts))
	}
	if got := products[0].Meta["title"]; got != "Alpha" {
		t.Errorf("first occurrence title = %v, want Alpha", got)
	}
	if got := products[1].Meta["title"]; got != "Alpha (2)" {
		t.Errorf("second occurrence title = %v, want Alpha (2)", got)
	}
}

// LLM returns colliding names: the deterministic pass must still make them unique.
func TestRewriteDuplicatesDeterministicSuffixAfterCollision(t *testing.T) {
	chat := &renameChat{response: `[{"id":"0","name":"Gamma"},{"id":"1","name":"Gamma"}]`}
	products := []common.Product{
		summaryProduct("Alpha", "body one"),
		summaryProduct("Alpha", "body two"),
	}
	rewriteDuplicateTreeNames(context.Background(), common.Deps{Chat: chat}, "llm-1", products)

	titles := map[string]bool{}
	for _, p := range products {
		titles[p.Meta["title"].(string)] = true
	}
	if len(titles) != 2 {
		t.Errorf("titles after rewrite = %v, want 2 distinct", titles)
	}
}

// LLM failure: titles survive via the deterministic suffix, no error escapes.
func TestRewriteDuplicatesLLMFailureBestEffort(t *testing.T) {
	chat := &failingRenameChat{}
	products := []common.Product{
		summaryProduct("Alpha", "body one"),
		summaryProduct("Alpha", "body two"),
	}
	rewriteDuplicateTreeNames(context.Background(), common.Deps{Chat: chat}, "llm-1", products)

	if products[0].Meta["title"] != "Alpha" || products[1].Meta["title"] != "Alpha (2)" {
		t.Errorf("titles after failure = %v / %v, want Alpha / Alpha (2)",
			products[0].Meta["title"], products[1].Meta["title"])
	}
}

type failingRenameChat struct{}

func (f *failingRenameChat) Chat(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
	return nil, fmt.Errorf("llm down")
}

// Fenced JSON and a hallucinated id are tolerated.
func TestRewriteDuplicatesFencedJSONAndBadID(t *testing.T) {
	chat := &renameChat{response: "```json\n[{\"id\":\"1\",\"name\":\"Alpha Two\"},{\"id\":\"99\",\"name\":\"Hijack\"}]\n```"}
	products := []common.Product{
		summaryProduct("Alpha", "body one"),
		summaryProduct("Alpha", "body two"),
	}
	rewriteDuplicateTreeNames(context.Background(), common.Deps{Chat: chat}, "llm-1", products)

	if products[1].Meta["title"] != "Alpha Two" {
		t.Errorf("fenced rename not applied: %v", products[1].Meta["title"])
	}
	if products[0].Meta["title"] != "Alpha" {
		t.Errorf("first product should keep its title (bad id): %v", products[0].Meta["title"])
	}
}

// No duplicates at all: no LLM call, no changes.
func TestRewriteDuplicatesNoop(t *testing.T) {
	chat := &renameChat{response: `[]`}
	products := []common.Product{
		summaryProduct("Alpha", "one"),
		summaryProduct("Beta", "two"),
	}
	rewriteDuplicateTreeNames(context.Background(), common.Deps{Chat: chat}, "llm-1", products)

	if len(chat.prompts) != 0 {
		t.Fatalf("LLM calls = %d, want 0", len(chat.prompts))
	}
	if products[0].Meta["title"] != "Alpha" || products[1].Meta["title"] != "Beta" {
		t.Errorf("titles changed on noop: %v / %v", products[0].Meta["title"], products[1].Meta["title"])
	}
}
