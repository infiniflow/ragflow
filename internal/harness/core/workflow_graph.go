// Package agentcore provides graph-based workflow agents (Sequential, Parallel,
// Loop) using the project's own StateGraph/Pregel engine.
//
// Unlike the legacy workflow.go implementation, these graph-based workflows:
//   - Auto-checkpoint at each sub-agent boundary (via graph.WithCheckpointer)
//   - Support interrupt/resume at any sub-agent (via graph.WithInterrupts)
//   - Emit streaming events through the Pregel StreamManager
//   - Use the Pregel engine's recursion limit and cancellation support
//
// Usage:
//
//	gwf, err := NewSequentialGraph(ctx, &SequentialConfig{...}, checkpointer)
//	state, err := gwf.Invoke(ctx, input)
package core

import (
	"context"
	"fmt"
	"sync"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

func init() {
	schema.RegisterType("_harness_wf_graph_state", func() any { return &WorkflowGraphState{} })
}

// WorkflowGraphState is the shared state for graph-based workflow agents.
// It carries messages between sub-agents and tracks the current position.
type WorkflowGraphState struct {
	Messages      []*schema.Message
	SubAgentNames []string   // names of sub-agents in order
	CurrentStep   int        // current sub-agent index
	LoopIter      int        // for loop mode
	MaxLoopIter   int        // for loop mode
	Done          bool

	mu sync.Mutex // protects Messages from concurrent access in inline execution
}

// AppendMessage safely appends a message to the Messages slice.
func (s *WorkflowGraphState) AppendMessage(msg *schema.Message) {
	s.mu.Lock()
	s.Messages = append(s.Messages, msg)
	s.mu.Unlock()
}

// SnapshotMessages safely returns a copy of the Messages slice.
func (s *WorkflowGraphState) SnapshotMessages() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*schema.Message(nil), s.Messages...)
}

// MessagesLen safely returns the length of Messages.
func (s *WorkflowGraphState) MessagesLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Messages)
}

// WorkflowGraph wraps a CompiledGraph that runs sub-agents as graph nodes.
type WorkflowGraph struct {
	compiled *graph.CompiledGraph
}

// ---- Sequential ----

// NewSequentialGraph builds a StateGraph where sub-agents run sequentially.
//
//	start → sub_0 → sub_1 → ... → sub_n → end
//
// Each sub-agent boundary is a checkpoint point. Interrupt can be enabled
// before any sub-agent via WithInterrupts.
func NewSequentialGraph(ctx context.Context, cfg *SequentialConfig, cptr graph.Checkpointer, interrupts ...string) (*WorkflowGraph, error) {
	if cfg == nil {
		return nil, fmt.Errorf("SequentialConfig is nil")
	}
	if len(cfg.SubAgents) == 0 {
		return nil, fmt.Errorf("SequentialConfig requires at least one sub-agent")
	}
	sg := graph.NewStateGraph(&WorkflowGraphState{})

	names := make([]string, len(cfg.SubAgents))
	for i, a := range cfg.SubAgents {
		names[i] = a.Name(ctx)
	}

	// Create one node per sub-agent.
	for i, agent := range cfg.SubAgents {
		idx := i
		ag := agent // capture
		nodeName := fmt.Sprintf("sub_%d", i)
		sg.AddNode(nodeName, func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*WorkflowGraphState)
			// Copy Messages slice so the sub-agent's goroutine doesn't share
			// the underlying array with the graph's concurrent goroutines.
			msgCopy := append([]*schema.Message(nil), s.Messages...)
			iter := ag.Run(ctx, &AgentInput{Messages: msgCopy})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					return nil, fmt.Errorf("sub-agent %s: %w", names[idx], ev.Err)
				}
				if ev.Output != nil && ev.Output.MessageOutput != nil &&
					!ev.Output.MessageOutput.IsStreaming &&
					ev.Output.MessageOutput.Message != nil {
					s.AppendMessage(ev.Output.MessageOutput.Message)
				}
			}
			s.CurrentStep = idx + 1
			return s, nil
		})
	}

	// Chain nodes sequentially.
	sg.AddEdge(constants.Start, "sub_0")
	for i := 1; i < len(cfg.SubAgents); i++ {
		sg.AddEdge(fmt.Sprintf("sub_%d", i-1), fmt.Sprintf("sub_%d", i))
	}
	sg.AddEdge(fmt.Sprintf("sub_%d", len(cfg.SubAgents)-1), constants.End)

	compileOpts := []graph.CompileOption{
		graph.WithRecursionLimit(len(cfg.SubAgents) + 2),
	}
	if cptr != nil {
		compileOpts = append(compileOpts, graph.WithCheckpointer(cptr))
	}
	for _, name := range interrupts {
		compileOpts = append(compileOpts, graph.WithInterrupts(name))
	}

	compiled, err := sg.Compile(compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("compile sequential graph: %w", err)
	}

	return &WorkflowGraph{compiled: compiled}, nil
}

