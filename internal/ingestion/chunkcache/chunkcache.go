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

// Package chunkcache owns the per-chunk result cache shared by the ingestion
// components that call an expensive external model (Extractor → chat LLM,
// Tokenizer → embedding model).
//
// Why it exists: the pipeline checkpoints only after the Parser, so a resumed
// task re-executes every stage after it. That is affordable only if the
// expensive per-chunk work is memoised, which is what this cache does — a
// resumed run re-walks the same chunks, hits the cache, and issues no model
// calls.
//
// Entry lifetime has two reclaim paths. The normal one is PurgeTask, called
// once the task's chunks are durably indexed: from that point the cache can
// never be re-read, so it is dropped immediately instead of occupying the
// store. TTL is the backstop for tasks that never reach persist (failed,
// cancelled, abandoned mid-resume) and for runs with no task scope.
package chunkcache

import (
	"context"
	"fmt"
	"time"

	"github.com/cespare/xxhash/v2"

	"ragflow/internal/engine/redis"
	"ragflow/internal/ingestion/component/globals"
)

// TTL bounds how long a cached per-chunk result stays useful. It matches the
// pipeline checkpoint TTL on purpose: a task that can still be resumed must
// still find its cache, and one that can no longer be resumed has no reader.
const TTL = 7 * 24 * time.Hour

// keyNamespace prefixes every key this package writes, including the
// manifests, so a live store can be inspected (or bulk-scanned) per concern.
const keyNamespace = "kc"

// Store is the slice of the Redis client this package needs.
// *redis.Client satisfies it; tests supply an in-memory double.
type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, exp time.Duration) bool
	SAdd(ctx context.Context, key, member string) bool
	SMembers(ctx context.Context, key string) ([]string, error)
	Delete(ctx context.Context, key string) bool
	Expire(ctx context.Context, key string, exp time.Duration) bool
}

// Client resolves the process-wide Redis client as a Store, or nil when Redis
// is not configured. Always resolve through this function: passing redis.Get()
// directly into a Store parameter would wrap a typed nil pointer in a non-nil
// interface, defeating the nil guards below.
func Client() Store {
	if c := redis.Get(); c != nil {
		return c
	}
	return nil
}

// Key builds the cache key for one (kind, model, chunk) triple, mixing any
// extra config that changes the result (a prompt, a JSON schema, a top-N).
//
// modelID is the identity of the model that produced the cached value:
// for the "emb" kind that is the dataset's embedding model id (kb.embd_id);
// for the "extractor:*"/"meta"/"tagger" kinds it is the chat model id (llm_id,
// or — when that is empty — the resolved tenant-default chat model). Callers
// must resolve the model id to the value that actually generated the result
// before calling Key, never a raw/possibly-empty override string: keying on an
// empty modelID would collapse every model onto one bucket and serve a result
// produced by a model the tenant no longer uses. The tokenizer keys on embd_id;
// the extractor resolves its chat target (see extractorCacheModelID in the
// component package) for the same reason.
//
// The chunk id — not the chunk text — is the identity input: it is the stable
// per-chunk handle the chunker assigns (component.ChunkID), and it already
// derives from the text, so keying on it keeps entries short and keeps the
// pipeline and the persist stage agreeing on what "the same chunk" means.
//
// Either an empty chunk id or an empty modelID yields an empty key: in both
// cases the caller must skip the cache rather than let every unidentified
// chunk (or every model-less entry) share one bucket.
func Key(kind, modelID, chunkID string, config ...string) string {
	if chunkID == "" || modelID == "" {
		return ""
	}
	h := xxhash.New()
	// Length-prefix every field so no two different field splits can hash to
	// the same digest (…"ab","c" vs …"a","bc").
	writeField := func(s string) {
		_, _ = fmt.Fprintf(h, "%d:%s", len(s), s)
	}
	writeField(kind)
	writeField(modelID)
	writeField(chunkID)
	for _, c := range config {
		writeField(c)
	}
	return fmt.Sprintf("%s:%s:%x", keyNamespace, kind, h.Sum64())
}

// manifestKey is the set listing every cache key produced for one task.
func manifestKey(taskID string) string {
	return fmt.Sprintf("%s:manifest:%s", keyNamespace, taskID)
}

// Get returns the cached value for key. The boolean distinguishes a miss from
// a cached empty string; Set refuses empty values, so a hit is always
// meaningful. Degrades to a miss when the store is unavailable.
func Get(ctx context.Context, s Store, key string) (string, bool) {
	if s == nil || key == "" {
		return "", false
	}
	v, err := s.Get(ctx, key)
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// Set caches value under key for TTL and records key on the current task's
// manifest so PurgeTask can reclaim it at persist time. Best-effort
// throughout: a store outage costs a recomputation, never a failed run.
//
// The manifest entry is only recorded once the value write succeeded, so the
// manifest never lists a key that does not exist. When the run carries no task
// id (canvas debug preview, headless test) the value is still cached but
// nothing is recorded: no persist stage will ever reclaim it, so TTL is the
// only reclaim path and a manifest would simply leak.
func Set(ctx context.Context, s Store, key, value string) {
	if s == nil || key == "" || value == "" {
		return
	}
	if !s.Set(ctx, key, value, TTL) {
		return
	}
	taskID := globals.TaskID(ctx)
	if taskID == "" {
		return
	}
	mk := manifestKey(taskID)
	if s.SAdd(ctx, mk, key) {
		// Bound the manifest's own lifetime: it must not outlive the entries
		// it lists, or an abandoned task would leak one set per task forever.
		// If the TTL cannot be applied, drop the manifest immediately rather
		// than leave a TTL-less set that leaks one empty key per task.
		if !s.Expire(ctx, mk, TTL) {
			s.Delete(ctx, mk)
		}
	}
}

// PurgeTask drops every cache entry recorded for taskID, then the manifest
// itself. Call it once the task's chunks are durably persisted: from that point
// nothing can read the entries back, so holding them for the remaining TTL is
// pure waste.
//
// The manifest is deleted last: losing it first would orphan the entries it
// still lists, leaving them to expire. Best-effort — a failed reclaim only
// means the entries wait out their TTL.
func PurgeTask(ctx context.Context, s Store, taskID string) {
	if s == nil || taskID == "" {
		return
	}
	mk := manifestKey(taskID)
	keys, err := s.SMembers(ctx, mk)
	if err != nil {
		return
	}
	for _, k := range keys {
		s.Delete(ctx, k)
	}
	s.Delete(ctx, mk)
}
