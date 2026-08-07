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
	"testing"
	"time"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// fakeReader returns a fixed per-document product set, keyed by docID.
type fakeReader struct {
	mu       sync.Mutex
	products []kccommon.Product
	calls    int
}

func (r *fakeReader) LoadDocProducts(_ context.Context, _, _, docID string) ([]kccommon.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	var out []kccommon.Product
	for _, p := range r.products {
		if p.DocID == docID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakeReader) SearchSimilar(_ context.Context, _, _ string, _ kccommon.Variant, _ []float64, _ int, _ float64) (kccommon.Product, float64, error) {
	return kccommon.Product{}, 0, nil
}

// fakeWriter captures written merged products.
type fakeWriter struct {
	mu              sync.Mutex
	written         [][]kccommon.Product
	deletedDocLevel []string
	strippedSources []string
}

func (w *fakeWriter) WriteMerged(_ context.Context, _, _ string, products []kccommon.Product) error {
	if len(products) == 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]kccommon.Product, len(products))
	copy(cp, products)
	w.written = append(w.written, cp)
	return nil
}

func (w *fakeWriter) DeleteDocLevelForDocs(_ context.Context, _, _ string, docIDs []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deletedDocLevel = append(w.deletedDocLevel, docIDs...)
	return nil
}

func (w *fakeWriter) StripMergedSources(_ context.Context, _, _ string, docIDs []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.strippedSources = append(w.strippedSources, docIDs...)
	return nil
}

func sampleProducts() []kccommon.Product {
	return []kccommon.Product{
		{ID: "p1", DocID: "d1", TenantID: "t1", Variant: kccommon.Variant("structure"),
			Content: `{"name":"X"}`, Meta: map[string]any{"name": "X", "kind": "entity"}},
		{ID: "p2", DocID: "d1", TenantID: "t1", Variant: kccommon.Variant("structure"),
			Content: `{"name":"Y"}`, Meta: map[string]any{"name": "Y", "kind": "entity"}},
	}
}

func newTestConsumer(sch *FakeScheduler, r *fakeReader, w *fakeWriter, factory DeduperFactory) *Consumer {
	return NewConsumer(sch,
		WithReader(r),
		WithWriter(w),
		WithDeduperFactory(factory),
	)
}

func TestConsumerCompletedWritesMerged(t *testing.T) {
	sch := NewFakeScheduler()
	r := &fakeReader{products: sampleProducts()}
	w := &fakeWriter{}
	c := newTestConsumer(sch, r, w, func(string) (Deduper, error) { return NewNoopDeduper(), nil })

	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("append: %v", err)
	}
	c.tryClaimAndProcess(context.Background())

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.written) != 1 {
		t.Fatalf("expected 1 WriteMerged call, got %d", len(w.written))
	}
	if len(w.written[0]) != 2 {
		t.Fatalf("expected 2 merged products, got %d", len(w.written[0]))
	}
	// After ack, the claim row must be cleared (no live lease left behind).
	if _, ok, _ := sch.TryClaim(context.Background()); ok {
		t.Fatalf("expected no claimable row after ack")
	}
}

func TestConsumerTombstoneSkipsCompletedBeforeDeleted(t *testing.T) {
	sch := NewFakeScheduler()
	r := &fakeReader{products: sampleProducts()}
	w := &fakeWriter{}
	c := newTestConsumer(sch, r, w, func(string) (Deduper, error) { return NewNoopDeduper(), nil })

	// completed (seq 1) before deleted (seq 2): the completion is the stale
	// original, so it must be skipped and d1's per-doc products orphaned.
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("append completed: %v", err)
	}
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeDeleted), 2); err != nil {
		t.Fatalf("append deleted: %v", err)
	}
	c.tryClaimAndProcess(context.Background())

	w.mu.Lock()
	defer w.mu.Unlock()
	// No merged write (completed skipped). The deletion is handled entirely on
	// the DocEngine: d1's per-doc products are dropped in one call and d1 is
	// stripped from every dataset-level product in one call. No products are
	// loaded into memory.
	if len(w.written) != 0 {
		t.Fatalf("expected no merged write, got %d", len(w.written))
	}
	if len(w.deletedDocLevel) != 1 || w.deletedDocLevel[0] != "d1" {
		t.Fatalf("expected DeleteDocLevelForDocs([d1]), got %v", w.deletedDocLevel)
	}
	if len(w.strippedSources) != 1 || w.strippedSources[0] != "d1" {
		t.Fatalf("expected StripMergedSources([d1]), got %v", w.strippedSources)
	}
}

