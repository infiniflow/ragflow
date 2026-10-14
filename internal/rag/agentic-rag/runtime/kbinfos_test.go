package runtime

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMergeSkipsAllWhenNoChunks: early return:
// `if not result or not result.get("chunks"): return`. When the incoming chunk
// list is empty, neither chunks nor doc_aggs are merged, and the call returns
// nil (no global indices contributed).
func TestMergeSkipsAllWhenNoChunks(t *testing.T) {
	kb := &Kbinfos{DocAggs: []map[string]any{{"doc_id": "d1"}}}
	added := kb.Merge(nil, []map[string]any{{"doc_id": "d2"}})
	if added != nil {
		t.Fatalf("Merge with no chunks should return nil, got %v", added)
	}
	if len(kb.DocAggs) != 1 {
		t.Fatalf("doc_aggs must not be merged without chunks, got %d", len(kb.DocAggs))
	}
	if len(kb.Chunks) != 0 {
		t.Fatalf("chunks should stay empty, got %d", len(kb.Chunks))
	}
}

// TestMergeCollapsesEmptyDocIDAggs: the dedup set is built from the raw doc_id value, so a
// missing/empty doc_id still enters the set and a second empty-doc_id agg is dropped. Only the
// first is kept.
func TestMergeCollapsesEmptyDocIDAggs(t *testing.T) {
	kb := &Kbinfos{}
	added := kb.Merge(
		[]map[string]any{{"id": "c1"}},
		[]map[string]any{{"name": "a"}, {"name": "b"}, {"name": "c"}},
	)
	if len(added) != 1 {
		t.Fatalf("added = %v, want 1 chunk index", added)
	}
	if len(kb.DocAggs) != 1 {
		t.Fatalf("empty-doc_id aggs should collapse to 1, got %d", len(kb.DocAggs))
	}
	if kb.DocAggs[0]["name"] != "a" {
		t.Fatalf("first empty-doc_id agg should be kept, got %v", kb.DocAggs[0])
	}
}

// TestMergeDedupsSameDocID covers the exact-match dedup for real doc_ids, independent of the
// empty bucket above.
func TestMergeDedupsSameDocID(t *testing.T) {
	kb := &Kbinfos{}
	kb.Merge([]map[string]any{{"id": "c1"}}, []map[string]any{{"doc_id": "x"}, {"doc_id": "x"}})
	if len(kb.DocAggs) != 1 {
		t.Fatalf("same-doc_id aggs should dedup, got %d", len(kb.DocAggs))
	}
}

