package core

import (
	"context"
	"fmt"
	"ragflow/internal/harness/graph/checkpoint"
	"strings"
	"sync"
	"sync/atomic"

	"ragflow/internal/harness/core/internal"
	"ragflow/internal/harness/core/schema"
)

// ReActConfig holds configuration for TypedReActAgent.
type ReActConfig[M MessageType] struct {
	Model              Model[M]
	Tools              []Tool
	Instruction        string
	MaxIterations      int
	Middlewares        []TypedReActMiddleware[M]
	RetryConfig        *TypedModelRetryConfig[M]
	FailoverConfig     *FailoverConfig[M]
	ReturnDirectly     map[string]bool
	OutputKey          string
	GenModelInput      TypedGenModelInput[M]
	StateModifier      StateModifier[M]
	ToolsConfig        *ToolsNodeConfig
	EmitInternalEvents bool
	// GraphReAct enables graph-based ReAct execution using the project's own
	// StateGraph/Pregel engine. When true, each ReAct iteration runs as a graph
	// node, providing automatic checkpoint, interrupt, and resume via the engine.
	// Default: false (uses the simple for-loop in chatmodel_react.go).
	GraphReAct bool
	// GraphReActCheckpointer is the checkpointer used when GraphReAct is enabled.
	// If nil, no checkpointing is performed (but interrupt is still available
	// via WithInterrupts).
	GraphReActCheckpointer checkpoint.BaseCheckpointer
	// GraphReActInterruptBefore lists node names to interrupt before.
	// Default: ["execute_tools"] (pause before tool execution for human approval).
	GraphReActInterruptBefore []string
}

func DefaultReActConfig[M MessageType]() *ReActConfig[M] {
	return &ReActConfig[M]{MaxIterations: 10, Instruction: internal.DefaultSystemPrompt}
}

// ReActAgentResumeData holds data provided during resume to modify agent behavior.
type ReActAgentResumeData struct {
	HistoryModifier func(ctx context.Context, messages []Message) []Message
}

// ReActAgent implements the ReAct (Reasoning + Acting) pattern.
//
// Production features:
//   - freeze-once: after first Run/Resume, configuration is frozen (atomic)
//   - ToolsNode abstraction with middleware chain support
//   - Enhanced Tool (4 endpoint types) support via handler interface
//   - DeferredToolInfos for server-side tool search
//   - EmitInternalEvents for AgentTool event forwarding
//   - AfterToolCallsHook for AgentLoop integration
//   - ResumeWithData / HistoryModifier for resume customization
//   - gob encodability check on SetRunLocalValue
type ReActAgent[M MessageType] struct {
	name   string
	desc   string
	config *ReActConfig[M]

	once   sync.Once
	frozen uint32
	run    typedRunFunc[M]
	exeCtx *execContext
}

var _ ResumableAgent = &ReActAgent[*schema.Message]{}
var _ TypedResumableAgent[*schema.AgenticMessage] = &ReActAgent[*schema.AgenticMessage]{}

type TypedGenModelInput[M MessageType] func(ctx context.Context, instruction string, input *TypedAgentInput[M]) ([]M, error)

// StateModifier allows transforming the agent state before model invocation.
type StateModifier[M MessageType] func(ctx context.Context, state *TypedReActAgentState[M]) (*TypedReActAgentState[M], error)

func defaultGenModelInput(ctx context.Context, instruction string, input *AgentInput) ([]Message, error) {
	msgs := make([]Message, 0, len(input.Messages)+1)
	if instruction != "" {
		processed := resolveTemplate(instruction, ctx)
		msgs = append(msgs, schema.SystemMessage(processed))
	}
	msgs = append(msgs, input.Messages...)
	return msgs, nil
}

func resolveTemplate(tmpl string, ctx context.Context) string {
	s := getSession(ctx)
	if s == nil {
		return tmpl
	}
	result := tmpl
	for k, v := range s.Values {
		repl := fmt.Sprintf("{%s}", k)
		if sv, ok := v.(string); ok {
			result = strings.ReplaceAll(result, repl, sv)
		}
	}
	return result
}

