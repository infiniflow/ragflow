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
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
)

// fakeCacheStore is an in-memory chunkcache.Store for the extractor cache
// tests. A real Redis would push these into the integration tier; the point
// under test is which key is read/written, not Redis itself.
type fakeCacheStore struct {
	mu   sync.Mutex
	kv   map[string]string
	ttl  map[string]time.Duration
	sets map[string]map[string]bool
}

func newFakeCacheStore() *fakeCacheStore {
	return &fakeCacheStore{
		kv:   map[string]string{},
		ttl:  map[string]time.Duration{},
		sets: map[string]map[string]bool{},
	}
}

func (f *fakeCacheStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.kv[key]
	if !ok {
		return "", errors.New("redis: nil")
	}
	return v, nil
}

func (f *fakeCacheStore) Set(_ context.Context, key, value string, exp time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kv[key] = value
	f.ttl[key] = exp
	return true
}

func (f *fakeCacheStore) SAdd(_ context.Context, key, member string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sets[key] == nil {
		f.sets[key] = map[string]bool{}
	}
	f.sets[key][member] = true
	return true
}

func (f *fakeCacheStore) SMembers(_ context.Context, key string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.sets[key]))
	for m := range f.sets[key] {
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeCacheStore) Delete(_ context.Context, key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.kv, key)
	delete(f.sets, key)
	return true
}

func (f *fakeCacheStore) Expire(_ context.Context, key string, exp time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ttl[key] = exp
	return true
}

func (f *fakeCacheStore) get(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.kv[key]
	return v, ok
}

func (f *fakeCacheStore) members(key string) []string {
	out, _ := f.SMembers(context.Background(), key)
	return out
}

// cacheTaskCtx returns a ctx carrying a CanvasState with taskID, mirroring the
// pipeline so cache writes get recorded on the task manifest.
func cacheTaskCtx(taskID string) context.Context {
	ctx := runtime.WithState(context.Background(), runtime.NewCanvasState("", ""))
	globals.SetTaskID(ctx, taskID)
	return ctx
}

// TestCallTextCached_HitSkipsLLMCall asserts a cached extraction short-circuits
// the model call. This is the property that makes the single Parser-only
// checkpoint affordable: a resumed run re-walks every chunk but pays nothing.
func TestCallTextCached_HitSkipsLLMCall(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "fresh"})
	store := newFakeCacheStore()
	key := chunkcache.Key("extractor:keywords", "model-1", "chunk-1", "sys-prompt")
	store.Set(context.Background(), key, "cached", chunkcache.TTL)

	c := &ExtractorComponent{}
	in := extractorInputs{llmID: "model-1", cache: store}
	got, err := c.callTextCached(t.Context(), nil, in, "keywords", "sys-prompt", "chunk body", "chunk-1")
	if err != nil {
		t.Fatalf("callTextCached: %v", err)
	}
	if got != "cached" {
		t.Errorf("result = %q, want %q", got, "cached")
	}
	if n := stub.Calls(); n != 0 {
		t.Errorf("LLM calls = %d, want 0 on a cache hit", n)
	}
}

// TestCallTextCached_KeyedByChunkIDNotText is the core of the key change. The
// chunk id already derives from the text, so hashing the text again only makes
// entries longer — and it forced every caller to keep the exact same text
// around. A hit must depend on the chunk id alone.
func TestCallTextCached_KeyedByChunkIDNotText(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "fresh"})
	store := newFakeCacheStore()
	store.Set(context.Background(), chunkcache.Key("extractor:keywords", "model-1", "chunk-1", "sys"), "cached", chunkcache.TTL)

	c := &ExtractorComponent{}
	in := extractorInputs{llmID: "model-1", cache: store}
	// Same chunk id, deliberately different text: still a hit.
	got, err := c.callTextCached(t.Context(), nil, in, "keywords", "sys", "totally unrelated body", "chunk-1")
	if err != nil {
		t.Fatalf("callTextCached: %v", err)
	}
	if got != "cached" {
		t.Errorf("result = %q, want the cached value (key must ignore chunk text)", got)
	}
	if n := stub.Calls(); n != 0 {
		t.Errorf("LLM calls = %d, want 0", n)
	}
}

// TestCallTextCached_DifferentChunkIDMisses asserts two distinct chunks never
// share an entry.
func TestCallTextCached_DifferentChunkIDMisses(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "fresh"})
	store := newFakeCacheStore()
	store.Set(context.Background(), chunkcache.Key("extractor:keywords", "model-1", "chunk-1", "sys"), "cached", chunkcache.TTL)

	c := &ExtractorComponent{}
	in := extractorInputs{llmID: "model-1", cache: store}
	got, err := c.callTextCached(t.Context(), nil, in, "keywords", "sys", "body", "chunk-2")
	if err != nil {
		t.Fatalf("callTextCached: %v", err)
	}
	if got != "fresh" {
		t.Errorf("result = %q, want the freshly generated value", got)
	}
	if n := stub.Calls(); n != 1 {
		t.Errorf("LLM calls = %d, want 1 on a miss", n)
	}
}