// TestChunkKeyUsesTextFallback pins that the dedup key reads the SAME alias chain
// as chunkText (content_with_weight -> content -> text). Reading only the first
// two made two id-less, text-only chunks from one document share the doc-level
// fallback key, so Merge/MemoryAdd discarded distinct evidence.
func TestChunkKeyUsesTextFallback(t *testing.T) {
	a := map[string]any{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"}
	b := map[string]any{"text": "beta", "doc_id": "d1", "docnm_kwd": "doc"}
	if chunkKey(a) == chunkKey(b) {
		t.Fatalf("distinct text-only chunks must not share a key: %q", chunkKey(a))
	}
	// The same text under any alias must produce the same key.
	if c := map[string]any{"content": "alpha"}; chunkKey(a) != chunkKey(c) {
		t.Errorf("text and content aliases must agree: %q vs %q", chunkKey(a), chunkKey(c))
	}
	// A chunk with no text at all still lands on the doc-level fallback.
	if got := chunkKey(map[string]any{"doc_id": "d1", "docnm_kwd": "doc"}); got == "" {
		t.Error("doc-level fallback key must not be empty")
	}
}

// TestMergeKeepsDistinctTextOnlyChunks pins the end-to-end consequence for the
// evidence pool: two id-less chunks that share only a document must both survive.
func TestMergeKeepsDistinctTextOnlyChunks(t *testing.T) {
	kb := &Kbinfos{}
	added := kb.Merge([]map[string]any{
		{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"},
		{"text": "beta", "doc_id": "d1", "docnm_kwd": "doc"},
	}, nil)
	if len(kb.Chunks) != 2 {
		t.Fatalf("chunks = %d, want 2 (distinct text-only evidence must not collapse)", len(kb.Chunks))
	}
	if len(added) != 2 {
		t.Fatalf("added = %v, want 2 global indices", added)
	}
	// Re-merging identical content still dedups.
	kb.Merge([]map[string]any{{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"}}, nil)
	if len(kb.Chunks) != 2 {
		t.Fatalf("identical text must still dedup, chunks = %d", len(kb.Chunks))
	}
}

// TestMemoryAddKeepsDistinctTextOnlyChunks pins the same consequence for the
// lossless memory store.
func TestMemoryAddKeepsDistinctTextOnlyChunks(t *testing.T) {
	kb := &Kbinfos{}
	MemoryAdd(kb, []map[string]any{
		{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"},
		{"text": "beta", "doc_id": "d1", "docnm_kwd": "doc"},
	})
	if len(kb.Memory) != 2 {
		t.Fatalf("memory = %d, want 2 (distinct text-only evidence must not collapse)", len(kb.Memory))
	}
	MemoryAdd(kb, []map[string]any{{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"}})
	if len(kb.Memory) != 2 {
		t.Fatalf("identical text must still dedup, memory = %d", len(kb.Memory))
	}
}

// TestKbinfosAdmitIsAtomic pins the pool's invariants under the concurrency the
// pool actually sees: one round's sessions run at the same time and share ONE
// Kbinfos (SessionDeps.KB; RunSlotResearchPass starts them as goroutines).
//
// Both invariants used to come for free from cooperative scheduling — an await-free admit
// stretch cannot be preempted — so the same stretch has to be locked here.
//
// The pool starts one chunk short of the cap, and both sessions offer the SAME
// first chunk. Whichever batch wins the lock pools it — once — and the cap then
// stops every further chunk, so the assertions hold whatever the schedule does:
// they fail only when the batches interleave (which is what an unlocked
// check-then-append does: both sessions read 59 and both append).
func TestKbinfosAdmitIsAtomic(t *testing.T) {
	pool := &Kbinfos{}
	for i := 0; i < evidencePoolCap-1; i++ {
		pool.Admit(func(p *PoolAdmitter) {
			p.Add(map[string]any{"chunk_id": fmt.Sprintf("p%d", i)})
		})
	}
	if len(pool.Chunks) != evidencePoolCap-1 {
		t.Fatalf("fixture pool = %d chunks, want %d", len(pool.Chunks), evidencePoolCap-1)
	}

	var wg sync.WaitGroup
	for _, tag := range []string{"a", "b"} {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()
			pool.Admit(func(p *PoolAdmitter) {
				for _, c := range []map[string]any{
					{"chunk_id": "shared"},
					{"chunk_id": tag},
				} {
					if p.Full() {
						continue
					}
					p.Add(c)
				}
			})
		}(tag)
	}
	wg.Wait()

	if len(pool.Chunks) != evidencePoolCap {
		t.Errorf("pool = %d chunks, want exactly the cap %d: a serialized batch stops at the cap, an interleaved one overshoots",
			len(pool.Chunks), evidencePoolCap)
	}
	counts := map[string]int{}
	for _, c := range pool.Chunks {
		id, _ := c["chunk_id"].(string)
		counts[id]++
	}
	if counts["shared"] != 1 {
		t.Errorf("chunk %q pooled %d time(s), want exactly 1: the pool must be deduped against its LIVE contents", "shared", counts["shared"])
	}
	if counts["a"]+counts["b"] != 0 {
		t.Errorf("session-private chunks were pooled (%d a, %d b) although only one slot was free", counts["a"], counts["b"])
	}
}

// TestToolCacheIsConcurrencySafe exercises the cache the way a round does: the
// SAME instance handed to concurrent sessions (one cache per round, passed to every session).
// Run under -race this is what a bare map cannot survive — concurrent map writes
// are a fatal error in Go, not merely a race report.
func TestToolCacheIsConcurrencySafe(t *testing.T) {
	cache := NewToolCache()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("tool-%d", i%4) // deliberate key collisions
			if _, ok := cache.Get(key); !ok {
				cache.Put(key, ToolOutcome{Reason: key})
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < 4; i++ {
		key := fmt.Sprintf("tool-%d", i)
		oc, ok := cache.Get(key)
		if !ok || oc.Reason != key {
			t.Errorf("cache[%q] = (%+v, %v), want the stored outcome", key, oc, ok)
		}
	}
}

// TestPoolNoveltyExemptsOnlyTheUnansweredProbeTerms pins the cap EXEMPTION: a
// FULL pool still takes the passage that answers a probe term nothing in the pool
// has reached yet — one seat per term — and refuses everything else exactly as the
// cap always did.
//
// The reason the exemption exists is that a probe's per-name window is the batch's
// RESULT rather than one more passage: drop it and "this name was found here"
// degrades into "nothing new", which the model reads as "not a member".
func TestPoolNoveltyExemptsOnlyTheUnansweredProbeTerms(t *testing.T) {
	newFullPool := func() *Kbinfos {
		pool := &Kbinfos{}
		for i := 0; i < evidencePoolCap; i++ {
			pool.Admit(func(p *PoolAdmitter) {
				p.Add(map[string]any{"chunk_id": fmt.Sprintf("p%d", i), "content": "already pooled prose"})
			})
		}
		return pool
	}
	admit := func(pool *Kbinfos, q string, chunks ...map[string]any) (admitted int) {
		pool.Admit(func(p *PoolAdmitter) {
			novel := p.Novelty(probeTerms(q))
			for _, c := range chunks {
				if p.Full() && !novel.Admits(c) {
					continue
				}
				if p.Add(c) {
					admitted++
				}
			}
		})
		return admitted
	}

	// A probe whose name the pool cannot answer: the window is admitted.
	pool := newFullPool()
	window := map[string]any{"chunk_id": "w1", "content": "荀正 被关公一刀斩于马下"}
	if got := admit(pool, "车胄|荀正|管亥", window); got != 1 {
		t.Fatalf("admitted = %d, want 1 (the pool cannot answer 荀正)", got)
	}
	if len(pool.Chunks) != evidencePoolCap+1 {
		t.Fatalf("pool = %d, want %d (one seat for one unanswered term)", len(pool.Chunks), evidencePoolCap+1)
	}
	// The same term is answered now: a second window carrying only it is refused,
	// so one term buys one seat and not a batch of them.
	if got := admit(pool, "车胄|荀正|管亥", map[string]any{"chunk_id": "w2", "content": "荀正 又出现了一次"}); got != 0 {
		t.Fatalf("admitted = %d, want 0: 荀正 is answered now", got)
	}
	// A query that is not a PROBE states no list of individuals, so it gets no
	// exemption at all — the plain cap behaviour.
	if got := admit(pool, "车胄", map[string]any{"chunk_id": "w3", "content": "车胄 守徐州"}); got != 0 {
		t.Fatalf("admitted = %d, want 0: a non-probe query gets no exemption", got)
	}
	if got := admit(pool, "车胄|管亥", map[string]any{"chunk_id": "w4", "content": "管亥 围北海"}); got != 1 {
		t.Fatalf("admitted = %d, want 1 (管亥 is still unanswered)", got)
	}
	// A pool that already carries the term leaves the cap in charge.
	carried := newFullPool()
	carried.Admit(func(p *PoolAdmitter) { p.Add(map[string]any{"chunk_id": "seed", "content": "管亥 围北海"}) })
	if got := admit(carried, "管亥", map[string]any{"chunk_id": "w5", "content": "管亥 围北海 续"}); got != 0 {
		t.Fatalf("admitted = %d, want 0: the pool already carries 管亥", got)
	}
	_ = carried

	// The exemption is BOUNDED: past cap+slack even an unanswered term is refused.
	bounded := newFullPool()
	bounded.novelAdmitted = evidencePoolNoveltySlack
	bounded.Chunks = append(bounded.Chunks, make([]map[string]any, evidencePoolNoveltySlack)...)
	if got := admit(bounded, "杨龄", map[string]any{"chunk_id": "w6", "content": "杨龄 出战"}); got != 0 {
		t.Fatalf("admitted = %d, want 0: the novelty slack is exhausted", got)
	}
}

// TestReachedTermsLedgerHoldsTheMembersWithTheirEvidence pins the confirmed-member
// ledger: it is what the rewrite context renders (the passage behind each name) and
// what lets the loop ask whether the LIST is growing rather than whether the pool
// got bigger.
func TestReachedTermsLedgerHoldsTheMembersWithTheirEvidence(t *testing.T) {
	pool := &Kbinfos{}
	pool.RecordReachedTerm("荀正", "w1")
	pool.RecordReachedTerm("杨龄", "w2")
	pool.RecordReachedTerm("荀正", "w3") // same name, later passage: must not duplicate

	got := pool.ReachedTerms()
	if len(got) != 2 {
		t.Fatalf("ReachedTerms = %v, want one entry per name", got)
	}
	if got[0].Term != "荀正" || got[0].ChunkID != "w1" {
		t.Errorf("first entry = %+v, want the name with the passage that proved it", got[0])
	}
	if got[1].Term != "杨龄" || got[1].ChunkID != "w2" {
		t.Errorf("second entry = %+v", got[1])
	}
	// Blank input records nothing (a seat without an id is not evidence).
	pool.RecordReachedTerm("", "w4")
	pool.RecordReachedTerm("管亥", "")
	if n := len(pool.ReachedTerms()); n != 2 {
		t.Errorf("entries = %d, want 2: an entry needs both a name and its passage", n)
	}
	// The ledger is bounded, and a nil pool is safe (the graph may run without one).
	for i := 0; i < reachedTermsMax+5; i++ {
		pool.RecordReachedTerm(fmt.Sprintf("n%d", i), fmt.Sprintf("c%d", i))
	}
	if n := len(pool.ReachedTerms()); n != reachedTermsMax {
		t.Errorf("entries = %d, want the cap %d", n, reachedTermsMax)
	}
	var nilPool *Kbinfos
	nilPool.RecordReachedTerm("x", "y")
	if len(nilPool.ReachedTerms()) != 0 {
		t.Error("a nil pool must record nothing")
	}
}

// TestRecordingInsideTheAdmitBatchDoesNotDeadlock pins the hardened ledger: the
// search record lives behind its OWN mutex, so a batch that answered a probe can
// write the answer down while the pool lock is held.
//
// This is the exact shape that hung a run (fixrecall2, 2026-09-15): the recorder
// used to take the same mutex Admit holds, and because Go mutexes are not
// reentrant the goroutine parked on a lock it already held — no I/O, no CPU, no
// log line, and a mutex wait ignores context cancellation, so the run never
// returned. The timeout here turns that hang into a failure message.
func TestRecordingInsideTheAdmitBatchDoesNotDeadlock(t *testing.T) {
	k := &Kbinfos{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		k.Admit(func(p *PoolAdmitter) {
			p.Add(map[string]any{"chunk_id": "w1", "content": "荀正 被关公一刀斩于马下"})
			// Both recorders run under the pool lock on purpose.
			k.RecordReachedTerm("荀正", "w1")
			k.RecordProbedAbsent("管亥")
		})
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("recording from inside the admit batch deadlocked: the ledger must not share the pool's mutex")
	}

	if got := k.ReachedTerms(); len(got) != 1 || got[0].Term != "荀正" || got[0].ChunkID != "w1" {
		t.Errorf("ReachedTerms = %+v, want the member recorded with its passage", got)
	}
	if absent := k.ProbedAbsentTerms(); len(absent) != 1 || absent[0] != "管亥" {
		t.Errorf("ProbedAbsent = %v, want 管亥", absent)
	}
	// The pool itself is unaffected by the ledger's separate lock.
	if len(k.Chunks) != 1 {
		t.Errorf("pool = %d chunk(s), want the admitted passage", len(k.Chunks))
	}
}

// TestLedgerAccessorsAreIndependentOfThePoolLock pins the other direction: a
// reader of the record never waits for a batch that is in flight, which is what
// lets the rewrite context be rendered while sessions are still admitting.
func TestLedgerAccessorsAreIndependentOfThePoolLock(t *testing.T) {
	k := &Kbinfos{}
	k.RecordReachedTerm("华雄", "w1")

	released := make(chan struct{})
	go func() {
		k.Admit(func(p *PoolAdmitter) {
			<-released // hold the pool lock until the reads below are done
		})
	}()
	// Give the batch a moment to take the pool lock.
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = k.ReachedTerms()
		_ = k.ProbedAbsentTerms()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		close(released)
		t.Fatal("reading the record blocked on the pool lock: the accessors must not need it")
	}
	close(released)
}
