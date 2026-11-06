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

package kvrocks

import (
	"context"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// TestKvrocksDeleteMany exercises the real go-redis DEL path behind
// DeleteMany: a single call removes every named key, a key that no longer
// exists is not an error, and an empty key list is a safe no-op (go-redis
// rejects a zero-argument DEL).
func TestKvrocksDeleteMany(t *testing.T) {
	resetKvrocksState()
	addr := os.Getenv("KVROCKS_TEST_ADDR")
	if addr == "" {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("miniredis: %v", err)
		}
		t.Cleanup(mr.Close)
		addr = mr.Addr()
	}
	loadConfigForTest(t, addr)
	if err := Init(context.Background()); err != nil {
		t.Fatalf("kvrocks.Init: %v", err)
	}
	defer Close()

	ctx := context.Background()
	c := Get()
	if c == nil {
		t.Fatal("kvrocks client should be initialized")
	}

	// Batch remove: set three keys, drop them all in one call.
	for _, k := range []string{"dm:a", "dm:b", "dm:c"} {
		if !c.Set(ctx, k, "v", 0) {
			t.Fatalf("seed %s failed", k)
		}
	}
	if !c.DeleteMany(ctx, "dm:a", "dm:b", "dm:c") {
		t.Fatal("DeleteMany returned false for a successful batch")
	}
	for _, k := range []string{"dm:a", "dm:b", "dm:c"} {
		if v, _ := c.Get(ctx, k); v != "" {
			t.Errorf("key %s not removed by batch delete", k)
		}
	}

	// A vanished key in the batch must not fail the call.
	if !c.DeleteMany(ctx, "dm:a", "dm:gone") {
		t.Error("DeleteMany should tolerate a missing key")
	}

	// Empty key list is a no-op and must not error (zero-arg DEL is rejected).
	if !c.DeleteMany(ctx) {
		t.Error("DeleteMany with no keys should be a successful no-op")
	}
}

// TestKvrocksDeleteManyChunks verifies DeleteMany splits a key list larger than
// deleteBatchSize into multiple DEL calls and still removes every key. Seeding
// five keys with a batch size of two forces three batches; a wrong chunk boundary
// (e.g. a dropped trailing partial batch) would leave keys alive.
func TestKvrocksDeleteManyChunks(t *testing.T) {
	resetKvrocksState()
	addr := os.Getenv("KVROCKS_TEST_ADDR")
	if addr == "" {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("miniredis: %v", err)
		}
		t.Cleanup(mr.Close)
		addr = mr.Addr()
	}
	loadConfigForTest(t, addr)
	if err := Init(context.Background()); err != nil {
		t.Fatalf("kvrocks.Init: %v", err)
	}
	defer Close()

	// Shrink the batch so a handful of keys spans several DEL calls.
	prev := deleteBatchSize
	deleteBatchSize = 2
	defer func() { deleteBatchSize = prev }()

	ctx := context.Background()
	c := Get()
	if c == nil {
		t.Fatal("kvrocks client should be initialized")
	}

	keys := []string{"ck:a", "ck:b", "ck:c", "ck:d", "ck:e"}
	for _, k := range keys {
		if !c.Set(ctx, k, "v", 0) {
			t.Fatalf("seed %s failed", k)
		}
	}
	if !c.DeleteMany(ctx, keys...) {
		t.Fatal("DeleteMany returned false across multiple batches")
	}
	for _, k := range keys {
		if v, _ := c.Get(ctx, k); v != "" {
			t.Errorf("key %s not removed by chunked delete", k)
		}
	}
}
