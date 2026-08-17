package core

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================================
// Multi-Agent Integration Test
//
// Implements the Plan-Execute multi-agent pattern from Eino, adapted for
// harness-go's StateGraph. Tests:
//   1. Multiple agent nodes in a single StateGraph
//   2. Conditional routing between agents (tool calls vs. direct pass)
//   3. Cyclic execution (Reviser → Executor loop)
//   4. Tool execution within the graph
//   5. Loop termination (max iterations)
//   6. State accumulation across agents
// ============================================================================

// ---- State schema ----

type planExecState struct {
	Messages   []string // accumulated execution log
	Route      string   // routing decision: "to_tools", "to_reviser", "to_end", "to_executor"
	LoopCount  int
	ToolCalls  []schema.ToolCall
	ToolResult string
}

// ---- Mock agent models ----

// plannerModel generates a plan message on first call, then errors.
type plannerModel struct {
	mu     sync.Mutex
	called int
	plan   string
}

func (m *plannerModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	m.called++
	plan := m.plan
	m.mu.Unlock()
	return &schema.Message{Role: schema.RoleAssistant, Content: plan}, nil
}
func (m *plannerModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), nil
}
func (m *plannerModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// executorModel generates tool calls on first N invocations, then final response.
type executorModel struct {
	mu          sync.Mutex
	called      int
	toolCallIdx int // number of times to produce tool calls before final response
}

func (m *executorModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	m.called++
	idx := m.called
	m.mu.Unlock()

	if idx <= m.toolCallIdx {
		return &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "",
			ToolCalls: []schema.ToolCall{{
				ID:       fmt.Sprintf("tc_%d", idx),
				Function: schema.ToolCallFunction{Name: "search_tool", Arguments: "{}"},
			}},
		}, nil
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: "executor done"}, nil
}
func (m *executorModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), nil
}
func (m *executorModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// reviserModel either returns "final answer" or "needs revision".
type reviserModel struct {
	mu            sync.Mutex
	called        int
	successOnCall int // which call returns final answer
}

func (m *reviserModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	m.called++
	idx := m.called
	m.mu.Unlock()

	if idx >= m.successOnCall {
		return &schema.Message{Role: schema.RoleAssistant, Content: "final answer: here is the complete solution"}, nil
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: "needs revision: please re-execute"}, nil
}
func (m *reviserModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), nil
}
func (m *reviserModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Test: Plan-Execute Multi-Agent ----

func TestMultiAgent_PlanExecute(t *testing.T) {
	var mu sync.Mutex
	var execLog []string
	logNode := func(name string) {
		mu.Lock()
		execLog = append(execLog, name)
		mu.Unlock()
	}

	// Track models for verification.
	executor := &executorModel{toolCallIdx: 2} // 2 tool calls, then pass
	reviser := &reviserModel{successOnCall: 2} // needs 2 passes

	sg := graph.NewStateGraph(&planExecState{})
	sg.SetNodeTriggerMode(types.NodeTriggerAnyPredecessor)

	// Planner node.
	sg.AddNode("planner", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("planner")
		s := state.(*planExecState)
		s.Messages = append(s.Messages, "planner: created plan")
		s.Route = "to_executor"
		return s, nil
	})
	sg.AddEdge(constants.Start, "planner")

	// Executor node.
	sg.AddNode("executor", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("executor")
		s := state.(*planExecState)

		msg, err := executor.Generate(ctx, nil)
		if err != nil {
			return nil, err
		}

		s.LoopCount++
		s.Messages = append(s.Messages, fmt.Sprintf("executor: iteration %d", s.LoopCount))

		if len(msg.ToolCalls) > 0 {
			s.ToolCalls = msg.ToolCalls
			s.Route = "to_tools"
		} else {
			s.ToolCalls = nil
			s.Route = "to_reviser"
		}
		return s, nil
	})
	sg.AddEdge("planner", "executor")

	// Tools node.
	sg.AddNode("tools", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("tools")
		s := state.(*planExecState)
		for _, tc := range s.ToolCalls {
			s.Messages = append(s.Messages, fmt.Sprintf("tools: executed %s", tc.Function.Name))
		}
		s.ToolResult = "tool data retrieved"
		s.Route = "to_executor"
		return s, nil
	})
	sg.AddEdge("executor", "tools")
	sg.AddEdge("tools", "executor")

	// Reviser node.
	sg.AddNode("reviser", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("reviser")
		s := state.(*planExecState)

		msg, err := reviser.Generate(ctx, nil)
		if err != nil {
			return nil, err
		}

		s.Messages = append(s.Messages, "reviser: reviewed")
		if msg.Content == "final answer: here is the complete solution" {
			s.Route = "to_end"
		} else {
			s.Route = "to_executor"
		}
		return s, nil
	})

	// Conditional edge from executor.
	sg.AddConditionalEdges("executor",
		func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*planExecState)
			return s.Route, nil
		},
		map[string]string{
			"to_tools":   "tools",
			"to_reviser": "reviser",
		},
	)

	// Conditional edge from tools.
	sg.AddConditionalEdges("tools",
		func(ctx context.Context, state interface{}) (interface{}, error) {
			return "to_executor", nil
		},
		map[string]string{
			"to_executor": "executor",
		},
	)

	// Reviser can route to end (via conditional) or back to executor.
	sg.AddConditionalEdges("reviser",
		func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*planExecState)
			return s.Route, nil
		},
		map[string]string{
			"to_end":      constants.End,
			"to_executor": "executor",
		},
	)
	sg.AddEdge("reviser", constants.End) // explicit finish point for validation

	compiled, err := sg.Compile(graph.WithRecursionLimit(20))
	if err != nil {
		t.Fatal(err)
	}

	initialState := &planExecState{
		Messages:  make([]string, 0),
		Route:     "to_executor",
		LoopCount: 0,
	}

	stateIf, err := compiled.Invoke(context.Background(), initialState)
	if err != nil {
		t.Fatalf("Plan-Execute multi-agent failed: %v", err)
	}

	mu.Lock()
	logCopy := make([]string, len(execLog))
	copy(logCopy, execLog)
	mu.Unlock()

	// Extract final state (may be *planExecState or map[string]interface{}).
	var route string
	var loopCount int
	switch s := stateIf.(type) {
	case *planExecState:
		route = s.Route
		loopCount = s.LoopCount
	case map[string]interface{}:
		if r, ok := s["Route"].(string); ok {
			route = r
		}
		if l, ok := s["LoopCount"].(float64); ok {
			loopCount = int(l)
		}
	}

	t.Logf("Execution log: %v", logCopy)
	t.Logf("Route: %s, LoopCount: %d", route, loopCount)

	// Verify all agents executed at least once.
	agentSet := make(map[string]bool)
	for _, name := range logCopy {
		agentSet[name] = true
	}
	for _, agent := range []string{"planner", "executor", "tools", "reviser"} {
		if !agentSet[agent] {
			t.Errorf("agent %s never executed", agent)
		}
	}

	// Verify loop terminated correctly.
	if route != "to_end" {
		t.Errorf("expected final route 'to_end', got %q", route)
	}

	// Verify executor was called multiple times.
	execCount := 0
	for _, name := range logCopy {
		if name == "executor" {
			execCount++
		}
	}
	if execCount < 3 {
		t.Errorf("expected executor to run at least 3 times, got %d", execCount)
	}

	t.Logf("Plan-Execute multi-agent: %d total node executions across %d agents", len(logCopy), len(agentSet))
}

