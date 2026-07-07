// Package component — Agent (T1).
//
// Multi-turn ReAct agent powered by eino's flow/agent/react package.
// Uses the RAGFlow model layer (models.EinoChatModel) as a
// ToolCallingChatModel, delegating the ReAct loop to eino's
// production-grade implementation.
//
// Public outputs (content / tool_calls / artifacts) match the
// plan-specified shape. The agent now wires AgentParam.Tools into
// eino's native react.AgentConfig.ToolsConfig; when no tools are
// configured the ReAct loop naturally degenerates to one model call.
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/component/prompts"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/entity/models"

	"go.uber.org/zap"
)

// agentLLMIDPattern matches `<model>@<provider>` and
// `<model>@<instance>@<provider>` (the trailing `@<provider>` is
// always the last segment — the segment just before the last `@`
// is treated as the bare model name for upstream API calls). The
// browser component has the same idea at browser.go:88-92, but
// keeps the regex greedy for its 2-part fixture; we keep both
// behaviours here via the in-function split below.

// agentProviderLastSegmentSplit takes a composite llm_id and
// returns (bareModelName, providerName, true) — or ("", "", false)
// when no `@<provider>` suffix exists. The bare model name is
// always `parts[0]` (the FIRST `@`-delimited segment); the
// provider is `parts[1]` for the 2-part shape and `parts[2]` for
// the 3+ shape. Any middle `@<seg>` segments (the "instance" in
// Python's split_model_name) are intentionally dropped — the Go
// drivers and the tenant_llm lookup both key on the bare model
// name + factory, not on the instance.
//
// Mirrors Python's split_model_name at
// api/db/joint_services/tenant_model_service.py:163-178:
//   - "model"                     → ("model", "",       false)
//   - "model@provider"            → ("model", "provider", true)
//   - "model@instance@provider"   → ("model", "provider", true)
//   - 4+ parts                    → ("parts[0]", "parts[2]", true) —
//     the trailing segment wins, anything between instance and
//     provider is dropped (Python uses parts[2] unconditionally).
func agentProviderLastSegmentSplit(s string) (modelName, providerName string, hasProvider bool) {
	return splitCompositeLLMID(s)
}

// AgentComponent is a multi-turn ReAct agent.
type AgentComponent struct {
	param AgentParam
}

// AgentParam captures the (resolved) DSL parameters for an Agent node.
type AgentParam struct {
	ModelID               string
	SystemPrompt          string
	UserPrompt            string
	TopP                  *float64
	Tools                 []string                  // Agent-visible tool names resolved into Eino BaseTool instances
	ToolParams            map[string]map[string]any // node-level tool constructor params keyed by tool name
	MaxRounds             int
	OptimizeMultiTurn     bool // when true (default), multi-turn history is condensed via full_question LLM call
	OptimizeHistoryWindow int  // number of history turns to include in the optimization prompt (default 3)
	// Meta is the OpenAI-style function-call schema the Agent exposes
	// when it is itself called as a tool by a parent component. Mirrors
	// Python's `meta: ToolMeta` field — describes the Agent's own
	// inputs (user_prompt / reasoning / context) for callers.
	Meta AgentMeta
	// Cite enables post-stream citation grounding. When true,
	// the Agent reads the chunks recorded in
	// state.Retrieval["chunks"] (populated by the Retrieval tool),
	// renders prompts.CitationPlusPrompt, and makes a second LLM
	// call to insert [ID:N] tags into the final content. Mirrors
	// Python's `_generate_with_citation` flow.
	Cite    bool
	Driver  string
	APIKey  string
	BaseURL string
}

const agentUserPromptSchemaDefault = "This is the order you need to send to the agent."

// AgentMeta declares the OpenAI-style function-call interface for the
// Agent component. Mirrors ragflow Python's ToolMeta shape.
type AgentMeta struct {
	Name        string
	Description string
	// Parameters is the JSON-Schema-shaped object describing the
	// Agent's own input parameters. Each key is the parameter name
	// (e.g. "user_prompt", "reasoning", "context") and the value
	// carries type/description/required.
	Parameters map[string]AgentMetaParam
}

// AgentMetaParam is a single field in the Agent's input schema.
type AgentMetaParam struct {
	Type        string
	Description string
	Required    bool
}

// AgentOutput mirrors the outputs map (per plan §2.11.3 row 8):
//
//	"content"     string
//	"tool_calls"  []map[string]any  (one entry per tool call observed)
//	"artifacts"   []map[string]any  (collected from tool responses — empty in P0)
type AgentOutput struct {
	Content   string
	ToolCalls []map[string]any
	Artifacts []map[string]any
}

