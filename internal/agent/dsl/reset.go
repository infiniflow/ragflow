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

// reset.go: DSL-level "reset" transform that mirrors the runtime
// behaviour of agent/canvas.py:Canvas.reset() in the Python backend.
//
// Python's Canvas.reset() does two things:
//
//  1. Graph.reset(): clears the per-component state (path, in-memory
//     caches) and removes the per-task Redis log/cancel keys.
//
//  2. Per-run state wipe: empties self.history / retrieval / memory,
//     then walks self.globals to zero out every "sys.*" key and to
//     restore every "env.*" key from its declared default in
//     self.variables.
//
// In the Go port there is no per-canvas "Graph" runtime — the
// executor is reconstructed from the DSL on every Run. So the
// Python "Graph.reset()" side (step 1) is implicitly handled by the
// per-run rebuild and the per-task Redis keys are still owned by the
// Python task executor. The Go port is responsible for the
// per-DSL-state wipe (step 2): it transforms the persisted DSL
// saved in user_canvas.dsl, the same way the Python handler does
// before writing it back via UserCanvasService.update_by_id.
//
// Frontend parity note: api/apps/restful_apis/agent_api.py:992
// (reset_agent) calls Canvas.reset() and returns the reset DSL in
// the response. The Go handler returns the same shape so existing
// frontends that call POST /api/v1/agents/:canvas_id/reset continue
// to receive the new DSL.

package dsl

// ResetForCanvas returns a defensive copy of dsl with all per-run
// state cleared, ready to be persisted back into user_canvas.dsl.
//
// The transform matches the Python Canvas.reset() semantics on the
// persisted DSL:
//
//   - history, retrieval, memory, path → emptied
//   - globals["sys.<name>"] → zeroed by type (string→"", number→0,
//     bool→false, list→[], dict→{}, other→nil)
//   - globals["env.<name>"] → restored from variables[name].value
//     when present; otherwise zeroed by the variable's declared
//     "type" (number→0, boolean→false, object→{}, array→[], else→"")
//
// Anything else in the DSL (graph, components, messages, ...)
// is left untouched, matching the Python implementation which
// only mutates history/retrieval/memory + globals.
func ResetForCanvas(dsl map[string]any) map[string]any {
	if dsl == nil {
		return map[string]any{}
	}
	out := copyMapStringAny(dsl)

	// Per-run accumulators. The Python implementation assigns fresh
	// empty lists to each; we mirror that by replacing whatever is
	// stored under these keys with a fresh slice. Using a fresh slice
	// (not a shared nil sentinel) matches the Python [] list literal
	// in __str__ / reset.
	out["history"] = []any{}
	out["retrieval"] = []any{}
	out["memory"] = []any{}
	out["path"] = []any{}

	// Snapshot variables (env.* defaults) so the env.* reset loop
	// below is stable even when globals is otherwise empty.
	// Deep-copy both maps — the reset loop mutates `globals` in
	// place, and the service layer feeds the same DSL back into
	// the response body after persistence. A shallow copy would
	// leak the wipe back into the caller's view of the row.
	vars, _ := out["variables"].(map[string]any)
	if vars == nil {
		vars = map[string]any{}
	}
	vars = deepCopyMap(vars)

	globals, _ := out["globals"].(map[string]any)
	if globals == nil {
		// An empty / missing globals map is valid: Python's reset
		// iterates self.globals.keys() and is a no-op when empty,
		// leaving globals as the (possibly empty) dict it was. We
		// preserve that shape instead of inserting a nil.
		return out
	}
	globals = deepCopyMap(globals)
	// Stash the (deep-copied) globals back into out so the
	// returned DSL reflects every change the reset loop makes.
	out["globals"] = globals

	// Reset in place on the snapshot. Go map iteration order is
	// non-deterministic, so collect the sys./env. keys first and
	// then mutate the map to avoid any "read+write during
	// iteration" gotcha.
	sysKeys := make([]string, 0)
	envKeys := make([]string, 0)
	for k := range globals {
		switch {
		case len(k) > 4 && k[:4] == "sys.":
			sysKeys = append(sysKeys, k)
		case len(k) > 4 && k[:4] == "env.":
			envKeys = append(envKeys, k)
		}
	}

	for _, k := range sysKeys {
		globals[k] = zeroByType(globals[k])
	}
	for _, k := range envKeys {
		name := k[4:]
		v, ok := vars[name].(map[string]any)
		if !ok {
			// No declared default → empty string, matching the
			// Python `else: self.globals[k] = ""` branch when
			// the variable entry is missing entirely.
			globals[k] = ""
			continue
		}
		if value, present := v["value"]; present && value != nil {
			globals[k] = value
			continue
		}
		globals[k] = zeroByVariableType(v)
	}

	return out
}

// zeroByType returns the type-appropriate "empty" value for v,
// matching the Python reset() branch for sys.* keys:
//
//	string  -> ""
//	int     -> 0
//	float   -> 0
//	list    -> []
//	dict    -> {}
//	other   -> nil
//
// The list / dict branches return a fresh empty container, not a
// shared nil — consistent with the Python literal `[]` / `{}`.
// Primitives (string, int, float) are returned as fresh zero
// values; this is fine because the caller is going to overwrite
// the map entry with the return value anyway.
func zeroByType(v any) any {
	switch v.(type) {
	case string:
		return ""
	case bool:
		return false
	case int:
		return 0
	case int32:
		return int32(0)
	case int64:
		return int64(0)
	case float32:
		return float32(0)
	case float64:
		return float64(0)
	case []any:
		return []any{}
	case map[string]any:
		return map[string]any{}
	default:
		return nil
	}
}

// zeroByVariableType mirrors the Python `else` branch that runs
// when an env.* variable is declared but has no `value` field.
// The Python source keys on the declared "type" string:
//
//	"number"   -> 0
//	"boolean"  -> False
//	"object"   -> {}
//	"array*"   -> []
//	else       -> ""   (covers "string" and unknown)
func zeroByVariableType(v map[string]any) any {
	t, _ := v["type"].(string)
	switch t {
	case "number":
		return 0
	case "boolean":
		return false
	case "object":
		return map[string]any{}
	}
	if len(t) >= 5 && t[:5] == "array" {
		return []any{}
	}
	return ""
}

// deepCopyMap returns a fresh map with the same keys, recursively
// copying nested map / slice values. Primitives are shared by
// reference (they are immutable in Go). This is a focused helper
// for the reset path: in practice globals is a flat
// string→primitive map and variables is a flat
// string→{type, value} map, so a full deep walk is overkill, but
// the cost is negligible and it eliminates a class of
// "the caller's map got mutated" bugs the shallow `copyMapStringAny`
// helper would let through.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			out[k] = deepCopyMap(x)
		case []any:
			out[k] = deepCopySlice(x)
		default:
			out[k] = v
		}
	}
	return out
}

func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		switch x := v.(type) {
		case map[string]any:
			out[i] = deepCopyMap(x)
		case []any:
			out[i] = deepCopySlice(x)
		default:
			out[i] = x
		}
	}
	return out
}
