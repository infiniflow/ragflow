//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ingestion

import (
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/ingestion/chunk"
)

/*
 DSL reference — see comment block above for the full JSON structure.

 Pipeline stages:
   1. "preprocess"  → chunk.PreprocessOperator
   2. "split"       → chunk.SplitOperator
   3. "postprocess" → chunk.PostprocessOperator
*/

// ChunkPlan holds the ordered pipeline operators.
type ChunkPlan struct {
	Operators   []chunk.Operator
	Version     string
	Description string
	Name        string
}

// ChunkEngine parses DSL JSON into a plan and executes it.
type ChunkEngine struct{}

func NewChunkEngine() *ChunkEngine {
	return &ChunkEngine{}
}

// ---------------------------------------------------------------------------
// Compile  — compile DSL JSON into an ordered operator list
// ---------------------------------------------------------------------------

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

	plan.Name, ok = parsed["name"].(string)
	if !ok {
		plan.Name = "No name"
	}
	plan.Description, ok = parsed["description"].(string)
	if !ok {
		plan.Description = "No description"
	}
	plan.Version, ok = parsed["version"].(string)
	if !ok {
		plan.Version = "1.0"
	}

	for i, operatorRaw := range pipelineRaw {
		var operator map[string]interface{}
		operator, ok = operatorRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("pipeline[%d]: expected object", i)
		}

		var op chunk.Operator
		var err error
		operatorName, _ := operator["operator"].(string)
		switch operatorName {
		case "preprocess":
			op, err = chunk.NewPreprocessOperator(operator)
			if err != nil {
				return nil, fmt.Errorf("create preprocess operator[%d]: %w", i, err)
			}
		case "split":
			op, err = chunk.NewSplitOperator(operator)
			if err != nil {
				return nil, fmt.Errorf("create split operator[%d]: %w", i, err)
			}
		case "postprocess":
			op, err = chunk.NewPostprocessOperator(operator)
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

// ---------------------------------------------------------------------------
// Execute — run the pipeline operators on input text
// ---------------------------------------------------------------------------

func (e *ChunkEngine) Execute(plan *ChunkPlan, text string) (*chunk.ChunkContext, error) {
	chunkContext := &chunk.ChunkContext{Origin: text}

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

// ---------------------------------------------------------------------------
// Explain — describe the plan in human-readable form
// ---------------------------------------------------------------------------

func (e *ChunkEngine) Explain(plan *ChunkPlan) (string, error) {
	var buf strings.Builder
	buf.WriteString("Chunk Pipeline Plan:\n")
	for i, op := range plan.Operators {
		buf.WriteString(fmt.Sprintf("  [%d] %s\n", i, op.String()))
	}
	return buf.String(), nil
}