func NewReActAgent[M MessageType](cfg *ReActConfig[M]) *ReActAgent[M] {
	if cfg == nil {
		cfg = DefaultReActConfig[M]()
	}
	a := &ReActAgent[M]{name: "react_agent", desc: "ReAct agent using a chat model", config: cfg}
	if cfg.ToolsConfig == nil && len(cfg.Tools) > 0 {
		cfg.ToolsConfig = &ToolsNodeConfig{Tools: cfg.Tools, ReturnDirectly: cfg.ReturnDirectly}
	}
	return a
}
func (a *ReActAgent[M]) WithName(n string) *ReActAgent[M]        { a.name = n; return a }
func (a *ReActAgent[M]) WithDescription(d string) *ReActAgent[M] { a.desc = d; return a }
func (a *ReActAgent[M]) Name(_ context.Context) string           { return a.name }
func (a *ReActAgent[M]) Description(_ context.Context) string    { return a.desc }
func (a *ReActAgent[M]) GetType() string                         { return "ReActAgent" }

// ---- Freeze mechanism ----

func (a *ReActAgent[M]) IsFrozen() bool { return atomic.LoadUint32(&a.frozen) == 1 }

func (a *ReActAgent[M]) freeze() { atomic.StoreUint32(&a.frozen, 1) }

// ---- Run / Resume ----

func (a *ReActAgent[M]) Run(ctx context.Context, input *TypedAgentInput[M], opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("panic: %v", r)})
			}
			gen.Close()
		}()
		runFunc := a.buildRunFunc(ctx)
		runFunc(ctx, &typedRunParams[M]{input: input, generator: gen})
		a.freeze()
	}()
	return it
}

func (a *ReActAgent[M]) Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("panic: %v", r)})
			}
			gen.Close()
		}()
		if info.WasInterrupted {
			if s, ok := info.InterruptState.(*TypedReActAgentState[M]); ok {
				runFunc := a.buildRunFunc(ctx)
				params := &typedRunParams[M]{input: &TypedAgentInput[M]{Messages: s.Messages, EnableStreaming: info.EnableStreaming}, generator: gen, interruptState: s, resumeInfo: info}
				if info.ResumeData != nil {
					if rd, ok := info.ResumeData.(*ReActAgentResumeData); ok && rd.HistoryModifier != nil {
						params.historyModifier = rd.HistoryModifier
					}
				}
				runFunc(ctx, params)
				a.freeze()
				return
			}
		}
		gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("resume called but agent was not interrupted or state is invalid")})
	}()
	return it
}

// ---- Internal types ----

type typedRunFunc[M MessageType] func(ctx context.Context, p *typedRunParams[M])

type typedRunParams[M MessageType] struct {
	input              *TypedAgentInput[M]
	generator          *AsyncGenerator[*TypedAgentEvent[M]]
	interruptState     *TypedReActAgentState[M]
	resumeInfo         *ResumeInfo
	historyModifier    func(context.Context, []Message) []Message
	afterToolCallsHook func(ctx context.Context) error
}

// reActExecCtx carries per-execution state for event sending, cancellation,
// retry signal propagation, and after-tool-calls hooks.
type reActExecCtx struct {
	generator          *AsyncGenerator[*TypedAgentEvent[*schema.Message]]
	cancelCtx          *cancelContext
	suppressEventSend  bool
	retrySignal        *retrySignal
	failoverLastModel  Model[*schema.Message]
	afterToolCallsHook func(ctx context.Context) error
}

func (ec *reActExecCtx) send(ev any) {
	if ec != nil && ec.generator != nil {
		if te, ok := ev.(*TypedAgentEvent[*schema.Message]); ok {
			ec.generator.Send(te)
		}
	}
}

type execContext struct {
	instruction        string
	returnDirectly     map[string]bool
	toolInfos          []*schema.ToolInfo // from config.Tools + contributor ToolInfos
	deferredToolInfos  []*schema.ToolInfo
	toolSearchTool     *schema.ToolInfo
	emitInternalEvents bool

	// ToolContributor results (collected once in once.Do).
	contribTools          []Tool
	contribToolInfos      []*schema.ToolInfo
	contribReturnDirectly map[string]bool
}

