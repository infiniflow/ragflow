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

package globals

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestTaskID_EmptyWithoutState asserts TaskID degrades to "" when no
// CanvasState is attached (headless unit tests) or the key was never seeded.
// Callers use "" as "no task scope" and must skip per-task bookkeeping.
func TestTaskID_EmptyWithoutState(t *testing.T) {
	if got := TaskID(context.Background()); got != "" {
		t.Errorf("TaskID(bare ctx) = %q, want \"\"", got)
	}
	ctx := runtime.WithState(context.Background(), &runtime.CanvasState{Globals: map[string]any{}})
	if got := TaskID(ctx); got != "" {
		t.Errorf("TaskID(empty globals) = %q, want \"\"", got)
	}
}

// TestSetTaskID_RoundTrips asserts the pipeline-side seeder and the
// component-side reader agree on the storage slot.
func TestSetTaskID_RoundTrips(t *testing.T) {
	st := &runtime.CanvasState{Globals: map[string]any{}}
	ctx := runtime.WithState(context.Background(), st)
	SetTaskID(ctx, "task-42")
	if got := TaskID(ctx); got != "task-42" {
		t.Errorf("TaskID = %q, want \"task-42\"", got)
	}
}

// TestSetTaskID_NoStateIsNoop asserts seeding without a CanvasState does not
// panic — headless component tests invoke stages with a bare context.
func TestSetTaskID_NoStateIsNoop(t *testing.T) {
	SetTaskID(context.Background(), "task-42")
}

// TestTaskIDKey_NotAWhitelistedMetadataKey asserts the task id is seeded
// explicitly by the pipeline and is NOT part of GlobalMetadataKeys. Being on
// that whitelist would let any component output (or run input) carrying a
// "task_id" field overwrite the run's real task id, which would silently
// redirect per-task cache bookkeeping to the wrong task.
func TestTaskIDKey_NotAWhitelistedMetadataKey(t *testing.T) {
	for _, k := range GlobalMetadataKeys {
		if k == taskIDKey {
			t.Fatalf("GlobalMetadataKeys must not contain %q", taskIDKey)
		}
	}
	st := &runtime.CanvasState{Globals: map[string]any{}}
	ctx := runtime.WithState(context.Background(), st)
	SetTaskID(ctx, "task-real")
	SeedIngestionGlobals(ctx, map[string]any{taskIDKey: "task-spoofed"})
	if got := TaskID(ctx); got != "task-real" {
		t.Errorf("TaskID after hostile seed = %q, want \"task-real\"", got)
	}
}
