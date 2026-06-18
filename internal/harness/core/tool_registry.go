package core

import (
	"fmt"
	"sync"

	"ragflow/internal/harness/core/schema"
)

// ToolRegistry provides centralized tool management with aliases, categories,
// and filtering. It replaces raw []Tool slices for more flexible tool discovery.
type ToolRegistry struct {
	mu       sync.RWMutex
	tools    map[string]Tool          // name -> tool
	aliases  map[string]string        // alias -> canonical name
	category map[string][]string      // category -> tool names
}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:    make(map[string]Tool),
		aliases:  make(map[string]string),
		category: make(map[string][]string),
	}
}

// Register adds a tool and optionally aliases and categories.
func (r *ToolRegistry) Register(tool Tool, opts ...RegistryOption) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	r.tools[name] = tool
	for _, opt := range opts {
		opt(name, r)
	}
}

// RegistryOption configures a tool registration.
type RegistryOption func(name string, r *ToolRegistry)

// WithAlias registers an alias for the tool.
func WithAlias(alias string) RegistryOption {
	return func(name string, r *ToolRegistry) {
		r.aliases[alias] = name
	}
}

// WithCategory assigns the tool to one or more categories.
func WithCategory(categories ...string) RegistryOption {
	return func(name string, r *ToolRegistry) {
		for _, cat := range categories {
			r.category[cat] = append(r.category[cat], name)
		}
	}
}

// Lookup finds a tool by name or alias.
func (r *ToolRegistry) Lookup(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.tools[name]; ok {
		return t, true
	}
	if canonical, ok := r.aliases[name]; ok {
		t, ok := r.tools[canonical]
		return t, ok
	}
	return nil, false
}

// LookupByCategory returns all tools in a category.
func (r *ToolRegistry) LookupByCategory(category string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := r.category[category]
	result := make([]Tool, 0, len(names))
	for _, n := range names {
		if t, ok := r.tools[n]; ok {
			result = append(result, t)
		}
	}
	return result
}

// AllTools returns all registered tools as a slice.
func (r *ToolRegistry) AllTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ToSlice converts the registry to a []Tool for use with existing APIs.
func (r *ToolRegistry) ToSlice() []Tool {
	return r.AllTools()
}

// Merge merges another registry into this one. Conflicts are resolved by source winning.
// Uses a snapshot-then-apply pattern to avoid deadlock: other's data is read under
// RLock before locking r. Self-merge (r.Merge(r)) is handled as a no-op.
func (r *ToolRegistry) Merge(other *ToolRegistry) {
	if r == other {
		return
	}

	// Snapshot other's data under read lock.
	other.mu.RLock()
	tools := make(map[string]Tool, len(other.tools))
	for k, v := range other.tools {
		tools[k] = v
	}
	aliases := make(map[string]string, len(other.aliases))
	for k, v := range other.aliases {
		aliases[k] = v
	}
	categories := make(map[string][]string, len(other.category))
	for k, v := range other.category {
		categories[k] = append([]string{}, v...)
	}
	other.mu.RUnlock()

	// Apply snapshot under our write lock.
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, tool := range tools {
		r.tools[name] = tool
	}
	for alias, canonical := range aliases {
		r.aliases[alias] = canonical
	}
	for cat, names := range categories {
		r.category[cat] = append(r.category[cat], names...)
	}
}

// Filter returns a new registry containing only tools matching the predicate.
func (r *ToolRegistry) Filter(fn func(Tool) bool) *ToolRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := NewToolRegistry()
	for _, t := range r.tools {
		if fn(t) {
			result.Register(t)
		}
	}
	return result
}

// MustLookup looks up a tool by name and panics if not found (for use in init()).
func (r *ToolRegistry) MustLookup(name string) Tool {
	t, ok := r.Lookup(name)
	if !ok {
		panic(fmt.Sprintf("tool '%s' not found in registry", name))
	}
	return t
}

// Unregister removes a tool by name.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
	for alias, canonical := range r.aliases {
		if canonical == name {
			delete(r.aliases, alias)
		}
	}
	for cat, names := range r.category {
		filtered := names[:0]
		for _, n := range names {
			if n != name {
				filtered = append(filtered, n)
			}
		}
		if len(filtered) == 0 {
			delete(r.category, cat)
		} else {
			r.category[cat] = filtered
		}
	}
}

// ToolInfos returns metadata for all registered tools. When a tool implements
// ToolInfoProvider, its full structured info is used; otherwise a minimal
// Name+Description info is created.
func (r *ToolRegistry) ToolInfos() []*schema.ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]*schema.ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		if p, ok := t.(ToolInfoProvider); ok {
			infos = append(infos, p.ToolInfo())
		} else {
			infos = append(infos, &schema.ToolInfo{Name: t.Name(), Description: t.Description()})
		}
	}
	return infos
}
