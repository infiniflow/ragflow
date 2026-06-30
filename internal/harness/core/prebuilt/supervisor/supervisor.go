// Package supervisor provides a Supervisor agent pattern for harness-go.
// The Supervisor uses an LLM to route user requests to specialized sub-agents,
// each with their own tools and expertise.
package supervisor

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// Config configures the Supervisor agent.
type Config struct {
	Name        string
	Description string
	Model       core.Model[*schema.Message]
	Agents      []AgentSpec // Available sub-agents
	OutputKey   string      // Store final answer to session under this key
}

// AgentSpec defines a sub-agent available to the supervisor.
type AgentSpec struct {
	Name        string
	Description string
	Agent       core.Agent
}

func DefaultConfig() *Config {
	return &Config{
		Name:        "supervisor",
		Description: "A supervisor agent that routes tasks to specialized sub-agents",
	}
}

// New creates a new Supervisor as a flow agent with transfer capability.
func New(ctx context.Context, cfg *Config) (core.ResumableAgent, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.Model == nil {
		return nil, fmt.Errorf("supervisor requires a Model")
	}

	// Build agent descriptions for the prompt
	agentDescs := buildAgentDescriptions(cfg.Agents)

	instruction := fmt.Sprintf(systemPrompt, agentDescs)

	// The supervisor itself is a ReActAgent that only transfers to sub-agents
	sup := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:       cfg.Model,
		Instruction: instruction,
	})

	supAgent := sup.WithName(cfg.Name).WithDescription(cfg.Description)

	// Wrap sub-agents with deterministic transfer constraint.
	// Each sub-agent can only transfer back to the supervisor.
	wrappedSubs := make([]core.Agent, 0, len(cfg.Agents))
	for _, as := range cfg.Agents {
		wrapped := core.AgentWithDeterministicTransfer(ctx, &core.DeterministicTransferConfig{
			Agent:        as.Agent,
			ToAgentNames: []string{cfg.Name},
		})
		wrappedSubs = append(wrappedSubs, wrapped)
	}

	// TODO: Add unified tracing container for supervisor identification.
	// Currently NewReActAgent returns a concrete type, so we cannot
	// easily add a GetType() method to identify the supervisor.

	flow, err := core.SetSubAgents(ctx, supAgent, wrappedSubs)
	if err != nil {
		return nil, fmt.Errorf("set sub-agents: %w", err)
	}

	return flow, nil
}

func buildAgentDescriptions(agents []AgentSpec) string {
	if len(agents) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", a.Name, a.Description))
	}
	return sb.String()
}

const systemPrompt = `You are a supervisor agent. Your job is to understand the user's request and route it to the most appropriate specialist agent.

Available agents:
%s

Instructions:
1. Analyze the user's request carefully
2. Choose the best agent from the list above
3. Use the transfer_to_agent tool to delegate the task to that agent
4. If no agent is suitable, respond directly with your best attempt to help

You should always try to route to a specialist agent when one matches the request domain.`

// ---- Convenience constructor with common patterns ----

// NewWithRouter creates a supervisor using a pure routing approach:
// the LLM chooses which agent handles the request, then transfers to it.
func NewWithRouter(ctx context.Context, model core.Model[*schema.Message], agents []AgentSpec) (core.ResumableAgent, error) {
	return New(ctx, &Config{Model: model, Agents: agents})
}
