// Package component — LLM (Phase 2 P0, plan §2.11.3 row 5).
//
// One-shot LLM call. Reads system_prompt + user_prompt, dispatches to a
// chat model, and returns the assistant's content. Streaming variant
// forwards incremental chunks via Stream.
//
// Model invocation is abstracted behind a small ChatInvoker interface so
// tests can inject a stub without touching the network. The default
// ChatInvoker is built around models.NewEinoChatModel so production paths
// flow through the eino bridge (plan §2.11.6 D1).
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/entity/models"
)

// LLMComponent is a one-shot chat call.
type LLMComponent struct {
	param LLMParam
}

// LLMParam captures the (resolved) DSL parameters for an LLM node.
type LLMParam struct {
	ModelID      string
	SystemPrompt string
	UserPrompt   string
	Temperature  *float64
	MaxTokens    *int
	JSONOutput   bool

	// Driver is the provider driver to use (e.g. "openai", "dummy"). When
	// empty, the default ChatInvoker will look up a driver from ModelID
	// (e.g. by attempting NewDummyModel for unknown providers).
	Driver string

	// APIKey overrides the default empty key. Tests may set this; prod
	// reads it from env / secret store at higher layers.
	APIKey string

	// BaseURL overrides the driver default endpoint (e.g. to point the
	// "openai" driver at a third-party gateway). Empty defers to the
	// driver's built-in default URL.
	BaseURL string
}

// LLMInput is the resolved input map the factory / Invoke expects.
type LLMInput struct {
	ModelID      string
	SystemPrompt string
	UserPrompt   string
	Temperature  *float64
	MaxTokens    *int
	JSONOutput   bool
	Driver       string
	APIKey       string
}

// LLMOutput mirrors the outputs map (per plan §2.11.3 row 5):
//
//	"content" string, "model" string, "stopped" bool, "tokens" int
//
// JSONOutput=true additionally populates "json" (map[string]any) when the
// content parses as a JSON object.
type LLMOutput struct {
	Content string
	Model   string
	Stopped bool
	Tokens  int
}

// ChatInvoker is the abstraction the LLM component uses to talk to a
// chat model. The default implementation lives in this file; tests can
// override the package-level defaultChatInvoker to inject a stub.
type ChatInvoker interface {
	Invoke(ctx context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error)
}

// ChatInvokeRequest is the minimal surface the LLM component needs to
// dispatch a chat call. Driver / APIKey / ModelName are kept here so the
// invoker can wire the right provider without the component caring.
type ChatInvokeRequest struct {
	Driver      string
	ModelName   string
	APIKey      string
	BaseURL     string
	Messages    []schema.Message
	Temperature *float64
	MaxTokens   *int
}

// ChatInvokeResponse mirrors what the LLM component writes to its outputs.
type ChatInvokeResponse struct {
	Content string
	Model   string
	Stopped bool
	Tokens  int
}

// defaultChatInvokerMu guards defaultChatInvoker swaps during tests.
var defaultChatInvokerMu sync.RWMutex

// defaultChatInvoker is the production ChatInvoker. Replaced in tests.
var defaultChatInvoker ChatInvoker = &einoChatInvoker{}

// SetDefaultChatInvoker swaps the package-level ChatInvoker (test helper).
// Pass nil to restore the default. Concurrent-safe.
func SetDefaultChatInvoker(inv ChatInvoker) {
	defaultChatInvokerMu.Lock()
	defer defaultChatInvokerMu.Unlock()
	defaultChatInvoker = inv
}

// getDefaultChatInvoker returns the current default ChatInvoker.
func getDefaultChatInvoker() ChatInvoker {
	defaultChatInvokerMu.RLock()
	defer defaultChatInvokerMu.RUnlock()
	if defaultChatInvoker == nil {
		return &einoChatInvoker{}
	}
	return defaultChatInvoker
}

// einoChatInvoker is the production ChatInvoker — it constructs a fresh
// models.EinoChatModel per call from the request and dispatches.
type einoChatInvoker struct{}

