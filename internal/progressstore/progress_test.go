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
//

package progressstore

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeKV struct {
	mu     sync.Mutex
	data   map[string][]byte
	writes int
}

func newFakeKV() *fakeKV { return &fakeKV{data: map[string][]byte{}} }

func (f *fakeKV) GetObj(_ context.Context, key string, dest any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, ok := f.data[key]
	if !ok {
		return false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false
	}
	return true
}

func (f *fakeKV) SetObj(_ context.Context, key string, obj any, _ time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	f.data[key] = raw
	f.writes++
	return true
}

func newTestStore(kv kvClient) *Store { return &Store{kv: kv} }

func loadSnapshot(t *testing.T, kv *fakeKV, id string) *Snapshot {
	t.Helper()
	var snap Snapshot
	if !kv.GetObj(context.Background(), Key(id), &snap) {
		t.Fatalf("snapshot for %q not stored", id)
	}
	return &snap
}

func findStage(snap *Snapshot, name string) *Stage {
	for i := range snap.Stages {
		if snap.Stages[i].Name == name {
			return &snap.Stages[i]
		}
	}
	return nil
}

func TestUpdateIsMonotonicWithinUnit(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()

	if !s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 100, 500) {
		t.Fatal("first update failed")
	}
	if !s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 250, 500) {
		t.Fatal("forward update failed")
	}
	// Out-of-order settle: a late callback reports a smaller position.
	if s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 120, 500) {
		t.Fatal("stale update should be rejected")
	}

	snap := loadSnapshot(t, kv, "run1")
	stage := findStage(snap, "Parser")
	if stage == nil {
		t.Fatal("Parser stage missing")
	}
	got := stage.Units["page:parse"]
	if got.Current != 250 || got.Total != 500 {
		t.Fatalf("position = %+v, want {250 500}", got)
	}
	if snap.Status != "RUNNING" {
		t.Fatalf("status = %q, want RUNNING", snap.Status)
	}
}

func TestUpdateKeepsUnitsIndependent(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()

	s.Update(ctx, "run1", "doc1", "ds1", "Extractor", "chunk:keywords", 5047, 5047)
	// A different phase restarts from 1; it must not overwrite the first unit.
	s.Update(ctx, "run1", "doc1", "ds1", "Extractor", "chunk:tags", 1, 5047)

	snap := loadSnapshot(t, kv, "run1")
	stage := findStage(snap, "Extractor")
	if stage == nil {
		t.Fatal("Extractor stage missing")
	}
	if stage.Units["chunk:keywords"].Current != 5047 {
		t.Fatalf("keywords regressed to %d", stage.Units["chunk:keywords"].Current)
	}
	if stage.Units["chunk:tags"].Current != 1 {
		t.Fatalf("tags = %d, want 1", stage.Units["chunk:tags"].Current)
	}
	if stage.Active != "chunk:tags" {
		t.Fatalf("active = %q, want chunk:tags", stage.Active)
	}
}

func TestUpdateSkipsWriteWhenUnchanged(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()

	s.Update(ctx, "run1", "doc1", "ds1", "Tokenizer", "chunk:embed", 10, 100)
	before := kv.writes
	s.Update(ctx, "run1", "doc1", "ds1", "Tokenizer", "chunk:embed", 10, 100)
	s.Update(ctx, "run1", "doc1", "ds1", "Tokenizer", "chunk:embed", 9, 100)
	if kv.writes != before {
		t.Fatalf("writes = %d, want unchanged %d for stale/equal update", kv.writes, before)
	}
}

func TestSetPhaseAndMarkTerminal(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()

	s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 10, 100)
	if !s.SetPhase(ctx, "run1", "doc1", "ds1", "Parser", "done") {
		t.Fatal("SetPhase failed")
	}
	snap := loadSnapshot(t, kv, "run1")
	if got := findStage(snap, "Parser").Phase; got != "done" {
		t.Fatalf("phase = %q, want done", got)
	}

	if !s.MarkTerminal(ctx, "run1", "doc1", "ds1", "COMPLETED") {
		t.Fatal("MarkTerminal failed")
	}
	// Idempotent: a second terminal call must not change status or fail.
	s.MarkTerminal(ctx, "run1", "doc1", "ds1", "FAILED")
	snap = loadSnapshot(t, kv, "run1")
	if snap.Status != "COMPLETED" {
		t.Fatalf("status = %q, want COMPLETED (terminal is one-shot)", snap.Status)
	}
}

func TestMarkTerminalCreatesSnapshotWhenAbsent(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	if !s.MarkTerminal(context.Background(), "run1", "doc1", "ds1", "FAILED") {
		t.Fatal("MarkTerminal failed on absent key")
	}
	snap := loadSnapshot(t, kv, "run1")
	if snap.Status != "FAILED" || snap.DocID != "doc1" || snap.DatasetID != "ds1" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestLoadMissingKey(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	snap, ok := s.Load(context.Background(), "missing")
	if ok || snap != nil {
		t.Fatalf("Load missing = (%+v, %v), want (nil, false)", snap, ok)
	}
}

func TestUnitTagSurvivesRoundTrip(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()
	s.Update(ctx, "run1", "doc1", "ds1", "Compiler", "page:refine", 31, 31)
	snap := loadSnapshot(t, kv, "run1")
	stage := findStage(snap, "Compiler")
	if stage == nil || stage.Units["page:refine"].Total != 31 {
		t.Fatalf("tagged unit lost: %+v", stage)
	}
}

func BenchmarkStoreUpdate(b *testing.B) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", uint32(i%5000+1), 5000)
	}
}

// racingKV injects a terminal write between an Update's load and save,
// deterministically reproducing the stale-status overwrite without a lock.
type racingKV struct {
	*fakeKV
	hook   func()
	once   sync.Once
	inHook atomic.Bool
}

func (r *racingKV) GetObj(ctx context.Context, key string, dest any) bool {
	if r.inHook.CompareAndSwap(false, true) {
		r.once.Do(r.hook)
		r.inHook.Store(false)
	}
	return r.fakeKV.GetObj(ctx, key, dest)
}

func TestLateUpdateCannotRevertTerminal(t *testing.T) {
	ctx := context.Background()
	kv := &racingKV{fakeKV: newFakeKV()}
	s := newTestStore(kv)
	kv.hook = func() {
		s.MarkTerminal(ctx, "run1", "doc1", "ds1", StatusCompleted)
	}

	// Update loads a RUNNING snapshot, then the injected terminal write lands,
	// then this save writes the stale snapshot back.
	s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 20, 100)

	snap, ok := s.Load(ctx, "run1")
	if !ok || snap.Status != StatusCompleted {
		t.Fatalf("status = %q, want terminal COMPLETED (late update reverted it)", snap.Status)
	}
}
