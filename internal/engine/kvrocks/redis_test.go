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

func TestClientSetNXDistinguishesContention(t *testing.T) {
	r, _ := newStrictTestClient(t)
	ctx := t.Context()

	acquired, err := r.SetNX(ctx, "lock:document", "owner-1", time.Minute)
	if err != nil {
		t.Fatalf("first SetNX returned error: %v", err)
	}
	if !acquired {
		t.Fatal("first SetNX acquired = false, want true")
	}

	acquired, err = r.SetNX(ctx, "lock:document", "owner-2", time.Minute)
	if err != nil {
		t.Fatalf("contended SetNX returned error: %v", err)
	}
	if acquired {
		t.Fatal("contended SetNX acquired = true, want false")
	}
}

func TestClientSetNXReturnsTransportError(t *testing.T) {
	r, mr := newStrictTestClient(t)
	mr.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	acquired, err := r.SetNX(ctx, "lock:document", "owner-1", time.Minute)
	if err == nil {
		t.Fatal("SetNX error = nil, want transport error")
	}
	if acquired {
		t.Fatal("SetNX acquired = true on transport failure, want false")
	}
}

func TestClientSetNXReturnsErrorForNilClient(t *testing.T) {
	var r *Client

	acquired, err := r.SetNX(t.Context(), "lock:document", "owner-1", time.Minute)
	if err == nil {
		t.Fatal("SetNX error = nil, want initialization error")
	}
	if acquired {
		t.Fatal("SetNX acquired = true for nil client, want false")
	}
}

func TestDistributedLockSpinAcquireReturnsTransportError(t *testing.T) {
	r, mr := newStrictTestClient(t)
	mr.Close()
	lock := &DistributedLock{
		client:    r,
		lockKey:   "lock:document",
		lockValue: "owner-1",
		timeout:   time.Minute,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	if err := lock.SpinAcquire(ctx); err == nil {
		t.Fatal("SpinAcquire error = nil, want transport error")
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
