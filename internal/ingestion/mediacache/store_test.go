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
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
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
	mediaPuts   int
	deleteFails map[string]bool
	quotaBytes  map[string]int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		kv:         map[string]string{},
		sets:       map[string]map[string]bool{},
		ttl:        map[string]time.Duration{},
		quotaBytes: map[string]int64{},
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

func (f *fakeStore) MediaPut(
	_ context.Context,
	mediaKey, digestKey, stateKey, digest, value string,
	manifestKey, manifestMember string,
	maxImages int,
	maxBytes int64,
	ttl time.Duration,
) (bool, bool, error) {
	f.mediaPuts++
	if f.failSet {
		return false, false, errors.New("fake media write failure")
	}
	if f.expireFails {
		return false, false, errors.New("fake manifest expire failure")
	}
	if f.sets[digestKey][digest] {
		if f.kv[mediaKey] == "" {
			f.setCalls++
			f.kv[mediaKey] = value
			f.ttl[mediaKey] = ttl
			f.ttl[digestKey] = ttl
			f.ttl[stateKey] = ttl
			if manifestKey != "" {
				f.addCalls++
				if f.sets[manifestKey] == nil {
					f.sets[manifestKey] = map[string]bool{}
				}
				f.sets[manifestKey][manifestMember] = true
				f.ttl[manifestKey] = ttl
			}
		}
		return true, false, nil
	}
	if maxImages > 0 && len(f.sets[digestKey]) >= maxImages {
		return false, false, nil
	}
	if maxBytes > 0 && int64(len(value)) > maxBytes-f.quotaBytes[stateKey] {
		return false, false, nil
	}

	f.setCalls++
	f.kv[mediaKey] = value
	f.ttl[mediaKey] = ttl
	if manifestKey != "" {
		f.addCalls++
		if f.sets[manifestKey] == nil {
			f.sets[manifestKey] = map[string]bool{}
		}
		f.sets[manifestKey][manifestMember] = true
	}
	if f.sets[digestKey] == nil {
		f.sets[digestKey] = map[string]bool{}
	}
	f.sets[digestKey][digest] = true
	f.quotaBytes[stateKey] += int64(len(value))
	f.ttl[digestKey] = ttl
	f.ttl[stateKey] = ttl
	if manifestKey != "" {
		f.ttl[manifestKey] = ttl
	}
	return true, true, nil
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
	if f.deleteFails[key] {
		return false
	}
	delete(f.kv, key)
	delete(f.sets, key)
	delete(f.quotaBytes, key)
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
		if backend.ttl[k] != TTL {
			t.Errorf("cached TTL = %v, want %v", backend.ttl[k], TTL)
		}
	}
	members, _ := backend.SMembers(ctx, manifestKey("task-1"))
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
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "5")
	data := make([]byte, 6)
	if _, err := New(backend, "task-1").Put(ctx, "html", "image/png", data); err == nil {
		t.Fatal("Put accepted an oversized image")
	}
}

func TestPurgeTask_RemovesMediaEntriesThroughMediaManifest(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	ctx := taskContext("task-1")
	if _, err := store.Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := PurgeTask(ctx, backend, "task-1"); err != nil {
		t.Fatal(err)
	}
	if len(backend.kv) != 0 {
		t.Fatalf("cache after purge = %v, want empty", backend.kv)
	}
	if len(backend.sets) != 0 {
		t.Fatalf("manifest after purge = %v, want empty", backend.sets)
	}
}

func TestTTLIsOneDay(t *testing.T) {
	if TTL != 24*time.Hour {
		t.Fatalf("TTL = %v, want 24h", TTL)
	}
}

func TestMediaManifestDoesNotTouchChunkManifest(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	backend.SAdd(ctx, "kc:manifest:task-1", "kc:model-result")
	backend.Set(ctx, "kc:model-result", "result", 7*24*time.Hour)
	if _, err := New(backend, "task-1").Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if backend.ttl[manifestKey("task-1")] != TTL {
		t.Fatal("media manifest must expire in one day")
	}
	if err := PurgeTask(ctx, backend, "task-1"); err != nil {
		t.Fatal(err)
	}
	if backend.kv["kc:model-result"] != "result" || !backend.sets["kc:manifest:task-1"]["kc:model-result"] {
		t.Fatal("media cleanup modified chunk cache")
	}
}

