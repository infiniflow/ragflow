// Package agentcore provides a graph-level ReAct loop using the project's own
// StateGraph engine, with built-in checkpoint/interrupt/resume support at each
// iteration boundary.
//
// The ReActGraph wraps a TypedChatModelAgent's loop into StateGraph nodes so that
// the graph engine's checkpointing (via graph.WithCheckpointer) and interrupt/resume
// (via graph.WithInterrupts) apply at each superstep automatically. This replaces
// the simple for-loop in chatmodel_react.go with the full Pregel execution engine.
//
// Key features:
//   - Checkpoint at every model_generate and execute_tools node boundary
//   - Interrupt before execute_tools for human-in-the-loop tool approval
//   - Resume from interrupt via graph checkpoint restoration
//   - Full middleware chain (BeforeAgent, BeforeModelRewrite, AfterModelRewrite, AfterAgent)
//   - ToolsNode integration with ToolCallMiddlewares
//   - Streaming events via pregel.StreamManager
//   - Generic: supports both *schema.Message and *schema.AgenticMessage
package core

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/pregel"
	"ragflow/internal/harness/graph/types"
)

func init() {
	schema.RegisterType("_harness_react_graph_state", func() any { return &ReActGraphState{} })
}

// ReActGraphState is the shared state for the graph-level ReAct loop.
// It persists across supersteps, enabling checkpoint and interrupt/resume.
type ReActGraphState struct {
	Messages       []*schema.Message
	ToolInfos      []*schema.ToolInfo
	IterationsLeft int
	MaxIterations  int
	AgentName      string
	Instruction    string
	HasToolCall    bool // signals whether the last model output had tool calls

	// ToolExecutedCache caches completed tool call results for interrupt/resume.
	// Key = tool call ID, value = result content string.
	// After successful completion of all tools in a superstep, this is cleared.
	// On interrupt, it persists via the Pregel checkpoint and allows skipping
	// already-executed tools on resume (equivalent to Eino's ToolsInterruptAndRerunExtra).
	ToolExecutedCache map[string]string
}

// ReActGraph wraps a ChatModelAgent's loop into a StateGraph with automatic
// checkpoint at each iteration and interrupt before tool execution.
type ReActGraph struct {
	compiled types.CompiledGraph
	config   *ReActConfig[*schema.Message]
	agent    *ReActAgent[*schema.Message]
	allInfos []*schema.ToolInfo // merged config + contributor tool infos
	allTools []Tool             // merged config + contributor tools
}

// ReActGraphConfig holds options for building a ReActGraph.
type ReActGraphConfig struct {
	Checkpointer    checkpoint.BaseCheckpointer
	InterruptBefore []string // node names to interrupt before (default: "execute_tools")
	RecursionLimit  int
}

