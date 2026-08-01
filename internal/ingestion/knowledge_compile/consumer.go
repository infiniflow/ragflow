//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package knowledge_compile

import (
	"context"
	"sync"
	"time"

	"ragflow/internal/engine"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Consumer is the dataset-level post-processing worker (§11.5). Multiple
// instances compete on the MySQL scheduling rows; each KB is processed by at
// most one instance at a time via the per-KB claim (so the same KB is handled
// by a single worker that serializes the batch). The MySQL row — not the
// broker — is the scheduling system of record and the source of same-KB
// serialization.
type Consumer struct {
	scheduler Claimer
	reader    Reader
	writer    Writer
	factory   DeduperFactory

	ttl            time.Duration
	heartbeat      time.Duration
	pollInterval   time.Duration
	sweepInterval  time.Duration
	mergeThreshold float64 // KNN similarity threshold for "existing merged row is a duplicate"

	mu    sync.Mutex
	seqs  map[string]map[string]uint64 // dataset -> docID -> last applied seq (per-doc out-of-order guard)
	tombs map[string]map[string]uint64 // dataset -> docID -> delete event seq (tombstone)
}

// NewConsumer constructs a Consumer driven by the given Claimer. Tests pass a
// FakeScheduler and override the Reader/Writer/Deduper via options.
func NewConsumer(scheduler Claimer, opts ...Option) *Consumer {
	c := &Consumer{
		scheduler:      scheduler,
		reader:         engineReader{eng: engine.Get()},
		writer:         engineWriter{eng: engine.Get()},
		factory:        defaultDeduperFactory,
		ttl:            2 * time.Minute,
		heartbeat:      20 * time.Second,
		pollInterval:   2 * time.Second,
		sweepInterval:  30 * time.Second,
		mergeThreshold: 0.99,
		seqs:           map[string]map[string]uint64{},
		tombs:          map[string]map[string]uint64{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Run is one owned worker loop (Option E §11.5/§11.7): it wakes on NATS notify
// and otherwise polls the scheduling table for a claimable KB, then claims the
// closed batch, processes it, and acks. It returns when ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	if c.scheduler == nil {
		return
	}
	notifyCh, _ := c.scheduler.SubscribeNotify(ctx)
	poll := time.NewTicker(c.pollInterval)
	sweep := time.NewTicker(c.sweepInterval)
	defer poll.Stop()
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case datasetID, ok := <-notifyCh:
			if !ok {
				notifyCh = nil
				continue
			}
			// The notify carries the dataset that just received backlog, so
			// claim that specific dataset directly instead of probing for an
			// arbitrary claimable one.
			c.claimAndProcess(ctx, datasetID)
		case <-poll.C:
			c.tryClaimAndProcess(ctx)
		case <-sweep.C:
			// Crash recovery: TryClaim reclaims expired inflight leases
			// (§11.5) before claiming any ready batch.
			c.tryClaimAndProcess(ctx)
		}
	}
}

// claimAndProcess claims the given dataset (a notify pointed at it) and processes
// the closed batch. ok=false means the dataset has no ready batch or a live lease
// already holds it (the race was lost).
func (c *Consumer) claimAndProcess(ctx context.Context, datasetID string) {
	cr, ok, err := c.scheduler.Claim(ctx, datasetID)
	if err != nil || !ok || len(cr.Entries) == 0 {
		return
	}
	c.processClaim(ctx, cr)
}

// tryClaimAndProcess claims one closed batch (ready or reclaimed) and processes
// it. ok=false means there was nothing to do this tick.
func (c *Consumer) tryClaimAndProcess(ctx context.Context) {
	cr, ok, err := c.scheduler.TryClaim(ctx)
	if err != nil || !ok || len(cr.Entries) == 0 {
		return // nothing to claim, or the race was lost
	}
	c.processClaim(ctx, cr)
}

