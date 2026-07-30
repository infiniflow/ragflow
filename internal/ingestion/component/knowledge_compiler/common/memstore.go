package common

import (
	"math"
	"sort"
	"sync"
)

// Hit is one TopK match returned by MemStore.
type Hit struct {
	Index int
	ID    string
	Score float64 // cosine similarity in [−1, 1]
}

// MemStore is an in-memory product store with exact-cosine TopK retrieval.
// It is the single source of truth for in-run dedup (replacing Python's ES
// KNN over the current run's products). Vectors are kept alongside precomputed
// L2 norms so TopK is a single matVec pass.
type MemStore struct {
	mu      sync.RWMutex
	items   []Product
	vectors [][]float32
	norms   []float64
	byID    map[string]int
}

// NewMemStore constructs an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{byID: make(map[string]int)}
}

// KeepAction is the decision returned by a DedupCallback during DedupeAdd.
type KeepAction int

const (
	// KeepAdd inserts the incoming product as a new entry.
	KeepAdd KeepAction = iota
	// KeepDrop discards the incoming product (it is a duplicate).
	KeepDrop
	// KeepMerge overwrites the best existing product with the incoming one
	// (identity preserved via the existing id).
	KeepMerge
)

// DedupCallback decides, given the best existing match (score is the cosine
// similarity in [−1, 1]; existing is the zero Product when no candidate
// qualifies), whether to add, drop, or merge the incoming product. When it
// returns KeepMerge, the returned Product replaces the existing entry — its
// ID is forced to the existing entry's ID so merges preserve identity
// (mirrors Python's _struct_rebuild_doc_storage_doc preserve_id=True).
type DedupCallback func(existing Product, score float64) (KeepAction, Product, error)

// Add appends a product and its vector to the store.
func (m *MemStore) Add(p Product) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addLocked(p)
}

// DedupeAdd atomically checks the store for a near-duplicate of row and acts on
// the DedupCallback's decision. The callback is invoked OUTSIDE the store lock
// so it may perform slow work (e.g. an LLM merge judgment) without blocking
// concurrent callers. The check-then-act is race-free: the best candidate is
// selected under a read lock, the callback runs unlocked, then the add/merge is
// applied under a write lock using the previously resolved index.
func (m *MemStore) DedupeAdd(row Product, threshold float64, cb DedupCallback) (KeepAction, error) {
	m.mu.RLock()
	qn := l2Norm(row.Vector)
	if qn == 0 {
		qn = 1
	}
	bestIdx, bestScore := -1, 0.0
	for i, v := range m.vectors {
		vn := m.norms[i]
		if vn == 0 {
			vn = 1
		}
		score := dotProduct(row.Vector, v) / (qn * vn)
		if threshold <= 0 || score >= threshold {
			if bestIdx == -1 || score > bestScore {
				bestIdx, bestScore = i, score
			}
		}
	}
	var best Product
	if bestIdx >= 0 && bestIdx < len(m.items) {
		best = m.items[bestIdx]
	}
	m.mu.RUnlock()

	action, replacement, err := cb(best, bestScore)
	if err != nil {
		return 0, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-resolve the candidate by ID under the write lock. The unlocked
	// callback may have allowed a concurrent Delete to delete or reindex the
	// slice, so the index captured under the read lock (bestIdx) is stale and
	// unsafe to index — it could point at a different product or be
	// out-of-range (M11).
	resolvedIdx := -1
	if best.ID != "" {
		if idx, ok := m.byID[best.ID]; ok && idx < len(m.items) && m.items[idx].ID == best.ID {
			resolvedIdx = idx
		}
	}
	switch action {
	case KeepDrop:
		return KeepDrop, nil
	case KeepMerge:
		if resolvedIdx < 0 {
			m.addLocked(row)
			return KeepAdd, nil
		}
		// Merges preserve the existing entry's identity (Python preserve_id).
		replacement.ID = m.items[resolvedIdx].ID
		m.items[resolvedIdx] = replacement
		m.vectors[resolvedIdx] = replacement.Vector
		m.norms[resolvedIdx] = l2Norm(replacement.Vector)
		return KeepMerge, nil
	default:
		m.addLocked(row)
		return KeepAdd, nil
	}
}

func (m *MemStore) addLocked(p Product) {
	m.items = append(m.items, p)
	m.vectors = append(m.vectors, p.Vector)
	m.norms = append(m.norms, l2Norm(p.Vector))
	if p.ID != "" {
		m.byID[p.ID] = len(m.items) - 1
	}
}

// Upsert replaces an existing product by ID, or appends if absent.
func (m *MemStore) Upsert(p Product) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx, ok := m.byID[p.ID]; ok {
		m.items[idx] = p
		m.vectors[idx] = p.Vector
		m.norms[idx] = l2Norm(p.Vector)
		return
	}
	m.items = append(m.items, p)
	m.vectors = append(m.vectors, p.Vector)
	m.norms = append(m.norms, l2Norm(p.Vector))
	m.byID[p.ID] = len(m.items) - 1
}