// NewReActGraph builds a StateGraph with nodes:
//
//	prepare_input → model_generate → execute_tools → check_done
//	                                                ↘ [end]
//
// Interrupt is set at "execute_tools" by default. With a Checkpointer, each node
// transition automatically saves a checkpoint via the Pregel engine.
//
// The graph applies the full middleware chain:
//   - prepare_input: BeforeAgent
//   - model_generate: BeforeModelRewrite → model call → AfterModelRewrite
//   - check_done (on exit): AfterAgent
func NewReActGraph(agent *ReActAgent[*schema.Message], cfg *ReActGraphConfig, allToolInfos []*schema.ToolInfo) (*ReActGraph, error) {
	if cfg == nil {
		cfg = &ReActGraphConfig{}
	}
	agentCfg := agent.config
	// Fallback: when allToolInfos is nil, derive from the agent's tools
	// so model wrappers always have tool metadata.
	if allToolInfos == nil {
		allToolInfos = toolsToInfosTyped[*schema.Message](agentCfg.Tools)
		if agentCfg.ToolsConfig != nil {
			allToolInfos = append(allToolInfos, toolsToInfosTyped[*schema.Message](agentCfg.ToolsConfig.Tools)...)
		}
	}
	sg := graph.NewStateGraph(&ReActGraphState{})

	// Register channels for state fields used by the graph engine.
	sg.AddChannel("messages", channels.NewLastValue([]*schema.Message{}))
	sg.AddChannel("iterations_left", channels.NewLastValue(0))
	sg.AddChannel("has_tool_call", channels.NewLastValue(false))
	sg.AddChannel("tool_cache", channels.NewLastValue(map[string]string{}))

	// --- Node: prepare_input ---
	// Runs once at the start. Applies BeforeAgent middleware.
	// Build allTools and allRD from config + contributor tools.
	allTools := make([]Tool, 0, len(agent.config.Tools))
	allTools = append(allTools, agent.config.Tools...)
	allRD := make(map[string]bool)
	for k, v := range agent.config.ReturnDirectly {
		allRD[k] = v
	}
	if ec := agent.exeCtx; ec != nil {
		allTools = append(allTools, ec.contribTools...)
		for k, v := range ec.contribReturnDirectly {
			allRD[k] = v
		}
	}

	sg.AddNode("prepare_input", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(*ReActGraphState)
		rc := &ReActAgentContext{
			Instruction:    s.Instruction,
			Tools:          allTools,
			ReturnDirectly: allRD,
		}
		for _, mw := range agentCfg.Middlewares {
			if mw == nil {
				continue
			}
			var err error
			ctx, rc, err = mw.BeforeAgent(ctx, rc)
			if err != nil {
				return nil, fmt.Errorf("BeforeAgent: %w", err)
			}
		}
		s.Instruction = rc.Instruction
		return s, nil
	})

	// --- Node: model_generate ---
	// Calls the LLM with the current message history. Applies BeforeModelRewrite
	// and AfterModelRewrite middleware chains.
	// Clears the tool cache so each iteration starts fresh.
	sg.AddNode("model_generate", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(*ReActGraphState)
		if s.IterationsLeft <= 0 {
			return s, nil
		}
		s.IterationsLeft--
		// Clear tool cache at start of each iteration.
		s.ToolExecutedCache = nil

		model := BuildModelWrapperChain(agentCfg.Model, nil, agentCfg, allToolInfos)

		agentState := NewReActAgentState(
			messageSliceToAny(s.Messages),
			allToolInfos,
			s.IterationsLeft+1,
		)
		typedState := (*TypedReActAgentState[*schema.Message])(agentState)
		mc := &TypedModelContext[*schema.Message]{
			Tools:               allToolInfos,
			ModelRetryConfig:    agentCfg.RetryConfig,
			ModelFailoverConfig: agentCfg.FailoverConfig,
		}

		// BeforeModelRewrite middleware chain.
		for _, mw := range agentCfg.Middlewares {
			if mw == nil {
				continue
			}
			var err error
			ctx, typedState, err = mw.BeforeModelRewrite(ctx, typedState, mc)
			if err != nil {
				return nil, fmt.Errorf("BeforeModelRewrite: %w", err)
			}
		}
		s.Messages = typedState.Messages

		// StateModifier hook (e.g., context window trimming).
		if agentCfg.StateModifier != nil {
			var err error
			typedState, err = agentCfg.StateModifier(ctx, typedState)
			if err != nil {
				return nil, fmt.Errorf("StateModifier: %w", err)
			}
			s.Messages = typedState.Messages
		}

		// Build model input (via GenModelInput or default).
		var modelMsgs []*schema.Message
		if agentCfg.GenModelInput != nil {
			var err error
			modelMsgs, err = agentCfg.GenModelInput(ctx, s.Instruction,
				&TypedAgentInput[*schema.Message]{Messages: s.Messages})
			if err != nil {
				return nil, fmt.Errorf("GenModelInput: %w", err)
			}
		} else {
			modelMsgs = buildModelInputFromState(s.Messages, s.Instruction)
		}

		// Call model.
		resp, err := model.Generate(ctx, modelMsgs)
		if err != nil {
			return nil, fmt.Errorf("model: %w", err)
		}
		s.Messages = append(s.Messages, resp)

		// AfterModelRewrite middleware chain.
		typedState.Messages = s.Messages
		for _, mw := range agentCfg.Middlewares {
			if mw == nil {
				continue
			}
			var err error
			ctx, typedState, err = mw.AfterModelRewrite(ctx, typedState, mc)
			if err != nil {
				return nil, fmt.Errorf("AfterModelRewrite: %w", err)
			}
		}
		s.Messages = typedState.Messages

		// Detect if the model produced tool calls.
		toolCalls := extractToolCalls(resp)
		s.HasToolCall = len(toolCalls) > 0

		return s, nil
	})

	// --- Node: execute_tools ---
	// Executes tool calls found in the last model response using ToolsNode.
	// Supports interrupt/resume via ToolExecutedCache: on interrupt, completed
	// tool results are saved to the cache (persisted via Pregel channel checkpoint).
	// On resume, already-cached tools are skipped.
	sg.AddNode("execute_tools", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(*ReActGraphState)
		if len(s.Messages) == 0 {
			return s, nil
		}
		last := s.Messages[len(s.Messages)-1]
		toolCalls := extractToolCalls(last)
		if len(toolCalls) == 0 {
			return s, nil
		}

		// Restore or initialize the tool execution cache.
		cache := s.ToolExecutedCache
		if cache == nil {
			cache = make(map[string]string)
		}

		// Filter out already-cached (previously completed) tool calls.
		var pendingCalls []schema.ToolCall
		for _, tc := range toolCalls {
			if _, done := cache[tc.ID]; !done {
				pendingCalls = append(pendingCalls, tc)
			}
		}
		if len(pendingCalls) == 0 {
			return s, nil
		}

		agentState := NewReActAgentState(
			messageSliceToAny(s.Messages),
			s.ToolInfos,
			s.IterationsLeft,
		)
		typedState := (*TypedReActAgentState[*schema.Message])(agentState)

		// Build ToolsNode from config ToolsConfig (for ToolInvokeMiddlewares etc.)
		// but overlay allTools (config + contributor).
		tnCfg := &ToolsNodeConfig{}
		if agentCfg.ToolsConfig != nil {
			*tnCfg = *agentCfg.ToolsConfig
		}
		tnCfg.Tools = allTools
		tnCfg.ReturnDirectly = allRD
		tn := NewToolsNode[*schema.Message](tnCfg)

		// Execute pending calls one at a time so we can track per-call results.
		// Each call uses a single-tool-call message to keep tracking simple.
		var firstErr error
		var toolInterrupted bool
		for _, tc := range pendingCalls {
			// Build a fresh message containing only this tool call.
			singleMsg := &schema.Message{
				Role:      schema.RoleAssistant,
				Content:   "",
				ToolCalls: []schema.ToolCall{tc},
			}
			var action *AgentAction
			var toolResults []*schema.Message
			toolResults, action, firstErr = tn.Execute(ctx, singleMsg, typedState, nil)
			if firstErr != nil {
				// Check if this is a tool interrupt (not a real error).
				var ir *interruptResult
				if errors.As(firstErr, &ir) {
					toolInterrupted = true
					firstErr = nil
					// Tool interrupted — still save its message to state.
					for _, tr := range toolResults {
						s.Messages = append(s.Messages, tr)
						if tr != nil && tr.Content != "" {
							cache[tc.ID] = tr.Content
						}
					}
					break
				}
				// Real error — stop.
				break
			}
			for _, tr := range toolResults {
				s.Messages = append(s.Messages, tr)
				if tr != nil && tr.Content != "" {
					cache[tc.ID] = tr.Content
				}
			}
			if action != nil && action.Exit {
				s.IterationsLeft = 0
				s.HasToolCall = false
				break
			}
		}

		if firstErr != nil {
			s.ToolExecutedCache = cache
			return s, fmt.Errorf("tools: %w", firstErr)
		}

		if toolInterrupted {
			// Save cache and return cleanly so Pregel engine checkpoints the state.
			s.ToolExecutedCache = cache
			return s, nil
		}

		// All tools completed successfully — clear cache for next iteration.
		s.ToolExecutedCache = nil
		return s, nil
	})

	// --- Node: check_done ---
	// Emits AfterAgent middleware and writes the final output.
	sg.AddNode("check_done", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(*ReActGraphState)
		agentState := NewReActAgentState(
			messageSliceToAny(s.Messages),
			s.ToolInfos,
			s.IterationsLeft,
		)
		typedState := (*TypedReActAgentState[*schema.Message])(agentState)

		for _, mw := range agentCfg.Middlewares {
			if mw == nil {
				continue
			}
			var err error
			ctx, err = mw.AfterAgent(ctx, typedState)
			if err != nil {
				return nil, fmt.Errorf("AfterAgent: %w", err)
			}
		}

		// Store output in session if configured.
		if agentCfg.OutputKey != "" && len(s.Messages) > 0 {
			last := s.Messages[len(s.Messages)-1]
			setOutputToSession(ctx, last, agentCfg.OutputKey)
		}
		return s, nil
	})

	// --- Edges ---
	sg.AddEdge(constants.Start, "prepare_input")
	sg.AddEdge("prepare_input", "model_generate")

	// Conditional: if no tool calls → check_done (which goes to end), else → execute_tools
	sg.AddConditionalEdges("model_generate", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(*ReActGraphState)
		if s.IterationsLeft <= 0 || !s.HasToolCall {
			return "check_done", nil
		}
		return "execute_tools", nil
	}, map[string]string{
		"check_done":    "check_done",
		"execute_tools": "execute_tools",
	})

	sg.AddEdge("execute_tools", "model_generate") // loop back for next iteration
	sg.AddEdge("check_done", constants.End)       // terminal node

	// --- Compile with checkpoint and interrupt ---
	interrupts := cfg.InterruptBefore
	if len(interrupts) == 0 {
		interrupts = []string{"execute_tools"}
	}
	rl := cfg.RecursionLimit
	if rl <= 0 {
		rl = constants.DefaultRecursionLimit
	}

	var compileOpts []interface{}
	compileOpts = append(compileOpts, graph.WithRecursionLimit(rl))
	if cfg.Checkpointer != nil {
		compileOpts = append(compileOpts, graph.WithCheckpointer(cfg.Checkpointer))
	}
	for _, name := range interrupts {
		compileOpts = append(compileOpts, graph.WithInterrupts(name))
	}

	compiled, err := sg.Compile(compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("compile ReAct graph: %w", err)
	}

	return &ReActGraph{
		compiled: compiled,
		config:   agentCfg,
		agent:    agent,
		allInfos: allToolInfos,
		allTools: allTools,
	}, nil
}