func TestConsumerReingestAfterDeletionWins(t *testing.T) {
	sch := NewFakeScheduler()
	r := &fakeReader{products: sampleProducts()}
	w := &fakeWriter{}
	c := newTestConsumer(sch, r, w, func(string) (Deduper, error) { return NewNoopDeduper(), nil })

	// deleted (seq 1) then completed (seq 2): the completion has the higher
	// sequence, so it wins — the doc is re-ingested, NOT deleted. The deletion
	// must not drop its per-doc products, and the completion must be merged.
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeDeleted), 1); err != nil {
		t.Fatalf("append deleted: %v", err)
	}
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 2); err != nil {
		t.Fatalf("append completed: %v", err)
	}
	c.tryClaimAndProcess(context.Background())

	w.mu.Lock()
	defer w.mu.Unlock()
	// No deletion calls: the higher-seq completion overrides the deletion.
	if len(w.deletedDocLevel) != 0 {
		t.Fatalf("expected no DeleteDocLevelForDocs, got %v", w.deletedDocLevel)
	}
	if len(w.strippedSources) != 0 {
		t.Fatalf("expected no StripMergedSources, got %v", w.strippedSources)
	}
	// The completion is merged into the dataset-level products.
	if len(w.written) != 1 {
		t.Fatalf("expected 1 WriteMerged call, got %d", len(w.written))
	}
	if len(w.written[0]) != 2 {
		t.Fatalf("expected 2 merged products, got %d", len(w.written[0]))
	}
}

func TestSchedulerClaimClosedBatch(t *testing.T) {
	sch := NewFakeScheduler()
	for i := 0; i < 40; i++ {
		docID := "d" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		if err := sch.Publish(context.Background(), "t1", "kb1", docID, string(EventTypeCompleted), uint64(i)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// First claim returns the bounded prefix (default batch=32), not all 40.
	cr1, ok, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim1: ok=%v err=%v", ok, err)
	}
	if len(cr1.Entries) != 32 {
		t.Fatalf("expected 32 entries in first claim, got %d", len(cr1.Entries))
	}
	// A second claim by the same holder (still live lease) must not re-claim
	// the same dataset until the first batch is acked.
	_, ok2, _ := sch.Claim(context.Background(), "kb1")
	if ok2 {
		t.Fatalf("second claim should have lost the race (live lease)")
	}
	// Ack the first batch, then the remaining 8 become claimable.
	if _, err := sch.Ack(context.Background(), "kb1", cr1.Token, cr1.Entries); err != nil {
		t.Fatalf("ack: %v", err)
	}
	cr3, ok3, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !ok3 {
		t.Fatalf("claim3: ok=%v err=%v", ok3, err)
	}
	if len(cr3.Entries) != 8 {
		t.Fatalf("expected 8 remaining entries, got %d", len(cr3.Entries))
	}
}

func TestSchedulerReclaimExpired(t *testing.T) {
	sch := NewFakeScheduler()
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("append: %v", err)
	}
	_, ok, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	// Simulate a crash: the inflight is never acked and the lease has expired.
	past := time.Now().Add(-time.Hour)
	sch.rows["kb1"].expires = &past
	// TryClaim reclaims the expired lease back into backlog and immediately
	// claims it again.
	cr2, ok2, err := sch.TryClaim(context.Background())
	if err != nil || !ok2 {
		t.Fatalf("reclaim claim: ok=%v err=%v", ok2, err)
	}
	if len(cr2.Entries) != 1 {
		t.Fatalf("expected 1 entry after reclaim, got %d", len(cr2.Entries))
	}
}

// rowCounts snapshots a fake scheduling row's inflight/backlog entry counts.
type rowCounts struct {
	inflight int
	backlog  int
}

func fakeRowCounts(sch *FakeScheduler, datasetID string) (rowCounts, string) {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	r := sch.rows[datasetID]
	if r == nil {
		return rowCounts{}, ""
	}
	return rowCounts{inflight: len(r.inflight), backlog: len(r.backlog)}, r.state
}

