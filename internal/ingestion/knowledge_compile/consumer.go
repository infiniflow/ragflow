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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service/nav"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// kcDB is the package-level MySQL handle installed by Provision. It backs the
// doc enumeration used by RebuildDataset; it is nil (and docLister returns no
// docs) when the scheduler was provisioned without a DB.
var kcDB *gorm.DB

// Consumer is the dataset-level post-processing worker (§11.5). Multiple
// instances compete on the MySQL scheduling rows; each KB is processed by at
// most one instance at a time via the per-KB claim (so the same KB is handled
// by a single worker that serializes the batch). The MySQL row — not the
// broker — is the scheduling system of record and the source of same-KB
// serialization.
type Consumer struct {
	scheduler     Claimer
	reader        Reader
	writer        Writer
	factory       DeduperFactory
	contributions wikiContributionStore

	ttl            time.Duration
	heartbeat      time.Duration
	pollInterval   time.Duration
	sweepInterval  time.Duration
	mergeThreshold float64 // KNN similarity threshold for "existing merged row is a duplicate"

	mu    sync.Mutex
	tombs map[string]map[string]uint64 // dataset -> docID -> delete marker (tombstone)

	// rebuildPause suppresses local claims while this consumer rebuilds a dataset.
	rebuildPause map[string]bool

	// docLister enumerates every doc id in a KB so a rewrite can republish them.
	docLister func(ctx context.Context, tenant, kb string) ([]string, error)
}

// rewriteScheduler is the subset of the scheduler API a dataset rewrite needs,
// satisfied by *mysqlScheduler (and FakeScheduler in tests). Using a narrow
// interface keeps the Claimer surface unchanged.
type rewriteScheduler interface {
	Publish(ctx context.Context, tenantID, datasetID, docID, eventType string, variants []string) error
	CancelInflight(ctx context.Context, datasetID, token string) error
}

// NewConsumer constructs a Consumer driven by the given Claimer. Tests pass a
// FakeScheduler and override the Reader/Writer/Deduper via options.
func NewConsumer(scheduler Claimer, opts ...Option) *Consumer {
	c := &Consumer{
		scheduler:      scheduler,
		reader:         engineReader{eng: engine.Get()},
		writer:         engineWriter{eng: engine.Get()},
		factory:        defaultDeduperFactory,
		contributions:  newWikiContributionStore(engine.Get()),
		ttl:            2 * time.Minute,
		heartbeat:      20 * time.Second,
		pollInterval:   2 * time.Second,
		sweepInterval:  30 * time.Second,
		mergeThreshold: 0.85,
		tombs:          map[string]map[string]uint64{},
		rebuildPause:   map[string]bool{},
		docLister:      defaultDocLister,
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

	c.mu.Lock()
	paused := c.rebuildPause[datasetID]
	c.mu.Unlock()
	if paused {
		return
	}
	if err := startDatasetCompileLog(ctx, cr.TenantID, datasetID, cr.Token, cr.Entries); err != nil {
		common.Warn("knowledge_compile: failed to create dataset ingestion log",
			zap.String("dataset_id", datasetID), zap.Error(err))
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
		batchErr = c.processBatch(ctx, cr.TenantID, datasetID, cr.Token, cr.Entries)
	}()

	select {
	case <-ctx.Done():
		close(stopHb)
		<-done
		if err := finishDatasetCompileLog(context.WithoutCancel(ctx), cr.Token, common.STOPPED, "Wiki dataset compilation stopped during shutdown", -1); err != nil {
			common.Warn("knowledge_compile: failed to finalize stopped dataset ingestion log",
				zap.String("dataset_id", datasetID), zap.Error(err))
		}
		return // graceful shutdown: leave inflight for reclamation, do not ack
	case <-hbFailed:
		close(stopHb)
		<-done
		if err := finishDatasetCompileLog(context.WithoutCancel(ctx), cr.Token, common.STOPPED, "Wiki dataset compilation lease was lost and will be retried", -1); err != nil {
			common.Warn("knowledge_compile: failed to finalize stopped dataset ingestion log",
				zap.String("dataset_id", datasetID), zap.Error(err))
		}
		return // lease lost: do not ack; sweeper/redelivery reprocesses (idempotent)
	case <-done:
		close(stopHb)
	}

	// Ack only on success. A batch error (reader/dedup/writer failure) means we
	// must leave the claimed batch in the backlog for reclamation/retry rather
	// than silently dropping it (C5: never ack what we failed to merge).
	if batchErr != nil {
		if errors.Is(batchErr, errClaimSuperseded) {
			return
		}
		common.Error("knowledge_compile: batch processing failed, leaving batch for retry",
			batchErr,
			zap.String("dataset_id", datasetID),
			zap.Int("entries", len(cr.Entries)))
		if err := c.scheduler.SetError(ctx, datasetID, cr.Token, batchErr.Error()); err != nil {
			common.Warn("knowledge_compile: failed to record error_msg",
				zap.String("dataset_id", datasetID), zap.Error(err))
		}
		if err := finishDatasetCompileLog(ctx, cr.Token, common.FAILED, batchErr.Error(), -1); err != nil {
			common.Warn("knowledge_compile: failed to finalize dataset ingestion log",
				zap.String("dataset_id", datasetID), zap.Error(err))
		}
		return
	}
	if _, err := c.scheduler.Ack(ctx, datasetID, cr.Token, cr.Entries); err != nil {
		common.Warn("knowledge_compile: ack failed",
			zap.String("dataset_id", datasetID), zap.Error(err))
		if logErr := finishDatasetCompileLog(ctx, cr.Token, common.FAILED, "Failed to acknowledge completed Wiki compilation: "+err.Error(), -1); logErr != nil {
			common.Warn("knowledge_compile: failed to finalize dataset ingestion log",
				zap.String("dataset_id", datasetID), zap.Error(logErr))
		}
		return
	}
	if err := finishDatasetCompileLog(ctx, cr.Token, common.COMPLETED, "Wiki dataset compilation completed", 1); err != nil {
		common.Warn("knowledge_compile: failed to finalize dataset ingestion log",
			zap.String("dataset_id", datasetID), zap.Error(err))
	}
}

func (c *Consumer) reportProgress(ctx context.Context, tenant, datasetID, token string, progress float64, phase, message string) {
	if err := c.scheduler.UpdateProgress(ctx, datasetID, token, progress, phase, message); err != nil {
		common.Warn("knowledge_compile: failed to update progress",
			zap.String("dataset_id", datasetID),
			zap.String("phase", phase),
			zap.Error(err))
	}
	if err := updateDatasetCompileLog(ctx, token, progress, message); err != nil {
		common.Warn("knowledge_compile: failed to update dataset ingestion log",
			zap.String("dataset_id", datasetID), zap.String("phase", phase), zap.Error(err))
	}
	common.Info("knowledge_compile: progress",
		zap.String("dataset_id", datasetID),
		zap.String("tenant_id", tenant),
		zap.Float64("progress", progress),
		zap.String("phase", phase),
		zap.String("message", message))
}

var errClaimSuperseded = errors.New("knowledge_compile: claim superseded by rewrite")

// withWriteLock runs a destructive side effect fn under the scheduler's
// per-dataset write/rebuild lock, verifying the claim token inside the lock
// immediately before fn. This closes the TOCTOU gap between cancellation and a
// writer side effect: a cancelled worker cannot write after the rebuild clears
// storage, and a rebuild cannot interleave while fn runs.
func (c *Consumer) withWriteLock(ctx context.Context, kb, token string, fn func() error) error {
	return c.scheduler.WithDatasetLock(ctx, kb, func(currentToken string) error {
		if currentToken != token {
			return errClaimSuperseded
		}
		return fn()
	})
}

// RebuildDataset performs a full incremental rewrite of a KB's dataset-level
// merged products (W1/W2/W5/W6). The order is fixed to close both the
// enumerate-then-clear race (M19/C-race) AND the cross-process stale-write
// window:
//  1. set the rewrite pause (in-process only; clears via defer on every path);
//  2. CancelInflight invalidates any in-flight claim before clearing storage.
//     Each worker compares its claim token under the write lock and therefore
//     cannot write after cancellation;
//  3. DeleteMerged + DropWikiGraph clear the old merged + graph state INSIDE the
//     same per-dataset row lock (WithDatasetLock). This is what actually closes
//     the TOCTOU gap: the generation check + write of every worker run under this
//     lock, so the rebuild's clear either runs after any in-flight worker write
//     has finished (it is later removed by the clear) or after a stale worker has
//     self-dropped — a worker can never repopulate cleared state with old results
//     because it cannot hold the lock concurrently with the clear;
//  4. enumerate every doc and republish;
//  5. clear the local pause.
//
// mode is "incremental" or "rewrite"; today both take the same clean-and-rebuild
// path, with mode retained for future differential strategies.
func (c *Consumer) RebuildDataset(ctx context.Context, tenant, kb, mode string) error {
	rs, ok := c.scheduler.(rewriteScheduler)
	if !ok {
		return fmt.Errorf("knowledge_compile: scheduler %T does not support rewrite", c.scheduler)
	}

	// 1. pause the rewrite window. The pause is cleared by a deferred cleanup so a
	// failure mid-rebuild (DeleteMerged/DropWikiGraph/CancelInflight/Bump/Publish)
	// can never leave the dataset permanently paused (which would drop every
	// future claim). Partially published docs are retried on the next claim.
	c.mu.Lock()
	c.rebuildPause[kb] = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.rebuildPause[kb] = false
		c.mu.Unlock()
	}()

	// 2. Cancel any in-flight claim before clearing storage. A worker that was
	// already running observes its revoked claim token inside withWriteLock and
	// returns without writing.
	if err := rs.CancelInflight(ctx, kb, ""); err != nil {
		return fmt.Errorf("knowledge_compile: rebuild cancel inflight: %w", err)
	}

	// 3. clear old merged + graph state (structural filter is the source of
	// truth; no per-doc rows are touched) under the per-dataset row lock. This
	// guarantees mutual exclusion with every worker's destructive write (which
	// runs under the same lock via withWriteLock): the clear either waits for an
	// in-flight worker write to finish (then removes its old-generation result)
	// or runs after stale workers have self-dropped. A worker cannot hold the
	// lock while the clear runs, so it can never repopulate cleared state with
	// old results.
	if err := c.scheduler.WithDatasetLock(ctx, kb, func(_ string) error {
		// B4/B1b: a full rebuild clears EVERY variant the consumer manages
		// (wiki merged rows, structure scope_kwd="dataset" rows, and the nav
		// tree rows) via the fixed full-set clean (empty variants), not just the
		// wiki rows that DeleteMerged removes — otherwise removed-template ghost
		// rows survive. DropWikiGraph additionally sweeps the legacy wiki graph.
		if derr := c.writer.DeleteMergedForVariant(ctx, tenant, kb, nil); derr != nil {
			return derr
		}
		if derr := c.writer.DropWikiGraph(ctx, tenant, kb); derr != nil {
			return derr
		}
		return c.contributions.ClearDataset(ctx, tenant, kb)
	}); err != nil {
		return fmt.Errorf("knowledge_compile: rebuild clear all variants + graph: %w", err)
	}

	// 4. enumerate every doc and republish. Per B1, the republished events must
	// carry the doc's compile variants so the consumer can route to the correct
	// dataset-level path (a nil/empty Variants would fall back to the legacy
	// unified path and tree nav could not be rebuilt). The variants are recovered
	// from the doc-level compiled products' authoritative `compilation_template_kind_kwd`
	// (via KindToVariant, O2a whitelist hard-fail), not re-derived from `compile_kwd`.
	docs, err := c.docLister(ctx, tenant, kb)
	if err != nil {
		return fmt.Errorf("knowledge_compile: rebuild list docs: %w", err)
	}
	for _, docID := range docs {
		variants, rerr := c.recoverDocVariants(ctx, tenant, kb, docID)
		if rerr != nil {
			// O2a hard failure: abort the rebuild so an unknown template kind
			// cannot leave the doc's dataset-level products unrebuilt.
			return fmt.Errorf("knowledge_compile: rebuild recover variants %s: %w", docID, rerr)
		}
		if len(variants) == 0 {
			// Documents without compiled products do not contribute to any
			// dataset-level variant and must not enqueue an empty completion
			// event that falls back to the legacy all-products path.
			continue
		}
		if perr := rs.Publish(ctx, tenant, kb, docID, string(EventTypeCompleted), variants); perr != nil {
			return fmt.Errorf("knowledge_compile: rebuild republish %s: %w", docID, perr)
		}
	}

	// 6. resweep: the deferred cleanup clears the pause so any claim arriving
	// post-enumeration is processed (and stale ones self-drop in processClaim).
	common.Info("knowledge_compile: dataset rebuild complete",
		zap.String("dataset_id", kb),
		zap.String("mode", mode),
		zap.Int("docs", len(docs)))
	return nil
}

