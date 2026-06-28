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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestStore spins up a miniredis-backed store for table-driven tests.
// Returns the store, the miniredis handle (caller must Close()), and a
// cleanup function. We construct the struct directly so we can inject the
// *redis.Client — NewRedisCheckPointStore reads from the global cache
// which is nil in unit tests.
func newTestStore(t *testing.T, ttl time.Duration) (*RedisCheckPointStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return &RedisCheckPointStore{client: client, ttl: ttl}, mr
}

func TestRedisCheckPointStore_RoundTrip(t *testing.T) {
	store, _ := newTestStore(t, 30*24*time.Hour)
	ctx := context.Background()

	// missing key → (nil, false, nil)
	got, ok, err := store.Get(ctx, "absent")
	if err != nil || ok || got != nil {
		t.Fatalf("Get(absent) = (%v, %v, %v); want (nil, false, nil)", got, ok, err)
	}

	// Set + Get round trip
	payload := []byte("eino-serialized-bytes-\x00\x01\x02")
	if err := store.Set(ctx, "cpn_42", payload); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err = store.Get(ctx, "cpn_42")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if !ok {
		t.Fatalf("Get after Set: ok = false, want true")
	}
	if string(got) != string(payload) {
		t.Fatalf("Get payload = %q, want %q", got, payload)
	}

	// Overwrite (eino re-uses ids; last write wins)
	updated := []byte("replacement-payload")
	if err := store.Set(ctx, "cpn_42", updated); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	got, _, _ = store.Get(ctx, "cpn_42")
	if string(got) != string(updated) {
		t.Fatalf("Get after overwrite = %q, want %q", got, updated)
	}
}

func TestRedisCheckPointStore_TTL(t *testing.T) {
	store, mr := newTestStore(t, 2*time.Second)
	ctx := context.Background()

	if err := store.Set(ctx, "cpn_ttl", []byte("x")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// miniredis exposes TTL on a key.
	if d := mr.TTL(checkpointKeyPrefix + "cpn_ttl"); d != 2*time.Second {
		t.Fatalf("TTL after Set = %v, want 2s", d)
	}
	// Fast-forward miniredis' internal clock past the TTL.
	mr.FastForward(3 * time.Second)
	_, ok, err := store.Get(ctx, "cpn_ttl")
	if err != nil {
		t.Fatalf("Get after expiry: %v", err)
	}
	if ok {
		t.Fatalf("Get after expiry: ok = true, want false (key should be gone)")
	}
}

func TestRedisCheckPointStore_Delete(t *testing.T) {
	store, _ := newTestStore(t, time.Minute)
	ctx := context.Background()

	// Delete on missing key is a no-op (no error).
	if err := store.Delete(ctx, "absent"); err != nil {
		t.Fatalf("Delete absent: %v", err)
	}
	// Set then Delete then Get → missing.
	if err := store.Set(ctx, "cpn_del", []byte("payload")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete(ctx, "cpn_del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "cpn_del"); ok {
		t.Fatalf("Get after Delete: ok = true, want false")
	}
}

func TestRedisCheckPointStore_NilClient(t *testing.T) {
	// Cache uninitialized → NewRedisCheckPointStore returns a store with
	// nil client. Operations must error rather than panic.
	store := &RedisCheckPointStore{client: nil, ttl: time.Minute}
	ctx := context.Background()

	if _, _, err := store.Get(ctx, "x"); err == nil {
		t.Fatal("Get with nil client: err = nil, want error")
	}
	if err := store.Set(ctx, "x", []byte("y")); err == nil {
		t.Fatal("Set with nil client: err = nil, want error")
	}
	if err := store.Delete(ctx, "x"); err == nil {
		t.Fatal("Delete with nil client: err = nil, want error")
	}
}
