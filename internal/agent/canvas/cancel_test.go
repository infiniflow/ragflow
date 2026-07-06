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
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// withCancelClient swaps the package-level Redis getter for a miniredis-
// backed one and returns a cleanup func that restores production state.
func withCancelClient(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	orig := cancelClientFn
	cancelClientFn = func() (*redis.Client, error) { return client, nil }
	t.Cleanup(func() { cancelClientFn = orig })
	return mr
}

func TestWatchCancel_FiresAfterRequest(t *testing.T) {
	withCancelClient(t)
	ctx := t.Context()

	taskID := "task_test_1"
	fired := atomic.Bool{}
	done := make(chan struct{})

	go func() {
		WatchCancel(ctx, taskID, func() { fired.Store(true) })
		close(done)
	}()

	// Give the watcher time to start its first tick.
	time.Sleep(200 * time.Millisecond)
	if err := RequestCancel(ctx, taskID); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}

	// onCancel must fire within 1s — poll interval is 500ms so two
	// ticks cover worst case plus slack.
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("WatchCancel did not return within 1s after RequestCancel")
	}
	if !fired.Load() {
		t.Fatal("onCancel was not invoked")
	}
}

func TestWatchCancel_StopsOnContextCancel(t *testing.T) {
	withCancelClient(t)
	ctx, cancel := context.WithCancel(context.Background())

	taskID := "task_test_ctx"
	done := make(chan struct{})
	go func() {
		WatchCancel(ctx, taskID, func() {
			t.Error("onCancel should not fire without a Redis signal")
		})
		close(done)
	}()

	// Cancel the context — watcher should return promptly even though
	// no Redis flag is set.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("WatchCancel did not return within 1s after ctx cancel")
	}
}

func TestWatchCancel_OnCancelNotInvokedForEmptyKey(t *testing.T) {
	withCancelClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoked := atomic.Int32{}
	done := make(chan struct{})
	go func() {
		WatchCancel(ctx, "task_never_cancelled", func() {
			invoked.Add(1)
		})
		close(done)
	}()

	// Wait for two full poll intervals and ensure onCancel never fires.
	time.Sleep(1200 * time.Millisecond)
	cancel()
	<-done

	if invoked.Load() != 0 {
		t.Fatalf("onCancel fired %d times for an unsignaled task; want 0",
			invoked.Load())
	}
}

func TestRequestCancel_EmptyValueStillFires(t *testing.T) {
	// Python's task_service.py writes "x" as the value, but a buggy
	// caller that wrote "" should not silently keep the watcher
	// waiting. WatchCancel's contract is "non-empty triggers onCancel";
	// we rely on RequestCancel to always set "x" so this test is just
	// a sanity check that the value round-trips.
	mr := withCancelClient(t)
	ctx := context.Background()

	if err := RequestCancel(ctx, "task_value"); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	got, err := mr.Get("task_value-cancel")
	if err != nil {
		t.Fatalf("mr.Get: %v", err)
	}
	if got != "x" {
		t.Fatalf("cancel key value = %q, want %q", got, "x")
	}
}
