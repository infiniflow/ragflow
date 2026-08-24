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

package llmcache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestEngineWithRedis[T any](t *testing.T, opts ...Option[T]) (*Engine[T], *miniredis.Miniredis, goredis.UniversalClient) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	allOpts := append([]Option[T]{WithUniversalClient[T](rdb)}, opts...)
	engine := New[T](allOpts...)
	return engine, mr, rdb
}

func TestBuildKey(t *testing.T) {
	k1 := BuildKey("tenant1", "keywords", "modelA", "prompt1", "text1")
	k2 := BuildKey("tenant1", "keywords", "modelA", "prompt1", "text1")
	if k1 != k2 {
		t.Errorf("BuildKey should be deterministic: %s != %s", k1, k2)
	}

	// Tenant isolation test
	kTenant2 := BuildKey("tenant2", "keywords", "modelA", "prompt1", "text1")
	if k1 == kTenant2 {
		t.Errorf("Different tenants must produce different keys: %s == %s", k1, kTenant2)
	}

	// TaskType isolation test
	kQuestions := BuildKey("tenant1", "questions", "modelA", "prompt1", "text1")
	if k1 == kQuestions {
		t.Errorf("Different task types must produce different keys: %s == %s", k1, kQuestions)
	}

	// Null-byte collision prevention test: ("ab", "c") vs ("a", "bc")
	kColl1 := BuildKey("t", "test", "ab", "c")
	kColl2 := BuildKey("t", "test", "a", "bc")
	if kColl1 == kColl2 {
		t.Errorf("Null byte should prevent cross-field collision: %s == %s", kColl1, kColl2)
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrTypeAssertion == nil {
		t.Error("ErrTypeAssertion must not be nil")
	}
}

func TestEngine_GetOrCompute_NoRedis(t *testing.T) {
	// In unit-test environment without Redis running, GetOrCompute should fall back
	// gracefully to computeFn (Fail-Open behavior).
	engine := New[string](
		WithTTL[string](time.Hour),
		WithValidator(func(v string) bool { return len(v) > 0 }),
	)

	var calls int32
	ctx := context.Background()
	res, err := engine.GetOrCompute(ctx, "t1", "summary", []string{"m1", "text1"}, func(ctx context.Context) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "summary_output", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "summary_output" {
		t.Errorf("got %q, want 'summary_output'", res)
	}
	if calls != 1 {
		t.Errorf("computeFn called %d times, want 1", calls)
	}
}

func TestEngine_GetOrCompute_ValidatorAllowsLegitimateEmpty(t *testing.T) {
	// Verify that validator allowing empty collections can return empty slice without error
	engine := New[[]string](
		WithValidator(func(v []string) bool { return true }), // Accept empty slices
	)

	ctx := context.Background()
	res, err := engine.GetOrCompute(ctx, "t1", "keywords", []string{"m1", "text_empty"}, func(ctx context.Context) ([]string, error) {
		return []string{}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("got len %d, want 0", len(res))
	}
}

func TestEngine_GetOrCompute_SingleFlightDeduplication(t *testing.T) {
	engine := New[string]()

	var computeCount int32
	var wg sync.WaitGroup
	concurrentReqs := 10
	results := make([]string, concurrentReqs)

	for i := 0; i < concurrentReqs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res, err := engine.GetOrCompute(context.Background(), "t1", "summary", []string{"same_model", "same_text"}, func(ctx context.Context) (string, error) {
				time.Sleep(50 * time.Millisecond) // Simulate slow LLM
				atomic.AddInt32(&computeCount, 1)
				return "computed_value", nil
			})
			if err == nil {
				results[idx] = res
			}
		}(i)
	}

	wg.Wait()

	if computeCount != 1 {
		t.Errorf("SingleFlight failed: computeCount = %d, want 1", computeCount)
	}
	for i, r := range results {
		if r != "computed_value" {
			t.Errorf("result[%d] = %q, want 'computed_value'", i, r)
		}
	}
}

func TestEngine_GetOrCompute_ContextTimeout(t *testing.T) {
	engine := New[string]()

	// Compute blocks for 300ms, but context times out in 30ms.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := engine.GetOrCompute(ctx, "t1", "summary", []string{"m1", "slow_prompt"}, func(computeCtx context.Context) (string, error) {
		time.Sleep(300 * time.Millisecond)
		return "should_not_wait", nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("GetOrCompute should return immediately on timeout, but elapsed %v", elapsed)
	}
}

