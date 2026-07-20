// Package component — LLM (T1).
//
// One-shot LLM call. Reads system_prompt + user_prompt, dispatches to a
// chat model, and returns the assistant content. Streaming variant
// forwards incremental chunks via Stream.
//
// Model invocation is abstracted behind a small ChatInvoker interface so
// tests can inject a stub without touching the network. The default
// ChatInvoker is the abstraction for LLM chat calls.

package component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/component/prompts"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity/models"
)

// LLMComponent is a one-shot chat call.
type LLMComponent struct {
	param LLMParam
}

// LLMParam captures the (resolved) DSL parameters for an LLM node.
type LLMParam struct {
	ModelID                  string
	SystemPrompt             string
	UserPrompt               string
	Temperature              *float64
	TopP                     *float64
	VisualFiles              []string       // extracted data:image URIs from inputs["visual_files"]
	Cite                     bool           // when true, citation-instruction prompt is appended to system message
	MessageHistoryWindowSize int            // when >0, the last N turns from state.History are prepended as prior messages
	ChatTemplateKwargs       map[string]any // optional provider-specific kwargs (e.g. response_format, seed)
	MaxTokens                *int
	JSONOutput               bool
	OutputStructure          map[string]any // when set, LLM is asked for JSON matching this schema (best-effort keys); outputs["structured"] populated

	// Driver is the provider driver to use (e.g. "openai", "dummy"). When
	// empty, the default ChatInvoker will look up a driver from ModelID
	// (e.g. by attempting NewDummyModel for unknown providers).
	Driver string

	// APIKey overrides the default empty key. Tests may set this; prod
	// reads it from env / secret store at higher layers.
	APIKey string

	// BaseURL overrides the driver default endpoint (e.g. to point the
	// "openai" driver at a third-party gateway). Empty defers to the
	// driver built-in default URL.
	BaseURL string

	// MaxRetries caps the retry loop in retryInvoker. Zero = default
	// (3). Negative = disable retries entirely (single attempt). The
	// retry loop honours ctx.Done() so a request cancel aborts on
	// the next backoff sleep.
	MaxRetries int

	// DelayAfterError is the initial backoff between retry attempts.
	// Doubles on each retry, capped at 1 minute. Zero = default
	// (2 seconds). Matches Python `delay_after_error` param.
	DelayAfterError time.Duration

	// Thinking mirrors the python `thinking` Agent LLM setting
	// (PR #15446). When set to "enabled" or "disabled", the LLM
	// driver is told to turn its reasoning mode on/off
	// (provider-specific; see chat_model.py for Qwen/Kimi/GLM
	// policy). Empty string means "system default" — the LLM
	// driver decides, which today means Qwen3 is sent
	// `enable_thinking=false` unless overridden.
	Thinking string
}

// LLMInput is the resolved input map the factory / Invoke expects.
type LLMInput struct {
	ModelID                  string
	SystemPrompt             string
	UserPrompt               string
	Temperature              *float64
	TopP                     *float64
	Cite                     bool
	MessageHistoryWindowSize int
	ChatTemplateKwargs       map[string]any
	MaxTokens                *int
	JSONOutput               bool
	OutputStructure          map[string]any
	Driver                   string
	APIKey                   string
	Thinking                 string // "enabled" | "disabled" | ""
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

// progressChKey is the context key for the streaming progress channel.
type progressChKey struct{}

// WithProgressCh attaches a progress channel to ctx so the Agent
// component's streaming LLM call can send content chunks upstream.
func WithProgressCh(ctx context.Context, ch chan<- string) context.Context {
	return context.WithValue(ctx, progressChKey{}, ch)
}

// ProgressChFromCtx extracts the progress channel from context.
func ProgressChFromCtx(ctx context.Context) chan<- string {
	if ch, ok := ctx.Value(progressChKey{}).(chan<- string); ok {
		return ch
	}
	return nil
}

// ChatInvokeRequest is the minimal surface the LLM component needs to
// dispatch a chat call. Driver / APIKey / ModelName are kept here so the
// invoker can wire the right provider without the component caring.
type ChatInvokeRequest struct {
	Driver           string
	ModelName        string
	APIKey           string
	BaseURL          string
	Messages         []ComponentMessage
	Temperature      *float64
	TopP             *float64
	PresencePenalty  *float64
	FrequencyPenalty *float64
	MaxTokens        *int
	// Tools carries function-calling tool definitions for the ReAct loop.
	// When non-empty, the invoker builds the request directly (bypassing
	// the model driver's ChatWithMessages) so the model driver layer
	// (deepseek.go etc.) does NOT need modification for tool support.
	Tools []map[string]any
	// Thinking mirrors the agent-level `thinking` setting
	// ("enabled" | "disabled" | "").
	Thinking string
}

// ChatInvokeResponse mirrors what the LLM component writes to its outputs.
type ChatInvokeResponse struct {
	Content       string
	Model         string
	Stopped       bool
	Tokens        int
	ToolCalls     []ToolCallResult // non-empty when the model wants to call a tool
	ReasonContent string           // thinking/reasoning content (DeepSeek, etc.)
	Thinking      string
}

// ToolCallResult captures a single function-call from the LLM response.
type ToolCallResult struct {
	ID        string
	Type      string
	FuncName  string
	Arguments string
}

// ModelLocator resolves a model ID (e.g. "qwen-max@Tongyi-Qianwen")
// to its driver name, resolved model name, API key, and base URL for
// a given tenant. The resolved model name is the pure model name
// without provider suffix (e.g. "deepseek-v4-flash" not
// "deepseek-v4-flash@MyDS@DeepSeek").
type ModelLocator func(tenantID, modelID string) (driver, resolvedModelName, apiKey, baseURL string, err error)

// modelLocator is the package-level model resolver. The default returns
// ("", "", "", nil) which causes the ChatInvoker to fall back to "dummy".
// Production wiring (cmd/server_main.go) sets this to a real implementation.
var modelLocator ModelLocator

// SetModelLocator installs the production model locator. Thread-safe.
func SetModelLocator(l ModelLocator) {
	modelLocator = l
}

// defaultChatInvokerMu guards defaultChatInvoker swaps during tests.
var defaultChatInvokerMu sync.RWMutex

// defaultChatInvoker is the production ChatInvoker. Replaced in tests.
var defaultChatInvoker ChatInvoker = &productionChatInvoker{}

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
		return &productionChatInvoker{}
	}
	return defaultChatInvoker
}

