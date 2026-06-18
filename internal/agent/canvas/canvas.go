// Package canvas implements the RAGFlow agent canvas Go port.
//
// Shared runtime contracts (CanvasState, Component, ComponentFactory,
// state context plumbing, template helpers) live in
// internal/agent/runtime. Canvas re-exports them through thin aliases
// so existing call sites keep working while breaking the historic
// canvas <-> component import cycle.
package canvas

import (
	"ragflow/internal/agent/runtime"
)

// legacyNoOpNames is the set of component names that the Go port
// recognises for DSL v1 compatibility but does not ship a real
// implementation for. Encountering one of these in a DSL is mapped to
// the same no-op echo lambda used for placeholder bodies by the
// BuildWorkflow in scheduler.go. New DSLs should not use these names —
// they exist only so v1 DSLs that reference Python-era sentinel
// components ("ExitLoop") still compile and run in the Go port.
//
// Membership semantics inside a Loop's sub-graph: legacy names that
// appear as descendants of a Loop are absorbed as no-op members of the
// sub-graph; they do not contribute to loop control. Termination is
// driven by the Loop's loop_termination_condition predicate, not by
// reaching an ExitLoop node.
var legacyNoOpNames = map[string]bool{
	"exitloop": true,
}

// CanvasState aliases runtime.CanvasState so existing canvas callers
// (and component tests that still import the canvas package) keep
// compiling without changes. The canonical definition lives in
// internal/agent/runtime/state.go.
type CanvasState = runtime.CanvasState

// NewCanvasState re-exports runtime.NewCanvasState.
func NewCanvasState(runID, taskID string) *CanvasState {
	return runtime.NewCanvasState(runID, taskID)
}

// Canvas is the in-memory DSL representation loaded from a user_canvas row.
// It is the input to compile.go which builds the eino Workflow.
type Canvas struct {
	Components map[string]CanvasComponent `json:"components"`
	Path       []string                   `json:"path"`
	History    []map[string]any           `json:"history,omitempty"`
	Retrieval  map[string]any             `json:"retrieval,omitempty"`
	Globals    map[string]any             `json:"globals,omitempty"`
}

// CanvasComponent is the in-memory DSL node. The Obj.ComponentName
// matches agent/component/<name>.py's class name (case-insensitive,
// per Python v1 DSL semantics).
type CanvasComponent struct {
	Obj        CanvasComponentObj `json:"obj"`
	Downstream []string           `json:"downstream"`
	Upstream   []string           `json:"upstream"`
}

type CanvasComponentObj struct {
	ComponentName string         `json:"component_name"`
	Params        map[string]any `json:"params"`
}

// Component is an alias for runtime.Component — the minimal runtime
// surface BuildWorkflow needs at sub-graph build time. The canonical
// definition (and the SetDefaultFactory / DefaultFactory plumbing)
// lives in internal/agent/runtime/component.go.
type Component = runtime.Component

// ComponentFactory aliases runtime.ComponentFactory.
type ComponentFactory = runtime.ComponentFactory

// SetDefaultFactory re-exports runtime.SetDefaultFactory. The
// orchestrator's main.go can call either entry point; new code
// should prefer the runtime package directly.
func SetDefaultFactory(f ComponentFactory) {
	runtime.SetDefaultFactory(f)
}
