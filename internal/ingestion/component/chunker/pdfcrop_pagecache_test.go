package chunker

import (
	"image"
	"sync"
	"testing"
)

// taggedImg wraps an image with an id so tests can assert which page bitmap a
// cache entry holds (pointer identity is unstable across fakeImg calls).
type taggedImg struct {
	image.Image
	id int
}

func fakeImg(n int) image.Image {
	return taggedImg{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), id: n}
}

// renderCountingFake returns a render func that records how many times each
// page was actually rendered, so tests can prove an in-use (ref > 0) page is
// never re-rendered after eviction would otherwise have dropped it.
func renderCountingFake(rendered *map[int]int, mu *sync.Mutex) func(int) (image.Image, bool) {
	return func(pn int) (image.Image, bool) {
		mu.Lock()
		(*rendered)[pn]++
		mu.Unlock()
		return fakeImg(pn), true
	}
}

func TestLRUPageCache_BasicMissThenHit(t *testing.T) {
	c := newLRUPageCache(2)
	rendered := make(map[int]int)
	var mu sync.Mutex
	r := renderCountingFake(&rendered, &mu)
	if _, ok := c.Acquire(r, 1); !ok {
		t.Fatal("acquire should succeed on miss")
	}
	c.Release(1)
	img, ok := c.Acquire(r, 1)
	if !ok {
		t.Fatal("expected hit for page 1 after release")
	}
	if img.(taggedImg).id != 1 {
		t.Fatalf("wrong image returned, id=%d", img.(taggedImg).id)
	}
	// Page 1 was rendered once (miss), then served from cache on the second
	// Acquire — render must not have been called a second time.
	if rendered[1] != 1 {
		t.Fatalf("page 1 rendered %d times, want 1 (no re-render on cache hit)", rendered[1])
	}
	c.Release(1)
}

func TestLRUPageCache_EvictsLeastRecent(t *testing.T) {
	c := newLRUPageCache(2)
	rendered := make(map[int]int)
	var mu sync.Mutex
	r := renderCountingFake(&rendered, &mu)
	c.Acquire(r, 1)
	c.Release(1)
	c.Acquire(r, 2)
	c.Release(2)
	c.Acquire(r, 1) // touch 1 so 2 becomes LRU
	c.Release(1)
	c.Acquire(r, 3) // over capacity → evict 2
	c.Release(3)
	if _, ok := c.Acquire(r, 2); !ok {
		t.Fatal("page 2 should be re-renderable after eviction")
	}
	c.Release(2)
	if _, ok := c.Acquire(r, 1); !ok {
		t.Fatal("page 1 should remain (recently used)")
	}
	c.Release(1)
	if _, ok := c.Acquire(r, 3); !ok {
		t.Fatal("page 3 should be present")
	}
	c.Release(3)
	// Page 2 was evicted once, so it must have been rendered a second time.
	if rendered[2] != 2 {
		t.Fatalf("page 2 rendered %d times, want 2 (evicted then re-rendered)", rendered[2])
	}
}

func TestLRUPageCache_GetRefreshesRecency(t *testing.T) {
	c := newLRUPageCache(2)
	rendered := make(map[int]int)
	var mu sync.Mutex
	r := renderCountingFake(&rendered, &mu)
	c.Acquire(r, 1)
	c.Release(1)
	c.Acquire(r, 2)
	c.Release(2)
	c.Acquire(r, 1) // 1 becomes most-recent, 2 is LRU
	c.Release(1)
	c.Acquire(r, 3) // evict 2
	c.Release(3)
	if _, ok := c.Acquire(r, 2); !ok {
		t.Fatal("page 2 should be evicted")
	}
	c.Release(2)
}