func TestEngine_GetOrCompute_ContextCancellation(t *testing.T) {
	engine := New[string]()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context asynchronously after 20ms
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := engine.GetOrCompute(ctx, "t1", "summary", []string{"m1", "cancel_prompt"}, func(computeCtx context.Context) (string, error) {
		time.Sleep(300 * time.Millisecond)
		return "should_be_cancelled", nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("GetOrCompute should return immediately on cancellation, but elapsed %v", elapsed)
	}
}

func TestEngine_JitterRange(t *testing.T) {
	t.Run("100s base duration", func(t *testing.T) {
		base := 100 * time.Second
		minExpected := 90 * time.Second
		maxExpected := 110 * time.Second

		var minObserved time.Duration = 1000 * time.Second
		var maxObserved time.Duration = 0

		for i := 0; i < 1000; i++ {
			jittered := ApplyJitter(base)
			if jittered < minExpected || jittered > maxExpected {
				t.Fatalf("jittered value %v out of bounds [%v, %v]", jittered, minExpected, maxExpected)
			}
			if jittered < minObserved {
				minObserved = jittered
			}
			if jittered > maxObserved {
				maxObserved = jittered
			}
		}

		// Verify variance across 1000 runs
		if minObserved > 95*time.Second || maxObserved < 105*time.Second {
			t.Fatalf("expected substantial jitter spread, got min=%v, max=%v", minObserved, maxObserved)
		}
	})

	t.Run("24h base duration", func(t *testing.T) {
		base := 24 * time.Hour
		minExpected := time.Duration(float64(base) * 0.90)
		maxExpected := time.Duration(float64(base) * 1.10)

		for i := 0; i < 500; i++ {
			jittered := ApplyJitter(base)
			if jittered < minExpected || jittered > maxExpected {
				t.Fatalf("jittered value %v out of bounds [%v, %v]", jittered, minExpected, maxExpected)
			}
		}
	})

	t.Run("non-positive durations", func(t *testing.T) {
		if got := ApplyJitter(0); got != 0 {
			t.Errorf("ApplyJitter(0) = %v, want 0", got)
		}
		if got := ApplyJitter(-5 * time.Second); got != -5*time.Second {
			t.Errorf("ApplyJitter(-5s) = %v, want -5s", got)
		}
	})
}

func TestEngine_FailClosed_EmptyTenantID(t *testing.T) {
	engine, _, rdb := newTestEngineWithRedis[string](t)
	ctx := context.Background()

	var computeCalls int32
	// 1. Calling GetOrCompute with tenantID == "" should bypass cache and execute computeFn
	res, err := engine.GetOrCompute(ctx, "", "summary", []string{"m1", "text1"}, func(ctx context.Context) (string, error) {
		atomic.AddInt32(&computeCalls, 1)
		return "compute_result", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "compute_result" {
		t.Errorf("got %q, want 'compute_result'", res)
	}
	if computeCalls != 1 {
		t.Errorf("computeFn called %d times, want 1", computeCalls)
	}

	// Verify that absolutely NO keys were written to Redis
	keys, err := rdb.Keys(ctx, "*").Result()
	if err != nil {
		t.Fatalf("Keys error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys written to Redis for empty tenantID, found %d: %v", len(keys), keys)
	}

	// 2. Calling FlushTenant with tenantID == "" should return 0, nil
	deleted, err := engine.FlushTenant(ctx, "")
	if err != nil {
		t.Fatalf("FlushTenant(\"\") returned error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("FlushTenant(\"\") = %d, want 0", deleted)
	}
}

func TestEngine_FlushTenant_ScanPipeline(t *testing.T) {
	t.Run("multi-tenant isolation purge", func(t *testing.T) {
		engine, _, rdb := newTestEngineWithRedis[string](t)
		ctx := context.Background()

		// Populate 10 keys for tenantA and 5 keys for tenantB
		for i := 0; i < 10; i++ {
			_, err := engine.GetOrCompute(ctx, "tenantA", "task", []string{fmt.Sprintf("item_%d", i)}, func(ctx context.Context) (string, error) {
				return fmt.Sprintf("val_A_%d", i), nil
			})
			if err != nil {
				t.Fatalf("GetOrCompute tenantA error: %v", err)
			}
		}
		for i := 0; i < 5; i++ {
			_, err := engine.GetOrCompute(ctx, "tenantB", "task", []string{fmt.Sprintf("item_%d", i)}, func(ctx context.Context) (string, error) {
				return fmt.Sprintf("val_B_%d", i), nil
			})
			if err != nil {
				t.Fatalf("GetOrCompute tenantB error: %v", err)
			}
		}

		// Verify total keys
		allKeys, err := rdb.Keys(ctx, "*").Result()
		if err != nil {
			t.Fatalf("Keys error: %v", err)
		}
		if len(allKeys) != 15 {
			t.Fatalf("expected 15 total keys in Redis, got %d", len(allKeys))
		}

		// Flush tenantA
		deleted, err := engine.FlushTenant(ctx, "tenantA")
		if err != nil {
			t.Fatalf("FlushTenant(tenantA) error: %v", err)
		}
		if deleted != 10 {
			t.Errorf("FlushTenant(tenantA) deleted %d keys, want 10", deleted)
		}

		// Verify tenantA keys are gone, tenantB keys remain
		remainingKeys, err := rdb.Keys(ctx, "*").Result()
		if err != nil {
			t.Fatalf("Keys error: %v", err)
		}
		if len(remainingKeys) != 5 {
			t.Fatalf("expected 5 remaining keys for tenantB, got %d: %v", len(remainingKeys), remainingKeys)
		}

		// Verify tenantB can still read from cache without recomputing
		var computeBCalled bool
		valB, err := engine.GetOrCompute(ctx, "tenantB", "task", []string{"item_0"}, func(ctx context.Context) (string, error) {
			computeBCalled = true
			return "new_computed_val", nil
		})
		if err != nil {
			t.Fatalf("GetOrCompute tenantB error: %v", err)
		}
		if valB != "val_B_0" {
			t.Errorf("got %q, want cached 'val_B_0'", valB)
		}
		if computeBCalled {
			t.Errorf("tenantB entry should have been a cache hit, but computeFn was called")
		}
	})

	t.Run("context cancelled before flush", func(t *testing.T) {
		engine, _, _ := newTestEngineWithRedis[string](t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // immediately cancel

		deleted, err := engine.FlushTenant(ctx, "tenantA")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
		if deleted != 0 {
			t.Errorf("FlushTenant with cancelled context returned %d, want 0", deleted)
		}
	})

	t.Run("multi-batch scan with large key count", func(t *testing.T) {
		engine, _, rdb := newTestEngineWithRedis[string](t)
		ctx := context.Background()

		totalKeys := 1050
		pipe := rdb.Pipeline()
		for i := 0; i < totalKeys; i++ {
			k := BuildKey("tenant_large", "task", fmt.Sprintf("key_%d", i))
			pipe.Set(ctx, k, fmt.Sprintf(`"val_%d"`, i), time.Hour)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			t.Fatalf("Pipeline setup error: %v", err)
		}

		// Verify miniredis has all 1050 keys
		keys, err := rdb.Keys(ctx, "kc:llm:tenant_large:*").Result()
		if err != nil {
			t.Fatalf("Keys error: %v", err)
		}
		if len(keys) != totalKeys {
			t.Fatalf("expected %d keys before flush, got %d", totalKeys, len(keys))
		}

		// Flush tenant_large
		deleted, err := engine.FlushTenant(ctx, "tenant_large")
		if err != nil {
			t.Fatalf("FlushTenant(tenant_large) error: %v", err)
		}
		if deleted != totalKeys {
			t.Errorf("FlushTenant deleted %d keys, want %d", deleted, totalKeys)
		}

		// Verify 0 remaining keys
		remaining, err := rdb.Keys(ctx, "kc:llm:tenant_large:*").Result()
		if err != nil {
			t.Fatalf("Keys error: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("expected 0 keys remaining, found %d", len(remaining))
		}
	})
}

func TestEngine_MetricsHook(t *testing.T) {
	var events []Event
	var mu sync.Mutex

	engine := New[string](
		WithMetrics[string](func(ev Event) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}),
	)

	ctx := context.Background()
	_, _ = engine.GetOrCompute(ctx, "tenantA", "keywords", []string{"p1"}, func(ctx context.Context) (string, error) {
		return "result", nil
	})

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Errorf("expected metrics events to be emitted, got none")
	}
}

func TestEngine_GetOrCompute_ContextCancellation_AllowsImmediateRetry(t *testing.T) {
	engine := New[string]()

	// 1. Slow flight with quick timeout
	ctx1, cancel1 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel1()

	computeStarted := make(chan struct{})
	computeFinish := make(chan struct{})

	go func() {
		_, _ = engine.GetOrCompute(ctx1, "tenant1", "summary", []string{"retry_key"}, func(ctx context.Context) (string, error) {
			close(computeStarted)
			<-computeFinish // Hold until we allow it to finish
			return "slow_result", nil
		})
	}()

	<-computeStarted
	// Wait for ctx1 to time out
	time.Sleep(35 * time.Millisecond)

	// 2. Immediate retry by a new caller should not hang or fail due to the slow first request
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()

	res2, err2 := engine.GetOrCompute(ctx2, "tenant1", "summary", []string{"retry_key"}, func(ctx context.Context) (string, error) {
		return "fast_result", nil
	})
	close(computeFinish)

	if err2 != nil {
		t.Fatalf("retry GetOrCompute failed: %v", err2)
	}
	if res2 != "fast_result" {
		t.Errorf("got %q, want 'fast_result'", res2)
	}
}

func TestEngine_FlushTenant_ScanError(t *testing.T) {
	engine, mr, _ := newTestEngineWithRedis[string](t)
	ctx := context.Background()

	// Close miniredis immediately so SCAN / pipeline fails
	mr.Close()

	deleted, err := engine.FlushTenant(ctx, "tenant_err")
	if err == nil {
		t.Errorf("expected error when redis server is closed, got nil")
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestEngine_GetOrCompute_ComplexTypes_NoReflect(t *testing.T) {
	engine, _, _ := newTestEngineWithRedis[map[string]int](t)
	ctx := context.Background()

	// Verify map types serialize and deserialize properly without reflect overhead
	val, err := engine.GetOrCompute(ctx, "tenant1", "tags", []string{"doc1"}, func(ctx context.Context) (map[string]int, error) {
		return map[string]int{"tagA": 10, "tagB": 20}, nil
	})
	if err != nil {
		t.Fatalf("first GetOrCompute failed: %v", err)
	}
	if val["tagA"] != 10 || val["tagB"] != 20 {
		t.Errorf("unexpected map content: %v", val)
	}

	// Read from cache
	var computeCalled bool
	cachedVal, err := engine.GetOrCompute(ctx, "tenant1", "tags", []string{"doc1"}, func(ctx context.Context) (map[string]int, error) {
		computeCalled = true
		return nil, errors.New("should not compute")
	})
	if err != nil {
		t.Fatalf("cached GetOrCompute failed: %v", err)
	}
	if computeCalled {
		t.Errorf("computeFn should not have been called on cache hit")
	}
	if cachedVal["tagA"] != 10 || cachedVal["tagB"] != 20 {
		t.Errorf("unexpected cached map content: %v", cachedVal)
	}
}

func TestEngine_GetOrCompute_WriteBack_ContextCancelled(t *testing.T) {
	engine, _, _ := newTestEngineWithRedis[string](t)

	ctx, cancel := context.WithCancel(context.Background())
	computeDone := make(chan struct{})

	go func() {
		_, _ = engine.GetOrCompute(ctx, "tenant1", "summary", []string{"slow_key"}, func(c context.Context) (string, error) {
			cancel() // Cancel caller context while computing
			time.Sleep(20 * time.Millisecond)
			return "computed_despite_cancel", nil
		})
		close(computeDone)
	}()

	<-computeDone
	// Allow a moment for background singleflight goroutine to finish set
	time.Sleep(30 * time.Millisecond)

	// Now check if a new request hits the cache
	newCtx := context.Background()
	var computeCalled bool
	val, err := engine.GetOrCompute(newCtx, "tenant1", "summary", []string{"slow_key"}, func(c context.Context) (string, error) {
		computeCalled = true
		return "should_not_recompute", nil
	})
	if err != nil {
		t.Fatalf("unexpected error on cache read: %v", err)
	}
	if computeCalled {
		t.Errorf("expected cache hit after previous cancelled request wrote back, but computeFn was called")
	}
	if val != "computed_despite_cancel" {
		t.Errorf("got %q, want 'computed_despite_cancel'", val)
	}
}

func TestEngine_GetOrCompute_Cancellation_SingleForgetNoRace(t *testing.T) {
	engine, _, _ := newTestEngineWithRedis[string](t,
		WithValidator[string](func(s string) bool {
			return s != "resultA" // Prevent cancelled A from populating Redis so C strictly tests SingleFlight joining B
		}),
	)

	// 1. Caller A starts a slow flight
	ctxA, cancelA := context.WithCancel(context.Background())
	startedA := make(chan struct{})
	finishA := make(chan struct{})

	go func() {
		_, _ = engine.GetOrCompute(ctxA, "tenant1", "summary", []string{"race_key"}, func(c context.Context) (string, error) {
			close(startedA)
			<-finishA
			return "resultA", nil
		})
	}()

	<-startedA
	// Cancel Caller A's context, triggering Forget(key)
	cancelA()
	time.Sleep(10 * time.Millisecond)

	// 2. Caller B starts a new flight for the same key while A is still running in background
	ctxB := context.Background()
	startedB := make(chan struct{})
	finishB := make(chan struct{})
	var computeCountB atomic.Int32

	var resB string
	var errB error
	doneB := make(chan struct{})
	go func() {
		resB, errB = engine.GetOrCompute(ctxB, "tenant1", "summary", []string{"race_key"}, func(c context.Context) (string, error) {
			computeCountB.Add(1)
			close(startedB)
			<-finishB
			return "resultB", nil
		})
		close(doneB)
	}()

	<-startedB

	// 3. Now let Caller A's original background task finish.
	// If there was a double-forget, A completing would evict B's flight from singleflight map!
	close(finishA)
	time.Sleep(20 * time.Millisecond)

	// 4. Caller C arrives while Caller B is still computing.
	// Caller C MUST merge into Caller B's existing flight and NOT start a new computation.
	ctxC := context.Background()
	var resC string
	var errC error
	doneC := make(chan struct{})
	go func() {
		resC, errC = engine.GetOrCompute(ctxC, "tenant1", "summary", []string{"race_key"}, func(c context.Context) (string, error) {
			computeCountB.Add(1) // Should NOT be called
			return "resultC_unwanted", nil
		})
		close(doneC)
	}()

	// 5. Allow Caller B's computation to finish
	close(finishB)
	<-doneB
	<-doneC

	if errB != nil || errC != nil {
		t.Fatalf("unexpected error: B=%v, C=%v", errB, errC)
	}
	if computeCountB.Load() != 1 {
		t.Errorf("compute count = %d, want 1 (SingleFlight dedup was defeated by double forget race)", computeCountB.Load())
	}
	if resB != "resultB" || resC != "resultB" {
		t.Errorf("resB = %q, resC = %q, both want 'resultB'", resB, resC)
	}
}

func BenchmarkBuildKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildKey("tenant1", "task", "part1", "part2", "part3")
	}
}

func BenchmarkEngine_ApplyJitter(b *testing.B) {
	base := 24 * time.Hour
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = ApplyJitter(base)
		}
	})
}
