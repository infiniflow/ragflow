// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mediacache

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/parser/media"
)

type fakeStore struct {
	kv          map[string]string
	sets        map[string]map[string]bool
	ttl         map[string]time.Duration
	getErr      error
	failSet     bool
	expireFails bool
	setCalls    int
	addCalls    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		kv:   map[string]string{},
		sets: map[string]map[string]bool{},
		ttl:  map[string]time.Duration{},
	}
}

func (f *fakeStore) Get(_ context.Context, key string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.kv[key], nil
}

func (f *fakeStore) Set(_ context.Context, key, value string, exp time.Duration) bool {
	f.setCalls++
	if f.failSet {
		return false
	}
	f.kv[key] = value
	f.ttl[key] = exp
	return true
}

func (f *fakeStore) SAdd(_ context.Context, key, member string) bool {
	f.addCalls++
	if f.sets[key] == nil {
		f.sets[key] = map[string]bool{}
	}
	f.sets[key][member] = true
	return true
}

func (f *fakeStore) SMembers(_ context.Context, key string) ([]string, error) {
	out := make([]string, 0, len(f.sets[key]))
	for member := range f.sets[key] {
		out = append(out, member)
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeStore) Delete(_ context.Context, key string) bool {
	delete(f.kv, key)
	delete(f.sets, key)
	return true
}

func (f *fakeStore) Expire(_ context.Context, key string, exp time.Duration) bool {
	if f.expireFails {
		return false
	}
	f.ttl[key] = exp
	return true
}

func taskContext(taskID string) context.Context {
	state := runtime.NewCanvasState("", "")
	ctx := runtime.WithState(context.Background(), state)
	globals.SetTaskID(ctx, taskID)
	return ctx
}

func TestPutAndGet_RoundTripAcrossRepeatedReads(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	ctx := taskContext("task-1")
	data := []byte("hello")

	ref, err := store.Put(ctx, "html", "image/png", data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ref.Format != "html" || ref.Kind != entity.MediaKindKV || ref.Scope != "task-1" || ref.MIME != "image/png" || ref.Size != int64(len(data)) {
		t.Fatalf("ref = %+v", ref)
	}
	if len(backend.kv) != 1 {
		t.Fatalf("cached keys = %v, want one", backend.kv)
	}
	var key string
	for k := range backend.kv {
		key = k
		if backend.ttl[k] != chunkcache.TTL {
			t.Errorf("cached TTL = %v, want %v", backend.ttl[k], chunkcache.TTL)
		}
	}
	members, _ := backend.SMembers(ctx, "kc:manifest:task-1")
	if len(members) != 1 || members[0] != key {
		t.Fatalf("manifest = %v, want [%s]", members, key)
	}

	for attempt := 0; attempt < 2; attempt++ {
		got, err := store.Get(ctx, ref)
		if err != nil {
			t.Fatalf("Get #%d: %v", attempt+1, err)
		}
		if string(got) != string(data) {
			t.Fatalf("Get #%d = %q, want %q", attempt+1, got, data)
		}
	}
}

func TestGet_MissingIsNotTreatedAsSuccess(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	ctx := taskContext("task-1")
	ref, err := store.Put(ctx, "html", "image/png", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	for key := range backend.kv {
		delete(backend.kv, key)
	}

	if _, err := store.Get(ctx, ref); !errors.Is(err, media.ErrMediaNotFound) {
		t.Fatalf("Get missing error = %v, want ErrMediaNotFound", err)
	}
}

func TestGet_RejectsCorruptPayloadAndWrongScope(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	ctx := taskContext("task-1")
	ref, err := store.Put(ctx, "html", "image/png", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	for key := range backend.kv {
		backend.kv[key] = "HELLO"
	}
	if _, err := store.Get(ctx, ref); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("Get corrupt error = %v, want digest mismatch", err)
	}

	wrongScope := ref
	wrongScope.Scope = "task-2"
	if _, err := store.Get(ctx, wrongScope); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("Get wrong scope error = %v, want scope rejection", err)
	}
	if _, err := store.Get(taskContext("task-2"), ref); err == nil || !strings.Contains(err.Error(), "task") {
		t.Fatalf("Get wrong task context error = %v, want task rejection", err)
	}
}

func TestGet_PropagatesBackendFailure(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	wantErr := errors.New("kvrocks unavailable")
	backend.getErr = wantErr
	ref := entity.MediaRef{
		Format: "html",
		Kind:   entity.MediaKindKV,
		Scope:  "task-1",
		SHA256: strings.Repeat("a", 64),
		MIME:   "image/png",
		Size:   5,
	}
	if _, err := store.Get(taskContext("task-1"), ref); !errors.Is(err, wantErr) {
		t.Fatalf("Get error = %v, want %v", err, wantErr)
	}
}

func TestPut_FailsRatherThanSkippingCache(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		backend := newFakeStore()
		backend.failSet = true
		if _, err := New(backend, "task-1").Put(taskContext("task-1"), "html", "image/png", []byte("hello")); err == nil {
			t.Fatal("Put succeeded when the backend rejected the write")
		}
	})
	t.Run("manifest", func(t *testing.T) {
		backend := newFakeStore()
		backend.expireFails = true
		if _, err := New(backend, "task-1").Put(taskContext("task-1"), "html", "image/png", []byte("hello")); err == nil {
			t.Fatal("Put succeeded when the manifest could not be bounded by TTL")
		}
		if len(backend.kv) != 0 {
			t.Fatalf("untracked media entries = %v, want rollback", backend.kv)
		}
	})
}

