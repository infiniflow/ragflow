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

// Changing the separator to || does not make arbitrary inputs unambiguous.
func TestEmbeddingLRUDoublePipeKeyNoCollision(t *testing.T) {
	c := NewEmbeddingLRU(2)
	c.Put("a||b", "c", []float64{1})
	c.Put("a", "b||c", []float64{2})
	first, ok := c.Get("a||b", "c")
	if !ok || first[0] != 1 {
		t.Fatalf("first value = %v, found=%v", first, ok)
	}
	second, ok := c.Get("a", "b||c")
	if !ok || second[0] != 2 {
		t.Fatalf("second value = %v, found=%v", second, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("cache length = %d, want 2", c.Len())
	}
}
func TestEmbeddingLRUConcurrentLenAndUpdates(t *testing.T) {
	c := NewEmbeddingLRU(20)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := string(rune(i % 20))
				c.Put(key, "m", []float64{float64(i)})
				c.Get(key, "m")
				if n := c.Len(); n < 0 || n > 20 {
					t.Errorf("cache length = %d", n)
				}
				if i%7 == 0 {
					c.Remove(key, "m")
				}
				if i%31 == 0 {
					c.Clear()
				}
			}
		}()
	}
	wg.Wait()
}
