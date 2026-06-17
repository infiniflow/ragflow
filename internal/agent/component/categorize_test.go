// Package component — Categorize unit tests.
package component

import (
	"context"
	"strings"
	"testing"
)

func TestCategorize_ChosenCategory(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "support", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "stub",
		Categories:      []string{"sales", "support", "billing"},
		DefaultCategory: "support",
	})
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["category"], "support"; got != want {
		t.Errorf("category=%v, want %v", got, want)
	}
	scores, ok := out["scores"].(map[string]float64)
	if !ok {
		t.Fatalf("scores missing or wrong type: %T", out["scores"])
	}
	if scores["support"] != 1 {
		t.Errorf("support score=%v, want 1", scores["support"])
	}
	if scores["sales"] != 0 || scores["billing"] != 0 {
		t.Errorf("non-chosen categories should score 0; got %v", scores)
	}
	next, ok := out["_next"].([]string)
	if !ok {
		t.Fatalf("_next missing or wrong type: %T", out["_next"])
	}
	if len(next) != 0 {
		t.Errorf("_next=%v, want [] (placeholder; MultiBranch wires the actual routing)", next)
	}
}

func TestCategorize_FallbackToDefault(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "totally not in the list", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "stub",
		Categories:      []string{"a", "b", "c"},
		DefaultCategory: "b",
	})
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["category"], "b"; got != want {
		t.Errorf("category=%v, want %v (default fallback)", got, want)
	}
}

func TestCategorize_DefaultDefaultsToFirstCategory(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "garbage", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:    "stub",
		Categories: []string{"alpha", "beta", "gamma"},
		// no default_category
	})
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["category"], "alpha"; got != want {
		t.Errorf("category=%v, want %v (auto-default to first)", got, want)
	}
}

func TestCategorize_CaseInsensitive(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "SUPPORT", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "stub",
		Categories:      []string{"sales", "support", "billing"},
		DefaultCategory: "sales",
	})
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["category"], "support"; got != want {
		t.Errorf("category=%v, want %v (case-insensitive match)", got, want)
	}
}

func TestCategorize_PromptListsCategories(t *testing.T) {
	// Verify the prompt passed to the invoker includes the categories
	// so a model choosing between A and B has the context to do so.
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "x", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "stub",
		Categories:      []string{"x", "y", "z"},
		DefaultCategory: "x",
		Items:           []string{"foo", "bar"},
	})
	_, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker not called")
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == "user" {
			userContent = m.Content
		}
	}
	if userContent == "" {
		t.Fatal("no user message in captured invoker request")
	}
	for _, want := range []string{"x", "y", "z", "foo", "bar"} {
		if !strings.Contains(userContent, want) {
			t.Errorf("prompt missing %q; got: %s", want, userContent)
		}
	}
}

func TestCategorize_Registered(t *testing.T) {
	c, err := New("Categorize", map[string]any{
		"model_id":         "stub",
		"categories":       []any{"a", "b"},
		"default_category": "a",
	})
	if err != nil {
		t.Fatalf("New(Categorize): %v", err)
	}
	if c.Name() != "Categorize" {
		t.Errorf("Name()=%q, want Categorize", c.Name())
	}
}