// Invoke satisfies ChatInvoker.
func (e *einoChatInvoker) Invoke(ctx context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	if req.ModelName == "" {
		return nil, fmt.Errorf("component: LLM: model_id is required")
	}
	driver := req.Driver
	if driver == "" {
		driver = "dummy"
	}
	// baseURL: drivers consult map["default"] as the canonical endpoint
	// (see internal/entity/models/base_model.go:GetBaseURL). When the
	// caller did not override, leave the driver default in place by
	// passing nil — every driver seeds its own map at construction time.
	var baseURL map[string]string
	if req.BaseURL != "" {
		baseURL = map[string]string{"default": req.BaseURL}
	}
	// urlSuffix: each driver appends URLSuffix.Chat to baseURL to form
	// the chat-completions endpoint (e.g. "chat/completions" for
	// openai-compatible drivers, "v1/messages" for anthropic). The
	// factory's NewModelDriver accepts a zero URLSuffix and stores it
	// as-is; the openai driver then builds `<base>/` (with no path),
	// which is the wrong endpoint for a v1-root base URL. We seed
	// the right suffix per driver here so the factory and the
	// openai driver's URL construction agree.
	urlSuffix := chatURLSuffixFor(driver)
	d, err := models.NewModelFactory().CreateModelDriver(driver, baseURL, urlSuffix)
	if err != nil {
		return nil, fmt.Errorf("component: LLM: resolve driver %q: %w", driver, err)
	}
	if d == nil {
		return nil, fmt.Errorf("component: LLM: no driver for %q", driver)
	}
	apiKey := req.APIKey
	cfg := &models.APIConfig{ApiKey: &apiKey}
	cm := models.NewChatModel(d, &req.ModelName, cfg)

	chatCfg := &models.ChatConfig{
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	wrapper := models.NewEinoChatModel(cm, chatCfg)
	out, err := wrapper.Generate(ctx, toEinoMessages(req.Messages))
	if err != nil {
		return nil, err
	}
	return &ChatInvokeResponse{
		Content: out.Content,
		Model:   req.ModelName,
		Stopped: true,
		Tokens:  0,
	}, nil
}

// toEinoMessages converts the LLM component's Message slice to eino's.
func toEinoMessages(msgs []schema.Message) []*schema.Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]*schema.Message, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		role := m.Role
		if role == "" {
			role = schema.User
		}
		out = append(out, &schema.Message{Role: role, Content: m.Content})
	}
	return out
}

// chatURLSuffixFor returns the URLSuffix the factory should pass to
// the driver for the chat endpoint. Each driver's ChatWithMessages
// builds `baseURL/URLSuffix.Chat`, so the suffix has to match the
// provider's actual chat path. We seed the common ones here; for any
// driver the factory has no entry for, we fall through to a default
// "chat/completions" path (the openai-compatible default), which
// matches the dummy driver and any third-party openai-compatible
// gateway.
func chatURLSuffixFor(driver string) models.URLSuffix {
	switch strings.ToLower(driver) {
	case "anthropic":
		return models.URLSuffix{Chat: "v1/messages"}
	default:
		return models.URLSuffix{Chat: "chat/completions"}
	}
}

// NewLLMComponent builds an LLMComponent from raw params.
func NewLLMComponent(p LLMParam) *LLMComponent {
	return &LLMComponent{param: p}
}

// Name returns the registered component name.
func (c *LLMComponent) Name() string { return "LLM" }

// Invoke runs the LLM and returns the output map.
func (c *LLMComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	p := mergeLLMParam(c.param, inputs)
	if p.ModelID == "" {
		return nil, &ParamError{Field: "model_id", Reason: "required"}
	}
	if p.UserPrompt == "" && p.SystemPrompt == "" {
		return nil, &ParamError{Field: "user_prompt", Reason: "at least one of user_prompt or system_prompt must be set"}
	}
	// The Anthropic driver (and the openai chat-completions driver
	// when the system role is dropped) reject a system-only message
	// list with "messages is empty" / 400. v1 fixtures frequently
	// ship only a system prompt; fall back to using the system text
	// as the user message so the call still goes through. The
	// answer text in that case is the model continuing the
	// instruction in its reply slot, which is what the v1 fixtures
	// also expect.
	if p.UserPrompt == "" {
		p.UserPrompt = p.SystemPrompt
	}

	msgs := buildMessages(p.SystemPrompt, p.UserPrompt)
	inv := getDefaultChatInvoker()
	resp, err := inv.Invoke(ctx, ChatInvokeRequest{
		Driver:      p.Driver,
		ModelName:   p.ModelID,
		APIKey:      p.APIKey,
		BaseURL:     p.BaseURL,
		Messages:    msgs,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("component: LLM.Invoke: %w", err)
	}

	out := map[string]any{
		"content": resp.Content,
		"model":   resp.Model,
		"stopped": resp.Stopped,
		"tokens":  resp.Tokens,
	}
	if p.JSONOutput {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(resp.Content), &parsed); err == nil {
			out["json"] = parsed
		} else {
			// Surface a non-fatal warning — caller can still read "content".
			log.Printf("component: LLM: json_output=true but content is not valid JSON: %v", err)
		}
	}
	return out, nil
}

// Stream implements Component.Stream. It forwards incremental chunks via
// the returned channel. When the model finishes, the channel is closed.
func (c *LLMComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out := make(chan map[string]any, 1)
	go func() {
		defer close(out)
		result, err := c.Invoke(ctx, inputs)
		if err != nil {
			out <- map[string]any{"error": err.Error()}
			return
		}
		out <- result
	}()
	return out, nil
}

// Inputs returns parameter metadata for tooling.
func (c *LLMComponent) Inputs() map[string]string {
	return map[string]string{
		"model_id":      "Provider-side model identifier (e.g. \"gpt-4o-mini\")",
		"system_prompt": "Optional system prompt prepended to the conversation",
		"user_prompt":   "User prompt; supports {{cpn_id@param}} references resolved by the canvas engine",
		"temperature":   "Sampling temperature (0.0-2.0). Optional.",
		"max_tokens":    "Maximum tokens to generate. Optional.",
		"json_output":   "If true, attempt to JSON-parse \"content\" into \"json\" output key.",
		"driver":        "Provider driver name (openai, anthropic, …). Defaults to \"dummy\".",
		"api_key":       "Override API key for this call. Empty defers to env.",
	}
}

