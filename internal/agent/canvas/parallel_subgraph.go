// parallel_subgraph.go — Parallel macro expansion utilities.
//
// The canvas layer uses the harness graph's built-in parallel support
// (graph/graph/parallel.go). This file retains the non-eino utility
// functions from the upstream parallel expansion.
package canvas

import (
	"encoding/json"
	"fmt"
)

// collectGroupedMembers returns the children of a group parent from
// the Canvas's NodeParents metadata.
func collectGroupedMembers(c *Canvas, parentID string) map[string]bool {
	out := make(map[string]bool)
	if c == nil || len(c.NodeParents) == 0 || parentID == "" {
		return out
	}
	for childID, groupID := range c.NodeParents {
		if groupID == parentID && c.Components[childID].Obj.ComponentName != "" {
			out[childID] = true
		}
	}
	return out
}

// toParallelItems converts a raw items value into the slice-of-maps
// format expected by the parallel node body. Mirrors the Python
// canvas-level item-to-parallel-input conversion.
func toParallelItems(raw any) ([]map[string]any, error) {
	switch items := raw.(type) {
	case nil:
		return []map[string]any{}, nil
	case []string:
		out := make([]map[string]any, 0, len(items))
		for i, item := range items {
			out = append(out, map[string]any{"item": item, "index": i})
		}
		return out, nil
	case []any:
		out := make([]map[string]any, 0, len(items))
		for i, item := range items {
			out = append(out, map[string]any{"item": item, "index": i})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", raw)
	}
}

// cloneCanvasState deep-copies a CanvasState via JSON round-trip.
// Used by the parallel body to give each concurrent item its own state.
func cloneCanvasState(src *CanvasState) (*CanvasState, error) {
	if src == nil {
		return NewCanvasState("", ""), nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	dst := NewCanvasState(src.RunID, src.TaskID)
	if err := json.Unmarshal(raw, dst); err != nil {
		return nil, err
	}
	return dst, nil
}

// shallowCopyAnyMap returns a new map with the same keys/values as src.
// A nil src yields an empty (non-nil) map so callers can assign into the
// result without nil checks. Values are shared, not deep-copied.
func shallowCopyAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// resolveParallelItemRef resolves a reference path against a single
// parallel item's output map. Supports "item", "index", and
// "cpnID@param[.subpath]" forms.
func resolveParallelItemRef(itemOut map[string]any, ref string) any {
	if ref == "" || itemOut == nil {
		return nil
	}
	if ref == "item" {
		return itemOut["item"]
	}
	if ref == "index" {
		return itemOut["index"]
	}

	if at := indexAt(ref); at > 0 {
		cpnID := ref[:at]
		param := ref[at+1:]
		if bucket, ok := itemOut[cpnID].(map[string]any); ok {
			return dotTraverseAny(bucket, param)
		}
	}
	return nil
}

func indexAt(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return i
		}
	}
	return -1
}

func dotTraverseAny(v any, path string) any {
	if path == "" {
		return v
	}
	cur := v
	segStart := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '.' {
			continue
		}
		seg := path[segStart:i]
		segStart = i + 1
		m, ok := cur.(map[string]any)
		if !ok || seg == "" {
			return nil
		}
		cur = m[seg]
		if cur == nil {
			return nil
		}
	}
	return cur
}
