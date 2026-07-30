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

// fakeReader returns a fixed product set.
type fakeReader struct {
	mu       sync.Mutex
	products []kccommon.Product
	calls    int
}

func (r *fakeReader) LoadCompiledProducts(_ context.Context, _, _ string) ([]kccommon.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	out := make([]kccommon.Product, len(r.products))
	copy(out, r.products)
	return out, nil
}

// fakeWriter captures written merged products.
type fakeWriter struct {
	mu      sync.Mutex
	written [][]kccommon.Product
	deleted []string
}

func (w *fakeWriter) WriteMerged(_ context.Context, _, _ string, products []kccommon.Product) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]kccommon.Product, len(products))
	copy(cp, products)
	w.written = append(w.written, cp)
	return nil
}

func (w *fakeWriter) DeleteMergedForDoc(_ context.Context, _, _, docID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deleted = append(w.deleted, docID)
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
		WithBatchSize(32),
	)
}

func TestConsumerCompletedWritesMerged(t *testing.T) {
	sch := NewFakeScheduler()
	r := &fakeReader{products: sampleProducts()}
	w := &fakeWriter{}
	c := newTestConsumer(sch, r, w, func(string) (Deduper, error) { return NewNoopDeduper(), nil })

	if err := sch.AppendBacklog(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("append: %v", err)
	}
	c.processDataset(context.Background(), "kb1")

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.written) != 1 {
		t.Fatalf("expected 1 WriteMerged call, got %d", len(w.written))
	}
	if len(w.written[0]) != 2 {
		t.Fatalf("expected 2 merged products, got %d", len(w.written[0]))
	}
	// After ack, the claim row must be cleared (no live lease left behind).
	if got, _ := sch.FindClaimable(context.Background(), 1); len(got) != 0 {
		t.Fatalf("expected no claimable row after ack, got %v", got)
	}
}

func TestConsumerTombstoneSkipsCompletedBeforeDeleted(t *testing.T) {
	sch := NewFakeScheduler()
	r := &fakeReader{products: sampleProducts()}
	w := &fakeWriter{}
	c := newTestConsumer(sch, r, w, func(string) (Deduper, error) { return NewNoopDeduper(), nil })

	// deleted before completed -> completed must be skipped.
	if err := sch.AppendBacklog(context.Background(), "t1", "kb1", "d1", string(EventTypeDeleted), 1); err != nil {
		t.Fatalf("append deleted: %v", err)
	}
	if err := sch.AppendBacklog(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 2); err != nil {
		t.Fatalf("append completed: %v", err)
	}
	c.processDataset(context.Background(), "kb1")

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.deleted) != 1 {
		t.Fatalf("expected 1 orphan-delete call for d1, got %d", len(w.deleted))
	}
}

func TestSchedulerClaimClosedBatch(t *testing.T) {
	sch := NewFakeScheduler()
	for i := 0; i < 40; i++ {
		docID := "d" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		if err := sch.AppendBacklog(context.Background(), "t1", "kb1", docID, string(EventTypeCompleted), uint64(i)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// First claim returns the bounded prefix (batchSize=32), not all 40.
	cr1, ok, err := sch.Claim(context.Background(), "kb1", 32)
	if err != nil || !ok {
		t.Fatalf("claim1: ok=%v err=%v", ok, err)
	}
	if len(cr1.Entries) != 32 {
		t.Fatalf("expected 32 entries in first claim, got %d", len(cr1.Entries))
	}
	// A second claim by the same holder (still live lease) must not re-claim
	// the same dataset until the first batch is acked.
	_, ok2, _ := sch.Claim(context.Background(), "kb1", 32)
	if ok2 {
		t.Fatalf("second claim should have lost the race (live lease)")
	}
	// Ack the first batch, then the remaining 8 become claimable.
	if _, err := sch.Ack(context.Background(), "kb1", cr1.Token, cr1.Entries); err != nil {
		t.Fatalf("ack: %v", err)
	}
	cr3, ok3, err := sch.Claim(context.Background(), "kb1", 32)
	if err != nil || !ok3 {
		t.Fatalf("claim3: ok=%v err=%v", ok3, err)
	}
	if len(cr3.Entries) != 8 {
		t.Fatalf("expected 8 remaining entries, got %d", len(cr3.Entries))
	}
}

func TestSchedulerReclaimExpired(t *testing.T) {
	sch := NewFakeScheduler()
	if err := sch.AppendBacklog(context.Background(), "t1", "kb1", "d1", string(EventTypeCompleted), 1); err != nil {
		t.Fatalf("append: %v", err)
	}
	_, ok, err := sch.Claim(context.Background(), "kb1", 32)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	// Simulate a crash: the inflight is never acked. Reclaim moves it back.
	n, err := sch.ReclaimExpired(context.Background(), time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reclaimed entry, got %d", n)
	}
	// The entry is claimable again.
	cr2, ok2, err := sch.Claim(context.Background(), "kb1", 32)
	if err != nil || !ok2 {
		t.Fatalf("reclaim claim: ok=%v err=%v", ok2, err)
	}
	if len(cr2.Entries) != 1 {
		t.Fatalf("expected 1 entry after reclaim, got %d", len(cr2.Entries))
	}
}
