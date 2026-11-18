package chunker

import (
	"image"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"
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
	c := newLRUPageCache(2, math.MaxInt64)
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
	c := newLRUPageCache(2, math.MaxInt64)
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
	c := newLRUPageCache(2, math.MaxInt64)
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
	c := newLRUPageCache(0, math.MaxInt64)
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
	c := newLRUPageCache(2, math.MaxInt64)
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
	c := newLRUPageCache(2, math.MaxInt64)
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
	c := newLRUPageCache(8, math.MaxInt64)
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

// TestLRUPageCache_ConcurrentSamePageRendersOnce proves that concurrent misses
// of the same page are coalesced: only the first worker renders, and every
// worker receives the shared bitmap.
func TestLRUPageCache_ConcurrentSamePageRendersOnce(t *testing.T) {
	const (
		workers = 8
		pn      = 7
	)
	c := newLRUPageCache(4, math.MaxInt64)

	var (
		mu      sync.Mutex
		renders int
	)
	started := make(chan struct{})
	release := make(chan struct{})

	render := func(page int) (image.Image, bool) {
		mu.Lock()
		renders++
		n := renders
		mu.Unlock()
		if n == 1 {
			close(started) // the single in-flight render has started
		}
		<-release // hold the page stripe until the test observes the state
		return fakeImg(page), true
	}

	imgs := make([]image.Image, workers)
	oks := make([]bool, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			imgs[i], oks[i] = c.Acquire(render, pn)
		}(i)
	}

	// Let the blocked workers finish and drop their cache refs even when an
	// assertion below fails before release is closed.
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		wg.Wait()
		for _, ok := range oks {
			if ok {
				c.Release(pn)
			}
		}
	}()

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the first render to start")
	}

	// While the first render is still blocked, every other worker must be
	// waiting on the page stripe rather than rendering the page again.
	mu.Lock()
	got := renders
	mu.Unlock()
	if got != 1 {
		t.Fatalf("render called %d times while the first render is in flight, want 1: same-page misses must wait on the page stripe", got)
	}

	close(release)
	wg.Wait()

	for i := 0; i < workers; i++ {
		if !oks[i] {
			t.Fatalf("worker %d failed to acquire page %d", i, pn)
		}
		if imgs[i] == nil {
			t.Fatalf("worker %d received a nil image", i)
		}
	}

	mu.Lock()
	got = renders
	mu.Unlock()
	if got != 1 {
		t.Fatalf("render called %d times for one page, want 1 (concurrent misses must be coalesced)", got)
	}
}

// TestLRUPageCache_HitDoesNotWaitForSameStripeRender proves a cache hit does
// not take the page-load stripe. Pages 7 and 71 share a stripe (71%64 == 7);
// rendering page 71 must not delay a hit on page 7.
func TestLRUPageCache_HitDoesNotWaitForSameStripeRender(t *testing.T) {
	c := newLRUPageCache(4, math.MaxInt64)
	if _, ok := c.Acquire(func(int) (image.Image, bool) { return fakeImg(7), true }, 7); !ok {
		t.Fatal("prime page 7")
	}
	c.Release(7)

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRender := func() { releaseOnce.Do(func() { close(release) }) }

	var renderWG sync.WaitGroup
	renderWG.Add(1)
	var ok71 bool
	go func() {
		defer renderWG.Done()
		_, ok71 = c.Acquire(func(pn int) (image.Image, bool) {
			close(started)
			<-release
			return fakeImg(pn), true
		}, 71)
	}()

	defer func() {
		releaseRender()
		renderWG.Wait()
		if ok71 {
			c.Release(71)
		}
		c.Release(7)
	}()

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for page 71 render to start")
	}

	hitDone := make(chan struct{})
	var hitOK bool
	go func() {
		defer close(hitDone)
		img, ok := c.Acquire(func(int) (image.Image, bool) {
			t.Error("cache hit on page 7 must not render")
			return nil, false
		}, 7)
		hitOK = ok
		if img == nil {
			t.Error("cache hit on page 7 returned a nil image")
		}
	}()

	select {
	case <-hitDone:
	case <-time.After(time.Second):
		releaseRender()
		<-hitDone
		t.Fatal("cache hit on page 7 waited for the page 71 render sharing its stripe")
	}

	if !hitOK {
		t.Fatal("cache hit on page 7 reported a miss")
	}
	c.Release(7)
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

