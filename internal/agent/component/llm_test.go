// Package component — LLM unit tests.
//
// Tests use a stub ChatInvoker to avoid the network. The production path
// flows through einoChatInvoker + models.NewEinoChatModel + the real
// provider driver; here we focus on the component contract:
//   - inputs → outputs map shape
//   - json_output parsing
//   - Stream variant emits the same payload + closes
//   - error path surfaces invoker errors
//   - variable reference substitution is the canvas engine's job, not
//     this component's — we only verify the raw user_prompt is passed
//     through to the invoker.
package component

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// stubInvoker is a programmable ChatInvoker used by these tests.
type stubInvoker struct {
	resp     *ChatInvokeResponse
	err      error
	captured *ChatInvokeRequest
	calls    int
}

func (s *stubInvoker) Invoke(_ context.Context, _ *gorm.DB, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	s.calls++
	cp := req
	s.captured = &cp
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

// withStubInvoker swaps the package-level ChatInvoker for the duration of t.
func withStubInvoker(t *testing.T, s ChatInvoker) {
	t.Helper()
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(s)
	t.Cleanup(func() { SetDefaultChatInvoker(prev) })
}

func TestLLM_Invoke_HappyPath(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "hello", Model: "echo-model", Stopped: true, Tokens: 7}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo-model"})
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"user_prompt": "hi",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "hello"; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	if got, want := out["model"], "echo-model"; got != want {
		t.Errorf("model=%v, want %v", got, want)
	}
	if got, want := out["stopped"], true; got != want {
		t.Errorf("stopped=%v, want %v", got, want)
	}
	if stub.calls != 1 {
		t.Errorf("invoker calls=%d, want 1", stub.calls)
	}
	if stub.captured == nil || stub.captured.ModelName != "echo-model" {
		t.Errorf("ModelName not propagated: %+v", stub.captured)
	}
	if len(stub.captured.Messages) != 1 || stub.captured.Messages[0].Role != schema.User || stub.captured.Messages[0].Content != "hi" {
		t.Errorf("messages not built correctly: %+v", stub.captured.Messages)
	}
}

func TestLLM_Invoke_JSONOutput(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: `{"k":"v"}`, Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"user_prompt": "give me json",
		"json_output": true,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], `{"k":"v"}`; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	parsed, ok := out["json"].(map[string]any)
	if !ok {
		t.Fatalf("json output missing or wrong type: %T", out["json"])
	}
	if parsed["k"] != "v" {
		t.Errorf("json[k]=%v, want v", parsed["k"])
	}
}

func TestLLM_Invoke_SystemAndUser(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"system_prompt": "you are helpful",
		"user_prompt":   "say hi",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := len(stub.captured.Messages); got != 2 {
		t.Fatalf("messages=%d, want 2", got)
	}
	if stub.captured.Messages[0].Role != schema.System || stub.captured.Messages[0].Content != "you are helpful" {
		t.Errorf("system msg wrong: %+v", stub.captured.Messages[0])
	}
	if stub.captured.Messages[1].Role != schema.User || stub.captured.Messages[1].Content != "say hi" {
		t.Errorf("user msg wrong: %+v", stub.captured.Messages[1])
	}
}

func TestLLM_Stream(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "streamed", Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(t.Context(), nil, map[string]any{"user_prompt": "go"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Drain all chunks; the implementation emits content + done
	// over the goroutine-streaming pattern.
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks (content + done), got %d", len(got))
	}
	if got[0]["content"] != "streamed" {
		t.Errorf("chunk[0].content=%v, want 'streamed'", got[0]["content"])
	}
	if got[1]["done"] != true {
		t.Errorf("chunk[1].done=%v, want true", got[1]["done"])
	}
}

func TestLLM_Invoke_MissingModelID(t *testing.T) {
	withStubInvoker(t, &stubInvoker{resp: &ChatInvokeResponse{Content: "should not be called"}})
	c := NewLLMComponent(LLMParam{}) // no model_id
	_, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected ParamError for missing model_id")
	}
	var pe *ParamError
	if !errors.As(err, &pe) {
		t.Errorf("err type=%T, want *ParamError", err)
	}
}