// Invoke runs the graph-level ReAct loop synchronously via the Pregel engine.
// When input is nil (resume path), the graph restores state from the checkpoint;
// buildInitialState returns nil to let the engine handle it.
func (rg *ReActGraph) Invoke(ctx context.Context, input *AgentInput, config *types.RunnableConfig) (*ReActGraphState, error) {
	var state interface{}
	if input != nil {
		state = rg.buildInitialState(input)
	}

	result, err := rg.compiled.Invoke(ctx, state, config)
	if err != nil {
		return nil, err
	}
	outState, ok := result.(*ReActGraphState)
	if !ok {
		return nil, fmt.Errorf("unexpected result type %T from graph", result)
	}
	return outState, nil
}

// Stream runs the graph-level ReAct loop with streaming events via Pregel.
// Returns (outputCh, errCh). The outputCh yields pregel.StreamEvent values
// including checkpoint, task start/end, values, and final state.
func (rg *ReActGraph) Stream(ctx context.Context, input *AgentInput, config *types.RunnableConfig, mode types.StreamMode) (<-chan interface{}, <-chan error) {
	state := rg.buildInitialState(input)
	return rg.compiled.Stream(ctx, state, mode, config)
}

// Resume resumes a previously interrupted graph execution from its checkpoint.
func (rg *ReActGraph) Resume(ctx context.Context, config *types.RunnableConfig) (*ReActGraphState, error) {
	// Pass config so Pregel engine can restore the correct checkpoint.
	result, err := rg.compiled.Invoke(ctx, nil, config)
	if err != nil {
		return nil, err
	}
	outState, ok := result.(*ReActGraphState)
	if !ok {
		return nil, fmt.Errorf("unexpected result type %T from resumed graph", result)
	}
	return outState, nil
}