// agentRunner is the package-level ReAct runner. The production value
// delegates to eino's flow/agent/react. Tests replace it with a function
// that returns canned *schema.Message values.
var agentRunner = runEinoReActAgent

// runEinoReActAgent creates an eino react agent and runs it against the
// model built from p.
func runEinoReActAgent(ctx context.Context, p AgentParam) (*schema.Message, error) {
	chatModel, err := buildAgentChatModel(p)
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
	}
	tools, err := buildAgentTools(p)
	if err != nil {
		return nil, fmt.Errorf("build tools: %w", err)
	}

	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MessageModifier: func(ctx context.Context, msgs []*schema.Message) []*schema.Message {
			if p.SystemPrompt != "" {
				return append([]*schema.Message{schema.SystemMessage(p.SystemPrompt)}, msgs...)
			}
			return msgs
		},
		MaxStep: p.MaxRounds,
	})
	if err != nil {
		return nil, fmt.Errorf("create react agent: %w", err)
	}

	input := []*schema.Message{schema.UserMessage(p.UserPrompt)}
	opt, future := react.WithMessageFuture()
	ctx = setArtifactCollector(ctx, future)
	msg, err := agent.Generate(ctx, input, opt)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// addToolCallMemory summarizes the tool calls observed in msg via
// a small LLM call and returns a one-line history entry. Mirrors
// Python's `add_memory(user, assist, func_name, params, results,
// user_defined_prompt)` — the LLM condenses the tool usage into a
// short, memory-worthy sentence.
//
// When the LLM call fails or there are no tool calls, the function
// returns ("", nil) and the caller skips appending to history.
func addToolCallMemory(ctx context.Context, p AgentParam, msg *schema.Message) (string, error) {
	calls := extractToolCalls(msg)
	if len(calls) == 0 {
		return "", nil
	}
	// Format a compact summary of the calls.
	var callsDesc strings.Builder
	for i, c := range calls {
		if i > 0 {
			callsDesc.WriteString("; ")
		}
		fmt.Fprintf(&callsDesc, "%s(%v)", c["name"], c["arguments"])
	}
	system := "You are a memory summarizer. Given a list of tool calls the assistant just made, output ONE short sentence (max 30 words) describing what the assistant did, suitable for a future-turn conversation history. Output ONLY the sentence, no preamble, no quotes."
	user := "Tool calls: " + callsDesc.String()
	inv := getDefaultChatInvoker()
	resp, err := inv.Invoke(ctx, ChatInvokeRequest{
		Driver:    p.Driver,
		ModelName: p.ModelID,
		APIKey:    p.APIKey,
		BaseURL:   p.BaseURL,
		Messages: []schema.Message{
			{Role: schema.System, Content: system},
			{Role: schema.User, Content: user},
		},
		TopP: p.TopP,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

// applyCitationGrounding is the post-stream citation grounding
// call. It reads the chunks recorded in state.Retrieval["chunks"]
// (populated by the Retrieval tool), renders
// prompts.CitationPlusPrompt, and makes a second LLM call asking
// the model to insert [ID:N] tags into the assistant's final
// content.
//
// Returns the grounded content on success, the original content
// unchanged when no chunks are available or the call fails. Mirrors
// Python's `cite_letter` / `generate_with_citation` flow.
func applyCitationGrounding(ctx context.Context, p AgentParam, content string, chunks []prompts.CitationSource) (string, error) {
	if !p.Cite {
		return content, nil
	}
	if len(chunks) == 0 {
		return content, nil
	}
	if strings.TrimSpace(content) == "" {
		return content, nil
	}
	systemPrompt, _ := prompts.CitationPlusPrompt(chunks)
	inv := getDefaultChatInvoker()
	resp, err := inv.Invoke(ctx, ChatInvokeRequest{
		Driver:    p.Driver,
		ModelName: p.ModelID,
		APIKey:    p.APIKey,
		BaseURL:   p.BaseURL,
		Messages: []schema.Message{
			{Role: schema.System, Content: systemPrompt},
			{Role: schema.User, Content: content},
		},
		TopP: p.TopP,
	})
	if err != nil {
		// Grounding is best-effort. Return the original content
		// so the message still flows; the caller can decide
		// whether to surface the error.
		return content, err
	}
	grounded := strings.TrimSpace(resp.Content)
	if grounded == "" {
		return content, nil
	}
	return grounded, nil
}

// chunksFromState extracts the recorded retrieval chunks from
// the canvas state in ctx. Returns nil when the state or the
// chunks key is absent / empty. The returned slice is shaped
// for prompts.CitationSource — the grounding renderer.
func chunksFromState(ctx context.Context) []prompts.CitationSource {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return nil
	}
	raw := state.GetRetrievalChunks()
	if len(raw) == 0 {
		return nil
	}
	out := make([]prompts.CitationSource, 0, len(raw))
	for _, m := range raw {
		id, _ := m["id"].(string)
		content, _ := m["content"].(string)
		if id == "" || content == "" {
			continue
		}
		out = append(out, prompts.CitationSource{ID: id, Content: content})
	}
	return out
}

// GetInputForm aggregates the Agent's own user-callable parameters with each
// sub-tool's input form. Mirrors Python's `Agent.get_input_form`, which
// returns a flat field-definition map keyed by input name.
//
// Today the sub-tool input forms are aggregated via an optional
// `InputForm() map[string]any` method on the eino tool (when tools
// implement it); tools that don't expose a structured input form
// are skipped silently.
func (c *AgentComponent) GetInputForm() map[string]any {
	out := extractAgentPromptInputForm(c.param.SystemPrompt, c.param.UserPrompt)
	tools, err := buildAgentTools(c.param)
	if err != nil {
		return out
	}
	ctx := context.Background()
	for _, t := range tools {
		info, ierr := t.Info(ctx)
		name := ""
		if ierr == nil && info != nil {
			name = info.Name
		}
		if name == "" {
			continue
		}
		if formGetter, ok := t.(interface{ InputForm() map[string]any }); ok {
			out[name] = formGetter.InputForm()
		}
	}
	return out
}

func extractAgentPromptInputForm(systemPrompt, userPrompt string) map[string]any {
	out := map[string]any{}
	seen := map[string]struct{}{}
	matches := append(runtime.VarRefPattern.FindAllStringSubmatch(systemPrompt, -1), runtime.VarRefPattern.FindAllStringSubmatch(userPrompt, -1)...)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := strings.TrimSpace(match[1])
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out[key] = map[string]any{
			"type":     "line",
			"name":     key,
			"optional": false,
		}
	}
	return out
}

func sortedAgentPromptInputKeys(systemPrompt, userPrompt string) []string {
	form := extractAgentPromptInputForm(systemPrompt, userPrompt)
	keys := make([]string, 0, len(form))
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Reset calls Reset on every sub-tool that implements the Resetter
// interface. Mirrors Python's per-tool reset() — useful for clearing
// per-invocation state (caches, scratch buffers) between calls.
func (c *AgentComponent) Reset() {
	tools, err := buildAgentTools(c.param)
	if err != nil {
		return
	}
	for _, t := range tools {
		if r, ok := t.(interface{ Reset() }); ok {
			r.Reset()
		}
	}
}

// optimizeMultiTurnQuestion asks the LLM to rephrase the current user
// prompt into a self-contained question that doesn't require the
// conversation history to understand. Mirrors Python's `full_question`
// LLM pass.
//
// Returns the original prompt unchanged if:
//   - history has < 2 entries (no prior turns to fold in)
//   - the rephrase LLM call fails
//
// Window defaults to AgentParam.OptimizeHistoryWindow (3) when zero.
func optimizeMultiTurnQuestion(ctx context.Context, p AgentParam, history []map[string]any) (string, error) {
	window := p.OptimizeHistoryWindow
	if window <= 0 {
		window = 3
	}
	if len(history) < 2 {
		return "", nil
	}
	start := 0
	if len(history) > window {
		start = len(history) - window
	}
	var histBuf strings.Builder
	for i := start; i < len(history); i++ {
		e := history[i]
		role, _ := e["role"].(string)
		content, _ := e["content"].(string)
		if role == "" || content == "" {
			continue
		}
		fmt.Fprintf(&histBuf, "%s: %s\n", role, content)
	}
	if histBuf.Len() == 0 {
		return "", nil
	}
	system := "You are a question rephraser. Given conversation history and the user's latest input, rewrite the latest input as a self-contained question that does not require the history to understand. Output ONLY the rephrased question, no preamble, no quotes."
	user := "Conversation history:\n" + histBuf.String() + "\n\nUser's latest input:\n" + p.UserPrompt
	inv := getDefaultChatInvoker()
	resp, err := inv.Invoke(ctx, ChatInvokeRequest{
		Driver:    p.Driver,
		ModelName: p.ModelID,
		APIKey:    p.APIKey,
		BaseURL:   p.BaseURL,
		Messages: []schema.Message{
			{Role: schema.System, Content: system},
			{Role: schema.User, Content: user},
		},
		TopP: p.TopP,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func buildAgentTools(p AgentParam) ([]einotool.BaseTool, error) {
	return agenttool.BuildAll(p.Tools, p.ToolParams)
}

// NewAgentComponent builds an AgentComponent from raw params.
func NewAgentComponent(p AgentParam) *AgentComponent {
	if p.MaxRounds <= 0 {
		p.MaxRounds = 3
	}
	return &AgentComponent{param: p}
}

// Name returns the registered component name.
func (c *AgentComponent) Name() string { return "Agent" }

// Invoke runs the ReAct loop via the configured agentRunner and returns
// the output map.
func (c *AgentComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	p := mergeAgentParam(c.param, inputs)
	hasRuntimeUserPrompt := false
	if v, ok := stringFrom(inputs, "user_prompt"); ok {
		hasRuntimeUserPrompt = !shouldFallbackToSysQuery(v)
	}

	// v3.6.1: derive the driver and bare model name from the
	// composite llm_id when the Agent DSL didn't set `driver`. The
	// Python side does the same in split_model_name at
	// api/db/joint_services/tenant_model_service.py:163-178. We
	// also use this opportunity to look up the tenant's LLM
	// credentials from `tenant_llm` when the DSL omitted `api_key`
	// — mirrors Python's get_model_config_from_provider_instance,
	// which is how the Python canvas finds the tenant's
	// provider-specific API key + base URL without storing them
	// in the canvas DSL.
	// Save the original composite llm_id before the split drops the
	// instance-name segment. We need it for the tenant_model_instance
	// fallback path below.
	originalModelID := p.ModelID

	if p.Driver == "" && p.ModelID != "" {
		if m, prov, ok := agentProviderLastSegmentSplit(p.ModelID); ok {
			p.Driver = prov
			p.ModelID = m
		}
	}
	p.APIKey, p.BaseURL = resolveTenantLLMConfig(ctx, p.Driver, p.ModelID, p.APIKey, p.BaseURL, originalModelID)

	var state *runtime.CanvasState
	if s, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && s != nil {
		state = s
		if resolved, rerr := runtime.ResolveTemplate(p.SystemPrompt, state); resolved != p.SystemPrompt || rerr == nil {
			p.SystemPrompt = resolved
			if rerr != nil {
				common.Debug("agent: resolve system_prompt", zap.Error(rerr))
			}
		}
		if resolved, rerr := runtime.ResolveTemplate(p.UserPrompt, state); resolved != p.UserPrompt || rerr == nil {
			p.UserPrompt = resolved
			if rerr != nil {
				common.Debug("agent: resolve user_prompt", zap.Error(rerr))
			}
		}
	}
	if hasRuntimeUserPrompt {
		p.UserPrompt = formatAgentRuntimePrompt(inputs, p.UserPrompt)
	} else if shouldFallbackToSysQuery(p.UserPrompt) && state != nil {
		if query, ok := stringFromState(state, "query"); ok {
			p.UserPrompt = query
		}
	}

	if p.ModelID == "" {
		return nil, &ParamError{Field: "model_id", Reason: "required"}
	}
	if p.UserPrompt == "" && p.SystemPrompt == "" {
		return nil, &ParamError{Field: "user_prompt", Reason: "at least one of user_prompt or system_prompt must be set"}
	}
	// v1 fixtures sometimes ship only a system prompt. Fall back to
	// using the system text as the user message so the underlying
	// chat call still has something to send to the model.
	if p.UserPrompt == "" {
		p.UserPrompt = p.SystemPrompt
	}

	// Multi-turn conversation optimization. When the canvas state
	// carries prior history and OptimizeMultiTurn is enabled
	// (default), rephrase the user prompt into a self-contained
	// question via a dedicated LLM call. The rephrased prompt is
	// what the Agent runner actually consumes.
	if p.OptimizeMultiTurn {
		if state, _, sErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); sErr == nil && state != nil {
			if rephrased, err := optimizeMultiTurnQuestion(ctx, p, state.History); err == nil && rephrased != "" {
				p.UserPrompt = rephrased
			}
		}
	}

	msg, err := agentRunner(ctx, p)
	// Tool-call memory summarization. After the ReAct loop
	// completes, summarize the tool calls via an LLM and append to
	// the canvas state's History so downstream turns (history
	// window) see the prior tool usage as prior assistant turns.
	if err == nil && msg != nil {
		if state, _, sErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); sErr == nil && state != nil {
			if summary, sErr2 := addToolCallMemory(ctx, p, msg); sErr2 == nil && summary != "" {
				state.History = append(state.History, map[string]any{
					"role":    "assistant",
					"content": summary,
				})
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("component: Agent.Invoke: %w", err)
	}
	// Post-stream citation grounding. When Cite is enabled and
	// the canvas state has recorded retrieval chunks (populated
	// by the Retrieval tool during the ReAct loop), make a second
	// LLM call to insert [ID:N] tags into the final content. The
	// grounding call is best-effort — on failure the original
	// content is kept and the error is surfaced under
	// outputs["grounding_error"].
	// Diagnostic sentinel (temporary — see plan): log the post-
	// agentRunner state right before the `msg.Content` deref so a
	// subsequent panic shows whether the agent returned (nil, nil).
	if msg == nil {
		common.Debug("agent.Invoke: msg is NIL after agentRunner",
			zap.String("driver", p.Driver),
			zap.String("modelID", p.ModelID),
			zap.Int("userPrompt_len", len(p.UserPrompt)),
			zap.Error(err))
		return nil, fmt.Errorf("component: Agent.Invoke: agent runner returned nil message (driver=%q modelID=%q): %w", p.Driver, p.ModelID, err)
	}
	common.Debug("agent.Invoke: msg OK",
		zap.String("driver", p.Driver),
		zap.String("modelID", p.ModelID),
		zap.Int("content_len", len(msg.Content)))
	content := msg.Content
	var groundingStatus string
	if p.Cite {
		chunks := chunksFromState(ctx)
		if len(chunks) == 0 {
			groundingStatus = "no_chunks"
		} else {
			grounded, gErr := applyCitationGrounding(ctx, p, content, chunks)
			if gErr == nil && grounded != content {
				content = grounded
				groundingStatus = "applied"
			} else if gErr != nil {
				groundingStatus = "error: " + gErr.Error()
			}
		}
	}
	artifacts := collectArtifactsFromToolCalls(ctx, msg)
	artifactMD := formatArtifactMarkdown(artifacts, content)
	out := map[string]any{
		"content":    content + artifactMD,
		"tool_calls": extractToolCalls(msg),
		"artifacts":  artifacts,
	}
	if groundingStatus != "" {
		out["grounding_status"] = groundingStatus
	}
	return out, nil
}

// Stream implements Component.Stream. Mirrors Invoke then pushes the
// single payload through the channel.
func (c *AgentComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
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
func (c *AgentComponent) Inputs() map[string]string {
	return map[string]string{
		"model_id":                "Provider-side model identifier (e.g. \"gpt-4o-mini\")",
		"system_prompt":           "Optional system prompt",
		"user_prompt":             "User prompt; supports {{cpn_id@param}} references",
		"top_p":                   "Top-p (nucleus) sampling cutoff (0.0-1.0). Optional.",
		"tools":                   "List of tool names to make available to the ReAct agent.",
		"tool_params":             "Optional node-level tool constructor params keyed by tool name (e.g. execute_sql DB config).",
		"max_rounds":              "Maximum ReAct rounds (default 3).",
		"optimize_multi_turn":     "When true (default), multi-turn history is condensed via full_question LLM call.",
		"optimize_history_window": "Number of history turns to include in the optimization prompt (default 3).",
		"driver":                  "Provider driver name",
		"api_key":                 "Override API key for this call.",
		"base_url":                "Override the driver default endpoint URL.",
		"cite":                    "When true, make a post-stream citation-grounding call (reads chunks from state.Retrieval).",
	}
}

// Outputs returns output metadata.
func (c *AgentComponent) Outputs() map[string]string {
	return map[string]string{
		"content":          "Final assistant content (after the ReAct loop terminates)",
		"tool_calls":       "One entry per tool call observed during the run",
		"artifacts":        "Artifacts collected from tool responses (empty in P0)",
		"grounding_status": "'applied' | 'no_chunks' | 'error: <msg>' (present when cite=true).",
	}
}

// buildAgentChatModel constructs an EinoChatModel from AgentParam by
// resolving the driver through the RAGFlow provider manager.
func buildAgentChatModel(p AgentParam) (*models.EinoChatModel, error) {
	driver := p.Driver
	modelID := p.ModelID

	// When the Agent DSL omits `driver`, derive it from the composite
	// llm_id format. The RAGFlow DSL stores the model identifier as
	// "<model>@<instance>@<provider>" (mirrors Python's
	// split_model_name at
	// api/db/joint_services/tenant_model_service.py:163-178 and the
	// Go-side SplitModelNameAndFactory at
	// internal/service/tenant.go:168). Two-part
	// "<model>@<provider>" and bare "<model>" are also accepted —
	// bare means no driver known, which falls through to the dummy
	// driver below. The trailing "@<provider>" suffix must also be
	// stripped from the model id before passing to the driver — the
	// upstream APIs (ZhipuAI, OpenAI, …) do not accept composite
	// names and would 400 on the "@<provider>" tail.
	if driver == "" && modelID != "" {
		if bareModelName, providerName, ok := splitCompositeLLMID(modelID); ok {
			driver = providerName
			modelID = bareModelName
		}
	}
	if driver == "" {
		driver = "dummy"
	}
	baseURL := baseURLMapForDriver(driver, p.BaseURL)
	// urlSuffix: see chatURLSuffixFor in llm.go for the rationale.
	// The factory's NewModelDriver stores URLSuffix verbatim; the
	// driver then appends URLSuffix.Chat to baseURL to build the
	// chat-completions endpoint, so an empty suffix leaves the URL
	// pointing at the v1 root (404). Seed the right suffix per
	// driver so the agent's ReAct loop hits a working endpoint.
	d, err := models.NewModelFactory().CreateModelDriver(driver, baseURL, chatURLSuffixFor(driver))
	if err != nil {
		return nil, fmt.Errorf("resolve driver %q: %w", driver, err)
	}
	if d == nil {
		return nil, fmt.Errorf("no driver for %q", driver)
	}
	apiKey := p.APIKey
	cfg := &models.APIConfig{ApiKey: &apiKey}
	cm := models.NewChatModel(d, &modelID, cfg)
	// ChatConfig construction is conditional on TopP being set, unlike
	// the LLM path which always builds a ChatConfig (Temperature/MaxTokens
	// pass-through). The asymmetry is intentional: AgentParam has no
	// Temperature/MaxTokens yet, so building a zero-config ChatConfig
	// would be dead weight. When AgentParam grows Temperature/
	// MaxTokens, switch to always-build.
	var chatCfg *models.ChatConfig
	if p.TopP != nil {
		chatCfg = &models.ChatConfig{TopP: p.TopP}
	}
	return models.NewEinoChatModel(cm, chatCfg), nil
}

// artifactEntry is the shape of a single tool-returned artifact
// surfaced through the Agent's outputs["artifacts"].
type artifactEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// artifactCollectorKey is the context key used to stash the
// MessageFuture from react.WithMessageFuture() so the AgentComponent
// can collect artifacts after the ReAct loop finishes. The collector
// is created per-invocation in runEinoReActAgent.
type artifactCollectorKey struct{}

// setArtifactCollector registers the MessageFuture for this agent run
// in the context. It is called from runEinoReActAgent after
// react.WithMessageFuture() returns a future.
func setArtifactCollector(ctx context.Context, future react.MessageFuture) context.Context {
	return context.WithValue(ctx, artifactCollectorKey{}, future)
}

// getArtifactCollector retrieves the MessageFuture registered for the
// current agent run. Returns nil when no collector was registered
// (e.g., tests that stub agentRunner).
func getArtifactCollector(ctx context.Context) react.MessageFuture {
	v := ctx.Value(artifactCollectorKey{})
	if v == nil {
		return nil
	}
	if f, ok := v.(react.MessageFuture); ok {
		return f
	}
	return nil
}

// collectArtifactsFromToolCalls drains the MessageFuture stored in
// ctx (if any) and extracts artifact entries from every tool response
// message that carries a `_ARTIFACTS` payload in its Extra field.
// The final message is ignored because it is an assistant message and
// does not contain tool results. Returns a de-duplicated list ordered
// by first appearance.
//
// The expected payload shape in each tool response is:
//
//	{ "_ARTIFACTS": [{ "name": "report.pdf", "url": "https://..." }, ...] }
func collectArtifactsFromToolCalls(ctx context.Context, _ *schema.Message) []artifactEntry {
	future := getArtifactCollector(ctx)
	if future == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var out []artifactEntry

	iter := future.GetMessages()
	for {
		msg, ok, err := iter.Next()
		if err != nil {
			common.Debug("agent: artifact collection iterator error", zap.Error(err))
			break
		}
		if !ok {
			break
		}
		if msg == nil || msg.Role != schema.Tool {
			continue
		}
		rawArtifacts := extractArtifactsFromToolMessage(msg)
		for _, a := range rawArtifacts {
			if a.URL == "" || a.Name == "" {
				continue
			}
			if _, exists := seen[a.URL]; exists {
				continue
			}
			seen[a.URL] = struct{}{}
			out = append(out, a)
		}
	}
	return out
}

// extractArtifactsFromToolMessage parses the JSON payload of a tool
// response message and returns the `_ARTIFACTS` list. The payload is
// read from msg.Content when it is non-empty; otherwise the first text
// element of msg.UserInputMultiContent is used. This matches the eino
// tool contract where tool results are delivered as a string.
func extractArtifactsFromToolMessage(msg *schema.Message) []artifactEntry {
	payload := msg.Content
	if payload == "" && len(msg.UserInputMultiContent) > 0 {
		payload = toolMessageTextContent(msg)
	}
	if payload == "" {
		return nil
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil
	}

	raw, ok := envelope["_ARTIFACTS"].([]any)
	if !ok {
		return nil
	}

	out := make([]artifactEntry, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		url, _ := m["url"].(string)
		if name == "" || url == "" {
			continue
		}
		out = append(out, artifactEntry{Name: name, URL: url})
	}
	return out
}

// toolMessageTextContent returns the first text content part of a tool
// message, or an empty string if no text part is found.
func toolMessageTextContent(msg *schema.Message) string {
	for i := range msg.UserInputMultiContent {
		part := &msg.UserInputMultiContent[i]
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			return part.Text
		}
	}
	return ""
}

// formatArtifactMarkdown renders a slice of artifacts as markdown
// links, omitting URLs already present in the existing text (Python's
// `_collect_tool_artifact_markdown` does the same de-duplication).
//
// Format:
//   - image URL  → ![name](url)
//   - other URL  → [Download name](url)
//
// Returns the empty string when no artifacts are present, so callers
// can safely concatenate without guarding.
func formatArtifactMarkdown(artifacts []artifactEntry, existingText string) string {
	if len(artifacts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, a := range artifacts {
		if a.URL == "" || a.Name == "" {
			continue
		}
		if strings.Contains(existingText, a.URL) {
			continue
		}
		lower := strings.ToLower(a.URL)
		if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
			strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".gif") ||
			strings.HasSuffix(lower, ".webp") {
			fmt.Fprintf(&sb, "\n\n![%s](%s)", a.Name, a.URL)
		} else {
			fmt.Fprintf(&sb, "\n\n[Download %s](%s)", a.Name, a.URL)
		}
	}
	return sb.String()
}

// extractToolCalls converts eino ToolCalls from a message into the
// output map format.
func extractToolCalls(msg *schema.Message) []map[string]any {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	calls := make([]map[string]any, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		calls = append(calls, map[string]any{
			"id":        tc.ID,
			"type":      tc.Type,
			"name":      tc.Function.Name,
			"arguments": tc.Function.Arguments,
		})
	}
	return calls
}

// promptMessagesFromParams extracts the Python DSL `prompts` list into
// the single system/user prompt shape supported by the Go ReAct runner.
func promptMessagesFromParams(params map[string]any) (systemPrompt, userPrompt string, ok bool) {
	raw, exists := params["prompts"]
	if !exists {
		return "", "", false
	}
	switch v := raw.(type) {
	case string:
		return "", v, true
	case []any:
		var systems, users []string
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			content, ok := stringFrom(m, "content")
			if !ok {
				continue
			}
			role, _ := stringFrom(m, "role")
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "system":
				systems = append(systems, content)
			case "user", "":
				users = append(users, content)
			}
		}
		if len(systems) == 0 && len(users) == 0 {
			return "", "", false
		}
		return strings.Join(systems, "\n"), strings.Join(users, "\n"), true
	case []map[string]any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return promptMessagesFromParams(map[string]any{"prompts": items})
	}
	return "", "", false
}

func appendPromptText(base, extra string) string {
	if strings.TrimSpace(extra) == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return extra
	}
	return base + "\n" + extra
}

func hasNonEmptyString(inputs map[string]any, name string) bool {
	v, ok := stringFrom(inputs, name)
	return ok && strings.TrimSpace(v) != ""
}

func shouldFallbackToSysQuery(prompt string) bool {
	p := strings.TrimSpace(prompt)
	return p == "" || p == agentUserPromptSchemaDefault
}

func stringFromState(state *runtime.CanvasState, name string) (string, bool) {
	if state == nil {
		return "", false
	}
	v, ok := state.Sys[name].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

func formatAgentRuntimePrompt(inputs map[string]any, userPrompt string) string {
	var b strings.Builder
	if reasoning, ok := stringFrom(inputs, "reasoning"); ok && reasoning != "" {
		fmt.Fprintf(&b, "\nREASONING:\n%s\n", reasoning)
	}
	if contextText, ok := stringFrom(inputs, "context"); ok && contextText != "" {
		fmt.Fprintf(&b, "\nCONTEXT:\n%s\n", contextText)
	}
	if b.Len() == 0 {
		return userPrompt
	}
	fmt.Fprintf(&b, "\nQUERY:\n%s\n", userPrompt)
	return b.String()
}

// mergeAgentParam layers raw inputs over the receiver's default param set.
//
// v1 aliases accepted alongside the v2 names: "llm_id" → "model_id",
// "sys_prompt" → "system_prompt", "base_url" → "BaseURL". v1 fixtures
// use the short forms; without these aliases the v1→v2 conversion
// step would have to run before the factory builds the component.
func mergeAgentParam(base AgentParam, inputs map[string]any) AgentParam {
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
	if promptSystem, promptUser, ok := promptMessagesFromParams(inputs); ok {
		p.SystemPrompt = appendPromptText(p.SystemPrompt, promptSystem)
		if strings.TrimSpace(promptUser) != "" {
			p.UserPrompt = promptUser
		}
	}
	if v, ok := stringFrom(inputs, "user_prompt"); ok && !shouldFallbackToSysQuery(v) {
		p.UserPrompt = v
	}
	if v, ok := floatFrom(inputs, "top_p"); ok {
		f := v
		p.TopP = &f
	}
	if v, ok := intFrom(inputs, "max_rounds"); ok {
		p.MaxRounds = v
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
	if v, ok := sliceFrom(inputs, "tools"); ok {
		p.Tools = v
	}
	if v, ok := nestedMapFrom(inputs, "tool_params"); ok {
		p.ToolParams = v
	}
	if v, ok := boolFrom(inputs, "optimize_multi_turn"); ok {
		p.OptimizeMultiTurn = v
	}
	if v, ok := intFrom(inputs, "optimize_history_window"); ok {
		p.OptimizeHistoryWindow = v
	}
	if v, ok := boolFrom(inputs, "cite"); ok {
		p.Cite = v
	}
	return p
}

// sliceFrom extracts []string from inputs[name].
func sliceFrom(inputs map[string]any, name string) ([]string, bool) {
	v, ok := inputs[name]
	if !ok {
		return nil, false
	}
	switch x := v.(type) {
	case []string:
		return x, true
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	}
	return nil, false
}

// nestedMapFrom extracts map[string]map[string]any from inputs[name].
func nestedMapFrom(inputs map[string]any, name string) (map[string]map[string]any, bool) {
	v, ok := inputs[name]
	if !ok {
		return nil, false
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]map[string]any, len(raw))
	for k, child := range raw {
		m, ok := child.(map[string]any)
		if !ok {
			continue
		}
		out[k] = m
	}
	return out, true
}

// init registers AgentComponent with the orchestrator-owned registry.
func init() {
	Register("Agent", func(params map[string]any) (Component, error) {
		var p AgentParam
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
		if promptSystem, promptUser, ok := promptMessagesFromParams(params); ok {
			p.SystemPrompt = appendPromptText(p.SystemPrompt, promptSystem)
			p.UserPrompt = promptUser
		}
		if v, ok := stringFrom(params, "user_prompt"); ok && p.UserPrompt == "" {
			p.UserPrompt = v
		}
		if v, ok := floatFrom(params, "top_p"); ok {
			f := v
			p.TopP = &f
		}
		if v, ok := sliceFrom(params, "tools"); ok {
			p.Tools = v
		}
		if v, ok := nestedMapFrom(params, "tool_params"); ok {
			p.ToolParams = v
		}
		if v, ok := intFrom(params, "max_rounds"); ok {
			p.MaxRounds = v
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
		return NewAgentComponent(p), nil
	})
}