// GetDefaultChatInvokerForTest exposes the current package-level invoker so
// tests can restore it after swapping in a stub.
func GetDefaultChatInvokerForTest() ChatInvoker {
	return getDefaultChatInvoker()
}

// it constructs a fresh
// models.ChatModel per call and dispatches.
type productionChatInvoker struct{}

// Invoke satisfies ChatInvoker.
func (e *productionChatInvoker) Invoke(ctx context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	if req.ModelName == "" {
		return nil, fmt.Errorf("component: LLM: model_id is required")
	}
	driver := req.Driver
	if driver == "" && modelLocator != nil {
		// Try to resolve the model from the user's tenant configuration.
		tenantID := ""
		if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
			if tid, ok := state.Sys["tenant_id"].(string); ok {
				tenantID = tid
			}
		}
		if d, resolvedName, ak, bu, err := modelLocator(tenantID, req.ModelName); err == nil && d != "" {
			driver = d
			req.ModelName = resolvedName
			req.APIKey = ak
			req.BaseURL = bu
		}
	}
	if driver == "" {
		driver = "dummy"
	}
	// baseURL: drivers consult map["default"] as the canonical endpoint
	// (see internal/entity/models/base_model.go:GetBaseURL). When the
	// caller did not override, look up the provider's default URL from
	// the model provider manager so the driver picks up the config-file
	// endpoint (e.g. https://api.deepseek.com for DeepSeek).
	var baseURL map[string]string
	if req.BaseURL != "" {
		baseURL = map[string]string{"default": req.BaseURL}
	} else if driver != "" && driver != "dummy" {
		if pi := dao.GetModelProviderManager().FindProvider(driver); pi != nil && len(pi.URL) > 0 {
			baseURL = pi.URL
		}
	}
	// urlSuffix: each driver appends URLSuffix.Chat to baseURL to form
	// the chat-completions endpoint (e.g. "chat/completions" for
	// openai-compatible drivers, "v1/messages" for anthropic). The
	// factory NewModelDriver accepts a zero URLSuffix and stores it
	// as-is; the openai driver then builds `<base>/` (with no path),
	// which is the wrong endpoint for a v1-root base URL. We seed
	// the right suffix per driver here so the factory and the
	// openai driver URL construction agree.
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
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
	}
	// When tools are present, pass messages as ComponentMessage so
	// tool_call_id and tool_calls fields are preserved in the API request.
	if len(req.Tools) > 0 {
		return invokeWithTools(ctx, req, driver, baseURL, urlSuffix, apiKey, req.Messages)
	}

	// Convert to models.Message for non-tool driver path.
	ragMsgs := make([]models.Message, len(req.Messages))
	for i, m := range req.Messages {
		ragMsgs[i] = models.Message{Role: m.Role, Content: m.Content}
	}
	resp, err := cm.ModelDriver.ChatWithMessages(*cm.ModelName, ragMsgs, cm.APIConfig, chatCfg, nil)
	if err != nil {
		return nil, err
	}
	if resp.Answer == nil {
		return nil, fmt.Errorf("LLM returned nil Answer")
	}
	return &ChatInvokeResponse{
		Content: *resp.Answer,
		Model:   req.ModelName,
		Stopped: true,
		Tokens:  0,
	}, nil
}

