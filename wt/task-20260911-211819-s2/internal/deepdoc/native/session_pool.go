//go:build cgo

package native

// session_pool.go — shared ONNX session pool for all recognizers.
//
// RunDLA / RunTSR / RunOCRRec (fixed-shape) and RunDet (variable-shape DB
// detector) all load an ONNX session per call. A long document pays that setup
// cost per region/page even though every call uses the same shapes within a
// model. This pool caches one session per (model signature) tuple and hands it
// back between calls.
//
// Sessions are pooled, not shared concurrently: session.Run copies the caller's
// input into the session's fixed-shape input tensor and then executes, so a
// single session must never be touched by two goroutines at once. Get returns a
// session owned by the caller until release is called; release returns it to
// the pool for reuse. This keeps the Get/Run/Release window single-owner, which
// is what makes reuse safe under the page/region worker pools.
//
// The pool is generic over the key type K and the pooled session type V so both
// the fixed-shape recognizers (DLA/TSR/OCR-rec, recSession) and the
// variable-shape detector (detSession) share one implementation. Two bounds
// apply:
//   - maxKeys caps the number of distinct key-pools; when exceeded the
//     least-recently-used key-pool is evicted and its idle sessions Destroyed
//     (bounds memory for the variable-shape detector, which can see many
//     distinct page sizes).
//   - maxFree caps idle sessions retained per key-pool; extras are Destroyed on
//     release instead of pooled. A maxFree / maxKeys of 0 means unbounded — the
//     degenerate case for the fixed-shape recognizers, whose key set is tiny.

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// pooledSession is the minimal contract the pool needs from any cached ONNX
// session: release its resources, and report/mark the poisoned flag set when a
// Run is force-terminated via context (ORT does not guarantee reuse safety
// after a termination, so the pool must Destroy rather than re-Put).
type pooledSession interface {
	Destroy()
	isPoisoned() bool
	markPoisoned()
}

// sessionKeyPool holds reusable sessions for one key.
type sessionKeyPool[V pooledSession] struct {
	mu   sync.Mutex
	live bool // false once evicted; checked-out sessions self-Destroy on release
	free []V
}

// sessionPool is a generic reusable ONNX session pool keyed by K, storing
// values of type V (any pooledSession).
type sessionPool[K comparable, V pooledSession] struct {
	mu      sync.Mutex
	pools   map[K]*sessionKeyPool[V]
	lru     []K // front = least-recently-used
	maxKeys int
	maxFree int
}

func newSessionPool[K comparable, V pooledSession](maxKeys, maxFree int) *sessionPool[K, V] {
	return &sessionPool[K, V]{
		pools:   make(map[K]*sessionKeyPool[V]),
		maxKeys: maxKeys,
		maxFree: maxFree,
	}
}

// Get returns a reusable session for key, constructing one with newFn on a pool
// miss, plus a release func. The caller must call release exactly once. A
// poisoned session is Destroyed on release rather than pooled (ORT does not
// guarantee reuse safety after a forced termination).
func (p *sessionPool[K, V]) Get(key K, newFn func() (V, error)) (V, func(), error) {
	p.mu.Lock()
	kp := p.pools[key]
	if kp == nil {
		if p.maxKeys > 0 && len(p.pools) >= p.maxKeys {
			p.evictLRU()
		}
		kp = &sessionKeyPool[V]{live: true}
		p.pools[key] = kp
		p.lru = append(p.lru, key)
	} else {
		p.touchLRU(key)
	}
	p.mu.Unlock()

	kp.mu.Lock()
	var s V
	if n := len(kp.free); n > 0 {
		s = kp.free[n-1]
		kp.free = kp.free[:n-1]
	}
	kp.mu.Unlock()

	if isNil(s) {
		var err error
		s, err = newFn()
		if err != nil {
			var zero V
			return zero, nil, err
		}
	}

	release := func() {
		if s.isPoisoned() {
			s.Destroy()
			return
		}
		kp.mu.Lock()
		if kp.live && (p.maxFree <= 0 || len(kp.free) < p.maxFree) {
			kp.free = append(kp.free, s)
			kp.mu.Unlock()
		} else {
			kp.mu.Unlock()
			s.Destroy()
		}
	}
	return s, release, nil
}

// isNil reports whether a generic pooledSession value is its zero value (a nil
// pointer). Every pooled type is a pointer, so reflection's IsNil is safe here.
// newFn only runs on a miss.
func isNil[V pooledSession](v V) bool {
	rv := reflect.ValueOf(v)
	return rv.IsNil()
}

// KeyCount returns the number of distinct key-pools currently live. Used by
// tests that assert the pool set is bounded.
func (p *sessionPool[K, V]) KeyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pools)
}