func TestLLM_Invoke_InvokerError(t *testing.T) {
	stub := &stubInvoker{err: errors.New("upstream blew up")}
	withStubInvoker(t, stub)
	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if stub.calls != 1 {
		t.Errorf("calls=%d, want 1", stub.calls)
	}
}

func TestLLM_Registered(t *testing.T) {
	names := RegisteredNames()
	if !slices.Contains(names, "llm") {
		t.Fatalf("LLM not registered; names=%v", names)
	}
	// And a factory round-trip.
	c, err := New("LLM", map[string]any{"model_id": "echo"})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	if c.Name() != "LLM" {
		t.Errorf("Name()=%q, want LLM", c.Name())
	}
}

// TestLLM_ThinkingFieldRoundTrip guards the agent-component
// portion of PR #15446 (thinking switch) and PR #16640 (gen_conf
// forwarding). The agent component accepts `thinking` from the DSL
// params (any non-empty, non-"default" value) and threads it through
// LLMParam and the ChatInvokeRequest. Downstream (einoChatInvoker)
// only acts on "enabled" / "disabled" and silently ignores other
// values, so lenient forwarding is safe.
func TestLLM_ThinkingFieldRoundTrip(t *testing.T) {
	t.Parallel()

	// Case 1: "enabled" round-trips into LLMParam and ChatInvokeRequest.
	enabled := mergeLLMParam(LLMParam{}, map[string]any{
		"thinking":      "enabled",
		"model_id":      "qwen3-max",
		"system_prompt": "s",
		"user_prompt":   "u",
	})
	if enabled.Thinking != "enabled" {
		t.Errorf("Thinking = %q, want enabled", enabled.Thinking)
	}

	// Case 2: "disabled" also round-trips.
	disabled := mergeLLMParam(LLMParam{}, map[string]any{
		"thinking":    "disabled",
		"model_id":    "kimi-k2.6",
		"user_prompt": "u",
	})
	if disabled.Thinking != "disabled" {
		t.Errorf("Thinking = %q, want disabled", disabled.Thinking)
	}

	// Case 3: empty / missing value → empty (system default).
	defaulted := mergeLLMParam(LLMParam{}, map[string]any{
		"model_id":    "glm-4.6",
		"user_prompt": "u",
	})
	if defaulted.Thinking != "" {
		t.Errorf("Thinking = %q, want empty (system default)", defaulted.Thinking)
	}

	// Case 4: "default" is explicitly rejected, matching Python's
	// `self.thinking != "default"` gate in gen_conf().
	defaultStr := mergeLLMParam(LLMParam{}, map[string]any{
		"thinking":    "default",
		"model_id":    "glm-4.6",
		"user_prompt": "u",
	})
	if defaultStr.Thinking != "" {
		t.Errorf(`Thinking = %q, want empty ("default" rejected)`, defaultStr.Thinking)
	}

	// Case 5: arbitrary / unknown values are leniently forwarded
	// (matches Python gen_conf() which passes through any truthy
	// non-"default" string). Downstream einoChatInvoker ignores
	// unknown values, so this is safe.
	arbitrary := mergeLLMParam(LLMParam{}, map[string]any{
		"thinking":    "auto",
		"model_id":    "glm-4.6",
		"user_prompt": "u",
	})
	if arbitrary.Thinking != "auto" {
		t.Errorf("arbitrary thinking = %q, want auto (lenient forwarding)", arbitrary.Thinking)
	}
}

