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
	"errors"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
)

type mockCanvasStage struct {
	output map[string]any
	called bool
	calls  int
}

func (m *mockCanvasStage) Invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	m.called = true
	m.calls++
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

type factorySentinelStage struct {
	marker string
}

func (s *factorySentinelStage) Invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	out := cloneMapOrEmpty(inputs)
	out["marker"] = s.marker
	return out, nil
}

// memCheckpointStore is a thread-safe in-memory canvas.CheckPointStore used
// to exercise the resumable run path without Redis.
type memCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemCheckpointStore() *memCheckpointStore {
	return &memCheckpointStore{data: map[string][]byte{}}
}

func (s *memCheckpointStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[id]
	return v, ok, nil
}

func (s *memCheckpointStore) Set(_ context.Context, id string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	s.data[id] = cp
	return nil
}

func (s *memCheckpointStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
	return nil
}

// TestPipelineRun_InstanceFactoryOverridesDefaultFactory verifies that a
// pipeline-scoped component factory can provide task-specific components.
func TestPipelineRun_InstanceFactoryOverridesDefaultFactory(t *testing.T) {
	pipe, err := NewPipelineFromDSL([]byte(`{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["stage"]},
				"stage": {"obj": {"component_name": "custom-stage", "params": {}}, "upstream": ["begin"]}
			},
			"path": ["begin", "stage"],
			"graph": {"nodes": []}
		}
	}`), "task-instance-factory")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	pipe.WithComponentFactory(func(_ string, _ map[string]any) (runtime.Component, error) {
		return &factorySentinelStage{marker: "instance"}, nil
	})

	out, err := pipe.Run(context.Background(), map[string]any{"name": "doc"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	stage, ok := out["stage"].(map[string]any)
	if !ok {
		t.Fatalf("stage = %T, want map[string]any", out["stage"])
	}
	if got := stage["marker"]; got != "instance" {
		t.Fatalf("stage.marker = %v, want instance", got)
	}
}

func TestPipelineRun_TaskScopedFactoriesDoNotLeakAcrossConcurrentPipelines(t *testing.T) {
	origFactory := runtime.DefaultFactory()
	runtime.SetDefaultFactory(func(_ string, _ map[string]any) (runtime.Component, error) {
		return &factorySentinelStage{marker: "default"}, nil
	})
	defer runtime.SetDefaultFactory(origFactory)

	newPipe := func(taskID string) *Pipeline {
		pipe, err := NewPipelineFromDSL([]byte(`{
			"dsl": {
				"components": {
					"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["stage"]},
					"stage": {"obj": {"component_name": "custom-stage", "params": {}}, "upstream": ["begin"]}
				},
				"path": ["begin", "stage"],
				"graph": {"nodes": []}
			}
		}`), taskID)
		if err != nil {
			t.Fatalf("NewPipelineFromDSL(%s): %v", taskID, err)
		}
		return pipe
	}

	pipeA := newPipe("task-A")
	pipeB := newPipe("task-B")
	pipeA.WithComponentFactory(func(_ string, _ map[string]any) (runtime.Component, error) {
		return &factorySentinelStage{marker: "A"}, nil
	})
	pipeB.WithComponentFactory(func(_ string, _ map[string]any) (runtime.Component, error) {
		return &factorySentinelStage{marker: "B"}, nil
	})

	var wg sync.WaitGroup
	type result struct {
		marker string
		err    error
	}
	results := make(chan result, 2)
	run := func(pipe *Pipeline) {
		defer wg.Done()
		out, err := pipe.Run(context.Background(), map[string]any{"name": "doc"})
		if err != nil {
			results <- result{err: err}
			return
		}
		stage, ok := out["stage"].(map[string]any)
		if !ok {
			results <- result{err: fmt.Errorf("stage = %T", out["stage"])}
			return
		}
		results <- result{marker: stage["marker"].(string)}
	}
	wg.Add(2)
	go run(pipeA)
	go run(pipeB)
	wg.Wait()
	close(results)

	got := map[string]int{}
	for res := range results {
		if res.err != nil {
			t.Fatalf("Run: %v", res.err)
		}
		got[res.marker]++
	}
	if got["A"] != 1 || got["B"] != 1 {
		t.Fatalf("markers = %#v, want one A and one B", got)
	}
}

func TestPipelineRunResumableAutoResumes(t *testing.T) {
	stageA := &mockCanvasStage{output: map[string]any{"a": 1}}
	stageB := &mockCanvasStage{output: map[string]any{"b": 2}}

	const (
		nameA = "p.ResumeStageA"
		nameB = "p.ResumeStageB"
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
	}`), "task-resume", WithCheckPointStore(newMemCheckpointStore()))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	out, err := pipe.Run(context.Background(), map[string]any{"name": "doc-resume"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !stageA.called || !stageB.called {
		t.Fatalf("expected both stages to run, got A=%v B=%v", stageA.called, stageB.called)
	}
	// No re-run on resume: each node must execute exactly once.
	if stageA.calls != 1 || stageB.calls != 1 {
		t.Fatalf("expected each stage to run exactly once, got A.calls=%d B.calls=%d", stageA.calls, stageB.calls)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

// TestPipelineRun_RequireResumeRejectsWithoutStore verifies M4 (plan §6.a
// 方案 A): with WithRequireResume set and no checkpoint store resolvable (no
// injected store, no global Redis in unit scope), Run must refuse to start
// and return ErrResumeUnavailable — a clear, distinguishable signal — rather
// than silently degrading to a non-resumable runPlain. The reject fires
// before compile, so the DSL does not need a runnable graph.
func TestPipelineRun_RequireResumeRejectsWithoutStore(t *testing.T) {
	pipe, err := NewPipelineFromDSL([]byte(`{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["a"]},
				"a": {"obj": {"component_name": "p.Docx", "params": {}}, "upstream": ["begin"]}
			},
			"path": ["begin", "a"],
			"graph": {"nodes": []}
		}
	}`), "task-req-resume", WithRequireResume())
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	_, err = pipe.Run(context.Background(), map[string]any{"name": "doc"})
	if !errors.Is(err, ErrResumeUnavailable) {
		t.Fatalf("expected ErrResumeUnavailable, got %v", err)
	}
}
