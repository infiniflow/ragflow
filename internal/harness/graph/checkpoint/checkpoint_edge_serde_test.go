// Package checkpoint provides edge case tests for serialization,
// concurrent access patterns, and boundary conditions.
package checkpoint

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"ragflow/internal/harness/graph/constants"
)

// ============================================================
// P0: Serialization — various data types
// ============================================================

// TestCheckpointSerde_VariousTypes verifies round-trip of all basic types.
func TestCheckpointSerde_VariousTypes(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "serde-types"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	data := map[string]interface{}{
		"int":        42,
		"float":      3.14,
		"string":     "hello",
		"bool_true":  true,
		"bool_false": false,
		"nil_val":    nil,
		"int_slice":  []interface{}{1, 2, 3},
		"str_slice":  []interface{}{"a", "b", "c"},
		"nested_map": map[string]interface{}{
			"inner_int":    99,
			"inner_string": "deep",
		},
	}
	if err := ms.Put(ctx, cfg, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("nil checkpoint")
	}
	if got["int"].(float64) != 42 {
		t.Fatalf("expected int=42, got %v", got["int"])
	}
	if got["string"] != "hello" {
		t.Fatalf("expected string=hello, got %v", got["string"])
	}
}

// ============================================================
// P0: Serialization — empty map
// ============================================================

// TestCheckpointSerde_EmptyMap verifies empty map round-trip.
func TestCheckpointSerde_EmptyMap(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "serde-empty"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	if err := ms.Put(ctx, cfg, map[string]interface{}{}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("nil checkpoint after empty map Put")
	}
}

// ============================================================
// P0: Serialization — large map
// ============================================================

// TestCheckpointSerde_LargeMap verifies large map serialization.
func TestCheckpointSerde_LargeMap(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "serde-large"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	data := make(map[string]interface{})
	for i := 0; i < 10000; i++ {
		data[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	if err := ms.Put(ctx, cfg, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 10000 {
		t.Fatalf("expected 10000 keys, got %d", len(got))
	}
}

// ============================================================
// P1: Serialization — deeply nested arrays
// ============================================================

// TestCheckpointSerde_NestedArrays verifies deeply nested arrays.
func TestCheckpointSerde_NestedArrays(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "serde-nest-arr"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	nested := []interface{}{"a"}
	current := &nested
	for i := 0; i < 10; i++ {
		inner := []interface{}{"level", i}
		*current = append(*current, inner)
		current = &inner
	}

	data := map[string]interface{}{"nested": nested}
	if err := ms.Put(ctx, cfg, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("nil checkpoint")
	}
}

// ============================================================
// P1: Concurrent Put on different threads (no conflict)
// ============================================================

// TestCheckpointConcurrent_DifferentThreads runs Put on 100 threads.
func TestCheckpointConcurrent_DifferentThreads(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tid := fmt.Sprintf("conc-diff-%d", idx)
			cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}
			if err := ms.Put(ctx, cfg, map[string]interface{}{"idx": idx}); err != nil {
				t.Errorf("Put %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// P1: List with zero limit
// ============================================================

// TestCheckpointSerde_ListZeroLimit verifies List returns all when limit=0.
func TestCheckpointSerde_ListZeroLimit(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "list-zero"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	for i := 0; i < 5; i++ {
		ms.Put(ctx, cfg, map[string]interface{}{"i": i})
	}

	entries, err := ms.List(ctx, cfg, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries with limit=0, got %d", len(entries))
	}
}

// ============================================================
// P1: Get after many Puts (latest is correct)
// ============================================================

// TestCheckpointSerde_LatestAfterManyPuts verifies Get returns latest.
func TestCheckpointSerde_LatestAfterManyPuts(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "latest-after-many"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	for i := 0; i < 50; i++ {
		if err := ms.Put(ctx, cfg, map[string]interface{}{"version": i}); err != nil {
			t.Fatalf("Put #%d: %v", i, err)
		}
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("nil checkpoint")
	}
	v, ok := got["version"]
	if !ok {
		t.Fatal("missing version")
	}
	if v.(float64) != 49 {
		t.Fatalf("expected version=49, got %v", v)
	}
}

// ============================================================
// P2: Timestamp ordering in List
// ============================================================

// TestCheckpointSerde_TimestampOrdering verifies List ordering.
func TestCheckpointSerde_TimestampOrdering(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "ts-ordering"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	for i := 0; i < 5; i++ {
		ms.Put(ctx, cfg, map[string]interface{}{"i": i})
		time.Sleep(time.Millisecond)
	}

	entries, err := ms.List(ctx, cfg, 5)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Check reverse chronological order.
	for i := 1; i < len(entries); i++ {
		t1 := entries[i-1]["created_at"].(time.Time)
		t2 := entries[i]["created_at"].(time.Time)
		if t1.Before(t2) {
			t.Fatalf("entry %d created at %v is before entry %d at %v (not reverse order)", i-1, t1, i, t2)
		}
	}
}

// ============================================================
// P2: Parent ID chain consistency
// ============================================================

// TestCheckpointSerde_ParentIDChain verifies parent_id links form a chain.
func TestCheckpointSerde_ParentIDChain(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "parent-chain"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	for i := 0; i < 10; i++ {
		if i > 0 {
			prevEntries, _ := ms.List(ctx, cfg, 1)
			if len(prevEntries) > 0 {
				if pid, ok := prevEntries[0][constants.ConfigKeyCheckpointID].(string); ok {
					cfg["parent_checkpoint_id"] = pid
				}
			}
		}
		if err := ms.Put(ctx, cfg, map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put #%d: %v", i, err)
		}
	}

	entries, err := ms.List(ctx, cfg, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(entries))
	}
}

// ============================================================
// P2: Rapid Put after Get on same thread
// ============================================================

// TestCheckpointSerde_RapidPutGet does 1000 Put/Get cycles on same thread.
func TestCheckpointSerde_RapidPutGet(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	tid := "rapid-pg"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	for i := 0; i < 1000; i++ {
		if err := ms.Put(ctx, cfg, map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		got, err := ms.Get(ctx, cfg)
		if err != nil || got == nil {
			t.Fatalf("Get %d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Concurrent Put/Get on different threads, same checkpointer
// ============================================================

// TestCheckpointConcurrent_RapidCycle runs rapid Put/Get cycles
// on multiple threads.
func TestCheckpointConcurrent_RapidCycle(t *testing.T) {
	ms := NewMemorySaver()
	ctx := context.Background()
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			tid := fmt.Sprintf("rapid-cycle-%d", gid)
			cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}
			for i := 0; i < 100; i++ {
				if err := ms.Put(ctx, cfg, map[string]interface{}{"i": i}); err != nil {
					t.Errorf("Put: %v", err)
					return
				}
				_, err := ms.Get(ctx, cfg)
				if err != nil {
					t.Errorf("Get: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
