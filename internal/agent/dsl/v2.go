// Package dsl provides the Go-native v2 schema, version auto-detection loader,
// and v1->v2 converter for the RAGFlow agent canvas.
//
// Phase 2.5 introduces the v2 schema. The Python-era v1 DSL (with the `obj`
// wrapper and `_feeded_deprecated_params` / `_deprecated_params` /
// `_user_feeded_params` legacy fields) is still loadable via the converter
// (see converter_v1_to_v2.go). v2 -> v1 round-trip is deferred to Phase 5.5.
//
// See plan: .claude/plans/agent-go-port.md
//   - §3.1 (In-scope)
//   - §2.11.7 (Python-era fields removed in v2)
//   - §4.6 (DSL v2 schema)
//   - §4.7 (v1 <-> v2 converter)
//   - §5 Phase 2.5
package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// CurrentVersion is the schema version emitted by the v2 loader and converter.
const CurrentVersion = 2

// Canvas is the v2 in-memory model of a RAGFlow agent canvas.
//
// A Canvas is a directed graph keyed by Component.ID. v2 removes the v1
// `obj` wrapper and the three deprecated param sets; the schema is flat.
type Canvas struct {
	// Version is always 2 for a Canvas produced by this package.
	Version int `json:"version"`

	// Components is keyed by Component.ID (v2 uses simple `name_id` keys,
	// e.g. "begin_0", "llm_3" — no `Name:UUID` colon-prefixed keys).
	Components map[string]Component `json:"components"`
}

// Component is a single node in a v2 canvas.
//
// Required fields are ID, Name, Downstream, Params. Outputs is optional and
// reserved for runtime materialization; it is not present in a freshly
// loaded DSL.
type Component struct {
	// ID is the canvas-local identifier. For v1-converted components, it is
	// the lowercased name + "_" + uuid, e.g. "agent_abc123".
	ID string `json:"id"`

	// Name is the human-readable component class name, e.g. "Retrieval",
	// "LLM", "Agent". Matches v1 `obj.component_name`.
	Name string `json:"name"`

	// Downstream lists canvas-local IDs of components that follow this one.
	// Empty for terminal nodes.
	Downstream []string `json:"downstream"`

	// Params is the v2 parameter bag. v1 legacy fields
	// (_feeded_deprecated_params / _deprecated_params / _user_feeded_params)
	// are dropped on conversion.
	Params map[string]any `json:"params"`

	// Outputs is the runtime materialization slot; not present in DSL on
	// load. It is reserved for future Phase 5 execution wiring.
	Outputs map[string]any `json:"outputs,omitempty"`
}

// Marshal serializes a Canvas to compact JSON bytes.
func (c *Canvas) Marshal() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("dsl: nil Canvas")
	}
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	return json.Marshal(c)
}

// MarshalIndent serializes a Canvas to indented JSON bytes.
func (c *Canvas) MarshalIndent() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("dsl: nil Canvas")
	}
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	return json.MarshalIndent(c, "", "  ")
}

// UnmarshalV2 parses a v2 JSON byte slice into a Canvas. It rejects v1
// payloads (no `version` field, contains `obj` wrappers) — use Load for
// auto-detect, or LoadV1 for v1-only callers.
func UnmarshalV2(raw []byte) (*Canvas, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c Canvas
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("dsl: parse v2: %w", err)
	}
	if c.Version != CurrentVersion {
		return nil, fmt.Errorf("dsl: expected version %d, got %d", CurrentVersion, c.Version)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("dsl: validate v2: %w", err)
	}
	return &c, nil
}

// Validate checks the structural well-formedness of a v2 Canvas.
//
// Rules:
//   - Version must be 2.
//   - Components map must be non-empty.
//   - Each Component.Name must be non-empty.
//   - Each Downstream entry must reference an existing component ID in
//     Components (no dangling refs).
//   - No `:` in component IDs (v2 forbids the v1 `Name:UUID` convention).
func (c *Canvas) Validate() error {
	if c == nil {
		return fmt.Errorf("nil Canvas")
	}
	if c.Version != CurrentVersion {
		return fmt.Errorf("version=%d, expected %d", c.Version, CurrentVersion)
	}
	if len(c.Components) == 0 {
		return fmt.Errorf("components map is empty")
	}
	for id, comp := range c.Components {
		if strings.Contains(id, ":") {
			return fmt.Errorf("component id %q contains ':' (v1 legacy)", id)
		}
		if comp.Name == "" {
			return fmt.Errorf("component %q has empty Name", id)
		}
		if id != comp.ID && comp.ID != "" {
			// comp.ID is set explicitly; allow if it matches the key.
			return fmt.Errorf("component key %q does not match Component.ID %q", id, comp.ID)
		}
		for _, ds := range comp.Downstream {
			if _, ok := c.Components[ds]; !ok {
				return fmt.Errorf("component %q downstream %q does not exist", id, ds)
			}
		}
	}
	return nil
}