// invokeWithTools sends a streaming chat request WITH tool definitions.
// Uses `stream: true` (matching Python) and parses the SSE response for
// both text content and tool_calls. Tool_calls are merged by index across
// multiple SSE chunks, matching the OpenAI streaming format.
// The parent ctx is used for timeout/honouring and MUST NOT be nil.
func invokeWithTools(ctx context.Context, req ChatInvokeRequest, driver string, baseURL map[string]string, urlSuffix models.URLSuffix, apiKey string, compMsgs []ComponentMessage) (*ChatInvokeResponse, error) {
	resolvedBaseURL, ok := baseURL["default"]
	if !ok || resolvedBaseURL == "" {
		return nil, fmt.Errorf("invokeWithTools: no base URL for driver %q", driver)
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, urlSuffix.Chat)

	apiMessages := make([]map[string]any, len(compMsgs))
	for i, m := range compMsgs {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		// Tool result messages MUST include the tool_call_id matching the
		// original tool call — required by the OpenAI API format.
		if m.Role == "tool" && m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		// Assistant messages with tool calls must include the tool_calls
		// array so the model sees the full call context.
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			tcList := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				tcList = append(tcList, map[string]any{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]any{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})
			}
			msg["tool_calls"] = tcList
		}
		apiMessages[i] = msg
	}

	body := map[string]any{
		"model":       req.ModelName,
		"messages":    apiMessages,
		"stream":      false, // Tools present → non-streaming for reliable tool_calls
		"tools":       req.Tools,
		"tool_choice": "auto",
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	}
	// Log tool names sent
	if len(req.Tools) > 0 {
		var names []string
		for _, t := range req.Tools {
			if fn, _ := t["function"].(map[string]any); fn != nil {
				if n, _ := fn["name"].(string); n != "" {
					names = append(names, n)
				}
			}
		}
		common.Info("invokeWithTools sending tools", zap.Strings("names", names))
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("invokeWithTools: marshal: %w", err)
	}

	// Derive from caller's ctx so cancellation propagates (e.g. Agent
	// run cancelled mid-request). The 300s inner timeout is a safety
	// bound even if ctx never cancels. The component-level timeout
	// (600s in realComponentBody, env COMPONENT_EXEC_TIMEOUT) governs
	// the total execution time across all tool calls.
	callCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("invokeWithTools: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("invokeWithTools: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("invokeWithTools: API %d: %s", resp.StatusCode, string(bodyBytes))
	}

	out := &ChatInvokeResponse{Model: req.ModelName}

	// Non-streaming response: single JSON body.
	var bodyResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&bodyResp); err != nil {
		return nil, fmt.Errorf("invokeWithTools: decode: %w", err)
	}

	choices, _ := bodyResp["choices"].([]any)
	if len(choices) == 0 {
		return nil, fmt.Errorf("invokeWithTools: no choices in response")
	}
	first, _ := choices[0].(map[string]any)
	msg, _ := first["message"].(map[string]any)
	if msg == nil {
		return nil, fmt.Errorf("invokeWithTools: no message in choice")
	}

	// Content
	if c, _ := msg["content"].(string); c != "" {
		out.Content = c
	}
	// Reasoning content (DeepSeek)
	if rc, _ := msg["reasoning_content"].(string); rc != "" {
		out.ReasonContent = rc
	}

	// Tool calls from JSON body — standard non-streaming format.
	if rawCalls, ok := msg["tool_calls"].([]any); ok && len(rawCalls) > 0 {
		for _, raw := range rawCalls {
			call, _ := raw.(map[string]any)
			id, _ := call["id"].(string)
			typ, _ := call["type"].(string)
			fnRaw, _ := call["function"].(map[string]any)
			if fnRaw == nil {
				continue
			}
			fnName, _ := fnRaw["name"].(string)
			fnArgs, _ := fnRaw["arguments"].(string)
			out.ToolCalls = append(out.ToolCalls, ToolCallResult{
				ID: id, Type: typ, FuncName: fnName, Arguments: fnArgs,
			})
		}
	}

	if len(out.ToolCalls) == 0 && out.Content == "" && out.ReasonContent == "" {
		return nil, fmt.Errorf("invokeWithTools: empty response")
	}
	return out, nil
}

// chatURLSuffixFor returns the URLSuffix the factory should pass to
// the driver for the chat endpoint. Each driver ChatWithMessages
// builds `baseURL/URLSuffix.Chat`, so the suffix has to match the
// provider actual chat path. We seed the common ones here; for any
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

