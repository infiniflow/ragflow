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

package component

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cacheKey is the key formula used by clientFor. Exposed so tests
// can derive the same key without re-implementing the formula.
func cacheKey(req RunTaskRequest) string {
	return req.BaseURL + "|" + req.APIKey + "|" + req.ModelName
}

// cacheSize returns the current entry count of r.cache.
func cacheSize(r *stagehandRuntime) int {
	n := 0
	r.cache.Range(func(_, _ any) bool { n++; return true })
	return n
}

// ---------------------------------------------------------------------------
// Validation (unchanged)
// ---------------------------------------------------------------------------

func TestStagehandRuntime_ValidatesRequiredFields(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 0, time.Minute) // TTL large → no sweeper interference
	t.Cleanup(func() { _ = r.Close() })

	cases := []struct {
		name string
		req  RunTaskRequest
		want string
	}{
		{"empty_instruction", RunTaskRequest{ModelName: "openai/gpt-4o", APIKey: "k"}, "Instruction"},
		{"empty_model", RunTaskRequest{Instruction: "x", APIKey: "k"}, "ModelName"},
		{"empty_api_key", RunTaskRequest{Instruction: "x", ModelName: "openai/gpt-4o"}, "APIKey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.RunTask(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err.Error(), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cache hit / miss / key formula
// ---------------------------------------------------------------------------

func TestStagehandRuntime_Cache_HitMiss(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	// First call: miss → build → cache has 1 entry.
	if _, err := r.clientFor(RunTaskRequest{
		ModelName: "openai/gpt-4o",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-a",
	}); err != nil {
		t.Fatalf("clientFor(a): %v", err)
	}
	if got := cacheSize(r); got != 1 {
		t.Errorf("after first clientFor: cache size = %d, want 1", got)
	}

	// Same key: hit → still 1 entry.
	if _, err := r.clientFor(RunTaskRequest{
		ModelName: "openai/gpt-4o",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-a",
	}); err != nil {
		t.Fatalf("clientFor(b): %v", err)
	}
	if got := cacheSize(r); got != 1 {
		t.Errorf("after same-key clientFor: cache size = %d, want 1", got)
	}

	// Different apiKey → miss → 2 entries.
	if _, err := r.clientFor(RunTaskRequest{
		ModelName: "openai/gpt-4o",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-b",
	}); err != nil {
		t.Fatalf("clientFor(c): %v", err)
	}
	if got := cacheSize(r); got != 2 {
		t.Errorf("after different-apiKey clientFor: cache size = %d, want 2", got)
	}

	// Different modelName → 3 entries.
	if _, err := r.clientFor(RunTaskRequest{
		ModelName: "openai/gpt-4o-mini",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-b",
	}); err != nil {
		t.Fatalf("clientFor(d): %v", err)
	}
	if got := cacheSize(r); got != 3 {
		t.Errorf("after different-modelName clientFor: cache size = %d, want 3", got)
	}

	// Different baseURL → 4 entries.
	if _, err := r.clientFor(RunTaskRequest{
		ModelName: "openai/gpt-4o-mini",
		BaseURL:   "https://api.deepseek.com/v1",
		APIKey:    "sk-b",
	}); err != nil {
		t.Fatalf("clientFor(e): %v", err)
	}
	if got := cacheSize(r); got != 4 {
		t.Errorf("after different-baseURL clientFor: cache size = %d, want 4", got)
	}
}

func TestStagehandRuntime_Cache_LastUsedTouched(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	req := RunTaskRequest{
		ModelName: "openai/gpt-4o",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-a",
	}
	if _, err := r.clientFor(req); err != nil {
		t.Fatalf("clientFor: %v", err)
	}
	key := cacheKey(req)
	v1, ok := r.cache.Load(key)
	if !ok {
		t.Fatal("entry missing after clientFor")
	}
	e1 := v1.(*stagehandClientEntry)
	t1 := e1.lastUsedAt.Load()

	// Sleep so the timestamp would advance if touched.
	time.Sleep(2 * time.Millisecond)

	if _, err := r.clientFor(req); err != nil {
		t.Fatalf("clientFor (second): %v", err)
	}
	v2, _ := r.cache.Load(key)
	e2 := v2.(*stagehandClientEntry)
	t2 := e2.lastUsedAt.Load()

	if t2 <= t1 {
		t.Errorf("lastUsedAt not advanced: t1=%d, t2=%d", t1, t2)
	}
}

// ---------------------------------------------------------------------------
// Concurrent dedup
// ---------------------------------------------------------------------------

func TestStagehandRuntime_Cache_ConcurrentSameKey_NoDuplicateBuild(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	req := RunTaskRequest{
		ModelName: "openai/gpt-4o",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-conc",
	}
	const N = 64

	var wg sync.WaitGroup
	var calls atomic.Int32
	for i := 0; i < N; i++ {
		wg.Go(func() {
			_, _ = r.clientFor(req)
			calls.Add(1)
		})
	}
	wg.Wait()
	if calls.Load() != N {
		t.Errorf("calls = %d, want %d", calls.Load(), N)
	}
	// Even with race conditions, the cache must end up with exactly
	// 1 entry — sync.Map.LoadOrStore guarantees deduplication.
	if got := cacheSize(r); got != 1 {
		t.Errorf("after %d concurrent same-key clientFor: cache size = %d, want 1", N, got)
	}
}

func TestStagehandRuntime_Cache_ConcurrentDifferentKeys(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	const N = 32
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		wg.Go(func() {
			_, _ = r.clientFor(RunTaskRequest{
				ModelName: "openai/gpt-4o",
				BaseURL:   "https://api.openai.com/v1",
				APIKey:    "sk-" + string(rune('a'+i%26)) + string(rune('A'+(i/26)%26)),
			})
		})
	}
	wg.Wait()
	// 32 goroutines, but the apiKey pattern has 26*2 = 52 unique
	// values. With N=32 < 52 collisions, expect exactly 32 entries.
	if got := cacheSize(r); got != N {
		t.Errorf("cache size = %d, want %d (one per distinct key)", got, N)
	}
}

// ---------------------------------------------------------------------------
// TTL eviction
// ---------------------------------------------------------------------------

func TestStagehandRuntime_Cache_TTLEviction(t *testing.T) {
	r := newStagehandRuntime(50*time.Millisecond, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	req := RunTaskRequest{
		ModelName: "openai/gpt-4o",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-ttl",
	}
	if _, err := r.clientFor(req); err != nil {
		t.Fatalf("clientFor: %v", err)
	}
	if got := cacheSize(r); got != 1 {
		t.Fatalf("after clientFor: cache size = %d, want 1", got)
	}

	// Backdate the entry past TTL and trigger eviction directly
	// (we don't want to wait for the sweeper in a unit test).
	key := cacheKey(req)
	v, _ := r.cache.Load(key)
	v.(*stagehandClientEntry).lastUsedAt.Store(time.Now().Add(-time.Hour).UnixNano())

	r.evictExpired()

	if got := cacheSize(r); got != 0 {
		t.Errorf("after evictExpired: cache size = %d, want 0", got)
	}
}

func TestStagehandRuntime_Cache_TTL_FreshEntriesSurvive(t *testing.T) {
	r := newStagehandRuntime(50*time.Millisecond, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	if _, err := r.clientFor(RunTaskRequest{ModelName: "openai/gpt-4o", APIKey: "sk-fresh"}); err != nil {
		t.Fatalf("clientFor: %v", err)
	}
	// Don't backdate → entry is fresh → should survive eviction.
	r.evictExpired()
	if got := cacheSize(r); got != 1 {
		t.Errorf("fresh entry evicted: cache size = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// LRU eviction
// ---------------------------------------------------------------------------

func TestStagehandRuntime_Cache_LRUEviction(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 2 /*cap=2*/, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	// Insert 3 entries with distinct, ordered lastUsedAt values.
	// enforceLRUCap fires inline after each insert, so after the
	// 3rd insert the cache is already at cap=2 with the oldest
	// (sk-1) evicted.
	mk := func(apiKey string) RunTaskRequest {
		return RunTaskRequest{ModelName: "openai/gpt-4o", APIKey: apiKey}
	}
	if _, err := r.clientFor(mk("sk-1")); err != nil {
		t.Fatalf("clientFor 1: %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := r.clientFor(mk("sk-2")); err != nil {
		t.Fatalf("clientFor 2: %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := r.clientFor(mk("sk-3")); err != nil {
		t.Fatalf("clientFor 3: %v", err)
	}

	if got := cacheSize(r); got != 2 {
		t.Errorf("after 3 inserts with cap=2: cache size = %d, want 2 (oldest evicted inline)", got)
	}
	if _, ok := r.cache.Load(cacheKey(mk("sk-1"))); ok {
		t.Errorf("oldest entry (sk-1) should be evicted; still present")
	}
	if _, ok := r.cache.Load(cacheKey(mk("sk-2"))); !ok {
		t.Errorf("second entry (sk-2) should still be present")
	}
	if _, ok := r.cache.Load(cacheKey(mk("sk-3"))); !ok {
		t.Errorf("newest entry (sk-3) should still be present")
	}
}

func TestStagehandRuntime_Cache_LRUEviction_TouchesLastUsed(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 2 /*cap=2*/, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	mk := func(apiKey string) RunTaskRequest {
		return RunTaskRequest{ModelName: "openai/gpt-4o", APIKey: apiKey}
	}
	if _, err := r.clientFor(mk("sk-1")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err := r.clientFor(mk("sk-2")); err != nil {
		t.Fatal(err)
	}

	// Touch sk-1 so it becomes the most-recently-used.
	time.Sleep(time.Millisecond)
	if _, err := r.clientFor(mk("sk-1")); err != nil {
		t.Fatal(err)
	}

	// Insert sk-3 → cap exceeded → oldest by lastUsedAt (sk-2)
	// should be evicted, sk-1 should survive.
	if _, err := r.clientFor(mk("sk-3")); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.cache.Load(cacheKey(mk("sk-1"))); !ok {
		t.Errorf("touched sk-1 should survive LRU eviction")
	}
	if _, ok := r.cache.Load(cacheKey(mk("sk-2"))); ok {
		t.Errorf("sk-2 should be evicted (was oldest after touch)")
	}
	if _, ok := r.cache.Load(cacheKey(mk("sk-3"))); !ok {
		t.Errorf("sk-3 should survive (newly inserted)")
	}
}

func TestStagehandRuntime_Cache_LRUCapZero_Unlimited(t *testing.T) {
	r := newStagehandRuntime(time.Hour, 0 /*unlimited*/, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	for i := 0; i < 100; i++ {
		_, _ = r.clientFor(RunTaskRequest{
			ModelName: "openai/gpt-4o",
			APIKey:    "sk-" + itoa(i),
		})
	}
	if got := cacheSize(r); got != 100 {
		t.Errorf("cap=0 should mean unlimited; cache size = %d, want 100", got)
	}
}

// itoa is a tiny helper to avoid pulling in strconv just for these tests.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestStagehandRuntime_Close_DrainsSweeperAndClosesAllEntries(t *testing.T) {
	r := newStagehandRuntime(50*time.Millisecond, 0, 10*time.Millisecond)
	for i := 0; i < 5; i++ {
		_, _ = r.clientFor(RunTaskRequest{ModelName: "openai/gpt-4o", APIKey: "sk-" + itoa(i)})
	}
	if got := cacheSize(r); got != 5 {
		t.Fatalf("precondition: cache size = %d, want 5", got)
	}

	done := make(chan struct{})
	go func() {
		_ = r.Close()
		close(done)
	}()
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s (sweeper did not drain)")
	}

	if got := cacheSize(r); got != 0 {
		t.Errorf("after Close: cache size = %d, want 0", got)
	}

	// Calling Close again is a safe no-op.
	if err := r.Close(); err != nil {
		t.Errorf("second Close: got %v, want nil", err)
	}
}

func TestStagehandRuntime_Close_TTLZero_NoSweeper(t *testing.T) {
	// ttl=0 disables the sweeper entirely; sweepDone must be
	// pre-closed so Close() doesn't block.
	r := newStagehandRuntime(0, 0, time.Second)
	_, _ = r.clientFor(RunTaskRequest{ModelName: "openai/gpt-4o", APIKey: "sk-z"})
	done := make(chan struct{})
	go func() {
		_ = r.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close hung with ttl=0 (sweeper not pre-closed)")
	}
}

// ---------------------------------------------------------------------------
// Env-driven config
// ---------------------------------------------------------------------------

func TestStagehandRuntime_EnvConfig_Defaults(t *testing.T) {
	// Clear env so we get the library defaults.
	t.Setenv("STAGEHAND_CACHE_TTL", "")
	t.Setenv("STAGEHAND_CACHE_CAP", "")
	t.Setenv("STAGEHAND_CACHE_SWEEP", "")
	r := newStagehandRuntimeFromEnv()
	t.Cleanup(func() { _ = r.Close() })
	if r.ttl != defaultStagehandCacheTTL {
		t.Errorf("default ttl: got %v, want %v", r.ttl, defaultStagehandCacheTTL)
	}
	if r.cap != defaultStagehandCacheCap {
		t.Errorf("default cap: got %d, want %d", r.cap, defaultStagehandCacheCap)
	}
}

func TestStagehandRuntime_EnvConfig_Overrides(t *testing.T) {
	t.Setenv("STAGEHAND_CACHE_TTL", "5m")
	t.Setenv("STAGEHAND_CACHE_CAP", "8")
	r := newStagehandRuntimeFromEnv()
	t.Cleanup(func() { _ = r.Close() })
	if r.ttl != 5*time.Minute {
		t.Errorf("override ttl: got %v, want 5m", r.ttl)
	}
	if r.cap != 8 {
		t.Errorf("override cap: got %d, want 8", r.cap)
	}
}

func TestStagehandRuntime_EnvConfig_InvalidFallsBack(t *testing.T) {
	t.Setenv("STAGEHAND_CACHE_TTL", "not-a-duration")
	t.Setenv("STAGEHAND_CACHE_CAP", "not-a-number")
	r := newStagehandRuntimeFromEnv()
	t.Cleanup(func() { _ = r.Close() })
	if r.ttl != defaultStagehandCacheTTL {
		t.Errorf("invalid ttl env should fall back to default: got %v, want %v",
			r.ttl, defaultStagehandCacheTTL)
	}
	if r.cap != defaultStagehandCacheCap {
		t.Errorf("invalid cap env should fall back to default: got %d, want %d",
			r.cap, defaultStagehandCacheCap)
	}
}

// ---------------------------------------------------------------------------
// SetDefaultStagehandInvoker (unchanged)
// ---------------------------------------------------------------------------

func TestStagehandRuntime_SetDefaultStagehandInvoker(t *testing.T) {
	prev := DefaultRuntime
	t.Cleanup(func() { SetDefaultStagehandInvoker(prev) })

	called := false
	mock := StagehandInvokerFunc(func(_ context.Context, _ RunTaskRequest) (string, error) {
		called = true
		return "ok", nil
	})
	var invoker StagehandInvoker = mock
	SetDefaultStagehandInvoker(invoker)
	got := getDefaultStagehandInvoker()
	if got == nil {
		t.Fatal("getDefaultStagehandInvoker returned nil after swap")
	}
	out, err := got.RunTask(context.Background(), RunTaskRequest{Instruction: "x", ModelName: "m", APIKey: "k"})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if out != "ok" || !called {
		t.Errorf("mock not invoked: out=%q, called=%v", out, called)
	}
}

// StagehandInvokerFunc is an adapter for the StagehandInvoker
// interface, useful in tests.
type StagehandInvokerFunc func(ctx context.Context, req RunTaskRequest) (string, error)

func (f StagehandInvokerFunc) RunTask(ctx context.Context, req RunTaskRequest) (string, error) {
	return f(ctx, req)
}

func (f StagehandInvokerFunc) RunExtract(_ context.Context, _ RunExtractRequest) (string, error) {
	return "", errors.New("RunExtract not implemented in StagehandInvokerFunc adapter")
}
