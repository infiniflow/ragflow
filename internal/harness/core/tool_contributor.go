package core

import (
	"context"

	"ragflow/internal/harness/core/schema"
)

// ToolContributor is an optional interface that middlewares can implement to
// contribute tools, tool infos, and return-directly entries to the agent.
//
// The agent loop collects contributions BEFORE calling BeforeAgent, ensuring
// that tools are available for both ToolsNode construction and BeforeAgent
// middleware processing. This replaces the unreliable pattern of modifying
// rc.Tools in BeforeAgent (which doesn't propagate to ToolsNode) and the
// timing-coupled BindToConfig pattern.
//
// Example:
//
//	type myMiddleware struct {
//	    BaseMiddleware[M]
//	}
//
//	func (m *myMiddleware) ContributeTools(ctx context.Context) []Tool {
//	    return []Tool{NewBaseTool("my_tool", "Does something", myFunc)}
//	}
type ToolContributor[M MessageType] interface {
	// ContributeTools returns tools to add to the agent's tool set.
	// Called once during agent build (before BeforeAgent).
	ContributeTools(ctx context.Context) []Tool

	// ContributeToolInfos returns structured ToolInfo entries to bind to the
	// model. These are merged with auto-generated infos from ContributeTools.
	// Use this for special entries that don't correspond to a Tool (e.g.,
	// a meta-tool like "search_tools" for dynamic tool search).
	ContributeToolInfos(ctx context.Context) []*schema.ToolInfo

	// ContributeReturnDirectly returns tool names that should cause the agent
	// to return immediately after execution. Merged with config-level
	// ReturnDirectly.
	ContributeReturnDirectly(ctx context.Context) map[string]bool
}

// ---- Collection helpers ----

// collectContributorTools returns tools contributed by all ToolContributor middlewares.
func collectContributorTools[M MessageType](ctx context.Context, middlewares []TypedReActMiddleware[M]) []Tool {
	var all []Tool
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		if c, ok := mw.(ToolContributor[M]); ok {
			all = append(all, c.ContributeTools(ctx)...)
		}
	}
	return all
}

// collectContributorToolInfos returns tool infos contributed by all ToolContributor middlewares.
func collectContributorToolInfos[M MessageType](ctx context.Context, middlewares []TypedReActMiddleware[M]) []*schema.ToolInfo {
	var all []*schema.ToolInfo
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		if c, ok := mw.(ToolContributor[M]); ok {
			all = append(all, c.ContributeToolInfos(ctx)...)
		}
	}
	return all
}

// collectContributorReturnDirectly merges return-directly entries from all ToolContributor middlewares.
func collectContributorReturnDirectly[M MessageType](ctx context.Context, middlewares []TypedReActMiddleware[M]) map[string]bool {
	all := make(map[string]bool)
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		if c, ok := mw.(ToolContributor[M]); ok {
			for k, v := range c.ContributeReturnDirectly(ctx) {
				all[k] = v
			}
		}
	}
	return all
}
