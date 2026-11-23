package store

import (
	"context"
	"testing"
	"time"
)

// newCleanupTestStore builds a store whose cleanup loop runs every 5ms so TTL
// expiry is observable without waiting for the production one-minute ticker.
func newCleanupTestStore(t *testing.T) *InMemoryStore {
	t.Helper()
	s := &InMemoryStore{
		data:          make(map[string]map[string]map[string]interface{}),
		ttl:           make(map[string]time.Time),
		indexes:       make(map[string]map[string][]float64),
		indexConfigs:  make(map[string]IndexConfig),
		stopCleanup:   make(chan struct{}),
		cleanupTicker: time.NewTicker(5 * time.Millisecond),
	}
	go s.cleanupExpired()
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func indexedEmbedding(s *InMemoryStore, nsKey, key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.indexes[nsKey][key]
	return ok
}

func waitForDataGone(t *testing.T, s *InMemoryStore, nsKey, key string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		_, ok := s.data[nsKey][key]
		s.mu.RUnlock()
		if !ok {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("cleanup did not remove expired data entry %q in namespace %q", key, nsKey)
}

func putIndexedItem(t *testing.T, s *InMemoryStore, ctx context.Context, namespace []string, key string, ttl *time.Duration) {
	t.Helper()
	index := IndexConfig{Embed: func() {}, Fields: []string{"embedding"}}
	value := map[string]interface{}{"embedding": []float64{1, 0, 0}}
	if err := s.PutItem(ctx, namespace, key, value, index, ttl); err != nil {
		t.Fatalf("PutItem(%q): %v", key, err)
	}
}

// TestCleanupExpiredRemovesEmbeddingIndex is the primary RED test: the index
// is written under the bare key but was deleted under fullKey, so the entry
// survived every TTL cleanup and grew without bound.
func TestCleanupExpiredRemovesEmbeddingIndex(t *testing.T) {
	s := newCleanupTestStore(t)
	namespace := []string{"tenant", "graph"}
	key := "node-1"
	ttl := 15 * time.Millisecond
	putIndexedItem(t, s, t.Context(), namespace, key, &ttl)

	nsKey := s.nsKey(namespace)
	if !indexedEmbedding(s, nsKey, key) {
		t.Fatal("embedding index was not created by PutItem")
	}
	waitForDataGone(t, s, nsKey, key)
	if indexedEmbedding(s, nsKey, key) {
		t.Fatal("expired embedding index entry was not removed by cleanup")
	}
}

// TestCleanupExpiredKeepsOtherNamespaceIndex exercises key-space isolation:
// the same key in a second namespace with a longer TTL must keep its index.
func TestCleanupExpiredKeepsOtherNamespaceIndex(t *testing.T) {
	s := newCleanupTestStore(t)
	ctx := t.Context()
	nsA, nsB := []string{"tenant", "a"}, []string{"tenant", "b"}
	sharedKey := "shared"
	shortTTL, longTTL := 15*time.Millisecond, 5*time.Second
	putIndexedItem(t, s, ctx, nsA, sharedKey, &shortTTL)
	putIndexedItem(t, s, ctx, nsB, sharedKey, &longTTL)

	keyA, keyB := s.nsKey(nsA), s.nsKey(nsB)
	waitForDataGone(t, s, keyA, sharedKey)
	if indexedEmbedding(s, keyA, sharedKey) {
		t.Fatal("expired index in first namespace was not removed")
	}
	if !indexedEmbedding(s, keyB, sharedKey) {
		t.Fatal("unexpired index in second namespace was wrongly removed")
	}
}

// TestPutItemWithoutTTLKeepsIndex pins the no-TTL behavior: cleanup must not
// touch an index entry that has no expiry.
func TestPutItemWithoutTTLKeepsIndex(t *testing.T) {
	s := newCleanupTestStore(t)
	namespace := []string{"tenant", "forever"}
	key := "persistent"
	putIndexedItem(t, s, t.Context(), namespace, key, nil)

	time.Sleep(50 * time.Millisecond)
	if !indexedEmbedding(s, s.nsKey(namespace), key) {
		t.Fatal("index entry without TTL was removed")
	}
}
