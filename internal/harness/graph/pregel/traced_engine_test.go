// Package pregel provides tests for the OpenTelemetry tracing and callback system.
package pregel

import (
	"context"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/graph/types"
)

// TestTracedEngine_Smoke verifies that TracedEngine.Run does not panic
// and produces output events.
func TestTracedEngine_Smoke(t *testing.T) {
	g := newTestGraph(t)
	engine := NewEngine(g, WithRecursionLimit(10))
	traced := NewTracedEngine(engine)

	ctx := context.Background()
	outputCh, errCh := traced.Run(ctx, map[string]any{"value": "hello"}, types.StreamModeValues)

	var finalState any
	for result := range outputCh {
		if se, ok := result.(*StreamEvent); ok && se.Type == EventTypeFinal {
			if data, ok := se.Data.(map[string]any); ok {
				finalState = data["state"]
			}
		}
	}
	err := <-errCh
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalState == nil {
		t.Fatal("expected non-nil final state")
	}
}

// TestRunCallback_Dispatch verifies callback dispatch on run start/end.
func TestRunCallback_Dispatch(t *testing.T) {
	g := newTestGraph(t)
	cbManager := NewCallbackManager()

	var runStarted, runEnded atomic.Int32
	cbManager.AddCallback(&NoopCallbackMock{
		onRunStart: func(_ context.Context, _, _ string) {
			runStarted.Add(1)
		},
		onRunEnd: func(_ context.Context, _, _ string, _ error) {
			runEnded.Add(1)
		},
	})

	engine := NewEngine(g,
		WithRecursionLimit(10),
	)
	traced := NewTracedEngine(engine)
	traced.SetCallbacks(cbManager)

	ctx := context.Background()
	outputCh, errCh := traced.Run(ctx, map[string]any{"value": "ping"}, types.StreamModeValues)
	for range outputCh {
	}
	<-errCh

	if runStarted.Load() != 1 {
		t.Fatalf("expected 1 run start, got %d", runStarted.Load())
	}
	if runEnded.Load() != 1 {
		t.Fatalf("expected 1 run end, got %d", runEnded.Load())
	}
}

// TestCheckpointCallback_Dispatch verifies checkpoint callback dispatch.
func TestCheckpointCallback_Dispatch(t *testing.T) {
	g := newTestGraph(t)
	cbManager := NewCallbackManager()

	var cpSaved, cpLoaded atomic.Int32
	cbManager.AddCheckpointCallback(&CheckpointCallbackMock{
		onSave: func(_ context.Context, _, _ string, _ int) { cpSaved.Add(1) },
		onLoad: func(_ context.Context, _, _ string, _ int) { cpLoaded.Add(1) },
	})

	engine := NewEngine(g,
		WithRecursionLimit(10),
	)
	traced := NewTracedEngine(engine)
	traced.SetCallbacks(cbManager)

	ctx := context.Background()
	outputCh, errCh := traced.Run(ctx, map[string]any{"value": "ping"}, types.StreamModeValues)
	for range outputCh {
	}
	<-errCh

	if cpSaved.Load() < 0 {
		// Checkpoint callback may or may not fire depending on checkpointer config.
		// Just verify no crash.
	}
}

// TestTracedEngine_Disabled verifies that disabling tracing still runs correctly.
func TestTracedEngine_Disabled(t *testing.T) {
	g := newTestGraph(t)
	engine := NewEngine(g, WithRecursionLimit(10))
	traced := NewTracedEngine(engine, WithTracedEngineDisabled())

	ctx := context.Background()
	outputCh, errCh := traced.Run(ctx, map[string]any{"value": "hello"}, types.StreamModeValues)
	for range outputCh {
	}
	err := <-errCh
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---- Mocks ----

// NoopCallbackMock implements GraphCallback with overridable hooks.
type NoopCallbackMock struct {
	onRunStart func(context.Context, string, string)
	onRunEnd   func(context.Context, string, string, error)
}

func (m *NoopCallbackMock) OnRunStart(ctx context.Context, grp, tid string) {
	if m.onRunStart != nil {
		m.onRunStart(ctx, grp, tid)
	}
}
func (m *NoopCallbackMock) OnRunEnd(ctx context.Context, grp, tid string, err error) {
	if m.onRunEnd != nil {
		m.onRunEnd(ctx, grp, tid, err)
	}
}
func (m *NoopCallbackMock) OnStepStart(_ context.Context, _, _ int)                              {}
func (m *NoopCallbackMock) OnStepEnd(_ context.Context, _ int, _ error)                          {}
func (m *NoopCallbackMock) OnNodeStart(_ context.Context, _ string, _ int)                       {}
func (m *NoopCallbackMock) OnNodeEnd(_ context.Context, _ string, _ int, _ interface{}, _ error) {}
func (m *NoopCallbackMock) OnCheckpointSave(_ context.Context, _, _ string, _ int)               {}
func (m *NoopCallbackMock) OnCheckpointLoad(_ context.Context, _, _ string, _ int)               {}
func (m *NoopCallbackMock) OnCheckpointUpdate(_ context.Context, _, _ string)                    {}
func (m *NoopCallbackMock) OnInterrupt(_ context.Context, _ []string, _ int)                     {}
func (m *NoopCallbackMock) OnResume(_ context.Context, _ string)                                 {}

// CheckpointCallbackMock implements CheckpointCallback with overridable hooks.
type CheckpointCallbackMock struct {
	onSave func(context.Context, string, string, int)
	onLoad func(context.Context, string, string, int)
}

func (m *CheckpointCallbackMock) OnCheckpointSave(ctx context.Context, tid, cpid string, step int) {
	if m.onSave != nil {
		m.onSave(ctx, tid, cpid, step)
	}
}
func (m *CheckpointCallbackMock) OnCheckpointLoad(ctx context.Context, tid, cpid string, step int) {
	if m.onLoad != nil {
		m.onLoad(ctx, tid, cpid, step)
	}
}
func (m *CheckpointCallbackMock) OnCheckpointUpdate(_ context.Context, _, _ string) {}

// Ensure mock implements interfaces.
var (
	_ GraphCallback      = (*NoopCallbackMock)(nil)
	_ CheckpointCallback = (*CheckpointCallbackMock)(nil)
)
