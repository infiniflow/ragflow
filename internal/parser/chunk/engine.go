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

// Chunk DSL helpers compile a JSON operator pipeline and execute it
// against text. The supported DSL shape is:
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
package chunk

import (
	"encoding/json"
	"fmt"
	"strings"
)

// chunkPlan holds the ordered pipeline operators compiled from a
// DSL blob.
type chunkPlan struct {
	Operators   []Operator
	Version     string
	Description string
	Name        string
}

func compileDSL(dsl string) (*chunkPlan, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dsl), &parsed); err != nil {
		return nil, fmt.Errorf("compile DSL: %w", err)
	}

	plan := &chunkPlan{}

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

func executePlan(plan *chunkPlan, text string) (*ChunkContext, error) {
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

func explainPlan(plan *chunkPlan) (string, error) {
	var buf strings.Builder
	buf.WriteString("Chunk Pipeline Plan:\n")
	for i, op := range plan.Operators {
		buf.WriteString(fmt.Sprintf("  [%d] %s\n", i, op.String()))
	}
	return buf.String(), nil
}
