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

package redis

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
		luaExpireIfEqual: redis.NewScript(luaExpireIfEqualScript),
		luaTokenBucket:   redis.NewScript(luaTokenBucketScript),
		// luaAutoIncrement intentionally not loaded; not used here.
	}, mr
}

func TestLeaseOwnershipLifecycle(t *testing.T) {
	r, mr := newStrictTestClient(t)
	ctx := t.Context()
	const key = "lease:task-1"

	acquired, err := r.AcquireLease(ctx, key, "worker-a", 5*time.Second)
	if err != nil || !acquired {
		t.Fatalf("AcquireLease(worker-a) = %v, %v; want true, nil", acquired, err)
	}
	acquired, err = r.AcquireLease(ctx, key, "worker-b", 5*time.Second)
	if err != nil || acquired {
		t.Fatalf("AcquireLease(worker-b) = %v, %v; want false, nil", acquired, err)
	}

	mr.FastForward(3 * time.Second)
	renewed, err := r.RenewLease(ctx, key, "worker-b", 5*time.Second)
	if err != nil || renewed {
		t.Fatalf("RenewLease(non-owner) = %v, %v; want false, nil", renewed, err)
	}
	renewed, err = r.RenewLease(ctx, key, "worker-a", 5*time.Second)
	if err != nil || !renewed {
		t.Fatalf("RenewLease(owner) = %v, %v; want true, nil", renewed, err)
	}
	mr.FastForward(3 * time.Second)
	if value, err := r.Get(ctx, key); err != nil || value != "worker-a" {
		t.Fatalf("lease after renewal = %q, %v; want worker-a, nil", value, err)
	}

	released, err := r.ReleaseLease(ctx, key, "worker-b")
	if err != nil || released {
		t.Fatalf("ReleaseLease(non-owner) = %v, %v; want false, nil", released, err)
	}
	released, err = r.ReleaseLease(ctx, key, "worker-a")
	if err != nil || !released {
		t.Fatalf("ReleaseLease(owner) = %v, %v; want true, nil", released, err)
	}
}

func TestRenewLeaseCannotReviveExpiredOwner(t *testing.T) {
	r, mr := newStrictTestClient(t)
	ctx := t.Context()
	const key = "lease:task-2"

	acquired, err := r.AcquireLease(ctx, key, "worker-a", time.Second)
	if err != nil || !acquired {
		t.Fatalf("AcquireLease(worker-a) = %v, %v; want true, nil", acquired, err)
	}
	mr.FastForward(2 * time.Second)
	acquired, err = r.AcquireLease(ctx, key, "worker-b", 5*time.Second)
	if err != nil || !acquired {
		t.Fatalf("AcquireLease(worker-b) = %v, %v; want true, nil", acquired, err)
	}

	renewed, err := r.RenewLease(ctx, key, "worker-a", 5*time.Second)
	if err != nil || renewed {
		t.Fatalf("RenewLease(expired owner) = %v, %v; want false, nil", renewed, err)
	}
	if value, err := r.Get(ctx, key); err != nil || value != "worker-b" {
		t.Fatalf("lease owner = %q, %v; want worker-b, nil", value, err)
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