func TestPutAllowsMoreThan256DistinctImages(t *testing.T) {
	backend := newFakeStore()
	store := New(backend, "task-1")
	ctx := taskContext("task-1")
	for i := 0; i < 257; i++ {
		if _, err := store.Put(ctx, "html", "image/png", []byte(fmt.Sprintf("image-%d", i))); err != nil {
			t.Fatalf("image %d: %v", i, err)
		}
	}
}

func TestPurgeTaskRetriesAfterPartialDeleteFailure(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	ref, err := store.Put(ctx, "html", "image/png", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("second")); err != nil {
		t.Fatal(err)
	}
	key := store.key(ref.SHA256)
	backend.deleteFails = map[string]bool{key: true}
	if err := PurgeTask(ctx, backend, "task-1"); err == nil {
		t.Fatal("expected delete failure")
	}
	if len(backend.sets[manifestKey("task-1")]) != 2 {
		t.Fatal("manifest lost after failed purge")
	}
	delete(backend.deleteFails, key)
	if err := PurgeTask(ctx, backend, "task-1"); err != nil {
		t.Fatal(err)
	}
	if len(backend.kv) != 0 || len(backend.sets) != 0 {
		t.Fatal("retry did not clear media")
	}
}

func TestRetryRepopulatesMediaAfterCacheMiss(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	ref, err := New(backend, "task-1").Put(ctx, "html", "image/png", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	for key := range backend.kv {
		delete(backend.kv, key)
	}
	if _, err := New(backend, "task-1").Get(ctx, ref); !errors.Is(err, media.ErrMediaNotFound) {
		t.Fatalf("miss = %v", err)
	}
	retry := New(backend, "task-1")
	retryRef, err := retry.Put(ctx, "html", "image/png", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := retry.Get(ctx, retryRef)
	if err != nil || string(got) != "hello" {
		t.Fatalf("retry read = %q, %v", got, err)
	}
}

func TestPutDuplicateWritesOnceAndReturnsCallerRef(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	data := []byte("hello")
	ref, err := store.Put(ctx, "html", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	key := store.key(ref.SHA256)
	backend.ttl[key] = time.Second
	backend.ttl[manifestKey("task-1")] = time.Second

	duplicate, err := store.Put(ctx, "epub", "image/jpeg", data)
	if err != nil {
		t.Fatalf("duplicate Put: %v", err)
	}
	if duplicate.Format != "epub" || duplicate.MIME != "image/jpeg" || duplicate.SHA256 != ref.SHA256 || duplicate.Size != ref.Size {
		t.Fatalf("duplicate ref = %+v, want the current caller's format/mime on the same digest", duplicate)
	}
	if backend.mediaPuts != 2 || backend.setCalls != 1 || backend.addCalls != 1 {
		t.Fatalf("backend calls = %d atomic Put, %d Set, %d SAdd, want 2, 1, 1", backend.mediaPuts, backend.setCalls, backend.addCalls)
	}
	if backend.ttl[key] != time.Second || backend.ttl[manifestKey("task-1")] != time.Second {
		t.Fatal("duplicate Put refreshed TTLs; dedupe must not touch the backend")
	}
}

func TestPutDedupeDoesNotConsumeRunLimits(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "2")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "5")
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := store.Put(ctx, "html", "image/png", []byte("abc")); err != nil {
			t.Fatalf("duplicate Put #%d: %v", attempt+1, err)
		}
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("de")); err != nil {
		t.Fatalf("second distinct image: %v", err)
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("f")); !errors.Is(err, media.ErrMediaLimitExceeded) {
		t.Fatalf("limit error = %v, want ErrMediaLimitExceeded", err)
	}
}

func TestPutEnforcesRunImageCountLimit(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "2")
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	for _, image := range []string{"a", "b"} {
		if _, err := store.Put(ctx, "html", "image/png", []byte(image)); err != nil {
			t.Fatalf("image %q: %v", image, err)
		}
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("c")); !errors.Is(err, media.ErrMediaLimitExceeded) {
		t.Fatalf("limit error = %v, want ErrMediaLimitExceeded", err)
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("c")); !errors.Is(err, media.ErrMediaLimitExceeded) {
		t.Fatalf("repeated rejected error = %v, want ErrMediaLimitExceeded", err)
	}
	if len(backend.kv) != 2 {
		t.Fatalf("cached entries = %d, want the 2 admitted images", len(backend.kv))
	}
	members, _ := backend.SMembers(ctx, manifestKey("task-1"))
	if len(members) != 2 {
		t.Fatalf("manifest = %v, want 2 entries", members)
	}
}

