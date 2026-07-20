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
	var systemContent string
	for _, m := range stub.captured.Messages {
		if m.Role == "system" {
			systemContent = m.Content
		}
	}
	if systemContent == "" {
		t.Fatal("no system message in captured invoker request")
	}
	for _, want := range []string{"x", "y", "z", "foo", "bar"} {
		if !strings.Contains(systemContent, want) {
			t.Errorf("prompt missing %q; got: %s", want, systemContent)
		}
	}
}

func TestCategorize_PromptIncludesRuntimeQuery(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "English", Model: "stub"}}
	withStubInvoker(t, stub)

	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["query"] = "he who desires but acts not"
	ctx := canvas.WithState(context.Background(), state)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:    "stub",
		Query:      "sys.query",
		Categories: []string{"Number", "chinese", "English"},
		CategoryDescriptions: map[string]string{
			"Number":  "This query has only a number",
			"chinese": "this query only has chinese",
			"English": "this query has english letter",
		},
	})
	_, err := c.Invoke(ctx, map[string]any{})
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
	if !strings.Contains(userContent, "he who desires but acts not") {
		t.Fatalf("user prompt = %q, want runtime query", userContent)
	}
	if strings.Contains(userContent, "Number") || strings.Contains(userContent, "chinese") {
		t.Fatalf("user prompt should carry real data only, got %q", userContent)
	}
}

func TestCategorize_PromptUsesInputQueryValue(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "chinese", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:    "stub",
		Query:      "sys.query",
		Categories: []string{"Number", "chinese", "English"},
	})
	_, err := c.Invoke(context.Background(), map[string]any{"query": "测试"})
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
	if !strings.Contains(userContent, "测试") {
		t.Fatalf("user prompt = %q, want input query value", userContent)
	}
}

func TestCategorize_HistoryWindowRealData(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "English", Model: "stub"}}
	withStubInvoker(t, stub)

	state := canvas.NewCanvasState("run-1", "task-1")
	state.History = []map[string]any{
		{"role": "user", "content": "old user"},
		{"role": "assistant", "content": "prior answer"},
		{"role": "user", "content": "stale latest"},
	}
	ctx := canvas.WithState(context.Background(), state)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:                  "stub",
		Query:                    "sys.query",
		Categories:               []string{"Number", "chinese", "English"},
		MessageHistoryWindowSize: 2,
	})
	_, err := c.Invoke(ctx, map[string]any{"query": "current question"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == "user" {
			userContent = m.Content
		}
	}
	if !strings.Contains(userContent, `ASSISTANT: "prior answer" | USER: "current question"`) {
		t.Fatalf("user prompt = %q, want bounded history plus current query", userContent)
	}
	if strings.Contains(userContent, "old user") || strings.Contains(userContent, "stale latest") {
		t.Fatalf("user prompt should truncate old history and overwrite latest content, got %q", userContent)
	}
}

func TestCategorizeRegistered_DefaultHistoryWindow(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "English", Model: "stub"}}
	withStubInvoker(t, stub)

	state := canvas.NewCanvasState("run-1", "task-1")
	state.History = []map[string]any{
		{"role": "user", "content": "old user"},
		{"role": "assistant", "content": "stale latest"},
	}
	ctx := canvas.WithState(context.Background(), state)

	c, err := New("Categorize", map[string]any{
		"model_id":   "stub",
		"query":      "sys.query",
		"categories": []any{"Number", "chinese", "English"},
	})
	if err != nil {
		t.Fatalf("New(Categorize): %v", err)
	}
	_, err = c.Invoke(ctx, map[string]any{"query": "current question"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == "user" {
			userContent = m.Content
		}
	}
	if !strings.Contains(userContent, `ASSISTANT: "current question"`) {
		t.Fatalf("user prompt = %q, want default one-message history with current query", userContent)
	}
	if strings.Contains(userContent, "old user") || strings.Contains(userContent, "stale latest") {
		t.Fatalf("user prompt should only include default one-message window with overwritten content, got %q", userContent)
	}
}

func TestCategorize_HistoryWindowRejectsNegative(t *testing.T) {
	c := NewCategorizeComponent(CategorizeParam{
		ModelID:                  "stub",
		Categories:               []string{"Number", "chinese", "English"},
		MessageHistoryWindowSize: -1,
	})
	_, err := c.Invoke(context.Background(), map[string]any{"query": "current question"})
	if err == nil {
		t.Fatal("expected negative message_history_window_size error")
	}
	if !strings.Contains(err.Error(), "message_history_window_size") {
		t.Fatalf("error = %v, want message_history_window_size", err)
	}
}

