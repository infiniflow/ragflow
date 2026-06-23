// Package component — Agent (T1).
//
// Multi-turn ReAct agent powered by harness's flow/agent/react package.
// Uses getDefaultChatInvoker for LLM calls.
// ToolCallingChatModel, using harness's
// production-grade implementation.
//
// Public outputs (content / tool_calls / artifacts) match the
// plan-specified shape.
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"ragflow/internal/agent/component/prompts"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
)

// AgentComponent is a multi-turn ReAct agent.
type AgentComponent struct {
	param AgentParam
}

type AgentParam struct {
	ModelID               string
	SystemPrompt          string
	UserPrompt            string
	TopP                  *float64
	Tools                 []string                  // Agent-visible tool names resolved into BaseTool instances
	ToolParams            map[string]map[string]any // node-level tool constructor params keyed by tool name
	MaxRounds             int
	OptimizeMultiTurn     bool
	OptimizeHistoryWindow int
	Meta                  AgentMeta
	Cite                  bool
	Driver                string
	APIKey                string
	BaseURL               string
	// SubAgents holds parsed sub-agent tool definitions. Populated from
	// the DSL's `tools` parameter (config objects) at component creation
	// time and from `inputs["tools"]` at invocation time. Each entry maps
	// a sanitized function name (LLM-visible) to the raw config map.
	SubAgents map[string]map[string]any
	// StreamFinal enables progressive streaming for the final answer via
	// StreamContentStreaming. Only true for the top-level agent, so
	// sub-agents never send IsThink:false events that would close the
	// parent's thinking section prematurely.
	StreamFinal bool
}

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
//	"content"   string
//	"tool_calls" []map[string]any (one entry per tool call observed)
//	"artifacts"  []map[string]any (collected from tool responses — empty in P0)
type AgentOutput struct {
	Content   string
	ToolCalls []map[string]any
	Artifacts []map[string]any
}

// agentRunner is the package-level ReAct runner. The production value
// that returns canned *ComponentMessage values.
var agentRunner = agentReActRunner

