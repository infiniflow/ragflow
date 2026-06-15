// Package profile provides a dual-track configuration system for agentcore:
//
//   - ProviderProfile: controls how an LLM model is constructed per provider
//     (api_key, temperature, max_tokens, api_base, use_responses_api, etc.)
//   - HarnessProfile: controls the agent's runtime behaviour per use-case
//     (system prompt, tool descriptions, middleware exclusions, recursion depth)
//
// Usage:
//
//	// Register once at init time.
//	profile.RegisterProvider("anthropic", &profile.ProviderProfile{
//	    InitModel: func(ctx, modelName string, opts map[string]any) (Model, error) {
//	        return anthropic.NewModel(modelName, opts["api_key"].(string)), nil
//	    },
//	    DefaultModel: "claude-sonnet-4-6",
//	})
//	profile.RegisterHarness("coding-agent", &profile.HarnessProfile{
//	    BaseSystemPrompt: strPtr("You are an expert software engineer."),
//	    MaxIterations:    20,
//	    RecursionDepth:   5,
//	})
//
//	// Create an agent in one call.
//	agent, err := profile.NewAgent(ctx, &profile.AgentConfig{
//	    ModelSpec:          "anthropic:claude-sonnet-4-6",
//	    HarnessProfileName: "coding-agent",
//	    Tools:              []core.Tool{myTool},
//	})
//
// Config precedence (highest wins): user > HarnessProfile > ProviderProfile > defaults.
package profile

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/internal"
	"ragflow/internal/harness/core/middlewares/subagent"
	"ragflow/internal/harness/core/schema"
)

// ========================================================================
// Phase 1: Core types + global registry
// ========================================================================

// ProviderProfile controls how a model is constructed for a given provider.
// Each provider (Anthropic, OpenAI, Google, …) registers one ProviderProfile
// that knows how to build a concrete Model from a model name + options.
type ProviderProfile struct {
	// Name identifies the provider, e.g. "anthropic", "openai".
	Name string

	// InitModel creates a Model instance for the given model name and options.
	// opts typically carries "api_key", "temperature", "max_tokens", "api_base", etc.
	InitModel func(ctx context.Context, modelName string, opts map[string]any) (core.Model[*schema.Message], error)

	// DefaultModel is returned when no model name is specified.
	DefaultModel string

	// DefaultOpts are the default options passed to InitModel.
	// Can be overridden by HarnessProfile or user config.
	DefaultOpts map[string]any
}

// HarnessProfile controls the agent's runtime behaviour for a specific use-case.
// The same model can be used with different harness profiles (coding, chat, research, …).
type HarnessProfile struct {
	// Name identifies the profile, e.g. "coding-agent", "research-agent".
	Name string

	// BaseSystemPrompt replaces the default system prompt entirely.
	// When nil, the system default is used.
	BaseSystemPrompt *string

	// SystemPromptSuffix is appended to the system prompt after BaseSystemPrompt.
	SystemPromptSuffix string

	// ToolDescriptionOverrides replaces the Description() of matching tools.
	// Key = tool name, value = new description.
	ToolDescriptionOverrides map[string]string

	// ExcludedToolNames removes matching tools from the agent's tool list.
	ExcludedToolNames []string

	// ExcludedMiddlewareNames removes matching middlewares from the agent's
	// middleware chain. Matching uses fmt.Sprintf("%T", mw) — include the
	// fully qualified type name, e.g. "*subagent.SubAgentMiddleware".
	ExcludedMiddlewareNames []string

	// ExtraMiddlewares are appended to the agent's middleware chain at the end.
	ExtraMiddlewares []core.ReActMiddleware

	// MaxIterations overrides the ReAct loop iteration limit. 0 = use default.
	MaxIterations int

	// RecursionDepth sets the sub-agent recursion depth limit.
	// 0 = unlimited (system default).
	RecursionDepth int
}

// Global registries.
var (
	providers sync.Map // map[string]*ProviderProfile
	harnesses sync.Map // map[string]*HarnessProfile
)

// RegisterProvider registers a provider profile. Panics on duplicate name.
func RegisterProvider(p *ProviderProfile) {
	if p == nil {
		panic("profile: RegisterProvider called with nil")
	}
	if p.Name == "" {
		panic("profile: ProviderProfile.Name is required")
	}
	if _, loaded := providers.LoadOrStore(p.Name, p); loaded {
		panic(fmt.Sprintf("profile: provider %q already registered", p.Name))
	}
}

// RegisterHarness registers a harness profile. Panics on duplicate name.
func RegisterHarness(h *HarnessProfile) {
	if h == nil {
		panic("profile: RegisterHarness called with nil")
	}
	if h.Name == "" {
		panic("profile: HarnessProfile.Name is required")
	}
	if _, loaded := harnesses.LoadOrStore(h.Name, h); loaded {
		panic(fmt.Sprintf("profile: harness profile %q already registered", h.Name))
	}
}