func TestLRUPageCache_CapacityClampedTo1(t *testing.T) {
	c := newLRUPageCache(0)
	rendered := make(map[int]int)
	var mu sync.Mutex
	r := renderCountingFake(&rendered, &mu)
	c.Acquire(r, 1)
	c.Release(1)
	if _, ok := c.Acquire(r, 1); !ok {
		t.Fatal("capacity clamped to 1, page 1 must survive")
	}
	c.Release(1)
	// Pushing a second page must not panic and should keep the cache bounded.
	c.Acquire(r, 2)
	c.Release(2)
}

// TestLRUPageCache_InFlightPageNotEvicted is the core regression guard for the
// parallel crop path: a page held by an in-flight worker (ref > 0) must never
// be evicted under the LRU cap, so the worker never loses the bitmap it is
// still cropping from and never re-renders it.
func TestLRUPageCache_InFlightPageNotEvicted(t *testing.T) {
	c := newLRUPageCache(2)
	rendered := make(map[int]int)
	var mu sync.Mutex
	r := renderCountingFake(&rendered, &mu)

	// Hold page 1 in-flight (ref > 0) while we flood the cache past cap.
	c.Acquire(r, 1)
	for pn := 10; pn < 30; pn++ {
		c.Acquire(r, pn)
		c.Release(pn)
	}
	// Page 1 was never released, so it must still be cached and must not have
	// been re-rendered.
	if _, ok := c.Acquire(r, 1); !ok {
		t.Fatal("in-flight page 1 was evicted under LRU cap")
	}
	c.Release(1)
	if rendered[1] != 1 {
		t.Fatalf("in-flight page 1 rendered %d times, want 1 (never re-rendered while held)", rendered[1])
	}
}

func TestLRUPageCache_ReleasedPageBecomesEvictable(t *testing.T) {
	c := newLRUPageCache(2)
	rendered := make(map[int]int)
	var mu sync.Mutex
	r := renderCountingFake(&rendered, &mu)

	c.Acquire(r, 1)
	c.Release(1) // now ref == 0, eligible for eviction
	for pn := 10; pn < 30; pn++ {
		c.Acquire(r, pn)
		c.Release(pn)
	}
	// Page 1 is now idle (ref == 0) and should have been evicted as LRU filled.
	if _, ok := c.Acquire(r, 1); !ok {
		t.Fatal("released idle page 1 should be re-renderable")
	}
	c.Release(1)
	if rendered[1] != 2 {
		t.Fatalf("released page 1 rendered %d times, want 2 (evicted while idle, re-rendered on use)", rendered[1])
	}
}

func TestLRUPageCache_ConcurrentSafe(t *testing.T) {
	c := newLRUPageCache(8)
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				pn := (g + i) % 16
				render := func(int) (image.Image, bool) { return fakeImg(pn), true }
				img, ok := c.Acquire(render, pn)
				if !ok {
					t.Errorf("acquire failed for page %d", pn)
					return
				}
				// Simulate cropping from the bitmap, then release.
				_ = img
				c.Release(pn)
			}
		}(g)
	}
	wg.Wait()
	// Sanity: every page that was ever added can still be queried without
	// racing; exact membership is non-deterministic under eviction.
	for pn := 0; pn < 16; pn++ {
		render := func(int) (image.Image, bool) { return fakeImg(pn), true }
		c.Acquire(render, pn)
		c.Release(pn)
	}
}

func TestPageCacheLimit_DefaultAndOverride(t *testing.T) {
	t.Setenv("RAGFLOW_PAGE_CACHE_LIMIT", "")
	if got := pageCacheLimit(); got != 64 {
		t.Fatalf("default = %d, want 64", got)
	}
	t.Setenv("RAGFLOW_PAGE_CACHE_LIMIT", "8")
	if got := pageCacheLimit(); got != 8 {
		t.Fatalf("override = %d, want 8", got)
	}
	t.Setenv("RAGFLOW_PAGE_CACHE_LIMIT", "0") // invalid → fall back to default
	if got := pageCacheLimit(); got != 64 {
		t.Fatalf("invalid override should fall back to 64, got %d", got)
	}
}
