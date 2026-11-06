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
	"testing"
)

// TestPurgeTaskUsesSingleBatchDelete is the adversarial test for the E2 batch
// optimization in the media cache: every entry key, the media manifest, and the
// quota (digest/state) keys must be reclaimed in exactly ONE Store.DeleteMany
// call, with no per-key Delete fallback. We pin the call shape rather than only
// asserting "everything is gone", because a loop-based implementation would pass
// the happy path.
func TestPurgeTaskUsesSingleBatchDelete(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	if _, err := store.Put(ctx, "html", "image/png", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	if err := PurgeTask(ctx, backend, "task-1"); err != nil {
		t.Fatalf("PurgeTask error = %v, want nil", err)
	}

	if backend.deleteManyCalls != 1 {
		t.Errorf("DeleteMany calls = %d, want 1 (single batch)", backend.deleteManyCalls)
	}
	if len(backend.deleted) != 0 {
		t.Errorf("per-key Delete still called for %v; batch path expected", backend.deleted)
	}
	if len(backend.kv) != 0 || len(backend.sets) != 0 {
		t.Errorf("leftover after purge kv=%v sets=%v", backend.kv, backend.sets)
	}

	// The single batch must carry the media manifest plus the quota keys.
	digestKey, stateKey := quotaKeys("task-1")
	required := []string{manifestKey("task-1"), digestKey, stateKey}
	for _, k := range required {
		found := false
		for _, got := range backend.lastDeleteMany {
			if got == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("batch did not include required key %q (batch=%v)", k, backend.lastDeleteMany)
		}
	}
}

// TestPurgeTaskBatchHandlesEmptyManifest asserts an empty media manifest is still
// reclaimed (manifest + quota keys) in one batch call, not a zero-iteration loop
// followed by separate deletes.
func TestPurgeTaskBatchHandlesEmptyManifest(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")

	if err := PurgeTask(ctx, backend, "task-1"); err != nil {
		t.Fatalf("PurgeTask error = %v, want nil", err)
	}
	if backend.deleteManyCalls != 1 {
		t.Errorf("DeleteMany calls = %d, want 1 for an empty manifest", backend.deleteManyCalls)
	}
	digestKey, stateKey := quotaKeys("task-1")
	if _, ok := backend.kv[digestKey]; ok {
		t.Errorf("quota digest key %q not removed", digestKey)
	}
	if _, ok := backend.kv[stateKey]; ok {
		t.Errorf("quota state key %q not removed", stateKey)
	}
}

// TestPurgeTaskBatchPropagatesFailure asserts the media batch is all-or-nothing:
// a failing key leaves the manifest intact.
func TestPurgeTaskBatchPropagatesFailure(t *testing.T) {
	backend := newFakeStore()
	ctx := taskContext("task-1")
	store := New(backend, "task-1")
	ref, err := store.Put(ctx, "html", "image/png", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	key := store.key(ref.SHA256)
	backend.deleteFails = map[string]bool{key: true}

	if err := PurgeTask(ctx, backend, "task-1"); err == nil {
		t.Fatal("expected error when a batch key cannot be deleted")
	}
	if len(backend.sets[manifestKey("task-1")]) == 0 {
		t.Fatal("manifest must survive a failed batch (all-or-nothing)")
	}
}

// TestPurgeTaskBatchNoopWithoutTaskOrStore pins that the batch path issues no
// DeleteMany when there is nothing to purge.
func TestPurgeTaskBatchNoopWithoutTaskOrStore(t *testing.T) {
	backend := newFakeStore()
	PurgeTask(context.Background(), backend, "")
	if backend.deleteManyCalls != 0 {
		t.Errorf("DeleteMany calls = %d, want 0 for an empty task id", backend.deleteManyCalls)
	}
	PurgeTask(context.Background(), nil, "task-1")
}
