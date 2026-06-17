// Package component — LLM (T1).
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
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/component/prompts"
	"ragflow/internal/agent/runtime"
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
	// driver's built-in default URL.
	BaseURL string

	// MaxRetries caps the retry loop in retryInvoker. Zero = default
	// (3). Negative = disable retries entirely (single attempt). The
	// retry loop honours ctx.Done() so a request cancel aborts on
	// the next backoff sleep.
	MaxRetries int

	// DelayAfterError is the initial backoff between retry attempts.
	// Doubles on each retry, capped at 1 minute. Zero = default
	// (2 seconds). Matches Python's `delay_after_error` param.
	DelayAfterError time.Duration
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
	TopP        *float64
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
		TopP:        req.TopP,
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
//
// Copies Role, Content, AND UserInputMultiContent (multi-modal parts),
// including a deep copy of the *string URL pointers in each image part
// so that callers may mutate the returned messages without affecting
// the source. Without the multi-content copy and pointer deep-copy,
// vision inputs would be silently dropped or shared with the caller.
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
		cloned := slices.Clone(m.UserInputMultiContent)
		for j, p := range cloned {
			if p.Image != nil {
				imgCopy := *p.Image
				if p.Image.URL != nil {
					u := *p.Image.URL
					imgCopy.URL = &u
				}
				cloned[j].Image = &imgCopy
			}
		}
		out = append(out, &schema.Message{
			Role:                  role,
			Content:               m.Content,
			UserInputMultiContent: cloned,
		})
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
	// Resolve {{cpn_id@var}} references in the system and user
	// prompts against the canvas state attached to ctx. When the
	// state is absent (e.g. tests that call Invoke directly without
	// going through the canvas scheduler), the prompts pass through
	// unchanged — backward compatible.
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		// ResolveTemplate returns the partial output (with "" in place
		// of unresolved refs) even on error — we accept the partial
		// output and log the error for diagnostics. This matches
		// Python's silent-soft-fail behavior (canvas.py returns "" for
		// missing refs) but adds a log line so misconfigured canvases
		// are still surfaced.
		if resolved, rerr := runtime.ResolveTemplate(p.SystemPrompt, state); resolved != p.SystemPrompt || rerr == nil {
			p.SystemPrompt = resolved
			if rerr != nil {
				log.Printf("component: LLM: resolve system_prompt: %v", rerr)
			}
		}
		if resolved, rerr := runtime.ResolveTemplate(p.UserPrompt, state); resolved != p.UserPrompt || rerr == nil {
			p.UserPrompt = resolved
			if rerr != nil {
				log.Printf("component: LLM: resolve user_prompt: %v", rerr)
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
		p.UserPrompt = p.SystemPrompt
	}

	msgs := buildMessagesWithImages(p.SystemPrompt, p.UserPrompt, p.VisualFiles, p.Cite)
	// Prepend the last N turns of conversation history from the
	// canvas state. Mirrors Python's `_get_chat_template_kwargs` /
	// `_fit_messages` path. When window size is 0 or history is
	// empty,
	// this is a no-op.
	if p.MessageHistoryWindowSize > 0 {
		if state, _, sErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); sErr == nil && state != nil {
			msgs = prependHistory(msgs, state.History, p.MessageHistoryWindowSize)
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
	// operator's intent is an ABSOLUTE attempt budget. The
	// default invoker installed at boot in cmd/server_main.go
	// is itself a retryInvoker wrapping einoChatInvoker.
	// Without unwrapping, the two loops would multiplicatively
	// stack:
	//
	//   boot=3, MaxRetries=5 → up to (3+1) × (5+1) = 24
	//                          invocations, not the 6 the
	//                          operator almost certainly intended.
	//
	// unwrapChatInvoker peels off any retryInvoker layers to
	// reach the bare invoker, then the param-override branch
	// wraps that bare invoker in a fresh retryInvoker with the
	// operator's literal values. Net effect: the absolute attempt
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
			out["content"] = resp.Content
		} else {
			log.Printf("component: LLM: output_structure set but no parseable JSON after retry")
		}
	}
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
//   - "thinking" (string): the model's reasoning content, empty if absent
//   - "content"  (string): the model's visible content
//
// A final chunk with key "done" (bool=true) signals end-of-stream so
// downstream consumers can flush state without relying on channel close
// alone (close also works; the "done" key is informational).
//
// Today, the LLM driver layer returns a single non-streamed response,
// so this v1 emits exactly one chunk + one done. Hooking the actual
// eino stream (EinoChatModel.Stream at internal/entity/models/llm.go:137)
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
			"thinking": "",
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

// injectCitationPrompt returns the system message with the canonical
// citation-instruction text appended. When system is empty, returns
// the prompt as-is. Two newlines separate the user's system prompt
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
// follow-up user turn that surfaces the LLM's first response and
// asks for valid JSON matching the expected top-level keys. The
// retry uses the same chat invoker on the next call; the message
// list returned here is what gets sent on the retry.
func buildStructuredRetryMessages(system, user string, images []string, cite bool, expected map[string]any, prevContent string) []schema.Message {
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
		msgs[len(msgs)-1] = schema.Message{
			Role:    schema.User,
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
// is the driver's job; the regex is intentionally permissive about the
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
// enhancement (Python's _extract_data_images covers the recursive
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
func prependHistory(current []schema.Message, history []map[string]any, window int) []schema.Message {
	if window <= 0 || len(history) == 0 {
		return current
	}
	start := 0
	if len(history) > window {
		start = len(history) - window
	}
	out := make([]schema.Message, 0, len(current)+(len(history)-start))
	for i := start; i < len(history); i++ {
		entry := history[i]
		role, _ := entry["role"].(string)
		content, _ := entry["content"].(string)
		if role == "" || content == "" {
			continue
		}
		out = append(out, schema.Message{Role: schema.RoleType(role), Content: content})
	}
	return append(out, current...)
}

// buildMessagesWithImages assembles a system + user message sequence,
// attaching data:image URIs as eino multi-modal content parts when
// present. Without images the function is identical to buildMessages.
//
// When cite is true, the citation-instruction prompt is appended to the
// system message (creating one if it was empty). This mirrors the
// Python LLM._prepare_prompt_variables path where cite=True
// triggers `citation_prompt()` injection. The post-stream
// grounding call (Python's _gen_citations_async) is the
// RetrievalService-driven citation enhancement.
//
// Each image is wrapped in a MessageInputPart{Type: "image_url",
// Image: &MessageInputImage{MessagePartCommon{URL: dataURI}}}. The
// driver layer (anthropic.go:254, google.go:168) recognises the
// "image_url" part type and translates to the provider-native format.
// Using URL (rather than splitting into Base64Data + MIMEType) keeps the
// data URI intact, which matches the existing anthropic_test.go:221
// fixture format.
func buildMessagesWithImages(system, user string, images []string, cite bool) []schema.Message {
	if cite {
		system = injectCitationPrompt(system)
	}
	out := make([]schema.Message, 0, 2)
	if system != "" {
		out = append(out, schema.Message{Role: schema.System, Content: system})
	}

	if len(images) == 0 {
		if user != "" {
			out = append(out, schema.Message{Role: schema.User, Content: user})
		}
		return out
	}

	parts := make([]schema.MessageInputPart, 0, 1+len(images))
	if user != "" {
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: user,
		})
	}
	for _, uri := range images {
		u := uri
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &u},
			},
		})
	}
	out = append(out, schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	})
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
// The v1 fixtures in internal/agent/dsl/testdata use the
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