// ---- Run function builder ----

func (a *ReActAgent[M]) buildRunFunc(ctx context.Context) typedRunFunc[M] {
	var onceRun typedRunFunc[M]
	a.once.Do(func() {
		ec, err := a.prepareExecContext(ctx)
		if err != nil {
			onceRun = func(_ context.Context, _ *typedRunParams[M]) {}
			a.run = onceRun
			return
		}
		a.exeCtx = ec
		// Check for tools: config.Tools + ToolContributor tools
		hasTools := len(a.config.Tools) > 0 ||
			(a.config.ToolsConfig != nil && len(a.config.ToolsConfig.Tools) > 0) ||
			len(ec.contribTools) > 0
		if !hasTools {
			onceRun = a.buildNoToolsRunFunc()
		} else if a.config.GraphReAct {
			onceRun = a.buildGraphReActRunFunc()
		} else {
			onceRun = a.buildReActRunFunc()
		}
		a.run = onceRun
	})
	return a.run
}

func (a *ReActAgent[M]) prepareExecContext(ctx context.Context) (*execContext, error) {
	instruction := a.config.Instruction
	if instruction == "" {
		instruction = internal.DefaultSystemPrompt
	}
	rd := a.config.ReturnDirectly
	if rd == nil {
		rd = make(map[string]bool)
	}

	// Collect from ToolContributor middlewares.
	contribTools := collectContributorTools(ctx, a.config.Middlewares)
	contribInfos := collectContributorToolInfos(ctx, a.config.Middlewares)
	contribRD := collectContributorReturnDirectly(ctx, a.config.Middlewares)

	// Merge return-directly.
	mergedRD := make(map[string]bool, len(rd)+len(contribRD))
	for k, v := range rd {
		mergedRD[k] = v
	}
	for k, v := range contribRD {
		mergedRD[k] = v
	}

	// Merge tool infos from a single source to avoid duplicates:
	// when ToolsConfig is nil, NewReActAgent populates ToolsConfig.Tools with a.config.Tools,
	// so building from both sources would produce duplicate entries.
	var baseInfos []*schema.ToolInfo
	if a.config.ToolsConfig != nil && len(a.config.ToolsConfig.Tools) > 0 {
		baseInfos = toolsToInfosTyped[M](a.config.ToolsConfig.Tools)
	} else {
		baseInfos = toolsToInfosTyped[M](a.config.Tools)
	}
	allInfos := make([]*schema.ToolInfo, 0, len(baseInfos)+len(contribInfos))
	allInfos = append(allInfos, baseInfos...)
	allInfos = append(allInfos, contribInfos...)

	return &execContext{
		instruction:           instruction,
		returnDirectly:        mergedRD,
		toolInfos:             allInfos,
		contribTools:          contribTools,
		contribToolInfos:      contribInfos,
		contribReturnDirectly: contribRD,
		emitInternalEvents:    a.config.EmitInternalEvents,
	}, nil
}

// ---- No-tools run function ----

