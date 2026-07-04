// Package component implements the RAGFlow agent canvas components
// in Go following the 5-tier porting strategy (T1–T5; see
// docs/develop/agent-go-port-design.md §4.1).
//
// Component is the runtime contract every RAGFlow component
// implements; it is a richer interface than
// internal/agent/runtime.Component (which is the minimal Invoke-only
// surface canvas needs at build time). Any concrete *Component here
// satisfies runtime.Component structurally, which is how the canvas
// builder consumes a registered component via
// runtime.DefaultFactory().
//
// ParamError and ErrNotImplemented are aliased from runtime so the
// canvas builder and the component implementations share the same
// types without a cycle.
package component

import (
	"context"

	"ragflow/internal/agent/runtime"
)

// Component is the runtime contract every RAGFlow component implements.
// Mirrors the Python ComponentBase.invoke / invoke_async surface
// (agent/component/base.py:365, 408, 422) plus a Stream variant for SSE
// output (the Message component).
//
// Inputs() and Outputs() return parameter metadata for tooling / docs /
// graph introspection — name → human description. Not used at runtime.
//
// Any value implementing this interface also satisfies the smaller
// runtime.Component interface (Invoke only), so the canvas builder
// can consume a *Component via runtime.DefaultFactory() without any
// extra adaptation.
type Component interface {
	// Name returns the registered component name (e.g. "LLM", "Agent",
	// "Switch"). Case-insensitive lookup — the registry normalizes input.
	Name() string

	// Invoke runs the component synchronously. inputs is the resolved
	// parameter map (variable references already substituted by the canvas
	// engine). Returns the output map; components should put their public
	// outputs at top-level keys.
	Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error)

	// Stream is the streaming variant. The default implementation may
	// return a buffered channel that emits the same payload as Invoke, then
	// closes — components that natively stream (LLM, Message) override.
	// May return (nil, nil) for non-streaming components.
	Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error)

	// Inputs returns parameter metadata: param_name → description.
	Inputs() map[string]string
	// Outputs returns output metadata: param_name → description.
	Outputs() map[string]string
}

// ParamBase is the optional parameter validation/serialization surface.
// Components that need validation can embed *BaseParam (below) or implement
// this directly. Components that don't need it (e.g. ExitLoop) can omit.
//
// Mirrors agent/component/param_base.py:ComponentParamBase (Python).
type ParamBase interface {
	// Update copies conf into the receiver, validating types. Used by
	// editors / APIs that hand-craft a params map.
	Update(conf map[string]any) error
	// Check performs deep validation (required fields, value ranges).
	// Called once before Invoke — returning an error aborts the run.
	Check() error
	// AsDict returns the params as a plain map for serialization / debug.
	AsDict() map[string]any
}

// ErrNotImplemented aliases runtime.ErrNotImplemented so component-side
// code (and the canvas builder it interoperates with) share a single
// sentinel value.
var ErrNotImplemented = runtime.ErrNotImplemented

// ParamError aliases runtime.ParamError. Existing code that constructs
// &ParamError{Field: ..., Reason: ...} continues to work; the value
// it produces is the same type runtime.SetDefaultFactory consumers see.
type ParamError = runtime.ParamError

// BeOutput wraps a single output value into the canonical
// {"content": v} frame downstream components (Message, VariableAggregator)
// consume. Mirrors agent/component/base.py:ComponentBase.be_output
// (restored by PR #16363 after the agent refactor dropped it). Most
// components can just return `map[string]any{"content": v}` inline,
// but this helper keeps the wrapper construction in one place so
// error/empty paths can produce a uniform output shape without
// duplicating the literal everywhere.
func BeOutput(v any) map[string]any {
	return map[string]any{"content": v}
}