// TestLLM_Invoke_CompositeModel_CustomContextOverride verifies the composite
// reference path of the tenant-configured override: a 2000-token extra
// max_tokens on the tenant's gpt-4o row drives trimming even though the
// catalog reports 128k.
func TestLLM_Invoke_CompositeModel_CustomContextOverride(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)

	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-comp-1",
		TenantID:     "tenant-1",
		ProviderName: "OpenAI",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-comp-1",
		ProviderID:   "provider-comp-1",
		InstanceName: "default",
		APIKey:       "test-key",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: "provider-comp-1",
		InstanceID: "instance-comp-1",
		ModelName:  "gpt-4o",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
		Extra:      `{"max_tokens": 2000}`,
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "stub"}}
	withStubInvoker(t, stub)

	bigPrompt := strings.Repeat("x ", 20000) // ~40k tokens
	c := NewLLMComponent(LLMParam{ModelID: "gpt-4o@OpenAI"})
	if _, err := c.Invoke(stateWithTenant("tenant-1"), db, map[string]any{"user_prompt": bigPrompt}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker was not called")
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == schema.User {
			userContent = m.Content
		}
	}
	if userContent == "" {
		t.Fatal("no user message captured")
	}
	if got := tokenizer.NumTokensFromString(userContent); got > 2000 || got < 1000 {
		t.Fatalf("user message = %d tokens; want trimmed to the custom 2000-token context window (~1940)", got)
	}
}

// TestLLM_Invoke_UUIDModel_CustomContextOverride verifies end to end that a
// tenant-configured "max_tokens" override in tenant_model.extra wins over the
// provider catalog's content_length: with an override of 2000 and a 40k-token
// prompt, the user message must be trimmed to roughly the override budget, not
// preserved under gpt-4o's 128k catalog window.
func TestLLM_Invoke_UUIDModel_CustomContextOverride(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)

	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-uuid-2",
		TenantID:     "tenant-1",
		ProviderName: "OpenAI",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-uuid-2",
		ProviderID:   "provider-uuid-2",
		InstanceName: "default",
		APIKey:       "test-key",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: "provider-uuid-2",
		InstanceID: "instance-uuid-2",
		ModelName:  "gpt-4o",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
		Extra:      `{"max_tokens": 2000}`,
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "stub"}}
	withStubInvoker(t, stub)

	bigPrompt := strings.Repeat("x ", 20000) // ~40k tokens
	c := NewLLMComponent(LLMParam{ModelID: "0123456789abcdef0123456789abcdef"})
	if _, err := c.Invoke(stateWithTenant("tenant-1"), db, map[string]any{"user_prompt": bigPrompt}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker was not called")
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == schema.User {
			userContent = m.Content
		}
	}
	if userContent == "" {
		t.Fatal("no user message captured")
	}
	// 97% of the 2000-token override budget; the catalog's 128k must not apply.
	if got := tokenizer.NumTokensFromString(userContent); got > 2000 || got < 1000 {
		t.Fatalf("user message = %d tokens; want trimmed to the custom 2000-token context window (~1940)", got)
	}
}

// TestLLM_Invoke_UUIDModel_ResolvesContentLength verifies the tenant-model
// UUID path of content_length resolution end to end: with a real in-memory
// DB row for gpt-4o@OpenAI, the fitting budget comes from the catalog's
// content_length (128000) rather than the 8192 fallback, so a 40k-token
// prompt survives.
func TestLLM_Invoke_UUIDModel_ResolvesContentLength(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)

	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-uuid-1",
		TenantID:     "tenant-1",
		ProviderName: "OpenAI",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-uuid-1",
		ProviderID:   "provider-uuid-1",
		InstanceName: "default",
		APIKey:       "test-key",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: "provider-uuid-1",
		InstanceID: "instance-uuid-1",
		ModelName:  "gpt-4o",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "stub"}}
	withStubInvoker(t, stub)

	bigPrompt := strings.Repeat("x ", 20000) // ~40k tokens
	c := NewLLMComponent(LLMParam{ModelID: "0123456789abcdef0123456789abcdef"})
	if _, err := c.Invoke(stateWithTenant("tenant-1"), db, map[string]any{"user_prompt": bigPrompt}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker was not called")
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == schema.User {
			userContent = m.Content
		}
	}
	if userContent == "" {
		t.Fatal("no user message captured")
	}
	if got := tokenizer.NumTokensFromString(userContent); got < 8000 {
		t.Fatalf("user message trimmed to %d tokens; UUID content_length resolution failed (want preserved under gpt-4o 128k)", got)
	}
}

