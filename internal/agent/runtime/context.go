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
	"sync"
)

// stateCtxKey is the unexported context key used by WithState /
// GetStateFromContext. Defined at package scope so its identity is
// stable across calls (a fresh struct{}{} per call would key
// distinctly and break ctx.Value lookups).
type stateCtxKey struct{}
type agentMessageEmitterCtxKey struct{}

// AgentMessageEmitter emits visible assistant deltas for an Agent component.
// The service layer owns the actual SSE envelope; runtime keeps the callback
// shape free of canvas/service imports.
type AgentMessageEmitter func(contentDelta, thinkingDelta string)

type agentMessageEmitterState struct {
	emit     AgentMessageEmitter
	finalize func() bool
	reset    func()
	emitted  bool
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

// HasAgentMessageEmitter reports whether the service layer installed an
// Agent message stream callback on ctx.
func HasAgentMessageEmitter(ctx context.Context) bool {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	return ok && state != nil && state.emit != nil
}

// EmitAgentMessage emits Agent answer/thinking deltas when the service layer
// installed a callback. It returns true when a callback was present.
func EmitAgentMessage(ctx context.Context, contentDelta, thinkingDelta string) bool {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil || state.emit == nil {
		return false
	}
	state.emit(contentDelta, thinkingDelta)
	if contentDelta != "" || thinkingDelta != "" {
		state.emitted = true
	}
	return true
}

// AgentMessageEventsEmitted reports whether the invocation-scoped Agent
// message emitter has emitted any deltas during the current run.
func AgentMessageEventsEmitted(ctx context.Context) bool {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	return ok && state != nil && state.emitted
}

// FinalizeAgentMessage flushes the invocation-scoped Agent message emitter.
func FinalizeAgentMessage(ctx context.Context) {
	state, ok := ctx.Value(agentMessageEmitterCtxKey{}).(*agentMessageEmitterState)
	if !ok || state == nil || state.finalize == nil {
		return
	}
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
	if state.reset != nil {
		state.reset()
	}
	state.emitted = false
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