func (a *ReActAgent[M]) buildNoToolsRunFunc() typedRunFunc[M] {
	return func(ctx context.Context, p *typedRunParams[M]) {
		// Build allTools: config.Tools + contribTools
		allTools := make([]Tool, 0, len(a.config.Tools)+len(a.exeCtx.contribTools))
		allTools = append(allTools, a.config.Tools...)
		allTools = append(allTools, a.exeCtx.contribTools...)

		// BeforeAgent middleware
		rc := &ReActAgentContext{Instruction: a.exeCtx.instruction, Tools: allTools, ReturnDirectly: a.exeCtx.returnDirectly}
		if err := a.runBeforeAgent(&ctx, rc, p.generator); err != nil {
			return
		}

		model := BuildModelWrapperChain(a.config.Model, nil, a.config, a.exeCtx.toolInfos)
		state := NewReActAgentState(p.input.Messages, a.exeCtx.toolInfos, a.config.MaxIterations)

		// BeforeModelRewrite middleware
		mc := &TypedModelContext[M]{Tools: state.ToolInfos, ModelRetryConfig: a.config.RetryConfig, ModelFailoverConfig: a.config.FailoverConfig}
		if err := a.runBeforeModelRewrite(&ctx, &state, mc, p.generator); err != nil {
			return
		}

		if a.config.StateModifier != nil {
			var err error
			state, err = a.config.StateModifier(ctx, state)
			if err != nil {
				p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("StateModifier: %w", err)})
				return
			}
		}

		modelMsgs := buildModelInputFromState[M](state.Messages, rc.Instruction)
		resp, err := model.Generate(ctx, modelMsgs)
		if err != nil {
			p.generator.Send(&TypedAgentEvent[M]{Err: err})
			return
		}
		p.generator.Send(typedModelOutputEvent(resp, nil))
		state.Messages = append(state.Messages, resp)

		// AfterModelRewrite middleware
		if err := a.runAfterModelRewrite(&ctx, &state, mc, p.generator); err != nil {
			return
		}

		if a.config.OutputKey != "" && !isNilMessage(resp) {
			setOutputToSession(ctx, resp, a.config.OutputKey)
		}

		// AfterAgent middleware
		a.runAfterAgent(&ctx, state, p.generator)
	}
}

// runBeforeAgent executes the BeforeAgent middleware chain.
// Returns a non-nil error if any middleware signals termination.
func (a *ReActAgent[M]) runBeforeAgent(ctx *context.Context, rc *ReActAgentContext, gen *AsyncGenerator[*TypedAgentEvent[M]]) error {
	for _, mw := range a.config.Middlewares {
		if mw == nil {
			continue
		}
		var err error
		*ctx, rc, err = mw.BeforeAgent(*ctx, rc)
		if err != nil {
			gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("BeforeAgent: %w", err)})
			return err
		}
	}
	return nil
}

// runBeforeModelRewrite executes the BeforeModelRewrite middleware chain.
func (a *ReActAgent[M]) runBeforeModelRewrite(ctx *context.Context, state **TypedReActAgentState[M], mc *TypedModelContext[M], gen *AsyncGenerator[*TypedAgentEvent[M]]) error {
	for _, mw := range a.config.Middlewares {
		if mw == nil {
			continue
		}
		var err error
		*ctx, *state, err = mw.BeforeModelRewrite(*ctx, *state, mc)
		if err != nil {
			gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("BeforeModelRewrite: %w", err)})
			return err
		}
	}
	return nil
}

// runAfterModelRewrite executes the AfterModelRewrite middleware chain.
func (a *ReActAgent[M]) runAfterModelRewrite(ctx *context.Context, state **TypedReActAgentState[M], mc *TypedModelContext[M], gen *AsyncGenerator[*TypedAgentEvent[M]]) error {
	for _, mw := range a.config.Middlewares {
		if mw == nil {
			continue
		}
		var err error
		*ctx, *state, err = mw.AfterModelRewrite(*ctx, *state, mc)
		if err != nil {
			gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("AfterModelRewrite: %w", err)})
			return err
		}
	}
	return nil
}

// runAfterAgent executes the AfterAgent middleware chain.
func (a *ReActAgent[M]) runAfterAgent(ctx *context.Context, state *TypedReActAgentState[M], gen *AsyncGenerator[*TypedAgentEvent[M]]) {
	for _, mw := range a.config.Middlewares {
		if mw == nil {
			continue
		}
		var err error
		*ctx, err = mw.AfterAgent(*ctx, state)
		if err != nil {
			gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("AfterAgent: %w", err)})
			return
		}
	}
}

// ---- Helpers ----

func buildModelInputFromState[M MessageType](messages []M, instruction string) []M {
	var msgs []M
	if instruction != "" {
		msgs = append(msgs, any(schema.SystemMessage(instruction)).(M))
	}
	for _, m := range messages {
		msgs = append(msgs, m)
	}
	return msgs
}

