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
)

// Consumer is the dataset-level post-processing worker (§11.5). Multiple
// instances compete on the MySQL scheduling rows; each KB is processed by at
// most one instance at a time via the per-KB claim (so the same KB is handled
// by a single worker that serializes the batch). The MySQL row — not the
// broker — is the scheduling system of record and the source of same-KB
// serialization.
type Consumer struct {
	scheduler Scheduler
	reader    Reader
	writer    Writer
	factory   DeduperFactory

	batchSize     int
	ttl           time.Duration
	heartbeat     time.Duration
	pollInterval  time.Duration
	sweepInterval time.Duration

	mu    sync.Mutex
	seqs  map[string]map[string]uint64 // dataset -> docID -> last applied seq (per-doc out-of-order guard)
	tombs map[string]map[string]uint64 // dataset -> docID -> delete event seq (tombstone)
}

// NewConsumer constructs a Consumer driven by the given Scheduler. Tests pass a
// FakeScheduler and override the Reader/Writer/Deduper via options.
func NewConsumer(scheduler Scheduler, opts ...Option) *Consumer {
	c := &Consumer{
		scheduler:     scheduler,
		reader:        infinityReader{},
		writer:        infinityWriter{},
		factory:       defaultDeduperFactory,
		batchSize:     32,
		ttl:           2 * time.Minute,
		heartbeat:     20 * time.Second,
		pollInterval:  2 * time.Second,
		sweepInterval: 30 * time.Second,
		seqs:          map[string]map[string]uint64{},
		tombs:         map[string]map[string]uint64{},
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
		case ds := <-notifyCh:
			if ds != "" {
				c.processDataset(ctx, ds)
			}
		case <-poll.C:
			c.claimOne(ctx)
		case <-sweep.C:
			// Recover inflight left by crashed workers (crash recovery, §11.5).
			if _, err := c.scheduler.ReclaimExpired(ctx, time.Now()); err != nil {
				// best-effort; next tick retries
				_ = err
			}
		}
	}
}

// claimOne finds a claimable KB and processes it. FindClaimable returns at most
// one dataset so the worker handles it before looking for more.
func (c *Consumer) claimOne(ctx context.Context) {
	ids, err := c.scheduler.FindClaimable(ctx, 1)
	if err != nil || len(ids) == 0 {
		return
	}
	for _, ds := range ids {
		c.processDataset(ctx, ds)
	}
}

// processDataset claims the closed batch for datasetID, processes it, and acks.
// It is the Option E replacement for the old processOnce (lease + drain + merge):
// the claim transaction returns a frozen batch boundary, so there is no moving
// target and no Nak-churn routing.
func (c *Consumer) processDataset(ctx context.Context, datasetID string) {
	cr, ok, err := c.scheduler.Claim(ctx, datasetID, c.batchSize)
	if err != nil {
		return
	}
	if !ok || len(cr.Entries) == 0 {
		return // race lost or nothing to claim
	}

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
	for _, e := range entries {
		switch EventType(e.EventType) {
		case EventTypeDeleted:
			if tomb == nil {
				tomb = map[string]uint64{}
				c.tombs[kb] = tomb
			}
			tomb[e.DocID] = e.Seq
			deleted = append(deleted, e.DocID)
		case EventTypeCompleted:
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
	}
	c.mu.Unlock()

	if len(deleted) == 0 && len(completed) == 0 {
		return nil
	}

	deduper, err := c.factory(tenant)
	if err != nil || deduper == nil {
		deduper = NewNoopDeduper()
	}

	products, err := c.reader.LoadCompiledProducts(ctx, tenant, kb)
	if err != nil {
		return err
	}
	if len(products) == 0 {
		// Nothing to merge. Still drop fully-orphaned merged products for
		// deleted docs, then treat as success (nothing to do).
		for _, d := range deleted {
			_ = c.writer.DeleteMergedForDoc(ctx, tenant, kb, d)
		}
		return nil
	}

	// With deletions present, recompute the whole KB; otherwise scope to the
	// contributing documents of this batch (efficiency).
	if len(deleted) == 0 {
		docSet := make(map[string]bool, len(completed))
		for _, e := range completed {
			docSet[e.DocID] = true
		}
		scoped := products[:0]
		for _, p := range products {
			if docSet[p.DocID] {
				scoped = append(scoped, p)
			}
		}
		products = scoped
	}

	merged, err := deduper.Dedup(ctx, products)
	if err != nil {
		return err
	}
	if err := c.writer.WriteMerged(ctx, tenant, kb, merged); err != nil {
		return err
	}
	for _, d := range deleted {
		_ = c.writer.DeleteMergedForDoc(ctx, tenant, kb, d)
	}
	return nil
}
