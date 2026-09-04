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

// Package component — registry adapter (legacy + new wiring co-exist).
//
// As of plan §4 Phase 0, this file is a THIN ADAPTER over
// runtime.DefaultRegistry. The internal `registry` map has been
// removed — all registrations flow through the runtime registry.
//
// The legacy `Register(name, f)` and `New(name, params)` signatures
// are preserved unchanged so every existing call site in this package
// and its tests keeps working without modification. The adapter
// translates the legacy `Factory = func(params) (Component, error)`
// shape into the runtime's `ComponentFactory = func(name, params)
// (Component, error)` shape at registration time, so the legacy
// signature (which takes no name argument) is honoured by wrapping.
//
// New code that wants Category metadata at registration time should
// call runtime.DefaultRegistry.Register directly with an explicit
// Category (see component/pipeline_chunker.go for the canonical
// example).
package component

import (
	"fmt"

	"ragflow/internal/agent/runtime"
)

// Factory constructs a Component from a params map (loaded from the DSL).
// Returning an error here aborts the run with a clear message.
type Factory func(params map[string]any) (Component, error)

// Register enrolls a component factory under name (case-insensitive).
// Intended to be called from init() in each component's <name>.go file.
//
// Legacy semantics preserved: duplicate registrations PANIC (init-time
// fail-fast). The runtime layer returns an error from Register; the
// adapter panics on that error so existing init() call sites behave
// identically to the pre-Phase-0 implementation.
//
// Per plan §4 Phase 0 task 2, the legacy adapter stamps
// Metadata{Version: "legacy"} so the runtime's empty-metadata
// rejection (plan §4 Phase 0 task 1) lets the catalog serve
// legacy agent components alongside ingestion components that
// supply a real version. The migration rule is: agent/shared
// components must be backfilled with real metadata before they
// are exposed to the component catalog; ingestion components
// must never register empty metadata. Today every call site
// passes Metadata{Version: "legacy"} via this shim; new code
// that wants full metadata should call
// runtime.DefaultRegistry.Register directly with an explicit
// Category (see component/pipeline_chunker.go for the canonical
// example).
func Register(name string, f Factory) {
	if err := runtime.DefaultRegistry.Register(name, runtime.CategoryAgent,
		func(_ string, params map[string]any) (runtime.Component, error) {
			return f(params)
		},
		runtime.Metadata{Version: "legacy"}); err != nil {
		panic(err)
	}
}

// New constructs a Component by name. Returns an error if the name is
// unknown or the factory rejects the params. The empty-string case is
// treated as "not found" so the error message is consistent.
//
// The runtime registry's ComponentFactory returns the minimal
// runtime.Component (Invoke-only). The component package's Component
// interface is richer (Name / Stream / Inputs / Outputs); every
// factory registered through the legacy Register(name, Factory)
// adapter returns a *concrete component that satisfies the richer
// interface, so the type assertion below is guaranteed to succeed at
// runtime. It surfaces as an explicit error rather than a panic so a
// misbehaving factory is reported cleanly.
func New(name string, params map[string]any) (Component, error) {
	factory, _, _, ok := runtime.DefaultRegistry.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("component: unknown component %q (registered: %s)", name, RegisteredNames())
	}
	c, err := factory(name, params)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("component: nil factory result for %q", name)
	}
	rc, ok := c.(Component)
	if !ok {
		return nil, fmt.Errorf("component: factory for %q returned %T, which does not satisfy Component (missing Name/Stream/Inputs/Outputs)", name, c)
	}
	return rc, nil
}

// RegisteredNames returns the sorted list of registered component
// names. Used for diagnostics and the API 500 path "list available
// components". Restricted to CategoryAgent — ingestion and shared
// components live under their own categories.
func RegisteredNames() []string {
	return runtime.DefaultRegistry.NamesByCategory(runtime.CategoryAgent)
}
