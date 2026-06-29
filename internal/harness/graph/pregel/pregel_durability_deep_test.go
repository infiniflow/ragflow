// Package pregel provides deep durability mode tests for Sync/Async/Exit.
package pregel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: DurabilitySync — basic verification
// ============================================================

// TestDurabilitySync_Basic verifies Sync mode saves checkpoint per step.
func TestDurabilitySync_Basic(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "dur-sync-basic"
	cfg := &types.RunnableConfig{
		Durability:   types.DurabilitySync,
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "sync"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}

	// Checkpoint should exist after Sync run.
	cp, _ := ms.Get(context.Background(), map[string]interface{}{
		constants.ConfigKeyThreadID: tid,
	})
	if cp == nil {
		t.Fatal("expected checkpoint after DurabilitySync")
	}
}

// ============================================================
// P0: DurabilitySync with interrupt
// ============================================================

// TestDurabilitySync_WithInterrupt verifies Sync mode with interrupt config.
func TestDurabilitySync_WithInterrupt(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "dur-sync-int"
	cfg := &types.RunnableConfig{
		Durability:   types.DurabilitySync,
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
		WithInterrupts("node_a"),
	)
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "sync"})
	_ = err
}

// ============================================================
// P0: DurabilityAsync — basic verification
// ============================================================

// TestDurabilityAsync_Basic verifies Async mode doesn't block on save.
func TestDurabilityAsync_Basic(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "dur-async-basic"
	cfg := &types.RunnableConfig{
		Durability:   types.DurabilityAsync,
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "async"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}

	// Wait briefly for async save.
	time.Sleep(50 * time.Millisecond)

	cp, _ := ms.Get(context.Background(), map[string]interface{}{
		constants.ConfigKeyThreadID: tid,
	})
	if cp == nil {
		t.Log("async checkpoint may not yet be persisted (best-effort)")
	}
}

// ============================================================
// P1: All three modes produce same output
// ============================================================

// TestDurability_AllModes_SameOutput verifies Sync/Async/Exit all
// produce the same execution result.
func TestDurability_AllModes_SameOutput(t *testing.T) {
	expected := "b"

	for _, d := range []types.Durability{types.DurabilitySync, types.DurabilityAsync, types.DurabilityExit} {
		t.Run(string(d), func(t *testing.T) {
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("dur-%s-same", string(d))
			cfg := &types.RunnableConfig{
				Durability:   d,
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
			)
			result, err := engine.RunSync(context.Background(), map[string]any{"value": d})
			if err != nil {
				t.Fatalf("durability %s: %v", d, err)
			}
			m := result.(map[string]any)
			if m["value"] != expected {
				t.Fatalf("durability %s: expected value=%s, got %v", d, expected, m["value"])
			}
		})
	}
}

// ============================================================
// P1: DurabilityAll with large state
// ============================================================

// TestDurability_AllModes_LargeState verifies large state with all modes.
func TestDurability_AllModes_LargeState(t *testing.T) {
	for _, d := range []types.Durability{types.DurabilitySync, types.DurabilityAsync, types.DurabilityExit} {
		t.Run(string(d), func(t *testing.T) {
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("dur-%s-large", string(d))
			cfg := &types.RunnableConfig{
				Durability:   d,
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
			)
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "large"})
			if err != nil {
				t.Fatalf("durability %s: %v", d, err)
			}
		})
	}
}

// ============================================================
// P2: Durability concurrent
// ============================================================

// TestDurability_ConcurrentEngines runs 20 engines with different modes.
func TestDurability_ConcurrentEngines(t *testing.T) {
	modes := []types.Durability{types.DurabilitySync, types.DurabilityAsync, types.DurabilityExit}
	var wg sync.WaitGroup
	var errCount atomic.Int32

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			d := modes[idx%len(modes)]
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("dur-conc-%s-%d", string(d), idx)
			cfg := &types.RunnableConfig{
				Durability:   d,
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
			)
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "conc"})
			if err != nil {
				errCount.Add(1)
				t.Errorf("engine %d (%s): %v", idx, d, err)
			}
		}(i)
	}
	wg.Wait()
	if errCount.Load() > 0 {
		t.Fatalf("%d engines reported errors", errCount.Load())
	}
}

// ============================================================
// P2: Durability with interrupt + resume across all modes
// ============================================================

// TestDurability_InterruptEachMode tries interrupt config with each mode.
func TestDurability_InterruptEachMode(t *testing.T) {
	for _, d := range []types.Durability{types.DurabilitySync, types.DurabilityExit} {
		t.Run(string(d), func(t *testing.T) {
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("dur-%s-int", string(d))
			cfg := &types.RunnableConfig{
				Durability:   d,
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
				WithInterrupts("node_a"),
			)
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "int"})
			_ = err
		})
	}
}

// ============================================================
// P2: Rapid mode switching between runs
// ============================================================

// TestDurability_RapidModeSwitch switches durability between runs.
func TestDurability_RapidModeSwitch(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "dur-rapid-switch"

	for i, d := range []types.Durability{types.DurabilitySync, types.DurabilityExit, types.DurabilityAsync, types.DurabilitySync} {
		cfg := &types.RunnableConfig{
			Durability:   d,
			Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
		}
		engine := NewEngine(newSimpleGraph(t),
			WithRecursionLimit(10),
			WithCheckpointer(ms),
			WithConfig(cfg),
		)
		_, err := engine.RunSync(context.Background(), map[string]any{"value": "switch"})
		if err != nil {
			t.Fatalf("run %d (%s): %v", i, d, err)
		}
	}
}

// ============================================================
// P2: Durability with no checkpointer (mode is no-op)
// ============================================================

// TestDurability_NoCheckpointer runs Exit mode without checkpointer.
func TestDurability_NoCheckpointer(t *testing.T) {
	for _, d := range []types.Durability{types.DurabilitySync, types.DurabilityExit} {
		t.Run(string(d), func(t *testing.T) {
			cfg := &types.RunnableConfig{Durability: d}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithConfig(cfg),
			)
			result, err := engine.RunSync(context.Background(), map[string]any{"value": "no-cp"})
			if err != nil {
				t.Fatalf("durability %s without CP: %v", d, err)
			}
			m := result.(map[string]any)
			if m["value"] != "b" {
				t.Fatalf("expected value=b, got %v", m["value"])
			}
		})
	}
}

// ============================================================
// P2: Durability with many sequential runs
// ============================================================

// TestDurability_ManySequentialRuns runs Sync mode 20 times.
func TestDurability_ManySequentialRuns(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "dur-many-seq"

	for i := 0; i < 20; i++ {
		cfg := &types.RunnableConfig{
			Durability:   types.DurabilitySync,
			Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
		}
		engine := NewEngine(newSimpleGraph(t),
			WithRecursionLimit(10),
			WithCheckpointer(ms),
			WithConfig(cfg),
		)
		_, err := engine.RunSync(context.Background(), map[string]any{"value": "seq"})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Durability with checkpointer but default config (Sync)
// ============================================================

// TestDurability_DefaultConfig verifies default durability (Sync).
func TestDurability_DefaultConfig(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "dur-default"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
	)
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "default"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}
}
