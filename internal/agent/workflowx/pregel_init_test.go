// Package workflowx — pregel engine init for tests.
//
// This file ensures the Pregel execution engine is registered (via its
// init()) when any workflowx test creates a CompiledGraph and calls
// Invoke/Stream. Without it, compiledGraph.run() returns
// "graph: pregel engine not installed".
package workflowx

import (
	_ "ragflow/internal/harness/graph/pregel"
)

// extractIntFromState extracts an int value from the pregel engine's
// map-based output. With the pregel engine, node output is wrapped in
// a map[string]any keyed by the channel name. When the graph has a
// "__root__" channel, the value is under m["__root__"].
func extractIntFromState(state any) (int, bool) {
	if state == nil {
		return 0, false
	}
	if v, ok := state.(int); ok {
		return v, true
	}
	m, ok := state.(map[string]any)
	if !ok {
		return 0, false
	}
	raw, has := m["__root__"]
	if !has {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

// extractSliceFromState extracts a []interface{} from the pregel engine's
// map-based output. When the graph's final state has a "__root__" channel
// containing a slice, this unwraps it.
func extractSliceFromState(state any) ([]interface{}, bool) {
	if state == nil {
		return nil, false
	}
	if s, ok := state.([]interface{}); ok {
		return s, true
	}
	m, ok := state.(map[string]any)
	if !ok {
		return nil, false
	}
	raw, has := m["__root__"]
	if !has {
		return nil, false
	}
	s, ok := raw.([]interface{})
	return s, ok
}
