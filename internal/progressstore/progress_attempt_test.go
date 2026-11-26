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
	"reflect"
	"testing"
	"time"
)

func TestBeginAttemptResetsStalePositions(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := testCtx()

	if !s.UpdateAttempt(ctx, "run1", "doc1", "ds1", testAttemptID, "Parser", "page:parse", 250, 500) {
		t.Fatal("seed update failed")
	}
	started := time.Now().UTC()
	if !s.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", started) {
		t.Fatal("BeginAttempt failed")
	}

	snap := loadSnapshot(t, kv, "run1")
	if snap.AttemptID != "attempt-2" {
		t.Errorf("attempt id = %q, want attempt-2", snap.AttemptID)
	}
	if !snap.AttemptStartedAt.Equal(started) {
		t.Errorf("attempt start = %v, want %v", snap.AttemptStartedAt, started)
	}
	if len(snap.Stages) != 0 {
		t.Fatalf("stages = %+v, want empty after a new attempt", snap.Stages)
	}
	if snap.Status != StatusRunning {
		t.Errorf("status = %q, want RUNNING", snap.Status)
	}
}

func TestBeginAttemptPreservesTerminal(t *testing.T) {
	// Adversarial: a late redelivery must not reopen a finished run, even
	// though it arrives with a brand-new attempt id.
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := testCtx()

	s.UpdateAttempt(ctx, "run1", "doc1", "ds1", testAttemptID, "Parser", "page:parse", 10, 100)
	if !s.MarkTerminal(ctx, "run1", "doc1", "ds1", StatusCompleted) {
		t.Fatal("MarkTerminal failed")
	}
	if !s.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", time.Now()) {
		t.Fatal("BeginAttempt failed")
	}

	snap := loadSnapshot(t, kv, "run1")
	if snap.Status != StatusCompleted {
		t.Fatalf("status = %q, want COMPLETED (terminal must survive a new attempt)", snap.Status)
	}
	if snap.AttemptID != testAttemptID {
		t.Errorf("attempt id = %q, want unchanged %q on a terminal snapshot", snap.AttemptID, testAttemptID)
	}
}

func TestBeginAttemptIsIdempotentForSameAttempt(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()
	first := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

	if !s.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", first) {
		t.Fatal("first BeginAttempt failed")
	}
	if !s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", "Parser", "page:parse", 1, 10) {
		t.Fatal("fenced update failed")
	}
	if !s.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", first.Add(time.Hour)) {
		t.Fatal("second BeginAttempt failed")
	}

	snap := loadSnapshot(t, kv, "run1")
	if got := findStage(snap, "Parser").Units["page:parse"].Current; got != 1 {
		t.Fatalf("position = %d, want 1 (same attempt must not reset progress)", got)
	}
	if !snap.AttemptStartedAt.Equal(first) {
		t.Errorf("attempt start = %v, want the first value %v", snap.AttemptStartedAt, first)
	}
}

func TestFencedWritesRejectStaleAttempt(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()
	started := time.Now()

	if !s.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", started) {
		t.Fatal("BeginAttempt failed")
	}
	// All three callbacks below belong to the crashed attempt-1. They may
	// arrive late but must never touch the new attempt's snapshot.
	if s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-1", "Parser", "page:parse", 400, 500) {
		t.Error("stale UpdateAttempt was accepted")
	}
	if s.SetPhaseAttempt(ctx, "run1", "doc1", "ds1", "attempt-1", "Parser", "done") {
		t.Error("stale SetPhaseAttempt was accepted")
	}
	if s.MarkTerminal(WithAttempt(ctx, "attempt-1"), "run1", "doc1", "ds1", StatusCompleted) {
		t.Error("stale MarkTerminal was accepted")
	}

	snap := loadSnapshot(t, kv, "run1")
	if len(snap.Stages) != 0 {
		t.Fatalf("stages = %+v, want none (all stale writes rejected)", snap.Stages)
	}
	if snap.Status != StatusRunning {
		t.Fatalf("status = %q, want RUNNING", snap.Status)
	}

	// The current attempt's writes still land.
	if !s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", "Parser", "page:parse", 1, 500) {
		t.Fatal("current attempt update failed")
	}
	if !s.MarkTerminal(WithAttempt(ctx, "attempt-2"), "run1", "doc1", "ds1", StatusCompleted) {
		t.Fatal("current attempt terminal failed")
	}
	snap = loadSnapshot(t, kv, "run1")
	if snap.Status != StatusCompleted || findStage(snap, "Parser").Units["page:parse"].Current != 1 {
		t.Fatalf("snapshot = %+v, want COMPLETED with current position 1", snap)
	}
}