// StreamingInvoke resolves the model, builds the streaming request with
// tool definitions (when req.Tools is non-empty), and returns the full
// response. Every content and reasoning_content chunk is forwarded to
// onProgress in real time. Tool calls from the streaming SSE stream are
// properly merged by index.
//
// When req.Tools is empty, StreamingInvoke falls back to the standard
// ChatInvoker.Invoke path (non-streaming). This is intentional: only
// the Agent ReAct loop needs streaming-with-tools; the one-shot LLM
// component uses a simpler path.
func StreamingInvoke(ctx context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	if req.ModelName == "" {
		return nil, fmt.Errorf("component: LLM: model_id is required")
	}
	if len(req.Tools) == 0 {
		// No tools → use standard non-streaming invoke. This path is
		// the simple one-shot call; the Agent loop always has tools.
		return getDefaultChatInvoker().Invoke(ctx, req)
	}
	driver := req.Driver
	if driver == "" && modelLocator != nil {
		tenantID := ""
		if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
			if tid, ok := state.Sys["tenant_id"].(string); ok {
				tenantID = tid
			}
		}
		if d, resolvedName, ak, bu, err := modelLocator(tenantID, req.ModelName); err == nil && d != "" {
			driver = d
			req.ModelName = resolvedName
			req.APIKey = ak
			req.BaseURL = bu
		}
	}
	if driver == "" {
		driver = "dummy"
	}
	// When the driver resolves to "dummy" (no real model found — typical
	// in tests), fall back to the standard ChatInvoker.Invoke path. The
	// direct HTTP path (invokeWithTools) requires a resolvable base URL
	// and fails with "no base URL for driver" for "dummy".
	// Stripping tools ensures the mock invoker (which returns canned
	// responses) is used instead of the production HTTP path.
	if driver == "dummy" {
		req.Tools = nil
		return getDefaultChatInvoker().Invoke(ctx, req)
	}
	var baseURL map[string]string
	if req.BaseURL != "" {
		baseURL = map[string]string{"default": req.BaseURL}
	} else if driver != "" && driver != "dummy" {
		if pi := dao.GetModelProviderManager().FindProvider(driver); pi != nil && len(pi.URL) > 0 {
			baseURL = pi.URL
		}
	}
	urlSuffix := chatURLSuffixFor(driver)
	apiKey := req.APIKey
	return invokeWithTools(ctx, req, driver, baseURL, urlSuffix, apiKey, req.Messages)
}

