package pregel

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/harness/graph/types"
)

func TestMemoryCache(t *testing.T) {
	ctx := context.Background()
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
	ctx := context.Background()
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

func TestCachedExecutor(t *testing.T) {
	ctx := context.Background()
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
	ctx := context.Background()
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