// agentReActRunner calls the model and returns the response.
// Uses streaming LLM for progressive output throughout the entire ReAct loop.
// The loop terminates naturally when the LLM returns no tool_calls (i.e. it
// decides the task is complete). Every content and reasoning chunk is
// forwarded to progCh in real time, matching Python's streaming behavior.
func agentReActRunner(ctx context.Context, p AgentParam) (*ComponentMessage, error) {
	msgs := make([]ComponentMessage, 0, 2)
	if p.SystemPrompt != "" {
		msgs = append(msgs, NewSystemMessage(p.SystemPrompt))
	}
	msgs = append(msgs, NewUserMessage(p.UserPrompt))

	progCh := runtime.ProgressChFromCtx(ctx)

	// Build tool definitions + sub-agent definitions.
	toolDefs := buildToolDefs(p)
	toolByName := make(map[string]agenttool.Tool, len(p.Tools))
	for _, name := range p.Tools {
		var params map[string]any
		if p.ToolParams != nil {
			params = p.ToolParams[name]
		}
		t, err := agenttool.BuildByName(name, params)
		if err != nil {
			continue
		}
		toolByName[t.ToolMeta().Name] = t
	}

	maxRounds := p.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}
	maxRounds += 2 // extra room for final answer after all sub-agents

	common.Info("agentReAct starting", zap.Int("maxRounds", maxRounds), zap.Int("tools", len(toolDefs)), zap.Int("subAgents", len(p.SubAgents)), zap.String("model", p.ModelID))

	for round := 0; round < maxRounds; round++ {
		common.Info("agentReAct round", zap.Int("round", round), zap.Int("msgs", len(msgs)))

		resp, err := StreamingInvoke(ctx, ChatInvokeRequest{
			Driver: p.Driver, ModelName: p.ModelID, APIKey: p.APIKey,
			BaseURL: p.BaseURL, Messages: msgs, TopP: p.TopP, Tools: toolDefs,
		})
		if err != nil {
			return nil, fmt.Errorf("component: Agent round %d: %w", round, err)
		}

		// Execute tool calls (sub-agents or regular tools).
		if len(resp.ToolCalls) > 0 {
			if progCh != nil {
				// Send initial model reasoning (reasoning_content) to
				// the thinking section BEFORE the tool-call markers so
				// the front-end shows the model's thought process first.
				if resp.ReasonContent != "" {
					progCh <- runtime.ProgEvent{Text: resp.ReasonContent + "\n\n", IsThink: true}
				}
				// Tool-call markers for the thinking section.
				for _, tc := range resp.ToolCalls {
					progCh <- runtime.ProgEvent{Text: "\n\n**Calling " + tc.FuncName + "...**\n\n", IsThink: true}
				}
			}
			// Build assistant message with tool_calls array.
			compTCs := make([]ComponentToolCall, 0, len(resp.ToolCalls))
			for _, tc := range resp.ToolCalls {
				compTCs = append(compTCs, ComponentToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: ComponentFunctionCall{
						Name:      tc.FuncName,
						Arguments: tc.Arguments,
					},
				})
			}
			assistantMsg := ComponentMessage{
				Role:      RoleAssistant,
				Content:   resp.Content,
				ToolCalls: compTCs,
			}
			msgs = append(msgs, assistantMsg)
			for idx, tc := range resp.ToolCalls {
				if subCfg, isSub := p.SubAgents[tc.FuncName]; isSub {
					if progCh != nil {
						// Log panel: sub-agent started.
						progCh <- runtime.ProgEvent{
							IsNodeEvent: true, NodeEventType: "started",
							NodeCPNID: tc.FuncName, NodeDisplayName: tc.FuncName, NodeClassName: "Agent",
						}
					}
					result := runSubAgent(ctx, subCfg, p, tc.Arguments)
					if progCh != nil {
						// Text marker for the thinking section.
						progCh <- runtime.ProgEvent{Text: "\n\n**" + tc.FuncName + " returned.**\n\n", IsThink: true}
						progCh <- runtime.ProgEvent{Text: result, IsThink: true}
						// Log panel: sub-agent finished with its inputs, outputs, and Thoughts.
						inputs := parseSubAgentInputs(tc.Arguments)
						outputs := map[string]any{"content": result}
						progCh <- runtime.ProgEvent{
							IsNodeEvent: true, NodeEventType: "finished",
							NodeCPNID: tc.FuncName, NodeDisplayName: tc.FuncName, NodeClassName: "Agent",
							Thoughts:    result,
							NodeInputs:  inputs,
							NodeOutputs: outputs,
						}
					}
					// Use the tool call's ID — required by OpenAI API format.
					tcID := tc.ID
					if tcID == "" {
						tcID = fmt.Sprintf("call_%d", idx)
					}
					msgs = append(msgs, ComponentMessage{Role: "tool", Content: result, ToolCallID: tcID})
					continue
				}
				tool, exists := toolByName[tc.FuncName]
				if !exists {
					msgs = append(msgs, ComponentMessage{
						Role: "tool", Content: fmt.Sprintf(`{"_ERROR": "unknown tool %q"}`, tc.FuncName),
					})
					continue
				}
				result, tErr := tool.InvokableRun(ctx, tc.Arguments)
				if tErr != nil {
					result = fmt.Sprintf(`{"_ERROR": %q}`, tErr.Error())
				}
				tcID := tc.ID
				if tcID == "" {
					tcID = fmt.Sprintf("call_%d", idx)
				}
				msgs = append(msgs, ComponentMessage{Role: "tool", Content: result, ToolCallID: tcID})
			}
			continue // next round — tool calls will be retried with tool results in history
		}

		// No tool calls → final answer.
		//
		// Content source: some models (DeepSeek R1 / V4) put the
		// final answer in "reasoning_content" when "content" is
		// empty.  Fall back to ReasonContent to capture both cases.
		content := resp.Content
		if content == "" {
			content = resp.ReasonContent
		}
		if content != "" {
			if progCh != nil && p.StreamFinal {
				// Stream the final answer progressively by
				// sending resp.Content as word-level chunks to
				// progCh.  This avoids a *second* API call
				// (StreamContentStreaming) whose message history
				// contains tool results that require tool_call_id
				// — the model driver serialisation drops those.
				streamContentToProgCh(content, progCh)
			}
			return &ComponentMessage{Role: RoleAssistant, Content: content}, nil
		}
	}

	return &ComponentMessage{Role: RoleAssistant, Content: "Agent reached max rounds."}, nil
}

