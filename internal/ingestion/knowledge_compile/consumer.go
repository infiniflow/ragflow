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
	maxIdle       time.Duration
	ttl           time.Duration
	heartbeat     time.Duration
	pollInterval  time.Duration
	sweepInterval time.Duration

	mu    sync.Mutex
	seqs  map[string]uint64          // dataset -> last applied seq (out-of-order guard)
	tombs map[string]map[string]bool // dataset -> docID -> deleted (tombstone)
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
		maxIdle:       30 * time.Second,
		ttl:           2 * time.Minute,
		heartbeat:     20 * time.Second,
		pollInterval:  2 * time.Second,
		sweepInterval: 30 * time.Second,
		seqs:          map[string]uint64{},
		tombs:         map[string]map[string]bool{},
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
	go func() {
		defer close(done)
		c.processBatch(ctx, cr.TenantID, datasetID, cr.Entries)
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

	// Ack only the batch we claimed. A token mismatch (lease taken over between
	// claim and now) surfaces as an error; the batch was already merged
	// idempotently, so skipping the ack just leaves it for reclamation.
	if _, err := c.scheduler.Ack(ctx, datasetID, cr.Token, cr.Entries); err != nil {
		_ = err
	}
}

// processBatch applies out-of-order / tombstone handling, then recomputes and
// writes the dataset-level merged products for the claimed closed batch.
func (c *Consumer) processBatch(ctx context.Context, tenant, kb string, entries []BacklogEntry) {
	c.mu.Lock()
	if c.tombs == nil {
		c.tombs = map[string]map[string]bool{}
	}
	if c.seqs == nil {
		c.seqs = map[string]uint64{}
	}
	tomb := c.tombs[kb]
	var completed []BacklogEntry
	var deleted []string
	for _, e := range entries {
		switch EventType(e.EventType) {
		case EventTypeDeleted:
			if tomb == nil {
				tomb = map[string]bool{}
				c.tombs[kb] = tomb
			}
			tomb[e.DocID] = true
			deleted = append(deleted, e.DocID)
		case EventTypeCompleted:
			if tomb != nil && tomb[e.DocID] {
				continue // deleted before it completed
			}
			if prev, ok := c.seqs[kb]; ok && e.Seq <= prev {
				continue // stale / duplicate completion
			}
			c.seqs[kb] = e.Seq
			completed = append(completed, e)
		}
	}
	c.mu.Unlock()

	if len(deleted) == 0 && len(completed) == 0 {
		return
	}

	deduper, err := c.factory(tenant)
	if err != nil || deduper == nil {
		deduper = NewNoopDeduper()
	}

	products, err := c.reader.LoadCompiledProducts(ctx, tenant, kb)
	if err != nil || len(products) == 0 {
		// Nothing to merge (or reader unavailable). Still drop fully-orphaned
		// merged products for deleted docs.
		for _, d := range deleted {
			_ = c.writer.DeleteMergedForDoc(ctx, tenant, kb, d)
		}
		return
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
		return
	}
	if err := c.writer.WriteMerged(ctx, tenant, kb, merged); err != nil {
		return
	}
	for _, d := range deleted {
		_ = c.writer.DeleteMergedForDoc(ctx, tenant, kb, d)
	}
}