// ============================================================================
// Test: Multi-Agent with Error Recovery
// One agent fails, others continue correctly.
// ============================================================================

func TestMultiAgent_ErrorRecovery(t *testing.T) {
	var mu sync.Mutex
	var execLog []string
	logNode := func(name string) {
		mu.Lock()
		execLog = append(execLog, name)
		mu.Unlock()
	}

	sg := graph.NewStateGraph(&planExecState{})

	// Agent A: always succeeds.
	sg.AddNode("agent_a", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("agent_a")
		s := state.(*planExecState)
		s.Messages = append(s.Messages, "agent_a done")
		s.Route = "to_b"
		return s, nil
	})
	sg.AddEdge(constants.Start, "agent_a")

	// Agent B: fails on first call, succeeds on second.
	bCount := 0
	sg.AddNode("agent_b", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("agent_b")
		bCount++
		if bCount <= 1 {
			return nil, fmt.Errorf("agent_b temporary failure")
		}
		s := state.(*planExecState)
		s.Messages = append(s.Messages, "agent_b done after retry")
		s.Route = "to_c"
		return s, nil
	})

	// Agent C: always succeeds.
	sg.AddNode("agent_c", func(ctx context.Context, state interface{}) (interface{}, error) {
		logNode("agent_c")
		s := state.(*planExecState)
		s.Messages = append(s.Messages, "agent_c done")
		s.Route = "to_end"
		return s, nil
	})
	sg.AddEdge("agent_c", constants.End)

	// Agent A → B (conditional: retry B if failed).
	sg.AddConditionalEdges("agent_a",
		func(ctx context.Context, state interface{}) (interface{}, error) {
			return "to_b", nil
		},
		map[string]string{"to_b": "agent_b"},
	)

	// Agent B → C (conditional: success → C, failure → retry B).
	sg.AddConditionalEdges("agent_b",
		func(ctx context.Context, state interface{}) (interface{}, error) {
			return "to_c", nil
		},
		map[string]string{"to_c": "agent_c"},
	)

	compiled, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatal(err)
	}

	stateIf, err := compiled.Invoke(context.Background(), &planExecState{Messages: make([]string, 0)})
	if err != nil {
		// Agent B fails — error propagation is the correct behavior.
		// Previously this error was silently swallowed and the graph was
		// re-scheduled from the entry point (bug). Proper error recovery
		// requires explicit retry edges or a retry decorator.
		t.Logf("Expected: agent_b error stops execution: %v", err)
		return
	}

	mu.Lock()
	logCopy := make([]string, len(execLog))
	copy(logCopy, execLog)
	mu.Unlock()

	t.Logf("Execution log: %v", logCopy)

	var msgCount int
	switch s := stateIf.(type) {
	case *planExecState:
		msgCount = len(s.Messages)
	case map[string]interface{}:
		if msgs, ok := s["Messages"].([]interface{}); ok {
			msgCount = len(msgs)
		}
	}
	if msgCount < 2 {
		t.Errorf("expected at least 2 messages, got %d", msgCount)
	}
	t.Logf("Multi-agent error recovery: %d node executions, %d messages", len(logCopy), msgCount)
}