func TestPut_RejectsMissingStoreScopeAndOversizedImage(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	if _, err := New(backend, "").Put(ctx, "html", "image/png", []byte("hello")); err == nil {
		t.Fatal("Put accepted an empty scope")
	}
	if _, err := New(nil, "task-1").Put(ctx, "html", "image/png", []byte("hello")); err == nil {
		t.Fatal("Put accepted an unavailable cache")
	}
	data := make([]byte, media.MaxImagePayloadBytes+1)
	if _, err := New(backend, "task-1").Put(ctx, "html", "image/png", data); err == nil {
		t.Fatal("Put accepted an oversized image")
	}
}

func TestPut_EnforcesRunByteBudgetAndDeduplicatesContent(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	store.maxBytes = 5
	ctx := taskContext("task-1")

	if _, err := store.Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	// Repeating the same content reuses the same key and consumes no extra
	// budget, as happens when VLM and several chunks reference one image.
	if _, err := store.Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatalf("duplicate Put: %v", err)
	}
	if backend.setCalls != 1 || backend.addCalls != 1 {
		t.Fatalf("backend writes after duplicate Put = Set %d / SAdd %d, want 1/1", backend.setCalls, backend.addCalls)
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("world")); err == nil || !strings.Contains(err.Error(), "run cache limit exceeded") {
		t.Fatalf("over-budget Put error = %v", err)
	}
	if len(backend.kv) != 1 {
		t.Fatalf("cached keys = %d, want only the first image", len(backend.kv))
	}
	if store.used != 5 {
		t.Fatalf("used bytes = %d, want 5", store.used)
	}
}

func TestPut_EnforcesRunImageCountBudget(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	store.maxItems = 1
	ctx := taskContext("task-1")

	if _, err := store.Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("world")); err == nil || !strings.Contains(err.Error(), "run image limit exceeded") {
		t.Fatalf("over-count Put error = %v", err)
	}
}

func TestPurgeTask_RemovesMediaEntriesThroughSharedManifest(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	ctx := taskContext("task-1")
	if _, err := store.Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := chunkcache.PurgeTask(ctx, backend, "task-1"); err != nil {
		t.Fatal(err)
	}
	if len(backend.kv) != 0 {
		t.Fatalf("cache after purge = %v, want empty", backend.kv)
	}
	if len(backend.sets) != 0 {
		t.Fatalf("manifest after purge = %v, want empty", backend.sets)
	}
}
