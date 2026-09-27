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

package canvas

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestTracker(t *testing.T, ttl time.Duration) (*RunTracker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return &RunTracker{client: client, ttl: ttl}, mr
}

func TestRunTracker_StateTransitions(t *testing.T) {
	tracker, mr := newTestTracker(t, 30*24*time.Hour)
	ctx := t.Context()

	// B1: Start
	if err := tracker.Start(ctx, "run_1", "canvas_42", "tenant_a", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got, err := tracker.Get(ctx, "run_1")
	if err != nil {
		t.Fatalf("Get after Start: %v", err)
	}
	if got["canvas_id"] != "canvas_42" {
		t.Fatalf("canvas_id = %q, want %q", got["canvas_id"], "canvas_42")
	}
	if got["tenant_id"] != "tenant_a" {
		t.Fatalf("tenant_id = %q, want %q", got["tenant_id"], "tenant_a")
	}
	if got["status"] != "0" {
		t.Fatalf("status after Start = %q, want 0 (running)", got["status"])
	}
	if got["cancel_requested"] != "0" {
		t.Fatalf("cancel_requested = %q, want 0", got["cancel_requested"])
	}
	if _, err := strconv.ParseInt(got["started_at"], 10, 64); err != nil {
		t.Fatalf("started_at %q is not an int: %v", got["started_at"], err)
	}
	// TTL was applied via the Start pipeline.
	if d := mr.TTL(runKey("run_1")); d != 30*24*time.Hour {
		t.Fatalf("TTL after Start = %v, want 30d", d)
	}

	// AttachCheckpoint
	if err = tracker.AttachCheckpoint(ctx, "run_1", "cpn_xyz"); err != nil {
		t.Fatalf("AttachCheckpoint: %v", err)
	}
	got, _ = tracker.Get(ctx, "run_1")
	if got["checkpoint_id"] != "cpn_xyz" {
		t.Fatalf("checkpoint_id = %q, want %q", got["checkpoint_id"], "cpn_xyz")
	}

	// B2: MarkSucceeded
	if err = tracker.MarkSucceeded(ctx, "run_1"); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	got, _ = tracker.Get(ctx, "run_1")
	if got["status"] != "1" {
		t.Fatalf("status = %q, want 1 (succeeded)", got["status"])
	}
	if _, err = strconv.ParseInt(got["finished_at"], 10, 64); err != nil {
		t.Fatalf("finished_at %q is not an int: %v", got["finished_at"], err)
	}
	// All previous fields preserved.
	if got["canvas_id"] != "canvas_42" || got["checkpoint_id"] != "cpn_xyz" {
		t.Fatalf("fields dropped: %v", got)
	}
}

func TestRunTracker_FailedAndCancelled(t *testing.T) {
	tracker, _ := newTestTracker(t, time.Hour)
	ctx := t.Context()

	// B3: MarkFailed
	if err := tracker.Start(ctx, "run_fail", "c", "t", "run_parent"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tracker.MarkFailed(ctx, "run_fail", "boom: nil deref"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	got, _ := tracker.Get(ctx, "run_fail")
	if got["status"] != "2" {
		t.Fatalf("status = %q, want 2 (failed)", got["status"])
	}
	if got["failure_reason"] != "boom: nil deref" {
		t.Fatalf("failure_reason = %q, want %q", got["failure_reason"], "boom: nil deref")
	}
	if got["parent_run_id"] != "run_parent" {
		t.Fatalf("parent_run_id = %q, want run_parent", got["parent_run_id"])
	}

	// B4: MarkCancelled
	if err := tracker.Start(ctx, "run_cancel", "c", "t", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tracker.MarkCancelled(ctx, "run_cancel"); err != nil {
		t.Fatalf("MarkCancelled: %v", err)
	}
	got, _ = tracker.Get(ctx, "run_cancel")
	if got["status"] != "3" {
		t.Fatalf("status = %q, want 3 (cancelled)", got["status"])
	}
	if got["cancel_requested"] != "1" {
		t.Fatalf("cancel_requested = %q, want 1", got["cancel_requested"])
	}
}

func TestRunTracker_TTLRefresh(t *testing.T) {
	tracker, mr := newTestTracker(t, 2*time.Second)
	ctx := t.Context()

	if err := tracker.Start(ctx, "run_ttl", "c", "t", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Fast-forward 1.5s — TTL is now ~500ms.
	mr.FastForward(1500 * time.Millisecond)
	if d := mr.TTL(runKey("run_ttl")); d > 1*time.Second {
		t.Fatalf("pre-refresh TTL = %v, want < 1s", d)
	}
	// Re-Start must reset the TTL back to the full 2s.
	if err := tracker.Start(ctx, "run_ttl", "c", "t", ""); err != nil {
		t.Fatalf("Start refresh: %v", err)
	}
	if d := mr.TTL(runKey("run_ttl")); d < 1500*time.Millisecond {
		t.Fatalf("TTL not refreshed: %v (want >= 1.5s)", d)
	}
	// Fast-forward less than the refreshed TTL — the key must still exist.
	mr.FastForward(1 * time.Second)
	got, err := tracker.Get(ctx, "run_ttl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("run key expired before refreshed TTL elapsed")
	}

	// A wait-for-user pause must also keep the run metadata alive for the
	// full tracker TTL so the next request can resume it.
	mr.FastForward(500 * time.Millisecond)
	if err = tracker.MarkWaiting(ctx, "run_ttl"); err != nil {
		t.Fatalf("MarkWaiting: %v", err)
	}
	if d := mr.TTL(runKey("run_ttl")); d < 1500*time.Millisecond {
		t.Fatalf("TTL after MarkWaiting = %v, want >= 1.5s", d)
	}
	got, err = tracker.Get(ctx, "run_ttl")
	if err != nil {
		t.Fatalf("Get after MarkWaiting: %v", err)
	}
	if got["status"] != runStatusWaiting {
		t.Fatalf("status after MarkWaiting = %q, want %q", got["status"], runStatusWaiting)
	}
}

func TestRunTracker_NilClient(t *testing.T) {
	tracker := &RunTracker{client: nil, ttl: time.Minute}
	ctx := t.Context()
	if err := tracker.Start(ctx, "x", "c", "t", ""); err == nil {
		t.Fatal("Start with nil client: err = nil, want error")
	}
	if err := tracker.AttachCheckpoint(ctx, "x", "cp"); err == nil {
		t.Fatal("AttachCheckpoint with nil client: err = nil, want error")
	}
	if err := tracker.MarkSucceeded(ctx, "x"); err == nil {
		t.Fatal("MarkSucceeded with nil client: err = nil, want error")
	}
	if err := tracker.MarkFailed(ctx, "x", "r"); err == nil {
		t.Fatal("MarkFailed with nil client: err = nil, want error")
	}
	if err := tracker.MarkCancelled(ctx, "x"); err == nil {
		t.Fatal("MarkCancelled with nil client: err = nil, want error")
	}
	if err := tracker.MarkWaiting(ctx, "x"); err == nil {
		t.Fatal("MarkWaiting with nil client: err = nil, want error")
	}
	if _, err := tracker.Get(ctx, "x"); err == nil {
		t.Fatal("Get with nil client: err = nil, want error")
	}
}

func TestRunTrackerActiveSessionRegistration(t *testing.T) {
	tracker, mr := newTestTracker(t, time.Hour)
	ctx := t.Context()
	if err := tracker.client.Set(ctx, cancelKey("session-1"), "x", time.Hour).Err(); err != nil {
		t.Fatalf("seed cancel marker: %v", err)
	}
	active := ActiveSession{
		SessionID: "session-1", Token: "owner-a", UserID: "user-1",
		CanvasID: "canvas-1", RunID: "run-1",
	}
	registered, err := tracker.RegisterActiveSession(ctx, active)
	if err != nil || !registered {
		t.Fatalf("RegisterActiveSession = %v, %v; want true, nil", registered, err)
	}
	if mr.Exists(cancelKey("session-1")) {
		t.Fatal("registration preserved a cancellation marker from an older run")
	}
	got, err := tracker.GetActiveSession(ctx, "session-1")
	if err != nil || got == nil {
		t.Fatalf("GetActiveSession = %#v, %v", got, err)
	}
	if got.UserID != "user-1" || got.CanvasID != "canvas-1" || got.RunID != "run-1" || got.Token != "owner-a" {
		t.Fatalf("unexpected active session: %#v", got)
	}
	if ttl := mr.TTL(activeSessionKey("session-1")); ttl <= 0 {
		t.Fatalf("active session TTL = %v, want finite positive TTL", ttl)
	}
	registered, err = tracker.RegisterActiveSession(ctx, ActiveSession{SessionID: "session-1", Token: "owner-b"})
	if err != nil || registered {
		t.Fatalf("second RegisterActiveSession = %v, %v; want false, nil", registered, err)
	}
	requested, err := tracker.RequestCancelActiveSession(ctx, "session-1", "owner-a")
	if err != nil || !requested {
		t.Fatalf("RequestCancelActiveSession = %v, %v; want true, nil", requested, err)
	}
	registered, err = tracker.RegisterActiveSession(ctx, ActiveSession{SessionID: "session-1", Token: "owner-b"})
	if err != nil || registered {
		t.Fatalf("registration racing active cancel = %v, %v; want false, nil", registered, err)
	}
	if !mr.Exists(cancelKey("session-1")) {
		t.Fatal("failed registration erased the active owner's cancel marker")
	}
	registered, err = tracker.RegisterActiveSession(ctx, ActiveSession{SessionID: "session-2", Token: "owner-b"})
	if err != nil || !registered {
		t.Fatalf("different-session RegisterActiveSession = %v, %v; want true, nil", registered, err)
	}
}

func TestRunTrackerActiveSessionTokenProtectsRefreshAndRelease(t *testing.T) {
	tracker, _ := newTestTracker(t, time.Hour)
	ctx := t.Context()
	registered, err := tracker.RegisterActiveSession(ctx, ActiveSession{SessionID: "session-1", Token: "owner-a"})
	if err != nil || !registered {
		t.Fatalf("RegisterActiveSession = %v, %v", registered, err)
	}
	owned, err := tracker.RefreshActiveSession(ctx, "session-1", "owner-b")
	if err != nil || owned {
		t.Fatalf("foreign RefreshActiveSession = %v, %v; want false, nil", owned, err)
	}
	released, err := tracker.ReleaseActiveSession(ctx, "session-1", "owner-b")
	if err != nil || released {
		t.Fatalf("foreign ReleaseActiveSession = %v, %v; want false, nil", released, err)
	}
	if got, _ := tracker.GetActiveSession(ctx, "session-1"); got == nil {
		t.Fatal("foreign release removed active session")
	}
	if err = tracker.client.Set(ctx, cancelKey("session-1"), "x", time.Hour).Err(); err != nil {
		t.Fatalf("seed cancel marker: %v", err)
	}
	released, err = tracker.ReleaseActiveSession(ctx, "session-1", "owner-a")
	if err != nil || !released {
		t.Fatalf("owner ReleaseActiveSession = %v, %v; want true, nil", released, err)
	}
	if got, _ := tracker.GetActiveSession(ctx, "session-1"); got != nil {
		t.Fatalf("active session remains after release: %#v", got)
	}
	if tracker.client.Exists(ctx, cancelKey("session-1")).Val() != 0 {
		t.Fatal("release did not clear session cancel marker")
	}
}

func TestRunTrackerActiveSessionTokenProtectsCancel(t *testing.T) {
	tracker, _ := newTestTracker(t, time.Hour)
	ctx := t.Context()
	registered, err := tracker.RegisterActiveSession(ctx, ActiveSession{SessionID: "session-1", Token: "owner-a"})
	if err != nil || !registered {
		t.Fatalf("RegisterActiveSession = %v, %v", registered, err)
	}
	requested, err := tracker.RequestCancelActiveSession(ctx, "session-1", "owner-b")
	if err != nil || requested {
		t.Fatalf("foreign RequestCancelActiveSession = %v, %v; want false, nil", requested, err)
	}
	if tracker.client.Exists(ctx, cancelKey("session-1")).Val() != 0 {
		t.Fatal("foreign cancel created a marker")
	}
	requested, err = tracker.RequestCancelActiveSession(ctx, "session-1", "owner-a")
	if err != nil || !requested {
		t.Fatalf("owner RequestCancelActiveSession = %v, %v; want true, nil", requested, err)
	}
	if tracker.client.Exists(ctx, cancelKey("session-1")).Val() != 1 {
		t.Fatal("owner cancel did not create a marker")
	}
	released, err := tracker.ReleaseActiveSession(ctx, "session-1", "owner-a")
	if err != nil || !released {
		t.Fatalf("owner ReleaseActiveSession = %v, %v; want true, nil", released, err)
	}
	requested, err = tracker.RequestCancelActiveSession(ctx, "session-1", "owner-a")
	if err != nil || requested {
		t.Fatalf("late RequestCancelActiveSession = %v, %v; want false, nil", requested, err)
	}
	if tracker.client.Exists(ctx, cancelKey("session-1")).Val() != 0 {
		t.Fatal("late cancel recreated a marker after active-session release")
	}
}

func TestRunTrackerWatchActiveSessionCancelsWhenLeaseOwnershipIsLost(t *testing.T) {
	tracker, _ := newTestTracker(t, 90*time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	registered, err := tracker.RegisterActiveSession(ctx, ActiveSession{SessionID: "session-1", Token: "owner-a"})
	if err != nil || !registered {
		t.Fatalf("RegisterActiveSession = %v, %v", registered, err)
	}
	lost := make(chan struct{})
	go tracker.WatchActiveSession(ctx, "session-1", "owner-a", func() { close(lost) })
	if err := tracker.client.HSet(ctx, activeSessionKey("session-1"), "token", "owner-b").Err(); err != nil {
		t.Fatalf("replace lease owner: %v", err)
	}
	select {
	case <-lost:
	case <-time.After(time.Second):
		t.Fatal("lease watcher did not cancel after ownership changed")
	}
}