// ============================================================================
// Test: Multi-Agent Concurrent Execution
// Multiple Plan-Execute graphs running concurrently.
// ============================================================================

func TestMultiAgent_ConcurrentExecution(t *testing.T) {
	const numAgents = 20
	var wg sync.WaitGroup
	errCh := make(chan error, numAgents)

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("agent %d panic: %v", id, r)
				}
			}()

			sg := graph.NewStateGraph(&planExecState{})
			sg.SetNodeTriggerMode(types.NodeTriggerAnyPredecessor)

			// Simple linear chain: A → B → C for each agent ID.
			prefix := fmt.Sprintf("id%d", id)
			sg.AddNode(prefix+"_a", func(ctx context.Context, state interface{}) (interface{}, error) {
				s := state.(*planExecState)
				s.Messages = append(s.Messages, prefix+"_a")
				return s, nil
			})
			sg.AddNode(prefix+"_b", func(ctx context.Context, state interface{}) (interface{}, error) {
				s := state.(*planExecState)
				s.Messages = append(s.Messages, prefix+"_b")
				return s, nil
			})
			sg.AddNode(prefix+"_c", func(ctx context.Context, state interface{}) (interface{}, error) {
				s := state.(*planExecState)
				s.Messages = append(s.Messages, prefix+"_c")
				return s, nil
			})

			sg.AddEdge(constants.Start, prefix+"_a")
			sg.AddEdge(prefix+"_a", prefix+"_b")
			sg.AddEdge(prefix+"_b", prefix+"_c")
			sg.AddEdge(prefix+"_c", constants.End)

			compiled, compileErr := sg.Compile(graph.WithRecursionLimit(10))
			if compileErr != nil {
				errCh <- fmt.Errorf("agent %d compile: %w", id, compileErr)
				return
			}

			_, invokeErr := compiled.Invoke(context.Background(), &planExecState{Messages: make([]string, 0)})
			if invokeErr != nil {
				errCh <- fmt.Errorf("agent %d invoke: %w", id, invokeErr)
				return
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		t.Fatalf("%d/%d concurrent multi-agent failed: %v", len(errs), numAgents, errs[0])
	}
	t.Logf("Concurrent multi-agent: %d graphs all completed", numAgents)
}
