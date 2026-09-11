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

// Package redis holds the regression guard for the Valkey→Kvrocks migration.
// These tests drive the real application Client (internal/engine/redis) against
// a live apache/kvrocks:2.16.0 instance and assert that every command family
// the codebase relies on behaves compatibly with Redis/Valkey. They are the
// "no regression" guard: if a future Kvrocks bump or config change breaks any
// contract the app depends on, these tests turn red.
//
// Run (mirrors the deployment: the entrypoint renders the config, including the
// password from REDIS_PASSWORD, at startup):
//
//	docker run -d -p 6379:6379 \
//	  -v $(pwd)/docker/kvrocks-entrypoint.sh:/etc/kvrocks/entrypoint.sh:ro \
//	  -e REDIS_PASSWORD=<pw> --user 0:0 --entrypoint /bin/sh \
//	  apache/kvrocks:2.16.0 -c "/etc/kvrocks/entrypoint.sh"
//	KV_ADDR=localhost:6379 KV_PASSWORD=<pw> bash build.sh --test-integration ./internal/engine/redis/...
//
// Without KV_ADDR the test skips locally for convenience, but fails hard (CI=true)
// so a misconfigured CI job never turns green without a live Kvrocks.
package redis

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// newKVClient wires a Client against a live Kvrocks.
//
// Guard policy: the parity tests are a regression fence, so a missing backend
// must be loud, not silent. KV_ADDR unset in CI (CI=true) is a misconfigured
// job and fails hard; unset outside CI simply skips (local convenience). A set
// but unreachable KV_ADDR always fails (the Ping below), so a CI run that forgot
// to start Kvrocks can never pass green.
func newKVClient(t *testing.T) *Client {
	t.Helper()
	addr := os.Getenv("KV_ADDR")
	if addr == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("KV_ADDR not set: Kvrocks parity tests require a live apache/kvrocks:2.16.0 in CI")
		}
		t.Skip("KV_ADDR not set; skipping Kvrocks parity test (needs apache/kvrocks:2.16.0)")
	}
	c := &Client{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: os.Getenv("KV_PASSWORD"),
		}),
		luaDeleteIfEqual: redis.NewScript(luaDeleteIfEqualScript),
		luaTokenBucket:   redis.NewScript(luaTokenBucketScript),
	}
	t.Cleanup(func() { _ = c.client.Close() })
	if err := c.client.Ping(t.Context()).Err(); err != nil {
		t.Fatalf("kvrocks ping %s: %v", addr, err)
	}
	return c
}

