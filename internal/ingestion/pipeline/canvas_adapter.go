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

// PipelineToCanvas — Phase 4 (port-rag-flow-pipeline-to-go.md §4
// Phase 4 task 4.1) bridges the ingestion PipelineDSL shape into
// the canvas DSL so a future Pipeline.Run() can drive the canvas
// runner (Runner.Run) rather than walking stages directly.
//
// Why an adapter rather than a re-shape:
//
//   - The PipelineDSL is a deliberate narrow contract for the
//     ingestion flow (linear 5-stage: File -> Parser -> Chunker ->
//     Tokenizer -> Extractor). It exists so the Ingestor's queue
//     payload stays trivial: a task carries a tiny JSON blob
//     describing "which components, in what order".
//
//   - The canvas DSL is a general DAG. It is rich (Upstream /
//     Downstream arrays, history, retrieval, globals) because the
//     front-end builds arbitrary agent graphs with branches, loops,
//     parallel sub-graphs, etc.
//
// Forcing the PipelineDSL into the canvas DSL would either (a)
// bloat the ingestion schema with canvas concepts it does not
// need, or (b) require a parallel ingestion-canvas DSL. The
// adapter in this file picks option (c) — keep the PipelineDSL
// narrow, build a Canvas at the boundary, hand it to the canvas
// runner.
//
// This file is the bridge between the ingestion DSL and the canvas
// DSL. Pipeline.Run now executes through canvas.Compile(...)+Invoke,
// and this adapter remains the unit-testable seam that pins the
// linear ingestion graph shape at that boundary.
//
// Boundaries:
//
//   - PipelineToCanvas(d *PipelineDSL) *canvas.Canvas — pure
//     data transform. No side effects, no registry lookups. The
//     test suite pins the shape so a refactor that adds branching
//     or loop support cannot regress the linear-path contract.
//
//   - The Canvas is built with each stage's id set to the stage
//     Type (not a per-instance id). The pipeline flow is linear
//     and singleton-per-stage today, so a per-stage-name id is
//     sufficient and matches the agent canvas convention of
//     "id == component_name" for one-of-each graphs. A future
//     multi-instance flow will need a different id strategy
//     (e.g. "<task>:<index>:<type>") — pin that as a follow-up.

package pipeline

import (
	"fmt"

	"ragflow/internal/agent/canvas"
)

// PipelineToCanvas converts an ingestion PipelineDSL into the
// canvas.Canvas DSL the canvas runner consumes. The conversion
// preserves stage order: pipeline stages become canvas path
// entries, and the linear chain is expressed through each
// component's Upstream / Downstream arrays so a downstream
// canvas workflow sees the same graph shape an agent graph
// would.
//
// Returns an error when:
//
//   - d is nil,
//   - d fails IsValid() (delegated so the caller does not need to
//     pre-validate),
//   - any stage Type is empty (a defensive check — IsValid
//     already catches this, but the adapter surfaces its own
//     structured error to make a future "convert a partial DSL"
//     path safe).
//
// On a successful conversion, every stage in d.Stages appears
// exactly once in the returned canvas's path AND in its
// components map. The order of the path matches the order of
// d.Stages.
func PipelineToCanvas(d *PipelineDSL) (*canvas.Canvas, error) {
	if d == nil {
		return nil, errNilDSL
	}
	if err := d.IsValid(); err != nil {
		return nil, err
	}

	out := &canvas.Canvas{
		Components: make(map[string]canvas.CanvasComponent, len(d.Stages)),
		Path:       make([]string, 0, len(d.Stages)),
		// Globals is left nil. The ingestion flow does not use
		// global variables the way an agent canvas does; the
		// field is reserved for the runner-time merge when a
		// future slice introduces cross-stage variable references.
		// History / Retrieval / NodeParents are likewise nil
		// — they have no analogue in the ingestion schema.
	}

	// Build a deterministic id for each stage. The pipeline flow
	// is single-instance per type, so the stage's Type doubles
	// as the canvas component id. A future multi-instance flow
	// must change this to "<type>:<index>".
	ids := make([]string, len(d.Stages))
	for i, s := range d.Stages {
		if s.Type == "" {
			return nil, errEmptyStageType(i)
		}
		ids[i] = s.Type
	}

	// First pass: register every component with empty edges so
	// the second pass can wire Upstream / Downstream without a
	// forward-reference problem.
	for i, s := range d.Stages {
		id := ids[i]
		out.Components[id] = canvas.CanvasComponent{
			Obj: canvas.CanvasComponentObj{
				ComponentName: s.Type,
				Params:        s.Params,
			},
			// Upstream / Downstream are populated in the next
			// loop. Canvas.Compile tolerates an empty slice —
			// the path array is the source of truth for
			// execution order, while the per-node Upstream /
			// Downstream arrays are used by the scheduler to
			// resolve preconditions.
		}
		out.Path = append(out.Path, id)
	}

	// Second pass: wire the linear chain. The first node has
	// no upstream; the last has no downstream; everything in
	// between has one of each.
	for i, id := range ids {
		comp := out.Components[id]
		if i > 0 {
			comp.Upstream = []string{ids[i-1]}
		}
		if i < len(ids)-1 {
			comp.Downstream = []string{ids[i+1]}
		}
		out.Components[id] = comp
	}

	return out, nil
}

// MustPipelineToCanvas is the panic-on-error form. Used by test
// fixtures that construct a PipelineDSL inline and want the
// conversion to be infallible at the call site. Production code
// must use PipelineToCanvas and handle the error.
func MustPipelineToCanvas(d *PipelineDSL) *canvas.Canvas {
	c, err := PipelineToCanvas(d)
	if err != nil {
		panic(fmt.Sprintf("pipeline: MustPipelineToCanvas: %v", err))
	}
	return c
}
