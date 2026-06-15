// Package component — Agent (Phase 2 P0, plan §2.11.3 row 8).
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
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/entity/models"
)

// AgentComponent is a multi-turn ReAct agent.
type AgentComponent struct {
	param AgentParam
}

// AgentParam captures the (resolved) DSL parameters for an Agent node.
type AgentParam struct {
	ModelID      string
	SystemPrompt string
	UserPrompt   string
	Tools        []string // Agent-visible tool names resolved into Eino BaseTool instances
	ToolParams   map[string]map[string]any // node-level tool constructor params keyed by tool name
	MaxRounds    int
	Driver       string
	APIKey       string
	BaseURL      string
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
	return agent.Generate(ctx, input)
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

	msg, err := agentRunner(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("component: Agent.Invoke: %w", err)
	}
	return map[string]any{
		"content":    msg.Content,
		"tool_calls": extractToolCalls(msg),
		"artifacts":  []map[string]any{},
	}, nil
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
		"tools":         "List of tool names to make available to the ReAct agent.",
		"tool_params":   "Optional node-level tool constructor params keyed by tool name (e.g. execute_sql DB config).",
		"max_rounds":    "Maximum ReAct rounds (default 3).",
		"driver":        "Provider driver name",
		"api_key":       "Override API key for this call.",
	}
}

// Outputs returns output metadata.
func (c *AgentComponent) Outputs() map[string]string {
	return map[string]string{
		"content":    "Final assistant content (after the ReAct loop terminates)",
		"tool_calls": "One entry per tool call observed during the run",
		"artifacts":  "Artifacts collected from tool responses (empty in P0)",
	}
}

// buildAgentChatModel constructs an EinoChatModel from AgentParam by
// resolving the driver through the RAGFlow provider manager.
func buildAgentChatModel(p AgentParam) (*models.EinoChatModel, error) {
	driver := p.Driver
	if driver == "" {
		driver = "dummy"
	}
	var baseURL map[string]string
	if p.BaseURL != "" {
		baseURL = map[string]string{"default": p.BaseURL}
	}
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
	cm := models.NewChatModel(d, &p.ModelID, cfg)
	return models.NewEinoChatModel(cm, nil), nil
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
		if v, ok := stringFrom(params, "user_prompt"); ok {
			p.UserPrompt = v
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