// StreamContentStreaming sends a streaming chat request WITHOUT tools and
// forwards every content and reasoning_content chunk to onChunk. Returns the
// full accumulated content on success. This is used by the Agent ReAct loop
// to stream the final answer progressively.
//
// When the driver cannot be resolved (test mode), the function falls back to
// sending the content from a non-streaming invoke as a single chunk.
func StreamContentStreaming(ctx context.Context, req ChatInvokeRequest, onChunk func(string, bool)) (string, error) {
	if req.ModelName == "" {
		return "", fmt.Errorf("component: LLM: model_id is required")
	}
	driver := req.Driver
	if driver == "" && modelLocator != nil {
		tenantID := ""
		if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
			if tid, ok := state.Sys["tenant_id"].(string); ok {
				tenantID = tid
			}
		}
		if d, resolvedName, ak, bu, err := modelLocator(tenantID, req.ModelName); err == nil && d != "" {
			driver = d
			req.ModelName = resolvedName
			req.APIKey = ak
			req.BaseURL = bu
		}
	}
	if driver == "" {
		driver = "dummy"
	}
	if driver == "dummy" {
		// No real model — just return the non-streaming content as a single chunk.
		resp, err := getDefaultChatInvoker().Invoke(ctx, req)
		if err != nil {
			return "", err
		}
		if resp.Content != "" {
			onChunk(resp.Content, false)
		}
		return resp.Content, nil
	}

	var baseURL map[string]string
	if req.BaseURL != "" {
		baseURL = map[string]string{"default": req.BaseURL}
	} else if pi := dao.GetModelProviderManager().FindProvider(driver); pi != nil && len(pi.URL) > 0 {
		baseURL = pi.URL
	}
	urlSuffix := chatURLSuffixFor(driver)

	d, err := models.NewModelFactory().CreateModelDriver(driver, baseURL, urlSuffix)
	if err != nil {
		return "", fmt.Errorf("component: streamContent: resolve driver %q: %w", driver, err)
	}
	if d == nil {
		return "", fmt.Errorf("component: streamContent: no driver for %q", driver)
	}

	apiKey := req.APIKey
	cfg := &models.APIConfig{ApiKey: &apiKey}
	cm := models.NewChatModel(d, &req.ModelName, cfg)
	chatCfg := &models.ChatConfig{
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
	}
	ragMsgs := make([]models.Message, len(req.Messages))
	for i, m := range req.Messages {
		ragMsgs[i] = models.Message{Role: m.Role, Content: m.Content}
	}

	var fullContent strings.Builder
	err = cm.ModelDriver.ChatStreamlyWithSender(req.ModelName, ragMsgs, cm.APIConfig, chatCfg, nil, func(content *string, reason *string) error {
		if content != nil && *content != "" {
			if *content == "[DONE]" {
				return nil
			}
			fullContent.WriteString(*content)
			// Both content and reasoning from the final streaming round
			// go to the main output (IsThink:false). The intermediate
			// reasoning from tool-calling rounds is already in thinking;
			// the final round's reasoning belongs with the answer.
			onChunk(*content, false)
		}
		if reason != nil && *reason != "" {
			fullContent.WriteString(*reason)
			onChunk(*reason, false)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("component: streamContent: %w", err)
	}
	return fullContent.String(), nil
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
	// Resolve {{cpn_id@var}} references in the system and user
	// prompts against the canvas state attached to ctx. When the
	// state is absent (e.g. tests that call Invoke directly without
	// going through the canvas scheduler), the prompts pass through
	// unchanged — backward compatible.
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		// ResolveTemplate returns the partial output (with "" in place
		// of unresolved refs) even on error — we accept the partial
		// output and log the error for diagnostics. This matches
		// Python silent-soft-fail behavior (canvas.py returns "" for
		// missing refs) but adds a log line so misconfigured canvases
		// are still surfaced.
		if resolved, rerr := runtime.ResolveTemplate(p.SystemPrompt, state); resolved != p.SystemPrompt || rerr == nil {
			p.SystemPrompt = resolved
			if rerr != nil {
				common.Warn("component: LLM: resolve system_prompt", zap.Error(rerr))
			}
		}
		if resolved, rerr := runtime.ResolveTemplate(p.UserPrompt, state); resolved != p.UserPrompt || rerr == nil {
			p.UserPrompt = resolved
			if rerr != nil {
				common.Warn("component: LLM: resolve user_prompt", zap.Error(rerr))
			}
		}
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
		p.UserPrompt = "Please process the instructions above."
	}

	msgs := buildMessagesWithImages(p.SystemPrompt, p.UserPrompt, p.VisualFiles, p.Cite)
	// Prepend the last N turns of conversation history from the
	// canvas state. Mirrors Python `_get_chat_template_kwargs` /
	// `_fit_messages` path. When window size is 0 or history is
	// empty,
	// this is a no-op.
	if p.MessageHistoryWindowSize > 0 {
		if state, _, sErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); sErr == nil && state != nil {
			history := state.SnapshotPriorHistory()
			if len(history) == 0 {
				history = state.History
			}
			msgs = prependHistory(msgs, history, p.MessageHistoryWindowSize)
		}
	}
	inv := getDefaultChatInvoker()
	// Param-level retry override. When MaxRetries OR
	// DelayAfterError is set on LLMParam, the user is asking
	// for a per-call retry budget. We RE-WRAP the default
	// invoker in a fresh retryInvoker that respects those
	// values literally.
	//
	// LLM retry normal-absolute-count: when MaxRetries OR
	// DelayAfterError is explicitly set on LLMParam, the
	// operator intent is an ABSOLUTE attempt budget. The
	// default invoker installed at boot in cmd/server_main.go
	// is itself a retryInvoker wrapping productionChatInvoker.
	// Without unwrapping, the two loops would multiplicatively
	// stack:
	//
	// boot=3, MaxRetries=5 → up to (3+1) × (5+1) = 24
	// invocations, not the 6 the
	// operator almost certainly intended.
	//
	// unwrapChatInvoker peels off any retryInvoker layers to
	// reach the bare invoker, then the param-override branch
	// wraps that bare invoker in a fresh retryInvoker with the
	// operator literal values. Net effect: the absolute attempt
	// count is exactly (MaxRetries + 1), independent of the boot
	// layer.
	//
	// Operators who do NOT set MaxRetries (both fields zero) get
	// the boot retry chain unchanged. The unit tests in
	// llm_retry_test.go pin both the unwrap behaviour and the
	// stacking-prevention contract.
	hasOverride := p.MaxRetries > 0 || p.DelayAfterError > 0
	if hasOverride {
		maxRetries := p.MaxRetries
		delay := p.DelayAfterError
		if delay <= 0 {
			delay = retryInvokerBackoff
		}
		// Normalise the attempt budget: peel off the boot
		// retryInvoker layer (if any) so the operator's
		// MaxRetries is an absolute count, not a stacked one.
		inv = newRetryInvoker(unwrapChatInvoker(inv), maxRetries, delay)
	}
	resp, err := inv.Invoke(ctx, ChatInvokeRequest{
		Driver:      p.Driver,
		ModelName:   p.ModelID,
		APIKey:      p.APIKey,
		BaseURL:     p.BaseURL,
		Messages:    msgs,
		Temperature: p.Temperature,
		TopP:        p.TopP,
		MaxTokens:   p.MaxTokens,
		Thinking:    p.Thinking,
	})
	if err != nil {
		return nil, fmt.Errorf("component: LLM.Invoke: %w", err)
	}

	// Strip think blocks + JSON fences from the response.
	// Mirrors Python's clean_formated_answer() exactly
	// (re.sub(r"^.*</think>", "", ...) + ^.*```json + trailing ```).
	// Python only cleans for structured output — keep raw content for
	// regular responses (llm.py:483: self.set_output("content", ans)).
	cleaned := resp.Content
	if p.OutputStructure != nil || p.JSONOutput {
		cleaned = cleanFormattedAnswer(resp.Content)
	}

	out := map[string]any{
		"content":  cleaned,
		"thinking": resp.Thinking,
		"model":    resp.Model,
		"stopped":  resp.Stopped,
		"tokens":   resp.Tokens,
	}
	if p.JSONOutput {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(resp.Content), &parsed); err == nil {
			out["json"] = parsed
		} else {
			// Surface a non-fatal warning — caller can still read "content".
			common.Warn("component: LLM: json_output=true but content is not valid JSON", zap.Error(err))
		}
	}
	if p.OutputStructure != nil {
		// Best-effort parse: if the first response isn't valid JSON
		// (or doesn't contain the expected top-level keys), retry once
		// with a re-prompt. OutputStructure is treated as a key-set
		// hint; deep schema validation (types, nested objects) is
		// deferred to a future phase.
		parsed, ok := matchOutputStructure(resp.Content, p.OutputStructure)
		if !ok {
			retryResp, err := inv.Invoke(ctx, ChatInvokeRequest{
				Driver:      p.Driver,
				ModelName:   p.ModelID,
				APIKey:      p.APIKey,
				BaseURL:     p.BaseURL,
				Messages:    buildStructuredRetryMessages(p.SystemPrompt, p.UserPrompt, p.VisualFiles, p.Cite, p.OutputStructure, resp.Content),
				Temperature: p.Temperature,
				TopP:        p.TopP,
				MaxTokens:   p.MaxTokens,
				Thinking:    p.Thinking,
			})
			if err == nil {
				parsed, ok = matchOutputStructure(retryResp.Content, p.OutputStructure)
				if ok {
					resp = retryResp
				}
			}
		}
		if ok {
			out["structured"] = parsed
			// Also update content to the validated response so
			// downstream consumers reading "content" get the JSON text.
			out["content"] = cleanFormattedAnswer(resp.Content)
		} else {
			common.Warn("component: LLM: output_structure set but no parseable JSON after retry")
		}
	}
	out["thinking"] = resp.Thinking
	return out, nil
}