// ResumeStream resumes a previously interrupted graph with streaming.
func (rg *ReActGraph) ResumeStream(ctx context.Context, config *types.RunnableConfig, mode types.StreamMode) (<-chan interface{}, <-chan error) {
	return rg.compiled.Stream(ctx, nil, mode, config)
}

// Compile returns the underlying compiled graph for direct access.
func (rg *ReActGraph) Compile() types.CompiledGraph { return rg.compiled }

// ---- helpers ----

func (rg *ReActGraph) buildInitialState(input *AgentInput) *ReActGraphState {
	maxIter := rg.config.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	state := &ReActGraphState{
		Messages:       input.Messages,
		IterationsLeft: maxIter,
		MaxIterations:  maxIter,
		AgentName:      rg.agent.name,
		Instruction:    rg.config.Instruction,
	}
	// Use merged tool infos (config + contributor).
	state.ToolInfos = make([]*schema.ToolInfo, len(rg.allInfos))
	copy(state.ToolInfos, rg.allInfos)
	return state
}

func messageSliceToAny(msgs []*schema.Message) []Message {
	r := make([]Message, len(msgs))
	for i, m := range msgs {
		r[i] = m
	}
	return r
}

// Ensure pregel is imported for side effects (engine registration).
var _ = pregel.Engine{}