// sizedRender returns a render func whose page pn is a w×w RGBA bitmap
// (4*w*w bytes) for w = widths[pn], counting renders per page.
func sizedRender(widths map[int]int, rendered map[int]int) func(int) (image.Image, bool) {
	return func(pn int) (image.Image, bool) {
		rendered[pn]++
		w := widths[pn]
		return image.NewRGBA(image.Rect(0, 0, w, w)), true
	}
}

// TestLRUPageCache_ByteBudgetEvictsLeastRecent proves the byte cap evicts idle
// pages even when the page cap has room: three 400-byte pages under an
// 800-byte budget keep only the two most recent.
func TestLRUPageCache_ByteBudgetEvictsLeastRecent(t *testing.T) {
	c := newLRUPageCache(64, 800)
	rendered := make(map[int]int)
	r := sizedRender(map[int]int{1: 10, 2: 10, 3: 10}, rendered)
	for _, pn := range []int{1, 2, 3} {
		if _, ok := c.Acquire(r, pn); !ok {
			t.Fatalf("acquire page %d failed", pn)
		}
		c.Release(pn)
	}
	if got := c.ll.Len(); got != 2 {
		t.Fatalf("retained pages = %d, want 2 (byte budget holds two 400-byte pages)", got)
	}
	if c.bytes != 800 {
		t.Fatalf("retained bytes = %d, want 800", c.bytes)
	}
	if _, ok := c.items[1]; ok {
		t.Fatal("page 1 is least recent and should have been evicted by the byte budget")
	}
	c.Acquire(r, 1)
	c.Release(1)
	if rendered[1] != 2 {
		t.Fatalf("page 1 rendered %d times, want 2 (re-render after byte eviction)", rendered[1])
	}
}

// TestLRUPageCache_OversizedPageServedThenDropped proves a page larger than the
// whole byte budget is still served to its holder, is not evicted while held,
// and is dropped as soon as it is released.
func TestLRUPageCache_OversizedPageServedThenDropped(t *testing.T) {
	c := newLRUPageCache(64, 800)
	rendered := make(map[int]int)
	r := sizedRender(map[int]int{1: 10, 9: 20}, rendered) // 400 and 1600 bytes
	c.Acquire(r, 1)
	c.Release(1)

	img, ok := c.Acquire(r, 9)
	if !ok || img.Bounds().Dx() != 20 {
		t.Fatalf("oversized page not served: ok=%v img=%v", ok, img)
	}
	if _, held := c.items[9]; !held {
		t.Fatal("oversized page evicted while still referenced")
	}
	if _, kept := c.items[1]; kept {
		t.Fatal("idle page 1 should have been evicted to make room")
	}
	c.Release(9)
	if c.ll.Len() != 0 || c.bytes != 0 {
		t.Fatalf("after release: %d pages / %d bytes retained, want 0 / 0", c.ll.Len(), c.bytes)
	}
}

func TestPageCacheBytes_DefaultAndOverride(t *testing.T) {
	const def = int64(1 << 30)
	maxMB := int64(math.MaxInt64 >> 20)
	for _, tc := range []struct {
		env  string
		want int64
	}{
		{"", def},
		{"256", 256 << 20},
		{strconv.FormatInt(maxMB, 10), maxMB << 20},
		{"0", def},
		{"-1", def},
		{"lots", def},
		{strconv.FormatInt(maxMB+1, 10), def}, // overflows when shifted into bytes
	} {
		t.Setenv("RAGFLOW_PAGE_CACHE_MB", tc.env)
		if got := pageCacheBytes(); got != tc.want {
			t.Errorf("RAGFLOW_PAGE_CACHE_MB=%q: pageCacheBytes() = %d, want %d", tc.env, got, tc.want)
		}
	}
}