// Stream implements Component.Stream. It yields incremental chunks via
// the returned channel; the channel is closed when the model finishes.
//
// The pattern follows the goroutine + buffered-channel + select-on-ctx
// idiom: one goroutine produces chunks, the consumer selects between
// receiving and ctx-cancellation. Backpressure is mitigated by the 16-
// element channel buffer.
//
// Each chunk is a map[string]any with two keys:
// - "thinking" (string): the model reasoning content, empty if absent
// - "content" (string): the model visible content
//
// A final chunk with key "done" (bool=true) signals end-of-stream so
// downstream consumers can flush state without relying on channel close
// alone (close also works; the "done" key is informational).
//
// Today, the LLM driver layer returns a single non-streamed response,
// so this v1 emits exactly one chunk + one done. Hooking the actual

// is deferred — the public surface here is correct, only the data
// source needs to be swapped to a real StreamReader consumer in a
// follow-up.
func (c *LLMComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out := make(chan map[string]any, 16)
	go func() {
		defer close(out)
		// Early bail-out for pre-cancelled contexts: don't run the
		// (potentially expensive) LLM call when the consumer has
		// already given up. Honors the documented select-on-ctx
		// pattern at the goroutine entry, not just between chunks.
		if err := ctx.Err(); err != nil {
			return
		}
		result, err := c.Invoke(ctx, inputs)
		if err != nil {
			select {
			case out <- map[string]any{"error": err.Error()}:
			case <-ctx.Done():
			}
			return
		}
		// Single non-streamed response — emit as one content chunk.
		// A real streaming integration would loop over a channel
		// here and emit multiple chunks with partial content.
		chunk := map[string]any{
			"thinking": result["thinking"],
			"content":  result["content"],
		}
		select {
		case out <- chunk:
		case <-ctx.Done():
			return
		}
		// Final done marker.
		select {
		case out <- map[string]any{"done": true, "model": result["model"]}:
		case <-ctx.Done():
		}
	}()
	return out, nil
}

// Inputs returns parameter metadata for tooling.
func (c *LLMComponent) Inputs() map[string]string {
	return map[string]string{
		"model_id":         "Provider-side model identifier (e.g. \"gpt-4o-mini\")",
		"system_prompt":    "Optional system prompt prepended to the conversation",
		"user_prompt":      "User prompt; supports {{cpn_id@param}} references resolved by the canvas engine",
		"temperature":      "Sampling temperature (0.0-2.0). Optional.",
		"top_p":            "Top-p (nucleus) sampling cutoff (0.0-1.0). Optional.",
		"visual_files":     "List of image URIs (data:image/... base64) attached to the user message as multi-modal content.",
		"cite":             "When true (default), the citation-instruction prompt is appended to the system message.",
		"output_structure": "Optional map of expected top-level keys. LLM is asked to produce JSON containing these keys; one retry on failure. Populates outputs[\"structured\"].",
		"max_tokens":       "Maximum tokens to generate. Optional.",
		"json_output":      "If true, attempt to JSON-parse \"content\" into \"json\" output key.",
		"driver":           "Provider driver name (openai, anthropic, …). Defaults to \"dummy\".",
		"api_key":          "Override API key for this call. Empty defers to env.",
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
func buildMessages(system, user string) []ComponentMessage {
	out := make([]ComponentMessage, 0, 2)
	if system != "" {
		out = append(out, ComponentMessage{Role: RoleSystem, Content: system})
	}
	if user != "" {
		out = append(out, ComponentMessage{Role: RoleUser, Content: user})
	}
	return out
}

// injectCitationPrompt returns the system message with the canonical
// citation-instruction text appended. When system is empty, returns
// the prompt as-is. Two newlines separate the user system prompt
// from the citation block so the LLM can parse them distinctly.
// matchOutputStructure parses the LLM response and returns the
// parsed map iff it is a JSON object that contains every top-level
// key in expected. Inner-type validation is deferred — a future
// phase will use a JSON-schema validator.
func matchOutputStructure(content string, expected map[string]any) (map[string]any, bool) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, false
	}
	for k := range expected {
		if _, ok := parsed[k]; !ok {
			return nil, false
		}
	}
	return parsed, true
}

