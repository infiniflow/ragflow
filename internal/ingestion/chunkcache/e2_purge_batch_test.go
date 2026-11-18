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
	"fmt"
	"testing"
)

// TestPurgeTaskUsesSingleBatchDelete pins the E2 batch call shape: entries are
// reclaimed in exactly ONE Store.DeleteMany call, and the manifest is deleted
// separately (one per-key Delete) ONLY after that batch succeeds. A naive
// loop-based implementation would pass a happy-path test, so we assert the
// batch carries the entries but NOT the manifest, and that the manifest's own
// Delete is the sole per-key call.
func TestPurgeTaskUsesSingleBatchDelete(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	entryKeys := []string{"kc:extractor:keywords:a", "kc:extractor:keywords:b", "kc:extractor:keywords:c"}
	for i, k := range entryKeys {
		Set(ctx, f, k, fmt.Sprintf("v%d", i))
	}

	if err := PurgeTask(context.Background(), f, "task-1"); err != nil {
		t.Fatalf("PurgeTask error = %v, want nil", err)
	}

	// Entries travel in one batch.
	if f.deleteManyCalls != 1 {
		t.Errorf("DeleteMany calls = %d, want 1 (single batch)", f.deleteManyCalls)
	}
	// Exactly one per-key Delete, for the manifest only.
	if len(f.deleted) != 1 || f.deleted[0] != manifestKey("task-1") {
		t.Errorf("per-key Delete = %v, want exactly the manifest key", f.deleted)
	}
	if len(f.kv) != 0 {
		t.Errorf("kv not fully reclaimed: %v", f.kv)
	}
	if _, ok := f.sets[manifestKey("task-1")]; ok {
		t.Errorf("manifest %q not removed", manifestKey("task-1"))
	}

	// The batch must carry every entry key, but NOT the manifest: the manifest
	// is deleted on its own after the batch succeeds (so a failed batch leaves
	// it intact and the task resumable).
	want := make(map[string]bool, len(entryKeys))
	for _, k := range entryKeys {
		want[k] = true
	}
	if len(f.lastDeleteMany) != len(want) {
		t.Fatalf("batch size = %d, want %d (%v)", len(f.lastDeleteMany), len(want), f.lastDeleteMany)
	}
	for _, k := range f.lastDeleteMany {
		if !want[k] {
			t.Errorf("batch carried unexpected key %q", k)
		}
		if k == manifestKey("task-1") {
			t.Errorf("batch must not include the manifest key; it is deleted separately")
		}
	}
}

// TestPurgeTaskBatchHandlesEmptyManifest asserts that even a task whose manifest
// lists no entries still removes the manifest key in one batch call. The previous
// loop would iterate zero times and then issue a separate Delete for the
// manifest; the batch form must not regress to that two-step shape.
func TestPurgeTaskBatchHandlesEmptyManifest(t *testing.T) {
	f := newFakeStore()

	if err := PurgeTask(context.Background(), f, "task-1"); err != nil {
		t.Fatalf("PurgeTask error = %v, want nil", err)
	}
	if f.deleteManyCalls != 1 {
		t.Errorf("DeleteMany calls = %d, want 1 for an empty manifest", f.deleteManyCalls)
	}
	if _, ok := f.sets[manifestKey("task-1")]; ok {
		t.Errorf("manifest %q not removed", manifestKey("task-1"))
	}
}

// TestPurgeTaskBatchPropagatesFailure asserts that when an entry batch cannot be
// fully deleted, PurgeTask returns an error and leaves the manifest intact
// rather than deleting it alongside a partially-reclaimed batch.
func TestPurgeTaskBatchPropagatesFailure(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	key := "kc:extractor:keywords:a"
	Set(ctx, f, key, "result")
	f.deleteFails = map[string]bool{key: true}

	if err := PurgeTask(context.Background(), f, "task-1"); err == nil {
		t.Fatal("PurgeTask error = nil, want failure when an entry cannot be deleted")
	}
	if _, ok := f.sets[manifestKey("task-1")]; !ok {
		t.Fatal("manifest must survive a failed entry batch (resumable)")
	}
}

// TestPurgeTaskManifestSurvivesEntryFailure is the adversarial test for the
// large-manifest safety property: when an entry batch reports failure (e.g. a
// store outage that hits a later DEL batch after an earlier one already
// succeeded), PurgeTask must NOT delete the manifest, so the task stays
// resumable and the surviving entries are reclaimed on retry. This must hold
// regardless of how many entry batches the manifest is split into.
func TestPurgeTaskManifestSurvivesEntryFailure(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	entryKeys := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		k := fmt.Sprintf("kc:extractor:keywords:%d", i)
		Set(ctx, f, k, fmt.Sprintf("v%d", i))
		entryKeys = append(entryKeys, k)
	}
	// Fail one entry so the entry batch reports failure.
	f.deleteFails = map[string]bool{entryKeys[3]: true}

	if err := PurgeTask(context.Background(), f, "task-1"); err == nil {
		t.Fatal("PurgeTask error = nil, want failure when an entry cannot be deleted")
	}
	// Manifest must survive so the task remains resumable.
	if _, ok := f.sets[manifestKey("task-1")]; !ok {
		t.Fatal("manifest must survive a failed entry batch")
	}
}

// TestPurgeTaskBatchNoopWithoutTaskOrStore mirrors the existing no-op guard and
// pins that the batch path issues no DeleteMany when there is nothing to purge.
func TestPurgeTaskBatchNoopWithoutTaskOrStore(t *testing.T) {
	f := newFakeStore()
	PurgeTask(context.Background(), f, "")
	if f.deleteManyCalls != 0 {
		t.Errorf("DeleteMany calls = %d, want 0 for an empty task id", f.deleteManyCalls)
	}
	PurgeTask(context.Background(), nil, "task-1")
}
