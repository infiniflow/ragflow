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
	"sync"
	"testing"
	"time"

	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/schema"
)

// vecOf reads a float64 vector stored by SetExtraValue back out of the chunk.
func vecOf(t *testing.T, doc schema.ChunkDoc, key string) []float64 {
	t.Helper()
	raw, ok := doc.Extra[key]
	if !ok {
		t.Fatalf("missing extra %s", key)
	}
	var v []float64
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal %s: %v", key, err)
	}
	return v
}

// memCacheStore is an in-memory chunkcache.Store used to exercise the
// per-chunk embedding cache without Redis.
type memCacheStore struct {
	mu   sync.Mutex
	data map[string]string
	sets map[string]map[string]struct{}
}

func newMemCacheStore() *memCacheStore {
	return &memCacheStore{data: map[string]string{}, sets: map[string]map[string]struct{}{}}
}

func (m *memCacheStore) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key], nil
}

func (m *memCacheStore) Set(_ context.Context, key, value string, _ time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return true
}

func (m *memCacheStore) SAdd(_ context.Context, key, member string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sets[key] == nil {
		m.sets[key] = map[string]struct{}{}
	}
	m.sets[key][member] = struct{}{}
	return true
}

func (m *memCacheStore) SMembers(_ context.Context, key string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.sets[key]))
	for k := range m.sets[key] {
		out = append(out, k)
	}
	return out, nil
}

func (m *memCacheStore) Delete(_ context.Context, key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	delete(m.sets, key)
	return true
}

func (m *memCacheStore) Expire(_ context.Context, key string, _ time.Duration) bool {
	return true
}

func (m *memCacheStore) countEmbKeys() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k := range m.data {
		if len(k) >= len("kc:emb:") && k[:len("kc:emb:")] == "kc:emb:" {
			n++
		}
	}
	return n
}

// chunkWithID builds a ChunkDoc carrying a chunk id in Extra (so the cache
// key can be derived) plus the given body text.
func chunkWithID(id, text string) schema.ChunkDoc {
	doc, _ := schema.ChunkDocFromMap(map[string]any{"id": id, "text": text})
	return doc
}

func TestEmbedChunks_PerChunkCacheMissThenHit(t *testing.T) {
	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}

	store := newMemCacheStore()
	chunks := []schema.ChunkDoc{chunkWithID("chunk-1", "hello world")}

	model := "test-model@provider"

	// First pass: cache miss -> embedder is called once.
	out1, _, err := comp.embedChunks(context.Background(), "tenant", "kb", model, "", chunks, store)
	if err != nil {
		t.Fatalf("embedChunks (miss): %v", err)
	}
	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("expected 1 embedder call on miss, got %d", got)
	}
	vec1 := vecOf(t, out1[0], "q_4_vec")
	if store.countEmbKeys() != 1 {
		t.Fatalf("expected 1 emb cache key after miss, got %d", store.countEmbKeys())
	}

	// Second pass with the same chunk id: cache hit -> embedder NOT called again.
	out2, _, err := comp.embedChunks(context.Background(), "tenant", "kb", model, "", chunks, store)
	if err != nil {
		t.Fatalf("embedChunks (hit): %v", err)
	}
	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("expected embedder to be skipped on cache hit, but calls=%d", got)
	}
	vec2 := vecOf(t, out2[0], "q_4_vec")
	if len(vec1) != len(vec2) {
		t.Fatalf("cached vector differs from embedded vector: %v vs %v", vec1, vec2)
	}
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Fatalf("cached vector differs from embedded vector: %v vs %v", vec1, vec2)
		}
	}
}

func TestEmbedChunks_CacheKeyScopedByChunkAndModel(t *testing.T) {
	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}

	store := newMemCacheStore()
	model := "test-model@provider"

	// Two distinct chunks -> two embedder calls, two cache keys.
	a := []schema.ChunkDoc{chunkWithID("chunk-a", "alpha"), chunkWithID("chunk-b", "beta")}
	if _, _, err := comp.embedChunks(context.Background(), "tenant", "kb", model, "", a, store); err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("expected 1 batch call for 2 chunks, got %d", got)
	}
	if n := store.countEmbKeys(); n != 2 {
		t.Fatalf("expected 2 emb cache keys for 2 distinct chunks, got %d", n)
	}

	// Re-embedding chunk-a with a DIFFERENT model must bypass the cache.
	b := []schema.ChunkDoc{chunkWithID("chunk-a", "alpha")}
	if _, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "other-model@provider", "", b, store); err != nil {
		t.Fatalf("embedChunks (other model): %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Fatalf("expected a new embedder call for a different model, got calls=%d", got)
	}
	if n := store.countEmbKeys(); n != 3 {
		t.Fatalf("expected 3 emb cache keys after distinct-model embed, got %d", n)
	}
}

func TestEmbedChunks_MissingChunkIDSkipsCache(t *testing.T) {
	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}

	store := newMemCacheStore()
	// Chunk without an id: cannot be cached, always embedded.
	noID, _ := schema.ChunkDocFromMap(map[string]any{"text": "no id here"})

	first, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "test-model@provider", "", []schema.ChunkDoc{noID}, store)
	if err != nil {
		t.Fatalf("embedChunks (no id): %v", err)
	}
	if stub.calls.Load() != 1 {
		t.Fatalf("expected embedder call for uncacheable chunk")
	}
	vecOf(t, first[0], "q_4_vec")
	if n := store.countEmbKeys(); n != 0 {
		t.Fatalf("expected no emb cache key for id-less chunk, got %d", n)
	}

	// Second pass: still no id -> still embedded (calls increment).
	comp.embedChunks(context.Background(), "tenant", "kb", "test-model@provider", "", []schema.ChunkDoc{noID}, store)
	if stub.calls.Load() != 2 {
		t.Fatalf("expected a second embedder call for id-less chunk (cache not used), got %d", stub.calls.Load())
	}
}

var _ chunkcache.Store = (*memCacheStore)(nil)
