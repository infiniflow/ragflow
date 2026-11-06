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

// TestPurgeTaskUsesSingleBatchDelete is the adversarial test for the E2 batch
// optimization: it asserts PurgeTask reclaims every entry key AND the manifest
// in exactly ONE Store.DeleteMany call and never falls back to per-key Delete.
// A naive loop-based implementation that merely "deletes everything" would pass
// a happy-path test, so we pin the call shape: deleteManyCalls must be 1 and the
// per-key Delete counter (deleted) must stay empty.
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

	if f.deleteManyCalls != 1 {
		t.Errorf("DeleteMany calls = %d, want 1 (single batch)", f.deleteManyCalls)
	}
	if len(f.deleted) != 0 {
		t.Errorf("per-key Delete still called for %v; batch path expected", f.deleted)
	}
	if len(f.kv) != 0 {
		t.Errorf("kv not fully reclaimed: %v", f.kv)
	}
	if _, ok := f.sets[manifestKey("task-1")]; ok {
		t.Errorf("manifest %q not removed", manifestKey("task-1"))
	}

	// The single batch must have carried the manifest plus every entry key.
	want := make(map[string]bool, len(entryKeys)+1)
	want[manifestKey("task-1")] = true
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

// TestPurgeTaskBatchPropagatesFailure asserts the batch is all-or-nothing: when
// one key in the batch cannot be deleted, PurgeTask returns an error and leaves
// the manifest intact rather than partially reclaiming.
func TestPurgeTaskBatchPropagatesFailure(t *testing.T) {
	f := newFakeStore()
	ctx := taskCtx("task-1")
	key := "kc:extractor:keywords:a"
	Set(ctx, f, key, "result")
	f.deleteFails = map[string]bool{key: true}

	if err := PurgeTask(context.Background(), f, "task-1"); err == nil {
		t.Fatal("PurgeTask error = nil, want failure when a batch key cannot be deleted")
	}
	if _, ok := f.sets[manifestKey("task-1")]; !ok {
		t.Fatal("manifest must survive a failed batch (all-or-nothing)")
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
