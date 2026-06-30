// Package component — registry (orchestrator-owned, DO NOT EDIT).
//
// Registry maps component names to factories. Each component's init() calls
// Register(name, factory) to enroll itself; lookup is case-insensitive
// (matches Python v1 component_name case-insensitivity).
package component

import (
	"fmt"
	"strings"
	"sync"
)

// Factory constructs a Component from a params map (loaded from the DSL).
// Returning an error here aborts the run with a clear message.
type Factory func(params map[string]any) (Component, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register enrolls a component factory under name (case-insensitive).
// Intended to be called from init() in each component's <name>.go file.
func Register(name string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		panic("component: Register called with empty name")
	}
	if _, exists := registry[key]; exists {
		panic(fmt.Sprintf("component: %q already registered", name))
	}
	registry[key] = f
}

// New constructs a Component by name. Returns an error if the name is
// unknown or the factory rejects the params. The empty-string case is
// treated as "not found" so the error message is consistent.
func New(name string, params map[string]any) (Component, error) {
	registryMu.RLock()
	f, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("component: unknown component %q (registered: %s)", name, RegisteredNames())
	}
	if f == nil {
		return nil, fmt.Errorf("component: nil factory for %q", name)
	}
	return f(params)
}

// RegisteredNames returns the sorted list of registered component names.
// Used for diagnostics and the API 500 path "list available components".
func RegisteredNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	// Stable order for error messages / UI listing.
	sortStrings(names)
	return names
}

// sortStrings is a small in-place insertion sort to avoid the sort package
// dependency for a list that's <50 items long in practice.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