// LookupProvider returns the registered provider, or nil if not found.
func LookupProvider(name string) *ProviderProfile {
	if v, ok := providers.Load(name); ok {
		return v.(*ProviderProfile)
	}
	return nil
}

// LookupHarness returns the registered harness profile, or nil if not found.
func LookupHarness(name string) *HarnessProfile {
	if v, ok := harnesses.Load(name); ok {
		return v.(*HarnessProfile)
	}
	return nil
}

// ParseModelSpec parses "provider:model" strings.
// Returns ("anthropic", "claude-sonnet-4-6") from "anthropic:claude-sonnet-4-6".
// If no colon, returns ("", raw, error).
func ParseModelSpec(spec string) (provider, model string, err error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("profile: invalid model spec %q (expected provider:model)", spec)
	}
	return parts[0], parts[1], nil
}

// ========================================================================
// AgentConfig combines model spec + harness profile + user overrides.
// ========================================================================

// AgentConfig is a high-level declarative config for creating a ReActAgent.
// It combines model selection (via ModelSpec), runtime behaviour (via
// HarnessProfileName), and direct user overrides.
type AgentConfig struct {
	// ModelSpec in "provider:model" format, e.g. "anthropic:claude-sonnet-4-6".
	ModelSpec string

	// HarnessProfileName selects a registered HarnessProfile.
	// Empty string means no harness profile.
	HarnessProfileName string

	// ProviderOpts overrides or augments the ProviderProfile's DefaultOpts.
	ProviderOpts map[string]any

	// Instruction overrides the system prompt. When non-nil, it takes
	// precedence over both HarnessProfile.BaseSystemPrompt and the system default.
	Instruction *string

	// Tools available to the agent. When non-nil, replaces all previous tool lists.
	Tools []core.Tool

	// Middlewares to apply. Appended after HarnessProfile.ExtraMiddlewares.
	Middlewares []core.ReActMiddleware

	// MaxIterations overrides both HarnessProfile and system default.
	MaxIterations int

	// SubAgentSpecs declares sub-agents. The SubAgentMiddleware is automatically
	// created with recursion depth from the active HarnessProfile.
	SubAgentSpecs []subagent.SubAgentSpec
}

// ========================================================================
// Phase 3: Override chain with precedence rules
// ========================================================================

// NewAgent creates a ReActAgent from a declarative AgentConfig.
//
// Precedence (highest wins): user explicit > HarnessProfile > ProviderProfile > defaults.
func NewAgent(ctx context.Context, cfg *AgentConfig) (core.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("profile: AgentConfig is nil")
	}

	// 1. Resolve model from ModelSpec.
	model, err := buildModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// 2. Build ReActConfig via override chain.
	reactCfg := buildReactConfig(ctx, cfg)

	// 3. Set the resolved model.
	reactCfg.Model = model

	// 4. Handle HarnessProfile's ExcludedToolNames.
	if harness := lookupHarness(cfg.HarnessProfileName); harness != nil && len(harness.ExcludedToolNames) > 0 {
		excluded := makeMap(harness.ExcludedToolNames)
		filtered := make([]core.Tool, 0, len(reactCfg.Tools))
		for _, t := range reactCfg.Tools {
			if excluded[t.Name()] {
				continue
			}
			filtered = append(filtered, t)
		}
		reactCfg.Tools = filtered
	}

	// 5. Apply tool description overrides (wrap matching tools).
	if harness := lookupHarness(cfg.HarnessProfileName); harness != nil && len(harness.ToolDescriptionOverrides) > 0 {
		for i, t := range reactCfg.Tools {
			if newDesc, ok := harness.ToolDescriptionOverrides[t.Name()]; ok && newDesc != "" {
				reactCfg.Tools[i] = &descriptionOverrideTool{Tool: t, newDesc: newDesc}
			}
		}
	}

	// 6. Handle SubAgentSpecs — create SubAgentMiddleware and bind.
	if len(cfg.SubAgentSpecs) > 0 {
		subCfg := &subagent.Config{}
		if harness := lookupHarness(cfg.HarnessProfileName); harness != nil && harness.RecursionDepth > 0 {
			subCfg.MaxDepth = harness.RecursionDepth
		}
		saMW := subagent.New(cfg.SubAgentSpecs, subCfg)
		reactCfg.Middlewares = append(reactCfg.Middlewares, saMW)
		saMW.BindToConfig(ctx, reactCfg)
	}

	return core.NewReActAgent(reactCfg), nil
}

// buildModel resolves the model from ModelSpec + ProviderProfile.
func buildModel(ctx context.Context, cfg *AgentConfig) (core.Model[*schema.Message], error) {
	providerName, modelName, err := ParseModelSpec(cfg.ModelSpec)
	if err != nil {
		return nil, err
	}

	provider := LookupProvider(providerName)
	if provider == nil {
		return nil, fmt.Errorf("profile: unknown provider %q (registered: %s)", providerName, listProviders())
	}

	// Merge options: DefaultOpts ← ProviderOpts.
	opts := copyMap(provider.DefaultOpts)
	for k, v := range cfg.ProviderOpts {
		opts[k] = v
	}

	m, err := provider.InitModel(ctx, modelName, opts)
	if err != nil {
		return nil, fmt.Errorf("profile: InitModel(%s, %s): %w", providerName, modelName, err)
	}
	return m, nil
}

