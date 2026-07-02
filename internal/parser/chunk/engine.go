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

// Chunk engine — compile-time DSL→operator list, run-time execute
// against text. The engine itself is a thin orchestrator on top of
// the per-stage operators in this package (PreprocessOperator /
// SplitOperator / PostprocessOperator); the DSL shape is:
//
//	{
//	  "name": "...",
//	  "version": "1.0",
//	  "description": "...",
//	  "pipeline": [
//	    {"operator": "preprocess",  ...},
//	    {"operator": "split",       ...},
//	    {"operator": "postprocess", ...}
//	  ]
//	}
//
// The engine was moved here from the legacy
// internal/ingestion/chunk_engine.go as part of the
// internal/parser reorganisation (see .claude/plans
// refactor-history for the move). The exported surface
// (NewChunkEngine / Compile / Execute / Explain) and the
// ChunkPlan shape are unchanged, so callers only need to swap
// the import path.
package chunk

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChunkPlan holds the ordered pipeline operators compiled from a
// DSL blob. Operators are run in declaration order during
// Execute. The Name / Description / Version fields are
// diagnostic-only and copied straight off the DSL for the
// Explain path.
type ChunkPlan struct {
	Operators   []Operator
	Version     string
	Description string
	Name        string
}

// ChunkEngine parses DSL JSON into a ChunkPlan and executes it
// against an input text. The zero value is a usable engine —
// callers should construct via NewChunkEngine for forward
// compatibility.
type ChunkEngine struct{}

// NewChunkEngine returns a ready-to-use engine.
//
// Deprecated: production callers must drive chunk pipelines through
// the typed Run entry point (see pipeline.go); the engine is
// retained as an internal implementation detail of RunDSL /
// ExplainDSL, which the CLI dev-chunk command uses to drive
// user-provided DSL files. Adding new callers is a plan AD-6
// violation (port-rag-flow-pipeline-to-go.md §6.2: ChunkEngine may
// not become the public ingestion-stage runtime).
func NewChunkEngine() *ChunkEngine {
	return &ChunkEngine{}
}

// Compile parses a JSON DSL blob into an ordered operator list.
// On error the returned plan is nil so callers can early-out
// without checking the plan separately.
func (e *ChunkEngine) Compile(dsl string) (*ChunkPlan, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dsl), &parsed); err != nil {
		return nil, fmt.Errorf("compile DSL: %w", err)
	}

	plan := &ChunkPlan{}

	pipelineRaw, ok := parsed["pipeline"].([]interface{})
	if !ok || len(pipelineRaw) == 0 {
		return plan, nil
	}

	if name, ok := parsed["name"].(string); ok {
		plan.Name = name
	} else {
		plan.Name = "No name"
	}
	if desc, ok := parsed["description"].(string); ok {
		plan.Description = desc
	} else {
		plan.Description = "No description"
	}
	if v, ok := parsed["version"].(string); ok {
		plan.Version = v
	} else {
		plan.Version = "1.0"
	}

	for i, operatorRaw := range pipelineRaw {
		operator, ok := operatorRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("pipeline[%d]: expected object", i)
		}

		operatorName, _ := operator["operator"].(string)
		var (
			op  Operator
			err error
		)
		switch operatorName {
		case "preprocess":
			op, err = NewPreprocessOperator(operator)
			if err != nil {
				return nil, fmt.Errorf("create preprocess operator[%d]: %w", i, err)
			}
		case "split":
			op, err = NewSplitOperator(operator)
			if err != nil {
				return nil, fmt.Errorf("create split operator[%d]: %w", i, err)
			}
		case "postprocess":
			op, err = NewPostprocessOperator(operator)
			if err != nil {
				return nil, fmt.Errorf("create postprocess operator[%d]: %w", i, err)
			}
		default:
			return nil, fmt.Errorf("pipeline[%d]: unknown operator %s", i, operatorName)
		}
		delete(operator, "operator")
		plan.Operators = append(plan.Operators, op)
	}

	return plan, nil
}

// Execute runs the plan's operators in order against `text` and
// returns the resulting ChunkContext. Prepare / Execute / Finish
// are called on every operator; a non-nil error short-circuits
// the pipeline.
func (e *ChunkEngine) Execute(plan *ChunkPlan, text string) (*ChunkContext, error) {
	chunkContext := &ChunkContext{Origin: text}

	for i, op := range plan.Operators {
		if err := op.Prepare(chunkContext); err != nil {
			return nil, fmt.Errorf("re-prepare operator[%d]: %w", i, err)
		}
		if err := op.Execute(chunkContext); err != nil {
			return nil, fmt.Errorf("execute operator[%d]: %w", i, err)
		}
		if err := op.Finish(chunkContext); err != nil {
			return nil, fmt.Errorf("finish operator[%d]: %w", i, err)
		}
	}

	return chunkContext, nil
}

// Explain renders a human-readable description of a plan. The
// output is a multi-line string suitable for logging or a
// debug-print call; the per-operator String() method is the
// authoritative description of each stage.
func (e *ChunkEngine) Explain(plan *ChunkPlan) (string, error) {
	var buf strings.Builder
	buf.WriteString("Chunk Pipeline Plan:\n")
	for i, op := range plan.Operators {
		buf.WriteString(fmt.Sprintf("  [%d] %s\n", i, op.String()))
	}
	return buf.String(), nil
}
