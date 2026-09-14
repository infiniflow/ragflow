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

//go:build integration

// Package canvas holds the RunTracker CAS regression guard for the
// Valkey→Kvrocks migration. RunTracker relies on four inline Lua scripts that
// use HGET/HSET/DEL/EXISTS/SET(PX)/PEXPIRE to implement a distributed
// active-run lease. These scripts must behave identically on Kvrocks 2.16.0,
// otherwise the agent runtime could lose its single-owner guarantee or fail to
// cancel/stale-clean runs. This file drives the real RunTracker against a live
// apache/kvrocks:2.16.0 and asserts the lease semantics.
//
// Run (KV_ADDR/KV_PASSWORD mirror the redis parity test):
//
//	KV_ADDR=localhost:6379 KV_PASSWORD=<pw> bash build.sh --test-integration ./internal/agent/canvas/...
package canvas

import (
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newKVRunTracker(t *testing.T) (*RunTracker, *redis.Client) {
	t.Helper()
	addr := os.Getenv("KV_ADDR")
	if addr == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("KV_ADDR not set: RunTracker Kvrocks parity tests require a live apache/kvrocks:2.16.0 in CI")
		}
		t.Skip("KV_ADDR not set; skipping RunTracker Kvrocks parity test")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("KV_PASSWORD"),
	})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Fatalf("kvrocks ping %s: %v", addr, err)
	}
	return NewRunTrackerWithClient(client, 30*time.Second), client
}

// TestRunTrackerKvrocksCAS walks every RunTracker Lua script against Kvrocks
// and asserts the single-owner lease contract holds.
func TestRunTrackerKvrocksCAS(t *testing.T) {
	rt, client := newKVRunTracker(t)
	ctx := t.Context()
	const (
		userID    = "u1"
		canvasID  = "c1"
		runA      = "run-a"
		runB      = "run-b"
		tokenA    = "tok-a"
		tokenB    = "tok-b"
		wrongTok  = "tok-z"
		sessionID = "kvp-session-1"
	)

	cleanup := func() {
		client.Del(ctx, activeSessionKey(sessionID), cancelKey(sessionID))
	}
	t.Cleanup(cleanup)

	t.Run("RegisterExclusivity", func(t *testing.T) {
		cleanup()
		ok, err := rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenA, UserID: userID, CanvasID: canvasID, RunID: runA,
		})
		if err != nil {
			t.Fatalf("register A: %v", err)
		}
		if !ok {
			t.Fatal("register A returned false, want true (first owner)")
		}
		got, err := rt.GetActiveSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("get A: %v", err)
		}
		if got == nil || got.RunID != runA || got.Token != tokenA {
			t.Fatalf("get A = %+v, want RunID=run-a Token=tok-a", got)
		}
		// A second, different owner must be rejected while A holds the lease.
		ok, err = rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenB, UserID: userID, CanvasID: canvasID, RunID: runB,
		})
		if err != nil {
			t.Fatalf("register B: %v", err)
		}
		if ok {
			t.Fatal("register B returned true, want false (lease already owned by A)")
		}
		got, _ = rt.GetActiveSession(ctx, sessionID)
		if got.RunID != runA {
			t.Fatalf("after rejected B, active RunID = %s, want run-a", got.RunID)
		}
	})

	t.Run("ReleaseWrongTokenKeepsLease", func(t *testing.T) {
		cleanup()
		if ok, _ := rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenA, UserID: userID, CanvasID: canvasID, RunID: runA,
		}); !ok {
			t.Fatal("register A failed")
		}
		ok, err := rt.ReleaseActiveSession(ctx, sessionID, wrongTok)
		if err != nil {
			t.Fatalf("release wrong: %v", err)
		}
		if ok {
			t.Fatal("release with wrong token succeeded, want false")
		}
		if got, _ := rt.GetActiveSession(ctx, sessionID); got == nil {
			t.Fatal("lease lost after wrong-token release, want retained")
		}
	})

	t.Run("ReleaseThenReRegister", func(t *testing.T) {
		cleanup()
		if ok, _ := rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenA, UserID: userID, CanvasID: canvasID, RunID: runA,
		}); !ok {
			t.Fatal("register A failed")
		}
		ok, err := rt.ReleaseActiveSession(ctx, sessionID, tokenA)
		if err != nil {
			t.Fatalf("release A: %v", err)
		}
		if !ok {
			t.Fatal("release A returned false, want true")
		}
		// After release, a new owner may take over.
		ok, err = rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenB, UserID: userID, CanvasID: canvasID, RunID: runB,
		})
		if err != nil {
			t.Fatalf("register B after release: %v", err)
		}
		if !ok {
			t.Fatal("register B after release returned false, want true")
		}
		got, _ := rt.GetActiveSession(ctx, sessionID)
		if got.RunID != runB || got.Token != tokenB {
			t.Fatalf("after re-register, active = %+v, want RunID=run-b Token=tok-b", got)
		}
	})

	t.Run("RefreshLease", func(t *testing.T) {
		cleanup()
		if ok, _ := rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenA, UserID: userID, CanvasID: canvasID, RunID: runA,
		}); !ok {
			t.Fatal("register A failed")
		}
		ok, err := rt.RefreshActiveSession(ctx, sessionID, tokenA)
		if err != nil {
			t.Fatalf("refresh A: %v", err)
		}
		if !ok {
			t.Fatal("refresh with correct token returned false, want true")
		}
		ok, err = rt.RefreshActiveSession(ctx, sessionID, wrongTok)
		if err != nil {
			t.Fatalf("refresh wrong: %v", err)
		}
		if ok {
			t.Fatal("refresh with wrong token returned true, want false")
		}
	})

	t.Run("CancelMarker", func(t *testing.T) {
		cleanup()
		if ok, _ := rt.RegisterActiveSession(ctx, ActiveSession{
			SessionID: sessionID, Token: tokenA, UserID: userID, CanvasID: canvasID, RunID: runA,
		}); !ok {
			t.Fatal("register A failed")
		}
		// Wrong token must not publish a cancel marker.
		ok, err := rt.RequestCancelActiveSession(ctx, sessionID, wrongTok)
		if err != nil {
			t.Fatalf("cancel wrong: %v", err)
		}
		if ok {
			t.Fatal("cancel with wrong token returned true, want false")
		}
		if n, _ := client.Exists(ctx, cancelKey(sessionID)).Result(); n != 0 {
			t.Fatal("cancel marker set by wrong token, want absent")
		}
		// Correct token publishes a PX-scoped cancel marker.
		ok, err = rt.RequestCancelActiveSession(ctx, sessionID, tokenA)
		if err != nil {
			t.Fatalf("cancel A: %v", err)
		}
		if !ok {
			t.Fatal("cancel with correct token returned false, want true")
		}
		if n, _ := client.Exists(ctx, cancelKey(sessionID)).Result(); n != 1 {
			t.Fatal("cancel marker not present after correct cancel")
		}
		ttl, err := client.TTL(ctx, cancelKey(sessionID)).Result()
		if err != nil {
			t.Fatalf("cancel marker TTL: %v", err)
		}
		// RequestCancelTTL is 24h; the marker must carry a positive expiry.
		if ttl <= 0 {
			t.Fatalf("cancel marker TTL = %v, want > 0", ttl)
		}
	})
}
