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
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// errTestModelCache is a sentinel error used by cache unit tests.
var errTestModelCache = errors.New("model cache test error")

// TestExtractor_ResolveChatTargetOncePerRun_NonDefault asserts that within a
// single ExtractorComponent.Invoke the chat-target resolution happens exactly
// ONCE per distinct llm_id and is reused across every chunk, instead of being
// re-resolved in every callRaw. The override seam is used as a counter because
// it is consulted inside the underlying resolver.
func TestExtractor_ResolveChatTargetOncePerRun_NonDefault(t *testing.T) {
	var resolverCalls atomic.Int32
	SetExtractorChatTargetResolverOverride(func(llmID string) (string, string, string, string, bool) {
		resolverCalls.Add(1)
		return "openai", "gpt-4o", "ak", "http://x", true
	})
	t.Cleanup(func() { SetExtractorChatTargetResolverOverride(nil) })

	stub := withStubChatInvoker(t,
		stubResponse{Content: "s0"},
		stubResponse{Content: "s1"},
		stubResponse{Content: "s2"},
	)

	const nChunks = 3
	chunks := make([]map[string]any, nChunks)
	for i := range chunks {
		chunks[i] = map[string]any{"text": "body"}
	}

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:   "some-model",
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": chunks}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if n := stub.Calls(); n != nChunks {
		t.Errorf("LLM calls = %d, want %d", n, nChunks)
	}
	if got := resolverCalls.Load(); got != 1 {
		t.Errorf("resolver calls = %d, want 1 (resolved once per run and reused across all %d chunks)", got, nChunks)
	}
}

// TestExtractor_ResolveChatTargetOncePerRun_Default covers the empty-llm_id
// (tenant default model) path: the chat-target resolution must happen exactly
// once per run and be reused across every chunk.
func TestExtractor_ResolveChatTargetOncePerRun_Default(t *testing.T) {
	var resolverCalls atomic.Int32
	SetExtractorChatTargetResolverOverride(func(llmID string) (string, string, string, string, bool) {
		resolverCalls.Add(1)
		return "openai", "gpt-4o", "ak", "http://x", true
	})
	t.Cleanup(func() { SetExtractorChatTargetResolverOverride(nil) })

	stub := withStubChatInvoker(t,
		stubResponse{Content: `{"category":"x"}`},
		stubResponse{Content: `{"category":"x"}`},
	)

	const nChunks = 2
	chunks := make([]map[string]any, nChunks)
	for i := range chunks {
		chunks[i] = map[string]any{"text": "body"}
	}

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:    "",
		Keywords: schema.KeywordExtractConfig{Enabled: true, TopN: 3},
		Metadata: schema.MetadataExtractConfig{
			Enabled:  true,
			Metadata: []common.MetadataFieldDef{{Key: "category", Type: "string"}},
		},
	}}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": chunks}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if n := stub.Calls(); n == 0 {
		t.Fatal("no LLM calls happened; test did not exercise the extraction path")
	}
	if got := resolverCalls.Load(); got != 1 {
		t.Errorf("resolver calls = %d, want 1 (default model resolved once per run, reused across all chunks)", got)
	}
}

// TestExtractor_ResolveChatTargetNoCacheCallsEachTime guards the NO-cache
// branch: when no per-run cache is present in ctx (e.g. direct callers /
// other entry points), resolveExtractorChatTarget must keep consulting the
// resolver exactly once per call.
func TestExtractor_ResolveChatTargetNoCacheCallsEachTime(t *testing.T) {
	var resolverCalls atomic.Int32
	SetExtractorChatTargetResolverOverride(func(llmID string) (string, string, string, string, bool) {
		resolverCalls.Add(1)
		return "openai", "gpt-4o", "ak", "http://x", true
	})
	t.Cleanup(func() { SetExtractorChatTargetResolverOverride(nil) })

	const n = 4
	for i := 0; i < n; i++ {
		if _, _, _, _, err := resolveExtractorChatTarget(t.Context(), nil, "some-model"); err != nil {
			t.Fatalf("resolveExtractorChatTarget: %v", err)
		}
	}
	if got := resolverCalls.Load(); got != n {
		t.Errorf("resolver calls = %d, want %d (no-cache path must resolve per call)", got, n)
	}
}

// TestExtractor_ContextLengthNoCacheCallsEachTime guards the NO-cache branch
// for extractorContextLength: without a per-run cache the context-length
// override seam must be consulted every call.
func TestExtractor_ContextLengthNoCacheCallsEachTime(t *testing.T) {
	var calls atomic.Int32
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int {
		calls.Add(1)
		return 123
	})
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	const n = 4
	for i := 0; i < n; i++ {
		if got := extractorContextLength(t.Context(), nil, "some-model"); got != 123 {
			t.Fatalf("extractorContextLength = %d, want 123", got)
		}
	}
	if got := calls.Load(); got != n {
		t.Errorf("context-length override calls = %d, want %d (no-cache path must resolve per call)", got, n)
	}
}

// newSpyModelCache builds an extractorModelCache with injected spy resolvers so
// the cache behavior can be tested without a database.
func newSpyModelCache(t *testing.T, targetCalls *atomic.Int32, ctxLenCalls *atomic.Int32) *extractorModelCache {
	t.Helper()
	return &extractorModelCache{
		targets: make(map[string]extractorResolvedTarget),
		ctxLens: make(map[string]int),
		resolveTarget: func(_ context.Context, _ *gorm.DB, llmID string) (string, string, string, string, error) {
			targetCalls.Add(1)
			return "d", llmID, "k", "u", nil
		},
		resolveCtxLen: func(_ context.Context, _ *gorm.DB, _ string) int {
			ctxLenCalls.Add(1)
			return 123
		},
	}
}

