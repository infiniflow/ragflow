package prebuilt

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/harness/graph/runnable"
)

func TestToolNode(t *testing.T) {
	ctx := context.Background()

	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		Function: func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"result":    input["value"],
				"processed": true,
			}, nil
		},
		Schema: map[string]interface{}{
			"type": "object",
		},
	}

	node := ToolNode(tool)

	input := map[string]interface{}{
		"value": "test",
		"extra": "data",
	}

	output, err := node.Invoke(ctx, input)
	if err != nil {
		t.Fatalf("ToolNode failed: %v", err)
	}

	if output["tool"] != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got %v", output["tool"])
	}

	if !output["success"].(bool) {
		t.Error("Expected success to be true")
	}
}

func TestValidationNode(t *testing.T) {
	ctx := context.Background()

	validationNode := ValidationNode(
		func(input map[string]interface{}) error {
			if input["required"] == nil {
				return fmt.Errorf("required field missing")
			}
			return nil
		},
		"validation failed",
	)

	// Test valid input
	validInput := map[string]interface{}{
		"required": "present",
		"extra":    "data",
	}

	_, err := validationNode.Invoke(ctx, validInput)
	if err != nil {
		t.Fatalf("ValidationNode failed on valid input: %v", err)
	}

	// Test invalid input
	invalidInput := map[string]interface{}{
		"extra": "data",
	}

	_, err = validationNode.Invoke(ctx, invalidInput)
	if err == nil {
		t.Error("Expected error for invalid input")
	}
}

func TestTransformNode(t *testing.T) {
	ctx := context.Background()

	transformNode := TransformNode(
		func(input map[string]interface{}) (map[string]interface{}, error) {
			transformed := make(map[string]interface{})
			for k, v := range input {
				transformed[k] = fmt.Sprintf("transformed_%v", v)
			}
			return transformed, nil
		},
	)

	input := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	output, err := transformNode.Invoke(ctx, input)
	if err != nil {
		t.Fatalf("TransformNode failed: %v", err)
	}

	if output["key1"] != "transformed_value1" {
		t.Errorf("Expected transformed value, got %v", output["key1"])
	}
}

func TestConditionalNode(t *testing.T) {
	ctx := context.Background()

	// Create branch runnables
	branchA := runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"branch": "A",
				"input":  input,
			}, nil
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("branch_a"),
	)

	branchB := runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"branch": "B",
				"input":  input,
			}, nil
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("branch_b"),
	)

	conditionalNode := ConditionalNode(
		func(input map[string]interface{}) string {
			if input["route"] == "a" {
				return "A"
			}
			return "B"
		},
		map[string]runnable.Runnable[map[string]interface{}, map[string]interface{}]{
			"A": branchA,
			"B": branchB,
		},
		"B", // default branch
	)

	// Test branch A
	inputA := map[string]interface{}{
		"route": "a",
		"data":  "test",
	}

	outputA, err := conditionalNode.Invoke(ctx, inputA)
	if err != nil {
		t.Fatalf("ConditionalNode failed for branch A: %v", err)
	}

	if outputA["branch"] != "A" {
		t.Errorf("Expected branch A, got %v", outputA["branch"])
	}

	// Test branch B
	inputB := map[string]interface{}{
		"route": "b",
		"data":  "test",
	}

	outputB, err := conditionalNode.Invoke(ctx, inputB)
	if err != nil {
		t.Fatalf("ConditionalNode failed for branch B: %v", err)
	}

	if outputB["branch"] != "B" {
		t.Errorf("Expected branch B, got %v", outputB["branch"])
	}
}

func TestNewReactAgent(t *testing.T) {
	// Skip this test in short mode as it requires more setup
	if testing.Short() {
		t.Skip("Skipping ReactAgent test in short mode")
	}

	ctx := context.Background()

	// Create mock tools
	tools := []Tool{
		{
			Name:        "search",
			Description: "Search for information",
			Function: func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
				return "Search results for: " + fmt.Sprintf("%v", input["query"]), nil
			},
			Schema: map[string]interface{}{
				"type": "object",
			},
		},
	}

	// Create mock LLM
	mockLLM := &mockLLM{
		responses: []string{
			"THINK: I need to search for information\nACTION: search\nQUERY: test query",
			"ANSWER: The answer is 42",
		},
	}

	config := ReactAgentConfig{
		Tools:         tools,
		Model:         mockLLM,
		SystemPrompt:  "You are a helpful assistant",
		MaxIterations: 3,
		StopCondition: nil,
	}

	agent, err := NewReactAgent(config)
	if err != nil {
		t.Fatalf("Failed to create ReactAgent: %v", err)
	}

	input := map[string]interface{}{
		"input": "What is the meaning of life?",
	}

	output, err := agent.Invoke(ctx, input)
	if err != nil {
		t.Fatalf("ReactAgent failed: %v", err)
	}

	if output["output"] == nil {
		t.Error("Expected output to have answer")
	}
}

// mockLLM implements the LLM interface for testing
type mockLLM struct {
	responses []string
	index     int
}

func (m *mockLLM) Generate(ctx context.Context, messages []map[string]interface{}) (string, error) {
	if m.index >= len(m.responses) {
		return "", fmt.Errorf("no more responses")
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

func (m *mockLLM) GenerateStream(ctx context.Context, messages []map[string]interface{}) (<-chan string, error) {
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
