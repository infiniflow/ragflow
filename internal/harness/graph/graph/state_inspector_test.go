// Package graph provides tests for the state inspection API.
package graph

import (
	"context"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/types"
)

// TestGetState_NoCheckpointer verifies GetState returns an error when no checkpointer is configured.
func TestGetState_NoCheckpointer(t *testing.T) {
	b := NewStateGraph(struct{ Value string }{})
	b.AddNode("nop", func(ctx context.Context, state any) (any, error) { return state, nil })
	b.AddEdge("__start__", "nop")
	b.AddEdge("nop", "__end__")
	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	_, err = insp.GetState(context.Background(), types.NewRunnableConfig())
	if err == nil {
		t.Fatal("expected error without checkpointer")
	}
}

// TestGetState_WithCheckpointer verifies GetState returns a snapshot after execution.
func TestGetState_WithCheckpointer(t *testing.T) {
	t.Skip("requires Pregel engine - see pregel/ for equivalent tests")
	b := NewStateGraph(struct {
		Messages []string `harness:"reducer=append"`
	}{})
	b.AddNode("node_a", func(ctx context.Context, state any) (any, error) {
		s := state.(struct{ Messages []string })
		s.Messages = append(s.Messages, "from node_a")
		return s, nil
	})
	b.AddEdge("__start__", "node_a")
	b.AddEdge("node_a", "__end__")

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			"thread_id": "test-get-state-thread",
		},
	}

	// Execute the graph.
	_, err = cg.Invoke(context.Background(), struct{ Messages []string }{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Get state.
	snap, err := insp.GetState(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if snap == nil {
		t.Fatal("GetState returned nil snapshot")
	}
	if len(snap.Values) == 0 {
		t.Fatal("expected non-empty values in snapshot")
	}
}

// TestGetStateHistory_Empty verifies GetStateHistory returns empty for a thread with no checkpoints.
func TestGetStateHistory_Empty(t *testing.T) {
	b := NewStateGraph(struct{ Value string }{})
	b.AddNode("nop", func(ctx context.Context, state any) (any, error) { return state, nil })
	b.AddEdge("__start__", "nop")
	b.AddEdge("nop", "__end__")
	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			"thread_id": "test-history-empty",
		},
	}

	history, err := insp.GetStateHistory(context.Background(), cfg, 10, nil)
	if err != nil {
		t.Fatalf("GetStateHistory: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(history))
	}
}

// TestGetStateHistory_WithData verifies GetStateHistory returns entries after execution.
func TestGetStateHistory_WithData(t *testing.T) {
	type counterState struct {
		Count int `harness:"reducer=add"`
	}
	b := NewStateGraph(counterState{})
	b.AddNode("counter", func(ctx context.Context, state any) (any, error) {
		s := state.(counterState)
		s.Count++
		return s, nil
	})
	b.AddEdge("__start__", "counter")
	b.AddEdge("counter", "__end__")

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			"thread_id": "test-history-data",
		},
	}

	_, err = cg.Invoke(context.Background(), counterState{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	history, err := insp.GetStateHistory(context.Background(), cfg, 10, nil)
	if err != nil {
		t.Fatalf("GetStateHistory: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected at least 1 entry in history")
	}
}

// TestUpdateState verifies UpdateState can inject values at a checkpoint.
func TestUpdateState(t *testing.T) {
	b := NewStateGraph(struct {
		Value string
	}{})
	b.AddNode("echo", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddEdge("__start__", "echo")
	b.AddEdge("echo", "__end__")

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			"thread_id": "test-update-state",
		},
	}

	// Execute once to create a checkpoint.
	_, err = cg.Invoke(context.Background(), struct{ Value string }{Value: "initial"}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Update state.
	update := &StateUpdate{
		Values:   map[string]interface{}{"Value": "updated"},
		AsNode:   "user",
		ThreadID: "test-update-state",
	}
	newCfg, err := insp.UpdateState(context.Background(), cfg, update)
	if err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	if newCfg == nil {
		t.Fatal("UpdateState returned nil config")
	}

	// Verify update was persisted.
	snap, err := insp.GetState(context.Background(), newCfg)
	if err != nil {
		t.Fatalf("GetState after update: %v", err)
	}
	if snap == nil {
		t.Fatal("snap is nil after update")
	}
	if v, ok := snap.Values["Value"]; !ok || v != "updated" {
		t.Fatalf("expected Value=updated, got %v", snap.Values)
	}
}

// TestCompiledStateGraph_Inspection verifies state inspection on CompiledStateGraph.
func TestCompiledStateGraph_Inspection(t *testing.T) {
	b := NewStateGraph(struct{ Value string }{})
	b.AddNode("nop", func(ctx context.Context, state any) (any, error) { return state, nil })
	b.AddEdge("__start__", "nop")
	b.AddEdge("nop", "__end__")
	cg, err := b.Compile(WithCheckpointer(checkpoint.NewMemorySaver()))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	csg := NewCompiledStateGraph(cg)

	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			"thread_id": "test-csg-inspect",
		},
	}

	snap, err := csg.GetState(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompiledStateGraph GetState: %v", err)
	}
	// After initial compile with no run, snap may be nil (no checkpoint yet).
	_ = snap

	history, err := csg.GetStateHistory(context.Background(), cfg, 10, nil)
	if err != nil {
		t.Fatalf("CompiledStateGraph GetStateHistory: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(history))
	}
}

// TestGetState_WithChannels verifies state with various channel types.
func TestGetState_WithChannels(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("counter", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))
	b.AddNode("incr", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 1}, nil
	})
	b.AddEdge("__start__", "incr")
	b.AddEdge("incr", "__end__")

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			"thread_id": "test-channels-state",
		},
	}

	_, err = cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	snap, err := insp.GetState(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if snap == nil {
		t.Fatal("snap is nil")
	}
	if counter, ok := snap.Values["counter"]; ok {
		if cnt, ok := counter.(int); ok && cnt != 1 {
			t.Fatalf("expected counter=1, got %d", cnt)
		}
	}
}
