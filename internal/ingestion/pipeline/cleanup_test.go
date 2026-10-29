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

package pipeline

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/mediacache"
)

type cleanupFakeStore struct {
	kv       map[string]string
	sets     map[string]map[string]bool
	ttl      map[string]time.Duration
	failRead map[string]bool
}

func newCleanupFakeStore() *cleanupFakeStore {
	return &cleanupFakeStore{
		kv:       map[string]string{},
		sets:     map[string]map[string]bool{},
		ttl:      map[string]time.Duration{},
		failRead: map[string]bool{},
	}
}

func (f *cleanupFakeStore) Get(_ context.Context, key string) (string, error) {
	return f.kv[key], nil
}

func (f *cleanupFakeStore) Set(_ context.Context, key, value string, exp time.Duration) bool {
	f.kv[key] = value
	f.ttl[key] = exp
	return true
}

func (f *cleanupFakeStore) SAdd(_ context.Context, key, member string) bool {
	if f.sets[key] == nil {
		f.sets[key] = map[string]bool{}
	}
	f.sets[key][member] = true
	return true
}

func (f *cleanupFakeStore) SMembers(_ context.Context, key string) ([]string, error) {
	if f.failRead[key] {
		return nil, errors.New("kvrocks unavailable")
	}
	out := make([]string, 0, len(f.sets[key]))
	for member := range f.sets[key] {
		out = append(out, member)
	}
	sort.Strings(out)
	return out, nil
}

func (f *cleanupFakeStore) Delete(_ context.Context, key string) bool {
	delete(f.kv, key)
	delete(f.sets, key)
	return true
}

func (f *cleanupFakeStore) Expire(_ context.Context, key string, exp time.Duration) bool {
	f.ttl[key] = exp
	return true
}

func cleanupTaskContext(taskID string) context.Context {
	ctx := runtime.WithState(context.Background(), runtime.NewCanvasState("", ""))
	globals.SetTaskID(ctx, taskID)
	return ctx
}

func seedTaskManifests(t *testing.T, backend *cleanupFakeStore, taskID string) {
	t.Helper()
	ctx := cleanupTaskContext(taskID)
	chunkcache.Set(ctx, backend, "kc:emb:chunk-1", "model result")
	if _, err := mediacache.New(backend, taskID).Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatalf("seed media cache: %v", err)
	}
	if len(backend.kv) != 2 || len(backend.sets) != 2 {
		t.Fatalf("seeded state = %d values, %d manifests, want 2 and 2", len(backend.kv), len(backend.sets))
	}
}

func TestPurgeTaskStateWithBackendRemovesChunkAndMediaManifests(t *testing.T) {
	backend := newCleanupFakeStore()
	ctx := cleanupTaskContext("task-1")
	seedTaskManifests(t, backend, "task-1")

	if err := purgeTaskStateWithBackend(ctx, backend, "task-1"); err != nil {
		t.Fatalf("purgeTaskStateWithBackend: %v", err)
	}
	if len(backend.kv) != 0 || len(backend.sets) != 0 {
		t.Fatalf("state after purge = %v values, %v manifests, want empty", backend.kv, backend.sets)
	}
}

func TestPurgeTaskStateWithBackendJoinsErrorsAndStillPurgesMedia(t *testing.T) {
	backend := newCleanupFakeStore()
	ctx := cleanupTaskContext("task-1")
	seedTaskManifests(t, backend, "task-1")
	backend.failRead["kc:manifest:task-1"] = true

	err := purgeTaskStateWithBackend(ctx, backend, "task-1")
	if err == nil || !strings.Contains(err.Error(), "chunk cache manifest") {
		t.Fatalf("error = %v, want the chunk manifest failure", err)
	}
	for key := range backend.kv {
		if strings.HasPrefix(key, "mc:") {
			t.Fatalf("media value %s survived a failed chunk purge", key)
		}
	}
	for key := range backend.sets {
		if strings.HasPrefix(key, "mc:") {
			t.Fatalf("media manifest %s survived a failed chunk purge", key)
		}
	}
	if !backend.sets["kc:manifest:task-1"]["kc:emb:chunk-1"] {
		t.Fatal("failed chunk purge lost the chunk manifest it must retry")
	}
}

func TestPurgeTaskStateWithBackendRejectsEmptyTaskID(t *testing.T) {
	backend := newCleanupFakeStore()
	if err := purgeTaskStateWithBackend(context.Background(), backend, ""); err == nil {
		t.Fatal("purge accepted an empty task id")
	}
	if len(backend.kv) != 0 || len(backend.sets) != 0 {
		t.Fatal("empty task id touched backend state")
	}
}
