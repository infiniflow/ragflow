// Package subagent provides a middleware that injects sub-agent tools into a
// parent ReAct agent, with support for declarative agent config, middleware
// inheritance, and recursion depth protection.
//
// Quick Start:
//
//	// Declarative sub-agent config (no pre-built Agent needed).
//	spec := subagent.SubAgentSpec{
//	    Name:        "researcher",
//	    Description: "Research a topic using web search",
//	    AgentConfig: &subagent.AgentConfig{
//	        Model:        anthropicModel,
//	        Tools:        []core.Tool{searchTool},
//	        SystemPrompt: "You are a research assistant.",
//	    },
//	}
//	mw := subagent.New([]subagent.SubAgentSpec{spec}, &subagent.Config{
//	    EmitInternalEvents: true,
//	    MaxDepth:           5,
//	})
//
//	cfg := &core.ReActConfig[*schema.Message]{
//	    Model:       parentModel,
//	    Middlewares: []core.ReActMiddleware{mw, filesystemMW},
//	}
//	mw.BindToConfig(ctx, cfg)  // injects sub-agent tools + forces inline dispatch
//	agent := core.NewReActAgent(cfg)
//
// The sub-agent automatically inherits the parent's non-subagent middlewares
// (e.g. filesystem) when InheritParentMiddlewares is true on the spec.
//
// MaxDepth limits nested sub-agent call depth. When exceeded, the sub-agent
// returns an error (checked via context.Context value propagation across
// AgentTool invocations).
package subagent