// TestKvrocksParity exercises the full command surface the application uses.
// It is one test with subtests so a single backend connection is reused.
func TestKvrocksParity(t *testing.T) {
	c := newKVClient(t)
	ctx := t.Context()
	// One-shot unique prefix: Kvrocks data persists across test processes, so
	// a fresh prefix each run prevents cross-run key collisions.
	prefix := "kvp:" + uuid.NewString() + ":"

	t.Run("String", func(t *testing.T) {
		k := prefix + "str"
		if !c.Set(ctx, k, "v1", time.Minute) {
			t.Fatal("Set failed")
		}
		if v, _ := c.Get(ctx, k); v != "v1" {
			t.Fatalf("Get = %q, want v1", v)
		}
		if ok, _ := c.Exist(ctx, k); !ok {
			t.Fatal("Exist = false, want true")
		}
		// SetNX must not overwrite an existing key.
		if c.SetNX(ctx, k, "v2", time.Minute) {
			t.Fatal("SetNX overwrote existing key")
		}
		if v, _ := c.Get(ctx, k); v != "v1" {
			t.Fatalf("SetNX side effect: Get = %q, want v1", v)
		}
		if n, _ := c.IncrBy(ctx, k+"-c", 5); n != 5 {
			t.Fatalf("IncrBy = %d, want 5", n)
		}
		if n, _ := c.IncrBy(ctx, k+"-c", 3); n != 8 {
			t.Fatalf("IncrBy = %d, want 8", n)
		}
		if n, _ := c.DecrBy(ctx, k+"-c", 2); n != 6 {
			t.Fatalf("DecrBy = %d, want 6", n)
		}
		if !c.Expire(ctx, k, 10*time.Second) {
			t.Fatal("Expire failed")
		}
		if tt := c.TTL(ctx, k); tt <= 0 || tt > 10*time.Second {
			t.Fatalf("TTL = %v, want (0,10s]", tt)
		}
		if !c.Delete(ctx, k) {
			t.Fatal("Delete failed")
		}
		if ok, _ := c.Exist(ctx, k); ok {
			t.Fatal("Exist = true after Delete, want false")
		}
	})

	t.Run("Hash", func(t *testing.T) {
		k := prefix + "hash"
		c.client.Del(ctx, k)
		if err := c.client.HSet(ctx, k, map[string]any{
			"a": "1", "b": "2",
		}).Err(); err != nil {
			t.Fatalf("HSet: %v", err)
		}
		got, err := c.client.HGetAll(ctx, k).Result()
		if err != nil {
			t.Fatalf("HGetAll: %v", err)
		}
		if got["a"] != "1" || got["b"] != "2" {
			t.Fatalf("HGetAll = %v, want a=1 b=2", got)
		}
		// TokenBucket Lua exercises HMGET/HMSET/EXPIRE inside Kvrocks.
		tb := &TokenBucket{client: c, key: k + "-tb", capacity: 2, rate: 0.1}
		if ok, _ := tb.Allow(ctx, 1); !ok {
			t.Fatal("TokenBucket first call denied, want allowed")
		}
		if ok, _ := tb.Allow(ctx, 1); !ok {
			t.Fatal("TokenBucket second call denied, want allowed")
		}
		if ok, _ := tb.Allow(ctx, 1); ok {
			t.Fatal("TokenBucket third call allowed, want denied (bucket exhausted)")
		}
	})

	t.Run("Set", func(t *testing.T) {
		k := prefix + "set"
		c.client.Del(ctx, k)
		if !c.SAdd(ctx, k, "m1") {
			t.Fatal("SAdd m1 failed")
		}
		if !c.SAdd(ctx, k, "m2") {
			t.Fatal("SAdd m2 failed")
		}
		if !c.SIsMember(ctx, k, "m1") {
			t.Fatal("SIsMember m1 = false, want true")
		}
		if c.SIsMember(ctx, k, "mX") {
			t.Fatal("SIsMember mX = true, want false")
		}
		members, err := c.SMembers(ctx, k)
		if err != nil {
			t.Fatalf("SMembers: %v", err)
		}
		if len(members) != 2 {
			t.Fatalf("SMembers len = %d, want 2", len(members))
		}
		if !c.SRem(ctx, k, "m1") {
			t.Fatal("SRem m1 failed")
		}
		if c.SIsMember(ctx, k, "m1") {
			t.Fatal("SIsMember m1 = true after SRem, want false")
		}
	})

	t.Run("ZSet", func(t *testing.T) {
		k := prefix + "zset"
		c.client.Del(ctx, k)
		if !c.ZAdd(ctx, k, "a", 1) {
			t.Fatal("ZAdd a failed")
		}
		if !c.ZAdd(ctx, k, "b", 5) {
			t.Fatal("ZAdd b failed")
		}
		if !c.ZAdd(ctx, k, "c", 10) {
			t.Fatal("ZAdd c failed")
		}
		if n := c.ZCount(ctx, k, 0, 6); n != 2 {
			t.Fatalf("ZCount(0,6) = %d, want 2", n)
		}
		got, err := c.ZRangeByScore(ctx, k, 0, 6)
		if err != nil {
			t.Fatalf("ZRangeByScore: %v", err)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("ZRangeByScore = %v, want [a b]", got)
		}
		popped, err := c.ZPopMin(ctx, k, 1)
		if err != nil {
			t.Fatalf("ZPopMin: %v", err)
		}
		if len(popped) != 1 || popped[0].Member.(string) != "a" {
			t.Fatalf("ZPopMin = %v, want [a]", popped)
		}
		// After ZPopMin removed "a"(score 1), remaining b(5) and c(10).
		// Removing the [0,6] range must drop only "b" -> 1 removed.
		if n := c.ZRemRangeByScore(ctx, k, 0, 6); n != 1 {
			t.Fatalf("ZRemRangeByScore(0,6) = %d, want 1", n)
		}
	})

	t.Run("Stream", func(t *testing.T) {
		q := prefix + "stream"
		c.client.Del(ctx, q)
		if !c.QueueProduct(ctx, q, map[string]string{"hello": "world"}) {
			t.Fatal("QueueProduct failed")
		}
		// Consumer group creation is implicit in QueueConsumer.
		msg, err := c.QueueConsumer(ctx, q, "grp", "cons", "")
		if err != nil {
			t.Fatalf("QueueConsumer: %v", err)
		}
		if msg == nil {
			t.Fatal("QueueConsumer returned nil message")
		}
		// QueueProduct stored {"hello":"world"} under the stream "message"
		// field; GetMessage() returns the JSON-decoded payload map.
		if got := msg.GetMessage()["hello"]; got != "world" {
			t.Fatalf("consumed payload hello = %v, want world", got)
		}
		if !msg.Ack(ctx) {
			t.Fatal("Ack failed")
		}
		pending, err := c.GetPendingMsg(ctx, q, "grp")
		if err != nil {
			t.Fatalf("GetPendingMsg: %v", err)
		}
		if len(pending) != 0 {
			t.Fatalf("GetPendingMsg len = %d, want 0 (acked)", len(pending))
		}
		// XRange round-trips the stored entry (RequeueMsg path).
		ranged, err := c.client.XRange(ctx, q, "-", "+").Result()
		if err != nil {
			t.Fatalf("XRange: %v", err)
		}
		if len(ranged) == 0 {
			t.Fatal("XRange returned no entries")
		}
	})

	t.Run("LuaDeleteIfEqual", func(t *testing.T) {
		k := prefix + "die"
		c.client.Del(ctx, k)
		if !c.Set(ctx, k, "expect", time.Minute) {
			t.Fatal("Set failed")
		}
		// Wrong expected value must not delete.
		if c.DeleteIfEqual(ctx, k, "wrong") {
			t.Fatal("DeleteIfEqual deleted on wrong value")
		}
		if ok, _ := c.Exist(ctx, k); !ok {
			t.Fatal("key gone after wrong DeleteIfEqual")
		}
		// Correct expected value deletes.
		if !c.DeleteIfEqual(ctx, k, "expect") {
			t.Fatal("DeleteIfEqual failed on correct value")
		}
		if ok, _ := c.Exist(ctx, k); ok {
			t.Fatal("key still present after correct DeleteIfEqual")
		}
	})

	t.Run("TokenBucketStrict", func(t *testing.T) {
		// EvalTokenBucketStrict is the fail-closed counterpart used by the webhook
		// rate limiter (agent_webhook_security.go). It shares the TokenBucket Lua
		// but must surface its result/errors to the caller rather than silently
		// passing traffic. Verify the allow/deny sequence on Kvrocks.
		k := prefix + "tbs"
		c.client.Del(ctx, k)
		ok, err := c.EvalTokenBucketStrict(ctx, k, 2, 0.1)
		if err != nil {
			t.Fatalf("call 1: %v", err)
		}
		if !ok {
			t.Fatal("call 1: expected allowed")
		}
		if ok, err = c.EvalTokenBucketStrict(ctx, k, 2, 0.1); err != nil || !ok {
			t.Fatalf("call 2: ok=%v err=%v, want allowed", ok, err)
		}
		ok, err = c.EvalTokenBucketStrict(ctx, k, 2, 0.1)
		if err != nil {
			t.Fatalf("call 3: %v", err)
		}
		if ok {
			t.Fatal("call 3: expected denied (bucket exhausted), got allowed")
		}
	})

	t.Run("PipelineSetNX", func(t *testing.T) {
		k := prefix + "pipe"
		c.client.Del(ctx, k)
		if !c.Transaction(ctx, k, "v", time.Minute) {
			t.Fatal("Transaction (pipeline SetNX) failed")
		}
		// Second call hits an existing key: SetNX inside the pipeline does not
		// overwrite, so the value must remain "v" (Transaction returns on
		// Exec success, not SetNX success — assert the value, not the bool).
		c.Transaction(ctx, k, "v2", time.Minute)
		if v, _ := c.Get(ctx, k); v != "v" {
			t.Fatalf("pipeline SetNX overwrote existing key: Get = %q, want v", v)
		}
	})

	t.Run("TTLContract", func(t *testing.T) {
		// Kvrocks >= 2.11 reads metadata.Expired() on the read path, so a key
		// whose TTL elapsed must report EXISTS=0 immediately (no 2-3s
		// compaction lag). This is the exact bug that 2.0.6 had and that
		// motivated pinning apache/kvrocks:2.16.0. Guard it.
		k := prefix + "ttl"
		c.client.Del(ctx, k)
		if !c.Set(ctx, k, "x", 1200*time.Millisecond) {
			t.Fatal("Set with TTL failed")
		}
		if ok, _ := c.Exist(ctx, k); !ok {
			t.Fatal("key should exist before TTL elapses")
		}
		time.Sleep(1500 * time.Millisecond)
		if ok, _ := c.Exist(ctx, k); ok {
			t.Fatal("EXISTS=1 long after TTL elapsed: Kvrocks read-path expiry broken (regression of 2.0.6 bug)")
		}
		if tt := c.TTL(ctx, k); tt != -2 {
			t.Fatalf("TTL = %v, want -2 (key gone)", tt)
		}
	})

	t.Run("Auth", func(t *testing.T) {
		// The live client authenticated at connect time. A second client with
		// a wrong password must fail to PING (NOAUTH), proving requirepass is
		// enforced — important because Kvrocks rejects multi-user ACLs.
		pw := os.Getenv("KV_PASSWORD")
		if pw == "" {
			t.Skip("KV_PASSWORD empty; auth-enforcement check skipped")
		}
		bad := redis.NewClient(&redis.Options{Addr: os.Getenv("KV_ADDR"), Password: "definitely-wrong"})
		defer func() { _ = bad.Close() }()
		if err := bad.Ping(ctx).Err(); err == nil {
			t.Fatal("PING with wrong password succeeded; requirepass not enforced")
		}
	})

	t.Run("Info", func(t *testing.T) {
		// Info must not panic on Kvrocks' differently-shaped INFO output.
		info := c.Info(ctx)
		if info == nil {
			t.Fatal("Info returned nil")
		}
		// server_mode is empty on Kvrocks (field name differs) — that is
		// acceptable and only affects monitoring, not functionality.
	})
}
