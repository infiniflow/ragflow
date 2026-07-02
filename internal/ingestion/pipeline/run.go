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
	"ragflow/internal/agent/runtime"
)

// Run drives the pipeline through the canvas compile/run path.
//
// The ingestion PipelineDSL remains the source of truth for stage
// order. Run always adapts the full DSL into a canvas.Canvas,
// compiles it, invokes the compiled workflow from the graph entry,
// then reconstructs the legacy accumulated output map from the
// per-node CanvasState outputs so checkpoint semantics remain
// unchanged.
func (p *Pipeline) Run(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline: Run on nil pipeline")
	}

	current := cloneMapOrEmpty(inputs)
	if len(p.stages) == 0 {
		return current, nil
	}
	if runtime.DefaultFactory() == nil {
		runtime.InstallDefaultRegistryFactory()
	}
	if runtime.DefaultFactory() == nil {
		return nil, fmt.Errorf("pipeline: Run: runtime default component factory is not installed")
	}

	cnv, err := PipelineToCanvas(&p.dsl)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: adapt DSL to canvas: %w", err)
	}

	compiled, err := canvas.Compile(ctx, cnv)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: compile canvas: %w", err)
	}

	runState := canvas.NewCanvasState("", p.taskID)
	runCtx := canvas.WithState(ctx, runState)
	if _, err := compiled.Workflow.Invoke(runCtx, current); err != nil {
		failedStage := p.firstUnfinishedStage(runState.Snapshot())
		if failedStage != nil {
			p.recordFailure(failedStage.Name, err)
		}
		_ = p.flushCheckpoint(ctx, "")
		return current, fmt.Errorf("pipeline: stage %q: %w", failedStageName(failedStage), err)
	}

	outputs := runState.Snapshot()
	completed := make([]completedStage, 0, len(p.stages))
	for i := 0; i < len(p.stages); i++ {
		step := p.stages[i]
		stageOut, ok := outputs[step.Name]
		if !ok {
			return current, fmt.Errorf("pipeline: stage %q: canvas run completed without node output", step.Name)
		}
		fingerprint := ComputeStageFingerprint(current, step.Params)
		current = mergeInto(current, cloneMap(stageOut))
		completed = append(completed, completedStage{
			step:        step,
			out:         cloneMap(current),
			fingerprint: fingerprint,
		})
	}

	for _, c := range completed {
		statuses := stageStatuses(c.step)
		if isChunkerStage(c.step.Name) {
			if err := p.persistChunkerBoundary(ctx, c.step.Name, c.out); err != nil {
				return current, fmt.Errorf("pipeline: persist chunks.jsonl after %q: %w", c.step.Name, err)
			}
		}
		if c.step.Name == stageFile {
			p.recordFileRef(c.out)
		}
		p.recordSuccess(c.step.Name, statuses)
		p.recordStageDoneWithKey(IdempotencyKey{
			TaskID:           p.taskID,
			PipelineVersion:  p.pipelineVersion,
			ComponentName:    c.step.Name,
			ComponentVersion: c.step.ComponentVersion,
			InputFingerprint: c.fingerprint,
		})
		if err := p.flushCheckpoint(ctx, c.step.Name); err != nil {
			return current, fmt.Errorf("pipeline: persist checkpoint after %q: %w", c.step.Name, err)
		}
	}

	return current, nil
}

type completedStage struct {
	step        ComponentStep
	out         map[string]any
	fingerprint string
}

func (p *Pipeline) firstUnfinishedStage(outputs map[string]map[string]any) *ComponentStep {
	for i := 0; i < len(p.stages); i++ {
		step := &p.stages[i]
		if _, ok := outputs[step.Name]; !ok {
			return step
		}
	}
	if len(p.stages) > 0 {
		return &p.stages[0]
	}
	return nil
}

func failedStageName(step *ComponentStep) string {
	if step == nil {
		return "unknown"
	}
	return step.Name
}

func stageStatuses(step ComponentStep) []GoroutineStatus {
	n := step.Parallelism
	if n < 1 {
		n = 1
	}
	statuses := make([]GoroutineStatus, 0, n)
	for i := 0; i < n; i++ {
		statuses = append(statuses, GoroutineStatus{Goroutine: i, Status: "done"})
	}
	return statuses
}

func cloneMapOrEmpty(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}
	return out
}