// Delete removes a product by ID.
func (m *MemStore) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.byID[id]
	if !ok {
		return
	}
	m.items = append(m.items[:idx], m.items[idx+1:]...)
	m.vectors = append(m.vectors[:idx], m.vectors[idx+1:]...)
	m.norms = append(m.norms[:idx], m.norms[idx+1:]...)
	delete(m.byID, id)
	m.reindexLocked()
}

func (m *MemStore) reindexLocked() {
	m.byID = make(map[string]int, len(m.items))
	for i, it := range m.items {
		if it.ID != "" {
			m.byID[it.ID] = i
		}
	}
}

// Len returns the number of stored products.
func (m *MemStore) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// TopK returns up to k products whose cosine similarity to vec is >= threshold,
// sorted by descending similarity. A zero threshold returns the top-k by score
// regardless of absolute value.
func (m *MemStore) TopK(vec []float32, k int, threshold float64) []Hit {
	m.mu.RLock()
	defer m.mu.RUnlock()
	qn := l2Norm(vec)
	if qn == 0 {
		qn = 1
	}
	type cand struct {
		idx   int
		score float64
	}
	var cands []cand
	for i, v := range m.vectors {
		vn := m.norms[i]
		if vn == 0 {
			vn = 1
		}
		score := dotProduct(vec, v) / (qn * vn)
		if threshold <= 0 || score >= threshold {
			cands = append(cands, cand{i, score})
		}
	}
	sort.Slice(cands, func(a, b int) bool { return cands[a].score > cands[b].score })
	if k > 0 && len(cands) > k {
		cands = cands[:k]
	}
	hits := make([]Hit, 0, len(cands))
	for _, c := range cands {
		hits = append(hits, Hit{Index: c.idx, ID: m.items[c.idx].ID, Score: c.score})
	}
	return hits
}

// Snapshot returns a shallow copy of the stored products.
func (m *MemStore) Snapshot() []Product {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Product, len(m.items))
	copy(out, m.items)
	return out
}

// MergeSourceChunkIDs unions the given chunk ids into the source_chunk_ids
// list of the entry identified by id. Used by structure's dedup path to fold
// a dropped duplicate's provenance into the surviving canonical entity
// (mirrors Python's _struct_merge_graph_entities). No-op when id is absent.
func (m *MemStore) MergeSourceChunkIDs(id string, chunkIDs []string) {
	if id == "" || len(chunkIDs) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.byID[id]
	if !ok {
		return
	}
	p := m.items[idx]
	if p.Meta == nil {
		p.Meta = map[string]any{}
	}
	existing, _ := p.Meta["source_chunk_ids"].([]string)
	seen := map[string]bool{}
	for _, s := range existing {
		seen[s] = true
	}
	for _, s := range chunkIDs {
		if !seen[s] {
			existing = append(existing, s)
			seen[s] = true
		}
	}
	p.Meta["source_chunk_ids"] = existing
	m.items[idx] = p
}

func dotProduct(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float64
	for i := 0; i < n; i++ {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func l2Norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}
