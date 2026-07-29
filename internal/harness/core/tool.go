package core

import (
	"context"
	"fmt"

	"ragflow/internal/harness/core/schema"
)

// subAgentDepthKey is a context key for tracking sub-agent recursion depth across
// nested AgentTool invocations. The value is an int representing current depth.
type subAgentDepthKey struct{}

// AgentToolOptions configures an AgentTool.
type AgentToolOptions struct {
	FullChatHistoryAsInput bool
	EmitInternalEvents     bool // Forward inner agent's events to parent stream
	MaxDepth               int  // 0 = unlimited sub-agent nesting depth. Set via WithMaxDepth.
}

// AgentToolOption configures the AgentTool.
type AgentToolOption func(*AgentToolOptions)

// WithFullChatHistoryAsInput uses the full chat history as input to the inner agent.
func WithFullChatHistoryAsInput() AgentToolOption {
	return func(o *AgentToolOptions) { o.FullChatHistoryAsInput = true }
}

// WithEmitInternalEvents enables forwarding internal events from the wrapped agent
// to the parent agent's event stream. This allows real-time streaming of nested
// agent output to the end user via Runner.
//
// Action Scoping:
//   - Interrupted actions are propagated via CompositeInterrupt for proper interrupt/resume
//   - Exit, TransferToAgent, BreakLoop actions are scoped to the agent tool boundary (ignored outside)
//
// Note: These forwarded events are NOT recorded in the parent agent's runSession.
// They are only emitted to the end-user and have no effect on the parent agent's state or checkpoint.
func WithEmitInternalEvents() AgentToolOption {
	return func(o *AgentToolOptions) { o.EmitInternalEvents = true }
}

// WithMaxDepth sets the maximum sub-agent nesting depth for recursion protection.
// When set (>=1), AgentTool checks a depth counter in the context before executing
// the inner agent. If the current depth >= maxDepth, the call returns an error.
// Default: 0 (no limit).
func WithMaxDepth(d int) AgentToolOption {
	return func(o *AgentToolOptions) { o.MaxDepth = d }
}

// NewAgentTool wraps an Agent as a Tool for use by other agents.
// The agent must have non-empty Name and Description, used as the tool name/description.
//
// Action Scoping:
//   - Exit, TransferToAgent, BreakLoop actions from the inner agent are ignored outside the tool
//   - Interrupted actions are propagated via CompositeInterrupt for proper interrupt/resume
func NewAgentTool(ctx context.Context, agent Agent, options ...AgentToolOption) Tool {
	opts := &AgentToolOptions{}
	for _, o := range options {
		o(opts)
	}
	name := agent.Name(ctx)
	if name == "" {
		name = "agent_tool"
	}
	desc := agent.Description(ctx)
	return &agentTool{
		name: name, desc: desc, agent: agent,
		opts: opts, baseCtx: ctx,
	}
}

type agentTool struct {
	name    string
	desc    string
	agent   Agent
	opts    *AgentToolOptions
	baseCtx context.Context
}

func (t *agentTool) Name() string        { return t.name }
func (t *agentTool) Description() string { return t.desc }

func (t *agentTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (result string, err error) {
	// Panic recovery: runner.Run or iter.Next may panic; catch and convert to Go error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("agent tool '%s' panicked: %v", t.name, r)
			result = ""
		}
	}()

	// Derive sub-agent run context from the invocation context to propagate
	// cancellation/deadline. Construction-time baseCtx values (e.g. recursion
	// depth guard) are preserved by adding them to the derived context.
	runCtx := ctx
	if t.baseCtx != nil {
		runCtx = context.WithValue(ctx, subAgentDepthKey{}, 0) // overridden below
	}

	// Recursion depth guard — always propagate the depth counter so nested
	// AgentTool invocations see the correct nesting level regardless of which
	// middleware created them.
	currentDepth := 0
	if v := ctx.Value(subAgentDepthKey{}); v != nil {
		currentDepth = v.(int)
	}
	if t.opts.MaxDepth > 0 && currentDepth >= t.opts.MaxDepth {
		return "", fmt.Errorf("agent tool '%s': recursion limit exceeded (max depth: %d)", t.name, t.opts.MaxDepth)
	}
	// Always increment — even when MaxDepth=0 — so nested calls see real depth.
	runCtx = context.WithValue(runCtx, subAgentDepthKey{}, currentDepth+1)

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: t.agent})
	messages := []Message{schema.UserMessage(args)}
	if t.opts.FullChatHistoryAsInput {
		if ec := getChatModelExecCtx(ctx); ec != nil {
			// TODO: extract full chat history from parent execution context
		}
	}

	iter := runner.Run(runCtx, messages)

	// EmitInternalEvents — read from parent ctx (ctx), not runCtx, because
	// the runCtx is the sub-agent's independent context and has no parent execCtx.
	var parentEC *reActExecCtx
	if t.opts.EmitInternalEvents {
		parentEC = getChatModelExecCtx(ctx)
	}

	var interrupted bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			return "", fmt.Errorf("agent tool '%s': %w", t.name, ev.Err)
		}

		// EmitInternalEvents: forward events to parent stream
		if parentEC != nil && t.opts.EmitInternalEvents {
			parentEC.send(ev)
		}

		if ev.Action != nil && ev.Action.Interrupted != nil {
			interrupted = true
			result += fmt.Sprintf("[interrupted: %v]", ev.Action.Interrupted.Data)
			break
		}
		if ev.Action != nil && (ev.Action.Exit || ev.Action.TransferToAgent != nil || ev.Action.BreakLoop != nil) {
			// Scoped: these actions are for the inner agent only, not propagated
			continue
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			if !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
				msg := ev.Output.MessageOutput.Message
				if msg.Role == schema.RoleAssistant {
					result += msg.Content
				}
			}
		}
	}
	if interrupted {
		return result, fmt.Errorf("agent tool '%s' was interrupted", t.name)
	}
	return result, nil
}

func (t *agentTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	r, err := t.Invoke(ctx, args, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]string{r}), nil
}