// buildStructuredRetryMessages rebuilds the message list with a
// follow-up user turn that surfaces the LLM first response and
// asks for valid JSON matching the expected top-level keys. The
// retry uses the same chat invoker on the next call; the message
// list returned here is what gets sent on the retry.
func buildStructuredRetryMessages(system, user string, images []string, cite bool, expected map[string]any, prevContent string) []ComponentMessage {
	msgs := buildMessagesWithImages(system, user, images, cite)
	keys := make([]string, 0, len(expected))
	for k := range expected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	keysList := strings.Join(keys, ", ")
	retryUser := "Your previous response was not valid JSON matching the requested schema.\n\n" +
		"Previous response:\n" + prevContent + "\n\n" +
		"Please re-generate the response as a single valid JSON object containing all of these top-level keys: " + keysList + ".\n" +
		"Output ONLY the JSON object — no prose, no markdown code fences."
	if len(msgs) > 0 {
		msgs[len(msgs)-1] = ComponentMessage{
			Role:    RoleUser,
			Content: retryUser,
		}
	}
	return msgs
}

func injectCitationPrompt(system string) string {
	prompt := prompts.CitationPrompt()
	if system == "" {
		return prompt
	}
	return system + "\n\n" + prompt
}

// dataImageRe matches RFC-2397 data URLs of the form
//
//	data:image/<subtype>;base64,<payload>
//
// where <subtype> is an image MIME subtype (including structured types
// like "svg+xml" and "vnd.foo") and <payload> is base64 in either the
// standard alphabet ("+/=") or URL-safe alphabet ("-_=") — the regex
// accepts both because real-world emitters (browser data URIs, Python
// base64.urlsafe_b64encode) mix them. Validation of the actual bytes
// is the driver job; the regex is intentionally permissive about the
// alphabet but strict about the "data:image/...;base64," prefix.
//
// Note: this regex requires ";base64," immediately after the subtype.
// It does NOT accept ";charset=utf-8;base64," or other parameter-prefixed
// forms — those are uncommon in canvas inputs and deferred.
var dataImageRe = regexp.MustCompile(`data:image/[a-zA-Z0-9.+-]+;base64,[A-Za-z0-9+/=_-]+`)

// extractDataImages scans the input strings for data:image/*
// base64 URIs and returns the deduplicated set in first-seen
// order. The current implementation only walks top-level string
// values; recursive walk over nested structs/lists is a future
// enhancement (Python _extract_data_images covers the recursive
// case).
func extractDataImages(values []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, v := range values {
		for _, m := range dataImageRe.FindAllString(v, -1) {
			if _, dup := seen[m]; dup {
				continue
			}
			seen[m] = struct{}{}
			out = append(out, m)
		}
	}
	return out
}

// prependHistory inserts up to `window` prior turns from the canvas
// history before the current system+user messages. Each history entry
// is a {role, content} map; only the last `window` are kept, with
// assistant/user roles preserved. Invalid entries (missing role or
// content) are skipped silently.
func prependHistory(current []ComponentMessage, history []map[string]any, window int) []ComponentMessage {
	if window <= 0 || len(history) == 0 {
		return current
	}
	start := 0
	if len(history) > window {
		start = len(history) - window
	}
	out := make([]ComponentMessage, 0, len(current)+(len(history)-start))
	for i := start; i < len(history); i++ {
		entry := history[i]
		role, _ := entry["role"].(string)
		content, _ := entry["content"].(string)
		if role == "" || content == "" {
			continue
		}
		out = append(out, ComponentMessage{Role: role, Content: content})
	}
	return append(out, current...)
}

