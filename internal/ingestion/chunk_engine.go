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
	Operators []chunk.Operator
}

// ChunkEngine parses DSL JSON into a plan and executes it.
type ChunkEngine struct{}

func NewChunkEngine() *ChunkEngine {
	return &ChunkEngine{}
}

// ---------------------------------------------------------------------------
// DSL JSON model
// ---------------------------------------------------------------------------

type dslPipeline struct {
	Version     string     `json:"version"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Pipeline    []dslStage `json:"pipeline"`
}

type dslStage struct {
	Stage string                 `json:"stage"`
	Body  map[string]interface{} `json:"-"` // everything else
}

// UnmarshalJSON custom unmarshaler for dslStage — captures all keys except "stage"
// into Body.
func (s *dslStage) UnmarshalJSON(data []byte) error {
	raw := make(map[string]interface{})
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if stage, ok := raw["stage"].(string); ok {
		s.Stage = stage
	}
	delete(raw, "stage")
	s.Body = raw
	return nil
}

// ---------------------------------------------------------------------------
// Plan  — parse DSL JSON into an ordered operator list
// ---------------------------------------------------------------------------

func (e *ChunkEngine) Plan(dsl *string) (*ChunkPlan, error) {
	var pipeline dslPipeline
	if err := json.Unmarshal([]byte(*dsl), &pipeline); err != nil {
		return nil, fmt.Errorf("parse DSL: %w", err)
	}

	plan := &ChunkPlan{}

	for _, stage := range pipeline.Pipeline {
		op, err := buildOperator(stage.Stage)
		if err != nil {
			return nil, fmt.Errorf("build operator %q: %w", stage.Stage, err)
		}
		if err := op.Prepare(stage.Body); err != nil {
			return nil, fmt.Errorf("prepare operator %q: %w", stage.Stage, err)
		}
		plan.Operators = append(plan.Operators, op)
	}

	return plan, nil
}

func buildOperator(stage string) (chunk.Operator, error) {
	switch stage {
	case "preprocess":
		return chunk.NewPreprocessOperator(), nil
	case "split":
		return chunk.NewSplitOperator(), nil
	case "postprocess":
		return chunk.NewPostprocessOperator(), nil
	default:
		return nil, fmt.Errorf("unknown stage: %q", stage)
	}
}

// ---------------------------------------------------------------------------
// Execute — run the pipeline operators on input text
// ---------------------------------------------------------------------------

func (e *ChunkEngine) Execute(plan *ChunkPlan, text string) (*chunk.Context, error) {
	ctx := &chunk.Context{Text: text}

	for i, op := range plan.Operators {
		if err := op.Prepare(nil); err != nil {
			return ctx, fmt.Errorf("re-prepare operator[%d]: %w", i, err)
		}
	}
	for i, op := range plan.Operators {
		if err := op.Execute(ctx); err != nil {
			return ctx, fmt.Errorf("execute operator[%d]: %w", i, err)
		}
	}
	for i, op := range plan.Operators {
		if err := op.Finish(); err != nil {
			return ctx, fmt.Errorf("finish operator[%d]: %w", i, err)
		}
	}

	return ctx, nil
}

// ---------------------------------------------------------------------------
// Explain — describe the plan in human-readable form
// ---------------------------------------------------------------------------

func (e *ChunkEngine) Explain(plan *ChunkPlan) (string, error) {
	var buf strings.Builder
	buf.WriteString("Chunk Pipeline Plan:\n")
	for i, op := range plan.Operators {
		buf.WriteString(fmt.Sprintf("  [%d] %T\n", i, op))
	}
	return buf.String(), nil
}
