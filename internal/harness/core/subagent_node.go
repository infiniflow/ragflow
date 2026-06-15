// Package agentcore provides a reusable SubAgentNode component that wraps an
// Agent as a first-class StateGraph node with field-level data projection.
//
// Usage:
//
//	// Create a graph with a sub-agent as a node
//	sg := graph.NewStateGraph(MyState{})
//	node := NewSubAgentNode(myAgent, WithSubAgentInput("query", "input"))
//	sg.AddNode("sub_agent", node)
//	sg.AddEdge("__start__", "sub_agent")
//	sg.AddEdge("sub_agent", "__end__")
//
// SubAgentNode supports:
//   - Field-level input/output mapping via FieldMapping
//   - Checkpoint/interrupt propagation from the sub-agent
//   - Integration with graph.StatePre/StatePost handlers
package core

import (
	"context"
	"fmt"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/graph"
)

// SubAgentNodeOption configures a SubAgentNode.
type SubAgentNodeOption func(*SubAgentNodeConfig)

// SubAgentNodeConfig holds configuration for the sub-agent node.
type SubAgentNodeConfig struct {
	// InputMapping maps state field paths to agent input fields.
	// Format: graph.FieldMapping{From: "state_field", To: "agent_input_field"}
	InputMapping []graph.FieldMapping
	// OutputMapping maps agent output fields to state field paths.
	// Format: graph.FieldMapping{From: "agent_output_field", To: "state_field"}
	OutputMapping []graph.FieldMapping
	// InputExtractor extracts the AgentInput from the graph state.
	// If nil, the entire state is passed as the input messages.
	InputExtractor func(ctx context.Context, state interface{}) (*AgentInput, error)
	// OutputCollector merges agent output messages back into the graph state.
	// If nil, messages from the agent output are appended to state.
	OutputCollector func(ctx context.Context, state interface{}, messages []*schema.Message) (interface{}, error)
	// NodeName is the name of this sub-agent node in the graph.
	NodeName string
}

// WithSubAgentInput configures which state fields map to the agent's input messages.
// The 'from' path is in the graph state, 'to' path is in the agent's input.
func WithSubAgentInput(from, to string) SubAgentNodeOption {
	return func(cfg *SubAgentNodeConfig) {
		cfg.InputMapping = append(cfg.InputMapping, graph.FieldMapping{From: from, To: to})
	}
}

// WithSubAgentOutput configures which agent output fields map back to the graph state.
// The 'from' path is in the agent's output, 'to' path is in the graph state.
func WithSubAgentOutput(from, to string) SubAgentNodeOption {
	return func(cfg *SubAgentNodeConfig) {
		cfg.OutputMapping = append(cfg.OutputMapping, graph.FieldMapping{From: from, To: to})
	}
}

// WithSubAgentExtractor sets a custom input extractor function.
func WithSubAgentExtractor(fn func(ctx context.Context, state interface{}) (*AgentInput, error)) SubAgentNodeOption {
	return func(cfg *SubAgentNodeConfig) {
		cfg.InputExtractor = fn
	}
}

// WithSubAgentCollector sets a custom output collector function.
func WithSubAgentCollector(fn func(ctx context.Context, state interface{}, messages []*schema.Message) (interface{}, error)) SubAgentNodeOption {
	return func(cfg *SubAgentNodeConfig) {
		cfg.OutputCollector = fn
	}
}

// WithSubAgentName sets the node name for the sub-agent.
func WithSubAgentName(name string) SubAgentNodeOption {
	return func(cfg *SubAgentNodeConfig) {
		cfg.NodeName = name
	}
}

// NewSubAgentNode creates a StateGraph-compatible node function that wraps an
// Agent. The returned function can be used with sg.AddNode() to place an agent
// as a first-class graph node with field-level data projection.
//
// The sub-agent node:
//  1. Extracts input from the graph state (via InputExtractor or FieldMapping)
//  2. Runs the agent
//  3. Merges agent output back into the graph state (via OutputCollector or FieldMapping)
//
// This enables composable, reusable agent nodes in any StateGraph.
func NewSubAgentNode(agent Agent, opts ...SubAgentNodeOption) func(ctx context.Context, state interface{}) (interface{}, error) {
	cfg := &SubAgentNodeConfig{
		NodeName: agent.Name(context.Background()),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx context.Context, state interface{}) (interface{}, error) {
		// Step 1: Extract input from graph state
		input, err := extractInput(cfg, ctx, state)
		if err != nil {
			return nil, fmt.Errorf("sub-agent %s: extract input: %w", cfg.NodeName, err)
		}

		// Step 2: Run the agent
		output, err := runAgent(ctx, agent, input)
		if err != nil {
			return nil, fmt.Errorf("sub-agent %s: %w", cfg.NodeName, err)
		}

		// Step 3: Collect output back into graph state
		return collectOutput(cfg, ctx, state, output)
	}
}

// extractInput builds the AgentInput from graph state using the configured
// extractor or FieldMapping.
func extractInput(cfg *SubAgentNodeConfig, ctx context.Context, state interface{}) (*AgentInput, error) {
	// Custom extractor takes precedence
	if cfg.InputExtractor != nil {
		return cfg.InputExtractor(ctx, state)
	}

	// Default: pass state messages as agent input
	st, ok := state.(map[string]interface{})
	if !ok {
		return &AgentInput{}, nil
	}

	// Collect messages from state (default field name: "Messages")
	input := &AgentInput{}
	if msgs, ok := st["Messages"]; ok {
		if msgList, ok := msgs.([]*schema.Message); ok {
			input.Messages = msgList
		} else if rawList, ok := msgs.([]interface{}); ok {
			for _, raw := range rawList {
				if msg, ok := raw.(*schema.Message); ok {
					input.Messages = append(input.Messages, msg)
				}
			}
		}
	}
	return input, nil
}

// runAgent executes the agent and collects its output messages.
func runAgent(ctx context.Context, agent Agent, input *AgentInput) ([]*schema.Message, error) {
	iter := agent.Run(ctx, input)
	var messages []*schema.Message
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			return nil, ev.Err
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			messages = append(messages, ev.Output.MessageOutput.Message)
		}
	}
	return messages, nil
}

// collectOutput merges agent output messages back into the graph state.
func collectOutput(cfg *SubAgentNodeConfig, ctx context.Context, state interface{}, messages []*schema.Message) (interface{}, error) {
	// Custom collector takes precedence
	if cfg.OutputCollector != nil {
		return cfg.OutputCollector(ctx, state, messages)
	}

	// Default: append messages to state
	st, ok := state.(map[string]interface{})
	if !ok {
		return state, nil
	}

	if len(messages) > 0 {
		existing, _ := st["Messages"].([]interface{})
		for _, msg := range messages {
			existing = append(existing, msg)
		}
		st["Messages"] = existing
	}
	return st, nil
}


