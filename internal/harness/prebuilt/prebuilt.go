// Package prebuilt provides pre-built components for common Agent Harness patterns.
package prebuilt

import (
	"context"
	"fmt"

	"ragflow/internal/harness/graph/runnable"
)

// ReactAgentConfig holds configuration for a ReAct agent.
type ReactAgentConfig struct {
	// Tools available to the agent
	Tools []Tool
	// LLM model to use
	Model LLM
	// System prompt
	SystemPrompt string
	// Maximum iterations
	MaxIterations int
	// Stop condition
	StopCondition func(*ReActState) bool
}

// ReActState represents the state of a ReAct agent.
type ReActState struct {
	// Input from user
	Input string
	// Current thought
	Thought string
	// Current action
	Action string
	// Observation from action
	Observation string
	// Final answer
	Answer string
	// Iteration count
	Iteration int
	// Tool calls history
	ToolCalls []ToolCall
}

// Tool represents a tool that can be called by the agent.
type Tool struct {
	Name        string
	Description string
	Function    func(context.Context, map[string]interface{}) (interface{}, error)
	Schema      map[string]interface{}
}

// ToolCall represents a call to a tool.
type ToolCall struct {
	ToolName string
	Input    map[string]interface{}
	Output   interface{}
	Error    error
}

// LLM represents a language model.
type LLM interface {
	Generate(ctx context.Context, messages []map[string]interface{}) (string, error)
	GenerateStream(ctx context.Context, messages []map[string]interface{}) (<-chan string, error)
}

// NewReactAgent creates a new ReAct (Reasoning + Acting) agent.
func NewReactAgent(config ReactAgentConfig) (runnable.Runnable[map[string]interface{}, map[string]interface{}], error) {
	if len(config.Tools) == 0 {
		return nil, fmt.Errorf("at least one tool is required")
	}
	if config.Model == nil {
		return nil, fmt.Errorf("model is required")
	}
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}

	// Create the agent as a runnable
	agent := runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			state := &ReActState{
				Input:     fmt.Sprintf("%v", input["input"]),
				Iteration: 0,
				ToolCalls: make([]ToolCall, 0),
			}

			for state.Iteration < config.MaxIterations {
				// Check stop condition
				if config.StopCondition != nil && config.StopCondition(state) {
					break
				}

				// Generate thought
				thought, err := config.Model.Generate(ctx, buildMessages(state, config.SystemPrompt))
				if err != nil {
					return nil, fmt.Errorf("failed to generate thought: %w", err)
				}
				state.Thought = thought

				// Parse action from thought (simplified)
				action := parseAction(thought)
				state.Action = action

				if action == "ANSWER" {
					// Extract answer
					state.Answer = extractAnswer(thought)
					break
				}

				// Execute tool
				toolOutput, err := executeTool(ctx, action, input, config.Tools)
				state.Observation = fmt.Sprintf("%v", toolOutput)
				state.ToolCalls = append(state.ToolCalls, ToolCall{
					ToolName: action,
					Input:    input,
					Output:   toolOutput,
					Error:    err,
				})

				if err != nil {
					state.Observation = fmt.Sprintf("Tool error: %v", err)
				}

				state.Iteration++
			}

			return map[string]interface{}{
				"output":      state.Answer,
				"thoughts":    state.Thought,
				"iterations":  state.Iteration,
				"tool_calls":  state.ToolCalls,
				"final_state": state,
			}, nil
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("react_agent"),
		runnable.WithDescription[map[string]interface{}, map[string]interface{}]("ReAct agent with tools"),
	)

	return agent, nil
}

// ToolNode creates a node that executes a tool.
func ToolNode(tool Tool) runnable.Runnable[map[string]interface{}, map[string]interface{}] {
	return runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			output, err := tool.Function(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("tool %s failed: %w", tool.Name, err)
			}

			return map[string]interface{}{
				"tool":     tool.Name,
				"input":    input,
				"output":   output,
				"success":  true,
				"metadata": map[string]interface{}{"tool_schema": tool.Schema},
			}, nil
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}](fmt.Sprintf("tool_%s", tool.Name)),
		runnable.WithDescription[map[string]interface{}, map[string]interface{}](tool.Description),
	)
}

// ValidationNode creates a node that validates input.
func ValidationNode(
	validateFunc func(map[string]interface{}) error,
	errorMessage string,
) runnable.Runnable[map[string]interface{}, map[string]interface{}] {
	return runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			if err := validateFunc(input); err != nil {
				return nil, fmt.Errorf("%s: %w", errorMessage, err)
			}
			// Pass through input if valid
			return input, nil
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("validation_node"),
		runnable.WithDescription[map[string]interface{}, map[string]interface{}]("Input validation node"),
	)
}

// ConditionalNode creates a node that routes based on a condition.
func ConditionalNode(
	condition func(map[string]interface{}) string,
	branches map[string]runnable.Runnable[map[string]interface{}, map[string]interface{}],
	defaultBranch string,
) runnable.Runnable[map[string]interface{}, map[string]interface{}] {
	return runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			branchName := condition(input)
			branch, exists := branches[branchName]
			if !exists {
				if defaultBranch == "" {
					return nil, fmt.Errorf("no branch for condition '%s' and no default branch", branchName)
				}
				branch = branches[defaultBranch]
				if branch == nil {
					return nil, fmt.Errorf("default branch '%s' not found", defaultBranch)
				}
			}

			return branch.Invoke(ctx, input)
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("conditional_node"),
		runnable.WithDescription[map[string]interface{}, map[string]interface{}]("Conditional routing node"),
	)
}

// TransformNode creates a node that transforms input.
func TransformNode(
	transformFunc func(map[string]interface{}) (map[string]interface{}, error),
) runnable.Runnable[map[string]interface{}, map[string]interface{}] {
	return runnable.NewRunnableFunc(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			return transformFunc(input)
		},
		runnable.WithName[map[string]interface{}, map[string]interface{}]("transform_node"),
		runnable.WithDescription[map[string]interface{}, map[string]interface{}]("Input transformation node"),
	)
}

// Helper functions

func buildMessages(state *ReActState, systemPrompt string) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0)

	if systemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": state.Input,
	})

	if state.Thought != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "assistant",
			"content": state.Thought,
		})
	}

	if state.Observation != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": state.Observation,
		})
	}

	return messages
}

func parseAction(thought string) string {
	// Simplified parsing - in reality would use more sophisticated parsing
	if len(thought) > 10 && thought[:5] == "THINK" {
		return "THINK"
	}
	if len(thought) > 10 && thought[:6] == "ACTION" {
		// Extract tool name
		return "TOOL_CALL"
	}
	if len(thought) > 10 && thought[:6] == "ANSWER" {
		return "ANSWER"
	}
	return "THINK"
}

func extractAnswer(thought string) string {
	// Simplified extraction
	return thought
}

func executeTool(ctx context.Context, action string, input map[string]interface{}, tools []Tool) (interface{}, error) {
	// Find the tool
	for _, tool := range tools {
		if tool.Name == action {
			return tool.Function(ctx, input)
		}
	}
	return nil, fmt.Errorf("tool not found: %s", action)
}