// TestCallTextCached_WriteUsesSharedTTLAndManifest asserts a fresh extraction
// is cached under the shared 7-day TTL and recorded on the task manifest, so
// the persist stage can reclaim it immediately instead of leaving it to expire.
func TestCallTextCached_WriteUsesSharedTTLAndManifest(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "generated"})
	store := newFakeCacheStore()
	c := &ExtractorComponent{}
	in := extractorInputs{llmID: "model-1", cache: store}

	if _, err := c.callTextCached(cacheTaskCtx("task-9"), nil, in, "keywords", "sys", "body", "chunk-1"); err != nil {
		t.Fatalf("callTextCached: %v", err)
	}

	key := chunkcache.Key("extractor:keywords", "model-1", "chunk-1", "sys")
	if got, ok := store.get(key); !ok || got != "generated" {
		t.Errorf("cached value = %q (ok=%v), want %q", got, ok, "generated")
	}
	if got := store.ttl[key]; got != chunkcache.TTL {
		t.Errorf("cache TTL = %v, want %v", got, chunkcache.TTL)
	}
	if members := store.members("kc:manifest:task-9"); len(members) != 1 || members[0] != key {
		t.Errorf("manifest = %v, want [%s]", members, key)
	}
}

// TestCallTextCached_NoChunkIDBypassesCache asserts a chunk with no stable id
// (no chunker upstream) is neither read from nor written to the cache, rather
// than every such chunk sharing one bucket.
func TestCallTextCached_NoChunkIDBypassesCache(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "generated"})
	store := newFakeCacheStore()
	c := &ExtractorComponent{}
	in := extractorInputs{llmID: "model-1", cache: store}

	got, err := c.callTextCached(cacheTaskCtx("task-9"), nil, in, "keywords", "sys", "body", "")
	if err != nil {
		t.Fatalf("callTextCached: %v", err)
	}
	if got != "generated" {
		t.Errorf("result = %q, want %q", got, "generated")
	}
	if n := stub.Calls(); n != 1 {
		t.Errorf("LLM calls = %d, want 1", n)
	}
	if len(store.kv) != 0 {
		t.Errorf("cache writes = %v, want none without a chunk id", store.kv)
	}
	if len(store.sets) != 0 {
		t.Errorf("manifest writes = %v, want none without a chunk id", store.sets)
	}
}

// TestCallTextCached_NoStoreStillCalls asserts a Redis-less deployment keeps
// working: no cache, every call reaches the model.
func TestCallTextCached_NoStoreStillCalls(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "generated"})
	c := &ExtractorComponent{}
	got, err := c.callTextCached(t.Context(), nil, extractorInputs{llmID: "model-1"}, "keywords", "sys", "body", "chunk-1")
	if err != nil {
		t.Fatalf("callTextCached: %v", err)
	}
	if got != "generated" || stub.Calls() != 1 {
		t.Errorf("result = %q, calls = %d; want (%q, 1)", got, stub.Calls(), "generated")
	}
}

// TestMetadataLLMCache_KeyedByChunkID asserts the metadata extraction cache
// followed the same key change, and that a hit skips the model.
func TestMetadataLLMCache_KeyedByChunkID(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: `{"category":"fresh"}`})
	store := newFakeCacheStore()
	c := newMetadataExtractor(common.MetadataFieldDef{Key: "category", Type: "string"})
	in := extractorInputs{llmID: "model-1", cache: store}

	// Seed via the setter so the test pins the getter/setter agreement rather
	// than duplicating the key format.
	schemaJSON, err := json.Marshal(common.Turn2JSONSchema(c.Param.Metadata.Metadata))
	if err != nil {
		t.Fatalf("marshal metadata schema: %v", err)
	}
	setMetadataLLMCache(cacheTaskCtx("task-9"), in, string(schemaJSON), "chunk-1", map[string]any{"category": "cached"})

	ck := map[string]any{"id": "chunk-1", "text": "a body that no longer participates in the key"}
	if err := c.runEnableMetadata(t.Context(), nil, in, ck, "a body that no longer participates in the key"); err != nil {
		t.Fatalf("runEnableMetadata: %v", err)
	}
	meta, _ := ck["metadata"].(map[string]any)
	if meta["category"] != "cached" {
		t.Errorf("metadata = %v, want category=cached from the chunk-id-keyed cache", meta)
	}
	if n := stub.Calls(); n != 0 {
		t.Errorf("LLM calls = %d, want 0 on a cache hit", n)
	}
}

// TestMetadataLLMCache_DistinctChunksDoNotShare asserts the metadata cache
// discriminates chunks.
func TestMetadataLLMCache_DistinctChunksDoNotShare(t *testing.T) {
	store := newFakeCacheStore()
	in := extractorInputs{llmID: "model-1", cache: store}
	ctx := cacheTaskCtx("task-9")
	setMetadataLLMCache(ctx, in, `{"category":"string"}`, "chunk-1", map[string]any{"category": "one"})

	if _, hit := getMetadataLLMCache(ctx, in, `{"category":"string"}`, "chunk-2"); hit {
		t.Error("chunk-2 must not read chunk-1's metadata entry")
	}
	got, hit := getMetadataLLMCache(ctx, in, `{"category":"string"}`, "chunk-1")
	if !hit || got["category"] != "one" {
		t.Errorf("getMetadataLLMCache(chunk-1) = %v (hit=%v), want category=one", got, hit)
	}
}

