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
// component package calls this in its init() via
// installDefaultRegistryFactory (see below). After Phase 0, the
// "first writer wins" guard is REMOVED: SetDefaultFactory now ALWAYS
// replaces the active default, regardless of whether one is already
// installed. This preserves the existing test-override pattern where
// tests save the previous factory, install a stub, and restore on
// t.Cleanup. Passing nil clears the factory — tests use this to
// assert "no factory registered" error paths.
//
// Two-layer model:
//
//   - Production: installDefaultRegistryFactory installs a closure
//     that calls runtime.DefaultRegistry.Lookup on every invocation.
//     It captures DefaultRegistry by reference, so even if
//     installDefaultRegistryFactory runs before all init()
//     registrations complete, the factory is correct at every
//     subsequent lookup (the registry is read lazily, not captured
//     at install time).
//   - Override: tests call SetDefaultFactory(stub) directly to stub
//     the default factory. t.Cleanup restores the production factory
//     by calling installDefaultRegistryFactory again (or by saving
//     the previous value and re-injecting it).
func SetDefaultFactory(f ComponentFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	defaultFactory = f
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

// InstallDefaultRegistryFactory installs the production
// ComponentFactory: a closure that resolves component names via
// runtime.DefaultRegistry.Lookup at every invocation. The closure
// captures DefaultRegistry by reference (the variable, not the
// concrete registry), so lookup always reads the current state of
// the singleton even if a test later swaps it out via SetDefaultFactory.
//
// This is the helper the component package's init() calls (from
// internal/agent/component/runtime_wire.go). Production callers
// should prefer InstallDefaultRegistryFactory over SetDefaultFactory
// directly so the wiring stays in one place — if the resolution
// strategy changes (e.g. switch to a per-call registry handle),
// only this function changes.
//
// Note: this is EXPORTED (unlike the helper sketched in plan §4
// Phase 0 task 3) because the call site lives in a different package
// (internal/agent/component) and Go's visibility rules don't allow
// access to unexported names across package boundaries. The "test
// override layer" still owns SetDefaultFactory — tests that want to
// stub the factory call SetDefaultFactory directly, not this helper.
func InstallDefaultRegistryFactory() {
	SetDefaultFactory(func(name string, params map[string]any) (Component, error) {
		f, _, _, ok := DefaultRegistry.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("runtime: unknown component %q", name)
		}
		return f(name, params)
	})
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
