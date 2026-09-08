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

package chunkcache

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/globals"
)

// fakeStore is an in-memory Store double: enough Redis surface for the
// manifest bookkeeping, with call counters so tests can assert the exact
// command sequence (a real Redis is an integration-tier dependency).
type fakeStore struct {
	kv      map[string]string
	sets    map[string]map[string]bool
	ttl     map[string]time.Duration
	deleted []string
	failSet bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		kv:   map[string]string{},
		sets: map[string]map[string]bool{},
		ttl:  map[string]time.Duration{},
	}
}

func (f *fakeStore) Get(_ context.Context, key string) (string, error) {
	v, ok := f.kv[key]
	if !ok {
		return "", errors.New("redis: nil")
	}
	return v, nil
}

func (f *fakeStore) Set(_ context.Context, key, value string, exp time.Duration) bool {
	if f.failSet {
		return false
	}
	f.kv[key] = value
	f.ttl[key] = exp
	return true
}

func (f *fakeStore) SAdd(_ context.Context, key, member string) bool {
	if f.sets[key] == nil {
		f.sets[key] = map[string]bool{}
	}
	f.sets[key][member] = true
	return true
}

func (f *fakeStore) SMembers(_ context.Context, key string) ([]string, error) {
	out := make([]string, 0, len(f.sets[key]))
	for m := range f.sets[key] {
		out = append(out, m)
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeStore) Delete(_ context.Context, key string) bool {
	f.deleted = append(f.deleted, key)
	delete(f.kv, key)
	delete(f.sets, key)
	return true
}

func (f *fakeStore) Expire(_ context.Context, key string, exp time.Duration) bool {
	f.ttl[key] = exp
	return true
}

// taskCtx returns a ctx whose CanvasState carries taskID, mirroring what the
// pipeline attaches for a real run.
func taskCtx(taskID string) context.Context {
	st := runtime.NewCanvasState("", "")
	ctx := runtime.WithState(context.Background(), st)
	globals.SetTaskID(ctx, taskID)
	return ctx
}

// TestTTL_IsSevenDays pins the shared cache lifetime. Resume must be able to
// reuse per-chunk results for as long as a checkpoint can live, so this value
// and the pipeline checkpoint TTL are deliberately equal.
func TestTTL_IsSevenDays(t *testing.T) {
	if TTL != 7*24*time.Hour {
		t.Errorf("TTL = %v, want 168h", TTL)
	}
}

// TestSet_WritesValueAndRegistersManifest asserts a cache write both stores the
// value under the shared TTL and records the key on the task manifest, so the
// persist stage can reclaim exactly the keys this task produced.
func TestSet_WritesValueAndRegistersManifest(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	Set(ctx, f, "kc:extractor:keywords:abc", "result")

	if got := f.kv["kc:extractor:keywords:abc"]; got != "result" {
		t.Errorf("cached value = %q, want %q", got, "result")
	}
	if got := f.ttl["kc:extractor:keywords:abc"]; got != TTL {
		t.Errorf("cached TTL = %v, want %v", got, TTL)
	}
	members, _ := f.SMembers(ctx, "kc:manifest:task-1")
	if len(members) != 1 || members[0] != "kc:extractor:keywords:abc" {
		t.Errorf("manifest members = %v, want [kc:extractor:keywords:abc]", members)
	}
	if got := f.ttl["kc:manifest:task-1"]; got != TTL {
		t.Errorf("manifest TTL = %v, want %v (must not outlive its entries)", got, TTL)
	}
}

// TestSet_SkipsManifestWithoutTaskScope asserts a run with no task id (canvas
// debug preview, headless test) still caches but records nothing: there is no
// task whose completion could reclaim the keys, so TTL is the only reclaim
// path and a manifest would leak.
func TestSet_SkipsManifestWithoutTaskScope(t *testing.T) {
	f := newFakeStore()
	Set(context.Background(), f, "kc:extractor:keywords:abc", "result")

	if got := f.kv["kc:extractor:keywords:abc"]; got != "result" {
		t.Errorf("cached value = %q, want it cached even without task scope", got)
	}
	if len(f.sets) != 0 {
		t.Errorf("manifest sets = %v, want none without a task id", f.sets)
	}
}

// TestSet_NoStoreIsNoop asserts a Redis-less deployment degrades silently
// instead of panicking: callers pass the resolved client straight through.
func TestSet_NoStoreIsNoop(t *testing.T) {
	Set(taskCtx("task-1"), nil, "kc:extractor:keywords:abc", "result")
	if got, ok := Get(taskCtx("task-1"), nil, "kc:extractor:keywords:abc"); ok || got != "" {
		t.Errorf("Get(nil store) = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestSet_SkipsEmptyKeyOrValue asserts we never cache under an empty key (the
// caller could not derive a chunk id) and never cache an empty payload (which
// Get cannot distinguish from a miss).
func TestSet_SkipsEmptyKeyOrValue(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	Set(ctx, f, "", "result")
	Set(ctx, f, "kc:extractor:keywords:abc", "")
	if len(f.kv) != 0 {
		t.Errorf("kv = %v, want no writes", f.kv)
	}
	if len(f.sets) != 0 {
		t.Errorf("manifest sets = %v, want none", f.sets)
	}
}

// TestSet_SkipsManifestWhenValueWriteFails asserts the manifest never lists a
// key whose value write failed — otherwise PurgeTask would issue deletes for
// keys that do not exist.
func TestSet_SkipsManifestWhenValueWriteFails(t *testing.T) {
	f := newFakeStore()
	f.failSet = true
	Set(taskCtx("task-1"), f, "kc:extractor:keywords:abc", "result")
	if len(f.sets) != 0 {
		t.Errorf("manifest sets = %v, want none when the value write failed", f.sets)
	}
}

// TestGet_HitAndMiss asserts the miss signal is a boolean rather than an empty
// string, so a caller can tell "not cached" from "cached empty".
func TestGet_HitAndMiss(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	Set(ctx, f, "k", "v")
	if got, ok := Get(ctx, f, "k"); !ok || got != "v" {
		t.Errorf("Get(hit) = (%q, %v), want (\"v\", true)", got, ok)
	}
	if got, ok := Get(ctx, f, "absent"); ok || got != "" {
		t.Errorf("Get(miss) = (%q, %v), want (\"\", false)", got, ok)
	}
	if got, ok := Get(ctx, f, ""); ok || got != "" {
		t.Errorf("Get(empty key) = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestPurgeTask_DeletesEveryEntryThenManifest asserts persist-time reclaim
// removes all recorded entries and finally the manifest itself, leaving no
// key behind that would otherwise wait out the 7-day TTL.
func TestPurgeTask_DeletesEveryEntryThenManifest(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	Set(ctx, f, "kc:extractor:keywords:a", "1")
	Set(ctx, f, "kc:meta:b", "2")
	Set(ctx, f, "kc:emb:c", "3")

	PurgeTask(context.Background(), f, "task-1")

	want := []string{"kc:emb:c", "kc:extractor:keywords:a", "kc:meta:b", "kc:manifest:task-1"}
	if len(f.deleted) != len(want) {
		t.Fatalf("deleted = %v, want %v", f.deleted, want)
	}
	// Entry deletes may arrive in any order, but the manifest must be last:
	// dropping it early would orphan the remaining entries if we crash midway.
	if f.deleted[len(f.deleted)-1] != "kc:manifest:task-1" {
		t.Errorf("last delete = %q, want the manifest key", f.deleted[len(f.deleted)-1])
	}
	entries := append([]string(nil), f.deleted[:len(f.deleted)-1]...)
	sort.Strings(entries)
	for i, w := range want[:len(want)-1] {
		if entries[i] != w {
			t.Errorf("entry deletes = %v, want %v", entries, want[:len(want)-1])
			break
		}
	}
	if len(f.kv) != 0 {
		t.Errorf("kv after purge = %v, want empty", f.kv)
	}
}

// TestPurgeTask_NoopWithoutTaskOrStore asserts the reclaim path is safe to
// call unconditionally from the persist stage.
func TestPurgeTask_NoopWithoutTaskOrStore(t *testing.T) {
	f := newFakeStore()
	PurgeTask(context.Background(), f, "")
	if len(f.deleted) != 0 {
		t.Errorf("deleted = %v, want none for an empty task id", f.deleted)
	}
	PurgeTask(context.Background(), nil, "task-1")
}

// TestKey_DiscriminatesEveryComponent asserts the key builder namespaces by
// kind and mixes every identity input, so a chat-model swap, a prompt edit or
// a different chunk can never read another entry's value.
func TestKey_DiscriminatesEveryComponent(t *testing.T) {
	base := Key("extractor:keywords", "gpt-4@openai", "chunk-1", "prompt-v1")
	cases := []struct {
		name string
		key  string
	}{
		{"different kind", Key("extractor:questions", "gpt-4@openai", "chunk-1", "prompt-v1")},
		{"different model", Key("extractor:keywords", "gpt-5@openai", "chunk-1", "prompt-v1")},
		{"different chunk", Key("extractor:keywords", "gpt-4@openai", "chunk-2", "prompt-v1")},
		{"different config", Key("extractor:keywords", "gpt-4@openai", "chunk-1", "prompt-v2")},
	}
	for _, c := range cases {
		if c.key == base {
			t.Errorf("%s: key collides with base (%s)", c.name, base)
		}
	}
	if same := Key("extractor:keywords", "gpt-4@openai", "chunk-1", "prompt-v1"); same != base {
		t.Errorf("Key is not deterministic: %q vs %q", same, base)
	}
}

// TestKey_IsPrefixedByKind asserts keys stay greppable/scannable per kind in a
// live Redis, and that they share the kc: namespace the manifest uses.
func TestKey_IsPrefixedByKind(t *testing.T) {
	k := Key("emb", "bge-m3@builtin", "chunk-1")
	if want := "kc:emb:"; len(k) <= len(want) || k[:len(want)] != want {
		t.Errorf("Key = %q, want prefix %q", k, want)
	}
}

// TestKey_EmptyChunkIDYieldsNoKey asserts a chunk with no stable id produces
// no key at all, so callers skip the cache instead of sharing one bucket for
// every unidentified chunk.
func TestKey_EmptyChunkIDYieldsNoKey(t *testing.T) {
	if k := Key("emb", "bge-m3@builtin", ""); k != "" {
		t.Errorf("Key(empty chunk id) = %q, want \"\"", k)
	}
}

// TestKey_FieldsAreUnambiguous asserts the identity inputs are separated when
// hashed: concatenating them differently must not collide.
func TestKey_FieldsAreUnambiguous(t *testing.T) {
	a := Key("extractor:keywords", "model", "chunk", "ab", "c")
	b := Key("extractor:keywords", "model", "chunk", "a", "bc")
	if a == b {
		t.Errorf("ambiguous field packing: %q == %q", a, b)
	}
}