// parseSubAgentInputs converts the tool-call argument JSON into a
// map for the node_finished log panel entry.  Returns a minimal
// map on parse failure so the panel still shows something.
func parseSubAgentInputs(argsJSON string) map[string]any {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &parsed); err != nil || len(parsed) == 0 {
		return map[string]any{"arguments": argsJSON}
	}
	return parsed
}

// streamContentToProgCh sends content to progCh as a single IsThink:false
// event that closes the thinking section and displays the answer in the
// main output area.  This replaces the abandoned StreamContentStreaming
// path (a second LLM call whose tool-result messages lacked tool_call_id
// fields).
func streamContentToProgCh(content string, progCh chan<- runtime.ProgEvent) {
	if content == "" {
		return
	}
	progCh <- runtime.ProgEvent{Text: content, IsThink: false}
}

// sanitizeFnName converts a display name to a valid OpenAI function name
// (only [a-zA-Z0-9_-] characters; spaces become underscores).
func sanitizeFnName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "agent_tool"
	}
	return b.String()
}

// parseSubAgentConfigs extracts sub-agent tool definitions from the DSL's
// `tools` field (an array of {component_name, id, name, params} objects).
// Returns a map keyed by sanitized function name → raw config object.
// Returns nil when tools is nil or empty, or contains no sub-agent objects.
func parseSubAgentConfigs(toolsRaw any) map[string]map[string]any {
	arr, ok := toolsRaw.([]any)
	if !ok {
		return nil
	}
	out := make(map[string]map[string]any)
	for _, item := range arr {
		cfg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Only process entries that look like sub-agent configs
		// (have component_name, id, name, and nested params).
		if _, ok := cfg["component_name"].(string); !ok {
			continue
		}
		if _, ok := cfg["id"].(string); !ok {
			continue
		}
		name, ok := cfg["name"].(string)
		if !ok || name == "" {
			continue
		}
		if _, ok := cfg["params"].(map[string]any); !ok {
			continue
		}
		fn := sanitizeFnName(name)
		out[fn] = cfg
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildToolDefs converts string tool names AND sub-agent configs into
// OpenAI-compatible tool definition maps. Returns nil when nothing is
// configured or all tools/sub-agents are unsupported/skipped.
func buildToolDefs(p AgentParam) []map[string]any {
	if len(p.Tools) == 0 && len(p.SubAgents) == 0 {
		return nil
	}
	var defs []map[string]any

	// Phase 1: string tool names from p.Tools.
	for _, name := range p.Tools {
		var params map[string]any
		if p.ToolParams != nil {
			params = p.ToolParams[name]
		}
		t, err := agenttool.BuildByName(name, params)
		if err != nil {
			continue
		}
		meta := t.ToolMeta()
		props := make(map[string]any)
		reqd := make([]string, 0)
		for pn, pi := range meta.Parameters {
			f := map[string]any{"type": pi.Type, "description": pi.Description}
			if pi.Enum != nil {
				f["enum"] = pi.Enum
			}
			props[pn] = f
			if pi.Required {
				reqd = append(reqd, pn)
			}
		}
		schema := map[string]any{"type": "object", "properties": props}
		if len(reqd) > 0 {
			schema["required"] = reqd
		}
		defs = append(defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        meta.Name,
				"description": meta.Description,
				"parameters":  schema,
			},
		})
	}

	// Phase 2: sub-agent configs from p.SubAgents.
	// Tool parameters MUST match Python's Agent.get_meta() shape:
	// {user_prompt, reasoning, context} — the model expects these specific
	// parameter names and descriptions to understand how to delegate tasks.
	for fn, cfg := range p.SubAgents {
		paramsRaw, _ := cfg["params"].(map[string]any)
		desc := ""
		if paramsRaw != nil {
			desc, _ = paramsRaw["description"].(string)
		}
		if desc == "" {
			if n, _ := cfg["name"].(string); n != "" {
				desc = "Execute agent: " + n
			} else {
				desc = "Execute a sub-agent"
			}
		}
		defs = append(defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        fn,
				"description": desc,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"user_prompt": map[string]any{
							"type":        "string",
							"description": "The task or query to send to this agent",
						},
						"reasoning": map[string]any{
							"type":        "string",
							"description": "Supervisor's reasoning for choosing this agent",
						},
						"context": map[string]any{
							"type":        "string",
							"description": "All relevant background information from prior research steps",
						},
					},
					"required": []string{"user_prompt", "reasoning", "context"},
				},
			},
		})
	}
	return defs
}

