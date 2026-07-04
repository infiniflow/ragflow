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

package pipeline

import (
	"encoding/json"
	"fmt"
	"time"

	"ragflow/internal/agent/canvas"
)

// Pipeline is a compiled ingestion canvas plus task-scoped metadata.
type Pipeline struct {
	taskID string
	canvas *canvas.Canvas
}

// NewPipelineFromDSL compiles the canonical ingestion canvas DSL.
// It accepts either the inner canvas DSL or the template wrapper whose
// top-level `dsl` field carries that canvas.
func NewPipelineFromDSL(dsl []byte, taskID string) (*Pipeline, error) {
	var raw map[string]any
	if err := json.Unmarshal(dsl, &raw); err != nil {
		return nil, fmt.Errorf("pipeline: decode DSL: %w", err)
	}
	canvasDSL, err := unwrapCanvasDSL(raw)
	if err != nil {
		return nil, err
	}
	cnv, err := canvas.DecodeFromDSL(canvasDSL)
	if err != nil {
		return nil, fmt.Errorf("pipeline: decode canvas DSL: %w", err)
	}
	return &Pipeline{
		taskID: taskID,
		canvas: cnv,
	}, nil
}

func unwrapCanvasDSL(raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, errNilDSL
	}
	if rawDSL, ok := raw["dsl"]; ok {
		canvasDSL, ok := rawDSL.(map[string]any)
		if !ok || len(canvasDSL) == 0 {
			return nil, errNilDSL
		}
		return canvasDSL, nil
	}
	return raw, nil
}

func mergeInto(dst, src map[string]any) map[string]any {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneMapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stageTimeout() time.Duration {
	return defaultStageTimeout
}

var defaultStageTimeout = 60 * time.Second