// buildReactConfig applies the override chain for non-model config fields.
func buildReactConfig(ctx context.Context, cfg *AgentConfig) *core.ReActConfig[*schema.Message] {
	// Start with system defaults.
	result := &core.ReActConfig[*schema.Message]{
		MaxIterations: 10,
		Instruction:   internal.DefaultSystemPrompt,
	}

	harness := lookupHarness(cfg.HarnessProfileName)

	// Layer 1: HarnessProfile.
	if harness != nil {
		if harness.MaxIterations > 0 {
			result.MaxIterations = harness.MaxIterations
		}
		if harness.BaseSystemPrompt != nil {
			result.Instruction = *harness.BaseSystemPrompt
		}
		result.Instruction += harness.SystemPromptSuffix

		// Extra middlewares (appended; ExcludedMiddlewareNames applied later).
		result.Middlewares = append(result.Middlewares, harness.ExtraMiddlewares...)
	}

	// Layer 2: User explicit config (highest priority).
	if cfg.Instruction != nil {
		result.Instruction = *cfg.Instruction
	}
	if cfg.Tools != nil {
		result.Tools = cfg.Tools
	}
	if cfg.MaxIterations > 0 {
		result.MaxIterations = cfg.MaxIterations
	}
	if cfg.Middlewares != nil {
		result.Middlewares = append(result.Middlewares, cfg.Middlewares...)
	}

	// Apply ExcludedMiddlewareNames from HarnessProfile.
	if harness != nil && len(harness.ExcludedMiddlewareNames) > 0 {
		result.Middlewares = filterMiddlewareByTypeName(result.Middlewares, harness.ExcludedMiddlewareNames)
	}

	return result
}

// ========================================================================
// Helpers
// ========================================================================

func lookupHarness(name string) *HarnessProfile {
	if name == "" {
		return nil
	}
	return LookupHarness(name)
}

func listProviders() string {
	var names []string
	providers.Range(func(key, _ any) bool {
		names = append(names, key.(string))
		return true
	})
	return strings.Join(names, ", ")
}

func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return make(map[string]any)
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func makeMap(keys []string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// filterMiddlewareByTypeName removes middlewares whose fmt.Sprintf("%T") matches
// any name in the exclusion list.
func filterMiddlewareByTypeName(mws []core.ReActMiddleware, exclude []string) []core.ReActMiddleware {
	if len(exclude) == 0 {
		return mws
	}
	excluded := makeMap(exclude)
	filtered := make([]core.ReActMiddleware, 0, len(mws))
	for _, mw := range mws {
		if mw == nil {
			continue
		}
		typeName := fmt.Sprintf("%T", mw)
		if excluded[typeName] {
			continue
		}
		filtered = append(filtered, mw)
	}
	return filtered
}

// descriptionOverrideTool wraps a Tool to override its Description().
type descriptionOverrideTool struct {
	core.Tool
	newDesc string
}

func (t *descriptionOverrideTool) Description() string { return t.newDesc }

// StrPtr is a helper for creating *string literals.
func StrPtr(s string) *string { return &s }

// Validate checks the AgentConfig for common errors and returns them all at once.
func Validate(cfg *AgentConfig) []error {
	var errs []error
	if cfg == nil {
		return []error{fmt.Errorf("profile: AgentConfig is nil")}
	}
	if cfg.ModelSpec == "" {
		errs = append(errs, fmt.Errorf("profile: ModelSpec is required"))
	} else if _, _, err := ParseModelSpec(cfg.ModelSpec); err != nil {
		errs = append(errs, err)
	}
	if cfg.HarnessProfileName != "" && LookupHarness(cfg.HarnessProfileName) == nil {
		errs = append(errs, fmt.Errorf("profile: harness profile %q not found", cfg.HarnessProfileName))
	}
	return errs
}

// ClearProviders removes all registered providers. Used in tests for isolation.
func ClearProviders() {
	providers = sync.Map{}
}

// ClearHarnesses removes all registered harness profiles. Used in tests.
func ClearHarnesses() {
	harnesses = sync.Map{}
}

// RegisterProviderModel is a convenience wrapper that registers both a provider
// profile (using ProviderProfile.Name) AND a harness profile for each supported
// model. This matches deepagents' double-registration pattern.
//
// Deprecated: Use separate RegisterProvider and RegisterHarness calls instead.
func RegisterProviderModel(provider *ProviderProfile, harnessProfiles ...*HarnessProfile) {
	RegisterProvider(provider)
	for _, h := range harnessProfiles {
		RegisterHarness(h)
	}
}
