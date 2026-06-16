package core

import (
	"context"

	"ragflow/internal/harness/core/internal"
)

// RunOption configures an agent run.
type RunOption interface{ apply(*runOptions) }

type runOptions struct {
	sessionValues        map[string]any
	sharedParentSession  bool
	checkPointID         *string
	cancelCtx            *cancelContext
	skipTransferMessages bool
	agentNames           []string
	callbacks            []any
	afterToolCallsHook   func(ctx context.Context) error
	chatModelOptions     []ModelOption
	toolOptions          []ToolOption
	agentToolOptions     map[string][]RunOption
	historyModifier      func(context.Context, []Message) []Message
}

type runOptFn func(*runOptions)

func (f runOptFn) apply(o *runOptions) { f(o) }

func WrapImplSpecificOptFn(fn func(*runOptions)) RunOption {
	return runOptFn(fn)
}

func getCommonOptions(o *runOptions, opts ...RunOption) *runOptions {
	if o == nil {
		o = &runOptions{}
	}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(o)
		}
	}
	return o
}

// WithSessionValues injects session-scoped key-value pairs into the run context.
func WithSessionValues(vals map[string]any) RunOption {
	return runOptFn(func(o *runOptions) { o.sessionValues = vals })
}

// WithCheckPointID sets the checkpoint ID for this run, enabling interrupt/resume.
func WithCheckPointID(id string) RunOption {
	return runOptFn(func(o *runOptions) { o.checkPointID = &id })
}

// WithSkipTransferMessages prevents the agent from receiving messages forwarded
// from parent agents during a transfer.
func WithSkipTransferMessages() RunOption {
	return runOptFn(func(o *runOptions) { o.skipTransferMessages = true })
}

// WithCallbacks registers agent lifecycle callbacks (onStart/onEnd/onError/onInterrupt).
func WithCallbacks(cbs ...any) RunOption {
	return runOptFn(func(o *runOptions) { o.callbacks = cbs })
}

// WithAgentNames scopes the associated options to specific agent names.
func WithAgentNames(names ...string) RunOption {
	return runOptFn(func(o *runOptions) { o.agentNames = names })
}

// WithSharedParentSession gives sub-agents access to the parent's session values.
func WithSharedParentSession() RunOption {
	return runOptFn(func(o *runOptions) { o.sharedParentSession = true })
}

// ---- Model-agent-specific options ----

// WithChatModelOptions passes model-level options (e.g., temperature, retry) to the underlying Model.
func WithChatModelOptions(opts []ModelOption) RunOption {
	return WrapImplSpecificOptFn(func(o *runOptions) { o.chatModelOptions = opts })
}

// WithToolOptions passes tool-level options to tool invocations during this run.
func WithToolOptions(opts []ToolOption) RunOption {
	return WrapImplSpecificOptFn(func(o *runOptions) { o.toolOptions = opts })
}

// WithAgentToolOptions passes agent-level options to a specific sub-agent identified by name.
func WithAgentToolOptions(agentName string, opts []RunOption) RunOption {
	return WrapImplSpecificOptFn(func(o *runOptions) {
		if o.agentToolOptions == nil { o.agentToolOptions = make(map[string][]RunOption) }
		o.agentToolOptions[agentName] = opts
	})
}

// WithHistoryModifier sets a function that can trim or transform message history before
// each model call. Useful for context-window management.
func WithHistoryModifier(fn func(context.Context, []Message) []Message) RunOption {
	return WrapImplSpecificOptFn(func(o *runOptions) { o.historyModifier = fn })
}

// WithAfterToolCallsHook registers a per-run hook that fires synchronously after
// all tool calls in a react iteration complete, before the next Model call.
// This is suitable for AgentLoop Push+Preempt patterns where the pushed item
// must be visible to the next turn's GenInput.
func WithAfterToolCallsHook(fn func(ctx context.Context) error) RunOption {
	return runOptFn(func(o *runOptions) { o.afterToolCallsHook = fn })
}

// ---- Agent callbacks (scoped per agent name) ----

// WithAgentErrorCallback registers an error callback for the agent run.
// It fires when an agent encounters a non-recoverable error during execution.
func WithAgentErrorCallback(fn func(ctx context.Context, err error)) RunOption {
	return WrapImplSpecificOptFn(func(o *runOptions) {
		o.callbacks = append(o.callbacks, callbackHandler{onError: fn})
	})
}

// WithAgentInterruptCallback registers an interrupt callback for the agent run.
// It fires when the agent execution is interrupted (e.g., for human-in-the-loop).
func WithAgentInterruptCallback(fn func(ctx context.Context, info *InterruptInfo)) RunOption {
	return WrapImplSpecificOptFn(func(o *runOptions) {
		o.callbacks = append(o.callbacks, callbackHandler{onInterrupt: fn})
	})
}

// ---- Cancel option ----

func WithCancel() (RunOption, AgentCancelFunc) {
	cc := newCancelContext()
	opt := WrapImplSpecificOptFn(func(o *runOptions) { o.cancelCtx = cc })
	return opt, cc.buildCancelFunc()
}

// ---- Configuration ----

// SetLanguage sets the language for agent prompts.
func SetLanguage(lang internal.Language) { internal.SetLanguage(lang) }

const (
	LanguageEnglish = internal.LanguageEnglish
	LanguageChinese = internal.LanguageChinese
)
