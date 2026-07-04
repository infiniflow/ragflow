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
	"testing"

	"ragflow/internal/agent/runtime"
)

type mockCanvasStage struct {
	output map[string]any
	called bool
}

func (m *mockCanvasStage) Invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	m.called = true
	out := cloneMapOrEmpty(inputs)
	for k, v := range m.output {
		out[k] = v
	}
	return out, nil
}

func (m *mockCanvasStage) Parallelism() int           { return 1 }
func (m *mockCanvasStage) Inputs() map[string]string  { return map[string]string{"name": "string"} }
func (m *mockCanvasStage) Outputs() map[string]string { return map[string]string{"output": "any"} }

func TestPipelineRunHappyPath(t *testing.T) {
	stageA := &mockCanvasStage{output: map[string]any{"a": 1}}
	stageB := &mockCanvasStage{output: map[string]any{"b": 2}}

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
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["a"]},
				"a": {"obj": {"component_name": "`+nameA+`", "params": {}}, "upstream": ["begin"], "downstream": ["b"]},
				"b": {"obj": {"component_name": "`+nameB+`", "params": {}}, "upstream": ["a"]}
			},
			"path": ["begin", "a", "b"],
			"graph": {"nodes": []}
		}
	}`), "task-canvas-happy")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	out, err := pipe.Run(context.Background(), map[string]any{"name": "doc-canvas"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !stageA.called || !stageB.called {
		t.Fatalf("expected both stages to run, got A=%v B=%v", stageA.called, stageB.called)
	}
	if got := out["name"]; got != "doc-canvas" {
		t.Fatalf("name = %v, want doc-canvas", got)
	}
	gotB, ok := out["b"].(map[string]any)
	if !ok {
		t.Fatalf("b = %T, want map[string]any", out["b"])
	}
	if got := gotB["b"]; got != 2 {
		t.Fatalf("b.b = %v, want 2", got)
	}
}

func TestPipelineRunNilPipeline(t *testing.T) {
	var p *Pipeline
	if _, err := p.Run(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil pipeline")
	}
}

func TestPipelineRunStageErrorBubbles(t *testing.T) {
	const name = "p.RunErrStage"
	runtime.MustRegister(name, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return &errCanvasStage{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	pipe, err := NewPipelineFromDSL([]byte(`{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["err"]},
				"err": {"obj": {"component_name": "`+name+`", "params": {}}, "upstream": ["begin"]}
			},
			"path": ["begin", "err"],
			"graph": {"nodes": []}
		}
	}`), "task-canvas-err")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	if _, err := pipe.Run(context.Background(), map[string]any{"name": "x"}); err == nil {
		t.Fatal("expected stage error")
	}
}

func TestNewPipelineFromDSLUnwrapsTemplateDSL(t *testing.T) {
	pipe, err := NewPipelineFromDSL([]byte(`{
		"id": "template-1",
		"title": "template",
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}}
			},
			"path": ["begin"],
			"graph": {"nodes": []}
		}
	}`), "task-template")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	if pipe.canvas == nil {
		t.Fatal("expected decoded canvas")
	}
}

type errCanvasStage struct{}

func (e *errCanvasStage) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, &stageError{Stage: "p.RunErrStage", Reason: "intentional"}
}
func (e *errCanvasStage) Parallelism() int           { return 1 }
func (e *errCanvasStage) Inputs() map[string]string  { return nil }
func (e *errCanvasStage) Outputs() map[string]string { return nil }
