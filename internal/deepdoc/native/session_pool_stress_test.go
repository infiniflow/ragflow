//go:build cgo

package native

import (
	"sync"
	"sync/atomic"
	"testing"
)

// fakeSession is a pooledSession stand-in that needs no ONNX model, so the
// sessionPool's concurrency (Get/Release/LRU-evict/poison) can be stressed
// without MODEL_DIR. It records Destroy calls to assert pool lifecycle
// behaviour under contention.
type fakeSession struct {
	id        int
	poisoned  atomic.Bool
	destroyed atomic.Bool
}

func (f *fakeSession) Destroy()         { f.destroyed.Store(true) }
func (f *fakeSession) isPoisoned() bool { return f.poisoned.Load() }
func (f *fakeSession) markPoisoned()    { f.poisoned.Store(true) }

// TestSessionPoolConcurrentGetReleaseStress hammers a generic sessionPool from
// many goroutines with a rotating key set (forcing LRU eviction) and a poison
// fraction, under -race. It proves the pool's shared state — the pools map, the
// lru slice, and each key-pool's free list plus their mutexes — is data-race
// free. This is the exact code path backing the real modelSessions / recSessions
// pools that the DLA/TSR/OCR-rec/Det recognizers rely on.
//
// Crucially it needs no ONNX weights, so unlike the MODEL_DIR-gated integration
// tests it runs in the default `go test ./...` (cgo) path and can be exercised
// with -race in CI, giving the concurrency-safety claim default-path coverage.
func TestSessionPoolConcurrentGetReleaseStress(t *testing.T) {
	const maxKeys = 8
	const maxFree = 2
	p := newSessionPool[int, *fakeSession](maxKeys, maxFree)

	var nextID atomic.Int64
	var mu sync.Mutex
	var created []*fakeSession

	newFn := func() (*fakeSession, error) {
		s := &fakeSession{id: int(nextID.Add(1))}
		mu.Lock()
		created = append(created, s)
		mu.Unlock()
		return s, nil
	}

	const goroutines = 64
	const iters = 200
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Rotate the key across more distinct values than maxKeys so the
				// LRU eviction path is exercised under contention.
				key := (g + i) % (maxKeys * 2)
				s, release, err := p.Get(key, newFn)
				if err != nil {
					t.Errorf("Get(%d): %v", key, err)
					return
				}
				// Every 7th session is poisoned to drive the Destroy-on-release
				// branch (ORT does not guarantee reuse safety after a forced
				// termination, so the pool must Destroy rather than re-Put).
				if i%7 == 0 {
					s.markPoisoned()
				}
				release()
			}
		}(g)
	}
	wg.Wait()

	// The pool must stay bounded: eviction keeps live key-pools <= maxKeys even
	// though goroutines cycled through 2*maxKeys distinct keys.
	if got := p.KeyCount(); got > maxKeys {
		t.Errorf("KeyCount %d exceeds maxKeys %d (LRU eviction not bounding the pool)", got, maxKeys)
	}

	// Poisoned sessions must have been Destroyed on release (never pooled). Idle,
	// unpoisoned sessions that are still pooled at the end are legitimately
	// NOT destroyed, so we only assert the poison contract.
	mu.Lock()
	all := created
	mu.Unlock()
	var poisonedSeen, poisonedDestroyed int
	for _, s := range all {
		if s.poisoned.Load() {
			poisonedSeen++
			if !s.destroyed.Load() {
				t.Errorf("poisoned session %d returned to the pool instead of being Destroyed", s.id)
			} else {
				poisonedDestroyed++
			}
		}
	}
	if poisonedSeen > 0 && poisonedDestroyed == 0 {
		t.Errorf("none of %d poisoned sessions were Destroyed", poisonedSeen)
	}
}
