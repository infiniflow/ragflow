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

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ragflow/internal/entity"
)

type fakeClaimEntry struct {
	owner     string
	expiresAt time.Time
}

// fakeTaskClaimStore is the in-memory taskClaimStore used by unit tests. It
// mirrors the owner-checked SET NX / PEXPIRE / DEL semantics of Kvrocks.
type fakeTaskClaimStore struct {
	mu      sync.Mutex
	entries map[string]fakeClaimEntry

	setNXErr  error
	expireErr error
}

func newFakeTaskClaimStore() *fakeTaskClaimStore {
	return &fakeTaskClaimStore{entries: make(map[string]fakeClaimEntry)}
}

func (f *fakeTaskClaimStore) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setNXErr != nil {
		return false, f.setNXErr
	}
	if entry, ok := f.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		return false, nil
	}
	f.entries[key] = fakeClaimEntry{owner: value, expiresAt: time.Now().Add(ttl)}
	return true, nil
}

func (f *fakeTaskClaimStore) ExpireIfEqual(_ context.Context, key, expectedValue string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.expireErr != nil {
		return false, f.expireErr
	}
	entry, ok := f.entries[key]
	if !ok || !time.Now().Before(entry.expiresAt) || entry.owner != expectedValue {
		return false, nil
	}
	entry.expiresAt = time.Now().Add(ttl)
	f.entries[key] = entry
	return true, nil
}

func (f *fakeTaskClaimStore) DeleteIfEqual(_ context.Context, key, expectedValue string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[key]
	if !ok || !time.Now().Before(entry.expiresAt) || entry.owner != expectedValue {
		return false, nil
	}
	delete(f.entries, key)
	return true, nil
}

func (f *fakeTaskClaimStore) owner(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.entries[key].owner
}

func (f *fakeTaskClaimStore) setOwner(key, owner string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[key] = fakeClaimEntry{owner: owner, expiresAt: time.Now().Add(time.Hour)}
}

func newClaimTestIngestor(name string, store taskClaimStore, ttl, renew time.Duration) *Ingestor {
	return &Ingestor{
		id:                     name,
		ctx:                    context.Background(),
		taskClaims:             store,
		taskClaimTTL:           ttl,
		taskClaimRenewInterval: renew,
	}
}

func TestAcquireTaskClaimExcludesAcrossIngestors(t *testing.T) {
	store := newFakeTaskClaimStore()
	first := newClaimTestIngestor("first", store, time.Minute, time.Hour)
	second := newClaimTestIngestor("second", store, time.Minute, time.Hour)

	firstClaim, ok, err := first.acquireTaskClaim("task-1")
	if err != nil || !ok {
		t.Fatalf("first ingestor claim should succeed: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { first.releaseTask(firstClaim) })

	if _, ok, err := second.acquireTaskClaim("task-1"); err != nil || ok {
		t.Fatalf("second ingestor claim should fail while the first owns the key: ok=%v err=%v", ok, err)
	}

	first.releaseTask(firstClaim)
	secondClaim, ok, err := second.acquireTaskClaim("task-1")
	if err != nil || !ok {
		t.Fatalf("second ingestor claim should succeed after release: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { second.releaseTask(secondClaim) })
}

func TestAcquireTaskClaimFailsClosedOnStoreError(t *testing.T) {
	store := newFakeTaskClaimStore()
	store.setNXErr = errors.New("kvrocks unavailable")
	ingestor := newClaimTestIngestor("fail-closed", store, time.Minute, time.Hour)

	claim, ok, err := ingestor.acquireTaskClaim("task-1")
	if err == nil {
		t.Fatal("acquireTaskClaim error = nil, want kvrocks failure")
	}
	if ok || claim != nil {
		t.Fatalf("acquireTaskClaim = (%v, %v), want (nil, false) on store failure", claim, ok)
	}
}

func TestAcquireTaskClaimFailsClosedWithoutStore(t *testing.T) {
	ingestor := newClaimTestIngestor("no-store", nil, time.Minute, time.Hour)

	claim, ok, err := ingestor.acquireTaskClaim("task-1")
	if err == nil {
		t.Fatal("acquireTaskClaim error = nil, want missing store error")
	}
	if ok || claim != nil {
		t.Fatalf("acquireTaskClaim = (%v, %v), want (nil, false) without a store", claim, ok)
	}
}

func TestTaskClaimWatchCancelsWhenTakenOver(t *testing.T) {
	store := newFakeTaskClaimStore()
	ingestor := newClaimTestIngestor("taken", store, time.Minute, 20*time.Millisecond)

	claim, ok, err := ingestor.acquireTaskClaim("task-1")
	if err != nil || !ok {
		t.Fatalf("claim should succeed: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { ingestor.releaseTask(claim) })

	store.setOwner(taskClaimKey("task-1"), "another-ingestor")
	select {
	case <-claim.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("claim watcher did not cancel the claim after takeover")
	}
	if !claim.Lost() {
		t.Fatal("claim.Lost() = false after takeover")
	}
}

func TestTaskClaimWatchCancelsWhenRenewalKeepsFailing(t *testing.T) {
	store := newFakeTaskClaimStore()
	ingestor := newClaimTestIngestor("renew-fail", store, 90*time.Millisecond, 10*time.Millisecond)

	claim, ok, err := ingestor.acquireTaskClaim("task-1")
	if err != nil || !ok {
		t.Fatalf("claim should succeed: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { ingestor.releaseTask(claim) })

	store.expireErr = errors.New("kvrocks unavailable")
	select {
	case <-claim.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("claim watcher did not fail closed while renewals kept failing")
	}
	if !claim.Lost() {
		t.Fatal("claim.Lost() = false after renewal failures exceeded the safety window")
	}
}

func TestReleaseTaskClaimKeepsNewOwner(t *testing.T) {
	store := newFakeTaskClaimStore()
	ingestor := newClaimTestIngestor("stale", store, time.Minute, time.Hour)

	claim, ok, err := ingestor.acquireTaskClaim("task-1")
	if err != nil || !ok {
		t.Fatalf("claim should succeed: ok=%v err=%v", ok, err)
	}
	store.setOwner(taskClaimKey("task-1"), "another-ingestor")
	ingestor.releaseTask(claim)

	if owner := store.owner(taskClaimKey("task-1")); owner != "another-ingestor" {
		t.Fatalf("claim owner = %q, want another-ingestor", owner)
	}
}

func TestRunTaskWithLostClaimSkipsPipelineAndTerminalWrite(t *testing.T) {
	ingestor := newClaimTestIngestor("lost", nil, 0, 0)
	ranPipeline := false
	ingestor.runDocumentTask = func(context.Context, *entity.IngestionTask) error {
		ranPipeline = true
		return nil
	}

	claim := &taskClaim{}
	claim.lost.Store(true)
	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{ID: "task-1"}, claim)

	if terminal {
		t.Fatal("runTask terminal = true, want false after claim loss")
	}
	if ranPipeline {
		t.Fatal("runTask ran the pipeline after claim loss")
	}
}
