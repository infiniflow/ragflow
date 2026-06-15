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
	ctx := context.Background()

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
	if err := tracker.AttachCheckpoint(ctx, "run_1", "cpn_xyz"); err != nil {
		t.Fatalf("AttachCheckpoint: %v", err)
	}
	got, _ = tracker.Get(ctx, "run_1")
	if got["checkpoint_id"] != "cpn_xyz" {
		t.Fatalf("checkpoint_id = %q, want %q", got["checkpoint_id"], "cpn_xyz")
	}

	// B2: MarkSucceeded
	if err := tracker.MarkSucceeded(ctx, "run_1"); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	got, _ = tracker.Get(ctx, "run_1")
	if got["status"] != "1" {
		t.Fatalf("status = %q, want 1 (succeeded)", got["status"])
	}
	if _, err := strconv.ParseInt(got["finished_at"], 10, 64); err != nil {
		t.Fatalf("finished_at %q is not an int: %v", got["finished_at"], err)
	}
	// All previous fields preserved.
	if got["canvas_id"] != "canvas_42" || got["checkpoint_id"] != "cpn_xyz" {
		t.Fatalf("fields dropped: %v", got)
	}
}

func TestRunTracker_FailedAndCancelled(t *testing.T) {
	tracker, _ := newTestTracker(t, time.Hour)
	ctx := context.Background()

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
	ctx := context.Background()

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
}

func TestRunTracker_NilClient(t *testing.T) {
	tracker := &RunTracker{client: nil, ttl: time.Minute}
	ctx := context.Background()
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
	if _, err := tracker.Get(ctx, "x"); err == nil {
		t.Fatal("Get with nil client: err = nil, want error")
	}
}