// defaultDocLister enumerates every doc id in a KB via the Document DAO. When
// no DB was provisioned it returns no docs (a rewrite becomes a no-op clean).
func defaultDocLister(ctx context.Context, tenant, kb string) ([]string, error) {
	if kcDB == nil {
		return nil, nil
	}
	return dao.NewDocumentDAO().ListIDsByKBIDWithOptions(ctx, kcDB, dao.DocumentListOptions{KbID: kb})
}

// recoverDocVariants reconstructs the compile-type set a document produced, by
// reading its persisted doc-level products and mapping each product's
// authoritative `compilation_template_kind_kwd` through common.KindToVariant.
// It is used by RebuildDataset (B1) so the republished completed events carry
// the variants needed to route the dataset-level re-compile (tree nav, structure
// merge, wiki merge) — a nil/empty Variants would fall back to the legacy
// unified path and tree nav could not be rebuilt.
//
// KindToVariant (O2a) is a fixed whitelist that HARD-FAILS on unknown kinds: a
// product whose kind is not in the whitelist is a corrupt/unsupported state and
// the rebuild must abort (returning an error) rather than silently drop the
// variant and leave the doc's dataset-level products unrebuilt. A product
// without an authoritative kind falls back to its reverse-mapped variant (which
// itself must be whitelist-valid, enforced by KwdToVariant in the Reader). The
// returned slice is sorted and de-duplicated.
func (c *Consumer) recoverDocVariants(ctx context.Context, tenant, kb, docID string) ([]string, error) {
	products, err := c.reader.LoadDocProducts(ctx, tenant, kb, docID)
	if err != nil {
		return nil, fmt.Errorf("knowledge_compile: recover variants load doc %s: %w", docID, err)
	}
	seen := map[string]struct{}{}
	var out []string
	for _, p := range products {
		// Authoritative: product.Kind (compilation_template_kind_kwd) when present.
		if p.Kind != "" {
			v, err := kccommon.KindToVariant(p.Kind)
			if err != nil {
				// O2a hard failure: do not continue with an incomplete variant set.
				return nil, fmt.Errorf("knowledge_compile: recover variants doc %s kind %q: %w", docID, p.Kind, err)
			}
			if _, dup := seen[string(v)]; dup {
				continue
			}
			seen[string(v)] = struct{}{}
			out = append(out, string(v))
			continue
		}
		// Fallback: the product's reverse-mapped variant (already whitelist-valid).
		if p.Variant == "" {
			// No authoritative kind and no reverse-mapped variant: this is a
			// legacy/unknown row. Skip it so we never republish an empty variant
			// (which the consumer cannot route and defeats "nil = legacy").
			continue
		}
		if _, dup := seen[string(p.Variant)]; dup {
			continue
		}
		seen[string(p.Variant)] = struct{}{}
		out = append(out, string(p.Variant))
	}
	sort.Strings(out)
	return out, nil
}

