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
	"errors"
	"strings"
	"testing"
)

// TestRecreateSkillIndexStopsOnDropFailure pins that a failed drop cannot be
// followed by creation on the stale table. The prior path only logged a
// warning, then created and reported success with the old dimension intact.
func TestRecreateSkillIndexStopsOnDropFailure(t *testing.T) {
	e := &chunkStoreCountingEngine{exists: true, dropErr: errors.New("drop failed")}
	err := recreateSkillIndex(t.Context(), &SkillIndexerService{}, "tenant-1", "space-1", "embd-1", e, 128)
	if err == nil || !strings.Contains(err.Error(), "drop failed") {
		t.Fatalf("recreate error = %v, want drop failure", err)
	}
	if e.createCalls != 0 {
		t.Fatalf("CreateChunkStore called %d times after a failed drop, want 0", e.createCalls)
	}
}

// TestRecreateSkillIndexProceedsWhenAbsent guards the normal path: no table
// means no drop, and creation still happens.
func TestRecreateSkillIndexProceedsWhenAbsent(t *testing.T) {
	e := &chunkStoreCountingEngine{exists: false}
	if err := recreateSkillIndex(t.Context(), &SkillIndexerService{}, "tenant-1", "space-1", "embd-1", e, 256); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if e.dropCalls != 0 || e.createCalls != 1 {
		t.Fatalf("drop/create = %d/%d, want 0/1", e.dropCalls, e.createCalls)
	}
	if len(e.createdDims) != 1 || e.createdDims[0] != 256 {
		t.Fatalf("created dimensions = %v, want [256]", e.createdDims)
	}
}

// TestRecreateSkillIndexDropsBeforeCreating pins the replace path: an existing
// table is dropped exactly once before the new one is created.
func TestRecreateSkillIndexDropsBeforeCreating(t *testing.T) {
	e := &chunkStoreCountingEngine{exists: true}
	if err := recreateSkillIndex(t.Context(), &SkillIndexerService{}, "tenant-1", "space-1", "embd-1", e, 512); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if e.dropCalls != 1 || e.createCalls != 1 {
		t.Fatalf("drop/create = %d/%d, want 1/1", e.dropCalls, e.createCalls)
	}
}
