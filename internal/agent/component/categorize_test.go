// Package component — Categorize unit tests.
package component

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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
	if got, want := out["category_name"], "support"; got != want {
		t.Errorf("category_name=%v, want %v", got, want)
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

func TestCategorize_SplitsCompositeLLMIDIntoDriverAndModel(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "support", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "Qwen/Qwen3-8B@default@SILICONFLOW",
		Categories:      []string{"sales", "support"},
		DefaultCategory: "support",
	})
	_, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker not called")
	}
	if stub.captured.Driver != "SILICONFLOW" {
		t.Fatalf("Driver=%q, want %q", stub.captured.Driver, "SILICONFLOW")
	}
	if stub.captured.ModelName != "Qwen/Qwen3-8B" {
		t.Fatalf("ModelName=%q, want %q", stub.captured.ModelName, "Qwen/Qwen3-8B")
	}
}

func TestSplitCompositeLLMID(t *testing.T) {
	cases := []struct {
		in        string
		wantModel string
		wantDrv   string
		wantOK    bool
	}{
		{"gpt-4o", "gpt-4o", "", false},
		{"gpt-4o@OpenAI", "gpt-4o", "OpenAI", true},
		{"Qwen/Qwen3-8B@default@SILICONFLOW", "Qwen/Qwen3-8B", "SILICONFLOW", true},
	}
	for _, tc := range cases {
		gotModel, gotDrv, gotOK := splitCompositeLLMID(tc.in)
		if gotModel != tc.wantModel || gotDrv != tc.wantDrv || gotOK != tc.wantOK {
			t.Fatalf("splitCompositeLLMID(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, gotModel, gotDrv, gotOK, tc.wantModel, tc.wantDrv, tc.wantOK)
		}
	}
}

func TestCategorize_ResolvesTenantLLMCredentials(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)
	apiKey := "tenant-llm-key"
	apiBase := "https://tenant-llm.example"
	modelName := "Qwen/Qwen3-8B"
	if err := db.Create(&entity.TenantLLM{
		TenantID:   "tenant-1",
		LLMFactory: "SILICONFLOW",
		LLMName:    &modelName,
		APIKey:     &apiKey,
		APIBase:    &apiBase,
		Status:     "1",
	}).Error; err != nil {
		t.Fatalf("create tenant_llm: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "support", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "Qwen/Qwen3-8B@default@SILICONFLOW",
		Categories:      []string{"sales", "support"},
		DefaultCategory: "support",
	})
	_, err := c.Invoke(stateWithTenant("tenant-1"), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker not called")
	}
	if stub.captured.APIKey != apiKey {
		t.Fatalf("APIKey=%q, want %q", stub.captured.APIKey, apiKey)
	}
	if stub.captured.BaseURL != apiBase {
		t.Fatalf("BaseURL=%q, want %q", stub.captured.BaseURL, apiBase)
	}
}

func TestCategorize_ResolvesTenantModelInstanceCredentials(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)
	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-1",
		TenantID:     "tenant-1",
		ProviderName: "SILICONFLOW",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		ProviderID:   "provider-1",
		InstanceName: "default",
		APIKey:       "instance-key",
		Status:       "active",
		Extra:        `{"base_url":"https://instance.example"}`,
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "support", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "Qwen/Qwen3-8B@default@SILICONFLOW",
		Categories:      []string{"sales", "support"},
		DefaultCategory: "support",
	})
	_, err := c.Invoke(stateWithTenant("tenant-1"), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker not called")
	}
	if stub.captured.APIKey != "instance-key" {
		t.Fatalf("APIKey=%q, want %q", stub.captured.APIKey, "instance-key")
	}
	if stub.captured.BaseURL != "https://instance.example" {
		t.Fatalf("BaseURL=%q, want %q", stub.captured.BaseURL, "https://instance.example")
	}
}

func TestCategorize_ResolvesSoleActiveInstanceWhenDefaultMissing(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)
	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-1",
		TenantID:     "tenant-1",
		ProviderName: "SILICONFLOW",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		ProviderID:   "provider-1",
		InstanceName: "prod-east",
		APIKey:       "instance-key",
		Status:       "active",
		Extra:        `{"base_url":"https://instance.example"}`,
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "support", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "Qwen/Qwen3-8B@default@SILICONFLOW",
		Categories:      []string{"sales", "support"},
		DefaultCategory: "support",
	})
	_, err := c.Invoke(stateWithTenant("tenant-1"), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker not called")
	}
	if stub.captured.APIKey != "instance-key" {
		t.Fatalf("APIKey=%q, want %q", stub.captured.APIKey, "instance-key")
	}
	if stub.captured.BaseURL != "https://instance.example" {
		t.Fatalf("BaseURL=%q, want %q", stub.captured.BaseURL, "https://instance.example")
	}
}

func TestCategorize_RoutesToSelectedCategoryHandle(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "Retrieval", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "stub",
		Categories:      []string{"打招呼", "Retrieval", "Other"},
		CategoryRoutes:  map[string]string{"打招呼": "a111", "Retrieval": "b222", "Other": "c333"},
		DefaultCategory: "Other",
	})
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	next, ok := out["_next"].([]string)
	if !ok {
		t.Fatalf("_next missing or wrong type: %T", out["_next"])
	}
	if len(next) != 1 || next[0] != "b222" {
		t.Fatalf("_next=%v, want [\"b222\"]", next)
	}
}

func TestCategorize_RoutesFromCategoryDescriptionToList(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "Retrieval", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{ModelID: "stub"})
	out, err := c.Invoke(context.Background(), map[string]any{
		"category_description": map[string]any{
			"打招呼":       map[string]any{"description": "hello", "to": []any{"Message:CateLoop"}},
			"Retrieval": map[string]any{"description": "rag", "to": []any{"Message:CateRetrieval"}},
			"Other":     map[string]any{"description": "other", "to": []any{"Message:CateOther"}},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	next, ok := out["_next"].([]string)
	if !ok {
		t.Fatalf("_next missing or wrong type: %T", out["_next"])
	}
	if len(next) != 1 || next[0] != "Message:CateRetrieval" {
		t.Fatalf("_next=%v, want [\"Message:CateRetrieval\"]", next)
	}
}

func stateWithTenant(tenantID string) context.Context {
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = tenantID
	return canvas.WithState(context.Background(), state)
}

func setupComponentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.TenantLLM{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func pushComponentDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
}