// TestExtractorModelCache_TargetCachesSuccess verifies a successful resolution
// is computed once and reused.
func TestExtractorModelCache_TargetCachesSuccess(t *testing.T) {
	var tc, cc atomic.Int32
	cache := newSpyModelCache(t, &tc, &cc)

	for i := 0; i < 5; i++ {
		d, m, k, u, err := cache.target(t.Context(), nil, "x")
		if err != nil {
			t.Fatalf("target: %v", err)
		}
		if d != "d" || m != "x" || k != "k" || u != "u" {
			t.Fatalf("unexpected target: %q %q %q %q", d, m, k, u)
		}
	}
	if got := tc.Load(); got != 1 {
		t.Errorf("target resolver calls = %d, want 1 (cached after first)", got)
	}
}

// TestExtractorModelCache_TargetDoesNotCacheError verifies a failed resolution
// is never memoized: every call re-invokes the resolver and returns the error,
// so a transient failure is re-attempted and the run fails fast on first use.
func TestExtractorModelCache_TargetDoesNotCacheError(t *testing.T) {
	var tc atomic.Int32
	cache := &extractorModelCache{
		targets: make(map[string]extractorResolvedTarget),
		ctxLens: make(map[string]int),
		resolveTarget: func(_ context.Context, _ *gorm.DB, _ string) (string, string, string, string, error) {
			tc.Add(1)
			return "", "", "", "", errTestModelCache
		},
	}

	const n = 4
	for i := 0; i < n; i++ {
		if _, _, _, _, err := cache.target(t.Context(), nil, "x"); err != errTestModelCache {
			t.Fatalf("target err = %v, want errTestModelCache", err)
		}
	}
	if got := tc.Load(); got != n {
		t.Errorf("target resolver calls = %d, want %d (errors must not be cached)", got, n)
	}
}

// TestExtractorModelCache_TargetConcurrentWarmThenRead proves the production
// shape: the run warms the cache once (serial), then the parallel chunk jobs
// read a populated entry — the resolver is invoked exactly once and every
// concurrent reader observes the identical value.
func TestExtractorModelCache_TargetConcurrentWarmThenRead(t *testing.T) {
	var tc, cc atomic.Int32
	cache := newSpyModelCache(t, &tc, &cc)

	// Warm serially (mirrors Invoke's pre-fan-out warm).
	if _, _, _, _, err := cache.target(t.Context(), nil, "x"); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if got := tc.Load(); got != 1 {
		t.Fatalf("warm resolver calls = %d, want 1", got)
	}

	const readers = 64
	var wg sync.WaitGroup
	wg.Add(readers)
	var bad atomic.Int32
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			d, m, k, u, err := cache.target(t.Context(), nil, "x")
			if err != nil || d != "d" || m != "x" || k != "k" || u != "u" {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()

	if bad.Load() != 0 {
		t.Errorf("%d concurrent readers observed a wrong value", bad.Load())
	}
	if got := tc.Load(); got != 1 {
		t.Errorf("target resolver calls = %d, want 1 (warm populated; concurrent reads hit cache)", got)
	}
}

// TestExtractorModelCache_TargetConcurrentMiss exercises the safety-net case
// where many goroutines miss simultaneously. It asserts correctness (all
// readers get the same value, no panic/deadlock) rather than an exact call
// count, which depends on scheduling.
func TestExtractorModelCache_TargetConcurrentMiss(t *testing.T) {
	var tc atomic.Int32
	cache := &extractorModelCache{
		targets: make(map[string]extractorResolvedTarget),
		ctxLens: make(map[string]int),
		resolveTarget: func(_ context.Context, _ *gorm.DB, llmID string) (string, string, string, string, error) {
			tc.Add(1)
			return "d", llmID, "k", "u", nil
		},
	}

	const readers = 64
	var wg sync.WaitGroup
	wg.Add(readers)
	var bad atomic.Int32
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			d, m, k, u, err := cache.target(t.Context(), nil, "x")
			if err != nil || d != "d" || m != "x" || k != "k" || u != "u" {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()

	if bad.Load() != 0 {
		t.Errorf("%d concurrent readers observed a wrong value", bad.Load())
	}
	if got := tc.Load(); got < 1 {
		t.Errorf("target resolver calls = %d, want >= 1", got)
	}
}

// TestExtractorModelCache_CtxLenCaches verifies the context length is resolved
// once and reused, including the zero (unknown model) result which is valid.
func TestExtractorModelCache_CtxLenCaches(t *testing.T) {
	var cc atomic.Int32
	cache := &extractorModelCache{
		targets: make(map[string]extractorResolvedTarget),
		ctxLens: make(map[string]int),
		resolveCtxLen: func(_ context.Context, _ *gorm.DB, _ string) int {
			cc.Add(1)
			return 0
		},
	}
	for i := 0; i < 5; i++ {
		if v := cache.ctxLen(t.Context(), nil, "x"); v != 0 {
			t.Fatalf("ctxLen = %d, want 0", v)
		}
	}
	if got := cc.Load(); got != 1 {
		t.Errorf("ctxLen resolver calls = %d, want 1 (zero result is cached)", got)
	}
}