// TestMetadataLLMCache_SchemaChangeInvalidates asserts a metadata schema edit
// does not silently reuse results extracted against the old field set.
func TestMetadataLLMCache_SchemaChangeInvalidates(t *testing.T) {
	store := newFakeCacheStore()
	in := extractorInputs{llmID: "model-1", cache: store}
	ctx := cacheTaskCtx("task-9")
	setMetadataLLMCache(ctx, in, `{"category":"string"}`, "chunk-1", map[string]any{"category": "one"})
	if _, hit := getMetadataLLMCache(ctx, in, `{"region":"string"}`, "chunk-1"); hit {
		t.Error("a different metadata schema must not reuse the previous entry")
	}
}

// TestMetadataLLMCache_NoChunkIDBypassesCache asserts the metadata path also
// skips the cache when the chunk has no stable id.
func TestMetadataLLMCache_NoChunkIDBypassesCache(t *testing.T) {
	store := newFakeCacheStore()
	in := extractorInputs{llmID: "model-1", cache: store}
	ctx := cacheTaskCtx("task-9")
	setMetadataLLMCache(ctx, in, `{"category":"string"}`, "", map[string]any{"category": "one"})
	if len(store.kv) != 0 {
		t.Errorf("cache writes = %v, want none without a chunk id", store.kv)
	}
	if _, hit := getMetadataLLMCache(ctx, in, `{"category":"string"}`, ""); hit {
		t.Error("getMetadataLLMCache with no chunk id must miss")
	}
}

// TestTaggerCacheKey_UsesChunkIDNotText asserts the tagger cache key switched
// to the chunk id while still discriminating the few-shot examples (whose
// content is not part of any chunk id).
func TestTaggerCacheKey_UsesChunkIDNotText(t *testing.T) {
	allTags := map[string]float64{"a": 1, "b": 2}
	ex := []schema.TaggedChunk{{Content: "example one", Tags: []string{"a"}}}

	base := taggerCacheKey("llm-1", "chunk-1", allTags, ex, 3)
	if base == "" {
		t.Fatal("taggerCacheKey returned empty for a valid chunk id")
	}
	if got := taggerCacheKey("llm-1", "chunk-1", allTags, ex, 3); got != base {
		t.Errorf("taggerCacheKey is not deterministic: %q vs %q", got, base)
	}
	if got := taggerCacheKey("llm-1", "chunk-2", allTags, ex, 3); got == base {
		t.Error("a different chunk id must not collide")
	}
	if got := taggerCacheKey("llm-2", "chunk-1", allTags, ex, 3); got == base {
		t.Error("a different model must not collide")
	}
	if got := taggerCacheKey("llm-1", "chunk-1", map[string]float64{"a": 1}, ex, 3); got == base {
		t.Error("a different tag set must not collide")
	}
	if got := taggerCacheKey("llm-1", "chunk-1", allTags, []schema.TaggedChunk{{Content: "example two", Tags: []string{"a"}}}, 3); got == base {
		t.Error("different few-shot examples must not collide")
	}
	if got := taggerCacheKey("llm-1", "chunk-1", allTags, ex, 5); got == base {
		t.Error("a different topN must not collide")
	}
	if got := taggerCacheKey("llm-1", "", allTags, ex, 3); got != "" {
		t.Errorf("taggerCacheKey(no chunk id) = %q, want \"\"", got)
	}
}

// TestTaggerLLMCache_RoundTripsUnderSharedTTL asserts the tagger cache uses the
// same store/TTL/manifest contract as the other per-chunk caches.
func TestTaggerLLMCache_RoundTripsUnderSharedTTL(t *testing.T) {
	store := newFakeCacheStore()
	ctx := cacheTaskCtx("task-9")
	allTags := map[string]float64{"a": 1}
	want := map[string]int{"a": 4}

	setTaggerLLMCache(ctx, store, "llm-1", "chunk-1", allTags, nil, 3, want)

	key := taggerCacheKey("llm-1", "chunk-1", allTags, nil, 3)
	if got := store.ttl[key]; got != chunkcache.TTL {
		t.Errorf("tagger cache TTL = %v, want %v", got, chunkcache.TTL)
	}
	if members := store.members("kc:manifest:task-9"); len(members) != 1 || members[0] != key {
		t.Errorf("manifest = %v, want [%s]", members, key)
	}
	got := getTaggerLLMCache(ctx, store, "llm-1", "chunk-1", allTags, nil, 3)
	if got == nil || got["a"] != 4 {
		t.Errorf("getTaggerLLMCache = %v, want %v", got, want)
	}
	if other := getTaggerLLMCache(ctx, store, "llm-1", "chunk-2", allTags, nil, 3); other != nil {
		t.Errorf("chunk-2 read chunk-1's entry: %v", other)
	}
}