func TestUpdateAttemptKeepsMonotonicWithinAttempt(t *testing.T) {
	kv := newFakeKV()
	s := newTestStore(kv)
	ctx := context.Background()
	s.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", time.Now())

	s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", "Parser", "page:parse", 100, 500)
	s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", "Parser", "page:parse", 250, 500)
	// Out-of-order callback within the same attempt must stay rejected.
	if s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", "Parser", "page:parse", 120, 500) {
		t.Fatal("out-of-order update within one attempt was accepted")
	}
	snap := loadSnapshot(t, kv, "run1")
	if got := findStage(snap, "Parser").Units["page:parse"].Current; got != 250 {
		t.Fatalf("position = %d, want 250 (monotonic guard must remain)", got)
	}
}

func TestBeginAttemptRejectsFailedRead(t *testing.T) {
	// Adversarial: a transient KV read failure must not be treated as an
	// absent key — otherwise a new attempt would overwrite a known snapshot.
	kv := &readFailKV{fakeKV: newFakeKV()}
	newTestStore(kv.fakeKV).UpdateAttempt(testCtx(), "run1", "doc1", "ds1", testAttemptID, "Parser", "page:parse", 250, 500)
	s := newTestStore(kv)
	kv.fail.Store(true)

	if s.BeginAttempt(context.Background(), "run1", "doc1", "ds1", "attempt-2", time.Now()) {
		t.Fatal("BeginAttempt succeeded despite a failed read")
	}
	snap := loadSnapshot(t, kv.fakeKV, "run1")
	if got := findStage(snap, "Parser").Units["page:parse"].Current; got != 250 {
		t.Fatalf("position = %d, want 250 preserved", got)
	}
}

func TestStaleUpdateCannotClobberAttemptOpenedDuringSave(t *testing.T) {
	// Adversarial: a late callback from attempt-1 passes the entry guard, then
	// a redelivery opens attempt-2 before commit. The re-read inside save must
	// reject the stale write instead of overwriting the fresh window.
	ctx := context.Background()
	kv := &racingKV{fakeKV: newFakeKV(), firesOn: 2}
	seed := newTestStore(kv.fakeKV)
	seed.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-1", time.Now())
	seed.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-1", "Parser", "page:parse", 10, 100)
	s := newTestStore(kv)
	kv.hook = func() {
		seed.BeginAttempt(ctx, "run1", "doc1", "ds1", "attempt-2", time.Now())
	}

	s.UpdateAttempt(ctx, "run1", "doc1", "ds1", "attempt-1", "Parser", "page:parse", 20, 100)

	snap := loadSnapshot(t, kv.fakeKV, "run1")
	if snap.AttemptID != "attempt-2" {
		t.Fatalf("attempt id = %q, want attempt-2 (stale write clobbered the new attempt)", snap.AttemptID)
	}
	if len(snap.Stages) != 0 {
		t.Fatalf("stages = %+v, want empty after the new attempt opened", snap.Stages)
	}
}

func TestStoreDoesNotExposeLegacyWriteMethods(t *testing.T) {
	// Writes must go through a fenced API. An untagged write against an
	// attempted snapshot is silently rejected, so leaving the old surface
	// reachable is a trap for future callers.
	typ := reflect.TypeOf(&Store{})
	for _, name := range []string{"Update", "SetPhase"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("Store still exposes legacy %s", name)
		}
	}
}

func TestBeginAttemptRejectsEmptyAttemptID(t *testing.T) {
	// An empty token would reset the window on every legacy-style write and
	// defeat fencing entirely.
	kv := newFakeKV()
	s := newTestStore(kv)
	if s.BeginAttempt(context.Background(), "run1", "doc1", "ds1", "", time.Now()) {
		t.Fatal("BeginAttempt accepted an empty attempt id")
	}
	if _, ok := s.Load(context.Background(), "run1"); ok {
		t.Fatal("empty attempt id must not create a snapshot")
	}
}

func TestAttemptContextRoundTrip(t *testing.T) {
	ctx := WithAttempt(context.Background(), "attempt-9")
	if got := AttemptFromContext(ctx); got != "attempt-9" {
		t.Fatalf("attempt = %q, want attempt-9", got)
	}
	if got := AttemptFromContext(context.Background()); got != "" {
		t.Fatalf("bare context attempt = %q, want empty", got)
	}
}
