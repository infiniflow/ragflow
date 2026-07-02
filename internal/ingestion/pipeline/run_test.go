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

// Slice 4 deferred-2 tests for port-rag-flow-pipeline-to-go.md.
// Pins the canvas-driven pipeline entry point (Pipeline.p.Run)
// against a small 2-stage chain using mock components registered
// under runtime.CategoryIngestion.

package pipeline

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

// mockCanvasStage is a minimal runtime.Component that records
// the call and emits a synthetic output. Used by the p.Run
// tests below.
type mockCanvasStage struct {
	name   string
	output map[string]any
	called bool
}

func (m *mockCanvasStage) Invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	m.called = true
	out := map[string]any{"name": inputs["name"]}
	for k, v := range m.output {
		out[k] = v
	}
	return out, nil
}

func (m *mockCanvasStage) Parallelism() int { return 1 }
func (m *mockCanvasStage) Inputs() map[string]string {
	return map[string]string{"name": "string"}
}
func (m *mockCanvasStage) Outputs() map[string]string {
	return map[string]string{"output": "any"}
}

// TestPipeline_TestPipeline_Run_HappyPath pins the canvas-driven Run
// path. Two mock components are registered under
// CategoryIngestion; the pipeline drives them through the
// canvas runner and the per-stage outputs accumulate into the
// final state map.
func TestPipeline_TestPipeline_Run_HappyPath(t *testing.T) {
	stageA := &mockCanvasStage{name: "StageA", output: map[string]any{"a": 1}}
	stageB := &mockCanvasStage{name: "StageB", output: map[string]any{"b": 2}}

	const (
		nameA = "p.RunStageA"
		nameB = "p.RunStageB"
	)
	runtime.MustRegister(nameA, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageA, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(nameB, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageB, nil },
		runtime.Metadata{Version: "1.0.0"})

	pipe, err := NewPipelineFromDSL([]byte(`{
		"version": "1",
		"name": "run-canvas-test",
		"stage_count": 2,
		"stages": [
			{"type": "`+nameA+`", "params": {}},
			{"type": "`+nameB+`", "params": {}}
		]
	}`), "task-canvas-happy", nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	out, err := pipe.Run(context.Background(), map[string]any{
		"name": "doc-canvas",
	})
	if err != nil {
		t.Fatalf("p.Run: %v", err)
	}
	if !stageA.called {
		t.Error("StageA not invoked")
	}
	if !stageB.called {
		t.Error("StageB not invoked")
	}
	if got, want := out["name"], "doc-canvas"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if got, want := out["b"], 2; got != want {
		t.Errorf("b = %v, want %v (StageB output)", got, want)
	}
}

// TestPipeline_TestPipeline_Run_FullGraphAlwaysRuns pins the
// canvas-entry contract: Pipeline.Run always executes the full graph
// from the graph entry.
func TestPipeline_TestPipeline_Run_FullGraphAlwaysRuns(t *testing.T) {
	stageA := &mockCanvasStage{name: "StageA"}
	stageB := &mockCanvasStage{name: "StageB"}

	const (
		nameA = "p.RunStartAtStageA"
		nameB = "p.RunStartAtStageB"
	)
	runtime.MustRegister(nameA, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageA, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(nameB, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageB, nil },
		runtime.Metadata{Version: "1.0.0"})

	pipe, _ := NewPipelineFromDSL([]byte(`{
		"version": "1",
		"stage_count": 2,
		"stages": [
			{"type": "`+nameA+`", "params": {}},
			{"type": "`+nameB+`", "params": {}}
		]
	}`), "task-canvas-skip", nil, nil, nil)

	_, err := pipe.Run(context.Background(), map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("p.Run: %v", err)
	}
	if !stageA.called {
		t.Error("StageA should be invoked")
	}
	if !stageB.called {
		t.Error("StageB should be invoked")
	}
}

// TestPipeline_TestPipeline_Run_NilPipeline pins the nil receiver
// rejection. p.Run must not panic on a nil pipeline.
func TestPipeline_TestPipeline_Run_NilPipeline(t *testing.T) {
	var p *Pipeline
	_, err := p.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("nil pipeline: want error, got nil")
	}
}

// TestPipeline_TestPipeline_Run_StageErrorBubbles pins the per-stage
// error propagation. A stage that returns an error surfaces as
// a wrapped p.Run error.
func TestPipeline_TestPipeline_Run_StageErrorBubbles(t *testing.T) {
	stageErr := &errCanvasStage{}
	const name = "p.RunErrStage"
	runtime.MustRegister(name, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageErr, nil },
		runtime.Metadata{Version: "1.0.0"})

	pipe, _ := NewPipelineFromDSL([]byte(`{
		"version": "1",
		"stage_count": 1,
		"stages": [{"type": "`+name+`", "params": {}}]
	}`), "task-canvas-err", nil, nil, nil)

	_, err := pipe.Run(context.Background(), map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("stage error: want error, got nil")
	}
}

type errCanvasStage struct{}

func (e *errCanvasStage) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, &stageError{Stage: "p.RunErrStage", Reason: "intentional"}
}
func (e *errCanvasStage) Parallelism() int           { return 1 }
func (e *errCanvasStage) Inputs() map[string]string  { return nil }
func (e *errCanvasStage) Outputs() map[string]string { return nil }
