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

// runtime — shared Component contract + factory injection point.
//
// The Component interface here is the minimal surface the canvas
// builder needs to invoke a component body. The full Component
// interface (with Name / Stream / Inputs / Outputs) lives in the
// component package; component.Component satisfies the smaller
// runtime.Component by Go's structural typing.
//
// Production wiring: the component package calls
// SetDefaultFactory(component.New) from its init() so the canvas
// builder can resolve real components via DefaultFactory() at
// BuildWorkflow time. No `canvas -> component` import edge is
// required.
package runtime

import (
	"context"
	"fmt"
	"sync"
)

// Component is the minimal interface the canvas builder needs at
// sub-graph build time and at iteration time. The component package's
// Component type has more methods (Name / Stream / Inputs / Outputs);
// it satisfies this smaller interface by Go's structural typing.
type Component interface {
	Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error)
}

// ComponentFactory builds a Component from a DSL name + params map.
// The canvas builder calls this per cpn at BuildWorkflow time and
// stores the resulting Component in the per-node lambda closure.
type ComponentFactory func(name string, params map[string]any) (Component, error)

// ErrNotImplemented is the sentinel returned by components that have
// not been fully ported to Go yet. The canvas builder does NOT
// intercept this error: it propagates through the workflowx layer and
// fails the run, mirroring Go's standard "error from a dependency
// fails the call" semantics. Callers that want to treat a not-yet-
// implemented component as a soft-fail should wrap Invoke themselves
// (e.g. with a placeholder lambda) or check errors.Is against this
// sentinel at the test layer.
//
// Test- or log-grep code that pattern-matches this string should use
// errors.Is(err, ErrNotImplemented) instead of substring matching —
// the message is for humans, the sentinel is for code.
var ErrNotImplemented = fmt.Errorf("component: not yet implemented (placeholder)")

// ParamError wraps a parameter validation failure with the field name
// for clearer error messages to the user.
type ParamError struct {
	Field  string
	Reason string
}

func (e *ParamError) Error() string {
	return fmt.Sprintf("component: invalid param %q: %s", e.Field, e.Reason)
}

var (
	factoryMu      sync.RWMutex
	defaultFactory ComponentFactory
)

// SetDefaultFactory installs the production ComponentFactory. The
// component package calls this in its init() with `component.New`.
// Calling SetDefaultFactory more than once with a non-nil factory is
// a no-op after the first call (the first wins) so concurrent
// registration is safe. Passing nil clears the factory — tests use
// this to assert "no factory registered" error paths.
func SetDefaultFactory(f ComponentFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if f == nil {
		defaultFactory = nil
		return
	}
	if defaultFactory == nil {
		defaultFactory = f
	}
}

// DefaultFactory returns the registered ComponentFactory, or nil if
// none has been registered. The canvas builder calls this once per
// BuildWorkflow and errors with a clear "no component factory
// registered" message if the result is nil.
func DefaultFactory() ComponentFactory {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return defaultFactory
}

// ResetDefaultFactoryForTesting clears the registered factory.
// Test-only helper for code paths that want to assert behaviour
// when no factory is installed. Not safe under concurrent use with
// production code paths.
func ResetDefaultFactoryForTesting() {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	defaultFactory = nil
}