func TestPutEnforcesRunLimitsAcrossStoreInstances(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "2")
	backend := newFakeStore()
	ctx := taskContext("task-1")
	first := New(backend, "task-1")
	second := New(backend, "task-1")

	if _, err := first.Put(ctx, "html", "image/png", []byte("a")); err != nil {
		t.Fatalf("first image: %v", err)
	}
	if _, err := second.Put(ctx, "html", "image/png", []byte("b")); err != nil {
		t.Fatalf("second image from another Store: %v", err)
	}
	if _, err := first.Put(ctx, "html", "image/png", []byte("c")); !errors.Is(err, media.ErrMediaLimitExceeded) {
		t.Fatalf("third image error = %v, want ErrMediaLimitExceeded", err)
	}
	if len(backend.kv) != 2 {
		t.Fatalf("cached entries = %d, want 2", len(backend.kv))
	}
}

func TestPutEnforcesRunBytesAcrossStoreInstances(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "5")
	backend := newFakeStore()
	ctx := taskContext("task-1")
	first := New(backend, "task-1")
	second := New(backend, "task-1")

	if _, err := first.Put(ctx, "html", "image/png", []byte("abc")); err != nil {
		t.Fatalf("first image: %v", err)
	}
	if _, err := second.Put(ctx, "html", "image/png", []byte("de")); err != nil {
		t.Fatalf("second image from another Store: %v", err)
	}
	if _, err := first.Put(ctx, "html", "image/png", []byte("f")); !errors.Is(err, media.ErrMediaLimitExceeded) {
		t.Fatalf("third image error = %v, want ErrMediaLimitExceeded", err)
	}
}

func TestPutEnforcesRunByteLimitAtExactBoundary(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "5")
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	for _, image := range []string{"abc", "de"} {
		if _, err := store.Put(ctx, "html", "image/png", []byte(image)); err != nil {
			t.Fatalf("image %q: %v", image, err)
		}
	}
	if _, err := store.Put(ctx, "html", "image/png", []byte("f")); !errors.Is(err, media.ErrMediaLimitExceeded) {
		t.Fatalf("limit error = %v, want ErrMediaLimitExceeded", err)
	}
	if len(backend.kv) != 2 {
		t.Fatalf("cached entries = %d, want the 2 admitted images", len(backend.kv))
	}
}

func TestPutRestoresLocallySeenDigestAfterBackendEviction(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	data := []byte("hello")
	ref, err := store.Put(ctx, "html", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	key := store.key(ref.SHA256)
	delete(backend.kv, key)

	restored, err := store.Put(ctx, "html", "image/png", data)
	if err != nil {
		t.Fatalf("restore Put: %v", err)
	}
	got, err := store.Get(ctx, restored)
	if err != nil || string(got) != string(data) {
		t.Fatalf("restored read = %q, %v; want %q, nil", got, err, data)
	}
}

func TestPutFailedWriteDoesNotConsumeRunLimits(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "2")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "2")

	t.Run("value write", func(t *testing.T) {
		backend := newFakeStore()
		ctx := taskContext("task-1")
		store := New(backend, "task-1")
		backend.failSet = true
		if _, err := store.Put(ctx, "html", "image/png", []byte("a")); err == nil {
			t.Fatal("Put succeeded when the value write failed")
		}
		backend.failSet = false
		for _, image := range []string{"a", "b"} {
			if _, err := store.Put(ctx, "html", "image/png", []byte(image)); err != nil {
				t.Fatalf("image %q after recovered write: %v", image, err)
			}
		}
	})

	t.Run("manifest track", func(t *testing.T) {
		backend := newFakeStore()
		ctx := taskContext("task-1")
		store := New(backend, "task-1")
		backend.expireFails = true
		if _, err := store.Put(ctx, "html", "image/png", []byte("a")); err == nil {
			t.Fatal("Put succeeded when the manifest TTL failed")
		}
		if len(backend.kv) != 0 {
			t.Fatalf("untracked media entries = %v, want rollback", backend.kv)
		}
		backend.expireFails = false
		for _, image := range []string{"a", "b"} {
			if _, err := store.Put(ctx, "html", "image/png", []byte(image)); err != nil {
				t.Fatalf("image %q after recovered track: %v", image, err)
			}
		}
	})
}
