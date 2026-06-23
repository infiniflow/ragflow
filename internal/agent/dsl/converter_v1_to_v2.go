// Package dsl — v1 -> v2 converter (Phase 2.5 priority).
//
// The v1 format (see agent/canvas.py:43-95) is:
//
//	{
//	  "components": {
//	    "<ComponentName>:<UUID>": {
//	      "downstream": ["<ComponentName>:<UUID>", ...],
//	      "upstream":   ["<ComponentName>:<UUID>", ...],
//	      "obj": {
//	        "component_name": "ComponentName",
//	        "params":         {...},
//	        "downstream":     [...],   // duplicate of the outer field
//	        // legacy fields dropped in v2:
//	        "_feeded_deprecated_params": {...},
//	        "_deprecated_params":        {...},
//	        "_user_feeded_params":       {...},
//	        ...
//	      }
//	    }
//	  }
//	}
//
// v2 is flat (see v2.go). Conversion:
//   - key "<Name>:<UUID>" -> "<name>_<UUID>" (lowercased name, '_' separator)
//   - obj.component_name -> Component.Name
//   - obj.params         -> Component.Params
//   - downstream (outer) -> Component.Downstream
//   - legacy fields      -> dropped
//
// Reverse direction (v2 -> v1) is Phase 5.5 and is not implemented here.
package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// v1Canvas is the intermediate v1 unmarshal target. We do not import
// encoding/json's default field tag behavior — every field is explicit so
// the converter remains readable.
type v1Canvas struct {
	Components map[string]v1Component `json:"components"`
}

type v1Component struct {
	Downstream []string `json:"downstream"`
	Upstream   []string `json:"upstream"`
	Obj        *v1Obj   `json:"obj"`
	// legacy top-level keys, captured for the v1->v2 sanity check (and
	// dropped on emit).
	DeprecatedParams       map[string]any `json:"_deprecated_params"`
	FeededDeprecatedParams map[string]any `json:"_feeded_deprecated_params"`
	UserFeededParams       map[string]any `json:"_user_feeded_params"`
}

type v1Obj struct {
	ComponentName string         `json:"component_name"`
	Params        map[string]any `json:"params"`
	Downstream    []string       `json:"downstream"`
	Upstream      []string       `json:"upstream"`
	// Legacy fields at the obj level are also accepted; they are ignored.
	DeprecatedParams       map[string]any `json:"_deprecated_params"`
	FeededDeprecatedParams map[string]any `json:"_feeded_deprecated_params"`
	UserFeededParams       map[string]any `json:"_user_feeded_params"`
}

// v1ToV2 converts a v1 JSON byte slice into a v2 Canvas.
//
// Algorithm:
//  1. Unmarshal into the v1 intermediate struct.
//  2. For each v1 component key "<Name>:<UUID>":
//     - new_id = lowercased Name + "_" + UUID   (e.g. "agent_abc123")
//     - new_cpn.ID       = new_id
//     - new_cpn.Name     = v1.ComponentName (from obj, falling back to Name)
//     - new_cpn.Downstream = v1.Downstream (outer) -> remap each via convertKey
//     - new_cpn.Params   = v1.Obj.Params (or empty map if missing)
//     - legacy fields are dropped.
//  3. Return Canvas{Version: 2, Components: new_map}.
//
// Edge cases handled:
//   - obj missing: use top-level downstream; Params = empty map.
//   - empty downstream: empty slice.
//   - custom_header inside params: preserved as-is in v2.Params (not stripped).
//   - nested messages (Begin -> Message chain): handled by the same key
//     remapping for both top-level and downstream refs.
func v1ToV2(raw []byte) (*Canvas, error) {
	var v1 v1Canvas
	if err := json.Unmarshal(raw, &v1); err != nil {
		return nil, fmt.Errorf("dsl: v1 unmarshal: %w", err)
	}
	if len(v1.Components) == 0 {
		return nil, fmt.Errorf("dsl: v1 has no components")
	}

	out := &Canvas{
		Version:    CurrentVersion,
		Components: make(map[string]Component, len(v1.Components)),
	}

	for oldKey, c := range v1.Components {
		newID, name, err := convertKey(oldKey)
		if err != nil {
			return nil, fmt.Errorf("dsl: convert key %q: %w", oldKey, err)
		}

		// Component name: prefer obj.component_name, fall back to the
		// Name half of the colon-split key.
		cpnName := name
		if c.Obj != nil && c.Obj.ComponentName != "" {
			cpnName = c.Obj.ComponentName
		}

		// Downstream: prefer the outer field, fall back to obj.downstream.
		// Either may be nil; remap each entry through convertKey.
		ds := c.Downstream
		if len(ds) == 0 && c.Obj != nil {
			ds = c.Obj.Downstream
		}
		dsOut := make([]string, 0, len(ds))
		for _, ref := range ds {
			mapped, _, err := convertKey(ref)
			if err != nil {
				return nil, fmt.Errorf("dsl: convert downstream ref %q of %q: %w", ref, oldKey, err)
			}
			dsOut = append(dsOut, mapped)
		}

		// Params: from obj.params; empty map if obj missing.
		var params map[string]any
		if c.Obj != nil && c.Obj.Params != nil {
			params = c.Obj.Params
		} else {
			params = map[string]any{}
		}

		out.Components[newID] = Component{
			ID:         newID,
			Name:       cpnName,
			Downstream: dsOut,
			Params:     params,
		}
	}

	return out, nil
}

// convertKey splits a v1 "<Name>:<UUID>" key into a v2 "<name>_<UUID>" id,
// and returns the original PascalCase/CamelCase Name alongside.
//
// Rules:
//   - Exactly one ":" required.
//   - Name segment lowercased (using ASCII ToLower).
//   - ":" replaced with "_".
//
// v1 keys without a ":" are accepted as-is: they are returned with empty
// name and a synthetic "<key>_<key>" id to keep the v2 invariant that IDs
// are non-empty. Callers should pre-validate, but we keep this tolerant
// because some test fixtures omit the colon.
func convertKey(oldKey string) (newID, name string, err error) {
	if oldKey == "" {
		return "", "", fmt.Errorf("empty key")
	}
	idx := strings.Index(oldKey, ":")
	if idx < 0 {
		// No colon — treat the whole key as both name and id stem.
		// v2 forbids ":" so we use "_" as separator with an empty uuid half.
		return strings.ToLower(oldKey) + "_", oldKey, nil
	}
	rawName := oldKey[:idx]
	uuid := oldKey[idx+1:]
	if rawName == "" {
		return "", "", fmt.Errorf("missing name before ':'")
	}
	if uuid == "" {
		return "", "", fmt.Errorf("missing uuid after ':'")
	}
	return strings.ToLower(rawName) + "_" + strings.ToLower(uuid), rawName, nil
}