// processClaim processes an already-claimed batch (cr) and acks on success. It
// is the Option E replacement for the old processOnce (lease + drain + merge):
// the claim returned a frozen batch boundary, so there is no moving target and
// no Nak-churn routing.
func (c *Consumer) processClaim(ctx context.Context, cr ClaimResult) {
	datasetID := cr.DatasetID

	// Heartbeat refreshes the claim TTL while we process; a failed touch means
	// the lease was taken over (or reclaimed) and we must abort without acking.
	stopHb := make(chan struct{})
	hbFailed := make(chan struct{}, 1)
	go func() {
		t := time.NewTicker(c.heartbeat)
		defer t.Stop()
		for {
			select {
			case <-stopHb:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				alive, e := c.scheduler.TouchClaim(ctx, datasetID, cr.Token, c.ttl)
				if e != nil || !alive {
					select {
					case hbFailed <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}()

	done := make(chan struct{})
	var batchErr error
	go func() {
		defer close(done)
		batchErr = c.processBatch(ctx, cr.TenantID, datasetID, cr.Entries)
	}()

	select {
	case <-ctx.Done():
		close(stopHb)
		<-done
		return // graceful shutdown: leave inflight for reclamation, do not ack
	case <-hbFailed:
		close(stopHb)
		<-done
		return // lease lost: do not ack; sweeper/redelivery reprocesses (idempotent)
	case <-done:
		close(stopHb)
	}

	// Ack only on success. A batch error (reader/dedup/writer failure) means we
	// must leave the claimed batch in the backlog for reclamation/retry rather
	// than silently dropping it (C5: never ack what we failed to merge).
	if batchErr != nil {
		return
	}
	if _, err := c.scheduler.Ack(ctx, datasetID, cr.Token, cr.Entries); err != nil {
		_ = err
	}
}

// processBatch applies out-of-order / tombstone handling, then recomputes and
// writes the dataset-level merged products for the claimed closed batch. It
// returns an error if any reader/dedup/writer step fails so the caller can
// leave the batch for reclamation instead of acking dropped work.
func (c *Consumer) processBatch(ctx context.Context, tenant, kb string, entries []BacklogEntry) error {
	c.mu.Lock()
	if c.tombs == nil {
		c.tombs = map[string]map[string]uint64{}
	}
	if c.seqs == nil {
		c.seqs = map[string]map[string]uint64{}
	}
	tomb := c.tombs[kb]
	docSeqs := c.seqs[kb]
	if docSeqs == nil {
		docSeqs = map[string]uint64{}
		c.seqs[kb] = docSeqs
	}
	var completed []BacklogEntry
	var deleted []string
	// First pass: record every deletion so the tombstone is complete before we
	// judge completions. Without this, a completion published before its
	// deletion in the same batch would be accepted even though the final event
	// is a delete (deletion must win regardless of batch order).
	for _, e := range entries {
		if EventType(e.EventType) == EventTypeDeleted {
			if tomb == nil {
				tomb = map[string]uint64{}
				c.tombs[kb] = tomb
			}
			tomb[e.DocID] = e.Seq
			deleted = append(deleted, e.DocID)
		}
	}
	for _, e := range entries {
		if EventType(e.EventType) != EventTypeCompleted {
			continue
		}
		// A tombstone means the doc was deleted; a completion with a seq
		// <= the delete seq is the stale original completion (skip it).
		// A completion with a higher seq is a genuine re-ingest after
		// deletion: clear the tombstone so the doc is processed again
		// (otherwise the tombstone would skip it forever and grow
		// unbounded across the consumer's lifetime).
		if delSeq, ok := tomb[e.DocID]; ok {
			if e.Seq <= delSeq {
				continue // deleted (or a stale completion) before it completed
			}
			delete(tomb, e.DocID)
		}
		// Seq is per-document, so the stale/duplicate check must be scoped
		// to the document, not the whole dataset (C4).
		if prev, ok := docSeqs[e.DocID]; ok && e.Seq <= prev {
			continue // stale / duplicate completion for this doc
		}
		docSeqs[e.DocID] = e.Seq
		completed = append(completed, e)
	}
	c.mu.Unlock()

	if len(deleted) == 0 && len(completed) == 0 {
		return nil
	}

	deduper, err := c.factory(tenant)
	if err != nil || deduper == nil {
		deduper = NewNoopDeduper()
	}

	completedSet := make(map[string]bool, len(completed))
	for _, e := range completed {
		completedSet[e.DocID] = true
	}
	deletedSet := make(map[string]bool, len(deleted))
	for _, d := range deleted {
		deletedSet[d] = true
	}

	// --- Deletion (two sequential DocEngine calls, no in-memory load) ---
	// Deleted wins regardless of batch order, so we process deletions first.
	// The deleted docs' products are never loaded into memory: the DocEngine
	// does all the work in two calls:
	//   1. DeleteDocLevelForDocs drops every per-document (doc-level) product of
	//      the deleted docs in a single engine call.
	//   2. StripMergedSources removes the deleted doc ids from the source_doc_ids
	//      array of every dataset-level merged product and deletes any product
	//      whose array became empty.
	// A merged product referencing several deleted docs is pruned in one pass,
	// and an emptied product is removed exactly once.
	if len(deleted) > 0 {
		delIDs := make([]string, 0, len(deletedSet))
		for d := range deletedSet {
			delIDs = append(delIDs, d)
		}
		if err := c.writer.DeleteDocLevelForDocs(ctx, tenant, kb, delIDs); err != nil {
			return err
		}
		if err := c.writer.StripMergedSources(ctx, tenant, kb, delIDs); err != nil {
			return err
		}
	}

	// --- Completion merge ---
	// Load only the per-document products of the completed (and not deleted)
	// docs — bounded by this batch, never the whole KB. A doc that is both
	// completed and deleted is a stale tombstone: the deletion wins, so we skip
	// its completion.
	var incoming []kccommon.Product
	for _, e := range completed {
		if deletedSet[e.DocID] {
			continue
		}
		docProducts, err := c.reader.LoadDocProducts(ctx, tenant, kb, e.DocID)
		if err != nil {
			return err
		}
		incoming = append(incoming, docProducts...)
	}

	// In-memory dedup among the completed batch first.
	candidates, err := deduper.Dedup(ctx, incoming)
	if err != nil {
		return err
	}

	// Then dedup each candidate against the DocEngine via KNN top1 + LLM judge,
	// mirroring Python _struct_doc_storage_dedup_batch. Candidates that KNN-hit
	// the same existing merged row are grouped so the row is merged/updated once
	// instead of being rewritten per candidate.
	type mergeGroup struct {
		existing   kccommon.Product
		candidates []kccommon.Product
		score      float64
	}
	groupsByID := make(map[string]*mergeGroup, len(candidates))
	var (
		unmatchedMu sync.Mutex
		unmatched   []kccommon.Product
		groupsMu    sync.Mutex
	)
	// The KNN pass is docengine-bounded (vector search), not CPU-bounded, so we
	// fan it out across the shared global compilerPool (vCPU-sized). Output order
	// is irrelevant: merged rows are upserted by their idempotent dataset-level
	// id, and each candidate lands in exactly one group / the unmatched set.
	jobs := make([]compilerJob, 0, len(candidates))
	for _, cand := range candidates {
		cand := cand
		jobs = append(jobs, func() error {
			var vec64 []float64
			if len(cand.Vector) > 0 {
				vec64 = make([]float64, len(cand.Vector))
				for i, v := range cand.Vector {
					vec64[i] = float64(v)
				}
			}
			hit, score, err := c.reader.SearchSimilar(ctx, tenant, kb, cand.Variant, vec64, 1, c.mergeThreshold)
			if err != nil {
				return err
			}
			if hit.ID == "" {
				// No sufficiently-similar merged row: insert the candidate as a new
				// merged row.
				cand.Merged = true
				cand.DocID = kb
				unmatchedMu.Lock()
				unmatched = append(unmatched, cand)
				unmatchedMu.Unlock()
				return nil
			}
			groupsMu.Lock()
			g := groupsByID[hit.ID]
			if g == nil {
				g = &mergeGroup{existing: hit, score: score}
				groupsByID[hit.ID] = g
			}
			g.candidates = append(g.candidates, cand)
			groupsMu.Unlock()
			return nil
		})
	}
	if err := runCompilerJobs(ctx, jobs); err != nil {
		return err
	}

	// Fold every KNN group into the LLM in a single batch round-trip (one
	// DecideBatch call instead of one Decide per pair), then collect the
	// updated existing rows and the candidates judged distinct (new rows).
	var newMerged []kccommon.Product
	if len(groupsByID) > 0 {
		batched := make([]MergeGroup, 0, len(groupsByID))
		for _, g := range groupsByID {
			batched = append(batched, MergeGroup{
				Existing:   g.existing,
				Candidates: g.candidates,
				Score:      g.score,
			})
		}
		batched, err = deduper.DecideBatch(ctx, batched)
		if err != nil {
			return err
		}
		for _, g := range batched {
			newMerged = append(newMerged, g.Merged)
			unmatched = append(unmatched, g.Distinct...)
		}
	}

	// Write the surviving merged set (updated existing + new distinct rows).
	mergedFinal := make([]kccommon.Product, 0, len(newMerged)+len(unmatched))
	mergedFinal = append(mergedFinal, newMerged...)
	mergedFinal = append(mergedFinal, unmatched...)
	if err := c.writer.WriteMerged(ctx, tenant, kb, mergedFinal); err != nil {
		return err
	}
	return nil
}