// addToolCallMemory summarizes the tool calls observed in msg via
// a small LLM call and returns a one-line history entry. Mirrors
// Python's `add_memory(user, assist, func_name, params, results,
// user_defined_prompt)` — the LLM condenses the tool usage into a
// short, memory-worthy sentence.
//
// When the LLM call fails or there are no tool calls, the function
// returns ("", nil) and the caller skips appending to history.
func addToolCallMemory(ctx context.Context, p AgentParam, msg *ComponentMessage) (string, error) {
	calls := extractToolCallsSimple(msg)
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
		Messages: []ComponentMessage{
			{Role: RoleSystem, Content: system},
			{Role: RoleUser, Content: user},
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
		Messages: []ComponentMessage{
			{Role: RoleSystem, Content: systemPrompt},
			{Role: RoleUser, Content: content},
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

// GetInputForm aggregates the Agent's own meta-schema with each
// sub-tool's input form. Mirrors Python's `Agent.get_input_form`.
//
// Today the sub-tool input forms are aggregated via an optional
// `InputForm() map[string]any` method on the tool (when tools
// implement it); tools that don't expose a structured input form
// are skipped silently.
func (c *AgentComponent) GetInputForm() map[string]any {
	out := map[string]any{
		"self": c.param.Meta,
	}
	tools, err := buildAgentTools(c.param)
	if err != nil {
		return out
	}
	for _, t := range tools {
		meta := t.ToolMeta()
		name := meta.Name
		if name == "" {
			continue
		}
		if formGetter, ok := t.(interface{ InputForm() map[string]any }); ok {
			out[name] = formGetter.InputForm()
		}
	}
	return out
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
		Messages: []ComponentMessage{
			{Role: RoleSystem, Content: system},
			{Role: RoleUser, Content: user},
		},
		TopP: p.TopP,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

// NewAgentComponent builds an AgentComponent from raw params.
func NewAgentComponent(p AgentParam) *AgentComponent {
	if p.MaxRounds <= 0 {
		p.MaxRounds = 3
	}
	p.StreamFinal = true
	return &AgentComponent{param: p}
}

// Name returns the registered component name.
func (c *AgentComponent) Name() string { return "Agent" }

// Invoke runs the ReAct loop via the configured agentRunner and returns
// the output map.
func (c *AgentComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	p := mergeAgentParam(c.param, inputs)
	if p.ModelID == "" {
		return nil, &ParamError{Field: "model_id", Reason: "required"}
	}
	// Resolve {{cpn_id@var}} template references in system and user
	// prompts against the canvas state. This MUST run BEFORE the
	// user_prompt fallback — otherwise template refs like {{begin@query}}
	// would be overwritten by the fallback text and never resolved.
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		if resolved, rerr := runtime.ResolveTemplate(p.SystemPrompt, state); resolved != p.SystemPrompt || rerr == nil {
			p.SystemPrompt = resolved
		}
		if resolved, rerr := runtime.ResolveTemplate(p.UserPrompt, state); resolved != p.UserPrompt || rerr == nil {
			p.UserPrompt = resolved
			if rerr != nil {
				common.Warn("Agent: resolve user_prompt", zap.Error(rerr))
			}
		}
	}

	// After template resolution: if user_prompt is still empty, try to
	// read the Begin node's "query" output directly from canvas state.
	// Do NOT fall back to system_prompt.
	if p.UserPrompt == "" {
		if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
			if raw, rErr := state.GetVar("begin@query"); rErr == nil {
				if q, ok := raw.(string); ok && q != "" {
					p.UserPrompt = q
				}
			}
		}
	}
	// Still empty → use a minimal placeholder.
	if p.UserPrompt == "" {
		p.UserPrompt = "Please process the instructions above."
	}
	// Validate AFTER fallback: both prompts may be empty before
	// begin@query resolution runs (lines above).
	if p.UserPrompt == "" && p.SystemPrompt == "" {
		return nil, &ParamError{Field: "user_prompt", Reason: "at least one of user_prompt or system_prompt must be set"}
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
	artifacts := emptyArtifactList()
	artifactMD := formatArtifactMarkdown(artifacts, content)
	out := map[string]any{
		"content":    content + artifactMD,
		"tool_calls": extractToolCallsSimple(msg),
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
		"model_id":      "Provider-side model identifier (e.g. \"gpt-4o-mini\")",
		"system_prompt": "Optional system prompt",
		"user_prompt":   "User prompt; supports {{cpn_id@param}} references",
		"top_p":         "Top-p (nucleus) sampling cutoff (0.0-1.0). Optional.",
		"tools":         "List of tool names to make available to the ReAct agent.",
		"tool_params":   "Optional node-level tool constructor params keyed by tool name (e.g. execute_sql DB config).",
		"max_rounds":    "Maximum ReAct rounds (default 3).",
		"driver":        "Provider driver name",
		"api_key":       "Override API key for this call.",
		"cite":          "When true, make a post-stream citation-grounding call (reads chunks from state.Retrieval).",
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

// buildAgentTools resolves tool names into tool instances.
func buildAgentTools(p AgentParam) ([]agenttool.Tool, error) {
	return agenttool.BuildAll(p.Tools, p.ToolParams)
}

// artifactEntry is the shape of a single tool-returned artifact
// surfaced through the Agent's outputs["artifacts"].
type artifactEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// collectArtifactsFromToolCalls extracts artifact entries from a
// the final message — the tool-response state lives inside the agent
// runtime and is not directly accessible. So this v1 returns an empty
// slice; the wiring lives in a follow-up that hoists the state
// into a place AgentComponent can read.
//
// When the tool response state becomes accessible (a future phase),
// the entry point to wire it is here: scan the conversation
// messages for entries whose `Extra["_ARTIFACTS"]` carries the
// per-tool artifact metadata, decode the JSON, and append to the
// returned slice. The shape expected from each tool is:
//
//	{ "name": "report.pdf", "url": "https://..." }
func collectArtifactsFromToolCalls(_ *ComponentMessage) []artifactEntry { return nil }

// formatArtifactMarkdown renders a slice of artifacts as markdown
// links, omitting URLs already present in the existing text (Python's
// `_collect_tool_artifact_markdown` does the same de-duplication).
//
// Format:
//   - image URL → ![name](url)
//   - other URL → [Download name](url)
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

func extractToolCallsSimple(msg *ComponentMessage) []map[string]any {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		out = append(out, map[string]any{
			"id":   tc.ID,
			"type": tc.Type,
			"name": tc.Function.Name,
			"function": map[string]any{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		})
	}
	return out
}

func emptyArtifactList() []artifactEntry {
	return nil
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
	if v, ok := stringFrom(inputs, "user_prompt"); ok {
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
	if sa := parseSubAgentConfigs(inputs["tools"]); sa != nil {
		if p.SubAgents == nil {
			p.SubAgents = sa
		} else {
			for k, v := range sa {
				p.SubAgents[k] = v
			}
		}
	}
	if v, ok := nestedMapFrom(inputs, "tool_params"); ok {
		p.ToolParams = v
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

// runSubAgent executes a sub-agent by building an AgentParam from the
// sub-agent config and calling agentReActRunner. Calls agentReActRunner
// directly (not through NewAgentComponent.Invoke) to avoid the init cycle
// agentRunner → agentReActRunner → runSubAgent → Invoke → agentRunner.
func runSubAgent(ctx context.Context, cfg map[string]any, parentParam AgentParam, argsJSON string) string {
	paramsRaw, _ := cfg["params"].(map[string]any)
	if paramsRaw == nil {
		return `{"_ERROR": "sub-agent: missing params"}`
	}
	subParam := AgentParam{}
	if v, ok := stringFrom(paramsRaw, "llm_id"); ok {
		subParam.ModelID = v
	} else if v, ok := stringFrom(paramsRaw, "model_id"); ok {
		subParam.ModelID = v
	}
	// Only inherit parent provider credentials when the sub-agent does
	// NOT specify its own model — otherwise use the sub-agent's own
	// provider chain (scheduler.go / service will resolve it).
	if subParam.ModelID == "" {
		subParam.Driver = parentParam.Driver
		subParam.APIKey = parentParam.APIKey
		subParam.BaseURL = parentParam.BaseURL
	}
	if v, ok := stringFrom(paramsRaw, "sys_prompt"); ok {
		subParam.SystemPrompt = v
	} else if v, ok := stringFrom(paramsRaw, "system_prompt"); ok {
		subParam.SystemPrompt = v
	}
	if v, ok := intFrom(paramsRaw, "max_rounds"); ok {
		subParam.MaxRounds = v
	}
	if v, ok := sliceFrom(paramsRaw, "tools"); ok {
		subParam.Tools = v
	}

	// User prompt: parse from argsJSON. Tool calling parameters are now
	// {user_prompt, reasoning, context} matching Python's Agent meta.
	var toolArgs struct {
		UserPrompt string `json:"user_prompt"`
		Reasoning  string `json:"reasoning"`
		Context    string `json:"context"`
		Input      string `json:"input"` // backward compat
	}
	if err := json.Unmarshal([]byte(argsJSON), &toolArgs); err == nil {
		// New format: user_prompt from the tool call.
		if toolArgs.UserPrompt != "" {
			subParam.UserPrompt = toolArgs.UserPrompt
		} else if toolArgs.Input != "" {
			// Old format fallback.
			if promptsRaw, ok := paramsRaw["prompts"].([]any); ok && len(promptsRaw) > 0 {
				if first, ok := promptsRaw[0].(map[string]any); ok {
					if content, _ := first["content"].(string); content != "" {
						subParam.UserPrompt = strings.ReplaceAll(content, "{sys.query}", toolArgs.Input)
					}
				}
			}
			if subParam.UserPrompt == "" {
				subParam.UserPrompt = toolArgs.Input
			}
		}
	}
	if subParam.UserPrompt == "" {
		subParam.UserPrompt = parentParam.UserPrompt
	}
	if subParam.ModelID == "" {
		subParam.ModelID = parentParam.ModelID
	}

	msg, err := agentReActRunner(ctx, subParam)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR": "sub-agent: %s"}`, err.Error())
	}
	if msg == nil {
		return `{"_ERROR": "sub-agent: nil response"}`
	}
	return msg.Content
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
		if v, ok := stringFrom(params, "user_prompt"); ok {
			p.UserPrompt = v
		}
		if v, ok := floatFrom(params, "top_p"); ok {
			f := v
			p.TopP = &f
		}
		if v, ok := sliceFrom(params, "tools"); ok {
			p.Tools = v
		}
		p.SubAgents = parseSubAgentConfigs(params["tools"])
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
