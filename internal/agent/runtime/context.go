//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// runtime — context-attached canvas state for cross-package access.
//
// The compile entry (canvas/compile.go) attaches *CanvasState to ctx
// once per run via WithState. Component bodies retrieve it via
// GetStateFromContext. eino's internal state plumbing also threads
// the state via WithGenLocalState; this context-key path is the
// fallback used by code that does not run as an eino handler (e.g.
// loop condition closures, ad-hoc tests, and the placeholder
// component bodies that BuildWorkflow installs).
package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// stateCtxKey is the unexported context key used by WithState /
// GetStateFromContext. Defined at package scope so its identity is
// stable across calls (a fresh struct{}{} per call would key
// distinctly and break ctx.Value lookups).
type stateCtxKey struct{}
type agentMessageEmitterCtxKey struct{}
type canvasMessageEmitterCtxKey struct{}

// AgentMessageEmitter emits visible assistant deltas for an Agent component.
// The service layer owns the actual SSE envelope; runtime keeps the callback
// shape free of canvas/service imports.
type AgentMessageEmitter func(contentDelta, thinkingDelta string)
type CanvasMessageEmitter func(content string)
type CanvasMessageEventEmitter func(content string, startToThink, endToThink bool)
type AgentDeltaSink func(contentDelta, thinkingDelta string)

// DeferredStream is a lazy component value. The producer is deliberately
// opened by the consuming Message node.
type DeferredStream struct {
	Open func(context.Context, AgentDeltaSink) (map[string]any, error)
}

func IsDeferredStream(v any) bool {
	deferred, ok := v.(*DeferredStream)
	return ok && deferred != nil && deferred.Open != nil
}

type agentMessageEmitterState struct {
	mu           sync.Mutex
	emit         AgentMessageEmitter
	finalize     func() bool
	reset        func()
	emitted      bool
	suppressed   bool
	agentContent strings.Builder
}

type agentDeltaSinkCtxKey struct{}
type canvasMessageEventEmitterCtxKey struct{}

type componentExecutionOptionsCtxKey struct{}
type deferredNodeRegistryCtxKey struct{}

type deferredNodeRegistry struct {
	mu          sync.Mutex
	completions map[string]func()
}

// ComponentExecutionOptions carries compile-time graph decisions into a
// component invocation without leaking internal flags into DSL parameters.
type ComponentExecutionOptions struct {
	DeferAgentToMessage        bool
	SuppressAgentMessageEvents bool
}

func WithComponentExecutionOptions(ctx context.Context, opts ComponentExecutionOptions) context.Context {
	return context.WithValue(ctx, componentExecutionOptionsCtxKey{}, opts)
}

func ComponentExecutionOptionsFromContext(ctx context.Context) ComponentExecutionOptions {
	opts, _ := ctx.Value(componentExecutionOptionsCtxKey{}).(ComponentExecutionOptions)
	return opts
}

// WithDeferredNodeRegistry installs the per-run callbacks used to delay an
// Agent node_finished event until its downstream Message has consumed the
// lazy stream.
func WithDeferredNodeRegistry(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferredNodeRegistryCtxKey{}, &deferredNodeRegistry{
		completions: make(map[string]func()),
	})
}

func RegisterDeferredNode(ctx context.Context, nodeID string, complete func()) {
	registry, _ := ctx.Value(deferredNodeRegistryCtxKey{}).(*deferredNodeRegistry)
	if registry == nil || nodeID == "" || complete == nil {
		return
	}
	registry.mu.Lock()
	registry.completions[nodeID] = complete
	registry.mu.Unlock()
}

func CompleteDeferredNode(ctx context.Context, nodeID string) {
	registry, _ := ctx.Value(deferredNodeRegistryCtxKey{}).(*deferredNodeRegistry)
	if registry == nil {
		return
	}
	registry.mu.Lock()
	complete := registry.completions[nodeID]
	delete(registry.completions, nodeID)
	registry.mu.Unlock()
	if complete != nil {
		complete()
	}
}