// TestLLM_ResolvesTenantModelID guards that custom-added tenant models selected
// in the agent canvas are resolved to their real provider/model name, driver,
// and credentials before the LLM call is dispatched.
func TestLLM_ResolvesTenantModelID(t *testing.T) {
	db := setupComponentTestDB(t)
	pushComponentDB(t, db)

	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-1",
		TenantID:     "tenant-1",
		ProviderName: "DeepSeek",
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
		ModelName:  "deepseek-chat",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "stub"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "3d2d824e7e5d11f1a845455b140cef90"})
	_, err := c.Invoke(stateWithTenant("tenant-1"), db, map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker not called")
	}
	if got, want := stub.captured.Driver, "DeepSeek"; got != want {
		t.Errorf("Driver=%q, want %q", got, want)
	}
	if got, want := stub.captured.ModelName, "deepseek-chat"; got != want {
		t.Errorf("ModelName=%q, want %q", got, want)
	}
	if got, want := stub.captured.APIKey, "instance-key"; got != want {
		t.Errorf("APIKey=%q, want %q", got, want)
	}
	if got, want := stub.captured.BaseURL, "https://instance.example"; got != want {
		t.Errorf("BaseURL=%q, want %q", got, want)
	}
}

func TestFitMessages_EverythingFits(t *testing.T) {
	msgs := []schema.Message{
		{Role: schema.System, Content: "you are helpful"},
		{Role: schema.User, Content: "hello"},
	}
	fitted, fitErr := fitMessages("", msgs, 100000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2", len(fitted))
	}
	if fitted[0].Content != "you are helpful" || fitted[1].Content != "hello" {
		t.Fatalf("messages modified when they fit: %+v", fitted)
	}
}

func TestFitMessages_PreservesImageOnlyTurn(t *testing.T) {
	imgURL := "data:image/png;base64,AAAA"
	msgs := []schema.Message{
		{Role: schema.System, Content: "you are helpful"},
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &imgURL},
			}},
		}},
	}
	fitted, fitErr := fitMessages("", msgs, 100000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (image-only turn must be preserved)", len(fitted))
	}
	last := fitted[len(fitted)-1]
	if len(last.UserInputMultiContent) != 1 || last.UserInputMultiContent[0].Type != schema.ChatMessagePartTypeImageURL {
		t.Fatalf("image parts lost after fitting: %+v", last)
	}
}