// touchLRU moves key to the most-recently-used end. Caller holds p.mu.
func (p *sessionPool[K, V]) touchLRU(key K) {
	for i, k := range p.lru {
		if k == key {
			p.lru = append(p.lru[:i], p.lru[i+1:]...)
			p.lru = append(p.lru, k)
			return
		}
	}
}

// evictLRU drops the least-recently-used key-pool and destroys its idle
// sessions. Caller holds p.mu.
func (p *sessionPool[K, V]) evictLRU() {
	if len(p.lru) == 0 {
		return
	}
	evict := p.lru[0]
	p.lru = p.lru[1:]
	kp := p.pools[evict]
	delete(p.pools, evict)
	if kp == nil {
		return
	}
	kp.mu.Lock()
	kp.live = false
	for _, s := range kp.free {
		s.Destroy()
	}
	kp.free = nil
	kp.mu.Unlock()
}

// ---- fixed-shape recognizer pool (DLA / TSR / OCR-rec) ----

// sessKey is the pool key for the fixed-shape models. Unlike the DB detector
// these models always run at a constant input size, so the tuple is constant
// per modelDir in practice.
type sessKey struct {
	modelPath, inName, outName string
	inShape, outShape          string
	intraOpThreads             int
}

func sessKeyOf(modelPath, inName string, inShape []int64, outName string, outShape []int64, intraOpThreads int) sessKey {
	return sessKey{
		modelPath:      modelPath,
		inName:         inName,
		outName:        outName,
		inShape:        shapeKey(inShape),
		outShape:       shapeKey(outShape),
		intraOpThreads: intraOpThreads,
	}
}

func shapeKey(s []int64) string {
	parts := make([]string, len(s))
	for i, d := range s {
		parts[i] = strconv.FormatInt(d, 10)
	}
	return strings.Join(parts, ",")
}

// modelSessions is unbounded (maxKeys/maxFree = 0): the fixed-shape key set is
// tiny, so the degenerate no-eviction case is correct here.
var modelSessions = newSessionPool[sessKey, *session](0, 0)

// getModelSession returns a reusable session for the given model signature plus
// a release func. The caller must call release exactly once.
func getModelSession(modelPath, inName string, inShape []int64, outName string, outShape []int64, intraOpThreads int) (*session, func(), error) {
	key := sessKeyOf(modelPath, inName, inShape, outName, outShape, intraOpThreads)
	return modelSessions.Get(key, func() (*session, error) {
		return NewSession(modelPath, inName, inShape, outName, outShape, intraOpThreads)
	})
}

// ---- dynamic-width OCR-rec pool ----

// recKey is the pool key for the dynamic-width OCR-rec model. The session is
// pinned to one input width (the input tensor is fixed-shape per width), and
// the output is auto-allocated at the model's true (width-dependent) sequence
// length, so the key carries the width but no output shape.
type recKey struct {
	modelPath, inName, outName string
	inShape                    string
	intraOpThreads             int
}

func recKeyOf(modelPath, inName string, inShape []int64, outName string, intraOpThreads int) recKey {
	return recKey{
		modelPath:      modelPath,
		inName:         inName,
		outName:        outName,
		inShape:        shapeKey(inShape),
		intraOpThreads: intraOpThreads,
	}
}

const (
	// recMaxShapePools caps distinct (modelPath, width) pools. Unlike the
	// fixed-shape DLA/TSR models, a long-running server ingesting many
	// differently-sized text lines would otherwise pin one pooled session (and
	// its ORT tensors) per distinct width forever. The shared sessionPool
	// evicts the least-recently-used width pool (Destroying its idle sessions)
	// once the cap is exceeded, bounding memory.
	recMaxShapePools = 64
	// recShapePoolCap caps idle sessions retained per width; extras are
	// Destroyed on release instead of pooled.
	recShapePoolCap = 4
)

// recSessions is the dynamic-width OCR-rec pool: bounded at recMaxShapePools
// distinct width-pools, each retaining up to recShapePoolCap idle sessions.
var recSessions = newSessionPool[recKey, *recSession](recMaxShapePools, recShapePoolCap)

// getRecSession returns a reusable dynamic-width OCR-rec session for the given
// input width plus a release func. The caller must call release exactly once.
func getRecSession(modelPath, inName string, inShape []int64, outName string, intraOpThreads int) (*recSession, func(), error) {
	key := recKeyOf(modelPath, inName, inShape, outName, intraOpThreads)
	return recSessions.Get(key, func() (*recSession, error) {
		return newRecSession(modelPath, inName, inShape, outName, intraOpThreads)
	})
}
