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

package kvrocks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newStrictTestClient wires a miniredis-backed Client for
// EvalTokenBucketStrict tests. Each call gets its own miniredis instance so
// tests do not share state via the package-level globalClient.
func newStrictTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return &Client{
		client:           rdb,
		luaDeleteIfEqual: redis.NewScript(luaDeleteIfEqualScript),
		luaTokenBucket:   redis.NewScript(luaTokenBucketScript),
		// luaAutoIncrement intentionally not loaded; not used here.
	}, mr
}

func TestTotalSystemMemoryHuman(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		info map[string]string
		want string
	}{
		{
			name: "prefers total_system_memory_human",
			info: map[string]string{
				"total_system_memory_human": "16.00G",
				"maxmemory_human":           "4.00G",
			},
			want: "16.00G",
		},
		{
			name: "falls back to maxmemory_human",
			info: map[string]string{
				"maxmemory_human": "4.00G",
			},
			want: "4.00G",
		},
		{
			name: "empty when neither field present",
			info: map[string]string{
				"used_memory_human": "128.00M",
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := totalSystemMemoryHuman(tc.info); got != tc.want {
				t.Fatalf("totalSystemMemoryHuman() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEvalTokenBucketStrict_AllowedThenDenied walks the bucket through
// capacity=2, rate=0.1 (slow refill). Two calls should be allowed; the
// third should be denied. This is the happy-path security gate.
func TestEvalTokenBucketStrict_AllowedThenDenied(t *testing.T) {
	r, _ := newStrictTestClient(t)
	ctx := t.Context()

	for i := 1; i <= 2; i++ {
		ok, err := r.EvalTokenBucketStrict(ctx, "tb:webhook", 2, 0.1)
		if err != nil {
			t.Fatalf("call %d unexpected error: %v", i, err)
		}
		if !ok {
			t.Fatalf("call %d: expected allowed=true", i)
		}
	}
	ok, err := r.EvalTokenBucketStrict(ctx, "tb:webhook", 2, 0.1)
	if err != nil {
		t.Fatalf("call 3 unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("call 3: expected allowed=false (bucket exhausted)")
	}
}

// TestEvalTokenBucketStrict_RedisDownFailsClosed confirms the strict
// contract: when the transport fails, the caller sees an error AND
// allowed=false. This is the explicit divergence from TokenBucket.Allow,
// which would silently return allowed=true in the same situation.
func TestEvalTokenBucketStrict_RedisDownFailsClosed(t *testing.T) {
	r, mr := newStrictTestClient(t)
	mr.Close() // break the connection

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	ok, err := r.EvalTokenBucketStrict(ctx, "tb:webhook", 5, 1)
	if err == nil {
		t.Fatalf("expected transport error, got nil")
	}
	if ok {
		t.Fatalf("expected allowed=false on transport failure (fail-closed)")
	}
}

// TestEvalTokenBucketStrict_NilClient confirms the nil-receiver guard.
// The uninitialised-Redis case must NOT silently pass; it must return
// (false, error) so the webhook handler can surface 102.
func TestEvalTokenBucketStrict_NilClient(t *testing.T) {
	var r *Client
	ok, err := r.EvalTokenBucketStrict(t.Context(), "tb:webhook", 1, 1)
	if err == nil {
		t.Fatalf("expected error on nil client, got nil")
	}
	if ok {
		t.Fatalf("expected allowed=false on nil client (fail-closed)")
	}
}

// TestSetNXErr_AllowedWhenNew pins the happy path: SetNXErr reports
// (true, nil) on a brand-new key. Regression guard for issue #20660.
func TestSetNXErr_AllowedWhenNew(t *testing.T) {
	r, _ := newStrictTestClient(t)
	ctx := t.Context()

	ok, err := r.SetNXErr(ctx, "lock:test:new", "v", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true on a fresh key")
	}
}

// TestSetNXErr_FalseOnContention confirms the second concurrent caller
// sees (false, nil), not (false, err). Only the transport-failure case
// should return an error - keep the two outcomes distinguishable so the
// DistributedLock wait-for-lock loop can retry on the former and fail
// fast on the latter.
func TestSetNXErr_FalseOnContention(t *testing.T) {
	r, _ := newStrictTestClient(t)
	ctx := t.Context()

	if ok, err := r.SetNXErr(ctx, "lock:test:contended", "first", time.Minute); err != nil || !ok {
		t.Fatalf("seed call: ok=%v err=%v", ok, err)
	}
	ok, err := r.SetNXErr(ctx, "lock:test:contended", "second", time.Minute)
	if err != nil {
		t.Fatalf("contention should NOT surface a transport error, got %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false on a held key")
	}
}

// TestSetNXErr_TransportErrorFailsFast is the core regression for
// #20660. With miniredis closed, the underlying SETNX cannot reach the
// server; SetNXErr must surface that as a non-nil error instead of
// returning false and forcing every distributed-lock caller to spin for
// its full timeout budget. Mirrors the EvalTokenBucketStrict
// fail-closed contract in the same package.
func TestSetNXErr_TransportErrorFailsFast(t *testing.T) {
	r, mr := newStrictTestClient(t)
	mr.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	ok, err := r.SetNXErr(ctx, "lock:test:down", "v", time.Minute)
	if err == nil {
		t.Fatalf("expected transport error, got nil (ok=%v)", ok)
	}
	if ok {
		t.Fatalf("expected ok=false on transport failure (fail-closed)")
	}
}

// TestDistributedLock_AcquireTransportError mirrors the issue body:
// DistributedLock.Acquire must surface the underlying transport error
// rather than collapsing it into (false, nil), otherwise every polling
// caller (the two withDDLLock waitForLock loops and any future spin
// caller) will treat "Redis is down" as "lock held by someone else".
func TestDistributedLock_AcquireTransportError(t *testing.T) {
	r, mr := newStrictTestClient(t)
	mr.Close()
	// Bypass NewDistributedLock's globalClient guard so we can drive
	// Acquire against a known-down client.
	lock := &DistributedLock{
		client:          r,
		lockKey:         "lock:test:down",
		lockValue:       "v",
		timeout:         time.Minute,
		blockingTimeout: 0,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	acquired, err := lock.Acquire(ctx)
	if err == nil {
		t.Fatalf("expected transport error from Acquire, got nil (acquired=%v)", acquired)
	}
	if acquired {
		t.Fatalf("expected acquired=false on transport failure (fail-closed)")
	}
}

// TestDistributedLock_SpinAcquireSurfacesTransportError confirms the
// second regression the issue called out: SpinAcquire used to swallow
// the underlying SetNX error and only return ctx.Err() at the next
// select-tick. With Redis down, SpinAcquire must return the transport
// error directly so the metadata-write path can fail fast (target:
// ~3s on a real outage, the go-redis connection-pool exhaustion time,
// rather than the 10s internal sleep plus caller budget).
//
// The 5s ctx is sized to give go-redis's 5-attempt connection pool
// exhaustion time to surface the connection-refused error before the
// test deadline cancels the call - otherwise the assertion would
// conflate "transport failure" with "ctx timed out first" (because
// go-redis's .Result() returns ctx.Err() when the parent context
// deadline beats the dial to the failed endpoint).
func TestDistributedLock_SpinAcquireSurfacesTransportError(t *testing.T) {
	r, mr := newStrictTestClient(t)
	mr.Close()
	lock := &DistributedLock{
		client:          r,
		lockKey:         "lock:test:down",
		lockValue:       "v",
		timeout:         time.Minute,
		blockingTimeout: 0,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	err := lock.SpinAcquire(ctx)
	if err == nil {
		t.Fatalf("expected transport error from SpinAcquire, got nil")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SpinAcquire returned ctx.Err() (deadline) instead of the transport error: %v", err)
	}
}
