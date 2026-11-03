package pregel

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ragflow/internal/harness/graph/types"
)

func TestMemoryCache(t *testing.T) {
	ctx := t.Context()
	cache := NewMemoryCache(100, EvictLRU)

	// Test Set and Get
	cache.Set(ctx, "key1", "value1", 0)
	val, ok := cache.Get(ctx, "key1")
	if !ok {
		t.Error("expected to get value")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got %v", val)
	}

	// Test non-existent key
	_, ok = cache.Get(ctx, "key2")
	if ok {
		t.Error("expected not to find key2")
	}

	// Test expiration
	cache.Set(ctx, "key3", "value3", 1*time.Millisecond)
	time.Sleep(2 * time.Millisecond)
	_, ok = cache.Get(ctx, "key3")
	if ok {
		t.Error("expected key3 to be expired")
	}

	// Test Delete
	cache.Set(ctx, "key4", "value4", 0)
	cache.Delete(ctx, "key4")
	_, ok = cache.Get(ctx, "key4")
	if ok {
		t.Error("expected key4 to be deleted")
	}

	// Test Clear
	cache.Set(ctx, "key5", "value5", 0)
	cache.Clear()
	_, ok = cache.Get(ctx, "key5")
	if ok {
		t.Error("expected cache to be cleared")
	}
}

func TestMemoryCacheEviction(t *testing.T) {
	ctx := t.Context()
	cache := NewMemoryCache(3, EvictLRU)

	// Fill cache
	cache.Set(ctx, "key1", "value1", 0)
	cache.Set(ctx, "key2", "value2", 0)
	cache.Set(ctx, "key3", "value3", 0)

	// Access key1 to make it more recent
	cache.Get(ctx, "key1")

	// Add new key, should evict key2 (least recently used)
	cache.Set(ctx, "key4", "value4", 0)

	_, ok := cache.Get(ctx, "key2")
	if ok {
		t.Error("expected key2 to be evicted")
	}

	// key1 should still exist
	_, ok = cache.Get(ctx, "key1")
	if !ok {
		t.Error("expected key1 to exist")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	key1 := GenerateCacheKey("node1", map[string]any{"input": "test"})
	key2 := GenerateCacheKey("node1", map[string]any{"input": "test"})
	key3 := GenerateCacheKey("node2", map[string]any{"input": "test"})

	if key1 != key2 {
		t.Error("expected same input to generate same key")
	}
	if key1 == key3 {
		t.Error("expected different nodes to generate different keys")
	}
}

// Regression: LRU eviction must follow the actual access order, not wall-clock
// timestamps. All the operations below complete within a single OS clock tick
// on Windows (and on any host with a coarse monotonic clock), so ordering by
// time.Now() left tied entries to map iteration order and evicted an arbitrary
// key. The cache now stamps lastAccess from a logical clock, which makes the
// eviction victim deterministic on every platform. Survivors are asserted via
// the entry map so the checks themselves do not refresh recency.
func TestMemoryCacheLRUEvictionFollowsLogicalClock(t *testing.T) {
	ctx := t.Context()
	cache := NewMemoryCache(3, EvictLRU)

	cache.Set(ctx, "key1", "value1", 0)
	cache.Set(ctx, "key2", "value2", 0)
	cache.Set(ctx, "key3", "value3", 0)

	// key1 was touched last, so key2 is the least recently used entry even
	// though every operation above shares one wall-clock tick.
	cache.Get(ctx, "key1")
	cache.Set(ctx, "key4", "value4", 0)

	if _, ok := cache.data["key2"]; ok {
		t.Error("expected key2 (least recently used) to be evicted")
	}
	for _, key := range []string{"key1", "key3", "key4"} {
		if _, ok := cache.data[key]; !ok {
			t.Errorf("expected %s to survive", key)
		}
	}

	// Same-tick churn must evict strictly in insertion order, never a
	// map-order random pick, while keeping the cache within maxSize.
	churn := NewMemoryCache(4, EvictLRU)
	for i := 0; i < 24; i++ {
		churn.Set(ctx, fmt.Sprintf("k%02d", i), i, 0)
	}
	if len(churn.data) != 4 {
		t.Errorf("cache size = %d, want 4", len(churn.data))
	}
	for i := 0; i < 20; i++ {
		if _, ok := churn.data[fmt.Sprintf("k%02d", i)]; ok {
			t.Errorf("expected k%02d to be evicted", i)
		}
	}
	for i := 20; i < 24; i++ {
		if _, ok := churn.data[fmt.Sprintf("k%02d", i)]; !ok {
			t.Errorf("expected k%02d to survive", i)
		}
	}
}

func TestMemoryCacheLFUEvictionBreaksHitTiesByLRU(t *testing.T) {
	ctx := t.Context()
	cache := NewMemoryCache(3, EvictLFU)

	cache.Set(ctx, "used", "a", 0)
	cache.Set(ctx, "old-tie", "b", 0)
	cache.Set(ctx, "new-tie", "c", 0)

	// "used" has one hit, the other two have none and tie on hits, so the
	// older of the two is the least valuable entry.
	cache.Get(ctx, "used")
	cache.Set(ctx, "fourth", "d", 0)
	if _, ok := cache.data["old-tie"]; ok {
		t.Error("expected old-tie (hit tie, least recently used) to be evicted")
	}
	for _, key := range []string{"used", "new-tie", "fourth"} {
		if _, ok := cache.data[key]; !ok {
			t.Errorf("expected %s to survive", key)
		}
	}

	// The next eviction faces the same 0-hit tie between new-tie and fourth
	// and must again take the older one, not a map-order random pick.
	cache.Set(ctx, "fifth", "e", 0)
	if _, ok := cache.data["new-tie"]; ok {
		t.Error("expected new-tie (hit tie, least recently used) to be evicted")
	}
	for _, key := range []string{"used", "fourth", "fifth"} {
		if _, ok := cache.data[key]; !ok {
			t.Errorf("expected %s to survive", key)
		}
	}
}

func TestCachedExecutor(t *testing.T) {
	ctx := t.Context()
	cache := NewMemoryCache(100, EvictLRU)

	callCount := 0
	fn := func(ctx context.Context, input any) (any, error) {
		callCount++
		return input.(int) * 2, nil
	}

	policy := &types.CachePolicy{
		TTL: &[]time.Duration{5 * time.Second}[0],
	}
	executor := NewCachedExecutor(cache, policy)

	// First call should execute
	result, err := executor.Execute(ctx, "node1", 5, fn)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 10 {
		t.Errorf("expected 10, got %v", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call with same input should use cache
	result, err = executor.Execute(ctx, "node1", 5, fn)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 10 {
		t.Errorf("expected 10, got %v", result)
	}
	if callCount != 1 {
		t.Errorf("expected cache hit, but function was called again (call count: %d)", callCount)
	}

	// Call with different input should execute
	result, err = executor.Execute(ctx, "node1", 7, fn)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 14 {
		t.Errorf("expected 14, got %v", result)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestNoopCache(t *testing.T) {
	ctx := t.Context()
	cache := &NoopCache{}

	cache.Set(ctx, "key", "value", 0)
	_, ok := cache.Get(ctx, "key")
	if ok {
		t.Error("expected noop cache to never return values")
	}

	// These should not panic
	cache.Delete(ctx, "key")
	cache.Clear()
}
