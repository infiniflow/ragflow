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
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"

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
	Publish(ctx context.Context, tenantID, datasetID, docID, eventType string) error
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
		return
	}
	if _, err := c.scheduler.Ack(ctx, datasetID, cr.Token, cr.Entries); err != nil {
		common.Warn("knowledge_compile: ack failed",
			zap.String("dataset_id", datasetID), zap.Error(err))
	}
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
		if derr := c.writer.DeleteMerged(ctx, tenant, kb); derr != nil {
			return derr
		}
		return c.writer.DropWikiGraph(ctx, tenant, kb)
	}); err != nil {
		return fmt.Errorf("knowledge_compile: rebuild clear merged+graph: %w", err)
	}

	// 4. enumerate every doc and republish.
	docs, err := c.docLister(ctx, tenant, kb)
	if err != nil {
		return fmt.Errorf("knowledge_compile: rebuild list docs: %w", err)
	}
	for _, docID := range docs {
		if perr := rs.Publish(ctx, tenant, kb, docID, string(EventTypeCompleted)); perr != nil {
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

// filterWikiPageCandidates returns only the candidates the dataset-level merge
// is allowed to fold: for the wiki variant, that is strictly page-kind products
// (Meta.kind == "page"). Sections are a doc-level concern and must never enter
// the dataset-level page bucket, and the deduper must not carry an implicit
// section-filter contract. Non-wiki variants pass through unchanged. Legacy wiki
// rows whose kind is empty are derived in the Reader; only truly page-kind wiki
// products proceed.
func filterWikiPageCandidates(candidates []kccommon.Product) []kccommon.Product {
	// Allocate a fresh slice: reusing candidates[:0] would overwrite the caller's
	// backing array in place, corrupting any other reference (e.g. the vector
	// audit that still reads `incoming`).
	filtered := make([]kccommon.Product, 0, len(candidates))
	for _, cand := range candidates {
		if cand.Variant == kccommon.VariantWiki {
			if metaString(cand.Meta, "kind") != "page" {
				continue
			}
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
	}
	byDoc := make(map[string]*docState, len(entries))
	for _, e := range entries {
		et := EventType(e.EventType)
		st := byDoc[e.DocID]
		if st == nil {
			byDoc[e.DocID] = &docState{winEvent: et}
			continue
		}
		// Later entries overwrite earlier ones: last event wins.
		st.winEvent = et
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
			completed = append(completed, BacklogEntry{DocID: docID, EventType: string(EventTypeCompleted)})
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

	// wiki_incremental port (M1): the dataset-level merge only processes wiki
	// PAGES. A wiki doc yields both page and section products (Meta.kind
	// "page"/"section"); sections are a doc-level concern and must never be folded
	// into the dataset-level page bucket. Filter BEFORE the in-memory dedup so
	// sections never enter the deduper (and never trigger LLM/embedding/alias
	// processing) — the decider must not carry an implicit section-filter
	// contract. Legacy rows whose kind is empty are derived in the Reader; only
	// truly page-kind wiki products proceed.
	candidates := filterWikiPageCandidates(incoming)
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
	// The KNN pass is docengine-bounded (vector search), not CPU-bounded, so we
	// fan it out across the shared global compilerPool (vCPU-sized). Output order
	// is irrelevant: merged rows are upserted by their idempotent dataset-level
	// id, and each candidate lands in exactly one group / the unmatched set.
	jobs := make([]CompilerJob, 0, len(candidates))
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

	// Fold every KNN group into the LLM in a single batch round-trip (one
	// DecideBatch call instead of one Decide per pair), then collect the
	// updated existing rows and the candidates judged distinct (new rows).
	var newMerged []kccommon.Product
	if len(groupsByID) > 0 {
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
	if err := c.withWriteLock(ctx, kb, token, func() error {
		return c.writer.WriteMerged(ctx, tenant, kb, mergedFinal)
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
	if err := c.withWriteLock(ctx, kb, token, func() error {
		return c.writer.ProjectWikiGraph(ctx, tenant, kb)
	}); err != nil {
		if errors.Is(err, errClaimSuperseded) {
			common.Info("knowledge_compile: batch stale before graph projection, aborting (rewrite barrier)",
				zap.String("dataset_id", kb))
		}
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
	return nil
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