// Outputs returns output metadata.
func (c *LLMComponent) Outputs() map[string]string {
	return map[string]string{
		"content": "Assistant text response",
		"model":   "Model identifier echoed back (sanity check)",
		"stopped": "True if the model finished naturally",
		"tokens":  "Reported token count (0 when not reported by the driver)",
		"json":    "When json_output=true and content parses as a JSON object, the parsed map",
	}
}

// buildMessages assembles a system + user message sequence. Order:
// system first (if set), then user.
func buildMessages(system, user string) []schema.Message {
	out := make([]schema.Message, 0, 2)
	if system != "" {
		out = append(out, schema.Message{Role: schema.System, Content: system})
	}
	if user != "" {
		out = append(out, schema.Message{Role: schema.User, Content: user})
	}
	return out
}

// mergeLLMParam layers raw inputs over the receiver's default param set.
//
// v1 DSL aliases accepted alongside the v2 names:
//
//	"llm_id"      → "model_id"
//	"sys_prompt"  → "system_prompt"
//	"base_url"    → "BaseURL"
//
// The v1 fixtures in internal/agent/dsl/testdata/v1_examples use the
// short forms; without these aliases the v1→v2 conversion (plan §2.5)
// would have to be run before the factory builds the component, which
// the e2e compile+invoke path doesn't do.
func mergeLLMParam(base LLMParam, inputs map[string]any) LLMParam {
	p := base
	if v, ok := stringFrom(inputs, "model_id"); ok {
		p.ModelID = v
	} else if v, ok := stringFrom(inputs, "llm_id"); ok {
		p.ModelID = v
	}
	if v, ok := stringFrom(inputs, "system_prompt"); ok {
		p.SystemPrompt = v
	} else if v, ok := stringFrom(inputs, "sys_prompt"); ok {
		p.SystemPrompt = v
	}
	if v, ok := stringFrom(inputs, "user_prompt"); ok {
		p.UserPrompt = v
	}
	if v, ok := boolFrom(inputs, "json_output"); ok {
		p.JSONOutput = v
	}
	if v, ok := stringFrom(inputs, "driver"); ok {
		p.Driver = v
	}
	if v, ok := stringFrom(inputs, "api_key"); ok {
		p.APIKey = v
	}
	if v, ok := stringFrom(inputs, "base_url"); ok {
		p.BaseURL = v
	}
	if v, ok := floatFrom(inputs, "temperature"); ok {
		f := v
		p.Temperature = &f
	}
	if v, ok := intFrom(inputs, "max_tokens"); ok {
		i := v
		p.MaxTokens = &i
	}
	return p
}

// stringFrom extracts a string from inputs[name], accepting both string and
// fmt.Stringer-able values.
func stringFrom(inputs map[string]any, name string) (string, bool) {
	v, ok := inputs[name]
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

// boolFrom extracts a bool from inputs[name].
func boolFrom(inputs map[string]any, name string) (bool, bool) {
	v, ok := inputs[name]
	if !ok {
		return false, false
	}
	if b, ok := v.(bool); ok {
		return b, true
	}
	return false, false
}

// floatFrom extracts a float64 from inputs[name], also accepting int.
func floatFrom(inputs map[string]any, name string) (float64, bool) {
	v, ok := inputs[name]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// intFrom extracts an int from inputs[name], also accepting float64.
func intFrom(inputs map[string]any, name string) (int, bool) {
	v, ok := inputs[name]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	}
	return 0, false
}

// init registers LLMComponent with the orchestrator-owned registry.
func init() {
	Register("LLM", func(params map[string]any) (Component, error) {
		var p LLMParam
		if v, ok := stringFrom(params, "model_id"); ok {
			p.ModelID = v
		} else if v, ok := stringFrom(params, "llm_id"); ok {
			p.ModelID = v
		}
		if v, ok := stringFrom(params, "system_prompt"); ok {
			p.SystemPrompt = v
		} else if v, ok := stringFrom(params, "sys_prompt"); ok {
			p.SystemPrompt = v
		}
		if v, ok := stringFrom(params, "user_prompt"); ok {
			p.UserPrompt = v
		}
		if v, ok := floatFrom(params, "temperature"); ok {
			f := v
			p.Temperature = &f
		}
		if v, ok := intFrom(params, "max_tokens"); ok {
			i := v
			p.MaxTokens = &i
		}
		if v, ok := boolFrom(params, "json_output"); ok {
			p.JSONOutput = v
		}
		if v, ok := stringFrom(params, "driver"); ok {
			p.Driver = v
		}
		if v, ok := stringFrom(params, "api_key"); ok {
			p.APIKey = v
		}
		if v, ok := stringFrom(params, "base_url"); ok {
			p.BaseURL = v
		}
		return NewLLMComponent(p), nil
	})
}
