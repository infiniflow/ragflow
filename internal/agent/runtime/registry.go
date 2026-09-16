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

// runtime — category-aware component registry.
//
// Phase 0 of plan port-rag-flow-pipeline-to-go.md lifts the component
// registry out of internal/agent/component into the runtime package
// so the ingestion pipeline (Phase 2) can register under
// CategoryIngestion without depending on the agent canvas. The legacy
// component.Register / component.New / component.RegisteredNames become
// thin adapters that delegate here.
//
// The single source of truth is DefaultRegistry. The component package's
// internal `registry` map has been removed — keeping two maps in sync
// during the transition would cause RegisteredNames() to return a
// partial set (the internal map only sees legacy Register calls; the
// new map only sees RegisterWithMeta calls). See plan §5.
package runtime

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Category tags each registered component with its domain so the UI can
// filter and the runtime can audit cross-domain wiring.
type Category string

const (
	CategoryAgent     Category = "agent"
	CategoryIngestion Category = "ingestion"
	CategoryShared    Category = "shared"
)

// Metadata is the static component descriptor exposed to the API.
// Components MUST provide this at registration time so the API can serve
// a complete component catalog (name, category, inputs, outputs) without
// having to instantiate the component.
//
// The plan §4 Phase 0 task 1 contract says Register REJECTS empty
// metadata. The "is empty" check is implemented in Register as
// "Version == \"\" AND Inputs == nil AND Outputs == nil"; this
// three-way check lets a caller fill in any single field as
// evidence of intent. The Version field is the canonical marker
// (plan §4 Phase 0 task 2 uses `Metadata{Version: "legacy"}` for
// the legacy-adapter shim), but a component that wants to skip
// the version stamp but still record inputs/outputs is also
// allowed.
type Metadata struct {
	Version string            // contract version; required for ingestion, "legacy" for the legacy adapter
	Inputs  map[string]string // input key → human-readable description
	Outputs map[string]string // output key → human-readable description
}

// entry is one slot in the registry.
type entry struct {
	factory  ComponentFactory
	category Category
	metadata Metadata
}

// Registry is the process-wide collection of named ComponentFactories,
// tagged with Category so callers can enumerate by domain. Each registration
// also carries static Metadata (Inputs/Outputs) consumed by the API layer.
type Registry interface {
	Register(name string, category Category, factory ComponentFactory, metadata Metadata) error
	Lookup(name string) (ComponentFactory, Category, Metadata, bool)
	NamesByCategory(category Category) []string
	Names() []string
}

// memoryRegistry is the production Registry. It is concurrency-safe; init()
// race-to-register is acceptable for `Register` — the duplicate-registration
// check rejects the second writer, so the FIRST successful registration
// for a given name wins. (This is independent of the `SetDefaultFactory`
// "first-wins" guard, which is REMOVED in Phase 0 task 3; see
// "two-layer model" comment there for the distinction.)
//
// Lookup is **case-insensitive**: keys are lowercased on Register and Lookup.
// This matches internal/agent/component/registry.go:28, 43, which lowercases
// at both ends. A case-sensitive implementation would silently fail canvas
// build with "unknown component" errors when an existing init() registers
// "ExampleComponent" but the canvas looks up "examplecomponent" (or vice
// versa).
type memoryRegistry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

// Register enrolls a ComponentFactory under name (case-insensitive).
// Returns an error on empty name, empty metadata, or duplicate key —
// callers who want the legacy panic-on-duplicate semantics at init()
// time should wrap the call with MustRegister.
//
// "Empty metadata" (plan §4 Phase 0 task 1) means all three of
// Version, Inputs, Outputs are unset. The Version field is the
// canonical marker — ingestion components MUST supply a real
// version string; the legacy agent-component adapter MUST supply
// "legacy"; the catalog endpoint only serves components whose
// metadata passes this check.
func (r *memoryRegistry) Register(name string, category Category, factory ComponentFactory, metadata Metadata) error {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return fmt.Errorf("runtime: Register called with empty name")
	}
	if metadata.Version == "" && metadata.Inputs == nil && metadata.Outputs == nil {
		return fmt.Errorf("runtime: %q registered with empty metadata (Version, Inputs, Outputs all unset; "+
			"see plan §4 Phase 0 task 1 — ingestion components MUST supply a version string, "+
			"legacy agent-component adapter MUST supply {Version: \"legacy\"})", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[key]; exists {
		return fmt.Errorf("runtime: %q already registered", name)
	}
	r.entries[key] = entry{factory: factory, category: category, metadata: metadata}
	return nil
}

// Lookup resolves a name (case-insensitive) to its factory + category +
// metadata. Returns ok=false on miss.
func (r *memoryRegistry) Lookup(name string) (ComponentFactory, Category, Metadata, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[key]
	if !ok {
		return nil, "", Metadata{}, false
	}
	return e.factory, e.category, e.metadata, true
}

// NamesByCategory returns the sorted list of names registered under the
// given category. Sorted output keeps UI listings and error messages
// stable.
func (r *memoryRegistry) NamesByCategory(category Category) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for n, e := range r.entries {
		if e.category == category {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Names returns the sorted list of all registered names.
func (r *memoryRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for n := range r.entries {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// NewMemoryRegistry constructs an empty in-memory Registry. Tests use
// this to spin up isolated registries; production code uses
// DefaultRegistry.
func NewMemoryRegistry() Registry {
	return &memoryRegistry{entries: make(map[string]entry)}
}

// DefaultRegistry is the process-wide singleton. Each component package's
// init() registers its factories here. Lookup is lazy: even if a canvas
// build occurs before every package's init() has run, lookups see all
// completed registrations because the registry is read at call time, not
// at SetDefaultFactory time.
var DefaultRegistry Registry = NewMemoryRegistry()

// MustRegister wraps Register and panics on error. Init()-time callers
// that want the legacy "panic on duplicate" behaviour can use this
// instead of Register + manual error check.
func MustRegister(name string, category Category, factory ComponentFactory, metadata Metadata) {
	if err := DefaultRegistry.Register(name, category, factory, metadata); err != nil {
		panic(err)
	}
}

// RegisterWithMeta is a thin convenience around DefaultRegistry.Register
// for callers that want explicit (name, category, factory, metadata) at
// the call site without going through MustRegister. It returns the same
// error as Register; callers that want panic-on-error should use
// MustRegister instead.
func RegisterWithMeta(name string, category Category, factory ComponentFactory, metadata Metadata) error {
	return DefaultRegistry.Register(name, category, factory, metadata)
}
