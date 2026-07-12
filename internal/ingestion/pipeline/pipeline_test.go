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
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
	mu      sync.Mutex
	data    map[string][]byte
	deleted int // number of times Delete was called
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
	s.deleted++
	return nil
}

func (s *memCheckpointStore) deleteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleted
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

// recordingSink captures OnComponentTotal / OnComponentProgress calls so tests
// can assert the pipeline forwards progress to the sink instead of writing
// the DAO layer directly.
type recordingSink struct {
	mu       sync.Mutex
	total    int
	totalSet bool
	events   []ProgressEvent
}

func (r *recordingSink) OnComponentTotal(taskID string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total = total
	r.totalSet = true
}

func (r *recordingSink) OnComponentProgress(ev ProgressEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

// TestPipelineRunForwardsProgressToSink verifies the pipeline reports the
// component-total denominator and each component lifecycle event to the
// injected ProgressSink, and carries task/document/total context on every
// event so the sink needs no canvas knowledge.
func TestPipelineRunForwardsProgressToSink(t *testing.T) {
	stageA := &mockCanvasStage{output: map[string]any{"a": 1}}
	stageB := &mockCanvasStage{output: map[string]any{"b": 2}}
	const (
		nameA = "p.SinkStageA"
		nameB = "p.SinkStageB"
	)
	runtime.MustRegister(nameA, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageA, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(nameB, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stageB, nil },
		runtime.Metadata{Version: "1.0.0"})

	sink := &recordingSink{}
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
	}`), "task-sink", WithProgressSink(sink), WithDocumentID("doc-sink"))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	if _, err := pipe.Run(context.Background(), map[string]any{"name": "doc-sink"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if !sink.totalSet || sink.total != 3 {
		t.Fatalf("OnComponentTotal = (%d, set=%v), want 3", sink.total, sink.totalSet)
	}
	if len(sink.events) == 0 {
		t.Fatal("expected progress events, got none")
	}
	seen := map[string]bool{}
	for _, ev := range sink.events {
		if ev.TaskID != "task-sink" {
			t.Fatalf("event TaskID = %q, want task-sink", ev.TaskID)
		}
		if ev.DocumentID != "doc-sink" {
			t.Fatalf("event DocumentID = %q, want doc-sink", ev.DocumentID)
		}
		if ev.Total != 3 {
			t.Fatalf("event Total = %d, want 3", ev.Total)
		}
		seen[ev.Component] = true
	}
	for _, want := range []string{"a", "b"} {
		if !seen[want] {
			t.Fatalf("expected progress event for component %q, seen=%v", want, seen)
		}
	}
}

// =============================================================================
// cleanupCheckpoint — direct unit test
// =============================================================================

func TestCleanupCheckpoint_DeletesStoreAndClearsTracker(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	store := newMemCheckpointStore()
	if err := store.Set(context.Background(), "cp-1", []byte("data")); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	tracker := canvas.NewRunTrackerWithClient(client, time.Hour)
	if err := tracker.AttachInterrupt(context.Background(), "cp-1", "interrupt-1"); err != nil {
		t.Fatalf("AttachInterrupt: %v", err)
	}

	p := &Pipeline{}
	p.cleanupCheckpoint(context.Background(), store, tracker, "cp-1")

	if store.deleteCount() != 1 {
		t.Fatalf("store.Delete was not called")
	}
	id, ok, err := tracker.GetInterruptID(context.Background(), "cp-1")
	if err != nil {
		t.Fatalf("GetInterruptID: %v", err)
	}
	if ok && id != "" {
		t.Fatalf("interrupt id should be cleared, got %q", id)
	}
}

// =============================================================================
// runPlain — tracker integration with miniredis
// =============================================================================

func TestRunPlain_WithTracker_Success(t *testing.T) {
	stage := &mockCanvasStage{output: map[string]any{"result": "ok"}}
	const name = "p.RunPlainSuccess"
	runtime.MustRegister(name, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return stage, nil },
		runtime.Metadata{Version: "1.0.0"})

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	tracker := canvas.NewRunTrackerWithClient(client, time.Hour)

	pipe, err := NewPipelineFromDSL([]byte(`{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["a"]},
				"a": {"obj": {"component_name": "`+name+`", "params": {}}, "upstream": ["begin"]}
			},
			"path": ["begin", "a"],
			"graph": {"nodes": []}
		}
	}`), "task-tracker-ok", WithRunTracker(tracker))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	_, err = pipe.Run(context.Background(), map[string]any{"name": "doc"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunPlain_WithTracker_Error(t *testing.T) {
	const name = "p.RunPlainErr"
	runtime.MustRegister(name, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return &errCanvasStage{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	tracker := canvas.NewRunTrackerWithClient(client, time.Hour)

	pipe, err := NewPipelineFromDSL([]byte(`{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["err"]},
				"err": {"obj": {"component_name": "`+name+`", "params": {}}, "upstream": ["begin"]}
			},
			"path": ["begin", "err"],
			"graph": {"nodes": []}
		}
	}`), "task-tracker-err", WithRunTracker(tracker))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	_, err = pipe.Run(context.Background(), map[string]any{"name": "doc"})
	if err == nil {
		t.Fatal("expected stage error, got nil")
	}
}