// filterWikiPageCandidates returns ONLY the wiki page products the dataset-level
// wiki merge may fold. It drops every non-wiki product (tree/structure/nav) so
// those never enter the wiki Dedup/KNN/WriteMerged path, and within the wiki
// variant keeps strictly page-kind products (Meta.kind == "page"). Sections are
// a doc-level concern and must never enter the dataset-level page bucket, and
// the deduper must not carry an implicit section-filter contract. This is the
// per-variant product selection (review issue 2): a completed wiki-only doc whose
// event carried tree/structure products on a prior run must not feed stale
// tree/structure rows into the wiki merge.
func filterWikiPageCandidates(candidates []kccommon.Product) []kccommon.Product {
	filtered := make([]kccommon.Product, 0, len(candidates))
	for _, cand := range candidates {
		if cand.Variant != kccommon.VariantWiki {
			continue // tree/structure products never enter the wiki merge
		}
		if metaString(cand.Meta, "kind") != "page" {
			continue
		}
		filtered = append(filtered, cand)
	}
	return filtered
}

// processBatch applies out-of-order / tombstone handling, then recomputes and
// writes the dataset-level merged products for the claimed closed batch. It
// returns an error if any reader/dedup/writer step fails so the caller can
// leave the batch for reclamation instead of acking dropped work.
func (c *Consumer) processBatch(ctx context.Context, tenant, kb, token string, entries []BacklogEntry) error {
	c.reportProgress(ctx, tenant, kb, token, 0.05, "classifying", fmt.Sprintf("Classifying %d scheduling entries", len(entries)))
	common.Info("knowledge_compile: processing claimed batch",
		zap.String("dataset_id", kb),
		zap.String("tenant_id", tenant),
		zap.Int("entries", len(entries)))
	c.mu.Lock()
	if c.tombs == nil {
		c.tombs = map[string]map[string]uint64{}
	}
	tomb := c.tombs[kb]
	var completed []BacklogEntry
	var deleted []string
	// Per-document tombstone clears are staged locally and committed only after
	// the write/delete paths below return without error, so a failed batch
	// leaves c.tombs untouched and can be retried on reclaim.
	var pendingTombClear []string
	// Reconcile each document to its last event before classifying it as
	// deleted or completed. A completion and a deletion for the same doc can
	// land in the same batch, and completions/deletions can also arrive out of
	// order across batches. The LAST event for a doc (by backlog append order)
	// decides its fate, so we must not delete a doc that was re-ingested after
	// its deletion, nor merge a completion shadowed by a later deletion. No
	// sequence number is needed: the backlog is an ordered log and the last
	// write wins, which is exactly the re-parse (completed last) vs delete
	// (deleted last) semantics we want.
	type docState struct {
		winEvent EventType
		variants []string
	}
	byDoc := make(map[string]*docState, len(entries))
	for _, e := range entries {
		et := EventType(e.EventType)
		st := byDoc[e.DocID]
		if st == nil {
			byDoc[e.DocID] = &docState{winEvent: et, variants: e.Variants}
			continue
		}
		// Later entries overwrite earlier ones: last event wins.
		st.winEvent = et
		st.variants = e.Variants
	}
	for docID, st := range byDoc {
		switch st.winEvent {
		case EventTypeDeleted:
			// Record the tombstone for this batch's deletion so a stale
			// completion is skipped. The tombstone is committed immediately
			// (deletions are not rolled back on a failed batch because
			// re-running the delete is idempotent).
			if tomb == nil {
				tomb = map[string]uint64{}
				c.tombs[kb] = tomb
			}
			tomb[docID] = 1
			deleted = append(deleted, docID)
		case EventTypeCompleted:
			// A re-ingest after an earlier deletion: clear the prior
			// tombstone (deferred until the batch succeeds).
			if _, hadTomb := tomb[docID]; hadTomb {
				pendingTombClear = append(pendingTombClear, docID)
			}
			// Preserve the winning event's variants so the dispatch below can
			// route this doc to the matching dataset-level compile path (A0-4).
			completed = append(completed, BacklogEntry{DocID: docID, EventType: string(EventTypeCompleted), Variants: st.variants})
		}
	}
	// Sort for deterministic iteration in the delete/load passes below.
	sort.Strings(deleted)
	c.mu.Unlock()

	// Per-entry detail (each claim carries a frozen, closed batch of doc events;
	// log them on a single line so a batch can be reconstructed from the ingestor
	// log alone).
	entryDetails := make([]string, 0, len(entries))
	for _, e := range entries {
		entryDetails = append(entryDetails, fmt.Sprintf("doc=%s event=%s", e.DocID, e.EventType))
	}

	common.Info("knowledge_compile: batch merge start",
		zap.String("dataset_id", kb),
		zap.Int("completed_docs", len(completed)),
		zap.Int("deleted_docs", len(deleted)),
		zap.Strings("entries", entryDetails))

	if len(deleted) == 0 && len(completed) == 0 {
		c.reportProgress(ctx, tenant, kb, token, 1, "completed", "No document products require merging")
		return nil
	}
	deletedSet := make(map[string]bool, len(deleted))
	for _, d := range deleted {
		deletedSet[d] = true
	}
	wikiDiff, err := c.prepareWikiContributionDiff(ctx, tenant, kb, completed, deleted)
	if err != nil {
		return err
	}
	c.reportProgress(ctx, tenant, kb, token, 0.10, "comparing_wiki_contributions",
		fmt.Sprintf("Comparing Wiki contributions: %d affected page(s)", len(wikiDiff.affectedKeys)))
	wikiOnly := len(completed) > 0
	for _, entry := range completed {
		if len(entry.Variants) != 1 || !variantsContain(entry.Variants, "wiki") {
			wikiOnly = false
			break
		}
	}
	if len(deleted) == 0 && wikiOnly && len(wikiDiff.affectedKeys) == 0 {
		c.mu.Lock()
		for _, docID := range pendingTombClear {
			delete(c.tombs[kb], docID)
		}
		c.mu.Unlock()
		c.reportProgress(ctx, tenant, kb, token, 1, "completed", "Wiki contributions are unchanged; no dataset pages require updating")
		return nil
	}

	deduper, err := c.factory(tenant)
	if err != nil || deduper == nil {
		// A no-op deduper would silently disable dataset-level LLM merging: every
		// candidate (including e.g. a "吕布" wiki_page) would be written as its own
		// merged row and duplicates would accumulate across runs. That degradation
		// is never acceptable here, so fail loudly instead of papering over it.
		common.Fatal("knowledge_compile: dataset-level LLM deduper unavailable, refusing to continue with a no-op merge",
			zap.String("kb_id", kb),
			zap.String("tenant_id", tenant),
			zap.String("factory_err", func() string {
				if err != nil {
					return err.Error()
				}
				return "deduper factory returned nil"
			}()))
	}
	c.reportProgress(ctx, tenant, kb, token, 0.12, "deleting", fmt.Sprintf("Deleting products for %d document(s)", len(deleted)))

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
		// Each destructive write runs under the scheduler's per-dataset
		// write/rebuild lock with the generation check performed INSIDE the lock,
		// so a rewrite can neither land between the check and the write nor
		// interleave with the write itself (it must wait for this lock). This
		// closes the TOCTOU window where a worker past its fence could repopulate
		// storage the rebuild just cleared.
		if err := c.withWriteLock(ctx, kb, token, func() error {
			return c.writer.DeleteDocLevelForDocs(ctx, tenant, kb, delIDs)
		}); err != nil {
			if errors.Is(err, errClaimSuperseded) {
				common.Info("knowledge_compile: batch stale before delete, aborting (rewrite barrier)",
					zap.String("dataset_id", kb))
			}
			return err
		}
		// The second destructive call is its own locked section: a rewrite can
		// land between the two, in which case the stale batch must not apply its
		// second side effect under the new generation.
		if err := c.withWriteLock(ctx, kb, token, func() error {
			return c.writer.StripMergedSources(ctx, tenant, kb, delIDs)
		}); err != nil {
			if errors.Is(err, errClaimSuperseded) {
				common.Info("knowledge_compile: batch stale before strip, aborting (rewrite barrier)",
					zap.String("dataset_id", kb))
			}
			return err
		}
		// G3: structure ghost cleanup — remove structure dataset rows whose only
		// source docs were just deleted. Also a locked destructive write.
		if err := c.withWriteLock(ctx, kb, token, func() error {
			return c.writer.DeleteStructureForDocs(ctx, tenant, kb, delIDs)
		}); err != nil {
			if errors.Is(err, errClaimSuperseded) {
				common.Info("knowledge_compile: batch stale before structure ghost cleanup, aborting (rewrite barrier)",
					zap.String("dataset_id", kb))
			}
			return err
		}
	}

	// --- Completion merge ---
	// Load only the per-document products of the completed (and not deleted)
	// docs — bounded by this batch, never the whole KB. A doc that is both
	// completed and deleted is a stale tombstone: the deletion wins, so we skip
	// its completion.
	var incoming []kccommon.Product
	c.reportProgress(ctx, tenant, kb, token, 0.28, "loading_products", fmt.Sprintf("Loading products for %d document(s)", len(completed)))
	for _, e := range completed {
		if deletedSet[e.DocID] {
			continue
		}
		docProducts := wikiDiff.productsByDoc[e.DocID]
		// A0-4: the event's Variants are the authoritative dispatch signal — route
		// ONLY the products whose variant the compiler actually produced for this
		// doc. A doc re-compiled under a new template may still hold stale
		// doc-level rows of an earlier variant in storage; without this gate those
		// would leak into the nav/structure paths despite the event asking only
		// for, say, wiki. Legacy events with empty Variants keep all products.
		selected := productsForVariants(docProducts, e.Variants)
		for _, product := range selected {
			if product.Variant != kccommon.VariantWiki {
				incoming = append(incoming, product)
			}
		}
	}
	if len(wikiDiff.affectedKeys) > 0 {
		reader, ok := c.reader.(documentWikiPageReader)
		if !ok {
			return fmt.Errorf("knowledge_compile: reader does not support Wiki contribution loading")
		}
		pages, err := reader.LoadDocumentWikiPagesBySlugs(ctx, tenant, kb, sortedStringSet(wikiDiff.affectedSlugs))
		if err != nil {
			return err
		}
		for _, page := range pages {
			if _, affected := wikiDiff.affectedKeys[wikiPageMergeKey(page)]; affected {
				incoming = append(incoming, page)
			}
		}
	}

	// A0-4 dispatch: route tree/structure products to the dataset-navigation tree
	// (NavService.UpsertDoc), and keep wiki (and only wiki) products on the
	// existing dataset-level merge path below. This is the per-variant dispatch
	// the plan requires: tree and structure both produce a nav by-product (B2),
	// so both upsert their summary into the cross-document nav tree; wiki products
	// continue to the page-specific evidence merge.
	navIn := navInputFromProducts(kb, incoming)
	c.reportProgress(ctx, tenant, kb, token, 0.40, "merging_navigation", fmt.Sprintf("Merging navigation products: %d", len(navIn)))
	if len(navIn) > 0 {
		ns := nav.GetNavService()
		if ns == nil {
			common.Warn("knowledge_compile: nav service unavailable, skipping dataset-nav upsert for batch",
				zap.String("dataset_id", kb))
		} else if err := c.upsertNavLocked(ctx, tenant, kb, token, ns, navIn); err != nil {
			return err
		}
	}

	// G1+G2: dataset-level structure merge and build-time stamp, both under the
	// claim-fenced write lock (B7 applies to structure writes too): a worker
	// invalidated by CancelInflight must not write structure rows after a rebuild
	// has cleared them.
	if err := c.withWriteLock(ctx, kb, token, func() error {
		return c.mergeStructureDataset(ctx, tenant, kb, incoming)
	}); err != nil {
		if errors.Is(err, errClaimSuperseded) {
			common.Info("knowledge_compile: batch stale before structure merge, aborting (rewrite barrier)",
				zap.String("dataset_id", kb))
		}
		return err
	}
	c.reportProgress(ctx, tenant, kb, token, 0.50, "merging_structure", fmt.Sprintf("Merging structure products: %d", len(incoming)))

	// wiki_incremental port (M1): the dataset-level merge only processes wiki
	// PAGES. A wiki doc yields both page and section products (Meta.kind
	// "page"/"section"); sections are a doc-level concern and must never be folded
	// into the dataset-level page bucket. Filter BEFORE the in-memory dedup so
	// sections never enter the deduper (and never trigger LLM/embedding/alias
	// processing) — the decider must not carry an implicit section-filter
	// contract. Legacy rows whose kind is empty are derived in the Reader; only
	// truly page-kind wiki products proceed.
	candidates := filterWikiPageCandidates(incoming)
	c.reportProgress(ctx, tenant, kb, token, 0.54, "filtering_candidates", fmt.Sprintf("Filtering Wiki page candidates: %d", len(candidates)))
	entityMerged, topicCandidates, err := c.mergeEntityModeCandidates(ctx, tenant, kb, candidates)
	if err != nil {
		return err
	}
	// A Wiki page is identified by its page type and canonical slug. Do not use
	// content KNN or an LLM to decide whether two pages are the same: different
	// slugs are distinct pages, while equal slugs are deterministically folded.
	topicMerged, err := c.mergeWikiProductsBySlug(ctx, tenant, kb, topicCandidates)
	if err != nil {
		return err
	}
	entityMerged = append(entityMerged, topicMerged...)
	if len(wikiDiff.affectedKeys) > 0 {
		if err := c.deleteMissingScopedWikiPages(ctx, tenant, kb, token, wikiDiff.affectedKeys, entityMerged); err != nil {
			return err
		}
	}
	c.reportProgress(ctx, tenant, kb, token, 0.58, "merging_by_slug", fmt.Sprintf("Merging Wiki pages by slug: %d", len(entityMerged)))
	// Entity pages have stable page identity and must bypass topic-mode KNN and
	// Page-specific entity/concept merging. Only topic-mode pages continue below.
	// The slug-only merge above has already handled every Wiki page candidate.
	// Leave the legacy generic dedup path with no candidates so it cannot apply
	// content similarity or LLM merge decisions to Wiki pages.
	candidates = nil
	// In-memory dedup among the completed batch first.
	candidates, dedupErr := deduper.Dedup(ctx, candidates)
	if dedupErr != nil {
		return dedupErr
	}
	// Diagnostics: after batch dedup, verify the per-doc products still carry
	// their embedding before the KNN/merge path consumes cand.Vector. If
	// candidates show 0 vectors while incoming had vectors, Dedup is dropping
	// them and both the KNN lookups and the merged rows lose embeddings.
	{
		incomingVec, candVec := 0, 0
		for _, p := range incoming {
			if len(p.Vector) > 0 {
				incomingVec++
			}
		}
		for _, p := range candidates {
			if len(p.Vector) > 0 {
				candVec++
			}
		}
		common.Info("knowledge_compile: dedup vector audit",
			zap.String("kb_id", kb),
			zap.Int("incoming", len(incoming)),
			zap.Int("incoming_with_vector", incomingVec),
			zap.Int("candidates", len(candidates)),
			zap.Int("candidates_with_vector", candVec))
	}

	// Diagnostics: break the KNN-input set down by variant and list the wiki_page
	// slugs, so the reader can reconcile the number of wiki_page candidates here
	// against the wiki_page rows WriteMerged emits (they differ when KNN merges a
	// candidate into an existing merged row, when DecideBatch returns distinct new
	// rows, or when candidates of the same variant collapse in-memory). This is
	// what explains "39 wiki_page written vs 21 KNN requests".
	{
		byVariant := map[string]int{}
		var wikiPageSlugs []string
		for _, cand := range candidates {
			v := string(cand.Variant)
			byVariant[v]++
			if v == "wiki_page" {
				wikiPageSlugs = append(wikiPageSlugs, candidateIdentity(cand))
			}
		}
		common.Info("knowledge_compile: candidates breakdown",
			zap.String("kb_id", kb),
			zap.Int("candidates", len(candidates)),
			zap.Any("by_variant", byVariant),
			zap.Strings("wiki_page_candidate_slugs", wikiPageSlugs))
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
	// Exact topic recall is independent of page-content KNN. A page can use
	// different wording from the existing topic while still belonging to the
	// same topic, so consult the current dataset pages before running KNN.
	topicPagesByKey := make(map[string]kccommon.Product)
	if pageReader, ok := c.reader.(mergedWikiPageReader); ok {
		existingPages, err := pageReader.LoadMergedWikiPages(ctx, tenant, kb)
		if err != nil {
			return err
		}
		for _, page := range existingPages {
			if !isTopicPage(page) {
				continue
			}
			key := topicKey(productTopic(page))
			if key != "" {
				if _, exists := topicPagesByKey[key]; !exists {
					topicPagesByKey[key] = page
				}
			}
		}
	}
	// The KNN pass is docengine-bounded (vector search), not CPU-bounded, so we
	// fan it out across the shared global compilerPool (vCPU-sized). Output order
	// is irrelevant: merged rows are upserted by their idempotent dataset-level
	// id, and each candidate lands in exactly one group / the unmatched set.
	jobs := make([]CompilerJob, 0, len(candidates))
	for _, cand := range candidates {
		cand := cand
		if isTopicPage(cand) {
			if existing, ok := topicPagesByKey[topicKey(productTopic(cand))]; ok {
				groupsMu.Lock()
				g := groupsByID[existing.ID]
				if g == nil {
					g = &mergeGroup{existing: existing, score: 1}
					groupsByID[existing.ID] = g
				}
				g.candidates = append(g.candidates, cand)
				groupsMu.Unlock()
				continue
			}
		}
		jobs = append(jobs, func() error {
			var vec64 []float64
			if len(cand.Vector) > 0 {
				vec64 = make([]float64, len(cand.Vector))
				for i, v := range cand.Vector {
					vec64[i] = float64(v)
				}
			}
			// wiki_incremental port (B2): request topN >= 2 so SearchSimilar can
			// apply its score-descending skip rule on a dirty top-1 (the reader
			// drops rows whose compile_kwd does not map to the searched variant;
			// a re-query with topN=2 lets the next clean candidate surface instead
			// of falling through to "no hit").
			hit, score, err := c.reader.SearchSimilar(ctx, tenant, kb, cand.Variant, vec64, 2, c.mergeThreshold)
			if err != nil {
				return err
			}
			if hit.ID == "" {
				// No sufficiently-similar merged row: insert the candidate as a new
				// merged row.
				common.Debug("knowledge_compile: KNN no hit, candidate becomes new merged row",
					zap.String("kb_id", kb),
					zap.String("candidate", candidateIdentity(cand)),
					zap.String("variant", string(cand.Variant)))
				cand.Merged = true
				cand.DocID = kb
				unmatchedMu.Lock()
				unmatched = append(unmatched, cand)
				unmatchedMu.Unlock()
				return nil
			}
			common.Debug("knowledge_compile: KNN hit",
				zap.String("kb_id", kb),
				zap.String("candidate", candidateIdentity(cand)),
				zap.String("variant", string(cand.Variant)),
				zap.String("hit", candidateIdentity(hit)),
				zap.Float64("score", score))
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
	c.reportProgress(ctx, tenant, kb, token, 0.72, "routing_pages", fmt.Sprintf("Routing %d Wiki pages against existing products", len(candidates)))

	// KNN only narrows topic-page candidates. When the hit is a topic page,
	// require an explicit topic match or an LLM topic-route decision before
	// allowing the generic Wiki evidence merge to fold the pages together.
	// Otherwise semantically unrelated pages with similar prose would inherit
	// the existing page's topic.
	router, hasRouter := deduper.(topicRouter)
	if hasRouter || len(groupsByID) > 0 {
		for _, group := range groupsByID {
			if !isTopicPage(group.existing) {
				continue
			}
			accepted := make([]kccommon.Product, 0, len(group.candidates))
			for _, candidate := range group.candidates {
				if !isTopicPage(candidate) {
					candidate.Merged = true
					candidate.DocID = kb
					unmatched = append(unmatched, candidate)
					continue
				}
				if topicKey(productTopic(candidate)) == topicKey(productTopic(group.existing)) {
					accepted = append(accepted, candidate)
					continue
				}
				merge, routedTopic := false, ""
				if hasRouter {
					var err error
					merge, routedTopic, err = router.RouteTopic(ctx, candidate, group.existing)
					if err != nil {
						return err
					}
				}
				if !merge {
					candidate.Merged = true
					candidate.DocID = kb
					unmatched = append(unmatched, candidate)
					continue
				}
				if routedTopic == "" {
					routedTopic = productTopic(group.existing)
				}
				candidate = prepareTopicProduct(candidate, routedTopic)
				candidate.Meta["title"] = routedTopic
				accepted = append(accepted, candidate)
			}
			group.candidates = accepted
		}
	}

	// Fold every KNN group into the LLM in a single batch round-trip (one
	// DecideBatch call instead of one Decide per pair), then collect the
	// updated existing rows and the candidates judged distinct (new rows).
	var newMerged []kccommon.Product
	newMerged = append(newMerged, entityMerged...)
	if len(groupsByID) > 0 {
		c.reportProgress(ctx, tenant, kb, token, 0.82, "llm_merge", fmt.Sprintf("Merging %d candidate groups with the LLM", len(groupsByID)))
		// Diagnostics: summarize the KNN groups before the LLM merge decision so
		// the reader can see which existing merged rows were hit and by how many
		// candidates (e.g. whether a "吕布" candidate hit an existing 吕布 row and
		// was then judged by the LLM).
		{
			type groupDump struct {
				Existing   string   `json:"existing"`
				Candidates []string `json:"candidates"`
				Score      float64  `json:"score"`
			}
			dumps := make([]groupDump, 0, len(groupsByID))
			for _, g := range groupsByID {
				cands := make([]string, 0, len(g.candidates))
				for _, cand := range g.candidates {
					cands = append(cands, candidateIdentity(cand))
				}
				dumps = append(dumps, groupDump{Existing: candidateIdentity(g.existing), Candidates: cands, Score: g.score})
			}
			common.Info("knowledge_compile: KNN groups for DecideBatch",
				zap.String("kb_id", kb),
				zap.Int("groups", len(groupsByID)),
				zap.Any("groups_detail", dumps))
		}
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

	// Write the surviving merged set (updated existing + new distinct rows). The
	// generation check + WriteMerged run atomically under the per-dataset
	// write/rebuild lock, so a rewrite that lands during the (now long) KNN + LLM
	// merge cannot leak into the freshly rewritten index, and a rewrite that is
	// clearing storage cannot interleave with the write.
	mergedFinal := make([]kccommon.Product, 0, len(newMerged)+len(unmatched))
	mergedFinal = append(mergedFinal, newMerged...)
	mergedFinal = append(mergedFinal, unmatched...)
	mergedFinal, staleTopicIDs := mergeTopicProducts(tenant, kb, mergedFinal)
	mergedFinal = refreshWikiProductVectors(ctx, tenant, mergedFinal)
	c.reportProgress(ctx, tenant, kb, token, 0.90, "writing_products", fmt.Sprintf("Writing %d merged products", len(mergedFinal)))
	if err := c.withWriteLock(ctx, kb, token, func() error {
		if err := c.writer.WriteMerged(ctx, tenant, kb, mergedFinal); err != nil {
			return err
		}
		if len(staleTopicIDs) == 0 {
			return nil
		}
		deleter, ok := c.writer.(interface {
			DeleteMergedWikiPages(context.Context, string, string, []string) error
		})
		if !ok {
			return fmt.Errorf("wiki topic merge requires page deletion support for %d stale page(s)", len(staleTopicIDs))
		}
		return deleter.DeleteMergedWikiPages(ctx, tenant, kb, staleTopicIDs)
	}); err != nil {
		if errors.Is(err, errClaimSuperseded) {
			common.Info("knowledge_compile: batch stale before write, aborting (rewrite barrier)",
				zap.String("dataset_id", kb))
		}
		return err
	}

	// Re-materialize the wiki page graph from the now-consistent merged set. This
	// runs unconditionally after a merge: the graph is a global per-dataset
	// projection rebuilt from a consistent read each time, so it must run even
	// when the current batch only prunes (deletes) wiki pages, and even for a
	// non-wiki batch where the dataset already has no wiki pages (ProjectWikiGraph
	// then just drops any stale graph rows). The graph is reconstructible, so the
	// cost of an occasional no-op reprojection is accepted. It is also a locked
	// destructive side effect for the same TOCTOU reasons as WriteMerged.
	c.reportProgress(ctx, tenant, kb, token, 0.97, "projecting_graph", "Projecting the Wiki graph")
	if err := c.withWriteLock(ctx, kb, token, func() error {
		return c.writer.ProjectWikiGraph(ctx, tenant, kb)
	}); err != nil {
		if errors.Is(err, errClaimSuperseded) {
			common.Info("knowledge_compile: batch stale before graph projection, aborting (rewrite barrier)",
				zap.String("dataset_id", kb))
		}
		return err
	}
	if err := c.commitWikiContributions(ctx, tenant, kb, wikiDiff.currentByDoc, deleted); err != nil {
		return err
	}

	// All merge and delete paths succeeded: commit the staged tombstone
	// clears. These are only now persisted so a failed batch leaves c.tombs
	// untouched and a later reclaim can retry.
	c.mu.Lock()
	for _, docID := range pendingTombClear {
		delete(c.tombs[kb], docID)
	}
	c.mu.Unlock()
	common.Info("knowledge_compile: batch merge complete",
		zap.String("dataset_id", kb),
		zap.Int("completed_docs", len(completed)),
		zap.Int("deleted_docs", len(deleted)),
		zap.Int("merged_rows_written", len(mergedFinal)))
	c.reportProgress(ctx, tenant, kb, token, 1, "completed", fmt.Sprintf("Wiki compilation completed: %d merged products", len(mergedFinal)))
	return nil
}

func copyMeta(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+2)
	for key, value := range input {
		output[key] = value
	}
	return output
}

// mergeEntityModeCandidates handles entity/concept pages by their deterministic
// page identity. A batch may contain evidence for the same page from multiple
// documents; fold it before WriteMerged. Topic pages are returned for the same
// slug-only path.
func (c *Consumer) mergeEntityModeCandidates(ctx context.Context, tenant, kb string, candidates []kccommon.Product) ([]kccommon.Product, []kccommon.Product, error) {
	var entityCandidates []kccommon.Product
	var topic []kccommon.Product
	for _, candidate := range candidates {
		pageType := strings.ToLower(strings.TrimSpace(metaString(candidate.Meta, "page_type")))
		if pageType != "entity" && pageType != "concept" {
			topic = append(topic, candidate)
			continue
		}
		entityCandidates = append(entityCandidates, candidate)
	}
	merged, err := c.mergeWikiProductsBySlug(ctx, tenant, kb, entityCandidates)
	return merged, topic, err
}

// mergeWikiProductsBySlug folds document-level Wiki pages into the dataset
// level using only page_type + slug. Equal identities are merged; different
// slugs remain independent pages. A single document page is already a complete
// Wiki page and passes through unchanged; only groups containing multiple
// document pages need an LLM rewrite.
func (c *Consumer) mergeWikiProductsBySlug(ctx context.Context, tenant, kb string, candidates []kccommon.Product) ([]kccommon.Product, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	groups := make(map[string][]kccommon.Product)
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		key := wikiPageMergeKey(candidate)
		if key == "" {
			// A malformed page without a slug cannot be safely merged with another
			// page. Keep it isolated so it remains visible for diagnosis.
			key = "id\x00" + candidate.ID
		}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], candidate)
	}

	merged := make([]kccommon.Product, 0, len(order))
	rewriteIndexes := make([]int, 0, len(order))
	for _, key := range order {
		group := groups[key]
		topic := selectMergedWikiTopicPath(group)
		var current kccommon.Product
		exists := false
		for _, candidate := range group {
			candidate.Merged = true
			candidate.DocID = kb
			if !exists || current.ID == "" {
				current = candidate
				exists = true
				continue
			}
			if strings.EqualFold(strings.TrimSpace(metaString(current.Meta, "page_type")), "topic") {
				current = mergeTopicPage(current, candidate)
			} else {
				current = wikiEntityMerge(current, candidate)
			}
		}
		if current.ID != "" {
			current.Meta = copyMeta(current.Meta)
			current.Meta["topic"] = topic
			current.Merged = true
			current.DocID = kb
			merged = append(merged, current)
			if len(group) > 1 {
				rewriteIndexes = append(rewriteIndexes, len(merged)-1)
			}
		}
	}
	if err := rewriteMergedWikiPages(ctx, tenant, merged, rewriteIndexes); err != nil {
		return nil, err
	}
	return merged, nil
}