func CompleteAllDeferredNodes(ctx context.Context) {
	registry, _ := ctx.Value(deferredNodeRegistryCtxKey{}).(*deferredNodeRegistry)
	if registry == nil {
		return
	}
	registry.mu.Lock()
	callbacks := make([]func(), 0, len(registry.completions))
	for nodeID, complete := range registry.completions {
		callbacks = append(callbacks, complete)
		delete(registry.completions, nodeID)
	}
	registry.mu.Unlock()
	for _, complete := range callbacks {
		if complete != nil {
			complete()
		}
	}
}

func WithAgentDeltaSink(ctx context.Context, sink AgentDeltaSink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, agentDeltaSinkCtxKey{}, sink)
}

func agentDeltaSinkFromContext(ctx context.Context) AgentDeltaSink {
	sink, _ := ctx.Value(agentDeltaSinkCtxKey{}).(AgentDeltaSink)
	return sink
}

// WithState attaches *CanvasState to ctx for retrieval by
// GetStateFromContext. Production code (canvas/compile.go) calls this
// once per run; cross-package tests call it directly to set up state
// before invoking a component.
func WithState(ctx context.Context, s *CanvasState) context.Context {
	return context.WithValue(ctx, stateCtxKey{}, s)
}

// WithAgentMessageEmitter attaches the Agent message stream callback used by
// components that can surface thinking before their node_finished event.
func WithAgentMessageEmitter(ctx context.Context, emit AgentMessageEmitter, finalize ...func() bool) context.Context {
	if emit == nil {
		return ctx
	}
	state := &agentMessageEmitterState{emit: emit}
	if len(finalize) > 0 {
		state.finalize = finalize[0]
	}
	return context.WithValue(ctx, agentMessageEmitterCtxKey{}, state)
}

// WithAgentMessageEmitterControl attaches the Agent message stream callback
// with explicit lifecycle hooks for invocation-scoped reset/finalization.
func WithAgentMessageEmitterControl(ctx context.Context, emit AgentMessageEmitter, finalize func() bool, reset func()) context.Context {
	if emit == nil {
		return ctx
	}
	return context.WithValue(ctx, agentMessageEmitterCtxKey{}, &agentMessageEmitterState{
		emit:     emit,
		finalize: finalize,
		reset:    reset,
	})
}

// WithCanvasMessageEmitter attaches the direct Message-component output
// callback. Unlike AgentMessageEmitter, this path does not parse <think> tags
// or buffer chunks; Message nodes are already resolved visible content.
func WithCanvasMessageEmitter(ctx context.Context, emit CanvasMessageEmitter) context.Context {
	if emit == nil {
		return ctx
	}
	return context.WithValue(ctx, canvasMessageEmitterCtxKey{}, emit)
}

// WithCanvasMessageEventEmitter installs the Message presentation callback.
// It is separate from CanvasMessageEmitter so existing plain-content callers
// remain source-compatible while Message can surface thinking boundaries.
func WithCanvasMessageEventEmitter(ctx context.Context, emit CanvasMessageEventEmitter) context.Context {
	if emit == nil {
		return ctx
	}
	return context.WithValue(ctx, canvasMessageEventEmitterCtxKey{}, emit)
}

// HasAgentMessageEmitter reports whether the service layer installed an
// Agent message stream callback on ctx.
func HasAgentMessageEmitter(ctx context.Context) bool {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	return ok && state != nil && state.emit != nil
}