func TestFitMessages_IncludesSyntheticSystemPrompt(t *testing.T) {
	msgs := []schema.Message{{Role: schema.User, Content: "hello"}}
	fitted, fitErr := fitMessages("be brief", msgs, 100000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (synthetic system prompt + user)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[0].Content != "be brief" {
		t.Fatalf("synthetic system prompt not preserved: %+v", fitted[0])
	}
}

func TestFitMessages_DropsMiddleWhenOverBudget(t *testing.T) {
	long := strings.Repeat("x ", 5000)
	msgs := []schema.Message{
		{Role: schema.System, Content: long},
		{Role: schema.User, Content: "middle"},
		{Role: schema.User, Content: "last"},
	}
	fitted, fitErr := fitMessages("", msgs, 1000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (middle dropped, system + last user kept)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[1].Role != schema.User {
		t.Fatalf("unexpected roles: %+v", fitted)
	}
	if !strings.Contains(fitted[1].Content, "last") {
		t.Fatalf("last user message not preserved: %+v", fitted[1])
	}
}

// TestFitMessages_SystemKeptButEmptied locks the write-back for a system
// message that the fitter keeps but trims to empty (the final user turn alone
// fills the budget): the fitted (empty) content must be written back instead
// of the original, so the conversation stays within the budget.
func TestFitMessages_SystemKeptButEmptied(t *testing.T) {
	origSys := strings.Repeat("s ", 3000) // dominates (>80% of tokens)
	msgs := []schema.Message{
		{Role: schema.System, Content: origSys},
		{Role: schema.User, Content: strings.Repeat("u ", 600)}, // alone exceeds the budget
	}
	fitted, fitErr := fitMessages("", msgs, 500)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (both kept)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[0].Content != "" {
		t.Fatalf("system should be kept but trimmed to empty, got %+v", fitted[0])
	}
	if fitted[1].Role != schema.User || fitted[1].Content == origSys {
		t.Fatalf("user turn wrong after fitting: %+v", fitted[1])
	}
	total := tokenizer.NumTokensFromString(fitted[0].Content) + tokenizer.NumTokensFromString(fitted[1].Content)
	if total > 500 {
		t.Fatalf("fitted total %d exceeds budget 500", total)
	}
}

// TestFitMessages_FoldsMultipleTextParts verifies that every non-empty text
// part of a multi-modal message participates in the token budget: the parts
// are folded into a single fitted text on the first text part and additional
// text parts are removed, so no text escapes the budget after reconstruction.
func TestFitMessages_FoldsMultipleTextParts(t *testing.T) {
	long1 := strings.Repeat("a ", 3000)
	long2 := strings.Repeat("b ", 3000)
	imgURL := "data:image/png;base64,AAAA"
	msgs := []schema.Message{
		{Role: schema.System, Content: "sys"},
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeText, Text: long1},
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &imgURL},
			}},
			{Type: schema.ChatMessagePartTypeText, Text: long2},
		}},
	}
	fitted, fitErr := fitMessages("", msgs, 2000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2", len(fitted))
	}
	last := fitted[len(fitted)-1]
	textParts := 0
	imageParts := 0
	for _, part := range last.UserInputMultiContent {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			textParts++
		case schema.ChatMessagePartTypeImageURL:
			imageParts++
		}
	}
	if textParts != 1 {
		t.Fatalf("got %d text parts, want 1 (folded): %+v", textParts, last.UserInputMultiContent)
	}
	if imageParts != 1 {
		t.Fatalf("image part lost after trimming: %+v", last.UserInputMultiContent)
	}
	if total := tokenizer.NumTokensFromString(last.UserInputMultiContent[0].Text); total > 2000 {
		t.Fatalf("fitted text totals %d tokens, exceeds budget 2000", total)
	}
}

// TestFitMessages_ImageOnlyLastTurnOverBudget locks the over-budget path where
// an image-only turn is the last non-system message (ll2 = 0 tokens): the
// fitter keeps it, gives the whole budget to the system messages, and the
// image-only turn must survive reconstruction untouched.
func TestFitMessages_ImageOnlyLastTurnOverBudget(t *testing.T) {
	imgURL := "data:image/png;base64,AAAA"
	msgs := []schema.Message{
		{Role: schema.System, Content: strings.Repeat("s ", 3000)}, // dominates (>80% of tokens)
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &imgURL},
			}},
		}},
	}
	fitted, fitErr := fitMessages("", msgs, 500)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (system + image-only turn)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[0].Content == msgs[0].Content {
		t.Fatalf("system should be trimmed to the budget: %+v", fitted[0])
	}
	if total := tokenizer.NumTokensFromString(fitted[0].Content); total > 500 {
		t.Fatalf("system exceeds budget after fit: %d tokens", total)
	}
	last := fitted[len(fitted)-1]
	if last.Role != schema.User || len(last.UserInputMultiContent) != 1 || last.UserInputMultiContent[0].Type != schema.ChatMessagePartTypeImageURL {
		t.Fatalf("image-only turn lost or modified after over-budget fitting: %+v", last)
	}
}

// TestLLM_Invoke_MaxTokensStillOutputCapAndNotBudget pins the core semantics
// of the content_length change: the canvas max_tokens must still reach the
// invoker as the generation cap, but must NOT be the message-fitting budget.
// A small max_tokens with a 40k-token prompt would be trimmed to ~500 tokens
// under the old behavior; the prompt must survive under the content_length
// budget.
func TestLLM_Invoke_MaxTokensStillOutputCapAndNotBudget(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	bigPrompt := strings.Repeat("x ", 20000) // ~40k tokens
	maxOut := 512
	c := NewLLMComponent(LLMParam{ModelID: "gpt-4o@openai", MaxTokens: &maxOut})
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": bigPrompt}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker was not called")
	}
	// Generation cap still flows to the invoker.
	if stub.captured.MaxTokens == nil || *stub.captured.MaxTokens != maxOut {
		t.Fatalf("MaxTokens = %v, want %d (generation cap must still be forwarded)", stub.captured.MaxTokens, maxOut)
	}
	// ...but is not the fitting budget: the 40k prompt must survive.
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == schema.User {
			userContent = m.Content
		}
	}
	if got := tokenizer.NumTokensFromString(userContent); got < 8000 {
		t.Fatalf("user message trimmed to %d tokens; max_tokens must not be the fitting budget", got)
	}
}