// ---- Parallel ----

// NewParallelGraph builds a StateGraph where sub-agents run in parallel via
// a split node that fans out to all sub-agents.
//
//	start → __wf_split__ ─┬→ sub_0 ─┬→ end
//	                      ├→ sub_1 ─┤
//	                      └→ sub_n ─┘
func NewParallelGraph(ctx context.Context, cfg *ParallelConfig, cptr graph.Checkpointer, interrupts ...string) (*WorkflowGraph, error) {
	if cfg == nil {
		return nil, fmt.Errorf("ParallelConfig is nil")
	}
	if len(cfg.SubAgents) == 0 {
		return nil, fmt.Errorf("ParallelConfig requires at least one sub-agent")
	}
	sg := graph.NewStateGraph(&WorkflowGraphState{})

	names := make([]string, len(cfg.SubAgents))
	for i, a := range cfg.SubAgents {
		names[i] = a.Name(ctx)
	}

	// Add a split node that fans out to all sub-agents via multiple outgoing edges.
	sg.AddNode("__wf_split__", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	sg.AddEdge(constants.Start, "__wf_split__")

	// Each sub-agent is a node that appends its output.
	for i, agent := range cfg.SubAgents {
		idx := i
		ag := agent
		nodeName := fmt.Sprintf("sub_%d", i)
		sg.AddNode(nodeName, func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*WorkflowGraphState)
			msgCopy := s.SnapshotMessages()
			iter := ag.Run(ctx, &AgentInput{Messages: msgCopy})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					return nil, fmt.Errorf("sub-agent %s: %w", names[idx], ev.Err)
				}
				if ev.Output != nil && ev.Output.MessageOutput != nil &&
					!ev.Output.MessageOutput.IsStreaming &&
					ev.Output.MessageOutput.Message != nil {
					s.AppendMessage(ev.Output.MessageOutput.Message)
				}
			}
			return map[string]interface{}{
				"Messages": s.SnapshotMessages(),
			}, nil
		})
		sg.AddEdge("__wf_split__", nodeName)
		sg.AddEdge(nodeName, constants.End)
	}

	compileOpts := []graph.CompileOption{
		graph.WithRecursionLimit(len(cfg.SubAgents) * 2),
	}
	if cptr != nil {
		compileOpts = append(compileOpts, graph.WithCheckpointer(cptr))
	}
	for _, name := range interrupts {
		compileOpts = append(compileOpts, graph.WithInterrupts(name))
	}

	compiled, err := sg.Compile(compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("compile parallel graph: %w", err)
	}

	return &WorkflowGraph{compiled: compiled}, nil
}

// ---- Loop ----

// NewLoopGraph builds a StateGraph that runs sub-agents in a loop with bounded
// iterations.
//
//	start → sub_0 → sub_1 → ... → sub_n → [iter < max?] → back to sub_0
//	                                        ↘ end
func NewLoopGraph(ctx context.Context, cfg *LoopConfig, cptr graph.Checkpointer, interrupts ...string) (*WorkflowGraph, error) {
	if cfg == nil {
		return nil, fmt.Errorf("LoopConfig is nil")
	}
	if len(cfg.SubAgents) == 0 {
		return nil, fmt.Errorf("LoopConfig requires at least one sub-agent")
	}
	sg := graph.NewStateGraph(&WorkflowGraphState{})

	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	names := make([]string, len(cfg.SubAgents))
	for i, a := range cfg.SubAgents {
		names[i] = a.Name(ctx)
	}

	// One node per sub-agent.
	for i, agent := range cfg.SubAgents {
		idx := i
		ag := agent
		nodeName := fmt.Sprintf("sub_%d", i)
		sg.AddNode(nodeName, func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*WorkflowGraphState)
			// Copy Messages slice so the sub-agent's goroutine doesn't share
			// the underlying array with the graph's concurrent goroutines.
			msgCopy := append([]*schema.Message(nil), s.Messages...)
			iter := ag.Run(ctx, &AgentInput{Messages: msgCopy})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					return nil, fmt.Errorf("sub-agent %s: %w", names[idx], ev.Err)
				}
				if ev.Output != nil && ev.Output.MessageOutput != nil &&
					!ev.Output.MessageOutput.IsStreaming &&
					ev.Output.MessageOutput.Message != nil {
					s.AppendMessage(ev.Output.MessageOutput.Message)
				}
			}
			s.CurrentStep = idx + 1
			return s, nil
		})
	}

	// Chain: start → sub_0 → sub_1 → ... → sub_n
	sg.AddEdge(constants.Start, "sub_0")
	for i := 1; i < len(cfg.SubAgents); i++ {
		sg.AddEdge(fmt.Sprintf("sub_%d", i-1), fmt.Sprintf("sub_%d", i))
	}

	// Conditional edge from last sub-agent: loop back or end.
	lastNode := fmt.Sprintf("sub_%d", len(cfg.SubAgents)-1)
	sg.AddConditionalEdges(lastNode,
		func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*WorkflowGraphState)
			s.LoopIter++
			if s.LoopIter >= maxIter {
				s.Done = true
				return constants.End, nil
			}
			s.CurrentStep = 0 // Reset for next iteration.
			return "sub_0", nil
		},
		map[string]string{
			constants.End: constants.End,
			"sub_0":       "sub_0",
		},
	)
	// Mark lastNode as a finish point so graph validation passes. The
	// conditional edge to End is the actual runtime termination path.
	sg.SetFinishPoint(lastNode)

	compileOpts := []graph.CompileOption{
		graph.WithRecursionLimit(maxIter*len(cfg.SubAgents) + 5),
	}
	if cptr != nil {
		compileOpts = append(compileOpts, graph.WithCheckpointer(cptr))
	}
	for _, name := range interrupts {
		compileOpts = append(compileOpts, graph.WithInterrupts(name))
	}

	compiled, err := sg.Compile(compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("compile loop graph: %w", err)
	}

	return &WorkflowGraph{compiled: compiled}, nil
}