func TestCategorize_OtherInstructionOnlyWhenAllowed(t *testing.T) {
	withoutOther := buildCategorizeSystemPrompt(CategorizeParam{
		Categories: []string{"Number", "Chinese", "English"},
	})
	if strings.Contains(withoutOther, `Use "Other"`) {
		t.Fatalf("prompt should not mention Other when category is absent: %s", withoutOther)
	}

	withOther := buildCategorizeSystemPrompt(CategorizeParam{
		Categories: []string{"Number", "Other"},
	})
	if !strings.Contains(withOther, `Use "Other" only when no other category fits`) {
		t.Fatalf("prompt should mention Other when category exists: %s", withOther)
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

func TestCategorize_ResolvesTenantModelID(t *testing.T) {
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
	if err := db.Create(&entity.TenantModel{
		ID:         "3d2d824e7e5d11f1a845455b140cef90",
		ProviderID: "provider-1",
		InstanceID: "instance-1",
		ModelName:  "Qwen/Qwen3-8B",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "support", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{
		ModelID:         "3d2d824e7e5d11f1a845455b140cef90",
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
	if stub.captured.Driver != "SILICONFLOW" {
		t.Fatalf("Driver=%q, want SILICONFLOW", stub.captured.Driver)
	}
	if stub.captured.ModelName != "Qwen/Qwen3-8B" {
		t.Fatalf("ModelName=%q, want Qwen/Qwen3-8B", stub.captured.ModelName)
	}
	if stub.captured.APIKey != "instance-key" {
		t.Fatalf("APIKey=%q, want instance-key", stub.captured.APIKey)
	}
	if stub.captured.BaseURL != "https://instance.example" {
		t.Fatalf("BaseURL=%q, want https://instance.example", stub.captured.BaseURL)
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

func TestCategorize_ExplicitCategoriesKeepCategoryDescriptionMetadata(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "English", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewCategorizeComponent(CategorizeParam{ModelID: "stub"})
	out, err := c.Invoke(context.Background(), map[string]any{
		"categories": []any{"Number", "chinese", "English"},
		"category_description": map[string]any{
			"Number":  map[string]any{"description": "This query has only a number", "examples": []any{"4321"}, "to": []any{"Message:Number"}},
			"chinese": map[string]any{"description": "this query only has chinese", "examples": []any{"测试"}, "to": []any{"Message:Chinese"}},
			"English": map[string]any{"description": "this query has english letter", "examples": []any{"hello"}, "to": []any{"Message:English"}},
		},
		"query": "hello",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	next, ok := out["_next"].([]string)
	if !ok {
		t.Fatalf("_next missing or wrong type: %T", out["_next"])
	}
	if len(next) != 1 || next[0] != "Message:English" {
		t.Fatalf("_next=%v, want [\"Message:English\"]", next)
	}
	var systemContent string
	for _, m := range stub.captured.Messages {
		if m.Role == "system" {
			systemContent = m.Content
		}
	}
	for _, want := range []string{"this query has english letter", `USER: "hello" -> English`} {
		if !strings.Contains(systemContent, want) {
			t.Fatalf("system prompt missing %q; got %s", want, systemContent)
		}
	}
}

func TestCategorizeRegistered_ExplicitCategoriesKeepCategoryDescriptionMetadata(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "English", Model: "stub"}}
	withStubInvoker(t, stub)

	c, err := New("Categorize", map[string]any{
		"model_id":   "stub",
		"categories": []any{"Number", "chinese", "English"},
		"category_description": map[string]any{
			"Number":  map[string]any{"description": "This query has only a number", "examples": []any{"4321"}, "to": []any{"Message:Number"}},
			"chinese": map[string]any{"description": "this query only has chinese", "examples": []any{"测试"}, "to": []any{"Message:Chinese"}},
			"English": map[string]any{"description": "this query has english letter", "examples": []any{"hello"}, "to": []any{"Message:English"}},
		},
	})
	if err != nil {
		t.Fatalf("New(Categorize): %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{"query": "hello"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	next, ok := out["_next"].([]string)
	if !ok {
		t.Fatalf("_next missing or wrong type: %T", out["_next"])
	}
	if len(next) != 1 || next[0] != "Message:English" {
		t.Fatalf("_next=%v, want [\"Message:English\"]", next)
	}
	var systemContent string
	for _, m := range stub.captured.Messages {
		if m.Role == "system" {
			systemContent = m.Content
		}
	}
	for _, want := range []string{"this query has english letter", `USER: "hello" -> English`} {
		if !strings.Contains(systemContent, want) {
			t.Fatalf("system prompt missing %q; got %s", want, systemContent)
		}
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
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
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
