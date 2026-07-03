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
	"context"
	"fmt"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
)

// Run executes the full ingestion graph described by the canonical DSL.
// There is no pipeline-layer partial resume entry point: execution always
// starts from the graph entry and component-level replay decisions belong to
// the components themselves.
func (p *Pipeline) Run(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline: Run on nil pipeline")
	}
	if p.canvas == nil {
		return nil, fmt.Errorf("pipeline: canvas is nil")
	}
	if runtime.DefaultFactory() == nil {
		runtime.InstallDefaultRegistryFactory()
	}
	if runtime.DefaultFactory() == nil {
		return nil, fmt.Errorf("pipeline: Run: runtime default component factory is not installed")
	}

	compiled, err := canvas.Compile(ctx, p.canvas)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: compile canvas: %w", err)
	}

	runState := canvas.NewCanvasState("", p.taskID)
	runCtx := canvas.WithState(ctx, runState)
	runCtx = canvas.WithComponentTimeoutOverride(runCtx, stageTimeout())

	current := cloneMapOrEmpty(inputs)
	out, err := compiled.Workflow.Invoke(runCtx, current)
	if err != nil {
		return current, fmt.Errorf("pipeline: run canvas workflow: %w", err)
	}
	if out == nil {
		p.recordCanvasCompletion(current)
		_ = p.flushCheckpoint(runCtx, "completed")
		return current, nil
	}
	current = mergeInto(current, out)
	p.recordCanvasCompletion(current)
	_ = p.flushCheckpoint(runCtx, "completed")
	return current, nil
}