const wikiPageRewriteSystemPrompt = `You are the final editor for a knowledge-base Wiki page.
Rewrite the supplied evidence into one coherent Markdown page.
Preserve every factual detail from the evidence, including names, dates, numbers,
relationships, quoted statements, and links. Do not invent facts or remove
information merely to shorten the page. Organize duplicated or fragmented
evidence into a clear structure. Return Markdown only, without commentary about
the editing process.`

// rewriteMergedWikiPages lets the LLM compose the final dataset-level page
// after deterministic slug grouping has selected the evidence that belongs to
// that page. The calls are independent and use the shared compiler pool.
func rewriteMergedWikiPages(ctx context.Context, tenant string, pages []kccommon.Product, indexes []int) error {
	if len(indexes) == 0 {
		return nil
	}
	deps, err := kccommon.ResolveDeps(tenant, defaultLLMID, defaultEmbedding)
	if err != nil {
		return fmt.Errorf("resolve Wiki page rewrite dependencies: %w", err)
	}
	if deps.Chat == nil {
		return fmt.Errorf("Wiki page rewrite chat model is unavailable")
	}
	maxTokens := deps.ModelMaxOutput
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	jobs := make([]CompilerJob, 0, len(indexes))
	for _, idx := range indexes {
		idx := idx
		jobs = append(jobs, func() error {
			page := pages[idx]
			prompt := fmt.Sprintf("PAGE TYPE: %s\nTITLE: %s\nTOPIC: %s\nENTITIES: %s\nSUMMARY: %s\n\nEVIDENCE MARKDOWN:\n%s",
				metaString(page.Meta, "page_type"),
				metaString(page.Meta, "title"),
				metaString(page.Meta, "topic"),
				strings.Join(metaStringSlice(page.Meta, "entity_names"), ", "),
				metaString(page.Meta, "summary"),
				page.Content)
			response, err := deps.Chat.Chat(ctx, kccommon.ChatRequest{
				LLMID:        defaultLLMID,
				SystemPrompt: wikiPageRewriteSystemPrompt,
				UserPrompt:   prompt,
				Temperature:  floatPtr(0.1),
				MaxTokens:    &maxTokens,
			})
			if err != nil {
				return fmt.Errorf("rewrite Wiki page %q: %w", metaString(page.Meta, "slug"), err)
			}
			if response == nil {
				return fmt.Errorf("rewrite Wiki page %q returned no response", metaString(page.Meta, "slug"))
			}
			content := strings.TrimSpace(response.Content)
			if content == "" {
				return fmt.Errorf("rewrite Wiki page %q returned empty content", metaString(page.Meta, "slug"))
			}
			pages[idx].Content = content
			return nil
		})
	}
	return SubmitCompilerJobs(ctx, jobs)
}

