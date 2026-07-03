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
	return p.runFromStage(ctx, inputs, 0)
}

// RunFromCheckpoint resumes the pipeline from the given materialized stage
// boundary. Unlike Run, this entry point is allowed to start from the middle
// of the linear stage list because RestoreFromCheckpoint has already proven
// the upstream boundary exists and rehydrated the required inputs.
func (p *Pipeline) RunFromCheckpoint(ctx context.Context, inputs map[string]any, startAt int) (map[string]any, error) {
	return p.runFromStage(ctx, inputs, startAt)
}

func (p *Pipeline) runFromStage(ctx context.Context, inputs map[string]any, startAt int) (map[string]any, error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline: Run on nil pipeline")
	}
	if startAt < 0 {
		startAt = 0
	}
	if startAt > len(p.stages) {
		startAt = len(p.stages)
	}

	current := cloneMapOrEmpty(inputs)
	if len(p.stages) == 0 || startAt == len(p.stages) {
		return current, nil
	}
	if runtime.DefaultFactory() == nil {
		runtime.InstallDefaultRegistryFactory()
	}
	if runtime.DefaultFactory() == nil {
		return nil, fmt.Errorf("pipeline: Run: runtime default component factory is not installed")
	}

	runDSL := p.sliceDSL(startAt)
	cnv, err := PipelineToCanvas(runDSL)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: adapt DSL to canvas: %w", err)
	}

	compiled, err := canvas.Compile(ctx, cnv)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: compile canvas: %w", err)
	}

	runState := canvas.NewCanvasState("", p.taskID)
	runCtx := canvas.WithState(ctx, runState)
	runCtx = canvas.WithComponentTimeoutOverride(runCtx, stageTimeout())
	if _, err := compiled.Workflow.Invoke(runCtx, current); err != nil {
		failedStage := p.firstUnfinishedStage(runState.Snapshot(), startAt)
		if failedStage != nil {
			p.recordFailure(failedStage.Name, err)
		}
		_ = p.flushCheckpoint(ctx, "")
		return current, fmt.Errorf("pipeline: stage %q: %w", failedStageName(failedStage), err)
	}

	outputs := runState.Snapshot()
	completed := make([]completedStage, 0, len(p.stages)-startAt)
	for i := startAt; i < len(p.stages); i++ {
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

func (p *Pipeline) firstUnfinishedStage(outputs map[string]map[string]any, startAt int) *ComponentStep {
	for i := startAt; i < len(p.stages); i++ {
		step := &p.stages[i]
		if _, ok := outputs[step.Name]; !ok {
			return step
		}
	}
	if startAt >= 0 && startAt < len(p.stages) {
		return &p.stages[startAt]
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

func (p *Pipeline) sliceDSL(startAt int) *PipelineDSL {
	stages := make([]StageDSL, 0, len(p.dsl.Stages)-startAt)
	for i := startAt; i < len(p.dsl.Stages); i++ {
		stage := p.dsl.Stages[i]
		stages = append(stages, StageDSL{
			Type:   stage.Type,
			Params: cloneMap(stage.Params),
		})
	}
	return &PipelineDSL{
		Version:     p.dsl.Version,
		Name:        p.dsl.Name,
		Description: p.dsl.Description,
		StageCount:  len(stages),
		Stages:      stages,
	}
}

func cloneMapOrEmpty(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}
	return out
}
