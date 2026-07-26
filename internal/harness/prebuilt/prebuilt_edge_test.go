package prebuilt

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/harness/graph/runnable"
)

// testLLM implements the LLM interface for testing
type testLLM struct {
	responses []string
	index     int
}

func (m *testLLM) Generate(ctx context.Context, messages []map[string]interface{}) (string, error) {
	if m.index >= len(m.responses) {
		return "", fmt.Errorf("no more responses")
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

func (m *testLLM) GenerateStream(ctx context.Context, messages []map[string]interface{}) (<-chan string, error) {
	ch := make(chan string, 1)
	if m.index >= len(m.responses) {
		close(ch)
		return ch, fmt.Errorf("no more responses")
	}
	response := m.responses[m.index]
	m.index++
	ch <- response
	close(ch)
	return ch, nil
}

func TestPrebuiltAgent_MaxIterations(t *testing.T) {
	t.Run("default_max_iterations", func(t *testing.T) {
		mock := &testLLM{
			responses: []string{
				"THINK: searching\nACTION: search\nQUERY: test",
				"THINK: still searching\nACTION: search\nQUERY: again",
				"ANSWER: done",
			},
		}
		config := ReactAgentConfig{
			Tools: []Tool{
				{Name: "search", Description: "search tool",
					Function: func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
						return "result", nil
					}},
			},
			Model:         mock,
			SystemPrompt:  "test",
			MaxIterations: 0,
		}

		agent, err := NewReactAgent(config)
		if err != nil {
			t.Fatalf("NewReactAgent: %v", err)
		}

		ctx := context.Background()
		output, err := agent.Invoke(ctx, map[string]interface{}{"input": "test"})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		iterations := output["iterations"].(int)
		if iterations <= 0 {
			t.Errorf("expected positive iterations, got %d", iterations)
		}
		t.Logf("MaxIterations=0: completed %d iterations", iterations)
	})

	t.Run("stop_condition_terminates_early", func(t *testing.T) {
		mock := &testLLM{
			responses: []string{
				"THINK: first\nACTION: search\nQUERY: q1",
				"THINK: second\nACTION: search\nQUERY: q2",
				"THINK: third\nACTION: search\nQUERY: q3",
			},
		}
		callCount := 0
		config := ReactAgentConfig{
			Tools: []Tool{
				{Name: "search", Description: "search",
					Function: func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
						callCount++
						return "result", nil
					}},
			},
			Model:        mock,
			SystemPrompt: "test",
			StopCondition: func(state *ReActState) bool {
				return state.Iteration >= 1
			},
			MaxIterations: 10,
		}

		agent, err := NewReactAgent(config)
		if err != nil {
			t.Fatalf("NewReactAgent: %v", err)
		}

		ctx := context.Background()
		output, err := agent.Invoke(ctx, map[string]interface{}{"input": "test"})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		iterations := output["iterations"].(int)
		if iterations > 2 {
			t.Errorf("expected <=2 iterations due to stop condition, got %d", iterations)
		}
		t.Logf("stop condition: completed %d iterations", iterations)
	})

	t.Run("no_tools_returns_error", func(t *testing.T) {
		_, err := NewReactAgent(ReactAgentConfig{
			Tools: nil,
			Model: &testLLM{},
		})
		if err == nil {
			t.Error("expected error for empty tools")
		}
	})

	t.Run("no_model_returns_error", func(t *testing.T) {
		_, err := NewReactAgent(ReactAgentConfig{
			Tools: []Tool{{Name: "t", Description: "t",
				Function: func(ctx context.Context, input map[string]interface{}) (interface{}, error) { return nil, nil }}},
			Model: nil,
		})
		if err == nil {
			t.Error("expected error for nil model")
		}
	})

	t.Run("unknown_action_handled", func(t *testing.T) {
		mock := &testLLM{
			responses: []string{
				"THINK: unknown action\nACTION: nonexistent\nQUERY: test",
				"THINK: try again\nACTION: search\nQUERY: retry",
				"ANSWER: done",
			},
		}
		config := ReactAgentConfig{
			Tools: []Tool{
				{Name: "search", Description: "search",
					Function: func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
						return "result", nil
					}},
			},
			Model:         mock,
			SystemPrompt:  "test",
			MaxIterations: 3,
		}

		agent, err := NewReactAgent(config)
		if err != nil {
			t.Fatalf("NewReactAgent: %v", err)
		}

		ctx := context.Background()
		output, err := agent.Invoke(ctx, map[string]interface{}{"input": "test"})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		t.Logf("unknown action handled: iterations=%d", output["iterations"])
	})
}

func TestPrebuiltAgent_TransformNodeEdgeCases(t *testing.T) {
	ctx := context.Background()

	tn := TransformNode(func(input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"transformed": fmt.Sprintf("%v", input["data"]),
		}, nil
	})

	output, err := tn.Invoke(ctx, map[string]interface{}{"data": "test"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if output["transformed"] != "test" {
		t.Errorf("expected 'test', got %v", output["transformed"])
	}
}

func TestPrebuiltAgent_ConditionalNodeEdgeCases(t *testing.T) {
	ctx := context.Background()

	branchA := runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"branch": "A"}, nil
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("branch_a"),
	)

	t.Run("no_matching_branch_no_default", func(t *testing.T) {
		cn := ConditionalNode(
			func(input map[string]interface{}) string { return "MISSING" },
			map[string]runnable.Runnable[map[string]interface{}, map[string]interface{}]{
				"A": branchA,
			},
			"",
		)
		_, err := cn.Invoke(ctx, map[string]interface{}{})
		if err == nil {
			t.Error("expected error for missing branch with no default")
		}
	})

	t.Run("default_branch_used", func(t *testing.T) {
		cn := ConditionalNode(
			func(input map[string]interface{}) string { return "MISSING" },
			map[string]runnable.Runnable[map[string]interface{}, map[string]interface{}]{
				"A": branchA,
			},
			"A",
		)
		output, err := cn.Invoke(ctx, map[string]interface{}{})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if output["branch"] != "A" {
			t.Errorf("expected branch A from default, got %v", output["branch"])
		}
	})
}
