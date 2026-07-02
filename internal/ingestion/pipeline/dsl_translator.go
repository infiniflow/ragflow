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

// TemplateToPipelineDSL — Phase 4 (port-rag-flow-pipeline-to-go.md §4
// row 4.2) bridges the production-template DSL shape into the
// PipelineDSL shape.
//
// The production templates under agent/templates/ingestion_pipeline_*.json
// use the modern keyed-components shape:
//
//	"dsl": {
//	  "components": {
//	    "File":              { "obj": { "component_name": "File", "params": {} }, ... },
//	    "Parser:HipSignsRhyme": { "obj": { "component_name": "Parser", "params": {...} }, ... },
//	    ...
//	  }
//	}
//
// PipelineDSL on the other hand is a linear Stages[] array. The
// canvas runner accepts the keyed-components shape directly, so
// the PipelineToCanvas adapter (commit f0bc8761e) is sufficient
// for the canvas path. But the pipeline package's existing
// `NewPipelineFromDSL` consumes `Stages[]`, and the per-stage
// component instantiation + parameter decoding in
// `Pipeline.Run` walks `stages` in order.
//
// TemplateToPipelineDSL walks the keyed-components shape and emits
// the linear Stages[] shape with id-sorted nodes. The canvas
// runner's downstream-ordering semantics are preserved by the
// existing PipelineToCanvas adapter's Upstream / Downstream
// wiring; this translator's job is to validate the keyed shape
// and produce the linear pipeline Stages[] that the legacy
// `NewPipelineFromDSL` consumes.
//
// Validation rules:
//
//   - canvas_category must equal "dataflow_canvas"
//   - every node's component_name must be a registered
//     CategoryIngestion component (Begin/Message/UserFillUp
//     etc. are explicitly rejected with ErrTemplateAgentNode)

package pipeline

import (
	"errors"
	"fmt"
	"sort"

	"ragflow/internal/agent/runtime"
)

// ErrTemplateNotIngestion is returned when the template's
// canvas_category is not "dataflow_canvas". Templates with
// canvas_category="agent" carry agent-side components (Begin,
// Message, UserFillUp, …) that the ingestion pipeline cannot run.
var ErrTemplateNotIngestion = errors.New("template is not an ingestion pipeline")

// ErrTemplateAgentNode is returned when a keyed-component node
// declares a component_name that is not registered under
// runtime.CategoryIngestion.
type ErrTemplateAgentNode struct {
	NodeID        string
	ComponentName string
}

func (e ErrTemplateAgentNode) Error() string {
	return fmt.Sprintf("template node %q references component %q which is not registered under CategoryIngestion",
		e.NodeID, e.ComponentName)
}

// TemplateToPipelineDSL walks the keyed-components shape and
// returns a PipelineDSL whose Stages[] is a sorted list of the
// input nodes (sorted by id for determinism). The canvas-side
// downstream ordering is preserved by PipelineToCanvas's
// Upstream / Downstream wiring on the converted Canvas — this
// translator's job is shape validation + linear emission, not
// edge walking.
func TemplateToPipelineDSL(template map[string]any) (*PipelineDSL, error) {
	if template == nil {
		return nil, errNilDSL
	}
	cat, _ := template["canvas_category"].(string)
	if cat != "dataflow_canvas" {
		return nil, fmt.Errorf("%w: canvas_category=%q", ErrTemplateNotIngestion, cat)
	}

	comps, ok := template["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template missing components map")
	}

	if len(comps) == 0 {
		return nil, errEmptyStages
	}

	ids := make([]string, 0, len(comps))
	for id, raw := range comps {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("template node %q: missing object map", id)
		}
		objInner, ok := obj["obj"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("template node %q: missing obj map", id)
		}
		name, _ := objInner["component_name"].(string)
		if name == "" {
			return nil, fmt.Errorf("template node %q: missing component_name", id)
		}
		if !isIngestionComponent(name) {
			return nil, ErrTemplateAgentNode{NodeID: id, ComponentName: name}
		}
		params, _ := objInner["params"].(map[string]any)
		comps[id] = map[string]any{
			"obj": map[string]any{
				"component_name": name,
				"params":         params,
			},
		}
		ids = append(ids, id)
	}

	sort.Strings(ids)
	stages := make([]StageDSL, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		name, params := extractStageSpec(comps, id)
		if seen[name] {
			// Duplicate component_name — pipeline shape is
			// singleton-per-component; reject rather than silently
			// dropping. The canvas shape carries per-instance ids,
			// so the legacy pipeline can't express multiple
			// instances of the same component.
			return nil, errDuplicateStage(name)
		}
		seen[name] = true
		stages = append(stages, StageDSL{Type: name, Params: params})
	}

	return &PipelineDSL{
		Version:    "1",
		Stages:     stages,
		StageCount: len(stages),
	}, nil
}

// isIngestionComponent checks whether the named component is
// registered under runtime.CategoryIngestion. Components outside
// that category (Begin / Message / UserFillUp / Retrieval / etc.)
// are explicitly rejected.
func isIngestionComponent(name string) bool {
	_, category, _, ok := runtime.DefaultRegistry.Lookup(name)
	if !ok {
		return false
	}
	return category == runtime.CategoryIngestion
}

// extractStageSpec returns the component_name + params map for
// the given keyed-component id. The translation reuses the
// decoded `comps` map so we avoid a second pass over the input.
func extractStageSpec(comps map[string]any, id string) (string, map[string]any) {
	raw, ok := comps[id].(map[string]any)
	if !ok {
		return "", nil
	}
	obj, ok := raw["obj"].(map[string]any)
	if !ok {
		return "", nil
	}
	name, _ := obj["component_name"].(string)
	params, _ := obj["params"].(map[string]any)
	return name, params
}
