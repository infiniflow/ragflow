package store

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStore_BasicOperations(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	namespace := []string{"test", "user"}
	key := "user123"

	// Test Put
	value := map[string]interface{}{"name": "Alice", "age": 30}
	err := store.Put(ctx, namespace, key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, namespace, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name 'Alice', got %v", retrieved["name"])
	}

	// Test Delete
	err = store.Delete(ctx, namespace, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	retrieved, err = store.Get(ctx, namespace, key)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if retrieved != nil {
		t.Errorf("Expected nil after delete, got %v", retrieved)
	}
}

func TestInMemoryStore_Search(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	namespace := []string{"test", "search"}
	store.Put(ctx, namespace, "doc1", map[string]interface{}{"content": "hello world"})
	store.Put(ctx, namespace, "doc2", map[string]interface{}{"content": "foo bar"})

	// Search for "hello"
	results, err := store.Search(ctx, namespace, "hello", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Search with limit
	results, err = store.Search(ctx, namespace, "", 1)
	if err != nil {
		t.Fatalf("Search with limit failed: %v", err)
	}
	if len(results) > 1 {
		t.Errorf("Expected at most 1 result, got %d", len(results))
	}
}

func TestInMemoryStore_BatchPut(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	ops := []PutOperation{
		{Namespace: []string{"batch", "test"}, Key: "key1", Value: map[string]interface{}{"v": 1}},
		{Namespace: []string{"batch", "test"}, Key: "key2", Value: map[string]interface{}{"v": 2}},
		{Namespace: []string{"batch", "test"}, Key: "key3", Value: map[string]interface{}{"v": 3}},
	}

	err := store.BatchPut(ctx, ops)
	if err != nil {
		t.Fatalf("BatchPut failed: %v", err)
	}

	// Verify all entries
	for i, op := range ops {
		retrieved, err := store.Get(ctx, op.Namespace, op.Key)
		if err != nil {
			t.Fatalf("Get batch entry %d failed: %v", i, err)
		}
		if retrieved["v"] != i+1 {
			t.Errorf("Expected value %d, got %v", i+1, retrieved["v"])
		}
	}
}

func TestInMemoryStore_TTL(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	namespace := []string{"ttl", "test"}
	key := "expiring"

	store.Put(ctx, namespace, key, map[string]interface{}{"data": "temp"})
	store.SetTTL(ctx, namespace, key, 100*time.Millisecond)

	// Should exist immediately
	retrieved, err := store.Get(ctx, namespace, key)
	if err != nil || retrieved == nil {
		t.Error("Value should exist immediately")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	retrieved, err = store.Get(ctx, namespace, key)
	if err != nil {
		t.Fatalf("Get after expiration failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Value should be expired")
	}
}

func TestInMemoryStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	namespace := []string{"list", "test"}
	store.Put(ctx, namespace, "key1", map[string]interface{}{"v": 1})
	store.Put(ctx, namespace, "key2", map[string]interface{}{"v": 2})

	keys, err := store.List(ctx, namespace, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	// Test with limit
	keys, err = store.List(ctx, namespace, 1)
	if err != nil {
		t.Fatalf("List with limit failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("Expected 1 key with limit, got %d", len(keys))
	}
}

func TestInMemoryStore_Clear(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	namespace := []string{"clear", "test"}
	store.Put(ctx, namespace, "key1", map[string]interface{}{"v": 1})
	store.Put(ctx, namespace, "key2", map[string]interface{}{"v": 2})

	err := store.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify cleared
	keys, _ := store.List(ctx, namespace, 0)
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys after clear, got %d", len(keys))
	}
}

func TestInMemoryStore_Closed(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	store.Close()

	namespace := []string{"closed", "test"}

	// Operations on closed store should fail
	err := store.Put(ctx, namespace, "key", map[string]interface{}{})
	if err == nil {
		t.Error("Put on closed store should fail")
	}

	_, err = store.Get(ctx, namespace, "key")
	if err == nil {
		t.Error("Get on closed store should fail")
	}
}