// buildMessagesWithImages assembles a system + user message sequence,
// attaching data:image URIs as multi-modal content parts when
// present. Without images the function is identical to buildMessages.
//
// When cite is true, the citation-instruction prompt is appended to the
// system message (creating one if it was empty). This mirrors the
// Python LLM._prepare_prompt_variables path where cite=True
// triggers `citation_prompt()` injection. The post-stream
// grounding call (Python _gen_citations_async) is the
// RetrievalService-driven citation enhancement.
//
// Each image is wrapped in a MessageInputPart{Type: "image_url",
// Image: &MessageInputImage{MessagePartCommon{URL: dataURI}}}. The
// driver layer (anthropic.go:254, google.go:168) recognises the
// "image_url" part type and translates to the provider-native format.
// Using URL (rather than splitting into Base64Data + MIMEType) keeps the
// data URI intact, which matches the existing anthropic_test.go:221
// fixture format.
func buildMessagesWithImages(system, user string, images []string, cite bool) []ComponentMessage {
	if cite {
		system = injectCitationPrompt(system)
	}
	msgs := make([]ComponentMessage, 0, 2)
	if system != "" {
		msgs = append(msgs, NewSystemMessage(system))
	}
	userMsg := NewUserMessage(user)
	if len(images) > 0 {
		parts := make([]ComponentMessagePart, 0, 1+len(images))
		parts = append(parts, ComponentMessagePart{Type: "text", Text: user})
		for _, uri := range images {
			parts = append(parts, ComponentMessagePart{Type: "image_url", ImageURL: uri})
		}
		userMsg.MultiContent = parts
		userMsg.Content = "" // text is in MultiContent parts
	}
	msgs = append(msgs, userMsg)
	return msgs
}

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
	if v, ok := mapFrom(inputs, "output_structure"); ok {
		p.OutputStructure = v
	}
	if v, ok := boolFrom(inputs, "cite"); ok {
		p.Cite = v
	}
	if v, ok := intFrom(inputs, "message_history_window_size"); ok {
		p.MessageHistoryWindowSize = v
	}
	if v, ok := mapFrom(inputs, "chat_template_kwargs"); ok {
		p.ChatTemplateKwargs = v
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
	if v, ok := floatFrom(inputs, "top_p"); ok {
		f := v
		p.TopP = &f
	}
	// visual_files: accept []string or single string with embedded
	// data URIs. The current implementation only walks top-level
	// string values; recursive walk is a future enhancement.
	if v, ok := sliceFrom(inputs, "visual_files"); ok {
		p.VisualFiles = extractDataImages(v)
	} else if v, ok := stringFrom(inputs, "visual_files"); ok {
		p.VisualFiles = extractDataImages([]string{v})
	}
	if v, ok := intFrom(inputs, "max_tokens"); ok {
		i := v
		p.MaxTokens = &i
	}
	if v, ok := stringFrom(inputs, "thinking"); ok {
		if v != "" && v != "default" {
			p.Thinking = v
		}
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

// mapFrom extracts a map[string]any from inputs[name]. Accepts the
// canonical map[string]any shape (the shape produced by
// json.Unmarshal into a map). For OutputStructure we only need the
// top-level shape — schema-validation against the inner types is
// deferred to a future phase.
func mapFrom(inputs map[string]any, name string) (map[string]any, bool) {
	v, ok := inputs[name]
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
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
		if v, ok := floatFrom(params, "top_p"); ok {
			f := v
			p.TopP = &f
		}
		if v, ok := intFrom(params, "max_tokens"); ok {
			i := v
			p.MaxTokens = &i
		}
		if v, ok := boolFrom(params, "json_output"); ok {
			p.JSONOutput = v
		}
		if v, ok := mapFrom(params, "output_structure"); ok {
			p.OutputStructure = v
		}
		// cite defaults to true (matches Python) when neither LLMParam
		// nor inputs set it.
		p.Cite = true
		if v, ok := boolFrom(params, "cite"); ok {
			p.Cite = v
		}
		if v, ok := intFrom(params, "message_history_window_size"); ok {
			p.MessageHistoryWindowSize = v
		}
		if v, ok := mapFrom(params, "chat_template_kwargs"); ok {
			p.ChatTemplateKwargs = v
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

// cleanFormattedAnswer mirrors Python's clean_formated_answer():
//
//  1. Strip everything up to and including </think> (dotall).
//  2. Strip everything up to and including ```json (dotall).
//  3. Strip trailing ``` and optional newlines.
//
// This removes DeepSeek-R1-style thinking blocks and JSON-fence
// prefixes/suffixes from the raw model response.
var (
	reThinkPrefix     = regexp.MustCompile(`(?s)^.*</think>`)
	reJSONFencePrefix = regexp.MustCompile(`(?s)^.*` + "```json")
	reJSONFenceSuffix = regexp.MustCompile("```\n*$")
)

func cleanFormattedAnswer(ans string) string {
	ans = reThinkPrefix.ReplaceAllString(ans, "")
	ans = reJSONFencePrefix.ReplaceAllString(ans, "")
	ans = reJSONFenceSuffix.ReplaceAllString(ans, "")
	return ans
}

// newChatModelDriver resolves a provider driver by name and optionally
// overrides the base URL. Returns a ModelDriver ready for invocation.
func newChatModelDriver(driver, override string) (models.ModelDriver, error) {
	pm := models.GetProviderManager()
	if pm != nil {
		provider := pm.FindProvider(driver)
		if provider != nil && provider.ModelDriver != nil {
			modelDriver := provider.ModelDriver
			if strings.TrimSpace(override) != "" {
				modelDriver = modelDriver.NewInstance(
					map[string]string{
						"default": strings.TrimRight(override, "/"),
					},
				)
				if modelDriver == nil {
					return nil, fmt.Errorf("provider does not support a custom base_url")
				}
			}
			return modelDriver, nil
		}
	}

	// Dummy is an explicit test/development driver and has no provider config.
	if strings.EqualFold(driver, "dummy") {
		baseURL := map[string]string(nil)
		if strings.TrimSpace(override) != "" {
			baseURL = map[string]string{"default": strings.TrimRight(override, "/")}
		}
		return models.NewDummyModel(baseURL, models.URLSuffix{Chat: "chat/completions"}), nil
	}
	return nil, fmt.Errorf("provider is not configured")
}