// TestLLM_Invoke_UnresolvableModelFallsBackTo8192 verifies the fallback: when
// content_length cannot be resolved, fitting falls back to the 8192 budget
// (matching Python's chat_mdl.max_length default) instead of panicking or
// passing the oversized prompt through.
func TestLLM_Invoke_UnresolvableModelFallsBackTo8192(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	bigPrompt := strings.Repeat("x ", 20000) // ~40k tokens
	c := NewLLMComponent(LLMParam{ModelID: "no-such-model@no-such-provider"})
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": bigPrompt}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker was not called")
	}
	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == schema.User {
			userContent = m.Content
		}
	}
	if userContent == "" {
		t.Fatal("no user message captured")
	}
	if got := tokenizer.NumTokensFromString(userContent); got >= 8000 {
		t.Fatalf("user message not trimmed under the 8192 fallback budget: %d tokens", got)
	}
}

// TestLLM_Invoke_UsesModelContentLengthBudget verifies that the message
// fitting budget in Invoke is the chat model's context window
// (content_length) resolved via dao.ResolveModelContentLength — NOT the
// canvas max_tokens / the 8192 fallback. A user prompt far larger than the
// 8192 fallback (but well inside gpt-4o@openai's 128k window) must be passed
// through to the invoker untrimmed.
//
// NOTE: this test couples to the provider catalog ("gpt-4o" must carry a
// content_length well above 8000). The >=8000 threshold is robust to catalog
// bumps; if gpt-4o's content_length were ever lowered below ~8k, the test
// failing is the correct signal.
func TestLLM_Invoke_UsesModelContentLengthBudget(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	// ~40k tokens: > 8192 (the fallback default) but << 128000 (gpt-4o).
	bigPrompt := strings.Repeat("x ", 20000)

	c := NewLLMComponent(LLMParam{ModelID: "gpt-4o@openai"})
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": bigPrompt}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker was not called")
	}

	var userContent string
	for _, m := range stub.captured.Messages {
		if m.Role == schema.User {
			userContent = m.Content
		}
	}
	if userContent == "" {
		t.Fatalf("no user message captured: %+v", stub.captured.Messages)
	}
	if got := tokenizer.NumTokensFromString(userContent); got < 8000 {
		t.Fatalf("user message trimmed to %d tokens; want it preserved under the gpt-4o content_length budget, got head: %.80q", got, userContent)
	}
	if !strings.Contains(userContent, bigPrompt) {
		t.Fatal("user prompt was modified by fitting despite fitting the content_length budget")
	}
}

// TestCleanFormattedAnswer pins cleanFormattedAnswer's pipeline: think-block
// strip (common.StripThinkTrailing) first, then JSON-fence prefix/suffix.
func TestCleanFormattedAnswer(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "plain answer", want: "plain answer"},
		{name: "think prefix", in: "<think>reasoning</think>{\"a\":1}", want: "{\"a\":1}"},
		{name: "mid-text think", in: "note<think>reasoning</think>{\"a\":1}", want: "{\"a\":1}"},
		{name: "json fence", in: "```json\n{\"a\":1}\n```", want: "\n{\"a\":1}\n"},
		{name: "think then fence", in: "<think>reasoning</think>```json\n{\"a\":1}\n```", want: "\n{\"a\":1}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanFormattedAnswer(tt.in); got != tt.want {
				t.Errorf("cleanFormattedAnswer(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
