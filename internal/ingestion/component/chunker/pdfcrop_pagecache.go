// Package chunker — bounded LRU + reference-counted cache for rendered PDF
// page bitmaps.
//
// cropImageChunks renders a PDF page once and reuses the bitmap across every
// chunk that spans it. The serial version kept the cache unbounded and relied
// on in-order processing to drop pages it would never touch again. The
// concurrent version no longer has that "earlier pages are done" invariant, so
// an unbounded cache retains the whole document: a 3266-page PDF peaked at
// ~21 GB. lruPageCache bounds that with two mechanisms:
//
//   - Reference counting: a page is held (ref > 0) only while an in-flight
//     worker is actively cropping from it. When the last user Release-s it
//     (ref → 0) it becomes eligible for eviction — so a page an in-flight
//     worker still needs is never dropped, and never re-rendered.
//   - LRU caps: at most cap pages and maxBytes of bitmap memory are kept; when
//     over either cap the least-recently used ref==0 pages are evicted. The
//     page cap bounds long PDFs; the byte cap bounds documents with few but
//     large pages, where the page cap alone still allows tens of gigabytes.
package chunker

import (
	"container/list"
	"image"
	"math"
	"os"
	"strconv"
	"sync"
)

// pageCacheEntry pairs a page number with its rendered bitmap and a reference
// count for storage in the LRU list.
type pageCacheEntry struct {
	pn    int
	img   image.Image
	bytes int64
	ref   int
}

// lruPageCache is a concurrency-safe bounded LRU of rendered page bitmaps with
// reference counting. Eviction only drops ref==0 pages, so an in-flight crop
// can never lose a page it still holds a reference to. Concurrent loads of the
// same page are serialized so workers share the loaded bitmap.
type lruPageCache struct {
	loading  [64]sync.Mutex
	mu       sync.Mutex
	ll       *list.List // front = most-recently-used; element value = pageCacheEntry
	items    map[int]*list.Element
	cap      int
	maxBytes int64
	bytes    int64 // bitmap bytes of every retained page, in use or not
}

// newLRUPageCache returns a cache that retains at most capacity pages and
// maxBytes of page bitmaps. A capacity below 1 is clamped to 1. A page larger
// than maxBytes is still served to the workers that hold it, and dropped as
// soon as the last one releases it.
func newLRUPageCache(capacity int, maxBytes int64) *lruPageCache {
	if capacity < 1 {
		capacity = 1
	}
	return &lruPageCache{
		ll:       list.New(),
		items:    make(map[int]*list.Element),
		cap:      capacity,
		maxBytes: maxBytes,
	}
}

// imageBytes returns the memory held by a rendered page bitmap, counted at 4
// bytes per pixel (the RGBA layout the page renderers produce).
func imageBytes(img image.Image) int64 {
	b := img.Bounds()
	return int64(b.Dx()) * int64(b.Dy()) * 4
}

// Acquire returns the cached or freshly rendered image for pn and marks it
// referenced. The caller must call Release(pn) once cropping from pn is done.
// render is invoked outside the lock on a miss, so the CGO pdfium render is
// never serialized by the cache mutex. Concurrent misses of the same page
// share one load. A hit is served without taking the page's load stripe, so it
// never waits for an unrelated page that hashes to the same stripe.
func (c *lruPageCache) Acquire(render func(int) (image.Image, bool), pn int) (image.Image, bool) {
	// Fast path: a cached page only needs the cache mutex. Taking the load
	// stripe here would make a hit wait behind an unrelated page's render.
	c.mu.Lock()
	if img, ok := c.lookupLocked(pn); ok {
		c.mu.Unlock()
		return img, true
	}
	c.mu.Unlock()

	load := &c.loading[uint(pn)%uint(len(c.loading))]
	load.Lock()
	defer load.Unlock()

	// Another worker may have loaded the page while this one waited for the
	// stripe; recheck before paying for a render.
	c.mu.Lock()
	if img, ok := c.lookupLocked(pn); ok {
		c.mu.Unlock()
		return img, true
	}
	c.mu.Unlock()

	img, ok := render(pn)
	if !ok {
		return nil, false
	}

	size := imageBytes(img)
	c.mu.Lock()
	el := c.ll.PushFront(pageCacheEntry{pn: pn, img: img, bytes: size, ref: 1})
	c.items[pn] = el
	c.bytes += size
	c.evictLocked()
	c.mu.Unlock()
	return img, true
}

// lookupLocked returns pn's bitmap and marks it referenced. Caller must hold
// c.mu.
func (c *lruPageCache) lookupLocked(pn int) (image.Image, bool) {
	el, ok := c.items[pn]
	if !ok {
		return nil, false
	}
	e := el.Value.(pageCacheEntry)
	e.ref++
	el.Value = e
	c.ll.MoveToFront(el)
	return e.img, true
}

// Release marks pn as no longer needed by the caller. It becomes a candidate
// for LRU eviction; an in-flight ref never drops to 0 while a worker still
// holds it.
func (c *lruPageCache) Release(pn int) {
	c.mu.Lock()
	if el, ok := c.items[pn]; ok {
		e := el.Value.(pageCacheEntry)
		if e.ref > 0 {
			e.ref--
		}
		el.Value = e
		c.evictLocked()
	}
	c.mu.Unlock()
}

// evictLocked drops least-recently-used pages whose ref is 0 until the cache
// is within both its page and byte caps. Caller must hold c.mu. It never
// evicts a page still referenced by an in-flight worker, so eviction stops
// early (never over-shrinks) only when every retained page is in use.
func (c *lruPageCache) evictLocked() {
	for c.ll.Len() > c.cap || c.bytes > c.maxBytes {
		var victim *list.Element
		for e := c.ll.Back(); e != nil; e = e.Prev() {
			if e.Value.(pageCacheEntry).ref == 0 {
				victim = e
				break
			}
		}
		if victim == nil {
			return // all retained pages are in use; cannot shrink further
		}
		e := victim.Value.(pageCacheEntry)
		c.ll.Remove(victim)
		delete(c.items, e.pn)
		c.bytes -= e.bytes
	}
}

// pageCacheLimit bounds the shared rendered-page cache so cropping a very long
// PDF does not retain every page bitmap for the whole document. 64 pages is
// enough to cover near-term reuse within a contiguous page window; override
// with RAGFLOW_PAGE_CACHE_LIMIT.
func pageCacheLimit() int {
	if v := os.Getenv("RAGFLOW_PAGE_CACHE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 64
}

// pageCacheBytes bounds the bitmap memory of the shared rendered-page cache.
// At 216 DPI an A4 page is ~18 MB but an A0 sheet is ~290 MB, so 64 A0 pages
// would retain ~18 GB under the page cap alone. 1 GiB still covers ~59 A4
// pages; override (in MiB) with RAGFLOW_PAGE_CACHE_MB.
func pageCacheBytes() int64 {
	if v := os.Getenv("RAGFLOW_PAGE_CACHE_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 && n <= math.MaxInt64>>20 {
			return n << 20
		}
	}
	return 1 << 30
}