// EmitAgentMessage emits Agent answer/thinking deltas when the service layer
// installed a callback. It returns true when a callback was present.
func EmitAgentMessage(ctx context.Context, contentDelta, thinkingDelta string) bool {
	if sink := agentDeltaSinkFromContext(ctx); sink != nil {
		// A deferred Agent is being consumed by Message. Do not call the
		// service SSE emitter here; Message owns the visible event stream.
		sink(contentDelta, thinkingDelta)
		return true
	}
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil || state.emit == nil {
		return false
	}
	if ComponentExecutionOptionsFromContext(ctx).SuppressAgentMessageEvents {
		state.mu.Lock()
		state.suppressed = true
		state.mu.Unlock()
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.emit(contentDelta, thinkingDelta)
	if contentDelta != "" || thinkingDelta != "" {
		state.emitted = true
	}
	if contentDelta != "" {
		state.agentContent.WriteString(contentDelta)
	}
	return true
}

func AgentMessageEventsSuppressed(ctx context.Context) bool {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.suppressed
}

// EmitCanvasMessageEvent emits one already-presented Message event. When no
// event callback is installed it falls back to the historical plain-content
// callback, preserving existing component tests and non-streaming callers.
func EmitCanvasMessageEvent(ctx context.Context, content string, startToThink, endToThink bool) bool {
	if emit, ok := ctx.Value(canvasMessageEventEmitterCtxKey{}).(CanvasMessageEventEmitter); ok && emit != nil {
		emit(content, startToThink, endToThink)
		if state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState); ok && state != nil {
			state.mu.Lock()
			state.emitted = true
			state.mu.Unlock()
		}
		return true
	}
	if startToThink || endToThink {
		return false
	}
	return EmitCanvasMessage(ctx, content)
}

// EmitCanvasMessage emits already-rendered Message-component content through
// the direct canvas emitter, falling back to the Agent emitter when necessary.
// When the content exactly matches the answer already streamed by an upstream
// Agent, it is not emitted again. Distinct Message output is preserved.
func EmitCanvasMessage(ctx context.Context, content string) bool {
	emit, ok := ctx.Value(canvasMessageEmitterCtxKey{}).(CanvasMessageEmitter)
	state, hasAgentEmitter := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if hasAgentEmitter && state != nil {
		state.mu.Lock()
		defer state.mu.Unlock()
		if content != "" && state.agentContent.Len() > 0 && state.agentContent.String() == content {
			state.emitted = true
			return true
		}
		if ok && emit != nil {
			emit(content)
			if content != "" {
				state.emitted = true
			}
			return true
		}
		if state.emit != nil {
			state.emit(content, "")
			if content != "" {
				state.emitted = true
			}
			return true
		}
		return false
	}
	if !ok || emit == nil {
		return false
	}
	emit(content)
	return true
}

// AgentMessageEventsEmitted reports whether the invocation-scoped Agent
// message emitter has emitted any deltas during the current run.
func AgentMessageEventsEmitted(ctx context.Context) bool {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.emitted
}

// FinalizeAgentMessage flushes the invocation-scoped Agent message emitter.
func FinalizeAgentMessage(ctx context.Context) {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil || state.finalize == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finalize() {
		state.emitted = true
	}
}

// ResetAgentMessageEmission starts a fresh Agent message emission scope.
func ResetAgentMessageEmission(ctx context.Context) {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.reset != nil {
		state.reset()
	}
	state.emitted = false
	state.suppressed = false
	state.agentContent.Reset()
}

// GetStateFromContext extracts a typed state attached via WithState.
// Returns the state and a nil *sync.Mutex for *CanvasState (the
// embedded RWMutex is what callers actually contend on through
// helper methods); the *sync.Mutex return value mirrors eino's
// getState shape for API parity.
//
// The generic type parameter is needed for compatibility with eino's
// compose.getState[S] signature so callers can write the same shape
// whether they're reading our state or eino's.
func GetStateFromContext[S any](ctx context.Context) (S, *sync.Mutex, error) {
	var zero S
	v := ctx.Value(stateCtxKey{})
	if v == nil {
		return zero, nil, fmt.Errorf("canvas: no state in context")
	}
	s, ok := v.(S)
	if !ok {
		return zero, nil, fmt.Errorf("canvas: state type mismatch: have %T, want %T", v, zero)
	}
	// For *CanvasState the returned *sync.Mutex is nil on purpose:
	// CanvasState exposes its own sync.RWMutex via the exported
	// methods (GetVar / SetVar / ReadVars), all of which lock
	// internally. Callers reading *CanvasState should prefer the
	// self-locking methods over holding the mutex themselves.
	return s, nil, nil
}
