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
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
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

// Get exposes the raw read used by the store. It mirrors kvrocks.Client.Get,
// which reports an absent key as an empty string with a nil error (it swallows
// redis.Nil) and a real failure as an error.
func (f *fakeKV) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, ok := f.data[key]
	if !ok {
		return "", nil
	}
	return string(raw), nil
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

// nilErrKV models a client that surfaces redis.Nil directly instead of the
// empty-string-with-nil-error shape returned by kvrocks.Client.
type nilErrKV struct{ *fakeKV }

func (n *nilErrKV) Get(ctx context.Context, key string) (string, error) {
	raw, err := n.fakeKV.Get(ctx, key)
	if raw == "" && err == nil {
		return "", redis.Nil
	}
	return raw, err
}

func TestLoadTreatsRedisNilAsMissing(t *testing.T) {
	kv := &nilErrKV{fakeKV: newFakeKV()}
	s := newTestStore(kv)
	if !s.Update(context.Background(), "run1", "doc1", "ds1", "Parser", "page:parse", 1, 10) {
		t.Fatal("first update on a redis.Nil key failed")
	}
}

// readFailKV simulates a transient read failure on an existing key. Both read
// entry points are overridden so the test stays valid across the store's
// GetObj-to-Get migration.
type readFailKV struct {
	*fakeKV
	fail atomic.Bool
}

func (f *readFailKV) GetObj(ctx context.Context, key string, dest any) bool {
	if f.fail.Load() {
		return false
	}
	return f.fakeKV.GetObj(ctx, key, dest)
}

func (f *readFailKV) Get(ctx context.Context, key string) (string, error) {
	if f.fail.Load() {
		return "", errors.New("simulated read failure")
	}
	return f.fakeKV.Get(ctx, key)
}

// ctxCaptureKV records whether the store's write path received a live context.
type ctxCaptureKV struct {
	*fakeKV
	gotErr error
}

func (c *ctxCaptureKV) GetObj(ctx context.Context, key string, dest any) bool {
	c.gotErr = ctx.Err()
	return c.fakeKV.GetObj(ctx, key, dest)
}

func (c *ctxCaptureKV) Get(ctx context.Context, key string) (string, error) {
	c.gotErr = ctx.Err()
	return c.fakeKV.Get(ctx, key)
}

func (c *ctxCaptureKV) SetObj(ctx context.Context, key string, obj any, exp time.Duration) bool {
	c.gotErr = ctx.Err()
	return c.fakeKV.SetObj(ctx, key, obj, exp)
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

func TestUpdateCreatesSnapshotOnFirstWrite(t *testing.T) {
	// The KV client reports a missing key as ("", nil). A first progress
	// callback must still create and persist a fresh snapshot.
	kv := newFakeKV()
	s := newTestStore(kv)
	if !s.Update(context.Background(), "run1", "doc1", "ds1", "Parser", "page:parse", 1, 10) {
		t.Fatal("first update on an absent key failed")
	}
	snap := loadSnapshot(t, kv, "run1")
	if got := findStage(snap, "Parser").Units["page:parse"]; got.Current != 1 || got.Total != 10 {
		t.Fatalf("position = %+v, want {1 10}", got)
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
	// Every timed iteration must advance the position and stay within the
	// total, otherwise Update rejects it before reaching the write path.
	total := uint32(b.N + 1)
	for i := 0; i < b.N; i++ {
		s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", uint32(i+1), total)
	}
}

func TestUpdateSkipsWriteWhenSnapshotReadFails(t *testing.T) {
	kv := &readFailKV{fakeKV: newFakeKV()}
	s := newTestStore(kv)
	ctx := context.Background()

	if !s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 250, 500) {
		t.Fatal("seed update failed")
	}
	before := kv.writes
	kv.fail.Store(true)
	// Adversarial: a transient read failure must not be treated as a missing
	// key and overwrite the known 250/500 position with a stale 120/500.
	if s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 120, 500) {
		t.Fatal("update succeeded despite a failed snapshot read")
	}
	if kv.writes != before {
		t.Fatalf("writes = %d, want unchanged %d after failed read", kv.writes, before)
	}
	kv.fail.Store(false)
	snap := loadSnapshot(t, kv.fakeKV, "run1")
	if got := findStage(snap, "Parser").Units["page:parse"]; got.Current != 250 {
		t.Fatalf("position regressed to %d after failed read, want 250", got.Current)
	}
}

func TestMarkTerminalUsesLiveContext(t *testing.T) {
	kv := &ctxCaptureKV{fakeKV: newFakeKV()}
	s := newTestStore(kv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // caller context is already cancelled (cancel/timeout branch)

	if !s.MarkTerminal(ctx, "run1", "doc1", "ds1", StatusCompleted) {
		t.Fatal("MarkTerminal failed")
	}
	if kv.gotErr != nil {
		t.Fatalf("terminal write received cancelled context: %v", kv.gotErr)
	}
	snap := loadSnapshot(t, kv.fakeKV, "run1")
	if snap.Status != StatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", snap.Status)
	}
}

// racingKV injects a terminal write at the store's re-read inside save, so
// the delayed detail writer reaches the commit attempt with a stale snapshot
// instead of being rejected at the entry guard.
type racingKV struct {
	*fakeKV
	hook    func()
	once    sync.Once
	gets    int
	firesOn int
}

func (r *racingKV) bump() {
	r.gets++
	if r.gets == r.firesOn {
		r.once.Do(r.hook)
	}
}

func (r *racingKV) GetObj(ctx context.Context, key string, dest any) bool {
	r.bump()
	return r.fakeKV.GetObj(ctx, key, dest)
}

func (r *racingKV) Get(ctx context.Context, key string) (string, error) {
	r.bump()
	return r.fakeKV.Get(ctx, key)
}

func TestLateUpdateCannotRevertTerminal(t *testing.T) {
	ctx := context.Background()
	kv := &racingKV{fakeKV: newFakeKV(), firesOn: 2}
	s := newTestStore(kv)
	// Seed the RUNNING snapshot through the raw client so the counter only
	// tracks reads made by the store under test.
	newTestStore(kv.fakeKV).Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 10, 100)
	kv.hook = func() {
		s.MarkTerminal(ctx, "run1", "doc1", "ds1", StatusCompleted)
	}

	// The update must be applied and reach save with a stale snapshot; the
	// save re-read then preserves the terminal status written by the hook.
	if !s.Update(ctx, "run1", "doc1", "ds1", "Parser", "page:parse", 20, 100) {
		t.Fatal("update should be applied and reach the commit attempt")
	}
	snap, ok := s.Load(ctx, "run1")
	if !ok || snap.Status != StatusCompleted {
		t.Fatalf("status = %q, want terminal COMPLETED (late update reverted it)", snap.Status)
	}
}
