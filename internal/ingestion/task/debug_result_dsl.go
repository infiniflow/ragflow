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
//  This file is the Go analogue of Python's `Graph.__str__`
//  (agent/canvas.py:119) used by the dataflow debug "View result" page.
//  Python attaches `dsl = json.loads(str(self))` to the END debug-log marker
//  (rag/flow/pipeline.py:98); the front-end reads `dsl.components[<id>].obj`
//  to render each component's parsed output. Go has no per-component `__str__`
//  serialization, so BuildDebugResultDSL combines the STATIC DSL structure
//  (component_name / downstream / params / graph.nodes) with the RUN output map
//  (state.go:284 `out[<componentID>] = <outputs>`) to emit the identical JSON
//  shape. Result is a strict subset (UI-relevant fields only) of Python's, so
//  the front-end renders chunks with parity and a smaller payload.

package task

import (
	"encoding/json"
	"fmt"
	"strings"

	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// ResultSink is an OPTIONAL capability a ProgressSink may implement to receive
// the debug-run result DSL plus the raw pipeline run output (output["state"]
// [<id>] is each component's outputs map). The pipeline executor probes for it
// via a type assertion, so the ProgressSink contract stays unchanged and
// non-debug (DB-backed) sinks simply ignore it — keeping the coupling
// one-directional.
type ResultSink interface {
	SetResult(dsl map[string]any, output map[string]any)
}

// outputFormats is the priority order used to pick a component's payload key,
// mirroring NormalizeChunks (chunk_utils.go:27).
var outputFormats = []string{"chunks", "json", "text", "html", "markdown"}

// vectorKeys are dropped from payloads while copying. The front-end never
// renders raw vectors and Python's serialized component obj excludes them;
// stripping keeps the Redis-stored debug log at Python-scale size.
var vectorKeys = map[string]struct{}{
	"vector":    {},
	"embedding": {},
	"q_vec":     {},
	"feature":   {},
}

// IsVectorKey reports whether k is a raw embedding-vector key that must be
// stripped from debug payloads. It matches the fixed legacy keys
// (vector/embedding/feature) AND the dimension-scoped pattern q_<dim>_vec that
// the tokenizer actually emits (see hasEmbeddingVector, tokenizer.go:828, e.g.
// q_4_vec, q_1024_vec). The literal "q_vec" entry never matches those, so a
// bare map lookup would let real vectors leak into the Redis log.
//
// Exported so the golden-compare tool (internal/ingestion/task/tool) reuses the
// exact same stripping rule instead of re-implementing a weaker copy.
func IsVectorKey(k string) bool {
	if _, ok := vectorKeys[k]; ok {
		return true
	}
	return strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec")
}

// BuildDebugResultDSL builds the `dsl` object the debug-log END marker carries
// so the front-end "View result" page can render each component's output.
//
// dsl is the raw canvas DSL JSON (optionally wrapped as {"dsl": {...}}). output
// is the pipeline run output keyed by component id (output[<id>] is that
// component's outputs map, which may carry chunks/json/text/html/markdown).
func BuildDebugResultDSL(dsl string, output map[string]any) (map[string]any, error) {
	var tpl map[string]any
	if err := json.Unmarshal([]byte(dsl), &tpl); err != nil {
		return nil, fmt.Errorf("BuildDebugResultDSL: unmarshal dsl: %w", err)
	}
	// Unwrap the canvas envelope via the shared helper so envelope handling
	// lives in exactly one place (pipeline.UnwrapCanvasDSL).
	root := tpl
	if inner, err := pipelinepkg.UnwrapCanvasDSL([]byte(dsl)); err == nil && inner != nil {
		root = inner
	}

	components, ok := root["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("BuildDebugResultDSL: dsl missing components map")
	}

	built := make(map[string]any, len(components))
	for id, raw := range components {
		comp, _ := raw.(map[string]any)
		if comp == nil {
			comp = map[string]any{}
		}

		// component_name: prefer the nested obj.component_name (Python shape),
		// fall back to a top-level component_name.
		name := ""
		var staticParams map[string]any
		if obj, _ := comp["obj"].(map[string]any); obj != nil {
			name, _ = obj["component_name"].(string)
			if p, _ := obj["params"].(map[string]any); p != nil {
				staticParams = p
			}
		}
		if name == "" {
			name, _ = comp["component_name"].(string)
		}

		down := comp["downstream"] // preserve as-is (string or []any)

		// Build obj.params: start from a deep copy of the static DSL params
		// (setups/field_name/...), then inject the runtime outputs wrapper.
		mergedParams := map[string]any{}
		for k, v := range staticParams {
			mergedParams[k] = deepCopy(v, false)
		}
		if format, payload := detectFormat(lookupComponentOutput(output, id)); format != "" {
			mergedParams["outputs"] = map[string]any{
				format: map[string]any{
					"value": deepCopy(payload, true),
					"type":  "",
				},
				"output_format": map[string]any{"value": format},
			}
		}

		built[id] = map[string]any{
			"obj": map[string]any{
				"component_name": name,
				"params":         mergedParams,
			},
			"downstream":     down,
			"component_name": name,
		}
	}

	result := map[string]any{
		"components": built,
		"graph":      deepCopy(root["graph"], false),
	}
	return result, nil
}

// lookupComponentOutput resolves a single component's runtime output map from
// the pipeline run result.
//
// The production `pipe.Run` return value nests every component's outputs under
// output["state"][<componentID>] — finalizeResult (pipeline.go:636) attaches
// runState.Snapshot() (state.go:296, map[string]map[string]any keyed by cpn
// id), and statePost (scheduler.go:229) writes each component's top-level
// output keys there via SetVar(cpnID, k, v). So the canonical lookup is
// output["state"][id].
//
// A flat keyed-by-id shape (output[id] directly) is accepted as a fallback so
// this builder stays usable for hand-built outputs in tests and any
// non-Snapshot callers; it is never produced by the real pipeline.
func lookupComponentOutput(output map[string]any, id string) any {
	// The production run output nests each component under
	// output["state"][<id>] (finalizeResult → runState.Snapshot(), state.go:296).
	// Snapshot returns map[string]map[string]any, but some callers build a
	// map[string]any-shaped state, so accept BOTH concrete types — a
	// single-type assertion would silently fail the real shape and fall through
	// to the (usually empty) top-level lookup.
	var found any
	var ok bool
	switch state := output["state"].(type) {
	case map[string]map[string]any:
		found, ok = state[id]
	case map[string]any:
		found, ok = state[id]
	}
	if ok {
		return found
	}
	// Fallback: flat shape (tests / non-Snapshot producers).
	return output[id]
}

// detectFormat returns the output key (chunks/json/text/html/markdown) present
// in a component's output map, by priority, plus the raw payload under it.
// Returns ("", nil) when the component produced no recognized output — the
// front-end then renders that step empty (matching Python's empty obj).
func detectFormat(out any) (string, any) {
	m, ok := out.(map[string]any)
	if !ok || m == nil {
		return "", nil
	}
	for _, f := range outputFormats {
		if v, exists := m[f]; exists && v != nil {
			return f, v
		}
	}
	return "", nil
}

// deepCopy returns a JSON-compatible deep copy of v (maps/slices/primitives),
// preserving structure but sharing nothing mutable with the source. When
// stripVector is true it additionally drops vector keys (see IsVectorKey) from
// every map it visits, so raw embedding vectors never reach the debug log.
func deepCopy(v any, stripVector bool) any {
	switch val := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, vv := range val {
			if stripVector && IsVectorKey(k) {
				continue
			}
			cp[k] = deepCopy(vv, stripVector)
		}
		return cp
	case []map[string]any:
		cp := make([]any, len(val))
		for i, vv := range val {
			cp[i] = deepCopy(vv, stripVector)
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, vv := range val {
			cp[i] = deepCopy(vv, stripVector)
		}
		return cp
	default:
		return v
	}
}