// ---- Invocation ----

// toWorkflowGraphState converts the engine's result (map or typed struct)
// back to *WorkflowGraphState. The Pregel engine serializes state through
// channels which may flatten structs into map[string]interface{}.
func toWorkflowGraphState(result interface{}) (*WorkflowGraphState, error) {
	switch v := result.(type) {
	case *WorkflowGraphState:
		return v, nil
	case map[string]interface{}:
		return mapToWorkflowGraphState(v)
	default:
		return nil, fmt.Errorf("unexpected result type %T from workflow graph", result)
	}
}

// mapToWorkflowGraphState converts a map result to WorkflowGraphState.
// The Pregel engine uses Go struct field names as channel keys (PascalCase).
func mapToWorkflowGraphState(m map[string]interface{}) (*WorkflowGraphState, error) {
	s := &WorkflowGraphState{}
	if msgs, ok := m["Messages"]; ok {
		if msgList, ok := msgs.([]*schema.Message); ok {
			s.Messages = msgList
		} else if rawList, ok := msgs.([]interface{}); ok {
			for _, raw := range rawList {
				if msg, ok := raw.(*schema.Message); ok {
					s.Messages = append(s.Messages, msg)
				}
			}
		}
	}
	if step, ok := m["CurrentStep"].(int); ok {
		s.CurrentStep = step
	} else if step, ok := m["CurrentStep"].(float64); ok {
		s.CurrentStep = int(step)
	}
	if iter, ok := m["LoopIter"].(int); ok {
		s.LoopIter = iter
	} else if iter, ok := m["LoopIter"].(float64); ok {
		s.LoopIter = int(iter)
	}
	if maxIter, ok := m["MaxLoopIter"].(int); ok {
		s.MaxLoopIter = maxIter
	} else if maxIter, ok := m["MaxLoopIter"].(float64); ok {
		s.MaxLoopIter = int(maxIter)
	}
	if done, ok := m["Done"].(bool); ok {
		s.Done = done
	}
	return s, nil
}

// Invoke runs the workflow graph synchronously and returns the final state.
func (wg *WorkflowGraph) Invoke(ctx context.Context, input *AgentInput) (*WorkflowGraphState, error) {
	if wg == nil || wg.compiled == nil {
		return nil, fmt.Errorf("workflow graph is not compiled")
	}
	if input == nil {
		input = &AgentInput{}
	}
	state := &WorkflowGraphState{
		Messages:    input.Messages,
		CurrentStep: 0,
	}
	result, err := wg.compiled.Invoke(ctx, state)
	if err != nil {
		return nil, err
	}
	return toWorkflowGraphState(result)
}

// Stream runs the workflow graph with streaming events via Pregel.
func (wg *WorkflowGraph) Stream(ctx context.Context, input *AgentInput, mode types.StreamMode) (<-chan interface{}, <-chan error) {
	if input == nil {
		input = &AgentInput{}
	}
	state := &WorkflowGraphState{
		Messages:    input.Messages,
		CurrentStep: 0,
	}
	return wg.compiled.Stream(ctx, state, mode)
}

// Resume resumes a previously interrupted workflow.
// Resume resumes a previously interrupted workflow graph from its checkpoint.
// Note: this is a thin wrapper that invokes the compiled graph with an empty state.
// For proper checkpoint resume, ensure the compiled graph was configured with a
// checkpointer and the config has the correct ThreadID for checkpoint lookup.
func (wg *WorkflowGraph) Resume(ctx context.Context) (*WorkflowGraphState, error) {
	result, err := wg.compiled.Invoke(ctx, &WorkflowGraphState{})
	if err != nil {
		return nil, err
	}
	return toWorkflowGraphState(result)
}

// Compile returns the underlying CompiledGraph.
func (wg *WorkflowGraph) Compile() *graph.CompiledGraph { return wg.compiled }

// ---- helpers ----