// TestSchedulerStateMachineLocksStateAndCounts locks the full lifecycle state
// machine (plan v4.1 §9.2) on the FakeScheduler, which mirrors the MySQL
// scheduler's transitions:
//
//	Publish -> pending; Claim -> running (+error cleared);
//	SetError (failed batch left for retry) keeps running + records error;
//	lease expiry -> reclaimOne -> pending (inflight moved back to backlog);
//	Ack with backlog drained -> completed.
func TestSchedulerStateMachineLocksStateAndCounts(t *testing.T) {
	sch := NewFakeScheduler()

	// Publish two docs: state=pending, backlog=2, inflight=0.
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("publish d1: %v", err)
	}
	if err := sch.Publish(context.Background(), "t1", "kb1", "d2", string(EventTypeCompleted), 2); err != nil {
		t.Fatalf("publish d2: %v", err)
	}
	if c, s := fakeRowCounts(sch, "kb1"); s != DatasetStatePending || c.backlog != 2 || c.inflight != 0 {
		t.Fatalf("after publish: want state=pending backlog=2 inflight=0, got state=%s %+v", s, c)
	}

	// Claim: state=running, backlog moves to inflight (batch=2), error cleared.
	cr, ok, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if len(cr.Entries) != 2 {
		t.Fatalf("expected 2-entry claim, got %d", len(cr.Entries))
	}
	if c, s := fakeRowCounts(sch, "kb1"); s != DatasetStateRunning || c.backlog != 0 || c.inflight != 2 {
		t.Fatalf("after claim: want state=running backlog=0 inflight=2, got state=%s %+v", s, c)
	}

	// Failed batch left for retry: SetError records a diagnostic, state stays running.
	if err := sch.SetError(context.Background(), "kb1", cr.Token, "boom"); err != nil {
		t.Fatalf("set error: %v", err)
	}
	sch.mu.Lock()
	gotErr := sch.rows["kb1"].errorMsg
	sch.mu.Unlock()
	if gotErr != "boom" {
		t.Fatalf("expected errorMsg=boom, got %q", gotErr)
	}
	if _, s := fakeRowCounts(sch, "kb1"); s != DatasetStateRunning {
		t.Fatalf("failed batch must stay running (left for retry), got state=%s", s)
	}

	// Lease expires: reclaimOne moves inflight back to backlog, clears lease -> pending.
	past := time.Now().Add(-time.Hour)
	sch.mu.Lock()
	sch.rows["kb1"].expires = &past
	sch.mu.Unlock()
	// Lock the reclaim transition in isolation (before any re-claim): the fake's
	// reclaim helper is the same code path TryClaim uses, mirroring reclaimOne.
	sch.mu.Lock()
	if id := sch.fakeReclaimExpired(time.Now()); id != "kb1" {
		sch.mu.Unlock()
		t.Fatalf("expected kb1 to be reclaimed, got %q", id)
	}
	sch.mu.Unlock()
	if c, s := fakeRowCounts(sch, "kb1"); s != DatasetStatePending || c.backlog != 2 || c.inflight != 0 {
		t.Fatalf("after reclaim: want state=pending backlog=2 inflight=0, got state=%s %+v", s, c)
	}

	// Claim again then Ack the drained batch -> completed, counts zeroed.
	cr2, ok3, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !ok3 {
		t.Fatalf("re-claim: ok=%v err=%v", ok3, err)
	}
	if _, err := sch.Ack(context.Background(), "kb1", cr2.Token, cr2.Entries); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if c, s := fakeRowCounts(sch, "kb1"); s != DatasetStateCompleted || c.backlog != 0 || c.inflight != 0 {
		t.Fatalf("after ack drain: want state=completed backlog=0 inflight=0, got state=%s %+v", s, c)
	}
}

// TestSchedulerSetErrorScopedToClaimToken locks the concurrency guard on
// SetError: a failed batch is diagnosed only while its own claim token is still
// live. If worker A's lease expires and is reclaimed, worker B re-claims and
// completes; a late SetError from A (stale token) must NOT overwrite the row's
// diagnostic, otherwise a completed state would be misread as failed.
func TestSchedulerSetErrorScopedToClaimToken(t *testing.T) {
	sch := NewFakeScheduler()
	if err := sch.Publish(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// A claims and begins processing (running).
	crA, ok, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim A: ok=%v err=%v", ok, err)
	}

	// A's lease expires before it finishes; the sweeper reclaims the inflight
	// batch back to backlog (pending) and B takes over.
	past := time.Now().Add(-time.Hour)
	sch.mu.Lock()
	sch.rows["kb1"].expires = &past
	sch.mu.Unlock()
	if id := sch.fakeReclaimExpired(time.Now()); id != "kb1" {
		t.Fatalf("expected kb1 reclaimed, got %q", id)
	}
	crB, okB, err := sch.Claim(context.Background(), "kb1")
	if err != nil || !okB {
		t.Fatalf("claim B: ok=%v err=%v", okB, err)
	}
	if crA.Token == crB.Token {
		t.Fatalf("expected distinct claim tokens, got %q", crA.Token)
	}

	// B succeeds and drains the backlog to completed.
	if _, err := sch.Ack(context.Background(), "kb1", crB.Token, crB.Entries); err != nil {
		t.Fatalf("ack B: %v", err)
	}
	if c, s := fakeRowCounts(sch, "kb1"); s != DatasetStateCompleted || c.backlog != 0 || c.inflight != 0 {
		t.Fatalf("after B ack: want completed empty, got state=%s %+v", s, c)
	}

	// A's late failure arrives with its now-stale token: it must be ignored.
	if err := sch.SetError(context.Background(), "kb1", crA.Token, "stale failure from A"); err != nil {
		t.Fatalf("stale set error: %v", err)
	}
	sch.mu.Lock()
	gotErr := sch.rows["kb1"].errorMsg
	sch.mu.Unlock()
	if gotErr != "" {
		t.Fatalf("stale SetError overwrote diagnostic: got errorMsg=%q want empty", gotErr)
	}
	if _, s := fakeRowCounts(sch, "kb1"); s != DatasetStateCompleted {
		t.Fatalf("stale SetError changed state: got %q want completed", s)
	}
}