import (
	"context"
	"fmt"
	"sync"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Marker interface for middleware inheritance filtering ----

type subAgentMarker interface{ isSubAgentMiddleware() }

// ---- Configuration types ----

// SubAgentSpec declares a sub-agent that can be invoked by the parent agent's
// LLM via a tool call.
//
// At least one of Agent or AgentConfig must be set:
//   - Agent: a pre-built core.Agent instance.
//   - AgentConfig: declarative config from which the Agent is built on BindToConfig.
//
// When both are nil, the spec is silently skipped.
type SubAgentSpec struct {
	// Name is the tool name the LLM uses to invoke this sub-agent.
	Name string
	// Description is the tool description shown to the LLM.
	Description string

	// Agent is a pre-built Agent instance. Mutually exclusive with AgentConfig
	// (AgentConfig takes precedence when both are set).
	Agent core.Agent

	// AgentConfig declaratively describes the sub-agent. The Agent is built
	// from this config when BindToConfig is called. Overrides Agent when both set.
	AgentConfig *AgentConfig

	// AgentFactory is called on first use (inside BindToConfig) to create the
	// Agent. Ignored when either Agent or AgentConfig is set.
	AgentFactory func(ctx context.Context) (core.Agent, error)

	// InheritParentMiddlewares copies the parent agent's non-subagent middlewares
	// into this sub-agent's middleware chain. The SubAgentMiddleware itself is
	// automatically excluded to prevent infinite recursion. Additional middlewares
	// can be excluded via ExcludedParentMiddlewareNames.
	//
	// Inherited middlewares are prepended before AgentConfig.Middlewares.
	InheritParentMiddlewares bool

	// ExcludedParentMiddlewareNames lists the fully-qualified type names (as
	// returned by fmt.Sprintf("%T", mw)) of parent middlewares to skip when
	// InheritParentMiddlewares is true. For example:
	//   "*filesystem.middleware[*schema.Message]"
	ExcludedParentMiddlewareNames []string
}

// AgentConfig declaratively describes an agent to be built by the
// SubAgentMiddleware. Use this instead of providing a pre-built Agent.
type AgentConfig struct {
	// Model is the chat model for the sub-agent.
	Model core.Model[*schema.Message]

	// Tools available to the sub-agent.
	Tools []core.Tool

	// SystemPrompt is the system instruction for the sub-agent.
	SystemPrompt string

	// MaxIterations limits the ReAct loop (default: 10).
	MaxIterations int

	// Middlewares specific to this sub-agent. When InheritParentMiddlewares
	// is true, these are appended AFTER inherited parent middlewares.
	Middlewares []core.ReActMiddleware
}

// Config configures the SubAgentMiddleware behaviour.
type Config struct {
	// EmitInternalEvents forwards the sub-agent's internal events to the
	// parent agent's event stream.
	EmitInternalEvents bool

	// MaxDepth limits sub-agent recursion depth. 0 = unlimited.
	// A depth of 1 allows one level of sub-agent nesting (parent → sub).
	// Each nested AgentTool call increments the depth via context.Context.
	MaxDepth int
}

// ---- Middleware ----

// SubAgentMiddleware injects sub-agents as dynamic tools into a parent ReAct agent.
//
// Tool contribution is now handled via the ToolContributor interface (ContributeTools).
// This eliminates the need to call BindToConfig in most cases. For specs that need
// middleware inheritance (InheritParentMiddlewares), call Init(ctx, parentConfig)
// after adding the middleware to the config's Middlewares slice.
//
// Deprecated: BindToConfig is retained for backward compatibility. Prefer using
// Init or the ToolContributor interface directly.
type SubAgentMiddleware struct {
	core.BaseMiddleware[*schema.Message]

	cfg        *Config
	specs      []SubAgentSpec
	mu         sync.Mutex
	tools      []core.Tool // AgentTool wrappers, built in ensureBuilt
	infos      []*schema.ToolInfo
	builtInfos []*schema.ToolInfo // only specs that were actually built
	built      bool
	parentCfg  *core.ReActConfig[*schema.Message] // stored by Init for middleware inheritance
}

// New creates a SubAgentMiddleware. Pass nil for cfg to use defaults.
//
// specs are validated immediately; AgentTool wrappers are created lazily in
// BindToConfig (where the parent's ReActConfig is available for middleware
// inheritance).
func New(specs []SubAgentSpec, cfg *Config) *SubAgentMiddleware {
	if cfg == nil {
		cfg = &Config{}
	}
	// Pre-build ToolInfo entries (names/descriptions are always available).
	infos := make([]*schema.ToolInfo, 0, len(specs))
	for _, spec := range specs {
		infos = append(infos, &schema.ToolInfo{
			Name:        spec.Name,
			Description: spec.Description,
		})
	}
	return &SubAgentMiddleware{
		cfg:   cfg,
		specs: specs,
		infos: infos,
	}
}

// ---- ToolContributor interface ----
//
// ContributeTools returns AgentTool wrappers for all specs that can be built
// without middleware inheritance. For specs needing InheritParentMiddlewares,
// call Init(ctx, parentConfig) before the agent is built.
func (m *SubAgentMiddleware) ContributeTools(ctx context.Context) []core.Tool {
	// If Init was called with a parent config, ensureBuilt handles inheritance.
	if m.parentCfg != nil {
		m.ensureBuilt(ctx, m.parentCfg)
	} else {
		// No parent config: build simple agents (no middleware inheritance).
		m.ensureBuiltSimple(ctx)
	}
	m.mu.Lock()
	tools := make([]core.Tool, len(m.tools))
	copy(tools, m.tools)
	m.mu.Unlock()
	return tools
}

func (m *SubAgentMiddleware) ContributeToolInfos(ctx context.Context) []*schema.ToolInfo {
	if m.parentCfg != nil {
		m.ensureBuilt(ctx, m.parentCfg)
	} else {
		m.ensureBuiltSimple(ctx)
	}
	m.mu.Lock()
	infos := make([]*schema.ToolInfo, len(m.builtInfos))
	copy(infos, m.builtInfos)
	m.mu.Unlock()
	return infos
}

func (m *SubAgentMiddleware) ContributeReturnDirectly(ctx context.Context) map[string]bool {
	// Sub-agent tools should not cause the parent to return directly.
	return nil
}

// ensureBuiltSimple builds all specs without middleware inheritance.
func (m *SubAgentMiddleware) ensureBuiltSimple(ctx context.Context) {
	m.mu.Lock()
	if m.built {
		m.mu.Unlock()
		return
	}
	m.built = true
	m.mu.Unlock()

	for _, spec := range m.specs {
		agent := m.resolveAgent(ctx, spec, nil)
		if agent == nil {
			continue
		}
		m.mu.Lock()
		m.builtInfos = append(m.builtInfos, &schema.ToolInfo{
			Name: spec.Name, Description: spec.Description,
		})
		opts := []core.AgentToolOption{}
		if m.cfg.EmitInternalEvents {
			opts = append(opts, core.WithEmitInternalEvents())
		}
		if m.cfg.MaxDepth > 0 {
			opts = append(opts, core.WithMaxDepth(m.cfg.MaxDepth))
		}
		tool := core.NewAgentTool(ctx, agent, opts...)
		m.tools = append(m.tools, tool)
		m.mu.Unlock()
	}
}

// Init initializes the middleware with the parent agent config, enabling
// middleware inheritance for SubAgentSpecs with InheritParentMiddlewares.
// Call this after adding the middleware to config.Middlewares, but before
// calling NewReActAgent.
//
// Example:
//
//	mw := subagent.New(specs, &subagent.Config{EmitInternalEvents: true, MaxDepth: 5})
//	cfg.Middlewares = append(cfg.Middlewares, mw)
//	mw.Init(ctx, cfg)
//	agent := core.NewReActAgent(cfg)
func (m *SubAgentMiddleware) Init(ctx context.Context, config *core.ReActConfig[*schema.Message]) {
	m.parentCfg = config
	// ensureBuilt is deferred to ContributeTools (called during agent build).
}

// BindToConfig adds sub-agent tools to the parent agent config.
//
// Deprecated: Prefer using Init(ctx, config) or relying on the ToolContributor
// interface (which is automatically collected during agent build). BindToConfig
// has a timing dependency (must be called before NewReActAgent) and sets
// config.ToolsConfig = nil as a side effect.
//
// For each spec, it:
//  1. Builds the Agent from AgentConfig (if provided) or uses pre-built Agent.
//  2. Applies middleware inheritance if InheritParentMiddlewares is true.
//  3. Creates an AgentTool wrapper with MaxDepth and EmitInternalEvents.
//  4. Appends the tool to config.Tools.
//
// MUST be called before agent.Run().
// The ctx is used for sub-agent construction (AgentFactory calls, AgentTool wrapping).
// Pass the parent agent's build context or context.Background() if none is available.
func (m *SubAgentMiddleware) BindToConfig(ctx context.Context, config *core.ReActConfig[*schema.Message]) {
	m.mu.Lock()
	if m.built {
		m.mu.Unlock()
		return // idempotent
	}
	m.built = true
	m.mu.Unlock()

	m.ensureBuilt(ctx, config)
	config.Tools = append(config.Tools, m.tools...)
	config.ToolsConfig = nil
}

func (m *SubAgentMiddleware) ensureBuilt(ctx context.Context, config *core.ReActConfig[*schema.Message]) {
	for _, spec := range m.specs {
		agent := m.resolveAgent(ctx, spec, config)
		if agent == nil {
			continue
		}

		// Track this spec as successfully built
		m.builtInfos = append(m.builtInfos, &schema.ToolInfo{
			Name:        spec.Name,
			Description: spec.Description,
		})

		var toolOpts []core.AgentToolOption
		if m.cfg.EmitInternalEvents {
			toolOpts = append(toolOpts, core.WithEmitInternalEvents())
		}
		if m.cfg.MaxDepth > 0 {
			toolOpts = append(toolOpts, core.WithMaxDepth(m.cfg.MaxDepth))
		}
		tool := core.NewAgentTool(ctx, agent, toolOpts...)
		m.tools = append(m.tools, tool)
	}
}

// resolveAgent returns a built Agent for the spec, applying middleware
// inheritance when requested.
//
// When both AgentConfig and Agent are set, AgentConfig takes precedence.
// When using a pre-built Agent with InheritParentMiddlewares, inheritance
// is NOT applied — middlewares are already fixed at construction time.
// Use AgentConfig instead when inheritance is needed.
func (m *SubAgentMiddleware) resolveAgent(ctx context.Context, spec SubAgentSpec, parentCfg *core.ReActConfig[*schema.Message]) core.Agent {
	// 1. Build from AgentConfig (takes precedence when both Agent and AgentConfig are set).
	if spec.AgentConfig != nil {
		cfg := m.buildConfig(spec, parentCfg)
		return core.NewReActAgent(cfg).
			WithName(spec.Name).
			WithDescription(spec.Description)
	}

	// 2. Use pre-built Agent.
	// Note: InheritParentMiddlewares is silently ignored for pre-built agents.
	// Middlewares are already fixed at Agent construction time.
	if spec.Agent != nil {
		return spec.Agent
	}

	// 3. Lazy factory (legacy path).
	if spec.AgentFactory != nil {
		agent, err := spec.AgentFactory(ctx)
		if err == nil && agent != nil {
			return agent
		}
	}

	return nil
}

// buildConfig creates a ReActConfig from an AgentConfig, applying middleware
// inheritance when InheritParentMiddlewares is true and parentCfg is non-nil.
func (m *SubAgentMiddleware) buildConfig(spec SubAgentSpec, parentCfg *core.ReActConfig[*schema.Message]) *core.ReActConfig[*schema.Message] {
	cfg := spec.AgentConfig
	subCfg := &core.ReActConfig[*schema.Message]{
		Model:         cfg.Model,
		Tools:         cfg.Tools,
		Instruction:   cfg.SystemPrompt,
		MaxIterations: cfg.MaxIterations,
	}

	// Apply middleware inheritance when parent config is available.
	if spec.InheritParentMiddlewares && parentCfg != nil {
		subCfg.Middlewares = m.inheritedMiddlewares(parentCfg, spec.ExcludedParentMiddlewareNames)
	}
	// Append sub-agent's own middlewares.
	subCfg.Middlewares = append(subCfg.Middlewares, cfg.Middlewares...)

	return subCfg
}

// inheritedMiddlewares returns parent middlewares excluding:
//   - The SubAgentMiddleware itself (always excluded, prevents infinite recursion).
//   - Any middleware whose type name matches an entry in excludedNames.
//
// Reference semantics: middleware interface values are copied (pointers to the
// same underlying instances). Shared mutable state in middlewares affects both
// parent and sub-agent.
func (m *SubAgentMiddleware) inheritedMiddlewares(parentCfg *core.ReActConfig[*schema.Message], excludedNames []string) []core.ReActMiddleware {
	excluded := make(map[string]bool, len(excludedNames)+1)
	for _, n := range excludedNames {
		excluded[n] = true
	}

	var inherited []core.ReActMiddleware
	for _, mw := range parentCfg.Middlewares {
		if mw == nil {
			continue
		}
		// Always exclude the SubAgentMiddleware itself.
		if _, ok := mw.(subAgentMarker); ok {
			continue
		}
		// Check additional exclusions by type name.
		typeName := fmt.Sprintf("%T", mw)
		if excluded[typeName] {
			continue
		}
		inherited = append(inherited, mw)
	}
	return inherited
}

// BeforeModelRewrite injects sub-agent ToolInfo entries into state.ToolInfos
// so the LLM sees the sub-agents as available tools. Only tools that were
// successfully built in ensureBuilt are advertised.
func (m *SubAgentMiddleware) BeforeModelRewrite(ctx context.Context, state *core.ReActAgentState, mc *core.ModelContext) (context.Context, *core.ReActAgentState, error) {
	state.ToolInfos = append(state.ToolInfos, m.builtInfos...)
	return ctx, state, nil
}

// isSubAgentMarker implements the subAgentMarker interface for self-identification
// during middleware inheritance filtering.
func (m *SubAgentMiddleware) isSubAgentMiddleware() {}

// ---- Compile-time interface checks ----

var _ core.ReActMiddleware = (*SubAgentMiddleware)(nil)