func floatPtr(value float64) *float64 { return &value }

func wikiPageMergeKey(product kccommon.Product) string {
	slug := strings.TrimSpace(metaString(product.Meta, "slug"))
	if slug == "" {
		return ""
	}
	pageType := strings.ToLower(strings.TrimSpace(metaString(product.Meta, "page_type")))
	return pageType + "\x00" + strings.ToLower(strings.Join(strings.Fields(slug), " "))
}

type mergedWikiPageDeleter interface {
	DeleteMergedWikiPages(ctx context.Context, tenant, kb string, ids []string) error
}

func (c *Consumer) deleteMissingScopedWikiPages(ctx context.Context, tenant, kb, token string, affected map[string]struct{}, current []kccommon.Product) error {
	reader, readerOK := c.reader.(mergedWikiPageReader)
	deleter, writerOK := c.writer.(mergedWikiPageDeleter)
	if !readerOK || !writerOK {
		return fmt.Errorf("knowledge_compile: scoped Wiki cleanup is unsupported")
	}
	present := make(map[string]struct{}, len(current))
	for _, page := range current {
		if key := wikiPageMergeKey(page); key != "" {
			present[key] = struct{}{}
		}
	}
	existing, err := reader.LoadMergedWikiPages(ctx, tenant, kb)
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for _, page := range existing {
		key := wikiPageMergeKey(page)
		if _, scoped := affected[key]; !scoped {
			continue
		}
		if _, remains := present[key]; !remains && page.ID != "" {
			ids = append(ids, page.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return c.withWriteLock(ctx, kb, token, func() error {
		return deleter.DeleteMergedWikiPages(ctx, tenant, kb, ids)
	})
}

func refreshWikiProductVectors(ctx context.Context, tenant string, products []kccommon.Product) []kccommon.Product {
	if len(products) == 0 {
		return products
	}
	deps, err := kccommon.ResolveDeps(tenant, defaultLLMID, defaultEmbedding)
	if err != nil || deps.Embed == nil {
		return products
	}
	vectors, err := deps.Embed.Encode(ctx, productContentsForEmbedding(products))
	if err != nil || len(vectors) != len(products) {
		return products
	}
	for i := range products {
		products[i].Vector = vectors[i]
	}
	return products
}

func productContentsForEmbedding(products []kccommon.Product) []string {
	contents := make([]string, len(products))
	for i := range products {
		contents[i] = products[i].Content
	}
	return contents
}

func variantsContain(variants []string, wanted string) bool {
	for _, variant := range variants {
		if variant == wanted {
			return true
		}
	}
	return false
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// candidateIdentity returns a compact identity string for a product, preferring
// the wiki slug (full "<page_type>/<slug>") when present, else the id/doc_id,
// so KNN/dedup diagnostics can tie a candidate to the page it belongs to.
func candidateIdentity(p kccommon.Product) string {
	if slug, _ := p.Meta["slug"].(string); slug != "" {
		return slug
	}
	if id := p.ID; id != "" {
		return id
	}
	if docID := p.DocID; docID != "" {
		return docID
	}
	return "<unknown>"
}

// productsForVariants filters a doc's loaded products down to those whose
// variant is in the requested set. An empty set (legacy backlog entries without
// variants, or a rebuild event that carries none) is treated as "all variants"
// to preserve existing behavior; otherwise a product is kept only when its
// variant appears in variants. This is what makes BacklogEntry.Variants actually
// drive per-variant dispatch instead of handing every doc's full product set to
// every dataset-level path.
func productsForVariants(products []kccommon.Product, variants []string) []kccommon.Product {
	if len(variants) == 0 {
		return products
	}
	want := make(map[string]struct{}, len(variants))
	for _, v := range variants {
		want[v] = struct{}{}
	}
	out := products[:0:0]
	for i := range products {
		if _, ok := want[string(products[i].Variant)]; ok {
			out = append(out, products[i])
		}
	}
	return out
}

// navInputFromProducts extracts the dataset-navigation upsert inputs for the
// tree and structure products in a batch (B2: nav trigger is tree || structure).
// Summary extraction matches the component's by-product hooks:
//   - tree: the root product (Meta.kind=="root") carries the document summary in
//     Content and its vector in Vector.
//   - structure: the graph product (Meta.kind=="graph") carries the graph JSON in
//     Content; its entity descriptions are folded into a document-level summary
//     via pageIndexSummary.
//
// Each document contributes at most one nav input, keyed by DocID, so a doc that
// yields several tree/structure products upserts once. NavService embeds the
// summary itself when Embedd is empty, so the consumer needs no embedder.
func navInputFromProducts(kb string, products []kccommon.Product) []nav.UpsertDocInput {
	type acc struct {
		in nav.UpsertDocInput
	}
	byDoc := make(map[string]*acc, len(products))
	for i := range products {
		p := &products[i]
		switch p.Variant {
		case kccommon.VariantTree:
			if kind, _ := p.Meta["kind"].(string); kind != "root" {
				continue
			}
			a := byDoc[p.DocID]
			if a == nil {
				byDoc[p.DocID] = &acc{in: nav.UpsertDocInput{TenantID: p.TenantID, KbID: kb, DocID: p.DocID}}
				a = byDoc[p.DocID]
			}
			a.in.Summary = p.Content
			a.in.Embedd = p.Vector
		case kccommon.VariantStructure:
			if kind, _ := p.Meta["kind"].(string); kind != "graph" {
				continue
			}
			a := byDoc[p.DocID]
			if a == nil {
				byDoc[p.DocID] = &acc{in: nav.UpsertDocInput{TenantID: p.TenantID, KbID: kb, DocID: p.DocID}}
				a = byDoc[p.DocID]
			}
			// Graph vector is NOT the summary vector; leave Embedd empty so
			// NavService embeds the folded summary text.
			a.in.Summary = pageIndexSummary(p.Content)
		}
	}
	out := make([]nav.UpsertDocInput, 0, len(byDoc))
	for _, a := range byDoc {
		if strings.TrimSpace(a.in.Summary) == "" {
			continue
		}
		out = append(out, a.in)
	}
	return out
}

// pageIndexSummary folds the entity descriptions of a structure graph JSON
// ({"entities":[{"name","description"},...]}) into a document-level summary for
// dataset navigation, matching the component's by-product logic.
func pageIndexSummary(graphJSON string) string {
	var graph struct {
		Entities []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"entities"`
	}
	if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
		return ""
	}
	var b strings.Builder
	for _, e := range graph.Entities {
		desc := strings.Join(strings.Fields(e.Description), " ")
		if desc == "" {
			continue
		}
		if e.Name != "" {
			b.WriteString(e.Name)
			b.WriteString(": ")
		}
		b.WriteString(desc)
		b.WriteString("\n")
	}
	return b.String()
}

// mergeStructureDataset performs the dataset-level structure merge (G1/G4): it
// groups the structure products of a batch by (name, type), folds their
// descriptions and source doc/chunk sets into one StructureBucket per group, and
// writes scope_kwd="dataset" rows via WriteMergedStructure. This mirrors Python
// dataset_structure_merger._merge_bucket but is a self-contained from-scratch
// path (B3: the legacy unified merge never aggregated structure by name/type).
func (c *Consumer) mergeStructureDataset(ctx context.Context, tenant, kb string, products []kccommon.Product) error {
	common.Info("knowledge_compile: mergeStructureDataset entry",
		zap.String("kb_id", kb),
		zap.Int("products", len(products)))
	// Bucket by (lower(name), type, compile kind) for entities and
	// (lower(from), lower(type), lower(to), compile kind) for relations —
	// relations have no "name" and must not be silently dropped (review issue 3),
	// and the relation type + compile kind keep distinct structure kinds and
	// distinct relation types from collapsing into one dataset row.
	type ekey struct{ name, typ, ckwd string }
	type rkey struct{ from, typ, to, ckwd string }
	entByKey := make(map[ekey]*StructureBucket, 16)
	relByKey := make(map[rkey]*StructureBucket, 16)
	// The authoritative Python (dataset_structure_merger._do_build) merges EVERY
	// structure-kind doc row unconditionally — dataset scope is driven by the
	// task type (structure_graph/timeline/structure_mindmap), NOT by a template
	// Config["dataset_merge"] flag (that was a stale runner.py concept). So
	// structure/mindmap entity/relation products always enter the dataset merge.
	for _, p := range products {
		if p.Variant != kccommon.VariantStructure && p.Variant != kccommon.VariantMindmap {
			continue
		}
		// Dataset rows carry the SAME compile_kwd as the doc rows (the inferred
		// compile type / autotype: "hypergraph"/"timeline"/"mindmap"/"list"/…),
		// mirroring Python dataset_structure_merger._do_build which passes the doc
		// row's compile_kwd through verbatim. The template kind (p.Kind, restored
		// from compilation_template_kind_kwd) is stored on the SEPARATE
		// compilation_template_kind_kwd field and is what read/delete paths match
		// on — NOT compile_kwd. Do not rewrite compile_kwd to the template kind:
		// that would diverge from Python (which never does) and split doc vs
		// dataset rows.
		ckwd := metaString(p.Meta, "compile_kwd")
		if ckwd == "" {
			ckwd = compileKwdForVariant(p.Variant)
		}
		kind := metaString(p.Meta, "kind")
		from := metaString(p.Meta, "from")
		to := metaString(p.Meta, "to")
		if kind == "relation" || (from != "" && to != "") {
			// Skip a relation with an incomplete endpoint pair: writing a bucket
			// like "A -> " would let multiple malformed relations collapse into
			// the same dataset row.
			if from == "" || to == "" {
				continue
			}
			relType := metaString(p.Meta, "relation_type")
			if relType == "" {
				relType = metaString(p.Meta, "type")
			}
			if relType == "" {
				relType = "related"
			}
			k := rkey{from: strings.ToLower(from), typ: strings.ToLower(relType), to: strings.ToLower(to), ckwd: ckwd}
			b := relByKey[k]
			if b == nil {
				b = &StructureBucket{Name: from + " -> " + to, Type: "relation", FromEntity: from, ToEntity: to, CompileKwd: ckwd, TemplateID: p.TemplateID, TemplateKind: p.Kind, RelationType: relType}
				relByKey[k] = b
			}
			appendBucket(b, p)
			continue
		}
		// entity bucket
		name := metaString(p.Meta, "name")
		typ := metaString(p.Meta, "entity_type")
		if typ == "" {
			typ = metaString(p.Meta, "type")
		}
		if name == "" {
			continue
		}
		k := ekey{name: strings.ToLower(name), typ: typ, ckwd: ckwd}
		b := entByKey[k]
		if b == nil {
			b = &StructureBucket{Name: name, Type: typ, CompileKwd: ckwd, TemplateID: p.TemplateID, TemplateKind: p.Kind}
			entByKey[k] = b
		}
		appendBucket(b, p)
	}
	buckets := make([]StructureBucket, 0, len(entByKey)+len(relByKey))
	for _, b := range entByKey {
		buckets = append(buckets, *b)
	}
	for _, b := range relByKey {
		buckets = append(buckets, *b)
	}
	common.Info("knowledge_compile: mergeStructureDataset buckets",
		zap.String("kb_id", kb),
		zap.Int("entities", len(entByKey)),
		zap.Int("relations", len(relByKey)),
		zap.Int("total_buckets", len(buckets)))
	if len(buckets) == 0 {
		return nil
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Name != buckets[j].Name {
			return buckets[i].Name < buckets[j].Name
		}
		return buckets[i].Type < buckets[j].Type
	})
	return c.writer.WriteMergedStructure(ctx, tenant, kb, buckets)
}

// appendBucket folds a structure product into a bucket: concatenates its
// description, unions its source doc/chunk ids, and folds its vector into a true
// element-wise running mean (review issue 13). VecCount tracks how many vectors
// have been folded so the mean is order-independent.
func appendBucket(b *StructureBucket, p kccommon.Product) {
	// The product's Content is the doc row's full content_with_weight JSON
	// (e.g. {"category":"Person","description":"…","name":"…","type":"entity"}),
	// NOT the plain-text description. Extract the "description" field so the
	// dataset row's description stays a plain-text string (mirror Python
	// _struct_merge_graph_entities, which folds the entity description, not the
	// whole payload). Fall back to Content only when it is not a JSON object.
	if desc := structureProductDescription(p.Content); desc != "" {
		if b.Description != "" {
			b.Description += "\n"
		}
		b.Description += desc
	}
	b.SourceDocIDs = appendUnique(b.SourceDocIDs, []string{p.DocID})
	b.SourceChunkIDs = appendUnique(b.SourceChunkIDs, metaStringSlice(p.Meta, "source_chunk_ids"))
	// mention_count_int: sum the per-entity mention counts (Python
	// _struct_merge_graph_entities sums mention_count across the merged entities).
	// Each entity product carries mention_count ≥ 1 (structure compile.go L231);
	// mindmap entities carry none, so they contribute a default of 0 and are
	// simply not stamped (mention_count_int is omitted when 0).
	if mc, ok := metaInt(p.Meta, "mention_count"); ok && mc > 0 {
		b.MentionCount += int(mc)
	}
	if len(p.Vector) > 0 {
		b.VecCount++
		if len(b.Vector) == 0 {
			b.Vector = append([]float32(nil), p.Vector...)
		} else {
			n := len(p.Vector)
			if len(b.Vector) < n {
				b.Vector = append(b.Vector, make([]float32, n-len(b.Vector))...)
			}
			for i := 0; i < n; i++ {
				// running mean: acc + (next - acc)/count
				b.Vector[i] += (p.Vector[i] - b.Vector[i]) / float32(b.VecCount)
			}
		}
	}
}

// structureProductDescription extracts the plain-text description from a structure
// product's Content. In production Content is the doc row's content_with_weight
// JSON ({"description":"…","name":"…","type":"…"}), so we parse it and return the
// "description" field — the whole payload must NOT leak into the dataset row's
// description (that is the bug: projectEntity then renders the raw JSON object).
// When Content is NOT a JSON object (plain text, legacy/unit-test rows), it is
// returned verbatim so existing folded-description behavior is preserved.
func structureProductDescription(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if content[0] == '{' {
		var m map[string]any
		if err := json.Unmarshal([]byte(content), &m); err == nil {
			if d, ok := m["description"].(string); ok {
				if s := strings.TrimSpace(d); s != "" {
					return s
				}
			}
			// JSON object without a plain-text description: nothing to fold.
			return ""
		}
	}
	return content
}

// appendUnique appends only values not already present, preserving order.
func appendUnique(dst, values []string) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
}

// upsertNavLocked places the batch's tree/structure summaries into the dataset
// navigation tree under the per-dataset write/rebuild lock, with the claim-token
// check inside the lock (B7: nav writes are claim-fenced like any other
// destructive write so a stale worker cannot repopulate a rebuilt tree).
func (c *Consumer) upsertNavLocked(ctx context.Context, tenant, kb, token string, ns nav.NavService, inputs []nav.UpsertDocInput) error {
	return c.withWriteLock(ctx, kb, token, func() error {
		for i := range inputs {
			// The minimal-loop NavService.UpsertDoc is idempotent by summary; a
			// failure here aborts the batch so the claim is not acked and the
			// batch is retried.
			if err := ns.UpsertDoc(ctx, inputs[i]); err != nil {
				return fmt.Errorf("knowledge_compile: nav upsert %s: %w", inputs[i].DocID, err)
			}
		}
		return nil
	})
}