func setOutputToSession[M MessageType](ctx context.Context, msg M, key string) {
	if !isNilMessage(msg) {
		s := getSession(ctx)
		if s != nil {
			s.Values[key] = extractTextContent(msg)
		}
	}
}

func toolsToInfosTyped[M MessageType](tools []Tool) []*schema.ToolInfo {
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		if p, ok := t.(ToolInfoProvider); ok {
			infos = append(infos, p.ToolInfo())
		} else {
			infos = append(infos, &schema.ToolInfo{Name: t.Name(), Description: t.Description()})
		}
	}
	return infos
}

func extractTextContent[M MessageType](msg M) string {
	switch v := any(msg).(type) {
	case *schema.Message:
		return v.Content
	case *schema.AgenticMessage:
		var texts []string
		for _, b := range v.ContentBlocks {
			if b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	default:
		return ""
	}
}

// findTool finds a tool by name from a list of tools.
func findTool(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

// extractToolCalls extracts tool calls from a model response message.
// It handles both *schema.Message (with ToolCalls field) and generic types.
func extractToolCalls[M MessageType](resp M) []schema.ToolCall {
	switch v := any(resp).(type) {
	case *schema.Message:
		if len(v.ToolCalls) > 0 {
			return v.ToolCalls
		}
	case *schema.AgenticMessage:
		var tc []schema.ToolCall
		for _, b := range v.ContentBlocks {
			if b.Type == "tool_use" && b.ToolCall != nil && b.ToolCall.ID != "" && b.ToolCall.Name != "" {
				tc = append(tc, schema.ToolCall{
					ID:       b.ToolCall.ID,
					Function: schema.ToolCallFunction{Name: b.ToolCall.Name, Arguments: b.ToolCall.Arguments},
				})
			}
		}
		return tc
	}
	return nil
}

// streamWithCancel wraps a streaming model call with cancel detection.
func streamWithCancel[M MessageType](s *schema.StreamReader[M], cc *cancelContext) *schema.StreamReader[M] {
	if cc == nil {
		return s
	}
	select {
	case <-cc.immediateChan:
		s.Close()
		r := schema.NewStreamReader[M]()
		var zero M
		r.Send(zero, ErrStreamCanceled)
		r.Close()
		return r
	default:
	}
	r := schema.NewStreamReader[M]()
	go func() {
		defer r.Close()
		defer s.Close()
		ch := make(chan struct {
			Data M
			Err  error
		}, 64)
		go func() {
			defer close(ch)
			for {
				select {
				case <-cc.immediateChan:
					return
				default:
				}
				d, e := s.Recv()
				select {
				case <-cc.immediateChan:
					return
				default:
				}
				select {
				case ch <- struct {
					Data M
					Err  error
				}{d, e}:
				case <-cc.immediateChan:
					return
				}
				if e != nil {
					return
				}
			}
		}()
		for {
			select {
			case <-cc.immediateChan:
				var z M
				r.Send(z, ErrStreamCanceled)
				return
			case v := <-ch:
				if v.Err != nil {
					return
				}
				r.Send(v.Data, nil)
			}
		}
	}()
	return r
}

// getChatModelExecCtx retrieves the chat model execution context from context.
func getChatModelExecCtx(ctx context.Context) *reActExecCtx {
	rc := getRunCtx(ctx)
	if rc == nil {
		return nil
	}
	// The exec ctx is stored on the run session or passed via context value
	if ec, ok := rc.Session.Values["__exec_ctx"].(*reActExecCtx); ok {
		return ec
	}
	return nil
}

// getReActExecCtx retrieves the typed execution context from context.
func getReActExecCtx[M MessageType](ctx context.Context) *reActExecCtx {
	return getChatModelExecCtx(ctx)
}

// CheckpointDataVersion is the version of checkpoint data format for forward compatibility.
type CheckpointDataVersion int

const CheckpointDataV1 CheckpointDataVersion = 1

// preprocessCheckpointData performs forward-compatible migration on resume data.
func preprocessCheckpointData(data any) any { return data }

// WithGraphReAct enables the graph-based ReAct execution engine for a ReActConfig.
// When enabled, each ReAct iteration runs as a StateGraph node with the Pregel engine,
// providing automatic checkpoint, interrupt before tool execution, and resume support.
//
// Usage:
//
//	cfg := DefaultReActConfig[*schema.Message]()
//	cfg.GraphReAct = true
//	cfg.GraphReActCheckpointer = checkpoint.NewMemorySaver()  // optional
func WithGraphReAct[M MessageType](cfg *ReActConfig[M], cptr checkpoint.BaseCheckpointer) {
	cfg.GraphReAct = true
	cfg.GraphReActCheckpointer = cptr
}

// WithGraphReActInterrupt sets which graph nodes to interrupt before.
// Default: ["execute_tools"]. Use this to customize interrupt behavior.
func WithGraphReActInterrupt[M MessageType](cfg *ReActConfig[M], interruptBefore ...string) {
	cfg.GraphReActInterruptBefore = interruptBefore
}

// ---- Graph-based ReAct run function ----
//
// When GraphReAct is enabled, the ReAct loop runs as a StateGraph with the
// Pregel engine. Each iteration is a superstep, providing:
//   - Automatic checkpoint at every node boundary (via graph.WithCheckpointer)
//   - Interrupt before tool execution (via graph.WithInterrupts)
//   - Resume from checkpoint on restart (via graph.Invoke with same config)
//   - Streaming events via pregel.StreamManager

func (a *ReActAgent[M]) buildGraphReActRunFunc() typedRunFunc[M] {
	return func(ctx context.Context, p *typedRunParams[M]) {
		// Graph-based ReAct currently supports *schema.Message only.
		// For AgenticMessage, fall back to the simple for-loop.
		var zero M
		_, isMessage := any(zero).(*schema.Message)
		if !isMessage {
			// Fallback: use standard for-loop for non-Message types.
			a.buildReActRunFunc()(ctx, p)
			return
		}

		// Build graph config from agent config.
		graphCfg := &ReActGraphConfig{
			Checkpointer:    a.config.GraphReActCheckpointer,
			InterruptBefore: a.config.GraphReActInterruptBefore,
			RecursionLimit:  a.config.MaxIterations * 2, // each iter = 2 nodes, so allow extra
		}

		// Type-assert the agent to *ReActAgent[*schema.Message].
		msgAgent, ok := any(a).(*ReActAgent[*schema.Message])
		if !ok {
			p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("graph ReAct: agent type assertion failed")})
			return
		}

		rg, err := NewReActGraph(msgAgent, graphCfg, a.exeCtx.toolInfos)
		if err != nil {
			p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("NewReActGraph: %w", err)})
			return
		}

		// Build agent input.
		input := &AgentInput{Messages: messageSliceToAny2(p.input.Messages)}

		// Run the graph (synchronous invoke or streaming).
		state, err := rg.Invoke(ctx, input, nil)
		if err != nil {
			p.generator.Send(&TypedAgentEvent[M]{Err: err})
			return
		}

		// Emit the final model response as an event.
		if len(state.Messages) > 0 {
			last := state.Messages[len(state.Messages)-1]
			if !isNilMessage(last) {
				p.generator.Send(any(typedModelOutputEvent(last, nil)).(*TypedAgentEvent[M]))
			}
		}

		// Emit afterToolCallsHook if configured.
		if p.afterToolCallsHook != nil {
			if err := p.afterToolCallsHook(ctx); err != nil {
				p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("after_tool_calls_hook: %w", err)})
			}
		}
	}
}

// messageSliceToAny2 converts a []M (MessageType) to []*schema.Message for graph ReAct.
func messageSliceToAny2[M MessageType](msgs []M) []*schema.Message {
	r := make([]*schema.Message, len(msgs))
	for i, m := range msgs {
		if msg, ok := any(m).(*schema.Message); ok {
			r[i] = msg
		} else {
			// Fallback: skip non-Message items.
			r[i] = nil
		}
	}
	return r
}
