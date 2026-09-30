package utility

import (
	"sync"
	"testing"
)

// Concurrent Get calls mutate the LRU list and must be race-free.
func TestEmbeddingLRUConcurrentGet(t *testing.T) {
	c := NewEmbeddingLRU(100)
	for i := 0; i < 20; i++ {
		c.Put(string(rune(i)), "m", []float64{1})
	}
	var wg sync.WaitGroup
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				c.Get(string(rune(i%20)), "m")
			}
		}()
	}
	wg.Wait()
}

// Distinct (question, embeddingID) pairs must not collide in the cache key.
func TestEmbeddingLRUCompositeKeyNoCollision(t *testing.T) {
	c := NewEmbeddingLRU(2)
	c.Put("a::b", "c", []float64{1})
	c.Put("a", "b::c", []float64{2})
	v, ok := c.Get("a::b", "c")
	if !ok || v[0] != 1 {
		t.Fatalf("colliding composite keys returned %v (ok=%v)", v, ok)
	}
}
