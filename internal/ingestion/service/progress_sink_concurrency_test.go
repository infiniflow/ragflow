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

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
)

// trackEmitterEnter records the peak number of emitter calls running at once.
// The emitter body sleeps to widen the overlap window, so a sink that does NOT
// serialize its emitter calls observes maxActive > 1 and fails deterministically.
func trackEmitterEnter(active, maxActive *int32) {
	cur := atomic.AddInt32(active, 1)
	for {
		old := atomic.LoadInt32(maxActive)
		if cur <= old || atomic.CompareAndSwapInt32(maxActive, old, cur) {
			return
		}
	}
}

// TestProgressSinkOnProgressDetailSerializesConcurrentEmitters is the
// adversarial fixture for the concurrency finding: eino runs parallel branches
// concurrently, so OnProgressDetail fires from multiple goroutines. The
// installed log emitter must be invoked one at a time. Without serialization
// the overlapping emitter bodies push maxActive above 1.
func TestProgressSinkOnProgressDetailSerializesConcurrentEmitters(t *testing.T) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()

	var active, maxActive int32
	sink.detailEmitter = func(component, unit string, current, total uint32) {
		trackEmitterEnter(&active, &maxActive)
		time.Sleep(time.Millisecond) // widen the window so overlap is observable
		atomic.AddInt32(&active, -1)
	}

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			sink.OnProgressDetail(context.Background(), "Tokenizer", "chunk", 1, 5)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("max concurrent emitter calls = %d, want 1 (emitter calls must be serialized)", got)
	}
}

// TestProgressSinkEmittersShareSerialization extends the guard across both
// emitter channels: detail and lifecycle reports must share ONE serialization
// domain, otherwise a concurrent pair can still overlap even if each channel
// is individually locked.
func TestProgressSinkEmittersShareSerialization(t *testing.T) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()

	var active, maxActive int32
	emit := func() {
		trackEmitterEnter(&active, &maxActive)
		time.Sleep(time.Millisecond)
		atomic.AddInt32(&active, -1)
	}
	sink.detailEmitter = func(component, unit string, current, total uint32) { emit() }
	sink.lifecycleEmitter = func(component string, phase uint8, message string) { emit() }

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			sink.OnProgressDetail(context.Background(), "Tokenizer", "chunk", 1, 5)
		}()
		go func() {
			defer wg.Done()
			sink.OnComponentProgress(context.Background(), pipeline.ProgressEvent{
				TaskID: "task-1", Component: "Parser", Phase: 0,
			})
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("max concurrent emitter calls across both channels = %d, want 1", got)
	}
}

// TestProgressSinkLockReleasedAfterEmitterPanic verifies that the emit
// serialization lock is released even when an emitter panics. A non-deferred
// unlock leaves the mutex held forever, deadlocking every later report.
func TestProgressSinkLockReleasedAfterEmitterPanic(t *testing.T) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()

	sink.lifecycleEmitter = func(component string, phase uint8, message string) {
		panic("emitter boom")
	}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected emitter panic to propagate to the caller")
			}
		}()
		sink.OnComponentProgress(context.Background(), pipeline.ProgressEvent{
			TaskID: "task-1", Component: "Parser", Phase: 0,
		})
	}()

	// A later detail report must not block on a mutex leaked by the panic.
	sink.detailEmitter = func(component, unit string, current, total uint32) {}
	done := make(chan struct{})
	go func() {
		sink.OnProgressDetail(context.Background(), "Tokenizer", "chunk", 1, 5)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnProgressDetail blocked after emitter panic: emitMu was not released")
	}
}

// BenchmarkProgressSinkOnProgressDetail guards the per-chunk hot path: the
// serialization lock must not add meaningful overhead to each progress report.
func BenchmarkProgressSinkOnProgressDetail(b *testing.B) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()
	sink.detailEmitter = func(component, unit string, current, total uint32) {}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.OnProgressDetail(ctx, "Tokenizer", "chunk", uint32(i), 3000)
	}
}
