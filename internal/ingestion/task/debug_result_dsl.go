// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package task

import (
	"encoding/json"
	"fmt"

	"ragflow/internal/common"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// vectorKeys are dropped from payloads while copying: the front-end never
// renders raw vectors, and the serialized component obj excludes them, so
// stripping keeps the Redis-stored debug log compact.
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
	return common.IsVectorColumn(k)
}

// buildPipelineLogDSL normalizes the canvas definition for operation-log storage.
func buildPipelineLogDSL(dsl string) (map[string]any, error) {
	var tpl map[string]any
	if err := json.Unmarshal([]byte(dsl), &tpl); err != nil {
		return nil, fmt.Errorf("buildPipelineLogDSL: unmarshal dsl: %w", err)
	}
	// Unwrap the canvas envelope via the shared helper so envelope handling
	// lives in exactly one place (pipeline.UnwrapCanvasDSL).
	root := tpl
	if inner, err := pipelinepkg.UnwrapCanvasDSL([]byte(dsl)); err == nil && inner != nil {
		root = inner
	}

	components, ok := root["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("buildPipelineLogDSL: dsl missing components map")
	}

	built := make(map[string]any, len(components))
	for id, raw := range components {
		comp, _ := raw.(map[string]any)
		if comp == nil {
			comp = map[string]any{}
		}

		// component_name: prefer the nested obj.component_name (the per-component payload shape),
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
		// Never forward a pre-existing outputs wrapper from the input DSL into
		// the rebuilt params: the input can be an output-bearing copy (e.g. a
		// stale pipeline_operation_log row loaded back as a canvas), and the
		// persisted log must carry the DSL definition ONLY.
		delete(mergedParams, "outputs")

		built[id] = map[string]any{
			"obj": map[string]any{
				"component_name": name,
				"params":         mergedParams,
			},
			"downstream":     down,
			"component_name": name,
		}
	}

	// Carry every non-components top-level key (graph, path, task_id,
	// canvas_type, ...) verbatim. The
	// persisted log DSL round-trips through the front-end rerun flow, so a
	// dropped key is lost to every consumer.
	result := make(map[string]any, len(root)+1)
	for k, v := range root {
		if k == "components" {
			continue
		}
		result[k] = deepCopy(v, false)
	}
	result["components"] = built
	return result, nil
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
